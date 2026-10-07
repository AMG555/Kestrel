package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"kestrel/internal/audit"
	"kestrel/internal/config"
	"kestrel/internal/database"
	"kestrel/internal/mcp"
	"kestrel/internal/monitor"
	"kestrel/internal/security"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// MonitorHandler is the monitoring handler
type MonitorHandler struct {
	mcpServer        *mcp.Server
	externalMCPMgr   *mcp.ExternalMCPManager
	taskManager      *AgentTaskManager
	agentHandler     *AgentHandler
	executor         *security.Executor
	db               *database.DB
	logger           *zap.Logger
	audit            *audit.Service
	monitorRetention *monitor.Service
}

// SetMonitorRetention wires MCP execution retention settings.
func (h *MonitorHandler) SetMonitorRetention(s *monitor.Service) {
	h.monitorRetention = s
}

// SetAudit wires platform audit logging.
func (h *MonitorHandler) SetAudit(s *audit.Service) {
	h.audit = s
}

// NewMonitorHandler creates a new monitoring handler
func NewMonitorHandler(mcpServer *mcp.Server, executor *security.Executor, db *database.DB, logger *zap.Logger) *MonitorHandler {
	return &MonitorHandler{
		mcpServer:      mcpServer,
		externalMCPMgr: nil, // will be set after creation
		executor:       executor,
		db:             db,
		logger:         logger,
	}
}

// SetExternalMCPManager sets the external MCP manager
func (h *MonitorHandler) SetExternalMCPManager(mgr *mcp.ExternalMCPManager) {
	h.externalMCPMgr = mgr
}

// SetTaskManager sets the Agent task manager (used to terminate Eino executes etc. by executionId).
func (h *MonitorHandler) SetTaskManager(mgr *AgentTaskManager) {
	h.taskManager = mgr
}

// SetAgentHandler sets the Agent handler (shared logic for MCP monitoring termination and conversation page "interrupt and continue").
func (h *MonitorHandler) SetAgentHandler(ah *AgentHandler) {
	h.agentHandler = ah
}

const monitorPageTopTools = 6

// MonitorStatsSummary is the tool call summary
type MonitorStatsSummary struct {
	TotalCalls   int        `json:"totalCalls"`
	SuccessCalls int        `json:"successCalls"`
	FailedCalls  int        `json:"failedCalls"`
	BlockedCalls int        `json:"blockedCalls"`
	LastCallTime *time.Time `json:"lastCallTime,omitempty"`
	ToolCount    int        `json:"toolCount"`
}

// MonitorResponse is the monitoring response
type MonitorResponse struct {
	Executions    []*mcp.ToolExecution `json:"executions"`
	Summary       *MonitorStatsSummary `json:"summary"`
	TopTools      []*mcp.ToolStats     `json:"topTools"`
	Timestamp     time.Time            `json:"timestamp"`
	Total         int                  `json:"total"`
	Page          int                  `json:"page"`
	PageSize      int                  `json:"pageSize"`
	TotalPages    int                  `json:"totalPages"`
	RetentionDays int                  `json:"retentionDays"`
}

// StatsResponse is the statistics info response (Dashboard etc.)
type StatsResponse struct {
	Summary  *MonitorStatsSummary `json:"summary"`
	TopTools []*mcp.ToolStats     `json:"topTools"`
}

// Monitor retrieves monitoring info
func (h *MonitorHandler) Monitor(c *gin.Context) {
	// parse pagination parameters
	page := 1
	pageSize := 20
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	// parse status filter parameter
	status := c.Query("status")
	// parse tool filter parameter (compatible with mcp__tool and internal mcp::tool)
	toolName := normalizeToolNameFilter(c.Query("tool"))

	access := notificationAccessFromContext(c)
	executions, total := h.loadExecutionListWithPagination(page, pageSize, status, toolName, access)
	h.enrichExecutionsConversationID(executions)
	var summary *MonitorStatsSummary
	var topTools []*mcp.ToolStats
	if access.Scope == database.RBACScopeAll {
		summary, topTools = h.loadStatsSummary(monitorPageTopTools)
	} else if h.db != nil {
		if scoped, err := h.db.LoadToolStatsSummaryForAccess(monitorPageTopTools, access); err == nil {
			summary, topTools = dbStatsSummaryToMonitor(scoped), scoped.TopTools
		} else {
			summary, topTools = summarizeAccessibleExecutionPage(executions, monitorPageTopTools)
		}
	} else {
		summary, topTools = summarizeAccessibleExecutionPage(executions, monitorPageTopTools)
	}

	totalPages := (total + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}

	c.JSON(http.StatusOK, MonitorResponse{
		Executions:    executions,
		Summary:       summary,
		TopTools:      topTools,
		Timestamp:     time.Now(),
		Total:         total,
		Page:          page,
		PageSize:      pageSize,
		TotalPages:    totalPages,
		RetentionDays: h.monitorRetentionDays(),
	})
}

