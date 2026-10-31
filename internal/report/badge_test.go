package report

import (
	"strings"
	"testing"
)

func TestGenerateBadgeSVG(t *testing.T) {
	// Critical badge
	svg := string(GenerateBadgeSVG(2, 0, 0, 0))
	if !strings.Contains(svg, "2 critical") {
		t.Fatalf("expected '2 critical' in badge SVG, got:\n%s", svg)
	}
	if !strings.Contains(svg, "#e11d48") {
		t.Errorf("expected red background #e11d48")
	}

	// High badge
	svg = string(GenerateBadgeSVG(0, 5, 0, 0))
	if !strings.Contains(svg, "5 high") {
		t.Fatalf("expected '5 high' in badge SVG, got:\n%s", svg)
	}
	if !strings.Contains(svg, "#f97316") {
		t.Errorf("expected orange background #f97316")
	}

	// Passing / 0 vulnerabilities badge
	svg = string(GenerateBadgeSVG(0, 0, 0, 0))
	if !strings.Contains(svg, "0 vulnerabilities") {
		t.Fatalf("expected '0 vulnerabilities' in badge SVG, got:\n%s", svg)
	}
	if !strings.Contains(svg, "#10b981") {
		t.Errorf("expected green background #10b981")
	}
}
