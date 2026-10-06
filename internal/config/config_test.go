package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"kestrel/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	// Write a minimal config file.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("version: test\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("default port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Auth.SessionDurationHours != 12 {
		t.Errorf("default session duration = %d, want 12", cfg.Auth.SessionDurationHours)
	}
	if cfg.Auth.JWTSecret == "" {
		t.Error("JWTSecret should be auto-generated")
	}
	if cfg.Database.Path == "" {
		t.Error("Database.Path should have a default")
	}
}

func TestLoadEnvExpansion(t *testing.T) {
	t.Setenv("KESTREL_JWT_SECRET", "test-secret-123")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "auth:\n  jwt_secret: ${KESTREL_JWT_SECRET}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Auth.JWTSecret != "test-secret-123" {
		t.Errorf("JWTSecret = %q, want %q", cfg.Auth.JWTSecret, "test-secret-123")
	}
}

func TestTLSActive(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.ServerConfig
		want bool
	}{
		{"none", config.ServerConfig{}, false},
		{"tls_enabled", config.ServerConfig{TLSEnabled: true}, true},
		{"auto_self_sign", config.ServerConfig{TLSAutoSelfSign: true}, true},
		{"cert_and_key", config.ServerConfig{TLSCertPath: "a.crt", TLSKeyPath: "a.key"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.TLSActive(); got != tt.want {
				t.Errorf("TLSActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddress(t *testing.T) {
	cfg := config.ServerConfig{Host: "0.0.0.0", Port: 9090}
	if addr := cfg.Address(); addr != "0.0.0.0:9090" {
		t.Errorf("Address() = %q, want %q", addr, "0.0.0.0:9090")
	}
}
