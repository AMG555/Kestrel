package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kestrel/internal/c2"
	"kestrel/internal/config"
	"kestrel/internal/mcp"
	"kestrel/internal/mcp/builtin"
	"kestrel/internal/openai"

	"go.uber.org/zap"
)

// Agent is the AI agent
type Agent struct {
	openAIClient        *openai.Client
	config              *config.OpenAIConfig
	agentConfig         *config.AgentConfig
	mcpServer           *mcp.Server
	externalMCPMgr      *mcp.ExternalMCPManager // external MCP manager
	logger              *zap.Logger
	maxIterations       int
	mu                  sync.RWMutex      // add mutex to support concurrent updates
	toolNameMapping     map[string]string // tool name map: OpenAI format -> original format (for external MCP tools)
	promptBaseDir       string            // base directory for resolving system_prompt_path relative paths (usually the directory containing config.yaml)
	toolDescriptionMode string            // tool description mode: "short" | "full", default short
}

type agentConversationIDKey struct{}

func withAgentConversationID(ctx context.Context, id string) context.Context {
	id = strings.TrimSpace(id)
	if id == "" || ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, agentConversationIDKey{}, id)
}

func agentConversationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(agentConversationIDKey{}).(string)
	return v
}

// ConversationIDFromContext returns the conversation ID injected into the current Agent request context (used by C2 MCP queuing and HITL gating).
func ConversationIDFromContext(ctx context.Context) string {
	return agentConversationIDFromContext(ctx)
}

// NewAgent creates a new Agent
func NewAgent(cfg *config.OpenAIConfig, agentCfg *config.AgentConfig, mcpServer *mcp.Server, externalMCPMgr *mcp.ExternalMCPManager, logger *zap.Logger, maxIterations int) *Agent {
	// if maxIterations is 0 or negative, use default value of 30
	if maxIterations <= 0 {
		maxIterations = 30
	}

	// configure HTTP Transport, optimize connection management and timeout settings
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   300 * time.Second,
			KeepAlive: 300 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 60 * time.Minute, // response header timeout: increased to 15 minutes to handle large responses
		DisableKeepAlives:     false,            // enable connection reuse
	}

	// increase timeout to 30 minutes to support long-running AI inference
	// especially when using streaming response or processing complex tasks
	httpClient := &http.Client{
		Timeout:   30 * time.Minute, // increased from 5 minutes to 30 minutes
		Transport: transport,
	}
	llmClient := openai.NewClient(cfg, httpClient, logger)

	return &Agent{
		openAIClient:        llmClient,
		config:              cfg,
		agentConfig:         agentCfg,
		mcpServer:           mcpServer,
		externalMCPMgr:      externalMCPMgr,
		logger:              logger,
		maxIterations:       maxIterations,
		toolNameMapping:     make(map[string]string), // initialize tool name map
		toolDescriptionMode: "short",
	}
}

// SetPromptBaseDir sets the base directory for resolving single-agent system_prompt_path relative paths (usually the directory containing config.yaml).
func (a *Agent) SetPromptBaseDir(dir string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.promptBaseDir = strings.TrimSpace(dir)
}

// ChatMessage is a chat message
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	// ToolName for tool role only: resume from Eino/trace JSON's name or tool_name field, used to construct ToolMessage when resuming.
	ToolName string `json:"tool_name,omitempty"`
	// ReasoningContent corresponds to OpenAI/DeepSeek reasoning_content; must be passed back when resuming after thinking mode + tool call (see DeepSeek docs).
	ReasoningContent string `json:"reasoning_content,omitempty"`
	// ModelFacingTrace is runtime-only metadata: true means Content was already the exact
	// payload seen at the model boundary and must be restored byte-for-byte.
	ModelFacingTrace bool `json:"-"`
}

