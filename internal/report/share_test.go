package report

import (
	"strings"
	"testing"
	"time"

	"kestrel/internal/database"
	"kestrel/internal/mcp"
)

func TestCreateAndVerifyShareToken(t *testing.T) {
	secret := []byte("share-signing-secret-key-32bytes")
	projectID := "proj-12345"
	expiresAt := time.Now().Add(24 * time.Hour)

	token, tokenHash, err := CreateShareToken(projectID, expiresAt, secret)
	if err != nil {
		t.Fatalf("CreateShareToken: %v", err)
	}
	if token == "" || tokenHash == "" {
		t.Fatal("expected non-empty token and hash")
	}

	gotProjectID, gotExpiry, err := VerifyShareToken(token, secret)
	if err != nil {
		t.Fatalf("VerifyShareToken: %v", err)
	}
	if gotProjectID != projectID {
		t.Errorf("project ID mismatch: got %q, want %q", gotProjectID, projectID)
	}
	if gotExpiry.Unix() != expiresAt.Unix() {
		t.Errorf("expiry mismatch: got %v, want %v", gotExpiry, expiresAt)
	}
}

func TestVerifyShareToken_Tampered(t *testing.T) {
	secret := []byte("share-signing-secret-key-32bytes")
	wrongSecret := []byte("wrong-signing-secret-key-32bytes")
	projectID := "proj-12345"
	expiresAt := time.Now().Add(24 * time.Hour)

	token, _, err := CreateShareToken(projectID, expiresAt, secret)
	if err != nil {
		t.Fatalf("CreateShareToken: %v", err)
	}

	_, _, err = VerifyShareToken(token, wrongSecret)
	if err != ErrShareTokenInvalid {
		t.Fatalf("expected ErrShareTokenInvalid, got %v", err)
	}
}

func TestVerifyShareToken_Expired(t *testing.T) {
	secret := []byte("share-signing-secret-key-32bytes")
	projectID := "proj-12345"
	expiresAt := time.Now().Add(-10 * time.Minute)

	token, _, err := CreateShareToken(projectID, expiresAt, secret)
	if err != nil {
		t.Fatalf("CreateShareToken: %v", err)
	}

	_, _, err = VerifyShareToken(token, secret)
	if err != ErrShareTokenExpired {
		t.Fatalf("expected ErrShareTokenExpired, got %v", err)
	}
}

func TestRenderHTMLReport_OpenGraphMeta(t *testing.T) {
	project := &database.Project{
		ID:   "p-1",
		Name: "ACME Production Web",
	}
	vulns := []*database.Vulnerability{
		{
			ID:          "v-1",
			Title:       "SQL Injection in Customer Portal",
			Severity:    "critical",
			Target:      "https://acme.test/api/login",
			Description: "Vulnerable to auth bypass",
			Evidence:    "UNION SELECT 1,2,3",
		},
	}

	htmlBytes := RenderHTMLReport(HTMLReportOptions{
		Project:    project,
		Vulns:      vulns,
		ShareToken: "tok-abc",
	})
	content := string(htmlBytes)

	if !strings.Contains(content, `<meta property="og:title" content="Security Assessment: ACME Production Web">`) {
		t.Errorf("missing og:title in report html")
	}
	if !strings.Contains(content, "1 Critical, 0 High, 0 Medium findings") {
		t.Errorf("missing severity counts in report html")
	}
	if !strings.Contains(content, "SQL Injection in Customer Portal") {
		t.Errorf("missing vulnerability title in report html")
	}
	if !strings.Contains(content, "Self-hosted pentest workbench for small teams") {
		t.Errorf("missing updated positioning CTA copy in report html")
	}
}

func TestRenderHTMLReport_ToolExecutions(t *testing.T) {
	project := &database.Project{
		ID:   "p-1",
		Name: "ACME Production Web",
	}
	execs := []*mcp.ToolExecution{
		{
			ID:        "exec-101",
			ToolName:  "nmap_scan",
			Status:    "completed",
			Arguments: map[string]interface{}{"target": "192.168.1.1", "ports": "80,443"},
			StartTime: time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC),
			Duration:  2500 * time.Millisecond,
			Result: &mcp.ToolResult{
				Content: []mcp.Content{
					{Type: "text", Text: "80/tcp open http\n443/tcp open https"},
				},
			},
		},
	}

	htmlBytes := RenderHTMLReport(HTMLReportOptions{
		Project:        project,
		ToolExecutions: execs,
		ShareToken:     "tok-execs",
	})
	content := string(htmlBytes)

	if !strings.Contains(content, "Tool Execution &amp; Verbatim Evidence Log") && !strings.Contains(content, "Tool Execution & Verbatim Evidence Log") {
		t.Errorf("expected tool execution log section in report html")
	}
	if !strings.Contains(content, "nmap_scan") {
		t.Errorf("expected tool name nmap_scan in report html")
	}
	if !strings.Contains(content, "80/tcp open http") {
		t.Errorf("expected verbatim stdout in report html")
	}
	if !strings.Contains(content, "exec-101") {
		t.Errorf("expected execution ID in report html")
	}
}

func TestRenderHTMLReport_PassphraseChallenge(t *testing.T) {
	htmlBytes := RenderHTMLReport(HTMLReportOptions{
		Project:            &database.Project{Name: "Classified Project"},
		ShareToken:         "tok-secret",
		PassphraseRequired: true,
	})
	content := string(htmlBytes)

	if !strings.Contains(content, "Passphrase Protected") {
		t.Errorf("expected passphrase challenge")
	}
	if !strings.Contains(content, `action="/r/tok-secret/unlock"`) {
		t.Errorf("expected unlock form action")
	}
}
