package multiagent

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// modelFacingTraceHolder saves a snapshot of the messages "about to be sent to ChatModel" (after summarization / reduction / orphan pruning etc.),
// used for persisting last_react_input, so resume runs align with the model's post-context-compression view rather than relying solely on event-stream-appended runAccumulatedMsgs.
type modelFacingTraceHolder struct {
	mu sync.Mutex
	// msgs is a deep-copied slice to prevent framework in-place modifications from polluting the snapshot
	msgs []adk.Message
}

func newModelFacingTraceHolder() *modelFacingTraceHolder {
	return &modelFacingTraceHolder{}
}

// Snapshot returns another deep copy of the current snapshot (for serialization/persistence, to avoid holding the holder mutex for a long time).
func (h *modelFacingTraceHolder) Snapshot() []adk.Message {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return cloneADKMessagesForTrace(h.msgs)
}

func (h *modelFacingTraceHolder) storeFromState(state *adk.ChatModelAgentState) {
	if h == nil || state == nil || len(state.Messages) == 0 {
		return
	}
	cloned := cloneADKMessagesForTrace(state.Messages)
	if len(cloned) == 0 {
		return
	}
	h.mu.Lock()
	h.msgs = cloned
	h.mu.Unlock()
}

func (h *modelFacingTraceHolder) storeFromAgenticState(state *adk.TypedChatModelAgentState[*schema.AgenticMessage]) {
	if h == nil || state == nil || len(state.Messages) == 0 {
		return
	}
	cloned := cloneADKMessagesForTrace(AgenticMessagesToEino(state.Messages))
	if len(cloned) == 0 {
		return
	}
	h.mu.Lock()
	h.msgs = cloned
	h.mu.Unlock()
}

func cloneADKMessagesForTrace(msgs []adk.Message) []adk.Message {
	if len(msgs) == 0 {
		return nil
	}
	b, err := json.Marshal(msgs)
	if err != nil {
		return nil
	}
	var out []adk.Message
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

// modelFacingTraceMiddleware must be last in the Handlers chain for **BeforeModel** (after telemetry),
// at which point state.Messages is the final input for this LLM call.
type modelFacingTraceMiddleware struct {
	adk.BaseChatModelAgentMiddleware
	holder *modelFacingTraceHolder
}

func newModelFacingTraceMiddleware(holder *modelFacingTraceHolder) adk.ChatModelAgentMiddleware {
	if holder == nil {
		return nil
	}
	return &modelFacingTraceMiddleware{holder: holder}
}

func (m *modelFacingTraceMiddleware) BeforeModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	if m.holder != nil && state != nil {
		m.holder.storeFromState(state)
		captureEinoTurnHistory(ctx, state.Messages)
	}
	return ctx, state, nil
}

type agenticModelFacingTraceMiddleware struct {
	*adk.TypedBaseChatModelAgentMiddleware[*schema.AgenticMessage]
	holder *modelFacingTraceHolder
}

func newAgenticModelFacingTraceMiddleware(holder *modelFacingTraceHolder) adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage] {
	if holder == nil {
		return nil
	}
	return &agenticModelFacingTraceMiddleware{
		TypedBaseChatModelAgentMiddleware: &adk.TypedBaseChatModelAgentMiddleware[*schema.AgenticMessage]{},
		holder:                            holder,
	}
}

func (m *agenticModelFacingTraceMiddleware) BeforeModelRewriteState(
	ctx context.Context,
	state *adk.TypedChatModelAgentState[*schema.AgenticMessage],
	mc *adk.TypedModelContext[*schema.AgenticMessage],
) (context.Context, *adk.TypedChatModelAgentState[*schema.AgenticMessage], error) {
	if m.holder != nil && state != nil {
		m.holder.storeFromAgenticState(state)
		captureEinoTurnHistory(ctx, AgenticMessagesToEino(state.Messages))
	}
	return ctx, state, nil
}

// Capture completed output separately from the model-input trace: changing
// Snapshot's meaning would affect last_react_input persistence and retries.
func (m *modelFacingTraceMiddleware) AfterModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, _ *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	if state != nil {
		captureEinoTurnHistory(ctx, state.Messages)
	}
	return ctx, state, nil
}

func (m *agenticModelFacingTraceMiddleware) AfterModelRewriteState(ctx context.Context, state *adk.TypedChatModelAgentState[*schema.AgenticMessage], _ *adk.TypedModelContext[*schema.AgenticMessage]) (context.Context, *adk.TypedChatModelAgentState[*schema.AgenticMessage], error) {
	if state != nil {
		captureEinoTurnHistory(ctx, AgenticMessagesToEino(state.Messages))
	}
	return ctx, state, nil
}

func (m *modelFacingTraceMiddleware) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	if runCtx != nil {
		ctx = context.WithValue(ctx, einoTurnInstructionKey{}, runCtx.Instruction)
	}
	return ctx, runCtx, nil
}

func (m *agenticModelFacingTraceMiddleware) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	if runCtx != nil {
		ctx = context.WithValue(ctx, einoTurnInstructionKey{}, runCtx.Instruction)
	}
	return ctx, runCtx, nil
}