// MarshalJSON custom JSON serialization, converts tool_calls arguments to JSON string
func (cm ChatMessage) MarshalJSON() ([]byte, error) {
	// build serialization structure
	aux := map[string]interface{}{
		"role": cm.Role,
	}

	// add content (if exists)
	if cm.Content != "" {
		aux["content"] = cm.Content
	}
	if cm.ReasoningContent != "" {
		aux["reasoning_content"] = cm.ReasoningContent
	}

	// add tool_call_id (if exists)
	if cm.ToolCallID != "" {
		aux["tool_call_id"] = cm.ToolCallID
	}
	if cm.ToolName != "" {
		aux["tool_name"] = cm.ToolName
	}

	// convert tool_calls, converting arguments to JSON string
	if len(cm.ToolCalls) > 0 {
		toolCallsJSON := make([]map[string]interface{}, len(cm.ToolCalls))
		for i, tc := range cm.ToolCalls {
			// convert arguments to JSON string
			argsJSON := ""
			if tc.Function.Arguments != nil {
				argsBytes, err := json.Marshal(tc.Function.Arguments)
				if err != nil {
					return nil, err
				}
				argsJSON = string(argsBytes)
			}

			toolCallsJSON[i] = map[string]interface{}{
				"id":   tc.ID,
				"type": tc.Type,
				"function": map[string]interface{}{
					"name":      tc.Function.Name,
					"arguments": argsJSON,
				},
			}
		}
		aux["tool_calls"] = toolCallsJSON
	}

	return json.Marshal(aux)
}

// OpenAIRequest OpenAI APIrequest
type OpenAIRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Tools    []Tool        `json:"tools,omitempty"`
	Stream   bool          `json:"stream,omitempty"`
}

// OpenAIResponse OpenAI APIresponse
type OpenAIResponse struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
	Error   *Error   `json:"error,omitempty"`
}

// Choice is a completion choice
type Choice struct {
	Message      MessageWithTools `json:"message"`
	FinishReason string           `json:"finish_reason"`
}

// MessageWithTools is a message with tool calls
type MessageWithTools struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// Tool is an OpenAI tool definition
type Tool struct {
	Type     string             `json:"type"`
	Function FunctionDefinition `json:"function"`
}

