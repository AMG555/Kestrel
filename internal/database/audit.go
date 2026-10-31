package database

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AuditLog platform operation audit record.
type AuditLog struct {
	ID                string                 `json:"id"`
	CreatedAt         time.Time              `json:"createdAt"`
	Level             string                 `json:"level"`
	Category          string                 `json:"category"`
	Action            string                 `json:"action"`
	Result            string                 `json:"result"`
	Actor             string                 `json:"actor"`
	SessionHint       string                 `json:"sessionHint,omitempty"`
	ClientIP          string                 `json:"clientIp,omitempty"`
	UserAgent         string                 `json:"userAgent,omitempty"`
	ResourceType      string                 `json:"resourceType,omitempty"`
	ResourceID        string                 `json:"resourceId,omitempty"`
	ResourceAvailable *bool                  `json:"resourceAvailable,omitempty"` // API-only: whether linked resource still exists
	Message           string                 `json:"message"`
	Detail            map[string]interface{} `json:"detail,omitempty"`
	PrevHash          string                 `json:"prevHash,omitempty"`
	Hash              string                 `json:"hash,omitempty"`
}

// ListAuditLogsFilter query parameters.
type ListAuditLogsFilter struct {
	Actor         string
	Level         string
	Category      string
	Action        string
	Result        string
	Query         string
	ResourceType  string
	ResourceID    string
	RelatedUserID string
	Since         *time.Time
	Until         *time.Time
	Limit         int
	Offset        int
}

func buildAuditLogsWhere(filter ListAuditLogsFilter) (string, []interface{}) {
	conditions := []string{"1=1"}
	args := []interface{}{}
	if filter.Actor != "" {
		conditions = append(conditions, "actor = ?")
		args = append(args, filter.Actor)
	}
	if filter.Level != "" {
		conditions = append(conditions, "level = ?")
		args = append(args, filter.Level)
	}
	if filter.Category != "" {
		conditions = append(conditions, "category = ?")
		args = append(args, filter.Category)
	}
	if filter.Action != "" {
		conditions = append(conditions, "action = ?")
		args = append(args, filter.Action)
	}
	if filter.Result != "" {
		conditions = append(conditions, "result = ?")
		args = append(args, filter.Result)
	}
	if filter.ResourceType != "" {
		conditions = append(conditions, "resource_type = ?")
		args = append(args, filter.ResourceType)
	}
	if filter.ResourceID != "" {
		conditions = append(conditions, "resource_id = ?")
		args = append(args, filter.ResourceID)
	}
	if relatedUserID := strings.TrimSpace(filter.RelatedUserID); relatedUserID != "" {
		conditions = append(conditions, `(actor = ? OR resource_id = ? OR detail_json LIKE ? OR detail_json LIKE ?)`)
		args = append(args, relatedUserID, relatedUserID, `%"user_id":"`+relatedUserID+`"%`, `%"userId":"`+relatedUserID+`"%`)
	}
	if filter.Since != nil {
		conditions = append(conditions, sqliteEpochGE("created_at", ">="))
		args = append(args, formatSQLiteUTC(*filter.Since))
	}
	if filter.Until != nil {
		conditions = append(conditions, sqliteEpochGE("created_at", "<="))
		args = append(args, formatSQLiteUTC(*filter.Until))
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		like := "%" + q + "%"
		conditions = append(conditions, "(message LIKE ? OR resource_id LIKE ? OR action LIKE ? OR category LIKE ? OR detail_json LIKE ?)")
		args = append(args, like, like, like, like, like)
	}
	return strings.Join(conditions, " AND "), args
}

