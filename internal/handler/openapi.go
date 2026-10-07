package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// OpenAPIHandler serves the OpenAPI 3.0 specification for Kestrel.
type OpenAPIHandler struct{}

// NewOpenAPIHandler creates an OpenAPIHandler.
func NewOpenAPIHandler() *OpenAPIHandler {
	return &OpenAPIHandler{}
}

// GetSpec handles GET /api/openapi/spec.
func (h *OpenAPIHandler) GetSpec(c *gin.Context) {
	spec := gin.H{
		"openapi": "3.0.0",
		"info": gin.H{
			"title":       "Kestrel Security Operations API",
			"version":     "1.0.0",
			"description": "API specification for Kestrel security operations platform covering task orchestration, recon tools, skills, projects, and RBAC.",
		},
		"servers": []gin.H{
			{"url": "/api", "description": "Current Kestrel Server"},
		},
		"paths": gin.H{
			"/auth/login": gin.H{
				"post": gin.H{
					"summary": "Authenticate user session",
					"tags":    []string{"Authentication"},
					"responses": gin.H{
						"200": gin.H{"description": "Authentication token and user profile"},
					},
				},
			},
			"/dashboard/stats": gin.H{
				"get": gin.H{
					"summary": "Retrieve system overview metrics",
					"tags":    []string{"Dashboard"},
					"responses": gin.H{
						"200": gin.H{"description": "Statistical summary counts"},
					},
				},
			},
			"/agent/run": gin.H{
				"post": gin.H{
					"summary": "Execute autonomous security agent turn",
					"tags":    []string{"Agent"},
					"responses": gin.H{
						"200": gin.H{"description": "Agent run output and execution steps"},
					},
				},
			},
			"/skills": gin.H{
				"get": gin.H{
					"summary": "List available Agent Skills packages",
					"tags":    []string{"Skills"},
					"responses": gin.H{
						"200": gin.H{"description": "Array of skill summaries"},
					},
				},
			},
			"/tools/catalog": gin.H{
				"get": gin.H{
					"summary": "Browse curated tool recipes catalog",
					"tags":    []string{"Tools"},
					"responses": gin.H{
						"200": gin.H{"description": "Cataloged security tool specifications"},
					},
				},
			},
			"/projects": gin.H{
				"get": gin.H{
					"summary": "List active testing projects",
					"tags":    []string{"Projects"},
					"responses": gin.H{
						"200": gin.H{"description": "List of projects"},
					},
				},
			},
			"/vulnerabilities": gin.H{
				"get": gin.H{
					"summary": "Query recorded vulnerabilities",
					"tags":    []string{"Vulnerabilities"},
					"responses": gin.H{
						"200": gin.H{"description": "Vulnerability findings"},
					},
				},
			},
			"/assets": gin.H{
				"get": gin.H{
					"summary": "Query normalized asset inventory",
					"tags":    []string{"Assets"},
					"responses": gin.H{
						"200": gin.H{"description": "Asset list"},
					},
				},
			},
			"/monitor/status": gin.H{
				"get": gin.H{
					"summary": "Query system runtime health and metrics",
					"tags":    []string{"Monitoring"},
					"responses": gin.H{
						"200": gin.H{"description": "Runtime health statistics"},
					},
				},
			},
			"/terminal/exec": gin.H{
				"post": gin.H{
					"summary": "Execute authorized shell command",
					"tags":    []string{"Terminal"},
					"responses": gin.H{
						"200": gin.H{"description": "Command execution stdout and return code"},
					},
				},
			},
		},
	}
	c.JSON(http.StatusOK, spec)
}
