package handler

import (
	"context"
	"fmt"
	"time"

	"kestrel/internal/agent"
	"kestrel/internal/config"
	"kestrel/internal/multiagent"

	"go.uber.org/zap"
)

// rebindEinoRunningTask interrupts and continues / empty-body re-runs: rebuilds the cancel chain and timeout ctx, keeping the task running.
func (h *AgentHandler) rebindEinoRunningTask(parent context.Context, conversationID string, timeoutCancel context.CancelFunc) (context.Context, context.CancelCauseFunc, context.Context, context.CancelFunc) {
	if timeoutCancel != nil {
		timeoutCancel()
	}
	baseCtx, cancelWithCause := context.WithCancelCause(detachedAgentContext(parent))
	h.tasks.BindTaskCancel(conversationID, cancelWithCause)
	taskCtx, newTimeoutCancel := context.WithTimeout(baseCtx, 600*time.Minute)
	h.tasks.UpdateTaskStatus(conversationID, "running")
	return baseCtx, cancelWithCause, taskCtx, newTimeoutCancel
}

// tryContinueOnEinoEmptyResponse backs off and re-runs when a Run succeeds but the Response is empty; returns true if the next Run segment is ready.
func (h *AgentHandler) tryContinueOnEinoEmptyResponse(
	taskCtx context.Context,
	mw *config.MultiAgentEinoMiddlewareConfig,
	conversationID string,
	result *multiagent.RunResult,
	attempt *int,
	curHistory *[]agent.ChatMessage,
	curFinalMessage *string,
	progressCallback func(eventType, message string, data interface{}),
) bool {
	if result == nil || !multiagent.IsEinoEmptyResponseResult(result) || !multiagent.HasEinoResumeTrace(result) {
		return false
	}
	maxAttempts := multiagent.EmptyResponseContinueMaxAttemptsFromConfig(mw)
	if *attempt >= maxAttempts {
		if h.logger != nil {
			h.logger.Warn("eino empty response continue exhausted",
				zap.String("conversationId", conversationID),
				zap.Int("maxAttempts", maxAttempts))
		}
		return false
	}
	*attempt++
	h.persistEinoAgentTraceForResume(conversationID, result)

	backoff := multiagent.EmptyResponseContinueBackoff(*attempt-1, mw)
	waitMsg := fmt.Sprintf("conversation ended but no assistant body was captured, auto-continuing attempt %d/%d in %d seconds…",
		int(backoff.Seconds()), *attempt, maxAttempts)
	if progressCallback != nil {
		progressCallback("eino_empty_response_continue", waitMsg, map[string]interface{}{
			"conversationId": conversationID,
			"source":         "eino",
			"attempt":        *attempt,
			"maxAttempts":    maxAttempts,
			"backoffSec":     int(backoff.Seconds()),
		})
	}
	select {
	case <-taskCtx.Done():
		return false
	case <-time.After(backoff):
	}

	h.applyEinoTraceResumeSegment(conversationID, result, curHistory, curFinalMessage, "")
	if progressCallback != nil {
		progressCallback("eino_empty_response_continue", "context resumed, continuing run…", map[string]interface{}{
			"conversationId":   conversationID,
			"source":           "eino",
			"attempt":          *attempt,
			"maxAttempts":      maxAttempts,
			"contextSource":    "empty_response_continue",
			"contextInjection": false,
		})
	}
	return true
}
