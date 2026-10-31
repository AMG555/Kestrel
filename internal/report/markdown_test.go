package report

import (
	"strings"
	"testing"
	"time"

	"kestrel/internal/database"
	"kestrel/internal/mcp"
)

func TestRenderMarkdownReport_StructureAndEvidence(t *testing.T) {
	proj := &database.Project{
		ID:   "proj-finance-qa",
		Name: "Financial Gateway Pentest",
	}

	vulns := []*database.Vulnerability{
		{
			ID:             "v-001",
			Title:          "BOLA IDOR in Transfer Endpoint",
			Severity:       "high",
			Status:         "confirmed",
			Type:           "CWE-639",
			Target:         "https://pay.internal/api/v1/transfer/9921",
			Description:    "Arbitrary account balance drain via insecure direct object reference.",
			Preconditions:  "Authenticated regular user account.",
			ReproSteps:     "1. Send POST request with victim account id.\n2. Observe 200 OK transfer success.",
			Evidence:       "HTTP/1.1 200 OK\r\n{\"transfer_id\":\"tx-999\",\"status\":\"executed\"}",
			Recommendation: "Enforce authorization check validating authenticated user against target account.",
		},
	}

	execs := []*mcp.ToolExecution{
		{
			ID:        "exec-test-1",
			ToolName:  "curl_probe",
			Status:    "completed",
			StartTime: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC),
			Duration:  150 * time.Millisecond,
			Arguments: map[string]interface{}{"url": "https://pay.internal/api/v1/transfer/9921", "method": "POST"},
			Result: &mcp.ToolResult{
				Content: []mcp.Content{
					{Type: "text", Text: "HTTP/1.1 200 OK\n{\"transfer_id\":\"tx-999\",\"status\":\"executed\"}"},
				},
			},
		},
	}

	reportMD := RenderMarkdownReport(MarkdownReportOptions{
		Project:        proj,
		Vulns:          vulns,
		ToolExecutions: execs,
		GeneratedAt:    time.Date(2026, 11, 1, 12, 5, 0, 0, time.UTC),
	})

	if !strings.Contains(reportMD, "# Security Assessment Report — Financial Gateway Pentest") {
		t.Errorf("missing report title header")
	}
	if !strings.Contains(reportMD, "BOLA IDOR in Transfer Endpoint") {
		t.Errorf("missing vulnerability title")
	}
	if !strings.Contains(reportMD, "| 🟠 High | 1 |") {
		t.Errorf("missing high severity count in table")
	}
	if !strings.Contains(reportMD, "Arbitrary account balance drain via insecure direct object reference.") {
		t.Errorf("missing vulnerability description")
	}
	if !strings.Contains(reportMD, "HTTP/1.1 200 OK\r\n{\"transfer_id\":\"tx-999\",\"status\":\"executed\"}") {
		t.Errorf("missing verbatim vulnerability evidence")
	}
	if !strings.Contains(reportMD, "## Tool Execution & Verbatim Forensic Log") {
		t.Errorf("missing tool execution forensic log section")
	}
	if !strings.Contains(reportMD, "Tool: `curl_probe` [COMPLETED]") {
		t.Errorf("missing tool execution entry")
	}
	if !strings.Contains(reportMD, "exec-test-1") {
		t.Errorf("missing execution ID")
	}
	if !strings.Contains(reportMD, "https://pay.internal/api/v1/transfer/9921") {
		t.Errorf("missing argument details")
	}
	if !strings.Contains(reportMD, "Self-hosted pentest workbench for small teams") {
		t.Errorf("missing workbench positioning footer")
	}
}

func TestRenderMarkdownReport_EmptyVulns(t *testing.T) {
	reportMD := RenderMarkdownReport(MarkdownReportOptions{
		Project:     &database.Project{Name: "Clean Project"},
		GeneratedAt: time.Now().UTC(),
	})

	if !strings.Contains(reportMD, "No confirmed vulnerabilities were identified during this assessment cycle.") {
		t.Errorf("expected clean assessment note")
	}
}
