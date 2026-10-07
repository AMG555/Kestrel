package c2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"kestrel/internal/database"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Manager is the unified facade for the C2 module:
//   - HTTP handler / MCP tool / multi-agent / attack chain recorder all operate C2 through Manager,
//     without touching listener implementation details directly, to avoid circular dependencies;
//   - holds the database handle + event bus + in-memory listener instance map;
//   - RestoreRunningListeners() can be called at startup to bring back listeners with status=running.
//
// Instantiation is handled by internal/app, which injects it into the global App and then passes it to handler/mcp.
type Manager struct {
	db       *database.DB
	logger   *zap.Logger
	bus      *EventBus
	registry *ListenerRegistry

	mu               sync.RWMutex
	runningListeners map[string]Listener // listener_id → started listener instances
	storageDir       string              // root directory for large results (screenshots/downloads) on disk

	hitlBridge        HITLBridge                                    // called in EnqueueTask to request approval for dangerous tasks (nil disables HITL)
	hitlDangerousGate func(conversationID, mcpToolName string) bool // consistent with human-in-the-loop: bridge is skipped when nil or returns false
	hooks             Hooks                                         // extension hooks: notify vulnerability library and attack chain on session online / task complete
}

// MCPToolC2Task matches the MCP builtin c2_task tool name for HITL whitelist and Agent-side alignment.
const MCPToolC2Task = "c2_task"

var (
	resultBlobSuffixPattern = regexp.MustCompile(`^\.[A-Za-z0-9][A-Za-z0-9_-]{0,31}$`)
	uploadTaskIDPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
)

// HITLBridge bridges "dangerous tasks" to the existing internal/handler/hitl approval flow.
// Passed in when internal/app instantiates; nil implementation disables HITL interception (convenient during development).
type HITLBridge interface {
	// RequestApproval blocks waiting for human approval; nil means approved, error means rejected/timed out.
	// ctx carries user/session info; a timeout ctx is created when calling dangerous tasks to avoid indefinite blocking.
	RequestApproval(ctx context.Context, req HITLApprovalRequest) error
}

// HITLApprovalRequest describes a C2 operation pending approval
type HITLApprovalRequest struct {
	TaskID         string
	SessionID      string
	TaskType       string
	PayloadJSON    string
	ConversationID string
	Source         string
	Reason         string
}

// Hooks provides callbacks for the upper layer (vulnerability management / attack chain)
type Hooks struct {
	OnSessionFirstSeen func(session *database.C2Session)             // new session first seen online
	OnTaskCompleted    func(task *database.C2Task, sessionID string) // Task completed (success/failed)
}

// NewManager creates a Manager; it will not start any listeners — call RestoreRunningListeners explicitly
func NewManager(db *database.DB, logger *zap.Logger, storageDir string) *Manager {
	if logger == nil {
		logger = zap.NewNop()
	}
	if storageDir == "" {
		storageDir = "tmp/c2"
	}
	return &Manager{
		db:               db,
		logger:           logger,
		bus:              NewEventBus(),
		registry:         NewListenerRegistry(),
		runningListeners: make(map[string]Listener),
		storageDir:       storageDir,
	}
}

// SetHITLBridge sets the dangerous-task approval bridge; nil disables it
func (m *Manager) SetHITLBridge(b HITLBridge) {
	m.mu.Lock()
	m.hitlBridge = b
	m.mu.Unlock()
}

// SetHITLDangerousGate sets whether C2 dangerous tasks should go through the HITL bridge; must be consistent with Agent human-in-the-loop logic (e.g. handler.HITLManager.NeedsToolApproval).
// When gate is nil, no approval is initiated for dangerous tasks even if the bridge is set (consistent with tool behaviour when human-in-the-loop is not enabled).
func (m *Manager) SetHITLDangerousGate(gate func(conversationID, mcpToolName string) bool) {
	m.mu.Lock()
	m.hitlDangerousGate = gate
	m.mu.Unlock()
}

// SetHooks injects business hooks
func (m *Manager) SetHooks(h Hooks) {
	m.mu.Lock()
	m.hooks = h
	m.mu.Unlock()
}

