// Package projectprompt provides project blackboard-related system prompt text (pure strings, no database dependency).
// Referenced by agent/multiagent packages to avoid import cycles between agent and project packages that break gopls metadata.
package projectprompt

import (
	"strings"

	"kestrel/internal/mcp/builtin"
)

const (
	factRhythmCore = "Do not wait until the end of a session to batch-write. After **confirming** each new finding (open port/service version, entry path, auth state or credential details, exploitable point or attack surface change), **immediately** call `upsert_project_fact` (overwrite with same fact_key). After **validating** each reproducible vulnerability (with POC/impact), **immediately** call `record_vulnerability`; facts and vulnerabilities can each be recorded once. Prioritise writing to the database before proceeding to the next step to avoid losing details after context compression. If not bound to a project, state that the blackboard is unavailable but retain evidence summaries in the current session."
	factRhythmCoordinatorSuffix = "When delegated/sub-tasks return new findings or vulnerabilities, the coordinator must write them promptly — do not assume the sub-agent has already recorded them."
	factRhythmSubAgentSuffix    = "If the tool set does not include the above tools, provide structured 'pending-write' entries at the end of the deliverable (suggested fact_key, summary, body/POC key points) for the coordinator to **immediately** write."
)

// FactRecordingIncrementalRhythmMarkdown returns the incremental record-as-you-pentest rhythm (Markdown, for agents/*.md and documentation alignment).
func FactRecordingIncrementalRhythmMarkdown(coordinator, subAgent bool) string {
	var b strings.Builder
	b.WriteString("- **Record as you pentest (mandatory rhythm)**: ")
	b.WriteString(factRhythmCore)
	if coordinator {
		b.WriteString(factRhythmCoordinatorSuffix)
	}
	if subAgent {
		b.WriteString(factRhythmSubAgentSuffix)
	}
	return b.String()
}

func factRecordingIncrementalRhythmBuiltin(coordinator, subAgent bool) string {
	var b strings.Builder
	b.WriteString("- **Record as you pentest (mandatory rhythm)**: Do not wait until session end or wrap-up to batch-write. After each confirmed new finding (open port/service version, entry path, auth state or credential characteristics, exploitable point or attack surface change), **immediately** call ")
	b.WriteString(builtin.ToolUpsertProjectFact)
	b.WriteString("(overwrite with same fact_key). After **validating** each reproducible vulnerability (with POC/impact), **immediately** call ")
	b.WriteString(builtin.ToolRecordVulnerability)
	b.WriteString("; facts and vulnerabilities can each be recorded once. Prioritise writing to the database before proceeding to the next step to avoid losing details after context compression. If not bound to a project, state that the blackboard is unavailable but retain evidence summaries in the current session.")
	if coordinator {
		b.WriteString(factRhythmCoordinatorSuffix)
	}
	if subAgent {
		b.WriteString(factRhythmSubAgentSuffix)
	}
	return b.String()
}

func factEdgeRecordingGuidance() string {
	return `### Fact relationship edges (links)

- When writing **finding / chain / exploit / poc**, you **must** provide ` + "`links`" + ` in ` + "`upsert_project_fact`" + ` (**recommended ` + "`from`" + `**: source fact points to the current fact, i.e. ` + "`from`" + ` → current ` + "`fact_key`" + `).
- **Minimum requirement**: finding type needs at least 1 edge from=target/* + type=discovered_on (i.e. target → finding); recording exploit on a finding uses from=exploit/* + type=exploits (i.e. exploit → finding).
- **Common types**: ` + "`discovered_on`" + ` (where discovered), ` + "`depends_on`" + ` (prereq for reproduction), ` + "`leads_to`" + ` (knowledge progression), ` + "`enables`" + ` (expands attack surface), ` + "`exploits`" + ` (exploitation relationship), ` + "`contains`" + ` (asset containment), ` + "`part_of`" + ` (belongs to chain/group), ` + "`supports`" + ` (evidence support).
- On update: **omitting links preserves existing edges**; passing links **replaces all** relationship edges (from → current fact).
- The "dependent facts" section in body can coexist with links (human-readable); structured relationships are governed by links.`
}

func factRecordingGuidanceBlock() string {
	return `### Fact writing specification (audit reproduction / knowledge capture)

- **summary**: one line for indexing; must include "what + where + how to trigger/validate" — do not write only the conclusion (e.g. just "SQLi exists").
- **body**: full reproducible context; written to the body field of ` + "`upsert_project_fact`" + `; index does not contain body — subsequent sessions must call ` + "`get_project_fact`" + ` to retrieve it.
- **category / fact_key suggestions**:
  - Environment/recon: ` + "`target/`" + `, ` + "`auth/`" + `, ` + "`infra/`" + `, ` + "`business/`" + ` (body can use environment template)
  - Discovery and exploitation: ` + "`finding/`" + `, ` + "`chain/`" + `, ` + "`exploit/`" + `, ` + "`poc/`" + ` (**must** fill body with attack chain template: entry point, step-by-step attack chain, raw request/response or commands, evidence, related vulnerability ID)
- **Division with vulnerability records**: ` + "`record_vulnerability`" + ` records deliverable findings; facts record **all context needed for reproduction** (including failed attempts, bypasses, dependent sessions) — each can be recorded once.
- When updating the same finding, keep the same ` + "`fact_key`" + ` and overwrite; do not scatter across multiple keys causing context loss.`
}

