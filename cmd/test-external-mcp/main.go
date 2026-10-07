package main

import (
	"context"
	"fmt"
	"os"
	"time"

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
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	if cfg.ExternalMCP.Servers == nil || len(cfg.ExternalMCP.Servers) == 0 {
		fmt.Println("No external MCP servers configured")
		os.Exit(0)
	}

	fmt.Printf("Found %d external MCP server(s)\n\n", len(cfg.ExternalMCP.Servers))

	// create logger
	log := logger.New("info", "stdout")

	// create external MCP manager
	manager := mcp.NewExternalMCPManager(log.Logger)
	manager.LoadConfigs(&cfg.ExternalMCP)

	// display config
	fmt.Println("=== config info ===")
	for name, srv := range cfg.ExternalMCP.Servers {
		fmt.Printf("\n%s:\n", name)
		fmt.Printf("  Transport: %s\n", getTransport(srv))
		if srv.Command != "" {
			fmt.Printf("  Command: %s\n", srv.Command)
			fmt.Printf("  Args: %v\n", srv.Args)
		}
		if srv.URL != "" {
			fmt.Printf("  URL: %s\n", srv.URL)
		}
		fmt.Printf("  Description: %s\n", srv.Description)
		fmt.Printf("  Timeout: %d seconds\n", srv.Timeout)
		fmt.Printf("  ExternalMCPEnable: %v\n", srv.ExternalMCPEnable)
	}

	// get statistics
	fmt.Println("\n=== statistics ===")
	stats := manager.GetStats()
	fmt.Printf("total: %d\n", stats["total"])
	fmt.Printf("enabled: %d\n", stats["enabled"])
	fmt.Printf("disabled: %d\n", stats["disabled"])
	fmt.Printf("connected: %d\n", stats["connected"])

	// test start (enabled only)
	fmt.Println("\n=== test start ===")
	for name, srv := range cfg.ExternalMCP.Servers {
		if srv.ExternalMCPEnable {
			fmt.Printf("\nattempting to start %s...\n", name)
			// note: actual start may fail if no real MCP server is available
			err := manager.StartClient(name)
			if err != nil {
				fmt.Printf("  startup failed (this is normal if no real MCP server is present): %v\n", err)
			} else {
				fmt.Printf("  start successful\n")
				// get client status
				if client, exists := manager.GetClient(name); exists {
					fmt.Printf("  status: %s\n", client.GetStatus())
					fmt.Printf("  connected: %v\n", client.IsConnected())
				}
			}
		}
	}

	// wait a moment
	time.Sleep(2 * time.Second)

	// test get tool list
	fmt.Println("\n=== test get tool list ===")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tools, err := manager.GetAllTools(ctx)
	if err != nil {
		fmt.Printf("failed to get tool list: %v\n", err)
	} else {
		fmt.Printf("retrieved %d tools\n", len(tools))
		for i, tool := range tools {
			if i < 5 { // show first 5 only
				fmt.Printf("  - %s: %s\n", tool.Name, tool.Description)
			}
		}
		if len(tools) > 5 {
			fmt.Printf("  ... and %d more tools\n", len(tools)-5)
		}
	}

	// test stop
	fmt.Println("\n=== test stop ===")
	for name := range cfg.ExternalMCP.Servers {
		fmt.Printf("\nstopping %s...\n", name)
		err := manager.StopClient(name)
		if err != nil {
			fmt.Printf("  shutdown failed: %v\n", err)
		} else {
			fmt.Printf("  stop successful\n")
		}
	}

	// final statistics
	fmt.Println("\n=== final statistics ===")
	stats = manager.GetStats()
	fmt.Printf("total: %d\n", stats["total"])
	fmt.Printf("enabled: %d\n", stats["enabled"])
	fmt.Printf("disabled: %d\n", stats["disabled"])
	fmt.Printf("connected: %d\n", stats["connected"])

	fmt.Println("\n=== test complete ===")
}

func getTransport(srv config.ExternalMCPServerConfig) string {
	t := srv.GetTransportType()
	if t == "" {
		return "unknown"
	}
	return t
}

