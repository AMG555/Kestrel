package attackchain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"kestrel/internal/agent"
	"kestrel/internal/config"
	"kestrel/internal/database"
	"kestrel/internal/openai"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Builder attack chain builder
type Builder struct {
	db           *database.DB
	logger       *zap.Logger
	openAIClient *openai.Client
	openAIConfig *config.OpenAIConfig
	tokenCounter agent.TokenCounter
	maxTokens    int // maximum token limit, default 100000
}

// Node attack chain node (uses database package type)
type Node = database.AttackChainNode

// Edge attack chain edge (uses database package type)
type Edge = database.AttackChainEdge

// Chain complete attack chain
type Chain struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// NewBuilder creates a new attack chain builder
func NewBuilder(db *database.DB, openAIConfig *config.OpenAIConfig, logger *zap.Logger) *Builder {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	httpClient := &http.Client{Timeout: 5 * time.Minute, Transport: transport}

	// prefer unified token limit from config file (config.yaml -> openai.max_total_tokens)
	maxTokens := 0
	if openAIConfig != nil && openAIConfig.MaxTotalTokens > 0 {
		maxTokens = openAIConfig.MaxTotalTokens
	} else if openAIConfig != nil {
		// if max_total_tokens is not explicitly configured, use a reasonable default based on model settings
		model := strings.ToLower(openAIConfig.Model)
		if strings.Contains(model, "gpt-4") {
			maxTokens = 128000 // gpt-4 typically supports 128k
		} else if strings.Contains(model, "gpt-3.5") {
			maxTokens = 16000 // gpt-3.5-turbo typically supports 16k
		} else if strings.Contains(model, "deepseek") {
			maxTokens = 131072 // deepseek-chat typically supports 131k
		} else {
			maxTokens = 100000 // fallback default value
		}
	} else {
		// use fallback value when no OpenAI config to avoid 0
		maxTokens = 100000
	}

	return &Builder{
		db:           db,
		logger:       logger,
		openAIClient: openai.NewClient(openAIConfig, httpClient, logger),
		openAIConfig: openAIConfig,
		tokenCounter: agent.NewTikTokenCounter(),
		maxTokens:    maxTokens,
	}
}