func summarizeAccessibleExecutionPage(executions []*mcp.ToolExecution, topN int) (*MonitorStatsSummary, []*mcp.ToolStats) {
	stats := map[string]*mcp.ToolStats{}
	for _, exec := range executions {
		if exec == nil {
			continue
		}
		stat := stats[exec.ToolName]
		if stat == nil {
			stat = &mcp.ToolStats{ToolName: exec.ToolName}
			stats[exec.ToolName] = stat
		}
		stat.TotalCalls++
		if monitorStatusCountsAsFailed(exec.Status) {
			stat.FailedCalls++
		} else if exec.Status == "completed" {
			stat.SuccessCalls++
		} else if exec.Status == mcp.ToolExecutionStatusBlocked {
			stat.BlockedCalls++
		}
		started := exec.StartTime
		if stat.LastCallTime == nil || started.After(*stat.LastCallTime) {
			stat.LastCallTime = &started
		}
	}
	return summarizeToolStats(stats, topN)
}

func monitorStatusCountsAsFailed(status string) bool {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "failed", "hard_timeout", "orphaned":
		return true
	default:
		return false
	}
}

func (h *MonitorHandler) monitorRetentionDays() int {
	if h.monitorRetention != nil {
		return h.monitorRetention.RetentionDays()
	}
	return config.MonitorConfig{}.RetentionDaysEffective()
}

func (h *MonitorHandler) loadExecutions() []*mcp.ToolExecution {
	executions, _ := h.loadExecutionsWithPagination(1, 1000, "", "")
	return executions
}

func (h *MonitorHandler) loadExecutionListWithPagination(page, pageSize int, status, toolName string, access database.RBACListAccess) ([]*mcp.ToolExecution, int) {
	if h.db == nil {
		allExecutions := filterToolExecutionsForAccess(h.mcpServer.GetAllExecutions(), access, h.db)
		if status != "" || toolName != "" {
			filtered := make([]*mcp.ToolExecution, 0)
			for _, exec := range allExecutions {
				matchStatus := status == "" || exec.Status == status
				matchTool := toolNameFilterMatches(exec.ToolName, toolName)
				if matchStatus && matchTool {
					filtered = append(filtered, exec)
				}
			}
			allExecutions = filtered
		}
		total := len(allExecutions)
		offset := (page - 1) * pageSize
		end := offset + pageSize
		if end > total {
			end = total
		}
		if offset >= total {
			return []*mcp.ToolExecution{}, total
		}
		pageSlice := allExecutions[offset:end]
		out := make([]*mcp.ToolExecution, 0, len(pageSlice))
		for _, exec := range pageSlice {
			if exec == nil {
				continue
			}
			out = append(out, slimToolExecution(exec))
		}
		return out, total
	}

	offset := (page - 1) * pageSize
	executions, err := h.db.LoadToolExecutionListPageForAccess(offset, pageSize, status, toolName, access)
	if err != nil {
		h.logger.Warn("failed to load execution record list from database; falling back to in-memory data", zap.Error(err))
		return h.loadExecutionListWithPaginationFromMemory(page, pageSize, status, toolName, access)
	}

	total, err := h.db.CountToolExecutionsForAccess(status, toolName, access)
	if err != nil {
		h.logger.Warn("failed to get execution record total", zap.Error(err))
		total = offset + len(executions)
		if len(executions) == pageSize {
			total = offset + len(executions) + 1
		}
	}

	return executions, total
}