// EventBus exposes the event bus for the SSE handler
func (m *Manager) EventBus() *EventBus { return m.bus }

// DB exposes the DB handle for handler/mcptools to read/write directly (to avoid wrapping everywhere)
func (m *Manager) DB() *database.DB { return m.db }

// Logger exposes the logger handle
func (m *Manager) Logger() *zap.Logger { return m.logger }

// StorageDir is the root directory for large results on disk
func (m *Manager) StorageDir() string { return m.storageDir }

// Registry exposes the listener registry, making it easy to register concrete implementations by type at internal/app startup
func (m *Manager) Registry() *ListenerRegistry { return m.registry }

// Close gracefully shuts down: stops all running listeners and closes the event bus
func (m *Manager) Close() {
	m.mu.Lock()
	listeners := make([]Listener, 0, len(m.runningListeners))
	for _, l := range m.runningListeners {
		listeners = append(listeners, l)
	}
	m.runningListeners = make(map[string]Listener)
	m.mu.Unlock()
	for _, l := range listeners {
		_ = l.Stop()
	}
	m.bus.Close()
}

// ----------------------------------------------------------------------------
// Listener lifecycle
// ----------------------------------------------------------------------------

// CreateListenerInput is the input for creating a listener via Web/MCP (already validated and trimmed)
type CreateListenerInput struct {
	Name      string
	ProjectID string
	Type      string
	BindHost  string
	BindPort  int
	ProfileID string
	Remark    string
	Config    *ListenerConfig
	// CallbackHost: if non-empty, written to config_json.callback_host for default payload callback (does not modify bind)
	CallbackHost string
}

// CreateListener validates and persists to DB; does not auto-start (consistent with systemd unit: create then start)
func (m *Manager) CreateListener(in CreateListenerInput) (*database.C2Listener, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, ErrInvalidInput
	}
	if !IsValidListenerType(in.Type) {
		return nil, ErrUnsupportedType
	}
	if err := SafeBindPort(in.BindPort); err != nil {
		return nil, &CommonError{Code: "invalid_port", Message: err.Error(), HTTP: 400}
	}
	bindHost := strings.TrimSpace(in.BindHost)
	if bindHost == "" {
		bindHost = "127.0.0.1" // default bind to loopback; operator must explicitly change for external access
	}
	cfg := in.Config
	if cfg == nil {
		cfg = &ListenerConfig{}
	} else {
		cp := *cfg
		cfg = &cp
	}
	if ch := strings.TrimSpace(in.CallbackHost); ch != "" {
		cfg.CallbackHost = ch
	}
	cfg.ApplyDefaults()
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal listener config: %w", err)
	}
	keyB64, err := GenerateAESKey()
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	tokenB64, err := GenerateImplantToken()
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	listener := &database.C2Listener{
		ID:            "l_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:14],
		ProjectID:     strings.TrimSpace(in.ProjectID),
		Name:          strings.TrimSpace(in.Name),
		Type:          strings.ToLower(strings.TrimSpace(in.Type)),
		BindHost:      bindHost,
		BindPort:      in.BindPort,
		ProfileID:     strings.TrimSpace(in.ProfileID),
		EncryptionKey: keyB64,
		ImplantToken:  tokenB64,
		Status:        "stopped",
		ConfigJSON:    string(cfgJSON),
		Remark:        strings.TrimSpace(in.Remark),
		CreatedAt:     time.Now(),
	}
	if err := m.db.CreateC2Listener(listener); err != nil {
		return nil, err
	}
	m.publishEvent("info", "listener", "", "", fmt.Sprintf("Listener %s created", listener.Name), map[string]interface{}{
		"listener_id": listener.ID,
		"type":        listener.Type,
	})
	return listener, nil
}

