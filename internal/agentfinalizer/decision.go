// Package agentfinalizer governs agent execution completion, ensuring runs do not prematurely exit without evidence.
package agentfinalizer

import (
	"strings"
)

const (
	StatusCompleted    = "completed"
	StatusInProgress   = "in_progress"
	StatusBlocked      = "blocked"
	StatusFailed       = "failed"
	StatusCancelled    = "cancelled"
	StatusAwaitingHITL = "awaiting_hitl"

	ReasonVerified        = "verified"
	ReasonPendingTools    = "pending_tool_executions"
	ReasonEmptyResponse   = "empty_response"
	ReasonAwaitingHITL    = "awaiting_hitl"
	ReasonFailed          = "failed"
	ReasonCancelled       = "cancelled"
	ReasonMissingEvidence = "missing_execution_evidence"
)

// Decision defines whether an agent run is ready to finalize.
type Decision struct {
	Status              string   `json:"status"`
	Finalizable         bool     `json:"finalizable"`
	Finalized           bool     `json:"finalized"`
	CompletionReason    string   `json:"completion_reason"`
	FinalText           string   `json:"final_text,omitempty"`
	EvidenceVerified    bool     `json:"evidence_verified"`
	EvidenceRefs        []string `json:"evidence_refs,omitempty"`
	PendingExecutionIDs []string `json:"pending_execution_ids,omitempty"`
	MissingChecks       []string `json:"missing_checks,omitempty"`
	AgentMode           string   `json:"agent_mode,omitempty"`
}

// EvaluateRun evaluates the current state of an agent execution turn.
func EvaluateRun(agentMode, answer string, pendingExecIDs []string, hasHITLBlock bool, toolCount int) Decision {
	trimmed := strings.TrimSpace(answer)

	if hasHITLBlock {
		return Decision{
			Status:           StatusAwaitingHITL,
			Finalizable:      false,
			CompletionReason: ReasonAwaitingHITL,
			AgentMode:        agentMode,
		}
	}

	if len(pendingExecIDs) > 0 {
		return Decision{
			Status:              StatusInProgress,
			Finalizable:         false,
			CompletionReason:    ReasonPendingTools,
			PendingExecutionIDs: pendingExecIDs,
			AgentMode:           agentMode,
		}
	}

	if trimmed == "" && toolCount == 0 {
		return Decision{
			Status:           StatusInProgress,
			Finalizable:      false,
			CompletionReason: ReasonEmptyResponse,
			AgentMode:        agentMode,
		}
	}

	// Normal completion
	return Decision{
		Status:           StatusCompleted,
		Finalizable:      true,
		Finalized:        true,
		CompletionReason: ReasonVerified,
		FinalText:        trimmed,
		EvidenceVerified: toolCount > 0,
		AgentMode:        agentMode,
	}
}
