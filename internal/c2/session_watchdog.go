package c2

import (
	"context"
	"time"

	"kestrel/internal/database"

	"go.uber.org/zap"
)

// SessionWatchdog is a session heartbeat watchdog that periodically scans all active/sleeping sessions
// and marks those that have not sent a heartbeat within (sleep * (1 + jitter%) * graceFactor + minGrace) as dead.
//
// Design notes:
//   - Single goroutine + ticker, avoids opening a timer per session; scales linearly even with many sessions;
//   - Threshold adapts to each session's own sleep/jitter (a session with sleep=300s cannot use sleep=5s thresholds);
//   - Global minimum grace period minGrace prevents false positives from mis-configured sleep sessions;
//   - Does not read implant_uuid; relies purely on the last_check_in field, decoupled from listener type.
type SessionWatchdog struct {
	manager  *Manager
	logger   *zap.Logger
	interval time.Duration // scan interval, default 15s
	minGrace time.Duration // minimum grace period, default 30s
	gracePct float64       // heartbeat timeout multiplier, default 3.0 (3× sleep period without heartbeat = offline)
	stopCh   chan struct{}
}

// NewSessionWatchdog createwatchdog
func NewSessionWatchdog(m *Manager) *SessionWatchdog {
	return &SessionWatchdog{
		manager:  m,
		logger:   m.Logger().With(zap.String("component", "c2-watchdog")),
		interval: 15 * time.Second,
		minGrace: 30 * time.Second,
		gracePct: 3.0,
		stopCh:   make(chan struct{}),
	}
}

// Run blocks until ctx.Done() or Stop() is called.
func (w *SessionWatchdog) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-t.C:
			w.tick()
		}
	}
}

// Stop stop
func (w *SessionWatchdog) Stop() {
	select {
	case <-w.stopCh:
	default:
		close(w.stopCh)
	}
}

func (w *SessionWatchdog) tick() {
	now := time.Now()
	for _, status := range []string{string(SessionActive), string(SessionSleeping)} {
		sessions, err := w.manager.DB().ListC2Sessions(database.ListC2SessionsFilter{Status: status})
		if err != nil {
			w.logger.Warn("watchdog listquery failed", zap.Error(err))
			continue
		}
		for _, s := range sessions {
			if w.isStale(s, now) {
				if err := w.manager.MarkSessionDead(s.ID); err != nil {
					w.logger.Warn("failed to mark session as dead", zap.String("session_id", s.ID), zap.Error(err))
				}
			}
		}
	}
	sessions, err := w.manager.DB().ListC2Sessions(database.ListC2SessionsFilter{Status: string(SessionDead)})
	if err != nil {
		w.logger.Warn("offlinetaskquery failed", zap.Error(err))
		return
	}
	for _, session := range sessions {
		w.expireOfflineCommands(session, now)
	}
}

func (w *SessionWatchdog) expireOfflineCommands(session *database.C2Session, now time.Time) {
	tasks, err := w.manager.DB().ListC2Tasks(database.ListC2TasksFilter{SessionID: session.ID, Status: string(TaskSent)})
	if err != nil {
		w.logger.Warn("failed to query offline commands", zap.Error(err))
		return
	}
	for _, task := range tasks {
		if task.TaskType != string(TaskTypeExec) && task.TaskType != string(TaskTypeShell) {
			continue
		}
		if task.SentAt == nil {
			continue
		}
		seconds := 60.0
		if value, ok := task.Payload["timeout_seconds"].(float64); ok && value > 0 {
			seconds = value
		}
		// Invalid huge deadlines remain unexpired rather than wrapping a duration.
		if seconds > float64((1<<63-1)/int64(time.Second)) {
			continue
		}
		if now.Sub(*task.SentAt) <= time.Duration(seconds)*time.Second+w.minGrace {
			continue
		}
		status := string(TaskFailed)
		errText := "session is offline and task result has exceeded the execution deadline; it is unconfirmed whether the remote process has terminated"
		if err := w.manager.DB().UpdateC2Task(task.ID, database.C2TaskUpdate{ExpectedStatus: &task.Status, Status: &status, Error: &errText, CompletedAt: &now}); err != nil {
			w.logger.Warn("failed to finalize offline command", zap.Error(err))
			continue
		}
		w.manager.publishEvent("warn", "task", session.ID, task.ID, errText, nil)
	}
}

// isStale determines whether a session has timed out.
func (w *SessionWatchdog) isStale(s *database.C2Session, now time.Time) bool {
	// No heartbeat on record: fall back to first_seen_at
	last := s.LastCheckIn
	if last.IsZero() {
		last = s.FirstSeenAt
	}
	sleep := s.SleepSeconds
	if sleep <= 0 {
		// TCP reverse pattern sleep=0 → use minimum grace period for determination
		return now.Sub(last) > w.minGrace*2
	}
	jitter := s.JitterPercent
	if jitter < 0 {
		jitter = 0
	}
	if jitter > 100 {
		jitter = 100
	}
	// Threshold = sleep * (1 + jitter%) * gracePct, plus minGrace as a floor
	expected := time.Duration(float64(sleep)*(1+float64(jitter)/100.0)*w.gracePct) * time.Second
	if expected < w.minGrace {
		expected = w.minGrace
	}
	return now.Sub(last) > expected
}
