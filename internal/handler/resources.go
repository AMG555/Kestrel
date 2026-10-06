package handler

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"kestrel/internal/database"
	"kestrel/internal/middleware"
)

// roleNameRe matches valid role names: alphanumeric, hyphens, underscores only.
var roleNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

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

// AuditSummary handles GET /api/audit/summary.
func (h *AuditHandler) AuditSummary(c *gin.Context) {
	var total, failures, recent7d int
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&total)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE result='failure'`).Scan(&failures)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE created_at >= datetime('now','-7 days')`).Scan(&recent7d)
	c.JSON(http.StatusOK, gin.H{
		"total":      total,
		"failures":   failures,
		"recent_7d":  recent7d,
	})
}

// ExportAuditCSV handles GET /api/audit/export.csv.
func (h *AuditHandler) ExportAuditCSV(c *gin.Context) {
	// Fetch up to 10,000 records for export.
	logs, _, err := h.db.ListAuditLogs(database.ListAuditLogsParams{Limit: 10000})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve audit logs"})
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="audit_%s.csv"`, time.Now().Format("20060102_150405")))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"id", "created_at", "actor_id", "actor_name", "action", "category", "result", "resource_type", "resource_id", "client_ip", "message"})
	for _, l := range logs {
		_ = w.Write([]string{
			l.ID, l.CreatedAt.Format(time.RFC3339),
			l.ActorID, l.ActorName,
			l.Action, l.Category, l.Result,
			l.ResourceType, l.ResourceID,
			l.ClientIP, l.Message,
		})
	}
	w.Flush()
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
	actorID, _ := c.Get(middleware.CtxUserID)
	aID, _ := actorID.(string)
	actorName, _ := c.Get(middleware.CtxUsername)
	aName, _ := actorName.(string)

	var req createRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	if !roleNameRe.MatchString(req.Name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role name must be 1-64 alphanumeric/hyphen/underscore characters"})
		return
	}

	role, err := h.db.CreateRole(req.Name, req.Description, false, req.Permissions, req.AllowedTools, req.HITLMode)
	if err != nil {
		_ = h.db.WriteAuditLog(database.AuditParams{
			ActorID: aID, ActorName: aName,
			Action: "create_role", Category: "rbac", Result: "failure",
			ResourceType: "role", Message: "role creation failed: " + err.Error(),
			ClientIP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
		})
		c.JSON(http.StatusConflict, gin.H{"error": "role name already exists or database error"})
		return
	}

	_ = h.db.WriteAuditLog(database.AuditParams{
		ActorID: aID, ActorName: aName,
		Action: "create_role", Category: "rbac", Result: "success",
		ResourceType: "role", ResourceID: role.ID, Message: "role created: " + role.Name,
		ClientIP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
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
	actorID, _ := c.Get(middleware.CtxUserID)
	aID, _ := actorID.(string)
	actorName, _ := c.Get(middleware.CtxUsername)
	aName, _ := actorName.(string)

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

	_ = h.db.WriteAuditLog(database.AuditParams{
		ActorID: aID, ActorName: aName,
		Action: "update_role", Category: "rbac", Result: "success",
		ResourceType: "role", ResourceID: id, Message: "role updated: " + role.Name,
		ClientIP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
	c.JSON(http.StatusOK, gin.H{"message": "role updated"})
}

// DeleteRole handles DELETE /api/roles/:id.
func (h *RoleHandler) DeleteRole(c *gin.Context) {
	actorID, _ := c.Get(middleware.CtxUserID)
	aID, _ := actorID.(string)
	actorName, _ := c.Get(middleware.CtxUsername)
	aName, _ := actorName.(string)

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

	_ = h.db.WriteAuditLog(database.AuditParams{
		ActorID: aID, ActorName: aName,
		Action: "delete_role", Category: "rbac", Result: "success",
		ResourceType: "role", ResourceID: id, Message: "role deleted: " + role.Name,
		ClientIP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
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

// ExportAssetsCSV handles GET /api/assets/export.csv.
func (h *AssetHandler) ExportAssetsCSV(c *gin.Context) {
	assets, _, err := h.db.ListAssets(database.ListAssetsParams{
		ProjectID: c.Query("project_id"),
		Limit:     50000,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list assets"})
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="assets_%s.csv"`, time.Now().Format("20060102_150405")))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"id", "host", "ip", "domain", "port", "protocol", "status", "risk_level", "source", "project_id", "tags", "created_at"})
	for _, a := range assets {
		_ = w.Write([]string{
			a.ID, a.Host, a.IP, a.Domain,
			strconv.Itoa(a.Port), a.Protocol,
			a.Status, a.RiskLevel, a.Source,
			a.ProjectID,
			strings.Join(a.Tags, "|"),
			a.CreatedAt.Format(time.RFC3339),
		})
	}
	w.Flush()
}

