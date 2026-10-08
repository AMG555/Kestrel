package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"kestrel/internal/database"
	"kestrel/internal/multiagent"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type hitlRuntimeConfig struct {
	Enabled        bool
	Mode           string
	Reviewer       string
	SensitiveTools map[string]struct{}
	Timeout        time.Duration
}

type hitlDecision struct {
	Decision        string
	Comment         string
	EditedArguments map[string]interface{}
}

type pendingInterrupt struct {
	ConversationID string
	InterruptID    string
	Mode           string
	ToolName       string
	ToolCallID     string
	decideCh       chan hitlDecision
}

type HITLManager struct {
	db     *database.DB
	logger *zap.Logger

	mu      sync.RWMutex
	runtime map[string]hitlRuntimeConfig
	pending map[string]*pendingInterrupt
	// approvedExec: approval granted; queue for pending tool_result write-back (per-session FIFO)
	approvedExec map[string][]hitlApprovedExecTrack
}

func NewHITLManager(db *database.DB, logger *zap.Logger) *HITLManager {
	return &HITLManager{
		db:      db,
		logger:  logger,
		runtime: make(map[string]hitlRuntimeConfig),
		pending: make(map[string]*pendingInterrupt),
	}
}

func (m *HITLManager) EnsureSchema() error {
	if _, err := m.db.Exec(`
CREATE TABLE IF NOT EXISTS hitl_interrupts (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    message_id TEXT,
    mode TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    tool_call_id TEXT,
    payload TEXT,
    status TEXT NOT NULL,
    reviewer TEXT NOT NULL DEFAULT 'human',
    decision TEXT,
    decision_comment TEXT,
    created_at DATETIME NOT NULL,
    decided_at DATETIME
);`); err != nil {
		return err
	}
	_, err := m.db.Exec(`
CREATE TABLE IF NOT EXISTS hitl_conversation_configs (
    conversation_id TEXT PRIMARY KEY,
    enabled INTEGER NOT NULL DEFAULT 0,
    mode TEXT NOT NULL DEFAULT 'off',
    sensitive_tools TEXT NOT NULL DEFAULT '[]',
    timeout_seconds INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL
);`)
	if err != nil {
		return err
	}
	m.migrateHitlSchemaColumns()

	// On startup, cancel all orphaned pending interrupts from previous process.
	// Their in-memory channels are gone, so they can never be resolved.
	res, err := m.db.Exec(`UPDATE hitl_interrupts SET status='cancelled', decision='reject',
		decision_comment='process restarted', decided_at=CURRENT_TIMESTAMP, decided_by='system'
		WHERE status='pending'`)
	if err != nil {
		m.logger.Warn("failed to cancel orphaned HITL interrupts", zap.Error(err))
	} else if n, _ := res.RowsAffected(); n > 0 {
		m.logger.Info("cancelled orphaned HITL interrupts from previous process", zap.Int64("count", n))
	}
	if err := m.reconcileRestartInterruptedMessages(); err != nil {
		m.logger.Warn("failed to finalize assistant messages interrupted by process restart", zap.Error(err))
	}
	return nil
}

