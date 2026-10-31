package demo

import (
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"kestrel/internal/database"
)

func TestSeedDemoProject(t *testing.T) {
	tempDB := filepath.Join(t.TempDir(), "demo_test.db")
	db, err := database.NewDB(tempDB, zap.NewNop())
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skip("skipping test in CGO_ENABLED=0 environment")
		}
		t.Fatalf("failed to create test db: %v", err)
	}
	defer db.Close()

	secret := []byte("super-secret-jwt-key-for-testing-only-1234")
	details, err := SeedDemoProject(db, secret, "http://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if details == nil {
		t.Fatal("expected non-nil demo details")
	}
	if details.ProjectID != "proj-demo-juiceshop" {
		t.Errorf("expected project ID 'proj-demo-juiceshop', got %s", details.ProjectID)
	}
	if details.AdminToken == "" {
		t.Error("expected non-empty admin token")
	}
	if !strings.HasPrefix(details.DirectLoginURL, "http://127.0.0.1:8080/?token=") {
		t.Errorf("unexpected direct login URL: %s", details.DirectLoginURL)
	}

	// Verify project exists in DB
	proj, err := db.GetProject("proj-demo-juiceshop")
	if err != nil || proj == nil {
		t.Fatalf("expected project to exist in db: %v", err)
	}

	// Verify vulnerabilities exist
	vulns, err := db.ListVulnerabilities(50, 0, database.VulnerabilityListFilter{ProjectID: "proj-demo-juiceshop"})
	if err != nil || len(vulns) < 3 {
		t.Fatalf("expected at least 3 vulnerabilities, got %d (err: %v)", len(vulns), err)
	}
}
