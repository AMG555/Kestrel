package database_test

import (
	"testing"
	"time"

	"kestrel/internal/database"
)

// ── AgentSession CRUD ─────────────────────────────────────────────────────────

func TestCreateAndListAgentSessions(t *testing.T) {
	db := newDB(t)

	s, err := db.CreateAgentSession(&database.AgentSession{
		UserID:    "user-1",
		Title:     "Recon for example.com",
		AgentMode: "single",
		HITLMode:  "auto",
	})
	if err != nil {
		t.Fatalf("CreateAgentSession() error: %v", err)
	}
	if s.ID == "" {
		t.Fatal("session ID must not be empty")
	}
	if s.Status != "active" {
		t.Errorf("default status = %q, want %q", s.Status, "active")
	}

	// Listing
	sessions, total, err := db.ListAgentSessions(database.ListAgentSessionsParams{UserID: "user-1"})
	if err != nil {
		t.Fatalf("ListAgentSessions() error: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(sessions) != 1 {
		t.Errorf("len(sessions) = %d, want 1", len(sessions))
	}
}

func TestGetAgentSessionByID(t *testing.T) {
	db := newDB(t)

	created, err := db.CreateAgentSession(&database.AgentSession{UserID: "user-1"})
	if err != nil {
		t.Fatalf("CreateAgentSession() error: %v", err)
	}

	got, err := db.GetAgentSessionByID(created.ID)
	if err != nil {
		t.Fatalf("GetAgentSessionByID() error: %v", err)
	}
	if got == nil {
		t.Fatal("GetAgentSessionByID() returned nil")
	}
	if got.ID != created.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, created.ID)
	}
}

func TestGetAgentSessionByIDNotFound(t *testing.T) {
	db := newDB(t)
	got, err := db.GetAgentSessionByID("nonexistent-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent session")
	}
}

func TestUpdateAgentSession(t *testing.T) {
	db := newDB(t)
	s, _ := db.CreateAgentSession(&database.AgentSession{UserID: "user-1"})

	pinned := true
	if err := db.UpdateAgentSession(s.ID, "New Title", "completed", &pinned); err != nil {
		t.Fatalf("UpdateAgentSession() error: %v", err)
	}

	got, _ := db.GetAgentSessionByID(s.ID)
	if got.Title != "New Title" {
		t.Errorf("Title = %q, want %q", got.Title, "New Title")
	}
	if got.Status != "completed" {
		t.Errorf("Status = %q, want %q", got.Status, "completed")
	}
	if !got.Pinned {
		t.Error("Pinned should be true")
	}
}

func TestDeleteAgentSession(t *testing.T) {
	db := newDB(t)
	s, _ := db.CreateAgentSession(&database.AgentSession{UserID: "user-1"})

	if err := db.DeleteAgentSession(s.ID); err != nil {
		t.Fatalf("DeleteAgentSession() error: %v", err)
	}

	got, err := db.GetAgentSessionByID(s.ID)
	if err != nil {
		t.Fatalf("GetAgentSessionByID() after delete error: %v", err)
	}
	if got != nil {
		t.Error("expected nil session after deletion")
	}
}

// ── ListAgentSessions filters ──────────────────────────────────────────────────

func TestListAgentSessionsStatusFilter(t *testing.T) {
	db := newDB(t)

	// Create two sessions with different statuses.
	s1, _ := db.CreateAgentSession(&database.AgentSession{UserID: "user-1"})
	db.UpdateAgentSession(s1.ID, "", "completed", nil)
	db.CreateAgentSession(&database.AgentSession{UserID: "user-1"}) // remains active

	active, total, err := db.ListAgentSessions(database.ListAgentSessionsParams{Status: "active"})
	if err != nil {
		t.Fatalf("ListAgentSessions(active) error: %v", err)
	}
	if total != 1 || len(active) != 1 {
		t.Errorf("active filter: got %d, want 1", total)
	}

	completed, _, _ := db.ListAgentSessions(database.ListAgentSessionsParams{Status: "completed"})
	if len(completed) != 1 {
		t.Errorf("completed filter: got %d, want 1", len(completed))
	}
}

func TestListAgentSessionsPinnedFirst(t *testing.T) {
	db := newDB(t)

	s1, _ := db.CreateAgentSession(&database.AgentSession{UserID: "u", Title: "Normal"})
	s2, _ := db.CreateAgentSession(&database.AgentSession{UserID: "u", Title: "Pinned"})
	pinned := true
	db.UpdateAgentSession(s2.ID, "", "", &pinned)

	sessions, _, _ := db.ListAgentSessions(database.ListAgentSessionsParams{UserID: "u"})
	if len(sessions) < 2 {
		t.Fatalf("expected at least 2 sessions, got %d", len(sessions))
	}
	_ = s1 // suppress unused
	if sessions[0].ID != s2.ID {
		t.Errorf("pinned session should come first; first = %q", sessions[0].ID)
	}
}

