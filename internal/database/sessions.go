package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AgentSession is a single agent run session.
type AgentSession struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id,omitempty"`
	UserID    string     `json:"user_id"`
	RoleID    string     `json:"role_id,omitempty"`
	Title     string     `json:"title"`
	AgentMode string     `json:"agent_mode"`
	Status    string     `json:"status"` // active | completed | failed | cancelled
	HITLMode  string     `json:"hitl_mode"`
	Pinned    bool       `json:"pinned"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	// Aggregated fields (populated on list).
	MessageCount   int `json:"message_count"`
	ToolExecCount  int `json:"tool_exec_count"`
}

// ToolExecution is a single tool call record.
type ToolExecution struct {
	ID             string     `json:"id"`
	SessionID      string     `json:"session_id"`
	MessageID      string     `json:"message_id,omitempty"`
	UserID         string     `json:"user_id"`
	ToolName       string     `json:"tool_name"`
	ArgumentsJSON  string     `json:"arguments_json"`
	Status         string     `json:"status"` // pending | running | completed | failed | cancelled
	Result         string     `json:"result,omitempty"`
	Error          string     `json:"error,omitempty"`
	OutputBytes    int        `json:"output_bytes"`
	OutputTruncated bool      `json:"output_truncated"`
	HITLRequired   bool       `json:"hitl_required"`
	HITLApproved   *bool      `json:"hitl_approved,omitempty"`
	StartedAt      time.Time  `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	DurationMs     int64      `json:"duration_ms"`
}

// ── AgentSession CRUD ──────────────────────────────────────────────────────────

// CreateAgentSession inserts a new agent session record.
func (db *DB) CreateAgentSession(s *AgentSession) (*AgentSession, error) {
	now := time.Now().UTC()
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	s.CreatedAt = now
	s.UpdatedAt = now
	if s.Status == "" {
		s.Status = "active"
	}
	if s.AgentMode == "" {
		s.AgentMode = "single"
	}
	if s.HITLMode == "" {
		s.HITLMode = "auto"
	}
	if s.Title == "" {
		s.Title = "Agent Session"
	}

	_, err := db.Exec(`
		INSERT INTO agent_sessions
		  (id,project_id,user_id,role_id,title,agent_mode,status,hitl_mode,pinned,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		s.ID, nullStr(s.ProjectID), s.UserID, s.RoleID, s.Title,
		s.AgentMode, s.Status, s.HITLMode, boolToInt(s.Pinned), now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("creating agent session: %w", err)
	}
	return s, nil
}

// GetAgentSessionByID retrieves a session by ID with message/tool counts.
func (db *DB) GetAgentSessionByID(id string) (*AgentSession, error) {
	s := &AgentSession{}
	var pinned int
	err := db.QueryRow(`
		SELECT id,COALESCE(project_id,''),user_id,role_id,title,agent_mode,status,hitl_mode,pinned,created_at,updated_at
		FROM agent_sessions WHERE id=?`, id,
	).Scan(&s.ID, &s.ProjectID, &s.UserID, &s.RoleID, &s.Title,
		&s.AgentMode, &s.Status, &s.HITLMode, &pinned, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Pinned = pinned == 1
	_ = db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id=?`, id).Scan(&s.MessageCount)
	_ = db.QueryRow(`SELECT COUNT(*) FROM tool_executions WHERE session_id=?`, id).Scan(&s.ToolExecCount)
	return s, nil
}

// ListAgentSessionsParams holds filter/pagination parameters.
type ListAgentSessionsParams struct {
	UserID    string
	ProjectID string
	Status    string
	Limit     int
	Offset    int
}