// FunctionDefinition is a function definition
type FunctionDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// Error OpenAIerror
type Error struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// ToolCall tool call
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall is a function call
type FunctionCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// UnmarshalJSON custom JSON parsing, handles arguments which may be string or object
func (fc *FunctionCall) UnmarshalJSON(data []byte) error {
	type Alias FunctionCall
	aux := &struct {
		Name      string      `json:"name"`
		Arguments interface{} `json:"arguments"`
		*Alias
	}{
		Alias: (*Alias)(fc),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	fc.Name = aux.Name

	// handle arguments which may be string or object
	switch v := aux.Arguments.(type) {
	case map[string]interface{}:
		fc.Arguments = v
	case string:
		// if it is a string, try to parse as JSON
		if err := json.Unmarshal([]byte(v), &fc.Arguments); err != nil {
			// if parsing failed, create a map containing the original string
			fc.Arguments = map[string]interface{}{
				"raw": v,
			}
		}
	case nil:
		fc.Arguments = make(map[string]interface{})
	default:
		// other types, try to convert to map
		fc.Arguments = map[string]interface{}{
			"value": v,
		}
	}

	return nil
}

// ProgressCallback is the progress callback function type
type ProgressCallback func(eventType, message string, data interface{})

// EinoSingleAgentSystemInstruction for use by Eino adk.ChatModelAgent.Instruction (includes system_prompt_path).
func (a *Agent) EinoSingleAgentSystemInstruction() string {
	systemPrompt := DefaultSingleAgentSystemPrompt()
	if a.agentConfig != nil {
		if p := strings.TrimSpace(a.agentConfig.SystemPromptPath); p != "" {
			path := p
			a.mu.RLock()
			base := a.promptBaseDir
			a.mu.RUnlock()
			if !filepath.IsAbs(path) && base != "" {
				path = filepath.Join(base, path)
			}
			if b, err := os.ReadFile(path); err != nil {
				a.logger.Warn("failed to read single-agent system_prompt_path, using built-in prompt", zap.String("path", path), zap.Error(err))
			} else if s := strings.TrimSpace(string(b)); s != "" {
				systemPrompt = s
			}
		}
	}
	return systemPrompt
}

// getAvailableTools gets available tools
// dynamically get tool list from MCP server; description mode controlled by tool_description_mode
// roleTools: role-configured tool list (toolKey format); if empty or nil, use all tools (default role)
func (a *Agent) getAvailableTools(roleTools []string) []Tool {
	// build role tool set (for quick lookup)
	roleToolSet := make(map[string]bool)
	if len(roleTools) > 0 {
		for _, toolKey := range roleTools {
			roleToolSet[toolKey] = true
		}
	}

	// get all registered internal tools from MCP server
	mcpTools := a.mcpServer.GetAllTools()

	// convert to OpenAI-format tool definitions
	tools := make([]Tool, 0, len(mcpTools))
	for _, mcpTool := range mcpTools {
		// if role tool list is specified, only add tools in the list
		if len(roleToolSet) > 0 {
			toolKey := mcpTool.Name // built-in tools use tool name as key
			if !roleToolSet[toolKey] {
				continue // not in role tool list, skip
			}
		}
		description := a.pickToolDescription(mcpTool.ShortDescription, mcpTool.Description)

		// convert schema types to OpenAI standard types
		convertedSchema := a.convertSchemaTypes(mcpTool.InputSchema)

		tools = append(tools, Tool{
			Type: "function",
			Function: FunctionDefinition{
				Name:        mcpTool.Name,
				Description: description, // use short description to reduce token consumption
				Parameters:  convertedSchema,
			},
		})
	}

	// get external MCP tools
	if a.externalMCPMgr != nil {
		// increase timeout to 30 seconds as connecting to remote servers via proxy may take longer
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		externalTools, err := a.externalMCPMgr.GetAllTools(ctx)
		extMap := make(map[string]string)
		if err != nil {
			a.logger.Warn("failed to get external MCP tools", zap.Error(err))
		} else {
			// get external MCP configuration to check tool enable status
			externalMCPConfigs := a.externalMCPMgr.GetConfigs()

			// add external MCP tools to tool list (only add enabled tools)
			for _, externalTool := range externalTools {
				// external tools use "mcpName::toolName" as toolKey
				externalToolKey := externalTool.Name

				// if role tool list is specified, only add tools in the list
				if len(roleToolSet) > 0 {
					if !roleToolSet[externalToolKey] {
						continue // not in role tool list, skip
					}
				}

				// parse tool name: mcpName::toolName
				var mcpName, actualToolName string
				if idx := strings.Index(externalTool.Name, "::"); idx > 0 {
					mcpName = externalTool.Name[:idx]
					actualToolName = externalTool.Name[idx+2:]
				} else {
					continue // skip incorrectly formatted tools
				}

				// checktoolenabled
				enabled := false
				if cfg, exists := externalMCPConfigs[mcpName]; exists {
					// first check if external MCP is enabled
					if !cfg.ExternalMCPEnable {
						enabled = false // MCP not enabled, all tools disabled
					} else {
						// MCP enabled, check individual tool enable status
						// if ToolEnabled is empty or tool not configured, default to enabled (backward compatibility)
						if cfg.ToolEnabled == nil {
							enabled = true // tool status not configured, default to enabled
						} else if toolEnabled, exists := cfg.ToolEnabled[actualToolName]; exists {
							enabled = toolEnabled // use configured tool status
						} else {
							enabled = true // tool not in config, default to enabled
						}
					}
				}

				// only add enabled tools
				if !enabled {
					continue
				}

				description := a.pickToolDescription(externalTool.ShortDescription, externalTool.Description)

				// convert schema types to OpenAI standard types
				convertedSchema := a.convertSchemaTypes(externalTool.InputSchema)

				// replace "::" with "__" in tool name to comply with OpenAI naming convention
				// OpenAI requires tool names to only contain [a-zA-Z0-9_-]
				openAIName := strings.ReplaceAll(externalTool.Name, "::", "__")

				// save name mapping (OpenAI format -> original format)
				extMap[openAIName] = externalTool.Name

				tools = append(tools, Tool{
					Type: "function",
					Function: FunctionDefinition{
						Name:        openAIName, // use OpenAI-compliant name
						Description: description,
						Parameters:  convertedSchema,
					},
				})
			}
		}
		a.mu.Lock()
		a.toolNameMapping = extMap
		a.mu.Unlock()
	}

	a.logger.Debug("getting available tool list",
		zap.Int("internalTools", len(mcpTools)),
		zap.Int("totalTools", len(tools)),
	)

	return tools
}

func (a *Agent) pickToolDescription(shortDesc, fullDesc string) string {
	a.mu.RLock()
	mode := strings.TrimSpace(strings.ToLower(a.toolDescriptionMode))
	a.mu.RUnlock()
	if mode == "full" {
		return fullDesc
	}
	if shortDesc != "" {
		return shortDesc
	}
	return fullDesc
}

// convertSchemaTypes recursively converts schema types to OpenAI standard types
func (a *Agent) convertSchemaTypes(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return schema
	}

	// create new schema copy
	converted := make(map[string]interface{})
	for k, v := range schema {
		converted[k] = v
	}

	// convert types in properties
	if properties, ok := converted["properties"].(map[string]interface{}); ok {
		convertedProperties := make(map[string]interface{})
		for propName, propValue := range properties {
			if prop, ok := propValue.(map[string]interface{}); ok {
				convertedProp := make(map[string]interface{})
				for pk, pv := range prop {
					if pk == "type" {
						// convert type
						if typeStr, ok := pv.(string); ok {
							convertedProp[pk] = a.convertToOpenAIType(typeStr)
						} else {
							convertedProp[pk] = pv
						}
					} else {
						convertedProp[pk] = pv
					}
				}
				convertedProperties[propName] = convertedProp
			} else {
				convertedProperties[propName] = propValue
			}
		}
		converted["properties"] = convertedProperties
	}

	return converted
}

