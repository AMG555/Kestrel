package report

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"kestrel/internal/database"
)

const (
	// SARIFVersion defines the supported OASIS SARIF version
	SARIFVersion = "2.1.0"
	// SARIFSchemaURL defines the canonical schema URI for validation
	SARIFSchemaURL = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json"
	// ToolName defines the analyzer driver name
	ToolName = "Kestrel"
	// ToolInformationURI links to the repository
	ToolInformationURI = "https://github.com/AMG555/Kestrel"
	// DefaultToolVersion is the fallback if build version is unset
	DefaultToolVersion = "1.0.0"
)

// SARIFReport represents the root container of an OASIS SARIF 2.1.0 log
type SARIFReport struct {
	Schema  string `json:"$schema"`
	Version string `json:"version"`
	Runs    []Run  `json:"runs"`
}

// Run describes a single invocation of the analysis tool
type Run struct {
	Tool    Tool     `json:"tool"`
	Results []Result `json:"results"`
}

// Tool describes the analysis tool that produced the run
type Tool struct {
	Driver Driver `json:"driver"`
}

// Driver represents the primary tool component
type Driver struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	InformationURI string `json:"informationUri,omitempty"`
	Rules          []Rule `json:"rules"`
}

// Rule describes a vulnerability class or inspection rule
type Rule struct {
	ID                   string                  `json:"id"`
	Name                 string                  `json:"name,omitempty"`
	ShortDescription     *MultiformatMessage     `json:"shortDescription,omitempty"`
	FullDescription      *MultiformatMessage     `json:"fullDescription,omitempty"`
	Help                 *MultiformatMessage     `json:"help,omitempty"`
	DefaultConfiguration *ReportingConfiguration `json:"defaultConfiguration,omitempty"`
	Properties           map[string]interface{}  `json:"properties,omitempty"`
}

// ReportingConfiguration defines rule default severity
type ReportingConfiguration struct {
	Level string `json:"level"` // "error", "warning", "note", "none"
}

// MultiformatMessage supports plain text and markdown representations
type MultiformatMessage struct {
	Text     string `json:"text"`
	Markdown string `json:"markdown,omitempty"`
}

// Result describes a single finding or vulnerability detected
type Result struct {
	RuleID     string                 `json:"ruleId"`
	RuleIndex  int                    `json:"ruleIndex"`
	Level      string                 `json:"level"` // "error", "warning", "note", "none"
	Message    MultiformatMessage     `json:"message"`
	Locations  []Location             `json:"locations,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// Location specifies the target location of a finding
type Location struct {
	PhysicalLocation *PhysicalLocation `json:"physicalLocation,omitempty"`
	LogicalLocations []LogicalLocation `json:"logicalLocations,omitempty"`
}

// PhysicalLocation points to a file, endpoint, or artifact
type PhysicalLocation struct {
	ArtifactLocation ArtifactLocation `json:"artifactLocation"`
	Region           *Region          `json:"region,omitempty"`
}

// ArtifactLocation holds the file URI or URL
type ArtifactLocation struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId,omitempty"`
}

