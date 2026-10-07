package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"kestrel/internal/config"
	"kestrel/internal/database"

	"go.uber.org/zap"
)

var (
	// ErrBatchQueueNotFound means the queue does not exist or has been unloaded from memory.
	ErrBatchQueueNotFound = errors.New("batch queue not found")
	// ErrBatchQueueExecutorActive means the executeBatchQueue goroutine is still finishing; deletion is not allowed.
	ErrBatchQueueExecutorActive = errors.New("batch queue executor is still active")
	// ErrBatchQueueStillRunning means the queue status is still running (fallback protection when no active executor).
	ErrBatchQueueStillRunning = errors.New("batch queue is still running")
)

// Batch task status constants
const (
	BatchQueueStatusPending   = "pending"
	BatchQueueStatusRunning   = "running"
	BatchQueueStatusPaused    = "paused"
	BatchQueueStatusCompleted = "completed"
	BatchQueueStatusCancelled = "cancelled"

	BatchTaskStatusPending   = "pending"
	BatchTaskStatusRunning   = "running"
	BatchTaskStatusCompleted = "completed"
	BatchTaskStatusFailed    = "failed"
	BatchTaskStatusCancelled = "cancelled"

	// MaxBatchTasksPerQueue is the maximum number of tasks per queue
	MaxBatchTasksPerQueue = 10000

	// MaxBatchQueueTitleLen queuetitlemaximum length
	MaxBatchQueueTitleLen = 200

	// MaxBatchQueueRoleLen is the maximum length of a role name
	MaxBatchQueueRoleLen = 100

	// DefaultBatchQueueConcurrency is the default batch queue concurrency (serial)
	DefaultBatchQueueConcurrency = 1

	// MaxBatchQueueConcurrency is the maximum batch queue concurrency
	MaxBatchQueueConcurrency = 8
)

// BatchTask is a batch task item
type BatchTask struct {
	ID             string     `json:"id"`
	Message        string     `json:"message"`
	ConversationID string     `json:"conversationId,omitempty"`
	Status         string     `json:"status"` // pending, running, completed, failed, cancelled
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
	Error          string     `json:"error,omitempty"`
	Result         string     `json:"result,omitempty"`
}

// BatchTaskQueue is a batch task queue
type BatchTaskQueue struct {
	ID                    string       `json:"id"`
	Title                 string       `json:"title,omitempty"`
	Role                  string       `json:"role,omitempty"` // role name (empty string means default role)
	HITLPolicy            string       `json:"hitlPolicy"`
	AgentMode             string       `json:"agentMode"`    // single | eino_single | deep | plan_execute | supervisor
	ScheduleMode          string       `json:"scheduleMode"` // manual | cron
	CronExpr              string       `json:"cronExpr,omitempty"`
	NextRunAt             *time.Time   `json:"nextRunAt,omitempty"`
	ScheduleEnabled       bool         `json:"scheduleEnabled"`
	LastScheduleTriggerAt *time.Time   `json:"lastScheduleTriggerAt,omitempty"`
	LastScheduleError     string       `json:"lastScheduleError,omitempty"`
	LastRunError          string       `json:"lastRunError,omitempty"`
	ProjectID             string       `json:"projectId,omitempty"`
	Concurrency           int          `json:"concurrency"` // number of sub-tasks executed concurrently; default 1
	Tasks                 []*BatchTask `json:"tasks"`
	Status                string       `json:"status"` // pending, running, paused, completed, cancelled
	CreatedAt             time.Time    `json:"createdAt"`
	StartedAt             *time.Time   `json:"startedAt,omitempty"`
	CompletedAt           *time.Time   `json:"completedAt,omitempty"`
	CurrentIndex          int          `json:"currentIndex"`
}

// BatchTaskManager manages batch tasks
type BatchTaskManager struct {
	db             *database.DB
	logger         *zap.Logger
	queues         map[string]*BatchTaskQueue
	taskCancels    map[string]map[string]context.CancelFunc // queueID -> taskID -> cancel function
	singleRunTasks map[string]string                        // queueID -> taskID; pause queue after single task completes
	queueExecutors map[string]struct{}                      // executeBatchQueue goroutine active marker (decoupled from queue status)
	mu             sync.RWMutex
}

// NewBatchTaskManager creates a batch task manager
func NewBatchTaskManager(logger *zap.Logger) *BatchTaskManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &BatchTaskManager{
		logger:         logger,
		queues:         make(map[string]*BatchTaskQueue),
		taskCancels:    make(map[string]map[string]context.CancelFunc),
		singleRunTasks: make(map[string]string),
		queueExecutors: make(map[string]struct{}),
	}
}

// batchQueueExecutionShouldStop determines whether the executeBatchQueue main loop should exit.
func batchQueueExecutionShouldStop(queue *BatchTaskQueue, exists bool) bool {
	if !exists || queue == nil {
		return true
	}
	switch queue.Status {
	case BatchQueueStatusCancelled, BatchQueueStatusCompleted, BatchQueueStatusPaused:
		return true
	default:
		return false
	}
}

// TryMarkQueueExecutor marks the queue executor goroutine as started; returns false if one is already running.
func (m *BatchTaskManager) TryMarkQueueExecutor(queueID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.queueExecutors[queueID]; exists {
		return false
	}
	m.queueExecutors[queueID] = struct{}{}
	return true
}

// UnmarkQueueExecutor clears the queue executor goroutine marker (called by executeBatchQueue defer).
func (m *BatchTaskManager) UnmarkQueueExecutor(queueID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.queueExecutors, queueID)
}

// ForceUnmarkQueueExecutor forcibly clears the executor goroutine marker (used to reclaim stale slots, e.g. single-task rerun in paused state).
func (m *BatchTaskManager) ForceUnmarkQueueExecutor(queueID string) {
	m.UnmarkQueueExecutor(queueID)
}