func (h *MonitorHandler) loadExecutionListWithPaginationFromMemory(page, pageSize int, status, toolName string, access database.RBACListAccess) ([]*mcp.ToolExecution, int) {
	allExecutions := filterToolExecutionsForAccess(h.mcpServer.GetAllExecutions(), access, h.db)
	if status != "" || toolName != "" {
		filtered := make([]*mcp.ToolExecution, 0)
		for _, exec := range allExecutions {
			matchStatus := status == "" || exec.Status == status
			matchTool := toolNameFilterMatches(exec.ToolName, toolName)
			if matchStatus && matchTool {
				filtered = append(filtered, exec)
			}
		}
		allExecutions = filtered
	}
	total := len(allExecutions)
	offset := (page - 1) * pageSize
	end := offset + pageSize
	if end > total {
		end = total
	}
	if offset >= total {
		return []*mcp.ToolExecution{}, total
	}
	pageSlice := allExecutions[offset:end]
	out := make([]*mcp.ToolExecution, 0, len(pageSlice))
	for _, exec := range pageSlice {
		if exec == nil {
			continue
		}
		out = append(out, slimToolExecution(exec))
	}
	return out, total
}

func slimToolExecution(exec *mcp.ToolExecution) *mcp.ToolExecution {
	if exec == nil {
		return nil
	}
	slim := &mcp.ToolExecution{
		ID:        exec.ID,
		ToolName:  exec.ToolName,
		Status:    exec.Status,
		StartTime: exec.StartTime,
	}
	if exec.EndTime != nil {
		end := *exec.EndTime
		slim.EndTime = &end
	}
	if exec.Duration > 0 {
		slim.Duration = exec.Duration
	}
	return slim
}

func filterToolExecutionsForAccess(executions []*mcp.ToolExecution, access database.RBACListAccess, db *database.DB) []*mcp.ToolExecution {
	if access.Scope == database.RBACScopeAll {
		return executions
	}
	out := make([]*mcp.ToolExecution, 0, len(executions))
	for _, exec := range executions {
		if toolExecutionVisible(exec, access, db) {
			out = append(out, exec)
		}
	}
	return out
}

func toolExecutionVisible(exec *mcp.ToolExecution, access database.RBACListAccess, db *database.DB) bool {
	if exec == nil || strings.TrimSpace(access.UserID) == "" {
		return false
	}
	if access.Scope == database.RBACScopeAll || strings.TrimSpace(exec.OwnerUserID) == strings.TrimSpace(access.UserID) {
		return true
	}
	conversationID := strings.TrimSpace(exec.ConversationID)
	return conversationID != "" && db != nil && db.UserCanAccessResource(access.UserID, access.Scope, "conversation", conversationID)
}

