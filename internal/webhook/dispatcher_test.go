package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"kestrel/internal/database"
)

func TestShouldAlert(t *testing.T) {
	cases := []struct {
		finding  string
		min      string
		expected bool
	}{
		{"critical", "high", true},
		{"high", "high", true},
		{"medium", "high", false},
		{"low", "medium", false},
		{"critical", "low", true},
		{"info", "info", true},
	}

	for _, tc := range cases {
		got := ShouldAlert(tc.finding, tc.min)
		if got != tc.expected {
			t.Errorf("ShouldAlert(%q, %q) = %v; want %v", tc.finding, tc.min, got, tc.expected)
		}
	}
}

func TestDispatchVulnerabilityAlert_StandardPayload(t *testing.T) {
	var receivedPayload Payload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		if err := json.Unmarshal(body, &receivedPayload); err != nil {
			t.Fatalf("failed to parse json: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	vuln := &database.Vulnerability{
		ID:             "v-alert-1",
		Title:          "RCE via Template Injection",
		Severity:       "critical",
		Type:           "rce",
		Target:         "http://victim.corp/eval",
		Impact:         "Full system takeover",
		Recommendation: "Sanitize user input",
	}

	err := DispatchVulnerabilityAlert(context.Background(), server.URL, "high", vuln)
	if err != nil {
		t.Fatalf("unexpected error dispatching webhook: %v", err)
	}

	if receivedPayload.Event != "vulnerability.detected" {
		t.Errorf("expected event 'vulnerability.detected', got %q", receivedPayload.Event)
	}
	if receivedPayload.Vulnerability.Title != "RCE via Template Injection" {
		t.Errorf("expected title 'RCE via Template Injection', got %q", receivedPayload.Vulnerability.Title)
	}
	if receivedPayload.Vulnerability.Severity != "critical" {
		t.Errorf("expected severity 'critical', got %q", receivedPayload.Vulnerability.Severity)
	}
}

func TestDispatchVulnerabilityAlert_BelowThreshold(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	vuln := &database.Vulnerability{
		ID:       "v-alert-low",
		Title:    "Minor Header Missing",
		Severity: "low",
	}

	err := DispatchVulnerabilityAlert(context.Background(), server.URL, "high", vuln)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 0 {
		t.Errorf("expected 0 calls for below-threshold finding, got %d", calls)
	}
}
