package handler

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"kestrel/internal/audit"
	"kestrel/internal/authctx"
	"kestrel/internal/config"
	"kestrel/internal/database"
	"kestrel/internal/security"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	robotCmdHelp          = "help"
	robotCmdList          = "list"
	robotCmdListAlt       = "conversationlist"
	robotCmdSwitch        = "switch"
	robotCmdContinue      = "continue"
	robotCmdNew           = "new"
	robotCmdClear         = "clear"
	robotCmdStatus        = "status"
	robotCmdStop          = "stop"
	robotCmdRoles         = "roles"
	robotCmdRolesList     = "rolelist"
	robotCmdSwitchRole    = "switchrole"
	robotCmdModes         = "modes"
	robotCmdModesList     = "modeslist"
	robotCmdSwitchMode    = "switchmode"
	robotCmdDelete        = "delete"
	robotCmdVersion       = "version"
	robotCmdProjects      = "projects"
	robotCmdProjectsList  = "projectslist"
	robotCmdBindProject   = "bindproject"
	robotCmdNewProject    = "newproject"
	robotCmdUnbindProject = "unbindproject"
	robotCmdBindUser      = "bind"
	robotCmdUnbindUser    = "unbind"
	robotCmdIdentity      = "identity"
	robotCmdTask          = "task"
	robotCmdRename        = "rename"
	robotCmdPermissions   = "permissions"
	robotCmdDoctor        = "doctor"
	robotCmdConfirm       = "confirm"
	robotCmdCancel        = "cancel"
	robotCmdVulnAlerts    = "vulnalerts"
	robotBindingCodeTTL   = 5 * time.Minute
)

type robotPendingConfirmation struct {
	Action    string
	Target    string
	ExpiresAt time.Time
}

// RobotHandler handles WeCom/DingTalk/Feishu and other bot callbacks
type RobotHandler struct {
	config               *config.Config
	db                   *database.DB
	agentHandler         *AgentHandler
	logger               *zap.Logger
	mu                   sync.RWMutex
	sessions             map[string]string             // key: "platform_userID", value: conversationID
	sessionRoles         map[string]string             // key: "platform_userID", value: roleName (default "default")
	sessionModes         map[string]string             // key: "platform_userID", value: agent mode
	cancelMu             sync.Mutex                    // protects runningCancels
	runningCancels       map[string]context.CancelFunc // key: "platform_userID", used to interrupt tasks via stop command
	wecomReplay          map[string]time.Time
	pendingConfirmations map[string]robotPendingConfirmation
	alertWake            chan struct{}
	audit                *audit.Service
}

// NewRobotHandler creates a bot handler
func NewRobotHandler(cfg *config.Config, db *database.DB, agentHandler *AgentHandler, logger *zap.Logger) *RobotHandler {
	return &RobotHandler{
		config:               cfg,
		db:                   db,
		agentHandler:         agentHandler,
		logger:               logger,
		sessions:             make(map[string]string),
		sessionRoles:         make(map[string]string),
		sessionModes:         make(map[string]string),
		runningCancels:       make(map[string]context.CancelFunc),
		wecomReplay:          make(map[string]time.Time),
		pendingConfirmations: make(map[string]robotPendingConfirmation),
		alertWake:            make(chan struct{}, 1),
	}
}

func (h *RobotHandler) SetAudit(s *audit.Service) {
	h.audit = s
}

func (h *RobotHandler) acceptFreshWecomRequest(timestamp, nonce, signature string) bool {
	unixSeconds, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil {
		return false
	}
	now := time.Now()
	requestTime := time.Unix(unixSeconds, 0)
	if requestTime.Before(now.Add(-5*time.Minute)) || requestTime.After(now.Add(5*time.Minute)) {
		return false
	}
	key := strings.TrimSpace(timestamp) + "\x00" + strings.TrimSpace(nonce) + "\x00" + strings.TrimSpace(signature)
	if strings.TrimSpace(nonce) == "" || strings.TrimSpace(signature) == "" {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for replayKey, seenAt := range h.wecomReplay {
		if now.Sub(seenAt) > 10*time.Minute {
			delete(h.wecomReplay, replayKey)
		}
	}
	if _, exists := h.wecomReplay[key]; exists {
		return false
	}
	h.wecomReplay[key] = now
	return true
}

// sessionKey generates a session key
func (h *RobotHandler) sessionKey(platform, userID string) string {
	return platform + "_" + userID
}

func normalizeRobotBindingCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
}

func hashRobotBindingCode(code string) string {
	sum := sha256.Sum256([]byte(normalizeRobotBindingCode(code)))
	return fmt.Sprintf("%x", sum[:])
}

func (h *RobotHandler) resolveRobotAccess(platform, userID string) (*database.RBACAccess, error) {
	if h.db == nil {
		return nil, fmt.Errorf("robot authentication service unavailable")
	}
	authorization := h.config.Robots.AuthorizationFor(platform)
	var access *database.RBACAccess
	var err error
	switch authorization.EffectiveMode() {
	case config.RobotAuthModeUserBinding:
		access, err = h.db.ResolveRobotRBACAccess(platform, userID)
	case config.RobotAuthModeServiceAccount:
		if !authorization.ExternalUserAllowed(userID) {
			return nil, fmt.Errorf("robot sender is not in the service account allowlist")
		}
		access, err = h.db.ResolveRBACAccess(strings.TrimSpace(authorization.ServiceUserID))
	default:
		return nil, fmt.Errorf("robot authentication mode is invalid")
	}
	if err != nil {
		return nil, err
	}
	if !access.User.Enabled {
		return nil, fmt.Errorf("bound platform account has been disabled")
	}
	return access, nil
}

func robotPrincipal(access *database.RBACAccess) authctx.Principal {
	if access == nil {
		return authctx.Principal{}
	}
	return authctx.NewPrincipalWithScopes(access.User.ID, access.User.Username, access.Scope, access.Permissions, access.PermissionScopes)
}

func (h *RobotHandler) robotAccessDeniedMessage(platform string) string {
	if h.config.Robots.AuthorizationFor(platform).EffectiveMode() == config.RobotAuthModeServiceAccount {
		return "current platform account is not in the robot service account allowlist, or the service account is unavailable."
	}
	return "current platform account is not yet bound to a Kestrel user. Please generate a bind code on the web interface first, then send: bind XXXX-XXXX"
}

func (h *RobotHandler) loadSessionBinding(sk string) (convID, role, agentMode string) {
	if h.db == nil || strings.TrimSpace(sk) == "" {
		return "", "", ""
	}
	binding, err := h.db.GetRobotSessionBinding(sk)
	if err != nil {
		h.logger.Warn("failed to read robot session binding", zap.String("session_key", sk), zap.Error(err))
		return "", "", ""
	}
	if binding == nil {
		return "", "", ""
	}
	return binding.ConversationID, binding.RoleName, binding.AgentMode
}

func (h *RobotHandler) persistSessionBinding(sk, convID, role, agentMode string) {
	if h.db == nil || strings.TrimSpace(sk) == "" || strings.TrimSpace(convID) == "" {
		return
	}
	if err := h.db.UpsertRobotSessionBinding(sk, convID, role, agentMode); err != nil {
		h.logger.Warn("failed to write robot session binding", zap.String("session_key", sk), zap.Error(err))
	}
}

func (h *RobotHandler) deleteSessionBinding(sk string) {
	if h.db == nil || strings.TrimSpace(sk) == "" {
		return
	}
	if err := h.db.DeleteRobotSessionBinding(sk); err != nil {
		h.logger.Warn("failed to delete robot session binding", zap.String("session_key", sk), zap.Error(err))
	}
}

// getOrCreateConversation gets or creates the current session; title is used for new conversation title (first 50 chars of user's first message)
func (h *RobotHandler) getOrCreateConversation(platform, userID, title string, access *database.RBACAccess) (convID string, isNew bool) {
	sk := h.sessionKey(platform, userID)
	h.mu.RLock()
	convID = h.sessions[sk]
	h.mu.RUnlock()
	ownerID := access.User.ID
	readScope := robotPrincipal(access).ScopeFor("chat:read")
	if convID != "" && access.Permissions["chat:read"] && h.db.UserCanAccessResource(ownerID, readScope, "conversation", convID) {
		return convID, false
	}
	if persistedConvID, persistedRole, persistedMode := h.loadSessionBinding(sk); strings.TrimSpace(persistedConvID) != "" {
		if !access.Permissions["chat:read"] || !h.db.UserCanAccessResource(ownerID, readScope, "conversation", persistedConvID) {
			h.deleteSessionBinding(sk)
		} else {
			// session binding persistence: current conversation and role can be resumed after service restart.
			h.mu.Lock()
			h.sessions[sk] = persistedConvID
			if strings.TrimSpace(persistedRole) != "" {
				h.sessionRoles[sk] = persistedRole
			}
			if strings.TrimSpace(persistedMode) != "" {
				h.sessionModes[sk] = config.NormalizeAgentMode(persistedMode)
			}
			h.mu.Unlock()
			return persistedConvID, false
		}
	}
	t := strings.TrimSpace(title)
	if t == "" {
		t = "new conversation " + time.Now().Format("01-02 15:04")
	} else {
		t = safeTruncateString(t, 50)
	}
	meta := database.ConversationCreateMeta{Source: "robot:" + platform}
	if !access.Permissions["chat:write"] {
		return "", false
	}
	meta.ProjectID = effectiveProjectID(h.config, "")
	if meta.ProjectID != "" && (!access.Permissions["project:read"] || !h.db.UserCanAccessResource(ownerID, robotPrincipal(access).ScopeFor("project:read"), "project", meta.ProjectID)) {
		meta.ProjectID = ""
	}
	conv, err := h.db.CreateConversation(t, meta)
	if err != nil {
		h.logger.Warn("failed to create robot session", zap.Error(err))
		return "", false
	}
	convID = conv.ID
	_ = h.db.SetResourceOwner("conversation", convID, ownerID)
	h.mu.Lock()
	role := h.sessionRoles[sk]
	agentMode := h.sessionModes[sk]
	h.sessions[sk] = convID
	h.mu.Unlock()
	if agentMode == "" {
		agentMode = config.NormalizeRobotAgentMode(h.config.MultiAgent)
	}
	h.persistSessionBinding(sk, convID, role, agentMode)
	return convID, true
}

