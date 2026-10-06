package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"kestrel/internal/config"
	"kestrel/internal/toolguard"
)

// ToolClass categorises tools by risk level. Only ReadOnly tools are wired in by default.
type ToolClass string

const (
	ToolClassReadOnly    ToolClass = "read_only"
	ToolClassDestructive ToolClass = "destructive" // not loaded by default
)

// ToolDefinition describes a callable tool.
type ToolDefinition struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Class       ToolClass         `json:"class"`
	Parameters  json.RawMessage   `json:"parameters"` // JSON Schema
	ServerID    string            `json:"server_id"`
}

// ToolCallResult carries the output of a tool invocation.
type ToolCallResult struct {
	Output    string `json:"output"`
	Truncated bool   `json:"truncated"`
	DurationMs int64 `json:"duration_ms"`
	Error     string `json:"error,omitempty"`
}

// ExecutionStatus represents the state of an async tool call.
type ExecutionStatus string

const (
	StatusPending   ExecutionStatus = "pending"
	StatusRunning   ExecutionStatus = "running"
	StatusCompleted ExecutionStatus = "completed"
	StatusFailed    ExecutionStatus = "failed"
	StatusCancelled ExecutionStatus = "cancelled"
)

// Execution tracks an async tool call.
type Execution struct {
	ID         string
	ToolName   string
	Arguments  map[string]interface{}
	Status     ExecutionStatus
	Result     *ToolCallResult
	StartedAt  time.Time
	cancelFunc context.CancelFunc
	mu         sync.Mutex
}

// Handler is a function that executes a tool call.
type Handler func(ctx context.Context, args map[string]interface{}) (string, error)

// Registry manages available tools and their handlers.
type Registry struct {
	cfg          *config.MCPConfig
	logger       *zap.Logger
	tools        map[string]*ToolDefinition
	handlers     map[string]Handler
	executions   map[string]*Execution
	workerPool   chan struct{}
	outputCapB   int
	guardMgr     *toolguard.Manager
	mu           sync.RWMutex
	execMu       sync.RWMutex
}

// NewRegistry creates a tool registry and pre-registers built-in recon tools.
// guardCfg is the toolguard policy; pass nil to use the default government-domain rule.
func NewRegistryWithGuard(cfg *config.MCPConfig, guardCfg *toolguard.Config, logger *zap.Logger) *Registry {
	var gc toolguard.Config
	if guardCfg != nil {
		gc = *guardCfg
	} else {
		gc = toolguard.DefaultConfig()
	}
	mgr, err := toolguard.NewManager(gc)
	if err != nil {
		logger.Warn("toolguard: failed to compile policy, using default", zap.Error(err))
		defCfg := toolguard.DefaultConfig()
		mgr, _ = toolguard.NewManager(defCfg)
	}
	return newRegistry(cfg, mgr, logger)
}

// NewRegistry creates a tool registry with the default toolguard policy.
func NewRegistry(cfg *config.MCPConfig, logger *zap.Logger) *Registry {
	return NewRegistryWithGuard(cfg, nil, logger)
}

func newRegistry(cfg *config.MCPConfig, guardMgr *toolguard.Manager, logger *zap.Logger) *Registry {
	poolSize := cfg.WorkerPoolSize
	if poolSize <= 0 {
		poolSize = 4
	}
	capB := cfg.OutputCapBytes
	if capB <= 0 {
		capB = 102400
	}

	r := &Registry{
		cfg:        cfg,
		logger:     logger,
		tools:      make(map[string]*ToolDefinition),
		handlers:   make(map[string]Handler),
		executions: make(map[string]*Execution),
		workerPool: make(chan struct{}, poolSize),
		outputCapB: capB,
		guardMgr:   guardMgr,
	}

	// Pre-fill the worker pool semaphore.
	for i := 0; i < poolSize; i++ {
		r.workerPool <- struct{}{}
	}

	// Register built-in read-only recon tools.
	r.registerBuiltins()
	return r
}

// RegisterTool adds a tool and its handler to the registry.
func (r *Registry) RegisterTool(def *ToolDefinition, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[def.Name] = def
	r.handlers[def.Name] = handler
}

// GetTool returns a tool definition by name.
func (r *Registry) GetTool(name string) (*ToolDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// ListTools returns all registered tool definitions.
func (r *Registry) ListTools() []*ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// ListToolsForRole returns tools allowed for the given allowlist. Pass nil to return all tools.
func (r *Registry) ListToolsForRole(allowedTools []string) []*ToolDefinition {
	if len(allowedTools) == 0 {
		return r.ListTools()
	}
	allowed := make(map[string]bool, len(allowedTools))
	for _, t := range allowedTools {
		allowed[t] = true
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*ToolDefinition
	for name, t := range r.tools {
		if allowed[name] || allowed["*"] {
			out = append(out, t)
		}
	}
	return out
}

// Execute runs a tool call synchronously, respecting the worker pool limit.
// Returns the execution result or an error.
func (r *Registry) Execute(ctx context.Context, toolName string, args map[string]interface{}, allowedTools []string) (*ToolCallResult, error) {
	r.mu.RLock()
	def, hasDef := r.tools[toolName]
	handler, hasHandler := r.handlers[toolName]
	r.mu.RUnlock()

	if !hasDef || !hasHandler {
		return nil, fmt.Errorf("tool %q not found", toolName)
	}

	// Allowlist check.
	if !isAllowed(toolName, allowedTools) {
		return nil, fmt.Errorf("tool %q is not permitted for this role", toolName)
	}

	// Only read-only tools are allowed by default guards.
	if def.Class == ToolClassDestructive {
		return nil, errors.New("destructive tools are not enabled in this deployment")
	}

	// ToolGuard: check configured blocking rules (e.g. government domains).
	if r.guardMgr != nil {
		if m := r.guardMgr.Check(toolName, args); m != nil {
			r.logger.Warn("toolguard blocked tool call",
				zap.String("tool", toolName),
				zap.String("rule", m.RuleID),
				zap.String("matched", m.MatchedText),
			)
			return nil, fmt.Errorf("blocked by security rule %q: %s", m.RuleID, m.Message)
		}
	}

	// Acquire a worker pool slot.
	timeout := time.Duration(r.cfg.CallTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	select {
	case <-r.workerPool:
		// Got a slot.
	case <-time.After(timeout):
		return nil, errors.New("worker pool timeout: all workers busy")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { r.workerPool <- struct{}{} }()

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	output, err := handler(callCtx, args)
	elapsed := time.Since(start)

	result := &ToolCallResult{
		DurationMs: elapsed.Milliseconds(),
	}
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	// Cap output size.
	if len(output) > r.outputCapB {
		output = output[:r.outputCapB]
		result.Truncated = true
	}
	result.Output = output
	return result, nil
}

// isAllowed checks if a tool is in an allowlist. An empty allowlist means all allowed.
func isAllowed(toolName string, allowedTools []string) bool {
	if len(allowedTools) == 0 {
		return true
	}
	for _, t := range allowedTools {
		if t == toolName || t == "*" {
			return true
		}
	}
	return false
}
