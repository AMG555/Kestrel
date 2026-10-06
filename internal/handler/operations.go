package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"kestrel/internal/agent"
	"kestrel/internal/database"
	"kestrel/internal/middleware"
	"kestrel/internal/mcp"
)

// BatchHandler handles batch task queue endpoints.
type BatchHandler struct {
	db       *database.DB
	runner   *agent.Runner
	registry *mcp.Registry
	logger   *zap.Logger
}

// NewBatchHandler creates a BatchHandler.
func NewBatchHandler(db *database.DB, runner *agent.Runner, registry *mcp.Registry, logger *zap.Logger) *BatchHandler {
	return &BatchHandler{db: db, runner: runner, registry: registry, logger: logger}
}

// ListQueues handles GET /api/batch/queues.
func (h *BatchHandler) ListQueues(c *gin.Context) {
	queues, err := h.db.ListBatchQueues(c.Query("project_id"), 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list queues"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"queues": queues, "total": len(queues)})
}

// createQueueReq is the body for POST /api/batch/queues.
type createQueueReq struct {
	ProjectID string   `json:"project_id"`
	Title     string   `json:"title" binding:"required"`
	RoleID    string   `json:"role_id"`
	AgentMode string   `json:"agent_mode"`
	HITLMode  string   `json:"hitl_mode"`
	Intents   []string `json:"intents"`
}

// CreateQueue handles POST /api/batch/queues.
func (h *BatchHandler) CreateQueue(c *gin.Context) {
	callerID, _ := c.Get(middleware.CtxUserID)
	uid, _ := callerID.(string)

	var req createQueueReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	if req.AgentMode == "" {
		req.AgentMode = "single"
	}
	if req.HITLMode == "" {
		req.HITLMode = "auto"
	}

	q, err := h.db.CreateBatchQueue(&database.BatchTaskQueue{
		ProjectID: req.ProjectID,
		Title:     req.Title,
		RoleID:    req.RoleID,
		AgentMode: req.AgentMode,
		HITLMode:  req.HITLMode,
		CreatedBy: uid,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create queue"})
		return
	}

	// Add all intents as tasks.
	for _, intent := range req.Intents {
		if _, err := h.db.AddBatchTask(q.ID, intent); err != nil {
			h.logger.Warn("failed to add batch task", zap.Error(err))
		}
	}

	c.JSON(http.StatusCreated, gin.H{"queue": q})
}

// GetQueue handles GET /api/batch/queues/:id.
func (h *BatchHandler) GetQueue(c *gin.Context) {
	q, err := h.db.GetBatchQueue(c.Param("id"))
	if err != nil || q == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue not found"})
		return
	}
	tasks, _ := h.db.GetBatchTasks(q.ID)
	c.JSON(http.StatusOK, gin.H{"queue": q, "tasks": tasks})
}

// RunQueue handles POST /api/batch/queues/:id/run — starts executing the queue.
func (h *BatchHandler) RunQueue(c *gin.Context) {
	callerID, _ := c.Get(middleware.CtxUserID)
	uid, _ := callerID.(string)
	queueID := c.Param("id")

	q, err := h.db.GetBatchQueue(queueID)
	if err != nil || q == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue not found"})
		return
	}
	if q.Status == "running" {
		c.JSON(http.StatusConflict, gin.H{"error": "queue is already running"})
		return
	}

	tasks, err := h.db.GetBatchTasks(queueID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load tasks"})
		return
	}

	if err := h.db.UpdateBatchQueueStatus(queueID, "running", 0); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start queue"})
		return
	}

	// Execute tasks sequentially in the background.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()

		for i, task := range tasks {
			if task.Status == "completed" {
				continue
			}
			_ = h.db.UpdateBatchTaskStatus(task.ID, "running", "", "", "")
			_ = h.db.UpdateBatchQueueStatus(queueID, "running", i)

			sessionID := fmt.Sprintf("batch_%s_%d", queueID, time.Now().UnixNano())
			result, err := h.runner.Run(ctx, agent.RunParams{
				SessionID:    sessionID,
				UserID:       uid,
				Intent:       task.Intent,
				Mode:         agent.Mode(q.AgentMode),
				HITLMode:     q.HITLMode,
			}, nil)

			if err != nil {
				_ = h.db.UpdateBatchTaskStatus(task.ID, "failed", "", err.Error(), sessionID)
			} else {
				_ = h.db.UpdateBatchTaskStatus(task.ID, "completed", result.FinalAnswer, result.Error, sessionID)
			}
		}
		_ = h.db.UpdateBatchQueueStatus(queueID, "completed", len(tasks))
	}()

	c.JSON(http.StatusAccepted, gin.H{"message": "queue started", "queue_id": queueID})
}

// CancelQueue handles POST /api/batch/queues/:id/cancel.
func (h *BatchHandler) CancelQueue(c *gin.Context) {
	if err := h.db.UpdateBatchQueueStatus(c.Param("id"), "cancelled", -1); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cancel queue"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "queue cancelled"})
}

