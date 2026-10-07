package handler

import (
	"strings"
)

type hitlCognitionState struct {
	AssistantMessageID string
	UserMessage        string
	Thinking           string
	ReasoningChain     string
	Planning           string
}

// GetHitlCognition returns the cached HITL context for the current running task round (excluding conversation history).
func (m *AgentTaskManager) GetHitlCognition(conversationID string) hitlCognitionFields {
	conversationID = strings.TrimSpace(conversationID)
	if m == nil || conversationID == "" {
		return hitlCognitionFields{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[conversationID]
	if !ok || t == nil || t.hitlCognition == nil {
		return hitlCognitionFields{}
	}
	c := t.hitlCognition
	return hitlCognitionFields{
		UserMessage:    c.UserMessage,
		Thinking:       c.Thinking,
		ReasoningChain: c.ReasoningChain,
		Planning:       c.Planning,
	}
}

// ResetHitlCognition resets the HITL context when a new task starts.
func (m *AgentTaskManager) ResetHitlCognition(conversationID, userMessage string) {
	conversationID = strings.TrimSpace(conversationID)
	if m == nil || conversationID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[conversationID]
	if !ok || t == nil {
		return
	}
	t.hitlCognition = &hitlCognitionState{UserMessage: strings.TrimSpace(userMessage)}
}

// SetHitlAssistantMessageID records the current assistant message ID for HITL and DB rollback alignment.
func (m *AgentTaskManager) SetHitlAssistantMessageID(conversationID, assistantMessageID string) {
	conversationID = strings.TrimSpace(conversationID)
	assistantMessageID = strings.TrimSpace(assistantMessageID)
	if m == nil || conversationID == "" || assistantMessageID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[conversationID]
	if !ok || t == nil {
		return
	}
	if t.hitlCognition == nil {
		t.hitlCognition = &hitlCognitionState{}
	}
	t.hitlCognition.AssistantMessageID = assistantMessageID
}

// UpdateHitlCognitionSnapshot updates thinking/reasoning/planning from the in-progress stream snapshot.
func (m *AgentTaskManager) UpdateHitlCognitionSnapshot(conversationID, assistantMessageID, thinking, reasoningChain, planning string) {
	conversationID = strings.TrimSpace(conversationID)
	if m == nil || conversationID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[conversationID]
	if !ok || t == nil {
		return
	}
	if t.hitlCognition == nil {
		t.hitlCognition = &hitlCognitionState{}
	}
	if id := strings.TrimSpace(assistantMessageID); id != "" {
		t.hitlCognition.AssistantMessageID = id
	}
	if s := strings.TrimSpace(thinking); s != "" {
		t.hitlCognition.Thinking = s
	}
	if s := strings.TrimSpace(reasoningChain); s != "" {
		t.hitlCognition.ReasoningChain = s
	}
	if s := strings.TrimSpace(planning); s != "" {
		t.hitlCognition.Planning = s
	}
}
