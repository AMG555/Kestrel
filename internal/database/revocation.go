package database

import (
	"context"
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
	// Bind UTC RFC3339Nano string to match formatSQLiteUTC storage format
	err := db.QueryRow(`SELECT COUNT(1) FROM revoked_tokens WHERE token_or_jti = ? AND expires_at > ?`,
		strings.TrimSpace(tokenOrJTI), formatSQLiteUTC(time.Now().UTC())).Scan(&count)
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

// SetRevocationCutoff persists the global session revocation cutoff.
func (db *DB) SetRevocationCutoff(cutoff time.Time) error {
	if db == nil {
		return nil
	}
	return db.SetAuthMetadata("global_revocation_cutoff", formatSQLiteUTC(cutoff.UTC()))
}

// GetRevocationCutoff retrieves the persistent global session revocation cutoff.
func (db *DB) GetRevocationCutoff() (time.Time, error) {
	if db == nil {
		return time.Time{}, nil
	}
	val, err := db.GetAuthMetadata("global_revocation_cutoff")
	if err != nil {
		return time.Time{}, err
	}
	if strings.TrimSpace(val) == "" {
		return time.Time{}, nil
	}
	return parseDBTime(val), nil
}

// StartJanitor starts a periodic background sweep of expired revocation records.
func (db *DB) StartJanitor(ctx context.Context, interval time.Duration) {
	if db == nil {
		return
	}
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = db.PurgeExpiredRevokedTokens()
			}
		}
	}()
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
