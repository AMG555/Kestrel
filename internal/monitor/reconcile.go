package monitor

import (
	"time"

	"kestrel/internal/database"
	"kestrel/internal/mcp"

	"go.uber.org/zap"
)

const (
	staleRunningMinAge       = 45 * time.Second
	staleRunningReconcileGap = 2 * time.Minute
)

// ExecutionReconciler finalizes running execution records that have no corresponding goroutine,
// marking them as orphaned on startup or during normal operation.
type ExecutionReconciler struct {
	db          *database.DB
	mcpServer   *mcp.Server
	externalMgr *mcp.ExternalMCPManager
	logger      *zap.Logger
}

// NewExecutionReconciler creates a reconciler for orphaned MCP tool executions.
func NewExecutionReconciler(db *database.DB, mcpServer *mcp.Server, externalMgr *mcp.ExternalMCPManager, logger *zap.Logger) *ExecutionReconciler {
	return &ExecutionReconciler{
		db:          db,
		mcpServer:   mcpServer,
		externalMgr: externalMgr,
		logger:      logger,
	}
}

// ReconcileOnStartup marks every persisted running row as orphaned (safe right after process start).
func (r *ExecutionReconciler) ReconcileOnStartup() {
	if r == nil || r.db == nil {
		return
	}
	now := time.Now()
	n, err := r.db.CancelOrphanedRunningToolExecutions(now, "execution interrupted (service restart)")
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("failed to clean up orphaned running tool execution records on startup", zap.Error(err))
		}
		return
	}
	if n > 0 && r.logger != nil {
		r.logger.Info("orphaned running tool execution records finalized on startup", zap.Int64("count", n))
	}
}

func (r *ExecutionReconciler) activeExecutionIDs() map[string]struct{} {
	ids := make(map[string]struct{})
	if r.mcpServer != nil {
		for id := range r.mcpServer.ActiveRunningExecutionIDs() {
			ids[id] = struct{}{}
		}
	}
	if r.externalMgr != nil {
		for id := range r.externalMgr.ActiveRunningExecutionIDs() {
			ids[id] = struct{}{}
		}
	}
	return ids
}

// ReconcileStaleRunning finalizes running rows that are not tracked in-memory and older than staleRunningMinAge.
func (r *ExecutionReconciler) ReconcileStaleRunning() {
	if r == nil || r.db == nil {
		return
	}
	now := time.Now()
	n, err := r.db.FinalizeStaleRunningToolExecutions(now, staleRunningMinAge, r.activeExecutionIDs(), "execution interrupted (session ended)")
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("failed to finalize stale running tool execution records", zap.Error(err))
		}
		return
	}
	if n > 0 && r.logger != nil {
		r.logger.Info("stale running tool execution records finalized", zap.Int64("count", n))
	}
}

// StartStaleRunningReconcileLoop periodically reconciles orphaned running tool executions.
func StartStaleRunningReconcileLoop(r *ExecutionReconciler, logger *zap.Logger) {
	if r == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(staleRunningReconcileGap)
		defer ticker.Stop()
		for range ticker.C {
			r.ReconcileStaleRunning()
			if logger != nil {
				logger.Debug("monitor stale running reconcile tick completed")
			}
		}
	}()
}
