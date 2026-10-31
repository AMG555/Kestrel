package database

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestAuditCheckpoint_LifecycleAndTamper(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "checkpoint_test.db")
	db, err := NewDB(dbPath, zap.NewNop())
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skip("skipping in CGO_ENABLED=0 environment")
		}
		t.Fatalf("failed to open test database: %v", err)
	}
	defer db.Close()

	// 1. Checkpoint on empty chain fails
	if _, err := db.CreateAuditCheckpoint(); err == nil {
		t.Fatal("expected error creating checkpoint on empty chain, got nil")
	}

	// 2. Append records
	for i := 1; i <= 5; i++ {
		row := &AuditLog{
			ID:        fmt.Sprintf("audit-cp-%d", i),
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Minute),
			Level:     "info",
			Category:  "auth",
			Action:    "login",
			Result:    "success",
			Actor:     "operator",
			Message:   "operator login event",
		}
		if err := db.AppendAuditLog(row); err != nil {
			t.Fatalf("failed to append log: %v", err)
		}
	}

	// 3. Create valid checkpoint
	cp, err := db.CreateAuditCheckpoint()
	if err != nil {
		t.Fatalf("CreateAuditCheckpoint failed: %v", err)
	}
	if cp.ChainedCount != 5 {
		t.Fatalf("expected ChainedCount 5, got %d", cp.ChainedCount)
	}
	if cp.HeadHash == "" || cp.Seal == "" {
		t.Fatalf("checkpoint headHash or seal is empty: %+v", cp)
	}

	// 4. Verify checkpoint against unchanged DB
	valid, err := db.VerifyAuditCheckpoint(cp)
	if err != nil || !valid {
		t.Fatalf("VerifyAuditCheckpoint failed on valid DB: valid=%v, err=%v", valid, err)
	}

	// 5. Tampered checkpoint seal rejected
	tamperedCP := *cp
	tamperedCP.Seal = "0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := db.VerifyAuditCheckpoint(&tamperedCP); err == nil {
		t.Fatal("expected error on tampered seal, got nil")
	}

	// 6. Append more rows — checkpoint head is still an ancestor, verification succeeds
	if err := db.AppendAuditLog(&AuditLog{
		ID:        "audit-cp-next-1",
		CreatedAt: time.Now().UTC().Add(10 * time.Minute),
		Level:     "info",
		Category:  "auth",
		Action:    "token_refresh",
		Result:    "success",
		Actor:     "operator",
		Message:   "refresh token",
	}); err != nil {
		t.Fatalf("failed to append subsequent log: %v", err)
	}

	valid, err = db.VerifyAuditCheckpoint(cp)
	if err != nil || !valid {
		t.Fatalf("VerifyAuditCheckpoint failed after extension: valid=%v, err=%v", valid, err)
	}

	// 7. External anchor detects table truncation / rewound chain
	dbPathRewound := filepath.Join(t.TempDir(), "checkpoint_rewound.db")
	dbRewound, err := NewDB(dbPathRewound, zap.NewNop())
	if err != nil {
		t.Fatalf("failed to open second test db: %v", err)
	}
	defer dbRewound.Close()

	// Append DIFFERENT logs in rewound db (so it has a valid chain, but lacks the checkpointed head)
	for i := 1; i <= 3; i++ {
		_ = dbRewound.AppendAuditLog(&AuditLog{
			ID:        fmt.Sprintf("audit-different-%d", i),
			CreatedAt: time.Now().UTC(),
			Level:     "info",
			Category:  "test",
			Action:    "run",
			Result:    "success",
			Actor:     "attacker",
			Message:   "fabricated history",
		})
	}

	valid, err = dbRewound.VerifyAuditCheckpoint(cp)
	if err == nil || valid {
		t.Fatalf("expected rewound/fabricated chain to be rejected by checkpoint, got valid=%v, err=%v", valid, err)
	}
}
