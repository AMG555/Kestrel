package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ProjectFilterUnbound: in the list API, project_id=__none__ means only conversations not bound to any project.
const ProjectFilterUnbound = "__none__"

// Conversation conversation
type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	ProjectID string    `json:"projectId,omitempty"`
	RoleName  string    `json:"roleName,omitempty"`
	AgentMode string    `json:"agentMode,omitempty"`
	Pinned    bool      `json:"pinned"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Messages  []Message `json:"messages,omitempty"`
}

// Message message
type Message struct {
	ID               string                   `json:"id"`
	ConversationID   string                   `json:"conversationId"`
	Role             string                   `json:"role"`
	Content          string                   `json:"content"`
	ReasoningContent string                   `json:"reasoningContent,omitempty"`
	MCPExecutionIDs  []string                 `json:"mcpExecutionIds,omitempty"`
	ProcessDetails   []map[string]interface{} `json:"processDetails,omitempty"`
	CreatedAt        time.Time                `json:"createdAt"`
	UpdatedAt        time.Time                `json:"updatedAt"`
}

// CreateConversation creates a new conversation
func (db *DB) CreateConversation(title string, meta ConversationCreateMeta) (*Conversation, error) {
	return db.CreateConversationWithWebshell("", title, meta)
}

// CreateConversationWithWebshell creates a new conversation, optionally bound to a WebShell connection ID (nil for a regular conversation)
func (db *DB) CreateConversationWithWebshell(webshellConnectionID, title string, meta ConversationCreateMeta) (*Conversation, error) {
	id := uuid.New().String()
	now := time.Now()

	projectID := strings.TrimSpace(meta.ProjectID)
	if projectID != "" {
		if _, err := db.GetProject(projectID); err != nil {
			return nil, err
		}
	}
	roleName := normalizeConversationRoleName(meta.RoleName)
	agentMode := normalizeConversationAgentMode(meta.AgentMode)

	var err error
	wsID := strings.TrimSpace(webshellConnectionID)
	switch {
	case wsID != "" && projectID != "":
		_, err = db.Exec(
			"INSERT INTO conversations (id, title, created_at, updated_at, webshell_connection_id, project_id, role_name, agent_mode) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			id, title, now, now, wsID, projectID, roleName, agentMode,
		)
	case wsID != "":
		_, err = db.Exec(
			"INSERT INTO conversations (id, title, created_at, updated_at, webshell_connection_id, role_name, agent_mode) VALUES (?, ?, ?, ?, ?, ?, ?)",
			id, title, now, now, wsID, roleName, agentMode,
		)
	case projectID != "":
		_, err = db.Exec(
			"INSERT INTO conversations (id, title, created_at, updated_at, project_id, role_name, agent_mode) VALUES (?, ?, ?, ?, ?, ?, ?)",
			id, title, now, now, projectID, roleName, agentMode,
		)
	default:
		_, err = db.Exec(
			"INSERT INTO conversations (id, title, created_at, updated_at, role_name, agent_mode) VALUES (?, ?, ?, ?, ?, ?)",
			id, title, now, now, roleName, agentMode,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create conversation: %w", err)
	}

	conv := &Conversation{
		ID:        id,
		Title:     title,
		ProjectID: projectID,
		RoleName:  roleName,
		AgentMode: agentMode,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if wsID != "" {
		meta.WebShellConnectionID = wsID
	}
	notifyConversationCreated(conv, meta)
	return conv, nil
}

// GetConversationByWebshellConnectionID retrieves the most recent conversation under a WebShell connection ID (used for AI assistant persistence)
func (db *DB) GetConversationByWebshellConnectionID(connectionID string) (*Conversation, error) {
	if connectionID == "" {
		return nil, fmt.Errorf("connectionID is empty")
	}
	var conv Conversation
	var createdAt, updatedAt string
	var pinned int
	err := db.QueryRow(
		"SELECT id, title, pinned, created_at, updated_at FROM conversations WHERE webshell_connection_id = ? ORDER BY updated_at DESC LIMIT 1",
		connectionID,
	).Scan(&conv.ID, &conv.Title, &pinned, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query conversation: %w", err)
	}
	conv.Pinned = pinned != 0
	if t, e := time.Parse("2006-01-02 15:04:05.999999999-07:00", createdAt); e == nil {
		conv.CreatedAt = t
	} else if t, e := time.Parse("2006-01-02 15:04:05", createdAt); e == nil {
		conv.CreatedAt = t
	} else {
		conv.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	}
	if t, e := time.Parse("2006-01-02 15:04:05.999999999-07:00", updatedAt); e == nil {
		conv.UpdatedAt = t
	} else if t, e := time.Parse("2006-01-02 15:04:05", updatedAt); e == nil {
		conv.UpdatedAt = t
	} else {
		conv.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	}
	messages, err := db.GetMessages(conv.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load messages: %w", err)
	}
	conv.Messages = messages

	// load process details and attach to corresponding messages (consistent with GetConversation so execution process is viewable after refresh)
	processDetailsMap, err := db.GetProcessDetailsByConversation(conv.ID)
	if err != nil {
		db.logger.Warn("failed to load process details", zap.Error(err))
		processDetailsMap = make(map[string][]ProcessDetail)
	}
	for i := range conv.Messages {
		if details, ok := processDetailsMap[conv.Messages[i].ID]; ok {
			details = DedupeConsecutiveProcessDetails(details)
			detailsJSON := make([]map[string]interface{}, len(details))
			for j, detail := range details {
				var data interface{}
				if detail.Data != "" {
					if err := json.Unmarshal([]byte(detail.Data), &data); err != nil {
						db.logger.Warn("failed to parse process details data", zap.Error(err))
					}
				}
				detailsJSON[j] = map[string]interface{}{
					"id":             detail.ID,
					"messageId":      detail.MessageID,
					"conversationId": detail.ConversationID,
					"eventType":      detail.EventType,
					"message":        detail.Message,
					"data":           data,
					"createdAt":      detail.CreatedAt,
				}
			}
			conv.Messages[i].ProcessDetails = detailsJSON
		}
	}

	return &conv, nil
}

// WebShellConversationItem is used for sidebar listing; does not include messages
type WebShellConversationItem struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ListConversationsByWebshellConnectionID lists all conversations under a WebShell connection ID (ordered by updated_at desc) for sidebar display
func (db *DB) ListConversationsByWebshellConnectionID(connectionID string) ([]WebShellConversationItem, error) {
	if connectionID == "" {
		return nil, nil
	}
	rows, err := db.Query(
		"SELECT id, title, updated_at FROM conversations WHERE webshell_connection_id = ? ORDER BY updated_at DESC",
		connectionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query conversation list: %w", err)
	}
	defer rows.Close()
	var list []WebShellConversationItem
	for rows.Next() {
		var item WebShellConversationItem
		var updatedAt string
		if err := rows.Scan(&item.ID, &item.Title, &updatedAt); err != nil {
			continue
		}
		if t, e := time.Parse("2006-01-02 15:04:05.999999999-07:00", updatedAt); e == nil {
			item.UpdatedAt = t
		} else if t, e := time.Parse("2006-01-02 15:04:05", updatedAt); e == nil {
			item.UpdatedAt = t
		} else {
			item.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

// ConversationExists reports whether a conversation row exists (lightweight check for audit links).
func (db *DB) ConversationExists(id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, nil
	}
	var one int
	err := db.QueryRow("SELECT 1 FROM conversations WHERE id = ? LIMIT 1", id).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetConversation retrieves a conversation
func (db *DB) GetConversation(id string) (*Conversation, error) {
	var conv Conversation
	var createdAt, updatedAt string
	var pinned int

	var projectID sql.NullString
	var roleName sql.NullString
	var agentMode sql.NullString
	err := db.QueryRow(
		"SELECT id, title, pinned, created_at, updated_at, project_id, role_name, agent_mode FROM conversations WHERE id = ?",
		id,
	).Scan(&conv.ID, &conv.Title, &pinned, &createdAt, &updatedAt, &projectID, &roleName, &agentMode)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("conversation not found")
		}
		return nil, fmt.Errorf("failed to query conversation: %w", err)
	}
	if projectID.Valid {
		conv.ProjectID = strings.TrimSpace(projectID.String)
	}
	if roleName.Valid {
		conv.RoleName = normalizeConversationRoleName(roleName.String)
	}
	if agentMode.Valid {
		conv.AgentMode = normalizeConversationAgentMode(agentMode.String)
	}

	// try multiple time format parsers
	var err1, err2 error
	conv.CreatedAt, err1 = time.Parse("2006-01-02 15:04:05.999999999-07:00", createdAt)
	if err1 != nil {
		conv.CreatedAt, err1 = time.Parse("2006-01-02 15:04:05", createdAt)
	}
	if err1 != nil {
		conv.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	}

	conv.UpdatedAt, err2 = time.Parse("2006-01-02 15:04:05.999999999-07:00", updatedAt)
	if err2 != nil {
		conv.UpdatedAt, err2 = time.Parse("2006-01-02 15:04:05", updatedAt)
	}
	if err2 != nil {
		conv.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	}

	conv.Pinned = pinned != 0

	// load messages
	messages, err := db.GetMessages(id)
	if err != nil {
		return nil, fmt.Errorf("failed to load messages: %w", err)
	}
	conv.Messages = messages

	// load process details (grouped by messageID)
	processDetailsMap, err := db.GetProcessDetailsByConversation(id)
	if err != nil {
		db.logger.Warn("failed to load process details", zap.Error(err))
		processDetailsMap = make(map[string][]ProcessDetail)
	}

	// attach process details to corresponding messages
	for i := range conv.Messages {
		if details, ok := processDetailsMap[conv.Messages[i].ID]; ok {
			details = DedupeConsecutiveProcessDetails(details)
			// convert ProcessDetail to JSON format for frontend use
			detailsJSON := make([]map[string]interface{}, len(details))
			for j, detail := range details {
				var data interface{}
				if detail.Data != "" {
					if err := json.Unmarshal([]byte(detail.Data), &data); err != nil {
						db.logger.Warn("failed to parse process details data", zap.Error(err))
					}
				}
				detailsJSON[j] = map[string]interface{}{
					"id":             detail.ID,
					"messageId":      detail.MessageID,
					"conversationId": detail.ConversationID,
					"eventType":      detail.EventType,
					"message":        detail.Message,
					"data":           data,
					"createdAt":      detail.CreatedAt,
				}
			}
			conv.Messages[i].ProcessDetails = detailsJSON
		}
	}

	return &conv, nil
}

// GetConversationLite retrieves a conversation (lightweight): includes messages but does not load process_details.
// Used for fast historical conversation switching to avoid sending large process_details to the frontend all at once and causing lag.
func (db *DB) GetConversationLite(id string) (*Conversation, error) {
	var conv Conversation
	var createdAt, updatedAt string
	var pinned int

	var projectID sql.NullString
	var roleName sql.NullString
	var agentMode sql.NullString
	err := db.QueryRow(
		"SELECT id, title, pinned, created_at, updated_at, project_id, role_name, agent_mode FROM conversations WHERE id = ?",
		id,
	).Scan(&conv.ID, &conv.Title, &pinned, &createdAt, &updatedAt, &projectID, &roleName, &agentMode)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("conversation not found")
		}
		return nil, fmt.Errorf("failed to query conversation: %w", err)
	}
	if projectID.Valid {
		conv.ProjectID = strings.TrimSpace(projectID.String)
	}
	if roleName.Valid {
		conv.RoleName = normalizeConversationRoleName(roleName.String)
	}
	if agentMode.Valid {
		conv.AgentMode = normalizeConversationAgentMode(agentMode.String)
	}

	// try multiple time format parsers
	var err1, err2 error
	conv.CreatedAt, err1 = time.Parse("2006-01-02 15:04:05.999999999-07:00", createdAt)
	if err1 != nil {
		conv.CreatedAt, err1 = time.Parse("2006-01-02 15:04:05", createdAt)
	}
	if err1 != nil {
		conv.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	}

	conv.UpdatedAt, err2 = time.Parse("2006-01-02 15:04:05.999999999-07:00", updatedAt)
	if err2 != nil {
		conv.UpdatedAt, err2 = time.Parse("2006-01-02 15:04:05", updatedAt)
	}
	if err2 != nil {
		conv.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	}

	conv.Pinned = pinned != 0

	// load messages (excluding process_details / reasoning_content to reduce historical conversation switching payload)
	messages, err := db.GetMessagesLite(id)
	if err != nil {
		return nil, fmt.Errorf("failed to load messages: %w", err)
	}
	conv.Messages = messages
	return &conv, nil
}

func normalizeConversationRoleName(roleName string) string {
	roleName = strings.TrimSpace(roleName)
	if roleName == "" {
		return "default"
	}
	return roleName
}

func normalizeConversationAgentMode(agentMode string) string {
	agentMode = strings.ToLower(strings.TrimSpace(agentMode))
	agentMode = strings.ReplaceAll(agentMode, "-", "_")
	switch agentMode {
	case "deep", "plan_execute", "supervisor":
		return agentMode
	default:
		return "eino_single"
	}
}

func (db *DB) SetConversationRoleName(id, roleName string) error {
	roleName = normalizeConversationRoleName(roleName)
	_, err := db.Exec(
		"UPDATE conversations SET role_name = ?, updated_at = ? WHERE id = ?",
		roleName, time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("failed to update conversation role: %w", err)
	}
	return nil
}

func (db *DB) SetConversationAgentMode(id, agentMode string) error {
	agentMode = normalizeConversationAgentMode(agentMode)
	_, err := db.Exec(
		"UPDATE conversations SET agent_mode = ? WHERE id = ?",
		agentMode, id,
	)
	if err != nil {
		return fmt.Errorf("updateconversationpatternfailed: %w", err)
	}
	return nil
}

func conversationProjectIDColumn(alias string) string {
	if alias != "" {
		return alias + ".project_id"
	}
	return "project_id"
}

func appendConversationProjectFilter(where string, args []interface{}, projectID, alias string) (string, []interface{}) {
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return where, args
	}
	col := conversationProjectIDColumn(alias)
	if pid == ProjectFilterUnbound {
		return where + fmt.Sprintf(" AND (%s IS NULL OR TRIM(COALESCE(%s, '')) = '')", col, col), args
	}
	return where + fmt.Sprintf(" AND %s = ?", col), append(args, pid)
}

func appendConversationAccessFilter(where string, args []interface{}, userID, scope, alias string) (string, []interface{}) {
	userID = strings.TrimSpace(userID)
	if userID == "" || scope == RBACScopeAll {
		return where, args
	}
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	where += fmt.Sprintf(` AND (%sowner_user_id = ? OR EXISTS (
		SELECT 1 FROM rbac_resource_assignments ra
		WHERE ra.user_id = ? AND ra.resource_type = 'conversation' AND ra.resource_id = %sid
	) OR EXISTS (
		SELECT 1 FROM projects p
		WHERE p.id = %sproject_id AND (
			p.owner_user_id = ? OR EXISTS (
				SELECT 1 FROM rbac_resource_assignments pra
				WHERE pra.user_id = ? AND pra.resource_type = 'project' AND pra.resource_id = p.id
			)
		)
	))`, prefix, prefix, prefix)
	args = append(args, userID, userID, userID, userID)
	return where, args
}

// CountConversations counts the number of conversations.
func (db *DB) CountConversations(search, projectID string) (int, error) {
	var count int
	var err error
	if search != "" {
		searchPattern := "%" + search + "%"
		where := ` WHERE (c.title LIKE ?
			    OR EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = c.id AND m.content LIKE ?))`
		args := []interface{}{searchPattern, searchPattern}
		where, args = appendConversationProjectFilter(where, args, projectID, "c")
		err = db.QueryRow(`SELECT COUNT(*) FROM conversations c`+where, args...).Scan(&count)
	} else {
		where := ""
		args := []interface{}{}
		where, args = appendConversationProjectFilter(where, args, projectID, "")
		if where != "" {
			where = " WHERE" + strings.TrimPrefix(where, " AND")
		}
		err = db.QueryRow(`SELECT COUNT(*) FROM conversations`+where, args...).Scan(&count)
	}
	if err != nil {
		return 0, fmt.Errorf("count conversations failed: %w", err)
	}
	return count, nil
}

func (db *DB) CountConversationsForAccess(search, projectID, userID, scope string) (int, error) {
	var count int
	var err error
	if search != "" {
		searchPattern := "%" + search + "%"
		where := ` WHERE (c.title LIKE ?
			    OR EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = c.id AND m.content LIKE ?))`
		args := []interface{}{searchPattern, searchPattern}
		where, args = appendConversationProjectFilter(where, args, projectID, "c")
		where, args = appendConversationAccessFilter(where, args, userID, scope, "c")
		err = db.QueryRow(`SELECT COUNT(*) FROM conversations c`+where, args...).Scan(&count)
	} else {
		where := ""
		args := []interface{}{}
		where, args = appendConversationProjectFilter(where, args, projectID, "")
		where, args = appendConversationAccessFilter(where, args, userID, scope, "")
		if where != "" {
			where = " WHERE" + strings.TrimPrefix(where, " AND")
		}
		err = db.QueryRow(`SELECT COUNT(*) FROM conversations`+where, args...).Scan(&count)
	}
	if err != nil {
		return 0, fmt.Errorf("count conversations failed: %w", err)
	}
	return count, nil
}

func conversationOrderClause(sortBy, tableAlias string) string {
	col := "updated_at"
	if strings.TrimSpace(strings.ToLower(sortBy)) == "created_at" {
		col = "created_at"
	}
	prefix := tableAlias
	if prefix != "" {
		prefix += "."
	}
	return "ORDER BY " + prefix + col + " DESC"
}

// ListConversations lists all conversations
func (db *DB) ListConversations(limit, offset int, search, sortBy, projectID string) ([]*Conversation, error) {
	var rows *sql.Rows
	var err error

	if search != "" {
		// use EXISTS subquery instead of LEFT JOIN + DISTINCT to avoid Cartesian product on large tables
		searchPattern := "%" + search + "%"
		orderClause := conversationOrderClause(sortBy, "c")
		where := ` WHERE (c.title LIKE ?
			    OR EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = c.id AND m.content LIKE ?))`
		args := []interface{}{searchPattern, searchPattern}
		where, args = appendConversationProjectFilter(where, args, projectID, "c")
		args = append(args, limit, offset)
		rows, err = db.Query(
			`SELECT c.id, c.title, COALESCE(c.pinned, 0), c.created_at, c.updated_at, c.project_id, c.role_name, c.agent_mode
			 FROM conversations c`+where+`
			 `+orderClause+`
			 LIMIT ? OFFSET ?`,
			args...,
		)
	} else {
		orderClause := conversationOrderClause(sortBy, "")
		where := ""
		args := []interface{}{}
		where, args = appendConversationProjectFilter(where, args, projectID, "")
		if where != "" {
			where = " WHERE" + strings.TrimPrefix(where, " AND")
		}
		args = append(args, limit, offset)
		rows, err = db.Query(
			"SELECT id, title, COALESCE(pinned, 0), created_at, updated_at, project_id, role_name, agent_mode FROM conversations"+where+" "+orderClause+" LIMIT ? OFFSET ?",
			args...,
		)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query conversation list: %w", err)
	}
	defer rows.Close()
	return scanConversationRows(rows)
}

func (db *DB) ListConversationsForAccess(limit, offset int, search, sortBy, projectID, userID, scope string) ([]*Conversation, error) {
	if scope == RBACScopeAll || strings.TrimSpace(userID) == "" {
		return db.ListConversations(limit, offset, search, sortBy, projectID)
	}
	var rows *sql.Rows
	var err error
	if search != "" {
		searchPattern := "%" + search + "%"
		orderClause := conversationOrderClause(sortBy, "c")
		where := ` WHERE (c.title LIKE ?
			    OR EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = c.id AND m.content LIKE ?))`
		args := []interface{}{searchPattern, searchPattern}
		where, args = appendConversationProjectFilter(where, args, projectID, "c")
		where, args = appendConversationAccessFilter(where, args, userID, scope, "c")
		args = append(args, limit, offset)
		rows, err = db.Query(
			`SELECT c.id, c.title, COALESCE(c.pinned, 0), c.created_at, c.updated_at, c.project_id, c.role_name, c.agent_mode
			 FROM conversations c`+where+`
			 `+orderClause+`
			 LIMIT ? OFFSET ?`, args...)
	} else {
		orderClause := conversationOrderClause(sortBy, "")
		where := ""
		args := []interface{}{}
		where, args = appendConversationProjectFilter(where, args, projectID, "")
		where, args = appendConversationAccessFilter(where, args, userID, scope, "")
		if where != "" {
			where = " WHERE" + strings.TrimPrefix(where, " AND")
		}
		args = append(args, limit, offset)
		rows, err = db.Query(
			"SELECT id, title, COALESCE(pinned, 0), created_at, updated_at, project_id, role_name, agent_mode FROM conversations"+where+" "+orderClause+" LIMIT ? OFFSET ?",
			args...)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query conversation list: %w", err)
	}
	defer rows.Close()
	return scanConversationRows(rows)
}

func scanConversationRows(rows *sql.Rows) ([]*Conversation, error) {
	var conversations []*Conversation
	for rows.Next() {
		var conv Conversation
		var createdAt, updatedAt string
		var pinned int
		var projectID sql.NullString
		var roleName sql.NullString
		var agentMode sql.NullString
		if err := rows.Scan(&conv.ID, &conv.Title, &pinned, &createdAt, &updatedAt, &projectID, &roleName, &agentMode); err != nil {
			return nil, fmt.Errorf("scanconversationfailed: %w", err)
		}
		if projectID.Valid {
			conv.ProjectID = strings.TrimSpace(projectID.String)
		}
		if roleName.Valid {
			conv.RoleName = normalizeConversationRoleName(roleName.String)
		}
		if agentMode.Valid {
			conv.AgentMode = normalizeConversationAgentMode(agentMode.String)
		}
		var err1, err2 error
		conv.CreatedAt, err1 = time.Parse("2006-01-02 15:04:05.999999999-07:00", createdAt)
		if err1 != nil {
			conv.CreatedAt, err1 = time.Parse("2006-01-02 15:04:05", createdAt)
		}
		if err1 != nil {
			conv.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		}
		conv.UpdatedAt, err2 = time.Parse("2006-01-02 15:04:05.999999999-07:00", updatedAt)
		if err2 != nil {
			conv.UpdatedAt, err2 = time.Parse("2006-01-02 15:04:05", updatedAt)
		}
		if err2 != nil {
			conv.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		}
		conv.Pinned = pinned != 0
		conversations = append(conversations, &conv)
	}
	return conversations, rows.Err()
}

// GetConversationTitle returns the conversation title (lightweight query, no messages loaded)
func (db *DB) GetConversationTitle(id string) (string, error) {
	var title string
	err := db.QueryRow("SELECT title FROM conversations WHERE id = ?", id).Scan(&title)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("conversation not found")
		}
		return "", fmt.Errorf("query conversation title failed: %w", err)
	}
	return title, nil
}

// UpdateConversationTitle updateconversation title
func (db *DB) UpdateConversationTitle(id, title string) error {
	// Note: do not update updated_at because rename should not change the conversation's updated_at
	_, err := db.Exec(
		"UPDATE conversations SET title = ? WHERE id = ?",
		title, id,
	)
	if err != nil {
		return fmt.Errorf("updateconversation titlefailed: %w", err)
	}
	return nil
}

// UpdateConversationPinned updates the conversation pinned status
func (db *DB) UpdateConversationPinned(id string, pinned bool) error {
	pinnedValue := 0
	if pinned {
		pinnedValue = 1
	}
	_, err := db.Exec(
		"UPDATE conversations SET pinned = ?, updated_at = ? WHERE id = ?",
		pinnedValue, time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("update conversation pinned status failed: %w", err)
	}
	return nil
}

// UpdateConversationTime updates the conversation timestamp
func (db *DB) UpdateConversationTime(id string) error {
	_, err := db.Exec(
		"UPDATE conversations SET updated_at = ? WHERE id = ?",
		time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("update conversation timestamp failed: %w", err)
	}
	return nil
}

// DeleteConversation deletes a conversation and all its associated data.
// Because the database foreign key constraint uses ON DELETE CASCADE, deleting a conversation automatically deletes:
// - messages
// - process_details (process details)
// - attack_chain_nodes (attack chain nodes)
// - attack_chain_edges (attack chain edges)
// Vulnerability records are retained: vulnerabilities.conversation_id uses ON DELETE SET NULL, only removing the association.
// Note: knowledge_retrieval_logs are explicitly cleaned up before deletion.
func (db *DB) DeleteConversation(id string) error {
	// Fill in vulnerability source tags before deleting the conversation, to allow tracing discoveries from deleted conversations in the vulnerability database.
	_, err := db.Exec(`
		UPDATE vulnerabilities
		SET conversation_tag = COALESCE(NULLIF(TRIM(conversation_tag), ''), (SELECT title FROM conversations WHERE id = ?))
		WHERE conversation_id = ?
	`, id, id)
	if err != nil {
		db.logger.Warn("update vulnerability source tags failed", zap.String("conversationId", id), zap.Error(err))
	}

	// Explicitly delete knowledge retrieval logs (even though the foreign key uses SET NULL, we manually delete for a clean purge)
	_, err = db.Exec("DELETE FROM knowledge_retrieval_logs WHERE conversation_id = ?", id)
	if err != nil {
		db.logger.Warn("deleteKnowledge retrievallogfailed", zap.String("conversationId", id), zap.Error(err))
		// do not return error, continue deleting conversation
	}

	projectID, _ := db.GetConversationProjectID(id)

	// delete conversation (CASCADE foreign key will automatically delete other related data)
	_, err = db.Exec("DELETE FROM conversations WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete conversationfailed: %w", err)
	}
	db.removeConversationScopedDirs(id, projectID)

	db.logger.Info("conversation deleted (vulnerability records retained)", zap.String("conversationId", id))
	return nil
}

func sanitizeConversationPathSegment(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "default"
	}
	s = strings.ReplaceAll(s, string(filepath.Separator), "-")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	s = strings.ReplaceAll(s, "..", "__")
	if len(s) > 180 {
		s = s[:180]
	}
	return s
}

func (db *DB) removeConversationScopedDir(base, conversationID, label string) {
	base = strings.TrimSpace(base)
	if base == "" {
		return
	}
	dir := filepath.Join(base, sanitizeConversationPathSegment(conversationID))
	if rmErr := os.RemoveAll(dir); rmErr != nil {
		if db.logger != nil {
			db.logger.Warn("delete session directory failed",
				zap.String("conversationId", conversationID),
				zap.String("kind", label),
				zap.String("dir", dir),
				zap.Error(rmErr))
		}
	}
}

func (db *DB) einoReductionBaseDir() string {
	if db == nil {
		return ""
	}
	if base := strings.TrimSpace(db.einoReductionRootDir); base != "" {
		return base
	}
	return filepath.Join("tmp", "reduction")
}

// EinoReductionBaseDir returns the configured reduction cache root.
func (db *DB) EinoReductionBaseDir() string {
	return db.einoReductionBaseDir()
}

// ConversationArtifactsBaseDir returns the conversation-scoped artifacts root.
func (db *DB) ConversationArtifactsBaseDir() string {
	if db == nil {
		return ""
	}
	return strings.TrimSpace(db.conversationArtifactsDir)
}

// EinoWorkspaceBaseDir returns the configured agent workspace root.
func (db *DB) EinoWorkspaceBaseDir() string {
	return db.einoWorkspaceBaseDir()
}

func (db *DB) einoWorkspaceBaseDir() string {
	if db == nil {
		return ""
	}
	if base := strings.TrimSpace(db.einoWorkspaceRootDir); base != "" {
		return base
	}
	return filepath.Join("tmp", "workspace")
}

func (db *DB) removeConversationScopedDirs(conversationID, projectID string) {
	// summarization transcript, etc.
	db.removeConversationScopedDir(db.conversationArtifactsDir, conversationID, "conversation_artifacts")
	// Eino plantask JSON boards (skills_dir/.eino/plantask/<id>/).
	db.removeConversationScopedDir(db.einoPlantaskBaseDir, conversationID, "plantask")
	// Eino ADK runner checkpoints (checkpoint_dir/<id>/).
	db.removeConversationScopedDir(db.einoCheckpointBaseDir, conversationID, "eino_checkpoint")
	// Upload attachments always belong to a single conversation; project-bound conversations are also deleted, so this is outside the projectID check.
	db.removeChatUploadDirs(conversationID)
	// Eino reduction persisted tool outputs (tmp/reduction/conversations/<id>/).
	// Project-bound sessions share projects/<id>/ — skip on single conversation delete.
	if strings.TrimSpace(projectID) == "" {
		reductionBase := filepath.Join(db.einoReductionBaseDir(), "conversations")
		db.removeConversationScopedDir(reductionBase, conversationID, "reduction")
		workspaceBase := filepath.Join(db.einoWorkspaceBaseDir(), "conversations")
		db.removeConversationScopedDir(workspaceBase, conversationID, "workspace")
	}
}

// removeChatUploadDirs deletes the upload directories for a conversation under chat_uploads/<date>/<session ID>/.
// This root directory has an extra date layer compared to other artifacts, so removeConversationScopedDir cannot be reused.
func (db *DB) removeChatUploadDirs(conversationID string) {
	base := strings.TrimSpace(db.chatUploadsDir)
	if base == "" || strings.TrimSpace(conversationID) == "" {
		return
	}
	seg := sanitizeConversationPathSegment(conversationID)
	dates, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, dateDir := range dates {
		if !dateDir.IsDir() {
			continue
		}
		dir := filepath.Join(base, dateDir.Name(), seg)
		if rmErr := os.RemoveAll(dir); rmErr != nil && db.logger != nil {
			db.logger.Warn("delete session upload directory failed",
				zap.String("conversationId", conversationID),
				zap.String("kind", "chat_uploads"),
				zap.String("dir", dir),
				zap.Error(rmErr))
		}
	}
}

func (db *DB) removeProjectScopedDirs(projectID string) {
	// Eino reduction persisted tool outputs (tmp/reduction/projects/<id>/).
	reductionBase := filepath.Join(db.einoReductionBaseDir(), "projects")
	db.removeConversationScopedDir(reductionBase, projectID, "reduction")
	// Agent download/analysis workspace (tmp/workspace/projects/<id>/).
	workspaceBase := filepath.Join(db.einoWorkspaceBaseDir(), "projects")
	db.removeConversationScopedDir(workspaceBase, projectID, "workspace")
}

// SaveAgentTrace saves the last-round agent message trace and assistant output summary.
// SQLite column names remain last_react_input / last_react_output for backwards compatibility; semantically these represent the full-mode agent trace, not ReAct-only.
func (db *DB) SaveAgentTrace(conversationID, traceInputJSON, assistantOutput string) error {
	_, err := db.Exec(
		"UPDATE conversations SET last_react_input = ?, last_react_output = ?, updated_at = ? WHERE id = ?",
		traceInputJSON, assistantOutput, time.Now(), conversationID,
	)
	if err != nil {
		return fmt.Errorf("save agent trace failed: %w", err)
	}
	return nil
}

// GetAgentTrace reads the saved agent trace from conversations (column names last_react_*).
func (db *DB) GetAgentTrace(conversationID string) (traceInputJSON, assistantOutput string, err error) {
	var input, output sql.NullString
	err = db.QueryRow(
		"SELECT last_react_input, last_react_output FROM conversations WHERE id = ?",
		conversationID,
	).Scan(&input, &output)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", fmt.Errorf("conversation not found")
		}
		return "", "", fmt.Errorf("get agent trace failed: %w", err)
	}

	if input.Valid {
		traceInputJSON = input.String
	}
	if output.Valid {
		assistantOutput = output.String
	}

	return traceInputJSON, assistantOutput, nil
}

// ConversationHasToolProcessDetails reports whether a conversation has persisted tool call/result records (used to determine attack chain when MCP execution IDs are not aggregated in multi-agent scenarios)。
func (db *DB) ConversationHasToolProcessDetails(conversationID string) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM process_details WHERE conversation_id = ? AND event_type IN ('tool_call', 'tool_result')`,
		conversationID,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("query process details failed: %w", err)
	}
	return n > 0, nil
}

