package security

import (
	"os"
	"testing"

	"kestrel/internal/config"
	"gopkg.in/yaml.v3"
)

func TestApplyPatchToolConfigurationAndArgumentBuilding(t *testing.T) {
	data, err := os.ReadFile("../../tools/apply-patch.yaml")
	if err != nil {
		t.Fatalf("failed to read apply-patch.yaml: %v", err)
	}

	var tool config.ToolConfig
	if err := yaml.Unmarshal(data, &tool); err != nil {
		t.Fatalf("failed to unmarshal apply-patch.yaml: %v", err)
	}

	if tool.Name != "apply-patch" {
		t.Errorf("expected tool name 'apply-patch', got %q", tool.Name)
	}
	if !tool.Enabled {
		t.Errorf("expected tool to be enabled")
	}
	if len(tool.Parameters) != 4 {
		t.Fatalf("expected 4 parameters, got %d", len(tool.Parameters))
	}

	executor, _ := setupTestExecutor(t)

	// Test 1: check operation with patch and directory
	argsCheck := map[string]interface{}{
		"operation": "check",
		"patch":     "--- a/file.go\n+++ b/file.go\n@@ -1,1 +1,1 @@\n-old\n+new\n",
		"directory": "/tmp/repo",
		"strip":     1,
	}
	builtCheck := executor.buildCommandArgs("apply-patch", &tool, argsCheck)
	if len(builtCheck) != 6 {
		t.Fatalf("expected 6 args (-c, script, operation, patch, directory, strip), got %d (%v)", len(builtCheck), builtCheck)
	}
	if builtCheck[0] != "-c" {
		t.Errorf("expected first arg '-c', got %q", builtCheck[0])
	}
	if builtCheck[2] != "check" {
		t.Errorf("expected arg 2 to be 'check', got %q", builtCheck[2])
	}
	if builtCheck[4] != "/tmp/repo" {
		t.Errorf("expected directory '/tmp/repo', got %q", builtCheck[4])
	}
	if builtCheck[5] != "1" {
		t.Errorf("expected strip '1', got %q", builtCheck[5])
	}
}