// BuildChainFromConversation build attack chain from conversation (single LLM call; input is the last_react trace for the current task turn, consistent with continue-conversation resume scope).
func (b *Builder) BuildChainFromConversation(ctx context.Context, conversationID string) (*Chain, error) {
	b.logger.Info("starting attack chain build (simplified)", zap.String("conversationId", conversationID))

	// 0. first check if there are actual tool execution records
	messages, err := b.db.GetMessages(conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation messages: %w", err)
	}

	if len(messages) == 0 {
		b.logger.Info("no data in conversation", zap.String("conversationId", conversationID))
		return &Chain{Nodes: []Node{}, Edges: []Edge{}}, nil
	}

	// check for actual tool executions: assistant mcp_execution_ids, or tool_call/tool_result in process details
	// (in multi-agent mode if MCP does not return execution_id, IDs may be empty, but tools have been executed via Eino and written to process_details)
	hasToolExecutions := false
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "assistant") {
			if len(messages[i].MCPExecutionIDs) > 0 {
				hasToolExecutions = true
				break
			}
		}
	}
	if !hasToolExecutions {
		if pdOK, err := b.db.ConversationHasToolProcessDetails(conversationID); err != nil {
			b.logger.Warn("failed to query process details to determine tool execution", zap.Error(err))
		} else if pdOK {
			hasToolExecutions = true
		}
	}

	// check if task was cancelled (by checking last assistant message content or process_details)
	taskCancelled := false
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "assistant") {
			content := strings.ToLower(messages[i].Content)
			if strings.Contains(content, "cancelled") || strings.Contains(content, "cancelled") {
				taskCancelled = true
			}
			break
		}
	}

	// if task was cancelled and no actual tool execution, return empty attack chain
	if taskCancelled && !hasToolExecutions {
		b.logger.Info("task cancelled and no actual tool execution, returning empty attack chain",
			zap.String("conversationId", conversationID),
			zap.Bool("taskCancelled", taskCancelled),
			zap.Bool("hasToolExecutions", hasToolExecutions))
		return &Chain{Nodes: []Node{}, Edges: []Edge{}}, nil
	}

	// if no actual tool execution, also return empty attack chain (to avoid AI fabrication)
	if !hasToolExecutions {
		b.logger.Info("no actual tool execution records, returning empty attack chain",
			zap.String("conversationId", conversationID))
		return &Chain{Nodes: []Node{}, Edges: []Edge{}}, nil
	}

	// 1. 1. first try to get saved last-round ReAct input and output from database
	reactInputJSON, modelOutput, err := b.db.GetAgentTrace(conversationID)
	if err != nil {
		b.logger.Warn("failed to get saved ReAct data, will build from message history", zap.Error(err))
		// continue with original logic
		reactInputJSON = ""
		modelOutput = ""
	}

	// var userInput string
	var reactInputFinal string
	var dataSource string // record data source

	// prefer persisted agent trace (same source as continueconversation loadHistoryFromAgentTrace), trimmed to current task turn
	if reactInputJSON != "" {
		trimmedJSON := agent.ExtractLastUserTurnTraceJSON(reactInputJSON)
		hash := sha256.Sum256([]byte(trimmedJSON))
		reactInputHash := hex.EncodeToString(hash[:])[:16]

		var messageCount int
		if msgs, parseErr := agent.ParseTraceMessages(trimmedJSON); parseErr == nil {
			messageCount = len(msgs)
			msgs = agent.MergeAssistantTraceOutput(msgs, modelOutput)
			reactInputFinal = b.formatAgentTraceFromChatMessages(msgs)
		} else {
			b.logger.Warn("failed to parse agent trace, falling back to raw JSON formatting", zap.Error(parseErr))
			reactInputFinal = b.formatAgentTraceInputFromJSON(trimmedJSON)
			if strings.TrimSpace(modelOutput) != "" {
				reactInputFinal += "\n\n## Assistant Conclusion (last_react_output)\n\n" + modelOutput
			}
		}

		dataSource = "last_user_turn_agent_trace"
		b.logger.Info("building attack chain from current task turn agent trace (consistent with resume context scope)",
			zap.String("conversationId", conversationID),
			zap.String("dataSource", dataSource),
			zap.Int("traceInputSizeBeforeTrim", len(reactInputJSON)),
			zap.Int("traceInputSizeAfterTrim", len(trimmedJSON)),
			zap.Int("messageCount", messageCount),
			zap.String("reactInputHash", reactInputHash),
			zap.Int("modelOutputSize", len(modelOutput)))
	} else {
		// 2. 2. if no saved ReAct data, build from conversation messages
		dataSource = "messages_table"
		b.logger.Info("building ReAct data from message history",
			zap.String("conversationId", conversationID),
			zap.String("dataSource", dataSource),
			zap.Int("messageCount", len(messages)))

		// extract user input (last user message)
		for i := len(messages) - 1; i >= 0; i-- {
			if strings.EqualFold(messages[i].Role, "user") {
				// userInput = messages[i].Content
				break
			}
		}

		// extract last ReAct round input (history messages + current user input)
		reactInputFinal = b.buildAgentTraceInput(messages)

		// extract last model output (last assistant message)
		for i := len(messages) - 1; i >= 0; i-- {
			if strings.EqualFold(messages[i].Role, "assistant") {
				modelOutput = messages[i].Content
				break
			}
		}
	}

	// multi-agent: saved trace column may only contain first-round user message without tool trace; supplement last assistant's process details (aligned with single-agent complete trace)
	hasMCPOnAssistant := false
	var lastAssistantID string
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "assistant") {
			lastAssistantID = messages[i].ID
			if len(messages[i].MCPExecutionIDs) > 0 {
				hasMCPOnAssistant = true
			}
			break
		}
	}
	if lastAssistantID != "" {
		pdHasTools, _ := b.db.ConversationHasToolProcessDetails(conversationID)
		if pdHasTools && !(hasMCPOnAssistant && reactInputContainsToolTrace(reactInputJSON)) {
			detailsMap, err := b.db.GetProcessDetailsByConversation(conversationID)
			if err != nil {
				b.logger.Warn("failed to load process details for attack chain", zap.Error(err))
			} else if dets := detailsMap[lastAssistantID]; len(dets) > 0 {
				extra := b.formatProcessDetailsForAttackChain(dets)
				if strings.TrimSpace(extra) != "" {
					reactInputFinal = reactInputFinal + "\n\n## Execution Process and Tool Records (including multi-agent orchestration and sub-tasks)\n\n" + extra
					b.logger.Info("attack chain input supplemented with process details",
						zap.String("conversationId", conversationID),
						zap.String("messageId", lastAssistantID),
						zap.Int("detailEvents", len(dets)))
				}
			}
		}
	}

	// 3. compress input by token budget, then build prompt (to avoid exceeding model context)
	reactInputFinal, modelOutput, _ = b.fitAttackChainPayload(reactInputFinal, modelOutput)

	// 4. build prompt and make a single model call (assistant conclusion not re-passed when already merged into trace)
	promptAssistantOut := modelOutput
	if reactInputJSON != "" {
		promptAssistantOut = ""
	}
	prompt := b.buildSimplePrompt(reactInputFinal, promptAssistantOut)
	// fmt.Println(prompt)
	// 6. call AI to generate attack chain (one-shot, no processing)
	chainJSON, err := b.callAIForChainGeneration(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("AI generation failed: %w", err)
	}

	// 7. parse JSON and generate node/edge IDs (frontend needs valid IDs)
	chainData, err := b.parseChainJSON(chainJSON)
	if err != nil {
		// if parsing failed, return empty chain and let frontend handle the error
		b.logger.Warn("failed to parse attack chain JSON", zap.Error(err), zap.String("raw_json", chainJSON))
		return &Chain{
			Nodes: []Node{},
			Edges: []Edge{},
		}, nil
	}

	b.logger.Info("attack chain build complete",
		zap.String("conversationId", conversationID),
		zap.String("dataSource", dataSource),
		zap.Int("nodes", len(chainData.Nodes)),
		zap.Int("edges", len(chainData.Edges)))

	// save to database (for subsequent loading)
	if err := b.saveChain(conversationID, chainData.Nodes, chainData.Edges); err != nil {
		b.logger.Warn("failed to save attack chain to database", zap.Error(err))
		// return data to frontend even if save failed
	}

	// return directly without any processing or validation
	return chainData, nil
}

