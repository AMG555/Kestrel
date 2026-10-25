package security

import (
	"path/filepath"
	"strings"
	"testing"

	"kestrel/internal/database"

	"go.uber.org/zap"
)

func TestAttachRBACStoreBootstrapsAdminPassword(t *testing.T) {
	db, err := database.NewDB(filepath.Join(t.TempDir(), "auth-bootstrap.db"), zap.NewNop())
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skip("skipping test: sqlite requires CGO_ENABLED=1")
		}
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	manager := NewAuthManager(12)
	generated, err := manager.AttachRBACStore(db)
	if err != nil {
		t.Fatalf("AttachRBACStore: %v", err)
	}
	if generated == "" {
		t.Fatal("expected generated admin password on first bootstrap")
	}
	if !manager.CheckUserPassword("admin", generated) {
		t.Fatal("generated password should authenticate admin")
	}

	second, err := manager.AttachRBACStore(db)
	if err != nil {
		t.Fatalf("AttachRBACStore second call: %v", err)
	}
	if second != "" {
		t.Fatalf("expected no password on second bootstrap, got %q", second)
	}
}
