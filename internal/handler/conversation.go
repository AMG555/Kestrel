package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"kestrel/internal/audit"
	"kestrel/internal/database"
	"kestrel/internal/security"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ConversationTaskStopper cancels in-flight agent work when a conversation is removed.
type ConversationTaskStopper interface {
	CancelRunningTaskForConversation(conversationID string)
}

// ConversationTaskStateProvider reports whether the in-memory agent task for
// a conversation is still genuinely running. Plan files may survive a service
// restart or cancellation, so their status alone is not authoritative.
type ConversationTaskStateProvider interface {
	ConversationTaskRuntimeState(conversationID string) (running bool, startedAt time.Time)
}

// ConversationHandler handles conversation requests.
type ConversationHandler struct {
	db          *database.DB
	logger      *zap.Logger
	audit       *audit.Service
	taskStopper ConversationTaskStopper
	taskState   ConversationTaskStateProvider
}

// SetAudit wires platform audit logging.
func (h *ConversationHandler) SetAudit(s *audit.Service) {
	h.audit = s
}

// SetTaskStopper wires cancellation of in-flight agent tasks on conversation delete.
func (h *ConversationHandler) SetTaskStopper(stopper ConversationTaskStopper) {
	h.taskStopper = stopper
}

// SetTaskStateProvider wires the live agent task registry used by supplemental
// conversation UI such as the agent-maintained plan list.
func (h *ConversationHandler) SetTaskStateProvider(provider ConversationTaskStateProvider) {
	h.taskState = provider
}

// NewConversationHandler creates a new conversation handler.
func NewConversationHandler(db *database.DB, logger *zap.Logger) *ConversationHandler {
	return &ConversationHandler{
		db:     db,
		logger: logger,
	}
}

// CreateConversationRequest create conversationrequest
type CreateConversationRequest struct {
	Title     string `json:"title"`
	ProjectID string `json:"projectId,omitempty"`
}

// SetConversationProjectRequest sets the project a conversation belongs to.
type SetConversationProjectRequest struct {
	ProjectID string `json:"projectId"` // Empty string means unbind.
}

// CreateConversation creates a new conversation
func (h *ConversationHandler) CreateConversation(c *gin.Context) {
	var req CreateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	title := req.Title
	if title == "" {
		title = "New conversation"
	}

	meta := audit.ConversationCreateMetaFromGin(c, "api")
	meta.ProjectID = strings.TrimSpace(req.ProjectID)
	if !h.conversationProjectAllowed(c, meta.ProjectID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access deniedtarget project"})
		return
	}
	conv, err := h.db.CreateConversation(title, meta)
	if err != nil {
		h.logger.Error("create conversation failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if session, ok := security.CurrentSession(c); ok {
		_ = h.db.SetResourceOwner("conversation", conv.ID, session.UserID)
		_ = h.db.AssignResourceToUser(session.UserID, "conversation", conv.ID)
		if conv.ProjectID != "" {
			_ = h.db.AssignResourceToUser(session.UserID, "project", conv.ProjectID)
		}
	}

	c.JSON(http.StatusOK, conv)
}

// SetConversationProject sets or clears the project bound to a conversation.
func (h *ConversationHandler) SetConversationProject(c *gin.Context) {
	id := c.Param("id")
	var req SetConversationProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := h.db.GetConversation(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		return
	}
	projectID := strings.TrimSpace(req.ProjectID)
	if !h.conversationProjectAllowed(c, projectID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access deniedtarget project"})
		return
	}
	if err := h.db.SetConversationProjectID(id, projectID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "projectId": projectID})
}

func (h *ConversationHandler) conversationProjectAllowed(c *gin.Context, projectID string) bool {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return true
	}
	session, ok := security.CurrentSession(c)
	if !ok {
		return false
	}
	return h.db.UserCanAccessResource(session.UserID, session.Scope, "project", projectID)
}

