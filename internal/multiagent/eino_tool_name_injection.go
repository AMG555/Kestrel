package multiagent

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/components/tool"
)

// injectToolNamesOnlyInstruction prepends a compact tool-name-only section into
// the system instruction so the model can reference current callable names.
// toolSearchMiddlewareActive must be true when prependEinoMiddlewares mounted toolsearch (dynamic tools); do not infer this
// by scanning tool names — tool_search is injected by middleware and is usually absent from the pre-split tools list.
func injectToolNamesOnlyInstruction(ctx context.Context, instruction string, tools []tool.BaseTool, toolSearchMiddlewareActive bool) string {
	names := collectToolNames(ctx, tools)
	if len(names) == 0 {
		return strings.TrimSpace(instruction)
	}
	hasToolSearch := toolSearchMiddlewareActive
	if !hasToolSearch {
		for _, n := range names {
			if strings.EqualFold(strings.TrimSpace(n), "tool_search") {
				hasToolSearch = true
				break
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("The following is the tool name index bound to the current session (names only; no parameter JSON Schema).\n")
	sb.WriteString("Note: if tool_search is enabled, the list may contain \"non-resident\" tools that may not appear in the tool definitions sent to the model in the current round; before seeing the full schema for a tool, do not infer parameters from the name alone.\n")
	for _, name := range names {
		sb.WriteString("- ")
		sb.WriteString(name)
		sb.WriteByte('\n')
	}
	sb.WriteString("\nUsage rules:\n")
	sb.WriteString("1) The table above is a name index only; it contains no parameter definitions. Do not guess parameter names, types, enum values, or required fields.\n")
	if hasToolSearch {
		sb.WriteString("[MANDATORY / HIGHEST PRIORITY] This session has tool_search enabled (dynamic tool pool). Any tool that appears in the name index but whose full parameter schema is not visible in the \"tools definitions attached to the current request\" MUST be looked up via tool_search first; skipping tool_search to save tokens or speed up progress and calling the business tool directly is an explicitly prohibited error flow.\n")
		sb.WriteString("2) Default policy: whenever there is any uncertainty about a target tool's parameter definitions, call tool_search first; one extra tool_search call is always preferable to invoking a business tool without having seen its schema.\n")
		sb.WriteString("3) Call order: first call tool_search (sole required parameter regex_pattern: regex matching tool names, e.g. substring nuclei or ^exact_tool_name$) → in subsequent rounds confirm the target tool appears in the tools list and that you have read its schema → then make the actual call to that tool.\n")
		sb.WriteString("4) tool_search returns only the list of matching tool names; the schema is delivered in the next round after unlocking. Do not fabricate JSON parameters before the schema has appeared.\n")
		sb.WriteString("5) Do not invent tool names that do not exist.\n\n")
	} else {
		sb.WriteString("2) Before calling a specific tool, first confirm its parameter requirements (use the tool definitions in the current request as the authoritative source); when uncertain, clarify before calling.\n")
		sb.WriteString("3) Do not invent tool names that do not exist.\n\n")
	}
	if s := strings.TrimSpace(injectShellToolGuidance("", names)); s != "" {
		sb.WriteString(s)
		sb.WriteString("\n\n")
	}
	if s := strings.TrimSpace(instruction); s != "" {
		sb.WriteString(s)
	}
	return sb.String()
}

func collectToolNames(ctx context.Context, tools []tool.BaseTool) []string {
	if len(tools) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tools))
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		if t == nil {
			continue
		}
		info, err := t.Info(ctx)
		if err != nil || info == nil {
			continue
		}
		name := strings.TrimSpace(info.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, name)
	}
	return out
}