// ── ToolExecution list ─────────────────────────────────────────────────────────

func TestListToolExecutionsFilters(t *testing.T) {
	db := newDB(t)

	// Insert two tool execution rows directly (no agent runner needed).
	db.Exec(`INSERT INTO tool_executions (id,session_id,user_id,tool_name,arguments_json,status,started_at,created_at)
		VALUES ('e1','s1','u1','subdomain_enum','{}','completed',?,CURRENT_TIMESTAMP)`, time.Now().UTC())
	db.Exec(`INSERT INTO tool_executions (id,session_id,user_id,tool_name,arguments_json,status,started_at,created_at)
		VALUES ('e2','s1','u1','dns_lookup','{}','failed',?,CURRENT_TIMESTAMP)`, time.Now().UTC())
	db.Exec(`INSERT INTO tool_executions (id,session_id,user_id,tool_name,arguments_json,status,started_at,created_at)
		VALUES ('e3','s2','u2','http_probe','{}','completed',?,CURRENT_TIMESTAMP)`, time.Now().UTC())

	// Filter by session.
	bySession, total, err := db.ListToolExecutions(database.ListToolExecutionsParams{SessionID: "s1"})
	if err != nil {
		t.Fatalf("ListToolExecutions(s1) error: %v", err)
	}
	if total != 2 || len(bySession) != 2 {
		t.Errorf("by session: got %d, want 2", total)
	}

	// Filter by tool name.
	byTool, _, _ := db.ListToolExecutions(database.ListToolExecutionsParams{ToolName: "dns_lookup"})
	if len(byTool) != 1 || byTool[0].ToolName != "dns_lookup" {
		t.Errorf("by tool name: got %d entries", len(byTool))
	}

	// Filter by status.
	completed, _, _ := db.ListToolExecutions(database.ListToolExecutionsParams{Status: "completed"})
	if len(completed) != 2 {
		t.Errorf("by status=completed: got %d, want 2", len(completed))
	}
}

// ── AuditLog filters ────────────────────────────────────────────────────────────

func TestListAuditLogsFilters(t *testing.T) {
	db := newDB(t)

	db.WriteAuditLog(database.AuditParams{ActorID: "u1", Action: "login", Category: "auth", Result: "success"})
	db.WriteAuditLog(database.AuditParams{ActorID: "u1", Action: "tool_execute", Category: "tool", Result: "success"})
	db.WriteAuditLog(database.AuditParams{ActorID: "u2", Action: "login", Category: "auth", Result: "failure"})

	// Filter by actor.
	byActor, total, err := db.ListAuditLogs(database.ListAuditLogsParams{ActorID: "u1"})
	if err != nil {
		t.Fatalf("ListAuditLogs(u1) error: %v", err)
	}
	if total != 2 {
		t.Errorf("actor filter: got %d, want 2", total)
	}
	_ = byActor

	// Filter by category.
	byCategory, _, _ := db.ListAuditLogs(database.ListAuditLogsParams{Category: "auth"})
	if len(byCategory) != 2 {
		t.Errorf("category filter: got %d, want 2", len(byCategory))
	}

	// Filter by result.
	byResult, _, _ := db.ListAuditLogs(database.ListAuditLogsParams{Result: "failure"})
	if len(byResult) != 1 {
		t.Errorf("result filter: got %d, want 1", len(byResult))
	}
}

func TestAuditLogPurge(t *testing.T) {
	db := newDB(t)
	db.WriteAuditLog(database.AuditParams{ActorID: "u1", Action: "a", Category: "c", Result: "success"})

	// Purge with 0 days — should be a no-op.
	n, err := db.PurgeOldAuditLogs(0)
	if err != nil {
		t.Fatalf("PurgeOldAuditLogs(0) error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 deleted for 0-day retention, got %d", n)
	}

	// Purge with a large retention window — should still delete nothing.
	n, err = db.PurgeOldAuditLogs(365)
	if err != nil {
		t.Fatalf("PurgeOldAuditLogs(365) error: %v", err)
	}
	if n != 0 {
		t.Errorf("fresh records should not be purged, deleted=%d", n)
	}
}

// ── RBAC ───────────────────────────────────────────────────────────────────────

func TestRBACRoles(t *testing.T) {
	db := newDB(t)

	role, err := db.CreateRole("analyst", "Security analyst", false,
		[]string{"read:assets", "read:vulns"}, []string{"subdomain_enum"}, "require_approval")
	if err != nil {
		t.Fatalf("CreateRole() error: %v", err)
	}
	if role.ID == "" {
		t.Fatal("role.ID must not be empty")
	}

	got, err := db.GetRoleByName("analyst")
	if err != nil {
		t.Fatalf("GetRoleByName() error: %v", err)
	}
	if got == nil || got.ID != role.ID {
		t.Error("GetRoleByName() returned wrong role")
	}
	if len(got.Permissions) != 2 {
		t.Errorf("permissions = %v, want 2", got.Permissions)
	}
}

