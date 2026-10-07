package project

import (
	"encoding/json"
	"fmt"
	"strings"

	"kestrel/internal/config"
	"kestrel/internal/database"
)

// projectScopePayload parses the projects.scope_json (agreed fields, extensible).
type projectScopePayload struct {
	Targets []string `json:"targets"`
	Exclude []string `json:"exclude"`
	Notes   string   `json:"notes"`
}

// BuildScopeBlock formats the project scope_json into an authorisation scope block readable by the Agent.
func BuildScopeBlock(proj *database.Project) string {
	if proj == nil {
		return ""
	}
	raw := strings.TrimSpace(proj.ScopeJSON)
	if raw == "" {
		return ""
	}

	var payload projectScopePayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return fmt.Sprintf("## Project Test Scope (project: %s)\n(scope_json is not valid JSON; please manually verify the configuration)\n```\n%s\n```\n"+
			"Only test explicitly authorised targets; stop and explain if out of scope.\n", proj.Name, truncateRunes(raw, 800))
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Project Test Scope (project: %s, id: %s)\n", proj.Name, proj.ID))
	b.WriteString("The following are the authorisation boundaries — **must be followed**: only test the listed targets, avoid excludes, do not expand the scope without permission.\n")

	if len(payload.Targets) > 0 {
		b.WriteString("\n**Allowed targets**:\n")
		for _, t := range payload.Targets {
			t = strings.TrimSpace(t)
			if t != "" {
				b.WriteString("- " + t + "\n")
			}
		}
	}
	if len(payload.Exclude) > 0 {
		b.WriteString("\n**Explicitly excluded**:\n")
		for _, t := range payload.Exclude {
			t = strings.TrimSpace(t)
			if t != "" {
				b.WriteString("- " + t + "\n")
			}
		}
	}
	if n := strings.TrimSpace(payload.Notes); n != "" {
		b.WriteString("\n**Notes**:\n" + n + "\n")
	}
	if len(payload.Targets) == 0 && len(payload.Exclude) == 0 && strings.TrimSpace(payload.Notes) == "" {
		b.WriteString("\n(scope_json is configured but no targets/exclude/notes fields were recognised; raw content shown for reference)\n```json\n")
		b.WriteString(truncateRunes(raw, 1200))
		b.WriteString("\n```\n")
	}
	b.WriteString("\nIf a target is not in targets or matches an exclude, do not actively scan/exploit it; explicit authorisation expansion from the user is required before continuing.\n")
	return b.String()
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// BuildProjectBlackboardBlock combines the test scope block and the fact blackboard index.
func BuildProjectBlackboardBlock(db *database.DB, projectID string, cfg config.ProjectConfig) (string, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return "", nil
	}
	proj, err := db.GetProject(projectID)
	if err != nil {
		return "", err
	}
	parts := []string{}
	if scope := strings.TrimSpace(BuildScopeBlock(proj)); scope != "" {
		parts = append(parts, scope)
	}
	index, err := BuildFactIndexBlock(db, projectID, cfg)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(index) != "" {
		parts = append(parts, index)
	}
	return strings.Join(parts, "\n\n"), nil
}
