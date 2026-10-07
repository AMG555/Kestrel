package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"kestrel/internal/authctx"
	"kestrel/internal/mcp/builtin"
	"kestrel/internal/toolguard"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// MonitorStorage is the storage interface for monitoring data
type MonitorStorage interface {
	SaveToolExecution(exec *ToolExecution) error
	UpdateToolExecutionResult(id string, result *ToolResult) error
	LoadToolExecutions() ([]*ToolExecution, error)
	GetToolExecution(id string) (*ToolExecution, error)
	SaveToolStats(toolName string, stats *ToolStats) error
	LoadToolStats() (map[string]*ToolStats, error)
	UpdateToolStats(toolName string, totalCalls, successCalls, failedCalls int, lastCallTime *time.Time) error
}

// Server is the MCP server
type Server struct {
	tools                 map[string]ToolHandler
	toolDefs              map[string]Tool // tool definitions
	executions            map[string]*ToolExecution
	stats                 map[string]*ToolStats
	prompts               map[string]*Prompt   // prompt templates
	resources             map[string]*Resource // resources
	storage               MonitorStorage       // optional persistent storage
	mu                    sync.RWMutex
	logger                *zap.Logger
	maxExecutionsInMemory int // maximum execution records in memory
	sseClients            map[string]*sseClient
	runningCancels        map[string]context.CancelFunc
	runningCancelsMu      sync.Mutex
	abortUserNotes        map[string]string // user notes attached when terminating from the monitoring page, keyed by executionID
	// httpToolTimeoutMinutes syncs agent.tool_timeout_minutes for tools/call via POST /api/mcp (path not wrapped by Agent).
	// nil means not configured, uses default 30 minutes; pointing to 0 means unlimited; >0 is the timeout in minutes.
	httpToolTimeoutMinutes *int
	httpToolTimeoutMu      sync.RWMutex
	toolAuthorizer         func(context.Context, string, map[string]interface{}) error
	toolGuard              *toolguard.Manager
	executionService       *ExecutionService
	toolWaitTimeout        time.Duration
	toolResultMaxBytes     int
	spillRootDir           string
}

const defaultPartialOutputMaxBytes = 64 * 1024

// SetToolAuthorizer installs the common policy decision point for every
// user-attributed tool call, whether it originates from HTTP or an Agent.
func (s *Server) SetToolAuthorizer(authorizer func(context.Context, string, map[string]interface{}) error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.toolAuthorizer = authorizer
	s.mu.Unlock()
}

// SetToolGuard installs the runtime safety rules shared by HTTP and internal calls.
func (s *Server) SetToolGuard(guard *toolguard.Manager) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.toolGuard = guard
	s.mu.Unlock()
}

func (s *Server) checkToolGuard(toolName string, args map[string]interface{}) *ToolResult {
	s.mu.RLock()
	guard := s.toolGuard
	s.mu.RUnlock()
	return toolGuardBlockedResult(guard, toolName, args)
}

type sseClient struct {
	id   string
	send chan []byte
}

// ToolHandler is the tool handler function
type ToolHandler func(ctx context.Context, args map[string]interface{}) (*ToolResult, error)

func executionStatusAndMessage(err error) (status string, errMsg string) {
	if errors.Is(err, context.Canceled) {
		return "cancelled", "manually terminated (MCP monitoring)"
	}
	return "failed", err.Error()
}

// NewServer creates a new MCP server
func NewServer(logger *zap.Logger) *Server {
	return NewServerWithStorage(logger, nil)
}

// NewServerWithStorage creates a new MCP server with persistent storage
func NewServerWithStorage(logger *zap.Logger, storage MonitorStorage) *Server {
	s := &Server{
		tools:                 make(map[string]ToolHandler),
		toolDefs:              make(map[string]Tool),
		executions:            make(map[string]*ToolExecution),
		stats:                 make(map[string]*ToolStats),
		prompts:               make(map[string]*Prompt),
		resources:             make(map[string]*Resource),
		storage:               storage,
		logger:                logger,
		maxExecutionsInMemory: 1000, // default: retain at most 1000 execution records in memory
		sseClients:            make(map[string]*sseClient),
		runningCancels:        make(map[string]context.CancelFunc),
		abortUserNotes:        make(map[string]string),
		toolWaitTimeout:       60 * time.Second,
		toolResultMaxBytes:    DefaultToolResultMaxBytes,
	}
	s.executionService = NewExecutionService(storage, logger)

	// initialize default prompts and resources
	s.initDefaultPrompts()
	s.initDefaultResources()

	return s
}

func (s *Server) ConfigureToolResultMaxBytes(maxBytes int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.toolResultMaxBytes = maxBytes
	s.mu.Unlock()
	if s.executionService != nil {
		s.executionService.ConfigureToolResultMaxBytes(maxBytes)
	}
}

// ConfigureToolResultSpillRoot sets the local directory root used when oversized
// tool results are spilled (aligned with reduction_root_dir; empty → tmp/reduction).
func (s *Server) ConfigureToolResultSpillRoot(rootDir string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.spillRootDir = strings.TrimSpace(rootDir)
	s.mu.Unlock()
	if s.executionService != nil {
		s.executionService.ConfigureToolResultSpillRoot(rootDir)
	}
}

// ConfigureHTTPToolCallTimeoutFromAgentMinutes syncs agent.tool_timeout_minutes to tools/call invoked via HTTP POST /api/mcp.
// minutes<=0 means no hard deadline (consistent with config "0 = unlimited"); minutes>0 is the maximum wait time for the call.
// Before this is called, tools/call uses a default of 30 minutes (consistent with historical hard-coding).
func (s *Server) ConfigureHTTPToolCallTimeoutFromAgentMinutes(minutes int) {
	if s == nil {
		return
	}
	v := minutes
	if v < 0 {
		v = 0
	}
	s.httpToolTimeoutMu.Lock()
	defer s.httpToolTimeoutMu.Unlock()
	s.httpToolTimeoutMinutes = &v
}

func (s *Server) ConfigureToolWaitTimeoutSeconds(seconds int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if seconds <= 0 {
		s.toolWaitTimeout = 0
		return
	}
	s.toolWaitTimeout = time.Duration(seconds) * time.Second
}

