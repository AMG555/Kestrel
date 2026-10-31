package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"kestrel/internal/database"
	"kestrel/internal/mcp"
)

// MarkdownReportOptions contains data for generating a Markdown assessment report.
type MarkdownReportOptions struct {
	Project        *database.Project
	Vulns          []*database.Vulnerability
	ToolExecutions []*mcp.ToolExecution
	GeneratedAt    time.Time
}

// RenderMarkdownReport generates a structured, audit-ready Markdown pentest assessment report.
func RenderMarkdownReport(opts MarkdownReportOptions) string {
	projectName := "Target Assessment"
	if opts.Project != nil && strings.TrimSpace(opts.Project.Name) != "" {
		projectName = opts.Project.Name
	}

	genTime := opts.GeneratedAt
	if genTime.IsZero() {
		genTime = time.Now().UTC()
	}

	var critical, high, medium, low, info int
	for _, v := range opts.Vulns {
		switch strings.ToLower(strings.TrimSpace(v.Severity)) {
		case "critical":
			critical++
		case "high":
			high++
		case "medium":
			medium++
		case "low":
			low++
		default:
			info++
		}
	}
	total := len(opts.Vulns)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Security Assessment Report — %s\n\n", projectName))
	sb.WriteString(fmt.Sprintf("- **Generated:** %s\n", genTime.Format("2006-01-02 15:04:05 UTC")))
	sb.WriteString("- **Platform:** Kestrel Security Workbench (Self-Hosted)\n")
	if opts.Project != nil && opts.Project.ID != "" {
		sb.WriteString(fmt.Sprintf("- **Project ID:** `%s`\n", opts.Project.ID))
	}
	sb.WriteString("- **Verification:** SHA-256 Hash-Chained Audit Ledger & Verbatim Evidence\n\n")

	sb.WriteString("## Executive Summary\n\n")
	sb.WriteString(fmt.Sprintf("This security assessment report summarizes findings identified for **%s**. ", projectName))
	if total == 0 {
		sb.WriteString("No confirmed vulnerabilities were identified during this assessment cycle.\n\n")
	} else {
		sb.WriteString(fmt.Sprintf("A total of **%d** vulnerabilities were identified (%d Critical, %d High, %d Medium, %d Low, %d Informational).\n\n",
			total, critical, high, medium, low, info))
	}

	sb.WriteString("| Severity | Findings Count |\n")
	sb.WriteString("| :--- | :---: |\n")
	sb.WriteString(fmt.Sprintf("| 🔴 Critical | %d |\n", critical))
	sb.WriteString(fmt.Sprintf("| 🟠 High | %d |\n", high))
	sb.WriteString(fmt.Sprintf("| 🟡 Medium | %d |\n", medium))
	sb.WriteString(fmt.Sprintf("| 🔵 Low | %d |\n", low))
	sb.WriteString(fmt.Sprintf("| ⚪ Informational | %d |\n", info))
	sb.WriteString(fmt.Sprintf("| **Total** | **%d** |\n\n", total))

	sb.WriteString("## Detailed Findings\n\n")
	if total == 0 {
		sb.WriteString("*No findings recorded.*\n\n")
	} else {
		for i, v := range opts.Vulns {
			sevLabel := strings.ToUpper(strings.TrimSpace(v.Severity))
			if sevLabel == "" {
				sevLabel = "INFO"
			}
			sb.WriteString(fmt.Sprintf("### %d. [%s] %s\n\n", i+1, sevLabel, v.Title))
			sb.WriteString(fmt.Sprintf("- **Finding ID:** `%s`\n", v.ID))
			sb.WriteString(fmt.Sprintf("- **Severity:** `%s`\n", sevLabel))
			if v.Status != "" {
				sb.WriteString(fmt.Sprintf("- **Status:** `%s`\n", v.Status))
			}
			if v.Type != "" {
				sb.WriteString(fmt.Sprintf("- **Type / Taxonomy:** `%s`\n", v.Type))
			}
			if v.Target != "" {
				sb.WriteString(fmt.Sprintf("- **Target:** `%s`\n", v.Target))
			}
			sb.WriteString("\n")

			if v.Description != "" {
				sb.WriteString("#### Description\n\n")
				sb.WriteString(v.Description + "\n\n")
			}
			if v.Preconditions != "" {
				sb.WriteString("#### Preconditions\n\n")
				sb.WriteString(v.Preconditions + "\n\n")
			}
			if v.ReproSteps != "" {
				sb.WriteString("#### Steps to Reproduce\n\n")
				sb.WriteString(v.ReproSteps + "\n\n")
			}
			if v.Evidence != "" {
				sb.WriteString("#### Verbatim Evidence / Proof of Concept\n\n")
				sb.WriteString("```\n" + v.Evidence + "\n```\n\n")
			}
			if v.Recommendation != "" {
				sb.WriteString("#### Remediation Recommendation\n\n")
				sb.WriteString(v.Recommendation + "\n\n")
			}
			sb.WriteString("---\n\n")
		}
	}

	if len(opts.ToolExecutions) > 0 {
		sb.WriteString("## Tool Execution & Verbatim Forensic Log\n\n")
		sb.WriteString("The following verbatim tool execution records document the commands, arguments, timestamps, and stdout/stderr outputs captured during the assessment.\n\n")

		for i, exec := range opts.ToolExecutions {
			durationStr := ""
			if exec.Duration > 0 {
				durationStr = fmt.Sprintf(" (Duration: %s)", exec.Duration.Round(time.Millisecond))
			}
			sb.WriteString(fmt.Sprintf("### %d. Tool: `%s` [%s]\n\n", i+1, exec.ToolName, strings.ToUpper(exec.Status)))
			sb.WriteString(fmt.Sprintf("- **Execution ID:** `%s`\n", exec.ID))
			sb.WriteString(fmt.Sprintf("- **Timestamp:** %s%s\n", exec.StartTime.UTC().Format("2006-01-02 15:04:05 UTC"), durationStr))
			sb.WriteString(fmt.Sprintf("- **Status:** `%s`\n\n", exec.Status))

			argsJSON := "{}"
			if len(exec.Arguments) > 0 {
				if b, err := json.MarshalIndent(exec.Arguments, "", "  "); err == nil {
					argsJSON = string(b)
				}
			}
			sb.WriteString("#### Arguments\n\n```json\n" + argsJSON + "\n```\n\n")

			var outTexts []string
			if exec.Result != nil {
				for _, c := range exec.Result.Content {
					if c.Text != "" {
						outTexts = append(outTexts, c.Text)
					}
				}
			}
			if len(outTexts) == 0 && exec.PartialOutput != "" {
				outTexts = append(outTexts, exec.PartialOutput)
			}
			if len(outTexts) > 0 {
				sb.WriteString("#### Standard Output / Result\n\n```\n" + strings.Join(outTexts, "\n") + "\n```\n\n")
			}
			if exec.Error != "" {
				sb.WriteString("#### Standard Error / Diagnostics\n\n```\n" + exec.Error + "\n```\n\n")
			}
			sb.WriteString("---\n\n")
		}
	}

	sb.WriteString("*Generated by [Kestrel](https://github.com/AMG555/Kestrel) — Self-hosted pentest workbench for small teams.*\n")
	return sb.String()
}
