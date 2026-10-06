package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"kestrel/internal/agent"
	"kestrel/internal/database"
	"kestrel/internal/middleware"
	"kestrel/internal/mcp"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// AgentHandler handles agent execution endpoints.
type AgentHandler struct {
	runner   *agent.Runner
	db       *database.DB
	registry *mcp.Registry
	logger   *zap.Logger
}

// NewAgentHandler creates an AgentHandler.
func NewAgentHandler(runner *agent.Runner, db *database.DB, registry *mcp.Registry, logger *zap.Logger) *AgentHandler {
	return &AgentHandler{runner: runner, db: db, registry: registry, logger: logger}
}

// runRequest is the body for POST /api/agent/run.
type runRequest struct {
	Intent       string          `json:"intent" binding:"required"`
	SessionID    string          `json:"session_id"`
	Mode         string          `json:"mode"`
	HITLMode     string          `json:"hitl_mode"`
	AllowedTools []string        `json:"allowed_tools"`
	History      []agent.Message `json:"history"`
}

// Run handles POST /api/agent/run (sync, no streaming).
func (h *AgentHandler) Run(c *gin.Context) {
	userID, _ := c.Get(middleware.CtxUserID)
	uid, _ := userID.(string)

	var req runRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "intent is required"})
		return
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("sess_%d", time.Now().UnixNano())
	}

	params := agent.RunParams{
		SessionID:    sessionID,
		UserID:       uid,
		Intent:       req.Intent,
		History:      req.History,
		Mode:         agent.Mode(req.Mode),
		HITLMode:     req.HITLMode,
		AllowedTools: req.AllowedTools,
	}
	if params.Mode == "" {
		params.Mode = agent.ModeSingle
	}

	result, err := h.runner.Run(c.Request.Context(), params, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// RunStream handles GET /api/agent/stream (WebSocket streaming).
func (h *AgentHandler) RunStream(c *gin.Context) {
	userID, _ := c.Get(middleware.CtxUserID)
	uid, _ := userID.(string)

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Warn("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	// Read the initial RunRequest from the WebSocket.
	var req runRequest
	if err := conn.ReadJSON(&req); err != nil {
		_ = conn.WriteJSON(agent.StepEvent{Type: "error", Content: "invalid request: " + err.Error()})
		return
	}
	if req.Intent == "" {
		_ = conn.WriteJSON(agent.StepEvent{Type: "error", Content: "intent is required"})
		return
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("sess_%d", time.Now().UnixNano())
	}

	events := make(chan agent.StepEvent, 32)
	go func() {
		params := agent.RunParams{
			SessionID:    sessionID,
			UserID:       uid,
			Intent:       req.Intent,
			History:      req.History,
			Mode:         agent.Mode(req.Mode),
			HITLMode:     req.HITLMode,
			AllowedTools: req.AllowedTools,
		}
		if params.Mode == "" {
			params.Mode = agent.ModeSingle
		}
		_, _ = h.runner.Run(c.Request.Context(), params, events)
		close(events)
	}()

	for ev := range events {
		if err := conn.WriteJSON(ev); err != nil {
			h.logger.Warn("WebSocket write error", zap.Error(err))
			return
		}
	}
}

// ListTools handles GET /api/tools.
func (h *AgentHandler) ListTools(c *gin.Context) {
	tools := h.registry.ListTools()
	c.JSON(http.StatusOK, gin.H{"tools": tools, "total": len(tools)})
}

// ExecuteTool handles POST /api/tools/:name/execute — direct (non-agent) tool call.
func (h *AgentHandler) ExecuteTool(c *gin.Context) {
	userID, _ := c.Get(middleware.CtxUserID)
	uid, _ := userID.(string)
	toolName := c.Param("name")

	var req struct {
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Check tool exists.
	if _, ok := h.registry.GetTool(toolName); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("tool %q not found", toolName)})
		return
	}

	// Gather allowed tools from user roles.
	roles, _ := h.db.GetUserRoles(uid)
	var roleAllowed []string
	for _, r := range roles {
		roleAllowed = append(roleAllowed, r.AllowedTools...)
	}

	result, err := h.registry.Execute(c.Request.Context(), toolName, req.Arguments, roleAllowed)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}

// DashboardStats handles GET /api/dashboard/stats.
func (h *AgentHandler) DashboardStats(c *gin.Context) {
	tools := h.registry.ListTools()

	var toolExecCount, assetCount, vulnCount, sessionCount int
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM tool_executions`).Scan(&toolExecCount)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM assets`).Scan(&assetCount)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM vulnerabilities`).Scan(&vulnCount)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM agent_sessions`).Scan(&sessionCount)

	c.JSON(http.StatusOK, gin.H{
		"tools_available": len(tools),
		"tool_executions": toolExecCount,
		"assets":          assetCount,
		"vulnerabilities": vulnCount,
		"agent_sessions":  sessionCount,
	})
}

// SystemInfo handles GET /api/system/info.
func (h *AgentHandler) SystemInfo(c *gin.Context) {
	tools := h.registry.ListTools()
	toolNames := make([]string, len(tools))
	for i, t := range tools {
		toolNames[i] = t.Name
	}
	c.JSON(http.StatusOK, gin.H{
		"version":     "v0.1.0",
		"status":      "under development",
		"tools":       toolNames,
		"agent_modes": []string{"single", "plan_execute", "supervisor"},
		"hitl_modes":  []string{"auto", "require_approval"},
	})
}
