package report

import (
	"testing"
)

func TestCalculateCVSS_KnownVectors(t *testing.T) {
	tests := []struct {
		name         string
		vector       string
		expectedBase float64
		expectedSev  string
	}{
		{
			name:         "Log4j / Remote RCE Scope Changed (CVE-2021-44228)",
			vector:       "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H",
			expectedBase: 10.0,
			expectedSev:  "critical",
		},
		{
			name:         "Heartbleed (CVE-2014-0160)",
			vector:       "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N",
			expectedBase: 7.5,
			expectedSev:  "high",
		},
		{
			name:         "Stored XSS with User Interaction",
			vector:       "CVSS:3.1/AV:N/AC:L/PR:L/UI:R/S:C/C:L/I:L/A:N",
			expectedBase: 5.4,
			expectedSev:  "medium",
		},
		{
			name:         "Local Privilege Escalation",
			vector:       "CVSS:3.1/AV:L/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:H",
			expectedBase: 7.8,
			expectedSev:  "high",
		},
		{
			name:         "Zero Impact None",
			vector:       "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:N",
			expectedBase: 0.0,
			expectedSev:  "none",
		},
		{
			name:         "Physical Access Only",
			vector:       "CVSS:3.1/AV:P/AC:H/PR:H/UI:R/S:U/C:L/I:N/A:N",
			expectedBase: 1.6,
			expectedSev:  "low",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			metrics, err := ParseCVSSVector(tc.vector)
			if err != nil {
				t.Fatalf("ParseCVSSVector failed: %v", err)
			}

			res := CalculateCVSS(metrics)
			if res.Score != tc.expectedBase {
				t.Errorf("Score = %v, expected %v", res.Score, tc.expectedBase)
			}
			if res.Severity != tc.expectedSev {
				t.Errorf("Severity = %s, expected %s", res.Severity, tc.expectedSev)
			}
			if res.Vector != tc.vector {
				t.Errorf("Vector = %s, expected %s", res.Vector, tc.vector)
			}
		})
	}
}

func TestDefaultCVSSForSeverity(t *testing.T) {
	cases := []struct {
		sev      string
		minScore float64
		maxScore float64
	}{
		{"critical", 9.0, 10.0},
		{"high", 7.0, 8.9},
		{"medium", 4.0, 6.9},
		{"low", 0.1, 3.9},
		{"info", 0.0, 0.0},
	}

	for _, c := range cases {
		t.Run(c.sev, func(t *testing.T) {
			res := DefaultCVSSForSeverity(c.sev)
			if res.Score < c.minScore || res.Score > c.maxScore {
				t.Errorf("DefaultCVSSForSeverity(%s) score %v outside expected range [%v, %v]",
					c.sev, res.Score, c.minScore, c.maxScore)
			}
			if res.Vector == "" {
				t.Errorf("Vector should not be empty")
			}
		})
	}
}

func TestRoundUpEdgeCases(t *testing.T) {
	if RoundUp(0.0) != 0.0 {
		t.Errorf("RoundUp(0.0) expected 0.0, got %v", RoundUp(0.0))
	}
	if RoundUp(10.0) != 10.0 {
		t.Errorf("RoundUp(10.0) expected 10.0, got %v", RoundUp(10.0))
	}
	if RoundUp(4.02) != 4.1 {
		t.Errorf("RoundUp(4.02) expected 4.1, got %v", RoundUp(4.02))
	}
	if RoundUp(4.00) != 4.0 {
		t.Errorf("RoundUp(4.00) expected 4.0, got %v", RoundUp(4.00))
	}
}