// IsQueueExecutorActive reports whether the executeBatchQueue goroutine is still running.
func (m *BatchTaskManager) IsQueueExecutorActive(queueID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.queueExecutors[queueID]
	return ok
}

// SetDB sets the database connection
func (m *BatchTaskManager) SetDB(db *database.DB) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.db = db
}

// normalizeBatchQueueConcurrency normalises queue concurrency.
func normalizeBatchQueueConcurrency(n int) int {
	if n < 1 {
		return DefaultBatchQueueConcurrency
	}
	if n > MaxBatchQueueConcurrency {
		return MaxBatchQueueConcurrency
	}
	return n
}

// CreateBatchQueue creates a batch task queue
func (m *BatchTaskManager) CreateBatchQueue(
	title, role, agentMode, scheduleMode, cronExpr, projectID string,
	nextRunAt *time.Time,
	concurrency int,
	tasks []string,
	hitlPolicies ...string,
) (*BatchTaskQueue, error) {
	policy := ""
	if len(hitlPolicies) > 0 {
		policy = hitlPolicies[0]
	}
	if err := validateBatchHITLPolicy(policy); err != nil {
		return nil, err
	}
	// input validation
	if utf8.RuneCountInString(title) > MaxBatchQueueTitleLen {
		return nil, fmt.Errorf("title cannot exceed %d characters", MaxBatchQueueTitleLen)
	}
	if utf8.RuneCountInString(role) > MaxBatchQueueRoleLen {
		return nil, fmt.Errorf("role name cannot exceed %d characters", MaxBatchQueueRoleLen)
	}
	if len(tasks) > MaxBatchTasksPerQueue {
		return nil, fmt.Errorf("a single queue supports at most %d tasks", MaxBatchTasksPerQueue)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	queueID := time.Now().Format("20060102150405") + "-" + generateShortID()
	queue := &BatchTaskQueue{
		ID:              queueID,
		HITLPolicy:      policy,
		Title:           title,
		Role:            role,
		ProjectID:       strings.TrimSpace(projectID),
		AgentMode:       config.NormalizeAgentMode(agentMode),
		ScheduleMode:    normalizeBatchQueueScheduleMode(scheduleMode),
		CronExpr:        strings.TrimSpace(cronExpr),
		NextRunAt:       nextRunAt,
		ScheduleEnabled: true,
		Concurrency:     normalizeBatchQueueConcurrency(concurrency),
		Tasks:           make([]*BatchTask, 0, len(tasks)),
		Status:          BatchQueueStatusPending,
		CreatedAt:       time.Now(),
		CurrentIndex:    0,
	}
	if queue.ScheduleMode != "cron" {
		queue.CronExpr = ""
		queue.NextRunAt = nil
	}

	// prepare task data for database save
	dbTasks := make([]map[string]interface{}, 0, len(tasks))

	for _, message := range tasks {
		if message == "" {
			continue // skip empty lines
		}
		taskID := generateShortID()
		task := &BatchTask{
			ID:      taskID,
			Message: message,
			Status:  BatchTaskStatusPending,
		}
		queue.Tasks = append(queue.Tasks, task)
		dbTasks = append(dbTasks, map[string]interface{}{
			"id":      taskID,
			"message": message,
		})
	}

	// save to database
	if m.db != nil {
		if err := m.db.CreateBatchQueue(
			queueID,
			title,
			role,
			queue.AgentMode,
			queue.ScheduleMode,
			queue.CronExpr,
			queue.NextRunAt,
			queue.ProjectID,
			queue.Concurrency,
			dbTasks,
			policy,
		); err != nil {
			return nil, fmt.Errorf("savetaskqueuefailed: %w", err)
		}
	}

	m.queues[queueID] = queue
	return queue, nil
}

// GetBatchQueue gets a batch task queue
func (m *BatchTaskManager) GetBatchQueue(queueID string) (*BatchTaskQueue, bool) {
	m.mu.RLock()
	queue, exists := m.queues[queueID]
	m.mu.RUnlock()

	if exists {
		return queue, true
	}

	// if not found in memory, try loading from database
	if m.db != nil {
		if queue := m.loadQueueFromDB(queueID); queue != nil {
			m.mu.Lock()
			m.queues[queueID] = queue
			m.mu.Unlock()
			return queue, true
		}
	}

	return nil, false
}

// loadQueueFromDB loads a single queue from the database
func (m *BatchTaskManager) loadQueueFromDB(queueID string) *BatchTaskQueue {
	if m.db == nil {
		return nil
	}

	queueRow, err := m.db.GetBatchQueue(queueID)
	if err != nil || queueRow == nil {
		return nil
	}

	taskRows, err := m.db.GetBatchTasks(queueID)
	if err != nil {
		return nil
	}

	queue := &BatchTaskQueue{
		ID:           queueRow.ID,
		HITLPolicy:   queueRow.HITLPolicy,
		AgentMode:    "eino_single",
		ScheduleMode: "manual",
		Status:       queueRow.Status,
		CreatedAt:    queueRow.CreatedAt,
		CurrentIndex: queueRow.CurrentIndex,
		Tasks:        make([]*BatchTask, 0, len(taskRows)),
	}

	if queueRow.Title.Valid {
		queue.Title = queueRow.Title.String
	}
	if queueRow.Role.Valid {
		queue.Role = queueRow.Role.String
	}
	if queueRow.AgentMode.Valid {
		queue.AgentMode = config.NormalizeAgentMode(queueRow.AgentMode.String)
	}
	if queueRow.ScheduleMode.Valid {
		queue.ScheduleMode = normalizeBatchQueueScheduleMode(queueRow.ScheduleMode.String)
	}
	if queueRow.CronExpr.Valid && queue.ScheduleMode == "cron" {
		queue.CronExpr = strings.TrimSpace(queueRow.CronExpr.String)
	}
	if queueRow.NextRunAt.Valid && queue.ScheduleMode == "cron" {
		t := queueRow.NextRunAt.Time
		queue.NextRunAt = &t
	}
	queue.ScheduleEnabled = true
	if queueRow.ScheduleEnabled.Valid && queueRow.ScheduleEnabled.Int64 == 0 {
		queue.ScheduleEnabled = false
	}
	if queueRow.LastScheduleTriggerAt.Valid {
		t := queueRow.LastScheduleTriggerAt.Time
		queue.LastScheduleTriggerAt = &t
	}
	if queueRow.LastScheduleError.Valid {
		queue.LastScheduleError = strings.TrimSpace(queueRow.LastScheduleError.String)
	}
	if queueRow.LastRunError.Valid {
		queue.LastRunError = strings.TrimSpace(queueRow.LastRunError.String)
	}
	if queueRow.ProjectID.Valid {
		queue.ProjectID = strings.TrimSpace(queueRow.ProjectID.String)
	}
	queue.Concurrency = batchQueueConcurrencyFromRow(queueRow)
	if queueRow.StartedAt.Valid {
		queue.StartedAt = &queueRow.StartedAt.Time
	}
	if queueRow.CompletedAt.Valid {
		queue.CompletedAt = &queueRow.CompletedAt.Time
	}

	for _, taskRow := range taskRows {
		task := &BatchTask{
			ID:      taskRow.ID,
			Message: taskRow.Message,
			Status:  taskRow.Status,
		}
		if taskRow.ConversationID.Valid {
			task.ConversationID = taskRow.ConversationID.String
		}
		if taskRow.StartedAt.Valid {
			task.StartedAt = &taskRow.StartedAt.Time
		}
		if taskRow.CompletedAt.Valid {
			task.CompletedAt = &taskRow.CompletedAt.Time
		}
		if taskRow.Error.Valid {
			task.Error = taskRow.Error.String
		}
		if taskRow.Result.Valid {
			task.Result = taskRow.Result.String
		}
		queue.Tasks = append(queue.Tasks, task)
	}

	return queue
}

// GetLoadedQueues returns queues already loaded in memory (no DB load triggered, uses RLock only)
func (m *BatchTaskManager) GetLoadedQueues() []*BatchTaskQueue {
	m.mu.RLock()
	result := make([]*BatchTaskQueue, 0, len(m.queues))
	for _, queue := range m.queues {
		result = append(result, queue)
	}
	m.mu.RUnlock()
	return result
}

// GetAllQueues returns all queues
func (m *BatchTaskManager) GetAllQueues() []*BatchTaskQueue {
	m.mu.RLock()
	result := make([]*BatchTaskQueue, 0, len(m.queues))
	for _, queue := range m.queues {
		result = append(result, queue)
	}
	m.mu.RUnlock()

	// if database is available, ensure all database queues are loaded into memory
	if m.db != nil {
		dbQueues, err := m.db.GetAllBatchQueues()
		if err == nil {
			m.mu.Lock()
			for _, queueRow := range dbQueues {
				if _, exists := m.queues[queueRow.ID]; !exists {
					if queue := m.loadQueueFromDB(queueRow.ID); queue != nil {
						m.queues[queueRow.ID] = queue
						result = append(result, queue)
					}
				}
			}
			m.mu.Unlock()
		}
	}

	return result
}

// ListQueues lists queues (supports filtering and pagination)
func (m *BatchTaskManager) ListQueues(limit, offset int, status, keyword string) ([]*BatchTaskQueue, int, error) {
	return m.ListQueuesForAccess(limit, offset, status, keyword, "", "")
}

func (m *BatchTaskManager) ListQueuesForAccess(limit, offset int, status, keyword, userID, scope string) ([]*BatchTaskQueue, int, error) {
	var queues []*BatchTaskQueue
	var total int

	// if database is available, query from database
	if m.db != nil {
		// get total count
		count, err := m.db.CountBatchQueuesForAccess(status, keyword, userID, scope)
		if err != nil {
			return nil, 0, fmt.Errorf("count queue total failed: %w", err)
		}
		total = count

		// get queue list (IDs only)
		queueRows, err := m.db.ListBatchQueuesForAccess(limit, offset, status, keyword, userID, scope)
		if err != nil {
			return nil, 0, fmt.Errorf("query queue list failed: %w", err)
		}

		// load full queue info (from memory or database)
		m.mu.Lock()
		for _, queueRow := range queueRows {
			var queue *BatchTaskQueue
			// check memory first
			if cached, exists := m.queues[queueRow.ID]; exists {
				queue = cached
			} else {
				// load from database
				queue = m.loadQueueFromDB(queueRow.ID)
				if queue != nil {
					m.queues[queueRow.ID] = queue
				}
			}
			if queue != nil {
				queues = append(queues, queue)
			}
		}
		m.mu.Unlock()
	} else {
		// no database, filter and paginate from memory
		m.mu.RLock()
		allQueues := make([]*BatchTaskQueue, 0, len(m.queues))
		for _, queue := range m.queues {
			allQueues = append(allQueues, queue)
		}
		m.mu.RUnlock()

		// filter
		filtered := make([]*BatchTaskQueue, 0)
		for _, queue := range allQueues {
			// status filter
			if status != "" && status != "all" && queue.Status != status {
				continue
			}
			// keyword search (search queue ID and title)
			if keyword != "" {
				keywordLower := strings.ToLower(keyword)
				queueIDLower := strings.ToLower(queue.ID)
				queueTitleLower := strings.ToLower(queue.Title)
				if !strings.Contains(queueIDLower, keywordLower) && !strings.Contains(queueTitleLower, keywordLower) {
					// also search by created_at
					createdAtStr := queue.CreatedAt.Format("2006-01-02 15:04:05")
					if !strings.Contains(createdAtStr, keyword) {
						continue
					}
				}
			}
			filtered = append(filtered, queue)
		}

		// sort by created_at descending
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
		})

		total = len(filtered)

		// paginate
		start := offset
		if start > len(filtered) {
			start = len(filtered)
		}
		end := start + limit
		if end > len(filtered) {
			end = len(filtered)
		}
		if start < len(filtered) {
			queues = filtered[start:end]
		}
	}

	return queues, total, nil
}

