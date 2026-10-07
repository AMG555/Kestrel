package handler

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"kestrel/internal/database"
	"kestrel/internal/security"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NotificationHandler aggregates notifications (Phase 2: server-side unified computation).
type NotificationHandler struct {
	db           *database.DB
	agentHandler *AgentHandler
	logger       *zap.Logger
}

const notificationReadMaxRows = 150

// NotificationSummaryItem is a notification item.
type NotificationSummaryItem struct {
	ID         string `json:"id"`
	Level      string `json:"level"` // p0/p1/p2
	Type       string `json:"type"`
	Title      string `json:"title"`
	Desc       string `json:"desc"`
	Ts         string `json:"ts"` // RFC3339
	Count      int    `json:"count,omitempty"`
	Actionable bool   `json:"actionable"`
	Read       bool   `json:"read"`
	// The following fields are used for frontend deep-link navigation (notification as entry point).
	ConversationID  string `json:"conversationId,omitempty"`
	VulnerabilityID string `json:"vulnerabilityId,omitempty"`
	ExecutionID     string `json:"executionId,omitempty"`
	InterruptID     string `json:"interruptId,omitempty"`
	SessionID       string `json:"sessionId,omitempty"` // C2 session (e.g. new session online)
}

// NotificationSummaryResponse is the aggregated response.
type NotificationSummaryResponse struct {
	SinceMs     int64                     `json:"sinceMs"`
	GeneratedAt string                    `json:"generatedAt"`
	P0Count     int                       `json:"p0Count"`
	UnreadCount int                       `json:"unreadCount"`
	Counts      map[string]int            `json:"counts"`
	Items       []NotificationSummaryItem `json:"items"`
}

func NewNotificationHandler(db *database.DB, agentHandler *AgentHandler, logger *zap.Logger) *NotificationHandler {
	return &NotificationHandler{
		db:           db,
		agentHandler: agentHandler,
		logger:       logger,
	}
}

func parseSinceMs(raw string) int64 {
	v := strings.TrimSpace(raw)
	if v == "" {
		return 0
	}
	if ms, err := strconv.ParseInt(v, 10, 64); err == nil && ms > 0 {
		return ms
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UnixMilli()
	}
	return 0
}

