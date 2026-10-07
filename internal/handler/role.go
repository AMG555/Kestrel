package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"kestrel/internal/audit"
	"kestrel/internal/config"
	"kestrel/internal/database"

	"gopkg.in/yaml.v3"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RoleHandler is the role handler
type RoleHandler struct {
	config     *config.Config
	configPath string
	logger     *zap.Logger
	audit      *audit.Service
	db         *database.DB
}

func (h *RoleHandler) SetDB(db *database.DB) { h.db = db }

// SetAudit wires platform audit logging.
func (h *RoleHandler) SetAudit(s *audit.Service) {
	h.audit = s
}

// NewRoleHandler creates a new role handler
func NewRoleHandler(cfg *config.Config, configPath string, logger *zap.Logger) *RoleHandler {
	return &RoleHandler{
		config:     cfg,
		configPath: configPath,
		logger:     logger,
	}
}

// GetRoles returns all roles
func (h *RoleHandler) GetRoles(c *gin.Context) {
	if h.config.Roles == nil {
		h.config.Roles = make(map[string]config.RoleConfig)
	}

	roles := make([]config.RoleConfig, 0, len(h.config.Roles))
	for key, role := range h.config.Roles {
		// ensure the role's key is consistent with name
		if role.Name == "" {
			role.Name = key
		}
		roles = append(roles, role)
	}

	c.JSON(http.StatusOK, gin.H{
		"roles": roles,
	})
}

// GetRole returns a single role
func (h *RoleHandler) GetRole(c *gin.Context) {
	roleName := c.Param("name")
	if roleName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role name cannot be empty"})
		return
	}

	if h.config.Roles == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}

	role, exists := h.config.Roles[roleName]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}

	// ensure the role's name is consistent with the key
	if role.Name == "" {
		role.Name = roleName
	}

	c.JSON(http.StatusOK, gin.H{
		"role": role,
	})
}

// UpdateRole update role
func (h *RoleHandler) UpdateRole(c *gin.Context) {
	roleName := c.Param("name")
	if roleName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role name cannot be empty"})
		return
	}

	var req config.RoleConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters: " + err.Error()})
		return
	}

	// ensure role name is consistent with the name in the request
	if req.Name == "" {
		req.Name = roleName
	}
	if err := h.validateRole(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Name != roleName {
		if _, exists := h.config.Roles[req.Name]; exists {
			c.JSON(http.StatusConflict, gin.H{"error": "role already exists"})
			return
		}
	}

	// initialiseRoles map
	if h.config.Roles == nil {
		h.config.Roles = make(map[string]config.RoleConfig)
	}

	// delete all old roles with the same name but different key (to avoid duplicates)
	// use role name as key to ensure uniqueness
	finalKey := req.Name
	keysToDelete := make([]string, 0)
	for key := range h.config.Roles {
		// if key differs from the final key but name matches, mark for deletion
		if key != finalKey {
			role := h.config.Roles[key]
			// ensure the role's name field is set correctly
			if role.Name == "" {
				role.Name = key
			}
			if role.Name == req.Name {
				keysToDelete = append(keysToDelete, key)
			}
		}
	}
	// delete old roles
	for _, key := range keysToDelete {
		delete(h.config.Roles, key)
		h.logger.Info("delete duplicate role", zap.String("oldKey", key), zap.String("name", req.Name))
	}

	// if the current update key differs from the final key, also delete the old one
	if roleName != finalKey {
		delete(h.config.Roles, roleName)
	}

	// if role name changed, need to delete old file
	if roleName != finalKey {
		configDir := filepath.Dir(h.configPath)
		rolesDir := h.config.RolesDir
		if rolesDir == "" {
			rolesDir = "roles" // default directory
		}

		// If it is a relative path, resolve relative to the configuration file directory
		if !filepath.IsAbs(rolesDir) {
			rolesDir = filepath.Join(configDir, rolesDir)
		}

		// delete old rolesfile
		oldSafeFileName := sanitizeFileName(roleName)
		oldRoleFileYaml := filepath.Join(rolesDir, oldSafeFileName+".yaml")
		oldRoleFileYml := filepath.Join(rolesDir, oldSafeFileName+".yml")

		if _, err := os.Stat(oldRoleFileYaml); err == nil {
			if err := os.Remove(oldRoleFileYaml); err != nil {
				h.logger.Warn("delete old role configuration file failed", zap.String("file", oldRoleFileYaml), zap.Error(err))
			}
		}
		if _, err := os.Stat(oldRoleFileYml); err == nil {
			if err := os.Remove(oldRoleFileYml); err != nil {
				h.logger.Warn("delete old role configuration file failed", zap.String("file", oldRoleFileYml), zap.Error(err))
			}
		}
	}

	// use role name as key to save (ensures uniqueness)
	h.config.Roles[finalKey] = req

	// save config to file
	if err := h.saveConfig(); err != nil {
		h.logger.Error("saveconfigfailed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "saveconfigfailed: " + err.Error()})
		return
	}

	h.logger.Info("update role", zap.String("oldKey", roleName), zap.String("newKey", finalKey), zap.String("name", req.Name))
	if h.audit != nil {
		h.audit.RecordOK(c, "role", "update", "update role", "role", finalKey, map[string]interface{}{"name": req.Name})
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Role updated",
		"role":    req,
	})
}

