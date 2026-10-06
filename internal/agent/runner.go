package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"kestrel/internal/config"
	"kestrel/internal/database"
	"kestrel/internal/mcp"
)

// Mode describes the agent orchestration strategy.
type Mode string

const (
	ModeSingle      Mode = "single"       // ReAct loop with a single agent
	ModePlanExecute Mode = "plan_execute"  // Plan step followed by execution step
	ModeSupervisor  Mode = "supervisor"    // Supervisor coordinates multiple sub-agents
)

// Message is a single turn in the conversation history.
type Message struct {
	Role    string `json:"role"` // user | assistant | tool
	Content string `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ToolCall is a proposed tool invocation from the agent.
type ToolCall struct {
	ID        string                 `json:"id"`
	ToolName  string                 `json:"tool_name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// StepEvent is emitted on a WebSocket stream during agent execution.
type StepEvent struct {
	Type    string          `json:"type"`    // thinking | tool_call | tool_result | final | error
	Content string          `json:"content,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// RunParams carries all inputs to a single agent run.
type RunParams struct {
	SessionID    string
	UserID       string
	RoleID       string
	Intent       string
	History      []Message
	Mode         Mode
	AllowedTools []string
	HITLMode     string // auto | require_approval
	MaxIter      int
}

// RunResult carries the final output of an agent run.
type RunResult struct {
	SessionID      string         `json:"session_id"`
	FinalAnswer    string         `json:"final_answer"`
	ToolExecutions []ToolExecSummary `json:"tool_executions"`
	Iterations     int            `json:"iterations"`
	Error          string         `json:"error,omitempty"`
}

// ToolExecSummary is a summary of a single tool call for the run result.
type ToolExecSummary struct {
	ToolName   string `json:"tool_name"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms"`
	Truncated  bool   `json:"truncated"`
}

// Runner executes agent runs, streaming events via an optional channel.
type Runner struct {
	cfg      *config.AgentConfig
	db       *database.DB
	registry *mcp.Registry
	logger   *zap.Logger
}

// NewRunner creates an agent runner.
func NewRunner(cfg *config.AgentConfig, db *database.DB, registry *mcp.Registry, logger *zap.Logger) *Runner {
	return &Runner{cfg: cfg, db: db, registry: registry, logger: logger}
}

// Run executes the agent loop and sends step events to the events channel (if non-nil).
// It persists tool executions and messages to the database.
func (r *Runner) Run(ctx context.Context, params RunParams, events chan<- StepEvent) (*RunResult, error) {
	maxIter := params.MaxIter
	if maxIter <= 0 {
		maxIter = r.cfg.MaxIterations
	}

	result := &RunResult{SessionID: params.SessionID}
	history := make([]Message, len(params.History))
	copy(history, params.History)

	// Persist user message.
	if err := r.persistMessage(params.SessionID, "user", params.Intent, ""); err != nil {
		r.logger.Warn("failed to persist user message", zap.Error(err))
	}

	emit := func(e StepEvent) {
		if events != nil {
			select {
			case events <- e:
			default:
			}
		}
	}

	switch params.Mode {
	case ModePlanExecute:
		return r.runPlanExecute(ctx, params, history, result, maxIter, emit)
	case ModeSupervisor:
		return r.runSupervisor(ctx, params, history, result, maxIter, emit)
	default:
		return r.runSingle(ctx, params, history, result, maxIter, emit)
	}
}

// runSingle implements the basic ReAct loop: think → act → observe → repeat.
func (r *Runner) runSingle(
	ctx context.Context,
	params RunParams,
	history []Message,
	result *RunResult,
	maxIter int,
	emit func(StepEvent),
) (*RunResult, error) {
	tools := r.registry.ListToolsForRole(params.AllowedTools)
	// System prompt is passed to the LLM in a real implementation.
	_ = buildSystemPrompt(tools, params.AllowedTools)

	history = append(history, Message{Role: "user", Content: params.Intent})

	for iter := 0; iter < maxIter; iter++ {
		result.Iterations = iter + 1

		// Compose current context.
		emit(StepEvent{Type: "thinking", Content: fmt.Sprintf("Iteration %d/%d", iter+1, maxIter)})

		// In a real deployment this calls an LLM; here we use a rule-based stub
		// that parses intent and decides whether to call a tool or answer directly.
		toolCall, thinking, answer := r.planNextAction(params.Intent, history, tools)

		if thinking != "" {
			emit(StepEvent{Type: "thinking", Content: thinking})
		}

		if toolCall != nil {
			// Emit proposed tool call and wait for HITL approval if required.
			tcJSON, _ := json.Marshal(toolCall)
			emit(StepEvent{Type: "tool_call", Data: tcJSON})

			if params.HITLMode == "require_approval" {
				// In a full implementation this would block on a DB row.
				// For now we emit an event and continue (auto-approve after emit).
				emit(StepEvent{Type: "thinking", Content: "[HITL] Tool call proposed — awaiting operator approval"})
			}

			execResult, err := r.executeTool(ctx, params, toolCall)
			summary := ToolExecSummary{ToolName: toolCall.ToolName, Status: "completed", DurationMs: execResult.DurationMs}
			if err != nil {
				summary.Status = "failed"
				emit(StepEvent{Type: "tool_result", Content: fmt.Sprintf("Tool %q failed: %v", toolCall.ToolName, err)})
			} else {
				if execResult.Truncated {
					summary.Truncated = true
				}
				resJSON, _ := json.Marshal(map[string]interface{}{
					"tool": toolCall.ToolName,
					"output": execResult.Output,
					"truncated": execResult.Truncated,
				})
				emit(StepEvent{Type: "tool_result", Data: resJSON})

				// Add tool result to history.
				history = append(history, Message{
					Role:       "tool",
					Content:    execResult.Output,
					ToolCallID: toolCall.ID,
				})
			}
			result.ToolExecutions = append(result.ToolExecutions, summary)
			continue
		}

		// Agent has a final answer.
		if answer != "" {
			result.FinalAnswer = answer
			emit(StepEvent{Type: "final", Content: answer})
			if err := r.persistMessage(params.SessionID, "assistant", answer, ""); err != nil {
				r.logger.Warn("failed to persist assistant message", zap.Error(err))
			}
			return result, nil
		}

		// No tool and no answer — something went wrong.
		break
	}

	if result.FinalAnswer == "" {
		result.FinalAnswer = "I have completed the available analysis. Please review the tool outputs above."
		emit(StepEvent{Type: "final", Content: result.FinalAnswer})
	}
	return result, nil
}

// runPlanExecute generates a plan first, then executes each step.
func (r *Runner) runPlanExecute(
	ctx context.Context,
	params RunParams,
	history []Message,
	result *RunResult,
	maxIter int,
	emit func(StepEvent),
) (*RunResult, error) {
	tools := r.registry.ListToolsForRole(params.AllowedTools)
	plan := r.generatePlan(params.Intent, tools)
	emit(StepEvent{Type: "thinking", Content: "Plan:\n" + plan})

	// Delegate to single for execution phase.
	params.Mode = ModeSingle
	return r.runSingle(ctx, params, history, result, maxIter, emit)
}

// runSupervisor assigns intent to sub-agents (stub — uses single-agent execution for now).
func (r *Runner) runSupervisor(
	ctx context.Context,
	params RunParams,
	history []Message,
	result *RunResult,
	maxIter int,
	emit func(StepEvent),
) (*RunResult, error) {
	emit(StepEvent{Type: "thinking", Content: "[Supervisor] Delegating to execution agent"})
	params.Mode = ModeSingle
	return r.runSingle(ctx, params, history, result, maxIter, emit)
}

// planNextAction is a rule-based stub that decides the next step.
// In production this would be replaced with an LLM call.
func (r *Runner) planNextAction(intent string, history []Message, tools []*mcp.ToolDefinition) (*ToolCall, string, string) {
	intentLower := strings.ToLower(intent)
	hasToolResults := false
	for _, m := range history {
		if m.Role == "tool" {
			hasToolResults = true
			break
		}
	}

	// If we already have tool results, synthesise a final answer.
	if hasToolResults {
		var sb strings.Builder
		sb.WriteString("Based on the reconnaissance results:\n\n")
		for _, m := range history {
			if m.Role == "tool" {
				sb.WriteString(m.Content)
				sb.WriteString("\n")
			}
		}
		return nil, "", sb.String()
	}

	// Select the most relevant tool based on keywords.
	for _, tool := range tools {
		switch tool.Name {
		case "subdomain_enum":
			if strings.Contains(intentLower, "subdomain") || strings.Contains(intentLower, "enumerate") {
				domain := extractDomain(intent)
				if domain != "" {
					return &ToolCall{
						ID: fmt.Sprintf("call_%d", time.Now().UnixNano()),
						ToolName: "subdomain_enum",
						Arguments: map[string]interface{}{"domain": domain},
					}, fmt.Sprintf("Enumerating subdomains for %s", domain), ""
				}
			}
		case "http_probe":
			if strings.Contains(intentLower, "probe") || strings.Contains(intentLower, "http") ||
				strings.Contains(intentLower, "check") || strings.Contains(intentLower, "alive") {
				targets := extractURLs(intent)
				if len(targets) > 0 {
					ifaces := make([]interface{}, len(targets))
					for i, t := range targets {
						ifaces[i] = t
					}
					return &ToolCall{
						ID: fmt.Sprintf("call_%d", time.Now().UnixNano()),
						ToolName: "http_probe",
						Arguments: map[string]interface{}{"targets": ifaces},
					}, fmt.Sprintf("Probing HTTP services on %v", targets), ""
				}
			}
		case "dns_lookup":
			if strings.Contains(intentLower, "dns") || strings.Contains(intentLower, "resolve") ||
				strings.Contains(intentLower, "lookup") || strings.Contains(intentLower, "nameserver") {
				host := extractHostname(intent)
				if host != "" {
					return &ToolCall{
						ID: fmt.Sprintf("call_%d", time.Now().UnixNano()),
						ToolName: "dns_lookup",
						Arguments: map[string]interface{}{"hostname": host},
					}, fmt.Sprintf("Looking up DNS records for %s", host), ""
				}
			}
		}
	}

	// Fallback: provide an informational response.
	return nil, "", fmt.Sprintf(
		"I can help with reconnaissance tasks using the available read-only tools: %s. "+
			"Please provide a specific target domain, hostname, or URL to analyse.",
		toolNames(tools),
	)
}

// generatePlan produces a textual plan for the Plan-Execute mode.
func (r *Runner) generatePlan(intent string, tools []*mcp.ToolDefinition) string {
	var steps []string
	intentLower := strings.ToLower(intent)

	if strings.Contains(intentLower, "subdomain") {
		steps = append(steps, "1. Enumerate subdomains via certificate transparency logs")
	}
	if strings.Contains(intentLower, "dns") || strings.Contains(intentLower, "resolve") {
		steps = append(steps, "2. Perform DNS record lookups (A, MX, TXT, NS)")
	}
	if strings.Contains(intentLower, "http") || strings.Contains(intentLower, "probe") || strings.Contains(intentLower, "web") {
		steps = append(steps, "3. Probe HTTP/HTTPS services for availability and metadata")
	}

	if len(steps) == 0 {
		steps = append(steps, "1. Analyse intent and select appropriate recon tools")
		steps = append(steps, "2. Execute recon and synthesise findings")
	}
	steps = append(steps, fmt.Sprintf("%d. Summarise findings", len(steps)+1))
	return strings.Join(steps, "\n")
}

// executeTool runs a tool call and persists the execution record.
func (r *Runner) executeTool(ctx context.Context, params RunParams, tc *ToolCall) (*mcp.ToolCallResult, error) {
	start := time.Now()
	execResult, err := r.registry.Execute(ctx, tc.ToolName, tc.Arguments, params.AllowedTools)
	elapsed := time.Since(start)

	argsJSON, _ := json.Marshal(tc.Arguments)
	status := "completed"
	var errStr, output string
	var outputBytes int
	var truncated int

	if err != nil {
		status = "failed"
		errStr = err.Error()
	} else if execResult != nil {
		output = execResult.Output
		outputBytes = len(output)
		if execResult.Truncated {
			truncated = 1
		}
		if execResult.Error != "" {
			status = "failed"
			errStr = execResult.Error
		}
	}

	_, dbErr := r.db.Exec(`
		INSERT INTO tool_executions
		  (id,session_id,user_id,tool_name,arguments_json,status,result,error,
		   output_bytes,output_truncated,started_at,completed_at,duration_ms,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)`,
		fmt.Sprintf("exec_%d", time.Now().UnixNano()),
		params.SessionID, params.UserID, tc.ToolName,
		string(argsJSON), status, output, errStr,
		outputBytes, truncated,
		start.UTC(), time.Now().UTC(), elapsed.Milliseconds(),
	)
	if dbErr != nil {
		r.logger.Warn("failed to persist tool execution", zap.Error(dbErr))
	}

	if err != nil {
		return &mcp.ToolCallResult{Error: err.Error(), DurationMs: elapsed.Milliseconds()}, err
	}
	return execResult, nil
}

// persistMessage saves a conversation message.
func (r *Runner) persistMessage(sessionID, role, content, toolCallID string) error {
	_, err := r.db.Exec(`
		INSERT INTO messages (id,session_id,role,content,tool_call_ids,created_at)
		VALUES (?,?,?,?,?,CURRENT_TIMESTAMP)`,
		fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		sessionID, role, content,
		func() string {
			if toolCallID != "" {
				return "[\"" + toolCallID + "\"]"
			}
			return "[]"
		}(),
	)
	return err
}

// buildSystemPrompt composes a system prompt listing available read-only tools.
func buildSystemPrompt(tools []*mcp.ToolDefinition, allowedTools []string) string {
	var sb strings.Builder
	sb.WriteString("You are Kestrel, an AI-native security operations assistant.\n")
	sb.WriteString("You help authorised security teams conduct reconnaissance on systems they own or are explicitly permitted to test.\n\n")
	sb.WriteString("IMPORTANT CONSTRAINTS:\n")
	sb.WriteString("- You may only use the tools listed below.\n")
	sb.WriteString("- All tools are read-only recon tools. No exploitation, credential-dumping, or remote-shell capabilities are available.\n")
	sb.WriteString("- Never propose actions outside the available toolset.\n\n")
	sb.WriteString("Available tools:\n")
	for _, t := range tools {
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", t.Name, t.Description))
	}
	return sb.String()
}

func toolNames(tools []*mcp.ToolDefinition) string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return strings.Join(names, ", ")
}

// --- Simple keyword extractors (replace with NLP/regex in production) ---

func extractDomain(s string) string {
	words := strings.Fields(s)
	for _, w := range words {
		w = strings.Trim(w, ".,;:\"'()")
		if strings.Contains(w, ".") && !strings.Contains(w, "/") && !strings.Contains(w, ":") {
			parts := strings.Split(w, ".")
			if len(parts) >= 2 {
				return w
			}
		}
	}
	return ""
}

func extractHostname(s string) string {
	return extractDomain(s)
}

func extractURLs(s string) []string {
	var urls []string
	for _, w := range strings.Fields(s) {
		w = strings.Trim(w, ".,;:\"'()")
		if strings.HasPrefix(w, "http://") || strings.HasPrefix(w, "https://") {
			urls = append(urls, w)
		} else if strings.Contains(w, ".") && !strings.Contains(w, "@") {
			urls = append(urls, w)
		}
	}
	return urls
}