// DeleteQueue handles DELETE /api/batch/queues/:id.
func (h *BatchHandler) DeleteQueue(c *gin.Context) {
	if err := h.db.DeleteBatchQueue(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete queue"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "queue deleted"})
}

// HITLHandler handles human-in-the-loop approval endpoints.
type HITLHandler struct {
	db *database.DB
}

// NewHITLHandler creates a HITLHandler.
func NewHITLHandler(db *database.DB) *HITLHandler {
	return &HITLHandler{db: db}
}

// ListPending handles GET /api/hitl/pending.
func (h *HITLHandler) ListPending(c *gin.Context) {
	items, err := h.db.ListPendingHITL()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list pending approvals"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

// Decide handles POST /api/hitl/:id/decide.
func (h *HITLHandler) Decide(c *gin.Context) {
	deciderID, _ := c.Get(middleware.CtxUserID)
	uid, _ := deciderID.(string)

	var req struct {
		Decision string `json:"decision" binding:"required"` // approved | rejected
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.Decision != "approved" && req.Decision != "rejected") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "decision must be 'approved' or 'rejected'"})
		return
	}

	item, err := h.db.GetHITLPending(c.Param("id"))
	if err != nil || item == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "approval request not found"})
		return
	}
	if item.Status != "pending" {
		c.JSON(http.StatusConflict, gin.H{"error": "approval request is no longer pending"})
		return
	}

	if err := h.db.DecideHITL(c.Param("id"), req.Decision, uid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record decision"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "decision recorded", "decision": req.Decision})
}

// ConversationHandler handles conversation management endpoints.
type ConversationHandler struct {
	db *database.DB
}

// NewConversationHandler creates a ConversationHandler.
func NewConversationHandler(db *database.DB) *ConversationHandler {
	return &ConversationHandler{db: db}
}

// ListConversations handles GET /api/conversations.
func (h *ConversationHandler) ListConversations(c *gin.Context) {
	userID, _ := c.Get(middleware.CtxUserID)
	uid, _ := userID.(string)
	convs, err := h.db.ListConversations(uid, c.Query("project_id"), 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list conversations"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"conversations": convs, "total": len(convs)})
}

// CreateConversation handles POST /api/conversations.
func (h *ConversationHandler) CreateConversation(c *gin.Context) {
	userID, _ := c.Get(middleware.CtxUserID)
	uid, _ := userID.(string)

	var req struct {
		ProjectID string `json:"project_id"`
		Title     string `json:"title"`
		AgentMode string `json:"agent_mode"`
		RoleID    string `json:"role_id"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Title == "" {
		req.Title = "New conversation"
	}

	conv, err := h.db.CreateConversation(&database.Conversation{
		ProjectID: req.ProjectID,
		UserID:    uid,
		Title:     req.Title,
		AgentMode: req.AgentMode,
		RoleID:    req.RoleID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create conversation"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"conversation": conv})
}

// UpdateConversation handles PATCH /api/conversations/:id.
func (h *ConversationHandler) UpdateConversation(c *gin.Context) {
	var req struct {
		Title  string `json:"title"`
		Pinned *bool  `json:"pinned"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	pinned := false
	if req.Pinned != nil {
		pinned = *req.Pinned
	}
	if err := h.db.UpdateConversation(c.Param("id"), req.Title, pinned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update conversation"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "conversation updated"})
}

// DeleteConversation handles DELETE /api/conversations/:id.
func (h *ConversationHandler) DeleteConversation(c *gin.Context) {
	if err := h.db.DeleteConversation(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete conversation"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "conversation deleted"})
}

// GetConversationMessages handles GET /api/conversations/:id/messages.
func (h *ConversationHandler) GetConversationMessages(c *gin.Context) {
	rows, err := h.db.Query(`
		SELECT id,session_id,role,content,tool_call_ids,created_at
		FROM messages WHERE session_id=? ORDER BY created_at`, c.Param("id"),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load messages"})
		return
	}
	defer rows.Close()

	type msg struct {
		ID         string    `json:"id"`
		SessionID  string    `json:"session_id"`
		Role       string    `json:"role"`
		Content    string    `json:"content"`
		ToolCallIDs string   `json:"tool_call_ids"`
		CreatedAt  time.Time `json:"created_at"`
	}
	var msgs []msg
	for rows.Next() {
		var m msg
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.ToolCallIDs, &m.CreatedAt); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	c.JSON(http.StatusOK, gin.H{"messages": msgs, "total": len(msgs)})
}

// MCPServerHandler handles MCP server management endpoints.
type MCPServerHandler struct {
	db *database.DB
}

// NewMCPServerHandler creates an MCPServerHandler.
func NewMCPServerHandler(db *database.DB) *MCPServerHandler {
	return &MCPServerHandler{db: db}
}

// ListMCPServers handles GET /api/mcp/servers.
func (h *MCPServerHandler) ListMCPServers(c *gin.Context) {
	servers, err := h.db.ListMCPServers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list MCP servers"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"servers": servers, "total": len(servers)})
}

// UpsertMCPServer handles POST /api/mcp/servers.
func (h *MCPServerHandler) UpsertMCPServer(c *gin.Context) {
	var s database.MCPServer
	if err := c.ShouldBindJSON(&s); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid MCP server data"})
		return
	}
	result, err := h.db.UpsertMCPServer(&s)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save MCP server"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"server": result})
}

// DeleteMCPServer handles DELETE /api/mcp/servers/:id.
func (h *MCPServerHandler) DeleteMCPServer(c *gin.Context) {
	if err := h.db.DeleteMCPServer(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete MCP server"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "server deleted"})
}

// ResetCircuit handles POST /api/mcp/servers/:id/reset-circuit.
func (h *MCPServerHandler) ResetCircuit(c *gin.Context) {
	if err := h.db.ResetMCPServerCircuit(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset circuit"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "circuit reset"})
}