func (h *MonitorHandler) monitorExecutionAllowed(c *gin.Context, id string) bool {
	access := notificationAccessFromContext(c)
	if access.Scope == database.RBACScopeAll {
		return true
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	if exec, ok := h.mcpServer.GetExecution(id); ok {
		return toolExecutionVisible(exec, access, h.db)
	}
	if h.externalMCPMgr != nil {
		if exec, ok := h.externalMCPMgr.GetExecution(id); ok {
			return toolExecutionVisible(exec, access, h.db)
		}
	}
	return h.db != nil && h.db.UserCanAccessToolExecution(access.UserID, access.Scope, id)
}

func (h *MonitorHandler) loadExecutionsWithPagination(page, pageSize int, status, toolName string) ([]*mcp.ToolExecution, int) {
	if h.db == nil {
		allExecutions := h.mcpServer.GetAllExecutions()
		// if status filter or tool filter is specified, apply filtering first
		if status != "" || toolName != "" {
			filtered := make([]*mcp.ToolExecution, 0)
			for _, exec := range allExecutions {
				matchStatus := status == "" || exec.Status == status
				// support partial matching (fuzzy search)
				matchTool := toolNameFilterMatches(exec.ToolName, toolName)
				if matchStatus && matchTool {
					filtered = append(filtered, exec)
				}
			}
			allExecutions = filtered
		}
		total := len(allExecutions)
		offset := (page - 1) * pageSize
		end := offset + pageSize
		if end > total {
			end = total
		}
		if offset >= total {
			return []*mcp.ToolExecution{}, total
		}
		return allExecutions[offset:end], total
	}

	offset := (page - 1) * pageSize
	executions, err := h.db.LoadToolExecutionsWithPagination(offset, pageSize, status, toolName)
	if err != nil {
		h.logger.Warn("failed to load execution records from database; falling back to in-memory data", zap.Error(err))
		allExecutions := h.mcpServer.GetAllExecutions()
		// if status filter or tool filter is specified, apply filtering first
		if status != "" || toolName != "" {
			filtered := make([]*mcp.ToolExecution, 0)
			for _, exec := range allExecutions {
				matchStatus := status == "" || exec.Status == status
				// support partial matching (fuzzy search)
				matchTool := toolNameFilterMatches(exec.ToolName, toolName)
				if matchStatus && matchTool {
					filtered = append(filtered, exec)
				}
			}
			allExecutions = filtered
		}
		total := len(allExecutions)
		offset := (page - 1) * pageSize
		end := offset + pageSize
		if end > total {
			end = total
		}
		if offset >= total {
			return []*mcp.ToolExecution{}, total
		}
		return allExecutions[offset:end], total
	}

	// get total (considering status filter and tool filter)
	total, err := h.db.CountToolExecutions(status, toolName)
	if err != nil {
		h.logger.Warn("failed to get execution record total", zap.Error(err))
		// fallback: estimate using the number of loaded records
		total = offset + len(executions)
		if len(executions) == pageSize {
			total = offset + len(executions) + 1
		}
	}

	return executions, total
}

func (h *MonitorHandler) loadStatsSummary(topN int) (*MonitorStatsSummary, []*mcp.ToolStats) {
	if topN <= 0 {
		topN = monitorPageTopTools
	}

	if h.db != nil {
		result, err := h.db.LoadToolStatsSummary(topN)
		if err == nil {
			return dbStatsSummaryToMonitor(result), result.TopTools
		}
		h.logger.Warn("failed to load statistics summary from database; falling back to in-memory data", zap.Error(err))
	}

	stats := h.loadStatsMap()
	return summarizeToolStats(stats, topN)
}

func dbStatsSummaryToMonitor(result *database.ToolStatsSummaryResult) *MonitorStatsSummary {
	if result == nil {
		return &MonitorStatsSummary{}
	}
	summary := &MonitorStatsSummary{
		TotalCalls:   result.Summary.TotalCalls,
		SuccessCalls: result.Summary.SuccessCalls,
		FailedCalls:  result.Summary.FailedCalls,
		BlockedCalls: result.Summary.BlockedCalls,
		ToolCount:    result.Summary.ToolCount,
	}
	if result.Summary.LastCallTime != nil {
		t := *result.Summary.LastCallTime
		summary.LastCallTime = &t
	}
	return summary
}

func summarizeToolStats(stats map[string]*mcp.ToolStats, topN int) (*MonitorStatsSummary, []*mcp.ToolStats) {
	summary := &MonitorStatsSummary{}
	if len(stats) == 0 {
		return summary, nil
	}

	all := make([]*mcp.ToolStats, 0, len(stats))
	for _, stat := range stats {
		if stat == nil {
			continue
		}
		summary.ToolCount++
		summary.TotalCalls += stat.TotalCalls
		summary.SuccessCalls += stat.SuccessCalls
		summary.FailedCalls += stat.FailedCalls
		summary.BlockedCalls += stat.BlockedCalls
		if stat.LastCallTime != nil && (summary.LastCallTime == nil || stat.LastCallTime.After(*summary.LastCallTime)) {
			t := *stat.LastCallTime
			summary.LastCallTime = &t
		}
		if stat.TotalCalls > 0 {
			statCopy := *stat
			all = append(all, &statCopy)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].TotalCalls == all[j].TotalCalls {
			return all[i].ToolName < all[j].ToolName
		}
		return all[i].TotalCalls > all[j].TotalCalls
	})
	if len(all) > topN {
		all = all[:topN]
	}
	return summary, all
}

func (h *MonitorHandler) loadStatsMap() map[string]*mcp.ToolStats {
	// merge statistics from the internal MCP server and external MCP manager
	stats := make(map[string]*mcp.ToolStats)

	// load statistics from the internal MCP server
	if h.db == nil {
		internalStats := h.mcpServer.GetStats()
		for k, v := range internalStats {
			stats[k] = v
		}
	} else {
		dbStats, err := h.db.LoadToolStats()
		if err != nil {
			h.logger.Warn("failed to load statistics from database; falling back to in-memory data", zap.Error(err))
			internalStats := h.mcpServer.GetStats()
			for k, v := range internalStats {
				stats[k] = v
			}
		} else {
			for k, v := range dbStats {
				stats[k] = v
			}
		}
	}

	// merge statistics from the external MCP manager
	if h.externalMCPMgr != nil {
		externalStats := h.externalMCPMgr.GetToolStats()
		for k, v := range externalStats {
			// if already exists, merge statistics
			if existing, exists := stats[k]; exists {
				existing.TotalCalls += v.TotalCalls
				existing.SuccessCalls += v.SuccessCalls
				existing.FailedCalls += v.FailedCalls
				existing.BlockedCalls += v.BlockedCalls
				// use the latest call time
				if v.LastCallTime != nil && (existing.LastCallTime == nil || v.LastCallTime.After(*existing.LastCallTime)) {
					existing.LastCallTime = v.LastCallTime
				}
			} else {
				stats[k] = v
			}
		}
	}

	return stats
}

