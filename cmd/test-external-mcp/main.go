package main

import (
	"fmt"
	"os"

	"kestrel/internal/config"
	"kestrel/internal/logger"
	"kestrel/internal/mcp"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run cmd/test-external-mcp/main.go <config.yaml>")
		os.Exit(1)
	}

	configPath := os.Args[1]
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	if len(cfg.MCP.Servers) == 0 {
		fmt.Println("No external MCP servers configured in mcp.servers")
		os.Exit(0)
	}

	fmt.Printf("Found %d external MCP server(s)\n\n", len(cfg.MCP.Servers))
	log, _ := logger.New(cfg.Log.Level, cfg.Log.Output)

	tgCfg := cfg.EffectiveToolGuard()
	registry := mcp.NewRegistryWithGuard(&cfg.MCP, &tgCfg, log)

	fmt.Println("=== Configured MCP Servers ===")
	for name, srv := range cfg.MCP.Servers {
		fmt.Printf("\nServer: %s\n", name)
		fmt.Printf("  Transport: %s\n", srv.Transport)
		if len(srv.Command) > 0 {
			fmt.Printf("  Command:   %v\n", srv.Command)
		}
		if srv.URL != "" {
			fmt.Printf("  URL:       %s\n", srv.URL)
		}
		if len(srv.AllowedTools) > 0 {
			fmt.Printf("  Allowed:   %v\n", srv.AllowedTools)
		}
		timeout := srv.TimeoutSeconds
		if timeout <= 0 {
			timeout = cfg.MCP.CallTimeoutSeconds
		}
		fmt.Printf("  Timeout:   %d seconds\n", timeout)
	}

	fmt.Println("\n=== Active Tool Catalog ===")
	tools := registry.ListTools()
	fmt.Printf("Registered tools count: %d\n", len(tools))
	for i, t := range tools {
		if i < 10 {
			fmt.Printf("  - %s (%s): %s\n", t.Name, t.Class, t.Description)
		}
	}
	if len(tools) > 10 {
		fmt.Printf("  ... and %d more tools\n", len(tools)-10)
	}

	fmt.Println("\n=== Pre-Flight Check Complete ===")
}