func unixSecToRFC3339(sec int64) string {
	if sec <= 0 {
		return time.Now().UTC().Format(time.RFC3339)
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

func normalizedSinceSec(sinceMs int64) int64 {
	sec := sinceMs / 1000
	// SQLite default time precision is seconds; give a 1s look-back window to avoid missing rows added within the same second.
	if sec > 0 {
		return sec - 1
	}
	return 0
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func normalizeSinceMs(raw int64) int64 {
	if raw > 0 {
		return raw
	}
	// Defaults to the last 24 hours only, to avoid pulling all historical noise on first open.
	return time.Now().Add(-24 * time.Hour).UnixMilli()
}

func levelBySeverity(sev string) string {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical", "high":
		return "p0"
	case "medium":
		return "p1"
	default:
		return "p2"
	}
}

func requestWantsEnglish(c *gin.Context) bool {
	if c == nil {
		return false
	}
	lang := strings.ToLower(strings.TrimSpace(c.Query("lang")))
	if lang == "" {
		lang = strings.ToLower(strings.TrimSpace(c.GetHeader("Accept-Language")))
	}
	return strings.HasPrefix(lang, "en")
}

func i18nText(english bool, zh string, en string) string {
	if english {
		return en
	}
	return zh
}

func notificationAccessFromContext(c *gin.Context) database.RBACListAccess {
	session, ok := security.CurrentSession(c)
	if !ok {
		return database.RBACListAccess{}
	}
	return database.RBACListAccess{UserID: session.UserID, Scope: session.Scope}
}

func appendConversationAccessSQL(query string, args []interface{}, column string, access database.RBACListAccess) (string, []interface{}) {
	userID := strings.TrimSpace(access.UserID)
	if access.Scope == database.RBACScopeAll {
		return query, args
	}
	if userID == "" {
		return query + ` AND 1=0`, args
	}
	query += ` AND ` + column + ` IS NOT NULL AND ` + column + ` <> '' AND (
		EXISTS (SELECT 1 FROM conversations c WHERE c.id = ` + column + ` AND c.owner_user_id = ?)
		OR EXISTS (
			SELECT 1 FROM rbac_resource_assignments ra
			WHERE ra.user_id = ? AND ra.resource_type = 'conversation' AND ra.resource_id = ` + column + `
		)
		OR EXISTS (
			SELECT 1 FROM conversations c
			JOIN projects p ON p.id = c.project_id
			WHERE c.id = ` + column + ` AND p.owner_user_id = ?
		)
		OR EXISTS (
			SELECT 1 FROM conversations c
			JOIN rbac_resource_assignments pra ON pra.resource_id = c.project_id
			WHERE c.id = ` + column + ` AND pra.user_id = ? AND pra.resource_type = 'project'
		)
	)`
	args = append(args, userID, userID, userID, userID)
	return query, args
}

func appendVulnerabilityNotificationAccessSQL(query string, args []interface{}, access database.RBACListAccess) (string, []interface{}) {
	userID := strings.TrimSpace(access.UserID)
	if access.Scope == database.RBACScopeAll {
		return query, args
	}
	if userID == "" {
		return query + ` AND 1=0`, args
	}
	query += ` AND (
		owner_user_id = ?
		OR EXISTS (
			SELECT 1 FROM rbac_resource_assignments ra
			WHERE ra.user_id = ? AND ra.resource_type = 'vulnerability' AND ra.resource_id = vulnerabilities.id
		)
		OR (
			project_id IS NOT NULL AND project_id <> '' AND (
				EXISTS (SELECT 1 FROM projects p WHERE p.id = vulnerabilities.project_id AND p.owner_user_id = ?)
				OR EXISTS (
					SELECT 1 FROM rbac_resource_assignments pra
					WHERE pra.user_id = ? AND pra.resource_type = 'project' AND pra.resource_id = vulnerabilities.project_id
				)
			)
		)
		OR (
			conversation_id IS NOT NULL AND conversation_id <> '' AND (
				EXISTS (SELECT 1 FROM conversations c WHERE c.id = vulnerabilities.conversation_id AND c.owner_user_id = ?)
				OR EXISTS (
					SELECT 1 FROM rbac_resource_assignments cra
					WHERE cra.user_id = ? AND cra.resource_type = 'conversation' AND cra.resource_id = vulnerabilities.conversation_id
				)
			)
		)
	)`
	args = append(args, userID, userID, userID, userID, userID, userID)
	return query, args
}

func (h *NotificationHandler) loadPendingHITLItems(limit int, english bool, access database.RBACListAccess) ([]NotificationSummaryItem, error) {
	query := `
		SELECT
			id,
			conversation_id,
			tool_name,
			COALESCE(CAST(strftime('%s', created_at) AS INTEGER), 0)
		FROM hitl_interrupts
		WHERE status = 'pending'
	`
	args := []interface{}{}
	query, args = appendConversationAccessSQL(query, args, "conversation_id", access)
	query += ` ORDER BY created_at DESC
		LIMIT ?
	`
	args = append(args, limit)
	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]NotificationSummaryItem, 0, limit)
	for rows.Next() {
		var id, conversationID, toolName string
		var createdSec int64
		if err := rows.Scan(&id, &conversationID, &toolName, &createdSec); err != nil {
			continue
		}
		desc := i18nText(english, "HITL approval pending for conversation "+conversationID, "Conversation "+conversationID+" has pending HITL approval")
		if strings.TrimSpace(toolName) != "" {
			desc = i18nText(english, "tool "+toolName+" is waiting for approval", "Tool "+toolName+" is waiting for approval")
		}
		items = append(items, NotificationSummaryItem{
			ID:             "hitl:" + id,
			Level:          "p0",
			Type:           "hitl_pending",
			Title:          i18nText(english, "HITL Pending Approval", "HITL Pending Approval"),
			Desc:           desc,
			Ts:             unixSecToRFC3339(createdSec),
			Count:          1,
			Actionable:     true,
			Read:           false,
			ConversationID: conversationID,
			InterruptID:    id,
		})
	}
	return items, nil
}

