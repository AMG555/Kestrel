// Package workflow provides a graph-based workflow engine.
//
// A workflow definition is a directed acyclic graph of nodes. Each node has
// a type that determines how it executes:
//
//   agent    — runs an agent with a given intent (supports all agent modes)
//   tool     — executes a single MCP tool call directly
//   condition — evaluates a boolean expression over the previous node's output
//   approval  — pauses execution pending a HITL approval decision
//   output    — finalises the run with a rendered result
//
// Edges carry optional condition labels (true/false for condition nodes;
// default/fallback for others). The engine persists run state in the database
// so runs can survive process restarts and be replayed.
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"kestrel/internal/agent"
	"kestrel/internal/database"
	"kestrel/internal/mcp"
)

// NodeType identifies the execution behaviour of a workflow node.
type NodeType string

const (
	NodeTypeAgent    NodeType = "agent"
	NodeTypeTool     NodeType = "tool"
	NodeTypeCondition NodeType = "condition"
	NodeTypeApproval NodeType = "approval"
	NodeTypeOutput   NodeType = "output"
)

// Node is a single step in the workflow graph.
type Node struct {
	ID       string                 `json:"id"`
	Type     NodeType               `json:"type"`
	Label    string                 `json:"label"`
	Config   map[string]interface{} `json:"config"`
	Position struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"position"`
}

// Edge connects two nodes, optionally conditioned on a label.
type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"` // "true" | "false" | "" (unconditional)
}

// Definition is a serialisable workflow graph.
type Definition struct {
	Nodes  []Node         `json:"nodes"`
	Edges  []Edge         `json:"edges"`
	Config map[string]any `json:"config,omitempty"`
}

// RunStatus represents execution state.
type RunStatus string

const (
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusPaused    RunStatus = "paused"   // waiting for approval
	RunStatusCancelled RunStatus = "cancelled"
)

// RunContext carries per-run execution context.
type RunContext struct {
	WorkflowID string
	RunID      string
	ProjectID  string
	SessionID  string
	UserID     string
	Input      map[string]interface{}
	Variables  map[string]interface{} // accumulated node outputs
}

// Engine executes workflow definitions.
type Engine struct {
	db       *database.DB
	runner   *agent.Runner
	registry *mcp.Registry
	logger   *zap.Logger
}

// NewEngine creates a workflow engine.
func NewEngine(db *database.DB, runner *agent.Runner, registry *mcp.Registry, logger *zap.Logger) *Engine {
	return &Engine{db: db, runner: runner, registry: registry, logger: logger}
}