// LoadFromDB loads all queues from the database
func (m *BatchTaskManager) LoadFromDB() error {
	if m.db == nil {
		return nil
	}

	queueRows, err := m.db.GetAllBatchQueues()
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, queueRow := range queueRows {
		if _, exists := m.queues[queueRow.ID]; exists {
			continue // already exists, skip
		}

		taskRows, err := m.db.GetBatchTasks(queueRow.ID)
		if err != nil {
			continue // skip tasks that failed to load
		}

		queue := &BatchTaskQueue{
			ID:           queueRow.ID,
			HITLPolicy:   queueRow.HITLPolicy,
			AgentMode:    "eino_single",
			ScheduleMode: "manual",
			Status:       queueRow.Status,
			CreatedAt:    queueRow.CreatedAt,
			CurrentIndex: queueRow.CurrentIndex,
			Tasks:        make([]*BatchTask, 0, len(taskRows)),
		}

		if queueRow.Title.Valid {
			queue.Title = queueRow.Title.String
		}
		if queueRow.Role.Valid {
			queue.Role = queueRow.Role.String
		}
		if queueRow.AgentMode.Valid {
			queue.AgentMode = config.NormalizeAgentMode(queueRow.AgentMode.String)
		}
		if queueRow.ScheduleMode.Valid {
			queue.ScheduleMode = normalizeBatchQueueScheduleMode(queueRow.ScheduleMode.String)
		}
		if queueRow.CronExpr.Valid && queue.ScheduleMode == "cron" {
			queue.CronExpr = strings.TrimSpace(queueRow.CronExpr.String)
		}
		if queueRow.NextRunAt.Valid && queue.ScheduleMode == "cron" {
			t := queueRow.NextRunAt.Time
			queue.NextRunAt = &t
		}
		queue.ScheduleEnabled = true
		if queueRow.ScheduleEnabled.Valid && queueRow.ScheduleEnabled.Int64 == 0 {
			queue.ScheduleEnabled = false
		}
		if queueRow.LastScheduleTriggerAt.Valid {
			t := queueRow.LastScheduleTriggerAt.Time
			queue.LastScheduleTriggerAt = &t
		}
		if queueRow.LastScheduleError.Valid {
			queue.LastScheduleError = strings.TrimSpace(queueRow.LastScheduleError.String)
		}
		if queueRow.LastRunError.Valid {
			queue.LastRunError = strings.TrimSpace(queueRow.LastRunError.String)
		}
		if queueRow.ProjectID.Valid {
			queue.ProjectID = strings.TrimSpace(queueRow.ProjectID.String)
		}
		queue.Concurrency = batchQueueConcurrencyFromRow(queueRow)
		if queueRow.StartedAt.Valid {
			queue.StartedAt = &queueRow.StartedAt.Time
		}
		if queueRow.CompletedAt.Valid {
			queue.CompletedAt = &queueRow.CompletedAt.Time
		}

		for _, taskRow := range taskRows {
			task := &BatchTask{
				ID:      taskRow.ID,
				Message: taskRow.Message,
				Status:  taskRow.Status,
			}
			if taskRow.ConversationID.Valid {
				task.ConversationID = taskRow.ConversationID.String
			}
			if taskRow.StartedAt.Valid {
				task.StartedAt = &taskRow.StartedAt.Time
			}
			if taskRow.CompletedAt.Valid {
				task.CompletedAt = &taskRow.CompletedAt.Time
			}
			if taskRow.Error.Valid {
				task.Error = taskRow.Error.String
			}
			if taskRow.Result.Valid {
				task.Result = taskRow.Result.String
			}
			queue.Tasks = append(queue.Tasks, task)
		}

		m.queues[queueRow.ID] = queue
	}

	return nil
}