func (h *NotificationHandler) loadVulnerabilityItems(sinceMs int64, limit int, english bool, access database.RBACListAccess) ([]NotificationSummaryItem, map[string]int, error) {
	sinceSec := normalizedSinceSec(sinceMs)
	query := `
		SELECT
			id,
			title,
			severity,
			conversation_id,
			COALESCE(CAST(strftime('%s', created_at) AS INTEGER), 0)
		FROM vulnerabilities
		WHERE CAST(strftime('%s', created_at) AS INTEGER) > ?
	`
	args := []interface{}{sinceSec}
	query, args = appendVulnerabilityNotificationAccessSQL(query, args, access)
	query += `
		ORDER BY created_at DESC
		LIMIT ?
	`
	args = append(args, limit)
	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	items := make([]NotificationSummaryItem, 0, limit)
	counts := map[string]int{
		"newCriticalVulns": 0,
		"newHighVulns":     0,
		"newMediumVulns":   0,
		"newLowVulns":      0,
		"newInfoVulns":     0,
	}
	for rows.Next() {
		var id, title, severity, conversationID string
		var createdSec int64
		if err := rows.Scan(&id, &title, &severity, &conversationID, &createdSec); err != nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(severity)) {
		case "critical":
			counts["newCriticalVulns"]++
		case "high":
			counts["newHighVulns"]++
		case "medium":
			counts["newMediumVulns"]++
		case "low":
			counts["newLowVulns"]++
		default:
			counts["newInfoVulns"]++
		}
		sevUpper := strings.ToUpper(strings.TrimSpace(severity))
		if sevUpper == "" {
			sevUpper = "INFO"
		}
		finalTitle := i18nText(english, "New Vulnerability ("+sevUpper+")", "New Vulnerability ("+sevUpper+")")
		finalDesc := strings.TrimSpace(title)
		if finalDesc == "" {
			finalDesc = i18nText(english, "(Untitled)", "(Untitled)")
		}
		items = append(items, NotificationSummaryItem{
			ID:              "vuln:" + id,
			Level:           levelBySeverity(severity),
			Type:            "vulnerability_created",
			Title:           finalTitle,
			Desc:            finalDesc,
			Ts:              unixSecToRFC3339(createdSec),
			Count:           1,
			Actionable:      false,
			Read:            false,
			ConversationID:  conversationID,
			VulnerabilityID: id,
		})
	}
	return items, counts, nil
}

// loadC2SessionOnlineEvents loads new session online events (c2_events: session + critical, consistent with Manager.IngestCheckIn).
func (h *NotificationHandler) loadC2SessionOnlineEvents(sinceMs int64, limit int, english bool, access database.RBACListAccess) ([]NotificationSummaryItem, int, error) {
	sinceSec := normalizedSinceSec(sinceMs)
	events, err := h.db.ListC2EventsForAccess(database.ListC2EventsFilter{
		Category: "session",
		Level:    "critical",
		Since:    ptrTime(time.Unix(sinceSec, 0)),
		Limit:    limit,
	}, access)
	if err != nil {
		return nil, 0, err
	}
	items := make([]NotificationSummaryItem, 0, limit)
	for _, e := range events {
		if e == nil {
			continue
		}
		desc := strings.TrimSpace(e.Message)
		if len(desc) > 220 {
			desc = desc[:200] + "…"
		}
		if desc == "" {
			desc = i18nText(english, "A new session was established", "A new session was created")
		}
		items = append(items, NotificationSummaryItem{
			ID:         "c2evt:" + e.ID,
			Level:      "p0",
			Type:       "c2_session_online",
			Title:      i18nText(english, "C2 new session online", "C2 new session online"),
			Desc:       desc,
			Ts:         e.CreatedAt.UTC().Format(time.RFC3339),
			Count:      1,
			Actionable: false,
			Read:       false,
			SessionID:  e.SessionID,
		})
	}
	return items, len(items), nil
}

