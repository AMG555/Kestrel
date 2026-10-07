package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"kestrel/internal/database"
)

// C2Handler manages authorized emulation listeners and beacon interactions.
type C2Handler struct {
	db *database.DB
}

// NewC2Handler creates a C2Handler and seeds default listener if none exists.
func NewC2Handler(db *database.DB) *C2Handler {
	h := &C2Handler{db: db}

	existing, err := db.ListC2Listeners()
	if err == nil && len(existing) == 0 {
		_, _ = db.CreateC2Listener(&database.C2ListenerRecord{
			Name:     "Default HTTP Emulation",
			Protocol: "http",
			BindHost: "127.0.0.1",
			BindPort: 8888,
			Status:   "active",
		})
	}

	return h
}

// ListListeners handles GET /api/c2/listeners.
func (h *C2Handler) ListListeners(c *gin.Context) {
	listeners, err := h.db.ListC2Listeners()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query listeners"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"listeners": listeners})
}

// CreateListener handles POST /api/c2/listeners.
func (h *C2Handler) CreateListener(c *gin.Context) {
	var req struct {
		Name     string `json:"name" binding:"required"`
		Protocol string `json:"protocol"`
		BindHost string `json:"bind_host"`
		BindPort int    `json:"bind_port" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and bind_port required"})
		return
	}
	if req.Protocol == "" {
		req.Protocol = "http"
	}
	if req.BindHost == "" {
		req.BindHost = "0.0.0.0"
	}

	record := &database.C2ListenerRecord{
		Name:     req.Name,
		Protocol: req.Protocol,
		BindHost: req.BindHost,
		BindPort: req.BindPort,
		Status:   "active",
	}

	saved, err := h.db.CreateC2Listener(record)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create listener"})
		return
	}

	c.JSON(http.StatusCreated, saved)
}

// ListBeacons handles GET /api/c2/beacons.
func (h *C2Handler) ListBeacons(c *gin.Context) {
	beacons, err := h.db.ListC2Beacons()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query beacons"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"beacons": beacons})
}

// QueueTask handles POST /api/c2/beacons/:id/tasks.
func (h *C2Handler) QueueTask(c *gin.Context) {
	beaconID := c.Param("id")
	var req struct {
		Type    string `json:"type" binding:"required"`
		Payload string `json:"payload" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type and payload required"})
		return
	}

	task := &database.C2TaskRecord{
		BeaconID: beaconID,
		Type:     req.Type,
		Payload:  req.Payload,
		Status:   "pending",
	}

	saved, err := h.db.CreateC2Task(task)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to queue task"})
		return
	}

	c.JSON(http.StatusCreated, saved)
}
