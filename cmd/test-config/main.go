package main

import (
	"fmt"
	"os"

	"kestrel/internal/config"
)

func main() {
	configPath := "config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	fmt.Printf("Validating Kestrel configuration file: %s\n", configPath)
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("❌ Configuration error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Configuration syntax is valid.\n\n")
	fmt.Printf("Server Address: %s\n", cfg.Server.Address())
	fmt.Printf("TLS Active:     %v\n", cfg.Server.TLSActive())
	fmt.Printf("Database Path:  %s\n", cfg.Database.Path)
	fmt.Printf("AI Provider:    %s\n", cfg.AI.DefaultChannel)

	fmt.Printf("\nExternal MCP Servers (%d configured):\n", len(cfg.MCP.Servers))
	for name, srv := range cfg.MCP.Servers {
		fmt.Printf("  - %s (transport: %s, command: %v, url: %s)\n", name, srv.Transport, srv.Command, srv.URL)
	}

	effectiveTG := cfg.EffectiveToolGuard()
	fmt.Printf("\nToolGuard Rules (%d rules, enabled: %v):\n", len(effectiveTG.Rules), effectiveTG.Enabled)
	for _, rule := range effectiveTG.Rules {
		fmt.Printf("  - %s [%s] (enabled: %v): %s\n", rule.ID, rule.Name, rule.Enabled, rule.Pattern)
	}
}
