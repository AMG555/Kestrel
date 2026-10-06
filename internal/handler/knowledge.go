package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"kestrel/internal/knowledge"
)

// KnowledgeHandler handles knowledge base endpoints.
type KnowledgeHandler struct {
	svc *knowledge.Service
}

// NewKnowledgeHandler creates a KnowledgeHandler.
func NewKnowledgeHandler(svc *knowledge.Service) *KnowledgeHandler {
	return &KnowledgeHandler{svc: svc}
}

// ListDocuments handles GET /api/knowledge/documents.
func (h *KnowledgeHandler) ListDocuments(c *gin.Context) {
	docs, err := h.svc.ListDocuments()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list documents"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"documents": docs, "total": len(docs)})
}

// IngestText handles POST /api/knowledge/ingest — ingests a text document from the request body.
func (h *KnowledgeHandler) IngestText(c *gin.Context) {
	if !h.svc.Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "knowledge base is not enabled; set knowledge.enabled: true in config.yaml"})
		return
	}

	title := strings.TrimSpace(c.GetHeader("X-Document-Title"))
	if title == "" {
		title = "Untitled"
	}

	doc, err := h.svc.IngestReader(c.Request.Context(), title, "", c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"document": doc})
}

// Query handles POST /api/knowledge/query.
func (h *KnowledgeHandler) Query(c *gin.Context) {
	if !h.svc.Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "knowledge base is not enabled"})
		return
	}

	var req struct {
		Query   string `json:"query" binding:"required"`
		TopK    int    `json:"top_k"`
		Rewrite bool   `json:"rewrite"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}

	results, err := h.svc.Retrieve(c.Request.Context(), knowledge.QueryParams{
		Query:   req.Query,
		TopK:    req.TopK,
		Rewrite: req.Rewrite,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"results": results, "total": len(results)})
}

// DeleteDocument handles DELETE /api/knowledge/documents/:id.
func (h *KnowledgeHandler) DeleteDocument(c *gin.Context) {
	if err := h.svc.DeleteDocument(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete document"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "document deleted"})
}
