package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"kestrel/internal/database"
	"kestrel/internal/middleware"
)

// AuditHandler handles audit log endpoints.
type AuditHandler struct {
	db *database.DB
}

// NewAuditHandler creates an AuditHandler.
func NewAuditHandler(db *database.DB) *AuditHandler {
	return &AuditHandler{db: db}
}

// ListAuditLogs handles GET /api/audit.
func (h *AuditHandler) ListAuditLogs(c *gin.Context) {
	p := database.ListAuditLogsParams{
		ActorID:      c.Query("actor_id"),
		Category:     c.Query("category"),
		Action:       c.Query("action"),
		Result:       c.Query("result"),
		ResourceType: c.Query("resource_type"),
		ResourceID:   c.Query("resource_id"),
	}

	if lim, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil {
		p.Limit = lim
	}
	if off, err := strconv.Atoi(c.DefaultQuery("offset", "0")); err == nil {
		p.Offset = off
	}

	logs, total, err := h.db.ListAuditLogs(p)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve audit logs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs, "total": total})
}

// RoleHandler handles role management endpoints.
type RoleHandler struct {
	db *database.DB
}

// NewRoleHandler creates a RoleHandler.
func NewRoleHandler(db *database.DB) *RoleHandler {
	return &RoleHandler{db: db}
}

// ListRoles handles GET /api/roles.
func (h *RoleHandler) ListRoles(c *gin.Context) {
	roles, err := h.db.ListRoles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list roles"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"roles": roles, "total": len(roles)})
}

// createRoleRequest is the body for POST /api/roles.
type createRoleRequest struct {
	Name         string   `json:"name" binding:"required"`
	Description  string   `json:"description"`
	Permissions  []string `json:"permissions"`
	AllowedTools []string `json:"allowed_tools"`
	HITLMode     string   `json:"hitl_mode"`
}

// CreateRole handles POST /api/roles.
func (h *RoleHandler) CreateRole(c *gin.Context) {
	var req createRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	role, err := h.db.CreateRole(req.Name, req.Description, false, req.Permissions, req.AllowedTools, req.HITLMode)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "role name already exists or database error"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"role": role})
}

// GetRole handles GET /api/roles/:id.
func (h *RoleHandler) GetRole(c *gin.Context) {
	id := c.Param("id")
	role, err := h.db.GetRoleByID(id)
	if err != nil || role == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"role": role})
}

// updateRoleRequest is the body for PATCH /api/roles/:id.
type updateRoleRequest struct {
	Description  string   `json:"description"`
	Permissions  []string `json:"permissions"`
	AllowedTools []string `json:"allowed_tools"`
	HITLMode     string   `json:"hitl_mode"`
}

// UpdateRole handles PATCH /api/roles/:id.
func (h *RoleHandler) UpdateRole(c *gin.Context) {
	id := c.Param("id")
	role, err := h.db.GetRoleByID(id)
	if err != nil || role == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	if role.IsSystem {
		c.JSON(http.StatusForbidden, gin.H{"error": "system roles cannot be modified"})
		return
	}

	var req updateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := h.db.UpdateRole(id, req.Description, req.Permissions, req.AllowedTools, req.HITLMode); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update role"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "role updated"})
}