func (s *Server) effectiveHTTPToolCallDeadline(parent context.Context) (context.Context, context.CancelFunc) {
	const defaultDur = 30 * time.Minute
	if parent == nil {
		parent = context.Background()
	}
	if s == nil {
		return context.WithTimeout(parent, defaultDur)
	}
	s.httpToolTimeoutMu.RLock()
	mPtr := s.httpToolTimeoutMinutes
	s.httpToolTimeoutMu.RUnlock()
	if mPtr == nil {
		return context.WithTimeout(parent, defaultDur)
	}
	if *mPtr <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, time.Duration(*mPtr)*time.Minute)
}

// RegisterTool registers a tool
func (s *Server) RegisterTool(tool Tool, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = handler
	s.toolDefs[tool.Name] = tool

	// automatically create resource documentation for tools
	resourceURI := fmt.Sprintf("tool://%s", tool.Name)
	s.resources[resourceURI] = &Resource{
		URI:         resourceURI,
		Name:        fmt.Sprintf("%s tool documentation", tool.Name),
		Description: tool.Description,
		MimeType:    "text/plain",
	}
}

// ClearTools clears all tools (used when reloading config)
func (s *Server) ClearTools() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// clear tools and tool definitions
	s.tools = make(map[string]ToolHandler)
	s.toolDefs = make(map[string]Tool)

	// clear tool-related resources (keep other resources)
	newResources := make(map[string]*Resource)
	for uri, resource := range s.resources {
		// keep non-tool resources
		if !strings.HasPrefix(uri, "tool://") {
			newResources[uri] = resource
		}
	}
	s.resources = newResources
}

// HandleHTTP handles HTTP requests
func (s *Server) HandleHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		s.handleSSE(w, r)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Official MCP SSE spec: a POST with sessionid sends a message to that SSE session; response is pushed back via the SSE stream
	if sessionID := r.URL.Query().Get("sessionid"); sessionID != "" {
		s.serveSSESessionMessage(w, r, sessionID)
		return
	}

	// Simple POST: request body is JSON-RPC; response is returned in the body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.sendError(w, nil, -32700, "Parse error", err.Error())
		return
	}

	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		s.sendError(w, nil, -32700, "Parse error", err.Error())
		return
	}

	response := s.handleMessage(r.Context(), &msg)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// serveSSESessionMessage handles a POST sent to an SSE session: reads the JSON-RPC request, processes it, and pushes the response via that session's SSE stream
func (s *Server) serveSSESessionMessage(w http.ResponseWriter, r *http.Request, sessionID string) {
	s.mu.RLock()
	client, exists := s.sseClients[sessionID]
	s.mu.RUnlock()
	if !exists || client == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		http.Error(w, "failed to parse body", http.StatusBadRequest)
		return
	}

	response := s.handleMessage(r.Context(), &msg)
	if response == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	respBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	select {
	case client.send <- respBytes:
		w.WriteHeader(http.StatusAccepted)
	default:
		http.Error(w, "session send buffer full", http.StatusServiceUnavailable)
	}
}

// handleSSE handles SSE connections, compatible with the official MCP 2024-11-05 SSE spec:
// 1. The first event must be event: endpoint, with data being the URL the client POSTs messages to (including sessionid)
// 2. Subsequent events are event: message, with data being JSON-RPC responses
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	sessionID := uuid.New().String()
	client := &sseClient{
		id:   sessionID,
		send: make(chan []byte, 32),
	}

	s.addSSEClient(client)
	defer s.removeSSEClient(client.id)

	// Official spec: the first event is endpoint, data is the message endpoint URL (the client will POST requests to this URL)
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if r.URL.Scheme != "" {
		scheme = r.URL.Scheme
	}
	endpointURL := fmt.Sprintf("%s://%s%s?sessionid=%s", scheme, r.Host, r.URL.Path, sessionID)
	fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", endpointURL)
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-client.send:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// addSSEClient registers an SSE client
func (s *Server) addSSEClient(client *sseClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sseClients[client.id] = client
}

// removeSSEClient removes an SSE client
func (s *Server) removeSSEClient(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if client, exists := s.sseClients[id]; exists {
		close(client.send)
		delete(s.sseClients, id)
	}
}

// handleMessage processes an MCP message
func (s *Server) handleMessage(ctx context.Context, msg *Message) *Message {
	// check if it is a notification — notifications have no id field and require no response
	isNotification := msg.ID.Value() == nil || msg.ID.String() == ""

	// if not a notification and ID is empty, generate a new UUID
	if !isNotification && msg.ID.String() == "" {
		msg.ID = MessageID{value: uuid.New().String()}
	}

	switch msg.Method {
	case "initialize":
		return s.handleInitialize(msg)
	case "tools/list":
		return s.handleListTools(msg)
	case "tools/call":
		return s.handleCallTool(ctx, msg)
	case "prompts/list":
		return s.handleListPrompts(msg)
	case "prompts/get":
		return s.handleGetPrompt(msg)
	case "resources/list":
		return s.handleListResources(msg)
	case "resources/read":
		return s.handleReadResource(msg)
	case "sampling/request":
		return s.handleSamplingRequest(msg)
	case "notifications/initialized":
		// notification type, no response needed
		s.logger.Debug("received initialized notification")
		return nil
	case "":
		// empty method name, may be a notification; do not return error
		if isNotification {
			s.logger.Debug("received notification message with empty method name")
			return nil
		}
		fallthrough
	default:
		// if it is a notification, do not return an error response
		if isNotification {
			s.logger.Debug("received unknown notification", zap.String("method", msg.Method))
			return nil
		}
		// for requests, return method not found error
		return &Message{
			ID:      msg.ID,
			Type:    MessageTypeError,
			Version: "2.0",
			Error:   &Error{Code: -32601, Message: "Method not found"},
		}
	}
}

// handleInitialize handles the initialize request
func (s *Server) handleInitialize(msg *Message) *Message {
	var req InitializeRequest
	if err := json.Unmarshal(msg.Params, &req); err != nil {
		return &Message{
			ID:      msg.ID,
			Type:    MessageTypeError,
			Version: "2.0",
			Error:   &Error{Code: -32602, Message: "Invalid params"},
		}
	}

	response := InitializeResponse{
		ProtocolVersion: ProtocolVersion,
		Capabilities: ServerCapabilities{
			Tools: map[string]interface{}{
				"listChanged": true,
			},
			Prompts: map[string]interface{}{
				"listChanged": true,
			},
			Resources: map[string]interface{}{
				"subscribe":   true,
				"listChanged": true,
			},
			Sampling: map[string]interface{}{},
		},
		ServerInfo: ServerInfo{
			Name:    "Kestrel",
			Version: "1.0.0",
		},
	}

	result, _ := json.Marshal(response)
	return &Message{
		ID:      msg.ID,
		Type:    MessageTypeResponse,
		Version: "2.0",
		Result:  result,
	}
}

