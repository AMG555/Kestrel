package database

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestAuditLogChain_VerifiesIntegrityAndDetectsTampering(t *testing.T) {
	tempDB := filepath.Join(t.TempDir(), "audit_test.db")
	db, err := NewDB(tempDB, zap.NewNop())
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skip("skipping in CGO_ENABLED=0 environment")
		}
		t.Fatalf("failed to create test db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Append 3 audit logs
	for i := 1; i <= 3; i++ {
		log := &AuditLog{
			ID:          "audit-log-" + string(rune('0'+i)),
			CreatedAt:   time.Now().UTC().Add(time.Duration(i) * time.Minute),
			Level:       "info",
			Category:    "auth",
			Action:      "login",
			Result:      "success",
			Actor:       "operator",
			SessionHint: "sess-123",
			ClientIP:    "192.168.1.50",
			UserAgent:   "Kestrel-Agent/1.0",
			Message:     "operator logged in successfully",
		}
		if err := db.AppendAuditLog(log); err != nil {
			t.Fatalf("AppendAuditLog failed: %v", err)
		}
	}

	// Verify uncorrupted chain
	valid, err := db.VerifyAuditLogChain()
	if err != nil || !valid {
		t.Fatalf("expected valid chain, got valid=%v, err=%v", valid, err)
	}

	// Tamper test: drop update trigger and tamper with client_ip
	_, _ = db.Exec("DROP TRIGGER IF EXISTS trg_audit_logs_no_update")
	_, err = db.Exec("UPDATE audit_logs SET client_ip = '10.0.0.99' WHERE id = 'audit-log-2'")
	if err != nil {
		t.Fatalf("failed to tamper with log: %v", err)
	}

	// Chain verification MUST now fail due to client_ip tamper
	valid, err = db.VerifyAuditLogChain()
	if valid || err == nil || !strings.Contains(err.Error(), "audit log tampered") {
		t.Fatalf("expected tamper detection, got valid=%v, err=%v", valid, err)
	}
}

func TestAuditRetention_ArchiveThenDelete(t *testing.T) {
	tempDB := filepath.Join(t.TempDir(), "retention_test.db")
	db, err := NewDB(tempDB, zap.NewNop())
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skip("skipping in CGO_ENABLED=0 environment")
		}
		t.Fatalf("failed to create test db: %v", err)
	}
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	oldTime := now.Add(-30 * 24 * time.Hour)

	// Append 2 old logs (30 days ago) and 2 recent logs
	for i := 1; i <= 2; i++ {
		oldLog := &AuditLog{
			ID:        "audit-old-" + string(rune('0'+i)),
			CreatedAt: oldTime.Add(time.Duration(i) * time.Hour),
			Level:     "info",
			Category:  "scan",
			Action:    "run",
			Result:    "success",
			Actor:     "operator",
			Message:   "old scan execution",
		}
		if err := db.AppendAuditLog(oldLog); err != nil {
			t.Fatalf("AppendAuditLog old failed: %v", err)
		}
	}

	for i := 1; i <= 2; i++ {
		recentLog := &AuditLog{
			ID:        "audit-new-" + string(rune('0'+i)),
			CreatedAt: now.Add(time.Duration(i) * time.Hour),
			Level:     "info",
			Category:  "scan",
			Action:    "run",
			Result:    "success",
			Actor:     "operator",
			Message:   "recent scan execution",
		}
		if err := db.AppendAuditLog(recentLog); err != nil {
			t.Fatalf("AppendAuditLog recent failed: %v", err)
		}
	}

	// Purge rows older than 7 days
	cutoff := now.Add(-7 * 24 * time.Hour)
	deleted, err := db.DeleteAuditLogsBefore(cutoff)
	if err != nil {
		t.Fatalf("DeleteAuditLogsBefore failed: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 deleted rows, got %d", deleted)
	}

	// Verify archived rows
	var archiveCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_log_archive").Scan(&archiveCount); err != nil {
		t.Fatalf("query archive count: %v", err)
	}
	if archiveCount != 2 {
		t.Fatalf("expected 2 archived rows, got %d", archiveCount)
	}

	// Verify remaining active rows
	var activeCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&activeCount); err != nil {
		t.Fatalf("query active count: %v", err)
	}
	if activeCount != 2 {
		t.Fatalf("expected 2 active rows, got %d", activeCount)
	}

	// Verify chain integrity of remaining active rows
	valid, err := db.VerifyAuditLogChain()
	if err != nil || !valid {
		t.Fatalf("expected remaining active chain to be valid, got valid=%v, err=%v", valid, err)
	}
}

