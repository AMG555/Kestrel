package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"kestrel/internal/database"
	"kestrel/internal/middleware"
)

// SessionHandler handles agent session and tool execution endpoints.
type SessionHandler struct {
	db *database.DB
}

// NewSessionHandler creates a SessionHandler.
func NewSessionHandler(db *database.DB) *SessionHandler {
	return &SessionHandler{db: db}
}

// ListSessions handles GET /api/sessions.
func (h *SessionHandler) ListSessions(c *gin.Context) {
	userID, _ := c.Get(middleware.CtxUserID)
	uid, _ := userID.(string)

	p := database.ListAgentSessionsParams{
		ProjectID: c.Query("project_id"),
		Status:    c.Query("status"),
	}
	// Admins can see all sessions; regular users only their own.
	if c.Query("all") != "1" {
		p.UserID = uid
	}
	if lim, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil {
		p.Limit = lim
	}
	if off, err := strconv.Atoi(c.DefaultQuery("offset", "0")); err == nil {
		p.Offset = off
	}

	sessions, total, err := h.db.ListAgentSessions(p)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sessions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessions": sessions, "total": total})
}

// GetSession handles GET /api/sessions/:id.
func (h *SessionHandler) GetSession(c *gin.Context) {
	sess, err := h.db.GetAgentSessionByID(c.Param("id"))
	if err != nil || sess == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Load messages.
	rows, err := h.db.Query(`
		SELECT id,session_id,role,content,reasoning_content,tool_call_ids,created_at
		FROM messages WHERE session_id=? ORDER BY created_at`, c.Param("id"),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load messages"})
		return
	}
	defer rows.Close()

	type msg struct {
		ID               string `json:"id"`
		SessionID        string `json:"session_id"`
		Role             string `json:"role"`
		Content          string `json:"content"`
		ReasoningContent string `json:"reasoning_content,omitempty"`
		ToolCallIDs      string `json:"tool_call_ids"`
		CreatedAt        string `json:"created_at"`
	}
	var messages []msg
	for rows.Next() {
		var m msg
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content,
			&m.ReasoningContent, &m.ToolCallIDs, &m.CreatedAt); err == nil {
			messages = append(messages, m)
		}
	}

	c.JSON(http.StatusOK, gin.H{"session": sess, "messages": messages})
}

// UpdateSession handles PATCH /api/sessions/:id.
func (h *SessionHandler) UpdateSession(c *gin.Context) {
	var req struct {
		Title  string `json:"title"`
		Status string `json:"status"`
		Pinned *bool  `json:"pinned"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if err := h.db.UpdateAgentSession(c.Param("id"), req.Title, req.Status, req.Pinned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update session"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "session updated"})
}

// DeleteSession handles DELETE /api/sessions/:id.
func (h *SessionHandler) DeleteSession(c *gin.Context) {
	if err := h.db.DeleteAgentSession(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete session"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "session deleted"})
}

// ListToolExecutions handles GET /api/tool-executions.
func (h *SessionHandler) ListToolExecutions(c *gin.Context) {
	userID, _ := c.Get(middleware.CtxUserID)
	uid, _ := userID.(string)

	p := database.ListToolExecutionsParams{
		SessionID: c.Query("session_id"),
		ToolName:  c.Query("tool_name"),
		Status:    c.Query("status"),
	}
	if c.Query("all") != "1" {
		p.UserID = uid
	}
	if lim, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil {
		p.Limit = lim
	}
	if off, err := strconv.Atoi(c.DefaultQuery("offset", "0")); err == nil {
		p.Offset = off
	}

	execs, total, err := h.db.ListToolExecutions(p)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tool executions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"executions": execs, "total": total})
}