// handleListTools handles the list tools request
func (s *Server) handleListTools(msg *Message) *Message {
	s.mu.RLock()
	tools := make([]Tool, 0, len(s.toolDefs))
	for _, tool := range s.toolDefs {
		tools = append(tools, tool)
	}
	s.mu.RUnlock()
	s.logger.Debug("tools/list request", zap.Int("tool_count", len(tools)))

	response := ListToolsResponse{Tools: tools}
	result, _ := json.Marshal(response)
	return &Message{
		ID:      msg.ID,
		Type:    MessageTypeResponse,
		Version: "2.0",
		Result:  result,
	}
}

// handleCallTool handles the call tool request
func (s *Server) handleCallTool(requestCtx context.Context, msg *Message) *Message {
	var req CallToolRequest
	if err := json.Unmarshal(msg.Params, &req); err != nil {
		return &Message{
			ID:      msg.ID,
			Type:    MessageTypeError,
			Version: "2.0",
			Error:   &Error{Code: -32602, Message: "Invalid params"},
		}
	}
	_, authenticated := authctx.PrincipalFromContext(requestCtx)
	s.mu.RLock()
	authorizer := s.toolAuthorizer
	s.mu.RUnlock()
	if authorizer != nil {
		if err := authorizer(requestCtx, req.Name, req.Arguments); err != nil {
			return &Message{ID: msg.ID, Type: MessageTypeError, Version: "2.0", Error: &Error{Code: -32003, Message: "Forbidden", Data: err.Error()}}
		}
	} else if authenticated {
		return &Message{ID: msg.ID, Type: MessageTypeError, Version: "2.0", Error: &Error{Code: -32003, Message: "Tool authorization policy is not configured"}}
	}

	executionID := uuid.New().String()
	execution := &ToolExecution{
		ID:        executionID,
		ToolName:  req.Name,
		Arguments: req.Arguments,
		Status:    "running",
		StartTime: time.Now(),
	}
	if principal, ok := authctx.PrincipalFromContext(requestCtx); ok {
		execution.OwnerUserID = principal.UserID
	}
	execution.ConversationID = MCPConversationIDFromContext(requestCtx)

	s.mu.Lock()
	s.executions[executionID] = execution
	// if in-memory execution records exceed the limit, clean up the oldest records
	s.cleanupOldExecutions()
	s.mu.Unlock()

	if s.storage != nil {
		if err := s.storage.SaveToolExecution(execution); err != nil {
			s.logger.Warn("save execution record to database failed", zap.Error(err))
		}
	}

	s.mu.RLock()
	handler, exists := s.tools[req.Name]
	s.mu.RUnlock()

	if !exists {
		execution.Status = "failed"
		execution.Error = "Tool not found"
		now := time.Now()
		execution.EndTime = &now
		execution.Duration = now.Sub(execution.StartTime)

		if s.storage != nil {
			if err := s.storage.SaveToolExecution(execution); err != nil {
				s.logger.Warn("save execution record to database failed", zap.Error(err))
			}
			s.mu.Lock()
			delete(s.executions, executionID)
			s.mu.Unlock()
		}

		s.updateStats(req.Name, ToolExecutionStatusFailed)

		return &Message{
			ID:      msg.ID,
			Type:    MessageTypeError,
			Version: "2.0",
			Error:   &Error{Code: -32601, Message: "Tool not found"},
		}
	}

	baseCtx, timeoutCancel := s.effectiveHTTPToolCallDeadline(requestCtx)
	defer timeoutCancel()
	execCtx, runCancel := context.WithCancel(baseCtx)
	s.registerRunningCancel(executionID, runCancel)
	defer func() {
		runCancel()
		s.unregisterRunningCancel(executionID)
	}()

	s.logger.Info("start executing tool",
		zap.String("toolName", req.Name),
		zap.Any("arguments", req.Arguments),
	)

	result := s.checkToolGuard(req.Name, req.Arguments)
	var err error
	if result == nil {
		result, err = handler(execCtx, req.Arguments)
	}
	cancelledWithUserNote := s.applyAbortUserNoteToCancelledToolResult(executionID, &result, &err)
	now := time.Now()
	var finalResult *ToolResult

	s.mu.Lock()
	execution.EndTime = &now
	execution.Duration = now.Sub(execution.StartTime)

	if err != nil {
		st, msg := executionStatusAndMessage(err)
		execution.Status = st
		execution.Error = msg
	} else if result != nil && result.Blocked {
		execution.Status = ToolExecutionStatusBlocked
		execution.Error = firstToolResultText(result, toolGuardBlockedPrefix)
		execution.Result = result
	} else if result != nil && result.IsError {
		if cancelledWithUserNote {
			execution.Status = "cancelled"
			execution.Error = ""
			execution.Result = result
		} else {
			execution.Status = "failed"
			if len(result.Content) > 0 {
				execution.Error = result.Content[0].Text
			} else {
				execution.Error = "tool execution returned error result"
			}
			execution.Result = result
		}
	} else {
		execution.Status = "completed"
		if result == nil {
			result = &ToolResult{
				Content: []Content{
					{Type: "text", Text: "tool execution completed but no result returned"},
				},
			}
		}
		execution.Result = result
	}

	finalResult = execution.Result
	s.mu.Unlock()

	if s.storage != nil {
		if err := s.storage.SaveToolExecution(execution); err != nil {
			s.logger.Warn("save execution record to database failed", zap.Error(err))
		}
	}

	s.updateStats(req.Name, execution.Status)

	if s.storage != nil {
		s.mu.Lock()
		delete(s.executions, executionID)
		s.mu.Unlock()
	}

	if err != nil {
		s.logger.Error("tool execution failed",
			zap.String("toolName", req.Name),
			zap.Error(err),
		)

		errText := fmt.Sprintf("tool execution failed: %v", err)
		if errors.Is(err, context.Canceled) {
			errText = "tool execution manually terminated (MCP monitor). Subsequent orchestration steps may continue."
		}
		errorResult, _ := json.Marshal(CallToolResponse{
			Content: []Content{
				{Type: "text", Text: errText},
			},
			IsError: true,
		})
		return &Message{
			ID:      msg.ID,
			Type:    MessageTypeResponse,
			Version: "2.0",
			Result:  errorResult,
		}
	}

	if finalResult != nil && finalResult.IsError {
		s.logger.Warn("tool execution returned error result",
			zap.String("toolName", req.Name),
		)

		errorResult, _ := json.Marshal(CallToolResponse{
			Content: finalResult.Content,
			IsError: true,
			Blocked: finalResult.Blocked,
			Meta:    toolResultProtocolMeta(finalResult),
		})
		return &Message{
			ID:      msg.ID,
			Type:    MessageTypeResponse,
			Version: "2.0",
			Result:  errorResult,
		}
	}

	if finalResult == nil {
		finalResult = &ToolResult{
			Content: []Content{
				{Type: "text", Text: "tool execution completed but no result returned"},
			},
		}
	}

	resultJSON, _ := json.Marshal(CallToolResponse{
		Content: finalResult.Content,
		IsError: false,
	})

	s.logger.Info("tool execution completed",
		zap.String("toolName", req.Name),
		zap.Bool("isError", finalResult.IsError),
	)

	return &Message{
		ID:      msg.ID,
		Type:    MessageTypeResponse,
		Version: "2.0",
		Result:  resultJSON,
	}
}

