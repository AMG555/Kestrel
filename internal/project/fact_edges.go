package project

import (
	"fmt"
	"strings"
)

// Fact category constants.
const (
	FactCategoryTarget   = "target"
	FactCategoryAuth     = "auth"
	FactCategoryInfra    = "infra"
	FactCategoryFinding  = "finding"
	FactCategoryChain    = "chain"
	FactCategoryExploit  = "exploit"
	FactCategoryPOC      = "poc"
	FactCategoryNote     = "note"
)

// GraphNode represents a node in the project knowledge graph.
type GraphNode struct {
	Key      string `json:"key"`
	Category string `json:"category"`
	Type     string `json:"type"`
	Summary  string `json:"summary"`
}

// FactEdge represents a directed relationship between two project facts.
type FactEdge struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Relation string `json:"relation"` // discovered_on | vulnerable_to | leads_to | depends_on | enables
}

// GraphNodeType maps fact category or key prefix to graph node type.
func GraphNodeType(category, factKey string) string {
	key := strings.ToLower(strings.TrimSpace(factKey))
	if strings.HasPrefix(key, "vuln:") {
		return "vulnerability"
	}
	c := strings.ToLower(strings.TrimSpace(category))
	if c != "" {
		switch c {
		case FactCategoryTarget:
			return "target"
		case FactCategoryExploit:
			return "exploit"
		case FactCategoryPOC:
			return "poc"
		case FactCategoryChain:
			return "chain"
		case FactCategoryFinding:
			return "finding"
		case "vuln", "vulnerability":
			return "vulnerability"
		case FactCategoryAuth:
			return "auth"
		case FactCategoryInfra:
			return "infra"
		case FactCategoryNote:
			return "note"
		default:
			return c
		}
	}
	switch {
	case strings.HasPrefix(key, "target/"):
		return "target"
	case strings.HasPrefix(key, "exploit/"), strings.HasPrefix(key, "evidence/"):
		return "exploit"
	case strings.HasPrefix(key, "poc/"):
		return "poc"
	case strings.HasPrefix(key, "chain/"):
		return "chain"
	case strings.HasPrefix(key, "finding/"):
		return "finding"
	default:
		return "note"
	}
}

// FormatMermaidGraph generates a clean Mermaid diagram representing the attack chain/facts graph.
func FormatMermaidGraph(nodes []GraphNode, edges []FactEdge) string {
	var b strings.Builder
	b.WriteString("graph LR\n")

	nodeMap := make(map[string]string)
	for i, n := range nodes {
		id := fmt.Sprintf("N%d", i)
		nodeMap[n.Key] = id
		label := strings.ReplaceAll(n.Key, "\"", "'")
		shapeOpen := "["
		shapeClose := "]"
		switch n.Type {
		case "vulnerability":
			shapeOpen = "{{"
			shapeClose = "}}"
		case "target":
			shapeOpen = "(("
			shapeClose = "))"
		case "exploit", "poc":
			shapeOpen = ">"
			shapeClose = "]"
		}
		b.WriteString(fmt.Sprintf("    %s%s\"%s\"%s\n", id, shapeOpen, label, shapeClose))
	}

	for _, e := range edges {
		srcID, ok1 := nodeMap[e.Source]
		tgtID, ok2 := nodeMap[e.Target]
		if ok1 && ok2 {
			rel := e.Relation
			if rel != "" {
				b.WriteString(fmt.Sprintf("    %s -->|\"%s\"| %s\n", srcID, rel, tgtID))
			} else {
				b.WriteString(fmt.Sprintf("    %s --> %s\n", srcID, tgtID))
			}
		}
	}

	return b.String()
}
