package multiagent

import (
	"strings"
)

// expandAlwaysVisibleNameSet expands always-visible tool names from config into a set that matches runtime tool names.
// Supports: built-in short names like read_file; external mcp::tool; runtime mcp__tool (OpenAI/Eino naming).
func expandAlwaysVisibleNameSet(names []string) map[string]struct{} {
	set := make(map[string]struct{}, len(names)*3)
	add := func(name string) {
		n := strings.TrimSpace(strings.ToLower(name))
		if n == "" {
			return
		}
		set[n] = struct{}{}
	}
	for _, raw := range names {
		n := strings.TrimSpace(strings.ToLower(raw))
		if n == "" {
			continue
		}
		add(n)
		if mcp, tool, ok := strings.Cut(n, "::"); ok && mcp != "" && tool != "" {
			// When an external tool is configured as mcp::tool, only expand to runtime mcp__tool to avoid short-name collisions with other MCP tools of the same name.
			add(mcp + "__" + tool)
			continue
		}
		if idx := strings.LastIndex(n, "__"); idx > 0 {
			mcp, tool := n[:idx], n[idx+2:]
			if mcp != "" && tool != "" {
				add(mcp + "::" + tool)
			}
			continue
		}
	}
	return set
}

// toolMatchesAlwaysVisible reports whether the runtime tool name matches the always-visible whitelist (including aliases).
func toolMatchesAlwaysVisible(runtimeName string, nameSet map[string]struct{}) bool {
	if len(nameSet) == 0 {
		return false
	}
	name := strings.TrimSpace(strings.ToLower(runtimeName))
	if name == "" {
		return false
	}
	if _, ok := nameSet[name]; ok {
		return true
	}
	if mcp, tool, ok := strings.Cut(name, "::"); ok && mcp != "" && tool != "" {
		if _, ok := nameSet[mcp+"__"+tool]; ok {
			return true
		}
		if _, ok := nameSet[tool]; ok {
			return true
		}
	}
	if idx := strings.LastIndex(name, "__"); idx > 0 {
		mcp, tool := name[:idx], name[idx+2:]
		if mcp != "" && tool != "" {
			if _, ok := nameSet[mcp+"::"+tool]; ok {
				return true
			}
			if _, ok := nameSet[tool]; ok {
				return true
			}
		}
	}
	return false
}
