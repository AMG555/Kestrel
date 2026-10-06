package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"kestrel/internal/report"
)

// ReportHandler serves project report endpoints.
type ReportHandler struct {
	gen *report.Generator
}

// NewReportHandler creates a ReportHandler.
func NewReportHandler(gen *report.Generator) *ReportHandler {
	return &ReportHandler{gen: gen}
}

// GenerateReport handles GET /api/projects/:id/report.
// Query param `format` = markdown (default) | json | csv
func (h *ReportHandler) GenerateReport(c *gin.Context) {
	projectID := c.Param("id")
	format := report.Format(strings.ToLower(c.DefaultQuery("format", "markdown")))

	switch format {
	case report.FormatMarkdown, report.FormatJSON, report.FormatCSV:
	default:
		format = report.FormatMarkdown
	}

	r, err := h.gen.Build(projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	data, contentType, err := h.gen.Render(r, format)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to render report"})
		return
	}

	ext := map[report.Format]string{
		report.FormatMarkdown: ".md",
		report.FormatJSON:     ".json",
		report.FormatCSV:      ".csv",
	}[format]

	c.Header("Content-Disposition", `attachment; filename="report`+ext+`"`)
	c.Data(http.StatusOK, contentType, data)
}