// ListConversations list conversations
func (h *ConversationHandler) ListConversations(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")
	search := c.Query("search") // Get search parameter.
	projectID := strings.TrimSpace(c.Query("project_id"))

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	sortBy := strings.TrimSpace(c.Query("sort_by"))
	session, _ := security.CurrentSession(c)

	var conversations []*database.Conversation
	var total int
	var err error
	conversations, err = h.db.ListConversationsForAccess(limit, offset, search, sortBy, projectID, session.UserID, session.Scope)
	if err == nil {
		total, err = h.db.CountConversationsForAccess(search, projectID, session.UserID, session.Scope)
	}
	if err != nil {
		h.logger.Error("get conversation list failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if conversations == nil {
		conversations = []*database.Conversation{}
	}
	c.JSON(http.StatusOK, gin.H{
		"conversations": conversations,
		"total":         total,
		"limit":         limit,
		"offset":        offset,
	})
}

// UpdateConversationPinnedRequest is the request to update a conversation's pinned status.
type UpdateConversationPinnedRequest struct {
	Pinned bool `json:"pinned"`
}

// UpdateConversationPinned updates a conversation's pinned status.
func (h *ConversationHandler) UpdateConversationPinned(c *gin.Context) {
	conversationID := c.Param("id")
	session, ok := security.CurrentSession(c)
	if !ok || !h.db.UserCanAccessResource(session.UserID, session.Scope, "conversation", conversationID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}

	var req UpdateConversationPinnedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.UpdateConversationPinned(conversationID, req.Pinned); err != nil {
		h.logger.Error("failed to update conversation pinned status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "update successful"})
}

// GetConversation retrieves a conversation
func (h *ConversationHandler) GetConversation(c *gin.Context) {
	id := c.Param("id")

	// Lightweight loading by default; full process details are fetched on demand when the user expands them.
	// include_process_details=1/true returns all processDetails (for backward compatibility).
	includeStr := c.DefaultQuery("include_process_details", "0")
	include := includeStr == "1" || includeStr == "true" || includeStr == "yes"

	var (
		conv *database.Conversation
		err  error
	)
	if include {
		conv, err = h.db.GetConversation(id)
	} else {
		conv, err = h.db.GetConversationLite(id)
	}
	if err != nil {
		h.logger.Error("failed to get conversation", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		return
	}

	c.JSON(http.StatusOK, conv)
}

// GetConversationPlanTasks returns the task list maintained by the agent's
// TaskCreate/TaskUpdate tools for this conversation.
func (h *ConversationHandler) GetConversationPlanTasks(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	session, ok := security.CurrentSession(c)
	if !ok || !h.db.UserCanAccessResource(session.UserID, session.Scope, "conversation", id) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this conversation"})
		return
	}
	if _, err := h.db.GetConversationLite(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		return
	}
	running := false
	startedAt := time.Time{}
	if h.taskState != nil {
		running, startedAt = h.taskState.ConversationTaskRuntimeState(id)
	}
	if !running {
		c.JSON(http.StatusOK, gin.H{
			"tasks": []database.ConversationPlanTask{}, "total": 0,
			"completed": 0, "activeStep": 0, "running": false,
		})
		return
	}
	tasks, err := h.db.ListConversationPlanTasksSince(id, startedAt)
	if err != nil {
		h.logger.Error("failed to get conversation task list", zap.String("conversationId", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get task list"})
		return
	}

	completed := 0
	activeStep := 0
	for i, task := range tasks {
		status := strings.ToLower(strings.TrimSpace(task.Status))
		if status == "completed" {
			completed++
		}
		if activeStep == 0 && status == "in_progress" {
			activeStep = i + 1
		}
	}
	if activeStep == 0 {
		for i, task := range tasks {
			if strings.ToLower(strings.TrimSpace(task.Status)) != "completed" {
				activeStep = i + 1
				break
			}
		}
	}
	if activeStep == 0 && len(tasks) > 0 {
		activeStep = len(tasks)
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks":      tasks,
		"total":      len(tasks),
		"completed":  completed,
		"activeStep": activeStep,
		"running":    true,
	})
}

const (
	defaultProcessDetailsPageLimit = 50
	maxProcessDetailsPageLimit     = 500
)

// GetMessageProcessDetails returns process details for a specific message (loaded on demand).
// Query parameters:
//   - summary=1: returns only the summary (total / iterationCount / maxIteration).
//   - limit + offset: paginate process details (defaults to 50 if limit is not specified).
//   - anchorId: returns the page containing the process detail anchor, suitable for precise navigation from a tool button.
//   - full=1: explicitly returns all process details (for export/legacy integrations; not recommended for UI expansion).
func (h *ConversationHandler) GetMessageProcessDetails(c *gin.Context) {
	messageID := c.Param("id")
	if messageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message id required"})
		return
	}

	summaryStr := strings.TrimSpace(c.Query("summary"))
	if summaryStr == "1" || strings.EqualFold(summaryStr, "true") || strings.EqualFold(summaryStr, "yes") {
		summary, err := h.db.GetProcessDetailsSummary(messageID)
		if err != nil {
			h.logger.Error("failed to get process details summary", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"summary": summary})
		return
	}

	fullStr := strings.TrimSpace(c.Query("full"))
	if fullStr == "1" || strings.EqualFold(fullStr, "true") || strings.EqualFold(fullStr, "yes") {
		details, err := h.db.GetProcessDetails(messageID)
		if err != nil {
			h.logger.Error("failed to get process details", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		details = database.DedupeConsecutiveProcessDetails(details)
		out := processDetailsToJSON(h.logger, h.db, details, true)
		c.JSON(http.StatusOK, gin.H{
			"processDetails": out,
			"total":          len(out),
			"offset":         0,
			"limit":          len(out),
			"hasMore":        false,
		})
		return
	}

	limitStr := strings.TrimSpace(c.Query("limit"))
	limit := defaultProcessDetailsPageLimit
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		limit = parsedLimit
	}
	if limit > maxProcessDetailsPageLimit {
		limit = maxProcessDetailsPageLimit
	}
	offset, _ := strconv.Atoi(strings.TrimSpace(c.Query("offset")))
	if offset < 0 {
		offset = 0
	}
	anchorID := strings.TrimSpace(c.Query("anchorId"))
	if anchorID != "" {
		anchorOffset, err := h.db.GetProcessDetailOffset(messageID, anchorID)
		if err != nil {
			h.logger.Warn("failed to get process detail anchor position", zap.Error(err), zap.String("messageID", messageID), zap.String("anchorID", anchorID))
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		offset = anchorOffset - limit/3
		if offset < 0 {
			offset = 0
		}
	}

	details, total, err := h.db.GetProcessDetailsPage(messageID, limit, offset)
	if err != nil {
		h.logger.Error("failed to paginate process details", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	details = database.DedupeConsecutiveProcessDetails(details)
	out := processDetailsToJSON(h.logger, h.db, details, false)
	// A page may end between tool_call and tool_result. Return the full-history
	// execution summary so the UI can render terminal status without pretending
	// that an unloaded result is still running.
	summary, summaryErr := h.db.GetProcessDetailsSummary(messageID)
	if summaryErr != nil {
		h.logger.Warn("failed to get paginated tool execution status", zap.Error(summaryErr), zap.String("messageID", messageID))
	}
	var toolExecutions []database.ProcessDetailsToolExecution
	if summary != nil {
		toolExecutions = summary.ToolExecutions
	}
	c.JSON(http.StatusOK, gin.H{
		"processDetails": out,
		"toolExecutions": toolExecutions,
		"total":          total,
		"offset":         offset,
		"limit":          limit,
		"hasMore":        offset+len(out) < total,
	})
}

// GetProcessDetail returns a single complete process detail. The list endpoint omits tool payloads by default; they are fetched here when the user opens a specific tool entry.
func (h *ConversationHandler) GetProcessDetail(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "process detail id required"})
		return
	}
	detail, err := h.db.GetProcessDetailByID(id)
	if err != nil {
		h.logger.Error("failed to get process details", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "process detail not found"})
		return
	}
	out := processDetailsToJSON(h.logger, h.db, []database.ProcessDetail{*detail}, true)
	if len(out) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "process detail not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"processDetail": out[0]})
}

func processDetailsToJSON(logger *zap.Logger, db *database.DB, details []database.ProcessDetail, includeToolPayload bool) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(details))
	for _, d := range details {
		var data interface{}
		if d.Data != "" {
			if err := json.Unmarshal([]byte(d.Data), &data); err != nil {
				logger.Warn("failed to parse process detail data", zap.Error(err))
			}
		}
		if m, ok := data.(map[string]interface{}); ok {
			enrichEmptyToolCallArgumentsFromExecution(logger, db, d, m)
		}
		if !includeToolPayload {
			data = summarizeProcessDetailData(d.EventType, data)
		}
		out = append(out, map[string]interface{}{
			"id":             d.ID,
			"messageId":      d.MessageID,
			"conversationId": d.ConversationID,
			"eventType":      d.EventType,
			"message":        d.Message,
			"data":           data,
			"createdAt":      d.CreatedAt,
		})
	}
	return out
}

func enrichEmptyToolCallArgumentsFromExecution(logger *zap.Logger, db *database.DB, detail database.ProcessDetail, data map[string]interface{}) {
	if db == nil || detail.EventType != "tool_call" || !toolCallArgumentsEmpty(data) {
		return
	}
	toolName := strings.TrimSpace(fmt.Sprint(data["toolName"]))
	if toolName == "" || detail.ConversationID == "" || detail.CreatedAt.IsZero() {
		return
	}
	execID, args, err := db.FindNearestToolExecutionArguments(detail.ConversationID, toolName, detail.CreatedAt, 5*time.Second)
	if err != nil {
		if logger != nil {
			logger.Debug("could not supplement process detail parameters from tool execution record",
				zap.Error(err),
				zap.String("processDetailId", detail.ID),
				zap.String("toolName", toolName))
		}
		return
	}
	if len(args) == 0 {
		return
	}
	data["argumentsObj"] = args
	if b, err := json.Marshal(args); err == nil {
		data["arguments"] = string(b)
	}
	if strings.TrimSpace(execID) != "" {
		data["executionId"] = strings.TrimSpace(execID)
	}
}

func toolCallArgumentsEmpty(data map[string]interface{}) bool {
	if data == nil {
		return true
	}
	if args, ok := data["argumentsObj"].(map[string]interface{}); ok && len(args) > 0 {
		return false
	}
	if raw, ok := data["arguments"]; ok {
		s := strings.TrimSpace(fmt.Sprint(raw))
		return s == "" || s == "{}" || s == "null"
	}
	return true
}

func summarizeProcessDetailData(eventType string, data interface{}) interface{} {
	m, ok := data.(map[string]interface{})
	if !ok || (eventType != "tool_call" && eventType != "tool_result") {
		return data
	}
	allow := map[string]bool{
		"toolName": true, "toolCallId": true, "index": true, "total": true,
		"arguments": true, "argumentsObj": true,
		"success": true, "isError": true, "executionId": true,
		"einoAgent": true, "einoRole": true, "einoScope": true, "orchestration": true,
		"agentFacing": true,
		"status":      true, "modelFacingIsError": true, "resultPreview": true,
	}
	out := make(map[string]interface{}, len(allow)+1)
	for k, v := range m {
		if allow[k] {
			out[k] = v
		}
	}
	out["_payloadDeferred"] = true
	return out
}

// UpdateConversationRequest updateconversationrequest
type UpdateConversationRequest struct {
	Title string `json:"title"`
}

// UpdateConversation updateconversation
func (h *ConversationHandler) UpdateConversation(c *gin.Context) {
	id := c.Param("id")

	var req UpdateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title cannot be empty"})
		return
	}

	if err := h.db.UpdateConversationTitle(id, req.Title); err != nil {
		h.logger.Error("update conversation failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return the updated conversation.
	conv, err := h.db.GetConversation(id)
	if err != nil {
		h.logger.Error("failed to get updated conversation", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, conv)
}

// DeleteConversation delete conversation
func (h *ConversationHandler) DeleteConversation(c *gin.Context) {
	id := c.Param("id")

	if h.taskStopper != nil {
		h.taskStopper.CancelRunningTaskForConversation(id)
	}

	if err := h.db.DeleteConversation(id); err != nil {
		h.logger.Error("delete conversation failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.audit != nil {
		h.audit.Record(c, audit.Entry{
			Category:     "conversation",
			Action:       "delete",
			Result:       "success",
			ResourceType: "conversation",
			ResourceID:   id,
			Message:      "delete conversation",
		})
	}

	c.JSON(http.StatusOK, gin.H{"message": "deletion successful"})
}

// DeleteTurnRequest deletes one conversation turn (POST /api/conversations/:id/delete-turn).
type DeleteTurnRequest struct {
	MessageID string `json:"messageId"`
}

// DeleteConversationTurn deletes the turn containing the anchor message (from that turn's user message up to the next user message), and clears last_react_*.
func (h *ConversationHandler) DeleteConversationTurn(c *gin.Context) {
	conversationID := c.Param("id")
	if conversationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversation id required"})
		return
	}

	var req DeleteTurnRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.MessageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "messageId required"})
		return
	}

	if _, err := h.db.GetConversation(conversationID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		return
	}

	deletedIDs, err := h.db.DeleteConversationTurn(conversationID, req.MessageID)
	if err != nil {
		h.logger.Warn("failed to delete conversation turn",
			zap.String("conversationId", conversationID),
			zap.String("messageId", req.MessageID),
			zap.Error(err),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if h.audit != nil {
		h.audit.RecordOK(c, "conversation", "delete_turn", "delete conversation turn", "conversation", conversationID, map[string]interface{}{
			"message_id": req.MessageID,
			"deleted":    len(deletedIDs),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"deletedMessageIds": deletedIDs,
		"message":           "ok",
	})
}
