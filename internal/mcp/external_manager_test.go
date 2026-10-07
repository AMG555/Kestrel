package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"kestrel/internal/authctx"
	"kestrel/internal/config"

	"go.uber.org/zap"
)

func TestExternalManagerEnforcesConfiguredAuthorizer(t *testing.T) {
	manager := NewExternalMCPManager(zap.NewNop())
	t.Cleanup(manager.StopAll)
	manager.SetToolAuthorizer(func(context.Context, string, map[string]interface{}) error {
		return errors.New("denied by policy")
	})
	ctx := authctx.WithPrincipal(context.Background(), authctx.NewPrincipal("u1", "user", "assigned", map[string]bool{"agent:execute": true}))
	_, executionID, err := manager.CallTool(ctx, "server::tool", map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "authorization denied") {
		t.Fatalf("external call bypassed authorizer: %v", err)
	}
	if executionID == "" {
		t.Fatal("denied external call should still return an execution id")
	}
	execution, ok := manager.GetExecution(executionID)
	if !ok || execution == nil {
		t.Fatalf("missing denied external execution %q", executionID)
	}
	if execution.Status != ToolExecutionStatusFailed || !strings.Contains(execution.Error, "denied by policy") {
		t.Fatalf("denied external execution = %#v, want failed with policy error", execution)
	}
}

func TestExternalMCPManager_AddOrUpdateConfig(t *testing.T) {
	logger := zap.NewNop()
	manager := NewExternalMCPManager(logger)

	// test添加stdioconfig
	stdioCfg := config.ExternalMCPServerConfig{
		Command:           "python3",
		Args:              []string{"/path/to/script.py"},
		Description:       "Test stdio MCP",
		Timeout:           30,
		ExternalMCPEnable: true,
	}

	err := manager.AddOrUpdateConfig("test-stdio", stdioCfg)
	if err != nil {
		t.Fatalf("添加stdioconfigfailed: %v", err)
	}

	// test添加HTTPconfig
	httpCfg := config.ExternalMCPServerConfig{
		Type:              "http",
		URL:               "http://127.0.0.1:8081/mcp",
		Description:       "Test HTTP MCP",
		Timeout:           30,
		ExternalMCPEnable: false,
	}

	err = manager.AddOrUpdateConfig("test-http", httpCfg)
	if err != nil {
		t.Fatalf("添加HTTPconfigfailed: %v", err)
	}

	// validateconfig已save
	configs := manager.GetConfigs()
	if len(configs) != 2 {
		t.Fatalf("期望2个config，实际%d个", len(configs))
	}

	if configs["test-stdio"].Command != stdioCfg.Command {
		t.Errorf("stdioconfig命令不匹配")
	}

	if configs["test-http"].URL != httpCfg.URL {
		t.Errorf("HTTPconfigURL不匹配")
	}
}

func TestExternalMCPManager_RemoveConfig(t *testing.T) {
	logger := zap.NewNop()
	manager := NewExternalMCPManager(logger)

	cfg := config.ExternalMCPServerConfig{
		Command:           "python3",
		ExternalMCPEnable: false,
	}

	manager.AddOrUpdateConfig("test-remove", cfg)

	// 移除config
	err := manager.RemoveConfig("test-remove")
	if err != nil {
		t.Fatalf("移除configfailed: %v", err)
	}

	configs := manager.GetConfigs()
	if _, exists := configs["test-remove"]; exists {
		t.Error("config应该已被移除")
	}
}

func TestExternalMCPManager_GetStats(t *testing.T) {
	logger := zap.NewNop()
	manager := NewExternalMCPManager(logger)

	// 添加多个config
	manager.AddOrUpdateConfig("enabled1", config.ExternalMCPServerConfig{
		Command:           "python3",
		ExternalMCPEnable: true,
	})

	manager.AddOrUpdateConfig("enabled2", config.ExternalMCPServerConfig{
		URL:               "http://127.0.0.1:8081/mcp",
		ExternalMCPEnable: true,
	})

	manager.AddOrUpdateConfig("disabled1", config.ExternalMCPServerConfig{
		Command:           "python3",
		ExternalMCPEnable: false,
	})

	stats := manager.GetStats()

	if stats["total"].(int) != 3 {
		t.Errorf("期望total3，实际%d", stats["total"])
	}

	if stats["enabled"].(int) != 2 {
		t.Errorf("期望enable数2，实际%d", stats["enabled"])
	}

	if stats["disabled"].(int) != 1 {
		t.Errorf("期望停用数1，实际%d", stats["disabled"])
	}
}

