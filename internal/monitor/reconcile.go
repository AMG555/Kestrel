package monitor

import (
	"context"
	"time"

	"go.uber.org/zap"
	"kestrel/internal/database"
)

const (
	defaultStaleRunningAge   = 5 * time.Minute
	defaultReconcileInterval = 2 * time.Minute
)

// ExecutionReconciler reconciles orphaned and stale running tool executions.
type ExecutionReconciler struct {
	db     *database.DB
	logger *zap.Logger
}

// NewExecutionReconciler creates a reconciler for tool executions.
func NewExecutionReconciler(db *database.DB, logger *zap.Logger) *ExecutionReconciler {
	return &ExecutionReconciler{
		db:     db,
		logger: logger,
	}
}

// ReconcileOnStartup marks all persisted running records as orphaned upon server restart.
func (r *ExecutionReconciler) ReconcileOnStartup() int64 {
	if r == nil || r.db == nil {
		return 0
	}
	now := time.Now().UTC()
	n, err := r.db.CancelOrphanedRunningToolExecutions(now, "execution interrupted by server restart")
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("Failed to reconcile orphaned running tool executions on startup", zap.Error(err))
		}
		return 0
	}
	if n > 0 && r.logger != nil {
		r.logger.Info("Reconciled orphaned running tool executions on startup", zap.Int64("count", n))
	}
	return n
}

// ReconcileStaleRunning marks executions running longer than staleAge as orphaned.
func (r *ExecutionReconciler) ReconcileStaleRunning(staleAge time.Duration) int64 {
	if r == nil || r.db == nil {
		return 0
	}
	if staleAge <= 0 {
		staleAge = defaultStaleRunningAge
	}
	cutoff := time.Now().UTC().Add(-staleAge)
	n, err := r.db.CancelOrphanedRunningToolExecutions(cutoff, "execution timed out / worker detached")
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("Failed to reconcile stale running tool executions", zap.Error(err))
		}
		return 0
	}
	if n > 0 && r.logger != nil {
		r.logger.Info("Reconciled stale running tool executions", zap.Int64("count", n))
	}
	return n
}

// StartReconcileLoop periodically cleans up stale running tool executions.
func (r *ExecutionReconciler) StartReconcileLoop(ctx context.Context, interval time.Duration) {
	if r == nil {
		return
	}
	if interval <= 0 {
		interval = defaultReconcileInterval
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.ReconcileStaleRunning(defaultStaleRunningAge)
			}
		}
	}()
}