// CreateRole creates a new role
func (h *RoleHandler) CreateRole(c *gin.Context) {
	var req config.RoleConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters: " + err.Error()})
		return
	}

	if err := h.validateRole(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// initialiseRoles map
	if h.config.Roles == nil {
		h.config.Roles = make(map[string]config.RoleConfig)
	}

	// check if role already exists
	if _, exists := h.config.Roles[req.Name]; exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role already exists"})
		return
	}

	// create role (enabled by default)
	if !req.Enabled {
		req.Enabled = true
	}

	h.config.Roles[req.Name] = req

	// save config to file
	if err := h.saveConfig(); err != nil {
		h.logger.Error("saveconfigfailed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "saveconfigfailed: " + err.Error()})
		return
	}

	h.logger.Info("create role", zap.String("roleName", req.Name))
	if h.audit != nil {
		h.audit.RecordOK(c, "role", "create", "create role", "role", req.Name, nil)
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Role created",
		"role":    req,
	})
}

// DeleteRole delete role
func (h *RoleHandler) DeleteRole(c *gin.Context) {
	roleName := c.Param("name")
	if roleName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role name cannot be empty"})
		return
	}

	if h.config.Roles == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}

	if _, exists := h.config.Roles[roleName]; !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}

	// deletion of "default" role is not allowed
	if roleName == "default" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete the default role"})
		return
	}

	delete(h.config.Roles, roleName)

	// delete the corresponding role file
	configDir := filepath.Dir(h.configPath)
	rolesDir := h.config.RolesDir
	if rolesDir == "" {
		rolesDir = "roles" // default directory
	}

	// If it is a relative path, resolve relative to the configuration file directory
	if !filepath.IsAbs(rolesDir) {
		rolesDir = filepath.Join(configDir, rolesDir)
	}

	// try to delete role file (.yaml and .yml)
	safeFileName := sanitizeFileName(roleName)
	roleFileYaml := filepath.Join(rolesDir, safeFileName+".yaml")
	roleFileYml := filepath.Join(rolesDir, safeFileName+".yml")

	// delete .yaml file (if it exists)
	if _, err := os.Stat(roleFileYaml); err == nil {
		if err := os.Remove(roleFileYaml); err != nil {
			h.logger.Warn("delete roleconfiguration filefailed", zap.String("file", roleFileYaml), zap.Error(err))
		} else {
			h.logger.Info("deleted role configuration file", zap.String("file", roleFileYaml))
		}
	}

	// delete .yml file (if it exists)
	if _, err := os.Stat(roleFileYml); err == nil {
		if err := os.Remove(roleFileYml); err != nil {
			h.logger.Warn("delete roleconfiguration filefailed", zap.String("file", roleFileYml), zap.Error(err))
		} else {
			h.logger.Info("deleted role configuration file", zap.String("file", roleFileYml))
		}
	}

	h.logger.Info("delete role", zap.String("roleName", roleName))
	if h.audit != nil {
		h.audit.RecordOK(c, "role", "delete", "delete role", "role", roleName, nil)
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Role deleted",
	})
}

