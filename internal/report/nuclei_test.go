package report

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"kestrel/internal/database"
)

func TestGenerateNucleiTemplate(t *testing.T) {
	vuln := &database.Vulnerability{
		ID:         "v-sql-100",
		Title:      "Blind SQL Injection in Account Lookup",
		Severity:   "high",
		Type:       "sqli",
		Target:     "http://test.corp.local:8080/api/lookup?user=admin",
		ReproSteps: "POST /api/lookup HTTP/1.1\nHost: test.corp.local\nContent-Type: application/json\n\n{\"query\": \"admin' OR 1=1--\"}",
		Evidence:   "SQL syntax error near admin",
	}

	yamlStr, err := GenerateNucleiTemplate(vuln)
	if err != nil {
		t.Fatalf("failed to generate nuclei template: %v", err)
	}

	if !strings.Contains(yamlStr, "id: kestrel-vuln-v-sql-100") {
		t.Errorf("expected template id 'kestrel-vuln-v-sql-100', got:\n%s", yamlStr)
	}
	if !strings.Contains(yamlStr, "{{BaseURL}}/api/lookup") {
		t.Errorf("expected path to contain '{{BaseURL}}/api/lookup'")
	}
	if !strings.Contains(yamlStr, "SQL syntax error near admin") {
		t.Errorf("expected matchers to contain expected indicator")
	}

	// Verify valid YAML parsing
	var parsed NucleiTemplate
	// Skip comments header
	cleanYaml := yamlStr
	if idx := strings.Index(yamlStr, "id:"); idx != -1 {
		cleanYaml = yamlStr[idx:]
	}
	if err := yaml.Unmarshal([]byte(cleanYaml), &parsed); err != nil {
		t.Fatalf("failed to unmarshal generated nuclei YAML: %v", err)
	}

	if parsed.Info.Severity != "high" {
		t.Errorf("expected severity 'high', got %q", parsed.Info.Severity)
	}
	if len(parsed.HTTP) != 1 {
		t.Fatalf("expected 1 HTTP block, got %d", len(parsed.HTTP))
	}
	if parsed.HTTP[0].Method != "POST" {
		t.Errorf("expected HTTP method 'POST', got %q", parsed.HTTP[0].Method)
	}
}
