package authctx

import (
	"context"
	"testing"
)

func TestPrincipalContextPropagation(t *testing.T) {
	p := NewPrincipalWithScopes("usr_123", "alice", "project:alpha", map[string]bool{
		"tools.execute":    true,
		"findings.publish": true,
	}, map[string]string{
		"tools.execute": "project:alpha",
	})

	ctx := WithPrincipal(context.Background(), p)

	extracted, ok := PrincipalFromContext(ctx)
	if !ok {
		t.Fatalf("failed to extract principal from context")
	}

	if extracted.UserID != "usr_123" {
		t.Errorf("expected UserID usr_123, got %s", extracted.UserID)
	}
	if !extracted.HasPermission("tools.execute") {
		t.Errorf("expected permission tools.execute")
	}
	if extracted.HasPermission("admin.users") {
		t.Errorf("did not expect permission admin.users")
	}
	if extracted.ScopeFor("tools.execute") != "project:alpha" {
		t.Errorf("expected scope project:alpha, got %s", extracted.ScopeFor("tools.execute"))
	}
}