func (h *NotificationHandler) loadFailedExecutionItems(sinceMs int64, limit int, english bool) ([]NotificationSummaryItem, int, error) {
	sinceSec := normalizedSinceSec(sinceMs)
	rows, err := h.db.Query(`
		SELECT
			id,
			tool_name,
			COALESCE(CAST(strftime('%s', start_time) AS INTEGER), 0)
		FROM tool_executions
		WHERE status = 'failed'
		  AND CAST(strftime('%s', start_time) AS INTEGER) > ?
		ORDER BY start_time DESC
		LIMIT ?
	`, sinceSec, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]NotificationSummaryItem, 0, limit)
	count := 0
	for rows.Next() {
		var id, toolName string
		var startSec int64
		if err := rows.Scan(&id, &toolName, &startSec); err != nil {
			continue
		}
		count++
		if strings.TrimSpace(toolName) == "" {
			toolName = i18nText(english, "unknowntool", "unknown")
		}
		items = append(items, NotificationSummaryItem{
			ID:          "exec_failed:" + id,
			Level:       "p0",
			Type:        "task_failed",
			Title:       i18nText(english, "Task Execution Failed", "Task Execution Failed"),
			Desc:        i18nText(english, "Tool "+toolName+" execution failed", "Tool "+toolName+" execution failed"),
			Ts:          unixSecToRFC3339(startSec),
			Count:       1,
			Actionable:  false,
			Read:        false,
			ExecutionID: id,
		})
	}
	return items, count, nil
}

func (h *NotificationHandler) summarizeLongRunningTasks(threshold time.Duration, english bool, access database.RBACListAccess) ([]NotificationSummaryItem, int) {
	if h.agentHandler == nil || h.agentHandler.tasks == nil {
		return nil, 0
	}
	tasks := h.agentHandler.tasks.GetActiveTasks()
	now := time.Now()
	items := make([]NotificationSummaryItem, 0, len(tasks))
	for _, t := range tasks {
		if t == nil {
			continue
		}
		if !h.notificationConversationAllowed(access, t.ConversationID) {
			continue
		}
		if now.Sub(t.StartedAt) >= threshold {
			items = append(items, NotificationSummaryItem{
				ID:             "task_long:" + t.ConversationID,
				Level:          "p1",
				Type:           "long_running_tasks",
				Title:          i18nText(english, "Long Running Task", "Long Running Task"),
				Desc:           i18nText(english, "Conversation "+t.ConversationID+" has been running over 15 minutes", "Conversation "+t.ConversationID+" has been running over 15 minutes"),
				Ts:             t.StartedAt.UTC().Format(time.RFC3339),
				Count:          1,
				Actionable:     true,
				Read:           false,
				ConversationID: t.ConversationID,
			})
		}
	}
	return items, len(items)
}

func (h *NotificationHandler) summarizeCompletedTasksSince(sinceMs int64, limit int, english bool, access database.RBACListAccess) ([]NotificationSummaryItem, int) {
	if h.agentHandler == nil || h.agentHandler.tasks == nil {
		return nil, 0
	}
	since := time.UnixMilli(sinceMs)
	completed := h.agentHandler.tasks.GetCompletedTasks()
	items := make([]NotificationSummaryItem, 0, limit)
	for _, t := range completed {
		if t == nil {
			continue
		}
		if !h.notificationConversationAllowed(access, t.ConversationID) {
			continue
		}
		if t.CompletedAt.After(since) {
			items = append(items, NotificationSummaryItem{
				ID:             "task_completed:" + t.ConversationID + ":" + strconv.FormatInt(t.CompletedAt.Unix(), 10),
				Level:          "p2",
				Type:           "task_completed",
				Title:          i18nText(english, "Task Completed", "Task Completed"),
				Desc:           i18nText(english, "Conversation "+t.ConversationID+" completed", "Conversation "+t.ConversationID+" completed"),
				Ts:             t.CompletedAt.UTC().Format(time.RFC3339),
				Count:          1,
				Actionable:     false,
				Read:           false,
				ConversationID: t.ConversationID,
			})
			if len(items) >= limit {
				break
			}
		}
	}
	return items, len(items)
}

func buildPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, "?")
	}
	return strings.Join(out, ",")
}

func (h *NotificationHandler) readStatesByIDs(userID string, ids []string) (map[string]bool, error) {
	result := make(map[string]bool, len(ids))
	userID = strings.TrimSpace(userID)
	if len(ids) == 0 || userID == "" {
		return result, nil
	}
	holders := buildPlaceholders(len(ids))
	query := "SELECT event_id FROM notification_reads_by_user WHERE user_id = ? AND event_id IN (" + holders + ")"
	args := make([]interface{}, 0, len(ids)+1)
	args = append(args, userID)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := h.db.Query(query, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		result[id] = true
	}
	return result, nil
}

