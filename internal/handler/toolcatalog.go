package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"kestrel/internal/toolrecipe"
)

// ToolCatalogHandler provides querying over the curated YAML tool recipe catalog.
type ToolCatalogHandler struct {
	registry *toolrecipe.Registry
}

// NewToolCatalogHandler creates a new tool catalog handler.
func NewToolCatalogHandler(registry *toolrecipe.Registry) *ToolCatalogHandler {
	return &ToolCatalogHandler{registry: registry}
}

// ListCatalog returns all curated tool recipes.
// GET /api/tools/catalog
func (h *ToolCatalogHandler) ListCatalog(c *gin.Context) {
	category := strings.ToLower(c.Query("category"))
	q := strings.ToLower(c.Query("q"))

	all := h.registry.List()
	filtered := make([]*toolrecipe.Recipe, 0, len(all))

	for _, rec := range all {
		if category != "" && strings.ToLower(rec.Category) != category {
			continue
		}
		if q != "" {
			nameMatch := strings.Contains(strings.ToLower(rec.Name), q)
			descMatch := strings.Contains(strings.ToLower(rec.ShortDescription), q) || strings.Contains(strings.ToLower(rec.Description), q)
			if !nameMatch && !descMatch {
				continue
			}
		}
		filtered = append(filtered, rec)
	}

	c.JSON(http.StatusOK, gin.H{
		"tools": filtered,
		"total": len(filtered),
	})
}

// GetCatalogItem returns details for a specific tool recipe.
// GET /api/tools/catalog/:name
func (h *ToolCatalogHandler) GetCatalogItem(c *gin.Context) {
	name := c.Param("name")
	rec, ok := h.registry.Get(name)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "tool recipe not found"})
		return
	}
	c.JSON(http.StatusOK, rec)
}
