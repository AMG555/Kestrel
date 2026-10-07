package multiagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"kestrel/internal/agent"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
)

// --- buildUserContextSupplement tests ---

func TestBuildUserContextSupplement_SingleMessage(t *testing.T) {
	result := buildUserContextSupplement("http://8.163.32.73:8081 testCommand execution", nil, 0)
	if result == "" {
		t.Fatal("expected non-empty supplement")
	}
	if !strings.Contains(result, "http://8.163.32.73:8081") {
		t.Error("expected URL in supplement")
	}
}

func TestBuildUserContextSupplement_MultiTurn(t *testing.T) {
	history := []agent.ChatMessage{
		{Role: "user", Content: "http://8.163.32.73:8081 is a pikachu target range, try testing Command execution"},
		{Role: "assistant", Content: "OK, let me test..."},
		{Role: "user", Content: "continue, and persist the webshell"},
		{Role: "assistant", Content: "processing..."},
	}
	result := buildUserContextSupplement("hello", history, 0)
	if !strings.Contains(result, "http://8.163.32.73:8081") {
		t.Error("expected first turn URL to be preserved")
	}
	if !strings.Contains(result, "hello") {
		t.Error("expected current message")
	}
}

func TestBuildUserContextSupplement_Empty(t *testing.T) {
	if result := buildUserContextSupplement("", nil, 0); result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}

func TestBuildUserContextSupplement_Deduplicate(t *testing.T) {
	history := []agent.ChatMessage{{Role: "user", Content: "hello"}}
	result := buildUserContextSupplement("hello", history, 0)
	if strings.Count(result, "hello") != 1 {
		t.Errorf("expected 'hello' once, got: %s", result)
	}
}

func TestBuildUserContextSupplement_SkipsNonUser(t *testing.T) {
	history := []agent.ChatMessage{
		{Role: "user", Content: "target is 10.0.0.1"},
		{Role: "assistant", Content: "should not appear"},
	}
	result := buildUserContextSupplement("confirm", history, 0)
	if strings.Contains(result, "should not appear") {
		t.Error("assistant message should not be included")
	}
}

func TestBuildUserContextSupplement_DisabledByNegative(t *testing.T) {
	if result := buildUserContextSupplement("test", nil, -1); result != "" {
		t.Errorf("expected empty when disabled, got %q", result)
	}
}

func TestBuildUserContextSupplement_CustomMaxRunes(t *testing.T) {
	msg := strings.Repeat("A", 200)
	result := buildUserContextSupplement(msg, nil, 50)
	header := userContextSupplementHeader
	body := strings.TrimPrefix(result, header)
	if len([]rune(body)) > 50 {
		t.Errorf("body should be capped at 50 runes, got %d", len([]rune(body)))
	}
}

func TestBuildUserContextSupplement_TruncateKeepsFirstAndLast(t *testing.T) {
	first := "http://target.com " + strings.Repeat("A", 500)
	var history []agent.ChatMessage
	history = append(history, agent.ChatMessage{Role: "user", Content: first})
	for i := 0; i < 10; i++ {
		history = append(history, agent.ChatMessage{Role: "user", Content: strings.Repeat("B", 500)})
	}
	last := "last instruction"
	result := buildUserContextSupplement(last, history, 800)
	if !strings.Contains(result, "http://target.com") {
		t.Error("first message (target URL) should survive truncation")
	}
	if !strings.Contains(result, last) {
		t.Error("last message should survive truncation")
	}
}

// --- middleware integration tests ---

func TestTaskContextEnrichMiddleware_EnrichesTaskDescription(t *testing.T) {
	mw := newTaskContextEnrichMiddleware(
		"continuetest",
		[]agent.ChatMessage{{Role: "user", Content: "http://8.163.32.73:8081 pikachu target range"}},
		0,
		"",
	)
	if mw == nil {
		t.Fatal("expected non-nil middleware")
	}

	called := false
	var capturedArgs string
	fakeEndpoint := func(ctx context.Context, args string, opts ...tool.Option) (string, error) {
		called = true
		capturedArgs = args
		return "ok", nil
	}

	wrapped, err := mw.(interface {
		WrapInvokableToolCall(context.Context, adk.InvokableToolCallEndpoint, *adk.ToolContext) (adk.InvokableToolCallEndpoint, error)
	}).WrapInvokableToolCall(context.Background(), fakeEndpoint, &adk.ToolContext{Name: "task"})
	if err != nil {
		t.Fatal(err)
	}

	taskArgs := `{"subagent_type":"recon","description":"scan target ports"}`
	wrapped(context.Background(), taskArgs)

	if !called {
		t.Fatal("endpoint was not called")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(capturedArgs), &parsed); err != nil {
		t.Fatalf("enriched args not valid JSON: %v", err)
	}
	desc := parsed["description"].(string)
	if !strings.Contains(desc, "scan target ports") {
		t.Error("original description should be preserved")
	}
	if !strings.Contains(desc, "http://8.163.32.73:8081") {
		t.Error("user context should be appended to description")
	}
	if !strings.Contains(desc, "continuetest") {
		t.Error("current user message should be in description")
	}
}

func TestTaskContextEnrichMiddleware_IgnoresNonTaskTools(t *testing.T) {
	mw := newTaskContextEnrichMiddleware("test", nil, 0, "")
	if mw == nil {
		t.Fatal("expected non-nil middleware")
	}

	original := `{"command":"nmap -sV target"}`
	var capturedArgs string
	fakeEndpoint := func(ctx context.Context, args string, opts ...tool.Option) (string, error) {
		capturedArgs = args
		return "ok", nil
	}

	wrapped, err := mw.(interface {
		WrapInvokableToolCall(context.Context, adk.InvokableToolCallEndpoint, *adk.ToolContext) (adk.InvokableToolCallEndpoint, error)
	}).WrapInvokableToolCall(context.Background(), fakeEndpoint, &adk.ToolContext{Name: "nmap_scan"})
	if err != nil {
		t.Fatal(err)
	}

	wrapped(context.Background(), original)
	if capturedArgs != original {
		t.Errorf("non-task tool args should not be modified, got %q", capturedArgs)
	}
}

func TestTaskContextEnrichMiddleware_NilWhenDisabled(t *testing.T) {
	mw := newTaskContextEnrichMiddleware("test", nil, -1, "")
	if mw != nil {
		t.Error("middleware should be nil when disabled")
	}
}
