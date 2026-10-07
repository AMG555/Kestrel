package project

import (
	"fmt"
	"strings"

	"kestrel/internal/database"
)

// AppendSystemPromptBlock appends a markdown section to an existing system prompt cleanly.
func AppendSystemPromptBlock(base, block string) string {
	base = strings.TrimSpace(base)
	block = strings.TrimSpace(block)
	if block == "" {
		return base
	}
	if base == "" {
		return block
	}
	return base + "\n\n" + block
}

// BuildFactIndexBlock formats project blackboard facts into a compact index for system prompt injection.
func BuildFactIndexBlock(projectName, projectID string, facts []*database.ProjectFact) string {
	if len(facts) == 0 {
		return fmt.Sprintf("## Project Blackboard Index (Project: %s, ID: %s)\nNo operational facts recorded yet. Use `upsert_project_fact` to persist verified findings and recon context.\n", projectName, projectID)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Project Blackboard Index (Project: %s, ID: %s)\n", projectName, projectID))
	b.WriteString("The following facts are verified operational memory from previous runs. Retrieve complete details via `get_project_fact(fact_key)`:\n\n")

	for _, f := range facts {
		conf := f.Confidence
		if conf == "" {
			conf = "confirmed"
		}
		pin := ""
		if f.Pinned {
			pin = " [PINNED]"
		}
		summary := strings.TrimSpace(f.Summary)
		if len(summary) > 120 {
			summary = summary[:117] + "..."
		}
		b.WriteString(fmt.Sprintf("- **`%s`** (`%s` / %s)%s: %s\n", f.FactKey, f.Category, conf, pin, summary))
	}

	b.WriteString("\n*Rule: Never fabricate details; invoke `get_project_fact` when full reproduction steps, response payloads, or exploit paths are required.*\n")
	return b.String()
}

// FormatFactDetail returns markdown formatted details for a single fact.
func FormatFactDetail(f *database.ProjectFact) string {
	if f == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("### Fact: %s\n", f.FactKey))
	b.WriteString(fmt.Sprintf("- **Category**: %s\n", f.Category))
	b.WriteString(fmt.Sprintf("- **Confidence**: %s\n", f.Confidence))
	b.WriteString(fmt.Sprintf("- **Updated**: %s\n\n", f.UpdatedAt.Format("2006-01-02 15:04:05 UTC")))
	b.WriteString(fmt.Sprintf("**Summary**:\n%s\n\n", f.Summary))

	if strings.TrimSpace(f.Body) != "" {
		b.WriteString(fmt.Sprintf("**Details / Reproduction**:\n%s\n", strings.TrimSpace(f.Body)))
	}
	return b.String()
}