// StartListener starts the specified listener; idempotent (returns ErrListenerRunning if already running)
func (m *Manager) StartListener(id string) (*database.C2Listener, error) {
	rec, err := m.db.GetC2Listener(id)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, ErrListenerNotFound
	}
	m.mu.Lock()
	if _, ok := m.runningListeners[id]; ok {
		m.mu.Unlock()
		return rec, ErrListenerRunning
	}
	m.mu.Unlock()

	cfg := &ListenerConfig{}
	if rec.ConfigJSON != "" {
		_ = json.Unmarshal([]byte(rec.ConfigJSON), cfg)
	}
	cfg.ApplyDefaults()

	// Create concrete implementation via factory. Must use a copy of rec: HTTP handler clears
	// rec.ImplantToken / EncryptionKey for desensitization; if the listener implementation holds the same pointer, beacon auth will permanently fail.
	listenerRec := *rec
	factory := m.registry.Get(rec.Type)
	if factory == nil {
		return nil, ErrUnsupportedType
	}
	inst, err := factory(ListenerCreationCtx{
		Listener: &listenerRec,
		Config:   cfg,
		Manager:  m,
		Logger:   m.logger.With(zap.String("listener_id", rec.ID), zap.String("type", rec.Type)),
	})
	if err != nil {
		return nil, err
	}
	if err := inst.Start(); err != nil {
		now := time.Now()
		_ = m.db.SetC2ListenerStatus(rec.ID, "error", err.Error(), &now)
		m.publishEvent("warn", "listener", "", "", fmt.Sprintf("Listener %s startup failed: %v", rec.Name, err), map[string]interface{}{
			"listener_id": rec.ID,
		})
		return nil, err
	}
	m.mu.Lock()
	m.runningListeners[rec.ID] = inst
	m.mu.Unlock()
	now := time.Now()
	_ = m.db.SetC2ListenerStatus(rec.ID, "running", "", &now)
	rec.Status = "running"
	rec.StartedAt = &now
	rec.LastError = ""
	m.publishEvent("info", "listener", "", "", fmt.Sprintf("Listener %s started", rec.Name), map[string]interface{}{
		"listener_id": rec.ID,
		"bind":        fmt.Sprintf("%s:%d", rec.BindHost, rec.BindPort),
	})
	return rec, nil
}

// StopListener stops the listener; idempotent (returns ErrListenerStopped if already stopped)
func (m *Manager) StopListener(id string) error {
	m.mu.Lock()
	inst, ok := m.runningListeners[id]
	if ok {
		delete(m.runningListeners, id)
	}
	m.mu.Unlock()
	if !ok {
		return ErrListenerStopped
	}
	if err := inst.Stop(); err != nil {
		return err
	}
	_ = m.db.SetC2ListenerStatus(id, "stopped", "", nil)
	rec, _ := m.db.GetC2Listener(id)
	name := id
	if rec != nil {
		name = rec.Name
	}
	m.publishEvent("info", "listener", "", "", fmt.Sprintf("listener %s stopped", name), map[string]interface{}{
		"listener_id": id,
	})
	return nil
}

// DeleteListener stops and deletes (cascades to sessions/tasks/files)
func (m *Manager) DeleteListener(id string) error {
	_ = m.StopListener(id)
	return m.db.DeleteC2Listener(id)
}

// IsListenerRunning returns the in-memory running status (DB status may be stale after a crash)
func (m *Manager) IsListenerRunning(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.runningListeners[id]
	return ok
}

// RestoreRunningListeners re-starts listeners with status=running in DB at startup;
// failed ones are changed to status=error and do not block the entire app startup.
func (m *Manager) RestoreRunningListeners() {
	listeners, err := m.db.ListC2Listeners()
	if err != nil {
		m.logger.Warn("resume C2 listener failed: list query error", zap.Error(err))
		return
	}
	for _, l := range listeners {
		if l.Status != "running" {
			continue
		}
		if _, err := m.StartListener(l.ID); err != nil && !errors.Is(err, ErrListenerRunning) {
			m.logger.Warn("resume C2 listener failed", zap.String("listener_id", l.ID), zap.Error(err))
		}
	}
}

// ----------------------------------------------------------------------------
// Session lifecycle
// ----------------------------------------------------------------------------

