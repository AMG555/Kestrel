package multiagent

import "testing"

func TestUnwrapPlanExecuteUserText(t *testing.T) {
	raw := `{"response": "Hello! Nice to meet you."}`
	if got := UnwrapPlanExecuteUserText(raw); got != "Hello! Nice to meet you." {
		t.Fatalf("got %q", got)
	}
	if got := UnwrapPlanExecuteUserText("plain"); got != "plain" {
		t.Fatalf("got %q", got)
	}
	steps := `{"steps":["a","b"]}`
	if got := UnwrapPlanExecuteUserText(steps); got != steps {
		t.Fatalf("expected unchanged steps json, got %q", got)
	}
}
