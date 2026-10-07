package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// BatchTaskQueueRow is a batch task queue database row
type BatchTaskQueueRow struct {
	ID                    string
	Title                 sql.NullString
	Role                  sql.NullString
	AgentMode             sql.NullString
	HITLPolicy            string
	ScheduleMode          sql.NullString
	CronExpr              sql.NullString
	NextRunAt             sql.NullTime
	ScheduleEnabled       sql.NullInt64
	LastScheduleTriggerAt sql.NullTime
	LastScheduleError     sql.NullString
	LastRunError          sql.NullString
	ProjectID             sql.NullString
	Concurrency           sql.NullInt64
	Status                string
	CreatedAt             time.Time
	StartedAt             sql.NullTime
	CompletedAt           sql.NullTime
	CurrentIndex          int
}

// BatchTaskRow is a batch task database row
type BatchTaskRow struct {
	ID             string
	QueueID        string
	Message        string
	ConversationID sql.NullString
	Status         string
	StartedAt      sql.NullTime
	CompletedAt    sql.NullTime
	Error          sql.NullString
	Result         sql.NullString
}

// CreateBatchQueue creates a batch task queue
func (db *DB) CreateBatchQueue(
	queueID string,
	title string,
	role string,
	agentMode string,
	scheduleMode string,
	cronExpr string,
	nextRunAt *time.Time,
	projectID string,
	concurrency int,
	tasks []map[string]interface{},
	hitlPolicies ...string,
) error {
	policy := ""
	if len(hitlPolicies) > 0 {
		policy = hitlPolicies[0]
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction failed: %w", err)
	}
	defer tx.Rollback()

	now := time.Now()
	var nextRunAtValue interface{}
	if nextRunAt != nil {
		nextRunAtValue = *nextRunAt
	}

	var projectIDVal interface{}
	if strings.TrimSpace(projectID) != "" {
		projectIDVal = strings.TrimSpace(projectID)
	}
	_, err = tx.Exec(
		"INSERT INTO batch_task_queues (id, title, role, agent_mode, hitl_policy, schedule_mode, cron_expr, next_run_at, schedule_enabled, project_id, concurrency, status, created_at, current_index) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		queueID, title, role, agentMode, policy, scheduleMode, cronExpr, nextRunAtValue, 1, projectIDVal, concurrency, "pending", now, 0,
	)
	if err != nil {
		return fmt.Errorf("create batch task queue failed: %w", err)
	}

	// insert tasks
	for _, task := range tasks {
		taskID, ok := task["id"].(string)
		if !ok {
			continue
		}
		message, ok := task["message"].(string)
		if !ok {
			continue
		}

		_, err = tx.Exec(
			"INSERT INTO batch_tasks (id, queue_id, message, status) VALUES (?, ?, ?, ?)",
			taskID, queueID, message, "pending",
		)
		if err != nil {
			return fmt.Errorf("create batch task failed: %w", err)
		}
	}

	return tx.Commit()
}

const batchQueueSelectColumns = `id, title, role, agent_mode, hitl_policy, schedule_mode, cron_expr, next_run_at, schedule_enabled, last_schedule_trigger_at, last_schedule_error, last_run_error, project_id, concurrency, status, created_at, started_at, completed_at, current_index`

// GetBatchQueue gets a batch task queue
func (db *DB) GetBatchQueue(queueID string) (*BatchTaskQueueRow, error) {
	var row BatchTaskQueueRow
	var createdAt string
	err := db.QueryRow(
		"SELECT "+batchQueueSelectColumns+" FROM batch_task_queues WHERE id = ?",
		queueID,
	).Scan(&row.ID, &row.Title, &row.Role, &row.AgentMode, &row.HITLPolicy, &row.ScheduleMode, &row.CronExpr, &row.NextRunAt, &row.ScheduleEnabled, &row.LastScheduleTriggerAt, &row.LastScheduleError, &row.LastRunError, &row.ProjectID, &row.Concurrency, &row.Status, &createdAt, &row.StartedAt, &row.CompletedAt, &row.CurrentIndex)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query batch task queue failed: %w", err)
	}

	parsedTime, parseErr := time.Parse("2006-01-02 15:04:05", createdAt)
	if parseErr != nil {
		// try other time formats
		parsedTime, parseErr = time.Parse(time.RFC3339, createdAt)
		if parseErr != nil {
			db.logger.Warn("parse created_at failed", zap.String("createdAt", createdAt), zap.Error(parseErr))
			parsedTime = time.Now()
		}
	}
	row.CreatedAt = parsedTime
	return &row, nil
}

