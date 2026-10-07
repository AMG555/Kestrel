package handler

import (
	"fmt"
	"net/http"
	"os"
	"sync"

	"kestrel/internal/audit"
	"kestrel/internal/config"
	"kestrel/internal/mcp"
	"kestrel/internal/security"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// ExternalMCPHandler is the external MCP handler
type ExternalMCPHandler struct {
	manager    *mcp.ExternalMCPManager
	config     *config.Config
	configPath string
	logger     *zap.Logger
	audit      *audit.Service
	mu         sync.RWMutex
}

// SetAudit wires platform audit logging.
func (h *ExternalMCPHandler) SetAudit(s *audit.Service) {
	h.audit = s
}

// NewExternalMCPHandler creates an external MCP handler
func NewExternalMCPHandler(manager *mcp.ExternalMCPManager, cfg *config.Config, configPath string, logger *zap.Logger) *ExternalMCPHandler {
	return &ExternalMCPHandler{
		manager:    manager,
		config:     cfg,
		configPath: configPath,
		logger:     logger,
	}
}

// GetExternalMCPs returns all external MCP configurations
func (h *ExternalMCPHandler) GetExternalMCPs(c *gin.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	configs := h.manager.GetConfigs()

	// get tool counts for all external MCPs
	toolCounts := h.manager.GetToolCounts()

	// convert to response format
	result := make(map[string]ExternalMCPResponse)
	for name, cfg := range configs {
		client, exists := h.manager.GetClient(name)
		status := "disconnected"
		if exists {
			status = client.GetStatus()
		} else if h.isEnabled(cfg) {
			status = "disconnected"
		} else {
			status = "disabled"
		}

		toolCount := toolCounts[name]
		errorMsg := externalMCPStatusError(h.manager, name, status)

		result[name] = ExternalMCPResponse{
			Config:    externalMCPConfigForResponse(c, cfg),
			Status:    status,
			ToolCount: toolCount,
			Error:     errorMsg,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"servers": result,
		"stats":   h.manager.GetStats(),
	})
}

// GetExternalMCP returns a single external MCP configuration
func (h *ExternalMCPHandler) GetExternalMCP(c *gin.Context) {
	name := c.Param("name")

	h.mu.RLock()
	defer h.mu.RUnlock()

	configs := h.manager.GetConfigs()
	cfg, exists := configs[name]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "external MCP configuration not found"})
		return
	}

	client, clientExists := h.manager.GetClient(name)
	status := "disconnected"
	if clientExists {
		status = client.GetStatus()
	} else if h.isEnabled(cfg) {
		status = "disconnected"
	} else {
		status = "disabled"
	}

	// get tool count
	toolCount := 0
	if clientExists && client.IsConnected() {
		if count, err := h.manager.GetToolCount(name); err == nil {
			toolCount = count
		}
	}

	c.JSON(http.StatusOK, ExternalMCPResponse{
		Config:    externalMCPConfigForResponse(c, cfg),
		Status:    status,
		ToolCount: toolCount,
		Error:     externalMCPStatusError(h.manager, name, status),
	})
}

func externalMCPConfigForResponse(c *gin.Context, cfg config.ExternalMCPServerConfig) config.ExternalMCPServerConfig {
	if security.SessionHasPermission(c, "mcp:write") {
		return cfg
	}
	copyCfg := cfg
	if len(cfg.Env) > 0 {
		copyCfg.Env = make(map[string]string, len(cfg.Env))
		for key := range cfg.Env {
			copyCfg.Env[key] = "***"
		}
	}
	if len(cfg.Headers) > 0 {
		copyCfg.Headers = make(map[string]string, len(cfg.Headers))
		for key := range cfg.Headers {
			copyCfg.Headers[key] = "***"
		}
	}
	return copyCfg
}

// externalMCPStatusError returns the most recent error (including disconnect reason) when status is error or disconnected.
func externalMCPStatusError(manager *mcp.ExternalMCPManager, name, status string) string {
	if status != "error" && status != "disconnected" {
		return ""
	}
	return manager.GetError(name)
}

