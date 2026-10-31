package integration_test

import (
	"bytes"
	"encoding/json"
	"kestrel/internal/app"
	"kestrel/internal/config"
	"kestrel/internal/database"
	"kestrel/internal/logger"
	"kestrel/internal/security"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestIntegration_ServerBootLoginAndAuditContinuity(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "integration.db")

	// 1. Prepare minimal valid configuration
	cfg := &config.Config{}
	cfg.Database.Path = dbPath
	cfg.Auth.JWTSecret = "test-integration-secret-32-bytes-long!"
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 0
	auditEnabled := true
	cfg.Audit.Enabled = &auditEnabled
	cfg.Audit.RetentionDays = 30

	log := logger.New("error", "")

	configFile := filepath.Join(tempDir, "config.yaml")
	_ = os.WriteFile(configFile, []byte(""), 0644)

	// 2. Boot App
	application, err := app.New(cfg, log, configFile)
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skip("skipping in CGO_ENABLED=0 environment")
		}
		t.Fatalf("app.New failed: %v", err)
	}
	defer application.Shutdown()

	db := application.DB()
	if db == nil {
		t.Fatal("expected application DB to be non-nil")
	}

	// 3. Set known admin password
	testPassword := "IntegrationAdminP@ss123!"
	pHash, err := security.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if err := db.UpdateRBACAdminPassword(pHash); err != nil {
		t.Fatalf("UpdateRBACAdminPassword failed: %v", err)
	}

	// 4. Test Login via HTTP handler (verifies query column/dest scan arity)
	router := application.Router()
	if router == nil {
		t.Fatal("expected application Router to be non-nil")
	}

	loginPayload, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": testPassword,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("login failed: HTTP %d, body: %s", w.Code, w.Body.String())
	}

	var loginResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("failed to parse login response: %v", err)
	}
	token := loginResp.Token
	if token == "" {
		t.Fatalf("login response returned empty token: %s", w.Body.String())
	}

	// 5. Test Authenticated Route: /api/auth/me
	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+token)
	meW := httptest.NewRecorder()
	router.ServeHTTP(meW, meReq)

	if meW.Code != http.StatusOK {
		t.Fatalf("/api/auth/me failed: HTTP %d, body: %s", meW.Code, meW.Body.String())
	}

	// 6. Test Authenticated Project Creation
	projPayload, _ := json.Marshal(map[string]interface{}{
		"name":        "Client Pentest Alpha",
		"description": "Scope verification engagement",
	})
	projReq := httptest.NewRequest(http.MethodPost, "/api/projects", bytes.NewReader(projPayload))
	projReq.Header.Set("Authorization", "Bearer "+token)
	projReq.Header.Set("Content-Type", "application/json")
	projW := httptest.NewRecorder()
	router.ServeHTTP(projW, projReq)

	if projW.Code != http.StatusOK && projW.Code != http.StatusCreated {
		t.Fatalf("project creation failed: HTTP %d, body: %s", projW.Code, projW.Body.String())
	}

	// 7. Append audit log and assert chain verification + external checkpoint
	if err := db.AppendAuditLog(&database.AuditLog{
		ID:        "audit-integration-1",
		CreatedAt: time.Now().UTC(),
		Level:     "info",
		Category:  "engagement",
		Action:    "create_project",
		Result:    "success",
		Actor:     "admin",
		Message:   "Client Pentest Alpha created",
	}); err != nil {
		t.Fatalf("AppendAuditLog failed: %v", err)
	}

	valid, err := db.VerifyAuditLogChain()
	if err != nil || !valid {
		t.Fatalf("VerifyAuditLogChain failed: valid=%v, err=%v", valid, err)
	}

	// 8. Create external checkpoint receipt & verify against it
	cp, err := db.CreateAuditCheckpoint()
	if err != nil {
		t.Fatalf("CreateAuditCheckpoint failed: %v", err)
	}
	if cp.HeadHash == "" || cp.Seal == "" {
		t.Fatalf("invalid checkpoint produced: %+v", cp)
	}

	cpValid, err := db.VerifyAuditCheckpoint(cp)
	if err != nil || !cpValid {
		t.Fatalf("VerifyAuditCheckpoint failed: valid=%v, err=%v", cpValid, err)
	}
}

func TestIntegration_RBACScanArityIntegrity(t *testing.T) {
	// Directly tests database scan methods to ensure SQL columns match destinations
	tempDB := filepath.Join(t.TempDir(), "arity_check.db")
	db, err := database.NewDB(tempDB, zap.NewNop())
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skip("skipping in CGO_ENABLED=0 environment")
		}
		t.Fatalf("NewDB failed: %v", err)
	}
	defer db.Close()

	authMgr := security.NewAuthManager(24, "test-secret-key-for-rbac-check")
	if _, err := authMgr.AttachRBACStore(db); err != nil {
		t.Fatalf("AttachRBACStore failed: %v", err)
	}

	// 1. GetRBACUserByUsername
	user, err := db.GetRBACUserByUsername("admin")
	if err != nil {
		t.Fatalf("GetRBACUserByUsername arity error: %v", err)
	}
	if user == nil || user.Username != "admin" {
		t.Fatalf("expected admin user, got %+v", user)
	}

	// 2. GetRBACUserByID
	userByID, err := db.GetRBACUserByID("admin")
	if err != nil {
		t.Fatalf("GetRBACUserByID arity error: %v", err)
	}
	if userByID == nil || userByID.ID != "admin" {
		t.Fatalf("expected admin user by ID, got %+v", userByID)
	}

	// 3. ListRBACUsers
	users, err := db.ListRBACUsers()
	if err != nil {
		t.Fatalf("ListRBACUsers scan error: %v", err)
	}
	if len(users) == 0 {
		t.Fatal("expected at least admin user in ListRBACUsers")
	}

	// 4. ListAuditLogs
	filter := database.ListAuditLogsFilter{Limit: 10}
	logs, err := db.ListAuditLogs(filter)
	if err != nil {
		t.Fatalf("ListAuditLogs scan error: %v", err)
	}
	_ = logs
}