// UpdateTaskStatus updatetask status
func (m *BatchTaskManager) UpdateTaskStatus(queueID, taskID, status string, result, errorMsg string) {
	m.UpdateTaskStatusWithConversationID(queueID, taskID, status, result, errorMsg, "")
}

// UpdateTaskStatusWithConversationID updates task status (including conversationId)
func (m *BatchTaskManager) UpdateTaskStatusWithConversationID(queueID, taskID, status string, result, errorMsg, conversationID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[queueID]
	if !exists {
		return
	}

	// DB first: persist first, then update memory on success, to avoid inconsistent status after restart
	if m.db != nil {
		if err := m.db.UpdateBatchTaskStatus(queueID, taskID, status, conversationID, result, errorMsg); err != nil {
			m.logger.Warn("batch task DB status update failed, skipping memory update",
				zap.String("queueId", queueID), zap.String("taskId", taskID), zap.Error(err))
			return
		}
	}

	for _, task := range queue.Tasks {
		if task.ID == taskID {
			task.Status = status
			if result != "" {
				task.Result = result
			}
			if errorMsg != "" {
				task.Error = errorMsg
			}
			if conversationID != "" {
				task.ConversationID = conversationID
			}
			now := time.Now()
			if status == BatchTaskStatusRunning && task.StartedAt == nil {
				task.StartedAt = &now
			}
			if status == BatchTaskStatusCompleted || status == BatchTaskStatusFailed || status == BatchTaskStatusCancelled {
				task.CompletedAt = &now
			}
			break
		}
	}
}