// AddOrUpdateExternalMCP adds or updates an external MCP configuration
func (h *ExternalMCPHandler) AddOrUpdateExternalMCP(c *gin.Context) {
	var req AddOrUpdateExternalMCPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters: " + err.Error()})
		return
	}

	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name cannot be empty"})
		return
	}

	// validateconfig
	if err := h.validateConfig(req.Config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// add or update configuration
	if err := h.manager.AddOrUpdateConfig(name, req.Config); err != nil {
		h.logger.Error("add or update external MCP configuration failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "add or update configuration failed: " + err.Error()})
		return
	}

	// update in-memory config
	if h.config.ExternalMCP.Servers == nil {
		h.config.ExternalMCP.Servers = make(map[string]config.ExternalMCPServerConfig)
	}

	cfg := req.Config

	// official disabled field → invert ExternalMCPEnable
	if cfg.Disabled {
		cfg.ExternalMCPEnable = false
	} else if !cfg.ExternalMCPEnable {
		// user did not explicitly set external_mcp_enable; official config defaults to enabled
		cfg.ExternalMCPEnable = true
	}

	// expand ${VAR} environment variables
	config.ExpandConfigEnv(&cfg)

	h.config.ExternalMCP.Servers[name] = cfg

	// save to configuration file
	if err := h.saveConfig(); err != nil {
		h.logger.Error("saveconfigfailed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "saveconfigfailed: " + err.Error()})
		return
	}

	h.logger.Info("external MCP configuration updated", zap.String("name", name))
	if h.audit != nil {
		h.audit.Record(c, audit.Entry{
			Category:     "external_mcp",
			Action:       "upsert",
			Result:       "success",
			ResourceType: "external_mcp",
			ResourceID:   name,
			Message:      "update external MCP config",
		})
	}
	c.JSON(http.StatusOK, gin.H{"message": "config updated"})
}

// DeleteExternalMCP deleteexternal MCP configuration
func (h *ExternalMCPHandler) DeleteExternalMCP(c *gin.Context) {
	name := c.Param("name")

	h.mu.Lock()
	defer h.mu.Unlock()

	// remove config
	if err := h.manager.RemoveConfig(name); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "config not found"})
		return
	}

	// delete from in-memory config
	if h.config.ExternalMCP.Servers != nil {
		delete(h.config.ExternalMCP.Servers, name)
	}

	// save to configuration file
	if err := h.saveConfig(); err != nil {
		h.logger.Error("saveconfigfailed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "saveconfigfailed: " + err.Error()})
		return
	}

	h.logger.Info("external MCP configurationdeleted", zap.String("name", name))
	if h.audit != nil {
		h.audit.Record(c, audit.Entry{
			Category:     "external_mcp",
			Action:       "delete",
			Result:       "success",
			ResourceType: "external_mcp",
			ResourceID:   name,
			Message:      "delete external MCP config",
		})
	}
	c.JSON(http.StatusOK, gin.H{"message": "configdeleted"})
}

// StartExternalMCP starts an external MCP
func (h *ExternalMCPHandler) StartExternalMCP(c *gin.Context) {
	name := c.Param("name")

	h.mu.Lock()
	defer h.mu.Unlock()

	// update configuration to enabled
	if h.config.ExternalMCP.Servers == nil {
		h.config.ExternalMCP.Servers = make(map[string]config.ExternalMCPServerConfig)
	}
	cfg := h.config.ExternalMCP.Servers[name]
	cfg.ExternalMCPEnable = true
	h.config.ExternalMCP.Servers[name] = cfg

	// save to configuration file
	if err := h.saveConfig(); err != nil {
		h.logger.Error("saveconfigfailed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "saveconfigfailed: " + err.Error()})
		return
	}

	// start client (immediately create client and set status to connecting; actual connection runs in background)
	h.logger.Info("starting external MCP", zap.String("name", name))
	if err := h.manager.StartClient(name); err != nil {
		h.logger.Error("start external MCP failed", zap.String("name", name), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  err.Error(),
			"status": "error",
		})
		return
	}

	// get client status (should be connecting)
	client, exists := h.manager.GetClient(name)
	status := "connecting"
	if exists {
		status = client.GetStatus()
	}

	// return immediately without waiting for connection to complete
	// the client connects asynchronously in the background; use the status endpoint to check connection status
	c.JSON(http.StatusOK, gin.H{
		"message": "external MCP start request submitted, connecting in background",
		"status":  status,
	})
}

