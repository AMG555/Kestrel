package security

import (
	"os"
	"os/exec"
	"runtime"
	"testing"

	"kestrel/internal/config"
	"gopkg.in/yaml.v3"
)

func TestInteractshToolConfigurationAndArgumentBuilding(t *testing.T) {
	data, err := os.ReadFile("../../tools/interactsh.yaml")
	if err != nil {
		t.Fatalf("failed to read interactsh.yaml: %v", err)
	}

	var tool config.ToolConfig
	if err := yaml.Unmarshal(data, &tool); err != nil {
		t.Fatalf("failed to unmarshal interactsh.yaml: %v", err)
	}

	if tool.Name != "interactsh" {
		t.Errorf("expected tool name 'interactsh', got %q", tool.Name)
	}
	if !tool.Enabled {
		t.Errorf("expected tool to be enabled")
	}
	if len(tool.Parameters) != 4 {
		t.Fatalf("expected 4 parameters, got %d", len(tool.Parameters))
	}

	executor, _ := setupTestExecutor(t)

	// Test 1: get_domain
	argsGetDomain := map[string]interface{}{
		"operation": "get_domain",
	}
	builtArgs := executor.buildCommandArgs("interactsh", &tool, argsGetDomain)
	if len(builtArgs) < 3 {
		t.Fatalf("expected at least 3 args (-c, script, operation), got %v", builtArgs)
	}
	if builtArgs[0] != "-c" {
		t.Errorf("expected first arg '-c', got %q", builtArgs[0])
	}
	if builtArgs[2] != "get_domain" {
		t.Errorf("expected positional arg 0 to be 'get_domain', got %q", builtArgs[2])
	}

	// Test 2: poll_records with domain and wait_time
	argsPoll := map[string]interface{}{
		"operation": "poll_records",
		"domain":    "testdomain.oast.pro",
		"wait_time": 5,
		"server":    "oast.pro",
	}
	builtPollArgs := executor.buildCommandArgs("interactsh", &tool, argsPoll)
	if len(builtPollArgs) != 6 {
		t.Fatalf("expected 6 args, got %d (%v)", len(builtPollArgs), builtPollArgs)
	}
	if builtPollArgs[2] != "poll_records" || builtPollArgs[3] != "testdomain.oast.pro" || builtPollArgs[4] != "5" || builtPollArgs[5] != "oast.pro" {
		t.Errorf("unexpected poll argument ordering: %v", builtPollArgs)
	}
}

func TestResolveExecutable_WindowsPythonFallback(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skipping Windows-specific executable fallback test on non-Windows")
	}

	resolved := resolveExecutable("python3")
	if _, err := exec.LookPath("python3"); err != nil {
		if _, pyErr := exec.LookPath("python"); pyErr == nil {
			if resolved != "python" {
				t.Errorf("expected python3 to resolve to python, got %q", resolved)
			}
		}
	}
}
