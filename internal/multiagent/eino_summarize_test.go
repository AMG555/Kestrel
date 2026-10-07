package multiagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kestrel/internal/config"
	"kestrel/internal/database"
	"kestrel/internal/project"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// fixedTokenCounter counts tool messages as tokensPerToolMessage, other messages as 1.
// Used to validate the branch where a tool-round exceeding budget is entirely skipped.
func fixedTokenCounter(tokensPerToolMessage int) summarization.TokenCounterFunc {
	return func(_ context.Context, in *summarization.TokenCounterInput) (int, error) {
		total := 0
		for _, msg := range in.Messages {
			if msg == nil {
				continue
			}
			switch msg.Role {
			case schema.Tool:
				total += tokensPerToolMessage
			default:
				total++
			}
		}
		return total, nil
	}
}

// variableTokenCounter counts tool messages as len(Content) (distinguishes different-sized tool results),
// other messages as 1; assistant adds len(ToolCalls) tokens to approximate tool_calls schema overhead.
func variableTokenCounter() summarization.TokenCounterFunc {
	return func(_ context.Context, in *summarization.TokenCounterInput) (int, error) {
		total := 0
		for _, msg := range in.Messages {
			if msg == nil {
				continue
			}
			if msg.Role == schema.Tool {
				total += len(msg.Content)
				continue
			}
			total++
			total += len(msg.ToolCalls)
		}
		return total, nil
	}
}