// GetAllBatchQueues returns all batch task queues
func (db *DB) GetAllBatchQueues() ([]*BatchTaskQueueRow, error) {
	rows, err := db.Query(
		"SELECT " + batchQueueSelectColumns + " FROM batch_task_queues ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, fmt.Errorf("query batch task queue list failed: %w", err)
	}
	defer rows.Close()

	var queues []*BatchTaskQueueRow
	for rows.Next() {
		var row BatchTaskQueueRow
		var createdAt string
		if err := rows.Scan(&row.ID, &row.Title, &row.Role, &row.AgentMode, &row.HITLPolicy, &row.ScheduleMode, &row.CronExpr, &row.NextRunAt, &row.ScheduleEnabled, &row.LastScheduleTriggerAt, &row.LastScheduleError, &row.LastRunError, &row.ProjectID, &row.Concurrency, &row.Status, &createdAt, &row.StartedAt, &row.CompletedAt, &row.CurrentIndex); err != nil {
			return nil, fmt.Errorf("scan batch task queue failed: %w", err)
		}
		parsedTime, parseErr := time.Parse("2006-01-02 15:04:05", createdAt)
		if parseErr != nil {
			parsedTime, parseErr = time.Parse(time.RFC3339, createdAt)
			if parseErr != nil {
				db.logger.Warn("parse created_at failed", zap.String("createdAt", createdAt), zap.Error(parseErr))
				parsedTime = time.Now()
			}
		}
		row.CreatedAt = parsedTime
		queues = append(queues, &row)
	}

	return queues, nil
}

// ListBatchQueues lists batch task queues (supports filtering and pagination)
func (db *DB) ListBatchQueues(limit, offset int, status, keyword string) ([]*BatchTaskQueueRow, error) {
	return db.ListBatchQueuesForAccess(limit, offset, status, keyword, "", "")
}

func (db *DB) ListBatchQueuesForAccess(limit, offset int, status, keyword, userID, scope string) ([]*BatchTaskQueueRow, error) {
	query := "SELECT " + batchQueueSelectColumns + " FROM batch_task_queues WHERE 1=1"
	args := []interface{}{}

	// status filter
	if status != "" && status != "all" {
		query += " AND status = ?"
		args = append(args, status)
	}

	// keyword search (searches queue ID and title)
	if keyword != "" {
		query += " AND (id LIKE ? OR title LIKE ?)"
		args = append(args, "%"+keyword+"%", "%"+keyword+"%")
	}
	userID = strings.TrimSpace(userID)
	if userID != "" && scope != RBACScopeAll {
		query += ` AND (
			owner_user_id = ?
			OR EXISTS (
				SELECT 1 FROM rbac_resource_assignments ra
				WHERE ra.user_id = ? AND ra.resource_type = 'batch_task' AND ra.resource_id = batch_task_queues.id
			)
			OR (
				project_id IS NOT NULL AND project_id <> '' AND (
					EXISTS (SELECT 1 FROM projects p WHERE p.id = batch_task_queues.project_id AND p.owner_user_id = ?)
					OR EXISTS (
						SELECT 1 FROM rbac_resource_assignments pra
						WHERE pra.user_id = ? AND pra.resource_type = 'project' AND pra.resource_id = batch_task_queues.project_id
					)
				)
			)
		)`
		args = append(args, userID, userID, userID, userID)
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query batch task queue list failed: %w", err)
	}
	defer rows.Close()

	var queues []*BatchTaskQueueRow
	for rows.Next() {
		var row BatchTaskQueueRow
		var createdAt string
		if err := rows.Scan(&row.ID, &row.Title, &row.Role, &row.AgentMode, &row.HITLPolicy, &row.ScheduleMode, &row.CronExpr, &row.NextRunAt, &row.ScheduleEnabled, &row.LastScheduleTriggerAt, &row.LastScheduleError, &row.LastRunError, &row.ProjectID, &row.Concurrency, &row.Status, &createdAt, &row.StartedAt, &row.CompletedAt, &row.CurrentIndex); err != nil {
			return nil, fmt.Errorf("scan batch task queue failed: %w", err)
		}
		parsedTime, parseErr := time.Parse("2006-01-02 15:04:05", createdAt)
		if parseErr != nil {
			parsedTime, parseErr = time.Parse(time.RFC3339, createdAt)
			if parseErr != nil {
				db.logger.Warn("parse created_at failed", zap.String("createdAt", createdAt), zap.Error(parseErr))
				parsedTime = time.Now()
			}
		}
		row.CreatedAt = parsedTime
		queues = append(queues, &row)
	}

	return queues, nil
}

