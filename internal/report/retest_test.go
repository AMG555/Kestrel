package report

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"kestrel/internal/database"
)

func TestRunVulnerabilityRetest_Resolved(t *testing.T) {
	// Server returns clean response (indicator NOT present)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Welcome! User profile updated securely.")
	}))
	defer server.Close()

	vuln := &database.Vulnerability{
		ID:         "v-retest-fixed",
		Title:      "SQL Injection in Search",
		Severity:   "high",
		Target:     server.URL + "/search",
		ReproSteps: "POST /search HTTP/1.1\n\n' OR '1'='1",
		Evidence:   "SQL syntax error near OR",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := RunVulnerabilityRetest(ctx, vuln, server.URL)
	if err != nil {
		t.Fatalf("unexpected error running retest: %v", err)
	}

	if !result.IsResolved {
		t.Errorf("expected IsResolved true for clean target, got false (output: %s)", result.Output)
	}
	if result.Status != "fixed" {
		t.Errorf("expected Status 'fixed', got %q", result.Status)
	}
}

func TestRunVulnerabilityRetest_StillVulnerable(t *testing.T) {
	// Server returns exploit indicator
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, "Database error: SQL syntax error near OR")
	}))
	defer server.Close()

	vuln := &database.Vulnerability{
		ID:         "v-retest-vuln",
		Title:      "SQL Injection in Search",
		Severity:   "high",
		Target:     server.URL + "/search",
		ReproSteps: "POST /search HTTP/1.1\n\n' OR '1'='1",
		Evidence:   "SQL syntax error near OR",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := RunVulnerabilityRetest(ctx, vuln, server.URL)
	if err != nil {
		t.Fatalf("unexpected error running retest: %v", err)
	}

	if result.IsResolved {
		t.Errorf("expected IsResolved false when indicator triggered, got true")
	}
	if result.Status != "open" {
		t.Errorf("expected Status 'open', got %q", result.Status)
	}
}
