package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"kestrel/internal/database"
	"kestrel/internal/mcp"
	"kestrel/internal/middleware"
	"kestrel/internal/toolguard"
)

// ToolGuardHandler exposes live toolguard config management.
type ToolGuardHandler struct {
	registry *mcp.Registry
	db       *database.DB
}

// NewToolGuardHandler creates a ToolGuardHandler.
func NewToolGuardHandler(registry *mcp.Registry, db *database.DB) *ToolGuardHandler {
	return &ToolGuardHandler{registry: registry, db: db}
}

// GetConfig handles GET /api/tool-guard/config.
func (h *ToolGuardHandler) GetConfig(c *gin.Context) {
	mgr := h.registry.GetGuardManager()
	if mgr == nil {
		c.JSON(http.StatusOK, gin.H{"config": toolguard.Config{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"config": mgr.Config()})
}

// UpdateConfig handles PUT /api/tool-guard/config.
func (h *ToolGuardHandler) UpdateConfig(c *gin.Context) {
	actorID, _ := c.Get(middleware.CtxUserID)
	aID, _ := actorID.(string)
	actorName, _ := c.Get(middleware.CtxUsername)
	aName, _ := actorName.(string)

	var cfg toolguard.Config
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config: " + err.Error()})
		return
	}

	mgr := h.registry.GetGuardManager()
	if mgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "toolguard manager not available"})
		return
	}

	if err := mgr.Update(cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid toolguard config: " + err.Error()})
		return
	}

	_ = h.db.WriteAuditLog(database.AuditParams{
		ActorID: aID, ActorName: aName,
		Action: "update_toolguard_config", Category: "security", Result: "success",
		ResourceType: "toolguard", Message: "toolguard config updated",
		ClientIP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
	c.JSON(http.StatusOK, gin.H{"message": "toolguard config updated", "config": mgr.Config()})
}
