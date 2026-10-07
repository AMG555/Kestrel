package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"kestrel/internal/multiagent"
	"kestrel/internal/runlease"
	"kestrel/internal/security"
)

// ErrTaskCancelled is the error returned when a user cancels a task
var ErrTaskCancelled = errors.New("agent task cancelled by user")

// ErrTaskAlreadyRunning indicates a task is already running in the conversation
var ErrTaskAlreadyRunning = errors.New("agent task already running for conversation")

// shouldPersistEinoAgentTraceAfterRunError: whether to still write last_react_* for use by loadHistoryFromAgentTrace in the next round, even when an Eino Run returns non-success.
// Current policy: regardless of normal termination, abnormal termination, or user-initiated stop, always try to preserve the last usable trace,
// so that when continuing in the same conversation the run can resume from the original context rather than falling back to message-text history only.
func shouldPersistEinoAgentTraceAfterRunError(baseCtx context.Context) bool {
	return true
}

// AgentTask describes a currently running Agent task
type AgentTask struct {
	RunID            string `json:"runId"`
	CleanupError     string `json:"cleanupError,omitempty"`
	processes        *security.ProcessScope
	workers          *runlease.Scope
	IsolationBackend string `json:"isolationBackend,omitempty"`
	finishing        chan struct{}
	stopping         chan struct{}
	cancelRequested  bool
	finalStatus      string
	ConversationID   string    `json:"conversationId"`
	Title            string    `json:"title,omitempty"`
	Message          string    `json:"message,omitempty"`
	StartedAt        time.Time `json:"startedAt"`
	Status           string    `json:"status"`
	CancellingAt     time.Time `json:"-"` // time of entering cancelling status; used to clean up tasks stuck for too long

	// ActiveMCPExecutionID is the executionId of the currently executing MCP tool (in-memory only; used for "interrupt and continue" = cancel only the current tool)
	ActiveMCPExecutionID string `json:"-"`

	// InterruptContinueNote is the supplement note filled by the user in the popup for "interrupt and continue" when no MCP tool is running (written before Cancel, cleared after the resuming round reads it)
	InterruptContinueNote string `json:"-"`

	// activeEinoExecuteCancel is the cancel function for the currently in-progress Eino filesystem execute (parallel to MCP tool; used for interrupt and continue)
	activeEinoExecuteCancel context.CancelFunc
	// activeEinoExecuteAbortNote is the user note written by AbortActiveEinoExecute; merged into tool result at execute completion
	activeEinoExecuteAbortNote string

	// hitlCognition is the context available to the HITL/audit Agent for this run (user message + thinking; no conversation history)
	hitlCognition *hitlCognitionState

	// agentRuntimeCancel wraps the current Eino ADK native AgentCancelFunc; triggered first when cancelling a task, then context cancellation as fallback.
	agentRuntimeCancel        func(error) bool
	agentRuntimeCancelVersion uint64

	// agentTurnLoopInterrupt is the current Eino TurnLoop user-supplement push hook; when interrupting and continuing, the supplement is queued as a new turn item first.
	agentTurnLoopInterrupt        func(string) bool
	agentTurnLoopInterruptVersion uint64

	cancel func(error)
}

// RegisterRunningTool implements mcp.ToolRunRegistry: registers the current executionId for this conversation when a tool starts.
func (m *AgentTaskManager) RegisterRunningTool(conversationID, executionID string) {
	conversationID = strings.TrimSpace(conversationID)
	executionID = strings.TrimSpace(executionID)
	if conversationID == "" || executionID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.tasks[conversationID]; ok && t != nil {
		t.ActiveMCPExecutionID = executionID
	}
}

// UnregisterRunningTool clears the registration when a tool ends (only clears if the id still matches, to avoid concurrent cross-contamination).
func (m *AgentTaskManager) UnregisterRunningTool(conversationID, executionID string) {
	conversationID = strings.TrimSpace(conversationID)
	executionID = strings.TrimSpace(executionID)
	if conversationID == "" || executionID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.tasks[conversationID]; ok && t != nil {
		if t.ActiveMCPExecutionID == executionID {
			t.ActiveMCPExecutionID = ""
		}
	}
}

// RegisterActiveEinoExecute registers an in-progress Eino filesystem execute (at most one per conversation at a time).
func (m *AgentTaskManager) RegisterActiveEinoExecute(conversationID string, cancel context.CancelFunc) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" || cancel == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.tasks[conversationID]; ok && t != nil {
		t.activeEinoExecuteCancel = cancel
		t.activeEinoExecuteAbortNote = ""
	}
}

