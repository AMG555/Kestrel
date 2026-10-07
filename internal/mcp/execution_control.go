package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"kestrel/internal/database"
)

// RegisterExecutionControlTools registers tools that allow operators and agents
// to monitor, query, and cancel long-running tool executions.
func RegisterExecutionControlTools(r *Registry, db *database.DB) {
	if r == nil || db == nil {
		return
	}

	r.RegisterTool(&ToolDefinition{
		Name:        "get_tool_execution_status",
		Description: "Query the status, duration, error, and output preview of a tool execution by its execution_id. Useful for tracking asynchronous scans or long operations.",
		Class:       ToolClassReadOnly,
		ServerID:    "builtin",
		Parameters: mustSchema(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"execution_id": map[string]interface{}{
					"type":        "string",
					"description": "Unique ID of the tool execution to inspect",
				},
				"include_output": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to include the output preview (default: true)",
					"default":     true,
				},
			},
			"required": []string{"execution_id"},
		}),
	}, func(ctx context.Context, args map[string]interface{}) (string, error) {
		id, _ := args["execution_id"].(string)
		id = strings.TrimSpace(id)
		if id == "" {
			return "", fmt.Errorf("execution_id is required")
		}

		includeOutput := true
		if val, ok := args["include_output"].(bool); ok {
			includeOutput = val
		}

		exec, err := db.GetToolExecutionByID(id)
		if err != nil {
			return "", fmt.Errorf("querying tool execution: %w", err)
		}
		if exec == nil {
			return "", fmt.Errorf("tool execution not found: %s", id)
		}

		resp := map[string]interface{}{
			"id":          exec.ID,
			"tool_name":   exec.ToolName,
			"status":      exec.Status,
			"started_at":  exec.StartedAt,
			"duration_ms": exec.DurationMs,
		}
		if exec.CompletedAt != nil {
			resp["completed_at"] = exec.CompletedAt
		}
		if exec.Error != "" {
			resp["error"] = exec.Error
		}

		if includeOutput && exec.Result != "" {
			preview := exec.Result
			if len(preview) > 4096 {
				preview = preview[:4096] + "\n...[truncated]"
			}
			resp["output_preview"] = preview
			resp["output_bytes"] = exec.OutputBytes
			resp["output_truncated"] = exec.OutputTruncated
		}

		b, _ := json.MarshalIndent(resp, "", "  ")
		return string(b), nil
	})

	r.RegisterTool(&ToolDefinition{
		Name:        "cancel_tool_execution",
		Description: "Request cancellation of an in-flight tool execution by its execution_id.",
		Class:       ToolClassReadOnly,
		ServerID:    "builtin",
		Parameters: mustSchema(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"execution_id": map[string]interface{}{
					"type":        "string",
					"description": "Unique ID of the in-flight tool execution to cancel",
				},
				"reason": map[string]interface{}{
					"type":        "string",
					"description": "Optional cancellation reason",
				},
			},
			"required": []string{"execution_id"},
		}),
	}, func(ctx context.Context, args map[string]interface{}) (string, error) {
		id, _ := args["execution_id"].(string)
		id = strings.TrimSpace(id)
		if id == "" {
			return "", fmt.Errorf("execution_id is required")
		}

		reason, _ := args["reason"].(string)
		if strings.TrimSpace(reason) == "" {
			reason = "cancelled by operator or agent request"
		}

		if err := db.CancelToolExecution(id, reason); err != nil {
			return "", fmt.Errorf("cancelling tool execution: %w", err)
		}

		return fmt.Sprintf("Tool execution %s cancellation requested. Reason: %s", id, reason), nil
	})
}