func TestExternalMCPManager_LoadConfigs(t *testing.T) {
	logger := zap.NewNop()
	manager := NewExternalMCPManager(logger)

	externalMCPConfig := config.ExternalMCPConfig{
		Servers: map[string]config.ExternalMCPServerConfig{
			"loaded1": {
				Command:           "python3",
				ExternalMCPEnable: true,
			},
			"loaded2": {
				URL:               "http://127.0.0.1:8081/mcp",
				ExternalMCPEnable: false,
			},
		},
	}

	manager.LoadConfigs(&externalMCPConfig)

	configs := manager.GetConfigs()
	if len(configs) != 2 {
		t.Fatalf("期望2个config，实际%d个", len(configs))
	}

	if configs["loaded1"].Command != "python3" {
		t.Error("config1load failed")
	}

	if configs["loaded2"].URL != "http://127.0.0.1:8081/mcp" {
		t.Error("config2load failed")
	}
}

// TestLazySDKClient_InitializeFails validatenone效config时 SDK 客户端 Initialize failed并settings error status
func TestLazySDKClient_InitializeFails(t *testing.T) {
	logger := zap.NewNop()
	// 使用不存在的 HTTP address，Initialize 应failed
	cfg := config.ExternalMCPServerConfig{
		Type:    "http",
		URL:     "http://127.0.0.1:19999/nonexistent",
		Timeout: 2,
	}
	c := newLazySDKClient(cfg, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := c.Initialize(ctx)
	if err == nil {
		t.Fatal("expected error when connecting to invalid server")
	}
	if c.GetStatus() != "error" {
		t.Errorf("expected status error, got %s", c.GetStatus())
	}
	c.Close()
}

func TestExternalMCPManager_StartStopClient(t *testing.T) {
	logger := zap.NewNop()
	manager := NewExternalMCPManager(logger)

	// 添加一个disable的config
	cfg := config.ExternalMCPServerConfig{
		Command:           "python3",
		ExternalMCPEnable: false,
	}

	manager.AddOrUpdateConfig("test-start-stop", cfg)

	// 尝试start（可能会failed，因为没有true实的服务器）
	err := manager.StartClient("test-start-stop")
	if err != nil {
		t.Logf("startup failed（可能yes没有服务器）: %v", err)
	}

	// stop
	err = manager.StopClient("test-start-stop")
	if err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	// validateconfig已update为disable
	configs := manager.GetConfigs()
	if configs["test-start-stop"].ExternalMCPEnable {
		t.Error("config应该已被disable")
	}
}

func TestExternalMCPManager_CallTool(t *testing.T) {
	logger := zap.NewNop()
	manager := NewExternalMCPManager(logger)

	// test调用不存在的tool
	_, _, err := manager.CallTool(context.Background(), "nonexistent::tool", map[string]interface{}{})
	if err == nil {
		t.Error("应该backerror")
	}

	// testnone效的tool nameformat
	_, _, err = manager.CallTool(context.Background(), "invalid-tool-name", map[string]interface{}{})
	if err == nil {
		t.Error("应该backerror（none效format）")
	}
}

func TestExternalMCPManager_GetAllTools(t *testing.T) {
	logger := zap.NewNop()
	manager := NewExternalMCPManager(logger)

	ctx := context.Background()
	tools, err := manager.GetAllTools(ctx)
	if err != nil {
		t.Fatalf("failed to get tool list: %v", err)
	}

	// 如果没有连接的客户端，应该backnulllist
	if len(tools) != 0 {
		t.Logf("获取到%d个tool", len(tools))
	}
}
