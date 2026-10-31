package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// sqlExecQuerier provides a unified interface for *sql.DB and *sql.Tx.
type sqlExecQuerier interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

// setAuthMetadata executes an upsert for key/value into auth_metadata table.
func setAuthMetadata(q sqlExecQuerier, key, value string) error {
	if q == nil {
		return fmt.Errorf("database querier is nil")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("auth_metadata key cannot be empty")
	}
	_, err := q.Exec(`
		INSERT INTO auth_metadata (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, key, value, formatSQLiteUTC(time.Now().UTC()))
	return err
}

// getAuthMetadata retrieves the value associated with key from auth_metadata table.
func getAuthMetadata(q sqlExecQuerier, key string) (string, error) {
	if q == nil {
		return "", fmt.Errorf("database querier is nil")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("auth_metadata key cannot be empty")
	}
	var val string
	err := q.QueryRow(`SELECT value FROM auth_metadata WHERE key = ?`, key).Scan(&val)
	if err != nil {
		return "", err
	}
	return val, nil
}

// SetAuthMetadata persists a key/value configuration pair in auth_metadata.
func (db *DB) SetAuthMetadata(key, value string) error {
	if db == nil {
		return nil
	}
	return setAuthMetadata(db.DB, key, value)
}

// GetAuthMetadata retrieves a key/value configuration pair from auth_metadata.
func (db *DB) GetAuthMetadata(key string) (string, error) {
	if db == nil {
		return "", nil
	}
	return getAuthMetadata(db.DB, key)
}

// lastAuditChainHash returns the last recorded hash in the audit chain.
// It checks the active audit_logs table first. If empty, it falls back to the
// tail of audit_log_archive. If both are empty, it returns "GENESIS".
func lastAuditChainHash(q sqlExecQuerier) (string, error) {
	if q == nil {
		return "GENESIS", nil
	}
	var lastHash string
	err := q.QueryRow(`SELECT hash FROM audit_logs WHERE hash != '' ORDER BY rowid DESC LIMIT 1`).Scan(&lastHash)
	if err == nil && lastHash != "" {
		return lastHash, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}

	// Active table has no chained rows; check archive tail
	err = q.QueryRow(`SELECT hash FROM audit_log_archive WHERE hash != '' ORDER BY rowid DESC LIMIT 1`).Scan(&lastHash)
	if err == nil && lastHash != "" {
		return lastHash, nil
	}
	if err != nil && err != sql.ErrNoRows {
		// Table might not exist yet if no purges have occurred
		return "GENESIS", nil
	}
	return "GENESIS", nil
}