// CountBatchQueues counts batch task queues (supports filter conditions)
func (db *DB) CountBatchQueues(status, keyword string) (int, error) {
	return db.CountBatchQueuesForAccess(status, keyword, "", "")
}

func (db *DB) CountBatchQueuesForAccess(status, keyword, userID, scope string) (int, error) {
	query := "SELECT COUNT(*) FROM batch_task_queues WHERE 1=1"
	args := []interface{}{}

	// status filter
	if status != "" && status != "all" {
		query += " AND status = ?"
		args = append(args, status)
	}

	// keyword search (searches queue ID and title)
	if keyword != "" {
		query += " AND (id LIKE ? OR title LIKE ?)"
		args = append(args, "%"+keyword+"%", "%"+keyword+"%")
	}
	userID = strings.TrimSpace(userID)
	if userID != "" && scope != RBACScopeAll {
		query += ` AND (
			owner_user_id = ?
			OR EXISTS (
				SELECT 1 FROM rbac_resource_assignments ra
				WHERE ra.user_id = ? AND ra.resource_type = 'batch_task' AND ra.resource_id = batch_task_queues.id
			)
			OR (
				project_id IS NOT NULL AND project_id <> '' AND (
					EXISTS (SELECT 1 FROM projects p WHERE p.id = batch_task_queues.project_id AND p.owner_user_id = ?)
					OR EXISTS (
						SELECT 1 FROM rbac_resource_assignments pra
						WHERE pra.user_id = ? AND pra.resource_type = 'project' AND pra.resource_id = batch_task_queues.project_id
					)
				)
			)
		)`
		args = append(args, userID, userID, userID, userID)
	}

	var count int
	err := db.QueryRow(query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count batch task queues failed: %w", err)
	}

	return count, nil
}

// GetBatchTasks returns all tasks in a batch task queue
func (db *DB) GetBatchTasks(queueID string) ([]*BatchTaskRow, error) {
	rows, err := db.Query(
		"SELECT id, queue_id, message, conversation_id, status, started_at, completed_at, error, result FROM batch_tasks WHERE queue_id = ? ORDER BY rowid ASC",
		queueID,
	)
	if err != nil {
		return nil, fmt.Errorf("query batch tasks failed: %w", err)
	}
	defer rows.Close()

	var tasks []*BatchTaskRow
	for rows.Next() {
		var task BatchTaskRow
		if err := rows.Scan(
			&task.ID, &task.QueueID, &task.Message, &task.ConversationID,
			&task.Status, &task.StartedAt, &task.CompletedAt, &task.Error, &task.Result,
		); err != nil {
			return nil, fmt.Errorf("scan batch task failed: %w", err)
		}
		tasks = append(tasks, &task)
	}

	return tasks, nil
}

// UpdateBatchQueueStatus updates the status of a batch task queue
func (db *DB) UpdateBatchQueueStatus(queueID, status string) error {
	var err error
	now := time.Now()

	if status == "running" {
		_, err = db.Exec(
			"UPDATE batch_task_queues SET status = ?, started_at = COALESCE(started_at, ?) WHERE id = ?",
			status, now, queueID,
		)
	} else if status == "completed" || status == "cancelled" {
		_, err = db.Exec(
			"UPDATE batch_task_queues SET status = ?, completed_at = COALESCE(completed_at, ?) WHERE id = ?",
			status, now, queueID,
		)
	} else {
		_, err = db.Exec(
			"UPDATE batch_task_queues SET status = ? WHERE id = ?",
			status, queueID,
		)
	}

	if err != nil {
		return fmt.Errorf("update batch task queue status failed: %w", err)
	}
	return nil
}

// UpdateBatchTaskStatus updates a batch task's status
func (db *DB) UpdateBatchTaskStatus(queueID, taskID, status string, conversationID, result, errorMsg string) error {
	var err error
	now := time.Now()

	// build update statement
	var updates []string
	var args []interface{}

	updates = append(updates, "status = ?")
	args = append(args, status)

	if conversationID != "" {
		updates = append(updates, "conversation_id = ?")
		args = append(args, conversationID)
	}

	if result != "" {
		updates = append(updates, "result = ?")
		args = append(args, result)
	}

	if errorMsg != "" {
		updates = append(updates, "error = ?")
		args = append(args, errorMsg)
	}

	if status == "running" {
		updates = append(updates, "started_at = COALESCE(started_at, ?)")
		args = append(args, now)
	}

	if status == "completed" || status == "failed" || status == "cancelled" {
		updates = append(updates, "completed_at = COALESCE(completed_at, ?)")
		args = append(args, now)
	}

	args = append(args, queueID, taskID)

	// build SQL statement
	sql := "UPDATE batch_tasks SET "
	for i, update := range updates {
		if i > 0 {
			sql += ", "
		}
		sql += update
	}
	sql += " WHERE queue_id = ? AND id = ?"

	_, err = db.Exec(sql, args...)
	if err != nil {
		return fmt.Errorf("update batch task status failed: %w", err)
	}
	return nil
}