// reconcileRestartInterruptedMessages completes durable terminal state for
// historical assistant placeholders that have explicit evidence of being over:
// a terminal HITL/process event, or a later message in the same conversation.
// The evidence requirement avoids rewriting a placeholder that could still be
// recoverable by another runtime.
func (m *HITLManager) reconcileRestartInterruptedMessages() error {
	rows, err := m.db.Query(`
SELECT msg.id, msg.conversation_id,
       COALESCE((
           SELECT pd.event_type
           FROM process_details pd
           WHERE pd.message_id = msg.id
             AND pd.event_type IN ('cancelled', 'timeout', 'error')
           ORDER BY pd.created_at DESC LIMIT 1
       ), '') AS terminal_event,
       COALESCE((
           SELECT hi.status
           FROM hitl_interrupts hi
           WHERE hi.message_id = msg.id
           ORDER BY COALESCE(hi.decided_at, hi.created_at) DESC LIMIT 1
       ), '') AS hitl_status,
       COALESCE((
           SELECT hi.decision
           FROM hitl_interrupts hi
           WHERE hi.message_id = msg.id
           ORDER BY COALESCE(hi.decided_at, hi.created_at) DESC LIMIT 1
       ), '') AS hitl_decision,
       COALESCE((
           SELECT hi.decision_comment
           FROM hitl_interrupts hi
           WHERE hi.message_id = msg.id
           ORDER BY COALESCE(hi.decided_at, hi.created_at) DESC LIMIT 1
       ), '') AS decision_comment,
       COALESCE((
           SELECT MAX(COALESCE(hi.decided_at, hi.created_at))
           FROM hitl_interrupts hi
           WHERE hi.message_id = msg.id
       ), (
           SELECT MIN(later.created_at)
           FROM messages later
           WHERE later.conversation_id = msg.conversation_id
             AND later.created_at > msg.created_at
       ), (
           SELECT MAX(pd.created_at)
           FROM process_details pd
           WHERE pd.message_id = msg.id
       ), msg.updated_at, msg.created_at) AS interrupted_at
FROM messages msg
WHERE msg.role = 'assistant'
  -- '处理中...' is the legacy Chinese "Processing..." placeholder; 'processing...' (lowercase) is the current production value;
  -- 'Processing...' is retained for backward compatibility with existing database records.
  AND TRIM(msg.content) IN ('处理中...', 'processing...', 'Processing...')
  AND (
      EXISTS (
          SELECT 1 FROM hitl_interrupts hi
          WHERE hi.message_id = msg.id
            AND (hi.status IN ('cancelled', 'timeout')
                 OR (hi.status = 'decided' AND hi.decision = 'reject'))
      )
      OR EXISTS (
          SELECT 1 FROM process_details pd
          WHERE pd.message_id = msg.id
            AND pd.event_type IN ('cancelled', 'timeout', 'error')
      )
      OR EXISTS (
          SELECT 1 FROM messages later
          WHERE later.conversation_id = msg.conversation_id
            AND later.created_at > msg.created_at
      )
  )`)
	if err != nil {
		return err
	}
	type interruptedMessage struct {
		messageID       string
		conversationID  string
		terminalEvent   string
		hitlStatus      string
		hitlDecision    string
		decisionComment string
		interruptedAt   string
	}
	var interrupted []interruptedMessage
	for rows.Next() {
		var item interruptedMessage
		if err := rows.Scan(&item.messageID, &item.conversationID, &item.terminalEvent,
			&item.hitlStatus, &item.hitlDecision, &item.decisionComment, &item.interruptedAt); err != nil {
			rows.Close()
			return err
		}
		interrupted = append(interrupted, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(interrupted) == 0 {
		return nil
	}

	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, item := range interrupted {
		eventType := strings.ToLower(strings.TrimSpace(item.terminalEvent))
		decision := strings.ToLower(strings.TrimSpace(item.hitlDecision))
		comment := strings.ToLower(strings.TrimSpace(item.decisionComment))
		if eventType == "" {
			if strings.EqualFold(strings.TrimSpace(item.hitlStatus), "timeout") || strings.Contains(comment, "timeout") {
				eventType = "timeout"
			} else {
				eventType = "cancelled"
			}
		}

		notice := "Task interrupted due to service restart."
		reason := "process_restarted"
		switch eventType {
		case "timeout":
			notice = "Task approval timed out and was auto-rejected."
			reason = "hitl_timeout"
		case "error":
			notice = "Task execution failed and stopped."
			reason = "execution_error"
		case "cancelled":
			if decision == "reject" && comment != "process restarted" {
				notice = "Task approval rejected; execution stopped."
				reason = "hitl_rejected"
			} else if comment == "process restarted" {
				notice = "Task interrupted due to service restart; approval cancelled."
			}
		default:
			eventType = "cancelled"
		}
		detailData, _ := json.Marshal(map[string]string{"reason": reason, "status": eventType})
		// '处理中...' is the legacy Chinese "Processing..." placeholder; matched alongside 'Processing...' for backward-compatibility with existing database rows.
		result, err := tx.Exec(`
UPDATE messages
SET content = ?, updated_at = ?
WHERE id = ? AND TRIM(content) IN ('处理中...', 'processing...', 'Processing...')`,
			notice, item.interruptedAt, item.messageID)
		if err != nil {
			return err
		}
		updated, _ := result.RowsAffected()
		if updated == 0 {
			continue
		}
		if _, err := tx.Exec(`
INSERT INTO process_details (id, message_id, conversation_id, event_type, message, data, created_at)
SELECT ?, ?, ?, ?, ?, ?, ?
WHERE NOT EXISTS (
    SELECT 1 FROM process_details
    WHERE message_id = ? AND event_type IN ('cancelled', 'timeout', 'error')
)`, uuid.NewString(), item.messageID, item.conversationID, eventType, notice, string(detailData),
			item.interruptedAt, item.messageID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func normalizeHitlMode(mode string) string {
	v := strings.ToLower(strings.TrimSpace(mode))
	if v == "" {
		return "approval"
	}
	switch v {
	case "off":
		return "off"
	case "feedback", "followup":
		return "approval"
	case "approval", "review_edit":
		return v
	default:
		return "approval"
	}
}

func normalizeHitlDefaultMode(mode string) string {
	v := strings.ToLower(strings.TrimSpace(mode))
	switch v {
	case "feedback", "followup":
		return "approval"
	case "approval", "review_edit":
		return v
	default:
		return "off"
	}
}

func (m *HITLManager) ActivateConversation(conversationID string, req *HITLRequest) {
	if req == nil || !req.Enabled {
		m.DeactivateConversation(conversationID)
		return
	}
	tools := make(map[string]struct{})
	for _, t := range req.SensitiveTools {
		n := strings.ToLower(strings.TrimSpace(t))
		if n != "" {
			tools[n] = struct{}{}
		}
	}
	// timeout <= 0 means wait forever (no timeout).
	timeout := time.Duration(0)
	if req.TimeoutSeconds > 0 {
		timeout = time.Duration(req.TimeoutSeconds) * time.Second
	}
	m.mu.Lock()
	m.runtime[conversationID] = hitlRuntimeConfig{
		Enabled:        true,
		Mode:           normalizeHitlMode(req.Mode),
		Reviewer:       normalizeHitlReviewer(req.Reviewer),
		SensitiveTools: tools,
		Timeout:        timeout,
	}
	m.mu.Unlock()
}

func (m *HITLManager) DeactivateConversation(conversationID string) {
	m.mu.Lock()
	delete(m.runtime, conversationID)
	m.mu.Unlock()
}

// hitlConfigGlobalToolWhitelist comes from config.yaml hitl.tool_whitelist (deduplicated and cleaned), merged with built-in meta-tool auto-approval items.
func (h *AgentHandler) hitlConfigGlobalToolWhitelist() []string {
	if h == nil || h.config == nil {
		return multiagent.MergeHitlExemptMetaTools(nil)
	}
	raw := h.config.Hitl.ToolWhitelist
	seen := make(map[string]struct{})
	out := make([]string, 0, len(raw)+len(multiagent.HitlExemptMetaTools))
	for _, t := range raw {
		n := strings.ToLower(strings.TrimSpace(t))
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, strings.TrimSpace(t))
	}
	return multiagent.MergeHitlExemptMetaTools(out)
}

// hitlRequestWithMergedConfigWhitelist merges the whitelist from session/API with the config.yaml global whitelist and built-in meta-tool auto-approval items (union); used only for runtime Activate; not written to database.
func (h *AgentHandler) hitlRequestWithMergedConfigWhitelist(req *HITLRequest) *HITLRequest {
	if req == nil {
		return nil
	}
	seen := make(map[string]struct{})
	union := make([]string, 0, len(req.SensitiveTools)+16)
	add := func(t string) {
		n := strings.ToLower(strings.TrimSpace(t))
		if n == "" {
			return
		}
		if _, ok := seen[n]; ok {
			return
		}
		seen[n] = struct{}{}
		union = append(union, strings.TrimSpace(t))
	}
	for _, t := range h.hitlConfigGlobalToolWhitelist() {
		add(t)
	}
	for _, t := range req.SensitiveTools {
		add(t)
	}
	out := *req
	out.SensitiveTools = multiagent.MergeHitlExemptMetaTools(union)
	return &out
}

func (m *HITLManager) shouldInterrupt(conversationID, toolName string) (hitlRuntimeConfig, bool) {
	m.mu.RLock()
	cfg, ok := m.runtime[conversationID]
	m.mu.RUnlock()
	if !ok || !cfg.Enabled {
		return hitlRuntimeConfig{}, false
	}
	// Semantics: SensitiveTools now acts as a whitelist (tools exempt from approval).
	// empty whitelist => all tools require approval
	if len(cfg.SensitiveTools) == 0 {
		return cfg, true
	}
	_, inWhitelist := cfg.SensitiveTools[strings.ToLower(strings.TrimSpace(toolName))]
	return cfg, !inWhitelist
}

// NeedsToolApproval has the same semantics as the Agent tool layer's shouldInterrupt: true only when the session has HITL enabled and the tool is not in the auto-approval whitelist.
func (m *HITLManager) NeedsToolApproval(conversationID, toolName string) bool {
	if m == nil {
		return false
	}
	_, need := m.shouldInterrupt(conversationID, toolName)
	return need
}

func (m *HITLManager) CreatePendingInterrupt(conversationID, assistantMessageID, mode, toolName, toolCallID, payload, reviewer string) (*pendingInterrupt, error) {
	now := time.Now()
	id := "hitl_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	reviewer = normalizeHitlReviewer(reviewer)
	if _, err := m.db.Exec(`INSERT INTO hitl_interrupts
		(id, conversation_id, message_id, mode, tool_name, tool_call_id, payload, status, reviewer, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
		id, conversationID, assistantMessageID, mode, toolName, toolCallID, payload, reviewer, now); err != nil {
		return nil, err
	}
	// After refreshing the page, the sidebar relies on DB config; if only in-memory Activate without DB write, it will show "has pending approval but appears closed"
	_ = m.ensureConversationHITLModePersisted(conversationID, mode)
	p := &pendingInterrupt{
		ConversationID: conversationID,
		InterruptID:    id,
		Mode:           normalizeHitlMode(mode),
		ToolName:       toolName,
		ToolCallID:     toolCallID,
		decideCh:       make(chan hitlDecision, 1),
	}
	// Agent review does not wait for human decision and should not enter the human approval in-memory queue.
	if reviewer != "audit_agent" {
		m.mu.Lock()
		m.pending[id] = p
		m.mu.Unlock()
	}
	return p, nil
}

// ensureConversationHITLModePersisted writes mode to hitl_conversation_configs when a pending approval is created, to avoid GET config still showing disabled after page refresh.
func (m *HITLManager) ensureConversationHITLModePersisted(conversationID, interruptMode string) error {
	if strings.TrimSpace(conversationID) == "" {
		return nil
	}
	nm := normalizeHitlMode(interruptMode)
	if nm == "off" {
		return nil
	}
	cfg, err := m.LoadConversationConfig(conversationID)
	if err != nil {
		return err
	}
	if cfg.Enabled && normalizeHitlMode(cfg.Mode) == nm {
		return nil
	}
	cfg.Enabled = true
	cfg.Mode = nm
	if cfg.TimeoutSeconds < 0 {
		cfg.TimeoutSeconds = 0
	}
	return m.SaveConversationConfig(conversationID, cfg)
}

// PendingHITLInterruptMode returns the latest pending interrupt mode for the session (used to align with the DB "disabled" status when GET config is called).
func (m *HITLManager) PendingHITLInterruptMode(conversationID string) (string, bool) {
	if strings.TrimSpace(conversationID) == "" {
		return "", false
	}
	var mode string
	err := m.db.QueryRow(`SELECT mode FROM hitl_interrupts WHERE conversation_id = ? AND status = 'pending' ORDER BY created_at DESC LIMIT 1`, conversationID).
		Scan(&mode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false
		}
		return "", false
	}
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return "", false
	}
	return mode, true
}

func hitlStoredConfigEffective(cfg *HITLRequest) bool {
	if cfg == nil {
		return false
	}
	if cfg.Enabled {
		return true
	}
	return normalizeHitlMode(cfg.Mode) != "off"
}

func (m *HITLManager) ResolveInterrupt(interruptID, decision, comment string, editedArguments map[string]interface{}) error {
	decision = strings.ToLower(strings.TrimSpace(decision))
	if decision != "approve" && decision != "reject" {
		return errors.New("decision must be approve/reject")
	}
	m.mu.RLock()
	p, ok := m.pending[interruptID]
	m.mu.RUnlock()
	if !ok {
		return errors.New("interrupt not found or already resolved")
	}
	d := hitlDecision{
		Decision:        decision,
		Comment:         strings.TrimSpace(comment),
		EditedArguments: editedArguments,
	}
	select {
	case p.decideCh <- d:
		return nil
	default:
		return errors.New("interrupt already resolved or decision channel busy")
	}
}

func (m *HITLManager) SaveConversationConfig(conversationID string, req *HITLRequest) error {
	if strings.TrimSpace(conversationID) == "" {
		return errors.New("conversationId is required")
	}
	if req == nil {
		req = &HITLRequest{Enabled: false, Mode: "off", TimeoutSeconds: 0}
	}
	mode := normalizeHitlMode(req.Mode)
	if !req.Enabled {
		mode = "off"
	}
	tools, _ := json.Marshal(req.SensitiveTools)
	timeout := req.TimeoutSeconds
	if timeout < 0 {
		timeout = 0
	}
	_, err := m.db.Exec(`INSERT INTO hitl_conversation_configs
		(conversation_id, enabled, mode, reviewer, sensitive_tools, timeout_seconds, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(conversation_id) DO UPDATE SET
		enabled=excluded.enabled, mode=excluded.mode, reviewer=excluded.reviewer, sensitive_tools=excluded.sensitive_tools, timeout_seconds=excluded.timeout_seconds, updated_at=excluded.updated_at`,
		conversationID, boolToInt(req.Enabled), mode, normalizeHitlReviewer(req.Reviewer), string(tools), timeout, time.Now())
	return err
}

func (m *HITLManager) LoadConversationConfig(conversationID string) (*HITLRequest, error) {
	var enabledInt int
	var mode, reviewer, toolsJSON string
	var timeout int
	err := m.db.QueryRow(`SELECT enabled, mode, COALESCE(reviewer,'human'), sensitive_tools, timeout_seconds FROM hitl_conversation_configs WHERE conversation_id = ?`, conversationID).
		Scan(&enabledInt, &mode, &reviewer, &toolsJSON, &timeout)
	if errors.Is(err, sql.ErrNoRows) {
		return &HITLRequest{Enabled: false, Mode: "off", Reviewer: "human", SensitiveTools: []string{}, TimeoutSeconds: 0}, nil
	}
	if err != nil {
		return nil, err
	}
	if timeout < 0 {
		timeout = 0
	}
	tools := make([]string, 0)
	_ = json.Unmarshal([]byte(toolsJSON), &tools)
	return &HITLRequest{
		Enabled:        enabledInt == 1,
		Mode:           mode,
		Reviewer:       normalizeHitlReviewer(reviewer),
		SensitiveTools: tools,
		TimeoutSeconds: timeout,
	}, nil
}

func (m *HITLManager) HasConversationConfig(conversationID string) (bool, error) {
	if strings.TrimSpace(conversationID) == "" {
		return false, nil
	}
	var one int
	err := m.db.QueryRow(`SELECT 1 FROM hitl_conversation_configs WHERE conversation_id = ? LIMIT 1`, conversationID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (m *HITLManager) waitDecision(ctx context.Context, p *pendingInterrupt, timeout time.Duration) (hitlDecision, error) {
	defer func() {
		m.mu.Lock()
		delete(m.pending, p.InterruptID)
		m.mu.Unlock()
	}()
	var timeoutCh <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		timeoutCh = timer.C
	}
	select {
	case d := <-p.decideCh:
		// only review_edit mode allows parameter editing; all other modes ignore edited arguments
		if p.Mode != "review_edit" && len(d.EditedArguments) > 0 {
			d.EditedArguments = nil
		}
		_, _ = m.db.Exec(`UPDATE hitl_interrupts SET status='decided', decision=?, decision_comment=?, decided_at=?, decided_by='human' WHERE id=?`,
			d.Decision, d.Comment, time.Now(), p.InterruptID)
		return d, nil
	case <-timeoutCh:
		comment := "HITL timeout auto-reject for safety"
		_, _ = m.db.Exec(`UPDATE hitl_interrupts SET status='timeout', decision='reject', decision_comment=?, decided_at=?, decided_by='system' WHERE id=?`,
			comment, time.Now(), p.InterruptID)
		return hitlDecision{Decision: "reject", Comment: comment}, nil
	case <-ctx.Done():
		_, _ = m.db.Exec(`UPDATE hitl_interrupts SET status='cancelled', decision='reject', decision_comment='task cancelled', decided_at=?, decided_by='system' WHERE id=?`,
			time.Now(), p.InterruptID)
		return hitlDecision{Decision: "reject", Comment: "task cancelled"}, ctx.Err()
	}
}

func (h *AgentHandler) activateHITLForConversation(conversationID string, req *HITLRequest) {
	if h.hitlManager == nil {
		return
	}
	if req == nil {
		cfg, err := h.loadHITLConversationConfig(conversationID)
		if err == nil {
			req = cfg
		}
	}
	if req != nil && strings.TrimSpace(req.Reviewer) == "" {
		req.Reviewer = h.hitlEffectiveDefaultReviewer()
	}
	h.hitlManager.ActivateConversation(conversationID, h.hitlRequestWithMergedConfigWhitelist(req))
}

func (h *AgentHandler) loadHITLConversationConfig(conversationID string) (*HITLRequest, error) {
	cfg, err := h.hitlManager.LoadConversationConfig(conversationID)
	if err != nil {
		return nil, err
	}
	has, err := h.hitlManager.HasConversationConfig(conversationID)
	if err != nil {
		return nil, err
	}
	if !has {
		return h.hitlEffectiveDefaultRequest(), nil
	}
	return cfg, nil
}

func (h *AgentHandler) waitHITLApproval(runCtx context.Context, cancelRun context.CancelCauseFunc, conversationID, assistantMessageID, toolName, toolCallID string, payload map[string]interface{}, sendEventFunc func(eventType, message string, data interface{})) (*hitlDecision, error) {
	cfg, need := h.hitlManager.shouldInterrupt(conversationID, toolName)
	if !need {
		return nil, nil
	}
	h.enrichHitlApprovalPayload(conversationID, assistantMessageID, payload)
	approvalStartedAt := time.Now().UTC()
	timeoutSeconds := int(cfg.Timeout / time.Second)
	var approvalExpiresAt *time.Time
	if timeoutSeconds > 0 {
		expiresAt := approvalStartedAt.Add(cfg.Timeout)
		approvalExpiresAt = &expiresAt
	}
	auditBackend, auditModel := h.hitlAuditEngineInfo()
	payload["hitlApproval"] = map[string]interface{}{
		"createdAt":      approvalStartedAt,
		"timeoutSeconds": timeoutSeconds,
		"expiresAt":      approvalExpiresAt,
		"auditBackend":   auditBackend,
		"auditModel":     auditModel,
	}
	payloadRaw, _ := json.Marshal(payload)
	p, err := h.hitlManager.CreatePendingInterrupt(conversationID, assistantMessageID, cfg.Mode, toolName, toolCallID, string(payloadRaw), cfg.Reviewer)
	if err != nil {
		h.logger.Warn("create HITL interrupt failed", zap.Error(err))
		return nil, err
	}
	emitHITL := func(eventType, message string, eventData map[string]interface{}) {
		clientData := enrichProgressEventData(eventData, conversationID, assistantMessageID)
		if sendEventFunc != nil {
			sendEventFunc(eventType, message, clientData)
		}
		if strings.TrimSpace(assistantMessageID) != "" && h.db != nil {
			if err := h.db.AddProcessDetail(assistantMessageID, conversationID, eventType, message, clientData); err != nil {
				h.logger.Warn("save HITL process details failed", zap.Error(err), zap.String("eventType", eventType))
			}
		}
	}

	if cfg.Reviewer == "audit_agent" {
		emitHITL("hitl_audit_agent_started", "Audit Agent is reviewing this request", map[string]interface{}{
			"conversationId": conversationID,
			"interruptId":    p.InterruptID,
			"toolName":       toolName,
			"toolCallId":     toolCallID,
			"mode":           cfg.Mode,
			"reviewer":       "audit_agent",
			"status":         "audit_running",
			"payload":        payload,
		})
		ad := h.auditAgentReview(runCtx, cfg.Mode, toolName, payload)
		now := time.Now()
		_, _ = h.db.Exec(`UPDATE hitl_interrupts SET status='decided', decision=?, decision_comment=?, decided_at=?, decided_by='audit_agent' WHERE id=?`,
			ad.Decision, ad.Comment, now, p.InterruptID)
		emitHITL("hitl_audit_agent", "Audit Agent has made a decision", map[string]interface{}{
			"conversationId": conversationID,
			"interruptId":    p.InterruptID,
			"toolName":       toolName,
			"toolCallId":     toolCallID,
			"mode":           cfg.Mode,
			"status":         "decided",
			"decision":       ad.Decision,
			"comment":        ad.Comment,
			"editedArgs":     ad.EditedArguments,
			"decidedBy":      "audit_agent",
			"reviewer":       "audit_agent",
		})
		if ad.Decision == "reject" {
			emitHITL("hitl_rejected", "Audit Agent rejected this tool call", map[string]interface{}{
				"conversationId": conversationID,
				"interruptId":    p.InterruptID,
				"toolName":       toolName,
				"toolCallId":     toolCallID,
				"mode":           cfg.Mode,
				"decision":       "reject",
				"comment":        ad.Comment,
				"decidedBy":      "audit_agent",
				"reviewer":       "audit_agent",
			})
			return &ad, nil
		}
		emitHITL("hitl_resumed", "Audit Agent approved, continuing execution", map[string]interface{}{
			"conversationId": conversationID,
			"interruptId":    p.InterruptID,
			"toolName":       toolName,
			"toolCallId":     toolCallID,
			"mode":           cfg.Mode,
			"decision":       "approve",
			"comment":        ad.Comment,
			"editedArgs":     ad.EditedArguments,
			"decidedBy":      "audit_agent",
			"reviewer":       "audit_agent",
		})
		h.hitlManager.TrackApprovedHitlExecution(p.InterruptID, conversationID, toolName, toolCallID)
		return &ad, nil
	}

	emitHITL("hitl_interrupt", "Human-in-the-loop approval triggered", map[string]interface{}{
		"conversationId": conversationID,
		"interruptId":    p.InterruptID,
		"mode":           cfg.Mode,
		"toolName":       toolName,
		"toolCallId":     toolCallID,
		"reviewer":       "human",
		"status":         "pending",
		"createdAt":      approvalStartedAt,
		"timeoutSeconds": timeoutSeconds,
		"expiresAt":      approvalExpiresAt,
		"payload":        payload,
	})
	d, waitErr := h.hitlManager.waitDecision(runCtx, p, cfg.Timeout)
	if waitErr != nil {
		if cancelRun != nil && (errors.Is(waitErr, context.Canceled) || errors.Is(waitErr, context.DeadlineExceeded)) {
			cause := context.Cause(runCtx)
			switch {
			case errors.Is(cause, ErrTaskCancelled):
				cancelRun(ErrTaskCancelled)
			case cause != nil:
				cancelRun(cause)
			case errors.Is(waitErr, context.DeadlineExceeded):
				cancelRun(context.DeadlineExceeded)
			default:
				cancelRun(ErrTaskCancelled)
			}
		}
		return nil, waitErr
	}
	if d.Decision == "reject" {
		rejectMsg := "Manually rejected this tool call; model will continue iterating based on feedback"
		timedOut := strings.Contains(strings.ToLower(strings.TrimSpace(d.Comment)), "timeout")
		if timedOut {
			rejectMsg = "Approval timed out; auto-rejected for safety; model will continue iterating based on feedback"
		}
		status := "decided"
		decidedBy := "human"
		if timedOut {
			status = "timeout"
			decidedBy = "system"
		}
		emitHITL("hitl_rejected", rejectMsg, map[string]interface{}{
			"conversationId": conversationID,
			"interruptId":    p.InterruptID,
			"toolName":       toolName,
			"toolCallId":     toolCallID,
			"mode":           cfg.Mode,
			"status":         status,
			"decision":       "reject",
			"comment":        d.Comment,
			"decidedBy":      decidedBy,
			"reviewer":       "human",
		})
		return &d, nil
	}
	emitHITL("hitl_resumed", "Manually confirmed and approved, continuing execution", map[string]interface{}{
		"conversationId": conversationID,
		"interruptId":    p.InterruptID,
		"toolName":       toolName,
		"toolCallId":     toolCallID,
		"mode":           cfg.Mode,
		"decision":       "approve",
		"comment":        d.Comment,
		"editedArgs":     d.EditedArguments,
		"reviewer":       "human",
	})
	h.hitlManager.TrackApprovedHitlExecution(p.InterruptID, conversationID, toolName, toolCallID)
	return &d, nil
}

func (h *AgentHandler) handleHITLToolCall(runCtx context.Context, cancelRun context.CancelCauseFunc, conversationID, assistantMessageID string, data map[string]interface{}, sendEventFunc func(eventType, message string, data interface{})) {
	if h.hitlManager == nil {
		return
	}
	toolName, _ := data["toolName"].(string)
	toolCallID, _ := data["toolCallId"].(string)
	d, err := h.waitHITLApproval(runCtx, cancelRun, conversationID, assistantMessageID, toolName, toolCallID, data, sendEventFunc)
	if err != nil || d == nil {
		return
	}
	if len(d.EditedArguments) > 0 {
		if argsObj, ok := data["argumentsObj"].(map[string]interface{}); ok {
			for k := range argsObj {
				delete(argsObj, k)
			}
			for k, v := range d.EditedArguments {
				argsObj[k] = v
			}
			if b, mErr := json.Marshal(argsObj); mErr == nil {
				data["arguments"] = string(b)
			}
		}
	}
}

func (h *AgentHandler) ListHITLPending(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	pageSize = int(math.Max(1, math.Min(float64(pageSize), 200)))
	offset := (page - 1) * pageSize
	q, args := h.buildHitlListQuery(false)
	q, args = h.appendHitlListFilters(q, args, c)
	q, args = appendConversationAccessSQL(q, args, "conversation_id", notificationAccessFromContext(c))
	total, err := h.countHitlQuery(q, args)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	q += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, pageSize, offset)
	rows, err := h.db.Query(q, args...)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	items, err := h.scanHitlInterruptRows(rows)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "page": page, "pageSize": pageSize, "total": total})
}

type hitlDecisionReq struct {
	InterruptID     string                 `json:"interruptId" binding:"required"`
	Decision        string                 `json:"decision" binding:"required"`
	Comment         string                 `json:"comment,omitempty"`
	EditedArguments map[string]interface{} `json:"editedArguments,omitempty"`
}

func (h *AgentHandler) DecideHITLInterrupt(c *gin.Context) {
	var req hitlDecisionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if h.hitlManager == nil {
		c.JSON(500, gin.H{"error": "hitl manager unavailable"})
		return
	}
	if !h.hitlInterruptAllowed(c, req.InterruptID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}
	if err := h.hitlManager.ResolveInterrupt(req.InterruptID, req.Decision, req.Comment, req.EditedArguments); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "hitl", "decision", "HITL approval decision", "hitl_interrupt", req.InterruptID, map[string]interface{}{
			"decision": req.Decision,
		})
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AgentHandler) DismissHITLInterrupt(c *gin.Context) {
	var req struct {
		InterruptID string `json:"interruptId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if h.hitlManager == nil {
		c.JSON(500, gin.H{"error": "hitl manager unavailable"})
		return
	}
	if !h.hitlInterruptAllowed(c, req.InterruptID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}
	res, err := h.db.Exec(`UPDATE hitl_interrupts SET status='cancelled', decision='reject',
		decision_comment='dismissed by user', decided_at=CURRENT_TIMESTAMP, decided_by='human'
		WHERE id=? AND status='pending'`, req.InterruptID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		c.JSON(404, gin.H{"error": "interrupt not found or already resolved"})
		return
	}
	// Also drain from in-memory map if present
	h.hitlManager.mu.Lock()
	if p, ok := h.hitlManager.pending[req.InterruptID]; ok {
		delete(h.hitlManager.pending, req.InterruptID)
		select {
		case p.decideCh <- hitlDecision{Decision: "reject", Comment: "dismissed by user"}:
		default:
		}
	}
	h.hitlManager.mu.Unlock()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AgentHandler) interceptHITLForEinoTool(runCtx context.Context, cancelRun context.CancelCauseFunc, conversationID, assistantMessageID string, sendEventFunc func(eventType, message string, data interface{}), toolName, arguments string) (string, error) {
	payload := map[string]interface{}{
		"toolName":   toolName,
		"arguments":  arguments,
		"source":     "eino_middleware",
		"toolCallId": "",
	}
	var argsObj map[string]interface{}
	if strings.TrimSpace(arguments) != "" {
		_ = json.Unmarshal([]byte(arguments), &argsObj)
		if argsObj != nil {
			payload["argumentsObj"] = argsObj
		}
	}
	d, err := h.waitHITLApproval(runCtx, cancelRun, conversationID, assistantMessageID, toolName, "", payload, sendEventFunc)
	if err != nil || d == nil {
		return arguments, err
	}
	if d.Decision == "reject" {
		return arguments, multiagent.NewHumanRejectError(d.Comment)
	}
	if len(d.EditedArguments) > 0 {
		edited, mErr := json.Marshal(d.EditedArguments)
		if mErr == nil {
			return string(edited), nil
		}
	}
	return arguments, nil
}

type hitlConfigReq struct {
	ConversationID string `json:"conversationId" binding:"required"`
	HITLRequest
}

func (h *AgentHandler) GetHITLConversationConfig(c *gin.Context) {
	conversationID := strings.TrimSpace(c.Param("conversationId"))
	if conversationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversationId is required"})
		return
	}
	if !h.hitlConversationAllowed(c, conversationID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}
	cfg, err := h.loadHITLConversationConfig(conversationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !hitlStoredConfigEffective(cfg) {
		if pendMode, ok := h.hitlManager.PendingHITLInterruptMode(conversationID); ok {
			cfg2 := *cfg
			cfg2.Enabled = true
			cfg2.Mode = normalizeHitlMode(pendMode)
			if cfg2.TimeoutSeconds < 0 {
				cfg2.TimeoutSeconds = 0
			}
			cfg = &cfg2
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"conversationId":          conversationID,
		"hitl":                    cfg,
		"defaultMode":             h.hitlEffectiveDefaultMode(),
		"defaultReviewer":         h.hitlEffectiveDefaultReviewer(),
		"defaultTimeoutSeconds":   h.hitlEffectiveDefaultTimeoutSeconds(),
		"hitlGlobalToolWhitelist": h.hitlConfigGlobalToolWhitelist(),
	})
}

func (h *AgentHandler) UpsertHITLConversationConfig(c *gin.Context) {
	var req hitlConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.hitlConversationAllowed(c, req.ConversationID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}
	req.Mode = normalizeHitlMode(req.Mode)
	req.Reviewer = normalizeHitlReviewer(req.Reviewer)
	if strings.TrimSpace(req.Reviewer) == "" {
		req.Reviewer = h.hitlEffectiveDefaultReviewer()
	}
	if err := h.hitlManager.SaveConversationConfig(req.ConversationID, &req.HITLRequest); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.hitlWhitelistSaver != nil && len(req.SensitiveTools) > 0 {
		if err := h.hitlWhitelistSaver.MergeHitlToolWhitelistIntoConfig(req.SensitiveTools); err != nil {
			h.logger.Warn("HITL session config saved, but merging tool whitelist to config.yaml failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Session config saved, but write to config.yaml failed: " + err.Error(),
			})
			return
		}
	}
	h.hitlManager.ActivateConversation(req.ConversationID, h.hitlRequestWithMergedConfigWhitelist(&req.HITLRequest))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type mergeHitlGlobalWhitelistReq struct {
	SensitiveTools []string `json:"sensitiveTools"`
}

type setHitlGlobalWhitelistReq struct {
	ToolWhitelist []string `json:"toolWhitelist"`
}

// GetHITLGlobalToolWhitelist returns the global auto-approval tool whitelist from config.yaml.
func (h *AgentHandler) GetHITLGlobalToolWhitelist(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"toolWhitelist":   h.hitlConfigGlobalToolWhitelist(),
		"defaultReviewer": h.hitlEffectiveDefaultReviewer(),
	})
}

type setHitlDefaultReviewerReq struct {
	Reviewer string `json:"reviewer"`
}

type setHitlDefaultConfigReq struct {
	Mode           string `json:"mode"`
	Reviewer       string `json:"reviewer"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

func (h *AgentHandler) hitlDefaultConfigResponse() gin.H {
	backend, model := h.hitlAuditEngineInfo()
	return gin.H{
		"defaultMode":             h.hitlEffectiveDefaultMode(),
		"defaultReviewer":         h.hitlEffectiveDefaultReviewer(),
		"defaultTimeoutSeconds":   h.hitlEffectiveDefaultTimeoutSeconds(),
		"hitlGlobalToolWhitelist": h.hitlConfigGlobalToolWhitelist(),
		"auditBackend":            backend,
		"auditModel":              model,
	}
}

// GetHITLDefaultConfig returns the global default HITL config from config.yaml.
func (h *AgentHandler) GetHITLDefaultConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.hitlDefaultConfigResponse())
}

// UpdateHITLDefaultConfig writes the global default HITL config to config.yaml.
func (h *AgentHandler) UpdateHITLDefaultConfig(c *gin.Context) {
	if h.hitlDefaultReviewerSaver == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "HITL config persistence unavailable"})
		return
	}
	var req setHitlDefaultConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	mode := normalizeHitlDefaultMode(req.Mode)
	reviewer := normalizeHitlReviewer(req.Reviewer)
	timeoutSeconds := req.TimeoutSeconds
	if timeoutSeconds < 0 {
		timeoutSeconds = 0
	}
	if err := h.hitlDefaultReviewerSaver.UpdateHitlDefaultConfig(mode, reviewer, timeoutSeconds); err != nil {
		h.logger.Warn("write HITL default config to config.yaml failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.config != nil {
		h.config.Hitl.DefaultMode = mode
		h.config.Hitl.DefaultReviewer = reviewer
		h.config.Hitl.DefaultTimeoutSeconds = &timeoutSeconds
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "hitl", "default_config_update", "HITL global default config updated", "hitl_config", "default", nil)
	}
	out := h.hitlDefaultConfigResponse()
	out["ok"] = true
	c.JSON(http.StatusOK, out)
}

// GetHITLDefaultReviewer returns the global default reviewer from config.yaml.
func (h *AgentHandler) GetHITLDefaultReviewer(c *gin.Context) {
	c.JSON(http.StatusOK, h.hitlDefaultConfigResponse())
}

// UpdateHITLDefaultReviewer writes the global default reviewer to config.yaml (switches reviewer when no session is selected).
func (h *AgentHandler) UpdateHITLDefaultReviewer(c *gin.Context) {
	if h.hitlDefaultReviewerSaver == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "HITL config persistence unavailable"})
		return
	}
	var req setHitlDefaultReviewerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	reviewer := normalizeHitlReviewer(req.Reviewer)
	if err := h.hitlDefaultReviewerSaver.UpdateHitlDefaultReviewer(reviewer); err != nil {
		h.logger.Warn("write HITL default reviewer to config.yaml failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.config != nil {
		h.config.Hitl.DefaultReviewer = reviewer
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "hitl", "default_reviewer_update", "HITL global default reviewer updated", "hitl_config", "default_reviewer", nil)
	}
	out := h.hitlDefaultConfigResponse()
	out["ok"] = true
	c.JSON(http.StatusOK, out)
}

// SetHITLGlobalToolWhitelist replaces the entire global auto-approval tool whitelist in config.yaml.
func (h *AgentHandler) SetHITLGlobalToolWhitelist(c *gin.Context) {
	if h.hitlWhitelistSaver == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "HITL config persistence unavailable"})
		return
	}
	var req setHitlGlobalWhitelistReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.hitlWhitelistSaver.SetHitlToolWhitelist(req.ToolWhitelist); err != nil {
		h.logger.Warn("write HITL tool whitelist to config.yaml failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "hitl", "tool_whitelist_update", "HITL global whitelist updated", "hitl_config", "tool_whitelist", nil)
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":                        true,
		"toolWhitelist":             h.hitlConfigGlobalToolWhitelist(),
		"hitlGlobalToolWhitelist":   h.hitlConfigGlobalToolWhitelist(),
		"hitlGlobalWhitelistMerged": false,
	})
}

// MergeHITLGlobalToolWhitelist merges auto-approval tools submitted from the sidebar into config.yaml when there is no session ID (consistent with the whitelist persistence rules in PUT /hitl/config).
func (h *AgentHandler) MergeHITLGlobalToolWhitelist(c *gin.Context) {
	if h.hitlWhitelistSaver == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "HITL config persistence unavailable"})
		return
	}
	var req mergeHitlGlobalWhitelistReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.SensitiveTools) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"ok":                        true,
			"hitlGlobalToolWhitelist":   h.hitlConfigGlobalToolWhitelist(),
			"hitlGlobalWhitelistMerged": false,
		})
		return
	}
	if err := h.hitlWhitelistSaver.MergeHitlToolWhitelistIntoConfig(req.SensitiveTools); err != nil {
		h.logger.Warn("merge HITL tool whitelist to config.yaml failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":                        true,
		"hitlGlobalToolWhitelist":   h.hitlConfigGlobalToolWhitelist(),
		"hitlGlobalWhitelistMerged": true,
	})
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