// IngestCheckIn is the unified entry for beacon online/heartbeat.
// Behavior:
//  1. if implant_uuid already has a session → update heartbeat/status
//  2. otherwise create a new session, trigger the OnSessionFirstSeen hook
func (m *Manager) IngestCheckIn(listenerID string, req ImplantCheckInRequest) (*database.C2Session, error) {
	if strings.TrimSpace(req.ImplantUUID) == "" {
		return nil, ErrInvalidInput
	}
	existing, err := m.db.GetC2SessionByImplantUUID(req.ImplantUUID)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.ListenerID != listenerID {
		return nil, ErrAuthFailed
	}
	now := time.Now()
	isFirstSeen := existing == nil
	var sessID string
	if existing != nil {
		sessID = existing.ID
	} else {
		sessID = "s_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:14]
	}
	session := &database.C2Session{
		ID:            sessID,
		ListenerID:    listenerID,
		ImplantUUID:   req.ImplantUUID,
		Hostname:      req.Hostname,
		Username:      req.Username,
		OS:            strings.ToLower(req.OS),
		Arch:          strings.ToLower(req.Arch),
		PID:           req.PID,
		ProcessName:   req.ProcessName,
		IsAdmin:       req.IsAdmin,
		InternalIP:    req.InternalIP,
		UserAgent:     req.UserAgent,
		SleepSeconds:  req.SleepSeconds,
		JitterPercent: req.JitterPercent,
		Status:        string(SessionActive),
		FirstSeenAt:   now,
		LastCheckIn:   now,
		Metadata:      req.Metadata,
	}
	if existing != nil {
		// Preserve original ID/FirstSeenAt/Note and operator-set sleep/jitter to avoid being overwritten by beacon heartbeat reports
		session.FirstSeenAt = existing.FirstSeenAt
		session.SleepSeconds = existing.SleepSeconds
		session.JitterPercent = existing.JitterPercent
		if session.Note == "" {
			session.Note = existing.Note
		}
	}
	if err := m.db.UpsertC2Session(session); err != nil {
		return nil, err
	}
	if isFirstSeen {
		m.publishEvent("critical", "session", session.ID, "",
			fmt.Sprintf("New session online: %s@%s (%s/%s)", session.Username, session.Hostname, session.OS, session.Arch),
			map[string]interface{}{
				"session_id":  session.ID,
				"listener_id": listenerID,
				"hostname":    session.Hostname,
				"os":          session.OS,
				"arch":        session.Arch,
				"internal_ip": session.InternalIP,
			})
		m.mu.RLock()
		hook := m.hooks.OnSessionFirstSeen
		m.mu.RUnlock()
		if hook != nil {
			go hook(session)
		}
	}
	// Regular heartbeat: last_check_in is already written to c2_sessions by UpsertC2Session; no longer recorded in c2_events.
	// Otherwise each heartbeat would generate one audit entry per sleep cycle, quickly bloating the DB and SSE; online/offline events still publishEvent normally.
	return session, nil
}

// SetSessionSleep updates the expected heartbeat interval for the session and dispatches a sleep task to the implant to take effect as soon as possible.
func (m *Manager) SetSessionSleep(sessionID string, sleepSeconds, jitterPercent int) (*database.C2Task, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, ErrInvalidInput
	}
	if sleepSeconds < 1 {
		sleepSeconds = 1
	}
	if jitterPercent < 0 {
		jitterPercent = 0
	}
	if jitterPercent > 100 {
		jitterPercent = 100
	}
	if err := m.db.SetC2SessionSleep(sessionID, sleepSeconds, jitterPercent); err != nil {
		return nil, err
	}
	task, err := m.EnqueueTask(EnqueueTaskInput{
		SessionID: sessionID,
		TaskType:  TaskTypeSleep,
		Payload: map[string]interface{}{
			"seconds": sleepSeconds,
			"jitter":  jitterPercent,
		},
		Source: "manual",
	})
	if err != nil {
		m.logger.Warn("sleep task enqueue failed", zap.Error(err), zap.String("session_id", sessionID))
	}
	m.publishEvent("info", "session", sessionID, "",
		fmt.Sprintf("Sleep updated: %ds (jitter %d%%)", sleepSeconds, jitterPercent),
		map[string]interface{}{
			"sleep_seconds":  sleepSeconds,
			"jitter_percent": jitterPercent,
		})
	return task, nil
}