// setConversation switches the current session
func (h *RobotHandler) setConversation(platform, userID, convID string) {
	sk := h.sessionKey(platform, userID)
	h.mu.Lock()
	role := h.sessionRoles[sk]
	agentMode := h.sessionModes[sk]
	h.sessions[sk] = convID
	h.mu.Unlock()
	h.persistSessionBinding(sk, convID, role, agentMode)
}

// getRole gets the role used by the current user; returns "default" if not set
func (h *RobotHandler) getRole(platform, userID string) string {
	sk := h.sessionKey(platform, userID)
	h.mu.RLock()
	role := h.sessionRoles[sk]
	h.mu.RUnlock()
	if strings.TrimSpace(role) != "" {
		return role
	}
	if _, persistedRole, _ := h.loadSessionBinding(sk); strings.TrimSpace(persistedRole) != "" {
		h.mu.Lock()
		h.sessionRoles[sk] = persistedRole
		h.mu.Unlock()
		return persistedRole
	}
	return "default"
}

// setRole sets the role used by the current user
func (h *RobotHandler) setRole(platform, userID, roleName string) {
	sk := h.sessionKey(platform, userID)
	h.mu.Lock()
	h.sessionRoles[sk] = roleName
	convID := h.sessions[sk]
	agentMode := h.sessionModes[sk]
	h.mu.Unlock()
	h.persistSessionBinding(sk, convID, roleName, agentMode)
}

func (h *RobotHandler) getAgentMode(platform, userID string) string {
	sk := h.sessionKey(platform, userID)
	h.mu.RLock()
	mode := h.sessionModes[sk]
	h.mu.RUnlock()
	if mode != "" {
		return config.NormalizeAgentMode(mode)
	}
	if _, _, persistedMode := h.loadSessionBinding(sk); persistedMode != "" {
		mode = config.NormalizeAgentMode(persistedMode)
		h.mu.Lock()
		h.sessionModes[sk] = mode
		h.mu.Unlock()
		return mode
	}
	return config.NormalizeRobotAgentMode(h.config.MultiAgent)
}

func (h *RobotHandler) setAgentMode(platform, userID, mode string) {
	sk := h.sessionKey(platform, userID)
	mode = config.NormalizeAgentMode(mode)
	h.mu.Lock()
	h.sessionModes[sk] = mode
	convID := h.sessions[sk]
	role := h.sessionRoles[sk]
	h.mu.Unlock()
	h.persistSessionBinding(sk, convID, role, mode)
}

// clearConversation clears the current session (switches to a new conversation)
func (h *RobotHandler) clearConversation(platform, userID string, access *database.RBACAccess) (newConvID string) {
	title := "new conversation " + time.Now().Format("01-02 15:04")
	meta := database.ConversationCreateMeta{Source: "robot:" + platform + ":new"}
	meta.ProjectID = effectiveProjectID(h.config, "")
	ownerID := access.User.ID
	if meta.ProjectID != "" && (!access.Permissions["project:read"] || !h.db.UserCanAccessResource(ownerID, robotPrincipal(access).ScopeFor("project:read"), "project", meta.ProjectID)) {
		meta.ProjectID = ""
	}
	conv, err := h.db.CreateConversation(title, meta)
	if err != nil {
		h.logger.Warn("failed to create new conversation", zap.Error(err))
		return ""
	}
	_ = h.db.SetResourceOwner("conversation", conv.ID, ownerID)
	h.setConversation(platform, userID, conv.ID)
	return conv.ID
}