// updateStats updateStatistics info
func (s *Server) updateStats(toolName string, status string) {
	now := time.Now()
	if s.storage != nil {
		totalCalls := 1
		successCalls := 0
		failedCalls := 0
		if executionStatusCountsAsFailed(status) {
			failedCalls = 1
		} else if status == ToolExecutionStatusCompleted {
			successCalls = 1
		}
		if err := s.storage.UpdateToolStats(toolName, totalCalls, successCalls, failedCalls, &now); err != nil {
			s.logger.Warn("save statistics to database failed", zap.Error(err))
		}
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stats[toolName] == nil {
		s.stats[toolName] = &ToolStats{
			ToolName: toolName,
		}
	}

	stats := s.stats[toolName]
	stats.TotalCalls++
	stats.LastCallTime = &now

	if executionStatusCountsAsFailed(status) {
		stats.FailedCalls++
	} else if status == ToolExecutionStatusCompleted {
		stats.SuccessCalls++
	} else if status == ToolExecutionStatusBlocked {
		stats.BlockedCalls++
	}
}

// GetExecution returns an execution record (checks memory first, then database)
func (s *Server) GetExecution(id string) (*ToolExecution, bool) {
	if s.executionService != nil {
		if snap, err := s.executionService.Get(id); err == nil && snap != nil && snap.Execution != nil {
			return snap.Execution, true
		}
	}
	s.mu.RLock()
	exec, exists := s.executions[id]
	s.mu.RUnlock()

	if exists {
		return exec, true
	}

	if s.storage != nil {
		exec, err := s.storage.GetToolExecution(id)
		if err == nil {
			return exec, true
		}
	}

	return nil, false
}

// loadHistoricalData loads historical data from the database
func (s *Server) loadHistoricalData() {
	if s.storage == nil {
		return
	}

	// load historical execution records (most recent 1000)
	executions, err := s.storage.LoadToolExecutions()
	if err != nil {
		s.logger.Warn("load historical execution records failed", zap.Error(err))
	} else {
		s.mu.Lock()
		for _, exec := range executions {
			// only load up to maxExecutionsInMemory records to limit memory usage
			if len(s.executions) < s.maxExecutionsInMemory {
				s.executions[exec.ID] = exec
			} else {
				break
			}
		}
		s.mu.Unlock()
		s.logger.Info("loaded historical execution records", zap.Int("count", len(executions)))
	}

	// load historical statistics
	stats, err := s.storage.LoadToolStats()
	if err != nil {
		s.logger.Warn("load historical statistics failed", zap.Error(err))
	} else {
		s.mu.Lock()
		for k, v := range stats {
			s.stats[k] = v
		}
		s.mu.Unlock()
		s.logger.Info("loaded historical statistics", zap.Int("count", len(stats)))
	}
}

// GetAllExecutions returns all execution records (merges memory and database)
func (s *Server) GetAllExecutions() []*ToolExecution {
	if s.storage != nil {
		dbExecutions, err := s.storage.LoadToolExecutions()
		if err == nil {
			execMap := make(map[string]*ToolExecution)
			for _, exec := range dbExecutions {
				if _, exists := execMap[exec.ID]; !exists {
					execMap[exec.ID] = exec
				}
			}

			s.mu.RLock()
			for id, exec := range s.executions {
				if _, exists := execMap[id]; !exists {
					execMap[id] = exec
				}
			}
			s.mu.RUnlock()

			result := make([]*ToolExecution, 0, len(execMap))
			for _, exec := range execMap {
				result = append(result, exec)
			}
			return result
		} else {
			s.logger.Warn("failed to load execution records from database", zap.Error(err))
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	memExecutions := make([]*ToolExecution, 0, len(s.executions))
	for _, exec := range s.executions {
		memExecutions = append(memExecutions, exec)
	}
	return memExecutions
}

// GetStats retrieves statistics (merges memory and database)
func (s *Server) GetStats() map[string]*ToolStats {
	if s.storage != nil {
		dbStats, err := s.storage.LoadToolStats()
		if err == nil {
			return dbStats
		}
		s.logger.Warn("load statistics from database failed", zap.Error(err))
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	memStats := make(map[string]*ToolStats)
	for k, v := range s.stats {
		statCopy := *v
		memStats[k] = &statCopy
	}

	return memStats
}

// GetAllTools returns all registered tools (used by the agent to dynamically retrieve the tool list)
func (s *Server) GetAllTools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tools := make([]Tool, 0, len(s.toolDefs))
	for _, tool := range s.toolDefs {
		tools = append(tools, tool)
	}
	return tools
}

// CallTool directly calls a tool (used for internal calls)
func (s *Server) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (*ToolResult, string, error) {
	if s.executionService == nil {
		s.executionService = NewExecutionService(s.storage, s.logger)
		s.executionService.ConfigureToolResultMaxBytes(s.toolResultMaxBytes)
		s.executionService.ConfigureToolResultSpillRoot(s.spillRootDir)
	}
	var ownerUserID string
	if principal, ok := authctx.PrincipalFromContext(ctx); ok {
		ownerUserID = principal.UserID
	}
	handle, err := s.executionService.Submit(ctx, ExecutionRequest{
		ToolName:       toolName,
		Arguments:      args,
		ConversationID: MCPConversationIDFromContext(ctx),
		OwnerUserID:    ownerUserID,
		Run: func(runCtx context.Context) (*ToolResult, error) {
			_, authenticated := authctx.PrincipalFromContext(runCtx)
			s.mu.RLock()
			authorizer := s.toolAuthorizer
			handler, exists := s.tools[toolName]
			s.mu.RUnlock()
			if authorizer != nil {
				if err := authorizer(runCtx, toolName, args); err != nil {
					return nil, fmt.Errorf("tool authorization denied: %w", err)
				}
			} else if authenticated {
				return nil, errors.New("tool authorization policy is not configured")
			}
			if !exists {
				return nil, fmt.Errorf("tool %s not found", toolName)
			}
			if blocked := s.checkToolGuard(toolName, args); blocked != nil {
				return blocked, nil
			}
			return handler(runCtx, args)
		},
		OnDone: func(exec *ToolExecution) {
			if exec != nil {
				s.updateStats(toolName, exec.Status)
			}
		},
	})
	if err != nil {
		return nil, "", err
	}

	s.mu.RLock()
	waitTimeout := s.toolWaitTimeout
	s.mu.RUnlock()
	if isExecutionControlTool(toolName) {
		waitTimeout = 0
	}
	snapshot, waitErr := s.executionService.Wait(ctx, handle.ID, waitTimeout)
	if errors.Is(waitErr, ErrExecutionWaitTimeout) {
		return internalMCPWaitTimeoutResult(snapshot, waitTimeout), handle.ID, nil
	}
	if waitErr != nil {
		return nil, handle.ID, waitErr
	}
	if snapshot == nil || snapshot.Execution == nil {
		return &ToolResult{Content: []Content{{Type: "text", Text: "tool execution completed but no execution snapshot returned"}}, IsError: true}, handle.ID, nil
	}
	if snapshot.Execution.Result != nil {
		return snapshot.Execution.Result, handle.ID, nil
	}
	if snapshot.Execution.Error != "" {
		return nil, handle.ID, errors.New(snapshot.Execution.Error)
	}
	return &ToolResult{Content: []Content{{Type: "text", Text: "tool execution completed but no result returned"}}, IsError: false}, handle.ID, nil
}

func internalMCPWaitTimeoutResult(snapshot *ExecutionSnapshot, waitTimeout time.Duration) *ToolResult {
	execID := ""
	status := ToolExecutionStatusRunning
	toolName := ""
	elapsed := time.Duration(0)
	if snapshot != nil && snapshot.Execution != nil {
		execID = snapshot.Execution.ID
		status = snapshot.Execution.Status
		toolName = snapshot.Execution.ToolName
		elapsed = time.Since(snapshot.Execution.StartTime).Round(time.Second)
	}
	waitText := "unbounded"
	if waitTimeout > 0 {
		waitText = waitTimeout.Round(time.Second).String()
	}
	msg := fmt.Sprintf(`Tool submitted to background execution, but this wait has reached the limit.

execution_id: %s
tool: %s
status: %s
wait_timeout: %s
elapsed: %s

You may continue reasoning, switch to another tool, or call wait_tool_execution to keep waiting for this execution_id; you may also call cancel_tool_execution to cancel.`, execID, toolName, status, waitText, elapsed)
	return &ToolResult{Content: []Content{{Type: "text", Text: msg}}, IsError: true}
}

func isExecutionControlTool(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case builtin.ToolGetToolExecution, builtin.ToolWaitToolExecution, builtin.ToolCancelToolExecution:
		return true
	default:
		return false
	}
}

// BeginToolExecution creates a running-status execution record, used by Eino and other non-CallTool paths to persist the record when a tool starts.
func (s *Server) BeginToolExecution(ctx context.Context, toolName string, args map[string]interface{}) string {
	if s == nil {
		return ""
	}
	if args == nil {
		args = map[string]interface{}{}
	}
	executionID := uuid.New().String()
	execution := &ToolExecution{
		ID:        executionID,
		ToolName:  toolName,
		Arguments: args,
		Status:    "running",
		StartTime: time.Now(),
	}
	if principal, ok := authctx.PrincipalFromContext(ctx); ok {
		execution.OwnerUserID = principal.UserID
	}
	execution.ConversationID = MCPConversationIDFromContext(ctx)

	s.mu.Lock()
	s.executions[executionID] = execution
	s.cleanupOldExecutions()
	s.mu.Unlock()

	if s.storage != nil {
		if err := s.storage.SaveToolExecution(execution); err != nil {
			s.logger.Warn("save execution record to database failed", zap.Error(err))
		}
	}
	return executionID
}

// FinishToolExecution completes a record previously created by BeginToolExecution; when executionID is empty it behaves like RecordCompletedToolInvocation.
func (s *Server) FinishToolExecution(ctx context.Context, executionID, toolName string, args map[string]interface{}, resultText string, invokeErr error) string {
	if s == nil {
		return ""
	}
	if args == nil {
		args = map[string]interface{}{}
	}
	id := strings.TrimSpace(executionID)
	if id == "" {
		id = uuid.New().String()
	}

	now := time.Now()
	failed := invokeErr != nil
	var finalResult *ToolResult

	s.mu.Lock()
	maxBytes := s.toolResultMaxBytes
	spillRoot := s.spillRootDir
	exec, inMem := s.executions[id]
	if !inMem || exec == nil {
		exec = &ToolExecution{
			ID:        id,
			ToolName:  toolName,
			Arguments: args,
			StartTime: now,
		}
		s.executions[id] = exec
	} else if toolName != "" {
		exec.ToolName = toolName
	}
	if len(args) > 0 {
		exec.Arguments = args
	}
	if principal, ok := authctx.PrincipalFromContext(ctx); ok {
		exec.OwnerUserID = principal.UserID
	}
	if conversationID := MCPConversationIDFromContext(ctx); conversationID != "" {
		exec.ConversationID = conversationID
	}
	exec.EndTime = &now
	if exec.StartTime.IsZero() {
		exec.StartTime = now
	}
	exec.Duration = now.Sub(exec.StartTime)

	spill := ToolResultSpillConfig{
		RootDir:        spillRoot,
		ProjectID:      MCPProjectIDFromContext(ctx),
		ConversationID: exec.ConversationID,
		ExecutionID:    id,
	}
	if failed {
		st, msg := executionStatusAndMessage(invokeErr)
		exec.Status = st
		exec.Error = msg
		if strings.TrimSpace(resultText) != "" {
			finalResult = &ToolResult{Content: []Content{{Type: "text", Text: resultText}}}
			finalResult = NormalizeToolResultForStorageWithSpill(finalResult, maxBytes, spill)
			exec.Result = finalResult
		}
	} else {
		exec.Status = "completed"
		text := resultText
		if strings.TrimSpace(text) == "" {
			text = "(no output)"
		}
		finalResult = &ToolResult{Content: []Content{{Type: "text", Text: text}}}
		finalResult = NormalizeToolResultForStorageWithSpill(finalResult, maxBytes, spill)
		exec.Result = finalResult
	}
	s.mu.Unlock()

	if s.storage != nil {
		if err := s.storage.SaveToolExecution(exec); err != nil {
			s.logger.Warn("save execution record to database failed", zap.Error(err))
		}
	}

	s.updateStats(exec.ToolName, exec.Status)

	if s.storage != nil {
		s.mu.Lock()
		delete(s.executions, id)
		s.mu.Unlock()
	}
	return id
}

// AppendToolExecutionPartialOutput records a bounded tail preview for a running local execution.
// The final Result remains authoritative and is written only when the tool finishes.
func (s *Server) AppendToolExecutionPartialOutput(executionID, chunk string) {
	if s == nil || strings.TrimSpace(executionID) == "" || chunk == "" {
		return
	}
	id := strings.TrimSpace(executionID)
	if s.executionService != nil && s.executionService.AppendPartialOutput(id, chunk) {
		return
	}
	now := time.Now()
	s.mu.Lock()
	exec := s.executions[id]
	if exec != nil {
		appendPartialOutput(exec, chunk, defaultPartialOutputMaxBytes, now)
	}
	s.mu.Unlock()
}

// RecordCompletedToolInvocation writes a tool call that was completed via another path into the monitor store
// (format consistent with CallTool completion). Used for Eino ADK filesystem execute and similar paths that
// bypass CallTool; returns the executionId for linking to assistant message mcpExecutionIds.
func (s *Server) RecordCompletedToolInvocation(ctx context.Context, toolName string, args map[string]interface{}, resultText string, invokeErr error) string {
	return s.FinishToolExecution(ctx, "", toolName, args, resultText, invokeErr)
}

// UpdateToolExecutionResult updates the tool result in the monitor store to the display text sent to the model (e.g. persisted-output after reduction).
func (s *Server) UpdateToolExecutionResult(executionID string, result *ToolResult) error {
	if s == nil {
		return nil
	}
	executionID = strings.TrimSpace(executionID)
	if executionID == "" || result == nil {
		return nil
	}
	if previous, ok := s.GetExecution(executionID); ok && previous != nil &&
		(previous.Status == ToolExecutionStatusBlocked || previous.Result != nil && previous.Result.Blocked) {
		result = cloneToolResult(result)
		result.Blocked, result.IsError = true, true
	}
	s.mu.Lock()
	spill := ToolResultSpillConfig{
		RootDir:     s.spillRootDir,
		ExecutionID: executionID,
	}
	if exec, ok := s.executions[executionID]; ok && exec != nil {
		spill.ConversationID = exec.ConversationID
		result = NormalizeToolResultForStorageWithSpill(result, s.toolResultMaxBytes, spill)
		exec.Result = result
	} else {
		result = NormalizeToolResultForStorageWithSpill(result, s.toolResultMaxBytes, spill)
	}
	s.mu.Unlock()
	if s.storage != nil {
		return s.storage.UpdateToolExecutionResult(executionID, result)
	}
	return nil
}

// cleanupOldExecutions removes old execution records to prevent unbounded memory growth
func (s *Server) cleanupOldExecutions() {
	if len(s.executions) <= s.maxExecutionsInMemory {
		return
	}

	// sort by start time to find the oldest records
	type execWithTime struct {
		id        string
		startTime time.Time
	}
	execs := make([]execWithTime, 0, len(s.executions))
	for id, exec := range s.executions {
		execs = append(execs, execWithTime{
			id:        id,
			startTime: exec.StartTime,
		})
	}

	// use sort package for efficient sorting (oldest first)
	sort.Slice(execs, func(i, j int) bool {
		return execs[i].startTime.Before(execs[j].startTime)
	})

	// delete the oldest records, keeping maxExecutionsInMemory entries
	toDelete := len(s.executions) - s.maxExecutionsInMemory
	for i := 0; i < toDelete; i++ {
		delete(s.executions, execs[i].id)
	}

	s.logger.Debug("cleaned up old execution records",
		zap.Int("before", len(execs)),
		zap.Int("after", len(s.executions)),
		zap.Int("deleted", toDelete),
	)
}

func (s *Server) registerRunningCancel(id string, cancel context.CancelFunc) {
	s.runningCancelsMu.Lock()
	s.runningCancels[id] = cancel
	s.runningCancelsMu.Unlock()
}

func (s *Server) unregisterRunningCancel(id string) {
	s.runningCancelsMu.Lock()
	delete(s.runningCancels, id)
	s.runningCancelsMu.Unlock()
}

// RegisterToolExecutionCancel lets non-ExecutionService tool paths, such as Eino
// filesystem execute, participate in cancel_tool_execution by execution_id.
func (s *Server) RegisterToolExecutionCancel(id string, cancel context.CancelFunc) {
	id = strings.TrimSpace(id)
	if s == nil || id == "" || cancel == nil {
		return
	}
	s.registerRunningCancel(id, cancel)
}

func (s *Server) UnregisterToolExecutionCancel(id string) {
	id = strings.TrimSpace(id)
	if s == nil || id == "" {
		return
	}
	s.unregisterRunningCancel(id)
}

func (s *Server) readAbortUserNote(id string) string {
	s.runningCancelsMu.Lock()
	defer s.runningCancelsMu.Unlock()
	if s.abortUserNotes == nil {
		return ""
	}
	return s.abortUserNotes[id]
}

func (s *Server) takeAbortUserNote(id string) string {
	s.runningCancelsMu.Lock()
	defer s.runningCancelsMu.Unlock()
	if s.abortUserNotes == nil {
		return ""
	}
	n := s.abortUserNotes[id]
	delete(s.abortUserNotes, id)
	return n
}

// applyAbortUserNoteToCancelledToolResult merges "tool output + user note" for the model when the monitor page
// "terminate and add note" action is triggered. Tools like exec write failures into *ToolResult with err==nil,
// so merging only on err!=nil would miss the note or incorrectly clear it.
func (s *Server) applyAbortUserNoteToCancelledToolResult(executionID string, result **ToolResult, err *error) (cancelledWithUserNote bool) {
	note := strings.TrimSpace(s.readAbortUserNote(executionID))
	if note == "" {
		return false
	}
	hasErr := err != nil && *err != nil
	hasRes := result != nil && *result != nil
	if hasRes && (*result).Blocked {
		return false
	}
	if !hasErr && !hasRes {
		return false
	}
	_ = s.takeAbortUserNote(executionID)
	partial := ""
	if hasRes {
		partial = ToolResultPlainText(*result)
	}
	if partial == "" && hasErr {
		partial = (*err).Error()
	}
	merged := MergePartialToolOutputAndAbortNote(partial, note)
	*err = nil
	*result = &ToolResult{Content: []Content{{Type: "text", Text: merged}}, IsError: true}
	return true
}

// CancelToolExecutionWithNote cancels an internal tool; if note is non-empty it is merged with the tool's output text before being passed to the model.
func (s *Server) CancelToolExecutionWithNote(id string, note string) bool {
	if s.executionService != nil && s.executionService.Cancel(id, note) {
		return true
	}
	s.runningCancelsMu.Lock()
	cancel, ok := s.runningCancels[id]
	if !ok || cancel == nil {
		s.runningCancelsMu.Unlock()
		return false
	}
	if strings.TrimSpace(note) != "" {
		if s.abortUserNotes == nil {
			s.abortUserNotes = make(map[string]string)
		}
		s.abortUserNotes[id] = strings.TrimSpace(note)
	}
	s.runningCancelsMu.Unlock()
	cancel()
	return true
}

// CancelToolExecution cancels a running internal tool call (no user note).
func (s *Server) CancelToolExecution(id string) bool {
	return s.CancelToolExecutionWithNote(id, "")
}

// ActiveRunningExecutionIDs returns a snapshot of executionIds still registered for cancellation in the current process.
func (s *Server) ActiveRunningExecutionIDs() map[string]struct{} {
	if s == nil {
		return nil
	}
	out := make(map[string]struct{})
	if s.executionService != nil {
		for id := range s.executionService.ActiveRunningExecutionIDs() {
			out[id] = struct{}{}
		}
	}
	s.runningCancelsMu.Lock()
	defer s.runningCancelsMu.Unlock()
	if len(s.runningCancels) == 0 && len(out) == 0 {
		return nil
	}
	for id := range s.runningCancels {
		out[id] = struct{}{}
	}
	return out
}

// initDefaultPrompts initializes the default prompt templates
func (s *Server) initDefaultPrompts() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// network security test prompt
	s.prompts["security_scan"] = &Prompt{
		Name:        "security_scan",
		Description: "Generate prompt for a network security scan task",
		Arguments: []PromptArgument{
			{Name: "target", Description: "scan target (IP address or domain name)", Required: true},
			{Name: "scan_type", Description: "scan type (port, vuln, web, etc.)", Required: false},
		},
	}

	// penetration test prompt
	s.prompts["penetration_test"] = &Prompt{
		Name:        "penetration_test",
		Description: "Generate prompt for a penetration test task",
		Arguments: []PromptArgument{
			{Name: "target", Description: "test target", Required: true},
			{Name: "scope", Description: "test scope", Required: false},
		},
	}
}

// initDefaultResources initializes the default resources
// Note: tool resources are now automatically created when RegisterTool is called; this function is kept for other non-tool resources
func (s *Server) initDefaultResources() {
	// tool resources are now automatically created in RegisterTool; no hardcoding needed here
}

// handleListPrompts handles the list prompts request
func (s *Server) handleListPrompts(msg *Message) *Message {
	s.mu.RLock()
	prompts := make([]Prompt, 0, len(s.prompts))
	for _, prompt := range s.prompts {
		prompts = append(prompts, *prompt)
	}
	s.mu.RUnlock()

	response := ListPromptsResponse{
		Prompts: prompts,
	}
	result, _ := json.Marshal(response)
	return &Message{
		ID:      msg.ID,
		Type:    MessageTypeResponse,
		Version: "2.0",
		Result:  result,
	}
}

// handleGetPrompt handles the get prompt request
func (s *Server) handleGetPrompt(msg *Message) *Message {
	var req GetPromptRequest
	if err := json.Unmarshal(msg.Params, &req); err != nil {
		return &Message{
			ID:      msg.ID,
			Type:    MessageTypeError,
			Version: "2.0",
			Error:   &Error{Code: -32602, Message: "Invalid params"},
		}
	}

	s.mu.RLock()
	prompt, exists := s.prompts[req.Name]
	s.mu.RUnlock()

	if !exists {
		return &Message{
			ID:      msg.ID,
			Type:    MessageTypeError,
			Version: "2.0",
			Error:   &Error{Code: -32601, Message: "Prompt not found"},
		}
	}

	// generate messages based on prompt name
	messages := s.generatePromptMessages(prompt, req.Arguments)

	response := GetPromptResponse{
		Messages: messages,
	}
	result, _ := json.Marshal(response)
	return &Message{
		ID:      msg.ID,
		Type:    MessageTypeResponse,
		Version: "2.0",
		Result:  result,
	}
}

// generatePromptMessages generates prompt messages
func (s *Server) generatePromptMessages(prompt *Prompt, args map[string]interface{}) []PromptMessage {
	messages := []PromptMessage{}

	switch prompt.Name {
	case "security_scan":
		target, _ := args["target"].(string)
		scanType, _ := args["scan_type"].(string)
		if scanType == "" {
			scanType = "comprehensive"
		}

		content := fmt.Sprintf(`Please perform a %s security scan on target %s. Include:
1. Port scan and service identification
2. Vulnerability detection
3. Web application security test
4. Generate a detailed security report`, scanType, target)

		messages = append(messages, PromptMessage{
			Role:    "user",
			Content: content,
		})

	case "penetration_test":
		target, _ := args["target"].(string)
		scope, _ := args["scope"].(string)

		content := fmt.Sprintf(`Please perform a penetration test on target %s.`, target)
		if scope != "" {
			content += fmt.Sprintf("Test scope: %s", scope)
		}
		content += "\nPlease conduct a comprehensive security test following OWASP Top 10."

		messages = append(messages, PromptMessage{
			Role:    "user",
			Content: content,
		})

	default:
		messages = append(messages, PromptMessage{
			Role:    "user",
			Content: "Please execute a security test task",
		})
	}

	return messages
}

// handleListResources handles the list resources request
func (s *Server) handleListResources(msg *Message) *Message {
	s.mu.RLock()
	resources := make([]Resource, 0, len(s.resources))
	for _, resource := range s.resources {
		resources = append(resources, *resource)
	}
	s.mu.RUnlock()

	response := ListResourcesResponse{
		Resources: resources,
	}
	result, _ := json.Marshal(response)
	return &Message{
		ID:      msg.ID,
		Type:    MessageTypeResponse,
		Version: "2.0",
		Result:  result,
	}
}

// handleReadResource handles the read resource request
func (s *Server) handleReadResource(msg *Message) *Message {
	var req ReadResourceRequest
	if err := json.Unmarshal(msg.Params, &req); err != nil {
		return &Message{
			ID:      msg.ID,
			Type:    MessageTypeError,
			Version: "2.0",
			Error:   &Error{Code: -32602, Message: "Invalid params"},
		}
	}

	s.mu.RLock()
	resource, exists := s.resources[req.URI]
	s.mu.RUnlock()

	if !exists {
		return &Message{
			ID:      msg.ID,
			Type:    MessageTypeError,
			Version: "2.0",
			Error:   &Error{Code: -32601, Message: "Resource not found"},
		}
	}

	// 生成资源内容
	content := s.generateResourceContent(resource)

	response := ReadResourceResponse{
		Contents: []ResourceContent{content},
	}
	result, _ := json.Marshal(response)
	return &Message{
		ID:      msg.ID,
		Type:    MessageTypeResponse,
		Version: "2.0",
		Result:  result,
	}
}

// generateResourceContent 生成资源内容
func (s *Server) generateResourceContent(resource *Resource) ResourceContent {
	content := ResourceContent{
		URI:      resource.URI,
		MimeType: resource.MimeType,
	}

	// 如果yestool资源，生成详细文档
	if strings.HasPrefix(resource.URI, "tool://") {
		toolName := strings.TrimPrefix(resource.URI, "tool://")
		content.Text = s.generateToolDocumentation(toolName, resource)
	} else {
		// 其他资源使用description或默认内容
		content.Text = resource.Description
	}

	return content
}

// generateToolDocumentation 生成tool文档
// 注意：硬编码的tool文档已移除，现在只使用tool定义中的info
func (s *Server) generateToolDocumentation(toolName string, resource *Resource) string {
	// 获取tool定义以获取更详细的info
	s.mu.RLock()
	tool, hasTool := s.toolDefs[toolName]
	s.mu.RUnlock()

	// 使用tool定义中的descriptioninfo
	if hasTool {
		doc := fmt.Sprintf("%s\n\n", resource.Description)
		if tool.InputSchema != nil {
			if props, ok := tool.InputSchema["properties"].(map[string]interface{}); ok {
				doc += "参数说明：\n"
				for paramName, paramInfo := range props {
					if paramMap, ok := paramInfo.(map[string]interface{}); ok {
						if desc, ok := paramMap["description"].(string); ok {
							doc += fmt.Sprintf("- %s: %s\n", paramName, desc)
						}
					}
				}
			}
		}
		return doc
	}
	return resource.Description
}

// handleSamplingRequest 处理采样request
func (s *Server) handleSamplingRequest(msg *Message) *Message {
	var req SamplingRequest
	if err := json.Unmarshal(msg.Params, &req); err != nil {
		return &Message{
			ID:      msg.ID,
			Type:    MessageTypeError,
			Version: "2.0",
			Error:   &Error{Code: -32602, Message: "Invalid params"},
		}
	}

	// 注意：采样功能通常需要连接到实际的LLM服务
	// 这里back一个占位符response，实际实现需要集成LLM API
	s.logger.Warn("Sampling request received but not fully implemented",
		zap.Any("request", req),
	)

	response := SamplingResponse{
		Content: []SamplingContent{
			{
				Type: "text",
				Text: "采样功能需要configLLM服务。请使用Agent Loop API进行AIconversation。",
			},
		},
		StopReason: "length",
	}
	result, _ := json.Marshal(response)
	return &Message{
		ID:      msg.ID,
		Type:    MessageTypeResponse,
		Version: "2.0",
		Result:  result,
	}
}

// RegisterPrompt 注册提示词模板
func (s *Server) RegisterPrompt(prompt *Prompt) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts[prompt.Name] = prompt
}