// saveConfig saves config to files in a directory
func (h *RoleHandler) saveConfig() error {
	configDir := filepath.Dir(h.configPath)
	rolesDir := h.config.RolesDir
	if rolesDir == "" {
		rolesDir = "roles" // default directory
	}

	// If it is a relative path, resolve relative to the configuration file directory
	if !filepath.IsAbs(rolesDir) {
		rolesDir = filepath.Join(configDir, rolesDir)
	}

	// ensure directory exists
	if err := os.MkdirAll(rolesDir, 0755); err != nil {
		return fmt.Errorf("createroles directoryfailed: %w", err)
	}

	// save each role to a separate file
	if h.config.Roles != nil {
		for roleName, role := range h.config.Roles {
			// ensure role name is set correctly
			if role.Name == "" {
				role.Name = roleName
			}

			// use role name as filename (sanitize filename to avoid special characters)
			safeFileName := sanitizeFileName(role.Name)
			roleFile := filepath.Join(rolesDir, safeFileName+".yaml")

			// serialize role config to YAML
			roleData, err := yaml.Marshal(&role)
			if err != nil {
				h.logger.Error("serialize role config failed", zap.String("role", roleName), zap.Error(err))
				continue
			}

			// handle icon field: ensure icon values containing \U are surrounded by quotes (YAML requires quotes to correctly parse Unicode escapes)
			roleDataStr := string(roleData)
			if role.Icon != "" && strings.HasPrefix(role.Icon, "\\U") {
				// match icon: \UXXXXXXXX format (without quotes), excluding cases that already have quotes
				// use negative lookahead to ensure no following quotes, or match directly when no quotes present
				re := regexp.MustCompile(`(?m)^(icon:\s+)(\\U[0-9A-F]{8})(\s*)$`)
				roleDataStr = re.ReplaceAllString(roleDataStr, `${1}"${2}"${3}`)
				roleData = []byte(roleDataStr)
			}

			// write file
			if err := os.WriteFile(roleFile, roleData, 0644); err != nil {
				h.logger.Error("failed to save role configuration file", zap.String("role", roleName), zap.String("file", roleFile), zap.Error(err))
				continue
			}

			h.logger.Info("role config saved to file", zap.String("role", roleName), zap.String("file", roleFile))
		}
	}

	return nil
}

// sanitizeFileName converts a role name to a safe filename.
func sanitizeFileName(name string) string {
	// Replace potentially unsafe characters.
	replacer := map[rune]string{
		'/':  "_",
		'\\': "_",
		':':  "_",
		'*':  "_",
		'?':  "_",
		'"':  "_",
		'<':  "_",
		'>':  "_",
		'|':  "_",
		' ':  "_",
	}

	var result []rune
	for _, r := range name {
		if replacement, ok := replacer[r]; ok {
			result = append(result, []rune(replacement)...)
		} else {
			result = append(result, r)
		}
	}

	fileName := string(result)
	// If filename is empty, use the default name.
	if fileName == "" {
		fileName = "role"
	}

	return fileName
}

// updateRolesConfig update roleconfig
func updateRolesConfig(doc *yaml.Node, cfg config.RolesConfig) {
	root := doc.Content[0]
	rolesNode := ensureMap(root, "roles")

	// Clear existing roles.
	if rolesNode.Kind == yaml.MappingNode {
		rolesNode.Content = nil
	}

	// Add new roles (using name as key to ensure uniqueness).
	if cfg.Roles != nil {
		// First build a map keyed by name for deduplication (keep the last one).
		rolesByName := make(map[string]config.RoleConfig)
		for roleKey, role := range cfg.Roles {
			// ensure the role's name field is set correctly
			if role.Name == "" {
				role.Name = roleKey
			}
			// Use name as the final key; if multiple keys map to the same name, keep only the last.
			rolesByName[role.Name] = role
		}

		// Write deduplicated roles to YAML.
		for roleName, role := range rolesByName {
			roleNode := ensureMap(rolesNode, roleName)
			setStringInMap(roleNode, "name", role.Name)
			setStringInMap(roleNode, "description", role.Description)
			setStringInMap(roleNode, "user_prompt", role.UserPrompt)
			if role.Icon != "" {
				setStringInMap(roleNode, "icon", role.Icon)
			}
			setBoolInMap(roleNode, "enabled", role.Enabled)

			// Add tool list (prefer the tools field).
			if len(role.Tools) > 0 {
				toolsNode := ensureArray(roleNode, "tools")
				toolsNode.Content = nil
				for _, toolKey := range role.Tools {
					toolNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: toolKey}
					toolsNode.Content = append(toolsNode.Content, toolNode)
				}
			} else if len(role.MCPs) > 0 {
				// Backward compatibility: if no tools but mcps exists, save mcps.
				mcpsNode := ensureArray(roleNode, "mcps")
				mcpsNode.Content = nil
				for _, mcpName := range role.MCPs {
					mcpNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: mcpName}
					mcpsNode.Content = append(mcpsNode.Content, mcpNode)
				}
			}
		}
	}
}

// ensureArray ensures an array node with the specified key exists in the map.
func ensureArray(parent *yaml.Node, key string) *yaml.Node {
	_, valueNode := ensureKeyValue(parent, key)
	if valueNode.Kind != yaml.SequenceNode {
		valueNode.Kind = yaml.SequenceNode
		valueNode.Tag = "!!seq"
		valueNode.Content = nil
	}
	return valueNode
}
