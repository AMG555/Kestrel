package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// RecordTokenUsage writes a token usage record after an LLM call.
// sessionID and userID may be empty strings if the call happened outside a session context.
func (db *DB) RecordTokenUsage(sessionID, userID, provider string, promptTokens, completionTokens int) error {
	total := promptTokens + completionTokens
	_, err := db.Exec(`
		INSERT INTO token_usage (id,session_id,user_id,provider,model,prompt_tokens,completion_tokens,total_tokens,created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		uuid.New().String(),
		sessionID, userID, provider, "", // model left blank; can be enriched later
		promptTokens, completionTokens, total,
		time.Now().UTC(),
	)
	return err
}

// TokenUsageSummary returns aggregate token usage stats.
func (db *DB) TokenUsageSummary() (map[string]interface{}, error) {
	var totalCalls int
	var totalPrompt, totalCompletion int
	_ = db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0) FROM token_usage`).
		Scan(&totalCalls, &totalPrompt, &totalCompletion)
	return map[string]interface{}{
		"total_calls":        totalCalls,
		"total_prompt":       totalPrompt,
		"total_completion":   totalCompletion,
		"total_tokens":       totalPrompt + totalCompletion,
	}, nil
}



// BatchTaskQueue is a named queue of agent tasks to be executed sequentially.
type BatchTaskQueue struct {
	ID           string     `json:"id"`
	ProjectID    string     `json:"project_id,omitempty"`
	Title        string     `json:"title"`
	RoleID       string     `json:"role_id"`
	AgentMode    string     `json:"agent_mode"`
	HITLMode     string     `json:"hitl_mode"`
	Concurrency  int        `json:"concurrency"`
	Status       string     `json:"status"` // pending | running | completed | failed | cancelled
	CurrentIndex int        `json:"current_index"`
	CreatedBy    string     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// BatchTask is a single intent within a queue.
type BatchTask struct {
	ID          string     `json:"id"`
	QueueID     string     `json:"queue_id"`
	Intent      string     `json:"intent"`
	SessionID   string     `json:"session_id,omitempty"`
	Status      string     `json:"status"` // pending | running | completed | failed | skipped
	Result      string     `json:"result,omitempty"`
	Error       string     `json:"error,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// CreateBatchQueue creates a new task queue.
func (db *DB) CreateBatchQueue(q *BatchTaskQueue) (*BatchTaskQueue, error) {
	now := time.Now().UTC()
	q.ID = uuid.New().String()
	q.CreatedAt = now
	q.Status = "pending"
	_, err := db.Exec(`
		INSERT INTO batch_task_queues
		  (id,project_id,title,role_id,agent_mode,hitl_mode,concurrency,status,current_index,created_by,created_at)
		VALUES (?,?,?,?,?,?,?,?,0,?,?)`,
		q.ID, nullStr(q.ProjectID), q.Title, q.RoleID, q.AgentMode, q.HITLMode,
		q.Concurrency, q.Status, q.CreatedBy, now,
	)
	if err != nil {
		return nil, fmt.Errorf("creating batch queue: %w", err)
	}
	return q, nil
}

// AddBatchTask appends a task to a queue.
func (db *DB) AddBatchTask(queueID, intent string) (*BatchTask, error) {
	t := &BatchTask{
		ID:      uuid.New().String(),
		QueueID: queueID,
		Intent:  intent,
		Status:  "pending",
	}
	_, err := db.Exec(`
		INSERT INTO batch_tasks (id,queue_id,intent,status) VALUES (?,?,?,?)`,
		t.ID, t.QueueID, t.Intent, t.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("adding batch task: %w", err)
	}
	return t, nil
}

// GetBatchQueue retrieves a queue by ID.
func (db *DB) GetBatchQueue(id string) (*BatchTaskQueue, error) {
	q := &BatchTaskQueue{}
	err := db.QueryRow(`
		SELECT id,COALESCE(project_id,''),title,role_id,agent_mode,hitl_mode,concurrency,status,current_index,created_by,created_at,started_at,completed_at
		FROM batch_task_queues WHERE id=?`, id,
	).Scan(
		&q.ID, &q.ProjectID, &q.Title, &q.RoleID, &q.AgentMode, &q.HITLMode,
		&q.Concurrency, &q.Status, &q.CurrentIndex, &q.CreatedBy, &q.CreatedAt, &q.StartedAt, &q.CompletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return q, err
}

// ListBatchQueues returns all queues newest-first.
func (db *DB) ListBatchQueues(projectID string, limit int) ([]*BatchTaskQueue, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows *sql.Rows
	var err error
	if projectID != "" {
		rows, err = db.Query(`
			SELECT id,COALESCE(project_id,''),title,role_id,agent_mode,hitl_mode,concurrency,status,current_index,created_by,created_at,started_at,completed_at
			FROM batch_task_queues WHERE project_id=? ORDER BY created_at DESC LIMIT ?`, projectID, limit,
		)
	} else {
		rows, err = db.Query(`
			SELECT id,COALESCE(project_id,''),title,role_id,agent_mode,hitl_mode,concurrency,status,current_index,created_by,created_at,started_at,completed_at
			FROM batch_task_queues ORDER BY created_at DESC LIMIT ?`, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var qs []*BatchTaskQueue
	for rows.Next() {
		q := &BatchTaskQueue{}
		if err := rows.Scan(
			&q.ID, &q.ProjectID, &q.Title, &q.RoleID, &q.AgentMode, &q.HITLMode,
			&q.Concurrency, &q.Status, &q.CurrentIndex, &q.CreatedBy, &q.CreatedAt, &q.StartedAt, &q.CompletedAt,
		); err != nil {
			return nil, err
		}
		qs = append(qs, q)
	}
	return qs, rows.Err()
}

// GetBatchTasks returns all tasks for a queue.
func (db *DB) GetBatchTasks(queueID string) ([]*BatchTask, error) {
	rows, err := db.Query(`
		SELECT id,queue_id,intent,COALESCE(session_id,''),status,COALESCE(result,''),error,started_at,completed_at
		FROM batch_tasks WHERE queue_id=? ORDER BY rowid`, queueID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*BatchTask
	for rows.Next() {
		t := &BatchTask{}
		if err := rows.Scan(&t.ID, &t.QueueID, &t.Intent, &t.SessionID, &t.Status, &t.Result, &t.Error, &t.StartedAt, &t.CompletedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// UpdateBatchTaskStatus updates the status and optional result of a task.
func (db *DB) UpdateBatchTaskStatus(taskID, status, result, errMsg, sessionID string) error {
	now := time.Now().UTC()
	switch status {
	case "running":
		_, err := db.Exec(`UPDATE batch_tasks SET status=?,session_id=?,started_at=? WHERE id=?`, status, nullStr(sessionID), now, taskID)
		return err
	case "completed", "failed", "skipped":
		_, err := db.Exec(`UPDATE batch_tasks SET status=?,result=?,error=?,completed_at=? WHERE id=?`, status, result, errMsg, now, taskID)
		return err
	default:
		_, err := db.Exec(`UPDATE batch_tasks SET status=? WHERE id=?`, status, taskID)
		return err
	}
}

// UpdateBatchQueueStatus updates queue status.
func (db *DB) UpdateBatchQueueStatus(queueID, status string, currentIndex int) error {
	now := time.Now().UTC()
	switch status {
	case "running":
		_, err := db.Exec(`UPDATE batch_task_queues SET status=?,current_index=?,started_at=? WHERE id=?`, status, currentIndex, now, queueID)
		return err
	case "completed", "failed", "cancelled":
		_, err := db.Exec(`UPDATE batch_task_queues SET status=?,current_index=?,completed_at=? WHERE id=?`, status, currentIndex, now, queueID)
		return err
	default:
		_, err := db.Exec(`UPDATE batch_task_queues SET status=?,current_index=? WHERE id=?`, status, currentIndex, queueID)
		return err
	}
}

// DeleteBatchQueue removes a queue and all its tasks.
func (db *DB) DeleteBatchQueue(id string) error {
	_, err := db.Exec(`DELETE FROM batch_task_queues WHERE id=?`, id)
	return err
}

// HITLPending is an approval request waiting for a human decision.
type HITLPending struct {
	ID             string     `json:"id"`
	SessionID      string     `json:"session_id"`
	UserID         string     `json:"user_id"`
	ToolName       string     `json:"tool_name"`
	Arguments      map[string]interface{} `json:"arguments"`
	ContextSummary string     `json:"context_summary"`
	Status         string     `json:"status"` // pending | approved | rejected | expired
	Decision       string     `json:"decision"`
	DecidedBy      string     `json:"decided_by"`
	DecidedAt      *time.Time `json:"decided_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
}

// CreateHITLPending creates a new approval request.
func (db *DB) CreateHITLPending(h *HITLPending) (*HITLPending, error) {
	h.ID = uuid.New().String()
	h.CreatedAt = time.Now().UTC()
	if h.ExpiresAt.IsZero() {
		h.ExpiresAt = h.CreatedAt.Add(5 * time.Minute)
	}
	argsJSON, _ := json.Marshal(h.Arguments)
	_, err := db.Exec(`
		INSERT INTO hitl_pending
		  (id,session_id,user_id,tool_name,arguments_json,context_summary,status,created_at,expires_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		h.ID, h.SessionID, h.UserID, h.ToolName, string(argsJSON), h.ContextSummary, "pending", h.CreatedAt, h.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating HITL pending: %w", err)
	}
	return h, nil
}

// DecideHITL records an approval or rejection decision.
func (db *DB) DecideHITL(id, decision, decidedBy string) error {
	now := time.Now().UTC()
	_, err := db.Exec(`
		UPDATE hitl_pending SET status=?,decision=?,decided_by=?,decided_at=? WHERE id=? AND status='pending'`,
		decision, decision, decidedBy, now, id,
	)
	return err
}

// GetHITLPending retrieves a single approval request.
func (db *DB) GetHITLPending(id string) (*HITLPending, error) {
	h := &HITLPending{}
	var argsJSON string
	err := db.QueryRow(`
		SELECT id,session_id,user_id,tool_name,arguments_json,context_summary,status,decision,decided_by,decided_at,created_at,expires_at
		FROM hitl_pending WHERE id=?`, id,
	).Scan(&h.ID, &h.SessionID, &h.UserID, &h.ToolName, &argsJSON, &h.ContextSummary, &h.Status, &h.Decision, &h.DecidedBy, &h.DecidedAt, &h.CreatedAt, &h.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(argsJSON), &h.Arguments)
	return h, nil
}

// ListPendingHITL returns all unresolved approval requests.
func (db *DB) ListPendingHITL() ([]*HITLPending, error) {
	rows, err := db.Query(`
		SELECT id,session_id,user_id,tool_name,arguments_json,context_summary,status,decision,decided_by,decided_at,created_at,expires_at
		FROM hitl_pending WHERE status='pending' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*HITLPending
	for rows.Next() {
		h := &HITLPending{}
		var argsJSON string
		if err := rows.Scan(&h.ID, &h.SessionID, &h.UserID, &h.ToolName, &argsJSON, &h.ContextSummary, &h.Status, &h.Decision, &h.DecidedBy, &h.DecidedAt, &h.CreatedAt, &h.ExpiresAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(argsJSON), &h.Arguments)
		items = append(items, h)
	}
	return items, rows.Err()
}

// ExpireOldHITL marks timed-out pending requests as expired.
func (db *DB) ExpireOldHITL() (int64, error) {
	res, err := db.Exec(`UPDATE hitl_pending SET status='expired' WHERE status='pending' AND expires_at < ?`, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Conversation is a named agent conversation thread.
type Conversation struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id,omitempty"`
	UserID    string    `json:"user_id"`
	Title     string    `json:"title"`
	AgentMode string    `json:"agent_mode"`
	RoleID    string    `json:"role_id"`
	Pinned    bool      `json:"pinned"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateConversation inserts a new conversation record.
func (db *DB) CreateConversation(c *Conversation) (*Conversation, error) {
	now := time.Now().UTC()
	c.ID = uuid.New().String()
	c.CreatedAt = now
	c.UpdatedAt = now
	if c.AgentMode == "" {
		c.AgentMode = "single"
	}
	_, err := db.Exec(`
		INSERT INTO conversations (id,project_id,user_id,title,agent_mode,role_id,pinned,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		c.ID, nullStr(c.ProjectID), c.UserID, c.Title, c.AgentMode, c.RoleID, boolToInt(c.Pinned), now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("creating conversation: %w", err)
	}
	return c, nil
}

// ListConversations returns conversations for a user, newest first.
func (db *DB) ListConversations(userID, projectID string, limit int) ([]*Conversation, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows *sql.Rows
	var err error
	if projectID != "" {
		rows, err = db.Query(`
			SELECT id,COALESCE(project_id,''),user_id,title,agent_mode,role_id,pinned,created_at,updated_at
			FROM conversations WHERE user_id=? AND project_id=?
			ORDER BY pinned DESC, updated_at DESC LIMIT ?`, userID, projectID, limit,
		)
	} else {
		rows, err = db.Query(`
			SELECT id,COALESCE(project_id,''),user_id,title,agent_mode,role_id,pinned,created_at,updated_at
			FROM conversations WHERE user_id=?
			ORDER BY pinned DESC, updated_at DESC LIMIT ?`, userID, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convs []*Conversation
	for rows.Next() {
		c := &Conversation{}
		var pinned int
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.UserID, &c.Title, &c.AgentMode, &c.RoleID, &pinned, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Pinned = pinned == 1
		convs = append(convs, c)
	}
	return convs, rows.Err()
}

// UpdateConversation updates title, pinned, and updated_at.
func (db *DB) UpdateConversation(id, title string, pinned bool) error {
	_, err := db.Exec(`UPDATE conversations SET title=?,pinned=?,updated_at=? WHERE id=?`,
		title, boolToInt(pinned), time.Now().UTC(), id)
	return err
}

// DeleteConversation removes a conversation and all messages.
func (db *DB) DeleteConversation(id string) error {
	_, err := db.Exec(`DELETE FROM conversations WHERE id=?`, id)
	return err
}

// MCPServer is a registered external MCP server.
type MCPServer struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Transport      string    `json:"transport"`
	URL            string    `json:"url"`
	Command        []string  `json:"command"`
	Headers        map[string]string `json:"headers,omitempty"`
	TimeoutSeconds int       `json:"timeout_seconds"`
	IsActive       bool      `json:"is_active"`
	AllowedTools   []string  `json:"allowed_tools"`
	CircuitOpen    bool      `json:"circuit_open"`
	FailureCount   int       `json:"failure_count"`
	LastFailureAt  *time.Time `json:"last_failure_at,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// UpsertMCPServer inserts or updates an MCP server registration.
func (db *DB) UpsertMCPServer(s *MCPServer) (*MCPServer, error) {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	cmdJSON, _ := json.Marshal(s.Command)
	headersJSON, _ := json.Marshal(s.Headers)
	toolsJSON, _ := json.Marshal(s.AllowedTools)
	_, err := db.Exec(`
		INSERT INTO mcp_servers (id,name,transport,url,command_json,headers_json,timeout_seconds,is_active,allowed_tools_json,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(name) DO UPDATE SET
		  transport=excluded.transport, url=excluded.url, command_json=excluded.command_json,
		  headers_json=excluded.headers_json, timeout_seconds=excluded.timeout_seconds,
		  is_active=excluded.is_active, allowed_tools_json=excluded.allowed_tools_json,
		  updated_at=excluded.updated_at`,
		s.ID, s.Name, s.Transport, s.URL, string(cmdJSON), string(headersJSON),
		s.TimeoutSeconds, boolToInt(s.IsActive), string(toolsJSON), now, now,
	)
	return s, err
}

// ListMCPServers returns all registered MCP servers.
func (db *DB) ListMCPServers() ([]*MCPServer, error) {
	rows, err := db.Query(`
		SELECT id,name,transport,url,command_json,headers_json,timeout_seconds,is_active,
		       allowed_tools_json,circuit_open,failure_count,last_failure_at,created_at,updated_at
		FROM mcp_servers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []*MCPServer
	for rows.Next() {
		s := &MCPServer{}
		var cmdJSON, headersJSON, toolsJSON string
		var isActive, circuitOpen int
		if err := rows.Scan(
			&s.ID, &s.Name, &s.Transport, &s.URL, &cmdJSON, &headersJSON,
			&s.TimeoutSeconds, &isActive, &toolsJSON, &circuitOpen, &s.FailureCount, &s.LastFailureAt,
			&s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		s.IsActive = isActive == 1
		s.CircuitOpen = circuitOpen == 1
		_ = json.Unmarshal([]byte(cmdJSON), &s.Command)
		_ = json.Unmarshal([]byte(headersJSON), &s.Headers)
		_ = json.Unmarshal([]byte(toolsJSON), &s.AllowedTools)
		servers = append(servers, s)
	}
	return servers, rows.Err()
}

// DeleteMCPServer removes a server registration.
func (db *DB) DeleteMCPServer(id string) error {
	_, err := db.Exec(`DELETE FROM mcp_servers WHERE id=?`, id)
	return err
}

// RecordMCPServerFailure increments failure count and opens the circuit after threshold.
func (db *DB) RecordMCPServerFailure(id string, threshold int) error {
	now := time.Now().UTC()
	_, err := db.Exec(`
		UPDATE mcp_servers SET failure_count=failure_count+1, last_failure_at=?,
		  circuit_open=CASE WHEN failure_count+1 >= ? THEN 1 ELSE circuit_open END
		WHERE id=?`, now, threshold, id,
	)
	return err
}

// ResetMCPServerCircuit resets circuit breaker and failure count.
func (db *DB) ResetMCPServerCircuit(id string) error {
	_, err := db.Exec(`UPDATE mcp_servers SET circuit_open=0, failure_count=0 WHERE id=?`, id)
	return err
}

// CancelOrphanedRunningToolExecutions marks in-flight executions as cancelled on restart.
func (db *DB) CancelOrphanedRunningToolExecutions(before time.Time, reason string) (int64, error) {
	res, err := db.Exec(`
		UPDATE tool_executions SET status='orphaned', error=?, completed_at=?
		WHERE status='running' AND started_at < ?`, reason, time.Now().UTC(), before,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
