package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ReportShare stores metadata and security settings for signed read-only report permalinks.
type ReportShare struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"project_id"`
	TokenHash      string    `json:"-"`
	PassphraseHash string    `json:"-"`
	HasPassphrase  bool      `json:"has_passphrase"`
	ExpiresAt      time.Time `json:"expires_at"`
	Views          int       `json:"views"`
	Revoked        bool      `json:"revoked"`
	CreatedAt      time.Time `json:"created_at"`
}

// HashShareToken calculates SHA-256 digest of a share token for safe DB lookup.
func HashShareToken(token string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(h[:])
}

// CreateReportShare saves a new share link record.
func (db *DB) CreateReportShare(share *ReportShare) error {
	if share == nil {
		return fmt.Errorf("report share is nil")
	}
	if share.ID == "" {
		share.ID = uuid.NewString()
	}
	if share.CreatedAt.IsZero() {
		share.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO report_shares (id, project_id, token_hash, passphrase_hash, expires_at, views, revoked, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query,
		share.ID,
		share.ProjectID,
		share.TokenHash,
		share.PassphraseHash,
		share.ExpiresAt,
		share.Views,
		share.Revoked,
		share.CreatedAt,
	)
	return err
}

// GetReportShareByTokenHash finds an active share link by token hash.
func (db *DB) GetReportShareByTokenHash(tokenHash string) (*ReportShare, error) {
	query := `
		SELECT id, project_id, token_hash, passphrase_hash, expires_at, views, revoked, created_at
		FROM report_shares
		WHERE token_hash = ?
	`
	row := db.QueryRow(query, tokenHash)
	var s ReportShare
	err := row.Scan(
		&s.ID,
		&s.ProjectID,
		&s.TokenHash,
		&s.PassphraseHash,
		&s.ExpiresAt,
		&s.Views,
		&s.Revoked,
		&s.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	s.HasPassphrase = strings.TrimSpace(s.PassphraseHash) != ""
	return &s, nil
}

// IncrementReportShareViews increments view counter for a share permalink.
func (db *DB) IncrementReportShareViews(id string) error {
	_, err := db.Exec("UPDATE report_shares SET views = views + 1 WHERE id = ?", id)
	return err
}

// RevokeReportShare marks a share link as revoked.
func (db *DB) RevokeReportShare(id, projectID string) error {
	_, err := db.Exec("UPDATE report_shares SET revoked = 1 WHERE id = ? AND project_id = ?", id, projectID)
	return err
}

// ListReportSharesForProject lists all share links for a specific project.
func (db *DB) ListReportSharesForProject(projectID string) ([]*ReportShare, error) {
	query := `
		SELECT id, project_id, token_hash, passphrase_hash, expires_at, views, revoked, created_at
		FROM report_shares
		WHERE project_id = ?
		ORDER BY created_at DESC
	`
	rows, err := db.Query(query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*ReportShare
	for rows.Next() {
		var s ReportShare
		if err := rows.Scan(
			&s.ID,
			&s.ProjectID,
			&s.TokenHash,
			&s.PassphraseHash,
			&s.ExpiresAt,
			&s.Views,
			&s.Revoked,
			&s.CreatedAt,
		); err != nil {
			return nil, err
		}
		s.HasPassphrase = strings.TrimSpace(s.PassphraseHash) != ""
		out = append(out, &s)
	}
	return out, rows.Err()
}