// UpdateBatchQueueCurrentIndex updates the current index of a batch task queue
func (db *DB) UpdateBatchQueueCurrentIndex(queueID string, currentIndex int) error {
	_, err := db.Exec(
		"UPDATE batch_task_queues SET current_index = ? WHERE id = ?",
		currentIndex, queueID,
	)
	if err != nil {
		return fmt.Errorf("update batch task queue current index failed: %w", err)
	}
	return nil
}

// UpdateBatchQueueMetadata updates batch task queue title, role, agent mode, and concurrency
func (db *DB) UpdateBatchQueueMetadata(queueID, title, role, agentMode string, concurrency int, hitlPolicies ...string) error {
	if len(hitlPolicies) > 0 {
		_, err := db.Exec("UPDATE batch_task_queues SET title = ?, role = ?, agent_mode = ?, concurrency = ?, hitl_policy = ? WHERE id = ?", title, role, agentMode, concurrency, hitlPolicies[0], queueID)
		return err
	}
	_, err := db.Exec(
		"UPDATE batch_task_queues SET title = ?, role = ?, agent_mode = ?, concurrency = ? WHERE id = ?",
		title, role, agentMode, concurrency, queueID,
	)
	if err != nil {
		return fmt.Errorf("update batch task queue metadata failed: %w", err)
	}
	return nil
}

// UpdateBatchQueueSchedule updates scheduling-related fields of a batch task queue
func (db *DB) UpdateBatchQueueSchedule(queueID, scheduleMode, cronExpr string, nextRunAt *time.Time) error {
	var nextRunAtValue interface{}
	if nextRunAt != nil {
		nextRunAtValue = *nextRunAt
	}
	_, err := db.Exec(
		"UPDATE batch_task_queues SET schedule_mode = ?, cron_expr = ?, next_run_at = ? WHERE id = ?",
		scheduleMode, cronExpr, nextRunAtValue, queueID,
	)
	if err != nil {
		return fmt.Errorf("update batch task schedule config failed: %w", err)
	}
	return nil
}

// UpdateBatchQueueScheduleEnabled enables or disables automatic Cron triggering (manual "start execution" is not affected)
func (db *DB) UpdateBatchQueueScheduleEnabled(queueID string, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := db.Exec(
		"UPDATE batch_task_queues SET schedule_enabled = ? WHERE id = ?",
		v, queueID,
	)
	if err != nil {
		return fmt.Errorf("update batch task schedule switch failed: %w", err)
	}
	return nil
}

// RecordBatchQueueScheduledTriggerStart records a schedule-triggered start time and clears the schedule-layer error
func (db *DB) RecordBatchQueueScheduledTriggerStart(queueID string, at time.Time) error {
	_, err := db.Exec(
		"UPDATE batch_task_queues SET last_schedule_trigger_at = ?, last_schedule_error = NULL WHERE id = ?",
		at, queueID,
	)
	if err != nil {
		return fmt.Errorf("record schedule trigger time failed: %w", err)
	}
	return nil
}

// SetBatchQueueLastScheduleError stores the last schedule error reason (e.g. status not allowing start, reset failed)
func (db *DB) SetBatchQueueLastScheduleError(queueID, msg string) error {
	_, err := db.Exec(
		"UPDATE batch_task_queues SET last_schedule_error = ? WHERE id = ?",
		msg, queueID,
	)
	if err != nil {
		return fmt.Errorf("write schedule error info failed: %w", err)
	}
	return nil
}

// SetBatchQueueLastRunError stores a summary of sub-task failures from the most recent run (empty string clears the field)
func (db *DB) SetBatchQueueLastRunError(queueID, msg string) error {
	var v interface{}
	if strings.TrimSpace(msg) == "" {
		v = nil
	} else {
		v = msg
	}
	_, err := db.Exec(
		"UPDATE batch_task_queues SET last_run_error = ? WHERE id = ?",
		v, queueID,
	)
	if err != nil {
		return fmt.Errorf("write last run error failed: %w", err)
	}
	return nil
}

