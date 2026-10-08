package handler

import (
	"net/http"

	"kestrel/internal/database"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// OpenAPIHandler OpenAPI handler
type OpenAPIHandler struct {
	db               *database.DB
	logger           *zap.Logger
	conversationHdlr *ConversationHandler
	agentHdlr        *AgentHandler
}

// NewOpenAPIHandler Create a new OpenAPI handler
func NewOpenAPIHandler(db *database.DB, logger *zap.Logger, conversationHdlr *ConversationHandler, agentHdlr *AgentHandler) *OpenAPIHandler {
	return &OpenAPIHandler{
		db:               db,
		logger:           logger,
		conversationHdlr: conversationHdlr,
		agentHdlr:        agentHdlr,
	}
}

// GetOpenAPISpec Get OpenAPI specification
func (h *OpenAPIHandler) GetOpenAPISpec(c *gin.Context) {
	host := c.Request.Host
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}

	finalizationRequestSchema := map[string]interface{}{
		"type":        "object",
		"description": "Final reply delivery strategy. The backend does not infer execution intent from natural language; execution entry points should explicitly declare yes/no requirements with completed tool evidence.",
		"properties": map[string]interface{}{
			"requireExecutionEvidence": map[string]interface{}{
				"type":        "boolean",
				"description": "When true, missing completed tool execution records trigger a no-inject re-run or final block; can be omitted or set to false for normal chat.",
			},
		},
	}

	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "Kestrel API",
			"description": "API documentation for the AI-powered automated security testing platform",
			"version":     "1.0.0",
			"contact": map[string]interface{}{
				"name": "Kestrel",
			},
		},
		"servers": []map[string]interface{}{
			{
				"url":         scheme + "://" + host,
				"description": "current server",
			},
		},
		"components": map[string]interface{}{
			"securitySchemes": map[string]interface{}{
				"bearerAuth": map[string]interface{}{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
					"description":  "Authenticate using a Bearer token. Obtain the token via /api/auth/login.",
				},
			},
			"schemas": map[string]interface{}{
				"CreateConversationRequest": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"title": map[string]interface{}{
							"type":        "string",
							"description": "conversation title",
							"example":     "Web application security test",
						},
						"projectId": map[string]interface{}{
							"type":        "string",
							"description": "Project ID to bind (optional, shared fact blackboard)",
						},
					},
				},
				"SetConversationProjectRequest": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"projectId": map[string]interface{}{
							"type":        "string",
							"description": "Project ID; empty string unbinds",
						},
					},
					"required": []string{"projectId"},
				},
				"AgentChatResponse": map[string]interface{}{
					"type":        "object",
					"description": "Agent non-streaming response. The response only delivers text; whether a successful Final reply must be determined by finalized/finalizable/status.",
					"properties": map[string]interface{}{
						"response": map[string]interface{}{
							"type":        "string",
							"description": "Text delivered to the user. When finalized=false this is a block/incomplete explanation, not a successful conclusion.",
						},
						"conversationId": map[string]interface{}{
							"type":        "string",
							"description": "conversation ID",
						},
						"assistantMessageId": map[string]interface{}{
							"type":        "string",
							"description": "Assistant message ID (returned by some endpoints)",
						},
						"mcpExecutionIds": map[string]interface{}{
							"type":        "array",
							"description": "MCP tool execution IDs associated with this round",
							"items":       map[string]interface{}{"type": "string"},
						},
						"agentMode": map[string]interface{}{
							"type":        "string",
							"description": "Agent pattern, e.g. eino_single, eino_deep, workflow",
						},
						"finalized": map[string]interface{}{
							"type":        "boolean",
							"description": "Whether the Final reply check has passed. Only true can be treated as a successful Final reply.",
						},
						"finalizable": map[string]interface{}{
							"type":        "boolean",
							"description": "Whether the candidate output can be promoted to a Final reply.",
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "Finalization status",
							"enum":        []string{"completed", "in_progress", "blocked", "failed", "cancelled", "awaiting_hitl"},
						},
						"completionReason": map[string]interface{}{
							"type":        "string",
							"description": "Finalization or block reason, e.g. verified, pending_tool_executions, missing_execution_evidence",
						},
						"evidenceVerified": map[string]interface{}{
							"type":        "boolean",
							"description": "Whether evidence meets finalization requirements",
						},
						"evidenceRefs": map[string]interface{}{
							"type":        "array",
							"description": "Evidence references, e.g. mcp_execution:<id>",
							"items":       map[string]interface{}{"type": "string"},
						},
						"pendingExecutionIds": map[string]interface{}{
							"type":        "array",
							"description": "Tool execution IDs still in queued/running state",
							"items":       map[string]interface{}{"type": "string"},
						},
						"missingChecks": map[string]interface{}{
							"type":        "array",
							"description": "List of reasons that failed the finalization check",
							"items":       map[string]interface{}{"type": "string"},
						},
					},
					"required": []string{"response", "conversationId", "finalized", "finalizable", "status", "evidenceVerified"},
				},
				"Conversation": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "conversation ID",
							"example":     "550e8400-e29b-41d4-a716-446655440000",
						},
						"title": map[string]interface{}{
							"type":        "string",
							"description": "conversation title",
							"example":     "Web application security test",
						},
						"createdAt": map[string]interface{}{
							"type":        "string",
							"format":      "date-time",
							"description": "created at",
						},
						"updatedAt": map[string]interface{}{
							"type":        "string",
							"format":      "date-time",
							"description": "updated at",
						},
						"projectId": map[string]interface{}{
							"type":        "string",
							"description": "Project ID to bind (optional)",
						},
					},
				},
				"ConversationDetail": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "conversation ID",
						},
						"title": map[string]interface{}{
							"type":        "string",
							"description": "conversation title",
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "Conversation status: active (in progress), completed (completed), failed (failed)",
							"enum":        []string{"active", "completed", "failed"},
						},
						"createdAt": map[string]interface{}{
							"type":        "string",
							"format":      "date-time",
							"description": "created at",
						},
						"updatedAt": map[string]interface{}{
							"type":        "string",
							"format":      "date-time",
							"description": "updated at",
						},
						"messages": map[string]interface{}{
							"type":        "array",
							"description": "Message list",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/Message",
							},
						},
						"messageCount": map[string]interface{}{
							"type":        "integer",
							"description": "Message count",
						},
					},
				},
				"Message": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "messageID",
						},
						"conversationId": map[string]interface{}{
							"type":        "string",
							"description": "conversation ID",
						},
						"role": map[string]interface{}{
							"type":        "string",
							"description": "Message role: user (user), assistant (assistant)",
							"enum":        []string{"user", "assistant"},
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "Message content",
						},
						"createdAt": map[string]interface{}{
							"type":        "string",
							"format":      "date-time",
							"description": "created at",
						},
					},
				},
				"ConversationResults": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"conversationId": map[string]interface{}{
							"type":        "string",
							"description": "conversation ID",
						},
						"messages": map[string]interface{}{
							"type":        "array",
							"description": "Message list",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/Message",
							},
						},
						"vulnerabilities": map[string]interface{}{
							"type":        "array",
							"description": "Discovered vulnerability list",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/Vulnerability",
							},
						},
						"executionResults": map[string]interface{}{
							"type":        "array",
							"description": "Execution result list",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/ExecutionResult",
							},
						},
					},
				},
				"Vulnerability": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "Vulnerability ID",
						},
						"title": map[string]interface{}{
							"type":        "string",
							"description": "vulnerability title",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "vulnerability description",
						},
						"severity": map[string]interface{}{
							"type":        "string",
							"description": "Severity level",
							"enum":        []string{"critical", "high", "medium", "low", "info"},
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "status",
							"enum":        []string{"open", "confirmed", "fixed", "false_positive", "ignored"},
						},
						"target": map[string]interface{}{
							"type":        "string",
							"description": "Affected target",
						},
					},
				},
				"AssetImportItem": map[string]interface{}{
					"type":        "object",
					"description": "Asset to import; at least one of host, ip, domain must be non-empty",
					"properties": map[string]interface{}{
						"project_id":         map[string]interface{}{"type": "string", "description": "Parent project ID; caller must have access rights"},
						"host":               map[string]interface{}{"type": "string", "maxLength": 500, "example": "https://app.example.com:443"},
						"ip":                 map[string]interface{}{"type": "string", "example": "192.0.2.10"},
						"port":               map[string]interface{}{"type": "integer", "minimum": 0, "maximum": 65535, "example": 443},
						"domain":             map[string]interface{}{"type": "string", "example": "app.example.com"},
						"protocol":           map[string]interface{}{"type": "string", "example": "https"},
						"title":              map[string]interface{}{"type": "string", "maxLength": 500},
						"server":             map[string]interface{}{"type": "string", "maxLength": 255, "example": "nginx"},
						"country":            map[string]interface{}{"type": "string"},
						"province":           map[string]interface{}{"type": "string"},
						"city":               map[string]interface{}{"type": "string"},
						"responsible_person": map[string]interface{}{"type": "string", "maxLength": 255, "description": "Asset responsible person"},
						"department":         map[string]interface{}{"type": "string", "maxLength": 255, "description": "Department"},
						"business_system":    map[string]interface{}{"type": "string", "maxLength": 255, "description": "Business system"},
						"environment":        map[string]interface{}{"type": "string", "enum": []string{"production", "staging", "testing", "development", "other"}},
						"criticality":        map[string]interface{}{"type": "string", "enum": []string{"critical", "high", "medium", "low"}},
						"source":             map[string]interface{}{"type": "string"},
						"source_query":       map[string]interface{}{"type": "string"},
						"status":             map[string]interface{}{"type": "string", "enum": []string{"active", "inactive"}, "default": "active"},
						"tags": map[string]interface{}{
							"type":     "array",
							"maxItems": 30,
							"items":    map[string]interface{}{"type": "string", "maxLength": 64},
						},
					},
				},
				"AssetImportRequest": map[string]interface{}{
					"type":     "object",
					"required": []string{"assets"},
					"properties": map[string]interface{}{
						"assets": map[string]interface{}{
							"type":     "array",
							"minItems": 1,
							"maxItems": 100000,
							"items":    map[string]interface{}{"$ref": "#/components/schemas/AssetImportItem"},
						},
						"source":       map[string]interface{}{"type": "string", "description": "Default source used when source is not specified in the asset"},
						"source_query": map[string]interface{}{"type": "string", "description": "Default source query or import filename"},
					},
				},
				"AssetImportResult": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"created": map[string]interface{}{"type": "integer", "description": "Number of newly created", "example": 120},
						"updated": map[string]interface{}{"type": "integer", "description": "Number of deduplicated/merged", "example": 8},
						"skipped": map[string]interface{}{"type": "integer", "description": "Number skipped", "example": 2},
					},
				},
				"ExecutionResult": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "Execution ID",
						},
						"toolName": map[string]interface{}{
							"type":        "string",
							"description": "tool name",
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "Execution status",
							"enum":        []string{"queued", "running", "completed", "failed", "cancelled", "hard_timeout", "orphaned"},
						},
						"result": map[string]interface{}{
							"type":        "string",
							"description": "execution result",
						},
						"createdAt": map[string]interface{}{
							"type":        "string",
							"format":      "date-time",
							"description": "created at",
						},
					},
				},
				"Error": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"error": map[string]interface{}{
							"type":        "string",
							"description": "Error info",
						},
					},
				},
				"LoginRequest": map[string]interface{}{
					"type":     "object",
					"required": []string{"password"},
					"properties": map[string]interface{}{
						"password": map[string]interface{}{
							"type":        "string",
							"description": "Login password",
						},
					},
				},
				"LoginResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"token": map[string]interface{}{
							"type":        "string",
							"description": "Authentication token",
						},
						"expires_at": map[string]interface{}{
							"type":        "string",
							"format":      "date-time",
							"description": "Tokenexpires at",
						},
						"session_duration_hr": map[string]interface{}{
							"type":        "integer",
							"description": "session duration (hours)",
						},
					},
				},
				"ChangePasswordRequest": map[string]interface{}{
					"type":     "object",
					"required": []string{"oldPassword", "newPassword"},
					"properties": map[string]interface{}{
						"oldPassword": map[string]interface{}{
							"type":        "string",
							"description": "Current password",
						},
						"newPassword": map[string]interface{}{
							"type":        "string",
							"description": "New password (at least 8 characters)",
						},
					},
				},
				"UpdateConversationRequest": map[string]interface{}{
					"type":     "object",
					"required": []string{"title"},
					"properties": map[string]interface{}{
						"title": map[string]interface{}{
							"type":        "string",
							"description": "conversation title",
						},
					},
				},
				"BatchTaskRequest": map[string]interface{}{
					"type":     "object",
					"required": []string{"tasks"},
					"properties": map[string]interface{}{
						"title": map[string]interface{}{
							"type":        "string",
							"description": "Task title (optional)",
						},
						"tasks": map[string]interface{}{
							"type":        "array",
							"description": "Task list, one task per line",
							"items": map[string]interface{}{
								"type": "string",
							},
						},
						"role": map[string]interface{}{
							"type":        "string",
							"description": "Role name (optional)",
						},
						"agentMode": map[string]interface{}{
							"type":        "string",
							"description": "Agent pattern: eino_single (Eino ADK single agent, default) | deep | plan_execute | supervisor",
							"enum":        []string{"eino_single", "deep", "plan_execute", "supervisor"},
						},
						"scheduleMode": map[string]interface{}{
							"type":        "string",
							"description": "Schedule mode (manual | cron)",
							"enum":        []string{"manual", "cron"},
						},
						"cronExpr": map[string]interface{}{
							"type":        "string",
							"description": "Cron expression (required when scheduleMode=cron)",
						},
						"executeNow": map[string]interface{}{
							"type":        "boolean",
							"description": "Whether to execute immediately after creation (default false)",
						},
					},
				},
				"BatchQueue": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "queueID",
						},
						"title": map[string]interface{}{
							"type":        "string",
							"description": "queuetitle",
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "queuestatus",
							"enum":        []string{"pending", "running", "paused", "completed", "failed"},
						},
						"tasks": map[string]interface{}{
							"type":        "array",
							"description": "tasklist",
							"items": map[string]interface{}{
								"type": "object",
							},
						},
						"createdAt": map[string]interface{}{
							"type":        "string",
							"format":      "date-time",
							"description": "created at",
						},
					},
				},
				"CancelAgentLoopRequest": map[string]interface{}{
					"type":     "object",
					"required": []string{"conversationId"},
					"properties": map[string]interface{}{
						"conversationId": map[string]interface{}{
							"type":        "string",
							"description": "conversation ID",
						},
						"reason": map[string]interface{}{
							"type":        "string",
							"description": "Optional. Consistent with the MCP monitor page \"Terminate and Explain\": when non-empty, merged into the current tool output returned to the model (includes USER INTERRUPT NOTE block)",
						},
						"continueAfter": map[string]interface{}{
							"type":        "boolean",
							"description": "When true, only terminates the currently in-progress MCP tool call (does not cancel the entire task round); a tool must already be executing, otherwise 400",
						},
					},
				},
				"AgentTask": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"conversationId": map[string]interface{}{
							"type":        "string",
							"description": "conversation ID",
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "task status",
							"enum":        []string{"running", "completed", "failed", "cancelled", "timeout"},
						},
						"startedAt": map[string]interface{}{
							"type":        "string",
							"format":      "date-time",
							"description": "start time",
						},
					},
				},
				"CreateVulnerabilityRequest": map[string]interface{}{
					"type":     "object",
					"required": []string{"conversation_id", "title", "description", "severity", "type", "target", "reproduction_steps", "evidence", "impact", "recommendation"},
					"properties": map[string]interface{}{
						"conversation_id": map[string]interface{}{
							"type":        "string",
							"description": "conversation ID",
						},
						"title": map[string]interface{}{
							"type":        "string",
							"description": "vulnerability title",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "vulnerability description",
						},
						"severity": map[string]interface{}{
							"type":        "string",
							"description": "Severity level",
							"enum":        []string{"critical", "high", "medium", "low", "info"},
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "status",
							"enum":        []string{"open", "closed", "fixed"},
						},
						"type": map[string]interface{}{
							"type":        "string",
							"description": "vulnerability type",
						},
						"target": map[string]interface{}{
							"type":        "string",
							"description": "Affected target",
						},
						"preconditions":      map[string]interface{}{"type": "string", "description": "preconditions"},
						"reproduction_steps": map[string]interface{}{"type": "string", "description": "Reproduction steps"},
						"evidence":           map[string]interface{}{"type": "string", "description": "evidence/POC, including request/response, command output, screenshot descriptions, logs, etc."},
						"impact": map[string]interface{}{
							"type":        "string",
							"description": "impact",
						},
						"recommendation": map[string]interface{}{
							"type":        "string",
							"description": "Remediation advice",
						},
						"retest_notes": map[string]interface{}{"type": "string", "description": "retest method"},
					},
				},
				"UpdateVulnerabilityRequest": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"title": map[string]interface{}{
							"type":        "string",
							"description": "vulnerability title",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "vulnerability description",
						},
						"severity": map[string]interface{}{
							"type":        "string",
							"description": "Severity level",
							"enum":        []string{"critical", "high", "medium", "low", "info"},
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "status",
							"enum":        []string{"open", "confirmed", "fixed", "false_positive", "ignored"},
						},
						"type": map[string]interface{}{
							"type":        "string",
							"description": "vulnerability type",
						},
						"target": map[string]interface{}{
							"type":        "string",
							"description": "Affected target",
						},
						"preconditions":      map[string]interface{}{"type": "string", "description": "preconditions"},
						"reproduction_steps": map[string]interface{}{"type": "string", "description": "Reproduction steps"},
						"evidence":           map[string]interface{}{"type": "string", "description": "evidence/POC, including request/response, command output, screenshot descriptions, logs, etc."},
						"impact": map[string]interface{}{
							"type":        "string",
							"description": "impact",
						},
						"recommendation": map[string]interface{}{
							"type":        "string",
							"description": "Remediation advice",
						},
						"retest_notes": map[string]interface{}{"type": "string", "description": "retest method"},
					},
				},
				"ListVulnerabilitiesResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"vulnerabilities": map[string]interface{}{
							"type":        "array",
							"description": "Vulnerability list",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/Vulnerability",
							},
						},
						"total": map[string]interface{}{
							"type":        "integer",
							"description": "total",
						},
						"page": map[string]interface{}{
							"type":        "integer",
							"description": "Current page",
						},
						"page_size": map[string]interface{}{
							"type":        "integer",
							"description": "page size",
						},
						"total_pages": map[string]interface{}{
							"type":        "integer",
							"description": "Total pages",
						},
					},
				},
				"VulnerabilityStats": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"total": map[string]interface{}{
							"type":        "integer",
							"description": "Total vulnerability count",
						},
						"by_severity": map[string]interface{}{
							"type":        "object",
							"description": "Statistics by severity",
						},
						"by_status": map[string]interface{}{
							"type":        "object",
							"description": "Statistics by status",
						},
					},
				},
				"RoleConfig": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "role name",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "role description",
						},
						"enabled": map[string]interface{}{
							"type":        "boolean",
							"description": "enabled",
						},
						"systemPrompt": map[string]interface{}{
							"type":        "string",
							"description": "System prompt",
						},
						"userPrompt": map[string]interface{}{
							"type":        "string",
							"description": "User prompt",
						},
						"tools": map[string]interface{}{
							"type":        "array",
							"description": "tool list",
							"items": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
				"Skill": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Skillname",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "Skilldescription",
						},
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Skillpath",
						},
					},
				},
				"CreateSkillRequest": map[string]interface{}{
					"type":     "object",
					"required": []string{"name", "description"},
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Skillname",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "Skilldescription",
						},
					},
				},
				"UpdateSkillRequest": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"description": map[string]interface{}{
							"type":        "string",
							"description": "Skilldescription",
						},
					},
				},
				"ToolExecution": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "Execution ID",
						},
						"toolName": map[string]interface{}{
							"type":        "string",
							"description": "tool name",
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "Execution status",
							"enum":        []string{"queued", "running", "completed", "failed", "cancelled", "hard_timeout", "orphaned"},
						},
						"createdAt": map[string]interface{}{
							"type":        "string",
							"format":      "date-time",
							"description": "created at",
						},
					},
				},
				"MonitorResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"executions": map[string]interface{}{
							"type":        "array",
							"description": "Execution record list (lightweight fields, excluding arguments/result)",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/ToolExecution",
							},
						},
						"summary": map[string]interface{}{
							"type":        "object",
							"description": "Tool call summary",
						},
						"topTools": map[string]interface{}{
							"type":        "array",
							"description": "top N tools by call volume",
							"items": map[string]interface{}{
								"type": "object",
							},
						},
						"timestamp": map[string]interface{}{
							"type":        "string",
							"format":      "date-time",
							"description": "Timestamp",
						},
						"total": map[string]interface{}{
							"type":        "integer",
							"description": "Execution record total",
						},
						"page": map[string]interface{}{
							"type":        "integer",
							"description": "Current page",
						},
						"pageSize": map[string]interface{}{
							"type":        "integer",
							"description": "page size",
						},
						"totalPages": map[string]interface{}{
							"type":        "integer",
							"description": "Total pages",
						},
						"retentionDays": map[string]interface{}{
							"type":        "integer",
							"description": "Execution record retention days",
						},
					},
				},
				"ConfigResponse": map[string]interface{}{
					"type":        "object",
					"description": "Configuration info (including openai, vision, multi_agent, etc.)",
					"properties": map[string]interface{}{
						"agent": map[string]interface{}{
							"$ref": "#/components/schemas/AgentConfig",
						},
						"vision": map[string]interface{}{
							"$ref": "#/components/schemas/VisionConfig",
						},
					},
				},
				"UpdateConfigRequest": map[string]interface{}{
					"type":        "object",
					"description": "update configurationrequest",
					"properties": map[string]interface{}{
						"agent": map[string]interface{}{
							"$ref": "#/components/schemas/AgentConfig",
						},
						"vision": map[string]interface{}{
							"$ref": "#/components/schemas/VisionConfig",
						},
					},
				},
				"AgentConfig": map[string]interface{}{
					"type":        "object",
					"description": "Agent runtime and external MCP deadlock protection config",
					"properties": map[string]interface{}{
						"max_iterations":                         map[string]interface{}{"type": "integer", "description": "maximum iterations"},
						"tool_timeout_minutes":                   map[string]interface{}{"type": "integer", "description": "Single tool execution hard timeout (minutes)"},
						"tool_wait_timeout_seconds":              map[string]interface{}{"type": "integer", "description": "Tool single-round wait seconds; returns execution_id when elapsed, worker continues background execution"},
						"external_mcp_max_concurrent_per_server": map[string]interface{}{"type": "integer", "description": "per external MCP server concurrent limit; 0=default 2; negative=unlimited"},
						"external_mcp_max_concurrent_total":      map[string]interface{}{"type": "integer", "description": "global external MCP concurrent limit; 0=default 16; negative=unlimited"},
						"external_mcp_circuit_failure_threshold": map[string]interface{}{"type": "integer", "description": "consecutive failure circuit breaker threshold; 0=default 3; negative=disable circuit breaker"},
						"external_mcp_circuit_cooldown_seconds":  map[string]interface{}{"type": "integer", "description": "circuit breaker cooldown seconds; 0=default 60"},
						"shell_no_output_timeout_seconds":        map[string]interface{}{"type": "integer", "description": "exec/shell consecutive no-output idle termination seconds"},
						"workspace_root_dir":                     map[string]interface{}{"type": "string", "description": "Session working directory root path"},
						"system_prompt_path":                     map[string]interface{}{"type": "string", "description": "Single agent system prompt file path"},
					},
				},
				"VisionConfig": map[string]interface{}{
					"type":        "object",
					"description": "vision analysis (analyze_image MCP tool); registers tool when enabled and model is non-empty",
					"properties": map[string]interface{}{
						"enabled":                     map[string]interface{}{"type": "boolean", "description": "enabled analyze_image"},
						"model":                       map[string]interface{}{"type": "string", "description": "vision model name (required)", "example": "qwen-vl-max"},
						"api_key":                     map[string]interface{}{"type": "string", "description": "API Key; leave empty to reuse openai.api_key"},
						"base_url":                    map[string]interface{}{"type": "string", "description": "Base URL; leave empty to reuse openai.base_url"},
						"provider":                    map[string]interface{}{"type": "string", "description": "provider; leave empty to inherit openai.provider"},
						"timeout_seconds":             map[string]interface{}{"type": "integer", "description": "VL call timeout (seconds)"},
						"max_image_bytes":             map[string]interface{}{"type": "integer", "description": "raw file size limit (bytes)"},
						"max_dimension":               map[string]interface{}{"type": "integer", "description": "long-edge resize pixels"},
						"jpeg_quality":                map[string]interface{}{"type": "integer", "description": "JPEG quality 60-100"},
						"max_payload_bytes":           map[string]interface{}{"type": "integer", "description": "API payload size limit (bytes)"},
						"skip_preprocess_below_bytes": map[string]interface{}{"type": "integer", "description": "can pass original image when below this size and dimensions are compliant; 0=always compress"},
						"detail":                      map[string]interface{}{"type": "string", "enum": []string{"low", "high", "auto"}, "description": "OpenAI-compatible image detail"},
					},
				},
				"AnalyzeImageToolCall": map[string]interface{}{
					"type":        "object",
					"description": "Built-in MCP tool analyze_image: analyzes server-local images, returns plain text (CAPTCHA/UI/error, etc.)",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Absolute image path or path relative to the process working directory",
						},
						"question": map[string]interface{}{
							"type":        "string",
							"description": "Optional: key question; for CAPTCHAs recommend \"output CAPTCHA characters only\"",
						},
					},
					"required": []string{"path"},
				},
				"ExternalMCPConfig": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"enabled": map[string]interface{}{
							"type":        "boolean",
							"description": "enabled",
						},
						"command": map[string]interface{}{
							"type":        "string",
							"description": "Command",
						},
						"args": map[string]interface{}{
							"type":        "array",
							"description": "Parameter list",
							"items": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
				"ExternalMCPResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"config": map[string]interface{}{
							"$ref": "#/components/schemas/ExternalMCPConfig",
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "status",
							"enum":        []string{"connected", "disconnected", "error", "disabled"},
						},
						"toolCount": map[string]interface{}{
							"type":        "integer",
							"description": "Tool count",
						},
						"error": map[string]interface{}{
							"type":        "string",
							"description": "Error info",
						},
					},
				},
				"AddOrUpdateExternalMCPRequest": map[string]interface{}{
					"type":     "object",
					"required": []string{"config"},
					"properties": map[string]interface{}{
						"config": map[string]interface{}{
							"$ref": "#/components/schemas/ExternalMCPConfig",
						},
					},
				},
				"AttackChain": map[string]interface{}{
					"type":        "object",
					"description": "Attack chain data",
				},
				"MCPMessage": map[string]interface{}{
					"type":        "object",
					"description": "MCP message (JSON-RPC 2.0 compliant)",
					"required":    []string{"jsonrpc"},
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"description": "Message ID, can be string, number, or null. Must be provided for requests; can be omitted for notifications",
							"oneOf": []map[string]interface{}{
								{"type": "string"},
								{"type": "number"},
								{"type": "null"},
							},
							"example": "550e8400-e29b-41d4-a716-446655440000",
						},
						"method": map[string]interface{}{
							"type":        "string",
							"description": "Method name. Supported methods:\n- `initialize`: Initialize MCP connection\n- `tools/list`: List all available tools\n- `tools/call`: Call a tool\n- `prompts/list`: List all prompt templates\n- `prompts/get`: Get a prompt template\n- `resources/list`: List all resources\n- `resources/read`: Read resource content\n- `sampling/request`: Sampling request",
							"enum": []string{
								"initialize",
								"tools/list",
								"tools/call",
								"prompts/list",
								"prompts/get",
								"resources/list",
								"resources/read",
								"sampling/request",
							},
							"example": "tools/list",
						},
						"params": map[string]interface{}{
							"description": "Method parameters (JSON object), structure varies by method",
							"type":        "object",
						},
						"jsonrpc": map[string]interface{}{
							"type":        "string",
							"description": "JSON-RPC version, fixed as \"2.0\"",
							"enum":        []string{"2.0"},
							"example":     "2.0",
						},
					},
				},
				"MCPInitializeParams": map[string]interface{}{
					"type":     "object",
					"required": []string{"protocolVersion", "capabilities", "clientInfo"},
					"properties": map[string]interface{}{
						"protocolVersion": map[string]interface{}{
							"type":        "string",
							"description": "Protocol version",
							"example":     "2024-11-05",
						},
						"capabilities": map[string]interface{}{
							"type":        "object",
							"description": "Client capabilities",
						},
						"clientInfo": map[string]interface{}{
							"type":     "object",
							"required": []string{"name", "version"},
							"properties": map[string]interface{}{
								"name": map[string]interface{}{
									"type":        "string",
									"description": "Client name",
									"example":     "MyClient",
								},
								"version": map[string]interface{}{
									"type":        "string",
									"description": "Client version",
									"example":     "1.0.0",
								},
							},
						},
					},
				},
				"MCPCallToolParams": map[string]interface{}{
					"type":     "object",
					"required": []string{"name", "arguments"},
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "tool name",
							"example":     "nmap",
						},
						"arguments": map[string]interface{}{
							"type":        "object",
							"description": "Tool parameters (key-value pairs), specific parameters depend on tool definition",
							"example": map[string]interface{}{
								"target": "192.168.1.1",
								"ports":  "80,443",
							},
						},
					},
				},
				"MCPResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"description": "Message ID (same as the id in the request)",
							"oneOf": []map[string]interface{}{
								{"type": "string"},
								{"type": "number"},
								{"type": "null"},
							},
						},
						"result": map[string]interface{}{
							"description": "Method execution result (JSON object), structure depends on the method called",
							"type":        "object",
						},
						"error": map[string]interface{}{
							"type":        "object",
							"description": "Error info (if execution failed)",
							"properties": map[string]interface{}{
								"code": map[string]interface{}{
									"type":        "integer",
									"description": "Error code",
									"example":     -32600,
								},
								"message": map[string]interface{}{
									"type":        "string",
									"description": "errormessage",
									"example":     "Invalid Request",
								},
								"data": map[string]interface{}{
									"description": "Error details (optional)",
								},
							},
						},
						"jsonrpc": map[string]interface{}{
							"type":        "string",
							"description": "JSON-RPC version",
							"example":     "2.0",
						},
					},
				},
			},
		},
		"security": []map[string]interface{}{
			{
				"bearerAuth": []string{},
			},
		},
		"paths": map[string]interface{}{
			"/api/auth/login": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Authentication"},
					"summary":     "User login",
					"description": "Login with password to obtain authentication token",
					"operationId": "login",
					"security":    []map[string]interface{}{},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/LoginRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "login successful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/LoginResponse",
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "incorrect password",
						},
					},
				},
			},
			"/api/auth/logout": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Authentication"},
					"summary":     "user logout",
					"description": "Log out of current session, invalidating the token",
					"operationId": "logout",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "logout successful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"message": map[string]interface{}{
												"type":    "string",
												"example": "Logged out successfully",
											},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/auth/change-password": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Authentication"},
					"summary":     "change password",
					"description": "Change login password; all sessions will be invalidated after change",
					"operationId": "changePassword",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/ChangePasswordRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Password changed successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"message": map[string]interface{}{
												"type":    "string",
												"example": "Password updated, please log in again with the new password",
											},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Request parameter error",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/auth/validate": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Authentication"},
					"summary":     "validateToken",
					"description": "Validate whether the current token is valid",
					"operationId": "validateToken",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Token valid",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"token": map[string]interface{}{
												"type":        "string",
												"description": "Token",
											},
											"expires_at": map[string]interface{}{
												"type":        "string",
												"format":      "date-time",
												"description": "expires at",
											},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Token invalid or expired",
						},
					},
				},
			},
			"/api/conversations": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Conversation Management"},
					"summary":     "create conversation",
					"description": "Create a new security test conversation.\n**Important notes**:\n- ✅ The created conversation is **immediately saved to the database**\n- ✅ The frontend will **auto-refresh** to show the new conversation\n- ✅ **Identical** to conversations created from the frontend\n**Two ways to create a conversation**:\n**Method 1 (recommended):** Send a message directly via `/api/eino-agent` **without** a `conversationId` parameter; the system automatically creates a new conversation and sends the message. This is the simplest approach.\n**Method 2:** First call this endpoint to create an empty conversation, then use the returned `conversationId` to call `/api/eino-agent`. Suitable for scenarios where you need to create a conversation before sending messages.\n**Example**:\n```json\n{\n  \"title\": \"Web application security test\"\n}\n```",
					"operationId": "createConversation",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/CreateConversationRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "conversationcreatesuccessful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Conversation",
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Request parameter error",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized, a valid token is required",
						},
						"500": map[string]interface{}{
							"description": "server internal error",
						},
					},
				},
				"get": map[string]interface{}{
					"tags":        []string{"Conversation Management"},
					"summary":     "list conversations",
					"description": "Get conversation list, supports pagination and search",
					"operationId": "listConversations",
					"parameters": []map[string]interface{}{
						{
							"name":        "limit",
							"in":          "query",
							"required":    false,
							"description": "Return count limit",
							"schema": map[string]interface{}{
								"type":    "integer",
								"default": 50,
								"minimum": 1,
								"maximum": 100,
							},
						},
						{
							"name":        "offset",
							"in":          "query",
							"required":    false,
							"description": "Offset",
							"schema": map[string]interface{}{
								"type":    "integer",
								"default": 0,
								"minimum": 0,
							},
						},
						{
							"name":        "search",
							"in":          "query",
							"required":    false,
							"description": "search keyword",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
						{
							"name":        "project_id",
							"in":          "query",
							"required":    false,
							"description": "Filter by project; pass __none__ to show only conversations not bound to a project",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
						{
							"name":        "sort_by",
							"in":          "query",
							"required":    false,
							"description": "Sort field: updated_at (default) or created_at",
							"schema": map[string]interface{}{
								"type": "string",
								"enum": []string{"updated_at", "created_at"},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "array",
										"items": map[string]interface{}{
											"$ref": "#/components/schemas/Conversation",
										},
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized, a valid token is required",
						},
					},
				},
			},
			"/api/conversations/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Conversation Management"},
					"summary":     "viewconversationdetails",
					"description": "Get detailed information for the specified conversation, including conversation info and message list",
					"operationId": "getConversation",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "conversation ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/ConversationDetail",
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "conversation not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized, a valid token is required",
						},
					},
				},
				"put": map[string]interface{}{
					"tags":        []string{"Conversation Management"},
					"summary":     "updateconversation",
					"description": "updateconversation title",
					"operationId": "updateConversation",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "conversation ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/UpdateConversationRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "update successful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Conversation",
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Request parameter error",
						},
						"404": map[string]interface{}{
							"description": "conversation not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized, a valid token is required",
						},
					},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"Conversation Management"},
					"summary":     "delete conversation",
					"description": "Delete the specified conversation and its session data (messages, attack chains, etc.). **Vulnerability records are retained**, only the association with the session is removed. **This operation cannot be undone**.",
					"operationId": "deleteConversation",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "conversation ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "deletion successful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"message": map[string]interface{}{
												"type":        "string",
												"description": "successfulmessage",
												"example":     "deletion successful",
											},
										},
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "conversation not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized, a valid token is required",
						},
						"500": map[string]interface{}{
							"description": "server internal error",
						},
					},
				},
			},
			"/api/conversations/{id}/project": map[string]interface{}{
				"put": map[string]interface{}{
					"tags":        []string{"Conversation Management"},
					"summary":     "set conversation project",
					"description": "Bind or unbind a conversation from a project, used for the shared facts blackboard",
					"operationId": "setConversationProject",
					"parameters": []map[string]interface{}{
						{
							"name": "id", "in": "path", "required": true,
							"description": "conversation ID",
							"schema":      map[string]interface{}{"type": "string"},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/SetConversationProjectRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "settingssuccessful"},
						"400": map[string]interface{}{"description": "Project not found or parameter error"},
						"404": map[string]interface{}{"description": "conversation not found"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/conversations/{id}/results": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Conversation Management"},
					"summary":     "get conversation result",
					"description": "get execution result for specified conversation, including messages, vulnerability info and execution results",
					"operationId": "getConversationResults",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "conversation ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/ConversationResults",
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "Conversation not found or results do not exist",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized, a valid token is required",
						},
					},
				},
			},
			"/api/eino-agent": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Conversation Interaction"},
					"summary":     "send message and get AI reply (Eino ADK single agent, non-streaming)",
					"description": "Send message to AI and get reply (non-streaming). Executed by **CloudWeGo Eino** `adk.NewChatModelAgent` + `adk.NewRunner.Run` as a single-agent MCP tool chain. **Independent of** `multi_agent.enabled`; `multi_agent.eino_skills` / `eino_middleware` etc. take effect when consistent with multi-agent primary agent. Supports `webshellConnectionId`, roles, and attachments.",
					"operationId": "sendMessageEinoSingleAgent",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"message":              map[string]interface{}{"type": "string"},
										"conversationId":       map[string]interface{}{"type": "string"},
										"role":                 map[string]interface{}{"type": "string"},
										"webshellConnectionId": map[string]interface{}{"type": "string"},
										"finalization":         finalizationRequestSchema,
									},
									"required": []string{"message"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Successful. Only finalized=true indicates a successful Final reply; when finalized=false, the response is an incomplete/blocked explanation.",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/AgentChatResponse"},
								},
							},
						},
						"400": map[string]interface{}{"description": "Parameter error"},
						"401": map[string]interface{}{"description": "Unauthorized"},
						"500": map[string]interface{}{"description": "Execution failed"},
					},
				},
			},
			"/api/eino-agent/stream": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Conversation Interaction"},
					"summary":     "send message and get AI reply (Eino ADK single agent, SSE)",
					"description": "Send message to AI and get streaming reply (SSE). Executed by Eino **single-agent** ADK; event types are consistent with multi-agent streaming (including `tool_call` / `response_delta` / `thinking`, etc.). `response_start` / `response_delta` are candidate/process output only; only `type: response` with `data.finalized=true` indicates a successful Final reply. When completed execution evidence is missing, `finalization_auto_continue` may be sent first, indicating server is performing a no-inject re-run based on existing trace. When `data.finalized=false`, message is an incomplete/blocked explanation. **Independent of** `multi_agent.enabled`.",
					"operationId": "sendMessageEinoSingleAgentStream",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"message":              map[string]interface{}{"type": "string"},
										"conversationId":       map[string]interface{}{"type": "string"},
										"role":                 map[string]interface{}{"type": "string"},
										"webshellConnectionId": map[string]interface{}{"type": "string"},
										"finalization":         finalizationRequestSchema,
									},
									"required": []string{"message"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "text/event-stream (SSE)",
							"content": map[string]interface{}{
								"text/event-stream": map[string]interface{}{
									"schema": map[string]interface{}{
										"type":        "string",
										"description": "SSE stream. Terminal response event data includes finalized, finalizable, status, completionReason, evidenceVerified, evidenceRefs, pendingExecutionIds, missingChecks; process events may include finalization_auto_continue.",
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/multi-agent": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Conversation Interaction"},
					"summary":     "send message and get AI reply (Eino multi-agent, non-streaming)",
					"description": "Same request body as `POST /api/eino-agent`, but executed by **CloudWeGo Eino** multi-agent. Orchestration is specified by the `orchestration` field in the request body (`deep` | `plan_execute` | `supervisor`), defaults to `deep`. **Prerequisite**: `multi_agent.enabled: true`; returns 404 JSON when not enabled. Supports `webshellConnectionId`.",
					"operationId": "sendMessageMultiAgent",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"message": map[string]interface{}{
											"type":        "string",
											"description": "Message to send (required)",
										},
										"conversationId": map[string]interface{}{
											"type":        "string",
											"description": "Conversation ID (optional; creates a new one if not provided)",
										},
										"role": map[string]interface{}{
											"type":        "string",
											"description": "Role name (optional)",
										},
										"webshellConnectionId": map[string]interface{}{
											"type":        "string",
											"description": "WebShell connection ID (optional, consistent with Eino single/multi-agent streaming behavior)",
										},
										"finalization": finalizationRequestSchema,
										"orchestration": map[string]interface{}{
											"type":        "string",
											"description": "Eino built-in orchestration: deep | plan_execute | supervisor; defaults to deep",
											"enum":        []string{"deep", "plan_execute", "supervisor"},
										},
									},
									"required": []string{"message"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Successful. Only finalized=true indicates a successful Final reply; when finalized=false, the response is an incomplete/blocked explanation.",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/AgentChatResponse"},
								},
							},
						},
						"400": map[string]interface{}{"description": "Parameter error"},
						"401": map[string]interface{}{"description": "Unauthorized"},
						"404": map[string]interface{}{"description": "Multi-agent not enabled or conversation not found"},
						"500": map[string]interface{}{"description": "Execution failed"},
					},
				},
			},
			"/api/multi-agent/stream": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Conversation Interaction"},
					"summary":     "send message and get AI reply (Eino multi-agent, SSE)",
					"description": "Similar to `POST /api/eino-agent/stream`; executed by Eino multi-agent. `orchestration` specifies deep / plan_execute / supervisor, defaults to deep. `response_start` / `response_delta` are only candidate/process outputs; only `type: response` with `data.finalized=true` indicates a successful Final reply. When completed execution evidence is missing, `finalization_auto_continue` may be sent first, indicating the server is performing a no-inject re-run based on existing trace. **Prerequisite**: `multi_agent.enabled: true`; returns `type: error` followed by `done` in SSE when not enabled. Supports `webshellConnectionId`.",
					"operationId": "sendMessageMultiAgentStream",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"message":              map[string]interface{}{"type": "string"},
										"conversationId":       map[string]interface{}{"type": "string"},
										"role":                 map[string]interface{}{"type": "string"},
										"webshellConnectionId": map[string]interface{}{"type": "string"},
										"finalization":         finalizationRequestSchema,
										"orchestration": map[string]interface{}{
											"type":        "string",
											"description": "deep | plan_execute | supervisor; defaults to deep",
											"enum":        []string{"deep", "plan_execute", "supervisor"},
										},
									},
									"required": []string{"message"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "text/event-stream (SSE)",
							"content": map[string]interface{}{
								"text/event-stream": map[string]interface{}{
									"schema": map[string]interface{}{
										"type":        "string",
										"description": "SSE stream. Terminal response event data includes finalized, finalizable, status, completionReason, evidenceVerified, evidenceRefs, pendingExecutionIds, missingChecks; process events may include finalization_auto_continue.",
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/agent-loop/cancel": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Conversation Interaction"},
					"summary":     "cancelledtask",
					"description": "Cancel the currently executing Agent Loop task",
					"operationId": "cancelAgentLoop",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/CancelAgentLoopRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Cancellation request submitted",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"status": map[string]interface{}{
												"type":    "string",
												"example": "cancelling",
											},
											"conversationId": map[string]interface{}{
												"type":        "string",
												"description": "conversation ID",
											},
											"message": map[string]interface{}{
												"type":    "string",
												"example": "Cancellation request submitted; task will stop after the current step completes.",
											},
										},
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "Running task not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/agent-loop/tasks": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Conversation Interaction"},
					"summary":     "list running tasks",
					"description": "get all running Agent Loop tasks",
					"operationId": "listAgentTasks",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"tasks": map[string]interface{}{
												"type":        "array",
												"description": "tasklist",
												"items": map[string]interface{}{
													"$ref": "#/components/schemas/AgentTask",
												},
											},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/agent-loop/tasks/completed": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Conversation Interaction"},
					"summary":     "list completed tasks",
					"description": "get recent completed Agent Loop task history",
					"operationId": "listCompletedTasks",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"tasks": map[string]interface{}{
												"type":        "array",
												"description": "completedtasklist",
												"items": map[string]interface{}{
													"$ref": "#/components/schemas/AgentTask",
												},
											},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/batch-tasks": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Batch Tasks"},
					"summary":     "create batch task queue",
					"description": "Create a batch task queue containing multiple tasks",
					"operationId": "createBatchQueue",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/BatchTaskRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "createsuccessful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"queueId": map[string]interface{}{
												"type":        "string",
												"description": "queueID",
											},
											"queue": map[string]interface{}{
												"$ref": "#/components/schemas/BatchQueue",
											},
											"started": map[string]interface{}{
												"type":        "boolean",
												"description": "Whether execution has started immediately",
											},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Request parameter error",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"get": map[string]interface{}{
					"tags":        []string{"Batch Tasks"},
					"summary":     "list batch task queues",
					"description": "get all batch task queues",
					"operationId": "listBatchQueues",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"queues": map[string]interface{}{
												"type":        "array",
												"description": "queuelist",
												"items": map[string]interface{}{
													"$ref": "#/components/schemas/BatchQueue",
												},
											},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/batch-tasks/{queueId}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Batch Tasks"},
					"summary":     "get batch task queue",
					"description": "get detailed info for specified batch task queue",
					"operationId": "getBatchQueue",
					"parameters": []map[string]interface{}{
						{
							"name":        "queueId",
							"in":          "path",
							"required":    true,
							"description": "queueID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/BatchQueue",
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "Queue not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"Batch Tasks"},
					"summary":     "delete batch task queue",
					"description": "Delete the specified batch task queue",
					"operationId": "deleteBatchQueue",
					"parameters": []map[string]interface{}{
						{
							"name":        "queueId",
							"in":          "path",
							"required":    true,
							"description": "queueID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "deletion successful",
						},
						"404": map[string]interface{}{
							"description": "Queue not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/batch-tasks/{queueId}/start": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Batch Tasks"},
					"summary":     "start batch task queue",
					"description": "Start executing tasks in the batch task queue",
					"operationId": "startBatchQueue",
					"parameters": []map[string]interface{}{
						{
							"name":        "queueId",
							"in":          "path",
							"required":    true,
							"description": "queueID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "startsuccessful",
						},
						"404": map[string]interface{}{
							"description": "Queue not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/batch-tasks/{queueId}/pause": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Batch Tasks"},
					"summary":     "pause batch task queue",
					"description": "Pause the currently executing batch task queue",
					"operationId": "pauseBatchQueue",
					"parameters": []map[string]interface{}{
						{
							"name":        "queueId",
							"in":          "path",
							"required":    true,
							"description": "queueID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "pausesuccessful",
						},
						"404": map[string]interface{}{
							"description": "Queue not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/batch-tasks/{queueId}/tasks": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Batch Tasks"},
					"summary":     "add task to queue",
					"description": "Add a new task to the batch task queue. Tasks are appended to the end and executed in queue order. Each task creates an independent conversation with full status tracking.\n**Task format**:\nTask content is a string describing the security test task to execute. For example:\n- \"Scan http://example.com for SQL injection vulnerabilities\"\n- \"Port scan 192.168.1.1\"\n- \"Detect XSS vulnerabilities on https://target.com\"\n**Usage example**:\n```json\n{\n  \"task\": \"Scan http://example.com for SQL injection vulnerabilities\"\n}\n```",
					"operationId": "addBatchTask",
					"parameters": []map[string]interface{}{
						{
							"name":        "queueId",
							"in":          "path",
							"required":    true,
							"description": "queueID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"task"},
									"properties": map[string]interface{}{
										"task": map[string]interface{}{
											"type":        "string",
											"description": "Task content, describe the security test task to execute (required)",
											"example":     "Scan http://example.com for SQL injection vulnerabilities",
										},
									},
								},
								"examples": map[string]interface{}{
									"sqlInjection": map[string]interface{}{
										"summary":     "SQL Injectionscan",
										"description": "Scan target website for SQL Injection vulnerabilities",
										"value": map[string]interface{}{
											"task": "Scan http://example.com for SQL injection vulnerabilities",
										},
									},
									"portScan": map[string]interface{}{
										"summary":     "portscan",
										"description": "Port scan the target IP",
										"value": map[string]interface{}{
											"task": "Port scan 192.168.1.1",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Added successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"taskId": map[string]interface{}{
												"type":        "string",
												"description": "Newly added task ID",
											},
											"message": map[string]interface{}{
												"type":        "string",
												"description": "successfulmessage",
												"example":     "Task has been added to queue",
											},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Request parameter error (e.g. task is empty)",
						},
						"404": map[string]interface{}{
							"description": "Queue not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/batch-tasks/{queueId}/tasks/{taskId}": map[string]interface{}{
				"put": map[string]interface{}{
					"tags":        []string{"Batch Tasks"},
					"summary":     "update batch task",
					"description": "Update the specified task in the batch task queue",
					"operationId": "updateBatchTask",
					"parameters": []map[string]interface{}{
						{
							"name":        "queueId",
							"in":          "path",
							"required":    true,
							"description": "queueID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
						{
							"name":        "taskId",
							"in":          "path",
							"required":    true,
							"description": "task ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"task": map[string]interface{}{
											"type":        "string",
											"description": "Task content",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "update successful",
						},
						"404": map[string]interface{}{
							"description": "task not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"Batch Tasks"},
					"summary":     "delete batch task",
					"description": "Delete the specified task from the batch task queue",
					"operationId": "deleteBatchTask",
					"parameters": []map[string]interface{}{
						{
							"name":        "queueId",
							"in":          "path",
							"required":    true,
							"description": "queueID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
						{
							"name":        "taskId",
							"in":          "path",
							"required":    true,
							"description": "task ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "deletion successful",
						},
						"404": map[string]interface{}{
							"description": "task not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/assets/import": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Asset Management"},
					"summary":     "bulk import assets",
					"description": "Add new assets or deduplicate-update by \"target + port + protocol\". Accepts JSON, not XLSX/CSV directly; max 100000 records per request, requires asset:write permission.",
					"operationId": "importAssets",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{"$ref": "#/components/schemas/AssetImportRequest"},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Import completed",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/AssetImportResult"},
								},
							},
						},
						"400": map[string]interface{}{"description": "Asset count or field validation failed"},
						"401": map[string]interface{}{"description": "Unauthorized"},
						"403": map[string]interface{}{"description": "Missing asset:write permission or access denied for specified project"},
						"500": map[string]interface{}{"description": "Import transaction failed"},
					},
				},
			},
			"/api/projects": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Project Management"},
					"summary":     "list projects",
					"operationId": "listProjects",
					"parameters": []map[string]interface{}{
						{"name": "status", "in": "query", "schema": map[string]interface{}{"type": "string", "enum": []string{"active", "archived"}}},
						{"name": "limit", "in": "query", "schema": map[string]interface{}{"type": "integer", "default": 200}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "projectlist"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
				"post": map[string]interface{}{
					"tags":        []string{"Project Management"},
					"summary":     "create project",
					"operationId": "createProject",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"name":        map[string]interface{}{"type": "string"},
										"description": map[string]interface{}{"type": "string"},
										"scope_json":  map[string]interface{}{"type": "string"},
									},
									"required": []string{"name"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "createsuccessful"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/projects/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags": []string{"Project Management"}, "summary": "Get project", "operationId": "getProject",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{"200": map[string]interface{}{"description": "projectdetails"}},
				},
				"put": map[string]interface{}{
					"tags": []string{"Project Management"}, "summary": "Update project", "operationId": "updateProject",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{"200": map[string]interface{}{"description": "update successful"}},
				},
				"delete": map[string]interface{}{
					"tags": []string{"Project Management"}, "summary": "Delete project", "operationId": "deleteProject",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{"200": map[string]interface{}{"description": "deletion successful"}},
				},
			},
			"/api/projects/{id}/facts": map[string]interface{}{
				"get": map[string]interface{}{
					"tags": []string{"Project Management"}, "summary": "List or get facts by key", "operationId": "listProjectFacts",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
						{"name": "fact_key", "in": "query", "schema": map[string]interface{}{"type": "string"}},
						{"name": "include_links", "in": "query", "schema": map[string]interface{}{"type": "boolean"}},
						{"name": "include_link_counts", "in": "query", "schema": map[string]interface{}{"type": "boolean"}},
					},
					"responses": map[string]interface{}{"200": map[string]interface{}{"description": "fact list or single item (may include link_counts / outgoing_links)"}},
				},
				"post": map[string]interface{}{
					"tags": []string{"Project Management"}, "summary": "Create/update fact", "operationId": "upsertProjectFactREST",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"fact_key": map[string]interface{}{"type": "string"},
										"summary":  map[string]interface{}{"type": "string"},
										"links": map[string]interface{}{
											"type": "array",
											"items": map[string]interface{}{
												"type": "object",
												"properties": map[string]interface{}{
													"to":   map[string]interface{}{"type": "string"},
													"type": map[string]interface{}{"type": "string"},
												},
											},
										},
										"links_text": map[string]interface{}{"type": "string", "description": "type: fact_key, one per line"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{"200": map[string]interface{}{"description": "successful"}},
				},
			},
			"/api/projects/{id}/fact-graph": map[string]interface{}{
				"get": map[string]interface{}{
					"tags": []string{"Project Management"}, "summary": "Get project facts attack path graph", "operationId": "getProjectFactGraph",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
						{"name": "view", "in": "query", "schema": map[string]interface{}{"type": "string", "enum": []string{"path", "full"}, "default": "path"}},
						{"name": "exclude_deprecated", "in": "query", "schema": map[string]interface{}{"type": "boolean", "default": true}},
					},
					"responses": map[string]interface{}{"200": map[string]interface{}{"description": "nodes + edges"}},
				},
			},
			"/api/projects/{id}/fact-edges": map[string]interface{}{
				"get": map[string]interface{}{
					"tags": []string{"Project Management"}, "summary": "List all project fact edges", "operationId": "listProjectFactEdges",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{"200": map[string]interface{}{"description": "edge list"}},
				},
				"post": map[string]interface{}{
					"tags": []string{"Project Management"}, "summary": "Add fact edge", "operationId": "createProjectFactEdge",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"source_fact_key", "target_fact_key", "edge_type"},
									"properties": map[string]interface{}{
										"source_fact_key": map[string]interface{}{"type": "string"},
										"target_fact_key": map[string]interface{}{"type": "string"},
										"edge_type":       map[string]interface{}{"type": "string"},
										"confidence":      map[string]interface{}{"type": "string"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{"200": map[string]interface{}{"description": "edge created"}},
				},
			},
			"/api/projects/{id}/fact-edges/{edgeId}": map[string]interface{}{
				"delete": map[string]interface{}{
					"tags": []string{"Project Management"}, "summary": "Delete fact edge", "operationId": "deleteProjectFactEdge",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
						{"name": "edgeId", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{"200": map[string]interface{}{"description": "deletion successful"}},
				},
			},
			"/api/projects/{id}/promote-attack-chain/{conversationId}": map[string]interface{}{
				"post": map[string]interface{}{
					"tags": []string{"Project Management"}, "summary": "Promote conversation attack chain to project facts graph", "operationId": "promoteAttackChainToProject",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
						{"name": "conversationId", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{"200": map[string]interface{}{"description": "sedimentation result (facts/edges/graph)"}},
				},
			},
			"/api/vulnerabilities": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Vulnerability management"},
					"summary":     "list vulnerabilities",
					"description": "Get vulnerability list, supports pagination and filtering",
					"operationId": "listVulnerabilities",
					"parameters": []map[string]interface{}{
						{
							"name":        "limit",
							"in":          "query",
							"required":    false,
							"description": "page size",
							"schema": map[string]interface{}{
								"type":    "integer",
								"default": 20,
								"minimum": 1,
								"maximum": 100,
							},
						},
						{
							"name":        "offset",
							"in":          "query",
							"required":    false,
							"description": "Offset",
							"schema": map[string]interface{}{
								"type":    "integer",
								"default": 0,
								"minimum": 0,
							},
						},
						{
							"name":        "page",
							"in":          "query",
							"required":    false,
							"description": "Page number (use either this or offset)",
							"schema": map[string]interface{}{
								"type":    "integer",
								"minimum": 1,
							},
						},
						{
							"name":        "id",
							"in":          "query",
							"required":    false,
							"description": "Vulnerability ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
						{
							"name":        "conversation_id",
							"in":          "query",
							"required":    false,
							"description": "conversation ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
						{
							"name":        "project_id",
							"in":          "query",
							"required":    false,
							"description": "project ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
						{
							"name":        "severity",
							"in":          "query",
							"required":    false,
							"description": "Severity level",
							"schema": map[string]interface{}{
								"type": "string",
								"enum": []string{"critical", "high", "medium", "low", "info"},
							},
						},
						{
							"name":        "status",
							"in":          "query",
							"required":    false,
							"description": "status",
							"schema": map[string]interface{}{
								"type": "string",
								"enum": []string{"open", "closed", "fixed"},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/ListVulnerabilitiesResponse",
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"post": map[string]interface{}{
					"tags":        []string{"Vulnerability management"},
					"summary":     "create vulnerability",
					"description": "Create a new vulnerability record",
					"operationId": "createVulnerability",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/CreateVulnerabilityRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "createsuccessful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Vulnerability",
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Request parameter error",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/vulnerabilities/stats": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Vulnerability management"},
					"summary":     "get vulnerability statistics",
					"description": "get vulnerability statistics info",
					"operationId": "getVulnerabilityStats",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/VulnerabilityStats",
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/vulnerabilities/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Vulnerability management"},
					"summary":     "get vulnerability",
					"description": "get detailed info for specified vulnerability",
					"operationId": "getVulnerability",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "Vulnerability ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Vulnerability",
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "vulnerability not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"put": map[string]interface{}{
					"tags":        []string{"Vulnerability management"},
					"summary":     "update vulnerability",
					"description": "update vulnerabilityinfo",
					"operationId": "updateVulnerability",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "Vulnerability ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/UpdateVulnerabilityRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "update successful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Vulnerability",
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Request parameter error",
						},
						"404": map[string]interface{}{
							"description": "vulnerability not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"Vulnerability management"},
					"summary":     "delete vulnerability",
					"description": "Delete the specified vulnerability",
					"operationId": "deleteVulnerability",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "Vulnerability ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "deletion successful",
						},
						"404": map[string]interface{}{
							"description": "vulnerability not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/roles": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Role Management"},
					"summary":     "list roles",
					"description": "get all security testing roles",
					"operationId": "getRoles",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"roles": map[string]interface{}{
												"type":        "array",
												"description": "Role list",
												"items": map[string]interface{}{
													"$ref": "#/components/schemas/RoleConfig",
												},
											},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"post": map[string]interface{}{
					"tags":        []string{"Role Management"},
					"summary":     "create role",
					"description": "Create a new security test role",
					"operationId": "createRole",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/RoleConfig",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "createsuccessful",
						},
						"400": map[string]interface{}{
							"description": "Request parameter error",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/roles/{name}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Role Management"},
					"summary":     "get role",
					"description": "get detailed info for specified role",
					"operationId": "getRole",
					"parameters": []map[string]interface{}{
						{
							"name":        "name",
							"in":          "path",
							"required":    true,
							"description": "role name",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"role": map[string]interface{}{
												"$ref": "#/components/schemas/RoleConfig",
											},
										},
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "role not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"put": map[string]interface{}{
					"tags":        []string{"Role Management"},
					"summary":     "update role",
					"description": "Update config for the specified role",
					"operationId": "updateRole",
					"parameters": []map[string]interface{}{
						{
							"name":        "name",
							"in":          "path",
							"required":    true,
							"description": "role name",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/RoleConfig",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "update successful",
						},
						"400": map[string]interface{}{
							"description": "Request parameter error",
						},
						"404": map[string]interface{}{
							"description": "role not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"Role Management"},
					"summary":     "delete role",
					"description": "Delete the specified role",
					"operationId": "deleteRole",
					"parameters": []map[string]interface{}{
						{
							"name":        "name",
							"in":          "path",
							"required":    true,
							"description": "role name",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "deletion successful",
						},
						"404": map[string]interface{}{
							"description": "role not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/skills": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Skills Management"},
					"summary":     "list skills",
					"description": "get all skills list, supports pagination and search",
					"operationId": "getSkills",
					"parameters": []map[string]interface{}{
						{
							"name":        "limit",
							"in":          "query",
							"required":    false,
							"description": "page size",
							"schema": map[string]interface{}{
								"type":    "integer",
								"default": 20,
							},
						},
						{
							"name":        "offset",
							"in":          "query",
							"required":    false,
							"description": "Offset",
							"schema": map[string]interface{}{
								"type":    "integer",
								"default": 0,
							},
						},
						{
							"name":        "search",
							"in":          "query",
							"required":    false,
							"description": "search keyword",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"skills": map[string]interface{}{
												"type":        "array",
												"description": "Skillslist",
												"items": map[string]interface{}{
													"$ref": "#/components/schemas/Skill",
												},
											},
											"total": map[string]interface{}{
												"type":        "integer",
												"description": "total",
											},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"post": map[string]interface{}{
					"tags":        []string{"Skills Management"},
					"summary":     "createSkill",
					"description": "Create a new Skill",
					"operationId": "createSkill",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/CreateSkillRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "createsuccessful",
						},
						"400": map[string]interface{}{
							"description": "Request parameter error",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/skills/stats": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Skills Management"},
					"summary":     "get skill statistics",
					"description": "get skill call statistics info",
					"operationId": "getSkillStats",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type":        "object",
										"description": "Statistics info",
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"Skills Management"},
					"summary":     "clear skill statistics",
					"description": "Clear call statistics for all Skills",
					"operationId": "clearSkillStats",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Cleared successfully",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/skills/{name}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Skills Management"},
					"summary":     "get skill",
					"description": "get detailed info for specified skill",
					"operationId": "getSkill",
					"parameters": []map[string]interface{}{
						{
							"name":        "name",
							"in":          "path",
							"required":    true,
							"description": "Skillname",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Skill",
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "Skill not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"put": map[string]interface{}{
					"tags":        []string{"Skills Management"},
					"summary":     "updateSkill",
					"description": "Update info for the specified Skill",
					"operationId": "updateSkill",
					"parameters": []map[string]interface{}{
						{
							"name":        "name",
							"in":          "path",
							"required":    true,
							"description": "Skillname",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/UpdateSkillRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "update successful",
						},
						"400": map[string]interface{}{
							"description": "Request parameter error",
						},
						"404": map[string]interface{}{
							"description": "Skill not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"Skills Management"},
					"summary":     "deleteSkill",
					"description": "Delete the specified Skill",
					"operationId": "deleteSkill",
					"parameters": []map[string]interface{}{
						{
							"name":        "name",
							"in":          "path",
							"required":    true,
							"description": "Skillname",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "deletion successful",
						},
						"404": map[string]interface{}{
							"description": "Skill not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/skills/{name}/bound-roles": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Skills Management"},
					"summary":     "get bound roles",
					"description": "get all roles using specified skill",
					"operationId": "getSkillBoundRoles",
					"parameters": []map[string]interface{}{
						{
							"name":        "name",
							"in":          "path",
							"required":    true,
							"description": "Skillname",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"roles": map[string]interface{}{
												"type":        "array",
												"description": "Role list",
												"items": map[string]interface{}{
													"type": "string",
												},
											},
										},
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "Skill not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/skills/{name}/stats": map[string]interface{}{
				"delete": map[string]interface{}{
					"tags":        []string{"Skills Management"},
					"summary":     "clear skill statistics",
					"description": "Clear call statistics for the specified Skill",
					"operationId": "clearSkillStatsByName",
					"parameters": []map[string]interface{}{
						{
							"name":        "name",
							"in":          "path",
							"required":    true,
							"description": "Skillname",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Cleared successfully",
						},
						"404": map[string]interface{}{
							"description": "Skill not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/monitor": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Monitoring"},
					"summary":     "get monitoring info",
					"description": "get tool execution monitoring info, supports pagination and filtering",
					"operationId": "monitor",
					"parameters": []map[string]interface{}{
						{
							"name":        "page",
							"in":          "query",
							"required":    false,
							"description": "page number",
							"schema": map[string]interface{}{
								"type":    "integer",
								"default": 1,
								"minimum": 1,
							},
						},
						{
							"name":        "page_size",
							"in":          "query",
							"required":    false,
							"description": "page size",
							"schema": map[string]interface{}{
								"type":    "integer",
								"default": 20,
								"minimum": 1,
								"maximum": 100,
							},
						},
						{
							"name":        "status",
							"in":          "query",
							"required":    false,
							"description": "Status filter",
							"schema": map[string]interface{}{
								"type": "string",
								"enum": []string{"queued", "running", "completed", "failed", "cancelled", "hard_timeout", "orphaned"},
							},
						},
						{
							"name":        "tool",
							"in":          "query",
							"required":    false,
							"description": "Tool name filter (supports partial matching)",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/MonitorResponse",
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/monitor/execution/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Monitoring"},
					"summary":     "get execution record",
					"description": "get detailed info for specified execution record",
					"operationId": "getExecution",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "Execution ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/ToolExecution",
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "Execution record not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"Monitoring"},
					"summary":     "delete execution record",
					"description": "Delete the specified execution record",
					"operationId": "deleteExecution",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "Execution ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "deletion successful",
						},
						"404": map[string]interface{}{
							"description": "Execution record not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/monitor/execution/{id}/cancel": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Monitoring"},
					"summary":     "cancel in-progress tool execution",
					"description": "Send a context cancellation signal to the currently executing MCP tool call in this process; the parent conversation/multi-step task can continue. Returns 404 if execution has ended or is not running in this process.",
					"operationId": "cancelExecution",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "Execution ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"requestBody": map[string]interface{}{
						"required": false,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"note": map[string]interface{}{
											"type":        "string",
											"description": "Optional. When non-null, merged with tool output and sent to the model with a 'user termination note' title block to distinguish from raw CLI output",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Termination signal sent",
						},
						"400": map[string]interface{}{
							"description": "Request body is not valid JSON",
						},
						"404": map[string]interface{}{
							"description": "In-progress tool execution not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/monitor/executions": map[string]interface{}{
				"delete": map[string]interface{}{
					"tags":        []string{"Monitoring"},
					"summary":     "batch delete execution records",
					"description": "Batch delete execution records",
					"operationId": "deleteExecutions",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "deletion successful",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/monitor/stats": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Monitoring"},
					"summary":     "get statistics info",
					"description": "get tool execution statistics info",
					"operationId": "getStats",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type":        "object",
										"description": "Statistics info",
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/config": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Config Management"},
					"summary":     "get configuration",
					"description": "get system configuration info",
					"operationId": "getConfig",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/ConfigResponse",
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"put": map[string]interface{}{
					"tags":        []string{"Config Management"},
					"summary":     "update configuration",
					"description": "updatesystemconfig",
					"operationId": "updateConfig",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/UpdateConfigRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "update successful",
						},
						"400": map[string]interface{}{
							"description": "Request parameter error",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/storage/meta": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Storage Cleanup"},
					"summary":     "get storage cleanup policy",
					"description": "Returns auto-cleanup switch, interval, grace window and enable status/retention days for all cleanup categories",
					"operationId": "getStorageMeta",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
						"403": map[string]interface{}{
							"description": "Missing storage:read permission",
						},
					},
				},
			},
			"/api/storage/status": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Storage Cleanup"},
					"summary":     "get runtime storage usage",
					"description": "Returns filesystem capacity (including inode) and usage/reclaimable amount per category; result is short TTL cached",
					"operationId": "getStorageStatus",
					"parameters": []interface{}{
						map[string]interface{}{
							"name":        "refresh",
							"in":          "query",
							"description": "When true, forces re-traversal of the directory, bypassing cache",
							"required":    false,
							"schema":      map[string]interface{}{"type": "boolean"},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
						"403": map[string]interface{}{
							"description": "Missing storage:read permission",
						},
					},
				},
			},
			"/api/storage/cleanup": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Storage Cleanup"},
					"summary":     "preview or execute runtime storage cleanup",
					"description": "default dry_run=true counts without deleting; actual deletion requires dry_run=false and confirm=true simultaneously. Only processes enabled categories, only one cleanup run allowed at a time",
					"operationId": "runStorageCleanup",
					"requestBody": map[string]interface{}{
						"required": false,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"dry_run": map[string]interface{}{
											"type":        "boolean",
											"description": "Treated as true if omitted",
										},
										"confirm": map[string]interface{}{
											"type":        "boolean",
											"description": "Must be true when dry_run=false",
										},
										"categories": map[string]interface{}{
											"type":        "array",
											"description": "Restrict to category; omit to include all enabled categories",
											"items":       map[string]interface{}{"type": "string"},
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Execution completed (or preview completed)",
						},
						"400": map[string]interface{}{
							"description": "Missing confirm, or category key not registered",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
						"403": map[string]interface{}{
							"description": "Missing storage:write permission",
						},
						"409": map[string]interface{}{
							"description": "A cleanup round is already running",
						},
					},
				},
			},
			"/api/config/tools": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Config Management"},
					"summary":     "get tool configuration",
					"description": "get configuration info for all tools",
					"operationId": "getTools",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type":        "array",
										"description": "toolconfiglist",
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/config/apply": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Config Management"},
					"summary":     "apply configuration",
					"description": "Apply config changes",
					"operationId": "applyConfig",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Applied successfully",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/external-mcp": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"External MCP Management"},
					"summary":     "list external MCPs",
					"description": "get all external MCP configurations and statuses",
					"operationId": "getExternalMCPs",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"servers": map[string]interface{}{
												"type":        "object",
												"description": "MCPserver configuration",
												"additionalProperties": map[string]interface{}{
													"$ref": "#/components/schemas/ExternalMCPResponse",
												},
											},
											"stats": map[string]interface{}{
												"type":        "object",
												"description": "Statistics info",
											},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/external-mcp/stats": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"External MCP Management"},
					"summary":     "get external MCP statistics",
					"description": "get external MCP statistics info",
					"operationId": "getExternalMCPStats",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type":        "object",
										"description": "Statistics info",
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/external-mcp/{name}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"External MCP Management"},
					"summary":     "get external MCP",
					"description": "get configuration and status for specified external MCP",
					"operationId": "getExternalMCP",
					"parameters": []map[string]interface{}{
						{
							"name":        "name",
							"in":          "path",
							"required":    true,
							"description": "MCPname",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/ExternalMCPResponse",
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "MCP not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"put": map[string]interface{}{
					"tags":        []string{"External MCP Management"},
					"summary":     "add or update external MCP",
					"description": "Add a new external MCP configuration or update existing config.\n**Transport modes**:\nSupports two transport modes:\n**1. stdio (standard input/output)**:\n```json\n{\n  \"config\": {\n    \"enabled\": true,\n    \"command\": \"node\",\n    \"args\": [\"/path/to/mcp-server.js\"],\n    \"env\": {}\n  }\n}\n```\n**2. sse (Server-Sent Events)**:\n```json\n{\n  \"config\": {\n    \"enabled\": true,\n    \"transport\": \"sse\",\n    \"url\": \"http://127.0.0.1:8082/sse\",\n    \"timeout\": 30\n  }\n}\n```\n**Config parameter description**:\n- `enabled`: enabled (boolean, required)\n- `command`: command (required for stdio mode, e.g. \"node\", \"python\")\n- `args`: command arguments array (required for stdio mode)\n- `env`: environment variables (object, optional)\n- `transport`: transport mode (\"stdio\" or \"sse\", required for sse mode)\n- `url`: SSE endpoint URL (required for sse mode)\n- `timeout`: timeout in seconds (optional, default 30)\n- `description`: description (optional)",
					"operationId": "addOrUpdateExternalMCP",
					"parameters": []map[string]interface{}{
						{
							"name":        "name",
							"in":          "path",
							"required":    true,
							"description": "MCP name (unique identifier)",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/AddOrUpdateExternalMCPRequest",
								},
								"examples": map[string]interface{}{
									"stdio": map[string]interface{}{
										"summary":     "stdiopatternconfig",
										"description": "Connect to external MCP server using standard input/output",
										"value": map[string]interface{}{
											"config": map[string]interface{}{
												"enabled":     true,
												"command":     "node",
												"args":        []string{"/path/to/mcp-server.js"},
												"env":         map[string]interface{}{},
												"timeout":     30,
												"description": "Node.js MCP server",
											},
										},
									},
									"sse": map[string]interface{}{
										"summary":     "SSEpatternconfig",
										"description": "Connect to external MCP server using Server-Sent Events",
										"value": map[string]interface{}{
											"config": map[string]interface{}{
												"enabled":     true,
												"transport":   "sse",
												"url":         "http://127.0.0.1:8082/sse",
												"timeout":     30,
												"description": "SSE MCP server",
											},
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "operation successful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"message": map[string]interface{}{
												"type":    "string",
												"example": "External MCP configuration saved",
											},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Request parameter error (e.g. config format incorrect, missing required fields, etc.)",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Error",
									},
									"example": map[string]interface{}{
										"error": "stdio mode requires command and args parameters",
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"External MCP Management"},
					"summary":     "delete external MCP",
					"description": "Delete the specified external MCP configuration",
					"operationId": "deleteExternalMCP",
					"parameters": []map[string]interface{}{
						{
							"name":        "name",
							"in":          "path",
							"required":    true,
							"description": "MCPname",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "deletion successful",
						},
						"404": map[string]interface{}{
							"description": "MCP not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/external-mcp/{name}/start": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"External MCP Management"},
					"summary":     "start external MCP",
					"description": "start specified external MCP server",
					"operationId": "startExternalMCP",
					"parameters": []map[string]interface{}{
						{
							"name":        "name",
							"in":          "path",
							"required":    true,
							"description": "MCPname",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "startsuccessful",
						},
						"404": map[string]interface{}{
							"description": "MCP not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/external-mcp/{name}/stop": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"External MCP Management"},
					"summary":     "stop external MCP",
					"description": "stop specified external MCP server",
					"operationId": "stopExternalMCP",
					"parameters": []map[string]interface{}{
						{
							"name":        "name",
							"in":          "path",
							"required":    true,
							"description": "MCPname",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "stopsuccessful",
						},
						"404": map[string]interface{}{
							"description": "MCP not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/attack-chain/{conversationId}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"attack chain"},
					"summary":     "get attack chain",
					"description": "get attack chain visualization data for specified conversation",
					"operationId": "getAttackChain",
					"parameters": []map[string]interface{}{
						{
							"name":        "conversationId",
							"in":          "path",
							"required":    true,
							"description": "conversation ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/AttackChain",
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "conversation not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/attack-chain/{conversationId}/regenerate": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"attack chain"},
					"summary":     "regenerate attack chain",
					"description": "regenerate attack chain visualization data for specified conversation",
					"operationId": "regenerateAttackChain",
					"parameters": []map[string]interface{}{
						{
							"name":        "conversationId",
							"in":          "path",
							"required":    true,
							"description": "conversation ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "regenerated successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/AttackChain",
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "conversation not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/conversations/{id}/pinned": map[string]interface{}{
				"put": map[string]interface{}{
					"tags":        []string{"Conversation Management"},
					"summary":     "pin/unpin conversation",
					"description": "Set or cancel the pinned status of a conversation",
					"operationId": "updateConversationPinned",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "conversation ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"pinned"},
									"properties": map[string]interface{}{
										"pinned": map[string]interface{}{
											"type":        "boolean",
											"description": "Whether pinned to top",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "update successful",
						},
						"404": map[string]interface{}{
							"description": "conversation not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/knowledge/categories": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Knowledge Base"},
					"summary":     "get categories",
					"description": "get all knowledge base categories",
					"operationId": "getKnowledgeCategories",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"categories": map[string]interface{}{
												"type":        "array",
												"description": "Category list",
												"items": map[string]interface{}{
													"type": "string",
												},
											},
											"enabled": map[string]interface{}{
												"type":        "boolean",
												"description": "Knowledge base enabled",
											},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/knowledge/items": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Knowledge Base"},
					"summary":     "list knowledge items",
					"description": "get all knowledge items in the knowledge base",
					"operationId": "getKnowledgeItems",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"items": map[string]interface{}{
												"type":        "array",
												"description": "Knowledge item list",
											},
											"enabled": map[string]interface{}{
												"type":        "boolean",
												"description": "Knowledge base enabled",
											},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"post": map[string]interface{}{
					"tags":        []string{"Knowledge Base"},
					"summary":     "create knowledge item",
					"description": "Create a new knowledge item",
					"operationId": "createKnowledgeItem",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":        "object",
									"description": "Knowledge item data",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "createsuccessful",
						},
						"400": map[string]interface{}{
							"description": "Request parameter error",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/knowledge/items/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Knowledge Base"},
					"summary":     "get knowledge item",
					"description": "get detailed info for specified knowledge item",
					"operationId": "getKnowledgeItem",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "Knowledge item ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
						},
						"404": map[string]interface{}{
							"description": "Knowledge item not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"put": map[string]interface{}{
					"tags":        []string{"Knowledge Base"},
					"summary":     "update knowledge item",
					"description": "Update the specified knowledge item",
					"operationId": "updateKnowledgeItem",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "Knowledge item ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":        "object",
									"description": "Knowledge item data",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "update successful",
						},
						"404": map[string]interface{}{
							"description": "Knowledge item not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"Knowledge Base"},
					"summary":     "delete knowledge item",
					"description": "Delete the specified knowledge item",
					"operationId": "deleteKnowledgeItem",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "Knowledge item ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "deletion successful",
						},
						"404": map[string]interface{}{
							"description": "Knowledge item not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/knowledge/index-status": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Knowledge Base"},
					"summary":     "get index status",
					"description": "Get knowledge base index build status",
					"operationId": "getIndexStatus",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"enabled": map[string]interface{}{
												"type":        "boolean",
												"description": "Knowledge base enabled",
											},
											"total_items": map[string]interface{}{
												"type":        "integer",
												"description": "Total knowledge item count",
											},
											"indexed_items": map[string]interface{}{
												"type":        "integer",
												"description": "Number of indexed knowledge items",
											},
											"progress_percent": map[string]interface{}{
												"type":        "number",
												"description": "Indexing progress percentage",
											},
											"is_complete": map[string]interface{}{
												"type":        "boolean",
												"description": "Whether indexing is complete",
											},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/knowledge/index": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Knowledge Base"},
					"summary":     "build index",
					"description": "Build knowledge base vector index. By default only processes items without vectors; mode=full for full rebuild.",
					"operationId": "startKnowledgeIndex",
					"parameters": []map[string]interface{}{
						{
							"name":        "mode",
							"in":          "query",
							"required":    false,
							"description": "Index pattern: missing (default, fills in missing vectors) or full (full rebuild)",
							"schema": map[string]interface{}{
								"type":    "string",
								"enum":    []string{"missing", "full"},
								"default": "missing",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Index task started",
						},
						"400": map[string]interface{}{
							"description": "Invalid mode parameter",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
						"409": map[string]interface{}{
							"description": "Index task already in progress",
						},
					},
				},
			},
			"/api/knowledge/scan": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Knowledge Base"},
					"summary":     "scan knowledge base",
					"description": "Scan knowledge base directory and import new knowledge files",
					"operationId": "scanKnowledgeBase",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Scan task started",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/knowledge/search": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Knowledge Base"},
					"summary":     "search knowledge base",
					"description": "Search for relevant content in the knowledge base. Based on vector retrieval, returns most relevant results by cosine semantic similarity between query and knowledge chunks.\n**Search notes**:\n- Semantic similarity search: embedding vectors + cosine similarity, configurable similarity threshold and TopK\n- Can filter by metadata such as risk type (e.g. SQL Injection, XSS, file upload, etc.)\n- Recommended to first call `/api/knowledge/categories` to get the list of available risk types\n**Usage example**:\n```json\n{\n  \"query\": \"SQL injection vulnerability detection methods\",\n  \"riskType\": \"SQL Injection\",\n  \"topK\": 5,\n  \"threshold\": 0.7\n}\n```",
					"operationId": "searchKnowledge",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"query"},
									"properties": map[string]interface{}{
										"query": map[string]interface{}{
											"type":        "string",
											"description": "Search query content, describe the security knowledge topic you want to learn about (required)",
											"example":     "SQL injection vulnerability detection methods",
										},
										"riskType": map[string]interface{}{
											"type":        "string",
											"description": "Optional: specify risk type (e.g. SQL Injection, XSS, File Upload). Recommended to first call `/api/knowledge/categories` to get available risk type list, then use the correct risk type for precise search to greatly reduce retrieval time. Searches all types if not specified.",
											"example":     "SQL Injection",
										},
										"topK": map[string]interface{}{
											"type":        "integer",
											"description": "Optional: number of top-K results to return, default 5",
											"default":     5,
											"minimum":     1,
											"maximum":     50,
											"example":     5,
										},
										"threshold": map[string]interface{}{
											"type":        "number",
											"format":      "float",
											"description": "Optional: similarity threshold (0–1), default 0.7. Only results with similarity >= this value will be returned",
											"default":     0.7,
											"minimum":     0,
											"maximum":     1,
											"example":     0.7,
										},
									},
								},
								"examples": map[string]interface{}{
									"basic": map[string]interface{}{
										"summary":     "basic search",
										"description": "Simplest search, only provide query content",
										"value": map[string]interface{}{
											"query": "SQL injection vulnerability detection methods",
										},
									},
									"withRiskType": map[string]interface{}{
										"summary":     "search by risk type",
										"description": "Specify risk type for precise search",
										"value": map[string]interface{}{
											"query":     "SQL injection vulnerability detection methods",
											"riskType":  "SQL Injection",
											"topK":      5,
											"threshold": 0.7,
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "searchsuccessful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"results": map[string]interface{}{
												"type":        "array",
												"description": "Search result list; each result contains: item (knowledge item info), chunks (matching knowledge chunks), score (similarity score)",
												"items": map[string]interface{}{
													"type": "object",
													"properties": map[string]interface{}{
														"item": map[string]interface{}{
															"type":        "object",
															"description": "Knowledge item info",
														},
														"chunks": map[string]interface{}{
															"type":        "array",
															"description": "Matching knowledge chunk list",
														},
														"score": map[string]interface{}{
															"type":        "number",
															"description": "Similarity score (between 0 and 1)",
														},
													},
												},
											},
											"enabled": map[string]interface{}{
												"type":        "boolean",
												"description": "Knowledge base enabled",
											},
										},
									},
									"example": map[string]interface{}{
										"results": []map[string]interface{}{
											{
												"item": map[string]interface{}{
													"id":       "item-1",
													"title":    "SQL Injection Vulnerability Detection",
													"category": "SQL Injection",
												},
												"chunks": []map[string]interface{}{
													{
														"text": "Methods for detecting SQL injection vulnerabilities include...",
													},
												},
												"score": 0.85,
											},
										},
										"enabled": true,
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Request parameter error (e.g. query is empty)",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Error",
									},
									"example": map[string]interface{}{
										"error": "query cannot be empty",
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
						"500": map[string]interface{}{
							"description": "Server internal error (e.g. knowledge base not enabled or retrieval failed)",
						},
					},
				},
			},
			"/api/knowledge/retrieval-logs": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Knowledge Base"},
					"summary":     "get retrieval log",
					"description": "get knowledge base retrieval log",
					"operationId": "getRetrievalLogs",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"logs": map[string]interface{}{
												"type":        "array",
												"description": "Retrieval log list",
											},
											"enabled": map[string]interface{}{
												"type":        "boolean",
												"description": "Knowledge base enabled",
											},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/api/knowledge/retrieval-logs/{id}": map[string]interface{}{
				"delete": map[string]interface{}{
					"tags":        []string{"Knowledge Base"},
					"summary":     "delete retrieval log",
					"description": "Delete the specified retrieval log",
					"operationId": "deleteRetrievalLog",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "logID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "deletion successful",
						},
						"404": map[string]interface{}{
							"description": "Log not found",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			// ==================== Conversation Interaction - missing endpoints ====================
			"/api/conversations/{id}/delete-turn": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Conversation Interaction"},
					"summary":     "delete conversation turn",
					"description": "Delete the conversation round containing the specified message (all messages from that user message to before the next user message), and clear the last_react status.",
					"operationId": "deleteConversationTurn",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "conversation ID",
							"schema":      map[string]interface{}{"type": "string"},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"messageId"},
									"properties": map[string]interface{}{
										"messageId": map[string]interface{}{
											"type":        "string",
											"description": "Anchor message ID identifying the round to delete",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "deletion successful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"deletedMessageIds": map[string]interface{}{
												"type":        "array",
												"items":       map[string]interface{}{"type": "string"},
												"description": "List of deleted message IDs",
											},
											"message": map[string]interface{}{
												"type":    "string",
												"example": "ok",
											},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{"description": "Parameter error or delete failed"},
						"401": map[string]interface{}{"description": "Unauthorized"},
						"404": map[string]interface{}{"description": "conversation not found"},
					},
				},
			},
			"/api/messages/{id}/process-details": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Conversation Interaction"},
					"summary":     "get cancellation process details",
					"description": "Load execution process details for the specified message on demand with pagination, including tool call, thinking process and other events. Default returns 50 items; pass full=1 explicitly when export or legacy integration needs full data.",
					"operationId": "getMessageProcessDetails",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "messageID",
							"schema":      map[string]interface{}{"type": "string"},
						},
						{
							"name":        "summary",
							"in":          "query",
							"required":    false,
							"description": "Only return process details summary (total / iterationCount / maxIteration)",
							"schema":      map[string]interface{}{"type": "boolean"},
						},
						{
							"name":        "limit",
							"in":          "query",
							"required":    false,
							"description": "Page size, default 50, max 500",
							"schema":      map[string]interface{}{"type": "integer", "default": 50, "maximum": 500},
						},
						{
							"name":        "offset",
							"in":          "query",
							"required":    false,
							"description": "Pagination offset, default 0",
							"schema":      map[string]interface{}{"type": "integer", "default": 0},
						},
						{
							"name":        "full",
							"in":          "query",
							"required":    false,
							"description": "Explicitly return full process details; recommended only for export/legacy integration compatibility",
							"schema":      map[string]interface{}{"type": "boolean"},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"processDetails": map[string]interface{}{
												"type": "array",
												"items": map[string]interface{}{
													"type": "object",
													"properties": map[string]interface{}{
														"id":             map[string]interface{}{"type": "string", "description": "Detail record ID"},
														"messageId":      map[string]interface{}{"type": "string", "description": "Parent message ID"},
														"conversationId": map[string]interface{}{"type": "string", "description": "Parent conversation ID"},
														"eventType":      map[string]interface{}{"type": "string", "description": "Event type (e.g. tool_call, thinking)"},
														"message":        map[string]interface{}{"type": "string", "description": "eventmessage"},
														"data":           map[string]interface{}{"description": "Event additional data (JSON object)"},
														"createdAt":      map[string]interface{}{"type": "string", "format": "date-time", "description": "created at"},
													},
												},
											},
											"total":   map[string]interface{}{"type": "integer", "description": "Process details total"},
											"offset":  map[string]interface{}{"type": "integer", "description": "current pagination offset"},
											"limit":   map[string]interface{}{"type": "integer", "description": "current page size"},
											"hasMore": map[string]interface{}{"type": "boolean", "description": "Whether there are more process details"},
											"summary": map[string]interface{}{
												"type":        "object",
												"description": "Summary returned when summary=1",
											},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{"description": "Parameter error"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},

			// ==================== Batch Tasks - missing endpoints ====================
			"/api/batch-tasks/{queueId}/rerun": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Batch Tasks"},
					"summary":     "re-run batch task queue",
					"description": "Reset a completed or cancelled batch task queue to restart execution of all tasks.",
					"operationId": "rerunBatchQueue",
					"parameters": []map[string]interface{}{
						{
							"name":        "queueId",
							"in":          "path",
							"required":    true,
							"description": "queueID",
							"schema":      map[string]interface{}{"type": "string"},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "re-run successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"message": map[string]interface{}{"type": "string", "example": "batch task has resumed execution"},
											"queueId": map[string]interface{}{"type": "string", "description": "queueID"},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{"description": "Only completed or cancelled queues can be re-run"},
						"401": map[string]interface{}{"description": "Unauthorized"},
						"404": map[string]interface{}{"description": "Queue not found"},
					},
				},
			},
			"/api/batch-tasks/{queueId}/metadata": map[string]interface{}{
				"put": map[string]interface{}{
					"tags":        []string{"Batch Tasks"},
					"summary":     "modify queue metadata",
					"description": "Modify the title, role, and agent pattern of the batch task queue.",
					"operationId": "updateBatchQueueMetadata",
					"parameters": []map[string]interface{}{
						{
							"name":        "queueId",
							"in":          "path",
							"required":    true,
							"description": "queueID",
							"schema":      map[string]interface{}{"type": "string"},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"title":     map[string]interface{}{"type": "string", "description": "queuetitle"},
										"role":      map[string]interface{}{"type": "string", "description": "role name to use"},
										"agentMode": map[string]interface{}{"type": "string", "description": "Agent pattern", "enum": []string{"eino_single", "deep", "plan_execute", "supervisor"}},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "update successful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"queue": map[string]interface{}{"$ref": "#/components/schemas/BatchQueue"},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{"description": "Parameter error"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/batch-tasks/{queueId}/schedule": map[string]interface{}{
				"put": map[string]interface{}{
					"tags":        []string{"Batch Tasks"},
					"summary":     "modify queue scheduling config",
					"description": "Modify the scheduling pattern and Cron expression of the batch task queue. Cannot be modified while queue is running.",
					"operationId": "updateBatchQueueSchedule",
					"parameters": []map[string]interface{}{
						{
							"name":        "queueId",
							"in":          "path",
							"required":    true,
							"description": "queueID",
							"schema":      map[string]interface{}{"type": "string"},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"scheduleMode": map[string]interface{}{"type": "string", "description": "scheduling mode", "enum": []string{"manual", "cron"}},
										"cronExpr":     map[string]interface{}{"type": "string", "description": "Cron expression (required when scheduleMode is cron)", "example": "0 2 * * *"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "update successful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"queue": map[string]interface{}{"$ref": "#/components/schemas/BatchQueue"},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{"description": "Parameter error or queue is running"},
						"401": map[string]interface{}{"description": "Unauthorized"},
						"404": map[string]interface{}{"description": "Queue not found"},
					},
				},
			},
			"/api/batch-tasks/{queueId}/schedule-enabled": map[string]interface{}{
				"put": map[string]interface{}{
					"tags":        []string{"Batch Tasks"},
					"summary":     "toggle Cron automatic scheduling",
					"description": "Enable or disable automatic Cron scheduling for the batch task queue; manual execution is not affected.",
					"operationId": "setBatchQueueScheduleEnabled",
					"parameters": []map[string]interface{}{
						{
							"name":        "queueId",
							"in":          "path",
							"required":    true,
							"description": "queueID",
							"schema":      map[string]interface{}{"type": "string"},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"scheduleEnabled"},
									"properties": map[string]interface{}{
										"scheduleEnabled": map[string]interface{}{"type": "boolean", "description": "enable automatic scheduling"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "settingssuccessful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"queue": map[string]interface{}{"$ref": "#/components/schemas/BatchQueue"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
						"404": map[string]interface{}{"description": "Queue not found"},
					},
				},
			},
			// ==================== FOFA Info Gathering ====================
			"/api/fofa/search": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"FOFA Info Gathering"},
					"summary":     "FOFAsearch",
					"description": "execute FOFA search query via backend proxy, returns asset info.",
					"operationId": "fofaSearch",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"query"},
									"properties": map[string]interface{}{
										"query":  map[string]interface{}{"type": "string", "description": "FOFA query syntax", "example": "domain=\"example.com\""},
										"size":   map[string]interface{}{"type": "integer", "description": "return count (default 100, max 10000)", "default": 100},
										"page":   map[string]interface{}{"type": "integer", "description": "page number (default 1)", "default": 1},
										"fields": map[string]interface{}{"type": "string", "description": "return fields, comma-separated", "example": "host,ip,port,title"},
										"full":   map[string]interface{}{"type": "boolean", "description": "whether to query all data", "default": false},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "searchsuccessful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"query":         map[string]interface{}{"type": "string", "description": "actually executed query"},
											"size":          map[string]interface{}{"type": "integer"},
											"page":          map[string]interface{}{"type": "integer"},
											"total":         map[string]interface{}{"type": "integer", "description": "Total match count"},
											"fields":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
											"results_count": map[string]interface{}{"type": "integer"},
											"results":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "object"}, "description": "search result list"},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{"description": "Parameter error"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/fofa/parse": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"FOFA Info Gathering"},
					"summary":     "parse natural language to FOFA syntax",
					"description": "Use AI to parse a natural language description into FOFA query syntax; requires human confirmation before executing the query.",
					"operationId": "fofaParse",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"text"},
									"properties": map[string]interface{}{
										"text": map[string]interface{}{"type": "string", "description": "Natural language description", "example": "Find websites using WordPress"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "parsed successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"query":       map[string]interface{}{"type": "string", "description": "generated FOFA query syntax"},
											"explanation": map[string]interface{}{"type": "string", "description": "Syntax explanation"},
											"warnings":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Potential risk or ambiguity hints"},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{"description": "Parameter error"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},

			// ==================== Config Management - missing endpoints ====================
			"/api/config/test-vision": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Config Management"},
					"summary":     "test vision model connection",
					"description": "Test whether the Vision model API is available. When vision.api_key/base_url is empty, the openai section can be passed as fallback.",
					"operationId": "testVision",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"vision"},
									"properties": map[string]interface{}{
										"vision": map[string]interface{}{"$ref": "#/components/schemas/VisionConfig"},
										"openai": map[string]interface{}{
											"type":        "object",
											"description": "Primary LLM config (used for API Key/Base URL fallback when vision field is null)",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Test result",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"success":    map[string]interface{}{"type": "boolean"},
											"error":      map[string]interface{}{"type": "string"},
											"model":      map[string]interface{}{"type": "string"},
											"latency_ms": map[string]interface{}{"type": "number"},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{"description": "Parameter error"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/config/test-openai": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Config Management"},
					"summary":     "test OpenAI API connection",
					"description": "Test whether the specified OpenAI/Claude API config is available by sending a minimal request to verify connectivity.",
					"operationId": "testOpenAI",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"api_key", "model"},
									"properties": map[string]interface{}{
										"provider": map[string]interface{}{"type": "string", "description": "LLM provider (openai/claude)", "example": "openai"},
										"base_url": map[string]interface{}{"type": "string", "description": "API base URL (optional, defaults to provider-specific URL)"},
										"api_key":  map[string]interface{}{"type": "string", "description": "API key"},
										"model":    map[string]interface{}{"type": "string", "description": "modelname", "example": "gpt-4"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Test result",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"success":    map[string]interface{}{"type": "boolean", "description": "whether connection succeeded"},
											"error":      map[string]interface{}{"type": "string", "description": "Failure reason (when success=false)"},
											"model":      map[string]interface{}{"type": "string", "description": "actually used model (when success=true)"},
											"latency_ms": map[string]interface{}{"type": "number", "description": "latency in milliseconds (when success=true)"},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{"description": "Parameter error"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/config/test-typesafe": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Config Management"},
					"summary":     "test TypeSafe Jev connection",
					"description": "Send a minimal Noul request to validate whether the TypeSafe System One API key is usable.",
					"operationId": "testTypeSafe",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"api_key"},
									"properties": map[string]interface{}{
										"base_url": map[string]interface{}{"type": "string", "description": "Optional, defaults to https://api.typesafe.ai"},
										"api_key":  map[string]interface{}{"type": "string", "description": "TypeSafe API Key"},
										"model":    map[string]interface{}{"type": "string", "description": "optional, default jev-latest", "example": "jev-latest"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Test result",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"success":    map[string]interface{}{"type": "boolean"},
											"error":      map[string]interface{}{"type": "string"},
											"model":      map[string]interface{}{"type": "string"},
											"latency_ms": map[string]interface{}{"type": "number"},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{"description": "Parameter error"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/config/list-models": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Config Management"},
					"summary":     "get model list",
					"description": "Agent calls OpenAI-compatible GET /models, returns available model ID list. Not supported for Claude.",
					"operationId": "listModels",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"api_key"},
									"properties": map[string]interface{}{
										"provider": map[string]interface{}{"type": "string", "description": "LLM provider (openai/claude)", "example": "openai"},
										"base_url": map[string]interface{}{"type": "string", "description": "API base URL (optional)"},
										"api_key":  map[string]interface{}{"type": "string", "description": "API key"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "get result",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"success":   map[string]interface{}{"type": "boolean"},
											"supported": map[string]interface{}{"type": "boolean"},
											"error":     map[string]interface{}{"type": "string"},
											"models":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
											"count":     map[string]interface{}{"type": "integer"},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{"description": "Parameter error"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},

			// ==================== Terminal ====================
			"/api/terminal/run": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Terminal"},
					"summary":     "execute terminal command",
					"description": "Execute a shell command on the server and return the result.",
					"operationId": "terminalRun",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"command"},
									"properties": map[string]interface{}{
										"command": map[string]interface{}{"type": "string", "description": "Command to execute"},
										"shell":   map[string]interface{}{"type": "string", "description": "shell type (default sh/cmd)"},
										"cwd":     map[string]interface{}{"type": "string", "description": "Working directory (optional)"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Execution completed",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"stdout":    map[string]interface{}{"type": "string", "description": "stdout"},
											"stderr":    map[string]interface{}{"type": "string", "description": "stderr"},
											"exit_code": map[string]interface{}{"type": "integer", "description": "Exit code"},
											"error":     map[string]interface{}{"type": "string", "description": "Execution error (optional)"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/terminal/run/stream": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Terminal"},
					"summary":     "streaming terminal command execution",
					"description": "Execute a shell command via SSE streaming, returning output in real time. Each event contains JSON: {\"t\": \"out\"|\"err\"|\"exit\", \"d\": \"data\", \"c\": exit code}",
					"operationId": "terminalRunStream",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"command"},
									"properties": map[string]interface{}{
										"command": map[string]interface{}{"type": "string", "description": "Command to execute"},
										"shell":   map[string]interface{}{"type": "string", "description": "shell type (default sh/cmd)"},
										"cwd":     map[string]interface{}{"type": "string", "description": "Working directory (optional)"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "SSE event stream",
							"content": map[string]interface{}{
								"text/event-stream": map[string]interface{}{
									"schema": map[string]interface{}{
										"type":        "string",
										"description": "Server-Sent Events stream, each event is JSON: {\"t\":\"out|err|exit\",\"d\":\"data\",\"c\":exitCode}",
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/terminal/ws": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Terminal"},
					"summary":     "WebSocket terminal",
					"description": "Establish an interactive terminal connection via WebSocket, with PTY support. The client sends text/binary data as command input, or JSON: {\"type\":\"resize\",\"cols\":80,\"rows\":24} to resize the terminal. The server returns binary PTY output.",
					"operationId": "terminalWS",
					"responses": map[string]interface{}{
						"101": map[string]interface{}{"description": "WebSocket connection established"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},

			// ==================== WebShell Management ====================
			"/api/webshell/connections": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"WebShell Management"},
					"summary":     "list WebShell connections",
					"description": "get all saved WebShell connection config list.",
					"operationId": "listWebshellConnections",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "array",
										"items": map[string]interface{}{
											"type": "object",
											"properties": map[string]interface{}{
												"id":         map[string]interface{}{"type": "string", "description": "Connection ID"},
												"url":        map[string]interface{}{"type": "string", "description": "WebShell URL"},
												"password":   map[string]interface{}{"type": "string", "description": "Connection password"},
												"type":       map[string]interface{}{"type": "string", "description": "Shelltype", "enum": []string{"php", "asp", "aspx", "jsp", "custom"}},
												"method":     map[string]interface{}{"type": "string", "description": "Request method", "enum": []string{"get", "post"}},
												"cmd_param":  map[string]interface{}{"type": "string", "description": "Command argument name"},
												"remark":     map[string]interface{}{"type": "string", "description": "Remark"},
												"created_at": map[string]interface{}{"type": "string", "format": "date-time"},
											},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
				"post": map[string]interface{}{
					"tags":        []string{"WebShell Management"},
					"summary":     "create WebShell connection",
					"description": "Save a new WebShell connection config.",
					"operationId": "createWebshellConnection",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"url"},
									"properties": map[string]interface{}{
										"url":       map[string]interface{}{"type": "string", "description": "WebShell URL"},
										"password":  map[string]interface{}{"type": "string", "description": "Connection password"},
										"type":      map[string]interface{}{"type": "string", "description": "Shelltype", "enum": []string{"php", "asp", "aspx", "jsp", "custom"}},
										"method":    map[string]interface{}{"type": "string", "description": "Request method", "enum": []string{"get", "post"}},
										"cmd_param": map[string]interface{}{"type": "string", "description": "Command argument name"},
										"remark":    map[string]interface{}{"type": "string", "description": "Remark"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "createsuccessful"},
						"400": map[string]interface{}{"description": "Parameter error"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/webshell/connections/{id}": map[string]interface{}{
				"put": map[string]interface{}{
					"tags":        []string{"WebShell Management"},
					"summary":     "update WebShell connection",
					"description": "Update an existing WebShell connection config.",
					"operationId": "updateWebshellConnection",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "description": "Connection ID", "schema": map[string]interface{}{"type": "string"}},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"url":       map[string]interface{}{"type": "string"},
										"password":  map[string]interface{}{"type": "string"},
										"type":      map[string]interface{}{"type": "string", "enum": []string{"php", "asp", "aspx", "jsp", "custom"}},
										"method":    map[string]interface{}{"type": "string", "enum": []string{"get", "post"}},
										"cmd_param": map[string]interface{}{"type": "string"},
										"remark":    map[string]interface{}{"type": "string"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "update successful"},
						"401": map[string]interface{}{"description": "Unauthorized"},
						"404": map[string]interface{}{"description": "Connection not found"},
					},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"WebShell Management"},
					"summary":     "delete WebShell connection",
					"description": "Delete the specified WebShell connection config.",
					"operationId": "deleteWebshellConnection",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "description": "Connection ID", "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "deletion successful"},
						"401": map[string]interface{}{"description": "Unauthorized"},
						"404": map[string]interface{}{"description": "Connection not found"},
					},
				},
			},
			"/api/webshell/connections/{id}/state": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"WebShell Management"},
					"summary":     "get connection status",
					"description": "get saved status data for WebShell connection.",
					"operationId": "getWebshellConnectionState",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "description": "Connection ID", "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"state": map[string]interface{}{"type": "object", "description": "status data (arbitrary JSON)"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
				"put": map[string]interface{}{
					"tags":        []string{"WebShell Management"},
					"summary":     "saveconnection status",
					"description": "Save WebShell connection status data.",
					"operationId": "saveWebshellConnectionState",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "description": "Connection ID", "schema": map[string]interface{}{"type": "string"}},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"state": map[string]interface{}{"type": "object", "description": "status data (arbitrary JSON)"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "savesuccessful"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/webshell/connections/{id}/ai-history": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"WebShell Management"},
					"summary":     "get AI conversation history",
					"description": "get AI-assisted conversation history messages for specified WebShell connection.",
					"operationId": "getWebshellAIHistory",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "description": "Connection ID", "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"conversationId": map[string]interface{}{"type": "string"},
											"messages": map[string]interface{}{
												"type": "array",
												"items": map[string]interface{}{
													"type": "object",
													"properties": map[string]interface{}{
														"id":        map[string]interface{}{"type": "string"},
														"role":      map[string]interface{}{"type": "string"},
														"content":   map[string]interface{}{"type": "string"},
														"createdAt": map[string]interface{}{"type": "string", "format": "date-time"},
													},
												},
											},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/webshell/connections/{id}/ai-conversations": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"WebShell Management"},
					"summary":     "list AI conversations",
					"description": "get all AI-assisted conversation list for specified WebShell connection.",
					"operationId": "listWebshellAIConversations",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "description": "Connection ID", "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "array",
										"items": map[string]interface{}{
											"type": "object",
											"properties": map[string]interface{}{
												"id":        map[string]interface{}{"type": "string"},
												"title":     map[string]interface{}{"type": "string"},
												"createdAt": map[string]interface{}{"type": "string", "format": "date-time"},
											},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/webshell/exec": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"WebShell Management"},
					"summary":     "execute WebShell command",
					"description": "execute remote command through specified WebShell connection.",
					"operationId": "webshellExec",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"url", "command"},
									"properties": map[string]interface{}{
										"url":       map[string]interface{}{"type": "string", "description": "WebShell URL"},
										"password":  map[string]interface{}{"type": "string"},
										"type":      map[string]interface{}{"type": "string", "enum": []string{"php", "asp", "aspx", "jsp", "custom"}},
										"method":    map[string]interface{}{"type": "string", "enum": []string{"get", "post"}},
										"cmd_param": map[string]interface{}{"type": "string"},
										"command":   map[string]interface{}{"type": "string", "description": "Command to execute"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "execution result",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"ok":        map[string]interface{}{"type": "boolean"},
											"output":    map[string]interface{}{"type": "string", "description": "command output"},
											"error":     map[string]interface{}{"type": "string", "description": "Error info"},
											"http_code": map[string]interface{}{"type": "integer", "description": "HTTP response code"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/webshell/file": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"WebShell Management"},
					"summary":     "WebShell file operations",
					"description": "perform remote file operations via WebShell (list directory, read/write file, create directory, rename, delete, upload, etc.).",
					"operationId": "webshellFileOp",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"url", "action", "path"},
									"properties": map[string]interface{}{
										"url":         map[string]interface{}{"type": "string", "description": "WebShell URL"},
										"password":    map[string]interface{}{"type": "string"},
										"type":        map[string]interface{}{"type": "string", "enum": []string{"php", "asp", "aspx", "jsp", "custom"}},
										"method":      map[string]interface{}{"type": "string", "enum": []string{"get", "post"}},
										"cmd_param":   map[string]interface{}{"type": "string"},
										"action":      map[string]interface{}{"type": "string", "description": "Operation type", "enum": []string{"list", "read", "delete", "write", "mkdir", "rename", "upload", "upload_chunk"}},
										"path":        map[string]interface{}{"type": "string", "description": "target file/directory path"},
										"target_path": map[string]interface{}{"type": "string", "description": "Target path (used when renaming)"},
										"content":     map[string]interface{}{"type": "string", "description": "File content (used when write/upload)"},
										"chunk_index": map[string]interface{}{"type": "integer", "description": "Chunk index (used when upload_chunk)"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Operation result",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"ok":     map[string]interface{}{"type": "boolean"},
											"output": map[string]interface{}{"type": "string"},
											"error":  map[string]interface{}{"type": "string"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},

			// ==================== Conversation Attachments ====================
			"/api/chat-uploads": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Conversation Attachments"},
					"summary":     "list attachments",
					"description": "Get conversation file list, including manually uploaded attachments, tool output and session artifacts, with filtering by session, project, source, filename and pagination.",
					"operationId": "listChatUploads",
					"parameters": []map[string]interface{}{
						{"name": "conversation", "in": "query", "required": false, "description": "Filter by conversation ID", "schema": map[string]interface{}{"type": "string"}},
						{"name": "project", "in": "query", "required": false, "description": "Filter by project ID", "schema": map[string]interface{}{"type": "string"}},
						{"name": "source", "in": "query", "required": false, "description": "Filter by source: upload/reduction/workspace/conversation_artifact/all", "schema": map[string]interface{}{"type": "string", "enum": []string{"all", "upload", "reduction", "workspace", "conversation_artifact"}}},
						{"name": "search", "in": "query", "required": false, "description": "Search by filename or subpath", "schema": map[string]interface{}{"type": "string"}},
						{"name": "page", "in": "query", "required": false, "description": "Page number, starts from 1", "schema": map[string]interface{}{"type": "integer", "default": 1}},
						{"name": "pageSize", "in": "query", "required": false, "description": "Page size, pass 'all' to return all", "schema": map[string]interface{}{"oneOf": []map[string]interface{}{{"type": "integer"}, {"type": "string", "enum": []string{"all"}}}}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"files": map[string]interface{}{
												"type": "array",
												"items": map[string]interface{}{
													"type": "object",
													"properties": map[string]interface{}{
														"relativePath":      map[string]interface{}{"type": "string"},
														"absolutePath":      map[string]interface{}{"type": "string"},
														"name":              map[string]interface{}{"type": "string"},
														"size":              map[string]interface{}{"type": "integer"},
														"modifiedUnix":      map[string]interface{}{"type": "integer"},
														"date":              map[string]interface{}{"type": "string"},
														"conversationId":    map[string]interface{}{"type": "string"},
														"conversationTitle": map[string]interface{}{"type": "string"},
														"projectId":         map[string]interface{}{"type": "string"},
														"projectName":       map[string]interface{}{"type": "string"},
														"subPath":           map[string]interface{}{"type": "string"},
														"source":            map[string]interface{}{"type": "string", "description": "upload/reduction/workspace/conversation_artifact"},
													},
												},
											},
											"folders": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
											"total":   map[string]interface{}{"type": "integer"},
											"page":    map[string]interface{}{"type": "integer"},
											"pageSize": map[string]interface{}{
												"type": "integer",
											},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
				"post": map[string]interface{}{
					"tags":        []string{"Conversation Attachments"},
					"summary":     "upload attachment",
					"description": "Upload file to the conversation attachment directory (multipart/form-data).",
					"operationId": "uploadChatFile",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"multipart/form-data": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"file"},
									"properties": map[string]interface{}{
										"file":           map[string]interface{}{"type": "string", "format": "binary", "description": "file to upload"},
										"conversationId": map[string]interface{}{"type": "string", "description": "Associated conversation ID (optional)"},
										"relativeDir":    map[string]interface{}{"type": "string", "description": "target directory relative path (optional)"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "uploadsuccessful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"ok":           map[string]interface{}{"type": "boolean"},
											"relativePath": map[string]interface{}{"type": "string"},
											"absolutePath": map[string]interface{}{"type": "string"},
											"name":         map[string]interface{}{"type": "string"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"Conversation Attachments"},
					"summary":     "delete attachment",
					"description": "Delete the specified conversation attachment file.",
					"operationId": "deleteChatUpload",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"path"},
									"properties": map[string]interface{}{
										"path": map[string]interface{}{"type": "string", "description": "File relative path"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "deletion successful"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/chat-uploads/export": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Conversation Attachments"},
					"summary":     "export attachment",
					"description": "Export conversation files as ZIP with current filters, includes manifest.json.",
					"operationId": "exportChatUploads",
					"parameters": []map[string]interface{}{
						{"name": "conversation", "in": "query", "required": false, "description": "Filter by conversation ID", "schema": map[string]interface{}{"type": "string"}},
						{"name": "project", "in": "query", "required": false, "description": "Filter by project ID", "schema": map[string]interface{}{"type": "string"}},
						{"name": "source", "in": "query", "required": false, "description": "Filter by source: upload/reduction/workspace/conversation_artifact/all", "schema": map[string]interface{}{"type": "string", "enum": []string{"all", "upload", "reduction", "workspace", "conversation_artifact"}}},
						{"name": "search", "in": "query", "required": false, "description": "Search by filename or subpath", "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "ZIPfile download",
							"content": map[string]interface{}{
								"application/zip": map[string]interface{}{
									"schema": map[string]interface{}{"type": "string", "format": "binary"},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/chat-uploads/download": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Conversation Attachments"},
					"summary":     "download attachment",
					"description": "Download the specified conversation attachment file.",
					"operationId": "downloadChatUpload",
					"parameters": []map[string]interface{}{
						{"name": "path", "in": "query", "required": true, "description": "File relative path", "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "file download",
							"content": map[string]interface{}{
								"application/octet-stream": map[string]interface{}{
									"schema": map[string]interface{}{"type": "string", "format": "binary"},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
						"404": map[string]interface{}{"description": "file not found"},
					},
				},
			},
			"/api/chat-uploads/path": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Conversation Attachments"},
					"summary":     "parse attachment path",
					"description": "Resolve a relative path or internal virtual path from file management to a server absolute path for file/directory path operations.",
					"operationId": "resolveChatUploadPath",
					"parameters": []map[string]interface{}{
						{"name": "path", "in": "query", "required": true, "description": "Relative path or virtual path (e.g. __workspace__/projects/<id>/csv)", "schema": map[string]interface{}{"type": "string"}},
						{"name": "kind", "in": "query", "required": false, "description": "Path type: file/directory, defaults to file", "schema": map[string]interface{}{"type": "string", "enum": []string{"file", "directory"}}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "parsed successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"absolutePath": map[string]interface{}{"type": "string"},
											"isDir":        map[string]interface{}{"type": "boolean"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
						"403": map[string]interface{}{"description": "access denied"},
						"404": map[string]interface{}{"description": "Path does not exist"},
					},
				},
			},
			"/api/chat-uploads/content": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Conversation Attachments"},
					"summary":     "get attachment text content",
					"description": "read and return text file content.",
					"operationId": "getChatUploadContent",
					"parameters": []map[string]interface{}{
						{"name": "path", "in": "query", "required": true, "description": "File relative path", "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"content": map[string]interface{}{"type": "string", "description": "File text content"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
						"404": map[string]interface{}{"description": "file not found"},
					},
				},
				"put": map[string]interface{}{
					"tags":        []string{"Conversation Attachments"},
					"summary":     "write attachment text content",
					"description": "Write or overwrite the content of a text file.",
					"operationId": "putChatUploadContent",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"path", "content"},
									"properties": map[string]interface{}{
										"path":    map[string]interface{}{"type": "string", "description": "File relative path"},
										"content": map[string]interface{}{"type": "string", "description": "File text content"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Write successful"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/chat-uploads/mkdir": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Conversation Attachments"},
					"summary":     "create attachment directory",
					"description": "Create a subdirectory under the conversation attachment directory.",
					"operationId": "mkdirChatUpload",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"name"},
									"properties": map[string]interface{}{
										"parent": map[string]interface{}{"type": "string", "description": "parent directory relative path"},
										"name":   map[string]interface{}{"type": "string", "description": "directoryname"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "createsuccessful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"ok":           map[string]interface{}{"type": "boolean"},
											"relativePath": map[string]interface{}{"type": "string"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/chat-uploads/rename": map[string]interface{}{
				"put": map[string]interface{}{
					"tags":        []string{"Conversation Attachments"},
					"summary":     "rename attachment",
					"description": "rename conversation attachment file or directory.",
					"operationId": "renameChatUpload",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"path", "newName"},
									"properties": map[string]interface{}{
										"path":    map[string]interface{}{"type": "string", "description": "current file relative path"},
										"newName": map[string]interface{}{"type": "string", "description": "new name"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "renamed successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"ok":           map[string]interface{}{"type": "boolean"},
											"relativePath": map[string]interface{}{"type": "string"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},

			// ==================== Bot Integration ====================
			"/api/robot/wecom": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Bot Integration"},
					"summary":     "WeCom callback verification",
					"description": "WeCom server URL verification callback (used when configuring message receiving address). No authentication required.",
					"operationId": "wecomCallbackVerify",
					"security":    []map[string]interface{}{},
					"parameters": []map[string]interface{}{
						{"name": "msg_signature", "in": "query", "required": true, "schema": map[string]interface{}{"type": "string"}},
						{"name": "timestamp", "in": "query", "required": true, "schema": map[string]interface{}{"type": "string"}},
						{"name": "nonce", "in": "query", "required": true, "schema": map[string]interface{}{"type": "string"}},
						{"name": "echostr", "in": "query", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Verification successful, returns decrypted echostr"},
					},
				},
				"post": map[string]interface{}{
					"tags":        []string{"Bot Integration"},
					"summary":     "WeCom message callback",
					"description": "Receive message events pushed by WeCom. No authentication required, called by WeCom servers.",
					"operationId": "wecomCallbackMessage",
					"security":    []map[string]interface{}{},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Processed successfully"},
					},
				},
			},
			"/api/robot/dingtalk": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Bot Integration"},
					"summary":     "DingTalk message callback",
					"description": "Receive message events pushed by DingTalk. No authentication required, called by DingTalk servers.",
					"operationId": "dingtalkCallback",
					"security":    []map[string]interface{}{},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Processed successfully"},
					},
				},
			},
			"/api/robot/lark": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Bot Integration"},
					"summary":     "Feishu message callback",
					"description": "Receive message events pushed by Feishu. No authentication required, called by Feishu servers.",
					"operationId": "larkCallback",
					"security":    []map[string]interface{}{},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Processed successfully"},
					},
				},
			},
			"/api/robot/test": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Bot Integration"},
					"summary":     "test robot message processing",
					"description": "Simulate bot message processing flow for debugging and validation. Requires login authentication.",
					"operationId": "testRobot",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"platform", "text"},
									"properties": map[string]interface{}{
										"platform": map[string]interface{}{"type": "string", "description": "platform type", "enum": []string{"dingtalk", "lark", "wecom"}},
										"user_id":  map[string]interface{}{"type": "string", "description": "Simulated user ID", "example": "test"},
										"text":     map[string]interface{}{"type": "string", "description": "Message text", "example": "help"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Processed successfully"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},

			// ==================== Multi-Agent Markdown ====================
			"/api/multi-agent/markdown-agents": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Multi-Agent Markdown"},
					"summary":     "list Markdown agents",
					"description": "get all multi-agent Markdown definition file list.",
					"operationId": "listMarkdownAgents",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"agents": map[string]interface{}{
												"type": "array",
												"items": map[string]interface{}{
													"type": "object",
													"properties": map[string]interface{}{
														"filename":        map[string]interface{}{"type": "string", "description": "Filename"},
														"id":              map[string]interface{}{"type": "string", "description": "Agent ID"},
														"name":            map[string]interface{}{"type": "string", "description": "Agent name"},
														"description":     map[string]interface{}{"type": "string", "description": "Agent description"},
														"is_orchestrator": map[string]interface{}{"type": "boolean", "description": "Whether this is the orchestrator"},
														"kind":            map[string]interface{}{"type": "string", "description": "Orchestration type"},
													},
												},
											},
											"dir": map[string]interface{}{"type": "string", "description": "Agent definition directory path"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
				"post": map[string]interface{}{
					"tags":        []string{"Multi-Agent Markdown"},
					"summary":     "create Markdown agent",
					"description": "Create a new multi-agent Markdown definition file.",
					"operationId": "createMarkdownAgent",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"name"},
									"properties": map[string]interface{}{
										"filename":       map[string]interface{}{"type": "string", "description": "filename (optional, auto-generated)"},
										"id":             map[string]interface{}{"type": "string", "description": "Agent ID"},
										"name":           map[string]interface{}{"type": "string", "description": "Agent name"},
										"description":    map[string]interface{}{"type": "string", "description": "Agent description"},
										"tools":          map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Available tool list"},
										"instruction":    map[string]interface{}{"type": "string", "description": "agent instruction"},
										"bind_role":      map[string]interface{}{"type": "string", "description": "Bound role"},
										"max_iterations": map[string]interface{}{"type": "integer", "description": "maximum iterations"},
										"kind":           map[string]interface{}{"type": "string", "description": "Orchestration type"},
										"raw":            map[string]interface{}{"type": "string", "description": "raw Markdown content"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "createsuccessful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"filename": map[string]interface{}{"type": "string"},
											"message":  map[string]interface{}{"type": "string", "example": "created"},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{"description": "Parameter error"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},
			"/api/multi-agent/markdown-agents/{filename}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Multi-Agent Markdown"},
					"summary":     "get Markdown agent details",
					"description": "get detailed content of specified Markdown agent definition file.",
					"operationId": "getMarkdownAgent",
					"parameters": []map[string]interface{}{
						{"name": "filename", "in": "path", "required": true, "description": "Filename", "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"filename":        map[string]interface{}{"type": "string"},
											"raw":             map[string]interface{}{"type": "string", "description": "raw Markdown content"},
											"id":              map[string]interface{}{"type": "string"},
											"name":            map[string]interface{}{"type": "string"},
											"description":     map[string]interface{}{"type": "string"},
											"tools":           map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
											"instruction":     map[string]interface{}{"type": "string"},
											"bind_role":       map[string]interface{}{"type": "string"},
											"max_iterations":  map[string]interface{}{"type": "integer"},
											"kind":            map[string]interface{}{"type": "string"},
											"is_orchestrator": map[string]interface{}{"type": "boolean"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
						"404": map[string]interface{}{"description": "Agent not found"},
					},
				},
				"put": map[string]interface{}{
					"tags":        []string{"Multi-Agent Markdown"},
					"summary":     "update Markdown agent",
					"description": "Update the specified Markdown agent definition.",
					"operationId": "updateMarkdownAgent",
					"parameters": []map[string]interface{}{
						{"name": "filename", "in": "path", "required": true, "description": "Filename", "schema": map[string]interface{}{"type": "string"}},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"name":           map[string]interface{}{"type": "string"},
										"description":    map[string]interface{}{"type": "string"},
										"tools":          map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
										"instruction":    map[string]interface{}{"type": "string"},
										"bind_role":      map[string]interface{}{"type": "string"},
										"max_iterations": map[string]interface{}{"type": "integer"},
										"kind":           map[string]interface{}{"type": "string"},
										"raw":            map[string]interface{}{"type": "string"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "update successful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"message": map[string]interface{}{"type": "string", "example": "saved"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
						"404": map[string]interface{}{"description": "Agent not found"},
					},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"Multi-Agent Markdown"},
					"summary":     "delete Markdown agent",
					"description": "Delete the specified Markdown agent definition file.",
					"operationId": "deleteMarkdownAgent",
					"parameters": []map[string]interface{}{
						{"name": "filename", "in": "path", "required": true, "description": "Filename", "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "deletion successful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"message": map[string]interface{}{"type": "string", "example": "deleted"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
						"404": map[string]interface{}{"description": "Agent not found"},
					},
				},
			},

			// ==================== Skills Management - missing endpoints ====================
			"/api/skills/{name}/files": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Skills Management"},
					"summary":     "list skill pack files",
					"description": "get all file list under specified skill pack directory.",
					"operationId": "listSkillPackageFiles",
					"parameters": []map[string]interface{}{
						{"name": "name", "in": "path", "required": true, "description": "Skill name/ID", "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"files": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "file pathlist"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
						"404": map[string]interface{}{"description": "skill not found"},
					},
				},
			},
			"/api/skills/{name}/file": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Skills Management"},
					"summary":     "get skill pack file content",
					"description": "read content of specified file in skill pack.",
					"operationId": "getSkillPackageFile",
					"parameters": []map[string]interface{}{
						{"name": "name", "in": "path", "required": true, "description": "Skill name/ID", "schema": map[string]interface{}{"type": "string"}},
						{"name": "path", "in": "query", "required": true, "description": "File relative path", "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"path":    map[string]interface{}{"type": "string", "description": "file path"},
											"content": map[string]interface{}{"type": "string", "description": "File content"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
						"404": map[string]interface{}{"description": "file not found"},
					},
				},
				"put": map[string]interface{}{
					"tags":        []string{"Skills Management"},
					"summary":     "write skill pack file",
					"description": "Write or update file content in the skill package.",
					"operationId": "putSkillPackageFile",
					"parameters": []map[string]interface{}{
						{"name": "name", "in": "path", "required": true, "description": "Skill name/ID", "schema": map[string]interface{}{"type": "string"}},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"path"},
									"properties": map[string]interface{}{
										"path":    map[string]interface{}{"type": "string", "description": "File relative path"},
										"content": map[string]interface{}{"type": "string", "description": "File content"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "savesuccessful",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"message": map[string]interface{}{"type": "string", "example": "saved"},
											"path":    map[string]interface{}{"type": "string"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},

			// ==================== Monitoring - missing endpoints ====================
			"/api/monitor/executions/names": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Monitoring"},
					"summary":     "batch get tool names",
					"description": "Batch get tool names by execution ID list, eliminating frontend N+1 request issues.",
					"operationId": "batchGetToolNames",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"ids"},
									"properties": map[string]interface{}{
										"ids": map[string]interface{}{
											"type":        "array",
											"items":       map[string]interface{}{"type": "string"},
											"description": "Execution record ID list",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully, returns map of ID to tool name",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type":                 "object",
										"additionalProperties": map[string]interface{}{"type": "string"},
										"description":          "key is execution ID, value is tool name",
										"example":              map[string]interface{}{"exec-001": "nmap", "exec-002": "sqlmap"},
									},
								},
							},
						},
						"400": map[string]interface{}{"description": "Parameter error"},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},

			// ==================== Knowledge Base - missing endpoints ====================
			"/api/knowledge/stats": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Knowledge Base"},
					"summary":     "get knowledge base statistics",
					"description": "get overall knowledge base statistics info, including category count and item count.",
					"operationId": "getKnowledgeStats",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "retrieved successfully",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"enabled":          map[string]interface{}{"type": "boolean", "description": "Knowledge base enabled"},
											"total_categories": map[string]interface{}{"type": "integer", "description": "Category total"},
											"total_items":      map[string]interface{}{"type": "integer", "description": "Item total"},
										},
									},
								},
							},
						},
						"401": map[string]interface{}{"description": "Unauthorized"},
					},
				},
			},

			"/api/mcp": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"MCP"},
					"summary":     "MCP endpoint",
					"description": "MCP (Model Context Protocol) endpoint for handling MCP protocol requests.\n**Protocol notes**:\nThis endpoint follows the JSON-RPC 2.0 specification and supports the following methods:\n**1. initialize** - Initialize MCP connection\n```json\n{\n  \"jsonrpc\": \"2.0\",\n  \"id\": \"init-1\",\n  \"method\": \"initialize\",\n  \"params\": {\n    \"protocolVersion\": \"2024-11-05\",\n    \"capabilities\": {},\n    \"clientInfo\": {\n      \"name\": \"MyClient\",\n      \"version\": \"1.0.0\"\n    }\n  }\n}\n```\n**2. tools/list** - List all available tools\n```json\n{\n  \"jsonrpc\": \"2.0\",\n  \"id\": \"list-1\",\n  \"method\": \"tools/list\",\n  \"params\": {}\n}\n```\n**3. tools/call** - Call a tool\n```json\n{\n  \"jsonrpc\": \"2.0\",\n  \"id\": \"call-1\",\n  \"method\": \"tools/call\",\n  \"params\": {\n    \"name\": \"nmap\",\n    \"arguments\": {\n      \"target\": \"192.168.1.1\",\n      \"ports\": \"80,443\"\n    }\n  }\n}\n```\n**4. prompts/list** - List all prompt templates\n```json\n{\n  \"jsonrpc\": \"2.0\",\n  \"id\": \"prompts-list-1\",\n  \"method\": \"prompts/list\",\n  \"params\": {}\n}\n```\n**5. prompts/get** - Get a prompt template\n```json\n{\n  \"jsonrpc\": \"2.0\",\n  \"id\": \"prompt-get-1\",\n  \"method\": \"prompts/get\",\n  \"params\": {\n    \"name\": \"prompt-name\",\n    \"arguments\": {}\n  }\n}\n```\n**6. resources/list** - List all resources\n```json\n{\n  \"jsonrpc\": \"2.0\",\n  \"id\": \"resources-list-1\",\n  \"method\": \"resources/list\",\n  \"params\": {}\n}\n```\n**7. resources/read** - Read resource content\n```json\n{\n  \"jsonrpc\": \"2.0\",\n  \"id\": \"resource-read-1\",\n  \"method\": \"resources/read\",\n  \"params\": {\n    \"uri\": \"resource://example\"\n  }\n}\n```\n**Error code descriptions**:\n- `-32700`: Parse error - JSON parse error\n- `-32600`: Invalid Request - invalid request\n- `-32601`: Method not found - method does not exist\n- `-32602`: Invalid params - invalid parameters\n- `-32603`: Internal error - internal error",
					"operationId": "mcpEndpoint",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/MCPMessage",
								},
								"examples": map[string]interface{}{
									"listTools": map[string]interface{}{
										"summary":     "list all tools",
										"description": "get all available MCP tool list in the system",
										"value": map[string]interface{}{
											"jsonrpc": "2.0",
											"id":      "list-tools-1",
											"method":  "tools/list",
											"params":  map[string]interface{}{},
										},
									},
									"callTool": map[string]interface{}{
										"summary":     "call tool",
										"description": "call specified MCP tool",
										"value": map[string]interface{}{
											"jsonrpc": "2.0",
											"id":      "call-tool-1",
											"method":  "tools/call",
											"params": map[string]interface{}{
												"name": "nmap",
												"arguments": map[string]interface{}{
													"target": "192.168.1.1",
													"ports":  "80,443",
												},
											},
										},
									},
									"initialize": map[string]interface{}{
										"summary":     "initialize connection",
										"description": "Initialize MCP connection and retrieve server capabilities",
										"value": map[string]interface{}{
											"jsonrpc": "2.0",
											"id":      "init-1",
											"method":  "initialize",
											"params": map[string]interface{}{
												"protocolVersion": "2024-11-05",
												"capabilities":    map[string]interface{}{},
												"clientInfo": map[string]interface{}{
													"name":    "MyClient",
													"version": "1.0.0",
												},
											},
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "MCP response (JSON-RPC 2.0 format)",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/MCPResponse",
									},
									"examples": map[string]interface{}{
										"success": map[string]interface{}{
											"summary":     "successfulresponse",
											"description": "Response example for a successful tool call",
											"value": map[string]interface{}{
												"jsonrpc": "2.0",
												"id":      "call-tool-1",
												"result": map[string]interface{}{
													"content": []map[string]interface{}{
														{
															"type": "text",
															"text": "tool execution result...",
														},
													},
													"isError": false,
												},
											},
										},
										"error": map[string]interface{}{
											"summary":     "errorresponse",
											"description": "Response example for a failed tool call",
											"value": map[string]interface{}{
												"jsonrpc": "2.0",
												"id":      "call-tool-1",
												"error": map[string]interface{}{
													"code":    -32601,
													"message": "Tool not found",
													"data":    "tool 'unknown-tool' not found",
												},
											},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "request invalid format (JSON parsing failed)",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/MCPResponse",
									},
									"example": map[string]interface{}{
										"id": nil,
										"error": map[string]interface{}{
											"code":    -32700,
											"message": "Parse error",
											"data":    "unexpected end of JSON input",
										},
										"jsonrpc": "2.0",
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "Unauthorized, a valid token is required",
						},
						"405": map[string]interface{}{
							"description": "Method not allowed (only POST requests supported)",
						},
					},
				},
			},
		},
	}

	enrichSpecWithI18nKeys(spec)
	c.JSON(http.StatusOK, spec)
}

// GetConversationResults gets conversation results (OpenAPI endpoint)
// Note: create conversation and get conversation details use the standard /api/conversations endpoint directly
// this endpoint exists only to provide result aggregation
func (h *OpenAPIHandler) GetConversationResults(c *gin.Context) {
	conversationID := c.Param("id")

	// validate whether conversation exists
	conv, err := h.db.GetConversation(conversationID)
	if err != nil {
		h.logger.Error("failed to get conversation", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		return
	}

	// get cancellation message list
	messages, err := h.db.GetMessages(conversationID)
	if err != nil {
		h.logger.Error("failed to get cancellation messages", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// get vulnerability list
	vulnList, err := h.db.ListVulnerabilities(1000, 0, database.VulnerabilityListFilter{ConversationID: conversationID})
	if err != nil {
		h.logger.Warn("get vulnerability list failed", zap.Error(err))
		vulnList = []*database.Vulnerability{}
	}
	vulnerabilities := make([]database.Vulnerability, len(vulnList))
	for i, v := range vulnList {
		vulnerabilities[i] = *v
	}

	// get execution results (large historical results persisted by Eino reduction; no longer aggregated from file storage here)
	executionResults := []map[string]interface{}{}

	response := map[string]interface{}{
		"conversationId":   conv.ID,
		"messages":         messages,
		"vulnerabilities":  vulnerabilities,
		"executionResults": executionResults,
	}

	c.JSON(http.StatusOK, response)
}