// reactInputContainsToolTrace check if saved ReAct JSON contains parseable tool call trace (true when single-agent saves completely).
func reactInputContainsToolTrace(reactInputJSON string) bool {
	s := strings.TrimSpace(reactInputJSON)
	if s == "" {
		return false
	}
	return strings.Contains(s, "tool_calls") ||
		strings.Contains(s, "tool_call_id") ||
		strings.Contains(s, `"role":"tool"`) ||
		strings.Contains(s, `"role": "tool"`)
}

// formatProcessDetailsForAttackChain format last assistant round's process details as attack chain analysis input (covers incomplete last_react_input in multi-agent mode).
func (b *Builder) formatProcessDetailsForAttackChain(details []database.ProcessDetail) string {
	if len(details) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, d := range details {
		// goal: output the full iteration round from the perspective of the primary agent (orchestrator)
		// - keep: orchestrator tool calls/results, task dispatches to sub-agents, sub-agent final replies (no reasoning)
		// - discard: thinking/planning/progress noise, sub-agent tool details and reasoning process
		if d.EventType == "progress" || d.EventType == "thinking" || d.EventType == "reasoning_chain" || d.EventType == "planning" {
			continue
		}

		// parse data (JSON string) to identify einoRole / toolName etc.
		var dataMap map[string]interface{}
		if strings.TrimSpace(d.Data) != "" {
			_ = json.Unmarshal([]byte(d.Data), &dataMap)
		}
		einoRole := ""
		if v, ok := dataMap["einoRole"]; ok {
			einoRole = strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
		}
		toolName := ""
		if v, ok := dataMap["toolName"]; ok {
			toolName = strings.TrimSpace(fmt.Sprint(v))
		}

		// 1) orchestrator tool calls/results: keep (these show "what tools the main agent called")
		if (d.EventType == "tool_call" || d.EventType == "tool_result" || d.EventType == "tool_calls_detected" || d.EventType == "iteration") && einoRole == "orchestrator" {
			sb.WriteString("[")
			sb.WriteString(d.EventType)
			sb.WriteString("] ")
			sb.WriteString(strings.TrimSpace(d.Message))
			sb.WriteString("\n")
			if strings.TrimSpace(d.Data) != "" {
				sb.WriteString(d.Data)
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
			continue
		}

		// 2) sub-agent dispatch: tool_call(toolName=="task") means orchestrator dispatched a sub-task; keep (task only, no sub-agent reasoning)
		if d.EventType == "tool_call" && strings.EqualFold(toolName, "task") {
			sb.WriteString("[dispatch_subagent_task] ")
			sb.WriteString(strings.TrimSpace(d.Message))
			sb.WriteString("\n")
			if strings.TrimSpace(d.Data) != "" {
				sb.WriteString(d.Data)
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
			continue
		}

		// 3) sub-agent final reply: keep (keep final output only, not the analysis process)
		if d.EventType == "eino_agent_reply" && einoRole == "sub" {
			sb.WriteString("[subagent_final_reply] ")
			sb.WriteString(strings.TrimSpace(d.Message))
			sb.WriteString("\n")
			// data contains meta-info like einoAgent; keeping it helps trace "which sub-agent said this"
			if strings.TrimSpace(d.Data) != "" {
				sb.WriteString(d.Data)
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
			continue
		}

		// other events are discarded by default to avoid stuffing sub-agent tool details/reasoning into the prompt, which would skew the "main agent single iteration" perspective.
	}
	return strings.TrimSpace(sb.String())
}

// buildAgentTraceInput build the last ReAct round input (starting from the last user message, excluding earlier rounds).
func (b *Builder) buildAgentTraceInput(messages []database.Message) string {
	start := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "user") {
			start = i
			break
		}
	}
	var builder strings.Builder
	for _, msg := range messages[start:] {
		builder.WriteString(fmt.Sprintf("[%s]: %s\n\n", msg.Role, msg.Content))
	}
	return builder.String()
}

// extractUserInputFromReActInput extract last user input from saved ReAct input (JSON-format messages array)
// func (b *Builder) extractUserInputFromReActInput(reactInputJSON string) string {
// 	// reactInputJSON is a JSON-format ChatMessage array, needs parsing
// 	var messages []map[string]interface{}
// 	if err := json.Unmarshal([]byte(reactInputJSON), &messages); err != nil {
// 		b.logger.Warn("failed to parse ReAct input JSON", zap.Error(err))
// 		return ""
// 	}

// 	// search backwards for the last user message
// 	for i := len(messages) - 1; i >= 0; i-- {
// 		if role, ok := messages[i]["role"].(string); ok && strings.EqualFold(role, "user") {
// 			if content, ok := messages[i]["content"].(string); ok {
// 				return content
// 			}
// 		}
// 	}

// 	return ""
// }

// formatAgentTraceInputFromJSON convert JSON trace to readable text (trimmed to current task turn first).
func (b *Builder) formatAgentTraceInputFromJSON(reactInputJSON string) string {
	trimmed := agent.ExtractLastUserTurnTraceJSON(reactInputJSON)
	msgs, err := agent.ParseTraceMessages(trimmed)
	if err != nil {
		b.logger.Warn("failed to parse ReAct input JSON", zap.Error(err))
		return trimmed
	}
	return b.formatAgentTraceFromChatMessages(msgs)
}

// formatAgentTraceFromChatMessages format agent messages as attack chain analysis input (consistent with resume trace fields).
func (b *Builder) formatAgentTraceFromChatMessages(msgs []agent.ChatMessage) string {
	var builder strings.Builder
	for _, msg := range msgs {
		role := msg.Role
		content := msg.Content

		if strings.EqualFold(role, "assistant") && len(msg.ToolCalls) > 0 {
			if content != "" {
				builder.WriteString(fmt.Sprintf("[%s]: %s\n", role, content))
			}
			builder.WriteString(fmt.Sprintf("[%s] tool call (%d calls):\n", role, len(msg.ToolCalls)))
			for i, tc := range msg.ToolCalls {
				args := ""
				if tc.Function.Arguments != nil {
					if b, err := json.Marshal(tc.Function.Arguments); err == nil {
						args = string(b)
					}
				}
				builder.WriteString(fmt.Sprintf("  [tool call %d]\n", i+1))
				builder.WriteString(fmt.Sprintf("    ID: %s\n", tc.ID))
				builder.WriteString(fmt.Sprintf("    tool name: %s\n", tc.Function.Name))
				builder.WriteString(fmt.Sprintf("    args: %s\n", args))
			}
			builder.WriteString("\n")
			continue
		}

		if strings.EqualFold(role, "tool") {
			if msg.ToolCallID != "" {
				builder.WriteString(fmt.Sprintf("[%s] (tool_call_id: %s):\n%s\n\n", role, msg.ToolCallID, content))
			} else {
				builder.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, content))
			}
			continue
		}

		builder.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, content))
	}
	return builder.String()
}