// UnregisterActiveEinoExecute clears the registration after execute finishes normally or is cancelled.
func (m *AgentTaskManager) UnregisterActiveEinoExecute(conversationID string) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.tasks[conversationID]; ok && t != nil {
		t.activeEinoExecuteCancel = nil
		t.activeEinoExecuteAbortNote = ""
	}
}

// ConversationIDForActiveMCPExecution looks up the conversation ID from the currently registered tool executionId (used by the MCP monitoring page to terminate by executionId).
func (m *AgentTaskManager) ConversationIDForActiveMCPExecution(executionID string) string {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for convID, t := range m.tasks {
		if t != nil && t.ActiveMCPExecutionID == executionID {
			return convID
		}
	}
	return ""
}

// ConversationIDForActiveEinoExecute returns the conversation ID of the single active Eino execute; returns empty when multiple conversations run in parallel.
func (m *AgentTaskManager) ConversationIDForActiveEinoExecute() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var found string
	count := 0
	for convID, t := range m.tasks {
		if t != nil && t.activeEinoExecuteCancel != nil {
			found = convID
			count++
		}
	}
	if count == 1 {
		return found, true
	}
	return "", false
}

// AbortActiveEinoExecute terminates the current Eino execute and stores the user note (consistent with MCP tool termination).
func (m *AgentTaskManager) AbortActiveEinoExecute(conversationID, note string) bool {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return false
	}
	m.mu.Lock()
	t, ok := m.tasks[conversationID]
	if !ok || t == nil || t.activeEinoExecuteCancel == nil {
		m.mu.Unlock()
		return false
	}
	t.activeEinoExecuteAbortNote = strings.TrimSpace(note)
	cancel := t.activeEinoExecuteCancel
	m.mu.Unlock()
	cancel()
	return true
}

// TakeEinoExecuteAbortNote reads and clears the execute abort note (called once at execute completion).
func (m *AgentTaskManager) TakeEinoExecuteAbortNote(conversationID string) string {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.tasks[conversationID]; ok && t != nil {
		n := t.activeEinoExecuteAbortNote
		t.activeEinoExecuteAbortNote = ""
		return n
	}
	return ""
}

// SetInterruptContinueNote writes the user supplement note before initiating an ErrInterruptContinue cancel (in-memory only).
func (m *AgentTaskManager) SetInterruptContinueNote(conversationID, note string) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.tasks[conversationID]; ok && t != nil {
		t.InterruptContinueNote = note
	}
}

// TakeInterruptContinueNote reads and clears the supplement note (called once when the resuming run starts).
func (m *AgentTaskManager) TakeInterruptContinueNote(conversationID string) string {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.tasks[conversationID]; ok && t != nil {
		n := t.InterruptContinueNote
		t.InterruptContinueNote = ""
		return n
	}
	return ""
}

// BindTaskCancel replaces the context-bound cancel function within the same running task (used when switching to a new baseCtx after interrupt-and-continue).
func (m *AgentTaskManager) BindTaskCancel(conversationID string, cancel context.CancelCauseFunc) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" || cancel == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.tasks[conversationID]; ok && t != nil {
		t.cancel = func(err error) {
			cancel(err)
		}
	}
}

// BindAgentRuntimeCancel registers the Eino native cancel hook for the current run segment.
func (m *AgentTaskManager) BindAgentRuntimeCancel(conversationID string, cancel func(error) bool) func() {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" || cancel == nil {
		return func() {}
	}
	m.mu.Lock()
	t, ok := m.tasks[conversationID]
	if !ok || t == nil {
		m.mu.Unlock()
		return func() {}
	}
	t.agentRuntimeCancelVersion++
	version := t.agentRuntimeCancelVersion
	t.agentRuntimeCancel = cancel
	m.mu.Unlock()

	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if cur, exists := m.tasks[conversationID]; exists && cur != nil && cur.agentRuntimeCancelVersion == version {
			cur.agentRuntimeCancel = nil
		}
	}
}

