package config_test

import (
	"path/filepath"
	"testing"

	"kestrel/internal/config"
)

func TestWorkspaceRolesAndTools(t *testing.T) {
	rolesDir := filepath.Join("..", "..", "roles")
	roles, err := config.LoadRolesFromDir(rolesDir)
	if err != nil {
		t.Fatalf("failed to load roles: %v", err)
	}
	if len(roles) < 14 {
		t.Fatalf("expected at least 14 roles, got %d", len(roles))
	}
	requiredRoles := []string{"Container Security", "CI/CD Pipeline Security", "Cryptographic Audit"}
	for _, req := range requiredRoles {
		if _, ok := roles[req]; !ok {
			t.Errorf("expected role %q to be loaded", req)
		}
	}

	toolsDir := filepath.Join("..", "..", "tools")
	tools, err := config.LoadToolsFromDir(toolsDir)
	if err != nil {
		t.Fatalf("failed to load tools: %v", err)
	}
	if len(tools) < 90 {
		t.Fatalf("expected at least 90 tools, got %d", len(tools))
	}
	foundTlsx := false
	for _, tool := range tools {
		if tool.Name == "tlsx" {
			foundTlsx = true
			if !tool.Enabled {
				t.Errorf("tlsx tool should be enabled")
			}
			if len(tool.Parameters) == 0 {
				t.Errorf("tlsx tool should have parameters defined")
			}
		}
	}
	if !foundTlsx {
		t.Errorf("expected tlsx tool to be present in tools catalog")
	}
}
