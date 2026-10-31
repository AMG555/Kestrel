package report

import (
	"strings"
	"testing"

	"kestrel/internal/database"
)

func TestExtractPoCDetails_RawHTTP(t *testing.T) {
	vuln := &database.Vulnerability{
		ID:         "v-test-1",
		Title:      "SQL Injection in /api/users",
		Severity:   "high",
		Target:     "http://api.staging.internal:8000/api/users",
		ReproSteps: "Execute the following request:\nPOST /api/users HTTP/1.1\nHost: api.staging.internal\nContent-Type: application/json\nX-Custom-Auth: secret123\n\n{\"id\": \"1' OR '1'='1\"}",
		Evidence:   "HTTP/1.1 500 Internal Server Error\nsyntax error at or near 'OR'",
	}

	d := ExtractPoCDetails(vuln)
	if d.Method != "POST" {
		t.Errorf("expected Method 'POST', got %q", d.Method)
	}
	if d.Path != "/api/users" {
		t.Errorf("expected Path '/api/users', got %q", d.Path)
	}
	if d.Headers["Content-Type"] != "application/json" {
		t.Errorf("expected Content-Type header 'application/json', got %q", d.Headers["Content-Type"])
	}
	if d.Headers["X-Custom-Auth"] != "secret123" {
		t.Errorf("expected X-Custom-Auth header 'secret123', got %q", d.Headers["X-Custom-Auth"])
	}
	if !strings.Contains(d.Body, "1' OR '1'='1") {
		t.Errorf("expected Body to contain SQL payload, got %q", d.Body)
	}
	if d.ExpectedIndicator != "syntax error at or near 'OR'" {
		t.Errorf("expected indicator 'syntax error at or near 'OR'', got %q", d.ExpectedIndicator)
	}
}

func TestGeneratePoC_Python(t *testing.T) {
	vuln := &database.Vulnerability{
		ID:         "v-test-2",
		Title:      "Information Disclosure via .env",
		Severity:   "critical",
		Target:     "https://target.com/.env",
		ReproSteps: "GET /.env HTTP/1.1\nHost: target.com",
		Evidence:   "DB_PASSWORD=secret_production_pass",
	}

	pyScript, err := GeneratePoC(vuln, "python")
	if err != nil {
		t.Fatalf("unexpected error generating python PoC: %v", err)
	}

	if !strings.Contains(pyScript, "urllib.request.Request") {
		t.Errorf("expected python script to contain urllib.request.Request")
	}
	if !strings.Contains(pyScript, "DB_PASSWORD=secret_production_pass") {
		t.Errorf("expected python script to check for DB_PASSWORD indicator")
	}
	if !strings.Contains(pyScript, "https://target.com") {
		t.Errorf("expected python script to have target.com default")
	}
}

func TestGeneratePoC_Bash(t *testing.T) {
	vuln := &database.Vulnerability{
		ID:         "v-test-3",
		Title:      "Open Redirect",
		Severity:   "medium",
		Target:     "http://example.com/redirect?url=https://attacker.com",
		ReproSteps: "curl -s http://example.com/redirect?url=https://attacker.com",
		Evidence:   "Location: https://attacker.com",
	}

	shScript, err := GeneratePoC(vuln, "bash")
	if err != nil {
		t.Fatalf("unexpected error generating bash PoC: %v", err)
	}

	if !strings.Contains(shScript, "curl -s -i -k") {
		t.Errorf("expected bash script to contain curl command")
	}
	if !strings.Contains(shScript, "http://example.com") {
		t.Errorf("expected bash script to have example.com default target")
	}
}