// BindAgentTurnLoopInterrupt registers the Eino TurnLoop user-supplement enqueue hook for the current running task.
func (m *AgentTaskManager) BindAgentTurnLoopInterrupt(conversationID string, push func(string) bool) func() {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" || push == nil {
		return func() {}
	}
	m.mu.Lock()
	t, ok := m.tasks[conversationID]
	if !ok || t == nil {
		m.mu.Unlock()
		return func() {}
	}
	t.agentTurnLoopInterruptVersion++
	version := t.agentTurnLoopInterruptVersion
	t.agentTurnLoopInterrupt = push
	m.mu.Unlock()

	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if cur, exists := m.tasks[conversationID]; exists && cur != nil && cur.agentTurnLoopInterruptVersion == version {
			cur.agentTurnLoopInterrupt = nil
		}
	}
}

// ActiveMCPExecutionID returns the executionId of the in-progress tool for the current conversation; empty string if none.
func (m *AgentTaskManager) ActiveMCPExecutionID(conversationID string) string {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t, ok := m.tasks[conversationID]; ok && t != nil {
		return strings.TrimSpace(t.ActiveMCPExecutionID)
	}
	return ""
}

// CompletedTask is a completed task (for historical records)
type CompletedTask struct {
	CleanupError     string    `json:"cleanupError,omitempty"`
	IsolationBackend string    `json:"isolationBackend,omitempty"`
	RunID            string    `json:"runId"`
	ConversationID   string    `json:"conversationId"`
	Title            string    `json:"title,omitempty"`
	Message          string    `json:"message,omitempty"`
	StartedAt        time.Time `json:"startedAt"`
	CompletedAt      time.Time `json:"completedAt"`
	Status           string    `json:"status"`
}

// AgentTaskManager manages currently running Agent tasks
type AgentTaskManager struct {
	mu               sync.RWMutex
	tasks            map[string]*AgentTask
	completedTasks   []*CompletedTask // recently completed task history
	maxHistorySize   int              // maximum history record count
	historyRetention time.Duration    // history record retention duration
	eventBus         *TaskEventBus    // optional: close mirrored SSE subscriptions when task ends
	// toolCanceler terminates any still-running MCP tools for the session when the user stops the whole task or the session ends (not for 'interrupt and continue').
	toolCanceler func(conversationID string)
	shuttingDown bool
	shutdown     chan struct{}
	shutdownOnce sync.Once
}

const (
	// cancellingStuckThreshold: force-remove from the running list if stuck in 'cancelling' longer than this duration. Normal cancellations return within the current step;
	// if exceeded, treat as stuck and release the session as soon as possible. Common practice is to release within 30–60s.
	cancellingStuckThreshold = 45 * time.Second
	// cancellingStuckThresholdLegacy is the fallback duration using StartedAt when CancellingAt is not recorded
	cancellingStuckThresholdLegacy = 2 * time.Minute
	cleanupInterval                = 15 * time.Second // paired with the threshold above; removes within approximately 60s
)

// NewAgentTaskManager creates a task manager
func NewAgentTaskManager() *AgentTaskManager {
	m := &AgentTaskManager{
		tasks:            make(map[string]*AgentTask),
		shutdown:         make(chan struct{}),
		completedTasks:   make([]*CompletedTask, 0),
		maxHistorySize:   50,             // keep at most 50 history records
		historyRetention: 24 * time.Hour, // retain for 24 hours
	}
	go m.runStuckCancellingCleanup()
	return m
}

// SetTaskEventBus sets the task event bus (shared instance with AgentHandler).
func (m *AgentTaskManager) SetTaskEventBus(b *TaskEventBus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventBus = b
}

// SetToolCanceler sets the callback for terminating still-running MCP tools when the whole task stops / session ends (injected by AgentHandler).
func (m *AgentTaskManager) SetToolCanceler(fn func(conversationID string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolCanceler = fn
}

// GetTask returns the running task (nil if none).
func (m *AgentTaskManager) GetTask(conversationID string) *AgentTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tasks[conversationID]
}

// GetTaskSnapshot returns a read-only copy of the running task for status display, avoiding reads of mutable task fields outside the lock.
func (m *AgentTaskManager) GetTaskSnapshot(conversationID string) *AgentTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task := m.tasks[conversationID]
	if task == nil {
		return nil
	}
	snapshot := *task
	snapshot.IsolationBackend = task.processes.IsolationBackend()
	return &snapshot
}

// runStuckCancellingCleanup periodically force-ends tasks stuck in 'cancelling' state too long, to avoid blocking new messages
func (m *AgentTaskManager) runStuckCancellingCleanup() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.shutdown:
			return
		case <-ticker.C:
			m.cleanupStuckCancelling()
		}
	}
}