// MarkSessionDead is called by the heartbeat timeout detector: marks session as dead
func (m *Manager) MarkSessionDead(sessionID string) error {
	if err := m.db.SetC2SessionStatus(sessionID, string(SessionDead)); err != nil {
		return err
	}
	m.publishEvent("warn", "session", sessionID, "", "Session offline (heartbeat timeout)", nil)
	return nil
}

// ----------------------------------------------------------------------------
// Task lifecycle
// ----------------------------------------------------------------------------

// EnqueueTaskInput is the input for dispatching a task
type EnqueueTaskInput struct {
	SessionID      string
	TaskType       TaskType
	Payload        map[string]interface{}
	Source         string // manual|ai|batch|api
	ConversationID string
	UserCtx        context.Context // for HITL use
	BypassHITL     bool            // true means skip HITL approval (for whitelist mechanism / internal system use only)
}

// EnqueueTask enqueues a new task; if the task type is dangerous and BypassHITL is false, and SetHITLDangerousGate returns true for the current session and MCPToolC2Task, it will invoke HITL bridge approval.
// Returns the task record; task dispatch is completed by PopTasksForBeacon when the beacon pulls tasks.
func (m *Manager) EnqueueTask(in EnqueueTaskInput) (*database.C2Task, error) {
	if strings.TrimSpace(in.SessionID) == "" {
		return nil, ErrInvalidInput
	}
	session, err := m.db.GetC2Session(in.SessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, ErrSessionNotFound
	}
	if session.Status == string(SessionDead) || session.Status == string(SessionKilled) {
		return nil, &CommonError{Code: "session_inactive", Message: "session is offline, cannot dispatch task", HTTP: 409}
	}

	// OPSEC: command deny regex enforcement
	if in.TaskType == TaskTypeExec || in.TaskType == TaskTypeShell {
		cmd, _ := in.Payload["command"].(string)
		if cmd != "" {
			listenerCfg := m.getListenerConfig(session.ListenerID)
			if listenerCfg != nil {
				for _, pattern := range listenerCfg.CommandDenyRegex {
					re, err := regexp.Compile(pattern)
					if err != nil {
						m.logger.Warn("invalid command_deny_regex", zap.String("pattern", pattern), zap.Error(err))
						continue
					}
					if re.MatchString(cmd) {
						return nil, &CommonError{
							Code:    "command_denied",
							Message: fmt.Sprintf("command rejected by OPSEC rule (matched: %s)", pattern),
							HTTP:    403,
						}
					}
				}
			}
		}
	}

	// OPSEC: max_concurrent_tasks enforcement
	listenerCfg := m.getListenerConfig(session.ListenerID)
	if listenerCfg != nil && listenerCfg.MaxConcurrentTasks > 0 {
		activeTasks, _ := m.db.ListC2Tasks(database.ListC2TasksFilter{
			SessionID: in.SessionID,
			Status:    string(TaskQueued),
		})
		sentTasks, _ := m.db.ListC2Tasks(database.ListC2TasksFilter{
			SessionID: in.SessionID,
			Status:    string(TaskSent),
		})
		concurrent := len(activeTasks) + len(sentTasks)
		if concurrent >= listenerCfg.MaxConcurrentTasks {
			return nil, &CommonError{
				Code:    "concurrent_limit",
				Message: fmt.Sprintf("session already has %d queued/running tasks, exceeding concurrency limit %d", concurrent, listenerCfg.MaxConcurrentTasks),
				HTTP:    429,
			}
		}
	}

	taskID := "t_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:14]
	task := &database.C2Task{
		ID:             taskID,
		SessionID:      in.SessionID,
		TaskType:       string(in.TaskType),
		Payload:        in.Payload,
		Status:         string(TaskQueued),
		Source:         strOr(in.Source, "manual"),
		ConversationID: in.ConversationID,
		CreatedAt:      time.Now(),
	}

	// HITL check: only invoke bridge when the injected gate determines this session should use human-in-the-loop for the unified MCP tool c2_task (when HITL is disabled, behaves like other tools and enqueues directly).
	if IsDangerousTaskType(in.TaskType) && !in.BypassHITL {
		m.mu.RLock()
		bridge := m.hitlBridge
		gate := m.hitlDangerousGate
		m.mu.RUnlock()
		convID := strings.TrimSpace(in.ConversationID)
		useBridge := bridge != nil && gate != nil && gate(convID, MCPToolC2Task)
		if useBridge {
			task.ApprovalStatus = "pending"
			if err := m.db.CreateC2Task(task); err != nil {
				return nil, err
			}
			m.publishEvent("warn", "task", in.SessionID, taskID, fmt.Sprintf("Dangerous task pending approval: %s", in.TaskType), map[string]interface{}{
				"task_id":   taskID,
				"task_type": in.TaskType,
			})
			payloadBytes, _ := json.Marshal(in.Payload)
			ctx := HITLUserContext(in.UserCtx)
			if ctx == nil {
				ctx = context.Background()
			}
			go func() {
				err := bridge.RequestApproval(ctx, HITLApprovalRequest{
					TaskID:         taskID,
					SessionID:      in.SessionID,
					TaskType:       string(in.TaskType),
					PayloadJSON:    string(payloadBytes),
					ConversationID: in.ConversationID,
					Source:         task.Source,
					Reason:         fmt.Sprintf("C2 dangerous task %s", in.TaskType),
				})
				if err != nil {
					rejected := "rejected"
					failed := string(TaskFailed)
					errMsg := "HITL rejected: " + err.Error()
					_ = m.db.UpdateC2Task(taskID, database.C2TaskUpdate{
						ApprovalStatus: &rejected,
						Status:         &failed,
						Error:          &errMsg,
					})
					m.publishEvent("warn", "task", in.SessionID, taskID, errMsg, nil)
					return
				}
				approved := "approved"
				_ = m.db.UpdateC2Task(taskID, database.C2TaskUpdate{ApprovalStatus: &approved})
				m.publishEvent("info", "task", in.SessionID, taskID, "Dangerous task approved", nil)
			}()
			return task, nil
		}
		// bridge not connected, session has HITL disabled, or tool is whitelisted: enqueue directly
		task.ApprovalStatus = "approved"
	}

	if err := m.db.CreateC2Task(task); err != nil {
		return nil, err
	}
	m.publishEvent("info", "task", in.SessionID, taskID, fmt.Sprintf("Task enqueued: %s", in.TaskType), map[string]interface{}{
		"task_id":   taskID,
		"task_type": in.TaskType,
		"source":    task.Source,
	})
	return task, nil
}

