package mcp

import (
	"fmt"
	"strings"

	"kestrel/internal/toolguard"
)

const toolGuardBlockedPrefix = "tool call blocked by security rule"
const toolGuardBlockedMetaKey = "kestrel.ai/blocked"

// toolGuardBlockError carries structured policy results through pre-run hooks.
type toolGuardBlockError struct{ result *ToolResult }

func (e *toolGuardBlockError) Error() string { return ToolResultPlainText(e.result) }

func toolResultProtocolMeta(result *ToolResult) map[string]interface{} {
	if result != nil && result.Blocked {
		return map[string]interface{}{toolGuardBlockedMetaKey: true}
	}
	return nil
}

// toolGuardBlockedResult uses the standard MCP error result so the refusal is
// visible both to the model and in persisted execution monitoring records.
func toolGuardBlockedResult(guard *toolguard.Manager, toolName string, args map[string]interface{}) *ToolResult {
	if guard == nil {
		return nil
	}
	match := guard.Check(toolName, args)
	if match == nil {
		return nil
	}
	message := toolGuardBlockedPrefix
	if custom := strings.TrimSpace(match.Message); custom != "" {
		message += ": " + custom
	}
	message += fmt.Sprintf("\nRule: %s (%s)\nMatched: %q", match.RuleName, match.RuleID, match.MatchedText)
	return &ToolResult{Content: []Content{{Type: "text", Text: message}}, IsError: true, Blocked: true}
}
