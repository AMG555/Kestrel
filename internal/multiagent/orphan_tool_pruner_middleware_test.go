package multiagent

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func assistantToolCallsMsg(content string, callIDs ...string) *schema.Message {
	tcs := make([]schema.ToolCall, 0, len(callIDs))
	for _, id := range callIDs {
		tcs = append(tcs, schema.ToolCall{
			ID:   id,
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "stub_tool",
				Arguments: `{}`,
			},
		})
	}
	return schema.AssistantMessage(content, tcs)
}

func TestOrphanToolPruner_NoOpWhenPaired(t *testing.T) {
	mw := newOrphanToolPrunerMiddleware(nil, "test").(*orphanToolPrunerMiddleware)

	msgs := []adk.Message{
		schema.SystemMessage("sys"),
		schema.UserMessage("hi"),
		assistantToolCallsMsg("", "c1", "c2"),
		schema.ToolMessage("r1", "c1"),
		schema.ToolMessage("r2", "c2"),
		schema.AssistantMessage("done", nil),
	}
	in := &adk.ChatModelAgentState{Messages: msgs}

	_, out, err := mw.BeforeModelRewriteState(context.Background(), in, &adk.ModelContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil state")
	}
	if len(out.Messages) != len(msgs) {
		t.Fatalf("expected %d messages kept, got %d", len(msgs), len(out.Messages))
	}
	// Fast path: when no orphan is found, state must be returned in-place without allocating a new slice.
	if &out.Messages[0] != &msgs[0] {
		t.Fatalf("expected state to be returned as-is (same backing slice) when no orphan present")
	}
}

func TestOrphanToolPruner_DropsOrphanToolMessages(t *testing.T) {
	mw := newOrphanToolPrunerMiddleware(nil, "test").(*orphanToolPrunerMiddleware)

	msgs := []adk.Message{
		schema.SystemMessage("sys"),
		// The assistant(tc: c_old) before the summary was trimmed, but its corresponding tool result was accidentally retained.
		schema.ToolMessage("orphan result", "c_old"),
		schema.UserMessage("continue"),
		assistantToolCallsMsg("", "c_new"),
		schema.ToolMessage("r_new", "c_new"),
	}
	in := &adk.ChatModelAgentState{Messages: msgs}

	_, out, err := mw.BeforeModelRewriteState(context.Background(), in, &adk.ModelContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil state")
	}
	if len(out.Messages) != len(msgs)-1 {
		t.Fatalf("expected %d messages after pruning, got %d", len(msgs)-1, len(out.Messages))
	}
	for _, m := range out.Messages {
		if m != nil && m.Role == schema.Tool && m.ToolCallID == "c_old" {
			t.Fatalf("orphan tool message with ToolCallID=c_old should have been dropped")
		}
	}
	// The legitimately paired tool(c_new) must be retained.
	foundNew := false
	for _, m := range out.Messages {
		if m != nil && m.Role == schema.Tool && m.ToolCallID == "c_new" {
			foundNew = true
			break
		}
	}
	if !foundNew {
		t.Fatal("paired tool message (c_new) must be retained")
	}
}

func TestOrphanToolPruner_EmptyToolCallIDIsIgnored(t *testing.T) {
	// A tool message with an empty ToolCallID is extremely rare in practice but must not be mistaken for an orphan.
	// Treat it as "unverifiable, retain" to avoid accidental deletion.
	mw := newOrphanToolPrunerMiddleware(nil, "test").(*orphanToolPrunerMiddleware)

	odd := schema.ToolMessage("no_id", "")
	msgs := []adk.Message{
		schema.UserMessage("hi"),
		odd,
		schema.AssistantMessage("ok", nil),
	}
	in := &adk.ChatModelAgentState{Messages: msgs}

	_, out, err := mw.BeforeModelRewriteState(context.Background(), in, &adk.ModelContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Messages) != len(msgs) {
		t.Fatalf("empty ToolCallID tool message should be kept, got %d messages", len(out.Messages))
	}
}

func TestOrphanToolPruner_NilAndEmpty(t *testing.T) {
	mw := newOrphanToolPrunerMiddleware(nil, "test").(*orphanToolPrunerMiddleware)

	ctx := context.Background()
	// nil state
	if _, out, err := mw.BeforeModelRewriteState(ctx, nil, &adk.ModelContext{}); err != nil || out != nil {
		t.Fatalf("nil state: expected (nil,nil), got (%v,%v)", out, err)
	}
	// empty messages
	empty := &adk.ChatModelAgentState{}
	if _, out, err := mw.BeforeModelRewriteState(ctx, empty, &adk.ModelContext{}); err != nil || out != empty {
		t.Fatalf("empty messages: expected same state, got (%v,%v)", out, err)
	}
}
