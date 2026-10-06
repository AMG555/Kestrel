package report

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"kestrel/internal/database"
)

// Format is the output format for a report.
type Format string

const (
	FormatMarkdown Format = "markdown"
	FormatJSON     Format = "json"
	FormatCSV      Format = "csv"
)

// ProjectReport is the complete report for a project.
type ProjectReport struct {
	GeneratedAt  time.Time              `json:"generated_at"`
	Project      *database.Project      `json:"project"`
	Stats        *database.ProjectStats `json:"stats"`
	Assets       []database.Asset       `json:"assets"`
	Vulns        []database.Vulnerability `json:"vulnerabilities"`
	Facts        []database.ProjectFact `json:"facts"`
	AuditEntries []database.AuditLog    `json:"audit_entries"`
}

// Generator produces reports for a project.
type Generator struct {
	db *database.DB
}

// NewGenerator creates a report Generator.
func NewGenerator(db *database.DB) *Generator {
	return &Generator{db: db}
}

// Build collects all data for a project and returns a ProjectReport.
func (g *Generator) Build(projectID string) (*ProjectReport, error) {
	proj, err := g.db.GetProjectByID(projectID)
	if err != nil || proj == nil {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}

	stats, _ := g.db.GetProjectStats(projectID)

	assets, _, _ := g.db.ListAssets(database.ListAssetsParams{ProjectID: projectID, Limit: 1000})
	vulnPtrs, _, _ := g.db.ListVulnerabilities(database.ListVulnsParams{ProjectID: projectID, Limit: 1000})
	factPtrs, _ := g.db.ListProjectFacts(projectID)
	auditPtrs, _, _ := g.db.ListAuditLogs(database.ListAuditLogsParams{Limit: 100})

	// Dereference pointer slices.
	var vulns []database.Vulnerability
	for _, v := range vulnPtrs {
		if v != nil {
			vulns = append(vulns, *v)
		}
	}
	var facts []database.ProjectFact
	for _, f := range factPtrs {
		if f != nil {
			facts = append(facts, *f)
		}
	}
	var auditLogs []database.AuditLog
	for _, a := range auditPtrs {
		if a != nil {
			auditLogs = append(auditLogs, *a)
		}
	}
	var assetSlice []database.Asset
	for _, a := range assets {
		if a != nil {
			assetSlice = append(assetSlice, *a)
		}
	}

	// Augment stats with severity counts.
	if stats != nil {
		for _, v := range vulns {
			switch strings.ToLower(v.Severity) {
			case "critical":
				stats.CriticalCount++
			case "high":
				stats.HighCount++
			case "medium":
				stats.MediumCount++
			case "low":
				stats.LowCount++
			}
		}
	}

	return &ProjectReport{
		GeneratedAt:  time.Now().UTC(),
		Project:      proj,
		Stats:        stats,
		Assets:       assetSlice,
		Vulns:        vulns,
		Facts:        facts,
		AuditEntries: auditLogs,
	}, nil
}

// Render serialises a report into the requested format.
func (g *Generator) Render(r *ProjectReport, format Format) ([]byte, string, error) {
	switch format {
	case FormatJSON:
		data, err := json.MarshalIndent(r, "", "  ")
		return data, "application/json", err
	case FormatCSV:
		data, err := renderCSV(r)
		return data, "text/csv", err
	default:
		data := renderMarkdown(r)
		return []byte(data), "text/markdown", nil
	}
}

// ─── Markdown renderer ────────────────────────────────────────────────────────