// GetExecution retrieves a specific execution record
func (h *MonitorHandler) GetExecution(c *gin.Context) {
	id := c.Param("id")
	if !h.monitorExecutionAllowed(c, id) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}

	// first search in the internal MCP server
	exec, exists := h.mcpServer.GetExecution(id)
	if exists {
		h.enrichExecutionsConversationID([]*mcp.ToolExecution{exec})
		c.JSON(http.StatusOK, exec)
		return
	}

	// if not found, try the external MCP manager
	if h.externalMCPMgr != nil {
		exec, exists = h.externalMCPMgr.GetExecution(id)
		if exists {
			h.enrichExecutionsConversationID([]*mcp.ToolExecution{exec})
			c.JSON(http.StatusOK, exec)
			return
		}
	}

	// if still not found, try the database (if database storage is used)
	if h.db != nil {
		exec, err := h.db.GetToolExecution(id)
		if err == nil && exec != nil {
			h.enrichExecutionsConversationID([]*mcp.ToolExecution{exec})
			c.JSON(http.StatusOK, exec)
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "execution record not found"})
}

// CancelExecution manually cancels an in-progress MCP tool call (cancels only that tools/call context; does not stop the entire Agent / iteration task)
// Optional request body JSON: { "note": "user note" }, merged with tool output already returned to the model (includes a "user termination note" title block, distinct from command-line output).
func (h *MonitorHandler) CancelExecution(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "execution record ID cannot be empty"})
		return
	}
	if !h.monitorExecutionAllowed(c, id) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}
	note := ""
	dec := json.NewDecoder(c.Request.Body)
	var body struct {
		Note string `json:"note"`
	}
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request body must be JSON, e.g. {\"note\":\"note text\"}, may be an empty object"})
		return
	}
	note = strings.TrimSpace(body.Note)

	convID := h.conversationIDForRunningExecution(id)
	if convID != "" && h.agentHandler != nil {
		if ok, payload := h.agentHandler.cancelToolContinueAfter(convID, id, note); ok {
			h.logger.Info("MCP monitoring page: terminating tool (consistent with conversation interrupt-and-continue)",
				zap.String("executionId", id),
				zap.String("conversationId", convID),
				zap.Bool("hasNote", note != ""),
			)
			c.JSON(http.StatusOK, payload)
			return
		}
	}
	if h.mcpServer.CancelToolExecutionWithNote(id, note) {
		h.logger.Info("requested cancellation of MCP tool execution", zap.String("executionId", id), zap.String("source", "internal"), zap.Bool("hasNote", note != ""))
		c.JSON(http.StatusOK, gin.H{"message": "termination signal sent", "executionId": id})
		return
	}
	if h.externalMCPMgr != nil && h.externalMCPMgr.CancelToolExecutionWithNote(id, note) {
		h.logger.Info("requested cancellation of MCP tool execution", zap.String("executionId", id), zap.String("source", "external"), zap.Bool("hasNote", note != ""))
		c.JSON(http.StatusOK, gin.H{"message": "termination signal sent", "executionId": id})
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "no in-progress tool execution found, or the task has already ended"})
}

func (h *MonitorHandler) enrichExecutionsConversationID(executions []*mcp.ToolExecution) {
	for _, exec := range executions {
		if exec == nil || exec.Status != "running" {
			continue
		}
		exec.ConversationID = h.conversationIDForRunningExecution(exec.ID)
	}
}

