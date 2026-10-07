package project

import (
	"fmt"
	"strings"

	"kestrel/internal/projectprompt"
)

// Fact category constants (written to the category field of upsert_project_fact).
const (
	FactCategoryTarget   = "target"
	FactCategoryAuth     = "auth"
	FactCategoryInfra    = "infra"
	FactCategoryBusiness = "business"
	FactCategoryFinding  = "finding"
	FactCategoryChain    = "chain"
	FactCategoryExploit  = "exploit"
	FactCategoryPOC      = "poc"
	FactCategoryNote     = "note"
)

// RequiresAttackChainBody returns true when the fact should carry a reproducible attack chain / exploit details in the body (not just a summary).
func RequiresAttackChainBody(category, factKey string) bool {
	c := strings.ToLower(strings.TrimSpace(category))
	switch c {
	case FactCategoryFinding, FactCategoryChain, FactCategoryExploit, FactCategoryPOC, "vuln":
		return true
	}
	key := strings.ToLower(strings.TrimSpace(factKey))
	for _, prefix := range []string{"finding/", "chain/", "exploit/", "poc/"} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// IsSparseFactBody returns true when an attack-chain fact's body is too short or missing key sections (soft check, does not block writes).
func IsSparseFactBody(category, factKey, body string) bool {
	if !RequiresAttackChainBody(category, factKey) {
		return false
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return true
	}
	lower := strings.ToLower(body)
	// Must contain at least one reproducible clue: steps/request/command/code block
	hasSteps := strings.Contains(lower, "attack chain") || strings.Contains(lower, "## attack") ||
		strings.Contains(lower, "## exploit") || strings.Contains(lower, "## poc")
	hasHTTP := strings.Contains(lower, "```http") || strings.Contains(lower, "```bash") ||
		strings.Contains(lower, "curl ") || strings.Contains(lower, "get ") || strings.Contains(lower, "post ")
	hasReq := strings.Contains(lower, "request") || strings.Contains(lower, "response") || strings.Contains(lower, "payload")
	// No structural clues such as attack chain/POC/request — treated as a conclusion-only description (regardless of length)
	return !(hasSteps || hasHTTP || hasReq)
}

// FactBodyTemplate returns the recommended body Markdown skeleton for the given category (for the Agent to fill in real content).
func FactBodyTemplate(category, factKey string) string {
	if RequiresAttackChainBody(category, factKey) {
		return attackChainFactBodyTemplate
	}
	return envFactBodyTemplate
}

const attackChainFactBodyTemplate = `## Conclusion (verifiable, one sentence)
<Do not just write "vulnerability exists"; specify type + location + trigger condition>

## Target and Entry Point
- Target: <URL / IP:Port / hostname>
- Entry point: <path / interface / parameter>
- Prerequisites: <anonymous / role / Cookie / other dependencies>

## Attack Chain (step-by-step reproducible)
1. <Reconnaissance/Discovery>
2. <Exploitation/trigger>
3. <Impact proof (file read, RCE output, privilege-escalation data, etc.)>

## Exploit / POC
### Request
` + "```http\n<METHOD> <path> HTTP/1.1\nHost: ...\n...\n\n<body>\n```" + `

### Response / Observed Behaviour
<Key response fragments, status code, differences>

### Command / Script (if any)
` + "```bash\n<command>\n```" + `

## Key Evidence
- <Tool output summary / screenshot path / session or message ID>

## Associations
- related_vulnerability_id: <optional, corresponds to the id from record_vulnerability>
- links (upsert parameter): [{ "from": "<fact_key>", "type": "discovered_on|..." }] (from → current fact)
- dependent fact (body readable mirror): <fact_key, e.g. auth/session_cookie>

## Remarks and Open Questions
<Pending validation hypotheses, environment differences, bypass attempt records>`

const envFactBodyTemplate = `## summary
<core insight of this fact>

## Details
<port/version/path/credential characteristics/business rules/etc.>

## Source and Evidence
<command output, response snippet, discovery timestamp>

## Associations
- related fact_key: <optional>`

// FactRecordingGuidanceBlock writes the fact recording guidance to the system prompt, requiring attack chain context rather than conclusions only.
func FactRecordingGuidanceBlock() string {
	return projectprompt.FactRecordingGuidanceBlock()
}

// SparseBodyWarning returns a tool-response hint when an attack-chain fact's body is insufficient (does not block saving).
func SparseBodyWarning(category, factKey string) string {
	if !IsSparseFactBody(category, factKey, "") {
		return ""
	}
	return fmt.Sprintf(
		"\n\n⚠ Hint: category=%q / fact_key=%q is an attack-chain fact, but the body is empty or too brief. Please add a complete attack chain and POC (refer to the template) to aid future audit reproduction.\nSuggested body skeleton:\n%s",
		category, factKey, FactBodyTemplate(category, factKey),
	)
}

// SparseBodyWarningIfNeeded checks the actual body and appends a warning if needed.
func SparseBodyWarningIfNeeded(category, factKey, body string) string {
	if !IsSparseFactBody(category, factKey, body) {
		return ""
	}
	return SparseBodyWarning(category, factKey)
}