// ListAgentSessions returns sessions matching the given filters, newest-first.
func (db *DB) ListAgentSessions(p ListAgentSessionsParams) ([]*AgentSession, int, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}

	where := "WHERE 1=1"
	args := []interface{}{}
	if p.UserID != "" {
		where += " AND user_id=?"
		args = append(args, p.UserID)
	}
	if p.ProjectID != "" {
		where += " AND project_id=?"
		args = append(args, p.ProjectID)
	}
	if p.Status != "" {
		where += " AND status=?"
		args = append(args, p.Status)
	}

	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	_ = db.QueryRow("SELECT COUNT(*) FROM agent_sessions "+where, countArgs...).Scan(&total)

	args = append(args, p.Limit, p.Offset)
	rows, err := db.Query(`
		SELECT id,COALESCE(project_id,''),user_id,role_id,title,agent_mode,status,hitl_mode,pinned,created_at,updated_at
		FROM agent_sessions `+where+` ORDER BY pinned DESC, created_at DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var sessions []*AgentSession
	for rows.Next() {
		s := &AgentSession{}
		var pinned int
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.UserID, &s.RoleID, &s.Title,
			&s.AgentMode, &s.Status, &s.HITLMode, &pinned, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, 0, err
		}
		s.Pinned = pinned == 1
		sessions = append(sessions, s)
	}

	// Batch-fetch counts.
	for _, s := range sessions {
		_ = db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id=?`, s.ID).Scan(&s.MessageCount)
		_ = db.QueryRow(`SELECT COUNT(*) FROM tool_executions WHERE session_id=?`, s.ID).Scan(&s.ToolExecCount)
	}

	return sessions, total, rows.Err()
}

// UpdateAgentSession updates mutable session fields.
func (db *DB) UpdateAgentSession(id, title, status string, pinned *bool) error {
	now := time.Now().UTC()
	cur, err := db.GetAgentSessionByID(id)
	if err != nil || cur == nil {
		return fmt.Errorf("session not found")
	}
	newTitle := cur.Title
	if title != "" {
		newTitle = title
	}
	newStatus := cur.Status
	if status != "" {
		newStatus = status
	}
	newPinned := cur.Pinned
	if pinned != nil {
		newPinned = *pinned
	}
	_, err = db.Exec(`UPDATE agent_sessions SET title=?,status=?,pinned=?,updated_at=? WHERE id=?`,
		newTitle, newStatus, boolToInt(newPinned), now, id)
	return err
}

// DeleteAgentSession removes a session and all its messages.
func (db *DB) DeleteAgentSession(id string) error {
	_, err := db.Exec(`DELETE FROM agent_sessions WHERE id=?`, id)
	return err
}

// ── ToolExecution CRUD ─────────────────────────────────────────────────────────

// ListToolExecutionsParams holds filter/pagination for tool execution queries.
type ListToolExecutionsParams struct {
	SessionID string
	UserID    string
	ToolName  string
	Status    string
	Limit     int
	Offset    int
}

// ListToolExecutions returns tool execution records matching the given filters.
func (db *DB) ListToolExecutions(p ListToolExecutionsParams) ([]*ToolExecution, int, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}

	where := "WHERE 1=1"
	args := []interface{}{}
	if p.SessionID != "" {
		where += " AND session_id=?"
		args = append(args, p.SessionID)
	}
	if p.UserID != "" {
		where += " AND user_id=?"
		args = append(args, p.UserID)
	}
	if p.ToolName != "" {
		where += " AND tool_name=?"
		args = append(args, p.ToolName)
	}
	if p.Status != "" {
		where += " AND status=?"
		args = append(args, p.Status)
	}

	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	_ = db.QueryRow("SELECT COUNT(*) FROM tool_executions "+where, countArgs...).Scan(&total)

	args = append(args, p.Limit, p.Offset)
	rows, err := db.Query(`
		SELECT id,session_id,COALESCE(message_id,''),COALESCE(user_id,''),tool_name,arguments_json,
		       status,COALESCE(result,''),COALESCE(error,''),output_bytes,output_truncated,
		       hitl_required,hitl_approved,started_at,completed_at,duration_ms
		FROM tool_executions `+where+` ORDER BY started_at DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var execs []*ToolExecution
	for rows.Next() {
		e := &ToolExecution{}
		var outputTruncated, hitlRequired int
		var hitlApproved sql.NullInt64
		if err := rows.Scan(
			&e.ID, &e.SessionID, &e.MessageID, &e.UserID, &e.ToolName, &e.ArgumentsJSON,
			&e.Status, &e.Result, &e.Error, &e.OutputBytes, &outputTruncated,
			&hitlRequired, &hitlApproved, &e.StartedAt, &e.CompletedAt, &e.DurationMs,
		); err != nil {
			return nil, 0, err
		}
		e.OutputTruncated = outputTruncated == 1
		e.HITLRequired = hitlRequired == 1
		if hitlApproved.Valid {
			v := hitlApproved.Int64 == 1
			e.HITLApproved = &v
		}
		execs = append(execs, e)
	}
	return execs, total, rows.Err()
}
