package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"kestrel/internal/config"
	"kestrel/internal/openai"

	"go.uber.org/zap"
)

type DraftTool struct {
	Key     string `json:"key"`
	Name    string `json:"name,omitempty"`
	Enabled bool   `json:"enabled"`
}

type DraftOptions struct {
	IncludeObjective bool `json:"include_objective"`
	AllowSchedule    bool `json:"allow_schedule"`
	AllowHighRisk    bool `json:"allow_high_risk"`
}

type DraftRequest struct {
	Prompt         string       `json:"prompt"`
	Options        DraftOptions `json:"options"`
	AvailableTools []DraftTool  `json:"available_tools,omitempty"`
}

type DraftMeta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type DraftCapability struct {
	Label          string   `json:"label"`
	ToolName       string   `json:"tool_name,omitempty"`
	ToolCandidates []string `json:"tool_candidates,omitempty"`
}

type DraftAudit struct {
	Savable       bool     `json:"savable"`
	Validation    []string `json:"validation,omitempty"`
	MissingFields []string `json:"missing_fields,omitempty"`
	RiskWarnings  []string `json:"risk_warnings,omitempty"`
	Assumptions   []string `json:"assumptions,omitempty"`
	HighRisk      bool     `json:"high_risk"`
	NeedsHITL     bool     `json:"needs_hitl"`
}

type DraftResult struct {
	Graph        *graphDef         `json:"graph"`
	Meta         DraftMeta         `json:"meta"`
	Generator    string            `json:"generator"`
	Audit        DraftAudit        `json:"audit"`
	Capabilities []DraftCapability `json:"capabilities,omitempty"`
	Stats        map[string]int    `json:"stats"`
}

type llmDraftEnvelope struct {
	Graph        graphDef          `json:"graph"`
	Meta         DraftMeta         `json:"meta"`
	Capabilities []DraftCapability `json:"capabilities,omitempty"`
	Audit        DraftAudit        `json:"audit,omitempty"`
}

type draftToolHint struct {
	Label    string
	Keywords []string
	Tools    []string
}

var draftToolHints = []draftToolHint{
	{Label: "Subdomain Discovery", Keywords: []string{"子域名", "subdomain", "subfinder", "amass"}, Tools: []string{"subfinder", "amass"}},
	{Label: "portscan", Keywords: []string{"port", "port", "nmap", "rustscan", "masscan"}, Tools: []string{"nmap", "rustscan", "masscan"}},
	{Label: "Vulnerability Scan", Keywords: []string{"漏洞", "vuln", "漏洞scan", "nuclei", "nikto", "zap"}, Tools: []string{"nuclei", "nikto", "zap"}},
	{Label: "Exposed Surface Detection", Keywords: []string{"directory", "path", "暴露页面", "dir", "ffuf", "gobuster", "feroxbuster"}, Tools: []string{"ffuf", "gobuster", "feroxbuster", "dirsearch"}},
	{Label: "Certificate and Domain Intelligence Collection", Keywords: []string{"证书", "certificate", "crt"}, Tools: []string{"subfinder"}},
	{Label: "Cloud Config Audit", Keywords: []string{"云", "cloud", "config审计", "prowler", "scout"}, Tools: []string{"prowler", "scout-suite"}},
	{Label: "Container Security Check", Keywords: []string{"容器", "镜像", "k8s", "kubernetes", "trivy", "kube"}, Tools: []string{"trivy", "kube-bench", "kube-hunter"}},
	{Label: "Threat Intelligence Collection", Keywords: []string{"情报", "威胁情报", "threat", "ioc", "virustotal", "shodan", "fofa"}, Tools: []string{"virustotal_search", "shodan_search", "fofa_search"}},
}

var highRiskDraftRE = regexp.MustCompile(`(?i)(isolate|block|harden|fix|execute|command|script|delete|cleanup|ban|attack|exploit|getshell|shell|payload|exploit|isolate|block|execute|script|delete|exploit|payload)`)