func (h *NotificationHandler) applyReadStates(userID string, items []NotificationSummaryItem) ([]NotificationSummaryItem, error) {
	markableIDs := make([]string, 0, len(items))
	for _, item := range items {
		if item.Actionable {
			continue
		}
		markableIDs = append(markableIDs, item.ID)
	}
	readMap, err := h.readStatesByIDs(userID, markableIDs)
	if err != nil {
		return items, err
	}
	for i := range items {
		if items[i].Actionable {
			items[i].Read = false
			continue
		}
		items[i].Read = readMap[items[i].ID]
	}
	return items, nil
}

func filterVisibleItems(items []NotificationSummaryItem) []NotificationSummaryItem {
	out := make([]NotificationSummaryItem, 0, len(items))
	for _, item := range items {
		if item.Actionable || !item.Read {
			out = append(out, item)
		}
	}
	return out
}

func countP0(items []NotificationSummaryItem) int {
	total := 0
	for _, item := range items {
		if item.Level == "p0" {
			if item.Count > 0 {
				total += item.Count
			} else {
				total++
			}
		}
	}
	return total
}

func countUnread(items []NotificationSummaryItem) int {
	total := 0
	for _, item := range items {
		if item.Actionable || !item.Read {
			if item.Count > 0 {
				total += item.Count
			} else {
				total++
			}
		}
	}
	return total
}

func createNotificationReadTableIfNeeded(db *database.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS notification_reads_by_user (
			user_id TEXT NOT NULL,
			event_id TEXT NOT NULL,
			read_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY(user_id, event_id)
		);
	`)
	if err != nil {
		return err
	}
	_, idxErr := db.Exec(`CREATE INDEX IF NOT EXISTS idx_notification_reads_user_read_at ON notification_reads_by_user(user_id, read_at DESC);`)
	return idxErr
}

func pruneNotificationReads(db *database.DB, userID string, maxRows int) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	userID = strings.TrimSpace(userID)
	if maxRows <= 0 || userID == "" {
		return nil
	}
	_, err := db.Exec(`
		DELETE FROM notification_reads_by_user
		WHERE user_id = ? AND event_id NOT IN (
			SELECT event_id
			FROM notification_reads_by_user
			WHERE user_id = ?
			ORDER BY read_at DESC, rowid DESC
			LIMIT ?
		)
	`, userID, userID, maxRows)
	return err
}

type markReadRequest struct {
	EventIDs []string `json:"eventIds"`
}

func normalizeMarkableEventID(id string) (string, bool) {
	v := strings.TrimSpace(id)
	if v == "" {
		return "", false
	}
	// Only allow "hide-after-read" info-class events; actionable events do not participate in the read marker.
	allowedPrefixes := []string{
		"vuln:",
		"exec_failed:",
		"task_completed:",
		"c2evt:",
	}
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(v, prefix) {
			return v, true
		}
	}
	return "", false
}

// MarkRead marks events as read by event ID.
func (h *NotificationHandler) MarkRead(c *gin.Context) {
	if err := createNotificationReadTableIfNeeded(h.db); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare notification read table"})
		return
	}
	var req markReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if len(req.EventIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{"ok": true, "marked": 0})
		return
	}
	session, ok := security.CurrentSession(c)
	if !ok || strings.TrimSpace(session.UserID) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authenticated user"})
		return
	}
	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to begin transaction"})
		return
	}
	defer func() {
		_ = tx.Rollback()
	}()
	stmt, err := tx.Prepare(`
		INSERT INTO notification_reads_by_user(user_id, event_id, read_at)
		VALUES(?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id, event_id) DO UPDATE SET read_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare statement"})
		return
	}
	defer stmt.Close()
	marked := 0
	for _, raw := range req.EventIDs {
		id, ok := normalizeMarkableEventID(raw)
		if !ok {
			continue
		}
		if _, err := stmt.Exec(session.UserID, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark read"})
			return
		}
		marked++
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to commit read marks"})
		return
	}
	if err := pruneNotificationReads(h.db, session.UserID, notificationReadMaxRows); err != nil {
		h.logger.Warn("failed to trim notification read records", zap.Error(err))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "marked": marked})
}

