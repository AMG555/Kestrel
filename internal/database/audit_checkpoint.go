package database

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// AuditCheckpoint represents an externally persisted cryptographic anchor of the audit chain state.
type AuditCheckpoint struct {
	Version      int       `json:"version"`
	Timestamp    time.Time `json:"timestamp"`
	HeadHash     string    `json:"headHash"`
	HeadAuditID  string    `json:"headAuditId,omitempty"`
	ChainedCount int64     `json:"chainedCount"`
	ArchiveCount int64     `json:"archiveCount"`
	ActiveCount  int64     `json:"activeCount"`
	Seal         string    `json:"seal"`
}

// computeCheckpointSeal produces a SHA-256 digest over the checkpoint fields.
func computeCheckpointSeal(version int, ts time.Time, headHash, headID string, chainedCount int64) string {
	raw := fmt.Sprintf("%d|%s|%s|%s|%d", version, ts.UTC().Format(time.RFC3339Nano), headHash, headID, chainedCount)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// CreateAuditCheckpoint verifies chain integrity and produces an exportable cryptographic checkpoint receipt.
func (db *DB) CreateAuditCheckpoint() (*AuditCheckpoint, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}

	valid, err := db.VerifyAuditLogChain()
	if err != nil {
		return nil, fmt.Errorf("cannot create checkpoint on invalid audit chain: %w", err)
	}
	if !valid {
		return nil, errors.New("audit log chain integrity check failed")
	}

	var headHash, headID string
	err = db.QueryRow(`SELECT hash, id FROM audit_logs WHERE hash != '' ORDER BY rowid DESC LIMIT 1`).Scan(&headHash, &headID)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query active audit head: %w", err)
	}

	var activeCount int64
	_ = db.QueryRow(`SELECT COUNT(1) FROM audit_logs WHERE hash != ''`).Scan(&activeCount)

	var archiveCount int64
	var archiveExists int
	_ = db.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name='audit_log_archive'`).Scan(&archiveExists)
	if archiveExists > 0 {
		_ = db.QueryRow(`SELECT COUNT(1) FROM audit_log_archive WHERE hash != ''`).Scan(&archiveCount)
		if headHash == "" {
			_ = db.QueryRow(`SELECT hash, id FROM audit_log_archive WHERE hash != '' ORDER BY rowid DESC LIMIT 1`).Scan(&headHash, &headID)
		}
	}

	if headHash == "" {
		return nil, errors.New("audit chain is empty: no chained records exist to checkpoint")
	}

	now := time.Now().UTC()
	chainedCount := activeCount + archiveCount
	seal := computeCheckpointSeal(1, now, headHash, headID, chainedCount)

	return &AuditCheckpoint{
		Version:      1,
		Timestamp:    now,
		HeadHash:     headHash,
		HeadAuditID:  headID,
		ChainedCount: chainedCount,
		ArchiveCount: archiveCount,
		ActiveCount:  activeCount,
		Seal:         seal,
	}, nil
}

// VerifyAuditCheckpoint asserts that an external checkpoint exists in the audit chain and has not been truncated or forked.
func (db *DB) VerifyAuditCheckpoint(cp *AuditCheckpoint) (bool, error) {
	if db == nil {
		return false, errors.New("database is nil")
	}
	if cp == nil {
		return false, errors.New("audit checkpoint is nil")
	}

	// 1. Verify checkpoint seal integrity
	expectedSeal := computeCheckpointSeal(cp.Version, cp.Timestamp, cp.HeadHash, cp.HeadAuditID, cp.ChainedCount)
	if cp.Seal != expectedSeal {
		return false, fmt.Errorf("audit checkpoint seal is invalid (tampered checkpoint receipt)")
	}

	// 2. Verify complete active + archive chain continuity
	valid, err := db.VerifyAuditLogChain()
	if err != nil {
		return false, fmt.Errorf("audit chain integrity verification failed: %w", err)
	}
	if !valid {
		return false, errors.New("audit log chain integrity check failed")
	}

	// 3. Assert checkpoint head exists in the database chain
	var existsInActive int
	_ = db.QueryRow(`SELECT COUNT(1) FROM audit_logs WHERE hash = ?`, cp.HeadHash).Scan(&existsInActive)

	var existsInArchive int
	var archiveTableExists int
	_ = db.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name='audit_log_archive'`).Scan(&archiveTableExists)
	if archiveTableExists > 0 {
		_ = db.QueryRow(`SELECT COUNT(1) FROM audit_log_archive WHERE hash = ?`, cp.HeadHash).Scan(&existsInArchive)
	}

	if existsInActive == 0 && existsInArchive == 0 {
		return false, fmt.Errorf("external checkpoint head %s (audit ID %s) was NOT found in the database (audit chain was truncated, rewound, or forked)", cp.HeadHash, cp.HeadAuditID)
	}

	return true, nil
}
