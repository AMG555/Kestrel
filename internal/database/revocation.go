package database

import (
	"fmt"
	"strings"
	"time"
)

// IsTokenRevoked checks if a token or JTI is recorded as revoked.
func (db *DB) IsTokenRevoked(tokenOrJTI string) bool {
	if db == nil || strings.TrimSpace(tokenOrJTI) == "" {
		return false
	}
	var count int
	err := db.QueryRow(`SELECT COUNT(1) FROM revoked_tokens WHERE token_or_jti = ? AND expires_at > ?`,
		strings.TrimSpace(tokenOrJTI), time.Now().UTC()).Scan(&count)
	return err == nil && count > 0
}

// RecordRevokedToken marks a token or JTI as revoked until its expiration time.
func (db *DB) RecordRevokedToken(tokenOrJTI string, expiresAt time.Time) error {
	if db == nil || strings.TrimSpace(tokenOrJTI) == "" {
		return nil
	}
	if expiresAt.IsZero() {
		expiresAt = time.Now().UTC().Add(7 * 24 * time.Hour)
	}
	_, err := db.Exec(`
		INSERT OR REPLACE INTO revoked_tokens (token_or_jti, revoked_at, expires_at)
		VALUES (?, ?, ?)
	`, strings.TrimSpace(tokenOrJTI), formatSQLiteUTC(time.Now().UTC()), formatSQLiteUTC(expiresAt.UTC()))
	return err
}

// PurgeExpiredRevokedTokens deletes expired revocation records.
func (db *DB) PurgeExpiredRevokedTokens() error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(`DELETE FROM revoked_tokens WHERE expires_at <= ?`, formatSQLiteUTC(time.Now().UTC()))
	return err
}

// TouchRBACUser updates the user's updated_at timestamp to invalidate earlier tokens.
func (db *DB) TouchRBACUser(userID string) error {
	if db == nil || strings.TrimSpace(userID) == "" {
		return nil
	}
	res, err := db.Exec(`UPDATE rbac_users SET updated_at = ? WHERE id = ?`, formatSQLiteUTC(time.Now().UTC()), strings.TrimSpace(userID))
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("user %s not found to touch", userID)
	}
	return nil
}