func renderMarkdown(r *ProjectReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s — Security Assessment Report\n\n", r.Project.Name))
	sb.WriteString(fmt.Sprintf("**Generated:** %s  \n", r.GeneratedAt.Format("2006-01-02 15:04 UTC")))
	sb.WriteString(fmt.Sprintf("**Status:** %s  \n\n", r.Project.Status))

	if r.Project.Description != "" {
		sb.WriteString(fmt.Sprintf("> %s\n\n", r.Project.Description))
	}

	if len(r.Project.Scope) > 0 {
		sb.WriteString("**Scope:**\n")
		for _, s := range r.Project.Scope {
			sb.WriteString(fmt.Sprintf("- `%s`\n", s))
		}
		sb.WriteString("\n")
	}

	// Stats summary.
	if r.Stats != nil {
		sb.WriteString("## Summary\n\n")
		sb.WriteString("| Metric | Count |\n|--------|-------|\n")
		sb.WriteString(fmt.Sprintf("| Assets | %d |\n", r.Stats.AssetCount))
		sb.WriteString(fmt.Sprintf("| Vulnerabilities | %d |\n", r.Stats.VulnCount))
		sb.WriteString(fmt.Sprintf("| Critical | %d |\n", r.Stats.CriticalCount))
		sb.WriteString(fmt.Sprintf("| High | %d |\n", r.Stats.HighCount))
		sb.WriteString(fmt.Sprintf("| Medium | %d |\n", r.Stats.MediumCount))
		sb.WriteString(fmt.Sprintf("| Low | %d |\n", r.Stats.LowCount))
		sb.WriteString("\n")
	}

	// Vulnerabilities.
	if len(r.Vulns) > 0 {
		sb.WriteString("## Vulnerabilities\n\n")
		for _, v := range r.Vulns {
			sb.WriteString(fmt.Sprintf("### [%s] %s\n\n", strings.ToUpper(v.Severity), v.Title))
			if v.Description != "" {
				sb.WriteString(v.Description + "\n\n")
			}
			if v.Target != "" {
				sb.WriteString(fmt.Sprintf("**Target:** `%s`  \n", v.Target))
			}
			sb.WriteString(fmt.Sprintf("**Status:** %s  \n", v.Status))
			if v.Recommendation != "" {
				sb.WriteString(fmt.Sprintf("**Recommendation:** %s\n\n", v.Recommendation))
			}
			if v.Evidence != "" {
				sb.WriteString(fmt.Sprintf("**Evidence:**\n```\n%s\n```\n\n", v.Evidence))
			}
			sb.WriteString("---\n\n")
		}
	}

	// Assets.
	if len(r.Assets) > 0 {
		sb.WriteString("## Assets\n\n")
		sb.WriteString("| Host/Domain | IP | Port | Service | Status | Risk |\n")
		sb.WriteString("|-------------|----|----|---------|--------|------|\n")
		for _, a := range r.Assets {
			host := a.Host
			if host == "" {
				host = a.Domain
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %d | %s | %s | %s |\n",
				host, a.IP, a.Port, a.Service, a.Status, a.RiskLevel))
		}
		sb.WriteString("\n")
	}

	// Key facts.
	if len(r.Facts) > 0 {
		sb.WriteString("## Key Findings / Facts\n\n")
		for _, f := range r.Facts {
			pinMark := ""
			if f.Pinned {
				pinMark = "📌 "
			}
			sb.WriteString(fmt.Sprintf("- %s**[%s]** %s: %s\n", pinMark, f.Category, f.FactKey, f.Summary))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("---\n*Generated by Kestrel — Authorized use only*\n")
	return sb.String()
}

// ─── CSV renderer ─────────────────────────────────────────────────────────────

func renderCSV(r *ProjectReport) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Section: Project
	_ = w.Write([]string{"SECTION", "PROJECT"})
	_ = w.Write([]string{"Name", r.Project.Name})
	_ = w.Write([]string{"Description", r.Project.Description})
	_ = w.Write([]string{"Status", r.Project.Status})
	_ = w.Write([]string{"Generated", r.GeneratedAt.Format(time.RFC3339)})
	_ = w.Write(nil)

	// Section: Vulnerabilities
	_ = w.Write([]string{"SECTION", "VULNERABILITIES"})
	_ = w.Write([]string{"ID", "Title", "Severity", "Status", "Target", "Description", "Recommendation"})
	for _, v := range r.Vulns {
		_ = w.Write([]string{v.ID, v.Title, v.Severity, v.Status, v.Target, v.Description, v.Recommendation})
	}
	_ = w.Write(nil)

	// Section: Assets
	_ = w.Write([]string{"SECTION", "ASSETS"})
	_ = w.Write([]string{"ID", "Host", "Domain", "IP", "Port", "Service", "Protocol", "Status", "RiskLevel"})
	for _, a := range r.Assets {
		_ = w.Write([]string{
			a.ID, a.Host, a.Domain, a.IP,
			fmt.Sprintf("%d", a.Port), a.Service, a.Protocol, a.Status, a.RiskLevel,
		})
	}

	w.Flush()
	return buf.Bytes(), w.Error()
}