// Region identifies line and column ranges
type Region struct {
	StartLine   int `json:"startLine,omitempty"`
	StartColumn int `json:"startColumn,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

// LogicalLocation holds structured endpoint / symbol info
type LogicalLocation struct {
	Name               string `json:"name"`
	FullyQualifiedName string `json:"fullyQualifiedName,omitempty"`
	Kind               string `json:"kind,omitempty"`
}

// MapSeverityToLevel translates Kestrel severity to SARIF level and security-severity score
func MapSeverityToLevel(severity string) (level string, securityScore string) {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return "error", "9.0"
	case "high":
		return "error", "7.5"
	case "medium":
		return "warning", "5.0"
	case "low":
		return "note", "2.5"
	case "info":
		return "note", "1.0"
	default:
		return "warning", "5.0"
	}
}

// ParseTargetLocation converts a target string into a SARIF Location object
func ParseTargetLocation(target string) Location {
	t := strings.TrimSpace(target)
	if t == "" {
		t = "unknown-target"
	}

	// Check if target is a web URL
	if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") {
		loc := Location{
			PhysicalLocation: &PhysicalLocation{
				ArtifactLocation: ArtifactLocation{
					URI: t,
				},
			},
		}
		if parsed, err := url.Parse(t); err == nil && parsed.Path != "" {
			loc.LogicalLocations = []LogicalLocation{
				{
					Name:               parsed.Path,
					FullyQualifiedName: t,
					Kind:               "endpoint",
				},
			}
		}
		return loc
	}

	// Check for file path with line/column (e.g. "internal/app.go:42" or "src/index.js:10:5")
	parts := strings.Split(t, ":")
	if len(parts) >= 2 {
		filePath := strings.ReplaceAll(parts[0], "\\", "/")
		if line, err := strconv.Atoi(parts[1]); err == nil && line > 0 {
			region := &Region{StartLine: line}
			if len(parts) >= 3 {
				if col, err := strconv.Atoi(parts[2]); err == nil && col > 0 {
					region.StartColumn = col
				}
			}
			return Location{
				PhysicalLocation: &PhysicalLocation{
					ArtifactLocation: ArtifactLocation{
						URI:       filePath,
						URIBaseID: "%SRCROOT%",
					},
					Region: region,
				},
			}
		}
	}

	// Plain file or host path
	normPath := strings.ReplaceAll(t, "\\", "/")
	return Location{
		PhysicalLocation: &PhysicalLocation{
			ArtifactLocation: ArtifactLocation{
				URI:       normPath,
				URIBaseID: "%SRCROOT%",
			},
		},
	}
}

// NormalizeRuleID ensures a non-empty, standardized rule identifier
func NormalizeRuleID(vulnType, title string) string {
	vt := strings.TrimSpace(vulnType)
	if vt != "" {
		return vt
	}
	t := strings.TrimSpace(title)
	if t != "" {
		// Replace spaces with hyphen and cap length
		clean := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
				return r
			}
			return '-'
		}, t)
		clean = strings.Trim(clean, "-")
		if clean != "" {
			if len(clean) > 64 {
				clean = clean[:64]
			}
			return clean
		}
	}
	return "KESTREL-VULN"
}

// GenerateSARIF transforms a slice of Kestrel database vulnerabilities into a SARIF 2.1.0 report
func GenerateSARIF(vulns []*database.Vulnerability, toolVersion string) *SARIFReport {
	if toolVersion == "" {
		toolVersion = DefaultToolVersion
	}

	rules := make([]Rule, 0)
	ruleIndexMap := make(map[string]int)
	results := make([]Result, 0, len(vulns))

	for _, v := range vulns {
		if v == nil {
			continue
		}

		ruleID := NormalizeRuleID(v.Type, v.Title)
		level, score := MapSeverityToLevel(v.Severity)
		cvss := DefaultCVSSForSeverity(v.Severity)

		idx, exists := ruleIndexMap[ruleID]
		if !exists {
			idx = len(rules)
			ruleIndexMap[ruleID] = idx

			ruleName := v.Title
			if ruleName == "" {
				ruleName = ruleID
			}

			ruleDesc := v.Description
			if ruleDesc == "" {
				ruleDesc = v.Title
			}

			helpText := v.Recommendation
			if helpText == "" {
				helpText = "Review identified component and apply security hardening."
			}

			ruleProps := map[string]interface{}{
				"security-severity": score,
				"cvss-score":        cvss.Score,
				"cvss-vector":       cvss.Vector,
				"tags": []string{
					"security",
					strings.ToLower(ruleID),
				},
			}

			rules = append(rules, Rule{
				ID:   ruleID,
				Name: ruleName,
				ShortDescription: &MultiformatMessage{
					Text: ruleName,
				},
				FullDescription: &MultiformatMessage{
					Text: ruleDesc,
				},
				Help: &MultiformatMessage{
					Text:     helpText,
					Markdown: fmt.Sprintf("### Recommendation\n%s", helpText),
				},
				DefaultConfiguration: &ReportingConfiguration{
					Level: level,
				},
				Properties: ruleProps,
			})
		}

		msgText := v.Description
		if msgText == "" {
			msgText = v.Title
		}

		resultProps := map[string]interface{}{
			"id":                v.ID,
			"severity":          v.Severity,
			"status":            v.Status,
			"evidence":          v.Evidence,
			"reproductionSteps": v.ReproSteps,
			"cvss-score":        cvss.Score,
			"cvss-vector":       cvss.Vector,
			"impact":            v.Impact,
			"preconditions":     v.Preconditions,
		}
		if v.ProjectID != "" {
			resultProps["projectId"] = v.ProjectID
		}
		if v.ConversationID != "" {
			resultProps["conversationId"] = v.ConversationID
		}

		loc := ParseTargetLocation(v.Target)

		results = append(results, Result{
			RuleID:    ruleID,
			RuleIndex: idx,
			Level:     level,
			Message: MultiformatMessage{
				Text: fmt.Sprintf("[%s] %s: %s", strings.ToUpper(v.Severity), v.Title, msgText),
			},
			Locations:  []Location{loc},
			Properties: resultProps,
		})
	}

	report := &SARIFReport{
		Schema:  SARIFSchemaURL,
		Version: SARIFVersion,
		Runs: []Run{
			{
				Tool: Tool{
					Driver: Driver{
						Name:           ToolName,
						Version:        toolVersion,
						InformationURI: ToolInformationURI,
						Rules:          rules,
					},
				},
				Results: results,
			},
		},
	}

	return report
}

// ToJSON serializes vulnerabilities directly into SARIF JSON bytes
func ToJSON(vulns []*database.Vulnerability, toolVersion string, pretty bool) ([]byte, error) {
	report := GenerateSARIF(vulns, toolVersion)
	if pretty {
		return json.MarshalIndent(report, "", "  ")
	}
	return json.Marshal(report)
}