// UpdateQueueStatus updatequeuestatus
func (m *BatchTaskManager) UpdateQueueStatus(queueID, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[queueID]
	if !exists {
		return
	}

	// DB first: persist first, then update memory on success
	if m.db != nil {
		if err := m.db.UpdateBatchQueueStatus(queueID, status); err != nil {
			m.logger.Warn("batch queue DB status update failed, skipping memory update",
				zap.String("queueId", queueID), zap.Error(err))
			return
		}
	}

	queue.Status = status
	now := time.Now()
	if status == BatchQueueStatusRunning && queue.StartedAt == nil {
		queue.StartedAt = &now
	}
	if status == BatchQueueStatusCompleted || status == BatchQueueStatusCancelled {
		queue.CompletedAt = &now
	}
}

// UpdateQueueSchedule updates the queue scheduling config
func (m *BatchTaskManager) UpdateQueueSchedule(queueID, scheduleMode, cronExpr string, nextRunAt *time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[queueID]
	if !exists {
		return
	}

	queue.ScheduleMode = normalizeBatchQueueScheduleMode(scheduleMode)
	if queue.ScheduleMode == "cron" {
		queue.CronExpr = strings.TrimSpace(cronExpr)
		queue.NextRunAt = nextRunAt
	} else {
		queue.CronExpr = ""
		queue.NextRunAt = nil
	}

	if m.db != nil {
		if err := m.db.UpdateBatchQueueSchedule(queueID, queue.ScheduleMode, queue.CronExpr, queue.NextRunAt); err != nil {
			m.logger.Warn("batch queue DB schedule update failed", zap.String("queueId", queueID), zap.Error(err))
		}
	}
}

// batchQueueConcurrencyFromRow reads Concurrency from a database row (default 1).
func batchQueueConcurrencyFromRow(row *database.BatchTaskQueueRow) int {
	if row == nil || !row.Concurrency.Valid {
		return DefaultBatchQueueConcurrency
	}
	return normalizeBatchQueueConcurrency(int(row.Concurrency.Int64))
}