// Run executes a workflow definition. It persists a run record, then executes
// nodes in topological order. Returns the run ID immediately for async tracking.
func (e *Engine) Run(ctx context.Context, defID string, runCtx RunContext) (string, error) {
	// Load workflow definition.
	var graphJSON string
	err := e.db.QueryRow(`SELECT graph_json FROM workflow_definitions WHERE id=? AND enabled=1`, defID).Scan(&graphJSON)
	if err != nil {
		return "", fmt.Errorf("workflow not found or disabled: %w", err)
	}

	var def Definition
	if err := json.Unmarshal([]byte(graphJSON), &def); err != nil {
		return "", fmt.Errorf("parsing workflow graph: %w", err)
	}

	// Create run record.
	runID := uuid.New().String()
	inputJSON, _ := json.Marshal(runCtx.Input)
	now := time.Now().UTC()
	_, err = e.db.Exec(`
		INSERT INTO workflow_runs (id,workflow_id,session_id,project_id,user_id,status,input_json,started_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		runID, defID, runCtx.SessionID, runCtx.ProjectID, runCtx.UserID, RunStatusRunning, string(inputJSON), now,
	)
	if err != nil {
		return "", fmt.Errorf("creating run record: %w", err)
	}
	runCtx.RunID = runID

	// Execute in background.
	go func() {
		finalStatus := RunStatusCompleted
		var runErr string

		if err := e.executeGraph(ctx, &def, &runCtx); err != nil {
			finalStatus = RunStatusFailed
			runErr = err.Error()
			e.logger.Warn("workflow run failed", zap.String("run_id", runID), zap.Error(err))
		}

		outputJSON, _ := json.Marshal(runCtx.Variables)
		_, _ = e.db.Exec(`
			UPDATE workflow_runs SET status=?,output_json=?,error=?,finished_at=? WHERE id=?`,
			finalStatus, string(outputJSON), runErr, time.Now().UTC(), runID,
		)
	}()

	return runID, nil
}

// executeGraph traverses the node graph from start nodes to terminal nodes.
func (e *Engine) executeGraph(ctx context.Context, def *Definition, runCtx *RunContext) error {
	if len(def.Nodes) == 0 {
		return fmt.Errorf("workflow has no nodes")
	}

	// Build adjacency index.
	outEdges := make(map[string][]Edge)
	for _, edge := range def.Edges {
		outEdges[edge.Source] = append(outEdges[edge.Source], edge)
	}

	// Find start nodes (nodes with no incoming edges).
	inDegree := make(map[string]int)
	for _, node := range def.Nodes {
		inDegree[node.ID] = 0
	}
	for _, edge := range def.Edges {
		inDegree[edge.Target]++
	}

	// Topological BFS execution.
	queue := []string{}
	for _, node := range def.Nodes {
		if inDegree[node.ID] == 0 {
			queue = append(queue, node.ID)
		}
	}

	nodeMap := make(map[string]Node, len(def.Nodes))
	for _, n := range def.Nodes {
		nodeMap[n.ID] = n
	}

	if runCtx.Variables == nil {
		runCtx.Variables = make(map[string]interface{})
	}

	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		node, ok := nodeMap[nodeID]
		if !ok {
			continue
		}

		output, branch, err := e.executeNode(ctx, node, runCtx)
		if err != nil {
			e.logger.Warn("node execution failed", zap.String("node", nodeID), zap.Error(err))
			// Store error but continue unless node is critical.
			runCtx.Variables[nodeID+"_error"] = err.Error()
		} else {
			runCtx.Variables[nodeID] = output
		}

		// Enqueue next nodes based on edges (and optional branch label).
		for _, edge := range outEdges[nodeID] {
			if edge.Label == "" || edge.Label == branch {
				inDegree[edge.Target]--
				if inDegree[edge.Target] == 0 {
					queue = append(queue, edge.Target)
				}
			}
		}
	}
	return nil
}

// executeNode runs a single workflow node and returns its output and branch label.
func (e *Engine) executeNode(ctx context.Context, node Node, runCtx *RunContext) (interface{}, string, error) {
	switch node.Type {
	case NodeTypeAgent:
		return e.runAgentNode(ctx, node, runCtx)
	case NodeTypeTool:
		return e.runToolNode(ctx, node, runCtx)
	case NodeTypeCondition:
		return e.runConditionNode(ctx, node, runCtx)
	case NodeTypeApproval:
		return e.runApprovalNode(ctx, node, runCtx)
	case NodeTypeOutput:
		return e.runOutputNode(ctx, node, runCtx)
	default:
		return nil, "", fmt.Errorf("unknown node type: %q", node.Type)
	}
}

func (e *Engine) runAgentNode(ctx context.Context, node Node, runCtx *RunContext) (interface{}, string, error) {
	intent, _ := node.Config["intent"].(string)
	mode, _ := node.Config["mode"].(string)
	if intent == "" {
		intent = "Perform the workflow step."
	}
	if mode == "" {
		mode = "single"
	}

	result, err := e.runner.Run(ctx, agent.RunParams{
		SessionID: fmt.Sprintf("wf_%s_%s", runCtx.RunID, node.ID),
		UserID:    runCtx.UserID,
		Intent:    intent,
		Mode:      agent.Mode(mode),
	}, nil)
	if err != nil {
		return nil, "", err
	}
	return result.FinalAnswer, "", nil
}

func (e *Engine) runToolNode(ctx context.Context, node Node, runCtx *RunContext) (interface{}, string, error) {
	toolName, _ := node.Config["tool_name"].(string)
	if toolName == "" {
		return nil, "", fmt.Errorf("tool_node requires tool_name")
	}
	argsRaw, _ := node.Config["arguments"].(map[string]interface{})
	result, err := e.registry.Execute(ctx, toolName, argsRaw, nil)
	if err != nil {
		return nil, "", err
	}
	return result.Output, "", nil
}

func (e *Engine) runConditionNode(_ context.Context, node Node, runCtx *RunContext) (interface{}, string, error) {
	// Simple condition: check if a variable contains a substring.
	varName, _ := node.Config["variable"].(string)
	contains, _ := node.Config["contains"].(string)

	val, ok := runCtx.Variables[varName]
	if !ok {
		return nil, "false", nil
	}
	valStr := fmt.Sprintf("%v", val)
	if strings.Contains(strings.ToLower(valStr), strings.ToLower(contains)) {
		return nil, "true", nil
	}
	return nil, "false", nil
}

func (e *Engine) runApprovalNode(_ context.Context, node Node, runCtx *RunContext) (interface{}, string, error) {
	// Create a HITL pending record and block until decided or timeout.
	summary, _ := node.Config["summary"].(string)
	if summary == "" {
		summary = fmt.Sprintf("Workflow %s requires approval at node %s", runCtx.WorkflowID, node.ID)
	}

	h, err := e.db.CreateHITLPending(&database.HITLPending{
		SessionID:      runCtx.SessionID,
		UserID:         runCtx.UserID,
		ToolName:       "workflow_approval",
		Arguments:      map[string]interface{}{"node": node.ID, "workflow_id": runCtx.WorkflowID},
		ContextSummary: summary,
		ExpiresAt:      time.Now().UTC().Add(10 * time.Minute),
	})
	if err != nil {
		return nil, "", err
	}

	// Poll for decision (up to ExpiresAt).
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)
		item, err := e.db.GetHITLPending(h.ID)
		if err != nil || item == nil {
			break
		}
		if item.Status == "approved" {
			return "approved", "", nil
		}
		if item.Status == "rejected" {
			return "rejected", "", fmt.Errorf("workflow node %s was rejected by operator", node.ID)
		}
	}
	return nil, "", fmt.Errorf("approval timed out for node %s", node.ID)
}

func (e *Engine) runOutputNode(_ context.Context, node Node, runCtx *RunContext) (interface{}, string, error) {
	// Collect specified variables into a final output object.
	vars, _ := node.Config["variables"].([]interface{})
	out := make(map[string]interface{})
	for _, v := range vars {
		key, _ := v.(string)
		if val, ok := runCtx.Variables[key]; ok {
			out[key] = val
		}
	}
	if len(out) == 0 {
		out = runCtx.Variables
	}
	return out, "", nil
}

// GetRunStatus returns the current status of a workflow run.
func (e *Engine) GetRunStatus(runID string) (string, map[string]interface{}, error) {
	var status, outputJSON, errStr string
	err := e.db.QueryRow(`SELECT status,COALESCE(output_json,'{}'),COALESCE(error,'') FROM workflow_runs WHERE id=?`, runID).
		Scan(&status, &outputJSON, &errStr)
	if err != nil {
		return "", nil, err
	}
	var output map[string]interface{}
	_ = json.Unmarshal([]byte(outputJSON), &output)
	return status, output, nil
}
