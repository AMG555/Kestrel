package database

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AuditLog is an immutable audit record.
type AuditLog struct {
	ID           string          `json:"id"`
	CreatedAt    time.Time       `json:"created_at"`
	ActorID      string          `json:"actor_id"`
	ActorName    string          `json:"actor_name"`
	Action       string          `json:"action"`
	Category     string          `json:"category"`
	Result       string          `json:"result"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	ClientIP     string          `json:"client_ip"`
	UserAgent    string          `json:"user_agent"`
	Message      string          `json:"message"`
	Detail       json.RawMessage `json:"detail,omitempty"`
}

// AuditParams carries parameters for writing an audit record.
type AuditParams struct {
	ActorID      string
	ActorName    string
	Action       string
	Category     string
	Result       string // "success" | "failure" | "blocked"
	ResourceType string
	ResourceID   string
	ClientIP     string
	UserAgent    string
	Message      string
	Detail       interface{}
}

// WriteAuditLog appends an immutable audit record. It is the only write path to the audit table.
func (db *DB) WriteAuditLog(p AuditParams) error {
	var detailJSON string
	if p.Detail != nil {
		b, err := json.Marshal(p.Detail)
		if err == nil {
			detailJSON = string(b)
		}
	}
	if detailJSON == "" {
		detailJSON = "{}"
	}

	_, err := db.Exec(`
		INSERT INTO audit_logs
		  (id,created_at,actor_id,actor_name,action,category,result,resource_type,resource_id,client_ip,user_agent,message,detail_json)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		uuid.New().String(), time.Now().UTC(),
		p.ActorID, p.ActorName, p.Action, p.Category, p.Result,
		p.ResourceType, p.ResourceID,
		p.ClientIP, p.UserAgent,
		p.Message, detailJSON,
	)
	if err != nil {
		return fmt.Errorf("writing audit log: %w", err)
	}
	return nil
}

// ListAuditLogsParams filters audit log queries.
type ListAuditLogsParams struct {
	ActorID      string
	Category     string
	Action       string
	Result       string
	ResourceType string
	ResourceID   string
	Limit        int
	Offset       int
}

// ListAuditLogs returns audit records matching the given filters, newest first.
func (db *DB) ListAuditLogs(p ListAuditLogsParams) ([]*AuditLog, int, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if p.Limit > 500 {
		p.Limit = 500
	}

	where := "WHERE 1=1"
	args := []interface{}{}

	if p.ActorID != "" {
		where += " AND actor_id=?"
		args = append(args, p.ActorID)
	}
	if p.Category != "" {
		where += " AND category=?"
		args = append(args, p.Category)
	}
	if p.Action != "" {
		where += " AND action=?"
		args = append(args, p.Action)
	}
	if p.Result != "" {
		where += " AND result=?"
		args = append(args, p.Result)
	}
	if p.ResourceType != "" {
		where += " AND resource_type=?"
		args = append(args, p.ResourceType)
	}
	if p.ResourceID != "" {
		where += " AND resource_id=?"
		args = append(args, p.ResourceID)
	}

	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_logs "+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, p.Limit, p.Offset)
	rows, err := db.Query(`
		SELECT id,created_at,actor_id,actor_name,action,category,result,resource_type,resource_id,client_ip,user_agent,message,detail_json
		FROM audit_logs `+where+`
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?`, args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []*AuditLog
	for rows.Next() {
		l := &AuditLog{}
		var detailStr string
		if err := rows.Scan(
			&l.ID, &l.CreatedAt, &l.ActorID, &l.ActorName,
			&l.Action, &l.Category, &l.Result,
			&l.ResourceType, &l.ResourceID,
			&l.ClientIP, &l.UserAgent,
			&l.Message, &detailStr,
		); err != nil {
			return nil, 0, err
		}
		l.Detail = json.RawMessage(detailStr)
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}

// PurgeOldAuditLogs deletes audit records older than retentionDays. Returns count deleted.
func (db *DB) PurgeOldAuditLogs(retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	res, err := db.Exec(`DELETE FROM audit_logs WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