func (m *AgentTaskManager) cleanupStuckCancelling() {
	m.mu.Lock()
	type pendingFinish struct{ id, runID, status string }
	var toFinish []pendingFinish
	now := time.Now()
	for id, task := range m.tasks {
		if task.Status == "cleanup_failed" {
			toFinish = append(toFinish, pendingFinish{id, task.RunID, task.finalStatus})
			continue
		}
		if task.Status != "cancelling" {
			continue
		}
		var elapsed time.Duration
		if !task.CancellingAt.IsZero() {
			elapsed = now.Sub(task.CancellingAt)
			if elapsed < cancellingStuckThreshold {
				continue
			}
		} else {
			elapsed = now.Sub(task.StartedAt)
			if elapsed < cancellingStuckThresholdLegacy {
				continue
			}
		}
		toFinish = append(toFinish, pendingFinish{id, task.RunID, "cancelled"})
	}
	m.mu.Unlock()
	for _, pending := range toFinish {
		_ = m.FinishTaskRun(pending.id, pending.runID, pending.status)
	}
}

// StartTask registers and starts a new task
func (m *AgentTaskManager) StartTask(conversationID, message string, cancel context.CancelCauseFunc) (*AgentTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shuttingDown {
		return nil, errors.New("task manager is shutting down")
	}
	if _, exists := m.tasks[conversationID]; exists {
		return nil, ErrTaskAlreadyRunning
	}

	scope := security.NewProcessScope()
	task := &AgentTask{
		RunID: scope.ID, processes: scope, workers: runlease.New(),
		ConversationID: conversationID,
		Message:        message,
		StartedAt:      time.Now(),
		Status:         "running",
		cancel: func(err error) {
			if cancel != nil {
				cancel(err)
			}
		},
	}

	m.tasks[conversationID] = task
	task.hitlCognition = &hitlCognitionState{UserMessage: strings.TrimSpace(message)}
	return task, nil
}

// CancelTask cancels the task for the specified conversation. If the task is already being cancelled, still returns (true, nil) for idempotency and to avoid frontend errors.
func (m *AgentTaskManager) CancelTask(conversationID string, cause error) (bool, error) {
	m.mu.Lock()
	task, exists := m.tasks[conversationID]
	if !exists {
		m.mu.Unlock()
		return false, nil
	}

	// if already in cancelling flow, treat as success (idempotent), avoids frontend duplicate-click reporting 'task not found'
	if task.Status == "cancelling" || task.finishing != nil {
		m.mu.Unlock()
		return true, nil
	}

	if cause == nil || errors.Is(cause, ErrTaskCancelled) {
		task.cancelRequested = true
		task.finalStatus = "cancelled"
	}

	// ErrInterruptContinue: only interrupts the current reasoning step, then the handler resumes; does not enter a long-lasting 'cancelling' state.
	if cause != nil && errors.Is(cause, multiagent.ErrInterruptContinue) {
		task.Status = "running"
	} else {
		task.Status = "cancelling"
		task.CancellingAt = time.Now()
	}
	if cause != nil && errors.Is(cause, ErrTaskCancelled) {
		task.InterruptContinueNote = ""
	}
	cancel := task.cancel
	if cause == nil {
		cause = ErrTaskCancelled
	}
	interruptPush := task.agentTurnLoopInterrupt
	interruptNote := task.InterruptContinueNote
	runtimeCancel := task.agentRuntimeCancel
	activeExecuteCancel := task.activeEinoExecuteCancel
	if !errors.Is(cause, multiagent.ErrInterruptContinue) {
		task.processes.Seal()
		task.workers.Seal()
		task.stopping = make(chan struct{})
		defer close(task.stopping)
	}
	var toolCanceler func(string)
	if errors.Is(cause, ErrTaskCancelled) {
		toolCanceler = m.toolCanceler
	}
	m.mu.Unlock()

	if errors.Is(cause, multiagent.ErrInterruptContinue) && interruptPush != nil && interruptPush(interruptNote) {
		m.mu.Lock()
		if cur, exists := m.tasks[conversationID]; exists && cur != nil {
			cur.InterruptContinueNote = ""
		}
		m.mu.Unlock()
		return true, nil
	}

	runtimeHandled := false
	if runtimeCancel != nil {
		runtimeHandled = runtimeCancel(cause)
	}
	// 'full stop' must also cancel the host context: even if native Agent Cancel was accepted,
	// it may only return at a safe point or report timeout; cannot let the whole task continue surviving.
	// 'interrupt and continue' retains its original semantics: the runtime is responsible for resuming when native cancellation is already handled.
	if cancel != nil && (!runtimeHandled || errors.Is(cause, ErrTaskCancelled)) {
		cancel(cause)
	}
	if toolCanceler != nil {
		toolCanceler(conversationID)
	}
	if !errors.Is(cause, multiagent.ErrInterruptContinue) {
		task.workers.Cancel()
		if activeExecuteCancel != nil {
			activeExecuteCancel()
		}
		return true, task.processes.Close()
	}
	return true, nil
}