// GetSummary returns the aggregated notification view (used for the header bell icon).
func (h *NotificationHandler) GetSummary(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database unavailable"})
		return
	}

	if err := createNotificationReadTableIfNeeded(h.db); err != nil {
		h.logger.Warn("failed to initialize notification read table", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize notification read table"})
		return
	}

	english := requestWantsEnglish(c)
	sinceMs := normalizeSinceMs(parseSinceMs(c.Query("since")))
	limit, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "50")))
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	access := notificationAccessFromContext(c)

	hitlItems := []NotificationSummaryItem{}
	if security.SessionHasPermission(c, "hitl:read") {
		var err error
		hitlItems, err = h.loadPendingHITLItems(limit, english, access)
		if err != nil {
			h.logger.Warn("failed to load HITL notifications", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to summarize hitl notifications"})
			return
		}
	}

	vulnItems := []NotificationSummaryItem{}
	vulnCounts := map[string]int{
		"newCriticalVulns": 0,
		"newHighVulns":     0,
		"newMediumVulns":   0,
		"newLowVulns":      0,
		"newInfoVulns":     0,
	}
	if security.SessionHasPermission(c, "vulnerability:read") {
		var err error
		vulnItems, vulnCounts, err = h.loadVulnerabilityItems(sinceMs, limit, english, access)
		if err != nil {
			h.logger.Warn("failed to load vulnerability notifications", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to summarize vulnerabilities"})
			return
		}
	}

	c2OnlineItems := []NotificationSummaryItem{}
	c2OnlineCount := 0
	if security.SessionHasPermission(c, "c2:read") {
		var err error
		c2OnlineItems, c2OnlineCount, err = h.loadC2SessionOnlineEvents(sinceMs, limit, english, access)
		if err != nil {
			h.logger.Warn("failed to load C2 session online notifications", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to summarize c2 session events"})
			return
		}
	}

	longRunningItems := []NotificationSummaryItem{}
	completedItems := []NotificationSummaryItem{}
	longRunningCount := 0
	completedCount := 0
	if security.SessionHasPermission(c, "tasks:read") || security.SessionHasPermission(c, "chat:read") {
		longRunningItems, longRunningCount = h.summarizeLongRunningTasks(15*time.Minute, english, access)
		completedItems, completedCount = h.summarizeCompletedTasksSince(sinceMs, limit, english, access)
	}

	items := make([]NotificationSummaryItem, 0, len(hitlItems)+len(vulnItems)+len(c2OnlineItems)+len(longRunningItems)+len(completedItems))
	items = append(items, hitlItems...)
	items = append(items, vulnItems...)
	items = append(items, c2OnlineItems...)
	items = append(items, longRunningItems...)
	items = append(items, completedItems...)

	session, _ := security.CurrentSession(c)
	items, err := h.applyReadStates(session.UserID, items)
	if err != nil {
		h.logger.Warn("failed to load notification read status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load notification read states"})
		return
	}
	items = filterVisibleItems(items)

	sort.Slice(items, func(i, j int) bool {
		ti, errI := time.Parse(time.RFC3339, items[i].Ts)
		tj, errJ := time.Parse(time.RFC3339, items[j].Ts)
		if errI != nil || errJ != nil {
			return i < j
		}
		return ti.After(tj)
	})

	p0Count := countP0(items)
	unreadCount := countUnread(items)
	c.JSON(http.StatusOK, NotificationSummaryResponse{
		SinceMs:     sinceMs,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		P0Count:     p0Count,
		UnreadCount: unreadCount,
		Counts: map[string]int{
			"hitlPending":      len(hitlItems),
			"newCriticalVulns": vulnCounts["newCriticalVulns"],
			"newHighVulns":     vulnCounts["newHighVulns"],
			"newMediumVulns":   vulnCounts["newMediumVulns"],
			"newLowVulns":      vulnCounts["newLowVulns"],
			"newInfoVulns":     vulnCounts["newInfoVulns"],
			"failedExecutions": 0,
			"longRunningTasks": longRunningCount,
			"completedTasks":   completedCount,
			"c2SessionOnline":  c2OnlineCount,
		},
		Items: items,
	})
}

func (h *NotificationHandler) notificationConversationAllowed(access database.RBACListAccess, conversationID string) bool {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return access.Scope == database.RBACScopeAll
	}
	return h.db.UserCanAccessResource(access.UserID, access.Scope, "conversation", conversationID)
}