// StopExternalMCP stops an external MCP
func (h *ExternalMCPHandler) StopExternalMCP(c *gin.Context) {
	name := c.Param("name")

	h.mu.Lock()
	defer h.mu.Unlock()

	// stop client
	if err := h.manager.StopClient(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// update configuration
	if h.config.ExternalMCP.Servers == nil {
		h.config.ExternalMCP.Servers = make(map[string]config.ExternalMCPServerConfig)
	}
	cfg := h.config.ExternalMCP.Servers[name]
	cfg.ExternalMCPEnable = false
	h.config.ExternalMCP.Servers[name] = cfg

	// save to configuration file
	if err := h.saveConfig(); err != nil {
		h.logger.Error("saveconfigfailed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "saveconfigfailed: " + err.Error()})
		return
	}

	h.logger.Info("external MCP stopped", zap.String("name", name))
	c.JSON(http.StatusOK, gin.H{"message": "external MCP stopped"})
}

// GetExternalMCPStats returns statistics info
func (h *ExternalMCPHandler) GetExternalMCPStats(c *gin.Context) {
	stats := h.manager.GetStats()
	c.JSON(http.StatusOK, stats)
}

// validateConfig validates the config (supports both the official type field and legacy transport field).
func (h *ExternalMCPHandler) validateConfig(cfg config.ExternalMCPServerConfig) error {
	transport := cfg.GetTransportType()
	if transport == "" {
		return fmt.Errorf("must specify command (stdio mode) or url + type (http/sse mode)")
	}

	switch transport {
	case "http":
		if cfg.URL == "" {
			return fmt.Errorf("HTTP mode requires url")
		}
	case "stdio":
		if cfg.Command == "" {
			return fmt.Errorf("stdio mode requires command")
		}
	case "sse":
		if cfg.URL == "" {
			return fmt.Errorf("SSE mode requires url")
		}
	default:
		return fmt.Errorf("unsupported transport mode: %s, supported modes: http, stdio, sse", transport)
	}

	return nil
}

// isEnabled checkenabled
func (h *ExternalMCPHandler) isEnabled(cfg config.ExternalMCPServerConfig) bool {
	return cfg.ExternalMCPEnable
}

// saveConfig saves the config to a file.
func (h *ExternalMCPHandler) saveConfig() error {
	configFileMu.Lock()
	defer configFileMu.Unlock()
	data, err := os.ReadFile(h.configPath)
	if err != nil {
		return fmt.Errorf("failed to read configuration file: %w", err)
	}

	if err := os.WriteFile(h.configPath+".backup", data, 0644); err != nil {
		h.logger.Warn("failed to create config backup", zap.Error(err))
	}

	root, err := loadYAMLDocument(h.configPath)
	if err != nil {
		return fmt.Errorf("failed to parse configuration file: %w", err)
	}

	updateExternalMCPConfig(root, h.config.ExternalMCP)

	if err := writeYAMLDocument(h.configPath, root); err != nil {
		return fmt.Errorf("saveconfiguration filefailed: %w", err)
	}

	h.logger.Info("config saved", zap.String("path", h.configPath))
	return nil
}

// updateExternalMCPConfig updateexternal MCP configuration
func updateExternalMCPConfig(doc *yaml.Node, cfg config.ExternalMCPConfig) {
	root := doc.Content[0]
	externalMCPNode := ensureMap(root, "external_mcp")
	serversNode := ensureMap(externalMCPNode, "servers")

	// Clear existing server configuration.
	serversNode.Content = nil

	// Add new server configuration.
	for name, serverCfg := range cfg.Servers {
		nameNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name}
		serverNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		serversNode.Content = append(serversNode.Content, nameNode, serverNode)

		// type (official MCP transport type)
		effectiveType := serverCfg.GetTransportType()
		if effectiveType != "" && effectiveType != "stdio" {
			// stdio can be omitted (inferred automatically when command is present)
			setStringInMap(serverNode, "type", effectiveType)
		}
		if serverCfg.Command != "" {
			setStringInMap(serverNode, "command", serverCfg.Command)
		}
		if len(serverCfg.Args) > 0 {
			setStringArrayInMap(serverNode, "args", serverCfg.Args)
		}
		if serverCfg.Env != nil && len(serverCfg.Env) > 0 {
			envNode := ensureMap(serverNode, "env")
			for envKey, envValue := range serverCfg.Env {
				setStringInMap(envNode, envKey, envValue)
			}
		}
		if serverCfg.URL != "" {
			setStringInMap(serverNode, "url", serverCfg.URL)
		}
		if serverCfg.Headers != nil && len(serverCfg.Headers) > 0 {
			headersNode := ensureMap(serverNode, "headers")
			for k, v := range serverCfg.Headers {
				setStringInMap(headersNode, k, v)
			}
		}
		if serverCfg.Description != "" {
			setStringInMap(serverNode, "description", serverCfg.Description)
		}
		if serverCfg.Timeout > 0 {
			setIntInMap(serverNode, "timeout", serverCfg.Timeout)
		}
		// Official standard fields
		if serverCfg.Disabled {
			setBoolInMap(serverNode, "disabled", true)
		}
		if len(serverCfg.AutoApprove) > 0 {
			setStringArrayInMap(serverNode, "autoApprove", serverCfg.AutoApprove)
		}

		// SDK advanced config
		if serverCfg.MaxRetries > 0 {
			setIntInMap(serverNode, "max_retries", serverCfg.MaxRetries)
		}
		if serverCfg.TerminateDuration > 0 {
			setIntInMap(serverNode, "terminate_duration", serverCfg.TerminateDuration)
		}
		if serverCfg.KeepAlive > 0 {
			setIntInMap(serverNode, "keep_alive", serverCfg.KeepAlive)
		}

		setBoolInMap(serverNode, "external_mcp_enable", serverCfg.ExternalMCPEnable)
		if serverCfg.ToolEnabled != nil && len(serverCfg.ToolEnabled) > 0 {
			toolEnabledNode := ensureMap(serverNode, "tool_enabled")
			for toolName, enabled := range serverCfg.ToolEnabled {
				setBoolInMap(toolEnabledNode, toolName, enabled)
			}
		}
	}
}

// setStringArrayInMap settingsstringarray
func setStringArrayInMap(mapNode *yaml.Node, key string, values []string) {
	_, valueNode := ensureKeyValue(mapNode, key)
	valueNode.Kind = yaml.SequenceNode
	valueNode.Tag = "!!seq"
	valueNode.Content = nil
	for _, v := range values {
		itemNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
		valueNode.Content = append(valueNode.Content, itemNode)
	}
}

// AddOrUpdateExternalMCPRequest is the request to add or update an external MCP server.
type AddOrUpdateExternalMCPRequest struct {
	Config config.ExternalMCPServerConfig `json:"config"`
}

// ExternalMCPResponse is the external MCP response.
type ExternalMCPResponse struct {
	Config    config.ExternalMCPServerConfig `json:"config"`
	Status    string                         `json:"status"`          // "connected", "disconnected", "disabled", "error", "connecting"
	ToolCount int                            `json:"tool_count"`      // Number of tools.
	Error     string                         `json:"error,omitempty"` // Error info (only present when status is error).
}
