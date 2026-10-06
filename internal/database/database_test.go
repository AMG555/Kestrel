package database_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"kestrel/internal/database"
)

func newDB(t *testing.T) *database.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := database.New(dbPath, zap.NewNop())
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skip("database tests require CGO (sqlite3) — skipping in CGO-disabled build")
		}
		t.Fatalf("database.New() error: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		os.Remove(dbPath)
	})
	return db
}

// ── Users ────────────────────────────────────────────────────────────────────

func TestCreateAndGetUser(t *testing.T) {
	db := newDB(t)

	user, err := db.CreateUser("alice", "hash123", "alice@example.com", "Alice")
	if err != nil {
		t.Fatalf("CreateUser() error: %v", err)
	}
	if user.ID == "" {
		t.Error("user.ID must not be empty")
	}
	if user.Username != "alice" {
		t.Errorf("Username = %q, want %q", user.Username, "alice")
	}

	got, err := db.GetUserByUsername("alice")
	if err != nil {
		t.Fatalf("GetUserByUsername() error: %v", err)
	}
	if got == nil || got.ID != user.ID {
		t.Errorf("GetUserByUsername() returned wrong user")
	}
}

func TestCreateDuplicateUser(t *testing.T) {
	db := newDB(t)
	if _, err := db.CreateUser("bob", "hash", "", ""); err != nil {
		t.Fatalf("first CreateUser() error: %v", err)
	}
	_, err := db.CreateUser("bob", "hash2", "", "")
	if err == nil {
		t.Error("duplicate username should return error")
	}
}

func TestUserNotFound(t *testing.T) {
	db := newDB(t)
	user, err := db.GetUserByUsername("nonexistent")
	if err != nil {
		t.Fatalf("GetUserByUsername() error: %v", err)
	}
	if user != nil {
		t.Error("expected nil user for nonexistent username")
	}
}

// ── Projects ─────────────────────────────────────────────────────────────────

func TestCreateProject(t *testing.T) {
	db := newDB(t)

	proj, err := db.CreateProject("Test Project", "desc", "user-1", []string{"example.com"})
	if err != nil {
		t.Fatalf("CreateProject() error: %v", err)
	}
	if proj.ID == "" {
		t.Error("project.ID must not be empty")
	}
	if proj.Status != "active" {
		t.Errorf("default status = %q, want %q", proj.Status, "active")
	}

	got, err := db.GetProjectByID(proj.ID)
	if err != nil {
		t.Fatalf("GetProjectByID() error: %v", err)
	}
	if got == nil || got.Name != "Test Project" {
		t.Error("GetProjectByID() returned wrong project")
	}
	if len(got.Scope) != 1 || got.Scope[0] != "example.com" {
		t.Error("scope not round-tripped correctly")
	}
}

// ── Assets ────────────────────────────────────────────────────────────────────

func TestUpsertAsset(t *testing.T) {
	db := newDB(t)

	a := &database.Asset{
		Host:      "example.com",
		Domain:    "example.com",
		IP:        "93.184.216.34",
		Port:      443,
		Protocol:  "https",
		Service:   "web",
		Status:    "active",
		RiskLevel: "unassessed",
		Source:    "manual",
	}

	created, err := db.UpsertAsset(a)
	if err != nil {
		t.Fatalf("UpsertAsset() error: %v", err)
	}
	if created.ID == "" {
		t.Error("created asset must have an ID")
	}

	// Upsert again — should update, not duplicate.
	created.IP = "93.184.216.99"
	updated, err := db.UpsertAsset(created)
	if err != nil {
		t.Fatalf("UpsertAsset() update error: %v", err)
	}
	if updated.IP != "93.184.216.99" {
		t.Errorf("IP after update = %q, want %q", updated.IP, "93.184.216.99")
	}

	assets, total, err := db.ListAssets(database.ListAssetsParams{Limit: 10})
	if err != nil {
		t.Fatalf("ListAssets() error: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1 (dedup)", total)
	}
	if len(assets) != 1 {
		t.Errorf("len(assets) = %d, want 1", len(assets))
	}
}

// ── Vulnerabilities ───────────────────────────────────────────────────────────

func TestCreateVulnerability(t *testing.T) {
	db := newDB(t)

	v := &database.Vulnerability{
		Title:       "SQL Injection",
		Description: "Classic SQLi in login form",
		Severity:    "high",
		Target:      "https://example.com/login",
	}
	created, err := db.CreateVulnerability(v)
	if err != nil {
		t.Fatalf("CreateVulnerability() error: %v", err)
	}
	if created.ID == "" {
		t.Error("created vuln must have an ID")
	}
	if created.Status != "open" {
		t.Errorf("default status = %q, want %q", created.Status, "open")
	}
}

// ── Audit logs ────────────────────────────────────────────────────────────────

func TestAuditLogAppendOnly(t *testing.T) {
	db := newDB(t)

	if err := db.WriteAuditLog(database.AuditParams{
		ActorID:  "user-1",
		Action:   "login",
		Category: "auth",
		Result:   "success",
		Message:  "user logged in",
	}); err != nil {
		t.Fatalf("WriteAuditLog() error: %v", err)
	}

	logs, total, err := db.ListAuditLogs(database.ListAuditLogsParams{Limit: 10})
	if err != nil {
		t.Fatalf("ListAuditLogs() error: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if logs[0].Action != "login" {
		t.Errorf("action = %q, want %q", logs[0].Action, "login")
	}
}

// ── AdminExists ───────────────────────────────────────────────────────────────

func TestAdminExists(t *testing.T) {
	db := newDB(t)

	exists, err := db.AdminExists()
	if err != nil {
		t.Fatalf("AdminExists() error: %v", err)
	}
	if exists {
		t.Error("fresh DB should not have an admin user")
	}

	if _, err := db.CreateUser("admin", "hash", "", ""); err != nil {
		t.Fatalf("CreateUser(admin) error: %v", err)
	}

	exists, err = db.AdminExists()
	if err != nil {
		t.Fatalf("AdminExists() error: %v", err)
	}
	if !exists {
		t.Error("AdminExists() returned false after creating admin")
	}
}