// ResetBatchQueueForRerun resets queue and task statuses for the next scheduled run
func (db *DB) ResetBatchQueueForRerun(queueID string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction failed: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		"UPDATE batch_task_queues SET status = ?, current_index = 0, started_at = NULL, completed_at = NULL, last_run_error = NULL, last_schedule_error = NULL WHERE id = ?",
		"pending", queueID,
	)
	if err != nil {
		return fmt.Errorf("reset batch task queue status failed: %w", err)
	}

	_, err = tx.Exec(
		"UPDATE batch_tasks SET status = ?, conversation_id = NULL, started_at = NULL, completed_at = NULL, error = NULL, result = NULL WHERE queue_id = ?",
		"pending", queueID,
	)
	if err != nil {
		return fmt.Errorf("reset batch task status failed: %w", err)
	}

	return tx.Commit()
}

// UpdateBatchTaskMessage updates a batch task's message
func (db *DB) UpdateBatchTaskMessage(queueID, taskID, message string) error {
	_, err := db.Exec(
		"UPDATE batch_tasks SET message = ? WHERE queue_id = ? AND id = ?",
		message, queueID, taskID,
	)
	if err != nil {
		return fmt.Errorf("update batch task message failed: %w", err)
	}
	return nil
}

// AddBatchTask adds a task to the batch task queue
func (db *DB) AddBatchTask(queueID, taskID, message string) error {
	_, err := db.Exec(
		"INSERT INTO batch_tasks (id, queue_id, message, status) VALUES (?, ?, ?, ?)",
		taskID, queueID, message, "pending",
	)
	if err != nil {
		return fmt.Errorf("add batch task failed: %w", err)
	}
	return nil
}

// CancelPendingBatchTasks bulk-cancels all pending tasks in a queue (single SQL statement)
func (db *DB) CancelPendingBatchTasks(queueID string, completedAt time.Time) error {
	_, err := db.Exec(
		"UPDATE batch_tasks SET status = ?, completed_at = ? WHERE queue_id = ? AND status = ?",
		"cancelled", completedAt, queueID, "pending",
	)
	if err != nil {
		return fmt.Errorf("bulk cancel pending tasks failed: %w", err)
	}
	return nil
}

// PrepareBatchSingleTaskRun prepares a single-task run: optionally resets the sub-task, and updates queue index and status
func (db *DB) PrepareBatchSingleTaskRun(queueID, taskID string, taskIndex int, resetTask, resumeQueue bool) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction failed: %w", err)
	}
	defer tx.Rollback()

	if resetTask {
		_, err = tx.Exec(
			"UPDATE batch_tasks SET status = ?, conversation_id = NULL, started_at = NULL, completed_at = NULL, error = NULL, result = NULL WHERE queue_id = ? AND id = ?",
			"pending", queueID, taskID,
		)
		if err != nil {
			return fmt.Errorf("reset batch task status failed: %w", err)
		}
	}

	if resumeQueue {
		_, err = tx.Exec(
			"UPDATE batch_task_queues SET status = ?, current_index = ?, completed_at = NULL, last_run_error = NULL WHERE id = ?",
			"paused", taskIndex, queueID,
		)
	} else {
		_, err = tx.Exec(
			"UPDATE batch_task_queues SET current_index = ?, last_run_error = NULL WHERE id = ?",
			taskIndex, queueID,
		)
	}
	if err != nil {
		return fmt.Errorf("update batch task queue status failed: %w", err)
	}

	return tx.Commit()
}

// DeleteBatchTask deletes a batch task
func (db *DB) DeleteBatchTask(queueID, taskID string) error {
	_, err := db.Exec(
		"DELETE FROM batch_tasks WHERE queue_id = ? AND id = ?",
		queueID, taskID,
	)
	if err != nil {
		return fmt.Errorf("delete batch task failed: %w", err)
	}
	return nil
}

// DeleteBatchQueue deletes a batch task queue
func (db *DB) DeleteBatchQueue(queueID string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction failed: %w", err)
	}
	defer tx.Rollback()

	// delete tasks (foreign key cascades automatically)
	_, err = tx.Exec("DELETE FROM batch_tasks WHERE queue_id = ?", queueID)
	if err != nil {
		return fmt.Errorf("delete batch task failed: %w", err)
	}

	// delete queue
	_, err = tx.Exec("DELETE FROM batch_task_queues WHERE id = ?", queueID)
	if err != nil {
		return fmt.Errorf("delete batch task queue failed: %w", err)
	}

	return tx.Commit()
}
