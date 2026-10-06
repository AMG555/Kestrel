package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"kestrel/internal/app"
	"kestrel/internal/config"
	"kestrel/internal/logger"
)

// Disclaimer is shown on every startup to reinforce authorized-use-only terms.
const disclaimer = `
╔══════════════════════════════════════════════════════════════════╗
║              KESTREL — AI-NATIVE SECURITY OPS PLATFORM           ║
║                     ⚠  STATUS: UNDER DEVELOPMENT  ⚠              ║
╠══════════════════════════════════════════════════════════════════╣
║  AUTHORIZED USE ONLY                                             ║
║  Use Kestrel only on systems you own or are explicitly           ║
║  authorized to test. Unauthorized use is prohibited.             ║
║                                                                  ║
║  By continuing you agree to:                                     ║
║    • Use this tool only with explicit written authorization.     ║
║    • Comply with all applicable laws and regulations.            ║
║    • Take full responsibility for any misuse.                    ║
╚══════════════════════════════════════════════════════════════════╝
`

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	httpsFlag  := flag.Bool("https", false, "Enable HTTPS with auto-generated self-signed certificate")
	flag.Parse()

	// Print consent gate on startup.
	fmt.Print(disclaimer)

	// Load configuration.
	cfg, err := config.Load(*configPath)
	if err != nil {
		// Fall back to defaults if config file is missing.
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "error: failed to load config: %v\n", err)
			os.Exit(1)
		}
		cfg = &config.Config{}
		// Apply defaults via re-export trick.
		cfgPtr, err2 := func() (*config.Config, error) {
			// Write a minimal config and reload (avoids exposing defaults() outside the package).
			tmpCfg := `server:
  host: "127.0.0.1"
  port: 8080
log:
  level: info
  output: stdout
`
			f, err := os.CreateTemp("", "kestrel-cfg-*.yaml")
			if err != nil {
				return nil, err
			}
			defer os.Remove(f.Name())
			if _, err := f.WriteString(tmpCfg); err != nil {
				return nil, err
			}
			_ = f.Close()
			return config.Load(f.Name())
		}()
		if err2 != nil {
			fmt.Fprintf(os.Stderr, "error: failed to generate defaults: %v\n", err2)
			os.Exit(1)
		}
		cfg = cfgPtr
		fmt.Fprintln(os.Stderr, "warning: config.yaml not found; using defaults. Copy config.example.yaml to config.yaml to customise.")
	}

	// Command-line flag overrides.
	if *httpsFlag {
		cfg.Server.TLSEnabled = true
		cfg.Server.TLSAutoSelfSign = true
	}

	// Build logger.
	log, err := logger.New(cfg.Log.Level, cfg.Log.Output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync() //nolint:errcheck

	// Build application.
	application, err := app.New(cfg, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to initialise application: %v\n", err)
		os.Exit(1)
	}

	// Set up signal-based graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Serve.
	if err := application.Serve(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(os.Stderr, "error: server error: %v\n", err)
		os.Exit(1)
	}
}
