package multiagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kestrel/internal/config"

	"go.uber.org/zap"
)

func prepareLatestUserMessageForModel(userMessage string, appCfg *config.Config, mwCfg *config.MultiAgentEinoMiddlewareConfig, conversationID string, logger *zap.Logger) string {
	if strings.TrimSpace(userMessage) == "" {
		return strings.TrimSpace(userMessage)
	}
	if mwCfg == nil {
		var zero config.MultiAgentEinoMiddlewareConfig
		mwCfg = &zero
	}
	maxRunes := mwCfg.LatestUserMessageMaxRunesEffective()
	if appCfg != nil {
		maxRunes = minPositiveInt(maxRunes, modelFacingRuneBudget(appCfg.OpenAI.MaxTotalTokens, 0.20))
	}
	if maxRunes <= 0 || utf8RuneLen(userMessage) <= maxRunes {
		return userMessage
	}

	headRunes, tailRunes := normalizeLatestUserPreviewBudget(
		maxRunes,
		mwCfg.LatestUserMessageHeadRunesEffective(),
		mwCfg.LatestUserMessageTailRunesEffective(),
	)
	head, tail := splitHeadTailRunes(userMessage, headRunes, tailRunes)
	artifactPath, writeErr := persistLatestUserMessageArtifact(userMessage, appCfg, conversationID)
	if writeErr != nil && logger != nil {
		logger.Warn("latest user message artifact write failed, will use preview only",
			zap.String("conversationId", conversationID),
			zap.Error(writeErr),
		)
	}

	var sb strings.Builder
	sb.WriteString("[system prompt: the user input for this round is too long; a trimmed preview has been generated for the model context.]\n")
	sb.WriteString("The original user input has been fully saved to the database (messages.role=user).")
	if artifactPath != "" {
		sb.WriteString(" It has also been persisted as an artifact that can be read when the full text is needed:\n")
		sb.WriteString("artifact_path: ")
		sb.WriteString(artifactPath)
		sb.WriteByte('\n')
	} else {
		sb.WriteString(" If the artifact write failed, the original message can still be retrieved from the database.\n")
	}
	sb.WriteString(fmt.Sprintf("original_runes: %d\n", utf8RuneLen(userMessage)))
	sb.WriteString(fmt.Sprintf("preview_head_runes: %d\n", utf8RuneLen(head)))
	sb.WriteString(fmt.Sprintf("preview_tail_runes: %d\n\n", utf8RuneLen(tail)))
	sb.WriteString("Please prioritise the following preview to understand the user's intent; if the full text is required, read the artifact or the original database message.\n\n")
	sb.WriteString("<latest_user_message_preview_head>\n")
	sb.WriteString(head)
	sb.WriteString("\n</latest_user_message_preview_head>\n")
	if tail != "" {
		sb.WriteString("\n<latest_user_message_preview_tail>\n")
		sb.WriteString(tail)
		sb.WriteString("\n</latest_user_message_preview_tail>\n")
	}
	return strings.TrimSpace(sb.String())
}

func modelFacingRuneBudget(maxTotalTokens int, ratio float64) int {
	if maxTotalTokens <= 0 {
		maxTotalTokens = 120000
	}
	if ratio <= 0 || ratio >= 1 {
		ratio = 0.20
	}
	budget := int(float64(maxTotalTokens) * ratio)
	if budget < 1024 {
		budget = 1024
	}
	return budget
}

func minPositiveInt(a, b int) int {
	if a <= 0 {
		return b
	}
	if b <= 0 || a < b {
		return a
	}
	return b
}

func normalizeLatestUserPreviewBudget(maxRunes, headRunes, tailRunes int) (int, int) {
	if maxRunes <= 0 {
		return headRunes, tailRunes
	}
	if headRunes <= 0 && tailRunes <= 0 {
		headRunes = maxRunes / 2
		tailRunes = maxRunes - headRunes
	}
	if headRunes < 0 {
		headRunes = 0
	}
	if tailRunes < 0 {
		tailRunes = 0
	}
	if headRunes+tailRunes <= maxRunes {
		return headRunes, tailRunes
	}
	if headRunes == 0 {
		return 0, maxRunes
	}
	if tailRunes == 0 {
		return maxRunes, 0
	}
	head := maxRunes / 2
	tail := maxRunes - head
	return head, tail
}

func splitHeadTailRunes(s string, headRunes, tailRunes int) (string, string) {
	runes := []rune(s)
	n := len(runes)
	if headRunes > n {
		headRunes = n
	}
	head := string(runes[:headRunes])
	if tailRunes <= 0 || headRunes >= n {
		return head, ""
	}
	if tailRunes > n-headRunes {
		tailRunes = n - headRunes
	}
	tail := string(runes[n-tailRunes:])
	return head, tail
}

func persistLatestUserMessageArtifact(content string, appCfg *config.Config, conversationID string) (string, error) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		conversationID = "unknown"
	}
	baseRoot := filepath.Join(os.TempDir(), "kestrel-user-inputs")
	if appCfg != nil {
		if dbPath := strings.TrimSpace(appCfg.Database.Path); dbPath != "" {
			baseRoot = filepath.Join(filepath.Dir(dbPath), "conversation_artifacts")
		}
	}
	if abs, err := filepath.Abs(baseRoot); err == nil {
		baseRoot = abs
	}
	dir := filepath.Join(baseRoot, sanitizeEinoPathSegment(conversationID), "user_inputs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir user input artifact dir: %w", err)
	}
	name := "latest_user_" + time.Now().UTC().Format("20060102T150405.000000000Z") + ".txt"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write user input artifact: %w", err)
	}
	return path, nil
}
