package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"kestrel/internal/audit"
	"kestrel/internal/c2"
	"kestrel/internal/database"
	"kestrel/internal/security"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// C2Handler handles C2-related REST APIs (manager can be set to nil at runtime to disable C2)
type C2Handler struct {
	mgrPtr atomic.Pointer[c2.Manager]
	logger *zap.Logger
	audit  *audit.Service
}

// SetAudit wires platform audit logging.
func (h *C2Handler) SetAudit(s *audit.Service) {
	h.audit = s
}

// NewC2Handler creates a C2 handler; manager may be nil (when the feature is disabled)
func NewC2Handler(manager *c2.Manager, logger *zap.Logger) *C2Handler {
	h := &C2Handler{logger: logger}
	if manager != nil {
		h.mgrPtr.Store(manager)
	}
	return h
}

func (h *C2Handler) mgr() *c2.Manager {
	return h.mgrPtr.Load()
}

// SetManager switches or clears the C2 Manager at runtime (synced with App start/stop)
func (h *C2Handler) SetManager(m *c2.Manager) {
	h.mgrPtr.Store(m)
}

// ============================================================================
// Listener API
// ============================================================================

// ListListeners returns the listener list
func (h *C2Handler) ListListeners(c *gin.Context) {
	listeners, err := h.mgr().DB().ListC2ListenersForAccess(c2AccessFromContext(c), c.Query("project_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// remove sensitive fields
	for _, l := range listeners {
		l.EncryptionKey = ""
		l.ImplantToken = ""
	}
	c.JSON(http.StatusOK, gin.H{"listeners": listeners})
}

// CreateListener creates a listener
func (h *C2Handler) CreateListener(c *gin.Context) {
	var req struct {
		Name         string             `json:"name"`
		ProjectID    string             `json:"project_id,omitempty"`
		Type         string             `json:"type"`
		BindHost     string             `json:"bind_host"`
		BindPort     int                `json:"bind_port"`
		ProfileID    string             `json:"profile_id,omitempty"`
		Remark       string             `json:"remark,omitempty"`
		CallbackHost string             `json:"callback_host,omitempty"`
		Config       *c2.ListenerConfig `json:"config,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input := c2.CreateListenerInput{
		Name:         req.Name,
		ProjectID:    req.ProjectID,
		Type:         req.Type,
		BindHost:     req.BindHost,
		BindPort:     req.BindPort,
		ProfileID:    req.ProfileID,
		Remark:       req.Remark,
		Config:       req.Config,
		CallbackHost: strings.TrimSpace(req.CallbackHost),
	}
	if !h.canAccessProject(c, input.ProjectID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "project access denied"})
		return
	}

	listener, err := h.mgr().CreateListener(input)
	if err != nil {
		code := http.StatusInternalServerError
		if e, ok := err.(*c2.CommonError); ok {
			code = e.HTTP
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	if session, ok := security.CurrentSession(c); ok {
		listener.OwnerUserID = session.UserID
		_ = h.mgr().DB().SetResourceOwner("c2_listener", listener.ID, session.UserID)
		_ = h.mgr().DB().AssignResourceToUser(session.UserID, "c2_listener", listener.ID)
	}
	implantToken := listener.ImplantToken
	listener.EncryptionKey = ""
	listener.ImplantToken = ""
	if h.audit != nil {
		h.audit.RecordOK(c, "c2", "listener_create", "create C2 listener", "c2_listener", listener.ID, map[string]interface{}{
			"name": listener.Name, "bind": listener.BindHost, "port": listener.BindPort,
		})
	}
	c.JSON(http.StatusOK, gin.H{"listener": listener, "implant_token": implantToken})
}

// GetListener retrieves a single listener
func (h *C2Handler) GetListener(c *gin.Context) {
	id := c.Param("id")
	listener, err := h.mgr().DB().GetC2Listener(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if listener == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "listener not found"})
		return
	}
	listener.EncryptionKey = ""
	listener.ImplantToken = ""
	c.JSON(http.StatusOK, gin.H{"listener": listener})
}

// UpdateListener updates a listener
func (h *C2Handler) UpdateListener(c *gin.Context) {
	id := c.Param("id")
	listener, err := h.mgr().DB().GetC2Listener(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if listener == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "listener not found"})
		return
	}

	var req struct {
		Name         string             `json:"name"`
		ProjectID    string             `json:"project_id"`
		BindHost     string             `json:"bind_host"`
		BindPort     int                `json:"bind_port"`
		ProfileID    string             `json:"profile_id"`
		Remark       string             `json:"remark"`
		CallbackHost *string            `json:"callback_host"`
		Config       *c2.ListenerConfig `json:"config,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "listener name is required"})
		return
	}
	if err := c2.SafeBindPort(req.BindPort); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// if the listener is running, key fields cannot be modified
	if h.mgr().IsListenerRunning(id) {
		if req.BindHost != listener.BindHost || req.BindPort != listener.BindPort {
			c.JSON(http.StatusConflict, gin.H{"error": "cannot modify bind address while listener is running"})
			return
		}
	}

	listener.Name = req.Name
	listener.ProjectID = strings.TrimSpace(req.ProjectID)
	listener.BindHost = req.BindHost
	listener.BindPort = req.BindPort
	listener.ProfileID = req.ProfileID
	listener.Remark = req.Remark
	if req.Config != nil {
		cfgJSON, _ := json.Marshal(req.Config)
		listener.ConfigJSON = string(cfgJSON)
	}
	if !h.canAccessProject(c, listener.ProjectID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "project access denied"})
		return
	}
	if req.CallbackHost != nil {
		cfg := &c2.ListenerConfig{}
		raw := strings.TrimSpace(listener.ConfigJSON)
		if raw == "" {
			raw = "{}"
		}
		_ = json.Unmarshal([]byte(raw), cfg)
		cfg.CallbackHost = strings.TrimSpace(*req.CallbackHost)
		cfg.ApplyDefaults()
		cfgJSON, err := json.Marshal(cfg)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		listener.ConfigJSON = string(cfgJSON)
	}

	if err := h.mgr().DB().UpdateC2Listener(listener); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	listener.EncryptionKey = ""
	listener.ImplantToken = ""
	c.JSON(http.StatusOK, gin.H{"listener": listener})
}