// AddMessage adds a message to the conversation
func (db *DB) AddMessage(conversationID, role, content string, mcpExecutionIDs []string) (*Message, error) {
	id := uuid.New().String()
	now := time.Now()

	var mcpIDsJSON string
	if len(mcpExecutionIDs) > 0 {
		jsonData, err := json.Marshal(mcpExecutionIDs)
		if err != nil {
			db.logger.Warn("serialize MCP execution IDs failed", zap.Error(err))
		} else {
			mcpIDsJSON = string(jsonData)
		}
	}

	_, err := db.Exec(
		"INSERT INTO messages (id, conversation_id, role, content, reasoning_content, mcp_execution_ids, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		id, conversationID, role, content, "", mcpIDsJSON, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("add message failed: %w", err)
	}

	// update conversation timestamp
	if err := db.UpdateConversationTime(conversationID); err != nil {
		db.logger.Warn("update conversation timestamp failed", zap.Error(err))
	}

	message := &Message{
		ID:              id,
		ConversationID:  conversationID,
		Role:            role,
		Content:         content,
		MCPExecutionIDs: mcpExecutionIDs,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	return message, nil
}

// UpdateAssistantMessageFinalize updates the assistant message to its final state (body, MCP IDs, reasoning chain text, for replay when no trace fallback is available).
func (db *DB) UpdateAssistantMessageFinalize(messageID, content string, mcpExecutionIDs []string, reasoningContent string) error {
	var mcpIDsJSON string
	if len(mcpExecutionIDs) > 0 {
		jsonData, err := json.Marshal(mcpExecutionIDs)
		if err != nil {
			return fmt.Errorf("serialize MCP execution IDs failed: %w", err)
		}
		mcpIDsJSON = string(jsonData)
	}
	_, err := db.Exec(
		"UPDATE messages SET content = ?, mcp_execution_ids = ?, reasoning_content = ?, updated_at = ? WHERE id = ?",
		content, mcpIDsJSON, strings.TrimSpace(reasoningContent), time.Now(), messageID,
	)
	if err != nil {
		return fmt.Errorf("updateassistant messagefailed: %w", err)
	}
	return nil
}

// GetMessages returns all messages for a conversation
func (db *DB) GetMessages(conversationID string) ([]Message, error) {
	rows, err := db.Query(
		"SELECT id, conversation_id, role, content, reasoning_content, mcp_execution_ids, created_at, updated_at FROM messages WHERE conversation_id = ? ORDER BY created_at ASC, rowid ASC",
		conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("query messages failed: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		var reasoning sql.NullString
		var mcpIDsJSON sql.NullString
		var createdAt string
		var updatedAt sql.NullString

		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &reasoning, &mcpIDsJSON, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scanmessagefailed: %w", err)
		}
		if reasoning.Valid {
			msg.ReasoningContent = reasoning.String
		}

		// try multiple time format parsers
		var err error
		msg.CreatedAt, err = time.Parse("2006-01-02 15:04:05.999999999-07:00", createdAt)
		if err != nil {
			msg.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt)
		}
		if err != nil {
			msg.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		}

		// updated_at compatibility with old databases: fall back to created_at when field is missing or null
		if updatedAt.Valid && strings.TrimSpace(updatedAt.String) != "" {
			msg.UpdatedAt, err = time.Parse("2006-01-02 15:04:05.999999999-07:00", updatedAt.String)
			if err != nil {
				msg.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAt.String)
			}
			if err != nil {
				msg.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt.String)
			}
		}
		if msg.UpdatedAt.IsZero() {
			msg.UpdatedAt = msg.CreatedAt
		}

		// parse MCP execution IDs
		if mcpIDsJSON.Valid && mcpIDsJSON.String != "" {
			if err := json.Unmarshal([]byte(mcpIDsJSON.String), &msg.MCPExecutionIDs); err != nil {
				db.logger.Warn("parse MCP execution IDs failed", zap.Error(err))
			}
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

// GetMessagesLite returns conversation messages (excluding reasoning_content), used for fast historical conversation switching.
func (db *DB) GetMessagesLite(conversationID string) ([]Message, error) {
	rows, err := db.Query(
		"SELECT id, conversation_id, role, content, mcp_execution_ids, created_at, updated_at FROM messages WHERE conversation_id = ? ORDER BY created_at ASC, rowid ASC",
		conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("query messages failed: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		var mcpIDsJSON sql.NullString
		var createdAt string
		var updatedAt sql.NullString

		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &mcpIDsJSON, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scanmessagefailed: %w", err)
		}

		var err error
		msg.CreatedAt, err = time.Parse("2006-01-02 15:04:05.999999999-07:00", createdAt)
		if err != nil {
			msg.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt)
		}
		if err != nil {
			msg.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		}

		if updatedAt.Valid && strings.TrimSpace(updatedAt.String) != "" {
			msg.UpdatedAt, err = time.Parse("2006-01-02 15:04:05.999999999-07:00", updatedAt.String)
			if err != nil {
				msg.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAt.String)
			}
			if err != nil {
				msg.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt.String)
			}
		}
		if msg.UpdatedAt.IsZero() {
			msg.UpdatedAt = msg.CreatedAt
		}

		if mcpIDsJSON.Valid && mcpIDsJSON.String != "" {
			if err := json.Unmarshal([]byte(mcpIDsJSON.String), &msg.MCPExecutionIDs); err != nil {
				db.logger.Warn("parse MCP execution IDs failed", zap.Error(err))
			}
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

// turnSliceRange locates the [start, end) index range of the conversation turn containing the given message ID within msgs (msgs must be in ascending time order, consistent with GetMessages).
// A turn = from a user message up to (but not including) the next user message (including all intermediate assistant messages).
func turnSliceRange(msgs []Message, anchorID string) (start, end int, err error) {
	idx := -1
	for i := range msgs {
		if msgs[i].ID == anchorID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, 0, fmt.Errorf("message not found")
	}
	start = idx
	for start > 0 && msgs[start].Role != "user" {
		start--
	}
	if start < len(msgs) && msgs[start].Role != "user" {
		start = 0
	}
	end = len(msgs)
	for i := start + 1; i < len(msgs); i++ {
		if msgs[i].Role == "user" {
			end = i
			break
		}
	}
	return start, end, nil
}

// DeleteConversationTurn deletes all messages in the turn containing the anchor message (user question + assistant replies for that turn), and clears last_react_* to avoid inconsistency with the messages table.
func (db *DB) DeleteConversationTurn(conversationID, anchorMessageID string) (deletedIDs []string, err error) {
	msgs, err := db.GetMessages(conversationID)
	if err != nil {
		return nil, err
	}
	start, end, err := turnSliceRange(msgs, anchorMessageID)
	if err != nil {
		return nil, err
	}
	if start >= end {
		return nil, fmt.Errorf("empty turn range")
	}
	deletedIDs = make([]string, 0, end-start)
	for i := start; i < end; i++ {
		deletedIDs = append(deletedIDs, msgs[i].ID)
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	ph := strings.Repeat("?,", len(deletedIDs))
	ph = ph[:len(ph)-1]
	args := make([]interface{}, 0, 1+len(deletedIDs))
	args = append(args, conversationID)
	for _, id := range deletedIDs {
		args = append(args, id)
	}
	res, err := tx.Exec(
		"DELETE FROM messages WHERE conversation_id = ? AND id IN ("+ph+")",
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("delete messages: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if int(n) != len(deletedIDs) {
		return nil, fmt.Errorf("deleted count mismatch")
	}

	_, err = tx.Exec(
		`UPDATE conversations SET last_react_input = NULL, last_react_output = NULL, updated_at = ? WHERE id = ?`,
		time.Now(), conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("clear react data: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	db.logger.Info("conversation turn deleted",
		zap.String("conversationId", conversationID),
		zap.Strings("deletedMessageIds", deletedIDs),
		zap.Int("count", len(deletedIDs)),
	)
	return deletedIDs, nil
}

// ProcessDetail is a process details event
type ProcessDetail struct {
	ID             string    `json:"id"`
	MessageID      string    `json:"messageId"`
	ConversationID string    `json:"conversationId"`
	EventType      string    `json:"eventType"` // iteration, thinking, reasoning_chain, tool_calls_detected, tool_call, tool_result, progress, error
	Message        string    `json:"message"`
	Data           string    `json:"data"` // data in JSON format
	CreatedAt      time.Time `json:"createdAt"`
}

// GetTurnUserMessage returns the user's original text in the turn containing the anchor message (most recent user message, without full history).
func (db *DB) GetTurnUserMessage(conversationID, anchorMessageID string) (string, error) {
	conversationID = strings.TrimSpace(conversationID)
	anchorMessageID = strings.TrimSpace(anchorMessageID)
	if conversationID == "" || anchorMessageID == "" {
		return "", nil
	}
	var content string
	err := db.QueryRow(`
SELECT m.content FROM messages m
WHERE m.conversation_id = ? AND m.role = 'user'
  AND m.created_at <= COALESCE((SELECT created_at FROM messages WHERE id = ? AND conversation_id = ?), m.created_at)
ORDER BY m.created_at DESC, m.rowid DESC
LIMIT 1`, conversationID, anchorMessageID, conversationID).Scan(&content)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("query turn user message: %w", err)
	}
	return content, nil
}

// AssistantCognitionTexts holds the thinking/reasoning/planning text on a single assistant message.
type AssistantCognitionTexts struct {
	Thinking       string
	ReasoningChain string
	Planning       string
}

// GetAssistantCognitionTexts aggregates thinking / reasoning_chain / planning for an assistant message from process_details.
func (db *DB) GetAssistantCognitionTexts(assistantMessageID string) (AssistantCognitionTexts, error) {
	assistantMessageID = strings.TrimSpace(assistantMessageID)
	if assistantMessageID == "" {
		return AssistantCognitionTexts{}, nil
	}
	rows, err := db.Query(`
SELECT event_type, message FROM process_details
WHERE message_id = ? AND event_type IN ('thinking', 'reasoning_chain', 'planning')
ORDER BY created_at ASC, rowid ASC`, assistantMessageID)
	if err != nil {
		return AssistantCognitionTexts{}, fmt.Errorf("query assistant cognition: %w", err)
	}
	defer rows.Close()

	var thinkingParts, reasoningParts, planningParts []string
	for rows.Next() {
		var eventType, message string
		if err := rows.Scan(&eventType, &message); err != nil {
			continue
		}
		msg := strings.TrimSpace(message)
		if msg == "" {
			continue
		}
		switch eventType {
		case "thinking":
			thinkingParts = append(thinkingParts, msg)
		case "reasoning_chain":
			reasoningParts = append(reasoningParts, msg)
		case "planning":
			planningParts = append(planningParts, msg)
		}
	}
	return AssistantCognitionTexts{
		Thinking:       strings.Join(thinkingParts, "\n\n"),
		ReasoningChain: strings.Join(reasoningParts, "\n\n"),
		Planning:       strings.Join(planningParts, "\n\n"),
	}, nil
}

// AddProcessDetail adds a process details event
func (db *DB) AddProcessDetail(messageID, conversationID, eventType, message string, data interface{}) error {
	_, err := db.AddProcessDetailWithID(messageID, conversationID, eventType, message, data)
	return err
}

// AddProcessDetailWithID adds a process details event and returns the record ID.
func (db *DB) AddProcessDetailWithID(messageID, conversationID, eventType, message string, data interface{}) (string, error) {
	id := uuid.New().String()

	var dataJSON string
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			db.logger.Warn("serialize process details data failed", zap.Error(err))
		} else {
			dataJSON = string(jsonData)
		}
	}

	_, err := db.Exec(
		"INSERT INTO process_details (id, message_id, conversation_id, event_type, message, data, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, messageID, conversationID, eventType, message, dataJSON, time.Now(),
	)
	if err != nil {
		return "", fmt.Errorf("add process details failed: %w", err)
	}

	db.maybeRecordModelTokenUsage(messageID, conversationID, id, eventType, data)

	return id, nil
}

// UpdateProcessDetailContent updates the body and metadata of a streaming aggregated details record. Uses a fixed record ID
// to avoid adding a new row per token, while allowing the page to read in-progress planning output.
func (db *DB) UpdateProcessDetailContent(id, message string, data interface{}) error {
	var dataJSON string
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("serialize process details data failed: %w", err)
		}
		dataJSON = string(jsonData)
	}
	result, err := db.Exec(
		"UPDATE process_details SET message = ?, data = ? WHERE id = ?",
		message, dataJSON, strings.TrimSpace(id),
	)
	if err != nil {
		return fmt.Errorf("update process details failed: %w", err)
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr == nil && affected == 0 {
		return fmt.Errorf("process details not found: %s", id)
	}
	return nil
}

