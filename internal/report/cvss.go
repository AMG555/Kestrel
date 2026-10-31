package report

import (
	"fmt"
	"math"
	"strings"
)

// CVSSMetrics holds parsed CVSS v3.1 base metric values.
type CVSSMetrics struct {
	AV string // N, A, L, P
	AC string // L, H
	PR string // N, L, H
	UI string // N, R
	S  string // U, C
	C  string // N, L, H
	I  string // N, L, H
	A  string // N, L, H
}

// CVSSResult holds calculated CVSS v3.1 score, severity, and vector.
type CVSSResult struct {
	Score    float64     `json:"score"`
	Severity string      `json:"severity"` // None, Low, Medium, High, Critical
	Vector   string      `json:"vector"`
	Metrics  CVSSMetrics `json:"metrics"`
}

// RoundUp implements the FIRST.org CVSS v3.1 formal rounding algorithm.
// The algorithm returns the smallest number with 1 decimal place >= input.
func RoundUp(val float64) float64 {
	if val <= 0.0 {
		return 0.0
	}
	if val >= 10.0 {
		return 10.0
	}
	valInt := int(math.Round(val * 100000.0))
	if valInt%10000 == 0 {
		return float64(valInt) / 100000.0
	}
	return (float64(valInt/10000) + 1.0) / 10.0
}

// CalculateCVSS computes the CVSS v3.1 Base Score from metrics.
func CalculateCVSS(m CVSSMetrics) CVSSResult {
	// Normalize
	av := strings.ToUpper(strings.TrimSpace(m.AV))
	ac := strings.ToUpper(strings.TrimSpace(m.AC))
	pr := strings.ToUpper(strings.TrimSpace(m.PR))
	ui := strings.ToUpper(strings.TrimSpace(m.UI))
	s := strings.ToUpper(strings.TrimSpace(m.S))
	c := strings.ToUpper(strings.TrimSpace(m.C))
	i := strings.ToUpper(strings.TrimSpace(m.I))
	a := strings.ToUpper(strings.TrimSpace(m.A))

	if av == "" {
		av = "N"
	}
	if ac == "" {
		ac = "L"
	}
	if pr == "" {
		pr = "N"
	}
	if ui == "" {
		ui = "N"
	}
	if s == "" {
		s = "U"
	}
	if c == "" {
		c = "H"
	}
	if i == "" {
		i = "H"
	}
	if a == "" {
		a = "H"
	}

	norm := CVSSMetrics{AV: av, AC: ac, PR: pr, UI: ui, S: s, C: c, I: i, A: a}

	// Attack Vector weights
	avWeight := 0.85
	switch av {
	case "A":
		avWeight = 0.62
	case "L":
		avWeight = 0.55
	case "P":
		avWeight = 0.20
	}

	// Attack Complexity weights
	acWeight := 0.77
	if ac == "H" {
		acWeight = 0.44
	}

	// Privileges Required weights depends on Scope
	prWeight := 0.85
	if s == "C" {
		switch pr {
		case "L":
			prWeight = 0.68
		case "H":
			prWeight = 0.50
		}
	} else {
		switch pr {
		case "L":
			prWeight = 0.62
		case "H":
			prWeight = 0.27
		}
	}

	// User Interaction weights
	uiWeight := 0.85
	if ui == "R" {
		uiWeight = 0.62
	}

	// Impact metric weights
	getImpactWeight := func(val string) float64 {
		switch val {
		case "H":
			return 0.56
		case "L":
			return 0.22
		default:
			return 0.0
		}
	}

	cWeight := getImpactWeight(c)
	iWeight := getImpactWeight(i)
	aWeight := getImpactWeight(a)

	iss := 1.0 - ((1.0 - cWeight) * (1.0 - iWeight) * (1.0 - aWeight))

	var impact float64
	if s == "U" {
		impact = 6.42 * iss
	} else {
		impact = 7.52*(iss-0.029) - 3.25*math.Pow(iss-0.02, 15)
	}

	exploitability := 8.22 * avWeight * acWeight * prWeight * uiWeight

	var score float64
	if impact <= 0.0 {
		score = 0.0
	} else {
		if s == "U" {
			score = RoundUp(math.Min(impact+exploitability, 10.0))
		} else {
			score = RoundUp(math.Min(1.08*(impact+exploitability), 10.0))
		}
	}

	vector := fmt.Sprintf("CVSS:3.1/AV:%s/AC:%s/PR:%s/UI:%s/S:%s/C:%s/I:%s/A:%s",
		av, ac, pr, ui, s, c, i, a)

	severity := SeverityFromScore(score)

	return CVSSResult{
		Score:    score,
		Severity: severity,
		Vector:   vector,
		Metrics:  norm,
	}
}

// SeverityFromScore returns the CVSS v3.1 severity rating from a numeric score.
func SeverityFromScore(score float64) string {
	switch {
	case score >= 9.0:
		return "critical"
	case score >= 7.0:
		return "high"
	case score >= 4.0:
		return "medium"
	case score > 0.0:
		return "low"
	default:
		return "none"
	}
}

// ParseCVSSVector parses a CVSS:3.1 or CVSS:3.0 vector string into CVSSMetrics.
func ParseCVSSVector(vector string) (CVSSMetrics, error) {
	var m CVSSMetrics
	parts := strings.Split(strings.TrimSpace(vector), "/")
	if len(parts) < 2 {
		return m, fmt.Errorf("invalid CVSS vector format: %s", vector)
	}

	prefix := strings.ToUpper(parts[0])
	if prefix != "CVSS:3.1" && prefix != "CVSS:3.0" {
		return m, fmt.Errorf("unsupported CVSS version: %s (expected CVSS:3.1)", parts[0])
	}

	for _, p := range parts[1:] {
		kv := strings.SplitN(p, ":", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.ToUpper(strings.TrimSpace(kv[0]))
		v := strings.ToUpper(strings.TrimSpace(kv[1]))

		switch k {
		case "AV":
			m.AV = v
		case "AC":
			m.AC = v
		case "PR":
			m.PR = v
		case "UI":
			m.UI = v
		case "S":
			m.S = v
		case "C":
			m.C = v
		case "I":
			m.I = v
		case "A":
			m.A = v
		}
	}

	return m, nil
}

// DefaultCVSSForSeverity generates a canonical CVSS v3.1 vector and score corresponding to a given severity text.
func DefaultCVSSForSeverity(severity string) CVSSResult {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		// Remote RCE, unauthenticated, full compromise: 9.8
		return CalculateCVSS(CVSSMetrics{
			AV: "N", AC: "L", PR: "N", UI: "N", S: "U", C: "H", I: "H", A: "H",
		})
	case "high":
		// Remote authenticated or high impact without full scope change: 7.5
		return CalculateCVSS(CVSSMetrics{
			AV: "N", AC: "L", PR: "N", UI: "N", S: "U", C: "H", I: "N", A: "N",
		})
	case "medium":
		// User interaction required, medium impact (e.g. Reflected XSS): 6.1
		return CalculateCVSS(CVSSMetrics{
			AV: "N", AC: "L", PR: "N", UI: "R", S: "C", C: "L", I: "L", A: "N",
		})
	case "low":
		// Minor info disclosure or hardening issue: 3.1
		return CalculateCVSS(CVSSMetrics{
			AV: "N", AC: "H", PR: "L", UI: "N", S: "U", C: "L", I: "N", A: "N",
		})
	default:
		// Info / None: 0.0
		return CalculateCVSS(CVSSMetrics{
			AV: "N", AC: "L", PR: "N", UI: "N", S: "U", C: "N", I: "N", A: "N",
		})
	}
}
