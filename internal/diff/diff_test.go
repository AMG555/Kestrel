package diff

import (
	"strings"
	"testing"

	"kestrel/internal/database"
)

func TestComputeDiff_Scenarios(t *testing.T) {
	baseVulns := []*database.Vulnerability{
		{
			ID:          "v-base-1",
			Title:       "SQL Injection in Search",
			Severity:    "high",
			Status:      "fixed", // was fixed in base!
			Target:      "https://target.corp/search",
			Fingerprint: "fp-sqli",
		},
		{
			ID:          "v-base-2",
			Title:       "Reflected XSS",
			Severity:    "medium",
			Status:      "confirmed",
			Target:      "https://target.corp/profile",
			Fingerprint: "fp-xss",
		},
		{
			ID:          "v-base-3",
			Title:       "Information Disclosure via Headers",
			Severity:    "low",
			Status:      "confirmed",
			Target:      "https://target.corp/",
			Fingerprint: "fp-info",
		},
	}

	targetVulns := []*database.Vulnerability{
		{
			ID:          "v-target-1",
			Title:       "SQL Injection in Search",
			Severity:    "high",
			Status:      "suspected", // Reappeared! Regression
			Target:      "https://target.corp/search",
			Fingerprint: "fp-sqli",
		},
		{
			ID:          "v-target-2",
			Title:       "Reflected XSS",
			Severity:    "medium",
			Status:      "confirmed", // Still active! Persistent
			Target:      "https://target.corp/profile",
			Fingerprint: "fp-xss",
		},
		{
			ID:          "v-target-4",
			Title:       "Remote Code Execution via Spring4Shell",
			Severity:    "critical",
			Status:      "suspected", // Brand new!
			Target:      "https://target.corp/api/helloworld",
			CVE:         "CVE-2022-22965",
			CVSSScore:   9.8,
			Fingerprint: "fp-rce",
		},
		// Notice "Information Disclosure via Headers" (fp-info) is NOT in targetVulns -> Fixed!
	}

	res := ComputeDiff(baseVulns, targetVulns, "Q1 Assessment", "Q2 Assessment")

	if res.Summary.TotalBase != 3 {
		t.Errorf("TotalBase = %d, want 3", res.Summary.TotalBase)
	}
	if res.Summary.TotalTarget != 3 {
		t.Errorf("TotalTarget = %d, want 3", res.Summary.TotalTarget)
	}
	if res.Summary.RegressedCount != 1 {
		t.Errorf("RegressedCount = %d, want 1", res.Summary.RegressedCount)
	}
	if res.Summary.NewCount != 1 {
		t.Errorf("NewCount = %d, want 1", res.Summary.NewCount)
	}
	if res.Summary.FixedCount != 1 {
		t.Errorf("FixedCount = %d, want 1", res.Summary.FixedCount)
	}
	if res.Summary.PersistentCount != 1 {
		t.Errorf("PersistentCount = %d, want 1", res.Summary.PersistentCount)
	}

	if len(res.Regressed) == 0 || res.Regressed[0].Fingerprint != "fp-sqli" {
		t.Errorf("expected SQLi in Regressed, got %+v", res.Regressed)
	}
	if len(res.New) == 0 || res.New[0].Fingerprint != "fp-rce" {
		t.Errorf("expected RCE in New, got %+v", res.New)
	}
	if len(res.Fixed) == 0 || res.Fixed[0].Fingerprint != "fp-info" {
		t.Errorf("expected info disclosure in Fixed, got %+v", res.Fixed)
	}
	if len(res.Persistent) == 0 || res.Persistent[0].Fingerprint != "fp-xss" {
		t.Errorf("expected XSS in Persistent, got %+v", res.Persistent)
	}

	// Verify Markdown output
	md := RenderMarkdown(res)
	if !strings.Contains(md, "Scan Assessment Run Diff: Q1 Assessment vs Q2 Assessment") {
		t.Errorf("missing title in markdown")
	}
	if !strings.Contains(md, "Regressed") || !strings.Contains(md, "Spring4Shell") {
		t.Errorf("missing sections in markdown")
	}

	// Verify CLI output
	cli := RenderCLI(res)
	if !strings.Contains(cli, "Regressed (⚠️) : 1") || !strings.Contains(cli, "New       (🔴) : 1") {
		t.Errorf("unexpected CLI output:\n%s", cli)
	}

	// Verify JSON output
	jsonBytes, err := RenderJSON(res)
	if err != nil || !strings.Contains(string(jsonBytes), `"regressed_count": 1`) {
		t.Errorf("unexpected JSON output: %v, %s", err, string(jsonBytes))
	}
}