// buildSimplePrompt build simplified prompt
func (b *Builder) buildSimplePrompt(reactInput, modelOutput string) string {
	return fmt.Sprintf(`You are a professional security test analyst and attack chain construction expert. Your task is to output the attack chain JSON in one shot based on the **current task turn** conversation records and tool execution results (do not ask follow-up questions in multiple rounds).

## Input Scope (consistent with "continue conversation" resume)
- The "ReAct Trace" below only includes messages and tool results **after the last user question** (last_react current task turn), excluding earlier user question rounds.
- The "assistant conclusion" is the final output summary for the same task turn (last_react_output); nodes must be consistent with actual tool executions in the trace, fabrication is strictly prohibited.

## Core Objectives

Build an attack chain that tells a complete attack story so learners can:
1. Understand the complete penetration test workflow and reasoning logic (every step from target identification to vulnerability discovery)
2. Learn how to gather clues from failures and adjust strategies
3. Master the actual effects and limitations of tool usage
4. Understand the causal relationship between vulnerability discovery and exploitation

**Key Principle**: Completeness first. Must include all meaningful tool executions and key steps; do not omit important information to control node count.

## Build Process (think in this order)

### Step 1: Understand Context
Carefully analyze the tool call sequence and model output in the ReAct input, identifying:
- Test targets (IP, domain, URL, etc.)
- Actually executed tools and parameters
- Key information returned by tools (successful results, error info, timeouts, etc.)
- AI analysis and decision-making process

### Step 2: Extract Key Nodes
Extract meaningful nodes from tool execution records, **ensure no key steps are missed**:
- **target nodes**: create one target node per independent test target
- **action nodes**: create one action node per meaningful tool execution (including failures that provide clues, successful info gathering, vulnerability validation, etc.)
- **vulnerability nodes**: create one vulnerability node per genuinely confirmed vulnerability
- **completeness check**: cross-reference against the tool call sequence in the ReAct input to ensure every meaningful tool execution is included in the attack chain

### Step 3: Build Logical Relationships (Tree Structure)
**Important: Must build a tree structure, not a simple linear chain.**
Connect nodes according to causal relationships to form a tree graph (since it's single agent execution, chronological order is not required):
- **branch structure**: one node can have multiple successor nodes (e.g. after port scan discovers multiple ports, multiple different tests can proceed simultaneously)
- **convergence structure**: multiple nodes can point to the same node (e.g. multiple different tests all discovered the same vulnerability)
- Identify which actions are executed based on results of previous actions
- Identify which vulnerabilities were discovered by which actions
- Identify how failed nodes provide clues for subsequent successes
- **Avoid linear chains**: do not connect all nodes in a line; build a tree structure based on actual parallel testing and branch exploration

### Step 4: Optimize and Streamline
- **completeness check**: ensure all meaningful tool executions are included; do not omit key steps
- **merge rules**: only merge genuinely similar or duplicate action nodes (e.g. similar calls of the same tool multiple times)
- **deletion rules**: only delete completely worthless failed nodes (no output at all, pure system errors, duplicate identical failures)
- **important reminder**: err on the side of keeping more nodes rather than omitting key steps. The attack chain must fully represent the penetration test process.
- ensure the attack chain logic is coherent and tells a complete story

## Node Type Details

### target (Target Node)
- **Purpose**: identify test targets
- **Creation Rules**: create one target node per independent target (different IP/domain)
- **multi-target handling**: nodes for different targets are not interconnected, each forms an independent subgraph
- **metadata.target**: precisely record the target identifier (IP address, domain, URL, etc.)

### action (Action Node)
- **Purpose**: record tool executions and AI analysis results
- **Label Rules**:
  * 15-25 Chinese characters, verb-object structure
  * successful nodes: describe execution result (e.g. "port scan found 80/443/8080", "directory scan found /admin path")
  * failed nodes: describe failure reason (e.g. "attempted SQL injection (blocked by WAF)", "port scan timed out (target unreachable)")
- **ai_analysis Requirements**:
  * successful nodes: summarize key discoveries from tool execution and explain their significance
  * failed nodes: must explain failure reason, clues obtained, and how these clues guide subsequent actions
  * no more than 150 words, be specific and informative
- **findings Requirements**:
  * extract key information points from tool return results
  * each finding should be an independent, valuable information fragment
  * successful nodes: list key discoveries (e.g. ["port 80 open", "port 443 open", "HTTP service is Apache 2.4"])
  * failed nodes: list failure clues (e.g. ["WAF blocked", "returned 403", "Cloudflare detected"])
- **Status Marks**:
  * successful nodes: omit or set to "success"
  * failed nodes providing clues: must be set to "failed_insight"
- **risk_score**: always 0 (action nodes do not assess risk)

### vulnerability (Vulnerability Node)
- **Purpose**: record truly confirmed security vulnerabilities
- **Creation Rules**:
  * must be truly confirmed vulnerabilities; not every discovery is a vulnerability
  * requires clear vulnerability evidence (e.g. SQL injection returns database error, XSS executes successfully, etc.)
- **risk_score Rules**:
  * critical (90-100): can lead to complete system compromise (RCE, SQL injection causing data leakage, etc.)
  * high (80-89): can lead to sensitive information leakage or privilege escalation
  * medium (60-79): security risk exists but impact is limited
  * low (40-59): minor security issues
- **metadata Requirements**:
  * vulnerability_type: vulnerability type (SQL Injection, XSS, RCE, etc.)
  * description: detailed description of vulnerability location, mechanism, and impact
  * severity: critical/high/medium/low
  * location: precise vulnerability location (URL, parameter, file path, etc.)

## Node Filter and Merge Rules

### Failed Nodes That Must Be Kept
The following failure situations must have nodes created because they provide valuable clues:
- tool returns clear error info (permission error, connection refused, authentication failed, etc.)
- timeouts or connection failures (may indicate firewalls, network isolation, etc.)
- WAF/firewall blocking (returns 403, 406, etc., indicating protective mechanisms are present)
- tool not installed or config error (but the call was executed)
- target unreachable (DNS resolution failed, network unreachable, etc.)

### Failed Nodes That Should Be Deleted
The following situations should not have nodes created:
- tool calls with completely no output
- pure system errors (unrelated to target, e.g. local environment issues)
- repeated identical failures (keep only the first occurrence of repeated identical errors)

### Node Merge Rules
The following situations should merge nodes:
- multiple similar calls of the same tool (e.g. multiple nmap scans of different port ranges, merged into one "port scan" node)
- multiple similar probes against the same target (e.g. multiple directory scan tools, merged into one "directory scan" node)

### Node Count Control
- **completeness priority**: must include all meaningful tool executions and key steps; do not delete important nodes to control count
- **recommended range**: typically 8-15 nodes for a single target, but can be increased appropriately if there are more actual execution steps (max 20 nodes)
- **priority preservation**: key successful steps, failures that provide clues, discovered vulnerabilities, important info gathering steps
- **can be merged**: multiple similar calls of the same tool (e.g. multiple nmap scans of different port ranges, merged into one "port scan" node)
- **can be deleted**: tool calls with absolutely no output, pure system errors, duplicate identical failures (keep only the first occurrence of repeated identical errors)
- **Important Principle**: prefer slightly more nodes over missing key steps. The attack chain must fully show the complete penetration test process

## Edge Types and Weights

### Edge Types
- **leads_to**: means "leads to" or "guides to", used for action→action, target→action
  * e.g.: port scan → directory scan (because port 80 was discovered, so directory scan is performed)
- **discovers**: means "discovered", **exclusively for action→vulnerability**
  * e.g.: SQL injection test → SQL injection vulnerability
  * **Important**: all action→vulnerability edges must use discovers type, even when multiple actions point to the same vulnerability
- **enables**: means "enables" or "facilitates", **only for vulnerability→vulnerability, action→action (when subsequent actions depend on earlier results)**
  * e.g.: information leakage vulnerability → privilege escalation vulnerability (info from the leakage facilitated privilege escalation)
  * **Important**: enables cannot be used for action→vulnerability; action→vulnerability must use discovers

### Edge Weights
- **weight 1-2**: weak association (e.g. initial probe to further probe)
- **weight 3-4**: medium association (e.g. discovering a port to service identification)
- **weight 5-7**: strong association (e.g. discovering vulnerability, key info leakage)
- **weight 8-10**: very strong association (e.g. successful vulnerability exploitation, privilege escalation)

### DAG Structure Requirements (Directed Acyclic Graph)
**Key: Must ensure the generated graph is a true DAG (Directed Acyclic Graph) with no cycles.**

- **Node Numbering Rules**: node ids increment from "node_1" (node_1, node_2, node_3...)
- **Edge Direction Rules**: all edge source node ids must be strictly less than target node ids (source < target); this is key to ensuring no cycles
  * e.g.: node_1 → node_2 ✓ (correct)
  * e.g.: node_2 → node_1 ✗ (error, would form a cycle)
  * e.g.: node_3 → node_5 ✓ (correct)
- **no cycle validation**: before outputting JSON, must check all edges to ensure no edge has source >= target
- **no isolated nodes**: ensure every node has at least one edge connection (except possible root nodes)
- **DAG Structural Characteristics**:
  * a node can have multiple successor nodes (branching), e.g.: node_2 (port scan) can connect to node_3, node_4, node_5 simultaneously
  * multiple nodes can converge on one node (merging), e.g.: node_3, node_4, node_5 all point to node_6 (vulnerability node)
  * avoid connecting all nodes in a line; build the DAG structure based on actual parallel testing and branch exploration
- **Topological Sort Validation**: if nodes are sorted by id from small to large, all edges should point from left to right (top to bottom), ensuring no cycles

## Attack Chain Logical Coherence Requirements

The built attack chain should be able to answer the following questions:
1. **Starting Point**: where does the test begin? (target node)
2. **Exploration Process**: how is information gathered step by step? (action node sequence)
3. **Failures and Adjustments**: how are strategies adjusted when obstacles are encountered? (failed_insight nodes)
4. **Key Discoveries**: what important information was found? (action findings)
5. **Vulnerability Confirmation**: how is the existence of vulnerabilities confirmed? (action→vulnerability)
6. **Attack Path**: what is the complete attack path? (path from target to vulnerability)

## Current Task ReAct Trace (includes tool executions; see assistant conclusion at end of trace)

%s
%s

## Output Format

Strictly output in the following JSON format, do not add any other text:

**Important: the example shows a tree structure; note that node_2 (port scan) connects to multiple successor nodes (node_3, node_4) simultaneously, forming a branching structure.**

{
   "nodes": [
     {
       "id": "node_1",
       "type": "target",
       "label": "test target: example.com",
       "risk_score": 40,
       "metadata": {
         "target": "example.com"
       }
     },
     {
       "id": "node_2",
       "type": "action",
       "label": "port scan found 80/443/8080",
       "risk_score": 0,
       "metadata": {
         "tool_name": "nmap",
         "tool_intent": "port scan",
         "ai_analysis": "Used nmap to port scan the target, discovered ports 80, 443, 8080 open. Port 80 runs HTTP service, port 443 runs HTTPS service, port 8080 may be admin backend. These open ports provide entry points for subsequent web application testing.",
         "findings": ["port 80 open", "port 443 open", "port 8080 open", "HTTP service is Apache 2.4"]
       }
     },
     {
       "id": "node_3",
       "type": "action",
       "label": "directory scan found /admin backend",
       "risk_score": 0,
       "metadata": {
         "tool_name": "dirsearch",
         "tool_intent": "directory scan",
         "ai_analysis": "Used dirsearch to perform directory scan on target, discovered /admin directory exists and is accessible. This directory may be the admin backend, an important test target.",
         "findings": ["/admin directory exists", "returned 200 status code", "suspected admin backend"]
       }
     },
     {
       "id": "node_4",
       "type": "action",
       "label": "identified web service as Apache 2.4",
       "risk_score": 0,
       "metadata": {
         "tool_name": "whatweb",
         "tool_intent": "web service identification",
         "ai_analysis": "Identified target is running Apache 2.4 server, providing important information for subsequent vulnerability testing.",
         "findings": ["Apache 2.4", "PHP version info"]
       }
     },
     {
       "id": "node_5",
       "type": "action",
       "label": "attempted SQL injection (blocked by WAF)",
       "risk_score": 0,
       "metadata": {
         "tool_name": "sqlmap",
         "tool_intent": "SQL injection detection",
         "ai_analysis": "SQL injection test on /login.php was blocked by WAF, returned 403 error. Error message indicates Cloudflare protection detected. This indicates the target has a WAF deployed; test strategy needs adjustment.",
         "findings": ["WAF blocked", "returned 403", "Cloudflare detected", "target has WAF deployed"],
         "status": "failed_insight"
       }
     },
     {
       "id": "node_6",
       "type": "vulnerability",
       "label": "SQL Injection Vulnerability",
       "risk_score": 85,
       "metadata": {
         "vulnerability_type": "SQL Injection",
         "description": "Discovered SQL injection vulnerability in the username parameter of /admin/login.php. Can bypass login validation by injecting payload to directly obtain admin privileges. Vulnerability returns database error messages, confirming the injection point.",
         "severity": "high",
         "location": "/admin/login.php?username="
       }
     }
   ],
   "edges": [
     {
       "source": "node_1",
       "target": "node_2",
       "type": "leads_to",
       "weight": 3
     },
     {
       "source": "node_2",
       "target": "node_3",
       "type": "leads_to",
       "weight": 4
     },
     {
       "source": "node_2",
       "target": "node_4",
       "type": "leads_to",
       "weight": 3
     },
     {
       "source": "node_3",
       "target": "node_5",
       "type": "leads_to",
       "weight": 4
     },
     {
       "source": "node_5",
       "target": "node_6",
       "type": "discovers",
       "weight": 7
     }
   ]
}

## Important Reminders

1. **No Fabrication**: only use tools actually executed in the ReAct input and results actually returned. If no actual data, return empty nodes and edges arrays.
2. **DAG Structure Required**: must build a true DAG (Directed Acyclic Graph) with no cycles. All edge source node ids must be strictly less than target node ids (source < target).
3. **Topological Order**: nodes should be numbered in logical order; target nodes are typically node_1, subsequent action nodes increment by execution order, vulnerability nodes come last.
4. **Completeness First**: must include all meaningful tool executions and key steps; do not delete important nodes to control node count. The attack chain must fully show the complete process from target identification to vulnerability discovery.
5. **Logical Coherence**: ensure the attack chain tells a complete, coherent penetration test story including all key steps and decision points.
6. **Educational Value**: prioritize keeping educationally meaningful nodes to help learners understand penetration testing mindset and complete workflow.
7. **Accuracy**: all node information must be based on actual data; do not speculate or assume.
8. **Completeness Check**: ensure every node has the necessary metadata fields, every edge has correct source and target, no isolated nodes, no cycles.
9. **No Over-simplification**: if there are many actual execution steps, node count can be increased appropriately (max 20) to ensure key steps are not missed.
10. **Validate Before Output**: before outputting JSON, must validate all edges satisfy source < target condition to ensure correct DAG structure.

Now begin analyzing and building the attack chain:`, reactInput, assistantOutSection(modelOutput))
}