// HandleMessage processes user input and returns reply text (called by platform webhooks)
func (h *RobotHandler) HandleMessage(platform, userID, text string) (reply string) {
	platform = strings.TrimSpace(platform)
	userID = strings.TrimSpace(userID)
	text = strings.TrimSpace(text)
	if platform == "" {
		platform = "unknown"
	}
	if userID == "" {
		h.logger.Warn("robot message missing user identifier, rejected", zap.String("platform", platform))
		return "cannot identify sender identity, please check robot event subscription permissions (must return a valid user ID)."
	}
	if text == "" {
		return `Please enter content or send "help" / help to view commands.`
	}

	// first try to handle as a command (supports Chinese and English)
	if cmdReply, ok := h.handleRobotCommand(platform, userID, text); ok {
		return cmdReply
	}
	access, err := h.resolveRobotAccess(platform, userID)
	if err != nil {
		return h.robotAccessDeniedMessage(platform)
	}
	if !access.Permissions["agent:execute"] || !access.Permissions["chat:read"] || !access.Permissions["chat:write"] {
		return "insufficient permissions: robot conversation requires agent:execute, chat:read, and chat:write permissions."
	}
	if h.audit != nil && h.config.Robots.AuthorizationFor(platform).EffectiveMode() == config.RobotAuthModeServiceAccount {
		hint := sha256.Sum256([]byte(userID))
		h.audit.RecordSystem(audit.Entry{
			Category: "robot", Action: "service_account_execute", Result: "success", Actor: access.User.Username,
			ResourceType: "robot_sender", ResourceID: platform + ":" + fmt.Sprintf("%x", hint[:4]),
			Message: "allowlisted platform sender executing Agent using robot service account",
		})
	}

	// normal message: go through Agent
	convID, _ := h.getOrCreateConversation(platform, userID, text, access)
	if convID == "" {
		return "cannot create or get conversation, please try again later."
	}
	// if conversation title is 'new conversation xx:xx' format (created by 'new conversation' command), update title to first message content, consistent with Web experience
	if conv, err := h.db.GetConversation(convID); err == nil && strings.HasPrefix(conv.Title, "new conversation ") {
		newTitle := safeTruncateString(text, 50)
		if newTitle != "" {
			_ = h.db.UpdateConversationTitle(convID, newTitle)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.robotMessageTimeout())
	sk := h.sessionKey(platform, userID)
	h.cancelMu.Lock()
	h.runningCancels[sk] = cancel
	h.cancelMu.Unlock()
	defer func() {
		cancel()
		h.cancelMu.Lock()
		delete(h.runningCancels, sk)
		h.cancelMu.Unlock()
	}()
	role := h.getRole(platform, userID)
	agentMode := h.getAgentMode(platform, userID)
	resp, newConvID, err := h.agentHandler.ProcessMessageForRobot(ctx, platform, robotPrincipal(access), convID, text, role, agentMode)
	if err != nil {
		h.logger.Warn("robot Agent execution failed", zap.String("platform", platform), zap.String("userID", userID), zap.Error(err))
		if errors.Is(err, context.Canceled) {
			return "task cancelled."
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return "task execution timed out, please try again later or narrow the scope of the request."
		}
		return "processing failed: " + err.Error()
	}
	if newConvID != convID {
		h.setConversation(platform, userID, newConvID)
	}
	return resp
}

func (h *RobotHandler) robotMessageTimeout() time.Duration {
	// robot full message processing timeout (decoupled from single tool timeout agent.tool_timeout_minutes).
	return 10 * time.Hour
}

func (h *RobotHandler) cmdHelp(platform, userID string) string {
	access, _ := h.resolveRobotAccess(platform, userID)
	can := func(permission string) bool {
		return access != nil && access.Permissions[permission]
	}
	var b strings.Builder
	b.WriteString("[Kestrel Robot Commands]\n\n")
	b.WriteString("[General]\n")
	b.WriteString("· help / help — show this help\n")
	b.WriteString("· version / version — show current version\n")
	b.WriteString("· bind <bind code> / bind <code> — bind RBAC user from web interface\n")
	b.WriteString("· unbind / unbind — request account unbind (requires confirmation)\n")
	b.WriteString("· identity / whoami — show platform sender, auth mode, and current RBAC identity\n")
	if can("chat:read") || can("chat:write") || can("chat:delete") {
		b.WriteString("\n[Conversation]\n")
		if can("chat:read") {
			b.WriteString("· list / list — list all conversation titles and IDs\n· switch <ID> / switch <ID> — continue specified conversation\n· status / status — show current selection summary\n· task / task — view current task status\n")
		}
		if can("chat:write") {
			b.WriteString("· new / new; clear / clear — start new conversation\n· rename <name> / rename <name> — change current conversation title\n")
		}
		if can("chat:delete") {
			b.WriteString("· delete <ID> / delete <ID> — delete specified conversation (requires confirmation)\n")
		}
	}
	if can("roles:read") {
		b.WriteString("\n[Role]\n· roles / roles — list all available roles\n· role <name> / role <name> — switch current role\n")
	}
	if can("agent:execute") {
		b.WriteString("\n[Mode]\n· modes / modes — list conversation modes and current selection\n· mode <name> / mode <name> — switch conversation mode\n· stop / stop — interrupt current task\n")
	}
	if can("vulnerability:read") {
		b.WriteString("\n[Vulnerability Alerts]\n· vulnalerts — view subscription status\n· vulnalerts on / vuln alerts on — enable alerts\n· vulnalerts critical|high|medium / vuln alerts critical|high|medium — set minimum level\n· vulnalerts off / vuln alerts off — disable alerts\n")
	}
	b.WriteString("\n[Diagnostics]\n")
	b.WriteString("· permissions / permissions — view current business permissions\n")
	if can("config:read") {
		b.WriteString("· doctor / doctor — check robot key configuration status\n")
	}
	b.WriteString("· confirm / confirm; cancel / cancel — handle high-risk operation confirmation\n")
	if h.projectsEnabled() && (can("project:read") || can("project:write")) {
		b.WriteString("\n[Project]\n")
		if can("project:read") {
			b.WriteString("· projects / projects — list all projects\n")
		}
		if can("project:write") {
			b.WriteString("· new project <name> / new project <name> — create and bind to current conversation\n· bind project <ID|name> / bind project <ID|name> — bind existing project\n· unbind project / unbind project — unbind project\n")
		}
	}
	b.WriteString("\n──────────────\n")
	b.WriteString("Beyond the above commands, directly typing content will send it to AI for penetration testing/security analysis.")
	return b.String()
}

func (h *RobotHandler) projectsEnabled() bool {
	return h.config != nil && h.config.Project.Enabled
}

func (h *RobotHandler) resolveProjectByIDOrName(access *database.RBACAccess, idOrName string) (*database.Project, string) {
	idOrName = strings.TrimSpace(idOrName)
	if idOrName == "" {
		return nil, "please specify project ID or name, e.g.: bind project xxx-xxx"
	}
	ownerID := access.User.ID
	scope := robotPrincipal(access).ScopeFor("project:read")
	if p, err := h.db.GetProject(idOrName); err == nil {
		if h.db.UserCanAccessResource(ownerID, scope, "project", p.ID) {
			return p, ""
		}
		return nil, "project not found or access denied."
	}
	list, err := h.db.ListProjectsForAccess("", "", 200, 0, ownerID, scope)
	if err != nil {
		return nil, "failed to query project: " + err.Error()
	}
	var matches []*database.Project
	for _, p := range list {
		if p.Name == idOrName {
			matches = append(matches, p)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Sprintf(`project %q does not exist. Send "projects" to view the list.`, idOrName)
	case 1:
		return matches[0], ""
	default:
		var b strings.Builder
		b.WriteString(fmt.Sprintf("name %q matches multiple projects, please use ID to bind:\n", idOrName))
		for _, p := range matches {
			b.WriteString(fmt.Sprintf("· %s\n  ID: %s\n", p.Name, p.ID))
		}
		return nil, strings.TrimSuffix(b.String(), "\n")
	}
}

func (h *RobotHandler) formatProjectLabel(projectID string) string {
	if strings.TrimSpace(projectID) == "" {
		return "not bound"
	}
	if p, err := h.db.GetProject(projectID); err == nil {
		return fmt.Sprintf("%q (%s)", p.Name, p.ID)
	}
	return projectID
}

func (h *RobotHandler) cmdProjects(platform, userID string) string {
	if !h.projectsEnabled() {
		return "project feature not enabled (config.project.enabled)."
	}
	access, err := h.resolveRobotAccess(platform, userID)
	if err != nil {
		return "current platform account is not yet bound."
	}
	list, err := h.db.ListProjectsForAccess("", "", 50, 0, access.User.ID, robotPrincipal(access).ScopeFor("project:read"))
	if err != nil {
		return "get project listfailed: " + err.Error()
	}
	if len(list) == 0 {
		return `no projects yet. Send "new project <name>" to create and bind to current conversation.`
	}
	var b strings.Builder
	b.WriteString("[Project List]\n")
	for i, p := range list {
		if i >= 20 {
			b.WriteString("… showing first 20 only\n")
			break
		}
		status := p.Status
		if status == "" {
			status = "active"
		}
		b.WriteString(fmt.Sprintf("· %s [%s]\n  ID: %s\n", p.Name, status, p.ID))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func (h *RobotHandler) cmdBindProject(platform, userID, idOrName string) string {
	if !h.projectsEnabled() {
		return "project feature not enabled (config.project.enabled)."
	}
	access, err := h.resolveRobotAccess(platform, userID)
	if err != nil {
		return "current platform account is not yet bound."
	}
	p, errMsg := h.resolveProjectByIDOrName(access, idOrName)
	if p == nil {
		return errMsg
	}
	convID, _ := h.getOrCreateConversation(platform, userID, "", access)
	if convID == "" {
		return "failed to get current conversation, please try again later."
	}
	if err := h.db.SetConversationProjectID(convID, p.ID); err != nil {
		return "bind failed: " + err.Error()
	}
	return fmt.Sprintf("Current conversation has been bound to project: %q\nID: %s", p.Name, p.ID)
}

func (h *RobotHandler) cmdNewProject(platform, userID, name string) string {
	if !h.projectsEnabled() {
		return "project feature not enabled (config.project.enabled)."
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "please specify a project name, e.g.: new project <target-name>"
	}
	access, accessErr := h.resolveRobotAccess(platform, userID)
	if accessErr != nil {
		return "current platform account is not yet bound."
	}
	p := &database.Project{Name: name, Status: "active"}
	created, err := h.db.CreateProject(p)
	if err != nil {
		return "create projectfailed: " + err.Error()
	}
	_ = h.db.SetResourceOwner("project", created.ID, access.User.ID)
	convID, _ := h.getOrCreateConversation(platform, userID, name, access)
	if convID == "" {
		return fmt.Sprintf("Project created: %q\nID: %s\n(binding current conversation failed; please send \"bind project %s\" manually)", created.Name, created.ID, created.ID)
	}
	if err := h.db.SetConversationProjectID(convID, created.ID); err != nil {
		return fmt.Sprintf("Project created: %q\nID: %s\nbind failed: %s", created.Name, created.ID, err.Error())
	}
	return fmt.Sprintf("Project created and bound to current conversation: %q\nID: %s", created.Name, created.ID)
}

func (h *RobotHandler) cmdUnbindProject(platform, userID string) string {
	if !h.projectsEnabled() {
		return "project feature not enabled (config.project.enabled)."
	}
	sk := h.sessionKey(platform, userID)
	h.mu.RLock()
	convID := h.sessions[sk]
	h.mu.RUnlock()
	if convID == "" {
		if persistedConvID, _, _ := h.loadSessionBinding(sk); persistedConvID != "" {
			convID = persistedConvID
		}
	}
	access, err := h.resolveRobotAccess(platform, userID)
	if err != nil {
		return "current platform account is not yet bound."
	}
	if !h.db.UserCanAccessResource(access.User.ID, robotPrincipal(access).ScopeFor("chat:write"), "conversation", convID) {
		return "current conversation not found or access denied."
	}
	if convID == "" {
		return "no active conversation; nothing to unbind."
	}
	projectID, err := h.db.GetConversationProjectID(convID)
	if err != nil {
		return "failed to get conversation project: " + err.Error()
	}
	if strings.TrimSpace(projectID) == "" {
		return "current conversation is not bound to a project."
	}
	if err := h.db.SetConversationProjectID(convID, ""); err != nil {
		return "unbind failed: " + err.Error()
	}
	return "project binding of current conversation has been removed."
}

func (h *RobotHandler) cmdList(platform, userID string) string {
	access, err := h.resolveRobotAccess(platform, userID)
	if err != nil {
		return "current platform account is not yet bound."
	}
	convs, err := h.db.ListConversationsForAccess(50, 0, "", "", "", access.User.ID, robotPrincipal(access).ScopeFor("chat:read"))
	if err != nil {
		return "get conversation listfailed: " + err.Error()
	}
	if len(convs) == 0 {
		return "no conversations yet. Send any message to create a new conversation."
	}
	var b strings.Builder
	b.WriteString("[Conversation List]\n")
	for i, c := range convs {
		if i >= 20 {
			b.WriteString("… showing first 20 only\n")
			break
		}
		b.WriteString(fmt.Sprintf("· %s\n  ID: %s\n", c.Title, c.ID))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func (h *RobotHandler) cmdSwitch(platform, userID, convID string) string {
	if convID == "" {
		return "please specify a conversation ID, e.g.: switch xxx-xxx-xxx"
	}
	access, accessErr := h.resolveRobotAccess(platform, userID)
	if accessErr != nil {
		return "current platform account is not yet bound."
	}
	conv, err := h.db.GetConversation(convID)
	if err != nil || !h.db.UserCanAccessResource(access.User.ID, robotPrincipal(access).ScopeFor("chat:read"), "conversation", convID) {
		return "conversation not found or ID error."
	}
	h.setConversation(platform, userID, conv.ID)
	return fmt.Sprintf("Switched to conversation: %q\nID: %s", conv.Title, conv.ID)
}

func (h *RobotHandler) cmdNew(platform, userID string) string {
	access, err := h.resolveRobotAccess(platform, userID)
	if err != nil {
		return "current platform account is not yet bound."
	}
	newID := h.clearConversation(platform, userID, access)
	if newID == "" {
		return "failed to create new conversation, please retry."
	}
	return "New conversation enabled, you can start sending messages."
}

func (h *RobotHandler) cmdClear(platform, userID string) string {
	return h.cmdNew(platform, userID)
}

func (h *RobotHandler) cmdStop(platform, userID string) string {
	sk := h.sessionKey(platform, userID)
	h.cancelMu.Lock()
	cancel, ok := h.runningCancels[sk]
	if ok {
		delete(h.runningCancels, sk)
		cancel()
	}
	h.cancelMu.Unlock()
	if !ok {
		return "no task is currently running."
	}
	return "current task stopped."
}

func (h *RobotHandler) cmdStatus(platform, userID string) string {
	convID := h.currentConversationID(platform, userID)
	if convID == "" {
		return fmt.Sprintf("[Current Status]\nCurrent conversation: none\nCurrent role: %s\nCurrent mode: %s\nCurrent project: none\n\nSend any message to create a new conversation.", h.getRole(platform, userID), robotAgentModeLabel(h.getAgentMode(platform, userID)))
	}
	access, err := h.resolveRobotAccess(platform, userID)
	if err != nil {
		return "current platform account is not yet bound."
	}
	if !h.db.UserCanAccessResource(access.User.ID, robotPrincipal(access).ScopeFor("chat:read"), "conversation", convID) {
		return "current conversation not found or access denied."
	}
	conv, err := h.db.GetConversation(convID)
	if err != nil {
		return "current conversation ID: " + convID + " (failed to get title)"
	}
	role := h.getRole(platform, userID)
	reply := fmt.Sprintf("[Current Status]\nCurrent conversation: %s\nConversation ID: %s\nCurrent mode: %s\nCurrent role: %s", conv.Title, conv.ID, robotAgentModeLabel(h.getAgentMode(platform, userID)), role)
	if h.projectsEnabled() {
		projectID, _ := h.db.GetConversationProjectID(conv.ID)
		reply += "\nCurrent project: " + h.formatProjectLabel(projectID)
	} else {
		reply += "\nCurrent project: disabled"
	}
	return reply
}

func (h *RobotHandler) currentConversationID(platform, userID string) string {
	sk := h.sessionKey(platform, userID)
	h.mu.RLock()
	convID := h.sessions[sk]
	h.mu.RUnlock()
	if convID != "" {
		return convID
	}
	persistedConvID, persistedRole, persistedMode := h.loadSessionBinding(sk)
	if persistedConvID == "" {
		return ""
	}
	h.mu.Lock()
	h.sessions[sk] = persistedConvID
	h.sessionRoles[sk] = persistedRole
	h.sessionModes[sk] = config.NormalizeAgentMode(persistedMode)
	h.mu.Unlock()
	return persistedConvID
}

func (h *RobotHandler) cmdTask(platform, userID string) string {
	convID := h.currentConversationID(platform, userID)
	if convID == "" {
		return "[Task Status]\nNo current conversation, no task in progress."
	}
	if h.agentHandler == nil || h.agentHandler.tasks == nil {
		return "Task status service unavailable."
	}
	task := h.agentHandler.tasks.GetTaskSnapshot(convID)
	if task == nil {
		return "[Task Status]\nStatus: idle\nNo task currently running."
	}
	elapsed := time.Since(task.StartedAt).Round(time.Second)
	return fmt.Sprintf("[Task Status]\nStatus: %s\nRunning for: %s\nConversation ID: %s\nMode: %s\nAvailable actions: stop / stop", task.Status, elapsed, convID, robotAgentModeLabel(h.getAgentMode(platform, userID)))
}

func (h *RobotHandler) cmdRename(platform, userID, title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "Please specify a new title, e.g.: rename External Asset Survey"
	}
	title = safeTruncateString(title, 100)
	convID := h.currentConversationID(platform, userID)
	if convID == "" {
		return "No current conversation, cannot rename."
	}
	access, err := h.resolveRobotAccess(platform, userID)
	if err != nil || !h.db.UserCanAccessResource(access.User.ID, robotPrincipal(access).ScopeFor("chat:write"), "conversation", convID) {
		return "Current conversation not found or no permission to modify."
	}
	if err := h.db.UpdateConversationTitle(convID, title); err != nil {
		return "Rename failed: " + err.Error()
	}
	h.recordRobotCommandAudit(access, platform, "conversation_rename", "conversation", convID, "robot renamed current conversation")
	return fmt.Sprintf("Renamed current conversation to: %q", title)
}

func (h *RobotHandler) cmdPermissions(platform, userID string) string {
	access, err := h.resolveRobotAccess(platform, userID)
	if err != nil {
		return h.robotAccessDeniedMessage(platform)
	}
	allowed := func(permission string) string {
		if access.Permissions[permission] {
			return "allowed"
		}
		return "denied"
	}
	return fmt.Sprintf("[Current Permissions]\nExecute Agent: %s\nRead conversation: %s\nEdit conversation: %s\nDelete conversation: %s\nRead roles: %s\nRead project: %s\nEdit project: %s\nResource scope: %s", allowed("agent:execute"), allowed("chat:read"), allowed("chat:write"), allowed("chat:delete"), allowed("roles:read"), allowed("project:read"), allowed("project:write"), access.Scope)
}

func (h *RobotHandler) cmdDoctor() string {
	configured := func(ok bool) string {
		if ok {
			return "normal"
		}
		return "not configured"
	}
	enabled := func(ok bool) string {
		if ok {
			return "enabled"
		}
		return "disabled"
	}
	enabledInternalTools := 0
	for _, tool := range h.config.Security.Tools {
		if tool.Enabled {
			enabledInternalTools++
		}
	}
	enabledExternal := 0
	for _, server := range h.config.ExternalMCP.Servers {
		if server.ExternalMCPEnable && !server.Disabled {
			enabledExternal++
		}
	}
	return fmt.Sprintf("[Config Diagnostics]\nPrimary model: %s\nEino multi-agent: %s\nBuilt-in MCP tools: %d/%d enabled\nHTTP MCP service: %s\nExternal MCP: %d enabled\nKnowledge base: %s\nProject feature: %s\nNote: built-in tools do not depend on HTTP MCP service; this command only checks config, does not probe external services.", configured(strings.TrimSpace(h.config.OpenAI.Model) != "" && strings.TrimSpace(h.config.OpenAI.BaseURL) != ""), enabled(h.config.MultiAgent.Enabled), enabledInternalTools, len(h.config.Security.Tools), enabled(h.config.MCP.Enabled), enabledExternal, enabled(h.config.Knowledge.Enabled), enabled(h.config.Project.Enabled))
}

func (h *RobotHandler) recordRobotCommandAudit(access *database.RBACAccess, platform, action, resourceType, resourceID, message string) {
	if h.audit == nil || access == nil {
		return
	}
	h.audit.RecordSystem(audit.Entry{Category: "robot", Action: action, Result: "success", Actor: access.User.Username, ResourceType: resourceType, ResourceID: resourceID, Message: message + " (" + platform + ")"})
}

func (h *RobotHandler) cmdRoles() string {
	if h.config.Roles == nil || len(h.config.Roles) == 0 {
		return "No roles available."
	}
	names := make([]string, 0, len(h.config.Roles))
	for name, role := range h.config.Roles {
		if role.Enabled {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "No roles available."
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == "default" {
			return true
		}
		if names[j] == "default" {
			return false
		}
		return names[i] < names[j]
	})
	var b strings.Builder
	b.WriteString("[Role list]\n")
	for _, name := range names {
		role := h.config.Roles[name]
		desc := role.Description
		if desc == "" {
			desc = "no description"
		}
		b.WriteString(fmt.Sprintf("· %s — %s\n", name, desc))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func (h *RobotHandler) cmdSwitchRole(platform, userID, roleName string) string {
	if roleName == "" {
		return "Please specify a role name, e.g.: role penetration-testing"
	}
	if h.config.Roles == nil {
		return "No roles available."
	}
	role, exists := h.config.Roles[roleName]
	if !exists {
		return fmt.Sprintf("Role %q does not exist. Send \"roles\" to view available roles.", roleName)
	}
	if !role.Enabled {
		return fmt.Sprintf("Role %q is disabled.", roleName)
	}
	h.setRole(platform, userID, roleName)
	return fmt.Sprintf("Switched to role: %q\n%s", roleName, role.Description)
}

func robotAgentModeLabel(mode string) string {
	switch config.NormalizeAgentMode(mode) {
	case "deep":
		return "Deep"
	case "plan_execute":
		return "Plan-Execute"
	case "supervisor":
		return "Supervisor"
	default:
		return "Eino Single-Agent"
	}
}

func parseRobotAgentMode(input string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "eino_single", "eino-single", "single":
		return "eino_single", true
	case "deep":
		return "deep", true
	case "plan_execute", "plan-execute", "planexecute", "pe":
		return "plan_execute", true
	case "supervisor", "super", "sv":
		return "supervisor", true
	default:
		return "", false
	}
}

func (h *RobotHandler) cmdModes(platform, userID string) string {
	current := h.getAgentMode(platform, userID)
	multiStatus := "available"
	if h.config == nil || !h.config.MultiAgent.Enabled {
		multiStatus = "unavailable (enable Eino multi-agent in system settings)"
	}
	return fmt.Sprintf("[Conversation Mode]\n· Eino Single-Agent — available\n· Deep — %s\n· Plan-Execute — %s\n· Supervisor — %s\n\nCurrent mode: %s\nSwitch example: mode deep", multiStatus, multiStatus, multiStatus, robotAgentModeLabel(current))
}

func (h *RobotHandler) cmdSwitchMode(platform, userID, input string) string {
	mode, ok := parseRobotAgentMode(input)
	if !ok {
		return fmt.Sprintf("Unsupported conversation mode %q. Send \"modes\" to view available modes.", strings.TrimSpace(input))
	}
	if mode != "eino_single" && (h.config == nil || !h.config.MultiAgent.Enabled) {
		return fmt.Sprintf("Cannot switch to %s: please enable Eino multi-agent in system settings first.", robotAgentModeLabel(mode))
	}
	h.setAgentMode(platform, userID, mode)
	return fmt.Sprintf("Switched conversation mode to: %s\nSubsequent messages and new conversations will use this mode.", robotAgentModeLabel(mode))
}

func (h *RobotHandler) cmdDelete(platform, userID, convID string) string {
	if convID == "" {
		return "Please specify a conversation ID, e.g.: delete xxx-xxx-xxx"
	}
	access, err := h.resolveRobotAccess(platform, userID)
	if err != nil {
		return "current platform account is not yet bound."
	}
	if !h.db.UserCanAccessResource(access.User.ID, robotPrincipal(access).ScopeFor("chat:delete"), "conversation", convID) {
		return "Conversation not found or access denied."
	}
	h.setPendingConfirmation(platform, userID, "delete_conversation", convID)
	return fmt.Sprintf("⚠️ About to delete conversation ID: %s\nThis action cannot be undone. Send \"confirm\" within 2 minutes to proceed, or send \"cancel\" to abort.", convID)
}

func (h *RobotHandler) executeDelete(platform, userID, convID string) string {
	access, err := h.resolveRobotAccess(platform, userID)
	if err != nil || !h.db.UserCanAccessResource(access.User.ID, robotPrincipal(access).ScopeFor("chat:delete"), "conversation", convID) {
		return "Conversation not found or no permission to delete."
	}
	sk := h.sessionKey(platform, userID)
	h.mu.RLock()
	currentConvID := h.sessions[sk]
	h.mu.RUnlock()
	if convID == currentConvID {
		// when deleting the current conversation, first clear the session binding
		h.mu.Lock()
		delete(h.sessions, sk)
		delete(h.sessionRoles, sk)
		delete(h.sessionModes, sk)
		h.mu.Unlock()
		h.deleteSessionBinding(sk)
	}
	if h.agentHandler != nil {
		h.agentHandler.CancelRunningTaskForConversation(convID)
	}
	if err := h.db.DeleteConversation(convID); err != nil {
		return "delete failed: " + err.Error()
	}
	h.recordRobotCommandAudit(access, platform, "conversation_delete", "conversation", convID, "robot deleted conversation")
	return fmt.Sprintf("deletedconversation ID: %s", convID)
}

func (h *RobotHandler) cmdVersion() string {
	v := h.config.Version
	if v == "" {
		v = "unknown"
	}
	return "Kestrel " + v
}

func (h *RobotHandler) cmdIdentity(platform, userID string) string {
	authorization := h.config.Robots.AuthorizationFor(platform)
	mode := authorization.EffectiveMode()
	modeLabel := "per-user binding (user_binding)"
	if mode == config.RobotAuthModeServiceAccount {
		modeLabel = "dedicated service account (service_account)"
	}
	var b strings.Builder
	b.WriteString("[Robot Identity]\n")
	b.WriteString("Platform: " + platform + "\n")
	b.WriteString("Sender ID: " + userID + "\n")
	b.WriteString("Auth mode: " + modeLabel + "\n")

	access, err := h.resolveRobotAccess(platform, userID)
	if err != nil {
		if mode == config.RobotAuthModeServiceAccount {
			b.WriteString("Auth status: denied (sender not in whitelist, or service account unavailable)")
		} else {
			b.WriteString("Auth status: not bound\n")
			b.WriteString("Tip: generate a bind code on the web interface, then send \"bind XXXX-XXXX\"")
		}
		return b.String()
	}

	name := strings.TrimSpace(access.User.DisplayName)
	if name == "" {
		name = access.User.Username
	}
	roleNames := make([]string, 0, len(access.Roles))
	for _, role := range access.Roles {
		roleNames = append(roleNames, role.Name)
	}
	if len(roleNames) == 0 {
		roleNames = append(roleNames, "no role assigned")
	}
	b.WriteString("Auth status: authorized\n")
	b.WriteString("Identity: " + name + " (" + access.User.Username + ")\n")
	b.WriteString("RBAC User ID: " + access.User.ID + "\n")
	b.WriteString("Platform roles: " + strings.Join(roleNames, ", ") + "\n")
	b.WriteString("Resource scope: " + access.Scope + "\n")
	b.WriteString(fmt.Sprintf("Effective permissions: %d", len(access.Permissions)))
	return b.String()
}

func robotCommandPermission(text string) (string, bool) {
	switch {
	case text == robotCmdHelp || text == "help" || text == "?", text == robotCmdVersion || text == "version", text == robotCmdIdentity || text == "whoami":
		return "", true
	case text == robotCmdList || text == robotCmdListAlt || text == "list",
		strings.HasPrefix(text, robotCmdSwitch+" "), strings.HasPrefix(text, robotCmdContinue+" "),
		strings.HasPrefix(text, "switch "), strings.HasPrefix(text, "continue "),
		text == robotCmdStatus || text == "status", text == robotCmdTask || text == "task":
		return "chat:read", true
	case text == robotCmdNew || text == "new", text == robotCmdClear || text == "clear",
		strings.HasPrefix(text, robotCmdRename+" "), strings.HasPrefix(text, "rename "):
		return "chat:write", true
	case strings.HasPrefix(text, robotCmdDelete+" "), strings.HasPrefix(text, "delete "):
		return "chat:delete", true
	case text == robotCmdStop || text == "stop":
		return "agent:execute", true
	case text == robotCmdRoles || text == robotCmdRolesList || text == "roles",
		strings.HasPrefix(text, robotCmdRoles+" "), strings.HasPrefix(text, robotCmdSwitchRole+" "), strings.HasPrefix(text, "role "):
		return "roles:read", true
	case text == robotCmdModes || text == robotCmdModesList || text == "modes",
		strings.HasPrefix(text, robotCmdModes+" "), strings.HasPrefix(text, robotCmdSwitchMode+" "), strings.HasPrefix(text, "mode "):
		return "agent:execute", true
	case text == robotCmdPermissions || text == "permissions":
		return "", true
	case text == robotCmdConfirm || text == "confirm", text == robotCmdCancel || text == "cancel":
		return "", true
	case text == robotCmdDoctor || text == "doctor":
		return "config:read", true
	case text == robotCmdProjects || text == robotCmdProjectsList || text == "projects":
		return "project:read", true
	case text == robotCmdVulnAlerts || strings.HasPrefix(text, robotCmdVulnAlerts+" "),
		text == "vuln alerts" || strings.HasPrefix(text, "vuln alerts "):
		return "vulnerability:read", true
	case text == robotCmdUnbindProject || text == "unbind project",
		strings.HasPrefix(text, robotCmdNewProject+" "), strings.HasPrefix(text, "new project "),
		strings.HasPrefix(text, robotCmdBindProject+" "), strings.HasPrefix(text, "bind project "):
		return "project:write", true
	default:
		return "", false
	}
}

func (h *RobotHandler) cmdBindUser(platform, userID, code string) string {
	if h.config.Robots.AuthorizationFor(platform).EffectiveMode() != config.RobotAuthModeUserBinding {
		return "This robot uses a managed service account mode and does not accept user binding."
	}
	code = normalizeRobotBindingCode(code)
	if code == "" {
		return "Please provide a bind code, e.g.: bind ABCD-1234"
	}
	user, err := h.db.ConsumeRobotBindingCode(platform, userID, hashRobotBindingCode(code))
	if err != nil {
		return "Bind failed: invalid bind code, already used, or expired. Please go to the web interface to regenerate."
	}
	// Never carry an old synthetic-owner conversation into the RBAC identity.
	sk := h.sessionKey(platform, userID)
	h.mu.Lock()
	delete(h.sessions, sk)
	delete(h.sessionRoles, sk)
	delete(h.sessionModes, sk)
	h.mu.Unlock()
	h.deleteSessionBinding(sk)
	name := strings.TrimSpace(user.DisplayName)
	if name == "" {
		name = user.Username
	}
	if h.audit != nil {
		hint := sha256.Sum256([]byte(userID))
		h.audit.RecordSystem(audit.Entry{
			Category: "auth", Action: "robot_bind", Result: "success", Actor: user.Username,
			ResourceType: "robot_binding", ResourceID: platform + ":" + fmt.Sprintf("%x", hint[:4]), Message: "Bot platform account bind successful",
		})
	}
	return fmt.Sprintf("Bind successful. Current identity: %s. Subsequent actions will use this user's RBAC permissions in real time.", name)
}

func (h *RobotHandler) cmdUnbindUser(platform, userID string) string {
	if h.config.Robots.AuthorizationFor(platform).EffectiveMode() != config.RobotAuthModeUserBinding {
		return "This robot uses a managed service account mode; user unbinding is not required."
	}
	_, accessErr := h.resolveRobotAccess(platform, userID)
	if accessErr != nil {
		return "current platform account is not yet bound."
	}
	h.setPendingConfirmation(platform, userID, "unbind_user", "")
	return "⚠️ About to unbind the current platform account. Send \"confirm\" within 2 minutes to proceed, or send \"cancel\" to abort."
}

func (h *RobotHandler) executeUnbindUser(platform, userID string) string {
	access, accessErr := h.resolveRobotAccess(platform, userID)
	if accessErr != nil {
		return "current platform account is not yet bound."
	}
	if err := h.db.DeleteRobotIdentityBinding(platform, userID); err != nil {
		return "Unbind failed. Please try again later."
	}
	sk := h.sessionKey(platform, userID)
	h.mu.Lock()
	delete(h.sessions, sk)
	delete(h.sessionRoles, sk)
	delete(h.sessionModes, sk)
	h.mu.Unlock()
	h.deleteSessionBinding(sk)
	if h.audit != nil {
		hint := sha256.Sum256([]byte(userID))
		h.audit.RecordSystem(audit.Entry{
			Category: "auth", Action: "robot_unbind", Result: "success", Actor: access.User.Username,
			ResourceType: "robot_binding", ResourceID: platform + ":" + fmt.Sprintf("%x", hint[:4]), Message: "Bot platform account unbind successful",
		})
	}
	return "Current platform account has been unbound from the Kestrel user."
}

func (h *RobotHandler) setPendingConfirmation(platform, userID, action, target string) {
	sk := h.sessionKey(platform, userID)
	now := time.Now()
	h.mu.Lock()
	for key, pending := range h.pendingConfirmations {
		if now.After(pending.ExpiresAt) {
			delete(h.pendingConfirmations, key)
		}
	}
	h.pendingConfirmations[sk] = robotPendingConfirmation{Action: action, Target: target, ExpiresAt: now.Add(2 * time.Minute)}
	h.mu.Unlock()
}

func (h *RobotHandler) cmdConfirm(platform, userID string) string {
	sk := h.sessionKey(platform, userID)
	h.mu.Lock()
	pending, ok := h.pendingConfirmations[sk]
	delete(h.pendingConfirmations, sk)
	h.mu.Unlock()
	if !ok || time.Now().After(pending.ExpiresAt) {
		return "No pending confirmation, or confirmation has timed out."
	}
	switch pending.Action {
	case "delete_conversation":
		return h.executeDelete(platform, userID, pending.Target)
	case "unbind_user":
		return h.executeUnbindUser(platform, userID)
	default:
		return "Pending confirmation is invalid and has been cancelled."
	}
}

func (h *RobotHandler) cmdCancelConfirmation(platform, userID string) string {
	sk := h.sessionKey(platform, userID)
	h.mu.Lock()
	_, ok := h.pendingConfirmations[sk]
	delete(h.pendingConfirmations, sk)
	h.mu.Unlock()
	if !ok {
		return "No pending confirmation."
	}
	return "Pending confirmation cancelled."
}

// handleRobotCommand processes built-in bot commands; returns (reply, true) if command matched, ("", false) otherwise
func (h *RobotHandler) handleRobotCommand(platform, userID, text string) (string, bool) {
	if (strings.HasPrefix(text, robotCmdBindUser+" ") || strings.HasPrefix(text, "bind ")) && !strings.HasPrefix(text, "bind project ") {
		parts := strings.SplitN(text, " ", 2)
		return h.cmdBindUser(platform, userID, strings.TrimSpace(parts[1])), true
	}
	if text == robotCmdUnbindUser || text == "unbind" {
		return h.cmdUnbindUser(platform, userID), true
	}
	if permission, recognized := robotCommandPermission(text); recognized && permission != "" {
		access, err := h.resolveRobotAccess(platform, userID)
		if err != nil {
			return h.robotAccessDeniedMessage(platform), true
		}
		if !access.Permissions[permission] {
			return fmt.Sprintf("Insufficient permissions: missing %s permission.", permission), true
		}
	}
	switch {
	case text == robotCmdVulnAlerts || text == "vuln alerts":
		return h.cmdVulnerabilityAlerts(platform, userID, ""), true
	case strings.HasPrefix(text, robotCmdVulnAlerts+" "):
		return h.cmdVulnerabilityAlerts(platform, userID, strings.TrimSpace(text[len(robotCmdVulnAlerts)+1:])), true
	case strings.HasPrefix(text, "vuln alerts "):
		return h.cmdVulnerabilityAlerts(platform, userID, strings.TrimSpace(text[len("vuln alerts "):])), true
	case text == robotCmdHelp || text == "help" || text == "?":
		return h.cmdHelp(platform, userID), true
	case text == robotCmdIdentity || text == "whoami":
		return h.cmdIdentity(platform, userID), true
	case text == robotCmdConfirm || text == "confirm":
		return h.cmdConfirm(platform, userID), true
	case text == robotCmdCancel || text == "cancel":
		return h.cmdCancelConfirmation(platform, userID), true
	case text == robotCmdList || text == robotCmdListAlt || text == "list":
		return h.cmdList(platform, userID), true
	case strings.HasPrefix(text, robotCmdSwitch+" ") || strings.HasPrefix(text, robotCmdContinue+" ") || strings.HasPrefix(text, "switch ") || strings.HasPrefix(text, "continue "):
		var id string
		switch {
		case strings.HasPrefix(text, robotCmdSwitch+" "):
			id = strings.TrimSpace(text[len(robotCmdSwitch)+1:])
		case strings.HasPrefix(text, robotCmdContinue+" "):
			id = strings.TrimSpace(text[len(robotCmdContinue)+1:])
		case strings.HasPrefix(text, "switch "):
			id = strings.TrimSpace(text[7:])
		default:
			id = strings.TrimSpace(text[9:])
		}
		return h.cmdSwitch(platform, userID, id), true
	case text == robotCmdNew || text == "new":
		return h.cmdNew(platform, userID), true
	case text == robotCmdClear || text == "clear":
		return h.cmdClear(platform, userID), true
	case text == robotCmdStatus || text == "status":
		return h.cmdStatus(platform, userID), true
	case text == robotCmdTask || text == "task":
		return h.cmdTask(platform, userID), true
	case strings.HasPrefix(text, robotCmdRename+" ") || strings.HasPrefix(text, "rename "):
		var title string
		if strings.HasPrefix(text, robotCmdRename+" ") {
			title = strings.TrimSpace(text[len(robotCmdRename)+1:])
		} else {
			title = strings.TrimSpace(text[len("rename "):])
		}
		return h.cmdRename(platform, userID, title), true
	case text == robotCmdStop || text == "stop":
		return h.cmdStop(platform, userID), true
	case text == robotCmdRoles || text == robotCmdRolesList || text == "roles":
		return h.cmdRoles(), true
	case strings.HasPrefix(text, robotCmdRoles+" ") || strings.HasPrefix(text, robotCmdSwitchRole+" ") || strings.HasPrefix(text, "role "):
		var roleName string
		switch {
		case strings.HasPrefix(text, robotCmdRoles+" "):
			roleName = strings.TrimSpace(text[len(robotCmdRoles)+1:])
		case strings.HasPrefix(text, robotCmdSwitchRole+" "):
			roleName = strings.TrimSpace(text[len(robotCmdSwitchRole)+1:])
		default:
			roleName = strings.TrimSpace(text[5:])
		}
		return h.cmdSwitchRole(platform, userID, roleName), true
	case text == robotCmdModes || text == robotCmdModesList || text == "modes":
		return h.cmdModes(platform, userID), true
	case strings.HasPrefix(text, robotCmdModes+" ") || strings.HasPrefix(text, robotCmdSwitchMode+" ") || strings.HasPrefix(text, "mode "):
		var mode string
		switch {
		case strings.HasPrefix(text, robotCmdModes+" "):
			mode = strings.TrimSpace(text[len(robotCmdModes)+1:])
		case strings.HasPrefix(text, robotCmdSwitchMode+" "):
			mode = strings.TrimSpace(text[len(robotCmdSwitchMode)+1:])
		default:
			mode = strings.TrimSpace(text[5:])
		}
		return h.cmdSwitchMode(platform, userID, mode), true
	case text == robotCmdPermissions || text == "permissions":
		return h.cmdPermissions(platform, userID), true
	case text == robotCmdDoctor || text == "doctor":
		return h.cmdDoctor(), true
	case strings.HasPrefix(text, robotCmdDelete+" ") || strings.HasPrefix(text, "delete "):
		var convID string
		if strings.HasPrefix(text, robotCmdDelete+" ") {
			convID = strings.TrimSpace(text[len(robotCmdDelete)+1:])
		} else {
			convID = strings.TrimSpace(text[7:])
		}
		return h.cmdDelete(platform, userID, convID), true
	case text == robotCmdVersion || text == "version":
		return h.cmdVersion(), true
	case text == robotCmdProjects || text == robotCmdProjectsList || text == "projects":
		return h.cmdProjects(platform, userID), true
	case text == robotCmdUnbindProject || text == "unbind project":
		return h.cmdUnbindProject(platform, userID), true
	case strings.HasPrefix(text, robotCmdNewProject+" ") || strings.HasPrefix(text, "new project "):
		var name string
		if strings.HasPrefix(text, robotCmdNewProject+" ") {
			name = strings.TrimSpace(text[len(robotCmdNewProject)+1:])
		} else {
			name = strings.TrimSpace(text[len("new project "):])
		}
		return h.cmdNewProject(platform, userID, name), true
	case strings.HasPrefix(text, robotCmdBindProject+" ") || strings.HasPrefix(text, "bind project "):
		var idOrName string
		if strings.HasPrefix(text, robotCmdBindProject+" ") {
			idOrName = strings.TrimSpace(text[len(robotCmdBindProject)+1:])
		} else {
			idOrName = strings.TrimSpace(text[len("bind project "):])
		}
		return h.cmdBindProject(platform, userID, idOrName), true
	default:
		return "", false
	}
}

// —————— WeCom ——————

// wecomXML WeCom callback XML (simplified structure for plaintext mode; encrypted mode requires decryption first)
type wecomXML struct {
	ToUserName   string `xml:"ToUserName"`
	FromUserName string `xml:"FromUserName"`
	CreateTime   int64  `xml:"CreateTime"`
	MsgType      string `xml:"MsgType"`
	Content      string `xml:"Content"`
	MsgID        string `xml:"MsgId"`
	AgentID      int64  `xml:"AgentID"`
	Encrypt      string `xml:"Encrypt"` // message body in encrypted mode
}

// wecomReplyXML passive reply XML (for compatibility only; currently using manually constructed XML)
type wecomReplyXML struct {
	XMLName      xml.Name `xml:"xml"`
	ToUserName   string   `xml:"ToUserName"`
	FromUserName string   `xml:"FromUserName"`
	CreateTime   int64    `xml:"CreateTime"`
	MsgType      string   `xml:"MsgType"`
	Content      string   `xml:"Content"`
}

// wecomRequireToken WeCom callback must configure Token; rejects requests when not configured to prevent unauthorized agent triggering.
func (h *RobotHandler) wecomRequireToken(c *gin.Context) (string, bool) {
	token := strings.TrimSpace(h.config.Robots.Wecom.Token)
	if token == "" {
		h.logger.Warn("WeCom enabled but token not configured, callback rejected (set robots.wecom.token in config)")
		c.String(http.StatusForbidden, "")
		return "", false
	}
	return token, true
}

// HandleWecomGET WeCom URL verification (GET)
func (h *RobotHandler) HandleWecomGET(c *gin.Context) {
	if !h.config.Robots.Wecom.Enabled {
		c.String(http.StatusNotFound, "")
		return
	}
	token, ok := h.wecomRequireToken(c)
	if !ok {
		return
	}
	// Gin's Query() automatically URL-decodes, so the result is already the correct base64 string
	echostr := c.Query("echostr")
	msgSignature := c.Query("msg_signature")
	timestamp := c.Query("timestamp")
	nonce := c.Query("nonce")

	// validate signature: sort token, timestamp, nonce, echostr four parameters, concatenate and compute SHA1
	signature := h.signWecomRequest(token, timestamp, nonce, echostr)
	if signature != msgSignature {
		h.logger.Warn("WeCom URL validate signature failed", zap.String("expected", msgSignature), zap.String("got", signature))
		c.String(http.StatusBadRequest, "invalid signature")
		return
	}

	if echostr == "" {
		c.String(http.StatusBadRequest, "missing echostr")
		return
	}

	// if EncodingAESKey is configured, encryption mode is active and echostr must be decrypted
	if h.config.Robots.Wecom.EncodingAESKey != "" {
		decrypted, err := wecomDecrypt(h.config.Robots.Wecom.EncodingAESKey, echostr)
		if err != nil {
			h.logger.Warn("WeCom echostr decryption failed", zap.Error(err))
			c.String(http.StatusBadRequest, "decrypt failed")
			return
		}
		c.String(http.StatusOK, string(decrypted))
		return
	}

	// plain text mode: return echostr directly
	c.String(http.StatusOK, echostr)
}

// signWecomRequest generates a WeCom request signature
// WeCom signature algorithm: sort token, timestamp, nonce, echostr four values, concatenate into string, then compute SHA1
func (h *RobotHandler) signWecomRequest(token, timestamp, nonce, echostr string) string {
	strs := []string{token, timestamp, nonce, echostr}
	sort.Strings(strs)
	s := strings.Join(strs, "")
	hash := sha1.Sum([]byte(s))
	return fmt.Sprintf("%x", hash)
}

// wecomDecrypt decrypts WeCom message (AES-256-CBC, PKCS7, plaintext format: 16-byte random + 4-byte length + message + corpID)
func wecomDecrypt(encodingAESKey, encryptedB64 string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(encodingAESKey + "=")
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("encoding_aes_key must be 32 bytes after base64 decoding")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedB64)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	iv := key[:16]
	mode := cipher.NewCBCDecrypter(block, iv)
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext length is not a multiple of the block size")
	}
	plain := make([]byte, len(ciphertext))
	mode.CryptBlocks(plain, ciphertext)
	// remove PKCS7 padding
	n := int(plain[len(plain)-1])
	if n < 1 || n > 32 {
		return nil, fmt.Errorf("invalid PKCS7 padding")
	}
	plain = plain[:len(plain)-n]
	// WeCom format: 16-byte random + 4-byte length (big-endian) + message + corpID
	if len(plain) < 20 {
		return nil, fmt.Errorf("plaintext too short")
	}
	msgLen := binary.BigEndian.Uint32(plain[16:20])
	if int(20+msgLen) > len(plain) {
		return nil, fmt.Errorf("message length out of bounds")
	}
	return plain[20 : 20+msgLen], nil
}

// wecomEncrypt encrypts WeCom message (AES-256-CBC, PKCS7, plaintext format: 16-byte random + 4-byte length + message + corpID)
func wecomEncrypt(encodingAESKey, message, corpID string) (string, error) {
	key, err := base64.StdEncoding.DecodeString(encodingAESKey + "=")
	if err != nil {
		return "", err
	}
	if len(key) != 32 {
		return "", fmt.Errorf("encoding_aes_key must be 32 bytes after base64 decoding")
	}
	// build plaintext: 16 random bytes + 4-byte length (big-endian) + message + corpID
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		// fallback: use timestamp to generate random bytes
		for i := range random {
			random[i] = byte(time.Now().UnixNano() % 256)
		}
	}
	msgLen := len(message)
	msgBytes := []byte(message)
	corpBytes := []byte(corpID)
	plain := make([]byte, 16+4+msgLen+len(corpBytes))
	copy(plain[:16], random)
	binary.BigEndian.PutUint32(plain[16:20], uint32(msgLen))
	copy(plain[20:20+msgLen], msgBytes)
	copy(plain[20+msgLen:], corpBytes)
	// PKCS7 padding
	padding := aes.BlockSize - len(plain)%aes.BlockSize
	pad := bytes.Repeat([]byte{byte(padding)}, padding)
	plain = append(plain, pad...)
	// AES-256-CBC encryption
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	iv := key[:16]
	ciphertext := make([]byte, len(plain))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, plain)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// HandleWecomPOST WeCom message callback (POST), supports plaintext and encrypted modes
func (h *RobotHandler) HandleWecomPOST(c *gin.Context) {
	if !h.config.Robots.Wecom.Enabled {
		h.logger.Debug("WeCom robot not enabled, skipping request")
		c.String(http.StatusOK, "")
		return
	}
	// get signature parameters from URL (needed when replying in encrypted mode)
	timestamp := c.Query("timestamp")
	nonce := c.Query("nonce")
	msgSignature := c.Query("msg_signature")

	// first read the request body; used for subsequent parsing and signature validation
	bodyRaw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logger.Warn("WeCom POST read request body failed", zap.Error(err))
		c.String(http.StatusOK, "")
		return
	}
	h.logger.Debug("WeCom POST received request", zap.String("body", string(bodyRaw)))

	// validate request signature to prevent forgery. WeCom signature algorithm same as URL verification, using token, timestamp, nonce, Encrypt four fields.
	// when WeCom is enabled, must configure token and verify signature to prevent unauthorized requests triggering the agent.
	token, ok := h.wecomRequireToken(c)
	if !ok {
		return
	}
	if msgSignature == "" {
		h.logger.Warn("WeCom POST missing signature, rejected (ensure callback carries msg_signature)")
		c.String(http.StatusOK, "")
		return
	}
	var tmp wecomXML
	if err := xml.Unmarshal(bodyRaw, &tmp); err != nil {
		h.logger.Warn("WeCom POST parse XML before signature validation failed", zap.Error(err))
		c.String(http.StatusOK, "")
		return
	}
	expected := h.signWecomRequest(token, timestamp, nonce, tmp.Encrypt)
	if expected != msgSignature {
		h.logger.Warn("WeCom POST signature validation failed", zap.String("expected", expected), zap.String("got", msgSignature))
		c.String(http.StatusOK, "")
		return
	}
	if !h.acceptFreshWecomRequest(timestamp, nonce, msgSignature) {
		h.logger.Warn("WeCom POST timestamp expired or request replay, rejected")
		c.String(http.StatusOK, "")
		return
	}

	var body wecomXML
	if err := xml.Unmarshal(bodyRaw, &body); err != nil {
		h.logger.Warn("WeCom POST parse XML failed", zap.Error(err))
		c.String(http.StatusOK, "")
		return
	}
	h.logger.Debug("WeCom XML parse successful", zap.String("ToUserName", body.ToUserName), zap.String("FromUserName", body.FromUserName), zap.String("MsgType", body.MsgType), zap.String("Content", body.Content), zap.String("Encrypt", body.Encrypt))

	// save corp ID (used for plaintext mode replies)
	enterpriseID := body.ToUserName

	// when EncodingAESKey is configured, must use encrypted message; reject plaintext XML bypass
	if strings.TrimSpace(h.config.Robots.Wecom.EncodingAESKey) != "" && strings.TrimSpace(body.Encrypt) == "" {
		h.logger.Warn("WeCom configured for encrypted mode but received plaintext message, rejected")
		c.String(http.StatusOK, "")
		return
	}

	// encrypted mode: decrypt first then parse inner XML
	if body.Encrypt != "" && h.config.Robots.Wecom.EncodingAESKey != "" {
		h.logger.Debug("WeCom entering encrypted mode decryption flow")
		decrypted, err := wecomDecrypt(h.config.Robots.Wecom.EncodingAESKey, body.Encrypt)
		if err != nil {
			h.logger.Warn("WeCommessagedecryption failed", zap.Error(err))
			c.String(http.StatusOK, "")
			return
		}
		h.logger.Debug("WeCom decryption successful", zap.String("decrypted", string(decrypted)))
		if err := xml.Unmarshal(decrypted, &body); err != nil {
			h.logger.Warn("WeCom post-decryption XML parsing failed", zap.Error(err))
			c.String(http.StatusOK, "")
			return
		}
		h.logger.Debug("WeCom inner XML parse successful", zap.String("FromUserName", body.FromUserName), zap.String("Content", body.Content))
	}

	tenantKey := strings.TrimSpace(enterpriseID)
	if tenantKey == "" {
		tenantKey = strings.TrimSpace(h.config.Robots.Wecom.CorpID)
	}
	if tenantKey == "" {
		tenantKey = "default"
	}
	rawUserID := strings.TrimSpace(body.FromUserName)
	replyUserID := rawUserID
	userID := ""
	if rawUserID != "" {
		userID = "t:" + tenantKey + "|u:" + rawUserID
	}
	text := strings.TrimSpace(body.Content)
	if userID == "" {
		h.logger.Warn("WeCom message missing usable user identifier, ignored")
		c.String(http.StatusOK, "success")
		return
	}

	// limit reply content length (WeCom limit is 2048 bytes)
	maxReplyLen := 2000
	limitReply := func(s string) string {
		if len(s) > maxReplyLen {
			return s[:maxReplyLen] + "\n\n(content too long, truncated)"
		}
		return s
	}

	if body.MsgType != "text" {
		h.logger.Debug("WeCom received non-text message", zap.String("MsgType", body.MsgType))
		h.sendWecomReply(c, replyUserID, enterpriseID, limitReply("Only text messages are supported. Please send text."), timestamp, nonce)
		return
	}

	// Text message: check whether it is a built-in command (e.g. help/list/new conversation, etc.).
	// These commands are fast and can use a passive reply, avoiding a dependency on the proactive send API.
	if cmdReply, ok := h.handleRobotCommand("wecom", userID, text); ok {
		h.logger.Debug("WeCom received command message, using passive reply", zap.String("userID", userID), zap.String("text", text))
		h.sendWecomReply(c, replyUserID, enterpriseID, limitReply(cmdReply), timestamp, nonce)
		return
	}

	h.logger.Debug("WeCom start processing message (async AI)", zap.String("userID", userID), zap.String("text", text))

	// WeCom passive reply has a 5-second timeout, while AI calls typically exceed this duration.
	// Use the recommended approach: immediately return success (or empty string), then push the full reply via the proactive send API.
	c.String(http.StatusOK, "success")

	// Process message asynchronously and send the result via the WeCom proactive message API
	go func() {
		reply := h.HandleMessage("wecom", userID, text)
		reply = limitReply(reply)
		h.logger.Debug("WeCommessageprocessing complete", zap.String("userID", userID), zap.String("reply", reply))
		// call WeCom API to proactively send message
		h.sendWecomMessageViaAPI(rawUserID, enterpriseID, reply)
	}()
}

// sendWecomReply sends a WeCom reply (auto-encrypts in encrypted mode)
// params: toUser=user ID, fromUser=corp ID (plaintext mode)/CorpID (encrypted mode), content=reply content, timestamp/nonce=request params
func (h *RobotHandler) sendWecomReply(c *gin.Context, toUser, fromUser, content, timestamp, nonce string) {
	// encrypted mode: check whether EncodingAESKey is configured
	if h.config.Robots.Wecom.EncodingAESKey != "" {
		// encrypted mode uses CorpID for encryption
		corpID := h.config.Robots.Wecom.CorpID
		if corpID == "" {
			h.logger.Warn("WeCom encrypted mode missing CorpID config")
			c.String(http.StatusOK, "")
			return
		}

		// construct full plaintext XML reply (format strictly follows WeCom documentation)
		plainResp := fmt.Sprintf(`<xml>
<ToUserName><![CDATA[%s]]></ToUserName>
<FromUserName><![CDATA[%s]]></FromUserName>
<CreateTime>%d</CreateTime>
<MsgType><![CDATA[text]]></MsgType>
<Content><![CDATA[%s]]></Content>
</xml>`, toUser, fromUser, time.Now().Unix(), content)

		encrypted, err := wecomEncrypt(h.config.Robots.Wecom.EncodingAESKey, plainResp, corpID)
		if err != nil {
			h.logger.Warn("WeCom reply encryption failed", zap.Error(err))
			c.String(http.StatusOK, "")
			return
		}
		// generate signature using timestamp/nonce from request (WeCom requires using the same timestamp and nonce as the request when replying)
		msgSignature := h.signWecomRequest(h.config.Robots.Wecom.Token, timestamp, nonce, encrypted)

		h.logger.Debug("WeCom sending encrypted reply",
			zap.String("Encrypt", encrypted[:50]+"..."),
			zap.String("MsgSignature", msgSignature),
			zap.String("TimeStamp", timestamp),
			zap.String("Nonce", nonce))

		// encrypted mode returns only 4 core fields (WeCom official requirement)
		xmlResp := fmt.Sprintf(`<xml><Encrypt><![CDATA[%s]]></Encrypt><MsgSignature><![CDATA[%s]]></MsgSignature><TimeStamp><![CDATA[%s]]></TimeStamp><Nonce><![CDATA[%s]]></Nonce></xml>`, encrypted, msgSignature, timestamp, nonce)
		// also log the final response body so we can cross-check with the
		// network traffic or developer console
		h.logger.Debug("WeCom encrypted reply packet", zap.String("xml", xmlResp))
		// for additional confidence, decrypt the payload ourselves and log it
		if dec, err2 := wecomDecrypt(h.config.Robots.Wecom.EncodingAESKey, encrypted); err2 == nil {
			h.logger.Debug("WeCom encrypted reply decryption check", zap.String("plain", string(dec)))
		} else {
			h.logger.Warn("WeCom encrypted reply decryption check failed", zap.Error(err2))
		}

		// use c.Writer.Write to write directly to response, avoiding escaping issues with c.String
		c.Writer.WriteHeader(http.StatusOK)
		// use text/xml as that's what WeCom examples show
		c.Writer.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = c.Writer.Write([]byte(xmlResp))
		h.logger.Debug("WeCom encrypted reply sent")
		return
	}

	// plaintext mode
	h.logger.Debug("WeCom sending plaintext reply", zap.String("ToUserName", toUser), zap.String("FromUserName", fromUser), zap.String("Content", content[:50]+"..."))

	// manually construct XML response (wrap all fields with CDATA, include AgentID)
	xmlResp := fmt.Sprintf(`<xml>
<ToUserName><![CDATA[%s]]></ToUserName>
<FromUserName><![CDATA[%s]]></FromUserName>
<CreateTime>%d</CreateTime>
<MsgType><![CDATA[text]]></MsgType>
<Content><![CDATA[%s]]></Content>
</xml>`, toUser, fromUser, time.Now().Unix(), content)

	// log the exact plaintext response for debugging
	h.logger.Debug("WeCom plaintext reply packet", zap.String("xml", xmlResp))

	// use text/xml as recommended by WeCom docs
	c.Header("Content-Type", "text/xml; charset=utf-8")
	c.String(http.StatusOK, xmlResp)
	h.logger.Debug("WeCom plaintext reply sent")
}

// —————— test endpoint (requires login, for validating bot logic, no DingTalk/Feishu client needed) ——————

// CreateRobotBindingCode creates a short-lived, single-use secret for the
// currently authenticated RBAC user. Only its hash is persisted.
func (h *RobotHandler) CreateRobotBindingCode(c *gin.Context) {
	session, ok := security.CurrentSession(c)
	if !ok || strings.TrimSpace(session.UserID) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized access"})
		return
	}
	random := make([]byte, 5)
	if _, err := rand.Read(random); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "generate a bind codefailed"})
		return
	}
	raw := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(random)
	code := raw[:4] + "-" + raw[4:]
	expiresAt := time.Now().Add(robotBindingCodeTTL)
	if err := h.db.CreateRobotBindingCode(session.UserID, hashRobotBindingCode(code), expiresAt); err != nil {
		h.logger.Warn("create robot binding code failed", zap.String("user_id", session.UserID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "generate a bind codefailed"})
		return
	}
	if h.audit != nil {
		h.audit.Record(c, audit.Entry{Category: "auth", Action: "robot_binding_code_create", Result: "success", ResourceType: "user", ResourceID: session.UserID, Message: "generated robot one-time binding code"})
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"code": code, "expires_at": expiresAt.UTC().Format(time.RFC3339), "expires_in_seconds": int(robotBindingCodeTTL.Seconds()),
	})
}

