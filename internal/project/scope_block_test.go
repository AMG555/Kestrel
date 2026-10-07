package project

import (
	"strings"
	"testing"

	"kestrel/internal/database"
)

func TestBuildScopeBlock(t *testing.T) {
	proj := &database.Project{
		ID:        "proj_123",
		Name:      "Acme DevSecOps Test",
		ScopeJSON: `{"targets":["192.168.1.0/24","app.internal.acme.corp"],"exclude":["192.168.1.1","auth.internal.acme.corp"],"notes":"No DoS testing"}`,
	}

	block := BuildScopeBlock(proj)
	if !strings.Contains(block, "192.168.1.0/24") {
		t.Errorf("expected targets to be present in scope block")
	}
	if !strings.Contains(block, "auth.internal.acme.corp") {
		t.Errorf("expected exclude rule to be present in scope block")
	}
	if !strings.Contains(block, "No DoS testing") {
		t.Errorf("expected operator notes to be present in scope block")
	}
}

func TestIsTargetInScope(t *testing.T) {
	scopeJSON := `{"targets":["*.acme.corp","10.0.0.0/16"],"exclude":["prod.acme.corp","10.0.100.1"]}`

	cases := []struct {
		target string
		want   bool
	}{
		{"api.acme.corp", true},
		{"sub.stage.acme.corp", true},
		{"prod.acme.corp", false},
		{"10.0.1.50", true},
		{"10.0.100.1", false},
		{"google.com", false},
		{"172.16.0.1", false},
	}

	for _, tc := range cases {
		inScope, _ := IsTargetInScope(tc.target, scopeJSON)
		if inScope != tc.want {
			t.Errorf("IsTargetInScope(%s) = %v; want %v", tc.target, inScope, tc.want)
		}
	}
}
