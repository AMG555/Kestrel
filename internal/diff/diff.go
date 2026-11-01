package diff

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"kestrel/internal/database"
)

// FindingDiffItem represents a finding in a diff comparison.
type FindingDiffItem struct {
	ID             string  `json:"id"`
	Title          string  `json:"title"`
	Severity       string  `json:"severity"`
	Target         string  `json:"target"`
	Status         string  `json:"status"`
	CVE            string  `json:"cve,omitempty"`
	CVSSScore      float64 `json:"cvss_score,omitempty"`
	KnownExploited bool    `json:"known_exploited,omitempty"`
	Fingerprint    string  `json:"fingerprint"`
}

// DiffSummary provides aggregate metrics between two scans.
type DiffSummary struct {
	TotalBase       int `json:"total_base"`
	TotalTarget     int `json:"total_target"`
	NewCount        int `json:"new_count"`
	FixedCount      int `json:"fixed_count"`
	RegressedCount  int `json:"regressed_count"`
	PersistentCount int `json:"persistent_count"`
}

// DiffResult holds the complete diff report between a base scan and target scan.
type DiffResult struct {
	BaseLabel   string             `json:"base_label"`
	TargetLabel string             `json:"target_label"`
	GeneratedAt time.Time          `json:"generated_at"`
	Summary     DiffSummary        `json:"summary"`
	Regressed   []*FindingDiffItem `json:"regressed"`
	New         []*FindingDiffItem `json:"new"`
	Fixed       []*FindingDiffItem `json:"fixed"`
	Persistent  []*FindingDiffItem `json:"persistent"`
}

func findingKey(v *database.Vulnerability) string {
	if v == nil {
		return ""
	}
	if strings.TrimSpace(v.Fingerprint) != "" {
		return v.Fingerprint
	}
	fp := database.ComputeFindingFingerprint(v)
	if fp != "" {
		return fp
	}
	return strings.ToLower(strings.TrimSpace(v.Title)) + "::" + strings.ToLower(strings.TrimSpace(v.Target))
}

func isFixedStatus(s string) bool {
	norm := strings.ToLower(strings.TrimSpace(s))
	return norm == "fixed" || norm == "resolved"
}

func toDiffItem(v *database.Vulnerability) *FindingDiffItem {
	if v == nil {
		return nil
	}
	return &FindingDiffItem{
		ID:             v.ID,
		Title:          v.Title,
		Severity:       strings.ToUpper(strings.TrimSpace(v.Severity)),
		Target:         v.Target,
		Status:         v.Status,
		CVE:            v.CVE,
		CVSSScore:      v.CVSSScore,
		KnownExploited: v.KnownExploited,
		Fingerprint:    v.Fingerprint,
	}
}

// ComputeDiff compares a baseline scan against a target scan and classifies findings as
// New, Fixed, Regressed (reappeared after being fixed), or Persistent.
func ComputeDiff(baseVulns, targetVulns []*database.Vulnerability, baseLabel, targetLabel string) *DiffResult {
	if baseLabel == "" {
		baseLabel = "Baseline Scan"
	}
	if targetLabel == "" {
		targetLabel = "Target Scan"
	}

	res := &DiffResult{
		BaseLabel:   baseLabel,
		TargetLabel: targetLabel,
		GeneratedAt: time.Now().UTC(),
		Summary: DiffSummary{
			TotalBase:   len(baseVulns),
			TotalTarget: len(targetVulns),
		},
	}

	baseMap := make(map[string]*database.Vulnerability)
	for _, b := range baseVulns {
		if b == nil {
			continue
		}
		k := findingKey(b)
		baseMap[k] = b
	}

	targetSeen := make(map[string]bool)

	for _, t := range targetVulns {
		if t == nil {
			continue
		}
		k := findingKey(t)
		targetSeen[k] = true

		baseItem, inBase := baseMap[k]
		if !inBase {
			// Found in target, not in base -> NEW
			res.New = append(res.New, toDiffItem(t))
		} else {
			// Present in both scans
			baseIsFixed := isFixedStatus(baseItem.Status)
			targetIsFixed := isFixedStatus(t.Status)

			switch {
			case baseIsFixed && !targetIsFixed:
				// Was fixed in base, but reappeared active -> REGRESSED
				res.Regressed = append(res.Regressed, toDiffItem(t))
			case targetIsFixed:
				// Target explicitly marked fixed
				res.Fixed = append(res.Fixed, toDiffItem(t))
			default:
				// Persistent active finding
				res.Persistent = append(res.Persistent, toDiffItem(t))
			}
		}
	}

	// Any finding in base that was active and did not appear in target scan is considered Fixed
	for _, b := range baseVulns {
		if b == nil {
			continue
		}
		k := findingKey(b)
		if !targetSeen[k] {
			if !isFixedStatus(b.Status) {
				fixedItem := toDiffItem(b)
				fixedItem.Status = "fixed"
				res.Fixed = append(res.Fixed, fixedItem)
			}
		}
	}

	res.Summary.NewCount = len(res.New)
	res.Summary.FixedCount = len(res.Fixed)
	res.Summary.RegressedCount = len(res.Regressed)
	res.Summary.PersistentCount = len(res.Persistent)

	return res
}