func assistantOutSection(modelOutput string) string {
	modelOutput = strings.TrimSpace(modelOutput)
	if modelOutput == "" {
		return ""
	}
	return "\n## Assistant Conclusion (supplementary)\n\n" + modelOutput + "\n"
}

// saveChain save attack chain to database
func (b *Builder) saveChain(conversationID string, nodes []Node, edges []Edge) error {
	// first delete old attack chain data
	if err := b.db.DeleteAttackChain(conversationID); err != nil {
		b.logger.Warn("failed to delete old attack chain", zap.Error(err))
	}

	for _, node := range nodes {
		metadataJSON, _ := json.Marshal(node.Metadata)
		if err := b.db.SaveAttackChainNode(conversationID, node.ID, node.Type, node.Label, "", string(metadataJSON), node.RiskScore); err != nil {
			b.logger.Warn("failed to save attack chain node", zap.String("nodeId", node.ID), zap.Error(err))
		}
	}

	// save edges
	for _, edge := range edges {
		if err := b.db.SaveAttackChainEdge(conversationID, edge.ID, edge.Source, edge.Target, edge.Type, edge.Weight); err != nil {
			b.logger.Warn("failed to save attack chain edge", zap.String("edgeId", edge.ID), zap.Error(err))
		}
	}

	return nil
}