func (h *MonitorHandler) conversationIDForRunningExecution(executionID string) string {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" || h.taskManager == nil {
		return ""
	}
	if conv := h.taskManager.ConversationIDForActiveMCPExecution(executionID); conv != "" {
		return conv
	}
	exec := h.lookupExecution(executionID)
	if exec == nil || exec.Status != "running" {
		return ""
	}
	if strings.TrimSpace(exec.ToolName) == "execute" {
		if onlyConv, ok := h.taskManager.ConversationIDForActiveEinoExecute(); ok {
			return onlyConv
		}
	}
	return ""
}

func (h *MonitorHandler) lookupExecution(id string) *mcp.ToolExecution {
	if exec, ok := h.mcpServer.GetExecution(id); ok {
		return exec
	}
	if h.externalMCPMgr != nil {
		if exec, ok := h.externalMCPMgr.GetExecution(id); ok {
			return exec
		}
	}
	if h.db != nil {
		if exec, err := h.db.GetToolExecution(id); err == nil && exec != nil {
			return exec
		}
	}
	return nil
}

// BatchGetToolNames batch-retrieves tool execution summaries (eliminates N+1 requests from the frontend)
func (h *MonitorHandler) BatchGetToolNames(c *gin.Context) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	type executionSummary struct {
		ToolName string `json:"toolName"`
		Status   string `json:"status"`
	}

	result := make(map[string]executionSummary, len(req.IDs))
	for _, id := range req.IDs {
		if !h.monitorExecutionAllowed(c, id) {
			continue
		}
		// first search in the internal MCP server
		if exec, exists := h.mcpServer.GetExecution(id); exists {
			result[id] = executionSummary{ToolName: exec.ToolName, Status: exec.Status}
			continue
		}
		// then search in the external MCP manager
		if h.externalMCPMgr != nil {
			if exec, exists := h.externalMCPMgr.GetExecution(id); exists {
				result[id] = executionSummary{ToolName: exec.ToolName, Status: exec.Status}
				continue
			}
		}
		// finally search in the database
		if h.db != nil {
			if exec, err := h.db.GetToolExecution(id); err == nil && exec != nil {
				result[id] = executionSummary{ToolName: exec.ToolName, Status: exec.Status}
			}
		}
	}

	c.JSON(http.StatusOK, result)
}

// GetStats retrieves statistics info
func (h *MonitorHandler) GetStats(c *gin.Context) {
	topN := 30
	if topStr := c.Query("top"); topStr != "" {
		if t, err := strconv.Atoi(topStr); err == nil && t > 0 && t <= 100 {
			topN = t
		}
	}
	summary, topTools := h.loadStatsSummary(topN)
	c.JSON(http.StatusOK, StatsResponse{
		Summary:  summary,
		TopTools: topTools,
	})
}

// CallsTimelinePoint is a call trend data point
type CallsTimelinePoint struct {
	T       time.Time `json:"t"`
	Total   int       `json:"total"`
	Failed  int       `json:"failed"`
	Blocked int       `json:"blocked"`
}

// CallsTimelineSummary is a call trend summary
type CallsTimelineSummary struct {
	TotalCalls int `json:"totalCalls"`
	Peak       int `json:"peak"`
}

// CallsTimelineResponse is the call trend response
type CallsTimelineResponse struct {
	Range   string               `json:"range"`
	Points  []CallsTimelinePoint `json:"points"`
	Summary CallsTimelineSummary `json:"summary"`
}

type callsTimelineConfig struct {
	rangeKey     string
	duration     time.Duration
	bucketSize   time.Duration
	dailyBuckets bool
}

func parseCallsTimelineRange(raw string) (callsTimelineConfig, bool) {
	switch strings.TrimSpace(raw) {
	case "24h":
		return callsTimelineConfig{rangeKey: "24h", duration: 24 * time.Hour, bucketSize: time.Hour, dailyBuckets: false}, true
	case "30d":
		return callsTimelineConfig{rangeKey: "30d", duration: 30 * 24 * time.Hour, bucketSize: 24 * time.Hour, dailyBuckets: true}, true
	default:
		return callsTimelineConfig{rangeKey: "7d", duration: 7 * 24 * time.Hour, bucketSize: time.Hour, dailyBuckets: false}, true
	}
}

func truncateToBucket(t time.Time, bucketSize time.Duration, dailyBuckets bool) time.Time {
	if dailyBuckets {
		y, m, d := t.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
	}
	return t.Truncate(bucketSize)
}