// CancelTask cancels a queued task (rollback of already sent/running tasks is not yet supported).
func (m *Manager) CancelTask(taskID string) error {
	t, err := m.db.GetC2Task(taskID)
	if err != nil {
		return err
	}
	if t == nil {
		return ErrTaskNotFound
	}
	if t.Status != string(TaskQueued) && t.Status != string(TaskSent) {
		return &CommonError{Code: "task_running", Message: "task is already running, cannot cancel", HTTP: 409}
	}
	cancelled := string(TaskCancelled)
	now := time.Now()
	if err := m.db.UpdateC2Task(taskID, database.C2TaskUpdate{ExpectedStatus: &t.Status, Status: &cancelled, CompletedAt: &now}); err != nil {
		return err
	}
	m.publishEvent("info", "task", t.SessionID, taskID, "task cancelled", nil)
	return nil
}

// PopTasksForBeacon is called after a beacon check-in: retrieves all queued+approved tasks for that session,
// internally marks them as sent, and returns TaskEnvelopes ready for the listener to encode and dispatch.
func (m *Manager) PopTasksForBeacon(sessionID string, limit int) ([]TaskEnvelope, error) {
	session, err := m.db.GetC2Session(sessionID)
	if err != nil {
		return nil, err
	}
	if session != nil {
		listener, err := m.db.GetC2Listener(session.ListenerID)
		if err != nil {
			return nil, err
		}
		if listener != nil && listener.Type == string(ListenerTypeTCPReverse) {
			m.mu.Lock()
			defer m.mu.Unlock()
			for _, status := range []string{string(TaskSent), string(TaskRunning)} {
				count, err := m.db.CountC2Tasks(database.ListC2TasksFilter{SessionID: sessionID, Status: status})
				if err != nil {
					return nil, err
				}
				if count > 0 {
					return []TaskEnvelope{}, nil
				}
			}
			limit = 1
		}
	}
	tasks, err := m.db.PopQueuedC2Tasks(sessionID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]TaskEnvelope, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, TaskEnvelope{TaskID: t.ID, TaskType: t.TaskType, Payload: t.Payload})
	}
	return out, nil
}