// RegisterResource 注册资源
func (s *Server) RegisterResource(resource *Resource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources[resource.URI] = resource
}

// HandleStdio 处理标准输入输出（用于 stdio 传输pattern）
// MCP protocol使用换行分隔的 JSON-RPC message；管道下需每次写入后 Flush，no则客户端会读不到response
func (s *Server) HandleStdio() error {
	decoder := json.NewDecoder(os.Stdin)
	stdout := bufio.NewWriter(os.Stdout)
	encoder := json.NewEncoder(stdout)
	// 注意：不settings缩进，MCP protocol期望紧凑的 JSON format

	for {
		var msg Message
		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			// log output到 stderr，避免干扰 stdout 的 JSON-RPC 通信
			s.logger.Error("读cancelled息failed", zap.Error(err))
			// 发送errorresponse
			errorMsg := Message{
				ID:      msg.ID,
				Type:    MessageTypeError,
				Version: "2.0",
				Error:   &Error{Code: -32700, Message: "Parse error", Data: err.Error()},
			}
			if err := encoder.Encode(errorMsg); err != nil {
				return fmt.Errorf("发送errorresponsefailed: %w", err)
			}
			if err := stdout.Flush(); err != nil {
				return fmt.Errorf("refresh stdout failed: %w", err)
			}
			continue
		}

		// 处理message
		response := s.handleMessage(context.Background(), &msg)

		// 如果yesnotification（response 为 nil），不需要发送response
		if response == nil {
			continue
		}

		// 发送response
		if err := encoder.Encode(response); err != nil {
			return fmt.Errorf("发送responsefailed: %w", err)
		}
		if err := stdout.Flush(); err != nil {
			return fmt.Errorf("refresh stdout failed: %w", err)
		}
	}

	return nil
}

// sendError 发送errorresponse
func (s *Server) sendError(w http.ResponseWriter, id interface{}, code int, message, data string) {
	var msgID MessageID
	if id != nil {
		msgID = MessageID{value: id}
	}
	response := Message{
		ID:      msgID,
		Type:    MessageTypeError,
		Version: "2.0",
		Error:   &Error{Code: code, Message: message, Data: data},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
