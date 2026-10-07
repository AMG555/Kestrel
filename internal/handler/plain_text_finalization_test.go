package handler

import (
	"context"
	"path/filepath"
	"testing"

	"kestrel/internal/database"
	"kestrel/internal/multiagent"
	"go.uber.org/zap"
)

func TestRobotDeliversPlainTextWithoutToolEvidence(t *testing.T) {
	db, err := database.NewDB(filepath.Join(t.TempDir(), "robot.db"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	conv, err := db.CreateConversation("text", database.ConversationCreateMeta{})
	if err != nil {
		t.Fatal(err)
	}
	h := &AgentHandler{db: db, logger: zap.NewNop()}
	result := &multiagent.RunResult{Response: "OK"}
	text, _, err := h.finalizeRobotAgentSuccess(context.Background(), "", conv.ID, result)
	if err != nil || text != "OK" || !result.Finalized {
		t.Fatalf("text=%q finalized=%v error=%v", text, result.Finalized, err)
	}
}