// DeleteProcessDetail deletes a temporary planning record that was determined to be a tool result echo.
func (db *DB) DeleteProcessDetail(id string) error {
	_, err := db.Exec("DELETE FROM process_details WHERE id = ?", strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("delete process details failed: %w", err)
	}
	return nil
}

// GetProcessDetails returns process details for a message
func (db *DB) GetProcessDetails(messageID string) ([]ProcessDetail, error) {
	rows, err := db.Query(
		"SELECT id, message_id, conversation_id, event_type, message, data, created_at FROM process_details WHERE message_id = ? ORDER BY created_at ASC, rowid ASC",
		messageID,
	)
	if err != nil {
		return nil, fmt.Errorf("query process details failed: %w", err)
	}
	defer rows.Close()

	var details []ProcessDetail
	for rows.Next() {
		var detail ProcessDetail
		var createdAt string

		if err := rows.Scan(&detail.ID, &detail.MessageID, &detail.ConversationID, &detail.EventType, &detail.Message, &detail.Data, &createdAt); err != nil {
			return nil, fmt.Errorf("scan process details failed: %w", err)
		}

		// try multiple time format parsers
		var err error
		detail.CreatedAt, err = time.Parse("2006-01-02 15:04:05.999999999-07:00", createdAt)
		if err != nil {
			detail.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt)
		}
		if err != nil {
			detail.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		}

		details = append(details, detail)
	}

	return details, nil
}

