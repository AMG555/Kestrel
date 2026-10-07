package main

import (
	"kestrel/internal/config"
	"kestrel/internal/logger"
	"kestrel/internal/mcp"
	"kestrel/internal/security"
	"kestrel/internal/toolguard"
	"flag"
	"fmt"
	"os"

	"go.uber.org/zap"
)

func main() {
	var configPath = flag.String("config", "config.yaml", "configuration filepath")
	flag.Parse()

	// load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// initialize log（stdio pattern下使用 stderr 输出log，避免干扰 JSON-RPC 通信）
	log := logger.New(cfg.Log.Level, "stderr", logger.DiagnosticOptions{
		Dir:           cfg.Log.DiagnosticDir,
		Disabled:      cfg.Log.DiagnosticDisabled,
		RetentionDays: cfg.Log.DiagnosticRetentionDays,
	})
	defer log.Sync()

	// createMCP服务器
	mcpServer := mcp.NewServer(log.Logger)
	guard, err := toolguard.NewManager(cfg.EffectiveToolGuard())
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化调用拦截failed: %v\n", err)
		os.Exit(1)
	}
	mcpServer.SetToolGuard(guard)

	// create secure tool executor
	executor := security.NewExecutor(&cfg.Security, mcpServer, log.Logger)

	// register tools
	executor.RegisterTools(mcpServer)
	mcp.RegisterExecutionControlTools(mcpServer, nil)

	log.Logger.Info("MCP服务器（stdiopattern）已start，等待message...")

	// 运行 stdio 循环
	if err := mcpServer.HandleStdio(); err != nil {
		log.Logger.Error("MCP服务器运行failed", zap.Error(err))
		os.Exit(1)
	}
}
