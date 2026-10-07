package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"kestrel/internal/agent"
	"kestrel/internal/audit"
	"kestrel/internal/config"
	"kestrel/internal/database"
	workflowrunner "kestrel/internal/workflow"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func validWorkflowID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for i, r := range id {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !(i > 0 && (r == '_' || r == '-')) {
			return false
		}
	}
	return true
}

type workflowDefinitionResponse struct {
	*database.WorkflowDefinition
	ValidationError string `json:"validation_error,omitempty"`
}

func workflowResponse(ctx context.Context, wf *database.WorkflowDefinition) workflowDefinitionResponse {
	r := workflowDefinitionResponse{WorkflowDefinition: wf}
	if wf != nil {
		if err := workflowrunner.ValidateGraphJSON(ctx, wf.GraphJSON); err != nil {
			r.ValidationError = err.Error()
		}
	}
	return r
}

type WorkflowHandler struct {
	db     *database.DB
	logger *zap.Logger
	audit  *audit.Service
	agent  *agent.Agent
	cfg    *config.Config
}

func NewWorkflowHandler(db *database.DB, logger *zap.Logger) *WorkflowHandler {
	return &WorkflowHandler{db: db, logger: logger}
}

func (h *WorkflowHandler) SetAudit(s *audit.Service) {
	h.audit = s
}

type workflowSaveRequest struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Version     int             `json:"version,omitempty"`
	Enabled     *bool           `json:"enabled,omitempty"`
	Graph       json.RawMessage `json:"graph,omitempty"`
	GraphJSON   json.RawMessage `json:"graph_json,omitempty"`
}

type workflowDryRunRequest struct {
	Graph     json.RawMessage        `json:"graph,omitempty"`
	GraphJSON json.RawMessage        `json:"graph_json,omitempty"`
	Inputs    map[string]interface{} `json:"inputs,omitempty"`
}

type workflowGenerateDraftRequest struct {
	Prompt         string                      `json:"prompt"`
	Options        workflowrunner.DraftOptions `json:"options"`
	AvailableTools []workflowrunner.DraftTool  `json:"available_tools,omitempty"`
}

func (h *WorkflowHandler) List(c *gin.Context) {
	includeDisabled := strings.EqualFold(c.Query("includeDisabled"), "true") || c.Query("include_disabled") == "1"
	items, err := h.db.ListWorkflowDefinitions(includeDisabled)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	responses := make([]workflowDefinitionResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, workflowResponse(c.Request.Context(), item))
	}
	c.JSON(http.StatusOK, gin.H{"workflows": responses})
}

func (h *WorkflowHandler) Get(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	wf, err := h.db.GetWorkflowDefinition(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if wf == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "workflow not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"workflow": workflowResponse(c.Request.Context(), wf)})
}

func (h *WorkflowHandler) Create(c *gin.Context) {
	h.save(c, "")
}

func (h *WorkflowHandler) Validate(c *gin.Context) {
	var req workflowSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid request parameters: " + err.Error()})
		return
	}
	graph := req.Graph
	if len(graph) == 0 {
		graph = req.GraphJSON
	}
	if len(graph) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "graph cannot be empty"})
		return
	}
	if !json.Valid(graph) {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "graph must be valid JSON"})
		return
	}
	if err := workflowrunner.ValidateGraphJSON(c.Request.Context(), string(graph)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *WorkflowHandler) DryRun(c *gin.Context) {
	var req workflowDryRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters: " + err.Error()})
		return
	}
	graph := req.Graph
	if len(graph) == 0 {
		graph = req.GraphJSON
	}
	if len(graph) == 0 || !json.Valid(graph) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "graph must be valid JSON"})
		return
	}
	inputs := make(map[string]any, len(req.Inputs))
	for k, v := range req.Inputs {
		inputs[k] = v
	}
	result, err := workflowrunner.DryRunGraphJSON(c.Request.Context(), string(graph), inputs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}