// GetProcessDetailByID returns a single process detail record.
func (db *DB) GetProcessDetailByID(id string) (*ProcessDetail, error) {
	var detail ProcessDetail
	var createdAt string
	err := db.QueryRow(
		"SELECT id, message_id, conversation_id, event_type, message, data, created_at FROM process_details WHERE id = ?",
		id,
	).Scan(&detail.ID, &detail.MessageID, &detail.ConversationID, &detail.EventType, &detail.Message, &detail.Data, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("query process details failed: %w", err)
	}

	var parseErr error
	detail.CreatedAt, parseErr = time.Parse("2006-01-02 15:04:05.999999999-07:00", createdAt)
	if parseErr != nil {
		detail.CreatedAt, parseErr = time.Parse("2006-01-02 15:04:05", createdAt)
	}
	if parseErr != nil {
		detail.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	}
	return &detail, nil
}

// ProcessDetailsSummary is a process details summary (used for collapsed display, avoiding full load).
type ProcessDetailsSummary struct {
	Total           int                           `json:"total"`
	IterationCount  int                           `json:"iterationCount"`
	MaxIteration    int                           `json:"maxIteration"`
	ToolCount       int                           `json:"toolCount"`
	ToolExecutions  []ProcessDetailsToolExecution `json:"toolExecutions,omitempty"`
	MCPExecutionIDs []string                      `json:"mcpExecutionIds,omitempty"`
	StartedAt       *time.Time                    `json:"startedAt,omitempty"`
	CompletedAt     *time.Time                    `json:"completedAt,omitempty"`
	DurationMs      int64                         `json:"durationMs"`
	Status          string                        `json:"status,omitempty"`
}