// UpdateTaskStatus updates task status without deleting the task (used to update status before sending events)
func (m *AgentTaskManager) UpdateTaskStatus(conversationID string, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, exists := m.tasks[conversationID]
	if !exists {
		return
	}

	if task.finishing != nil || task.Status == "cleanup_failed" {
		return
	}
	if task.cancelRequested {
		status = "cancelled"
	}
	switch status {
	case "completed", "cancelled", "failed", "timeout":
		task.finalStatus = status
		task.Status = "cleaning"
		task.processes.Seal()
		task.workers.Seal()
	default:
		if status != "" {
			task.Status = status
		}
	}
}

// BindProcessScope snapshots ownership once at task start. Continuations must
// derive from this context, never resolve ownership again using conversation ID.
func (m *AgentTaskManager) BindProcessScope(ctx context.Context, conversationID, runID string) context.Context {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if task := m.tasks[conversationID]; task != nil && task.RunID == runID {
		return runlease.WithScope(security.WithProcessScope(ctx, task.processes), task.workers)
	}
	// Fail closed if a task disappeared before its execution context was bound.
	scope := security.NewProcessScope()
	scope.Seal()
	workers := runlease.New()
	workers.Seal()
	return runlease.WithScope(security.WithProcessScope(ctx, scope), workers)
}

// FinishTask is retained for callers that operate on the current task. Owners
// use FinishTaskRun, so a delayed defer cannot finish a newer conversation run.
func (m *AgentTaskManager) FinishTask(conversationID string, finalStatus string) {
	m.mu.RLock()
	task := m.tasks[conversationID]
	m.mu.RUnlock()
	if task != nil {
		_ = m.FinishTaskRun(conversationID, task.RunID, finalStatus)
	}
}

