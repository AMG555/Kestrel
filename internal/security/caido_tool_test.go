package security

import (
	"os"
	"testing"

	"kestrel/internal/config"
	"gopkg.in/yaml.v3"
)

func TestCaidoToolConfigurationAndArgumentBuilding(t *testing.T) {
	data, err := os.ReadFile("../../tools/caido.yaml")
	if err != nil {
		t.Fatalf("failed to read caido.yaml: %v", err)
	}

	var tool config.ToolConfig
	if err := yaml.Unmarshal(data, &tool); err != nil {
		t.Fatalf("failed to unmarshal caido.yaml: %v", err)
	}

	if tool.Name != "caido" {
		t.Errorf("expected tool name 'caido', got %q", tool.Name)
	}
	if !tool.Enabled {
		t.Errorf("expected tool to be enabled")
	}
	if len(tool.Parameters) != 7 {
		t.Fatalf("expected 7 parameters, got %d", len(tool.Parameters))
	}

	executor, _ := setupTestExecutor(t)

	// Test 1: health check
	argsHealth := map[string]interface{}{
		"operation": "health",
	}
	builtHealth := executor.buildCommandArgs("caido", &tool, argsHealth)
	if len(builtHealth) < 3 {
		t.Fatalf("expected at least 3 args (-c, script, operation), got %v", builtHealth)
	}
	if builtHealth[0] != "-c" {
		t.Errorf("expected first arg '-c', got %q", builtHealth[0])
	}
	if builtHealth[2] != "health" {
		t.Errorf("expected positional arg 0 to be 'health', got %q", builtHealth[2])
	}

	// Test 2: replay operation with all arguments
	argsReplay := map[string]interface{}{
		"operation": "replay",
		"target":    "http://target.local/api/user",
		"method":    "POST",
		"payload":   `{"admin": true}`,
		"headers":   `{"Content-Type": "application/json"}`,
		"caido_url": "http://127.0.0.1:8080",
		"api_key":   "token123",
	}
	builtReplay := executor.buildCommandArgs("caido", &tool, argsReplay)
	if len(builtReplay) != 9 {
		t.Fatalf("expected 9 args, got %d (%v)", len(builtReplay), builtReplay)
	}
	if builtReplay[2] != "replay" ||
		builtReplay[3] != "http://target.local/api/user" ||
		builtReplay[4] != "POST" ||
		builtReplay[5] != `{"admin": true}` ||
		builtReplay[6] != `{"Content-Type": "application/json"}` ||
		builtReplay[7] != "http://127.0.0.1:8080" ||
		builtReplay[8] != "token123" {
		t.Errorf("unexpected replay argument sequence: %v", builtReplay)
	}
}
