package project

import (
	"strings"
	"testing"
	"time"

	"kestrel/internal/database"
)

func TestBuildFactIndexBlock(t *testing.T) {
	facts := []*database.ProjectFact{
		{
			FactKey:    "target/main-api",
			Category:   FactCategoryTarget,
			Summary:    "Main REST API service discovered on port 8443",
			Confidence: "confirmed",
			Pinned:     true,
			UpdatedAt:  time.Now(),
		},
		{
			FactKey:    "finding/jwt-secret",
			Category:   FactCategoryFinding,
			Summary:    "Hardcoded JWT HMAC secret key exposed in swagger.json",
			Confidence: "confirmed",
			UpdatedAt:  time.Now(),
		},
	}

	index := BuildFactIndexBlock("Acme Product", "proj_456", facts)
	if !strings.Contains(index, "target/main-api") {
		t.Errorf("expected target fact key in index")
	}
	if !strings.Contains(index, "[PINNED]") {
		t.Errorf("expected PINNED indicator in index")
	}
	if !strings.Contains(index, "finding/jwt-secret") {
		t.Errorf("expected finding fact key in index")
	}
}

func TestGraphNodeType(t *testing.T) {
	if GraphNodeType(FactCategoryTarget, "target/web") != "target" {
		t.Errorf("expected target node type")
	}
	if GraphNodeType(FactCategoryFinding, "finding/sqli") != "finding" {
		t.Errorf("expected finding node type")
	}
	if GraphNodeType("", "vuln:CVE-2024-1234") != "vulnerability" {
		t.Errorf("expected vulnerability node type")
	}
}

func TestFormatMermaidGraph(t *testing.T) {
	nodes := []GraphNode{
		{Key: "target/api", Type: "target", Category: FactCategoryTarget},
		{Key: "vuln:sql-injection", Type: "vulnerability", Category: "vuln"},
	}
	edges := []FactEdge{
		{Source: "target/api", Target: "vuln:sql-injection", Relation: "vulnerable_to"},
	}

	mermaid := FormatMermaidGraph(nodes, edges)
	if !strings.Contains(mermaid, "graph LR") {
		t.Errorf("expected graph LR header")
	}
	if !strings.Contains(mermaid, "vulnerable_to") {
		t.Errorf("expected relation in mermaid")
	}
}
