package multiagent

import (
	"strings"
	"time"

	"kestrel/internal/config"
)

const defaultEmptyResponseContinueMaxAttempts = 5

// IsEinoEmptyResponseResult checks whether a Run ended with the "no assistant text captured" placeholder (not a real user-visible reply).
func IsEinoEmptyResponseResult(result *RunResult) bool {
	if result == nil {
		return false
	}
	return isEinoEmptyResponseText(result.Response)
}

func isEinoEmptyResponseText(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	return strings.Contains(s, "no assistant text was captured") ||
		strings.Contains(s, "failed to capture assistant text output")
}

// HasEinoResumeTrace returns true if the trace is non-empty, meaning there is context available for a resume run.
func HasEinoResumeTrace(result *RunResult) bool {
	if result == nil {
		return false
	}
	s := strings.TrimSpace(result.LastAgentTraceInput)
	return s != "" && s != "[]" && s != "null"
}

// EmptyResponseContinueMaxAttemptsFromConfig returns the max Handler-layer backoff retry limit when no assistant text is captured; 0 defaults to 5.
func EmptyResponseContinueMaxAttemptsFromConfig(mw *config.MultiAgentEinoMiddlewareConfig) int {
	if mw != nil && mw.EmptyResponseContinueMaxAttempts > 0 {
		return mw.EmptyResponseContinueMaxAttempts
	}
	return defaultEmptyResponseContinueMaxAttempts
}

// EmptyResponseContinueBackoff uses the same exponential backoff as run_retry (2s, 4s, 8s... capped).
func EmptyResponseContinueBackoff(attempt int, mw *config.MultiAgentEinoMiddlewareConfig) time.Duration {
	maxBackoff := defaultEinoRunRetryMaxBackoff
	if mw != nil && mw.RunRetryMaxBackoffSec > 0 {
		maxBackoff = time.Duration(mw.RunRetryMaxBackoffSec) * time.Second
	}
	return einoTransientRetryBackoff(attempt, maxBackoff)
}

// FormatEmptyResponseContinueUserMessage returns the user turn injected during system auto-continue (not persisted to the messages table as a bubble).
func FormatEmptyResponseContinueUserMessage() string {
	return strings.TrimSpace(`[System Auto-continue / Auto resume]
The previous Eino session produced no visible assistant text (possibly streaming interrupted or only completed tool calls). Please continue advancing based on the existing trace and tool results, and provide an interim summary; do not repeat completed steps.`)
}
