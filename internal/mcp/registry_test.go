package mcp_test

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"
	"kestrel/internal/config"
	"kestrel/internal/mcp"
)

func newRegistry(t *testing.T) *mcp.Registry {
	t.Helper()
	cfg := &config.MCPConfig{
		WorkerPoolSize:     4,
		CallTimeoutSeconds: 30,
		OutputCapBytes:     102400,
	}
	return mcp.NewRegistry(cfg, zap.NewNop())
}

func TestRegistryListTools(t *testing.T) {
	r := newRegistry(t)
	tools := r.ListTools()
	if len(tools) == 0 {
		t.Fatal("expected at least one built-in tool")
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}

	required := []string{"subdomain_enum", "http_probe", "dns_lookup", "whois_lookup", "ssl_cert_check", "tech_fingerprint", "port_scan"}
	for _, name := range required {
		if !names[name] {
			t.Errorf("expected tool %q to be registered", name)
		}
	}
}

func TestRegistryGetTool(t *testing.T) {
	r := newRegistry(t)

	tool, ok := r.GetTool("subdomain_enum")
	if !ok {
		t.Fatal("subdomain_enum tool not found")
	}
	if tool.Class != mcp.ToolClassReadOnly {
		t.Errorf("subdomain_enum class = %q, want %q", tool.Class, mcp.ToolClassReadOnly)
	}
}

func TestRegistryAllowlistBlocking(t *testing.T) {
	r := newRegistry(t)

	_, err := r.Execute(context.Background(), "subdomain_enum",
		map[string]interface{}{"domain": "example.com"},
		[]string{"http_probe"}, // subdomain_enum NOT in allowlist
	)
	if err == nil {
		t.Error("Execute should fail when tool is not in allowlist")
	}
}

func TestRegistryAllowlistWildcard(t *testing.T) {
	r := newRegistry(t)
	tools := r.ListToolsForRole([]string{"*"})
	all := r.ListTools()
	if len(tools) != len(all) {
		t.Errorf("wildcard allowlist: got %d tools, want %d", len(tools), len(all))
	}
}

func TestCustomToolRegistration(t *testing.T) {
	r := newRegistry(t)

	called := false
	r.RegisterTool(&mcp.ToolDefinition{
		Name:        "test_tool",
		Description: "A test tool",
		Class:       mcp.ToolClassReadOnly,
		ServerID:    "test",
	}, func(ctx context.Context, args map[string]interface{}) (string, error) {
		called = true
		return "ok", nil
	})

	result, err := r.Execute(context.Background(), "test_tool", nil, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
	if result.Output != "ok" {
		t.Errorf("output = %q, want %q", result.Output, "ok")
	}
}

func TestOutputCap(t *testing.T) {
	cfg := &config.MCPConfig{
		WorkerPoolSize:     2,
		CallTimeoutSeconds: 10,
		OutputCapBytes:     10, // tiny cap
	}
	r := mcp.NewRegistry(cfg, zap.NewNop())
	r.RegisterTool(&mcp.ToolDefinition{
		Name:  "big_tool",
		Class: mcp.ToolClassReadOnly,
	}, func(_ context.Context, _ map[string]interface{}) (string, error) {
		return "this is a very long output string that exceeds the cap", nil
	})

	result, err := r.Execute(context.Background(), "big_tool", nil, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !result.Truncated {
		t.Error("expected output to be truncated")
	}
	if len(result.Output) > 10 {
		t.Errorf("output len = %d, want <= 10", len(result.Output))
	}
}

func TestToolGuardBlocksGovernmentDomain(t *testing.T) {
	cfg := &config.MCPConfig{
		WorkerPoolSize:     2,
		CallTimeoutSeconds: 10,
		OutputCapBytes:     102400,
	}
	r := mcp.NewRegistry(cfg, zap.NewNop()) // uses default government-domain rule

	// Register a harmless echo tool.
	r.RegisterTool(&mcp.ToolDefinition{
		Name:  "echo_tool",
		Class: mcp.ToolClassReadOnly,
	}, func(_ context.Context, args map[string]interface{}) (string, error) {
		return "ok", nil
	})

	// Call with a government domain in the arguments — must be blocked.
	_, err := r.Execute(context.Background(), "echo_tool",
		map[string]interface{}{"target": "https://agency.gov/login"},
		nil,
	)
	if err == nil {
		t.Fatal("toolguard should have blocked the government domain call")
	}
	if !strings.Contains(err.Error(), "blocked by security rule") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestToolGuardAllowsNormalDomain(t *testing.T) {
	cfg := &config.MCPConfig{
		WorkerPoolSize:     2,
		CallTimeoutSeconds: 10,
		OutputCapBytes:     102400,
	}
	r := mcp.NewRegistry(cfg, zap.NewNop())

	r.RegisterTool(&mcp.ToolDefinition{
		Name:  "safe_tool",
		Class: mcp.ToolClassReadOnly,
	}, func(_ context.Context, _ map[string]interface{}) (string, error) {
		return "done", nil
	})

	result, err := r.Execute(context.Background(), "safe_tool",
		map[string]interface{}{"target": "https://example.com"},
		nil,
	)
	if err != nil {
		t.Fatalf("safe domain should not be blocked: %v", err)
	}
	if result.Output != "done" {
		t.Errorf("output = %q, want done", result.Output)
	}
}