// AppendAuditLog inserts one audit row atomically under a mutex and transaction.
func (db *DB) AppendAuditLog(row *AuditLog) error {
	if row == nil {
		return errors.New("audit log is nil")
	}
	if strings.TrimSpace(row.ID) == "" {
		return errors.New("audit id is required")
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
	} else {
		row.CreatedAt = row.CreatedAt.UTC()
	}
	if strings.TrimSpace(row.Level) == "" {
		row.Level = "info"
	}
	detailJSON := ""
	if len(row.Detail) > 0 {
		if b, err := json.Marshal(row.Detail); err == nil {
			detailJSON = string(b)
		}
	}

	db.auditAppendMu.Lock()
	defer db.auditAppendMu.Unlock()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin audit log transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	lastHash, err := lastAuditChainHash(tx)
	if err != nil {
		return fmt.Errorf("failed to determine audit chain predecessor: %w", err)
	}
	row.PrevHash = lastHash
	row.Hash = computeAuditHash(lastHash, row, detailJSON)

	query := `
		INSERT INTO audit_logs (
			id, created_at, level, category, action, result, actor, session_hint,
			client_ip, user_agent, resource_type, resource_id, message, detail_json,
			prev_hash, hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = tx.Exec(query,
		row.ID, formatSQLiteUTC(row.CreatedAt), row.Level, row.Category, row.Action, row.Result,
		row.Actor, row.SessionHint, row.ClientIP, row.UserAgent,
		row.ResourceType, row.ResourceID, row.Message, detailJSON,
		row.PrevHash, row.Hash,
	)
	if err != nil {
		return err
	}
	if err := setAuthMetadata(tx, "audit_chain_head", row.Hash); err != nil {
		return fmt.Errorf("failed to pin audit chain head: %w", err)
	}
	return tx.Commit()
}

// GetAuditLogByID returns one row.
func (db *DB) GetAuditLogByID(id string) (*AuditLog, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("id is required")
	}
	query := `
		SELECT id, created_at, level, category, action, result, actor,
			COALESCE(session_hint, ''), COALESCE(client_ip, ''), COALESCE(user_agent, ''),
			COALESCE(resource_type, ''), COALESCE(resource_id, ''), message, COALESCE(detail_json, ''),
			COALESCE(prev_hash, ''), COALESCE(hash, '')
		FROM audit_logs WHERE id = ?
	`
	var row AuditLog
	var detailJSON string
	err := db.QueryRow(query, id).Scan(
		&row.ID, &row.CreatedAt, &row.Level, &row.Category, &row.Action, &row.Result, &row.Actor,
		&row.SessionHint, &row.ClientIP, &row.UserAgent,
		&row.ResourceType, &row.ResourceID, &row.Message, &detailJSON,
		&row.PrevHash, &row.Hash,
	)
	if err != nil {
		return nil, err
	}
	if detailJSON != "" {
		_ = json.Unmarshal([]byte(detailJSON), &row.Detail)
	}
	return &row, nil
}

// CountAuditLogs counts rows matching filter.
func (db *DB) CountAuditLogs(filter ListAuditLogsFilter) (int64, error) {
	where, args := buildAuditLogsWhere(filter)
	query := `SELECT COUNT(*) FROM audit_logs WHERE ` + where
	var n int64
	err := db.QueryRow(query, args...).Scan(&n)
	return n, err
}

// ListAuditLogs lists audit rows newest first.
func (db *DB) ListAuditLogs(filter ListAuditLogsFilter) ([]*AuditLog, error) {
	where, args := buildAuditLogsWhere(filter)
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	query := `
		SELECT id, created_at, level, category, action, result, actor,
			COALESCE(session_hint, ''), COALESCE(client_ip, ''), COALESCE(user_agent, ''),
			COALESCE(resource_type, ''), COALESCE(resource_id, ''), message, COALESCE(detail_json, ''),
			COALESCE(prev_hash, ''), COALESCE(hash, '')
		FROM audit_logs
		WHERE ` + where + `
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`
	args = append(args, limit, offset)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*AuditLog
	for rows.Next() {
		var row AuditLog
		var detailJSON string
		if err := rows.Scan(
			&row.ID, &row.CreatedAt, &row.Level, &row.Category, &row.Action, &row.Result, &row.Actor,
			&row.SessionHint, &row.ClientIP, &row.UserAgent,
			&row.ResourceType, &row.ResourceID, &row.Message, &detailJSON,
			&row.PrevHash, &row.Hash,
		); err != nil {
			return nil, fmt.Errorf("failed to scan audit log row: %w", err)
		}
		if detailJSON != "" {
			_ = json.Unmarshal([]byte(detailJSON), &row.Detail)
		}
		list = append(list, &row)
	}
	return list, rows.Err()
}

// DeleteAuditLogsBefore archives rows older than cutoff into audit_log_archive and removes them from audit_logs.
func (db *DB) DeleteAuditLogsBefore(cutoff time.Time) (int64, error) {
	if db == nil {
		return 0, nil
	}
	db.auditAppendMu.Lock()
	defer db.auditAppendMu.Unlock()

	cutoffStr := formatSQLiteUTC(cutoff.UTC())

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin audit retention transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	createArchiveStmt := `
		CREATE TABLE IF NOT EXISTS audit_log_archive (
			id TEXT PRIMARY KEY,
			created_at DATETIME NOT NULL,
			level TEXT NOT NULL DEFAULT 'info',
			category TEXT NOT NULL,
			action TEXT NOT NULL,
			result TEXT NOT NULL,
			actor TEXT NOT NULL DEFAULT 'admin',
			session_hint TEXT,
			client_ip TEXT,
			user_agent TEXT,
			resource_type TEXT,
			resource_id TEXT,
			message TEXT NOT NULL,
			detail_json TEXT,
			prev_hash TEXT NOT NULL DEFAULT '',
			hash TEXT NOT NULL DEFAULT '',
			archived_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_audit_log_archive_created ON audit_log_archive(created_at);
		CREATE TRIGGER IF NOT EXISTS trg_audit_archive_no_update
		BEFORE UPDATE ON audit_log_archive
		BEGIN
			SELECT RAISE(ABORT, 'audit log archives are immutable and cannot be updated');
		END;
		CREATE TRIGGER IF NOT EXISTS trg_audit_archive_no_delete
		BEFORE DELETE ON audit_log_archive
		BEGIN
			SELECT RAISE(ABORT, 'audit log archives are immutable and cannot be deleted');
		END;
	`
	if _, err := tx.Exec(createArchiveStmt); err != nil {
		return 0, fmt.Errorf("failed to ensure audit_log_archive schema: %w", err)
	}

	archiveQuery := `
		INSERT OR IGNORE INTO audit_log_archive (
			id, created_at, level, category, action, result, actor, session_hint,
			client_ip, user_agent, resource_type, resource_id, message, detail_json,
			prev_hash, hash, archived_at
		)
		SELECT
			id, created_at, level, category, action, result, actor, session_hint,
			client_ip, user_agent, resource_type, resource_id, message, detail_json,
			prev_hash, hash, CURRENT_TIMESTAMP
		FROM audit_logs
		WHERE created_at < ?
		ORDER BY rowid ASC
	`
	res, err := tx.Exec(archiveQuery, cutoffStr)
	if err != nil {
		return 0, fmt.Errorf("failed to archive audit logs: %w", err)
	}
	archivedCount, _ := res.RowsAffected()
	if archivedCount == 0 {
		return 0, tx.Commit()
	}

	if _, err := tx.Exec(`DROP TRIGGER IF EXISTS trg_audit_logs_no_delete`); err != nil {
		return 0, fmt.Errorf("failed to suspend delete trigger: %w", err)
	}

	delRes, err := tx.Exec(`DELETE FROM audit_logs WHERE created_at < ?`, cutoffStr)
	if err != nil {
		return 0, fmt.Errorf("failed to purge archived audit logs: %w", err)
	}
	deletedCount, _ := delRes.RowsAffected()

	recreateTrigger := `
		CREATE TRIGGER IF NOT EXISTS trg_audit_logs_no_delete
		BEFORE DELETE ON audit_logs
		BEGIN
			SELECT RAISE(ABORT, 'audit logs are immutable and cannot be deleted');
		END;
	`
	if _, err := tx.Exec(recreateTrigger); err != nil {
		return 0, fmt.Errorf("failed to restore delete trigger: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deletedCount, nil
}

// PruneAuditLogArchiveBefore removes archived audit rows older than cutoff.
// Suspends and restores trg_audit_archive_no_delete within an exclusive transaction.
func (db *DB) PruneAuditLogArchiveBefore(cutoff time.Time) (int64, error) {
	if db == nil {
		return 0, nil
	}
	db.auditAppendMu.Lock()
	defer db.auditAppendMu.Unlock()

	cutoffStr := formatSQLiteUTC(cutoff.UTC())
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin archive pruning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DROP TRIGGER IF EXISTS trg_audit_archive_no_delete`); err != nil {
		return 0, fmt.Errorf("failed to suspend archive delete trigger: %w", err)
	}

	delRes, err := tx.Exec(`DELETE FROM audit_log_archive WHERE created_at < ?`, cutoffStr)
	if err != nil {
		return 0, fmt.Errorf("failed to prune audit log archive: %w", err)
	}
	deletedCount, _ := delRes.RowsAffected()

	recreateTrigger := `
		CREATE TRIGGER IF NOT EXISTS trg_audit_archive_no_delete
		BEFORE DELETE ON audit_log_archive
		BEGIN
			SELECT RAISE(ABORT, 'audit log archives are immutable and cannot be deleted');
		END;
	`
	if _, err := tx.Exec(recreateTrigger); err != nil {
		return 0, fmt.Errorf("failed to restore archive delete trigger: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deletedCount, nil
}

func computeAuditHash(prevHash string, row *AuditLog, detailJSON string) string {
	raw := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		prevHash,
		row.ID,
		row.CreatedAt.Format(time.RFC3339Nano),
		row.Level,
		row.Category,
		row.Action,
		row.Result,
		row.Actor,
		row.SessionHint,
		row.ClientIP,
		row.UserAgent,
		row.ResourceType,
		row.ResourceID,
		row.Message,
		detailJSON,
	)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// VerifyAuditLogChain checks the integrity of the audit log cryptographic hash chain across both active and archived logs, safely handling pre-migration unchained prefixes and verifying against the pinned head.
func (db *DB) VerifyAuditLogChain() (bool, error) {
	if db == nil {
		return true, nil
	}

	pinnedHead, _ := getAuthMetadata(db.DB, "audit_chain_head")

	var archiveExists int
	_ = db.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name='audit_log_archive'`).Scan(&archiveExists)

	expectedPrev := "GENESIS"
	hasVerifiedAny := false

	if archiveExists > 0 {
		aRows, err := db.Query(`
			SELECT id, created_at, level, category, action, result, actor,
				COALESCE(session_hint, ''), COALESCE(client_ip, ''), COALESCE(user_agent, ''),
				COALESCE(resource_type, ''), COALESCE(resource_id, ''), message, COALESCE(detail_json, ''),
				COALESCE(prev_hash, ''), COALESCE(hash, '')
			FROM audit_log_archive
			ORDER BY rowid ASC
		`)
		if err != nil {
			return false, err
		}
		defer aRows.Close()

		for aRows.Next() {
			var row AuditLog
			var createdAt, detailJSON, prevHash, hash string
			if err := aRows.Scan(
				&row.ID, &createdAt, &row.Level, &row.Category, &row.Action, &row.Result, &row.Actor,
				&row.SessionHint, &row.ClientIP, &row.UserAgent,
				&row.ResourceType, &row.ResourceID, &row.Message, &detailJSON,
				&prevHash, &hash,
			); err != nil {
				return false, err
			}
			if hash == "" {
				continue
			}
			row.CreatedAt = parseDBTime(createdAt).UTC()
			if prevHash != expectedPrev {
				return false, fmt.Errorf("hash chain broken in archive at log ID %s: expected prev_hash %s, got %s", row.ID, expectedPrev, prevHash)
			}
			computed := computeAuditHash(prevHash, &row, detailJSON)
			if computed != hash {
				return false, fmt.Errorf("audit log archive tampered at ID %s: expected hash %s, got %s", row.ID, computed, hash)
			}
			expectedPrev = hash
			hasVerifiedAny = true
		}
		if err := aRows.Err(); err != nil {
			return false, err
		}
	}

	rows, err := db.Query(`
		SELECT id, created_at, level, category, action, result, actor,
			COALESCE(session_hint, ''), COALESCE(client_ip, ''), COALESCE(user_agent, ''),
			COALESCE(resource_type, ''), COALESCE(resource_id, ''), message, COALESCE(detail_json, ''),
			COALESCE(prev_hash, ''), COALESCE(hash, '')
		FROM audit_logs
		ORDER BY rowid ASC
	`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var row AuditLog
		var createdAt, detailJSON, prevHash, hash string
		if err := rows.Scan(
			&row.ID, &createdAt, &row.Level, &row.Category, &row.Action, &row.Result, &row.Actor,
			&row.SessionHint, &row.ClientIP, &row.UserAgent,
			&row.ResourceType, &row.ResourceID, &row.Message, &detailJSON,
			&prevHash, &hash,
		); err != nil {
			return false, err
		}
		if hash == "" {
			// Unchained prefix from pre-migration schema — skip without failing integrity
			continue
		}
		row.CreatedAt = parseDBTime(createdAt).UTC()
		if prevHash != expectedPrev {
			return false, fmt.Errorf("hash chain broken at log ID %s: expected prev_hash %s, got %s", row.ID, expectedPrev, prevHash)
		}
		computed := computeAuditHash(prevHash, &row, detailJSON)
		if computed != hash {
			return false, fmt.Errorf("audit log tampered at ID %s: expected hash %s, got %s", row.ID, computed, hash)
		}
		expectedPrev = hash
		hasVerifiedAny = true
	}
	if err := rows.Err(); err != nil {
		return false, err
	}

	if pinnedHead != "" {
		if !hasVerifiedAny || expectedPrev != pinnedHead {
			return false, fmt.Errorf("audit log head mismatch: expected pinned head %s, got %s", pinnedHead, expectedPrev)
		}
	}

	return true, nil
}
