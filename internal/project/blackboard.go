package project

import (
	"fmt"
	"strings"

	"kestrel/internal/config"
	"kestrel/internal/database"
)

// AppendSystemPromptBlock appends a block to the system prompt.
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

const (
	factIndexFooterGetDetail = "When you need the full content (attack chain, POC, request/response, etc.) you must call get_project_fact(fact_key) — never fabricate details from the summary alone."
	factIndexFooterWriteHint = "When writing fact links, use from (source fact_key → current fact), e.g. for a finding: {from:target/*, type:discovered_on}; body should contain the full reproducible flow (for discovery/exploitation fact_keys, recommended prefixes: finding|chain|exploit|poc/)."
	factIndexFooterEmpty     = "To write facts use upsert_project_fact; for details call get_project_fact(fact_key)."
)

// BuildFactIndexBlock generates the project blackboard index for the Agent system prompt (key + summary + relationship edges + attack path, no body).
func BuildFactIndexBlock(db *database.DB, projectID string, cfg config.ProjectConfig) (string, error) {
	if db == nil || !cfg.Enabled {
		return "", nil
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return "", nil
	}

	proj, err := db.GetProject(projectID)
	if err != nil {
		return "", err
	}

	facts, err := db.ListProjectFactsForIndex(projectID, cfg.DefaultInjectDeprecated)
	if err != nil {
		return "", err
	}
	allEdges, _ := db.ListProjectFactEdgesByProject(projectID)
	_, incomingByTarget := indexEdgeGroupMaps(allEdges)

	if len(facts) == 0 {
		return wrapFactIndexBlock(fmt.Sprintf("## Project Blackboard Index (project: %s, id: %s)\n(No facts yet)\n%s", proj.Name, proj.ID, factIndexFooterEmpty)), nil
	}

	sortFactsForIndex(facts)

	maxRunes := cfg.FactIndexMaxRunesEffective()
	pathMaxRunes := cfg.FactIndexPathMaxRunesEffective()
	footer := factIndexFooterGetDetail + "\n" + factIndexFooterWriteHint
	footerRunes := len([]rune(footer))
	factsBudget := maxRunes - pathMaxRunes - footerRunes
	if factsBudget < 800 {
		factsBudget = maxRunes - footerRunes
		pathMaxRunes = 0
	}

	indexedKeys := make(map[string]struct{}, len(facts))
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Project Blackboard Index (project: %s, id: %s)\n", proj.Name, proj.ID))
	used := len([]rune(b.String()))
	omitted := 0

	for _, f := range facts {
		indexedKeys[f.FactKey] = struct{}{}
		line := fmt.Sprintf("- [%s] %s — %s (%s)", f.FactKey, f.Category, strings.TrimSpace(f.Summary), f.Confidence)
		line += FormatFactIndexLinksHint(f.FactKey, incomingByTarget[f.FactKey])
		line += "\n"
		lineRunes := len([]rune(line))
		if used+lineRunes > factsBudget {
			omitted++
			continue
		}
		b.WriteString(line)
		used += lineRunes
	}

	if omitted > 0 {
		b.WriteString(fmt.Sprintf("\n(%d more facts not listed in index; use list_project_facts or search_project_facts to query.)\n", omitted))
	}

	if pathSection := BuildFactPathOverviewSection(allEdges, indexedKeys, pathMaxRunes); pathSection != "" {
		b.WriteString("\n")
		b.WriteString(pathSection)
	}

	b.WriteString(footer)
	return wrapFactIndexBlock(b.String()), nil
}
