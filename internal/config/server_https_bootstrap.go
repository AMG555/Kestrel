package config

import "strings"

// MainWebUIUsesHTTPS determines whether the main Web UI listens on HTTPS (consistent with the preconditions for internal/app.prepareMainServerTLS).
func MainWebUIUsesHTTPS(s *ServerConfig) bool {
	if s == nil {
		return false
	}
	if s.TLSEnabled {
		return true
	}
	if s.TLSAutoSelfSign {
		return true
	}
	cert := strings.TrimSpace(s.TLSCertPath)
	key := strings.TrimSpace(s.TLSKeyPath)
	return cert != "" && key != ""
}

// ServerHTTPRedirectEnabled returns whether to redirect plain HTTP requests to HTTPS when HTTPS is enabled on the main server (enabled by default).
func ServerHTTPRedirectEnabled(s *ServerConfig) bool {
	if s == nil || !MainWebUIUsesHTTPS(s) {
		return false
	}
	if s.TLSHTTPRedirect == nil {
		return true
	}
	return *s.TLSHTTPRedirect
}

// ApplyDevHTTPSBootstrap is used by --https / one-key scripts: forces main server TLS on.
// If tls_cert_path and tls_key_path are already configured, only PEM is used (no self-sign); otherwise tls_auto_self_sign is enabled (in-memory cert, for local testing only).
func ApplyDevHTTPSBootstrap(cfg *Config) {
	if cfg == nil {
		return
	}
	cfg.Server.TLSEnabled = true
	cert := strings.TrimSpace(cfg.Server.TLSCertPath)
	key := strings.TrimSpace(cfg.Server.TLSKeyPath)
	if cert != "" && key != "" {
		cfg.Server.TLSAutoSelfSign = false
		return
	}
	cfg.Server.TLSAutoSelfSign = true
}

// ApplyPlainHTTPBootstrap is used by --http / one-key scripts: forces the main server to use plain HTTP.
// It overrides TLS switches, self-signed cert, and cert path from the config file, to prevent --http being re-enabled by HTTPS options in the config.
func ApplyPlainHTTPBootstrap(cfg *Config) {
	if cfg == nil {
		return
	}
	cfg.Server.TLSEnabled = false
	cfg.Server.TLSAutoSelfSign = false
	cfg.Server.TLSCertPath = ""
	cfg.Server.TLSKeyPath = ""
	disabled := false
	cfg.Server.TLSHTTPRedirect = &disabled
}
