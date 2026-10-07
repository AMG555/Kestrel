package project

import (
	"fmt"
	"regexp"
	"strings"

	"kestrel/internal/database"
)

var (
	bodyDepFactLine   = regexp.MustCompile(`(?im)^[\s\-*]*(?:dependent fact|depends on)\s*[:]\s*([a-zA-Z0-9][a-zA-Z0-9._/-]*)`)
	bodyRelFactLine   = regexp.MustCompile(`(?im)^[\s\-*]*related\s*fact_key\s*[:]\s*([a-zA-Z0-9][a-zA-Z0-9._/-]*)`)
	bodyAssocSection  = regexp.MustCompile(`(?im)^##\s*(?:associations?|related)\s*$`)
	bodySyncLinksHead = "structured relationship edges (auto-synced)"
)

// ParseLinksFromBody parses from-semantics relationship edges from the "Associations" section of a fact body (fallback when no explicit links are provided).
func ParseLinksFromBody(body string) []database.ProjectFactEdgeFromInput {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []database.ProjectFactEdgeFromInput
	add := func(key, edgeType string) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		if err := database.ValidateFactKey(key); err != nil {
			return
		}
		sig := edgeType + "\x00" + key
		if _, ok := seen[sig]; ok {
			return
		}
		seen[sig] = struct{}{}
		out = append(out, database.ProjectFactEdgeFromInput{From: key, Type: edgeType})
	}
	for _, m := range bodyDepFactLine.FindAllStringSubmatch(body, -1) {
		if len(m) > 1 {
			add(m[1], "depends_on")
		}
	}
	for _, m := range bodyRelFactLine.FindAllStringSubmatch(body, -1) {
		if len(m) > 1 {
			add(m[1], "supports")
		}
	}
	// Auto-sync block: type: key
	syncBlock := extractBodySyncLinksBlock(body)
	for _, line := range strings.Split(syncBlock, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
		if line == "" {
			continue
		}
		edgeType, source, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		edgeType = strings.TrimSpace(edgeType)
		source = strings.TrimSpace(source)
		if err := database.ValidateProjectFactEdgeType(edgeType); err != nil {
			continue
		}
		add(source, edgeType)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func extractBodySyncLinksBlock(body string) string {
	lines := strings.Split(body, "\n")
	var b strings.Builder
	inAssoc := false
	inSync := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if bodyAssocSection.MatchString(trim) {
			inAssoc = true
			inSync = false
			continue
		}
		if inAssoc && strings.HasPrefix(trim, "## ") && !bodyAssocSection.MatchString(trim) {
			break
		}
		if inAssoc && strings.Contains(trim, bodySyncLinksHead) {
			inSync = true
			continue
		}
		if inSync {
			if trim == "" || strings.HasPrefix(trim, "-") || strings.Contains(trim, ":") {
				if strings.HasPrefix(trim, "-") || (strings.Contains(trim, ":") && !strings.Contains(trim, "related_vulnerability")) {
					b.WriteString(trim)
					b.WriteByte('\n')
				}
			} else if strings.HasPrefix(trim, "##") {
				break
			}
		}
	}
	return b.String()
}