type ProcessDetailsToolExecution struct {
	ProcessDetailID string `json:"processDetailId,omitempty"`
	ResultDetailID  string `json:"resultDetailId,omitempty"`
	ToolName        string `json:"toolName,omitempty"`
	ToolCallID      string `json:"toolCallId,omitempty"`
	ExecutionID     string `json:"executionId,omitempty"`
	Status          string `json:"status,omitempty"`
}

// GetProcessDetailsSummary counts the process details count and iteration rounds for a message.
func (db *DB) GetProcessDetailsSummary(messageID string) (*ProcessDetailsSummary, error) {
	var total int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM process_details WHERE message_id = ?",
		messageID,
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("count process details failed: %w", err)
	}

	summary := &ProcessDetailsSummary{Total: total}
	var messageCreatedAt, messageUpdatedAt sql.NullString
	var messageContent string
	if err := db.QueryRow(
		"SELECT created_at, updated_at, content FROM messages WHERE id = ?",
		messageID,
	).Scan(&messageCreatedAt, &messageUpdatedAt, &messageContent); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("query process details elapsed time failed: %w", err)
	}
	if messageCreatedAt.Valid {
		if startedAt := parseDBTime(messageCreatedAt.String); !startedAt.IsZero() {
			summary.StartedAt = &startedAt
		}
	}
	var terminalEvent, terminalCreatedAt string
	terminalErr := db.QueryRow(`
SELECT event_type, created_at
FROM process_details
WHERE message_id = ? AND event_type IN ('cancelled', 'timeout', 'error')
ORDER BY created_at DESC, rowid DESC
LIMIT 1`, messageID).Scan(&terminalEvent, &terminalCreatedAt)
	if terminalErr != nil && !errors.Is(terminalErr, sql.ErrNoRows) {
		return nil, fmt.Errorf("query process details final status failed: %w", terminalErr)
	}
	if terminalEvent != "" {
		switch terminalEvent {
		case "cancelled":
			summary.Status = "cancelled"
		case "timeout":
			summary.Status = "timeout"
		default:
			summary.Status = "failed"
		}
		if completedAt := parseDBTime(terminalCreatedAt); !completedAt.IsZero() {
			summary.CompletedAt = &completedAt
		}
	} else if strings.TrimSpace(messageContent) == "processing..." || strings.TrimSpace(messageContent) == "Processing..." {
		summary.Status = "running"
	} else {
		summary.Status = "completed"
		if messageUpdatedAt.Valid {
			if completedAt := parseDBTime(messageUpdatedAt.String); !completedAt.IsZero() {
				summary.CompletedAt = &completedAt
			}
		}
	}
	if summary.StartedAt != nil && summary.CompletedAt != nil && !summary.CompletedAt.Before(*summary.StartedAt) {
		summary.DurationMs = summary.CompletedAt.Sub(*summary.StartedAt).Milliseconds()
	}
	if total == 0 {
		return summary, nil
	}

	if err := db.QueryRow(
		"SELECT COUNT(*) FROM process_details WHERE message_id = ? AND event_type = 'tool_call'",
		messageID,
	).Scan(&summary.ToolCount); err != nil {
		return nil, fmt.Errorf("count tool call details failed: %w", err)
	}

	pendingToolStatus := "result_missing"
	if summary.Status == "running" {
		pendingToolStatus = "running"
	}

	execRows, err := db.Query(
		"SELECT id, event_type, data FROM process_details WHERE message_id = ? AND event_type IN ('tool_call', 'tool_result') ORDER BY created_at ASC, rowid ASC",
		messageID,
	)
	if err != nil {
		return nil, fmt.Errorf("query tool execution summary failed: %w", err)
	}
	seenExecIDs := make(map[string]bool)
	// A provider may reuse a fallback toolCallId across streaming rounds. Keep a
	// FIFO per ID instead of a single index so every persisted call gets at most
	// one result. ID-less results still attach to an unmatched call with the same
	// tool name (parallel nmap 1/2, 2/2 often lose one ID); different tools stay
	// unlinked so a leftover preview cannot steal another call's slot.
	toolIndexesByCallID := make(map[string][]int)
	lastMatchedToolIndexByCallID := make(map[string]int)
	matchedToolIndexes := make([]bool, 0)
	for execRows.Next() {
		var detailID string
		var eventType string
		var dataJSON string
		if err := execRows.Scan(&detailID, &eventType, &dataJSON); err != nil {
			execRows.Close()
			return nil, fmt.Errorf("scantool executionsummaryfailed: %w", err)
		}
		if dataJSON == "" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(dataJSON), &payload); err != nil {
			continue
		}
		toolName := processDetailString(payload, "toolName")
		toolCallID := processDetailString(payload, "toolCallId")
		execID := processDetailString(payload, "executionId")
		status := toolResultStatusFromPayload(payload, eventType)
		if eventType == "tool_call" {
			summary.ToolExecutions = append(summary.ToolExecutions, ProcessDetailsToolExecution{
				ProcessDetailID: strings.TrimSpace(detailID),
				ToolName:        toolName,
				ToolCallID:      toolCallID,
				// This summary is reconstructed from persisted history. For an
				// active assistant turn, a missing result means the call is still
				// pending; after the turn is terminal it is genuinely incomplete.
				Status: pendingToolStatus,
			})
			matchedToolIndexes = append(matchedToolIndexes, false)
			if toolCallID != "" {
				toolIndexesByCallID[toolCallID] = append(toolIndexesByCallID[toolCallID], len(summary.ToolExecutions)-1)
			}
		}
		if eventType == "tool_result" {
			idx := matchToolExecutionIndex(
				summary.ToolExecutions,
				matchedToolIndexes,
				toolCallID,
				toolName,
				toolIndexesByCallID,
				lastMatchedToolIndexByCallID,
			)
			if idx >= 0 && idx < len(summary.ToolExecutions) {
				matchedToolIndexes[idx] = true
				if toolCallID != "" {
					lastMatchedToolIndexByCallID[toolCallID] = idx
				}
				summary.ToolExecutions[idx].ResultDetailID = strings.TrimSpace(detailID)
				if summary.ToolExecutions[idx].ToolName == "" {
					summary.ToolExecutions[idx].ToolName = toolName
				}
				if summary.ToolExecutions[idx].ToolCallID == "" {
					summary.ToolExecutions[idx].ToolCallID = toolCallID
				}
				summary.ToolExecutions[idx].ExecutionID = execID
				if status != "" {
					summary.ToolExecutions[idx].Status = status
				} else {
					summary.ToolExecutions[idx].Status = "completed"
				}
			} else {
				summary.ToolExecutions = append(summary.ToolExecutions, ProcessDetailsToolExecution{
					ProcessDetailID: strings.TrimSpace(detailID),
					ToolName:        toolName,
					ToolCallID:      toolCallID,
					ExecutionID:     execID,
					Status:          status,
				})
				matchedToolIndexes = append(matchedToolIndexes, true)
			}
		}
		if execID != "" && !seenExecIDs[execID] {
			seenExecIDs[execID] = true
			summary.MCPExecutionIDs = append(summary.MCPExecutionIDs, execID)
		}
	}
	if err := execRows.Err(); err != nil {
		execRows.Close()
		return nil, fmt.Errorf("iterate tool execution summary failed: %w", err)
	}
	execRows.Close()
	db.applyPersistedToolExecutionStatuses(summary.ToolExecutions)

	rows, err := db.Query(
		"SELECT data FROM process_details WHERE message_id = ? AND event_type = 'iteration' ORDER BY created_at ASC, rowid ASC",
		messageID,
	)
	if err != nil {
		return nil, fmt.Errorf("query iteration details failed: %w", err)
	}
	defer rows.Close()

	maxIter := 0
	iterCount := 0
	for rows.Next() {
		var dataJSON string
		if err := rows.Scan(&dataJSON); err != nil {
			return nil, fmt.Errorf("scan iteration details failed: %w", err)
		}
		iterCount++
		if dataJSON == "" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(dataJSON), &payload); err != nil {
			continue
		}
		if n, ok := payload["iteration"].(float64); ok && int(n) > maxIter {
			maxIter = int(n)
		}
	}
	summary.IterationCount = iterCount
	summary.MaxIteration = maxIter
	return summary, nil
}