// convertToOpenAIType converts config types to OpenAI/JSON Schema standard types
func (a *Agent) convertToOpenAIType(configType string) string {
	switch configType {
	case "bool":
		return "boolean"
	case "int", "integer":
		return "number"
	case "float", "double":
		return "number"
	case "string", "array", "object":
		return configType
	default:
		// default: return original type
		return configType
	}
}

// ToolExecutionResult is the MCP tool execution result (used by Eino bridge and monitoring persistence).
type ToolExecutionResult struct {
	Result      string
	ExecutionID string
	IsError     bool
	Blocked     bool
}

func buildToolFailureMessage(toolName, detail string, err error) string {
	var b strings.Builder
	fmt.Fprintf(&b, "tool call failed\n\n")
	fmt.Fprintf(&b, "tool name: %s\n", toolName)
	fmt.Fprintf(&b, "error details: %s", detail)
	return strings.TrimRight(b.String(), "\n")
}

// executeToolViaMCP executes a tool via MCP
// return result even if tool execution failed, instead of error, so AI can handle error situations
func (a *Agent) executeToolViaMCP(ctx context.Context, toolName string, args map[string]interface{}) (*ToolExecutionResult, error) {
	a.logger.Info("executing tool via MCP",
		zap.String("tool", toolName),
		zap.Any("args", args),
	)

	// if it is a record_vulnerability tool, automatically add conversation_id
	if toolName == builtin.ToolRecordVulnerability {
		conversationID := agentConversationIDFromContext(ctx)
		if conversationID != "" {
			args["conversation_id"] = conversationID
			a.logger.Debug("auto-adding conversation_id to record_vulnerability tool",
				zap.String("conversation_id", conversationID),
			)
		} else {
			a.logger.Warn("conversation_id is empty when calling record_vulnerability tool")
		}
	}

	var result *mcp.ToolResult
	var executionID string
	var err error

	// single tool execution timeout: prevent a single tool from hanging indefinitely (e.g. still showing as running after 30 minutes)
	toolCtx := ctx
	var toolCancel context.CancelFunc
	if a.agentConfig != nil && a.agentConfig.ToolTimeoutMinutes > 0 {
		toolCtx, toolCancel = context.WithTimeout(ctx, time.Duration(a.agentConfig.ToolTimeoutMinutes)*time.Minute)
		defer func() {
			if toolCancel != nil {
				toolCancel()
			}
		}()
	}
	// C2 dangerous task HITL async wait: must bind to the full Agent lifetime ctx, not the single-tool sub-ctx (which gets cancelled on return)
	toolCtx = c2.WithHITLRunContext(toolCtx, ctx)

	// check if it is an external MCP tool (via tool name map)
	a.mu.RLock()
	originalToolName, isExternalTool := a.toolNameMapping[toolName]
	a.mu.RUnlock()

	if isExternalTool && a.externalMCPMgr != nil {
		// use original tool name to call external MCP tool
		a.logger.Debug("calling external MCP tool",
			zap.String("openAIName", toolName),
			zap.String("originalName", originalToolName),
		)
		result, executionID, err = a.externalMCPMgr.CallTool(toolCtx, originalToolName, args)
	} else {
		// call internal MCP tool
		result, executionID, err = a.mcpServer.CallTool(toolCtx, toolName, args)
	}

	// if call failed (e.g. tool not found, timeout), return friendly error info instead of throwing exception
	if err != nil {
		detail := err.Error()
		timeoutMinutes := 10
		if a.agentConfig != nil && a.agentConfig.ToolTimeoutMinutes > 0 {
			timeoutMinutes = a.agentConfig.ToolTimeoutMinutes
		}
		if errors.Is(err, context.Canceled) {
			detail = "tool call was manually terminated (MCP monitor page). The agent will carry this result and continue subsequent steps; the overall task will not be stopped."
		} else if errors.Is(err, context.DeadlineExceeded) {
			detail = fmt.Sprintf("tool execution exceeded %d minutes and was automatically terminated (configurable via agent.tool_timeout_minutes in config.yaml)", timeoutMinutes)
		}
		errorMsg := buildToolFailureMessage(toolName, detail, err)

		return &ToolExecutionResult{
			Result:      errorMsg,
			ExecutionID: executionID,
			IsError:     true,
		}, nil // return nil error, let caller handle the result
	}

	// format result
	var resultText strings.Builder
	for _, content := range result.Content {
		resultText.WriteString(content.Text)
		resultText.WriteString("\n")
	}

	resultStr := resultText.String()

	return &ToolExecutionResult{
		Result:      resultStr,
		ExecutionID: executionID,
		IsError:     result != nil && result.IsError,
		Blocked:     result != nil && result.Blocked,
	}, nil
}