func TestRBACDuplicateRoleName(t *testing.T) {
	db := newDB(t)
	if _, err := db.CreateRole("ops", "", false, nil, nil, "auto"); err != nil {
		t.Fatalf("first CreateRole() error: %v", err)
	}
	_, err := db.CreateRole("ops", "", false, nil, nil, "auto")
	if err == nil {
		t.Error("duplicate role name should return error")
	}
}

func TestAssignAndRevokeRole(t *testing.T) {
	db := newDB(t)

	user, _ := db.CreateUser("charlie", "hash", "", "")
	role, _ := db.CreateRole("operator", "", false, nil, nil, "auto")

	if err := db.AssignRole(user.ID, role.ID, "test"); err != nil {
		t.Fatalf("AssignRole() error: %v", err)
	}

	roles, err := db.GetUserRoles(user.ID)
	if err != nil {
		t.Fatalf("GetUserRoles() error: %v", err)
	}
	if len(roles) != 1 || roles[0].ID != role.ID {
		t.Errorf("user should have 1 role, got %d", len(roles))
	}

	if err := db.RevokeRole(user.ID, role.ID); err != nil {
		t.Fatalf("RevokeRole() error: %v", err)
	}

	roles, _ = db.GetUserRoles(user.ID)
	if len(roles) != 0 {
		t.Errorf("user should have 0 roles after revoke, got %d", len(roles))
	}
}

// ── Vulnerability CRUD ────────────────────────────────────────────────────────

func TestVulnerabilityLifecycle(t *testing.T) {
	db := newDB(t)

	v, err := db.CreateVulnerability(&database.Vulnerability{
		Title:    "Open Redirect",
		Severity: "medium",
		Target:   "https://example.com/redirect",
	})
	if err != nil {
		t.Fatalf("CreateVulnerability() error: %v", err)
	}
	if v.Status != "open" {
		t.Errorf("default status = %q, want open", v.Status)
	}

	// Update lifecycle status.
	if err := db.UpdateVulnerability(v.ID, "", "fixed", "", ""); err != nil {
		t.Fatalf("UpdateVulnerability() error: %v", err)
	}

	updated, _ := db.GetVulnerabilityByID(v.ID)
	if updated == nil || updated.Status != "fixed" {
		t.Errorf("status after update = %q, want fixed", updated.Status)
	}
}

func TestListVulnerabilitiesFilters(t *testing.T) {
	db := newDB(t)

	db.CreateVulnerability(&database.Vulnerability{Title: "SQLi",   Severity: "critical", Target: "t1"})
	db.CreateVulnerability(&database.Vulnerability{Title: "XSS",    Severity: "medium",   Target: "t2"})
	db.CreateVulnerability(&database.Vulnerability{Title: "SSRF",   Severity: "high",     Target: "t3"})

	critical, total, err := db.ListVulnerabilities(database.ListVulnsParams{Severity: "critical"})
	if err != nil {
		t.Fatalf("ListVulnerabilities(critical) error: %v", err)
	}
	if total != 1 || len(critical) != 1 {
		t.Errorf("critical filter: got %d, want 1", total)
	}

	all, allTotal, _ := db.ListVulnerabilities(database.ListVulnsParams{Limit: 10})
	if allTotal != 3 || len(all) != 3 {
		t.Errorf("all vulns: got %d, want 3", allTotal)
	}
}

// ── Orphan cleanup ────────────────────────────────────────────────────────────

func TestCancelOrphanedRunningToolExecutions(t *testing.T) {
	db := newDB(t)

	// Insert a "running" execution that started before now.
	past := time.Now().Add(-10 * time.Second)
	db.Exec(`INSERT INTO tool_executions (id,session_id,user_id,tool_name,arguments_json,status,started_at,created_at)
		VALUES ('orphan','s1','u1','subdomain_enum','{}','running',?,CURRENT_TIMESTAMP)`, past)

	// Insert a "running" execution that just started (should NOT be cancelled).
	future := time.Now().Add(10 * time.Second)
	db.Exec(`INSERT INTO tool_executions (id,session_id,user_id,tool_name,arguments_json,status,started_at,created_at)
		VALUES ('recent','s1','u1','subdomain_enum','{}','running',?,CURRENT_TIMESTAMP)`, future)

	n, err := db.CancelOrphanedRunningToolExecutions(time.Now(), "server_restart")
	if err != nil {
		t.Fatalf("CancelOrphanedRunningToolExecutions() error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 orphan cancelled, got %d", n)
	}

	var status string
	db.QueryRow(`SELECT status FROM tool_executions WHERE id='orphan'`).Scan(&status)
	if status != "orphaned" && status != "cancelled" {
		t.Errorf("orphan status = %q, want orphaned or cancelled", status)
	}

	var recentStatus string
	db.QueryRow(`SELECT status FROM tool_executions WHERE id='recent'`).Scan(&recentStatus)
	if recentStatus != "running" {
		t.Errorf("recent execution should still be running, got %q", recentStatus)
	}
}
