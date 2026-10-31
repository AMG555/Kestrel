package initcmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kestrel/internal/config"
	"kestrel/internal/database"
	"kestrel/internal/initcmd"
	"kestrel/internal/toolguard"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

func TestRunEngagementInit(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_init.db")

	cfg := &config.Config{}
	cfg.Database.Path = dbPath

	opts := initcmd.InitOptions{
		Name:   "Client Alpha Pentest",
		Target: "https://api.client.corp",
		OutDir: tempDir,
		Actor:  "alice",
	}

	res, err := initcmd.RunEngagementInit(cfg, opts)
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skip("skipping in CGO_ENABLED=0 environment")
		}
		t.Fatalf("RunEngagementInit failed: %v", err)
	}

	if res.ProjectID == "" {
		t.Fatal("expected non-empty project ID")
	}

	// 1. Verify scope.yaml was generated and compiles
	policy, err := toolguard.LoadScopeFile(res.ScopeFile)
	if err != nil {
		t.Fatalf("LoadScopeFile failed on generated scope: %v", err)
	}
	if policy.TargetName != "Client Alpha Pentest" {
		t.Errorf("policy TargetName = %q, want 'Client Alpha Pentest'", policy.TargetName)
	}
	if len(policy.InScopeDomains) == 0 {
		t.Errorf("expected in-scope domains for api.client.corp, got %v", policy.InScopeDomains)
	}

	// 2. Verify audit-checkpoint.json exists and verifies
	db, err := database.NewDB(dbPath, zap.NewNop())
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	valid, err := db.VerifyAuditCheckpoint(res.Checkpoint)
	if err != nil || !valid {
		t.Fatalf("VerifyAuditCheckpoint failed: valid=%v, err=%v", valid, err)
	}

	// 3. Verify scope file content options
	data, err := os.ReadFile(res.ScopeFile)
	if err != nil {
		t.Fatalf("read scope.yaml: %v", err)
	}
	var scopeDoc toolguard.ScopeConfigFile
	if err := yaml.Unmarshal(data, &scopeDoc); err != nil {
		t.Fatalf("unmarshal scope.yaml: %v", err)
	}
	if scopeDoc.Options.FailClosed == nil || !*scopeDoc.Options.FailClosed {
		t.Error("expected fail_closed=true in generated scope")
	}
	if scopeDoc.Options.BlockCloudMetadata == nil || !*scopeDoc.Options.BlockCloudMetadata {
		t.Error("expected block_cloud_metadata=true in generated scope")
	}
}