func buildCallsTimelinePoints(cfg callsTimelineConfig, buckets map[time.Time]struct{ total, failed, blocked int }) []CallsTimelinePoint {
	now := time.Now()
	start := truncateToBucket(now.Add(-cfg.duration), cfg.bucketSize, cfg.dailyBuckets)
	end := truncateToBucket(now, cfg.bucketSize, cfg.dailyBuckets)

	points := make([]CallsTimelinePoint, 0)
	for current := start; !current.After(end); current = current.Add(cfg.bucketSize) {
		val := buckets[current]
		points = append(points, CallsTimelinePoint{
			T:       current,
			Total:   val.total,
			Failed:  val.failed,
			Blocked: val.blocked,
		})
	}
	return points
}

func (h *MonitorHandler) loadCallsTimeline(cfg callsTimelineConfig) []CallsTimelinePoint {
	since := time.Now().Add(-cfg.duration)
	bucketMap := make(map[time.Time]struct{ total, failed, blocked int })

	if h.db != nil {
		dbBuckets, err := h.db.LoadCallsTimeline(since, cfg.dailyBuckets)
		if err != nil {
			h.logger.Warn("failed to load call timeline from database, falling back to in-memory data", zap.Error(err))
		} else {
			for _, b := range dbBuckets {
				key := truncateToBucket(b.BucketTime, cfg.bucketSize, cfg.dailyBuckets)
				entry := bucketMap[key]
				entry.total += b.Total
				entry.failed += b.Failed
				entry.blocked += b.Blocked
				bucketMap[key] = entry
			}
			return buildCallsTimelinePoints(cfg, bucketMap)
		}
	}

	for _, exec := range h.mcpServer.GetAllExecutions() {
		if exec == nil || exec.StartTime.Before(since) {
			continue
		}
		key := truncateToBucket(exec.StartTime, cfg.bucketSize, cfg.dailyBuckets)
		entry := bucketMap[key]
		entry.total++
		if monitorStatusCountsAsFailed(exec.Status) {
			entry.failed++
		} else if exec.Status == mcp.ToolExecutionStatusBlocked {
			entry.blocked++
		}
		bucketMap[key] = entry
	}
	return buildCallsTimelinePoints(cfg, bucketMap)
}

// GetCallsTimeline returns the MCP tool call trend
func (h *MonitorHandler) GetCallsTimeline(c *gin.Context) {
	cfg, _ := parseCallsTimelineRange(c.Query("range"))
	points := h.loadCallsTimeline(cfg)

	summary := CallsTimelineSummary{}
	for _, p := range points {
		summary.TotalCalls += p.Total
		if p.Total > summary.Peak {
			summary.Peak = p.Total
		}
	}

	c.JSON(http.StatusOK, CallsTimelineResponse{
		Range:   cfg.rangeKey,
		Points:  points,
		Summary: summary,
	})
}

// DeleteExecution deletes an execution record
func (h *MonitorHandler) DeleteExecution(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "execution record ID cannot be empty"})
		return
	}
	if !h.monitorExecutionAllowed(c, id) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}

	// If using a database, first get the execution record info, then delete and update statistics
	if h.db != nil {
		// First get the execution record info (for updating statistics)
		exec, err := h.db.GetToolExecution(id)
		if err != nil {
			// If the record is not found, it may have already been deleted; return success directly
			h.logger.Warn("execution record not found, may have already been deleted", zap.String("executionId", id), zap.Error(err))
			c.JSON(http.StatusOK, gin.H{"message": "execution record not found or already deleted"})
			return
		}

		// Delete the execution record
		err = h.db.DeleteToolExecution(id)
		if err != nil {
			h.logger.Error("failed to delete execution record", zap.Error(err), zap.String("executionId", id))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete execution record: " + err.Error()})
			return
		}

		// Update statistics (decrement the corresponding counts)
		totalCalls := 1
		successCalls := 0
		failedCalls := 0
		if monitorStatusCountsAsFailed(exec.Status) {
			failedCalls = 1
		} else if exec.Status == "completed" {
			successCalls = 1
		}

		if exec.ToolName != "" {
			if err := h.db.DecreaseToolStats(exec.ToolName, totalCalls, successCalls, failedCalls); err != nil {
				h.logger.Warn("updateStatistics infofailed", zap.Error(err), zap.String("toolName", exec.ToolName))
				// Do not return an error; the record was already successfully deleted
			}
		}

		h.logger.Info("execution record deleted from database", zap.String("executionId", id), zap.String("toolName", exec.ToolName))
		if h.audit != nil {
			h.audit.RecordOK(c, "tool", "execution_delete", "delete tool execution record", "tool_execution", id, map[string]interface{}{
				"tool_name": exec.ToolName,
			})
		}
		c.JSON(http.StatusOK, gin.H{"message": "execution record deleted"})
		return
	}

	// If not using a database, attempt to delete from in-memory (internal MCP server)
	// Note: in-memory records may have already been cleaned up, so only log here
	h.logger.Info("attempting to delete in-memory execution record", zap.String("executionId", id))
	c.JSON(http.StatusOK, gin.H{"message": "execution record deleted (if it existed)"})
}

