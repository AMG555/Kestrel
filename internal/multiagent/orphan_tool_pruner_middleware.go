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
//     本project通过自定义 Finalize（summarizeFinalizeWithRecentAssistantToolTrail）在 summary 后回填
//     最近的 assistant/tool 轨迹。若 Finalize 的保留策略按"条数"截断而未按 round 对齐，可能保留
//     tool results while the corresponding assistant(tool_calls) falls before the summary, creating orphan tool messages.
//   - Similarly, reduction / tool_search / custom checkpoint-resume or any logic that rewrites history may break
//     tool_call ↔ tool_result 配对。
//
// 一旦孤儿 tool message进入 ChatModel，OpenAI 兼容 API（含 DashScope / 各类中转）会back
// 400 "No tool call found for function call output with call_id ...", which Eino wraps into
// a [NodeRunError], terminating the entire round of orchestration.
//
// Design trade-offs:
//   - 官方 patchtoolcalls 中间件只补反向（assistant(tc) 缺 tool_result），不处理孤儿 tool。
//     本中间件与之互补，专职兜底正向孤儿。
//   - 仅剔除message，不向历史里注入虚构 assistant(tc)：虚构 tool_calls 反而会误导model后续推理。
//     summary已覆盖被裁剪段的语义，丢一条原始 tool 结果对conversation连贯性impact最小。
//   - 位置建议：挂在 summarization / reduction / skill / plantask / system 合并 / 续聊 dedup 之后，
//     and after tool_search, close to the ChatModel call end.
type orphanToolPrunerMiddleware struct {
	adk.BaseChatModelAgentMiddleware
	logger *zap.Logger
	phase  string
}

// newOrphanToolPrunerMiddleware constructs the middleware. phase is used only for log distinction between deep / supervisor /
// plan_execute_executor / sub_agent，不impact运行时行为。
func newOrphanToolPrunerMiddleware(logger *zap.Logger, phase string) adk.ChatModelAgentMiddleware {
	return &orphanToolPrunerMiddleware{
		logger: logger,
		phase:  phase,
	}
}

// BeforeModelRewriteState scanMessage list，收集 assistant.tool_calls 提供的 call_id set，
// 再剔除掉 ToolCallID 不在该set中的 role=tool message。
//
// 复杂度：O(N)。当未Discovery孤儿时不产生任何分配，state 原样back以便上游快path。
func (m *orphanToolPrunerMiddleware) BeforeModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	_ = mc
	if m == nil || state == nil || len(state.Messages) == 0 {
		return ctx, state, nil
	}

	// 第一遍：收集所有已提供的 tool_call_id；同时快path判定yesnotrue的存在孤儿。
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