// UpdateQueueMetadata updates queue title, role, agent mode, and Concurrency (available when not running)
func (m *BatchTaskManager) UpdateQueueMetadata(queueID, title, role, agentMode string, concurrency *int, hitlPolicies ...string) error {
	if concurrency != nil && (*concurrency < 1 || *concurrency > MaxBatchQueueConcurrency) {
		return fmt.Errorf("Concurrency must be between 1 and %d", MaxBatchQueueConcurrency)
	}
	if utf8.RuneCountInString(title) > MaxBatchQueueTitleLen {
		return fmt.Errorf("title cannot exceed %d characters", MaxBatchQueueTitleLen)
	}
	if utf8.RuneCountInString(role) > MaxBatchQueueRoleLen {
		return fmt.Errorf("role name cannot exceed %d characters", MaxBatchQueueRoleLen)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[queueID]
	if !exists {
		return fmt.Errorf("queue not found")
	}
	if queue.Status == BatchQueueStatusRunning {
		return fmt.Errorf("queue is running, cannot modify")
	}

	policy := queue.HITLPolicy
	if len(hitlPolicies) > 0 {
		if !queueAllowsTaskListMutationLocked(queue) {
			return fmt.Errorf("queue has tasks in progress, cannot modify approval settings")
		}
		policy = hitlPolicies[0]
		if err := validateBatchHITLPolicy(policy); err != nil {
			return err
		}
	}
	nextConcurrency := queue.Concurrency
	if concurrency != nil {
		nextConcurrency = normalizeBatchQueueConcurrency(*concurrency)
	}

	// if agentMode is not provided, keep the original value
	if strings.TrimSpace(agentMode) != "" {
		agentMode = config.NormalizeAgentMode(agentMode)
	} else {
		agentMode = queue.AgentMode
	}

	if m.db != nil {
		if err := m.db.UpdateBatchQueueMetadata(queueID, title, role, agentMode, nextConcurrency, policy); err != nil {
			return fmt.Errorf("savetaskqueuefailed: %w", err)
		}
	}
	queue.Title = title
	queue.Role = role
	queue.AgentMode = agentMode
	queue.Concurrency = nextConcurrency
	queue.HITLPolicy = policy
	return nil
}

// SetScheduleEnabled pauses/resumes Cron automatic scheduling (does not affect manual execution)
func (m *BatchTaskManager) SetScheduleEnabled(queueID string, enabled bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[queueID]
	if !exists {
		return false
	}
	queue.ScheduleEnabled = enabled
	if m.db != nil {
		_ = m.db.UpdateBatchQueueScheduleEnabled(queueID, enabled)
	}
	return true
}

// RecordScheduledRunStart is called when Cron triggers successfully and is about to execute sub-tasks
func (m *BatchTaskManager) RecordScheduledRunStart(queueID string) {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[queueID]
	if !exists {
		return
	}
	queue.LastScheduleTriggerAt = &now
	queue.LastScheduleError = ""
	if m.db != nil {
		_ = m.db.RecordBatchQueueScheduledTriggerStart(queueID, now)
	}
}

// SetLastScheduleError records a scheduling-layer failure (execution did not start successfully)
func (m *BatchTaskManager) SetLastScheduleError(queueID, msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[queueID]
	if !exists {
		return
	}
	queue.LastScheduleError = strings.TrimSpace(msg)
	if m.db != nil {
		_ = m.db.SetBatchQueueLastScheduleError(queueID, queue.LastScheduleError)
	}
}

// SetLastRunError records the failure summary from the most recent batch execution run
func (m *BatchTaskManager) SetLastRunError(queueID, msg string) {
	msg = strings.TrimSpace(msg)
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[queueID]
	if !exists {
		return
	}
	queue.LastRunError = msg
	if m.db != nil {
		_ = m.db.SetBatchQueueLastRunError(queueID, msg)
	}
}

// ResetQueueForRerun resets the queue and sub-task statuses for the next cron run
func (m *BatchTaskManager) ResetQueueForRerun(queueID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[queueID]
	if !exists {
		return false
	}

	// DB first: persist reset first, then update memory on success, to avoid dirty in-memory state if DB fails
	if m.db != nil {
		if err := m.db.ResetBatchQueueForRerun(queueID); err != nil {
			m.logger.Warn("batch queue DB reset for rerun failed, skipping memory update",
				zap.String("queueId", queueID), zap.Error(err))
			return false
		}
	}

	queue.Status = BatchQueueStatusPending
	queue.CurrentIndex = 0
	queue.StartedAt = nil
	queue.CompletedAt = nil
	queue.NextRunAt = nil
	queue.LastRunError = ""
	queue.LastScheduleError = ""
	for _, task := range queue.Tasks {
		task.Status = BatchTaskStatusPending
		task.ConversationID = ""
		task.StartedAt = nil
		task.CompletedAt = nil
		task.Error = ""
		task.Result = ""
	}
	return true
}

// UpdateTaskMessage updates a task message (can be changed when queue is idle; task must not be running)
func (m *BatchTaskManager) UpdateTaskMessage(queueID, taskID, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[queueID]
	if !exists {
		return fmt.Errorf("queue not found")
	}

	if !queueAllowsTaskListMutationLocked(queue) {
		return fmt.Errorf("queue is running or not ready, cannot edit task")
	}

	// find and update task
	for _, task := range queue.Tasks {
		if task.ID == taskID {
			if task.Status == BatchTaskStatusRunning {
				return fmt.Errorf("cannot edit a running task")
			}
			task.Message = message

			// sync to database
			if m.db != nil {
				if err := m.db.UpdateBatchTaskMessage(queueID, taskID, message); err != nil {
					return fmt.Errorf("updatetaskmessagefailed: %w", err)
				}
			}
			return nil
		}
	}

	return fmt.Errorf("task not found")
}

// AddTaskToQueue adds a task to the queue (can be added when queue is idle: including cron-completed this round, manually paused, etc.)
func (m *BatchTaskManager) AddTaskToQueue(queueID, message string) (*BatchTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[queueID]
	if !exists {
		return nil, fmt.Errorf("queue not found")
	}

	if !queueAllowsTaskListMutationLocked(queue) {
		return nil, fmt.Errorf("queue is running or not ready, cannot add task")
	}

	if message == "" {
		return nil, fmt.Errorf("task message cannot be empty")
	}

	// Generate task ID
	taskID := generateShortID()
	task := &BatchTask{
		ID:      taskID,
		Message: message,
		Status:  BatchTaskStatusPending,
	}

	// Add to in-memory queue
	queue.Tasks = append(queue.Tasks, task)

	// sync to database
	if m.db != nil {
		if err := m.db.AddBatchTask(queueID, taskID, message); err != nil {
			// If database save failed, remove from memory
			queue.Tasks = queue.Tasks[:len(queue.Tasks)-1]
			return nil, fmt.Errorf("failed to add task: %w", err)
		}
	}

	return task, nil
}

// PrepareSingleTaskRun prepares a single-task run: resets the target task (if it has results) and locates its queue index
func (m *BatchTaskManager) PrepareSingleTaskRun(queueID, taskID string) error {
	var siblingRunningIDs []string

	m.mu.Lock()
	queue, exists := m.queues[queueID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("queue not found")
	}

	var task *BatchTask
	taskIndex := -1
	for i, t := range queue.Tasks {
		if t.ID == taskID {
			taskIndex = i
			task = t
			break
		}
	}
	if task == nil {
		m.mu.Unlock()
		return fmt.Errorf("task not found")
	}

	if !queueAllowsSingleTaskRunLocked(queue, task) {
		m.mu.Unlock()
		return fmt.Errorf("queue is running or not ready; cannot execute single task")
	}

	// Paused state: cancel in-flight sub-tasks and finalize other sub-tasks still marked running, to allow single-task execution of non-conflicting items
	var cancelFuncs []context.CancelFunc
	if queue.Status == BatchQueueStatusPaused {
		cancelFuncs = m.drainTaskCancelsLocked(queueID)
		for _, t := range queue.Tasks {
			if t != nil && t.ID != taskID && t.Status == BatchTaskStatusRunning {
				siblingRunningIDs = append(siblingRunningIDs, t.ID)
			}
		}
	}

	needsReset := task.Status != BatchTaskStatusPending
	resumeQueue := queue.Status == BatchQueueStatusCompleted || queue.Status == BatchQueueStatusCancelled
	m.mu.Unlock()

	for _, c := range cancelFuncs {
		if c != nil {
			c()
		}
	}
	const staleRunMsg = "cancelled to allow single-task execution of another task"
	for _, sid := range siblingRunningIDs {
		m.UpdateTaskStatus(queueID, sid, BatchTaskStatusCancelled, "", staleRunMsg)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists = m.queues[queueID]
	if !exists {
		return fmt.Errorf("queue not found")
	}

	task = nil
	taskIndex = -1
	for i, t := range queue.Tasks {
		if t.ID == taskID {
			taskIndex = i
			task = t
			break
		}
	}
	if task == nil {
		return fmt.Errorf("task not found")
	}

	if m.db != nil {
		if err := m.db.PrepareBatchSingleTaskRun(queueID, taskID, taskIndex, needsReset, resumeQueue); err != nil {
			return fmt.Errorf("failed to prepare single-task run: %w", err)
		}
	}

	if needsReset {
		task.Status = BatchTaskStatusPending
		task.ConversationID = ""
		task.StartedAt = nil
		task.CompletedAt = nil
		task.Error = ""
		task.Result = ""
	}
	queue.CurrentIndex = taskIndex
	queue.LastRunError = ""
	if resumeQueue {
		queue.Status = BatchQueueStatusPaused
		queue.CompletedAt = nil
	}

	return nil
}

// SetSingleRunTask marks the queue to execute only the specified sub-task; the queue auto-pauses after completion
func (m *BatchTaskManager) SetSingleRunTask(queueID, taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.singleRunTasks == nil {
		m.singleRunTasks = make(map[string]string)
	}
	m.singleRunTasks[queueID] = taskID
}

// ClearSingleRunTask clears the single-task execution flag
func (m *BatchTaskManager) ClearSingleRunTask(queueID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.singleRunTasks, queueID)
}

// TakeSingleRunTaskIfMatch clears the flag and returns true if the just-completed sub-task was the single-execution target
func (m *BatchTaskManager) TakeSingleRunTaskIfMatch(queueID, taskID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.singleRunTasks == nil {
		return false
	}
	if m.singleRunTasks[queueID] != taskID {
		return false
	}
	delete(m.singleRunTasks, queueID)
	return true
}

// DeleteTask deletes a task (can be deleted when queue is idle; cannot delete running tasks)
func (m *BatchTaskManager) DeleteTask(queueID, taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[queueID]
	if !exists {
		return fmt.Errorf("queue not found")
	}

	if !queueAllowsTaskListMutationLocked(queue) {
		return fmt.Errorf("queue is running or not ready; cannot delete task")
	}

	// Find the task
	taskIndex := -1
	for i, task := range queue.Tasks {
		if task.ID == taskID {
			if task.Status == BatchTaskStatusRunning {
				return fmt.Errorf("cannot delete a running task")
			}
			taskIndex = i
			break
		}
	}

	if taskIndex == -1 {
		return fmt.Errorf("task not found")
	}

	// DB first: delete from database first, then remove from memory on success
	if m.db != nil {
		if err := m.db.DeleteBatchTask(queueID, taskID); err != nil {
			return fmt.Errorf("deletetaskfailed: %w", err)
		}
	}

	queue.Tasks = append(queue.Tasks[:taskIndex], queue.Tasks[taskIndex+1:]...)
	return nil
}

func queueHasRunningTaskLocked(queue *BatchTaskQueue) bool {
	if queue == nil {
		return false
	}
	for _, t := range queue.Tasks {
		if t != nil && t.Status == BatchTaskStatusRunning {
			return true
		}
	}
	return false
}

// queueAllowsTaskListMutationLocked returns whether adding/removing/modifying sub-tasks is allowed (must be called while holding BatchTaskManager.mu)
func queueAllowsTaskListMutationLocked(queue *BatchTaskQueue) bool {
	if queue == nil {
		return false
	}
	if queue.Status == BatchQueueStatusRunning {
		return false
	}
	if queueHasRunningTaskLocked(queue) {
		return false
	}
	switch queue.Status {
	case BatchQueueStatusPending, BatchQueueStatusPaused, BatchQueueStatusCompleted, BatchQueueStatusCancelled:
		return true
	default:
		return false
	}
}

// queueAllowsSingleTaskRunLocked returns whether a single-task run can be initiated for the given sub-task (must be called while holding BatchTaskManager.mu)
func queueAllowsSingleTaskRunLocked(queue *BatchTaskQueue, task *BatchTask) bool {
	if queue == nil || task == nil {
		return false
	}
	if task.Status == BatchTaskStatusRunning {
		return false
	}
	if queue.Status == BatchQueueStatusRunning {
		return false
	}
	switch queue.Status {
	case BatchQueueStatusPending, BatchQueueStatusPaused, BatchQueueStatusCompleted, BatchQueueStatusCancelled:
		return true
	default:
		return false
	}
}

// ClaimNextPendingTask atomically claims the next pending sub-task (safe for concurrent workers).
func (m *BatchTaskManager) ClaimNextPendingTask(queueID string) (*BatchTask, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[queueID]
	if !exists || queue == nil {
		return nil, false
	}
	if queue.Status == BatchQueueStatusCancelled || queue.Status == BatchQueueStatusCompleted || queue.Status == BatchQueueStatusPaused {
		return nil, false
	}

	onlyTaskID := ""
	if m.singleRunTasks != nil {
		onlyTaskID = m.singleRunTasks[queueID]
	}

	for i, task := range queue.Tasks {
		if task == nil || task.Status != BatchTaskStatusPending {
			continue
		}
		if onlyTaskID != "" && task.ID != onlyTaskID {
			continue
		}
		task.Status = BatchTaskStatusRunning
		queue.CurrentIndex = i
		return task, true
	}
	return nil, false
}

// HasRunningTasks returns whether the queue still has sub-tasks in running status.
func (m *BatchTaskManager) HasRunningTasks(queueID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	queue, exists := m.queues[queueID]
	if !exists || queue == nil {
		return false
	}
	for _, task := range queue.Tasks {
		if task != nil && task.Status == BatchTaskStatusRunning {
			return true
		}
	}
	return false
}

// HasPendingOrRunningTasks returns whether the queue still has unfinished sub-tasks.
func (m *BatchTaskManager) HasPendingOrRunningTasks(queueID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	queue, exists := m.queues[queueID]
	if !exists || queue == nil {
		return false
	}
	for _, task := range queue.Tasks {
		if task == nil {
			continue
		}
		if task.Status == BatchTaskStatusPending || task.Status == BatchTaskStatusRunning {
			return true
		}
	}
	return false
}

// drainTaskCancelsLocked drains and clears all sub-task cancel functions for the queue (caller must hold m.mu).
func (m *BatchTaskManager) drainTaskCancelsLocked(queueID string) []context.CancelFunc {
	taskMap, ok := m.taskCancels[queueID]
	if !ok || len(taskMap) == 0 {
		return nil
	}
	cancels := make([]context.CancelFunc, 0, len(taskMap))
	for _, c := range taskMap {
		if c != nil {
			cancels = append(cancels, c)
		}
	}
	delete(m.taskCancels, queueID)
	return cancels
}

// GetNextTask returns the next pending task (serial-compatible; prefer ClaimNextPendingTask for concurrent use)
func (m *BatchTaskManager) GetNextTask(queueID string) (*BatchTask, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[queueID]
	if !exists {
		return nil, false
	}

	for i := queue.CurrentIndex; i < len(queue.Tasks); i++ {
		task := queue.Tasks[i]
		if task.Status == BatchTaskStatusPending {
			queue.CurrentIndex = i
			return task, true
		}
	}

	return nil, false
}

// MoveToNextTask advances to the next task
func (m *BatchTaskManager) MoveToNextTask(queueID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[queueID]
	if !exists {
		return
	}

	queue.CurrentIndex++

	// sync to database
	if m.db != nil {
		if err := m.db.UpdateBatchQueueCurrentIndex(queueID, queue.CurrentIndex); err != nil {
			m.logger.Warn("batch queue DB index update failed", zap.String("queueId", queueID), zap.Error(err))
		}
	}
}

// SetTaskCancel sets the cancel function for a sub-task
func (m *BatchTaskManager) SetTaskCancel(queueID, taskID string, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cancel == nil {
		if taskMap, ok := m.taskCancels[queueID]; ok {
			delete(taskMap, taskID)
			if len(taskMap) == 0 {
				delete(m.taskCancels, queueID)
			}
		}
		return
	}
	if m.taskCancels[queueID] == nil {
		m.taskCancels[queueID] = make(map[string]context.CancelFunc)
	}
	m.taskCancels[queueID][taskID] = cancel
}

// PauseQueue pausequeue
func (m *BatchTaskManager) PauseQueue(queueID string) bool {
	var cancelFuncs []context.CancelFunc

	m.mu.Lock()
	queue, exists := m.queues[queueID]
	if !exists {
		m.mu.Unlock()
		return false
	}

	if queue.Status != BatchQueueStatusRunning {
		m.mu.Unlock()
		return false
	}

	// DB first: persist first, then update memory on success
	if m.db != nil {
		if err := m.db.UpdateBatchQueueStatus(queueID, BatchQueueStatusPaused); err != nil {
			m.logger.Warn("batch queue DB pause update failed, skipping memory update",
				zap.String("queueId", queueID), zap.Error(err))
			m.mu.Unlock()
			return false
		}
	}

	queue.Status = BatchQueueStatusPaused
	cancelFuncs = m.drainTaskCancelsLocked(queueID)
	m.mu.Unlock()

	for _, c := range cancelFuncs {
		c()
	}

	return true
}

// CancelQueue cancels a queue (retained for backward compatibility; prefer PauseQueue)
func (m *BatchTaskManager) CancelQueue(queueID string) bool {
	now := time.Now()
	var cancelFuncs []context.CancelFunc

	m.mu.Lock()
	queue, exists := m.queues[queueID]
	if !exists {
		m.mu.Unlock()
		return false
	}

	if queue.Status == BatchQueueStatusCompleted || queue.Status == BatchQueueStatusCancelled {
		m.mu.Unlock()
		return false
	}

	// DB first: persist first, then update memory on success
	if m.db != nil {
		if err := m.db.CancelPendingBatchTasks(queueID, now); err != nil {
			m.logger.Warn("batch task DB batch cancel failed, skipping memory update",
				zap.String("queueId", queueID), zap.Error(err))
			m.mu.Unlock()
			return false
		}
		if err := m.db.UpdateBatchQueueStatus(queueID, BatchQueueStatusCancelled); err != nil {
			m.logger.Warn("batch queue DB cancel update failed, skipping memory update",
				zap.String("queueId", queueID), zap.Error(err))
			m.mu.Unlock()
			return false
		}
	}

	queue.Status = BatchQueueStatusCancelled
	queue.CompletedAt = &now

	// Batch-mark all pending tasks as cancelled in memory
	for _, task := range queue.Tasks {
		if task.Status == BatchTaskStatusPending {
			task.Status = BatchTaskStatusCancelled
			task.CompletedAt = &now
		}
	}

	cancelFuncs = m.drainTaskCancelsLocked(queueID)
	m.mu.Unlock()

	for _, c := range cancelFuncs {
		c()
	}

	return true
}

// DeleteQueue deletes a queue. Refuses deletion when an executor goroutine is active or status is running, to avoid nil-pointer panics in executeBatchQueue.
func (m *BatchTaskManager) DeleteQueue(queueID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[queueID]
	if !exists {
		return ErrBatchQueueNotFound
	}

	if _, exec := m.queueExecutors[queueID]; exec {
		return ErrBatchQueueExecutorActive
	}

	// Running queues cannot be deleted to prevent orphan goroutines and data loss
	if queue.Status == BatchQueueStatusRunning {
		return ErrBatchQueueStillRunning
	}

	// Clean up cancel functions
	delete(m.taskCancels, queueID)

	// Delete from database
	if m.db != nil {
		if err := m.db.DeleteBatchQueue(queueID); err != nil {
			m.logger.Warn("batch queue DB delete failed", zap.String("queueId", queueID), zap.Error(err))
		}
	}

	delete(m.queues, queueID)
	return nil
}

// generateShortID generates a short ID
func generateShortID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return time.Now().Format("150405") + "-" + hex.EncodeToString(b)
}