func (h *WorkflowHandler) GenerateDraft(c *gin.Context) {
	var req workflowGenerateDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters: " + err.Error()})
		return
	}
	draftReq := workflowrunner.DraftRequest{
		Prompt:         req.Prompt,
		Options:        req.Options,
		AvailableTools: req.AvailableTools,
	}
	var result *workflowrunner.DraftResult
	var llmErr error
	if h.cfg != nil {
		if llmCfg, _, ok := h.cfg.ResolveAIChannel(""); ok && strings.TrimSpace(llmCfg.APIKey) != "" && strings.TrimSpace(llmCfg.Model) != "" {
			result, llmErr = workflowrunner.GenerateDraftFromLLM(c.Request.Context(), draftReq, llmCfg, h.logger)
		} else {
			llmErr = errors.New("AI channel has no api_key or model configured")
		}
	} else {
		llmErr = errors.New("workflow generator has not loaded platform AI config")
	}
	if llmErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "large model generation failed: " + llmErr.Error()})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "workflow", "generate_draft", "generate workflow draft from natural language", "", "", map[string]interface{}{
			"generator": result.Generator,
			"nodes":     result.Stats["nodes"],
			"edges":     result.Stats["edges"],
			"high_risk": result.Audit.HighRisk,
			"savable":   result.Audit.Savable,
		})
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}

func (h *WorkflowHandler) Update(c *gin.Context) {
	h.save(c, c.Param("id"))
}

func (h *WorkflowHandler) save(c *gin.Context, pathID string) {
	var req workflowSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters: " + err.Error()})
		return
	}
	id := strings.TrimSpace(req.ID)
	if strings.TrimSpace(pathID) != "" {
		id = strings.TrimSpace(pathID)
	}
	name := strings.TrimSpace(req.Name)
	if id == "" || name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workflow id and name cannot be empty"})
		return
	}
	if pathID == "" && !validWorkflowID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workflow ID must start with a letter or digit, contain only letters, digits, underscores, or hyphens, and must not exceed 128 bytes"})
		return
	}
	if pathID != "" {
		if req.ID != "" && strings.TrimSpace(req.ID) != id {
			c.JSON(http.StatusBadRequest, gin.H{"error": "workflow ID cannot be changed via edit"})
			return
		}
		existing, err := h.db.GetWorkflowDefinition(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if existing == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "workflow not found"})
			return
		}
	}
	graph := req.Graph
	if len(graph) == 0 {
		graph = req.GraphJSON
	}
	if len(graph) == 0 {
		graph = []byte(`{"nodes":[],"edges":[],"config":{}}`)
	}
	if !json.Valid(graph) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "graph must be valid JSON"})
		return
	}
	if err := workflowrunner.ValidateGraphJSON(c.Request.Context(), string(graph)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to compile workflow graph: " + err.Error()})
		return
	}
	var probe interface{}
	if err := json.Unmarshal(graph, &probe); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "graph JSON parsing failed: " + err.Error()})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	wf := &database.WorkflowDefinition{
		ID:          id,
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Version:     req.Version,
		GraphJSON:   string(graph),
		Enabled:     enabled,
	}
	var saveErr error
	if pathID == "" {
		saveErr = h.db.CreateWorkflowDefinition(wf)
	} else {
		saveErr = h.db.UpsertWorkflowDefinition(wf)
	}
	if err := saveErr; err != nil {
		if errors.Is(err, database.ErrWorkflowAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if h.logger != nil {
			h.logger.Warn("failed to save workflow", zap.String("id", id), zap.Error(err))
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	saved, _ := h.db.GetWorkflowDefinition(id)
	workflowrunner.InvalidateCompiledCache(id)
	if h.audit != nil {
		h.audit.RecordOK(c, "workflow", "save", "save workflow", "workflow", id, map[string]interface{}{"name": name})
	}
	c.JSON(http.StatusOK, gin.H{"message": "workflow saved", "workflow": saved})
}

func (h *WorkflowHandler) Delete(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workflow id cannot be empty"})
		return
	}
	if h.cfg != nil {
		for _, role := range h.cfg.Roles {
			if role.WorkflowID == id {
				c.JSON(http.StatusConflict, gin.H{"error": "workflow is still referenced by a role, please unbind it first"})
				return
			}
		}
	}
	if err := h.db.DeleteWorkflowDefinition(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	workflowrunner.InvalidateCompiledCache(id)
	if h.audit != nil {
		h.audit.RecordOK(c, "workflow", "delete", "delete workflow", "workflow", id, nil)
	}
	c.JSON(http.StatusOK, gin.H{"message": "workflow deleted"})
}
