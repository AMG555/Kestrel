package config

import (
	"os"
	"strings"
)

// expandEnvVar expands ${VAR} and ${VAR:-default} environment variable references in a string.
// Consistent with the official MCP config format (supported by Claude Desktop / Cursor / VS Code).
func expandEnvVar(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		// find ${
		idx := strings.Index(s[i:], "${")
		if idx < 0 {
			b.WriteString(s[i:])
			break
		}
		b.WriteString(s[i : i+idx])
		i += idx + 2 // skip ${

		// find matching }
		end := strings.IndexByte(s[i:], '}')
		if end < 0 {
			// no }, keep as-is
			b.WriteString("${")
			continue
		}
		expr := s[i : i+end]
		i += end + 1 // skip }

		// parse VAR:-default
		varName := expr
		defaultVal := ""
		hasDefault := false
		if colonIdx := strings.Index(expr, ":-"); colonIdx >= 0 {
			varName = expr[:colonIdx]
			defaultVal = expr[colonIdx+2:]
			hasDefault = true
		}

		val := os.Getenv(varName)
		if val == "" && hasDefault {
			val = defaultVal
		}
		b.WriteString(val)
	}
	return b.String()
}

// ExpandConfigEnv expands all environment variable references in fields of ExternalMCPServerConfig.
// Scope: Command, Args, Env values, URL, Headers values.
func ExpandConfigEnv(cfg *ExternalMCPServerConfig) {
	cfg.Command = expandEnvVar(cfg.Command)
	for i, arg := range cfg.Args {
		cfg.Args[i] = expandEnvVar(arg)
	}
	for k, v := range cfg.Env {
		cfg.Env[k] = expandEnvVar(v)
	}
	cfg.URL = expandEnvVar(cfg.URL)
	for k, v := range cfg.Headers {
		cfg.Headers[k] = expandEnvVar(v)
	}
}