func processDetailString(payload map[string]interface{}, key string) string {
	if payload == nil {
		return ""
	}
	v, ok := payload[key]
	if !ok || v == nil {
		return ""
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" || s == "<nil>" {
		return ""
	}
	return s
}

func toolResultStatusFromPayload(payload map[string]interface{}, eventType string) string {
	if eventType != "tool_result" {
		return ""
	}
	if blocked, _ := payload["blocked"].(bool); blocked || strings.EqualFold(processDetailString(payload, "status"), "blocked") {
		return "blocked"
	}
	if status := processDetailString(payload, "status"); strings.EqualFold(status, "background_running") {
		return "background_running"
	}
	if success, ok := payload["success"].(bool); ok {
		if success {
			return "completed"
		}
		return "failed"
	}
	if isErr, ok := payload["isError"].(bool); ok && isErr {
		return "failed"
	}
	return "completed"
}

func (db *DB) applyPersistedToolExecutionStatuses(executions []ProcessDetailsToolExecution) {
	for i := range executions {
		execID := strings.TrimSpace(executions[i].ExecutionID)
		if execID == "" {
			continue
		}
		var status string
		if err := db.QueryRow(`SELECT status FROM tool_executions WHERE id = ?`, execID).Scan(&status); err != nil {
			continue
		}
		status = strings.ToLower(strings.TrimSpace(status))
		if status == "" {
			continue
		}
		executions[i].Status = status
	}
}