// CreateAsset handles POST /api/assets.
func (h *AssetHandler) CreateAsset(c *gin.Context) {
	actorID, _ := c.Get(middleware.CtxUserID)
	aID, _ := actorID.(string)
	actorName, _ := c.Get(middleware.CtxUsername)
	aName, _ := actorName.(string)

	var a database.Asset
	if err := c.ShouldBindJSON(&a); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid asset data"})
		return
	}
	a.OwnerUserID = aID
	a.Source = "manual"

	asset, err := h.db.UpsertAsset(&a)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create asset"})
		return
	}

	_ = h.db.WriteAuditLog(database.AuditParams{
		ActorID: aID, ActorName: aName,
		Action: "create_asset", Category: "assets", Result: "success",
		ResourceType: "asset", ResourceID: asset.ID,
		Message:   fmt.Sprintf("asset created: %s (%s:%d)", asset.Host, asset.IP, asset.Port),
		ClientIP:  c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
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
	actorID, _ := c.Get(middleware.CtxUserID)
	aID, _ := actorID.(string)
	actorName, _ := c.Get(middleware.CtxUsername)
	aName, _ := actorName.(string)
	id := c.Param("id")

	if err := h.db.DeleteAsset(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete asset"})
		return
	}

	_ = h.db.WriteAuditLog(database.AuditParams{
		ActorID: aID, ActorName: aName,
		Action: "delete_asset", Category: "assets", Result: "success",
		ResourceType: "asset", ResourceID: id, Message: "asset deleted",
		ClientIP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
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

// ExportVulnsCSV handles GET /api/vulnerabilities/export.csv.
func (h *VulnHandler) ExportVulnsCSV(c *gin.Context) {
	vulns, _, err := h.db.ListVulnerabilities(database.ListVulnsParams{
		ProjectID: c.Query("project_id"),
		Limit:     50000,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list vulnerabilities"})
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="vulns_%s.csv"`, time.Now().Format("20060102_150405")))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"id", "title", "severity", "status", "asset_id", "project_id", "vuln_type", "target", "description", "recommendation", "created_at"})
	for _, v := range vulns {
		_ = w.Write([]string{
			v.ID, v.Title, v.Severity, v.Status,
			v.AssetID, v.ProjectID, v.VulnType, v.Target,
			v.Description, v.Recommendation,
			v.CreatedAt.Format(time.RFC3339),
		})
	}
	w.Flush()
}

// CreateVulnerability handles POST /api/vulnerabilities.
func (h *VulnHandler) CreateVulnerability(c *gin.Context) {
	actorID, _ := c.Get(middleware.CtxUserID)
	aID, _ := actorID.(string)
	actorName, _ := c.Get(middleware.CtxUsername)
	aName, _ := actorName.(string)

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

	_ = h.db.WriteAuditLog(database.AuditParams{
		ActorID: aID, ActorName: aName,
		Action: "create_vuln", Category: "vulns", Result: "success",
		ResourceType: "vulnerability", ResourceID: vuln.ID,
		Message:   fmt.Sprintf("vulnerability created: %s [%s]", vuln.Title, vuln.Severity),
		ClientIP:  c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
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
	actorID, _ := c.Get(middleware.CtxUserID)
	aID, _ := actorID.(string)
	actorName, _ := c.Get(middleware.CtxUsername)
	aName, _ := actorName.(string)

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

	_ = h.db.WriteAuditLog(database.AuditParams{
		ActorID: aID, ActorName: aName,
		Action: "update_vuln", Category: "vulns", Result: "success",
		ResourceType: "vulnerability", ResourceID: id,
		Message:   fmt.Sprintf("vulnerability updated: status=%s severity=%s", req.Status, req.Severity),
		ClientIP:  c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
	c.JSON(http.StatusOK, gin.H{"message": "vulnerability updated"})
}

// DeleteVulnerability handles DELETE /api/vulnerabilities/:id.
func (h *VulnHandler) DeleteVulnerability(c *gin.Context) {
	actorID, _ := c.Get(middleware.CtxUserID)
	aID, _ := actorID.(string)
	actorName, _ := c.Get(middleware.CtxUsername)
	aName, _ := actorName.(string)
	id := c.Param("id")

	if err := h.db.DeleteVulnerability(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete vulnerability"})
		return
	}

	_ = h.db.WriteAuditLog(database.AuditParams{
		ActorID: aID, ActorName: aName,
		Action: "delete_vuln", Category: "vulns", Result: "success",
		ResourceType: "vulnerability", ResourceID: id, Message: "vulnerability deleted",
		ClientIP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
	c.JSON(http.StatusOK, gin.H{"message": "vulnerability deleted"})
}
