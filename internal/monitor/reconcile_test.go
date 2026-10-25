package monitor

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kestrel/internal/database"
	"kestrel/internal/mcp"

	"go.uber.org/zap"
)

func TestExecutionReconciler_ReconcileOnStartup(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "monitor.db")
	db, err := database.NewDB(dbPath, zap.NewNop())
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skip("skipping test: sqlite requires CGO_ENABLED=1")
		}
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	if err := db.SaveToolExecution(&mcp.ToolExecution{
		ID: "run-1", ToolName: "hydra", Status: "running", StartTime: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("SaveToolExecution: %v", err)
	}

	r := NewExecutionReconciler(db, mcp.NewServer(zap.NewNop()), nil, zap.NewNop())
	r.ReconcileOnStartup()

	got, err := db.GetToolExecution("run-1")
	if err != nil {
		t.Fatalf("GetToolExecution: %v", err)
	}
	if got.Status != "orphaned" {
		t.Fatalf("expected orphaned after startup reconcile, got %s", got.Status)
	}
}