func matchToolExecutionIndex(
	executions []ProcessDetailsToolExecution,
	matched []bool,
	toolCallID, toolName string,
	toolIndexesByCallID map[string][]int,
	lastMatchedToolIndexByCallID map[string]int,
) int {
	if toolCallID != "" {
		queue := toolIndexesByCallID[toolCallID]
		for len(queue) > 0 {
			candidate := queue[0]
			queue = queue[1:]
			if candidate >= 0 && candidate < len(matched) && !matched[candidate] {
				toolIndexesByCallID[toolCallID] = queue
				return candidate
			}
		}
		toolIndexesByCallID[toolCallID] = queue
		if previous, ok := lastMatchedToolIndexByCallID[toolCallID]; ok {
			return previous
		}
	}
	if toolName != "" {
		for i := range matched {
			if matched[i] {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(executions[i].ToolName), toolName) {
				return i
			}
		}
	}
	if toolCallID != "" {
		for i := range matched {
			if !matched[i] {
				return i
			}
		}
	}
	return -1
}

// GetProcessDetailsPage returns paginated process details for a message (ascending time order).
func (db *DB) GetProcessDetailsPage(messageID string, limit, offset int) ([]ProcessDetail, int, error) {
	var total int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM process_details WHERE message_id = ?",
		messageID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count process details failed: %w", err)
	}
	if total == 0 || offset >= total {
		return nil, total, nil
	}

	rows, err := db.Query(
		"SELECT id, message_id, conversation_id, event_type, message, data, created_at FROM process_details WHERE message_id = ? ORDER BY created_at ASC, rowid ASC LIMIT ? OFFSET ?",
		messageID, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query process details failed: %w", err)
	}
	defer rows.Close()

	var details []ProcessDetail
	for rows.Next() {
		var detail ProcessDetail
		var createdAt string

		if err := rows.Scan(&detail.ID, &detail.MessageID, &detail.ConversationID, &detail.EventType, &detail.Message, &detail.Data, &createdAt); err != nil {
			return nil, 0, fmt.Errorf("scan process details failed: %w", err)
		}

		var parseErr error
		detail.CreatedAt, parseErr = time.Parse("2006-01-02 15:04:05.999999999-07:00", createdAt)
		if parseErr != nil {
			detail.CreatedAt, parseErr = time.Parse("2006-01-02 15:04:05", createdAt)
		}
		if parseErr != nil {
			detail.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		}

		details = append(details, detail)
	}

	return details, total, nil
}

