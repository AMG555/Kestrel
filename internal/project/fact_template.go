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

// RequiresAttackChainBody 判断该事实yesno应携带可复现的attack chain / exploit details（写在 body，非仅 summary）。
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

// IsSparseFactBody attack chain类事实 body 过短或缺少关key段落时back true（软校验，不Block写入）。
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
	// noneattack chain/POC/request等结构线索，视为仅结论性description（不论长短）
	return !(hasSteps || hasHTTP || hasReq)
}

// FactBodyTemplate returns the recommended body Markdown skeleton for the given category (for the Agent to fill in real content).
func FactBodyTemplate(category, factKey string) string {
	if RequiresAttackChainBody(category, factKey) {
		return attackChainFactBodyTemplate
	}
	return envFactBodyTemplate
}

const attackChainFactBodyTemplate = `## 结论（可validate，一句话）
<勿仅写「存在漏洞」；写明type + 位置 + 触发条件>

## Target and Entry Point
- Target: <URL / IP:Port / hostname>
- Entry point: <path / interface / parameter>
- Prerequisites: <anonymous / role / Cookie / other dependencies>

## Attack Chain (step-by-step reproducible)
1. <侦察/Discovery>
2. <exploitation/trigger>
3. <impact证明（读file、RCE 回显、越权数据等）>

## Exploit / POC
### request
` + "```http\n<METHOD> <path> HTTP/1.1\nHost: ...\n...\n\n<body>\n```" + `

### Response / Observed Behaviour
<关keyresponse片段、status码、差异点>

### Command / Script (if any)
` + "```bash\n<command>\n```" + `

## Key Evidence
- <tool outputsummary / 截图path / 会话或message ID>

## Associations
- related_vulnerability_id: <optional, corresponds to the id from record_vulnerability>
- links（upsert 参数）: [{ "from": "<fact_key>", "type": "discovered_on|..." }]（from → 当前 fact）
- dependent fact (body readable mirror): <fact_key, e.g. auth/session_cookie>

## Remark与不OK性
<待validatefalse设、环境差异、绕过尝试记录>`

const envFactBodyTemplate = `## summary
<core insight of this fact>

## Details
<port/version/path/credential characteristics/business rules/etc.>

## Source and Evidence
<command output, response snippet, discovery timestamp>

## Associations
- related fact_key: <optional>`

// FactRecordingGuidanceBlock 写入system prompt：要求事实沉淀attack chain上下文而非仅结论。
func FactRecordingGuidanceBlock() string {
	return projectprompt.FactRecordingGuidanceBlock()
}

// SparseBodyWarning attack chain类事实 body 不足时的toolback提示（不Blocksave）。
func SparseBodyWarning(category, factKey string) string {
	if !IsSparseFactBody(category, factKey, "") {
		return ""
	}
	return fmt.Sprintf(
		"\n\n⚠ 提示：category=%q / fact_key=%q 属于attack chain类事实，但 body 为null或过简。请补充完整attack chain与 POC（参考模板），便于后续审计复现。\n建议 body 骨架：\n%s",
		category, factKey, FactBodyTemplate(category, factKey),
	)
}

// SparseBodyWarningIfNeeded 根据实际 body 判断yesno追加warning。
func SparseBodyWarningIfNeeded(category, factKey, body string) string {
	if !IsSparseFactBody(category, factKey, body) {
		return ""
	}
	return SparseBodyWarning(category, factKey)
}
