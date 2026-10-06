package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"kestrel/internal/database"
	"kestrel/internal/middleware"
)

// ProjectHandler handles project management endpoints.
type ProjectHandler struct {
	db *database.DB
}

// NewProjectHandler creates a ProjectHandler.
func NewProjectHandler(db *database.DB) *ProjectHandler {
	return &ProjectHandler{db: db}
}

// ListProjects handles GET /api/projects.
func (h *ProjectHandler) ListProjects(c *gin.Context) {
	p := database.ListProjectsParams{
		Status: c.Query("status"),
	}
	if lim, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil {
		p.Limit = lim
	}
	if off, err := strconv.Atoi(c.DefaultQuery("offset", "0")); err == nil {
		p.Offset = off
	}
	projects, total, err := h.db.ListProjects(p)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list projects"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"projects": projects, "total": total})
}

// createProjectReq is the body for POST /api/projects.
type createProjectReq struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Scope       []string `json:"scope"`
}

// CreateProject handles POST /api/projects.
func (h *ProjectHandler) CreateProject(c *gin.Context) {
	ownerID, _ := c.Get(middleware.CtxUserID)
	uid, _ := ownerID.(string)
	var req createProjectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	proj, err := h.db.CreateProject(req.Name, req.Description, uid, req.Scope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create project"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"project": proj})
}

// GetProject handles GET /api/projects/:id.
func (h *ProjectHandler) GetProject(c *gin.Context) {
	proj, err := h.db.GetProjectByID(c.Param("id"))
	if err != nil || proj == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}
	stats, _ := h.db.GetProjectStats(proj.ID)
	c.JSON(http.StatusOK, gin.H{"project": proj, "stats": stats})
}

// updateProjectReq is the body for PATCH /api/projects/:id.
type updateProjectReq struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Scope       []string `json:"scope"`
	Pinned      *bool    `json:"pinned"`
}

// UpdateProject handles PATCH /api/projects/:id.
func (h *ProjectHandler) UpdateProject(c *gin.Context) {
	id := c.Param("id")
	proj, err := h.db.GetProjectByID(id)
	if err != nil || proj == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}
	var req updateProjectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	name := proj.Name
	if req.Name != "" {
		name = req.Name
	}
	desc := proj.Description
	if req.Description != "" {
		desc = req.Description
	}
	status := proj.Status
	if req.Status != "" {
		status = req.Status
	}
	scope := proj.Scope
	if req.Scope != nil {
		scope = req.Scope
	}
	pinned := proj.Pinned
	if req.Pinned != nil {
		pinned = *req.Pinned
	}
	if err := h.db.UpdateProject(id, name, desc, status, scope, pinned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update project"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "project updated"})
}

// DeleteProject handles DELETE /api/projects/:id.
func (h *ProjectHandler) DeleteProject(c *gin.Context) {
	if err := h.db.DeleteProject(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete project"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "project deleted"})
}

// ListProjectFacts handles GET /api/projects/:id/facts.
func (h *ProjectHandler) ListProjectFacts(c *gin.Context) {
	facts, err := h.db.ListProjectFacts(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list facts"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"facts": facts, "total": len(facts)})
}

// upsertFactReq is the body for POST /api/projects/:id/facts.
type upsertFactReq struct {
	FactKey    string `json:"fact_key" binding:"required"`
	Category   string `json:"category"`
	Summary    string `json:"summary"`
	Body       string `json:"body"`
	Confidence string `json:"confidence"`
	Pinned     bool   `json:"pinned"`
}

// UpsertProjectFact handles POST /api/projects/:id/facts.
func (h *ProjectHandler) UpsertProjectFact(c *gin.Context) {
	sessionID, _ := c.Get(middleware.CtxUserID)
	sid, _ := sessionID.(string)
	var req upsertFactReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "fact_key is required"})
		return
	}
	fact := &database.ProjectFact{
		ProjectID:    c.Param("id"),
		FactKey:      req.FactKey,
		Category:     req.Category,
		Summary:      req.Summary,
		Body:         req.Body,
		Confidence:   req.Confidence,
		SourceSessID: sid,
		Pinned:       req.Pinned,
	}
	if fact.Category == "" {
		fact.Category = "note"
	}
	if fact.Confidence == "" {
		fact.Confidence = "tentative"
	}
	f, err := h.db.UpsertProjectFact(fact)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upsert fact"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"fact": f})
}

// DeleteProjectFact handles DELETE /api/projects/:id/facts/:fact_id.
func (h *ProjectHandler) DeleteProjectFact(c *gin.Context) {
	if err := h.db.DeleteProjectFact(c.Param("fact_id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete fact"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "fact deleted"})
}

// GetAttackChain handles GET /api/projects/:id/attack-chain.
func (h *ProjectHandler) GetAttackChain(c *gin.Context) {
	nodes, edges, err := h.db.GetAttackChain(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get attack chain"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"nodes": nodes, "edges": edges})
}

// addChainNodeReq is the body for POST /api/projects/:id/attack-chain/nodes.
type addChainNodeReq struct {
	NodeType  string                 `json:"node_type" binding:"required"`
	NodeName  string                 `json:"node_name" binding:"required"`
	SessionID string                 `json:"session_id"`
	RiskScore int                    `json:"risk_score"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// AddChainNode handles POST /api/projects/:id/attack-chain/nodes.
func (h *ProjectHandler) AddChainNode(c *gin.Context) {
	var req addChainNodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_type and node_name are required"})
		return
	}
	n, err := h.db.AddAttackChainNode(&database.AttackChainNode{
		ProjectID: c.Param("id"),
		SessionID: req.SessionID,
		NodeType:  req.NodeType,
		NodeName:  req.NodeName,
		RiskScore: req.RiskScore,
		Metadata:  req.Metadata,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add node"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"node": n})
}

// addChainEdgeReq is the body for POST /api/projects/:id/attack-chain/edges.
type addChainEdgeReq struct {
	SourceNodeID string `json:"source_node_id" binding:"required"`
	TargetNodeID string `json:"target_node_id" binding:"required"`
	EdgeType     string `json:"edge_type"`
	Weight       int    `json:"weight"`
}

// AddChainEdge handles POST /api/projects/:id/attack-chain/edges.
func (h *ProjectHandler) AddChainEdge(c *gin.Context) {
	var req addChainEdgeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_node_id and target_node_id are required"})
		return
	}
	if req.EdgeType == "" {
		req.EdgeType = "leads_to"
	}
	e, err := h.db.AddAttackChainEdge(&database.AttackChainEdge{
		ProjectID:    c.Param("id"),
		SourceNodeID: req.SourceNodeID,
		TargetNodeID: req.TargetNodeID,
		EdgeType:     req.EdgeType,
		Weight:       req.Weight,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add edge"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"edge": e})
}