func (h *RobotHandler) ListMyRobotBindings(c *gin.Context) {
	session, ok := security.CurrentSession(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized access"})
		return
	}
	bindings, err := h.db.ListRobotUserBindings(session.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get robot bindings failed"})
		return
	}
	items := make([]gin.H, 0, len(bindings))
	for _, binding := range bindings {
		sum := sha256.Sum256([]byte(binding.ExternalUserID))
		items = append(items, gin.H{
			"id": binding.ID, "platform": binding.Platform, "external_user_hint": fmt.Sprintf("%x", sum[:4]),
			"enabled": binding.Enabled, "created_at": binding.CreatedAt, "updated_at": binding.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"bindings": items})
}

func (h *RobotHandler) DeleteMyRobotBinding(c *gin.Context) {
	session, ok := security.CurrentSession(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized access"})
		return
	}
	if err := h.db.DeleteRobotUserBindingForUser(c.Param("id"), session.UserID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "binding not found"})
		return
	}
	if h.audit != nil {
		h.audit.Record(c, audit.Entry{Category: "auth", Action: "robot_binding_revoke", Result: "success", ResourceType: "robot_binding", ResourceID: c.Param("id"), Message: "revoked bot platform account binding"})
	}
	c.Status(http.StatusNoContent)
}

