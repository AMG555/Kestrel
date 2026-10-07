package project

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

	"kestrel/internal/database"
)

// ScopePayload represents structured scope boundary definitions.
type ScopePayload struct {
	Targets []string `json:"targets"`
	Exclude []string `json:"exclude"`
	Notes   string   `json:"notes"`
}

// BuildScopeBlock formats project scope definitions into system prompt instructions.
func BuildScopeBlock(proj *database.Project) string {
	if proj == nil {
		return ""
	}

	var payload ScopePayload
	raw := strings.TrimSpace(proj.ScopeJSON)
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			// Fallback: check proj.Scope slice directly
			payload.Targets = proj.Scope
		}
	} else if len(proj.Scope) > 0 {
		payload.Targets = proj.Scope
	}

	if len(payload.Targets) == 0 && len(payload.Exclude) == 0 && payload.Notes == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Authorized Assessment Scope (Project: %s, ID: %s)\n", proj.Name, proj.ID))
	b.WriteString("The following boundaries define strictly authorized assets. **Never test assets outside these boundaries**:\n\n")

	if len(payload.Targets) > 0 {
		b.WriteString("**In-Scope Targets**:\n")
		for _, t := range payload.Targets {
			t = strings.TrimSpace(t)
			if t != "" {
				b.WriteString(fmt.Sprintf("- `%s`\n", t))
			}
		}
		b.WriteString("\n")
	}

	if len(payload.Exclude) > 0 {
		b.WriteString("**Explicitly Excluded (Do Not Touch)**:\n")
		for _, ex := range payload.Exclude {
			ex = strings.TrimSpace(ex)
			if ex != "" {
				b.WriteString(fmt.Sprintf("- `%s`\n", ex))
			}
		}
		b.WriteString("\n")
	}

	if payload.Notes != "" {
		b.WriteString(fmt.Sprintf("**Operator Rules of Engagement**:\n%s\n\n", strings.TrimSpace(payload.Notes)))
	}

	return b.String()
}

// IsTargetInScope tests whether a candidate host/IP is permitted by the project scope.
func IsTargetInScope(candidate string, scopeJSON string) (bool, string) {
	candidate = cleanHost(candidate)
	if candidate == "" {
		return false, "empty candidate host"
	}

	var p ScopePayload
	if err := json.Unmarshal([]byte(scopeJSON), &p); err != nil {
		// Try parsing as simple array of string
		var arr []string
		if err2 := json.Unmarshal([]byte(scopeJSON), &arr); err2 == nil {
			p.Targets = arr
		} else {
			return true, "" // Permissive if unparseable
		}
	}

	// First check explicit excludes
	for _, ex := range p.Exclude {
		if matchHost(candidate, ex) {
			return false, fmt.Sprintf("target matches exclude rule: %s", ex)
		}
	}

	// If no targets specified, scope is unconstrained
	if len(p.Targets) == 0 {
		return true, ""
	}

	// Check if matches any in-scope target
	for _, t := range p.Targets {
		if matchHost(candidate, t) {
			return true, ""
		}
	}

	return false, "target does not match any authorized in-scope pattern"
}

func cleanHost(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if strings.Contains(raw, "://") {
		if u, err := url.Parse(raw); err == nil {
			raw = u.Hostname()
		}
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}
	return strings.TrimPrefix(raw, "*.")
}

func matchHost(candidate, pattern string) bool {
	candidate = cleanHost(candidate)
	pattern = strings.TrimSpace(strings.ToLower(pattern))
	if pattern == "" || candidate == "" {
		return false
	}

	// Exact match
	if candidate == pattern {
		return true
	}

	// CIDR match
	if strings.Contains(pattern, "/") {
		_, ipNet, err := net.ParseCIDR(pattern)
		if err == nil {
			ip := net.ParseIP(candidate)
			if ip != nil && ipNet.Contains(ip) {
				return true
			}
		}
	}

	// Wildcard domain match (e.g., *.example.com or example.com)
	root := strings.TrimPrefix(pattern, "*.")
	if candidate == root || strings.HasSuffix(candidate, "."+root) {
		return true
	}

	return false
}
