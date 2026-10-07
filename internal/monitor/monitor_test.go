package monitor

import (
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"kestrel/internal/database"
)

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath, zap.NewNop())
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skip("database tests require CGO (sqlite3) — skipping in CGO-disabled build")
		}
		t.Fatalf("failed to create test db: %v", err)
	}
	return db
}

func TestReconcileOnStartup(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rec := NewExecutionReconciler(db, zap.NewNop())
	count := rec.ReconcileOnStartup()
	if count < 0 {
		t.Errorf("expected non-negative count, got %d", count)
	}
}

func TestRetentionPurge(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := NewRetentionService(db, zap.NewNop())
	purged := svc.PurgeExpired(30)
	if purged < 0 {
		t.Errorf("expected non-negative purged count, got %d", purged)
	}
}