// LoadChainFromDatabase load attack chain from database
func (b *Builder) LoadChainFromDatabase(conversationID string) (*Chain, error) {
	nodes, err := b.db.LoadAttackChainNodes(conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to load attack chain nodes: %w", err)
	}

	edges, err := b.db.LoadAttackChainEdges(conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to load attack chain edges: %w", err)
	}

	return &Chain{
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// callAIForChainGeneration call AI to generate attack chain
func (b *Builder) callAIForChainGeneration(ctx context.Context, prompt string) (string, error) {
	requestBody := map[string]interface{}{
		"model": b.openAIConfig.Model,
		"messages": []map[string]interface{}{
			{
				"role":    "system",
				"content": "You are a professional security test analyst skilled in building attack chain graphs. Please strictly return attack chain data in JSON format.",
			},
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature":           0.3,
		"max_completion_tokens": attackChainMaxCompletionTokens(b.maxTokens),
	}

	var apiResponse struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if b.openAIClient == nil {
		return "", fmt.Errorf("OpenAI client not initialized")
	}
	if err := b.openAIClient.ChatCompletion(ctx, requestBody, &apiResponse); err != nil {
		var apiErr *openai.APIError
		if errors.As(err, &apiErr) {
			bodyStr := strings.ToLower(apiErr.Body)
			if strings.Contains(bodyStr, "context") || strings.Contains(bodyStr, "length") || strings.Contains(bodyStr, "too long") {
				return "", fmt.Errorf("context length exceeded")
			}
		} else if strings.Contains(strings.ToLower(err.Error()), "context") || strings.Contains(strings.ToLower(err.Error()), "length") {
			return "", fmt.Errorf("context length exceeded")
		}
		return "", fmt.Errorf("request failed: %w", err)
	}

	if len(apiResponse.Choices) == 0 {
		return "", fmt.Errorf("API did not return a valid response")
	}

	content := strings.TrimSpace(apiResponse.Choices[0].Message.Content)
	// try to extract JSON (may contain markdown code blocks)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	return content, nil
}

// ChainJSON attack chain JSON structure
type ChainJSON struct {
	Nodes []struct {
		ID        string                 `json:"id"`
		Type      string                 `json:"type"`
		Label     string                 `json:"label"`
		RiskScore int                    `json:"risk_score"`
		Metadata  map[string]interface{} `json:"metadata"`
	} `json:"nodes"`
	Edges []struct {
		Source string `json:"source"`
		Target string `json:"target"`
		Type   string `json:"type"`
		Weight int    `json:"weight"`
	} `json:"edges"`
}

// parseChainJSON parse attack chain JSON
func (b *Builder) parseChainJSON(chainJSON string) (*Chain, error) {
	var chainData ChainJSON
	if err := json.Unmarshal([]byte(chainJSON), &chainData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// create node ID map (AI-returned ID -> new UUID)
	nodeIDMap := make(map[string]string)

	// convert to Chain structure
	nodes := make([]Node, 0, len(chainData.Nodes))
	for _, n := range chainData.Nodes {
		// generate new UUID node ID
		newNodeID := fmt.Sprintf("node_%s", uuid.New().String())
		nodeIDMap[n.ID] = newNodeID

		node := Node{
			ID:        newNodeID,
			Type:      n.Type,
			Label:     n.Label,
			RiskScore: n.RiskScore,
			Metadata:  n.Metadata,
		}
		if node.Metadata == nil {
			node.Metadata = make(map[string]interface{})
		}
		nodes = append(nodes, node)
	}

	// convert edges
	edges := make([]Edge, 0, len(chainData.Edges))
	for _, e := range chainData.Edges {
		sourceID, ok := nodeIDMap[e.Source]
		if !ok {
			continue
		}
		targetID, ok := nodeIDMap[e.Target]
		if !ok {
			continue
		}

		// generate edge ID (required by frontend)
		edgeID := fmt.Sprintf("edge_%s", uuid.New().String())

		edges = append(edges, Edge{
			ID:     edgeID,
			Source: sourceID,
			Target: targetID,
			Type:   e.Type,
			Weight: e.Weight,
		})
	}

	return &Chain{
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// all methods below are no longer used, removed to simplify code
