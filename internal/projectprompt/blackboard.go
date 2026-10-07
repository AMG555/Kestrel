// Package projectprompt provides project blackboard-related system prompt text (pure strings, no database dependency).
// Referenced by agent/multiagent packages to avoid import cycles between agent and project packages that break gopls metadata.
package projectprompt

import (
	"strings"

	"kestrel/internal/mcp/builtin"
)

const (
	factRhythmCore = "Do not wait until the end of a session to batch-write. After **confirming** each new finding (open port/service version, entry path, auth state or credential details, exploitable point or attack surface change), **immediately** call `upsert_project_fact`（同 fact_key 覆盖update）。每**validate**出一条可复现漏洞（含 POC/impact）后，**立即**调用 `record_vulnerability`；与事实可各记一次。continue下一步工作前优先落库，避免context compression后细节丢失。未绑project时说明none法写黑板，仍在本轮保留证据summary。"
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
	b.WriteString("- **Record as you pentest (mandatory rhythm)**: Do not wait until session end or wrap-up to batch-write. After each confirmed new finding (open port/service version, entry path, auth state or credential characteristics, exploitable point or attack surface change),**立即**调用 ")
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

- When writing **finding / chain / exploit / poc**, you **must** provide ` + "`links`" + ` in ` + "`upsert_project_fact`" + ` (**recommended ` + "`from`" + `**：来源 fact 指向当前 fact，即 ` + "`from`" + ` → 当前 ` + "`fact_key`" + `）。
- **Minimum requirement**: finding type needs at least 1 edge from=target/* + type=discovered_on (i.e. target → finding); recording exploit on a finding uses from=exploit/* + type=exploits（即 exploit → finding）。
- **Common types**: ` + "`discovered_on`" + ` (where discovered), ` + "`depends_on`" + ` (prereq for reproduction), ` + "`leads_to`" + ` (knowledge progression), ` + "`enables`" + `（扩大attack surface）、` + "`exploits`" + `（利用关系）、` + "`contains`" + `（资产包含）、` + "`part_of`" + `（属于链/组）、` + "`supports`" + `（证据支撑）。
- On update: **omitting links preserves existing edges**; passing links **replaces all** relationship edges (from → current fact).
- The "dependent facts" section in body can coexist with links (human-readable); structured relationships are governed by links.`
}

func factRecordingGuidanceBlock() string {
	return `### Fact writing specification (audit reproduction / knowledge capture)

- **summary**: one line for indexing; must include "what + where + how to trigger/validate" — do not write only the conclusion (e.g. just "SQLi exists").
- **body**: full reproducible context; written to the body field of ` + "`upsert_project_fact`" + `; index does not contain body — subsequent sessions must call ` + "`get_project_fact`" + ` to retrieve it.
- **category / fact_key suggestions**:
  - Environment/recon: ` + "`target/`" + `, ` + "`auth/`" + `, ` + "`infra/`" + `, ` + "`business/`" + ` (body can use environment template)
  - Discovery and exploitation: ` + "`finding/`" + `, ` + "`chain/`" + `, ` + "`exploit/`" + `, ` + "`poc/`" + ` (**must** fill b with attack chain templateody：入口、逐步attack chain、原始request/response或命令、证据、关联漏洞 ID）
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
	b.WriteString("- **Discovery and exploitation context** (audit reproduction): fact_key recommended prefixes: finding/, chain/, exploit/, poc/; **body is required** with full attack chain (entry → 步骤 → 原始request/response或命令 → 现象 → 关联 related_vulnerability_id），**禁止仅写结论**；summary 写「什么 + 在哪 + 如何validate」一行要点。\n")
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
	b.WriteString("If the current conversation has a bound project, the system automatically injects the project blackboard index (fact_key + summary only). **When the summary is insufficient, you must call `get_project_fact(fact_key)` 获取 body，禁止凭summary臆造细节。**\n\n")
	b.WriteString(FactRecordingIncrementalRhythmMarkdown(coordinatorDelegate, false))
	b.WriteString("\n\n")
	b.WriteString("- **Environment/target/auth findings** (non-formal vulnerabilities): use **`upsert_project_fact`**, `fact_key` recommended as `category/slug` (e.g. `target/primary_domain`），同 key 覆盖update；body 记port/版本/凭据特征与证据来源。\n")
	b.WriteString("- **Discovery and exploitation context** (audit reproduction): `fact_key` recommended prefixes `finding/`, `chain/`, `exploit/`, `poc/`; **body is required** with full attack chain（入口 → 步骤 → 原始request/response或命令 → 现象 → 关联 `related_vulnerability_id`），**禁止仅写结论**；summary 写「什么 + 在哪 + 如何validate」一行要点。\n")
	b.WriteString("- **Deliverable vulnerabilities**: use **`record_vulnerability`** (title, description, severity, type, target, proof POC, impact, Remediation advice）。critical程度 critical / high / medium / low / info。\n")
	b.WriteString("- The same discovery may need to be **recorded in both** (fact records the reproducible attack chain; vulnerability records the formal finding). For false positives use **`deprecate_project_fact`** 或vulnerability status false_positive。\n")
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