// IngestTaskResult is the unified entry point for beacons to return task results.
// IngestTaskResultFromListener verifies transport-established ownership before
// accepting a result. HTTP transports authenticate the listener; persistent
// connections additionally bind the result to the session established at check-in.
func (m *Manager) IngestTaskResultFromListener(listenerID, sessionID string, report TaskResultReport) error {
	if listenerID == "" {
		return ErrAuthFailed
	}
	task, err := m.db.GetC2Task(report.TaskID)
	if err != nil {
		return err
	}
	if task == nil {
		return ErrTaskNotFound
	}
	session, err := m.db.GetC2Session(task.SessionID)
	if err != nil {
		return err
	}
	if session == nil || session.ListenerID != listenerID || (sessionID != "" && session.ID != sessionID) {
		return ErrAuthFailed
	}
	return m.IngestTaskResult(report)
}

func (m *Manager) IngestTaskResult(report TaskResultReport) error {
	if strings.TrimSpace(report.TaskID) == "" {
		return ErrInvalidInput
	}
	t, err := m.db.GetC2Task(report.TaskID)
	if err != nil {
		return err
	}
	if t == nil {
		return ErrTaskNotFound
	}
	if t.Status == string(TaskCancelled) || t.Status == string(TaskSuccess) || t.Status == string(TaskFailed) {
		return nil
	}

	startedAt := time.Unix(0, report.StartedAt*int64(time.Millisecond))
	endedAt := time.Unix(0, report.EndedAt*int64(time.Millisecond))
	if report.StartedAt == 0 {
		startedAt = time.Now()
	}
	if report.EndedAt == 0 {
		endedAt = time.Now()
	}

	status := string(TaskSuccess)
	if !report.Success {
		status = string(TaskFailed)
	}
	duration := endedAt.Sub(startedAt).Milliseconds()

	sessionOS := ""
	if sess, serr := m.db.GetC2Session(t.SessionID); serr == nil && sess != nil {
		sessionOS = sess.OS
	}
	resultText := ResolveTaskResultText(report.Output, report.OutputB64, sessionOS)
	errText := ResolveTaskResultText(report.Error, report.ErrorB64, sessionOS)

	upd := database.C2TaskUpdate{
		ExpectedStatus: &t.Status,
		Status:         &status,
		ResultText:     &resultText,
		Error:          &errText,
		StartedAt:      &startedAt,
		CompletedAt:    &endedAt,
		DurationMS:     &duration,
	}

	// Persist blob to disk (e.g. screenshots)
	if len(report.BlobBase64) > 0 {
		blobPath, err := m.saveResultBlob(t.ID, report.BlobBase64, report.BlobSuffix)
		if err == nil {
			upd.ResultBlobPath = &blobPath
		} else {
			m.logger.Warn("failed to persist result blob to disk", zap.Error(err), zap.String("task_id", t.ID))
		}
	}

	if err := m.db.UpdateC2Task(t.ID, upd); err != nil {
		return err
	}
	t.Status = status
	t.ResultText = resultText
	t.Error = errText

	level := "info"
	msg := fmt.Sprintf("task completed: %s", t.TaskType)
	if !report.Success {
		level = "warn"
		msg = fmt.Sprintf("taskfailed: %s (%s)", t.TaskType, report.Error)
	}
	m.publishEvent(level, "task", t.SessionID, t.ID, msg, map[string]interface{}{
		"task_id":   t.ID,
		"task_type": t.TaskType,
		"duration":  duration,
	})

	m.mu.RLock()
	hook := m.hooks.OnTaskCompleted
	m.mu.RUnlock()
	if hook != nil {
		go hook(t, t.SessionID)
	}
	return nil
}

