package database_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kestrel/internal/database"
	"go.uber.org/zap"
)

func TestRBACUserScanArityAndFields(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_rbac_scan.db")
	db, err := database.NewDB(dbPath, zap.NewNop())
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skip("skipping test: sqlite requires CGO_ENABLED=1")
		}
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	passHash := "$2a$10$abcdefghijklmnopqrstuvwxyz1234567890"
	if err := db.BootstrapRBAC(passHash, map[string]string{}); err != nil {
		t.Fatalf("BootstrapRBAC failed: %v", err)
	}

	// 1. Create a user
	username := "test_scanner"
	displayName := "Scanner Test User"
	user, err := db.CreateRBACUser(username, displayName, passHash, true, []string{"operator"})
	if err != nil {
		t.Fatalf("CreateRBACUser failed (scan arity mismatch?): %v", err)
	}
	if user == nil {
		t.Fatalf("expected non-nil user")
	}

	// 2. Query by username
	byUser, err := db.GetRBACUserByUsername(username)
	if err != nil {
		t.Fatalf("GetRBACUserByUsername failed (scan arity mismatch?): %v", err)
	}
	if byUser.ID != user.ID || byUser.Username != username {
		t.Errorf("GetRBACUserByUsername mismatch: got %+v", byUser)
	}

	// 3. Query by ID
	byID, err := db.GetRBACUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetRBACUserByID failed (scan arity mismatch?): %v", err)
	}
	if byID.ID != user.ID {
		t.Errorf("GetRBACUserByID mismatch: got %+v", byID)
	}

	// 4. Test SetRBACUserMustChangePassword
	if err := db.SetRBACUserMustChangePassword(user.ID, true); err != nil {
		t.Fatalf("SetRBACUserMustChangePassword failed: %v", err)
	}
	reloaded, err := db.GetRBACUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetRBACUserByID after must_change failed: %v", err)
	}
	if !reloaded.MustChangePassword {
		t.Errorf("expected MustChangePassword=true, got false")
	}

	// 5. ListRBACUsers
	list, err := db.ListRBACUsers()
	if err != nil {
		t.Fatalf("ListRBACUsers failed (scan arity mismatch?): %v", err)
	}
	found := false
	for _, u := range list {
		if u.ID == user.ID {
			found = true
			if !u.MustChangePassword {
				t.Errorf("ListRBACUsers: expected MustChangePassword=true, got false")
			}
			break
		}
	}
	if !found {
		t.Errorf("ListRBACUsers: created user not found in list")
	}
}

func TestRevocationCutoffPersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_cutoff.db")
	db, err := database.NewDB(dbPath, zap.NewNop())
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skip("skipping test: sqlite requires CGO_ENABLED=1")
		}
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	cutoff := time.Now().UTC().Truncate(time.Second)
	if err := db.SetRevocationCutoff(cutoff); err != nil {
		t.Fatalf("SetRevocationCutoff failed: %v", err)
	}

	loaded, err := db.GetRevocationCutoff()
	if err != nil {
		t.Fatalf("GetRevocationCutoff failed: %v", err)
	}
	if loaded.Unix() != cutoff.Unix() {
		t.Errorf("expected cutoff %v, got %v", cutoff, loaded)
	}
}