// UpdateConfig updateOpenAIconfig
func (a *Agent) UpdateConfig(cfg *config.OpenAIConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.config = cfg

	a.logger.Info("Agent configuration updated",
		zap.String("base_url", cfg.BaseURL),
		zap.String("model", cfg.Model),
	)
}

// UpdateMaxIterations updatemaximum iterations
func (a *Agent) UpdateMaxIterations(maxIterations int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if maxIterations > 0 {
		a.maxIterations = maxIterations
		a.logger.Info("Agent maximum iterations updated", zap.Int("max_iterations", maxIterations))
	}
}

// UpdateToolDescriptionMode updates the tool description pattern (short/full).
func (a *Agent) UpdateToolDescriptionMode(mode string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode != "full" {
		mode = "short"
	}
	a.toolDescriptionMode = mode
	a.logger.Debug("Agent tool description mode updated", zap.String("tool_description_mode", mode))
}

// RepairOrphanToolMessages cleans up orphaned tool messages and incomplete tool_calls to avoid OpenAI errors
// also ensures tool_calls in history messages only serve as context memory and do not trigger re-execution
// this is a public method that can be called when resuming history messages
func (a *Agent) RepairOrphanToolMessages(messages *[]ChatMessage) bool {
	return a.repairOrphanToolMessages(messages)
}

// repairOrphanToolMessages cleans up orphaned tool messages and incomplete tool_calls to avoid OpenAI errors
// also ensures tool_calls in history messages only serve as context memory and do not trigger re-execution
func (a *Agent) repairOrphanToolMessages(messages *[]ChatMessage) bool {
	if messages == nil {
		return false
	}

	msgs := *messages
	if len(msgs) == 0 {
		return false
	}

	pending := make(map[string]int)
	cleaned := make([]ChatMessage, 0, len(msgs))
	removed := false

	for _, msg := range msgs {
		switch strings.ToLower(msg.Role) {
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				// record all tool_call IDs
				for _, tc := range msg.ToolCalls {
					if tc.ID != "" {
						pending[tc.ID]++
					}
				}
			}
			cleaned = append(cleaned, msg)
		case "tool":
			callID := msg.ToolCallID
			if callID == "" {
				removed = true
				continue
			}
			if count, exists := pending[callID]; exists && count > 0 {
				if count == 1 {
					delete(pending, callID)
				} else {
					pending[callID] = count - 1
				}
				cleaned = append(cleaned, msg)
			} else {
				removed = true
				continue
			}
		default:
			cleaned = append(cleaned, msg)
		}
	}

	// if there are still unmatched tool_calls (i.e. assistant message has tool_calls but no corresponding tool response)
	// need to remove these tool_calls from the last assistant message to prevent AI from re-executing them
	if len(pending) > 0 {
		// search backwards for the last assistant message
		for i := len(cleaned) - 1; i >= 0; i-- {
			if strings.ToLower(cleaned[i].Role) == "assistant" && len(cleaned[i].ToolCalls) > 0 {
				// remove unmatched tool_calls
				originalCount := len(cleaned[i].ToolCalls)
				validToolCalls := make([]ToolCall, 0)
				for _, tc := range cleaned[i].ToolCalls {
					if tc.ID != "" && pending[tc.ID] > 0 {
						// this tool_call has no corresponding tool response, remove it
						removed = true
						delete(pending, tc.ID)
					} else {
						validToolCalls = append(validToolCalls, tc)
					}
				}
				// update message's ToolCalls
				if len(validToolCalls) != originalCount {
					cleaned[i].ToolCalls = validToolCalls
					a.logger.Info("removed incomplete tool_calls to prevent re-execution",
						zap.Int("removed_count", originalCount-len(validToolCalls)),
					)
				}
				break
			}
		}
	}

	if removed {
		a.logger.Warn("repaired tool messages and tool_calls in conversation history",
			zap.Int("original_messages", len(msgs)),
			zap.Int("cleaned_messages", len(cleaned)),
		)
		*messages = cleaned
	}

	return removed
}