// DeleteExecutions deletes execution records in batch
func (h *MonitorHandler) DeleteExecutions(c *gin.Context) {
	var request struct {
		IDs []string `json:"ids"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters: " + err.Error()})
		return
	}

	if len(request.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "execution record ID list cannot be empty"})
		return
	}
	for _, id := range request.IDs {
		if !h.monitorExecutionAllowed(c, id) {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied for one or more execution records"})
			return
		}
	}

	// If using a database, first get execution record info, then delete and update statistics
	if h.db != nil {
		// First get the execution record info (for updating statistics)
		executions, err := h.db.GetToolExecutionsByIds(request.IDs)
		if err != nil {
			h.logger.Error("failed to get execution records", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get execution records: " + err.Error()})
			return
		}

		// Group by tool name to count the amounts to decrement
		toolStats := make(map[string]struct {
			totalCalls   int
			successCalls int
			failedCalls  int
		})

		for _, exec := range executions {
			if exec.ToolName == "" {
				continue
			}

			stats := toolStats[exec.ToolName]
			stats.totalCalls++
			if monitorStatusCountsAsFailed(exec.Status) {
				stats.failedCalls++
			} else if exec.Status == "completed" {
				stats.successCalls++
			}
			toolStats[exec.ToolName] = stats
		}

		// Delete execution records in batch
		err = h.db.DeleteToolExecutions(request.IDs)
		if err != nil {
			h.logger.Error("failed to batch delete execution records", zap.Error(err), zap.Int("count", len(request.IDs)))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to batch delete execution records: " + err.Error()})
			return
		}

		// Update statistics (decrement the corresponding counts)
		for toolName, stats := range toolStats {
			if err := h.db.DecreaseToolStats(toolName, stats.totalCalls, stats.successCalls, stats.failedCalls); err != nil {
				h.logger.Warn("updateStatistics infofailed", zap.Error(err), zap.String("toolName", toolName))
				// Do not return an error; the records were already successfully deleted
			}
		}

		h.logger.Info("batch delete execution records successful", zap.Int("count", len(request.IDs)))
		if h.audit != nil {
			h.audit.RecordOK(c, "tool", "execution_delete_batch", "batch delete tool execution records", "tool_execution", "", map[string]interface{}{
				"count": len(request.IDs),
			})
		}
		c.JSON(http.StatusOK, gin.H{"message": "execution records deleted", "deleted": len(executions)})
		return
	}

	// If not using a database, attempt to delete from in-memory (internal MCP server)
	// Note: in-memory records may have already been cleaned up, so only log here
	h.logger.Info("attempting to batch delete in-memory execution records", zap.Int("count", len(request.IDs)))
	c.JSON(http.StatusOK, gin.H{"message": "execution record deleted (if it existed)"})
}

// normalizeToolNameFilter converts model-side mcp__tool to internal storage mcp::tool.
func normalizeToolNameFilter(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	if strings.Contains(name, "::") {
		return name
	}
	if idx := strings.Index(name, "__"); idx > 0 {
		return name[:idx] + "::" + name[idx+2:]
	}
	return name
}

func toolNameFilterMatches(storedName, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	storedLower := strings.ToLower(storedName)
	filterLower := strings.ToLower(filter)
	if strings.Contains(storedLower, filterLower) {
		return true
	}
	normFilter := strings.ToLower(normalizeToolNameFilter(filter))
	if normFilter != filterLower && strings.Contains(storedLower, normFilter) {
		return true
	}
	return strings.Contains(strings.ReplaceAll(storedLower, "::", "__"), filterLower)
}
