package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"kestrel/internal/database"
	"kestrel/internal/middleware"
	"kestrel/internal/workflow"
)

// WorkflowHandler serves workflow definition and run management endpoints.
type WorkflowHandler struct {
	db     *database.DB
	engine *workflow.Engine
	logger *zap.Logger
}

// NewWorkflowHandler creates a WorkflowHandler.
func NewWorkflowHandler(db *database.DB, engine *workflow.Engine, logger *zap.Logger) *WorkflowHandler {
	return &WorkflowHandler{db: db, engine: engine, logger: logger}
}

// ListWorkflows handles GET /api/workflows.
func (h *WorkflowHandler) ListWorkflows(c *gin.Context) {
	rows, err := h.db.Query(`SELECT id,name,description,enabled,created_at FROM workflow_definitions ORDER BY created_at DESC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list workflows"})
		return
	}
	defer rows.Close()
	type wfRow struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Enabled     bool   `json:"enabled"`
		CreatedAt   string `json:"created_at"`
	}
	var items []wfRow
	for rows.Next() {
		var r wfRow
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Enabled, &r.CreatedAt); err == nil {
			items = append(items, r)
		}
	}
	c.JSON(http.StatusOK, gin.H{"workflows": items, "total": len(items)})
}

// createWorkflowReq is the body for POST /api/workflows.
type createWorkflowReq struct {
	Name        string             `json:"name" binding:"required"`
	Description string             `json:"description"`
	Graph       workflow.Definition `json:"graph" binding:"required"`
}

// CreateWorkflow handles POST /api/workflows.
func (h *WorkflowHandler) CreateWorkflow(c *gin.Context) {
	uid, _ := c.Get(middleware.CtxUserID)
	var req createWorkflowReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and graph are required"})
		return
	}
	graphJSON, _ := json.Marshal(req.Graph)
	id, err := h.db.CreateWorkflowDefinition(req.Name, req.Description, string(graphJSON), uid.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create workflow"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "name": req.Name})
}

// GetWorkflow handles GET /api/workflows/:id.
func (h *WorkflowHandler) GetWorkflow(c *gin.Context) {
	row, err := h.db.GetWorkflowDefinition(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "workflow not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"workflow": row})
}

// UpdateWorkflow handles PATCH /api/workflows/:id.
func (h *WorkflowHandler) UpdateWorkflow(c *gin.Context) {
	var req struct {
		Name        string             `json:"name"`
		Description string             `json:"description"`
		Graph       *workflow.Definition `json:"graph"`
		Enabled     *bool              `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	var graphJSON *string
	if req.Graph != nil {
		b, _ := json.Marshal(req.Graph)
		s := string(b)
		graphJSON = &s
	}
	if err := h.db.UpdateWorkflowDefinition(c.Param("id"), req.Name, req.Description, graphJSON, req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update workflow"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "workflow updated"})
}

// DeleteWorkflow handles DELETE /api/workflows/:id.
func (h *WorkflowHandler) DeleteWorkflow(c *gin.Context) {
	if err := h.db.DeleteWorkflowDefinition(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete workflow"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "workflow deleted"})
}

// RunWorkflow handles POST /api/workflows/:id/run.
func (h *WorkflowHandler) RunWorkflow(c *gin.Context) {
	uid, _ := c.Get(middleware.CtxUserID)
	var req struct {
		ProjectID string                 `json:"project_id"`
		Input     map[string]interface{} `json:"input"`
	}
	_ = c.ShouldBindJSON(&req)

	runID, err := h.engine.Run(c.Request.Context(), c.Param("id"), workflow.RunContext{
		WorkflowID: c.Param("id"),
		UserID:     uid.(string),
		ProjectID:  req.ProjectID,
		Input:      req.Input,
		SessionID:  "wf_" + c.Param("id"),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"run_id": runID})
}

// GetRun handles GET /api/workflows/runs/:run_id.
func (h *WorkflowHandler) GetRun(c *gin.Context) {
	status, output, err := h.engine.GetRunStatus(c.Param("run_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": status, "output": output})
}
