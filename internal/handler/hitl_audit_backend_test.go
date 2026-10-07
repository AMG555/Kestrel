package handler

import (
	"testing"

	"kestrel/internal/config"
)

func TestHitlAuditEngineInfoTypeSafe(t *testing.T) {
	h := &AgentHandler{config: &config.Config{
		OpenAI: config.OpenAIConfig{Model: "gpt-4o"},
		Hitl:   config.HitlConfig{AuditBackend: "typesafe"},
	}}
	backend, model := h.hitlAuditEngineInfo()
	if backend != config.HitlAuditBackendTypeSafe {
		t.Fatalf("backend=%q", backend)
	}
	if model != config.TypeSafeDefaultModel {
		t.Fatalf("model=%q, want %s", model, config.TypeSafeDefaultModel)
	}
}

func TestHitlAuditEngineInfoOpenAIInheritsMainModel(t *testing.T) {
	h := &AgentHandler{config: &config.Config{
		OpenAI: config.OpenAIConfig{Model: "gpt-4o-mini"},
		Hitl:   config.HitlConfig{AuditBackend: "openai"},
	}}
	backend, model := h.hitlAuditEngineInfo()
	if backend != config.HitlAuditBackendOpenAI {
		t.Fatalf("backend=%q", backend)
	}
	if model != "gpt-4o-mini" {
		t.Fatalf("model=%q", model)
	}
}

func TestHitlAuditBackendFromRecordPrefersPayload(t *testing.T) {
	backend, model := hitlAuditBackendFromRecord("audit_agent", "audit agent: actual operation: probing", `{
		"hitlApproval": {"auditBackend": "typesafe", "auditModel": "jev-latest"}
	}`)
	if backend != config.HitlAuditBackendTypeSafe || model != "jev-latest" {
		t.Fatalf("backend=%q model=%q", backend, model)
	}
}

func TestHitlAuditBackendFromRecordInfersJevComment(t *testing.T) {
	backend, _ := hitlAuditBackendFromRecord("audit_agent",
		"audit agent: no destructive rule matched, default allow; max damage score=business availability disruption 0.12; choice=approve(0.90)",
		`{}`)
	if backend != config.HitlAuditBackendTypeSafe {
		t.Fatalf("backend=%q", backend)
	}
}

func TestHitlAuditBackendFromRecordInfersOpenAIComment(t *testing.T) {
	backend, _ := hitlAuditBackendFromRecord("audit_agent",
		"audit agent: actual operation: read /etc/passwd; matched rule: A3",
		`{}`)
	if backend != config.HitlAuditBackendOpenAI {
		t.Fatalf("backend=%q", backend)
	}
}

func TestHitlAuditBackendFromRecordIgnoresHuman(t *testing.T) {
	backend, model := hitlAuditBackendFromRecord("human", "manually approved", `{"hitlApproval":{"auditBackend":"typesafe"}}`)
	if backend != "" || model != "" {
		t.Fatalf("backend=%q model=%q", backend, model)
	}
}
