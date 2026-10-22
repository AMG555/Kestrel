package security

import (
	"os"
	"testing"

	"kestrel/internal/config"
	"gopkg.in/yaml.v3"
)

func TestSemgrepToolConfigurationAndArgumentBuilding(t *testing.T) {
	data, err := os.ReadFile("../../tools/semgrep.yaml")
	if err != nil {
		t.Fatalf("failed to read semgrep.yaml: %v", err)
	}

	var tool config.ToolConfig
	if err := yaml.Unmarshal(data, &tool); err != nil {
		t.Fatalf("failed to unmarshal semgrep.yaml: %v", err)
	}

	if tool.Name != "semgrep" {
		t.Errorf("expected tool name 'semgrep', got %q", tool.Name)
	}
	if !tool.Enabled {
		t.Errorf("expected tool to be enabled")
	}

	executor, _ := setupTestExecutor(t)

	args := map[string]interface{}{
		"target":      "./src",
		"config":      "p/owasp-top-ten",
		"severity":    "ERROR",
		"json_output": true,
	}

	cmdArgs := executor.buildCommandArgs("semgrep", &tool, args)
	if len(cmdArgs) < 4 {
		t.Fatalf("expected at least 4 arguments, got %v", cmdArgs)
	}

	foundConfig := false
	foundSeverity := false
	foundJSON := false
	foundTarget := false

	for i, a := range cmdArgs {
		if a == "--config" && i+1 < len(cmdArgs) && cmdArgs[i+1] == "p/owasp-top-ten" {
			foundConfig = true
		}
		if a == "--severity" && i+1 < len(cmdArgs) && cmdArgs[i+1] == "ERROR" {
			foundSeverity = true
		}
		if a == "--json" {
			foundJSON = true
		}
		if a == "./src" {
			foundTarget = true
		}
	}

	if !foundConfig {
		t.Errorf("expected --config p/owasp-top-ten in args: %v", cmdArgs)
	}
	if !foundSeverity {
		t.Errorf("expected --severity ERROR in args: %v", cmdArgs)
	}
	if !foundJSON {
		t.Errorf("expected --json in args: %v", cmdArgs)
	}
	if !foundTarget {
		t.Errorf("expected target ./src in args: %v", cmdArgs)
	}
}
