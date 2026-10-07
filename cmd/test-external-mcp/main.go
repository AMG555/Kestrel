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

	// createlog
	log := logger.New("info", "stdout")

	// create外部MCP管理器
	manager := mcp.NewExternalMCPManager(log.Logger)
	manager.LoadConfigs(&cfg.ExternalMCP)

	// 显示config
	fmt.Println("=== configinfo ===")
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

	// 获取Statistics info
	fmt.Println("\n=== Statistics info ===")
	stats := manager.GetStats()
	fmt.Printf("total: %d\n", stats["total"])
	fmt.Printf("enabled: %d\n", stats["enabled"])
	fmt.Printf("已停用: %d\n", stats["disabled"])
	fmt.Printf("已连接: %d\n", stats["connected"])

	// teststart（仅testenable的）
	fmt.Println("\n=== teststart ===")
	for name, srv := range cfg.ExternalMCP.Servers {
		if srv.ExternalMCPEnable {
			fmt.Printf("\n尝试start %s...\n", name)
			// 注意：实际start可能会failed，因为需要true实的MCP服务器
			err := manager.StartClient(name)
			if err != nil {
				fmt.Printf("  startup failed（这yesnormal的，如果没有true实的MCP服务器）: %v\n", err)
			} else {
				fmt.Printf("  startsuccessful\n")
				// 获取客户端status
				if client, exists := manager.GetClient(name); exists {
					fmt.Printf("  status: %s\n", client.GetStatus())
					fmt.Printf("  已连接: %v\n", client.IsConnected())
				}
			}
		}
	}

	// 等待一下
	time.Sleep(2 * time.Second)

	// test获取tool list
	fmt.Println("\n=== test获取tool list ===")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tools, err := manager.GetAllTools(ctx)
	if err != nil {
		fmt.Printf("failed to get tool list: %v\n", err)
	} else {
		fmt.Printf("获取到 %d 个tool\n", len(tools))
		for i, tool := range tools {
			if i < 5 { // 只显示前5个
				fmt.Printf("  - %s: %s\n", tool.Name, tool.Description)
			}
		}
		if len(tools) > 5 {
			fmt.Printf("  ... 还有 %d 个tool\n", len(tools)-5)
		}
	}

	// teststop
	fmt.Println("\n=== teststop ===")
	for name := range cfg.ExternalMCP.Servers {
		fmt.Printf("\nstop %s...\n", name)
		err := manager.StopClient(name)
		if err != nil {
			fmt.Printf("  shutdown failed: %v\n", err)
		} else {
			fmt.Printf("  stopsuccessful\n")
		}
	}

	// 最终统计
	fmt.Println("\n=== 最终统计 ===")
	stats = manager.GetStats()
	fmt.Printf("total: %d\n", stats["total"])
	fmt.Printf("enabled: %d\n", stats["enabled"])
	fmt.Printf("已停用: %d\n", stats["disabled"])
	fmt.Printf("已连接: %d\n", stats["connected"])

	fmt.Println("\n=== test完成 ===")
}

func getTransport(srv config.ExternalMCPServerConfig) string {
	t := srv.GetTransportType()
	if t == "" {
		return "unknown"
	}
	return t
}