func (m *AgentTaskManager) FinishTaskRun(conversationID, runID, finalStatus string) error {
	m.mu.Lock()
	task := m.tasks[conversationID]
	if task == nil || task.RunID != runID {
		m.mu.Unlock()
		return nil
	}
	if task.stopping != nil {
		select {
		case <-task.stopping:
		default:
			stopping := task.stopping
			m.mu.Unlock()
			<-stopping
			return m.FinishTaskRun(conversationID, runID, finalStatus)
		}
	}
	if task.finishing != nil {
		done := task.finishing
		m.mu.Unlock()
		<-done
		m.mu.RLock()
		cleanupError := task.CleanupError
		m.mu.RUnlock()
		if cleanupError != "" {
			return errors.New(cleanupError)
		}
		return nil
	}
	if task.cancelRequested {
		finalStatus = "cancelled"
	}
	done := make(chan struct{})
	task.finishing = done
	task.finalStatus = finalStatus
	task.Status = "cleaning"
	task.processes.Seal()
	task.workers.Seal()
	toolCanceler := m.toolCanceler
	activeCancel := task.activeEinoExecuteCancel
	cancel := task.cancel
	bus := m.eventBus
	m.mu.Unlock()

	// Keep the conversation occupied throughout cleanup, including callbacks.
	if cancel != nil {
		cancel(nil)
	}
	if toolCanceler != nil {
		toolCanceler(conversationID)
	}
	if activeCancel != nil {
		activeCancel()
	}
	task.workers.Cancel()
	processErr := task.processes.Close()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 3*time.Second)
	workerErr := task.workers.Wait(waitCtx)
	waitCancel()
	cleanupErr := errors.Join(processErr, workerErr)
	if processErr != nil && errors.Is(workerErr, runlease.ErrUnconfirmed) {
		// A simultaneous local failure must not be labelled as local completion.
		cleanupErr = fmt.Errorf("local cleanup: %w; remote state: %v", processErr, workerErr)
	}
	// The local worker has returned, but remote notification cancellation is
	// not an acknowledgement. Preserve an actionable history/tool status.
	unconfirmed := processErr == nil && errors.Is(workerErr, runlease.ErrUnconfirmed)
	if unconfirmed {
		finalStatus = "cleanup_unconfirmed"
	}
	cleanupMessage := ""
	if cleanupErr != nil {
		cleanupMessage = cleanupErr.Error()
	}
	if (cleanupErr == nil || unconfirmed) && bus != nil {
		// Subscribers must receive completion only after local processes are reaped.
		payload, _ := json.Marshal(StreamEvent{Type: "done", Data: map[string]interface{}{"conversationId": conversationID, "runId": runID, "status": finalStatus, "cleanupError": cleanupMessage}})
		bus.Publish(conversationID, append(append([]byte("data: "), payload...), '\n', '\n'))
		bus.CloseConversation(conversationID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	defer close(done)
	if cleanupErr != nil && !unconfirmed {
		task.Status = "cleanup_failed"
		task.CleanupError = cleanupErr.Error()
		task.finishing = nil
		return cleanupErr
	}
	task.CleanupError = ""
	if unconfirmed {
		task.CleanupError = cleanupErr.Error()
	}
	task.Status = finalStatus
	m.completedTasks = append(m.completedTasks, &CompletedTask{
		RunID: task.RunID, CleanupError: task.CleanupError, IsolationBackend: task.processes.IsolationBackend(), ConversationID: task.ConversationID, Message: task.Message,
		StartedAt: task.StartedAt, CompletedAt: time.Now(), Status: finalStatus,
	})
	m.cleanupHistory()
	delete(m.tasks, conversationID)
	return cleanupErr
}

// Shutdown rejects new tasks before cancelling and reaping existing task jobs.
func (m *AgentTaskManager) Shutdown() {
	m.mu.Lock()
	m.shuttingDown = true
	m.shutdownOnce.Do(func() { close(m.shutdown) })
	tasks := make([]*AgentTask, 0, len(m.tasks))
	for _, task := range m.tasks {
		tasks = append(tasks, task)
		task.processes.Seal()
		task.workers.Seal()
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, task := range tasks {
		wg.Add(1)
		go func(task *AgentTask) {
			defer wg.Done()
			_, _ = m.CancelTask(task.ConversationID, ErrTaskCancelled)
			_ = m.FinishTaskRun(task.ConversationID, task.RunID, "cancelled")
		}(task)
	}
	wg.Wait()
}

// cleanupHistory cleans up expired history records
func (m *AgentTaskManager) cleanupHistory() {
	now := time.Now()
	cutoffTime := now.Add(-m.historyRetention)

	// filter out expired records
	validTasks := make([]*CompletedTask, 0, len(m.completedTasks))
	for _, task := range m.completedTasks {
		if task.CompletedAt.After(cutoffTime) {
			validTasks = append(validTasks, task)
		}
	}

	// if still exceeding the max count, keep only the newest
	if len(validTasks) > m.maxHistorySize {
		// sort by completion time, keep the newest
		// since records are appended, the newest is last; take the last N directly
		start := len(validTasks) - m.maxHistorySize
		validTasks = validTasks[start:]
	}

	m.completedTasks = validTasks
}

// GetActiveTasks returns all currently running tasks
func (m *AgentTaskManager) GetActiveTasks() []*AgentTask {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*AgentTask, 0, len(m.tasks))
	for _, task := range m.tasks {
		result = append(result, &AgentTask{
			RunID: task.RunID, CleanupError: task.CleanupError, IsolationBackend: task.processes.IsolationBackend(),
			ConversationID: task.ConversationID,
			Message:        task.Message,
			StartedAt:      task.StartedAt,
			Status:         task.Status,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].StartedAt.Equal(result[j].StartedAt) {
			return result[i].ConversationID < result[j].ConversationID
		}
		return result[i].StartedAt.Before(result[j].StartedAt)
	})
	return result
}

// GetCompletedTasks returns the recently completed task history
func (m *AgentTaskManager) GetCompletedTasks() []*CompletedTask {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// clean up expired records (read lock, does not affect other operations)
	// Note: cannot directly call cleanupHistory here because it requires a write lock
	// so filter expired records at return time
	now := time.Now()
	cutoffTime := now.Add(-m.historyRetention)

	result := make([]*CompletedTask, 0, len(m.completedTasks))
	for _, task := range m.completedTasks {
		if task.CompletedAt.After(cutoffTime) {
			result = append(result, task)
		}
	}

	// sort by completion time descending (newest first)
	// since records are appended, the newest is last; need to reverse
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	// limit the number of records returned
	if len(result) > m.maxHistorySize {
		result = result[:m.maxHistorySize]
	}

	return result
}
