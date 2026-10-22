package report

import (
	"encoding/json"
	"testing"
	"time"

	"kestrel/internal/database"
)

func TestGenerateSARIF_Empty(t *testing.T) {
	report := GenerateSARIF([]*database.Vulnerability{}, "1.0.0")
	if report == nil {
		t.Fatal("expected non-nil SARIF report")
	}
	if report.Version != "2.1.0" {
		t.Errorf("expected version 2.1.0, got %s", report.Version)
	}
	if len(report.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(report.Runs))
	}
	run := report.Runs[0]
	if run.Tool.Driver.Name != "Kestrel" {
		t.Errorf("expected tool name Kestrel, got %s", run.Tool.Driver.Name)
	}
	if len(run.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(run.Results))
	}
	if len(run.Tool.Driver.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(run.Tool.Driver.Rules))
	}
}

func TestGenerateSARIF_WithFindings(t *testing.T) {
	vulns := []*database.Vulnerability{
		{
			ID:             "vuln-1",
			Title:          "SQL Injection in Login Handler",
			Description:    "User input concatenated directly into SQL statement",
			Severity:       "critical",
			Status:         "confirmed",
			Type:           "CWE-89",
			Target:         "internal/handler/auth.go:120:5",
			Evidence:       "' OR '1'='1' returns 200 OK with admin session",
			Recommendation: "Use parameterized queries or prepared statements",
			CreatedAt:      time.Now(),
		},
		{
			ID:             "vuln-2",
			Title:          "Second SQL Injection Finding",
			Description:    "Another unescaped parameter in search handler",
			Severity:       "high",
			Status:         "open",
			Type:           "CWE-89", // Same rule ID, should deduplicate rule definition
			Target:         "internal/handler/search.go:45",
			Evidence:       "UNION SELECT 1,2,3 returns schema",
			Recommendation: "Use parameterized queries",
			CreatedAt:      time.Now(),
		},
		{
			ID:             "vuln-3",
			Title:          "Information Disclosure on Webhook",
			Description:    "Debug stack traces returned in HTTP 500 response",
			Severity:       "medium",
			Status:         "confirmed",
			Type:           "CWE-209",
			Target:         "https://api.target.com/webhooks/receive",
			Evidence:       "Stack trace prints internal file paths and Go version",
			Recommendation: "Disable debug error pages in production",
			CreatedAt:      time.Now(),
		},
	}

	report := GenerateSARIF(vulns, "2.0.0")
	if len(report.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(report.Runs))
	}
	run := report.Runs[0]

	// Check deduplicated rules: CWE-89 and CWE-209
	if len(run.Tool.Driver.Rules) != 2 {
		t.Fatalf("expected 2 unique rules, got %d", len(run.Tool.Driver.Rules))
	}
	if run.Tool.Driver.Rules[0].ID != "CWE-89" {
		t.Errorf("expected first rule CWE-89, got %s", run.Tool.Driver.Rules[0].ID)
	}
	if run.Tool.Driver.Rules[0].Properties["security-severity"] != "9.0" {
		t.Errorf("expected security-severity 9.0, got %v", run.Tool.Driver.Rules[0].Properties["security-severity"])
	}

	// Check results
	if len(run.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(run.Results))
	}

	// Check result 1 location
	res1 := run.Results[0]
	if res1.RuleID != "CWE-89" || res1.RuleIndex != 0 {
		t.Errorf("res1 rule mismatch: id=%s index=%d", res1.RuleID, res1.RuleIndex)
	}
	if res1.Level != "error" {
		t.Errorf("expected level error for critical, got %s", res1.Level)
	}
	if len(res1.Locations) != 1 || res1.Locations[0].PhysicalLocation == nil {
		t.Fatalf("expected physical location for res1")
	}
	physLoc := res1.Locations[0].PhysicalLocation
	if physLoc.ArtifactLocation.URI != "internal/handler/auth.go" {
		t.Errorf("expected URI internal/handler/auth.go, got %s", physLoc.ArtifactLocation.URI)
	}
	if physLoc.Region == nil || physLoc.Region.StartLine != 120 || physLoc.Region.StartColumn != 5 {
		t.Errorf("unexpected region for res1: %+v", physLoc.Region)
	}

	// Check result 3 (URL target)
	res3 := run.Results[2]
	if res3.Level != "warning" {
		t.Errorf("expected level warning for medium, got %s", res3.Level)
	}
	if len(res3.Locations[0].LogicalLocations) != 1 {
		t.Fatalf("expected 1 logical location for URL target")
	}
	if res3.Locations[0].LogicalLocations[0].Name != "/webhooks/receive" {
		t.Errorf("expected logical name /webhooks/receive, got %s", res3.Locations[0].LogicalLocations[0].Name)
	}
}

func TestParseTargetLocation(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantURI    string
		wantLine   int
		wantCol    int
		isEndpoint bool
	}{
		{
			name:     "file with line and col",
			input:    "pkg/auth/token.go:10:4",
			wantURI:  "pkg/auth/token.go",
			wantLine: 10,
			wantCol:  4,
		},
		{
			name:     "file with line only",
			input:    "cmd/main.go:88",
			wantURI:  "cmd/main.go",
			wantLine: 88,
			wantCol:  0,
		},
		{
			name:     "windows backslash path",
			input:    "internal\\config\\settings.go:30",
			wantURI:  "internal/config/settings.go",
			wantLine: 30,
			wantCol:  0,
		},
		{
			name:       "web url",
			input:      "https://example.com/v1/users",
			wantURI:    "https://example.com/v1/users",
			isEndpoint: true,
		},
		{
			name:    "empty fallback",
			input:   "   ",
			wantURI: "unknown-target",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := ParseTargetLocation(tt.input)
			if loc.PhysicalLocation == nil {
				t.Fatalf("expected non-nil PhysicalLocation")
			}
			if loc.PhysicalLocation.ArtifactLocation.URI != tt.wantURI {
				t.Errorf("expected URI %s, got %s", tt.wantURI, loc.PhysicalLocation.ArtifactLocation.URI)
			}
			if tt.wantLine > 0 {
				if loc.PhysicalLocation.Region == nil || loc.PhysicalLocation.Region.StartLine != tt.wantLine {
					t.Errorf("expected StartLine %d, got %+v", tt.wantLine, loc.PhysicalLocation.Region)
				}
				if tt.wantCol > 0 && loc.PhysicalLocation.Region.StartColumn != tt.wantCol {
					t.Errorf("expected StartColumn %d, got %d", tt.wantCol, loc.PhysicalLocation.Region.StartColumn)
				}
			}
			if tt.isEndpoint && len(loc.LogicalLocations) == 0 {
				t.Errorf("expected LogicalLocation for endpoint")
			}
		})
	}
}

func TestToJSON_ValidSchema(t *testing.T) {
	vulns := []*database.Vulnerability{
		{
			ID:          "v-1",
			Title:       "XSS Vulnerability",
			Severity:    "low",
			Type:        "CWE-79",
			Target:      "web/views/index.html:15",
			Description: "Unsanitized user template variable",
		},
	}

	raw, err := ToJSON(vulns, "1.0.0", true)
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if parsed["$schema"] != SARIFSchemaURL {
		t.Errorf("expected $schema %s, got %v", SARIFSchemaURL, parsed["$schema"])
	}
	if parsed["version"] != "2.1.0" {
		t.Errorf("expected version 2.1.0, got %v", parsed["version"])
	}
}
