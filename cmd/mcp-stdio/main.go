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

	// initialize logger (in stdio mode, write to stderr to avoid interfering with JSON-RPC communication)
	log := logger.New(cfg.Log.Level, "stderr", logger.DiagnosticOptions{
		Dir:           cfg.Log.DiagnosticDir,
		Disabled:      cfg.Log.DiagnosticDisabled,
		RetentionDays: cfg.Log.DiagnosticRetentionDays,
	})
	defer log.Sync()

	// create MCP server
	mcpServer := mcp.NewServer(log.Logger)
	guard, err := toolguard.NewManager(cfg.EffectiveToolGuard())
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize tool guard: %v\n", err)
		os.Exit(1)
	}
	mcpServer.SetToolGuard(guard)

	// create secure tool executor
	executor := security.NewExecutor(&cfg.Security, mcpServer, log.Logger)

	// register tools
	executor.RegisterTools(mcpServer)
	mcp.RegisterExecutionControlTools(mcpServer, nil)

	log.Logger.Info("MCP server (stdio mode) started, waiting for messages...")

	// run stdio loop
	if err := mcpServer.HandleStdio(); err != nil {
		log.Logger.Error("MCP server run failed", zap.Error(err))
		os.Exit(1)
	}
}