func (m *Manager) saveResultBlob(taskID, b64Content, suffix string) (string, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || taskID == "." || taskID == ".." ||
		strings.ContainsAny(taskID, `/\`) {
		return "", fmt.Errorf("invalid task_id")
	}

	suffix, err := normalizeResultBlobSuffix(suffix)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(m.storageDir, "results")
	if err := osMkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, taskID+suffix)
	if err := ensurePathInDir(dir, path); err != nil {
		return "", err
	}
	data, err := base64Decode(b64Content)
	if err != nil {
		return "", err
	}
	if err := osWriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func uploadPathForTask(storageDir, taskID string) (dir, path string, err error) {
	taskID = strings.TrimSpace(taskID)
	if !uploadTaskIDPattern.MatchString(taskID) {
		return "", "", fmt.Errorf("invalid task_id")
	}
	dir = filepath.Join(storageDir, "uploads")
	path = filepath.Join(dir, taskID+".bin")
	if err := ensurePathInDir(dir, path); err != nil {
		return "", "", err
	}
	return dir, path, nil
}

func normalizeResultBlobSuffix(suffix string) (string, error) {
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return ".bin", nil
	}
	if !strings.HasPrefix(suffix, ".") {
		suffix = "." + suffix
	}
	if !resultBlobSuffixPattern.MatchString(suffix) {
		return "", fmt.Errorf("invalid blob suffix")
	}
	return suffix, nil
}

func ensurePathInDir(dir, path string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("path escapes result directory")
	}
	return nil
}

// ----------------------------------------------------------------------------
// event bus helpers
// ----------------------------------------------------------------------------

// publishEvent synchronously writes to the c2_events table and publishes to the in-memory event bus.
func (m *Manager) publishEvent(level, category, sessionID, taskID, message string, data map[string]interface{}) {
	id := "e_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:14]
	now := time.Now()
	e := &database.C2Event{
		ID:        id,
		Level:     level,
		Category:  category,
		SessionID: sessionID,
		TaskID:    taskID,
		Message:   message,
		Data:      data,
		CreatedAt: now,
	}
	if err := m.db.AppendC2Event(e); err != nil {
		m.logger.Warn("failed to write C2 event", zap.Error(err), zap.String("category", category))
	}
	m.bus.Publish(&Event{
		ID:        id,
		Level:     level,
		Category:  category,
		SessionID: sessionID,
		TaskID:    taskID,
		Message:   message,
		Data:      data,
		CreatedAt: now,
	})
}

// PublishCustomEvent allows external components (HITL bridge / handler) to write custom events.
func (m *Manager) PublishCustomEvent(level, category, sessionID, taskID, message string, data map[string]interface{}) {
	m.publishEvent(level, category, sessionID, taskID, message, data)
}

// ----------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------

func strOr(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// getListenerConfig loads and parses the listener's config JSON from DB.
func (m *Manager) getListenerConfig(listenerID string) *ListenerConfig {
	listener, err := m.db.GetC2Listener(listenerID)
	if err != nil || listener == nil {
		return nil
	}
	cfg := &ListenerConfig{}
	if listener.ConfigJSON != "" && listener.ConfigJSON != "{}" {
		_ = json.Unmarshal([]byte(listener.ConfigJSON), cfg)
	}
	return cfg
}

// GetProfile loads a C2Profile from DB by ID.
func (m *Manager) GetProfile(profileID string) (*database.C2Profile, error) {
	if strings.TrimSpace(profileID) == "" {
		return nil, nil
	}
	return m.db.GetC2Profile(profileID)
}