// GetProcessDetailOffset returns the zero-based offset of a process detail within its message's details stream.
func (db *DB) GetProcessDetailOffset(messageID, detailID string) (int, error) {
	messageID = strings.TrimSpace(messageID)
	detailID = strings.TrimSpace(detailID)
	if messageID == "" || detailID == "" {
		return 0, fmt.Errorf("messageID and detailID are required")
	}
	var createdAt string
	var rowID int64
	if err := db.QueryRow(
		"SELECT created_at, rowid FROM process_details WHERE message_id = ? AND id = ?",
		messageID, detailID,
	).Scan(&createdAt, &rowID); err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("process details not found")
		}
		return 0, fmt.Errorf("query process details anchor failed: %w", err)
	}
	var offset int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM process_details
		 WHERE message_id = ?
		   AND (created_at < ? OR (created_at = ? AND rowid < ?))`,
		messageID, createdAt, createdAt, rowID,
	).Scan(&offset); err != nil {
		return 0, fmt.Errorf("calculate process details anchor position failed: %w", err)
	}
	return offset, nil
}

// GetProcessDetailsByConversation returns all process details for a conversation (grouped by message)
func (db *DB) GetProcessDetailsByConversation(conversationID string) (map[string][]ProcessDetail, error) {
	rows, err := db.Query(
		"SELECT id, message_id, conversation_id, event_type, message, data, created_at FROM process_details WHERE conversation_id = ? ORDER BY created_at ASC, rowid ASC",
		conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("query process details failed: %w", err)
	}
	defer rows.Close()

	detailsMap := make(map[string][]ProcessDetail)
	for rows.Next() {
		var detail ProcessDetail
		var createdAt string

		if err := rows.Scan(&detail.ID, &detail.MessageID, &detail.ConversationID, &detail.EventType, &detail.Message, &detail.Data, &createdAt); err != nil {
			return nil, fmt.Errorf("scan process details failed: %w", err)
		}

		// try multiple time format parsers
		var err error
		detail.CreatedAt, err = time.Parse("2006-01-02 15:04:05.999999999-07:00", createdAt)
		if err != nil {
			detail.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt)
		}
		if err != nil {
			detail.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		}

		detailsMap[detail.MessageID] = append(detailsMap[detail.MessageID], detail)
	}

	return detailsMap, nil
}