// FactRecordingBlackboardSection is the complete system prompt block for the project blackboard and vulnerability recording (shared by single/multi-Agent primary agent).
func FactRecordingBlackboardSection(coordinatorDelegate bool) string {
	var b strings.Builder
	b.WriteString("## Project blackboard (facts) and vulnerability records (separated)\n\n")
	b.WriteString("If the current conversation is bound to a project, the system will automatically inject the 'project blackboard index' (fact_key + summary only). **When summary is insufficient, you must call ")
	b.WriteString(builtin.ToolGetProjectFact)
	b.WriteString("(fact_key) to retrieve the body — never fabricate details from summary alone.**\n\n")
	b.WriteString(factRecordingIncrementalRhythmBuiltin(coordinatorDelegate, false))
	b.WriteString("\n\n")
	b.WriteString("- **Environment/target/authentication knowledge** (not a formal vulnerability entry): use ")
	b.WriteString(builtin.ToolUpsertProjectFact)
	b.WriteString(", fact_key suggested as `category/slug` (e.g. target/primary_domain), overwrite with same key; body records port/version/credential details and evidence source.\n")
	b.WriteString("- **Discovery and exploitation context** (audit reproduction): fact_key recommended prefixes: finding/, chain/, exploit/, poc/; **body is required** with full attack chain (entry → steps → raw request/response or commands → result → related related_vulnerability_id); **do not write conclusions only**; summary: one-line key point of \"what + where + how to validate\".\n")
	b.WriteString("- **Deliverable vulnerabilities**: use ")
	b.WriteString(builtin.ToolRecordVulnerability)
	b.WriteString(", including title, severity, type, target, proof (POC), impact, and remediation advice. Before recording, you may ")
	b.WriteString(builtin.ToolListVulnerabilities)
	b.WriteString(" to deduplicate; for details use ")
	b.WriteString(builtin.ToolGetVulnerability)
	b.WriteString("(id) (defaults to current project/session only).\n")
	b.WriteString("- The same finding may need to be **recorded once each** (facts record the **full attack chain and exploit details** for reproduction; vulnerabilities record formal findings). Use ")
	b.WriteString(builtin.ToolDeprecateProjectFact)
	b.WriteString(" or vulnerability status false_positive.\n")
	b.WriteString("- When there are many facts, use ")
	b.WriteString(builtin.ToolListProjectFacts)
	b.WriteString(" / ")
	b.WriteString(builtin.ToolSearchProjectFacts)
	b.WriteString(" to search.\n\n")
	b.WriteString(factEdgeRecordingGuidance())
	b.WriteString("\n\n")
	b.WriteString(factRecordingGuidanceBlock())
	b.WriteString("\n\nSeverity levels: critical / high / medium / low / info. Proof must contain sufficient evidence (request/response, screenshots, command output, etc.).")
	return b.String()
}

// FactRecordingSubAgentSection is the sub-agent record-as-you-pentest section (outputs pending entries when no tool is available).
func FactRecordingSubAgentSection() string {
	return "## Record As You Pentest\n\n" + factRecordingIncrementalRhythmBuiltin(false, true) + "\n"
}

// FactRecordingBlackboardSectionMarkdown is the Markdown equivalent of FactRecordingBlackboardSection (tool names as literals, for agents/*.md).
func FactRecordingBlackboardSectionMarkdown(coordinatorDelegate bool) string {
	var b strings.Builder
	b.WriteString("## Project blackboard (facts) and vulnerability records (separated)\n\n")
	b.WriteString("If the current conversation has a bound project, the system automatically injects the project blackboard index (fact_key + summary only). **When the summary is insufficient, you must call `get_project_fact(fact_key)` to retrieve the body — never fabricate details from summary alone.**\n\n")
	b.WriteString(FactRecordingIncrementalRhythmMarkdown(coordinatorDelegate, false))
	b.WriteString("\n\n")
	b.WriteString("- **Environment/target/auth findings** (non-formal vulnerabilities): use **`upsert_project_fact`**, `fact_key` recommended as `category/slug` (e.g. `target/primary_domain`), overwrite with same key; body records port/version/credential details and evidence source.\n")
	b.WriteString("- **Discovery and exploitation context** (audit reproduction): `fact_key` recommended prefixes `finding/`, `chain/`, `exploit/`, `poc/`; **body is required** with full attack chain (entry → steps → raw request/response or commands → result → related `related_vulnerability_id`); **do not write conclusions only**; summary: one-line key point of \"what + where + how to validate\".\n")
	b.WriteString("- **Deliverable vulnerabilities**: use **`record_vulnerability`** (title, description, severity, type, target, proof POC, impact, remediation advice). Severity: critical / high / medium / low / info.\n")
	b.WriteString("- The same discovery may need to be **recorded in both** (fact records the reproducible attack chain; vulnerability records the formal finding). For false positives use **`deprecate_project_fact`** or vulnerability status false_positive.\n")
	b.WriteString("- When there are many facts, use **`list_project_facts`** / **`search_project_facts`** to search.\n\n")
	b.WriteString(factEdgeRecordingGuidance())
	b.WriteString("\n\n")
	b.WriteString(factRecordingGuidanceBlock())
	b.WriteString("\n\nSeverity levels: critical / high / medium / low / info. Proof must contain sufficient evidence (request/response, screenshots, command output, etc.).")
	return b.String()
}

// FactEdgeRecordingGuidance is the Agent guidelines for writing edges (reusable by the project package).
func FactEdgeRecordingGuidance() string { return factEdgeRecordingGuidance() }

// FactRecordingGuidanceBlock is the fact writing spec block (reusable by the project package).
func FactRecordingGuidanceBlock() string { return factRecordingGuidanceBlock() }
