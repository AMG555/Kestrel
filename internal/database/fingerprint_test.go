package database

import (
	"testing"
)

func TestNormalizeTargetForFingerprint(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		expected string
	}{
		{
			name:     "Strip token and session query params",
			raw:      "https://example.com/api/users?token=abc&id=1&session=xyz",
			expected: "example.com/api/users?id",
		},
		{
			name:     "Normalize uppercase host and default path",
			raw:      "HTTP://EXAMPLE.COM",
			expected: "example.com/",
		},
		{
			name:     "Sort query parameters alphabetically for deterministic matching",
			raw:      "http://api.target.com/search?z=last&a=first&m=mid",
			expected: "api.target.com/search?a&m&z",
		},
		{
			name:     "Raw domain fallback",
			raw:      "target.corp/admin/",
			expected: "target.corp/admin",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizeTargetForFingerprint(c.raw)
			if got != c.expected {
				t.Errorf("NormalizeTargetForFingerprint(%q) = %q, want %q", c.raw, got, c.expected)
			}
		})
	}
}

func TestComputeFindingFingerprint(t *testing.T) {
	v1 := &Vulnerability{
		Title:  "SQL Injection in Search",
		Type:   "sqli",
		Target: "https://example.com/search?q=test&session=123",
	}

	v2 := &Vulnerability{
		Title:  "SQL Injection in Search",
		Type:   "sqli",
		Target: "https://example.com/search?session=456&q=test", // different session, same vulnerability
	}

	v3 := &Vulnerability{
		Title:  "XSS in Search",
		Type:   "xss",
		Target: "https://example.com/search?q=test",
	}

	fp1 := ComputeFindingFingerprint(v1)
	fp2 := ComputeFindingFingerprint(v2)
	fp3 := ComputeFindingFingerprint(v3)

	if fp1 == "" || fp2 == "" || fp3 == "" {
		t.Fatalf("fingerprints must not be empty")
	}

	if fp1 != fp2 {
		t.Errorf("v1 and v2 should produce identical fingerprint, got %s != %s", fp1, fp2)
	}

	if fp1 == fp3 {
		t.Errorf("v1 (sqli) and v3 (xss) should produce different fingerprints, got collision %s", fp1)
	}
}