func GenerateDraftFromNaturalLanguage(ctx context.Context, req DraftRequest) (*DraftResult, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("workflow requirements cannot be empty")
	}
	capabilities := detectDraftCapabilities(prompt, req.AvailableTools)
	wantsApproval := containsAnyFold(prompt, "review", "confirm", "approve", "responsible", "manual", "approval", "human")
	wantsReport := containsAnyFold(prompt, "report", "summary", "output", "notification", "task", "ticket", "summary", "notify", "ticket")
	wantsCondition := containsAnyFold(prompt, "if", "discovery", "exists", "critical", "new", "failed", "pass", "else", "when", "high", "critical", "new", "fail")
	highRisk := highRiskDraftRE.MatchString(prompt)

	builder := &draftGraphBuilder{x: 120, y: 150}
	assumptions := make([]string, 0)
	riskWarnings := make([]string, 0)
	missingFields := make([]string, 0)

	start := builder.add("start", "Start", map[string]any{"input_keys": "message, conversationId, projectId, target"}, 0)
	previous := start
	for _, capability := range capabilities {
		hasTool := strings.TrimSpace(capability.ToolName) != ""
		var id string
		if hasTool {
			id = builder.add("tool", capability.Label, map[string]any{
				"tool_name":       capability.ToolName,
				"arguments":       `{"target":"{{inputs.target}}","message":"{{inputs.message}}"}`,
				"timeout_seconds": "120",
				"join_strategy":   "all_merge",
			}, 0)
		} else {
			id = builder.add("agent", capability.Label, map[string]any{
				"agent_mode":              "eino_single",
				"input_binding":           map[string]any{"from": "previous", "field": "output"},
				"instruction":             capability.Label + ". Execute security process steps according to user requirements and output structured results: " + prompt,
				"output_key":              "agent_result",
				"join_strategy":           "all_merge",
				"missing_tool_candidates": strings.Join(capability.ToolCandidates, ", "),
			}, 0)
			if len(capability.ToolCandidates) > 0 {
				assumptions = append(assumptions, capability.Label+" did not match any enabled tool; an Agent draft node was generated.")
				missingFields = append(missingFields, capability.Label+": select or enable the corresponding MCP tool")
			}
		}
		builder.connect(previous, id, "", nil)
		previous = id
	}

	openConditionID := ""
	if wantsCondition {
		expr := `{{previous.output}} != ""`
		label := "Condition met?"
		if highRisk {
			expr = `{{previous.output}} contains "critical"`
			label = "High-risk action required?"
		}
		condition := builder.add("condition", label, map[string]any{"expression": expr, "join_strategy": "all_merge"}, 0)
		builder.connect(previous, condition, "", nil)
		openConditionID = condition
		report := builder.add("output", draftOutputLabel(wantsReport), map[string]any{
			"output_key":     "result",
			"source_binding": map[string]any{"from": "previous", "field": "output"},
			"static_value":   "",
			"join_strategy":  "all_merge",
		}, 130)
		builder.connect(condition, report, "no", map[string]any{"condition": `{{previous.matched}} == "false"`, "branch": "false"})
		previous = condition
	}

	insertedHITL := false
	if highRisk {
		if !req.Options.AllowHighRisk || wantsApproval {
			approval := builder.add("hitl", "human approval", map[string]any{
				"prompt":         "Please confirm whether to allow high-risk remediation to proceed: " + prompt,
				"prompt_binding": map[string]any{"from": "previous", "field": "output"},
				"reviewer":       "human",
				"join_strategy":  "all_merge",
				"risk_level":     "high",
			}, 0)
			builder.connect(previous, approval, branchLabel(previous, openConditionID), branchConfig(previous, openConditionID, true))
			if previous == openConditionID {
				openConditionID = ""
			}
			previous = approval
			insertedHITL = true
		}
		action := builder.add("agent", "Execute Controlled Remediation", map[string]any{
			"agent_mode":                  "eino_single",
			"input_binding":               map[string]any{"from": "previous", "field": "output"},
			"instruction":                 "Only generate remediation step drafts within the authorized scope; must be confirmed manually before actual execution. User requirements: " + prompt,
			"output_key":                  "remediation_plan",
			"join_strategy":               "all_merge",
			"risk_level":                  "high",
			"requires_human_confirmation": "true",
		}, 0)
		builder.connect(previous, action, branchLabel(previous, openConditionID), branchConfig(previous, openConditionID, true))
		if previous == openConditionID {
			openConditionID = ""
		}
		previous = action
		if insertedHITL {
			riskWarnings = append(riskWarnings, "High-risk actions detected; added human approval and requires_human_confirmation markers.")
		} else {
			riskWarnings = append(riskWarnings, "High-risk actions detected; kept as draft and added requires_human_confirmation marker.")
		}
	} else if wantsApproval {
		approval := builder.add("hitl", "human approval", map[string]any{
			"prompt":         "Please review the workflow phase result: " + prompt,
			"prompt_binding": map[string]any{"from": "previous", "field": "output"},
			"reviewer":       "human",
			"join_strategy":  "all_merge",
		}, 0)
		builder.connect(previous, approval, "", nil)
		previous = approval
		insertedHITL = true
	}

	output := builder.add("output", draftOutputLabel(wantsReport), map[string]any{
		"output_key":     "result",
		"source_binding": map[string]any{"from": "previous", "field": "output"},
		"static_value":   "",
		"join_strategy":  "all_merge",
	}, 0)
	builder.connect(previous, output, branchLabel(previous, openConditionID), branchConfig(previous, openConditionID, true))

	graph := &graphDef{Nodes: builder.nodes, Edges: builder.edges, Config: map[string]any{
		"schema_version": 1,
		"generated_by":   "natural_language",
		"source_prompt":  prompt,
	}}
	if req.Options.IncludeObjective {
		graph.Config["objective"] = prompt
	}
	if req.Options.AllowSchedule && containsAnyFold(prompt, "daily", "weekly", "scheduled", "periodic", "continuous", "every day", "every week", "schedule", "monitor") {
		if containsAnyFold(prompt, "daily", "every day") {
			graph.Config["trigger_suggestion"] = "daily"
		} else {
			graph.Config["trigger_suggestion"] = "scheduled"
		}
		assumptions = append(assumptions, "Scheduled trigger recommendation recorded; still needs configuration in triggers or role binding after saving.")
	}

	raw, _ := json.Marshal(graph)
	validation := make([]string, 0)
	if err := ValidateGraphJSON(ctx, string(raw)); err != nil {
		validation = append(validation, err.Error())
	}
	return &DraftResult{
		Graph:     graph,
		Meta:      DraftMeta{ID: draftSlug(prompt), Name: draftName(prompt), Description: prompt, Enabled: true},
		Generator: "deterministic",
		Audit: DraftAudit{
			Savable:       len(validation) == 0,
			Validation:    validation,
			MissingFields: missingFields,
			RiskWarnings:  riskWarnings,
			Assumptions:   assumptions,
			HighRisk:      highRisk,
			NeedsHITL:     insertedHITL,
		},
		Capabilities: capabilities,
		Stats:        map[string]int{"nodes": len(graph.Nodes), "edges": len(graph.Edges)},
	}, nil
}

