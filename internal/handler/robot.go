package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"kestrel/internal/robot"
)

// RobotHandler manages notification bot settings and test dispatches.
type RobotHandler struct {
	service *robot.Service
}

// NewRobotHandler creates a new robot handler.
func NewRobotHandler(service *robot.Service) *RobotHandler {
	return &RobotHandler{service: service}
}

// GetConfig returns the current notification bot configuration.
// GET /api/robots/config
func (h *RobotHandler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.GetConfig())
}

// UpdateConfig updates notification bot settings.
// PUT /api/robots/config
func (h *RobotHandler) UpdateConfig(c *gin.Context) {
	var cfg robot.Config
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.service.UpdateConfig(cfg)
	c.JSON(http.StatusOK, gin.H{"status": "ok", "config": h.service.GetConfig()})
}

// TestNotification sends a test alert to a specific channel.
// POST /api/robots/test
func (h *RobotHandler) TestNotification(c *gin.Context) {
	var req struct {
		Channel string `json:"channel"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Channel = "all"
	}
	if err := h.service.SendTest(req.Channel); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "Test notification dispatched"})
}