// DeleteRole handles DELETE /api/roles/:id.
func (h *RoleHandler) DeleteRole(c *gin.Context) {
	id := c.Param("id")
	role, err := h.db.GetRoleByID(id)
	if err != nil || role == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	if role.IsSystem {
		c.JSON(http.StatusForbidden, gin.H{"error": "system roles cannot be deleted"})
		return
	}
	if err := h.db.DeleteRole(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete role"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "role deleted"})
}

// AssetHandler handles asset management endpoints.
type AssetHandler struct {
	db *database.DB
}

// NewAssetHandler creates an AssetHandler.
func NewAssetHandler(db *database.DB) *AssetHandler {
	return &AssetHandler{db: db}
}

// ListAssets handles GET /api/assets.
func (h *AssetHandler) ListAssets(c *gin.Context) {
	p := database.ListAssetsParams{
		ProjectID: c.Query("project_id"),
		Status:    c.Query("status"),
		RiskLevel: c.Query("risk_level"),
		Search:    c.Query("search"),
	}
	if lim, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil {
		p.Limit = lim
	}
	if off, err := strconv.Atoi(c.DefaultQuery("offset", "0")); err == nil {
		p.Offset = off
	}

	assets, total, err := h.db.ListAssets(p)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list assets"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"assets": assets, "total": total})
}

// CreateAsset handles POST /api/assets.
func (h *AssetHandler) CreateAsset(c *gin.Context) {
	var a database.Asset
	if err := c.ShouldBindJSON(&a); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid asset data"})
		return
	}
	ownerID, _ := c.Get(middleware.CtxUserID)
	a.OwnerUserID, _ = ownerID.(string)
	a.Source = "manual"

	asset, err := h.db.UpsertAsset(&a)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create asset"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"asset": asset})
}

// GetAsset handles GET /api/assets/:id.
func (h *AssetHandler) GetAsset(c *gin.Context) {
	asset, err := h.db.GetAssetByID(c.Param("id"))
	if err != nil || asset == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "asset not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"asset": asset})
}

// DeleteAsset handles DELETE /api/assets/:id.
func (h *AssetHandler) DeleteAsset(c *gin.Context) {
	if err := h.db.DeleteAsset(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete asset"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "asset deleted"})
}

// VulnHandler handles vulnerability management endpoints.
type VulnHandler struct {
	db *database.DB
}

// NewVulnHandler creates a VulnHandler.
func NewVulnHandler(db *database.DB) *VulnHandler {
	return &VulnHandler{db: db}
}

// ListVulnerabilities handles GET /api/vulnerabilities.
func (h *VulnHandler) ListVulnerabilities(c *gin.Context) {
	p := database.ListVulnsParams{
		ProjectID: c.Query("project_id"),
		AssetID:   c.Query("asset_id"),
		Severity:  c.Query("severity"),
		Status:    c.Query("status"),
	}
	if lim, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil {
		p.Limit = lim
	}
	if off, err := strconv.Atoi(c.DefaultQuery("offset", "0")); err == nil {
		p.Offset = off
	}

	vulns, total, err := h.db.ListVulnerabilities(p)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list vulnerabilities"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"vulnerabilities": vulns, "total": total})
}

// CreateVulnerability handles POST /api/vulnerabilities.
func (h *VulnHandler) CreateVulnerability(c *gin.Context) {
	var v database.Vulnerability
	if err := c.ShouldBindJSON(&v); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vulnerability data"})
		return
	}
	vuln, err := h.db.CreateVulnerability(&v)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create vulnerability"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"vulnerability": vuln})
}

// GetVulnerability handles GET /api/vulnerabilities/:id.
func (h *VulnHandler) GetVulnerability(c *gin.Context) {
	vuln, err := h.db.GetVulnerabilityByID(c.Param("id"))
	if err != nil || vuln == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "vulnerability not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"vulnerability": vuln})
}

// updateVulnRequest is the body for PATCH /api/vulnerabilities/:id.
type updateVulnRequest struct {
	Severity       string `json:"severity"`
	Status         string `json:"status"`
	Description    string `json:"description"`
	Recommendation string `json:"recommendation"`
}

// UpdateVulnerability handles PATCH /api/vulnerabilities/:id.
func (h *VulnHandler) UpdateVulnerability(c *gin.Context) {
	id := c.Param("id")
	var req updateVulnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if err := h.db.UpdateVulnerability(id, req.Severity, req.Status, req.Description, req.Recommendation); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update vulnerability"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "vulnerability updated"})
}

// DeleteVulnerability handles DELETE /api/vulnerabilities/:id.
func (h *VulnHandler) DeleteVulnerability(c *gin.Context) {
	if err := h.db.DeleteVulnerability(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete vulnerability"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "vulnerability deleted"})
}