// RobotTestRequest simulates a bot message request
type RobotTestRequest struct {
	Platform string `json:"platform"` // e.g. "dingtalk", "lark", "wecom"
	UserID   string `json:"user_id"`
	Text     string `json:"text"`
}

// HandleRobotTest for local validation: POST JSON { "platform", "user_id", "text" }, returns { "reply": "..." }
func (h *RobotHandler) HandleRobotTest(c *gin.Context) {
	var req RobotTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request body must be JSON with platform, user_id, text fields"})
		return
	}
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = "test"
	}
	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		userID = "test_user"
	}
	reply := h.HandleMessage(platform, userID, req.Text)
	c.JSON(http.StatusOK, gin.H{"reply": reply})
}

// sendWecomMessageViaAPI proactively sends a message via WeCom API (used for sending results after async processing)
func (h *RobotHandler) sendWecomMessageViaAPI(toUser, toParty, content string) {
	if !h.config.Robots.Wecom.Enabled {
		return
	}

	secret := h.config.Robots.Wecom.Secret
	corpID := h.config.Robots.Wecom.CorpID
	agentID := h.config.Robots.Wecom.AgentID

	if secret == "" || corpID == "" {
		h.logger.Warn("WeCom proactive API missing secret or corpID config")
		return
	}

	// Step 1: get access_token
	tokenURL := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s", corpID, secret)
	resp, err := http.Get(tokenURL)
	if err != nil {
		h.logger.Warn("WeCom get token failed", zap.Error(err))
		return
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		h.logger.Warn("WeCom token responseparsing failed", zap.Error(err))
		return
	}
	if tokenResp.ErrCode != 0 {
		h.logger.Warn("WeCom token fetch error", zap.String("errmsg", tokenResp.ErrMsg), zap.Int("errcode", tokenResp.ErrCode))
		return
	}

	// Step 2: construct send message request
	msgReq := map[string]interface{}{
		"touser":  toUser,
		"msgtype": "text",
		"agentid": agentID,
		"text": map[string]interface{}{
			"content": content,
		},
	}

	msgBody, err := json.Marshal(msgReq)
	if err != nil {
		h.logger.Warn("WeCommessageserialization failed", zap.Error(err))
		return
	}

	// Step 3: send message
	sendURL := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=%s", tokenResp.AccessToken)
	msgResp, err := http.Post(sendURL, "application/json", bytes.NewReader(msgBody))
	if err != nil {
		h.logger.Warn("WeCom proactive send message failed", zap.Error(err))
		return
	}
	defer msgResp.Body.Close()

	var sendResp struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		InvalidUser string `json:"invaliduser"`
		MsgID       string `json:"msgid"`
	}
	if err := json.NewDecoder(msgResp.Body).Decode(&sendResp); err != nil {
		h.logger.Warn("WeCom send response parsing failed", zap.Error(err))
		return
	}

	if sendResp.ErrCode == 0 {
		h.logger.Debug("WeCom proactive send message successful", zap.String("msgid", sendResp.MsgID))
	} else {
		h.logger.Warn("WeCom proactive send message failed", zap.String("errmsg", sendResp.ErrMsg), zap.Int("errcode", sendResp.ErrCode), zap.String("invaliduser", sendResp.InvalidUser))
	}
}

// —————— DingTalk ——————

// HandleDingtalkPOST DingTalk event callback (streaming etc.); currently a placeholder, returns 200
func (h *RobotHandler) HandleDingtalkPOST(c *gin.Context) {
	if !h.config.Robots.Dingtalk.Enabled {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	// DingTalk streaming/event callback format must be parsed per official docs and replied to asynchronously; here we just return 200
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// —————— Feishu ——————

// HandleLarkPOST Feishu event callback; currently a placeholder, returns 200; must return challenge during verification
func (h *RobotHandler) HandleLarkPOST(c *gin.Context) {
	if !h.config.Robots.Lark.Enabled {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	var body struct {
		Challenge string `json:"challenge"`
	}
	if err := c.ShouldBindJSON(&body); err == nil && body.Challenge != "" {
		c.JSON(http.StatusOK, gin.H{"challenge": body.Challenge})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}
