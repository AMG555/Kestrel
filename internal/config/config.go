package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration structure.
type Config struct {
	Version  string         `yaml:"version,omitempty"`
	Server   ServerConfig   `yaml:"server"`
	Auth     AuthConfig     `yaml:"auth"`
	Log      LogConfig      `yaml:"log"`
	Database DatabaseConfig `yaml:"database"`
	Audit    AuditConfig    `yaml:"audit"`
	AI       AIConfig       `yaml:"ai"`
	Agent    AgentConfig    `yaml:"agent"`
	MCP      MCPConfig      `yaml:"mcp"`
	HITL     HITLConfig     `yaml:"hitl"`
	Knowledge KnowledgeConfig `yaml:"knowledge"`
	RolesDir string         `yaml:"roles_dir"`
}

// ServerConfig holds HTTP/HTTPS server settings.
type ServerConfig struct {
	Host            string   `yaml:"host"`
	Port            int      `yaml:"port"`
	TLSEnabled      bool     `yaml:"tls_enabled"`
	TLSAutoSelfSign bool     `yaml:"tls_auto_self_sign"`
	TLSCertPath     string   `yaml:"tls_cert_path"`
	TLSKeyPath      string   `yaml:"tls_key_path"`
	CORSOrigins     []string `yaml:"cors_allowed_origins"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	SessionDurationHours int    `yaml:"session_duration_hours"`
	JWTSecret            string `yaml:"jwt_secret"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string `yaml:"level"`
	Output string `yaml:"output"`
}

// DatabaseConfig holds SQLite database settings.
type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// AuditConfig holds audit logging settings.
type AuditConfig struct {
	Enabled         bool `yaml:"enabled"`
	RetentionDays   int  `yaml:"retention_days"`
	MaxDetailBytes  int  `yaml:"max_detail_bytes"`
}

// AIChannelConfig holds a single AI provider channel.
type AIChannelConfig struct {
	Name                string `yaml:"name"`
	Provider            string `yaml:"provider"`
	APIKey              string `yaml:"api_key"`
	BaseURL             string `yaml:"base_url"`
	Model               string `yaml:"model"`
	MaxTotalTokens      int    `yaml:"max_total_tokens"`
	MaxCompletionTokens int    `yaml:"max_completion_tokens"`
}

// AIConfig holds AI model configuration.
type AIConfig struct {
	DefaultChannel string                     `yaml:"default_channel"`
	Channels       map[string]AIChannelConfig `yaml:"channels"`
}

// AgentConfig holds agent execution settings.
type AgentConfig struct {
	MaxIterations  int    `yaml:"max_iterations"`
	OutputCapBytes int    `yaml:"output_cap_bytes"`
	WorkspaceDir   string `yaml:"workspace_dir"`
}

// MCPServerConfig holds a single MCP server definition.
type MCPServerConfig struct {
	Transport      string            `yaml:"transport"` // http | stdio | sse
	Command        []string          `yaml:"command"`
	URL            string            `yaml:"url"`
	AllowedTools   []string          `yaml:"allowed_tools"`
	Headers        map[string]string `yaml:"headers"`
	TimeoutSeconds int               `yaml:"timeout_seconds"`
}

// MCPConfig holds global MCP settings and server registry.
type MCPConfig struct {
	WorkerPoolSize      int                        `yaml:"worker_pool_size"`
	CallTimeoutSeconds  int                        `yaml:"call_timeout_seconds"`
	OutputCapBytes      int                        `yaml:"output_cap_bytes"`
	Servers             map[string]MCPServerConfig `yaml:"servers"`
}

// HITLConfig holds human-in-the-loop approval settings.
type HITLConfig struct {
	Enabled                bool   `yaml:"enabled"`
	DefaultMode            string `yaml:"default_mode"`
	ApprovalTimeoutSeconds int    `yaml:"approval_timeout_seconds"`
}

// KnowledgeConfig holds RAG pipeline settings.
type KnowledgeConfig struct {
	Enabled         bool   `yaml:"enabled"`
	VectorStorePath string `yaml:"vector_store_path"`
	EmbeddingModel  string `yaml:"embedding_model"`
	ChunkSize       int    `yaml:"chunk_size"`
	ChunkOverlap    int    `yaml:"chunk_overlap"`
	TopK            int    `yaml:"top_k"`
}

// Load reads and parses a YAML config file, expanding ${ENV_VAR} references.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	expanded := expandEnvVars(string(data))

	cfg := defaults()
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	applyDefaults(cfg)
	return cfg, nil
}

// defaults returns a Config populated with sensible defaults.
func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
		},
		Auth: AuthConfig{
			SessionDurationHours: 12,
		},
		Log: LogConfig{
			Level:  "info",
			Output: "stdout",
		},
		Database: DatabaseConfig{
			Path: "data/kestrel.db",
		},
		Audit: AuditConfig{
			Enabled:        true,
			RetentionDays:  90,
			MaxDetailBytes: 8192,
		},
		Agent: AgentConfig{
			MaxIterations:  15,
			OutputCapBytes: 102400,
			WorkspaceDir:   "tmp/workspace",
		},
		MCP: MCPConfig{
			WorkerPoolSize:     4,
			CallTimeoutSeconds: 120,
			OutputCapBytes:     102400,
		},
		HITL: HITLConfig{
			Enabled:                true,
			DefaultMode:            "auto",
			ApprovalTimeoutSeconds: 300,
		},
		Knowledge: KnowledgeConfig{
			ChunkSize:    512,
			ChunkOverlap: 64,
			TopK:         5,
		},
		RolesDir: "roles",
	}
}

// applyDefaults fills in any computed defaults that require runtime values.
func applyDefaults(cfg *Config) {
	if cfg.Auth.JWTSecret == "" {
		cfg.Auth.JWTSecret = mustGenerateSecret(32)
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = "data/kestrel.db"
	}
	if cfg.Agent.MaxIterations <= 0 {
		cfg.Agent.MaxIterations = 15
	}
	if cfg.MCP.WorkerPoolSize <= 0 {
		cfg.MCP.WorkerPoolSize = 4
	}
	if cfg.MCP.CallTimeoutSeconds <= 0 {
		cfg.MCP.CallTimeoutSeconds = 120
	}
	if cfg.Server.Port <= 0 {
		cfg.Server.Port = 8080
	}
	if cfg.HITL.ApprovalTimeoutSeconds <= 0 {
		cfg.HITL.ApprovalTimeoutSeconds = 300
	}
}

// expandEnvVars replaces ${VAR} and $VAR patterns with their environment values.
func expandEnvVars(s string) string {
	return os.Expand(s, func(key string) string {
		key = strings.TrimSpace(key)
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return ""
	})
}

// mustGenerateSecret generates a cryptographically random hex secret.
func mustGenerateSecret(bytes int) string {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("failed to generate random secret: %v", err))
	}
	return hex.EncodeToString(buf)
}

// Address returns the host:port string for the server listener.
func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// TLSActive returns true if any TLS option is configured.
func (s ServerConfig) TLSActive() bool {
	return s.TLSEnabled || s.TLSAutoSelfSign || (s.TLSCertPath != "" && s.TLSKeyPath != "")
}