func GenerateDraftFromLLM(ctx context.Context, req DraftRequest, oa config.OpenAIConfig, logger *zap.Logger) (*DraftResult, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("workflow requirements cannot be empty")
	}
	if strings.TrimSpace(oa.APIKey) == "" || strings.TrimSpace(oa.Model) == "" {
		return nil, fmt.Errorf("AI channel has no api_key or model configured")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	callCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	toolJSON, _ := json.Marshal(req.AvailableTools)
	systemPrompt := `You are Kestrel's workflow orchestration assistant. You must convert the user's one-sentence requirement into a saveable Workflow draft JSON.
Return only a JSON object, no Markdown, no explanation. The JSON must conform to:
{
  "meta": {"id":"kebab-case-id","name":"short name","description":"user requirement","enabled":true},
	"graph": {
    "nodes": [{"id":"start-1","type":"start","label":"display name","position":{"x":120,"y":150},"config":{}}],
    "edges": [{"id":"edge-1","source":"start-1","target":"node-2","label":"","config":{}}],
    "config": {"schema_version":1,"generated_by":"llm","source_prompt":"original user prompt"}
  },
  "capabilities": [{"label":"capability name","tool_name":"matched tool name","tool_candidates":["candidate tools"]}],
  "audit": {"assumptions":[],"missing_fields":[],"risk_warnings":[]}
}
Hard rules:
- Only output one valid JSON object; do not output JSON Schema, comments, explanatory text, Markdown code blocks, or extra prefixes/suffixes.
- Do not use pipe-delimited enumerations in JSON string values; the type field can only contain one node type string at a time.
- At least 1 start and 1 output node; output/end nodes cannot have outgoing edges.
- Node type can only be one of: start, tool, agent, condition, hitl, output, end.
- Each agent, tool, and output node must configure a unique output_key; output nodes default to result.
- agent nodes must configure instruction or input_binding; default input_binding is {"from":"previous","field":"output"}.
- output nodes must configure source_binding or static_value; default source_binding is {"from":"previous","field":"output"}.
- tool nodes must configure tool_name, arguments, timeout_seconds; arguments must be a valid JSON string.
- All non-start nodes that may have multiple upstream nodes must configure join_strategy:"all_merge".
- condition nodes have at most 2 outgoing edges; must use branch true/false with label yes/no.
- tool nodes are only used when an enabled tool exists in available_tools; otherwise use an agent node and note the missing tool in audit.missing_fields.
- High-risk actions (script execution, isolation, blocking, delete, exploitation, payload, command execution, etc.) must include hitl approval, or mark requires_human_confirmation:"true"、risk_level:"high"。
- Do not generate parameters that would actually execute attacks; use {{inputs.target}}, {{inputs.message}} as placeholders in tool parameters.
- 所有节点 config 加 generated_by:"llm" 和 needs_review:"true"。`
	userPrompt := fmt.Sprintf("user需求：%s\n\n选项：%+v\n\n可用tool JSON：%s", prompt, req.Options, string(toolJSON))
	requestBody := map[string]interface{}{
		"model": strings.TrimSpace(oa.Model),
		"messages": []map[string]interface{}{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature":           0,
		"max_completion_tokens": 4096,
		"response_format":       map[string]interface{}{"type": "json_object"},
		"thinking":              map[string]interface{}{"type": "disabled"},
	}
	var apiResponse struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	client := openai.NewClient(&oa, nil, logger)
	if err := client.ChatCompletion(callCtx, requestBody, &apiResponse); err != nil {
		return nil, fmt.Errorf("调用大modelfailed: %w", err)
	}
	if len(apiResponse.Choices) == 0 {
		return nil, fmt.Errorf("大model未back候选结果")
	}
	raw := strings.TrimSpace(apiResponse.Choices[0].Message.Content)
	if raw == "" {
		raw = strings.TrimSpace(apiResponse.Choices[0].Message.ReasoningContent)
	}
	env, err := parseLLMDraftEnvelope(raw)
	if err != nil {
		return nil, err
	}
	result := normalizeLLMDraft(prompt, req, env)
	graphRaw, _ := json.Marshal(result.Graph)
	validation := make([]string, 0)
	if err := ValidateGraphJSON(ctx, string(graphRaw)); err != nil {
		validation = append(validation, err.Error())
	}
	result.Audit.Validation = validation
	result.Audit.Savable = len(validation) == 0
	if !result.Audit.Savable {
		return nil, fmt.Errorf("大model生成的工作流未通过校验: %s", strings.Join(validation, "；"))
	}
	return result, nil
}

type draftGraphBuilder struct {
	nodes   []graphNode
	edges   []graphEdge
	x       float64
	y       float64
	nodeSeq int
	edgeSeq int
}

func (b *draftGraphBuilder) add(nodeType, label string, config map[string]any, yOffset float64) string {
	b.nodeSeq++
	id := fmt.Sprintf("%s-%d", nodeType, b.nodeSeq)
	if config == nil {
		config = make(map[string]any)
	}
	config["generated_by"] = "natural_language"
	config["needs_review"] = "true"
	b.nodes = append(b.nodes, graphNode{
		ID:       id,
		Type:     nodeType,
		Label:    label,
		Position: graphPosition{X: b.x, Y: b.y + yOffset},
		Config:   config,
	})
	b.x += 210
	return id
}

func (b *draftGraphBuilder) connect(source, target, label string, config map[string]any) {
	b.edgeSeq++
	if config == nil {
		config = make(map[string]any)
	}
	b.edges = append(b.edges, graphEdge{ID: fmt.Sprintf("edge-ai-%d", b.edgeSeq), Source: source, Target: target, Label: label, Config: config})
}

func parseLLMDraftEnvelope(raw string) (llmDraftEnvelope, error) {
	var lastErr error
	for _, candidate := range jsonObjectCandidates(raw) {
		var env llmDraftEnvelope
		if err := json.Unmarshal([]byte(candidate), &env); err == nil {
			if len(env.Graph.Nodes) == 0 {
				lastErr = fmt.Errorf("大model JSON 缺少 graph.nodes")
				continue
			}
			return env, nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("大empty model response")
	}
	return llmDraftEnvelope{}, fmt.Errorf("解析大model工作流 JSON failed: %w", lastErr)
}

func jsonObjectCandidates(raw string) []string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	candidates := []string{s}
	if start := strings.Index(s, "{"); start >= 0 {
		if end := strings.LastIndex(s, "}"); end > start {
			candidates = append(candidates, s[start:end+1])
		}
	}
	return candidates
}

func normalizeLLMDraft(prompt string, req DraftRequest, env llmDraftEnvelope) *DraftResult {
	g := env.Graph
	if g.Config == nil {
		g.Config = make(map[string]any)
	}
	g.Config["schema_version"] = 1
	g.Config["generated_by"] = "llm"
	g.Config["source_prompt"] = prompt
	if req.Options.IncludeObjective {
		g.Config["objective"] = prompt
	}
	enabledTools := enabledDraftToolNames(req.AvailableTools)
	usedOutputKeys := make(map[string]bool)
	nodeTypes := make(map[string]string, len(g.Nodes))
	for i := range g.Nodes {
		if strings.TrimSpace(g.Nodes[i].ID) == "" {
			g.Nodes[i].ID = fmt.Sprintf("%s-%d", firstNonEmpty(g.Nodes[i].Type, "node"), i+1)
		}
		if strings.TrimSpace(g.Nodes[i].Type) == "" {
			g.Nodes[i].Type = "agent"
		}
		if strings.TrimSpace(g.Nodes[i].Label) == "" {
			g.Nodes[i].Label = displayNodeType(g.Nodes[i].Type)
		}
		if g.Nodes[i].Position.X == 0 && g.Nodes[i].Position.Y == 0 {
			g.Nodes[i].Position = graphPosition{X: 120 + float64(i)*210, Y: 150}
		}
		if g.Nodes[i].Config == nil {
			g.Nodes[i].Config = make(map[string]any)
		}
		g.Nodes[i].Config["generated_by"] = "llm"
		g.Nodes[i].Config["needs_review"] = "true"
		normalizeLLMNodeConfig(prompt, &g.Nodes[i], enabledTools, usedOutputKeys)
		nodeTypes[g.Nodes[i].ID] = strings.ToLower(strings.TrimSpace(g.Nodes[i].Type))
	}
	conditionBranchCounts := make(map[string]int)
	for i := range g.Edges {
		if strings.TrimSpace(g.Edges[i].ID) == "" {
			g.Edges[i].ID = fmt.Sprintf("edge-llm-%d", i+1)
		}
		if g.Edges[i].Config == nil {
			g.Edges[i].Config = make(map[string]any)
		}
		normalizeLLMEdgeConfig(&g.Edges[i], nodeTypes, conditionBranchCounts)
	}
	audit := env.Audit
	highRisk := highRiskDraftRE.MatchString(prompt) || graphHasHighRisk(g)
	audit.HighRisk = highRisk
	audit.NeedsHITL = graphHasNodeType(g, "hitl")
	if highRisk && !audit.NeedsHITL && !graphHasConfirmation(g) {
		audit.RiskWarnings = append(audit.RiskWarnings, "大model生成包含高风险语义，请补充human approval或confirm标记后再运行。")
	}
	if len(audit.RiskWarnings) == 0 && highRisk {
		audit.RiskWarnings = append(audit.RiskWarnings, "检测到高风险动作，已标记为需要重点审计。")
	}
	meta := env.Meta
	if strings.TrimSpace(meta.Description) == "" {
		meta.Description = prompt
	}
	if strings.TrimSpace(meta.Name) == "" {
		meta.Name = draftName(prompt)
	}
	if strings.TrimSpace(meta.ID) == "" {
		meta.ID = draftSlug(prompt)
	}
	meta.Enabled = true
	return &DraftResult{
		Graph:        &g,
		Meta:         meta,
		Generator:    "llm",
		Audit:        audit,
		Capabilities: env.Capabilities,
		Stats:        map[string]int{"nodes": len(g.Nodes), "edges": len(g.Edges)},
	}
}

func normalizeLLMEdgeConfig(edge *graphEdge, nodeTypes map[string]string, conditionBranchCounts map[string]int) {
	if nodeTypes[strings.TrimSpace(edge.Source)] != "condition" {
		return
	}
	if conditionBranchHint(*edge) != "" {
		return
	}
	conditionBranchCounts[edge.Source]++
	branch := "true"
	label := "yes"
	if conditionBranchCounts[edge.Source] > 1 {
		branch = "false"
		label = "no"
	}
	edge.Label = label
	edge.Config["branch"] = branch
}

func normalizeLLMNodeConfig(prompt string, node *graphNode, enabledTools map[string]bool, usedOutputKeys map[string]bool) {
	nodeType := strings.ToLower(strings.TrimSpace(node.Type))
	switch nodeType {
	case "start":
		if cfgString(node.Config, "input_keys") == "" {
			node.Config["input_keys"] = "message, conversationId, projectId, target"
		}
	case "tool":
		toolName := cfgString(node.Config, "tool_name")
		if toolName == "" || !enabledTools[strings.ToLower(toolName)] {
			node.Type = "agent"
			node.Config["missing_tool_name"] = toolName
			normalizeAgentDraftConfig(prompt, node, usedOutputKeys)
			return
		}
		if cfgString(node.Config, "arguments") == "" {
			node.Config["arguments"] = `{"target":"{{inputs.target}}","message":"{{inputs.message}}"}`
		}
		if cfgString(node.Config, "timeout_seconds") == "" {
			node.Config["timeout_seconds"] = "120"
		}
		ensureNodeOutputKey(node, usedOutputKeys, draftOutputKeyBase(node, "tool_result"))
		ensureJoinStrategy(node)
	case "agent":
		normalizeAgentDraftConfig(prompt, node, usedOutputKeys)
	case "condition":
		if cfgString(node.Config, "expression") == "" {
			node.Config["expression"] = `{{previous.output}} != ""`
		}
		ensureJoinStrategy(node)
	case "hitl":
		if cfgString(node.Config, "prompt") == "" {
			node.Config["prompt"] = "请审核工作流阶段结果：" + prompt
		}
		if cfgString(node.Config, "reviewer") == "" {
			node.Config["reviewer"] = "human"
		}
		ensureJoinStrategy(node)
	case "output":
		ensureNodeOutputKey(node, usedOutputKeys, "result")
		if cfgString(node.Config, "static_value") == "" {
			if _, ok := parseFieldBinding(node.Config, "source_binding"); !ok {
				node.Config["source_binding"] = map[string]any{"from": "previous", "field": "output"}
			}
		}
		ensureJoinStrategy(node)
	case "end":
		ensureJoinStrategy(node)
	}
}

func normalizeAgentDraftConfig(prompt string, node *graphNode, usedOutputKeys map[string]bool) {
	if cfgString(node.Config, "agent_mode") == "" {
		node.Config["agent_mode"] = "eino_single"
	}
	if cfgString(node.Config, "instruction") == "" {
		node.Config["instruction"] = node.Label + "。根据user需求执行安全流程步骤，并输出结构化结果：" + prompt
	}
	if _, ok := parseFieldBinding(node.Config, "input_binding"); !ok {
		node.Config["input_binding"] = map[string]any{"from": "previous", "field": "output"}
	}
	ensureNodeOutputKey(node, usedOutputKeys, draftOutputKeyBase(node, "agent_result"))
	ensureJoinStrategy(node)
}

func ensureJoinStrategy(node *graphNode) {
	if cfgString(node.Config, "join_strategy") == "" {
		node.Config["join_strategy"] = "all_merge"
	}
}

func ensureNodeOutputKey(node *graphNode, used map[string]bool, fallback string) {
	current := sanitizeOutputKey(cfgString(node.Config, "output_key"))
	if current == "" {
		current = sanitizeOutputKey(fallback)
	}
	if current == "" {
		current = "result"
	}
	base := current
	for i := 2; used[current]; i++ {
		current = fmt.Sprintf("%s_%d", base, i)
	}
	node.Config["output_key"] = current
	used[current] = true
}

func draftOutputKeyBase(node *graphNode, fallback string) string {
	if name := cfgString(node.Config, "tool_name"); name != "" {
		return name + "_result"
	}
	if node.ID != "" {
		return node.ID + "_result"
	}
	return fallback
}

func sanitizeOutputKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if b.Len() > 0 && !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func enabledDraftToolNames(tools []DraftTool) map[string]bool {
	names := make(map[string]bool, len(tools)*2)
	for _, tool := range tools {
		if !tool.Enabled {
			continue
		}
		if key := strings.ToLower(strings.TrimSpace(tool.Key)); key != "" {
			names[key] = true
		}
		if name := strings.ToLower(strings.TrimSpace(tool.Name)); name != "" {
			names[name] = true
		}
	}
	return names
}

func graphHasNodeType(g graphDef, nodeType string) bool {
	for _, node := range g.Nodes {
		if strings.EqualFold(node.Type, nodeType) {
			return true
		}
	}
	return false
}

func graphHasConfirmation(g graphDef) bool {
	for _, node := range g.Nodes {
		if cfgString(node.Config, "requires_human_confirmation") == "true" {
			return true
		}
	}
	return false
}

func graphHasHighRisk(g graphDef) bool {
	for _, node := range g.Nodes {
		if cfgString(node.Config, "risk_level") == "high" || cfgString(node.Config, "requires_human_confirmation") == "true" {
			return true
		}
		if highRiskDraftRE.MatchString(node.Label) || highRiskDraftRE.MatchString(cfgString(node.Config, "instruction")) {
			return true
		}
	}
	return false
}

func detectDraftCapabilities(prompt string, tools []DraftTool) []DraftCapability {
	capabilities := make([]DraftCapability, 0)
	for _, hint := range draftToolHints {
		if containsAnyFold(prompt, hint.Keywords...) {
			capabilities = append(capabilities, DraftCapability{
				Label:          hint.Label,
				ToolName:       matchDraftTool(hint.Tools, tools),
				ToolCandidates: append([]string(nil), hint.Tools...),
			})
		}
	}
	if len(capabilities) == 0 {
		capabilities = append(capabilities, DraftCapability{Label: "节点能力", ToolCandidates: nil})
	}
	return capabilities
}

func matchDraftTool(candidates []string, tools []DraftTool) string {
	if len(candidates) == 0 || len(tools) == 0 {
		return ""
	}
	for _, enabledOnly := range []bool{true, false} {
		for _, candidate := range candidates {
			candidate = strings.ToLower(strings.TrimSpace(candidate))
			for _, tool := range tools {
				if enabledOnly && !tool.Enabled {
					continue
				}
				key := strings.ToLower(strings.TrimSpace(firstNonEmpty(tool.Key, tool.Name)))
				if key != "" && strings.Contains(key, candidate) {
					return firstNonEmpty(tool.Key, tool.Name)
				}
			}
		}
	}
	return ""
}

func containsAnyFold(text string, needles ...string) bool {
	lower := strings.ToLower(text)
	for _, needle := range needles {
		if strings.Contains(lower, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func draftOutputLabel(wantsReport bool) string {
	if wantsReport {
		return "输出report"
	}
	return "输出"
}

func branchLabel(source, conditionID string) string {
	if source == conditionID && conditionID != "" {
		return "yes"
	}
	return ""
}

func branchConfig(source, conditionID string, yes bool) map[string]any {
	if source != conditionID || conditionID == "" {
		return nil
	}
	if yes {
		return map[string]any{"condition": `{{previous.matched}} == "true"`, "branch": "true"}
	}
	return map[string]any{"condition": `{{previous.matched}} == "false"`, "branch": "false"}
}

func draftName(prompt string) string {
	runes := []rune(strings.TrimSpace(prompt))
	if len(runes) > 22 {
		return string(runes[:22]) + "..."
	}
	return string(runes)
}

func draftSlug(prompt string) string {
	lower := strings.ToLower(strings.TrimSpace(prompt))
	var b strings.Builder
	lastDash := false
	for _, r := range lower {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug != "" {
		if len(slug) > 48 {
			return strings.Trim(slug[:48], "-")
		}
		return slug
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(lower))
	if !utf8.ValidString(lower) || lower == "" {
		lower = "workflow"
	}
	return fmt.Sprintf("ai-workflow-%x", h.Sum32())
}
