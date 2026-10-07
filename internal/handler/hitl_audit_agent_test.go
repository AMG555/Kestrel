package handler

import (
	"context"
	"strings"
	"testing"

	"kestrel/internal/config"
)

func TestParseAuditAgentLLMContentApprove(t *testing.T) {
	d, err := parseAuditAgentLLMContent(`{"decision":"approve","comment":"consistent with task"}`)
	if err != nil {
		t.Fatal(err)
	}
	if d.Decision != "approve" || d.Comment != "consistent with task" {
		t.Fatalf("unexpected %+v", d)
	}
}

func TestParseAuditAgentLLMContentReject(t *testing.T) {
	d, err := parseAuditAgentLLMContent("```json\n{\"decision\":\"reject\",\"comment\":\"risk too high\"}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if d.Decision != "reject" {
		t.Fatalf("expected reject, got %s", d.Decision)
	}
}

func TestParseAuditAgentLLMContentInvalid(t *testing.T) {
	_, err := parseAuditAgentLLMContent(`{"decision":"maybe"}`)
	if err == nil {
		t.Fatal("expected error for invalid decision")
	}
}

func TestParseAuditAgentLLMContentProseWrapped(t *testing.T) {
	d, err := parseAuditAgentLLMContent("Okay, ruling as follows:\n```json\n{\"decision\":\"approve\",\"comment\":\"read-only ls\"}\n```\nThat is all.")
	if err != nil {
		t.Fatal(err)
	}
	if d.Decision != "approve" {
		t.Fatalf("expected approve, got %s", d.Decision)
	}
}

func TestParseAuditAgentLLMContentAlternateApproveDecision(t *testing.T) {
	d, err := parseAuditAgentLLMContent(`{"decision":"approve","comment":"low risk"}`)
	if err != nil {
		t.Fatal(err)
	}
	if d.Decision != "approve" {
		t.Fatalf("expected approve, got %s", d.Decision)
	}
}

func TestParseAuditAgentLLMContentWithEditedArguments(t *testing.T) {
	d, err := parseAuditAgentLLMContent(`{"decision":"approve","comment":"narrowed path","editedArguments":{"path":"/safe"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if d.Decision != "approve" {
		t.Fatalf("expected approve, got %s", d.Decision)
	}
	if d.EditedArguments == nil || d.EditedArguments["path"] != "/safe" {
		t.Fatalf("unexpected edited args: %+v", d.EditedArguments)
	}
}

func TestAuditAgentReviewTypeSafeMissingAPIKey(t *testing.T) {
	h := &AgentHandler{config: &config.Config{Hitl: config.HitlConfig{AuditBackend: "typesafe"}}}
	d := h.auditAgentReview(context.Background(), "approval", "exec", nil)
	if d.Decision != "reject" {
		t.Fatalf("decision=%s", d.Decision)
	}
	if !strings.Contains(d.Comment, "TypeSafe API Key") {
		t.Fatalf("comment=%s", d.Comment)
	}
}

func TestBuildAuditAgentReviewInputIncludesMode(t *testing.T) {
	s := buildAuditAgentReviewInput("review_edit", "execute", map[string]interface{}{
		"arguments": `{"command":"pwd"}`,
	})
	if !strings.Contains(s, "review_edit") || !strings.Contains(s, "execute") {
		t.Fatalf("unexpected input: %s", s)
	}
}

func TestBuildAuditAgentReviewInput(t *testing.T) {
	s := buildAuditAgentReviewInput("approval", "nmap", map[string]interface{}{
		"arguments":   `{"target":"10.0.0.1"}`,
		"userMessage": "scan internal network",
	})
	if s == "" {
		t.Fatal("expected non-empty input")
	}
	if !strings.Contains(s, "nmap") || !strings.Contains(s, "10.0.0.1") || !strings.Contains(s, "scan internal network") {
		t.Fatalf("unexpected input: %s", s)
	}
}
