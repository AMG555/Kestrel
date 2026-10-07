package multiagent

import (
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// orphanToolPrunerMiddleware prunes orphan tool messages (those without a corresponding assistant(tool_calls)) before each ChatModel call.
//
// Background:
//   - The eino summarization middleware, after triggering a summary, replaces all non-system messages with 1 summary message by default;
//     this project uses a custom Finalize (summarizeFinalizeWithRecentAssistantToolTrail) to backfill
//     the most recent assistant/tool trail after the summary. If Finalize's retention policy truncates
//     by count rather than round alignment, it may retain tool results whose corresponding
//     assistant(tool_calls) falls before the summary, creating orphan tool messages.
//   - Similarly, reduction / tool_search / custom checkpoint-resume or any logic that rewrites history may break
//     tool_call ↔ tool_result pairing.
//
// Once orphan tool messages reach ChatModel, OpenAI-compatible APIs (including DashScope / various proxies) return
// 400 "No tool call found for function call output with call_id ...", which Eino wraps into
// a [NodeRunError], terminating the entire round of orchestration.
//
// Design trade-offs:
//   - The official patchtoolcalls middleware only fills the reverse direction (assistant(tc) missing tool_result) and does not handle orphan tools.
//     This middleware is complementary, handling forward orphans as a safety net.
//   - Only messages are removed; no fabricated assistant(tc) is injected into history: fabricated tool_calls would mislead the model's subsequent reasoning.
//     The summary already covers the semantics of the trimmed segment; dropping one raw tool result has minimal impact on conversation continuity.
//   - Recommended position: attach after summarization / reduction / skill / plantask / system merge / continuation dedup,
//     and after tool_search, close to the ChatModel call end.
type orphanToolPrunerMiddleware struct {
	adk.BaseChatModelAgentMiddleware
	logger *zap.Logger
	phase  string
}

// newOrphanToolPrunerMiddleware constructs the middleware. phase is used only for log distinction between deep / supervisor /
// plan_execute_executor / sub_agent — does not affect runtime behaviour.
func newOrphanToolPrunerMiddleware(logger *zap.Logger, phase string) adk.ChatModelAgentMiddleware {
	return &orphanToolPrunerMiddleware{
		logger: logger,
		phase:  phase,
	}
}

// BeforeModelRewriteState scans the message list, collects the call_id set provided by assistant.tool_calls,
// then removes role=tool messages whose ToolCallID is not in that set.
//
// Complexity: O(N). When no orphan is found, no allocation occurs and the state is returned as-is for the upstream fast path.
func (m *orphanToolPrunerMiddleware) BeforeModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	_ = mc
	if m == nil || state == nil || len(state.Messages) == 0 {
		return ctx, state, nil
	}

	// First pass: collect all provided tool_call_ids; also fast-path check whether any orphan actually exists.
	provided := make(map[string]struct{}, 8)
	for _, msg := range state.Messages {
		if msg == nil {
			continue
		}
		if msg.Role == schema.Assistant {
			for _, tc := range msg.ToolCalls {
				if tc.ID != "" {
					provided[tc.ID] = struct{}{}
				}
			}
		}
	}

	hasOrphan := false
	for _, msg := range state.Messages {
		if msg == nil {
			continue
		}
		if msg.Role == schema.Tool && msg.ToolCallID != "" {
			if _, ok := provided[msg.ToolCallID]; !ok {
				hasOrphan = true
				break
			}
		}
	}
	if !hasOrphan {
		return ctx, state, nil
	}

	// Second pass: generate the new message list with orphans removed.
	pruned := make([]adk.Message, 0, len(state.Messages))
	droppedIDs := make([]string, 0, 2)
	droppedNames := make([]string, 0, 2)
	for _, msg := range state.Messages {
		if msg == nil {
			continue
		}
		if msg.Role == schema.Tool && msg.ToolCallID != "" {
			if _, ok := provided[msg.ToolCallID]; !ok {
				droppedIDs = append(droppedIDs, msg.ToolCallID)
				droppedNames = append(droppedNames, msg.ToolName)
				continue
			}
		}
		pruned = append(pruned, msg)
	}

	if m.logger != nil {
		m.logger.Warn("eino orphan tool messages pruned before model call",
			zap.String("phase", m.phase),
			zap.Int("dropped_count", len(droppedIDs)),
			zap.Strings("dropped_tool_call_ids", droppedIDs),
			zap.Strings("dropped_tool_names", droppedNames),
			zap.Int("messages_before", len(state.Messages)),
			zap.Int("messages_after", len(pruned)),
		)
	}

	ns := *state
	ns.Messages = pruned
	return ctx, &ns, nil
}