func TestBuildBudgetedSummarizationModelInputKeepsRecentCompleteRounds(t *testing.T) {
	msgs := []adk.Message{
		schema.UserMessage("old-user"),
		schema.AssistantMessage("old-answer", nil),
		schema.UserMessage("latest-user"),
		assistantToolCallsMsg("", "call-latest"),
		schema.ToolMessage("latest-tool-result", "call-latest"),
	}
	input, dropped, err := buildBudgetedSummarizationModelInput(
		context.Background(), schema.SystemMessage("summary-system"), schema.UserMessage("summary-instruction"),
		msgs, fixedTokenCounter(2), 7, summarizationInputBudgetOpts{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if dropped == 0 {
		t.Fatal("expected older rounds to be omitted")
	}
	joined := formatSummarizationTranscript(input)
	if strings.Contains(joined, "old-user") || strings.Contains(joined, "old-answer") {
		t.Fatalf("old rounds leaked into bounded input: %s", joined)
	}
	if !strings.Contains(joined, "latest-user") || !strings.Contains(joined, "latest-tool-result") {
		t.Fatalf("latest complete rounds missing: %s", joined)
	}
	for _, msg := range input {
		if msg.Role == schema.Tool || len(msg.ToolCalls) > 0 {
			t.Fatalf("summary input must use inert plaintext history, got role=%s tool_calls=%d", msg.Role, len(msg.ToolCalls))
		}
	}
}

func TestBuildBudgetedSummarizationModelInputNeutralizesMalformedToolCallProtocol(t *testing.T) {
	call := assistantToolCallsMsg("", "broken")
	call.ToolCalls[0].Function.Arguments = `{"command":"unterminated`
	input, _, err := buildBudgetedSummarizationModelInput(
		context.Background(), schema.SystemMessage("summary-system"), schema.UserMessage("summary-instruction"),
		[]adk.Message{schema.UserMessage("run it"), call, schema.ToolMessage("parse failed", "broken")},
		einoSummarizationTokenCounter("gpt-4o"), 4096, summarizationInputBudgetOpts{},
	)
	if err != nil {
		t.Fatal(err)
	}
	joined := formatSummarizationTranscript(input)
	if !strings.Contains(joined, `unterminated`) || !strings.Contains(joined, "parse failed") {
		t.Fatalf("historical evidence missing from plaintext transcript: %s", joined)
	}
	for _, msg := range input {
		if msg.Role == schema.Tool || len(msg.ToolCalls) != 0 {
			t.Fatalf("provider-visible tool protocol leaked into summary input: %+v", msg)
		}
	}
}

func TestSplitMessagesIntoRounds_Complex(t *testing.T) {
	msgs := []adk.Message{
		schema.UserMessage("q1"),
		assistantToolCallsMsg("", "c1", "c2"),
		schema.ToolMessage("r1", "c1"),
		schema.ToolMessage("r2", "c2"),
		schema.AssistantMessage("reply1", nil),
		schema.UserMessage("q2"),
		assistantToolCallsMsg("", "c3"),
		schema.ToolMessage("r3", "c3"),
	}
	rounds := splitMessagesIntoRounds(msgs)
	// 5 rounds: user(q1) | assistant(tc:c1,c2)+tool*2 | assistant(reply1) | user(q2) | assistant(tc:c3)+tool(c3)
	if len(rounds) != 5 {
		t.Fatalf("want 5 rounds, got %d", len(rounds))
	}
	// round 1 should be a tool-round and must be paired
	r1 := rounds[1]
	if len(r1.messages) != 3 {
		t.Fatalf("rounds[1] size: want 3, got %d", len(r1.messages))
	}
	if r1.messages[0].Role != schema.Assistant || len(r1.messages[0].ToolCalls) != 2 {
		t.Fatalf("rounds[1][0] must be assistant(tc=2)")
	}
	for i := 1; i < 3; i++ {
		if r1.messages[i].Role != schema.Tool {
			t.Fatalf("rounds[1][%d] must be tool, got %s", i, r1.messages[i].Role)
		}
	}
	// last round is paired
	rLast := rounds[len(rounds)-1]
	if len(rLast.messages) != 2 {
		t.Fatalf("rounds[last] size: want 2, got %d", len(rLast.messages))
	}
	if rLast.messages[0].Role != schema.Assistant || rLast.messages[1].Role != schema.Tool {
		t.Fatalf("last round must be assistant(tc)+tool(c3)")
	}
}

func TestSplitMessagesIntoRounds_DropsOrphanTool(t *testing.T) {
	// starting directly with a tool message (orphan) — should be discarded, not form its own round.
	msgs := []adk.Message{
		schema.ToolMessage("orphan", "c_old"),
		schema.UserMessage("continue"),
		assistantToolCallsMsg("", "c_new"),
		schema.ToolMessage("r_new", "c_new"),
	}
	rounds := splitMessagesIntoRounds(msgs)
	// user(continue) | assistant(tc:c_new)+tool(c_new) → 2 rounds
	if len(rounds) != 2 {
		t.Fatalf("want 2 rounds after dropping orphan, got %d", len(rounds))
	}
	for _, r := range rounds {
		for _, m := range r.messages {
			if m.Role == schema.Tool && m.ToolCallID == "c_old" {
				t.Fatalf("orphan tool c_old must not appear in any round")
			}
		}
	}
}

func TestSplitMessagesIntoRounds_ToolBelongsToCurrentAssistantOnly(t *testing.T) {
	// two adjacent assistant(tc) messages; the second one's tool should not be attributed to the first assistant.
	msgs := []adk.Message{
		assistantToolCallsMsg("", "c1"),
		schema.ToolMessage("r1", "c1"),
		assistantToolCallsMsg("", "c2"),
		schema.ToolMessage("r2", "c2"),
	}
	rounds := splitMessagesIntoRounds(msgs)
	if len(rounds) != 2 {
		t.Fatalf("want 2 rounds, got %d", len(rounds))
	}
	if len(rounds[0].messages) != 2 || rounds[0].messages[0].ToolCalls[0].ID != "c1" {
		t.Fatalf("round[0] wrong: %+v", rounds[0].messages)
	}
	if len(rounds[1].messages) != 2 || rounds[1].messages[0].ToolCalls[0].ID != "c2" {
		t.Fatalf("round[1] wrong: %+v", rounds[1].messages)
	}
}

func TestSplitMessagesIntoRounds_ToolBelongsToWrongAssistant(t *testing.T) {
	// assistant(tc:c1) followed by a tool message with tool_call_id=c999 (which does not belong to it).
	// Splitting rule: this tool should not be joined to the first round (pairing incomplete); the round ends here.
	// And c999 has no corresponding assistant, so it should be treated as an orphan and discarded.
	msgs := []adk.Message{
		assistantToolCallsMsg("", "c1"),
		schema.ToolMessage("wrong", "c999"),
		schema.UserMessage("hi"),
	}
	rounds := splitMessagesIntoRounds(msgs)
	// assistant(tc:c1) has no corresponding tool(c1), but is not an orphan (patchtoolcalls provides a fallback);
	// it forms its own round for upstream post-processing. user(hi) forms its own round. Total: 2 rounds.
	if len(rounds) != 2 {
		t.Fatalf("want 2 rounds, got %d: %+v", len(rounds), rounds)
	}
	for _, r := range rounds {
		for _, m := range r.messages {
			if m.Role == schema.Tool && m.ToolCallID == "c999" {
				t.Fatalf("wrong-owner tool must be dropped as orphan")
			}
		}
	}
}

func TestSummarizeFinalize_KeepsToolRoundIntact(t *testing.T) {
	// Key regression test: an entire tool-round is kept, not just the tool message.
	sys := schema.SystemMessage("sys")
	summary := schema.AssistantMessage("summary_content", nil)
	msgs := []adk.Message{
		sys,
		schema.UserMessage("q1"),
		schema.AssistantMessage("reply_before_tc", nil), // filler, consumes budget
		assistantToolCallsMsg("", "c1"),
		schema.ToolMessage("r1", "c1"),
	}

	// token budget: 2 messages (1 assistant + 1 tool) exactly fits.
	// If kept by count, might first consume tool(c1) then assistant(reply) within budget, squeezing out assistant(tc:c1) and causing an orphan.
	// When kept by round, the entire tool-round is atomic — either both 2 messages are kept or neither is.
	out, err := summarizeFinalizeWithRecentAssistantToolTrail(
		context.Background(),
		msgs,
		summary,
		fixedTokenCounter(1),
		2, // budget: 2 tokens
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// must include system + summary
	if len(out) < 2 {
		t.Fatalf("output too short: %d", len(out))
	}
	if out[0].Role != schema.System || out[0].Content != "sys" {
		t.Fatalf("first message must be system sys, got %s: %q", out[0].Role, out[0].Content)
	}
	if out[1] != summary {
		t.Fatalf("second message must be summary")
	}

	// Key invariant: every retained tool message must have a corresponding assistant(tc) providing its ToolCallID in the output.
	assertNoOrphanTool(t, out)
}

func TestSummarizeFinalize_SkipsOversizedToolRoundButKeepsSmallerRound(t *testing.T) {
	// Construct two tool-rounds with significantly different sizes:
	//   c_big round tool result content="aaaaaaaaaa" (10 bytes), round token ≈ 2 (assistant+tc) + 10 = 12
	//   c_ok  round tool result content="ok" (2 bytes), round token ≈ 2 + 2 = 4
	// With budget=8, such that:
	//   - the latest c_ok round (4) fits;
	//   - the further intermediate round (assistant reply + user) also fits;
	//   - the earlier c_big round (12) does not fit and is skipped (continue), not break.
	sys := schema.SystemMessage("sys")
	summary := schema.AssistantMessage("summary_content", nil)
	msgs := []adk.Message{
		sys,
		schema.UserMessage("q1"),
		assistantToolCallsMsg("", "c_big"),
		schema.ToolMessage("aaaaaaaaaa", "c_big"),
		schema.AssistantMessage("s", nil),
		schema.UserMessage("q2"),
		assistantToolCallsMsg("", "c_ok"),
		schema.ToolMessage("ok", "c_ok"),
	}

	out, err := summarizeFinalizeWithRecentAssistantToolTrail(
		context.Background(),
		msgs,
		summary,
		variableTokenCounter(),
		8,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNoOrphanTool(t, out)

	// the entire c_big round must be discarded (neither tool nor assistant should appear)
	for _, m := range out {
		if m == nil {
			continue
		}
		if m.Role == schema.Tool && m.ToolCallID == "c_big" {
			t.Fatal("oversized tool round must be skipped: tool(c_big) leaked")
		}
		if m.Role == schema.Assistant {
			for _, tc := range m.ToolCalls {
				if tc.ID == "c_big" {
					t.Fatal("oversized tool round must be skipped: assistant(tc:c_big) leaked")
				}
			}
		}
	}

	// the latest round (c_ok) as an atomic unit must be kept in its entirety.
	foundOKTool, foundOKAsst := false, false
	for _, m := range out {
		if m == nil {
			continue
		}
		if m.Role == schema.Tool && m.ToolCallID == "c_ok" {
			foundOKTool = true
		}
		if m.Role == schema.Assistant {
			for _, tc := range m.ToolCalls {
				if tc.ID == "c_ok" {
					foundOKAsst = true
				}
			}
		}
	}
	if !foundOKTool || !foundOKAsst {
		t.Fatalf("recent tool-round (c_ok) must be retained as an atomic pair: assistantKept=%v toolKept=%v", foundOKAsst, foundOKTool)
	}
}

func TestSummarizeFinalize_BudgetZeroFallsBackToSummaryOnly(t *testing.T) {
	sys := schema.SystemMessage("sys")
	summary := schema.AssistantMessage("summary", nil)
	msgs := []adk.Message{
		sys,
		assistantToolCallsMsg("", "c1"),
		schema.ToolMessage("r1", "c1"),
	}
	out, err := summarizeFinalizeWithRecentAssistantToolTrail(
		context.Background(),
		msgs,
		summary,
		fixedTokenCounter(1),
		0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 || out[0].Role != schema.System || out[0].Content != "sys" || out[1] != summary {
		t.Fatalf("budget=0 must yield [system, summary] only, got %+v", out)
	}
}

func TestSummarizeFinalize_MergesSystemMessages(t *testing.T) {
	sys1 := schema.SystemMessage("sys1")
	sys2 := schema.SystemMessage("sys2")
	summary := schema.AssistantMessage("s", nil)
	msgs := []adk.Message{
		sys1,
		schema.UserMessage("q"),
		sys2, // atypical position, but should be captured by the system group
	}
	out, err := summarizeFinalizeWithRecentAssistantToolTrail(
		context.Background(),
		msgs,
		summary,
		fixedTokenCounter(1),
		100,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	systemCount := 0
	for _, m := range out {
		if m != nil && m.Role == schema.System {
			systemCount++
			if got := m.Content; got != "sys1\n\nsys2" {
				t.Fatalf("unexpected merged system content: %q", got)
			}
		}
	}
	if systemCount != 1 {
		t.Fatalf("want 1 merged system message, got %d", systemCount)
	}
}

func TestEinoSummarizationMiddlewareRetriesWhenSummaryModelReturnsEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	emit := false
	summaryModel := &capturingClassicChatModel{outputs: []*schema.Message{
		schema.AssistantMessage("", nil),
		{Role: schema.Assistant, Content: "<summary>Valid summary: continue validating SQL injection path</summary>", ResponseMeta: &schema.ResponseMeta{FinishReason: "stop"}},
	}}
	appCfg := &config.Config{}
	appCfg.OpenAI.Model = "gpt-4o"
	appCfg.OpenAI.MaxTotalTokens = 5000
	appCfg.Database.Path = filepath.Join(t.TempDir(), "kestrel.db")
	mwCfg := &config.MultiAgentEinoMiddlewareConfig{
		SummarizationEmitInternalEvents:  &emit,
		SummarizationOutputReserveTokens: 1024,
	}

	mw, err := newEinoSummarizationMiddleware(ctx, summaryModel, appCfg, mwCfg, "conv-empty-summary", nil, "", nil)
	if err != nil {
		t.Fatalf("newEinoSummarizationMiddleware: %v", err)
	}
	state := &adk.ChatModelAgentState{Messages: []adk.Message{
		schema.SystemMessage("system root"),
		schema.UserMessage("Authorized scope: example.com\n" + strings.Repeat("historical scan output ", 12000)),
		schema.AssistantMessage("Scope recorded", nil),
		schema.UserMessage("continue validating SQL injection path"),
	}}

	_, after, err := mw.BeforeModelRewriteState(ctx, state, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewriteState should retry instead of failing on empty summary: %v", err)
	}
	if after == nil {
		t.Fatal("after state is nil")
	}
	if summaryModel.calls < 2 {
		t.Fatalf("summary model calls=%d, want retry after empty output", summaryModel.calls)
	}
	joined := joinClassicMessageContent(after.Messages)
	for _, want := range []string{"Valid summary", "continue validating SQL injection path", "Original user input and constraint ledger"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("retried compacted context missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "local compressed summary") {
		t.Fatalf("local fallback should not be used:\n%s", joined)
	}
}

type capturingClassicChatModel struct {
	output  *schema.Message
	outputs []*schema.Message
	calls   int
}

func (m *capturingClassicChatModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.calls++
	if len(m.outputs) > 0 {
		idx := m.calls - 1
		if idx >= len(m.outputs) {
			idx = len(m.outputs) - 1
		}
		return m.outputs[idx], nil
	}
	if m.output != nil {
		return m.output, nil
	}
	return schema.AssistantMessage("classic answer", nil), nil
}

func (m *capturingClassicChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

// assertNoOrphanTool asserts that every role=tool message in the message list has a corresponding
// assistant(tool_calls) with the same ID earlier in the list; otherwise an orphan was produced (the root cause of LLM 400).
func assertNoOrphanTool(t *testing.T, msgs []adk.Message) {
	t.Helper()
	provided := make(map[string]struct{})
	for _, m := range msgs {
		if m == nil {
			continue
		}
		if m.Role == schema.Assistant {
			for _, tc := range m.ToolCalls {
				if tc.ID != "" {
					provided[tc.ID] = struct{}{}
				}
			}
		}
		if m.Role == schema.Tool && m.ToolCallID != "" {
			if _, ok := provided[m.ToolCallID]; !ok {
				t.Fatalf("orphan tool message found: ToolCallID=%q has no preceding assistant(tool_calls)", m.ToolCallID)
			}
		}
	}
}

func TestWriteSummarizationTranscript(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "summarization", "transcript.txt")
	msgs := []adk.Message{
		schema.UserMessage("scan target"),
		assistantToolCallsMsg("", "tc1"),
		schema.ToolMessage("nmap output", "tc1"),
	}
	if err := writeSummarizationTranscript(path, msgs); err != nil {
		t.Fatalf("writeSummarizationTranscript: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "Pre-compaction session record") {
		t.Fatalf("missing transcript header: %q", text)
	}
	if !strings.Contains(text, "[user]") || !strings.Contains(text, "scan target") {
		t.Fatalf("missing user section: %q", text)
	}
	if !strings.Contains(text, "tool_calls:") || !strings.Contains(text, "nmap output") {
		t.Fatalf("missing tool round: %q", text)
	}
	if !strings.Contains(text, `"name":"stub_tool"`) || !strings.Contains(text, `"arguments":"{}"`) {
		t.Fatalf("missing tool name/arguments: %q", text)
	}
	if strings.Contains(text, "tool_call_id") || strings.Contains(text, `"id":"tc1"`) {
		t.Fatalf("transcript should omit tool_call_id: %q", text)
	}
}

func TestSanitizeSystemContentForTranscript_BestPractice(t *testing.T) {
	t.Parallel()
	system := strings.Join([]string{
		"The following is the tool name index for the current session (names only, no parameter JSON Schema).",
		"- nmap",
		"- nuclei",
		"",
		"Usage rules:",
		"1) The above table is a name index only",
		"5) Do not fabricate tool names that do not exist.",
		"",
		"You are Kestrel, a professional network security penetration testing expert.",
		"High-intensity scan requirements: go all out",
		"",
		project.FactIndexSectionStartMarker,
		"## project blackboard index (project: 123, id: abc)",
		"(No facts yet)",
		"Use upsert_project_fact to write data.",
		project.FactIndexSectionEndMarker,
		"",
		transcriptSkillsSystemMarker,
		"**How to use Skills (progressive display):**",
		"Remember: Skills make you more powerful and stable",
	}, "\n")

	out := sanitizeSystemContentForTranscript(system)
	if strings.Contains(out, "The following is the tool name index") {
		t.Fatalf("tool index should be stripped: %q", out)
	}
	if strings.Contains(out, "- nmap") || strings.Contains(out, "High-intensity scan requirements") {
		t.Fatalf("static persona should be stripped: %q", out)
	}
	if strings.Contains(out, transcriptSkillsSystemMarker) || strings.Contains(out, "How to use Skills") {
		t.Fatalf("skills boilerplate should be stripped: %q", out)
	}
	if !strings.Contains(out, transcriptStaticSystemOmitNote) {
		t.Fatalf("missing omission note: %q", out)
	}
	if !strings.Contains(out, "## project blackboard index (project: 123, id: abc)") {
		t.Fatalf("project blackboard should be kept: %q", out)
	}
}

func TestFormatSummarizationTranscript_OmitsBloatedSystem(t *testing.T) {
	t.Parallel()
	msgs := []adk.Message{
		schema.SystemMessage("以下yes当前会话bind的tool nameindex\n- nmap\n\n你yesKestrel\n" + project.FactIndexSectionStartMarker + "\n## project黑板index（project: p1, id: x）\n（暂none事实）\n" + project.FactIndexSectionEndMarker + "\n" + transcriptSkillsSystemMarker + "\nboiler"),
		schema.UserMessage("hello"),
		schema.AssistantMessage("reply", nil),
	}
	out := formatSummarizationTranscript(msgs)
	if strings.Contains(out, "- nmap") {
		t.Fatalf("tool list leaked into transcript: %q", out)
	}
	if !strings.Contains(out, "hello") || !strings.Contains(out, "reply") {
		t.Fatalf("conversation turns missing: %q", out)
	}
	if !strings.Contains(out, "## project黑板index（project: p1, id: x）") {
		t.Fatalf("dynamic blackboard missing: %q", out)
	}
}

func TestRefreshFactIndexInMessages(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "summarize-facts.db")
	db, err := database.NewDB(dbPath, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	proj, err := db.CreateProject(&database.Project{Name: "summarize-proj"})
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.ProjectConfig{Enabled: true}
	oldIndex, err := project.BuildFactIndexBlock(db, proj.ID, cfg)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.UpsertProjectFact(&database.ProjectFact{
		ProjectID: proj.ID,
		FactKey:   "target/host",
		Category:  "target",
		Summary:   "fresh host fact",
	})
	if err != nil {
		t.Fatal(err)
	}

	msgs := []adk.Message{
		schema.SystemMessage("instruction\n\n" + oldIndex),
		schema.UserMessage("hi"),
	}

	out := refreshFactIndexInMessages(msgs, db, proj.ID, cfg, nil)
	sys := out[0].Content
	if strings.Contains(sys, "(No facts yet)") {
		t.Fatalf("expected refreshed index, got: %q", sys)
	}
	if !strings.Contains(sys, "fresh host fact") {
		t.Fatalf("expected new fact in index: %q", sys)
	}
	if !strings.Contains(sys, "instruction") {
		t.Fatalf("non-index system content should be preserved: %q", sys)
	}
}

func TestBuildOriginalUserIntentLedgerUsesOnlyModelFacingMessages(t *testing.T) {
	ledger := buildOriginalUserIntentLedgerMessage(
		[]adk.Message{schema.UserMessage("model实际看到的裁剪预览")},
		config.DefaultSummarizationUserIntentLedgerMaxRunes,
		config.DefaultSummarizationUserIntentLedgerEntryMaxRunes,
	)
	if ledger == nil {
		t.Fatal("expected ledger message")
	}
	body := ledger.Content
	if !strings.Contains(body, "model实际看到的裁剪预览") {
		t.Fatalf("ledger should preserve the model-facing user message: %q", body)
	}
}