// SyncBodyLinksSection mirrors incoming edges into the "Associations" section of the fact body (human-readable; structural source of truth is the links).
func SyncBodyLinksSection(body string, edges []*database.ProjectFactEdge) string {
	body = strings.TrimSpace(body)
	block := formatBodySyncLinksBlock(edges)
	if block == "" {
		return body
	}
	if body == "" {
		return "## Associations\n" + block
	}
	lines := strings.Split(body, "\n")
	var out []string
	inAssoc := false
	replaced := false
	for i := 0; i < len(lines); i++ {
		trim := strings.TrimSpace(lines[i])
		if bodyAssocSection.MatchString(trim) {
			inAssoc = true
			out = append(out, lines[i])
			// Skip old sync block
			j := i + 1
			for j < len(lines) {
				t := strings.TrimSpace(lines[j])
				if strings.HasPrefix(t, "## ") {
					break
				}
				if strings.Contains(t, bodySyncLinksHead) {
					for j < len(lines) {
						t2 := strings.TrimSpace(lines[j])
						if t2 != "" && !strings.HasPrefix(t2, "-") && !strings.Contains(t2, ":") && !strings.Contains(t2, bodySyncLinksHead) {
							if strings.HasPrefix(t2, "##") {
								break
							}
						}
						j++
						if j < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[j]), "## ") {
							break
						}
						if j >= len(lines) {
							break
						}
						if j > i+1 && strings.TrimSpace(lines[j-1]) == "" && strings.HasPrefix(strings.TrimSpace(lines[j]), "## ") {
							break
						}
					}
					break
				}
				j++
			}
			out = append(out, block)
			i = j - 1
			replaced = true
			continue
		}
		out = append(out, lines[i])
	}
	if !replaced {
		if !inAssoc {
			out = append(out, "", "## Associations", block)
		} else {
			out = append(out, block)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func formatBodySyncLinksBlock(edges []*database.ProjectFactEdge) string {
	if len(edges) == 0 {
		return fmt.Sprintf("- %s:\n  (none)", bodySyncLinksHead)
	}
	var b strings.Builder
	b.WriteString("- ")
	b.WriteString(bodySyncLinksHead)
	b.WriteString(":\n")
	for _, e := range edges {
		b.WriteString(fmt.Sprintf("  - %s: %s\n", e.EdgeType, e.SourceFactKey))
	}
	return strings.TrimRight(b.String(), "\n")
}

// ResolveFactLinksForUpsert merges explicit links, links_text, and body-parsed results.
func ResolveFactLinksForUpsert(explicit []database.ProjectFactEdgeFromInput, linksText *string, body string, explicitSet bool) ([]database.ProjectFactEdgeFromInput, bool, error) {
	if explicitSet {
		if len(explicit) > 0 {
			return explicit, true, nil
		}
		if linksText != nil {
			parsed, err := ParseFactLinksText(*linksText)
			if err != nil {
				return nil, true, err
			}
			if parsed == nil {
				return []database.ProjectFactEdgeFromInput{}, true, nil
			}
			return parsed, true, nil
		}
		return []database.ProjectFactEdgeFromInput{}, true, nil
	}
	if parsed := ParseLinksFromBody(body); len(parsed) > 0 {
		return parsed, true, nil
	}
	return nil, false, nil
}

// MergeLinkFromInputsUnique merges multiple groups of from-incoming edge inputs and deduplicates.
func MergeLinkFromInputsUnique(groups ...[]database.ProjectFactEdgeFromInput) []database.ProjectFactEdgeFromInput {
	seen := map[string]struct{}{}
	var out []database.ProjectFactEdgeFromInput
	for _, g := range groups {
		for _, in := range g {
			sig := in.Type + "\x00" + in.From
			if _, ok := seen[sig]; ok {
				continue
			}
			if err := database.ValidateProjectFactEdgeType(in.Type); err != nil {
				continue
			}
			if err := database.ValidateFactKey(in.From); err != nil {
				continue
			}
			seen[sig] = struct{}{}
			out = append(out, in)
		}
	}
	return out
}

// MergeLinkInputsUnique merges multiple groups of link inputs and deduplicates (for internal outgoing edge writes).
func MergeLinkInputsUnique(groups ...[]database.ProjectFactEdgeInput) []database.ProjectFactEdgeInput {
	seen := map[string]struct{}{}
	var out []database.ProjectFactEdgeInput
	for _, g := range groups {
		for _, in := range g {
			sig := in.Type + "\x00" + in.To
			if _, ok := seen[sig]; ok {
				continue
			}
			if err := database.ValidateProjectFactEdgeType(in.Type); err != nil {
				continue
			}
			if err := database.ValidateFactKey(in.To); err != nil {
				continue
			}
			seen[sig] = struct{}{}
			out = append(out, in)
		}
	}
	return out
}