// ToolsForRole returns tool definitions consistent with single Agent loop (OpenAI function format), for Eino DeepAgent etc. orchestration layers to bind MCP tools.
func (a *Agent) ToolsForRole(roleTools []string) []Tool {
	return a.getAvailableTools(roleTools)
}

// ExecuteMCPToolForConversation executes an MCP tool in a specified conversation context (behavior consistent with tool calls in the main Agent loop, e.g. auto-inject conversation_id).
func (a *Agent) ExecuteMCPToolForConversation(ctx context.Context, conversationID, toolName string, args map[string]interface{}) (*ToolExecutionResult, error) {
	ctx = withAgentConversationID(ctx, conversationID)
	ctx = mcp.WithMCPConversationID(ctx, conversationID)
	return a.executeToolViaMCP(ctx, toolName, args)
}

// BeginLocalToolExecution writes running status when a non-CallTool path tool starts, for the MCP monitor page to display "executing".
func (a *Agent) BeginLocalToolExecution(ctx context.Context, toolName string, args map[string]interface{}) string {
	if a == nil || a.mcpServer == nil {
		return ""
	}
	return a.mcpServer.BeginToolExecution(ctx, toolName, args)
}

// FinishLocalToolExecution completes the record created by BeginLocalToolExecution; when executionID is empty, writes a completed record in one shot.
func (a *Agent) FinishLocalToolExecution(ctx context.Context, executionID, toolName string, args map[string]interface{}, resultText string, invokeErr error) string {
	if a == nil || a.mcpServer == nil {
		return ""
	}
	return a.mcpServer.FinishToolExecution(ctx, executionID, toolName, args, resultText, invokeErr)
}

// AppendLocalToolExecutionPartialOutput records a bounded live-output preview for a running local tool.
func (a *Agent) AppendLocalToolExecutionPartialOutput(executionID, chunk string) {
	if a == nil || a.mcpServer == nil {
		return
	}
	a.mcpServer.AppendToolExecutionPartialOutput(executionID, chunk)
}

func (a *Agent) RegisterLocalToolExecutionCancel(executionID string, cancel context.CancelFunc) {
	if a == nil || a.mcpServer == nil {
		return
	}
	a.mcpServer.RegisterToolExecutionCancel(executionID, cancel)
}

func (a *Agent) UnregisterLocalToolExecutionCancel(executionID string) {
	if a == nil || a.mcpServer == nil {
		return
	}
	a.mcpServer.UnregisterToolExecutionCancel(executionID)
}

// RecordLocalToolExecution writes completed non-CallTool path tool calls to the MCP monitor database (consistent with CallTool persistence), returns executionId.
// used for Eino filesystem execute scenarios, making the assistant bubble 'penetration test details' consistent with regular MCP and clickable in the monitor.
func (a *Agent) RecordLocalToolExecution(ctx context.Context, toolName string, args map[string]interface{}, resultText string, invokeErr error) string {
	return a.FinishLocalToolExecution(ctx, "", toolName, args, resultText, invokeErr)
}