// DeleteListener deletes a listener
func (h *C2Handler) DeleteListener(c *gin.Context) {
	id := c.Param("id")
	if err := h.mgr().DeleteListener(id); err != nil {
		code := http.StatusInternalServerError
		if e, ok := err.(*c2.CommonError); ok {
			code = e.HTTP
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "c2", "listener_delete", "delete C2 listener", "c2_listener", id, nil)
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// StartListener starts a listener
func (h *C2Handler) StartListener(c *gin.Context) {
	id := c.Param("id")
	listener, err := h.mgr().StartListener(id)
	if err != nil {
		code := http.StatusInternalServerError
		if e, ok := err.(*c2.CommonError); ok {
			code = e.HTTP
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	listener.EncryptionKey = ""
	listener.ImplantToken = ""
	if h.audit != nil {
		h.audit.RecordOK(c, "c2", "listener_start", "start C2 listener", "c2_listener", id, nil)
	}
	c.JSON(http.StatusOK, gin.H{"listener": listener})
}

// StopListener stops a listener
func (h *C2Handler) StopListener(c *gin.Context) {
	id := c.Param("id")
	if err := h.mgr().StopListener(id); err != nil {
		code := http.StatusInternalServerError
		if e, ok := err.(*c2.CommonError); ok {
			code = e.HTTP
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "c2", "listener_stop", "stop C2 listener", "c2_listener", id, nil)
	}
	c.JSON(http.StatusOK, gin.H{"stopped": true})
}

// ============================================================================
// Session API
// ============================================================================

// ListSessions returns the session list
func (h *C2Handler) ListSessions(c *gin.Context) {
	filter := database.ListC2SessionsFilter{
		ListenerID: c.Query("listener_id"),
		ProjectID:  c.Query("project_id"),
		Status:     c.Query("status"),
		OS:         c.Query("os"),
		Search:     c.Query("search"),
	}
	if limit := c.Query("limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil && n > 0 {
			filter.Limit = n
		}
	}
	if c.Query("suspicious") == "1" || strings.EqualFold(c.Query("suspicious"), "true") {
		filter.Suspicious = true
	}

	sessions, err := h.mgr().DB().ListC2SessionsForAccess(filter, c2AccessFromContext(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

// GetSession retrieves a single session
func (h *C2Handler) GetSession(c *gin.Context) {
	id := c.Param("id")
	session, err := h.mgr().DB().GetC2Session(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// get recent tasks
	tasks, _ := h.mgr().DB().ListC2TasksForAccess(database.ListC2TasksFilter{
		SessionID: id,
		Limit:     20,
	}, c2AccessFromContext(c))

	c.JSON(http.StatusOK, gin.H{
		"session": session,
		"tasks":   tasks,
	})
}

// DeleteSession deletes a session
func (h *C2Handler) DeleteSession(c *gin.Context) {
	id := c.Param("id")
	if err := h.mgr().DB().DeleteC2Session(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "c2", "session_delete", "delete C2 session", "c2_session", id, nil)
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// DeleteSessions bulk-deletes sessions (request body JSON: {"ids":["s_xxx",...]})
func (h *C2Handler) DeleteSessions(c *gin.Context) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json: " + err.Error()})
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ids is required"})
		return
	}
	n, err := h.mgr().DB().DeleteC2SessionsByIDsForAccess(req.IDs, c2AccessFromContext(c))
	if err != nil {
		if errors.Is(err, database.ErrNoValidC2SessionIDs) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "c2", "session_delete", "bulk delete C2 sessions", "c2_session", "", map[string]interface{}{
			"count": n, "ids": req.IDs,
		})
	}
	c.JSON(http.StatusOK, gin.H{"deleted": n})
}

// SetSessionSleep sets the session sleep/jitter and dispatches a sleep task to the implant
func (h *C2Handler) SetSessionSleep(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		SleepSeconds  int `json:"sleep_seconds"`
		JitterPercent int `json:"jitter_percent"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.SleepSeconds < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sleep_seconds must be >= 1"})
		return
	}
	if req.JitterPercent < 0 || req.JitterPercent > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "jitter_percent must be 0-100"})
		return
	}

	task, err := h.mgr().SetSessionSleep(id, req.SleepSeconds, req.JitterPercent)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := gin.H{
		"updated":        true,
		"sleep_seconds":  req.SleepSeconds,
		"jitter_percent": req.JitterPercent,
	}
	if task != nil {
		out["task_id"] = task.ID
	}
	c.JSON(http.StatusOK, out)
}

// SetSessionNote updates session note (server-side metadata only, not dispatched to implant)
func (h *C2Handler) SetSessionNote(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Note string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	note := strings.TrimSpace(req.Note)
	if len(note) > 2000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "note too long (max 2000 characters)"})
		return
	}

	session, err := h.mgr().DB().GetC2Session(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	if err := h.mgr().DB().SetC2SessionNote(id, note); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "c2", "session_note", "update C2 session note", "c2_session", id, map[string]interface{}{
			"note_len": len(note),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"updated": true,
		"note":    note,
	})
}

// ============================================================================
// Task API
// ============================================================================

// ListTasks returns the task list
func (h *C2Handler) ListTasks(c *gin.Context) {
	filter := database.ListC2TasksFilter{
		SessionID: c.Query("session_id"),
		ProjectID: c.Query("project_id"),
		Status:    c.Query("status"),
		TaskType:  c.Query("task_type"),
	}
	if since := c.Query("since"); since != "" {
		if t, err := database.ParseRFC3339Time(since); err == nil {
			filter.Since = &t
		}
	}

	paginated := false
	page := 1
	pageSize := 10
	if c.Query("page") != "" || c.Query("page_size") != "" {
		paginated = true
		if p, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && p > 0 {
			page = p
		}
		if ps, err := strconv.Atoi(c.DefaultQuery("page_size", "10")); err == nil && ps > 0 {
			pageSize = ps
			if pageSize > 100 {
				pageSize = 100
			}
		}
		filter.Limit = pageSize
		filter.Offset = (page - 1) * pageSize
	} else {
		if limit := c.Query("limit"); limit != "" {
			if n, err := strconv.Atoi(limit); err == nil && n > 0 {
				filter.Limit = n
			}
		}
	}

	access := c2AccessFromContext(c)
	tasks, err := h.mgr().DB().ListC2TasksForAccess(filter, access)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// dashboard "pending tasks" count is the global queued/pending count, unrelated to session list filter
	pendingN, _ := h.mgr().DB().CountC2TasksQueuedOrPendingForAccess("", filter.ProjectID, access)

	if !paginated {
		c.JSON(http.StatusOK, gin.H{
			"tasks":                tasks,
			"pending_queued_count": pendingN,
		})
		return
	}

	total, err := h.mgr().DB().CountC2TasksForAccess(filter, access)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	statusCounts, err := h.mgr().DB().CountC2TasksByStatusForAccess(filter, access)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"tasks":                tasks,
		"total":                total,
		"status_counts":        statusCounts,
		"page":                 page,
		"page_size":            pageSize,
		"pending_queued_count": pendingN,
	})
}

// DeleteTasks bulk-deletes tasks (request body JSON: {"ids":["t_xxx",...]})
func (h *C2Handler) DeleteTasks(c *gin.Context) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json: " + err.Error()})
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ids is required"})
		return
	}
	n, err := h.mgr().DB().DeleteC2TasksByIDsForAccess(req.IDs, c2AccessFromContext(c))
	if err != nil {
		if errors.Is(err, database.ErrNoValidC2TaskIDs) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "c2", "task_delete", "bulk delete C2 tasks", "c2_task", "", map[string]interface{}{
			"count": n, "ids": req.IDs,
		})
	}
	c.JSON(http.StatusOK, gin.H{"deleted": n})
}

// GetTask retrieves a single task
func (h *C2Handler) GetTask(c *gin.Context) {
	id := c.Param("id")
	task, err := h.mgr().DB().GetC2Task(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": task})
}

// CreateTask createtask
func (h *C2Handler) CreateTask(c *gin.Context) {
	var req struct {
		SessionID      string                 `json:"session_id"`
		TaskType       string                 `json:"task_type"`
		Payload        map[string]interface{} `json:"payload"`
		Source         string                 `json:"source"`
		ConversationID string                 `json:"conversation_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.SessionID) == "" {
		req.SessionID = strings.TrimSpace(c.Param("id"))
	}
	if !h.c2ResourceAllowed(c, "c2_session", req.SessionID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}
	if conversationID := strings.TrimSpace(req.ConversationID); conversationID != "" {
		session, ok := security.CurrentSession(c)
		if !ok || !h.mgr().DB().UserCanAccessResource(session.UserID, session.Scope, "conversation", conversationID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "no permission to associate target conversation"})
			return
		}
		req.ConversationID = conversationID
	}

	input := c2.EnqueueTaskInput{
		SessionID:      req.SessionID,
		TaskType:       c2.TaskType(req.TaskType),
		Payload:        req.Payload,
		Source:         firstNonEmpty(req.Source, "manual"),
		ConversationID: req.ConversationID,
		UserCtx:        c.Request.Context(),
	}

	task, err := h.mgr().EnqueueTask(input)
	if err != nil {
		code := http.StatusInternalServerError
		if e, ok := err.(*c2.CommonError); ok {
			code = e.HTTP
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "c2", "task_create", "create C2 task", "c2_task", task.ID, map[string]interface{}{
			"session_id": req.SessionID, "task_type": req.TaskType,
		})
	}
	c.JSON(http.StatusOK, gin.H{"task": task})
}

// CancelTask cancelledtask
func (h *C2Handler) CancelTask(c *gin.Context) {
	id := c.Param("id")
	if !h.c2ResourceAllowed(c, "c2_task", id) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}
	if err := h.mgr().CancelTask(id); err != nil {
		code := http.StatusInternalServerError
		if e, ok := err.(*c2.CommonError); ok {
			code = e.HTTP
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "c2", "task_cancel", "cancelled C2 task", "c2_task", id, nil)
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": true})
}

// WaitTask waits for task completion
func (h *C2Handler) WaitTask(c *gin.Context) {
	id := c.Param("id")
	if !h.c2ResourceAllowed(c, "c2_task", id) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}
	timeout := 60 * time.Second
	if t := c.Query("timeout"); t != "" {
		if n, err := strconv.Atoi(t); err == nil && n > 0 {
			timeout = time.Duration(n) * time.Second
		}
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		task, err := h.mgr().DB().GetC2Task(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if task == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		if task.Status == "success" || task.Status == "failed" || task.Status == "cancelled" {
			c.JSON(http.StatusOK, gin.H{"task": task})
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	c.JSON(http.StatusRequestTimeout, gin.H{"error": "timeout waiting for task completion"})
}

// ============================================================================
// Payload API
// ============================================================================

// PayloadOneliner generates a single-line payload
func (h *C2Handler) PayloadOneliner(c *gin.Context) {
	var req struct {
		ListenerID string `json:"listener_id"`
		Kind       string `json:"kind"` // bash, python, powershell, curl_beacon
		Host       string `json:"host"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	listener, err := h.mgr().DB().GetC2Listener(req.ListenerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if listener == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "listener not found"})
		return
	}
	if !h.c2ResourceAllowed(c, "c2_listener", req.ListenerID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}

	host := c2.ResolveBeaconDialHost(listener, strings.TrimSpace(req.Host), h.logger, listener.ID)

	kind := c2.OnelinerKind(req.Kind)
	if !c2.IsOnelinerCompatible(listener.Type, kind) {
		compatible := c2.OnelinerKindsForListener(listener.Type)
		names := make([]string, len(compatible))
		for i, k := range compatible {
			names[i] = string(k)
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error":            fmt.Sprintf("listener type %s does not support oneliner of kind %s, please select a compatible kind", listener.Type, req.Kind),
			"compatible_kinds": names,
		})
		return
	}

	scheme := "http"
	if listener.Type == "https_beacon" {
		scheme = "https"
	}
	input := c2.OnelinerInput{
		Kind:         kind,
		Host:         host,
		Port:         listener.BindPort,
		HTTPBaseURL:  fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, strconv.Itoa(listener.BindPort))),
		ImplantToken: listener.ImplantToken,
	}

	oneliner, err := c2.GenerateOneliner(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"oneliner": oneliner,
		"kind":     req.Kind,
		"host":     host,
		"port":     listener.BindPort,
	})
}

// PayloadBuild builds a beacon binary
func (h *C2Handler) PayloadBuild(c *gin.Context) {
	var req struct {
		ListenerID    string `json:"listener_id"`
		OS            string `json:"os"`
		Arch          string `json:"arch"`
		SleepSeconds  int    `json:"sleep_seconds"`
		JitterPercent int    `json:"jitter_percent"`
		Host          string `json:"host"` // optional: callback address compiled into Beacon, overrides listener bind_host
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	listener, err := h.mgr().DB().GetC2Listener(req.ListenerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if listener == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "listener not found"})
		return
	}
	if !h.c2ResourceAllowed(c, "c2_listener", req.ListenerID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}

	builder := c2.NewPayloadBuilder(h.mgr(), h.logger, "", "")
	input := c2.PayloadBuilderInput{
		ListenerID:    req.ListenerID,
		OS:            req.OS,
		Arch:          req.Arch,
		SleepSeconds:  req.SleepSeconds,
		JitterPercent: req.JitterPercent,
		Host:          strings.TrimSpace(req.Host),
	}

	result, err := builder.BuildBeacon(input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if session, ok := security.CurrentSession(c); ok {
		_ = h.mgr().DB().RecordC2PayloadArtifact(filepath.Base(result.OutputPath), result.PayloadID, result.ListenerID, session.UserID)
	}

	c.JSON(http.StatusOK, gin.H{
		"payload": result,
	})
}

// PayloadDownload download payload
func (h *C2Handler) PayloadDownload(c *gin.Context) {
	id := c.Param("id")
	filename := id
	if !strings.HasPrefix(filename, "beacon_") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload id"})
		return
	}
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload id"})
		return
	}
	mgr := h.mgr()
	if mgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "c2 disabled"})
		return
	}
	session, ok := security.CurrentSession(c)
	if !ok || !mgr.DB().UserCanAccessC2Payload(session.UserID, session.Scope, filename) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}

	builder := c2.NewPayloadBuilder(mgr, h.logger, "", "")
	storageDir := builder.GetPayloadStoragePath()
	targetPath := filepath.Join(storageDir, filename)

	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	absDir, err := filepath.Abs(storageDir)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload id"})
		return
	}
	rel, err := filepath.Rel(absDir, absTarget)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload id"})
		return
	}

	c.FileAttachment(absTarget, filepath.Base(absTarget))
}

// ============================================================================
// event API
// ============================================================================

// ListEvents returns the event list
func (h *C2Handler) ListEvents(c *gin.Context) {
	filter := database.ListC2EventsFilter{
		Level:     c.Query("level"),
		Category:  c.Query("category"),
		ProjectID: c.Query("project_id"),
		SessionID: c.Query("session_id"),
		TaskID:    c.Query("task_id"),
	}
	if since := c.Query("since"); since != "" {
		if t, err := database.ParseRFC3339Time(since); err == nil {
			filter.Since = &t
		}
	}

	paginated := false
	page := 1
	pageSize := 10
	if c.Query("page") != "" || c.Query("page_size") != "" {
		paginated = true
		if p, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && p > 0 {
			page = p
		}
		if ps, err := strconv.Atoi(c.DefaultQuery("page_size", "10")); err == nil && ps > 0 {
			pageSize = ps
			if pageSize > 100 {
				pageSize = 100
			}
		}
		filter.Limit = pageSize
		filter.Offset = (page - 1) * pageSize
	} else {
		if limit := c.Query("limit"); limit != "" {
			if n, err := strconv.Atoi(limit); err == nil && n > 0 {
				filter.Limit = n
			}
		}
	}

	access := c2AccessFromContext(c)
	events, err := h.mgr().DB().ListC2EventsForAccess(filter, access)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !paginated {
		c.JSON(http.StatusOK, gin.H{"events": events})
		return
	}
	total, err := h.mgr().DB().CountC2EventsForAccess(filter, access)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	levelCounts, err := h.mgr().DB().CountC2EventsByLevelForAccess(filter, access)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"events":       events,
		"total":        total,
		"level_counts": levelCounts,
		"page":         page,
		"page_size":    pageSize,
	})
}

// DeleteEvents bulk-deletes events (request body JSON: {"ids":["e_xxx",...]})
func (h *C2Handler) DeleteEvents(c *gin.Context) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json: " + err.Error()})
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ids is required"})
		return
	}
	n, err := h.mgr().DB().DeleteC2EventsByIDsForAccess(req.IDs, c2AccessFromContext(c))
	if err != nil {
		if errors.Is(err, database.ErrNoValidC2EventIDs) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": n})
}

// EventStream is a real-time SSE event stream.
func (h *C2Handler) EventStream(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	mgr := h.mgr()
	if mgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "c2 disabled"})
		return
	}

	sessionFilter := c.Query("session_id")
	categoryFilter := c.Query("category")
	levels := c.QueryArray("level")

	sub := mgr.EventBus().Subscribe(
		"sse-"+uuid.New().String(),
		128,
		sessionFilter,
		categoryFilter,
		levels,
	)
	defer mgr.EventBus().Unsubscribe(sub.ID)

	c.Stream(func(w io.Writer) bool {
		select {
		case e, ok := <-sub.Ch:
			if !ok {
				return false
			}
			if !h.c2EventAllowed(c, e) {
				return true
			}
			data, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", data)
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}

// ============================================================================
// Profile API
// ============================================================================

// ListProfiles returns the Malleable Profile list.
func (h *C2Handler) ListProfiles(c *gin.Context) {
	profiles, err := h.mgr().DB().ListC2Profiles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"profiles": profiles})
}

// GetProfile returns a single Profile.
func (h *C2Handler) GetProfile(c *gin.Context) {
	id := c.Param("id")
	profile, err := h.mgr().DB().GetC2Profile(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if profile == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"profile": profile})
}

// CreateProfile create Profile
func (h *C2Handler) CreateProfile(c *gin.Context) {
	var req database.C2Profile
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.JitterMinMS < 0 || req.JitterMaxMS < req.JitterMinMS {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Jitter must satisfy 0 <= min <= max"})
		return
	}
	req.ID = "p_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:14]
	req.CreatedAt = time.Now()

	if err := h.mgr().DB().CreateC2Profile(&req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"profile": req})
}

// UpdateProfile update Profile
func (h *C2Handler) UpdateProfile(c *gin.Context) {
	id := c.Param("id")
	profile, err := h.mgr().DB().GetC2Profile(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if profile == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
		return
	}

	var req database.C2Profile
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.JitterMinMS < 0 || req.JitterMaxMS < req.JitterMinMS {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Jitter must satisfy 0 <= min <= max"})
		return
	}
	profile.Name = req.Name
	profile.UserAgent = req.UserAgent
	profile.URIs = req.URIs
	profile.RequestHeaders = req.RequestHeaders
	profile.ResponseHeaders = req.ResponseHeaders
	profile.BodyTemplate = req.BodyTemplate
	profile.JitterMinMS = req.JitterMinMS
	profile.JitterMaxMS = req.JitterMaxMS

	if err := h.mgr().DB().UpdateC2Profile(profile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"profile": profile})
}

// DeleteProfile delete Profile
func (h *C2Handler) DeleteProfile(c *gin.Context) {
	id := c.Param("id")
	if err := h.mgr().DB().DeleteC2Profile(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// ============================================================================
// File management API (C2 upload tasks must first upload files to the downstream directory via this API).
// ============================================================================

// UploadFileForImplant allows operators to upload a file that an upload task will push to the implant.
func (h *C2Handler) UploadFileForImplant(c *gin.Context) {
	sessionID := strings.TrimSpace(c.PostForm("session_id"))
	remotePath := strings.TrimSpace(c.PostForm("remote_path"))
	if sessionID == "" || remotePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id and remote_path required"})
		return
	}
	if !h.c2ResourceAllowed(c, "c2_session", sessionID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file field required: " + err.Error()})
		return
	}
	defer file.Close()

	fileID := "f_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:14]
	dir := filepath.Join(h.mgr().StorageDir(), "downstream")
	if err := osMkdirAll(dir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	dstPath := filepath.Join(dir, fileID+".bin")
	dst, err := osCreate(dstPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	n, err := io.Copy(dst, file)
	dst.Close()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Record in DB
	dbFile := &database.C2File{
		ID:         fileID,
		SessionID:  sessionID,
		Direction:  "upload",
		RemotePath: remotePath,
		LocalPath:  dstPath,
		SizeBytes:  n,
		CreatedAt:  time.Now(),
	}
	_ = h.mgr().DB().CreateC2File(dbFile)

	c.JSON(http.StatusOK, gin.H{
		"file_id":     fileID,
		"size":        n,
		"filename":    header.Filename,
		"remote_path": remotePath,
	})
}

// ListFiles lists file records for a conversation.
func (h *C2Handler) ListFiles(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id required"})
		return
	}
	if !h.c2ResourceAllowed(c, "c2_session", sessionID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}
	files, err := h.mgr().DB().ListC2FilesBySession(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"files": files})
}

// DownloadResultFile downloads a task result file (screenshot and other blob results).
func (h *C2Handler) DownloadResultFile(c *gin.Context) {
	taskID := c.Param("id")
	task, err := h.mgr().DB().GetC2Task(taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if !h.c2ResourceAllowed(c, "c2_task", taskID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}
	if task.ResultBlobPath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "no result file for this task"})
		return
	}
	c.FileAttachment(task.ResultBlobPath, filepath.Base(task.ResultBlobPath))
}

func osMkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}

func osCreate(path string) (*os.File, error) {
	return os.Create(path)
}

func c2AccessFromContext(c *gin.Context) database.RBACListAccess {
	session, ok := security.CurrentSession(c)
	if !ok {
		return database.RBACListAccess{}
	}
	return database.RBACListAccess{UserID: session.UserID, Scope: session.Scope}
}

func (h *C2Handler) canAccessProject(c *gin.Context, projectID string) bool {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return true
	}
	session, ok := security.CurrentSession(c)
	if !ok {
		return false
	}
	if session.Scope == database.RBACScopeAll {
		return true
	}
	mgr := h.mgr()
	if mgr == nil {
		return false
	}
	return mgr.DB().UserCanAccessResource(session.UserID, session.Scope, "project", projectID)
}

func (h *C2Handler) c2ResourceAllowed(c *gin.Context, resourceType, resourceID string) bool {
	session, ok := security.CurrentSession(c)
	if !ok {
		return false
	}
	mgr := h.mgr()
	if mgr == nil {
		return false
	}
	return mgr.DB().UserCanAccessResource(session.UserID, session.Scope, resourceType, resourceID)
}

func (h *C2Handler) c2EventAllowed(c *gin.Context, e *c2.Event) bool {
	if e == nil {
		return false
	}
	session, ok := security.CurrentSession(c)
	if !ok {
		return false
	}
	if session.Scope == database.RBACScopeAll {
		return true
	}
	mgr := h.mgr()
	if mgr == nil {
		return false
	}
	if strings.TrimSpace(e.SessionID) != "" {
		return mgr.DB().UserCanAccessResource(session.UserID, session.Scope, "c2_session", e.SessionID)
	}
	if strings.TrimSpace(e.TaskID) != "" {
		return mgr.DB().UserCanAccessResource(session.UserID, session.Scope, "c2_task", e.TaskID)
	}
	return false
}

// ============================================================================
// Helper functions (firstNonEmpty is defined in vulnerability.go).
// ============================================================================