// RenderMarkdown renders the diff results into an audit-ready Markdown report.
func RenderMarkdown(res *DiffResult) string {
	if res == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Scan Assessment Run Diff: %s vs %s\n\n", res.BaseLabel, res.TargetLabel))
	sb.WriteString(fmt.Sprintf("- **Generated:** %s\n", res.GeneratedAt.Format("2006-01-02 15:04:05 UTC")))
	sb.WriteString(fmt.Sprintf("- **Baseline:** `%s` (%d findings)\n", res.BaseLabel, res.Summary.TotalBase))
	sb.WriteString(fmt.Sprintf("- **Target:** `%s` (%d findings)\n\n", res.TargetLabel, res.Summary.TotalTarget))

	sb.WriteString("## Executive Summary\n\n")
	sb.WriteString("| Diff Category | Count | Status Description |\n")
	sb.WriteString("| :--- | :---: | :--- |\n")
	sb.WriteString(fmt.Sprintf("| ⚠️ **Regressed** | **%d** | Previously fixed findings that reappeared in target scan |\n", res.Summary.RegressedCount))
	sb.WriteString(fmt.Sprintf("| 🔴 **New** | **%d** | Newly identified findings in target scan |\n", res.Summary.NewCount))
	sb.WriteString(fmt.Sprintf("| 🟢 **Fixed / Resolved** | **%d** | Baseline findings resolved in target scan |\n", res.Summary.FixedCount))
	sb.WriteString(fmt.Sprintf("| 🟡 **Persistent** | **%d** | Unresolved findings present across both scans |\n\n", res.Summary.PersistentCount))

	if len(res.Regressed) > 0 {
		sb.WriteString("## ⚠️ Regressed Findings (Attention Required)\n\n")
		sb.WriteString("These vulnerabilities were previously resolved or marked fixed, but were detected again in the latest assessment.\n\n")
		for i, item := range res.Regressed {
			renderFindingItem(&sb, i+1, item)
		}
	}

	if len(res.New) > 0 {
		sb.WriteString("## 🔴 Newly Identified Findings\n\n")
		sb.WriteString("These vulnerabilities are new attack surface identified in the target scan.\n\n")
		for i, item := range res.New {
			renderFindingItem(&sb, i+1, item)
		}
	}

	if len(res.Fixed) > 0 {
		sb.WriteString("## 🟢 Resolved / Fixed Findings\n\n")
		sb.WriteString("These vulnerabilities from the baseline scan are no longer present or confirmed resolved.\n\n")
		for i, item := range res.Fixed {
			renderFindingItem(&sb, i+1, item)
		}
	}

	if len(res.Persistent) > 0 {
		sb.WriteString("## 🟡 Persistent Active Findings\n\n")
		sb.WriteString("These vulnerabilities remain active and unmitigated across both assessment cycles.\n\n")
		for i, item := range res.Persistent {
			renderFindingItem(&sb, i+1, item)
		}
	}

	sb.WriteString("*Generated by [Kestrel](https://github.com/AMG555/Kestrel) — Retainer Assessment Run Diff Engine.*\n")
	return sb.String()
}

func renderFindingItem(sb *strings.Builder, index int, item *FindingDiffItem) {
	sb.WriteString(fmt.Sprintf("### %d. [%s] %s\n\n", index, item.Severity, item.Title))
	if item.Target != "" {
		sb.WriteString(fmt.Sprintf("- **Target:** `%s`\n", item.Target))
	}
	if item.CVE != "" {
		sb.WriteString(fmt.Sprintf("- **CVE:** `%s`\n", item.CVE))
	}
	if item.CVSSScore > 0 {
		sb.WriteString(fmt.Sprintf("- **CVSS v3.1:** `%.1f`\n", item.CVSSScore))
	}
	if item.KnownExploited {
		sb.WriteString("- **CISA KEV:** ⚠️ **YES (Actively Exploited in the Wild)**\n")
	}
	if item.Status != "" {
		sb.WriteString(fmt.Sprintf("- **Status:** `%s`\n", item.Status))
	}
	sb.WriteString("\n")
}

// RenderCLI renders a compact terminal summary suitable for CLI output.
func RenderCLI(res *DiffResult) string {
	if res == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Kestrel Scan Run Diff: %s -> %s\n", res.BaseLabel, res.TargetLabel))
	sb.WriteString("------------------------------------------------------------------\n")
	sb.WriteString(fmt.Sprintf("  Regressed (⚠️) : %d\n", res.Summary.RegressedCount))
	sb.WriteString(fmt.Sprintf("  New       (🔴) : %d\n", res.Summary.NewCount))
	sb.WriteString(fmt.Sprintf("  Fixed     (🟢) : %d\n", res.Summary.FixedCount))
	sb.WriteString(fmt.Sprintf("  Persistent(🟡) : %d\n", res.Summary.PersistentCount))
	sb.WriteString("------------------------------------------------------------------\n")

	if len(res.Regressed) > 0 {
		sb.WriteString("\n⚠️  REGRESSED FINDINGS:\n")
		for _, item := range res.Regressed {
			sb.WriteString(fmt.Sprintf("  - [%s] %s (%s)\n", item.Severity, item.Title, item.Target))
		}
	}
	if len(res.New) > 0 {
		sb.WriteString("\n🔴 NEW FINDINGS:\n")
		for _, item := range res.New {
			sb.WriteString(fmt.Sprintf("  - [%s] %s (%s)\n", item.Severity, item.Title, item.Target))
		}
	}
	if len(res.Fixed) > 0 {
		sb.WriteString("\n🟢 FIXED FINDINGS:\n")
		for _, item := range res.Fixed {
			sb.WriteString(fmt.Sprintf("  - [%s] %s (%s)\n", item.Severity, item.Title, item.Target))
		}
	}
	return sb.String()
}

// RenderJSON serializes the diff result into JSON bytes.
func RenderJSON(res *DiffResult) ([]byte, error) {
	return json.MarshalIndent(res, "", "  ")
}