// UpdateMCPExecutionDisplayResult updates the tool result in the monitor database to the display body sent to the model (after reduction).
func (a *Agent) UpdateMCPExecutionDisplayResult(executionID, resultText string) {
	if a == nil || strings.TrimSpace(executionID) == "" {
		return
	}
	text := resultText
	if strings.TrimSpace(text) == "" {
		text = "(no output)"
	}
	tr := &mcp.ToolResult{
		Content: []mcp.Content{{Type: "text", Text: text}},
	}
	if exec := a.mcpExecution(executionID); exec != nil && exec.Result != nil {
		tr.IsError = exec.Result.IsError
		tr.Blocked = exec.Result.Blocked
	}
	if a.mcpServer != nil {
		_ = a.mcpServer.UpdateToolExecutionResult(executionID, tr)
	}
}

// MCPExecutionResultText returns the monitor-facing result text after storage
// guards such as large-output spilling have been applied.
func (a *Agent) MCPExecutionResultText(executionID string) string {
	exec := a.mcpExecution(executionID)
	if exec == nil || exec.Result == nil {
		return ""
	}
	return mcp.ToolResultPlainText(exec.Result)
}

// MCPExecutionStatus returns the recorded outcome independently of model-facing
// text reduction, which can remove the original refusal wording.
func (a *Agent) MCPExecutionStatus(executionID string) string {
	if exec := a.mcpExecution(executionID); exec != nil {
		return exec.Status
	}
	return ""
}

func (a *Agent) mcpExecution(executionID string) *mcp.ToolExecution {
	if a == nil || strings.TrimSpace(executionID) == "" {
		return nil
	}
	if a.mcpServer != nil {
		if exec, ok := a.mcpServer.GetExecution(executionID); ok && exec != nil {
			return exec
		}
	}
	if a.externalMCPMgr != nil {
		if exec, ok := a.externalMCPMgr.GetExecution(executionID); ok {
			return exec
		}
	}
	return nil
}

// CancelMCPToolExecutionWithNote cancels an in-progress MCP tool (internal first, then external), consistent with the monitor page 'terminate tool'; when note is non-empty, merges into the text returned to the model.
func (a *Agent) CancelMCPToolExecutionWithNote(executionID, note string) bool {
	executionID = strings.TrimSpace(executionID)
	note = strings.TrimSpace(note)
	if executionID == "" {
		return false
	}
	if a.mcpServer != nil && a.mcpServer.CancelToolExecutionWithNote(executionID, note) {
		return true
	}
	if a.externalMCPMgr != nil && a.externalMCPMgr.CancelToolExecutionWithNote(executionID, note) {
		return true
	}
	return false
}

// CancelRunningMCPToolsForConversation cancels all currently running internal/external MCP executions
// owned by the conversation. It is used when a session ends or the user stops a task.
func (a *Agent) CancelRunningMCPToolsForConversation(conversationID, note string) int {
	conversationID = strings.TrimSpace(conversationID)
	if a == nil || conversationID == "" {
		return 0
	}
	note = strings.TrimSpace(note)
	seen := make(map[string]struct{})
	cancelled := 0
	cancelIfConversationMatches := func(execID string, get func(string) (*mcp.ToolExecution, bool), cancel func(string, string) bool) {
		execID = strings.TrimSpace(execID)
		if execID == "" {
			return
		}
		if _, ok := seen[execID]; ok {
			return
		}
		seen[execID] = struct{}{}
		exec, ok := get(execID)
		if !ok || exec == nil || strings.TrimSpace(exec.ConversationID) != conversationID {
			return
		}
		if cancel(execID, note) {
			cancelled++
		}
	}
	if a.mcpServer != nil {
		for execID := range a.mcpServer.ActiveRunningExecutionIDs() {
			cancelIfConversationMatches(execID, a.mcpServer.GetExecution, a.mcpServer.CancelToolExecutionWithNote)
		}
	}
	if a.externalMCPMgr != nil {
		for execID := range a.externalMCPMgr.ActiveRunningExecutionIDs() {
			cancelIfConversationMatches(execID, a.externalMCPMgr.GetExecution, a.externalMCPMgr.CancelToolExecutionWithNote)
		}
	}
	return cancelled
}

// extractQuotedToolName tries to extract the quoted tool name from error info
func extractQuotedToolName(errMsg string) string {
	start := strings.Index(errMsg, "\"")
	if start == -1 {
		return ""
	}
	rest := errMsg[start+1:]
	end := strings.Index(rest, "\"")
	if end == -1 {
		return ""
	}
	return rest[:end]
}
