package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"kestrel/internal/termout"
	"kestrel/internal/toolguard"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version     string                `yaml:"version,omitempty" json:"version,omitempty"` // Version displayed in the frontend, e.g. v1.3.3
	Server      ServerConfig          `yaml:"server"`
	Log         LogConfig             `yaml:"log"`
	MCP         MCPConfig             `yaml:"mcp"`
	AI          AIConfig              `yaml:"ai,omitempty" json:"ai,omitempty"`
	OpenAI      OpenAIConfig          `yaml:"openai,omitempty" json:"openai,omitempty"`
	FOFA        FofaConfig            `yaml:"fofa,omitempty" json:"fofa,omitempty"`
	ZoomEye     SpaceSearchConfig     `yaml:"zoomeye,omitempty" json:"zoomeye,omitempty"`
	Quake       SpaceSearchConfig     `yaml:"quake,omitempty" json:"quake,omitempty"`
	Shodan      SpaceSearchConfig     `yaml:"shodan,omitempty" json:"shodan,omitempty"`
	Agent       AgentConfig           `yaml:"agent"`
	Hitl        HitlConfig            `yaml:"hitl,omitempty" json:"hitl,omitempty"`
	ToolGuard   *toolguard.Config     `yaml:"tool_guard,omitempty" json:"tool_guard,omitempty"`
	Security    SecurityConfig        `yaml:"security"`
	Database    DatabaseConfig        `yaml:"database"`
	Auth        AuthConfig            `yaml:"auth"`
	Audit       AuditConfig           `yaml:"audit,omitempty" json:"audit,omitempty"`
	Monitor     MonitorConfig         `yaml:"monitor,omitempty" json:"monitor,omitempty"`
	Storage     StorageConfig         `yaml:"storage,omitempty" json:"storage,omitempty"`
	ExternalMCP ExternalMCPConfig     `yaml:"external_mcp,omitempty"`
	Knowledge   KnowledgeConfig       `yaml:"knowledge,omitempty"`
	C2          C2Config              `yaml:"c2,omitempty" json:"c2,omitempty"`                 // Built-in C2 master switch; enabled by default if not configured
	Robots      RobotsConfig          `yaml:"robots,omitempty" json:"robots,omitempty"`         // Bot configuration for WeCom/DingTalk/Feishu/Telegram/Slack etc.
	RolesDir    string                `yaml:"roles_dir,omitempty" json:"roles_dir,omitempty"`   // Role configuration file directory (new method)
	Roles       map[string]RoleConfig `yaml:"roles,omitempty" json:"roles,omitempty"`           // Backward compatible: supports defining roles in main config file
	SkillsDir   string                `yaml:"skills_dir,omitempty" json:"skills_dir,omitempty"` // Skills configuration file directory
	AgentsDir   string                `yaml:"agents_dir,omitempty" json:"agents_dir,omitempty"` // Multi-agent sub-Agent Markdown definition directory (*.md, YAML front matter)
	MultiAgent  MultiAgentConfig      `yaml:"multi_agent,omitempty" json:"multi_agent,omitempty"`
	Project     ProjectConfig         `yaml:"project,omitempty" json:"project,omitempty"`
	Vision      VisionConfig          `yaml:"vision,omitempty" json:"vision,omitempty"`
}

type EnsureLocalConfigResult struct {
	Created     bool
	ExamplePath string
}

const (
	DefaultMaxCompletionTokens                        = 16384
	DefaultSummarizationUserIntentLedgerMaxRunes      = 96000
	DefaultSummarizationUserIntentLedgerEntryMaxRunes = 16000
	DefaultLatestUserMessageMaxRunes                  = 48000
	DefaultLatestUserMessageHeadRunes                 = 24000
	DefaultLatestUserMessageTailRunes                 = 24000
	DefaultSummarizationOutputReserveTokens           = 40960
)

// ProjectConfig Project blackboard (shared facts across conversations) configuration.
type ProjectConfig struct {
	Enabled                 bool   `yaml:"enabled" json:"enabled"`
	DefaultProjectID        string `yaml:"default_project_id,omitempty" json:"default_project_id,omitempty"` // Default project to bind when no explicit project (bot/batch etc.)
	FactIndexMaxRunes       int    `yaml:"fact_index_max_runes,omitempty" json:"fact_index_max_runes,omitempty"`
	FactIndexPathMaxRunes   int    `yaml:"fact_index_path_max_runes,omitempty" json:"fact_index_path_max_runes,omitempty"`
	FactSummaryMaxRunes     int    `yaml:"fact_summary_max_runes,omitempty" json:"fact_summary_max_runes,omitempty"`
	DefaultInjectDeprecated bool   `yaml:"default_inject_deprecated,omitempty" json:"default_inject_deprecated,omitempty"`
}

// FactIndexMaxRunesEffective Maximum runes for auto-injected blackboard index.
func (c ProjectConfig) FactIndexMaxRunesEffective() int {
	if c.FactIndexMaxRunes <= 0 {
		return 3500
	}
	return c.FactIndexMaxRunes
}

// FactIndexPathMaxRunesEffective Maximum runes for the attack path preview segment (reserved from the fact_index_max_runes budget).
func (c ProjectConfig) FactIndexPathMaxRunesEffective() int {
	if c.FactIndexPathMaxRunes <= 0 {
		return 1000
	}
	return c.FactIndexPathMaxRunes
}

// FactSummaryMaxRunesEffective Maximum runes for summary during upsert (one index line, should contain key validation points).
func (c ProjectConfig) FactSummaryMaxRunesEffective() int {
	if c.FactSummaryMaxRunes <= 0 {
		return 200
	}
	return c.FactSummaryMaxRunes
}

// MultiAgentConfig Multi-agent orchestration based on CloudWeGo Eino adk/prebuilt (deep | plan_execute | supervisor).
type MultiAgentConfig struct {
	Enabled               bool   `yaml:"enabled" json:"enabled"`
	RobotDefaultAgentMode string `yaml:"robot_default_agent_mode,omitempty" json:"robot_default_agent_mode,omitempty"` // eino_single | deep | plan_execute | supervisor
	BatchUseMultiAgent    bool   `yaml:"batch_use_multi_agent" json:"batch_use_multi_agent"`                           // When true, each sub-task in the batch task queue runs through Eino multi-agent
	// Orchestration is deprecated: retained only for compatibility with old config.yaml; orchestration is determined by chat/WebShell request body orchestration field, defaults to deep if not provided.
	Orchestration string `yaml:"orchestration,omitempty" json:"orchestration,omitempty"`
	// MaxIteration is deprecated: use agent.max_iterations instead (field retained in YAML only for old config compatibility, not read at runtime).
	MaxIteration int `yaml:"max_iteration,omitempty" json:"max_iteration,omitempty"`
	// PlanExecuteLoopMaxIterations is the outer loop limit for execute↔replan under the plan_execute pattern; 0 uses Eino's default of 10.
	PlanExecuteLoopMaxIterations int `yaml:"plan_execute_loop_max_iterations,omitempty" json:"plan_execute_loop_max_iterations,omitempty"`
	// SubAgentMaxIterations is deprecated: both sub-agents and the primary agent use agent.max_iterations (Markdown max_iterations>0 can override).
	SubAgentMaxIterations   int    `yaml:"sub_agent_max_iterations,omitempty" json:"sub_agent_max_iterations,omitempty"`
	WithoutGeneralSubAgent  bool   `yaml:"without_general_sub_agent" json:"without_general_sub_agent"`
	WithoutWriteTodos       bool   `yaml:"without_write_todos" json:"without_write_todos"`
	OrchestratorInstruction string `yaml:"orchestrator_instruction" json:"orchestrator_instruction"`
	// OrchestratorInstructionPlanExecute is the system prompt for the plan_execute primary agent (planning side); takes effect when non-empty and agents/orchestrator-plan-execute.md body is empty or missing. Do not mix with Deep's orchestrator_instruction.
	OrchestratorInstructionPlanExecute string `yaml:"orchestrator_instruction_plan_execute,omitempty" json:"orchestrator_instruction_plan_execute,omitempty"`
	// OrchestratorInstructionSupervisor is the supervisor primary agent system prompt (transfer/exit instructions are still appended at runtime); takes effect when non-empty and agents/orchestrator-supervisor.md body is empty or missing.
	OrchestratorInstructionSupervisor string                `yaml:"orchestrator_instruction_supervisor,omitempty" json:"orchestrator_instruction_supervisor,omitempty"`
	SubAgents                         []MultiAgentSubConfig `yaml:"sub_agents" json:"sub_agents"`
	// SubAgentUserContextMaxRunes caps user-context supplement for sub-agent task descriptions.
	// 0 (default) preserves all user turns verbatim; >0 caps total runes; negative disables injection.
	SubAgentUserContextMaxRunes int `yaml:"sub_agent_user_context_max_runes,omitempty" json:"sub_agent_user_context_max_runes,omitempty"`
	// EinoSkills configures CloudWeGo Eino ADK skill middleware + optional local filesystem/execute on DeepAgent.
	EinoSkills MultiAgentEinoSkillsConfig `yaml:"eino_skills,omitempty" json:"eino_skills,omitempty"`
	// EinoMiddleware wires optional ADK middleware (patchtoolcalls, toolsearch, plantask, reduction) and Deep extras.
	EinoMiddleware MultiAgentEinoMiddlewareConfig `yaml:"eino_middleware,omitempty" json:"eino_middleware,omitempty"`
	// EinoCallbacks attaches CloudWeGo eino callbacks.InitCallbacks on ADK Runner context (structured logs + optional SSE trace).
	EinoCallbacks MultiAgentEinoCallbacksConfig `yaml:"eino_callbacks,omitempty" json:"eino_callbacks,omitempty"`
}

// SubAgentUserContextMaxRunesEffective returns max runes for sub-agent task supplement; 0 = unlimited; negative = disabled.
func (c MultiAgentConfig) SubAgentUserContextMaxRunesEffective() int {
	return c.SubAgentUserContextMaxRunes
}

// MultiAgentEinoCallbacksConfig enables Eino unified callbacks on each ADK agent run (deep / plan_execute / supervisor / eino_single).
// Modes: log_only (zap + optional OTel; no SSE to browser), sse (adds client SSE eino_trace_* when sse_trace_to_client), full (sse rules + stream callback copies closed).
type MultiAgentEinoCallbacksConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Mode    string `yaml:"mode,omitempty" json:"mode,omitempty"` // log_only | sse | full; empty with enabled=true defaults to log_only
	// SseTraceToClient when true emits eino_trace_* SSE for UI (use only for admin/debug; nil/false recommended in production).
	SseTraceToClient *bool `yaml:"sse_trace_to_client,omitempty" json:"sse_trace_to_client,omitempty"`
	// Otel configures OpenTelemetry trace export (independent of mode; exporter none disables export even if enabled).
	Otel MultiAgentEinoCallbacksOtelConfig `yaml:"otel,omitempty" json:"otel,omitempty"`
	// MaxInputSummaryRunes / MaxOutputSummaryRunes cap text placed in SSE payloads and debug logs (not full payloads).
	MaxInputSummaryRunes  int `yaml:"max_input_summary_runes,omitempty" json:"max_input_summary_runes,omitempty"`
	MaxOutputSummaryRunes int `yaml:"max_output_summary_runes,omitempty" json:"max_output_summary_runes,omitempty"`
	// ZapVerbose when true logs input/output summaries at zap.Debug on start/end; false uses Info with short fields only.
	ZapVerbose bool `yaml:"zap_verbose,omitempty" json:"zap_verbose,omitempty"`
}

// MultiAgentEinoCallbacksOtelConfig OpenTelemetry for Eino callback spans (W3C trace in collector / stdout).
type MultiAgentEinoCallbacksOtelConfig struct {
	Enabled      bool    `yaml:"enabled" json:"enabled"`
	ServiceName  string  `yaml:"service_name,omitempty" json:"service_name,omitempty"`
	Exporter     string  `yaml:"exporter,omitempty" json:"exporter,omitempty"`           // none | stdout | otlphttp
	OTLPEndpoint string  `yaml:"otlp_endpoint,omitempty" json:"otlp_endpoint,omitempty"` // host:port, e.g. localhost:4318 (path /v1/traces)
	SampleRatio  float64 `yaml:"sample_ratio,omitempty" json:"sample_ratio,omitempty"`   // 0–1, default 1.0
}

// EinoCallbacksModeEffective returns off | log_only | sse | full.
func (c MultiAgentEinoCallbacksConfig) EinoCallbacksModeEffective() string {
	if !c.Enabled {
		return "off"
	}
	m := strings.TrimSpace(strings.ToLower(c.Mode))
	switch m {
	case "log_only":
		return "log_only"
	case "sse":
		return "sse"
	case "full":
		return "full"
	case "":
		return "log_only"
	default:
		return "log_only"
	}
}

// SseTraceToClientEffective is false unless explicitly set true (best practice: do not expose framework traces to end users by default).
func (c MultiAgentEinoCallbacksConfig) SseTraceToClientEffective() bool {
	if c.SseTraceToClient == nil {
		return false
	}
	return *c.SseTraceToClient
}

// ShouldEmitEinoTraceSSE is true when client-visible trace events should be sent over progress/SSE.
func (c MultiAgentEinoCallbacksConfig) ShouldEmitEinoTraceSSE(mode string) bool {
	if !c.SseTraceToClientEffective() {
		return false
	}
	return mode == "sse" || mode == "full"
}

// OtelExporterEffective returns none | stdout | otlphttp.
func (c MultiAgentEinoCallbacksOtelConfig) OtelExporterEffective() string {
	e := strings.TrimSpace(strings.ToLower(c.Exporter))
	switch e {
	case "none", "stdout", "otlphttp":
		return e
	case "":
		if c.Enabled {
			return "stdout"
		}
		return "none"
	default:
		return "none"
	}
}

// OtelTracingActive is true when spans should be started (enabled + non-none exporter).
func (c MultiAgentEinoCallbacksConfig) OtelTracingActive() bool {
	if !c.Otel.Enabled {
		return false
	}
	return c.Otel.OtelExporterEffective() != "none"
}

func (c MultiAgentEinoCallbacksOtelConfig) ServiceNameEffective() string {
	s := strings.TrimSpace(c.ServiceName)
	if s != "" {
		return s
	}
	return "kestrel"
}

func (c MultiAgentEinoCallbacksOtelConfig) SampleRatioEffective() float64 {
	r := c.SampleRatio
	if r <= 0 {
		return 1.0
	}
	if r > 1 {
		return 1.0
	}
	return r
}

func (c MultiAgentEinoCallbacksConfig) EinoCallbacksMaxInputSummaryRunes() int {
	if c.MaxInputSummaryRunes > 0 {
		return c.MaxInputSummaryRunes
	}
	return 400
}

func (c MultiAgentEinoCallbacksConfig) EinoCallbacksMaxOutputSummaryRunes() int {
	if c.MaxOutputSummaryRunes > 0 {
		return c.MaxOutputSummaryRunes
	}
	return 400
}

// MultiAgentEinoMiddlewareConfig optional Eino ADK middleware and Deep / supervisor tuning.
type MultiAgentEinoMiddlewareConfig struct {
	// PatchToolCalls inserts placeholder tool results for dangling assistant tool_calls (nil = enabled).
	PatchToolCalls *bool `yaml:"patch_tool_calls,omitempty" json:"patch_tool_calls,omitempty"`
	// ToolSearch enables dynamictool/toolsearch: hide tail tools until model calls tool_search (reduces prompt tools).
	ToolSearchEnable        bool `yaml:"tool_search_enable,omitempty" json:"tool_search_enable,omitempty"`
	ToolSearchMinTools      int  `yaml:"tool_search_min_tools,omitempty" json:"tool_search_min_tools,omitempty"`           // default 20; applies when len(tools) >= this
	ToolSearchAlwaysVisible int  `yaml:"tool_search_always_visible,omitempty" json:"tool_search_always_visible,omitempty"` // default 12; first N tools stay always visible
	// ToolSearchAlwaysVisibleTools keeps specified tool names always visible (never hidden by tool_search).
	ToolSearchAlwaysVisibleTools []string `yaml:"tool_search_always_visible_tools,omitempty" json:"tool_search_always_visible_tools,omitempty"`
	// Plantask adds TaskCreate/Get/Update/List (file-backed under skills dir); requires eino_skills + local backend.
	PlantaskEnable bool `yaml:"plantask_enable,omitempty" json:"plantask_enable,omitempty"`
	// PlantaskRelDir relative to skills_dir for per-conversation task boards (default .eino/plantask).
	PlantaskRelDir string `yaml:"plantask_rel_dir,omitempty" json:"plantask_rel_dir,omitempty"`
	// Reduction truncates/offloads large tool outputs (requires eino local backend for Write).
	ReductionEnable            bool     `yaml:"reduction_enable,omitempty" json:"reduction_enable,omitempty"`
	ReductionRootDir           string   `yaml:"reduction_root_dir,omitempty" json:"reduction_root_dir,omitempty"`                         // non-empty: disk write root directory (default tmp/reduction); isolated by projects/{id} or conversations/{id}
	ReductionMaxLengthForTrunc int      `yaml:"reduction_max_length_for_trunc,omitempty" json:"reduction_max_length_for_trunc,omitempty"` // default 12000
	ReductionMaxTokensForClear int      `yaml:"reduction_max_tokens_for_clear,omitempty" json:"reduction_max_tokens_for_clear,omitempty"` // default 50000
	ReductionClearExclude      []string `yaml:"reduction_clear_exclude,omitempty" json:"reduction_clear_exclude,omitempty"`
	ReductionSubAgents         bool     `yaml:"reduction_sub_agents,omitempty" json:"reduction_sub_agents,omitempty"` // also attach to sub-agents
	// SummarizationTriggerRatio controls summarization trigger threshold as max_total_tokens * ratio (default 0.8).
	SummarizationTriggerRatio float64 `yaml:"summarization_trigger_ratio,omitempty" json:"summarization_trigger_ratio,omitempty"`
	// SummarizationOutputReserveTokens reserves completion headroom for the summarization model call (default 40960).
	SummarizationOutputReserveTokens int `yaml:"summarization_output_reserve_tokens,omitempty" json:"summarization_output_reserve_tokens,omitempty"`
	// SummarizationEmitInternalEvents controls middleware internal event emission (default true).
	SummarizationEmitInternalEvents *bool `yaml:"summarization_emit_internal_events,omitempty" json:"summarization_emit_internal_events,omitempty"`
	// SummarizationUserIntentLedgerMaxRunes caps the DB-backed immutable user input ledger injected into model context.
	SummarizationUserIntentLedgerMaxRunes int `yaml:"summarization_user_intent_ledger_max_runes,omitempty" json:"summarization_user_intent_ledger_max_runes,omitempty"`
	// SummarizationUserIntentLedgerEntryMaxRunes caps each user message entry inside the immutable user input ledger.
	SummarizationUserIntentLedgerEntryMaxRunes int `yaml:"summarization_user_intent_ledger_entry_max_runes,omitempty" json:"summarization_user_intent_ledger_entry_max_runes,omitempty"`
	// LatestUserMessageMaxRunes caps the current user turn inserted into model context; full text is persisted as an artifact when capped.
	LatestUserMessageMaxRunes int `yaml:"latest_user_message_max_runes,omitempty" json:"latest_user_message_max_runes,omitempty"`
	// LatestUserMessageHeadRunes keeps the head preview for an oversized current user turn.
	LatestUserMessageHeadRunes int `yaml:"latest_user_message_head_runes,omitempty" json:"latest_user_message_head_runes,omitempty"`
	// LatestUserMessageTailRunes keeps the tail preview for an oversized current user turn.
	LatestUserMessageTailRunes int `yaml:"latest_user_message_tail_runes,omitempty" json:"latest_user_message_tail_runes,omitempty"`
	// SummarizationRetryMaxAttempts deprecated: summarization shares model_retry_max_retries and isEinoTransientRunError with Eino native ModelRetry.
	SummarizationRetryMaxAttempts int `yaml:"summarization_retry_max_attempts,omitempty" json:"summarization_retry_max_attempts,omitempty"`
	// PlanExecuteUserInputBudgetRatio caps planner/replanner/executor userInput prompt budget ratio (default 0.35).
	PlanExecuteUserInputBudgetRatio float64 `yaml:"plan_execute_user_input_budget_ratio,omitempty" json:"plan_execute_user_input_budget_ratio,omitempty"`
	// PlanExecuteExecutedStepsBudgetRatio caps executed_steps prompt budget ratio (default 0.2).
	PlanExecuteExecutedStepsBudgetRatio float64 `yaml:"plan_execute_executed_steps_budget_ratio,omitempty" json:"plan_execute_executed_steps_budget_ratio,omitempty"`
	// PlanExecuteMaxStepResultRunes caps each executed step result length for prompt view (default 4000).
	PlanExecuteMaxStepResultRunes int `yaml:"plan_execute_max_step_result_runes,omitempty" json:"plan_execute_max_step_result_runes,omitempty"`
	// PlanExecuteKeepLastSteps keeps only the tail steps in prompt view (default 8).
	PlanExecuteKeepLastSteps int `yaml:"plan_execute_keep_last_steps,omitempty" json:"plan_execute_keep_last_steps,omitempty"`
	// CheckpointDir is retained for config compatibility. Chat agent runs do
	// not consume it; cross-turn recovery is centralized in conversations.last_react_*.
	CheckpointDir string `yaml:"checkpoint_dir,omitempty" json:"checkpoint_dir,omitempty"`
	// DeepOutputKey passed to deep.Config OutputKey (session final text); empty = off.
	DeepOutputKey string `yaml:"deep_output_key,omitempty" json:"deep_output_key,omitempty"`
	// DeepModelRetryMaxRetries deprecated: use model_retry_max_retries instead; field retained only for old config compatibility.
	DeepModelRetryMaxRetries int `yaml:"deep_model_retry_max_retries,omitempty" json:"deep_model_retry_max_retries,omitempty"`
	// ModelRetryMaxRetries configures Eino ADK native ChatModel retry attempts; 0=default 4.
	ModelRetryMaxRetries int `yaml:"model_retry_max_retries,omitempty" json:"model_retry_max_retries,omitempty"`
	// ModelRetryMaxBackoffSec caps native model retry backoff seconds; 0=default 30.
	ModelRetryMaxBackoffSec int `yaml:"model_retry_max_backoff_sec,omitempty" json:"model_retry_max_backoff_sec,omitempty"`
	// ModelFailoverChannels lists ai.channels IDs to try after native model retry is exhausted.
	ModelFailoverChannels []string `yaml:"model_failover_channels,omitempty" json:"model_failover_channels,omitempty"`
	// ModelFailoverMaxRetries caps distinct failover channel attempts; 0=all configured failover channels.
	ModelFailoverMaxRetries int `yaml:"model_failover_max_retries,omitempty" json:"model_failover_max_retries,omitempty"`
	// RunRetryMaxAttempts deprecated: model transient errors are handled by Eino native ModelRetry; retained only for non-model layer run loop fallback and summarization legacy fields.
	RunRetryMaxAttempts int `yaml:"run_retry_max_attempts,omitempty" json:"run_retry_max_attempts,omitempty"`
	// RunRetryMaxBackoffSec deprecated: use model_retry_max_backoff_sec instead; retained only for non-model layer run loop fallback and summarization legacy fields.
	RunRetryMaxBackoffSec int `yaml:"run_retry_max_backoff_sec,omitempty" json:"run_retry_max_backoff_sec,omitempty"`
	// EmptyResponseContinueMaxAttempts number of Handler layer backoff retries when Run succeeds but assistant body is not captured; 0=default 5.
	EmptyResponseContinueMaxAttempts int `yaml:"empty_response_continue_max_attempts,omitempty" json:"empty_response_continue_max_attempts,omitempty"`
	// TaskToolDescriptionPrefix when non-empty sets deep.Config TaskToolDescriptionGenerator (sub-agent names appended).
	TaskToolDescriptionPrefix string `yaml:"task_tool_description_prefix,omitempty" json:"task_tool_description_prefix,omitempty"`
}

func (c MultiAgentEinoMiddlewareConfig) SummarizationTriggerRatioEffective() float64 {
	v := c.SummarizationTriggerRatio
	if v <= 0 {
		return 0.8
	}
	if v < 0.5 {
		return 0.5
	}
	if v > 0.95 {
		return 0.95
	}
	return v
}

func (c MultiAgentEinoMiddlewareConfig) SummarizationOutputReserveTokensEffective() int {
	if c.SummarizationOutputReserveTokens > 0 {
		return c.SummarizationOutputReserveTokens
	}
	return DefaultSummarizationOutputReserveTokens
}

func (c MultiAgentEinoMiddlewareConfig) SummarizationEmitInternalEventsEffective() bool {
	if c.SummarizationEmitInternalEvents != nil {
		return *c.SummarizationEmitInternalEvents
	}
	return true
}

func (c MultiAgentEinoMiddlewareConfig) SummarizationUserIntentLedgerMaxRunesEffective() int {
	if c.SummarizationUserIntentLedgerMaxRunes > 0 {
		return c.SummarizationUserIntentLedgerMaxRunes
	}
	return DefaultSummarizationUserIntentLedgerMaxRunes
}

func (c MultiAgentEinoMiddlewareConfig) SummarizationUserIntentLedgerEntryMaxRunesEffective() int {
	if c.SummarizationUserIntentLedgerEntryMaxRunes > 0 {
		return c.SummarizationUserIntentLedgerEntryMaxRunes
	}
	return DefaultSummarizationUserIntentLedgerEntryMaxRunes
}

func (c MultiAgentEinoMiddlewareConfig) LatestUserMessageMaxRunesEffective() int {
	if c.LatestUserMessageMaxRunes > 0 {
		return c.LatestUserMessageMaxRunes
	}
	return DefaultLatestUserMessageMaxRunes
}

func (c MultiAgentEinoMiddlewareConfig) LatestUserMessageHeadRunesEffective() int {
	if c.LatestUserMessageHeadRunes > 0 {
		return c.LatestUserMessageHeadRunes
	}
	return DefaultLatestUserMessageHeadRunes
}

func (c MultiAgentEinoMiddlewareConfig) LatestUserMessageTailRunesEffective() int {
	if c.LatestUserMessageTailRunes > 0 {
		return c.LatestUserMessageTailRunes
	}
	return DefaultLatestUserMessageTailRunes
}

func (c MultiAgentEinoMiddlewareConfig) PlanExecuteUserInputBudgetRatioEffective() float64 {
	v := c.PlanExecuteUserInputBudgetRatio
	if v <= 0 {
		return 0.35
	}
	if v < 0.1 {
		return 0.1
	}
	if v > 0.6 {
		return 0.6
	}
	return v
}

func (c MultiAgentEinoMiddlewareConfig) PlanExecuteExecutedStepsBudgetRatioEffective() float64 {
	v := c.PlanExecuteExecutedStepsBudgetRatio
	if v <= 0 {
		return 0.2
	}
	if v < 0.08 {
		return 0.08
	}
	if v > 0.5 {
		return 0.5
	}
	return v
}

func (c MultiAgentEinoMiddlewareConfig) PlanExecuteMaxStepResultRunesEffective() int {
	if c.PlanExecuteMaxStepResultRunes > 0 {
		return c.PlanExecuteMaxStepResultRunes
	}
	return 4000
}

func (c MultiAgentEinoMiddlewareConfig) PlanExecuteKeepLastStepsEffective() int {
	if c.PlanExecuteKeepLastSteps > 0 {
		return c.PlanExecuteKeepLastSteps
	}
	return 8
}

func (c MultiAgentEinoMiddlewareConfig) ReductionMaxLengthForTruncEffective() int {
	if c.ReductionMaxLengthForTrunc > 0 {
		return c.ReductionMaxLengthForTrunc
	}
	return 12000
}

func (c MultiAgentEinoMiddlewareConfig) ReductionMaxTokensForClearEffective() int {
	if c.ReductionMaxTokensForClear > 0 {
		return c.ReductionMaxTokensForClear
	}
	return 50000
}

// MultiAgentEinoSkillsConfig toggles Eino official skill progressive disclosure and host filesystem tools.
type MultiAgentEinoSkillsConfig struct {
	// Disable skips skill middleware (and does not attach local FS tools for Deep).
	Disable bool `yaml:"disable" json:"disable"`
	// FilesystemTools registers read_file/glob/grep/write/edit/execute (eino-ext local backend). Nil/omitted = true.
	FilesystemTools *bool `yaml:"filesystem_tools,omitempty" json:"filesystem_tools,omitempty"`
	// SkillToolName overrides the default Eino tool name "skill".
	SkillToolName string `yaml:"skill_tool_name,omitempty" json:"skill_tool_name,omitempty"`
}

// EinoSkillFilesystemToolsEffective returns whether Deep/sub-agents should attach local filesystem + streaming shell.
func (c MultiAgentEinoSkillsConfig) EinoSkillFilesystemToolsEffective() bool {
	if c.FilesystemTools != nil {
		return *c.FilesystemTools
	}
	return true
}

// PatchToolCallsEffective returns whether patchtoolcalls middleware should run (default true).
func (c MultiAgentEinoMiddlewareConfig) PatchToolCallsEffective() bool {
	if c.PatchToolCalls != nil {
		return *c.PatchToolCalls
	}
	return true
}

// MultiAgentSubConfig sub-agent (Eino ChatModelAgent): dispatched by task under deep; delegated by transfer under supervisor; plan_execute does not use sub-agent list.
type MultiAgentSubConfig struct {
	ID            string   `yaml:"id" json:"id"`
	Name          string   `yaml:"name" json:"name"`
	Description   string   `yaml:"description" json:"description"`
	Instruction   string   `yaml:"instruction" json:"instruction"`
	BindRole      string   `yaml:"bind_role,omitempty" json:"bind_role,omitempty"` // optional: bind to a role name in main config roles; when role_tools is not configured, inherits tools from that role
	RoleTools     []string `yaml:"role_tools" json:"role_tools"`                   // same key as single Agent role tool; empty means all tools (bind_role can supplement tools)
	MaxIterations int      `yaml:"max_iterations" json:"max_iterations"`
	Kind          string   `yaml:"kind,omitempty" json:"kind,omitempty"` // Markdown only: kind=orchestrator indicates Deep primary agent (alternative to orchestrator.md)
}

// MultiAgentPublic condensed info to return to frontend (does not include full sub-agent instructions).
type MultiAgentPublic struct {
	Enabled                                    bool     `json:"enabled"`
	RobotDefaultAgentMode                      string   `json:"robot_default_agent_mode,omitempty"`
	BatchUseMultiAgent                         bool     `json:"batch_use_multi_agent"`
	SubAgentCount                              int      `json:"sub_agent_count"`
	Orchestration                              string   `json:"orchestration,omitempty"`
	PlanExecuteLoopMaxIterations               int      `json:"plan_execute_loop_max_iterations"`
	SummarizationUserIntentLedgerMaxRunes      int      `json:"summarization_user_intent_ledger_max_runes"`
	SummarizationUserIntentLedgerEntryMaxRunes int      `json:"summarization_user_intent_ledger_entry_max_runes"`
	LatestUserMessageMaxRunes                  int      `json:"latest_user_message_max_runes"`
	LatestUserMessageHeadRunes                 int      `json:"latest_user_message_head_runes"`
	LatestUserMessageTailRunes                 int      `json:"latest_user_message_tail_runes"`
	ModelRetryMaxRetries                       int      `json:"model_retry_max_retries"`
	ModelRetryMaxBackoffSec                    int      `json:"model_retry_max_backoff_sec"`
	ModelFailoverChannels                      []string `json:"model_failover_channels,omitempty"`
	ModelFailoverMaxRetries                    int      `json:"model_failover_max_retries"`
	ToolSearchAlwaysVisibleTools               []string `json:"tool_search_always_visible_tools,omitempty"`
	ToolSearchAlwaysVisibleEffectiveTools      []string `json:"tool_search_always_visible_effective_tools,omitempty"`
}

// NormalizeAgentMode parse agent mode (eino_single | deep | plan_execute | supervisor); empty value defaults to eino_single.
func NormalizeAgentMode(mode string) string {
	s := strings.TrimSpace(strings.ToLower(mode))
	switch s {
	case "", "eino_single":
		return "eino_single"
	case "deep":
		return "deep"
	case "plan_execute", "plan-execute", "planexecute", "pe":
		return "plan_execute"
	case "supervisor", "super", "sv":
		return "supervisor"
	default:
		return "eino_single"
	}
}

// NormalizeRobotAgentMode parse robot default conversation mode.
func NormalizeRobotAgentMode(ma MultiAgentConfig) string {
	return NormalizeAgentMode(ma.RobotDefaultAgentMode)
}

// NormalizeMultiAgentOrchestration return deep, plan_execute or supervisor.
func NormalizeMultiAgentOrchestration(s string) string {
	v := strings.TrimSpace(strings.ToLower(s))
	switch v {
	case "plan_execute", "plan-execute", "planexecute", "pe":
		return "plan_execute"
	case "supervisor", "super", "sv":
		return "supervisor"
	default:
		return "deep"
	}
}

// MultiAgentAPIUpdate settings page/API only updates multi-agent scalar fields; does not overwrite sub_agents blocks when writing YAML.
type MultiAgentAPIUpdate struct {
	Enabled                                    bool      `json:"enabled"`
	RobotDefaultAgentMode                      string    `json:"robot_default_agent_mode,omitempty"`
	BatchUseMultiAgent                         bool      `json:"batch_use_multi_agent"`
	PlanExecuteLoopMaxIterations               *int      `json:"plan_execute_loop_max_iterations,omitempty"`
	SummarizationUserIntentLedgerMaxRunes      *int      `json:"summarization_user_intent_ledger_max_runes,omitempty"`
	SummarizationUserIntentLedgerEntryMaxRunes *int      `json:"summarization_user_intent_ledger_entry_max_runes,omitempty"`
	LatestUserMessageMaxRunes                  *int      `json:"latest_user_message_max_runes,omitempty"`
	LatestUserMessageHeadRunes                 *int      `json:"latest_user_message_head_runes,omitempty"`
	LatestUserMessageTailRunes                 *int      `json:"latest_user_message_tail_runes,omitempty"`
	ModelRetryMaxRetries                       *int      `json:"model_retry_max_retries,omitempty"`
	ModelRetryMaxBackoffSec                    *int      `json:"model_retry_max_backoff_sec,omitempty"`
	ModelFailoverChannels                      *[]string `json:"model_failover_channels,omitempty"`
	ModelFailoverMaxRetries                    *int      `json:"model_failover_max_retries,omitempty"`
	// pointer distinguishes "JSON field not passed" from "pass empty array to clear"; when omitted, should not overwrite the permanent tool whitelist in YAML.
	ToolSearchAlwaysVisibleTools *[]string `json:"tool_search_always_visible_tools,omitempty"`
}

// RobotsConfig robot configuration (WeCom, DingTalk, Feishu, WeChat iLink, Telegram, Slack, Discord, QQ, etc.)
type RobotsConfig struct {
	Session  RobotSessionConfig  `yaml:"session,omitempty" json:"session,omitempty"`   // robot session isolation strategy
	Wechat   RobotWechatConfig   `yaml:"wechat,omitempty" json:"wechat,omitempty"`     // WeChat (iLink scan code binding)
	Wecom    RobotWecomConfig    `yaml:"wecom,omitempty" json:"wecom,omitempty"`       // WeCom
	Dingtalk RobotDingtalkConfig `yaml:"dingtalk,omitempty" json:"dingtalk,omitempty"` // DingTalk
	Lark     RobotLarkConfig     `yaml:"lark,omitempty" json:"lark,omitempty"`         // Feishu
	Telegram RobotTelegramConfig `yaml:"telegram,omitempty" json:"telegram,omitempty"` // Telegram
	Slack    RobotSlackConfig    `yaml:"slack,omitempty" json:"slack,omitempty"`       // Slack
	Discord  RobotDiscordConfig  `yaml:"discord,omitempty" json:"discord,omitempty"`   // Discord
	QQ       RobotQQConfig       `yaml:"qq,omitempty" json:"qq,omitempty"`             // QQ robot
}

// RobotWechatConfig WeChat iLink robot configuration (personal WeChat ClawBot / iLink protocol)
type RobotWechatConfig struct {
	Enabled       bool                     `yaml:"enabled" json:"enabled"`
	BotToken      string                   `yaml:"bot_token,omitempty" json:"bot_token,omitempty"`
	ILinkBotID    string                   `yaml:"ilink_bot_id,omitempty" json:"ilink_bot_id,omitempty"`
	ILinkUserID   string                   `yaml:"ilink_user_id,omitempty" json:"ilink_user_id,omitempty"`
	BaseURL       string                   `yaml:"base_url,omitempty" json:"base_url,omitempty"`               // default https://ilinkai.weixin.qq.com
	BotType       string                   `yaml:"bot_type,omitempty" json:"bot_type,omitempty"`               // get_bot_qrcode parameter, default 3
	BotAgent      string                   `yaml:"bot_agent,omitempty" json:"bot_agent,omitempty"`             // base_info.bot_agent
	GetUpdatesBuf string                   `yaml:"get_updates_buf,omitempty" json:"get_updates_buf,omitempty"` // long polling cursor (runtime)
	Auth          RobotAuthorizationConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

const (
	RobotAuthModeUserBinding    = "user_binding"
	RobotAuthModeServiceAccount = "service_account"
)

// RobotAuthorizationConfig controls how a verified platform sender becomes
// an RBAC principal. service_account is intentionally fail-closed unless an
// explicit non-admin service user and sender allowlist are both configured.
type RobotAuthorizationConfig struct {
	Mode                 string   `yaml:"mode,omitempty" json:"mode,omitempty"`
	ServiceUserID        string   `yaml:"service_user_id,omitempty" json:"service_user_id,omitempty"`
	AllowedExternalUsers []string `yaml:"allowed_external_users,omitempty" json:"allowed_external_users,omitempty"`
}

func (c RobotAuthorizationConfig) EffectiveMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	if mode == "" {
		return RobotAuthModeUserBinding
	}
	return mode
}

func (c RobotAuthorizationConfig) ExternalUserAllowed(externalUserID string) bool {
	externalUserID = strings.TrimSpace(externalUserID)
	if externalUserID == "" {
		return false
	}
	for _, allowed := range c.AllowedExternalUsers {
		if strings.TrimSpace(allowed) == externalUserID {
			return true
		}
	}
	return false
}

// RobotSessionConfig robot session isolation strategy
type RobotSessionConfig struct {
	StrictUserIdentity *bool `yaml:"strict_user_identity,omitempty" json:"strict_user_identity,omitempty"` // when true, only real user identifiers are allowed; session/group ID fallback is not permitted
}

// StrictUserIdentityEnabled returns whether strict user identity mode is enabled; defaults to true when not configured.
func (c RobotSessionConfig) StrictUserIdentityEnabled() bool {
	if c.StrictUserIdentity == nil {
		return true
	}
	return *c.StrictUserIdentity
}

// RobotWecomConfig WeComrobot configuration
type RobotWecomConfig struct {
	Enabled        bool                     `yaml:"enabled" json:"enabled"`
	Token          string                   `yaml:"token" json:"token"`                       // callback URL verification token
	EncodingAESKey string                   `yaml:"encoding_aes_key" json:"encoding_aes_key"` // EncodingAESKey
	CorpID         string                   `yaml:"corp_id" json:"corp_id"`                   // enterprise ID
	Secret         string                   `yaml:"secret" json:"secret"`                     // application secret
	AgentID        int64                    `yaml:"agent_id" json:"agent_id"`                 // application AgentId
	Auth           RobotAuthorizationConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// ValidateWecomConfig validate WeCom robot configuration; when enabled, token must be configured, otherwise callbacks cannot prevent forgery.
func ValidateWecomConfig(w RobotWecomConfig) error {
	if !w.Enabled {
		return nil
	}
	if strings.TrimSpace(w.Token) == "" {
		return fmt.Errorf("robots.wecom.token must be configured when robots.wecom.enabled is true")
	}
	return nil
}

// RobotDingtalkConfig DingTalkrobot configuration
type RobotDingtalkConfig struct {
	Enabled                     bool                     `yaml:"enabled" json:"enabled"`
	ClientID                    string                   `yaml:"client_id" json:"client_id"`                                           // application Key (AppKey)
	ClientSecret                string                   `yaml:"client_secret" json:"client_secret"`                                   // application secret
	AllowConversationIDFallback bool                     `yaml:"allow_conversation_id_fallback" json:"allow_conversation_id_fallback"` // whether to allow fallback to session ID when sender_id is missing
	Auth                        RobotAuthorizationConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// RobotLarkConfig Feishurobot configuration
type RobotLarkConfig struct {
	Enabled             bool                     `yaml:"enabled" json:"enabled"`
	AppID               string                   `yaml:"app_id" json:"app_id"`                                 // application App ID
	AppSecret           string                   `yaml:"app_secret" json:"app_secret"`                         // application App Secret
	VerifyToken         string                   `yaml:"verify_token" json:"verify_token"`                     // event subscription Verification Token (optional)
	AllowChatIDFallback bool                     `yaml:"allow_chat_id_fallback" json:"allow_chat_id_fallback"` // whether to allow fallback to chat_id when user ID is missing
	Auth                RobotAuthorizationConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// RobotTelegramConfig is the Telegram robot configuration (Bot API long polling)
type RobotTelegramConfig struct {
	Enabled            bool                     `yaml:"enabled" json:"enabled"`
	BotToken           string                   `yaml:"bot_token" json:"bot_token"`
	BotUsername        string                   `yaml:"bot_username,omitempty" json:"bot_username,omitempty"` // optional, for group chat @ recognition; if empty, getMe is called at startup
	AllowGroupMessages bool                     `yaml:"allow_group_messages" json:"allow_group_messages"`     // in group chats, only respond when @ mentioned
	UpdateOffset       int64                    `yaml:"update_offset,omitempty" json:"update_offset,omitempty"`
	Auth               RobotAuthorizationConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// RobotSlackConfig is the Slack robot configuration (Socket Mode, no public callback required)
type RobotSlackConfig struct {
	Enabled  bool                     `yaml:"enabled" json:"enabled"`
	BotToken string                   `yaml:"bot_token" json:"bot_token"` // xoxb-
	AppToken string                   `yaml:"app_token" json:"app_token"` // xapp- (connections:write)
	Auth     RobotAuthorizationConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// RobotDiscordConfig Discord robot configuration (Gateway WebSocket)
type RobotDiscordConfig struct {
	Enabled            bool                     `yaml:"enabled" json:"enabled"`
	BotToken           string                   `yaml:"bot_token" json:"bot_token"`
	AllowGuildMessages bool                     `yaml:"allow_guild_messages" json:"allow_guild_messages"` // in server channels, only respond when @ mentioned
	Auth               RobotAuthorizationConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// RobotQQConfig is the QQ robot configuration (QQ open platform WebSocket)
type RobotQQConfig struct {
	Enabled      bool                     `yaml:"enabled" json:"enabled"`
	AppID        string                   `yaml:"app_id" json:"app_id"`
	ClientSecret string                   `yaml:"client_secret" json:"client_secret"`
	Sandbox      bool                     `yaml:"sandbox" json:"sandbox"` // sandbox environment (pre-launch testing)
	Auth         RobotAuthorizationConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

func (c RobotsConfig) AuthorizationFor(platform string) RobotAuthorizationConfig {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "wechat":
		return c.Wechat.Auth
	case "wecom":
		return c.Wecom.Auth
	case "dingtalk":
		return c.Dingtalk.Auth
	case "lark":
		return c.Lark.Auth
	case "telegram":
		return c.Telegram.Auth
	case "slack":
		return c.Slack.Auth
	case "discord":
		return c.Discord.Auth
	case "qq":
		return c.QQ.Auth
	default:
		return RobotAuthorizationConfig{}
	}
}

func ValidateRobotAuthorization(c RobotAuthorizationConfig, path string) error {
	switch c.EffectiveMode() {
	case RobotAuthModeUserBinding:
		return nil
	case RobotAuthModeServiceAccount:
		serviceUserID := strings.TrimSpace(c.ServiceUserID)
		if serviceUserID == "" {
			return fmt.Errorf("%s.auth.service_user_id cannot be empty", path)
		}
		if len(c.AllowedExternalUsers) == 0 {
			return fmt.Errorf("%s.auth.allowed_external_users must configure at least one real sender", path)
		}
		seen := map[string]bool{}
		for _, userID := range c.AllowedExternalUsers {
			userID = strings.TrimSpace(userID)
			if userID == "" || userID == "*" {
				return fmt.Errorf("%s.auth.allowed_external_users does not allow null values or wildcards", path)
			}
			if seen[userID] {
				return fmt.Errorf("%s.auth.allowed_external_users contains duplicate users", path)
			}
			seen[userID] = true
		}
		return nil
	default:
		return fmt.Errorf("%s.auth.mode only supports user_binding or service_account", path)
	}
}

func ValidateRobotsAuthorization(c RobotsConfig) error {
	items := []struct {
		path string
		auth RobotAuthorizationConfig
	}{
		{"robots.wechat", c.Wechat.Auth}, {"robots.wecom", c.Wecom.Auth},
		{"robots.dingtalk", c.Dingtalk.Auth}, {"robots.lark", c.Lark.Auth},
		{"robots.telegram", c.Telegram.Auth}, {"robots.slack", c.Slack.Auth},
		{"robots.discord", c.Discord.Auth}, {"robots.qq", c.QQ.Auth},
	}
	for _, item := range items {
		if err := ValidateRobotAuthorization(item.auth, item.path); err != nil {
			return err
		}
	}
	return nil
}

func (c RobotsConfig) ServiceAccountUserIDs() map[string]string {
	out := map[string]string{}
	for _, platform := range []string{"wechat", "wecom", "dingtalk", "lark", "telegram", "slack", "discord", "qq"} {
		auth := c.AuthorizationFor(platform)
		if auth.EffectiveMode() == RobotAuthModeServiceAccount {
			out[platform] = strings.TrimSpace(auth.ServiceUserID)
		}
	}
	return out
}

type ServerConfig struct {
	Host string `yaml:"host" json:"host"`
	Port int    `yaml:"port" json:"port"`
	// CORSAllowedOrigins contains additional, exact origins that may call the API.
	// Same-origin browser requests are always allowed. Wildcards are intentionally unsupported.
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins,omitempty" json:"cors_allowed_origins,omitempty"`
	// when TLSEnabled is true, the main Web UI uses HTTPS; modern browsers negotiate HTTP/2 under the same origin, mitigating HTTP/1.1 per-origin concurrent connection limits.
	TLSEnabled bool `yaml:"tls_enabled,omitempty" json:"tls_enabled,omitempty"`
	// when TLSCertPath / TLSKeyPath are non-empty, certificates are loaded from PEM files (recommended for production).
	TLSCertPath string `yaml:"tls_cert_path,omitempty" json:"tls_cert_path,omitempty"`
	TLSKeyPath  string `yaml:"tls_key_path,omitempty" json:"tls_key_path,omitempty"`
	// when TLSAutoSelfSign is true and no valid certificate path is configured, an in-memory self-signed certificate is generated at startup (local/test only; browsers will warn about untrusted cert).
	TLSAutoSelfSign bool `yaml:"tls_auto_self_sign,omitempty" json:"tls_auto_self_sign,omitempty"`
	// when TLSHTTPRedirect is false, HTTP→HTTPS redirect is disabled; when omitted or true and HTTPS is enabled, plain HTTP access is 308-redirected to HTTPS (same port sniff-based splitting).
	TLSHTTPRedirect *bool `yaml:"tls_http_redirect,omitempty" json:"tls_http_redirect,omitempty"`
}

type LogConfig struct {
	Level                   string `yaml:"level"`
	Output                  string `yaml:"output"`
	DiagnosticDir           string `yaml:"diagnostic_dir"`
	DiagnosticDisabled      bool   `yaml:"diagnostic_disabled"`
	DiagnosticRetentionDays int    `yaml:"diagnostic_retention_days"`
}

type MCPConfig struct {
	Enabled           bool   `yaml:"enabled"`
	Host              string `yaml:"host"`
	Port              int    `yaml:"port"`
	AuthHeader        string `yaml:"auth_header,omitempty"`         // optional global service credential header; regular calls prefer user Bearer Token
	AuthHeaderValue   string `yaml:"auth_header_value,omitempty"`   // global service credential, only accepted when allow_global_access=true
	AllowGlobalAccess bool   `yaml:"allow_global_access,omitempty"` // whether static service key maps to global service identity (default disabled)
}

type OpenAIConfig struct {
	Provider            string `yaml:"provider,omitempty" json:"provider,omitempty"` // API provider: "openai" (default) or "claude"; claude uses Eino native Anthropic Messages API
	APIKey              string `yaml:"api_key" json:"api_key"`
	BaseURL             string `yaml:"base_url" json:"base_url"`
	Model               string `yaml:"model" json:"model"`
	MaxTotalTokens      int    `yaml:"max_total_tokens,omitempty" json:"max_total_tokens,omitempty"`
	MaxCompletionTokens int    `yaml:"max_completion_tokens,omitempty" json:"max_completion_tokens,omitempty"`
	// Reasoning controls Eino ChatModel's thinking / reasoning_effort / output_config etc. (takes effect for Eino single/multi-agent paths).
	Reasoning OpenAIReasoningConfig `yaml:"reasoning,omitempty" json:"reasoning,omitempty"`
}

// AIConfig stores first-class model channels. Runtime callers resolve a channel
// into OpenAIConfig at the edge instead of moving API credentials through chat requests.
type AIConfig struct {
	DefaultChannel string                     `yaml:"default_channel,omitempty" json:"default_channel,omitempty"`
	Channels       map[string]AIChannelConfig `yaml:"channels,omitempty" json:"channels,omitempty"`
}

type AIChannelConfig struct {
	Name                string                `yaml:"name,omitempty" json:"name,omitempty"`
	Provider            string                `yaml:"provider,omitempty" json:"provider,omitempty"`
	APIKey              string                `yaml:"api_key" json:"api_key"`
	BaseURL             string                `yaml:"base_url" json:"base_url"`
	Model               string                `yaml:"model" json:"model"`
	MaxTotalTokens      int                   `yaml:"max_total_tokens,omitempty" json:"max_total_tokens,omitempty"`
	MaxCompletionTokens int                   `yaml:"max_completion_tokens,omitempty" json:"max_completion_tokens,omitempty"`
	Reasoning           OpenAIReasoningConfig `yaml:"reasoning,omitempty" json:"reasoning,omitempty"`
}

func (c AIChannelConfig) ToOpenAIConfig() OpenAIConfig {
	provider := strings.TrimSpace(c.Provider)
	if provider == "" || provider == "openai_compatible" {
		provider = "openai"
	}
	return OpenAIConfig{
		Provider:            provider,
		APIKey:              c.APIKey,
		BaseURL:             c.BaseURL,
		Model:               c.Model,
		MaxTotalTokens:      c.MaxTotalTokens,
		MaxCompletionTokens: c.MaxCompletionTokens,
		Reasoning:           c.Reasoning,
	}
}

func AIChannelFromOpenAI(id, name string, oa OpenAIConfig) AIChannelConfig {
	if strings.TrimSpace(name) == "" {
		name = id
	}
	return AIChannelConfig{
		Name:                name,
		Provider:            oa.Provider,
		APIKey:              oa.APIKey,
		BaseURL:             oa.BaseURL,
		Model:               oa.Model,
		MaxTotalTokens:      oa.MaxTotalTokens,
		MaxCompletionTokens: oa.MaxCompletionTokens,
		Reasoning:           oa.Reasoning,
	}
}

func NormalizeAIChannelID(s string) string {
	id := strings.ToLower(strings.TrimSpace(s))
	id = strings.ReplaceAll(id, "_", "-")
	var b strings.Builder
	lastDash := false
	for _, r := range id {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "default"
	}
	return out
}

func (c *AIConfig) EnsureDefaultFromOpenAI(openAI OpenAIConfig) {
	if c.Channels == nil {
		c.Channels = make(map[string]AIChannelConfig)
	}
	def := NormalizeAIChannelID(c.DefaultChannel)
	if def == "default" && strings.TrimSpace(c.DefaultChannel) == "" {
		def = "default"
	}
	c.DefaultChannel = def
	if _, ok := c.Channels[def]; !ok {
		c.Channels[def] = AIChannelFromOpenAI(def, "Default", openAI)
	}
}

func (c AIConfig) ResolveChannel(channelID string) (OpenAIConfig, string, bool) {
	id := NormalizeAIChannelID(channelID)
	if strings.TrimSpace(channelID) == "" {
		id = NormalizeAIChannelID(c.DefaultChannel)
	}
	if id == "" {
		id = "default"
	}
	if c.Channels != nil {
		if ch, ok := c.Channels[id]; ok {
			return ch.ToOpenAIConfig(), id, true
		}
	}
	return OpenAIConfig{}, id, false
}

func (c *Config) ResolveAIChannel(channelID string) (OpenAIConfig, string, bool) {
	if c == nil {
		return OpenAIConfig{}, "", false
	}
	if oa, id, ok := c.AI.ResolveChannel(channelID); ok {
		return oa, id, true
	}
	return c.OpenAI, NormalizeAIChannelID(channelID), strings.TrimSpace(c.OpenAI.Model) != "" || strings.TrimSpace(c.OpenAI.BaseURL) != ""
}

func (c *Config) ApplyDefaultAIChannel() {
	if c == nil {
		return
	}
	c.NormalizeAIProviderProfiles()
	c.AI.EnsureDefaultFromOpenAI(c.OpenAI)
	if oa, _, ok := c.AI.ResolveChannel(c.AI.DefaultChannel); ok {
		c.OpenAI = oa
	}
	c.NormalizeAIProviderProfiles()
}

func (c OpenAIConfig) MaxCompletionTokensEffective() int {
	if c.MaxCompletionTokens > 0 {
		return c.MaxCompletionTokens
	}
	return DefaultMaxCompletionTokens
}

// IsDeepSeekEndpointOrModel reports whether the channel targets DeepSeek's
// official-compatible API endpoint. The historical name is kept for compatibility;
// model names alone are not enough to infer DeepSeek wire behavior behind
// OpenAI-compatible gateways.
func (c OpenAIConfig) IsDeepSeekEndpointOrModel() bool {
	baseURL := strings.ToLower(strings.TrimSpace(c.BaseURL))
	return strings.Contains(baseURL, "deepseek")
}

func (c OpenAIConfig) IsDeepSeekOfficialEndpoint() bool {
	host := normalizedURLHost(c.BaseURL)
	return host == "api.deepseek.com"
}

func normalizedURLHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		parsed, err = url.Parse("https://" + strings.TrimLeft(raw, "/"))
		if err != nil {
			return ""
		}
	}
	return strings.ToLower(strings.TrimPrefix(parsed.Hostname(), "www."))
}

func NormalizeOpenAIProviderProfile(oa *OpenAIConfig) {
	if oa == nil {
		return
	}
	if oa.IsDeepSeekOfficialEndpoint() {
		oa.Reasoning.Profile = "deepseek"
	}
}

func (c *Config) NormalizeAIProviderProfiles() {
	if c == nil {
		return
	}
	NormalizeOpenAIProviderProfile(&c.OpenAI)
	if c.AI.Channels != nil {
		for id, ch := range c.AI.Channels {
			oa := ch.ToOpenAIConfig()
			NormalizeOpenAIProviderProfile(&oa)
			ch.Reasoning = oa.Reasoning
			c.AI.Channels[id] = ch
		}
	}
}

// OpenAIReasoningConfig is the global default and gateway profile (can be overridden via ChatRequest.reasoning on the conversation page, subject to AllowClientReasoning constraint).
type OpenAIReasoningConfig struct {
	// Mode: auto (default) | on | off | default (same as auto).
	// off omits reasoning fields under OpenAI/Claude profile; sends thinking.type=disabled under DeepSeek profile (which has thinking enabled by default).
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`
	// Effort: low | medium | high | max | xhigh; max/xhigh are highest tier names for different gateways, passed as-is without conversion. Empty means do not specify effort separately.
	Effort string `yaml:"effort,omitempty" json:"effort,omitempty"`
	// when AllowClientReasoning is false, ignores request body reasoning; nil or unconfigured equals true.
	AllowClientReasoning *bool `yaml:"allow_client_reasoning,omitempty" json:"allow_client_reasoning,omitempty"`
	// Profile: auto | deepseek_compat | openai_compat | output_config_effort
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`
	// ExtraRequestFields merges into Chat Completions root JSON (admin use; if same name as auto-generated fields, the latter overrides).
	// when Mode=off, removes reasoning control fields but retains other extension fields; DeepSeek profile subsequently adds explicit disable switch.
	ExtraRequestFields map[string]interface{} `yaml:"extra_request_fields,omitempty" json:"extra_request_fields,omitempty"`
}

// ModeEffective returns auto when empty or default.
func (c OpenAIReasoningConfig) ModeEffective() string {
	m := strings.ToLower(strings.TrimSpace(c.Mode))
	if m == "" || m == "default" {
		return "auto"
	}
	return m
}

// ProfileEffective returns auto when empty.
func (c OpenAIReasoningConfig) ProfileEffective() string {
	p := strings.ToLower(strings.TrimSpace(c.Profile))
	if p == "" {
		return "auto"
	}
	return p
}

// AllowClientReasoningEffective true when client may send ChatRequest.reasoning.
func (c OpenAIReasoningConfig) AllowClientReasoningEffective() bool {
	if c.AllowClientReasoning == nil {
		return true
	}
	return *c.AllowClientReasoning
}

type FofaConfig struct {
	// APIKey is the FOFA API Key (recommend using a read-only key)
	APIKey  string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	BaseURL string `yaml:"base_url,omitempty" json:"base_url,omitempty"` // default https://fofa.info/api/v1/search/all
}

type SpaceSearchConfig struct {
	APIKey  string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	BaseURL string `yaml:"base_url,omitempty" json:"base_url,omitempty"`
}

type ProcessIsolationConfig struct {
	Mode           string `yaml:"mode" json:"mode"`
	CgroupRoot     string `yaml:"cgroup_root" json:"cgroup_root"`
	MaxProcesses   int    `yaml:"max_processes" json:"max_processes"`
	MemoryMaxBytes int64  `yaml:"memory_max_bytes" json:"memory_max_bytes"`
	CPUQuotaMicros int64  `yaml:"cpu_quota_micros" json:"cpu_quota_micros"`
}

type SecurityConfig struct {
	ProcessIsolation ProcessIsolationConfig `yaml:"process_isolation,omitempty" json:"process_isolation"`

	Tools               []ToolConfig `yaml:"tools,omitempty"`                 // backward compatible: supports defining tools in main config file
	ToolsDir            string       `yaml:"tools_dir,omitempty"`             // tool configuration file directory (new method)
	ToolDescriptionMode string       `yaml:"tool_description_mode,omitempty"` // tool description mode: "short" | "full", default short
}

type DatabaseConfig struct {
	Path            string `yaml:"path"`                        // session database path
	KnowledgeDBPath string `yaml:"knowledge_db_path,omitempty"` // knowledge base database path (optional, uses session database when empty)
}

type AgentConfig struct {
	MaxIterations                      int `yaml:"max_iterations" json:"max_iterations"`
	ToolTimeoutMinutes                 int `yaml:"tool_timeout_minutes" json:"tool_timeout_minutes"`                                     // maximum duration per tool execution (minutes), auto-terminated on timeout to prevent long hangs; 0 means unlimited (not recommended)
	ToolWaitTimeoutSeconds             int `yaml:"tool_wait_timeout_seconds" json:"tool_wait_timeout_seconds"`                           // seconds to wait for tool in this round; returns execution_id when expired, worker continues background execution; 0 means wait until complete
	ExternalMCPMaxConcurrentPerServer  int `yaml:"external_mcp_max_concurrent_per_server" json:"external_mcp_max_concurrent_per_server"` // number of tools running concurrently per external MCP server; 0 means default 2
	ExternalMCPMaxConcurrentTotal      int `yaml:"external_mcp_max_concurrent_total" json:"external_mcp_max_concurrent_total"`           // global concurrent limit for all external MCP tools; 0 means default 16
	ExternalMCPCircuitFailureThreshold int `yaml:"external_mcp_circuit_failure_threshold" json:"external_mcp_circuit_failure_threshold"` // how many consecutive failures before circuit breaker opens for a single MCP server; 0 means default 3; negative disables
	ExternalMCPCircuitCooldownSeconds  int `yaml:"external_mcp_circuit_cooldown_seconds" json:"external_mcp_circuit_cooldown_seconds"`   // cooldown seconds after circuit breaker opens; 0 means default 60
	// ShellNoOutputTimeoutSeconds is the idle termination seconds when exec/shell has no stdout/stderr (general anti-hang, no command blacklist); 0=default 300 (5 minutes); -1=disable.
	ShellNoOutputTimeoutSeconds int `yaml:"shell_no_output_timeout_seconds" json:"shell_no_output_timeout_seconds"`
	// WorkspaceRootDir is the session working directory root path (for curl/wget downloads, local read_file/glob/grep); empty=tmp/workspace, isolated by projects/{id} or conversations/{id}.
	WorkspaceRootDir string `yaml:"workspace_root_dir,omitempty" json:"workspace_root_dir,omitempty"`
	// SystemPromptPath is the single-agent system prompt Markdown/text file path (relative to config.yaml directory, or absolute). When non-empty and readable, replaces built-in single-agent prompt; leave empty to use built-in.
	SystemPromptPath string `yaml:"system_prompt_path,omitempty" json:"system_prompt_path,omitempty"`
}

// HitlConfig is the global HITL (human-in-the-loop) options; merged with session sidebar/API whitelist as a union for evaluation.
// tool_whitelist can be merged into config.yaml when 'applying' from the sidebar and takes effect immediately.
// audit_agent_prompt / audit_agent_prompt_review_edit can be edited on the HITL page and take effect immediately; empty uses built-in defaults.
type HitlConfig struct {
	// AuditBackend is the audit Agent backend: openai (compatible protocol chat model) or typesafe (Jev structured verdict). Empty value is treated as openai.
	AuditBackend string `yaml:"audit_backend,omitempty" json:"audit_backend,omitempty"`
	// AuditModel is the dedicated audit Agent model. openai backend empty field inherits main model; typesafe backend requires api_key, does not inherit main model key.
	AuditModel OpenAIConfig `yaml:"audit_model,omitempty" json:"audit_model,omitempty"`
	// ToolWhitelist is the global no-approval tool names (tools in the whitelist do not trigger HITL approval).
	ToolWhitelist []string `yaml:"tool_whitelist,omitempty" json:"tool_whitelist,omitempty"`
	// AuditAgentPrompt is the audit Agent system prompt for approval mode.
	AuditAgentPrompt string `yaml:"audit_agent_prompt,omitempty" json:"audit_agent_prompt,omitempty"`
	// AuditAgentPromptReviewEdit is the audit Agent system prompt for review_edit mode.
	AuditAgentPromptReviewEdit string `yaml:"audit_agent_prompt_review_edit,omitempty" json:"audit_agent_prompt_review_edit,omitempty"`
	// RetentionDays is the retention days for decided audit logs (hitl_interrupts non-pending); defaults to 90 when omitted; 0 means no auto-cleanup.
	RetentionDays *int `yaml:"retention_days,omitempty" json:"retention_days,omitempty"`
	// DefaultMode is the global default HITL mode (off | approval | review_edit); used for new sessions with no independent config.
	DefaultMode string `yaml:"default_mode,omitempty" json:"default_mode,omitempty"`
	// DefaultReviewer is the global default reviewer (human | audit_agent); used for new sessions with no independent config.
	DefaultReviewer string `yaml:"default_reviewer,omitempty" json:"default_reviewer,omitempty"`
	// DefaultTimeoutSeconds is the global default approval wait seconds; nil means use frontend historical default 300 seconds; 0 means unlimited.
	DefaultTimeoutSeconds *int `yaml:"default_timeout_seconds,omitempty" json:"default_timeout_seconds,omitempty"`
}

// EffectiveDefaultMode returns off, approval, or review_edit; omitted or unknown values default to off.
func (h HitlConfig) EffectiveDefaultMode() string {
	switch strings.ToLower(strings.TrimSpace(h.DefaultMode)) {
	case "feedback", "followup":
		return "approval"
	case "approval", "review_edit":
		return strings.ToLower(strings.TrimSpace(h.DefaultMode))
	default:
		return "off"
	}
}

// EffectiveDefaultReviewer returns human or audit_agent; omitted or unknown values default to human.
func (h HitlConfig) EffectiveDefaultReviewer() string {
	switch strings.ToLower(strings.TrimSpace(h.DefaultReviewer)) {
	case "audit_agent", "agent", "ai":
		return "audit_agent"
	default:
		return "human"
	}
}

// EffectiveDefaultTimeoutSeconds returns the default HITL approval timeout; nil defaults to 5 minutes.
func (h HitlConfig) EffectiveDefaultTimeoutSeconds() int {
	if h.DefaultTimeoutSeconds == nil {
		return 300
	}
	if *h.DefaultTimeoutSeconds < 0 {
		return 0
	}
	return *h.DefaultTimeoutSeconds
}

// RetentionDaysEffective returns retention; 0 means keep forever; omitted defaults to 90.
func (h HitlConfig) RetentionDaysEffective() int {
	if h.RetentionDays == nil {
		return 90
	}
	if *h.RetentionDays < 0 {
		return 0
	}
	return *h.RetentionDays
}

const (
	HitlAuditBackendOpenAI   = "openai"
	HitlAuditBackendTypeSafe = "typesafe"
	TypeSafeDefaultBaseURL   = "https://api.typesafe.ai"
	TypeSafeDefaultModel     = "jev-latest"
)

// EffectiveAuditBackend returns openai or typesafe. Omitted or unknown values default to openai.
func (h HitlConfig) EffectiveAuditBackend() string {
	switch strings.ToLower(strings.TrimSpace(h.AuditBackend)) {
	case HitlAuditBackendTypeSafe, "jev", "type-safe", "typesafe-ai":
		return HitlAuditBackendTypeSafe
	default:
		return HitlAuditBackendOpenAI
	}
}

// TypeSafeConfigEffective returns TypeSafe endpoint settings. Empty base_url/model use defaults; API key is never inherited from the main OpenAI channel.
func (h HitlConfig) TypeSafeConfigEffective() (baseURL, apiKey, model string) {
	baseURL = strings.TrimSpace(h.AuditModel.BaseURL)
	if baseURL == "" {
		baseURL = TypeSafeDefaultBaseURL
	}
	apiKey = strings.TrimSpace(h.AuditModel.APIKey)
	model = strings.TrimSpace(h.AuditModel.Model)
	if model == "" {
		model = TypeSafeDefaultModel
	}
	return strings.TrimSuffix(baseURL, "/"), apiKey, model
}

// AuditModelEffective returns the audit-agent model config with empty fields inherited from the main model config.
func (h HitlConfig) AuditModelEffective(main OpenAIConfig) OpenAIConfig {
	out := main
	am := h.AuditModel
	if strings.TrimSpace(am.Provider) != "" {
		out.Provider = strings.TrimSpace(am.Provider)
	}
	if strings.TrimSpace(am.BaseURL) != "" {
		out.BaseURL = strings.TrimSpace(am.BaseURL)
	}
	if strings.TrimSpace(am.APIKey) != "" {
		out.APIKey = strings.TrimSpace(am.APIKey)
	}
	if strings.TrimSpace(am.Model) != "" {
		out.Model = strings.TrimSpace(am.Model)
	}
	if am.MaxTotalTokens > 0 {
		out.MaxTotalTokens = am.MaxTotalTokens
	}
	if am.MaxCompletionTokens > 0 {
		out.MaxCompletionTokens = am.MaxCompletionTokens
	}
	return out
}

const hitlAuditAgentPromptBase = `You are the Kestrel human-in-the-loop audit Agent. Review tool calls the Agent is about to execute to determine whether they would cause material damage to the system.

You will receive a JSON object containing fields: hitlMode, toolName, arguments/argumentsObj, userMessage, thinking, reasoningChain, planning, etc.

Rule priority:
- Rules that must be rejected
- Rules that must be approved
- Default allow rule

Rule matching requirements:
- If both reject and approve rules are triggered simultaneously, must reject.
- Every decision must cite a rule number; must not output "matched rule: none".
- When no explicit reject or approve rule is matched but no clear destructive behavior is discovered, D1 applies: default allow.
- Do not judge a script harmless solely on "local execution", "settings timed out", or "filename contains test"; must inspect the actual command, code, or request parameters.
- If script content is not visible, state "current parameters do not show explicit critical operations" — never assert "no persistence impact".

Decision baseline (default: approve):
- The vast majority of penetration test operations (including all attack payloads) should be allowed
- Before ruling, determine: 1. What does this request actually do (understand the request content)? 2. What are the consequences of this request?
- Only reject when 'explicitly and directly disrupts normal operation of target business system, or causes real user data loss / irreversible tampering'
- Insufficient info, slightly broad parameters, probing intent only → approve
- When uncertain → approve (better to let resumable tests through than to block legitimate attacks)

Situations that must be rejected (examples, non-exhaustive):
- Modifying/resetting any user or admin password, forcibly logging out real users
- Modifying server or service configuration (system config, web/database parameters, firewall, startup items, scheduled tasks)
- Modifying/creating/deleting users, roles, or permissions (account system tampering)
- Deleting or mass-destroying data/files (DROP/TRUNCATE/DELETE full table, rm of important files, wiping the database)
- Stopping, disabling, or restarting business services (systemctl stop, kill of key processes, shutdown, reboot)
- Large-scale load testing or high concurrency causing DoS

The following situations must be approved (even if they appear "highly offensive" or include write operations):
- All types of vulnerability probing and exploitation payloads (SQL injection including read/write, command injection, deserialization, file inclusion, path traversal, SSRF, SSTI, XXE, etc.)
- File upload testing, including uploading webshells, writing webshell files
- Command execution probing, reverse shell, dnslog / data exfiltration probing
- Database command execution, writing files, writing scheduled tasks, memory shell injection and other getshell exploits
- Credential brute force at normal rate (not DoS-level)
- Info collection, port/directory scanning, enumeration, crawling
- Read, query, probe operations (including sensitive files)
- Harmless reconnaissance commands after compromise (whoami/id/uname/ls/cat etc.)
- Requests with broad parameters but no clear destructive intent

Decision principles:
- "Write" alone is not a reason to block — penetration testing relies heavily on write operations (injection, upload, brute force, getshell)
- Key question: does it disrupt 'business availability / key data integrity'
- Only reject when explicitly matching the above reject list`

const hitlAuditAgentPromptApprovalOutput = `
Output only a single line of JSON, no markdown code block:
{"decision":"approve"|"reject","comment":"actual operation: ...; post-success consequences: ...; matched rule: ..."}`

const hitlAuditAgentPromptReviewEditOutput = `
Output only a single line of JSON, no markdown code block:
{"decision":"approve"|"reject","comment":"actual operation: ...; post-success consequences: ...; matched rule: ...","editedArguments":{...}}

editedArguments rules (only fill in when approving with parameter edits; otherwise omit the field):
- Provide the complete replaced tool parameters object; key names must match argumentsObj
- Make only minimal necessary modifications to narrow scope and eliminate risk (e.g. restrict path, remove dangerous flags)
- Forbidden to expand attack surface: must not expand target scope, elevate privileges, or introduce destructive parameters
- When parameters cannot be safely modified, reject rather than force approve`

// DefaultHitlAuditAgentPrompt is the built-in approval-mode audit Agent prompt.
func DefaultHitlAuditAgentPrompt() string {
	return hitlAuditAgentPromptBase + hitlAuditAgentPromptApprovalOutput
}

// DefaultHitlAuditAgentPromptReviewEdit is the built-in review-and-edit mode audit Agent prompt.
func DefaultHitlAuditAgentPromptReviewEdit() string {
	return hitlAuditAgentPromptBase + hitlAuditAgentPromptReviewEditOutput
}

// EffectiveAuditAgentPrompt returns the audit Agent prompt effective in approval mode.
func (c HitlConfig) EffectiveAuditAgentPrompt() string {
	return c.EffectiveAuditAgentPromptForMode("approval")
}

// EffectiveAuditAgentPromptForMode returns the audit Agent prompt effective for the given HITL mode.
func (c HitlConfig) EffectiveAuditAgentPromptForMode(mode string) string {
	if normalizeHitlModeForPrompt(mode) == "review_edit" {
		if s := strings.TrimSpace(c.AuditAgentPromptReviewEdit); s != "" {
			return s
		}
		return DefaultHitlAuditAgentPromptReviewEdit()
	}
	if s := strings.TrimSpace(c.AuditAgentPrompt); s != "" {
		return s
	}
	return DefaultHitlAuditAgentPrompt()
}

// JevOperatorPolicy returns a custom audit-strategy prompt for TypeSafe Jev.
// Built-in default prompts stay encoded as Jev questions and are not copied into state.
func (c HitlConfig) JevOperatorPolicy(mode string) string {
	effective := strings.TrimSpace(c.EffectiveAuditAgentPromptForMode(mode))
	var def string
	if normalizeHitlModeForPrompt(mode) == "review_edit" {
		def = strings.TrimSpace(DefaultHitlAuditAgentPromptReviewEdit())
	} else {
		def = strings.TrimSpace(DefaultHitlAuditAgentPrompt())
	}
	if effective == "" || effective == def {
		return ""
	}
	return effective
}

func normalizeHitlModeForPrompt(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "review_edit":
		return "review_edit"
	default:
		return "approval"
	}
}

type AuthConfig struct {
	SessionDurationHours int `yaml:"session_duration_hours" json:"session_duration_hours"`
}

// MonitorConfig holds the MCP status monitoring (tool_executions) retention policy.
type MonitorConfig struct {
	// RetentionDays is the number of days to retain execution records; defaults to 90 if omitted; 0 disables automatic cleanup.
	RetentionDays *int `yaml:"retention_days,omitempty" json:"retention_days,omitempty"`
}

// RetentionDaysEffective returns retention; 0 means keep forever; omitted defaults to 90.
func (m MonitorConfig) RetentionDaysEffective() int {
	if m.RetentionDays == nil {
		return 90
	}
	if *m.RetentionDays < 0 {
		return 0
	}
	return *m.RetentionDays
}

// StorageCategoryKeys lists the cleanup category keys. Order defines the frontend display order; do not rely on map iteration order.
const (
	StorageCategoryWorkspace            = "workspace"
	StorageCategoryReduction            = "reduction"
	StorageCategoryConversationArtifact = "conversation_artifacts"
	StorageCategoryPlantask             = "plantask"
	StorageCategoryC2Artifacts          = "c2_artifacts"
	StorageCategoryChatUploads          = "chat_uploads"
	StorageCategoryWorkflowCheckpoints  = "workflow_checkpoints"
	StorageCategoryDiagnosticLogs       = "diagnostic_logs"
)

// StorageCategoryOrder lists all cleanable categories for stable UI and report ordering.
var StorageCategoryOrder = []string{
	StorageCategoryWorkspace,
	StorageCategoryReduction,
	StorageCategoryConversationArtifact,
	StorageCategoryPlantask,
	StorageCategoryC2Artifacts,
	StorageCategoryChatUploads,
	StorageCategoryWorkflowCheckpoints,
	StorageCategoryDiagnosticLogs,
}

// StorageCategoryDefaults holds the default retention days per category. Shorter values apply to pure derived artifacts
// (reduction/checkpoint); longer values apply to uploaded files that may still need human review.
var StorageCategoryDefaults = map[string]int{
	StorageCategoryWorkspace:            30,
	StorageCategoryReduction:            7,
	StorageCategoryConversationArtifact: 30,
	StorageCategoryPlantask:             30,
	StorageCategoryC2Artifacts:          30,
	StorageCategoryChatUploads:          90,
	StorageCategoryWorkflowCheckpoints:  7,
	StorageCategoryDiagnosticLogs:       14,
}

// StorageCategoryConfig holds the policy override for a single cleanup category.
type StorageCategoryConfig struct {
	// Enabled defaults to true if omitted; explicit false means the category is excluded from both automatic and manual cleanup.
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	// RetentionDays uses StorageCategoryDefaults if omitted; 0 disables retention-based cleanup (orphan directories are still reclaimed).
	RetentionDays *int `yaml:"retention_days,omitempty" json:"retention_days,omitempty"`
}

// StorageConfig defines the runtime garbage cleanup policy.
// Automatic cleanup is disabled by default: consistent with Argo ttlStrategy / K8s ttlSecondsAfterFinished unset semantics,
// so existing data will not be deleted without administrator awareness after an upgrade.
type StorageConfig struct {
	// AutoClean disables background automatic cleanup if omitted or false; must be explicitly true to enable.
	AutoClean *bool `yaml:"auto_clean,omitempty" json:"auto_clean,omitempty"`
	// IntervalMinutes is the background cleanup polling interval; defaults to 60 if omitted, minimum 5.
	IntervalMinutes *int `yaml:"interval_minutes,omitempty" json:"interval_minutes,omitempty"`
	// OrphanGraceDays is the minimum retention days when a session/project is deleted but the directory remains; defaults to 1.
	OrphanGraceDays *int `yaml:"orphan_grace_days,omitempty" json:"orphan_grace_days,omitempty"`
	// ActiveGraceHours skips cleanup for sessions with recent activity; defaults to 24.
	ActiveGraceHours *int `yaml:"active_grace_hours,omitempty" json:"active_grace_hours,omitempty"`
	// Categories overrides policy per category; unlisted categories use built-in defaults.
	Categories map[string]StorageCategoryConfig `yaml:"categories,omitempty" json:"categories,omitempty"`
}

// AutoCleanEffective returns true only when storage.auto_clean is explicitly true.
func (s StorageConfig) AutoCleanEffective() bool {
	return s.AutoClean != nil && *s.AutoClean
}

// IntervalMinutesEffective returns the background sweep interval; defaults to 60, floor 5.
func (s StorageConfig) IntervalMinutesEffective() int {
	if s.IntervalMinutes == nil {
		return 60
	}
	if *s.IntervalMinutes < 5 {
		return 5
	}
	return *s.IntervalMinutes
}

// OrphanGraceDaysEffective returns the minimum age before an orphaned session dir is reclaimed; defaults to 1.
func (s StorageConfig) OrphanGraceDaysEffective() int {
	if s.OrphanGraceDays == nil {
		return 1
	}
	if *s.OrphanGraceDays < 0 {
		return 0
	}
	return *s.OrphanGraceDays
}

// ActiveGraceHoursEffective returns the recent-activity protection window; defaults to 24, floor 1.
func (s StorageConfig) ActiveGraceHoursEffective() int {
	if s.ActiveGraceHours == nil {
		return 24
	}
	if *s.ActiveGraceHours < 1 {
		return 1
	}
	return *s.ActiveGraceHours
}

// CategoryEnabled reports whether a category participates in cleanup; unknown keys default to false.
func (s StorageConfig) CategoryEnabled(key string) bool {
	if _, known := StorageCategoryDefaults[key]; !known {
		return false
	}
	if c, ok := s.Categories[key]; ok && c.Enabled != nil {
		return *c.Enabled
	}
	return true
}

// CategoryRetentionDays returns the effective retention for a category; unknown keys yield 0 (keep forever).
func (s StorageConfig) CategoryRetentionDays(key string) int {
	def, known := StorageCategoryDefaults[key]
	if !known {
		return 0
	}
	c, ok := s.Categories[key]
	if !ok || c.RetentionDays == nil {
		return def
	}
	if *c.RetentionDays < 0 {
		return 0
	}
	return *c.RetentionDays
}

// AuditConfig platform operation audit log settings (not chat/tool execution bodies).
type AuditConfig struct {
	// Enabled nil or true enables persistence; explicit false disables.
	Enabled        *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	RetentionDays  int   `yaml:"retention_days,omitempty" json:"retention_days,omitempty"`
	MaxDetailBytes int   `yaml:"max_detail_bytes,omitempty" json:"max_detail_bytes,omitempty"`
	// AuthFailureCooldownSeconds: per-IP cooldown for auth login/change_password failure audit rows; -1 disables; 0 uses default 60.
	AuthFailureCooldownSeconds int `yaml:"auth_failure_cooldown_seconds,omitempty" json:"auth_failure_cooldown_seconds,omitempty"`
}

// EnabledEffective returns true unless audit.enabled is explicitly false.
func (a AuditConfig) EnabledEffective() bool {
	if a.Enabled == nil {
		return true
	}
	return *a.Enabled
}

// RetentionDaysEffective returns retention; 0 means keep forever.
func (a AuditConfig) RetentionDaysEffective() int {
	if a.RetentionDays < 0 {
		return 0
	}
	return a.RetentionDays
}

// MaxDetailBytesEffective caps serialized detail JSON size.
func (a AuditConfig) MaxDetailBytesEffective() int {
	if a.MaxDetailBytes <= 0 {
		return 8192
	}
	return a.MaxDetailBytes
}

// AuthFailureCooldownEffective returns seconds between duplicate auth-failure audit rows per IP (default 60; -1 disables).
func (a AuditConfig) AuthFailureCooldownEffective() int {
	if a.AuthFailureCooldownSeconds < 0 {
		return 0
	}
	if a.AuthFailureCooldownSeconds == 0 {
		return 60
	}
	return a.AuthFailureCooldownSeconds
}

// ExternalMCPConfig external MCP configuration
type ExternalMCPConfig struct {
	Servers map[string]ExternalMCPServerConfig `yaml:"servers,omitempty" json:"servers,omitempty"`
}

// ExternalMCPServerConfig is the external MCP server config (follows the official MCP config format, compatible with Claude Desktop / Cursor / VS Code).
// All string fields support ${VAR} and ${VAR:-default} environment variable expansion syntax.
type ExternalMCPServerConfig struct {
	// Transport type: "stdio" | "sse" | "http" (Streamable HTTP).
	// stdio pattern may be omitted; it is inferred automatically when a command field is present.
	Type string `yaml:"type,omitempty" json:"type,omitempty"`

	// stdio patternconfig
	Command string            `yaml:"command,omitempty" json:"command,omitempty"`
	Args    []string          `yaml:"args,omitempty" json:"args,omitempty"`
	Env     map[string]string `yaml:"env,omitempty" json:"env,omitempty"`

	// HTTP/SSE patternconfig
	URL     string            `yaml:"url,omitempty" json:"url,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`

	// Official standard fields
	Disabled    bool     `yaml:"disabled,omitempty" json:"disabled,omitempty"`       // disable server (official field)
	AutoApprove []string `yaml:"autoApprove,omitempty" json:"autoApprove,omitempty"` // auto-approve tool list (official field)

	// SDK advanced config (corresponds to MCP Go SDK transport-layer parameters)
	MaxRetries        int `yaml:"max_retries,omitempty" json:"max_retries,omitempty"`               // Streamable HTTP reconnect retry count on disconnect (default 5)
	TerminateDuration int `yaml:"terminate_duration,omitempty" json:"terminate_duration,omitempty"` // stdio process graceful shutdown wait seconds (default 5)
	KeepAlive         int `yaml:"keep_alive,omitempty" json:"keep_alive,omitempty"`                 // client heartbeat interval seconds (0 = disabled)

	// General config
	Description       string          `yaml:"description,omitempty" json:"description,omitempty"`
	Timeout           int             `yaml:"timeout,omitempty" json:"timeout,omitempty"`                         // connection timeout (seconds)
	ExternalMCPEnable bool            `yaml:"external_mcp_enable,omitempty" json:"external_mcp_enable,omitempty"` // enabled
	ToolEnabled       map[string]bool `yaml:"tool_enabled,omitempty" json:"tool_enabled,omitempty"`               // per-tool enable status
}

// GetTransportType returns the effective transport type. Reads Type first; infers from Command/URL if not set.
func (c ExternalMCPServerConfig) GetTransportType() string {
	if c.Type != "" {
		return c.Type
	}
	if c.Command != "" {
		return "stdio"
	}
	if c.URL != "" {
		return "http"
	}
	return ""
}

type ToolConfig struct {
	Name             string            `yaml:"name"`
	Command          string            `yaml:"command"`
	Args             []string          `yaml:"args,omitempty"`              // Fixed arguments (optional)
	ShortDescription string            `yaml:"short_description,omitempty"` // Short description (used in tool list to reduce token consumption)
	Description      string            `yaml:"description"`                 // Detailed description (used in tool documentation)
	Enabled          bool              `yaml:"enabled"`
	Parameters       []ParameterConfig `yaml:"parameters,omitempty"`         // Parameter definitions (optional)
	ArgMapping       string            `yaml:"arg_mapping,omitempty"`        // Argument mapping mode: "auto", "manual", "template" (optional)
	AllowedExitCodes []int             `yaml:"allowed_exit_codes,omitempty"` // Allowed exit codes (some tools return non-zero exit codes on success)
}

// ParameterConfig holds a parameter definition
type ParameterConfig struct {
	Name        string      `yaml:"name"`                // Parameter name
	Type        string      `yaml:"type"`                // Parameter type: string, int, bool, array
	Description string      `yaml:"description"`         // Parameter description
	Required    bool        `yaml:"required,omitempty"`  // Whether the parameter is required
	Default     interface{} `yaml:"default,omitempty"`   // default value
	ItemType    string      `yaml:"item_type,omitempty"` // When type is array, the element type, e.g. string, number, object
	Flag        string      `yaml:"flag,omitempty"`      // Command-line flag, e.g. "-u", "--url", "-p"
	Position    *int        `yaml:"position,omitempty"`  // Position of positional argument (0-based)
	Format      string      `yaml:"format,omitempty"`    // Parameter format: "flag", "positional", "combined" (flag=value), "template"ate"
	Template    string      `yaml:"template,omitempty"`  // Template string, e.g. "{flag} {value}" or "{value}"
	Options     []string    `yaml:"options,omitempty"`   // Optional value list (for enumerations)
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse configuration file: %w", err)
	}
	if cfg.ToolGuard != nil {
		if err := validateToolGuardYAML(data); err != nil {
			return nil, fmt.Errorf("interceptor config is invalid: %w", err)
		}
	}
	if _, err := toolguard.Compile(cfg.EffectiveToolGuard()); err != nil {
		return nil, fmt.Errorf("interceptor config is invalid: %w", err)
	}

	if cfg.Auth.SessionDurationHours <= 0 {
		cfg.Auth.SessionDurationHours = 12
	}
	if cfg.Audit.MaxDetailBytes <= 0 {
		cfg.Audit.MaxDetailBytes = 8192
	}
	cfg.NormalizeAIProviderProfiles()
	cfg.ApplyDefaultAIChannel()
	if err := validateOpenAIOutputLimits(cfg.OpenAI); err != nil {
		return nil, err
	}
	// If a tools directory is configured, load tool configs from that directory
	if cfg.Security.ToolsDir != "" {
		inlineTools := append([]ToolConfig(nil), cfg.Security.Tools...)
		toolsDir := ResolveToolsDir(cfg.Security.ToolsDir, path)
		merged, err := MergeToolsFromDir(toolsDir, inlineTools)
		if err != nil {
			return nil, fmt.Errorf("failed to load tool config from tools directory: %w", err)
		}
		cfg.Security.Tools = merged
	}

	// External MCP: migrate + expand environment variables
	if cfg.ExternalMCP.Servers != nil {
		for name, serverCfg := range cfg.ExternalMCP.Servers {
			// official disabled field → ExternalMCPEnable
			if serverCfg.Disabled {
				serverCfg.ExternalMCPEnable = false
			} else if !serverCfg.ExternalMCPEnable {
				// enabled by default
				serverCfg.ExternalMCPEnable = true
			}

			// expand all ${VAR} / ${VAR:-default} environment variable references
			ExpandConfigEnv(&serverCfg)

			cfg.ExternalMCP.Servers[name] = serverCfg
		}
	}

	// Load role configs from the roles directory
	if cfg.RolesDir != "" {
		configDir := filepath.Dir(path)
		rolesDir := cfg.RolesDir

		// If it is a relative path, resolve relative to the configuration file directory
		if !filepath.IsAbs(rolesDir) {
			rolesDir = filepath.Join(configDir, rolesDir)
		}

		roles, err := LoadRolesFromDir(rolesDir)
		if err != nil {
			return nil, fmt.Errorf("failed to load role config from roles directory: %w", err)
		}

		cfg.Roles = roles
	} else {
		// if roles_dir is not configured, initialize to empty map
		if cfg.Roles == nil {
			cfg.Roles = make(map[string]RoleConfig)
		}
	}

	if err := ValidateWecomConfig(cfg.Robots.Wecom); err != nil {
		return nil, err
	}
	if err := ValidateRobotsAuthorization(cfg.Robots); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func validateOpenAIOutputLimits(openAI OpenAIConfig) error {
	if openAI.MaxCompletionTokens < 0 {
		return fmt.Errorf("openai.max_completion_tokens must be a positive number")
	}
	return nil
}

func EnsureLocalConfig(path string) (EnsureLocalConfigResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "config.yaml"
	}

	if _, err := os.Stat(path); err == nil {
		return EnsureLocalConfigResult{}, nil
	} else if !os.IsNotExist(err) {
		return EnsureLocalConfigResult{}, fmt.Errorf("checkconfiguration filefailed: %w", err)
	}

	examplePath := filepath.Join(filepath.Dir(path), "config.example.yaml")
	if _, err := os.Stat(examplePath); err != nil {
		if os.IsNotExist(err) {
			if alt := "config.example.yaml"; examplePath != alt {
				if _, altErr := os.Stat(alt); altErr == nil {
					examplePath = alt
				} else {
					return EnsureLocalConfigResult{}, fmt.Errorf("configuration file %s does not exist and template %s was not found", path, examplePath)
				}
			} else {
				return EnsureLocalConfigResult{}, fmt.Errorf("configuration file %s does not exist and template %s was not found", path, examplePath)
			}
		} else {
			return EnsureLocalConfigResult{}, fmt.Errorf("check config template failed: %w", err)
		}
	}

	data, err := os.ReadFile(examplePath)
	if err != nil {
		return EnsureLocalConfigResult{}, fmt.Errorf("read config template failed: %w", err)
	}

	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return EnsureLocalConfigResult{}, fmt.Errorf("create config directory failed: %w", err)
		}
	}
	if err := os.WriteFile(path, data, fs.FileMode(0600)); err != nil {
		return EnsureLocalConfigResult{}, fmt.Errorf("createconfiguration filefailed: %w", err)
	}

	return EnsureLocalConfigResult{
		Created:     true,
		ExamplePath: examplePath,
	}, nil
}

func PrintBootstrapAdminPassword(password string) {
	termout.PrintBootstrapAdminCredentials(password)
}

// generateRandomToken generates a random string for MCP authentication (64-character hex)
func generateRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// persistMCPAuth writes the MCP auth_header / auth_header_value back to the configuration file
func persistMCPAuth(path string, mcp *MCPConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	inMcpBlock := false
	mcpIndent := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inMcpBlock {
			if strings.HasPrefix(trimmed, "mcp:") {
				inMcpBlock = true
				mcpIndent = len(line) - len(strings.TrimLeft(line, " "))
			}
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		leadingSpaces := len(line) - len(strings.TrimLeft(line, " "))
		if leadingSpaces <= mcpIndent {
			inMcpBlock = false
			mcpIndent = -1
			if strings.HasPrefix(trimmed, "mcp:") {
				inMcpBlock = true
				mcpIndent = leadingSpaces
			}
			continue
		}

		prefix := line[:leadingSpaces]
		rest := strings.TrimSpace(line[leadingSpaces:])
		comment := ""
		if idx := strings.Index(line, "#"); idx >= 0 {
			comment = strings.TrimRight(line[idx:], " ")
		}
		withComment := ""
		if comment != "" {
			if !strings.HasPrefix(comment, " ") {
				withComment = " "
			}
			withComment += comment
		}

		if strings.HasPrefix(rest, "auth_header_value:") {
			lines[i] = fmt.Sprintf("%sauth_header_value: %q%s", prefix, mcp.AuthHeaderValue, withComment)
		} else if strings.HasPrefix(rest, "auth_header:") {
			lines[i] = fmt.Sprintf("%sauth_header: %q%s", prefix, mcp.AuthHeader, withComment)
		}
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// EnsureMCPAuth only provisions the privileged static service credential when
// global service access was explicitly enabled.
func EnsureMCPAuth(path string, cfg *Config) error {
	if !cfg.MCP.Enabled || !cfg.MCP.AllowGlobalAccess || strings.TrimSpace(cfg.MCP.AuthHeaderValue) != "" {
		return nil
	}
	token, err := generateRandomToken()
	if err != nil {
		return fmt.Errorf("generate MCP auth key failed: %w", err)
	}
	cfg.MCP.AuthHeaderValue = token
	if strings.TrimSpace(cfg.MCP.AuthHeader) == "" {
		cfg.MCP.AuthHeader = "X-MCP-Token"
	}
	return persistMCPAuth(path, &cfg.MCP)
}

// PrintMCPConfigJSON prints the MCP config JSON to the terminal, ready to copy into Cursor / Claude Code mcp config
func PrintMCPConfigJSON(mcp MCPConfig) {
	if !mcp.Enabled {
		return
	}
	hostForURL := strings.TrimSpace(mcp.Host)
	if hostForURL == "" || hostForURL == "0.0.0.0" {
		hostForURL = "localhost"
	}
	url := fmt.Sprintf("http://%s:%d/mcp", hostForURL, mcp.Port)
	headers := map[string]string{"Authorization": "Bearer <USER_SESSION_TOKEN>"}
	if mcp.AllowGlobalAccess && mcp.AuthHeader != "" {
		delete(headers, "Authorization")
		headers[mcp.AuthHeader] = mcp.AuthHeaderValue
	}
	serverEntry := map[string]interface{}{
		"url": url,
	}
	if len(headers) > 0 {
		serverEntry["headers"] = headers
	}
	// Claude Code requires type: "http"
	serverEntry["type"] = "http"
	out := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"kestrel": serverEntry,
		},
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println("[Kestrel] MCP config (copy into Cursor / Claude Code):")
	fmt.Println("  Cursor: place in mcpServers of ~/.cursor/mcp.json, or project .cursor/mcp.json")
	fmt.Println("  Claude Code: place in mcpServers of .mcp.json or ~/.claude.json")
	fmt.Println("----------------------------------------------------------------")
	fmt.Println(string(b))
	fmt.Println("----------------------------------------------------------------")
}

// ResolveToolsDir resolves tools_dir to an absolute path (relative paths are relative to the directory containing configPath).
func ResolveToolsDir(toolsDir, configPath string) string {
	toolsDir = strings.TrimSpace(toolsDir)
	if toolsDir == "" {
		return ""
	}
	if filepath.IsAbs(toolsDir) {
		return toolsDir
	}
	return filepath.Join(filepath.Dir(configPath), toolsDir)
}

// MergeToolsFromDir loads tools from a directory and merges them with the inline list: directory tools take priority, inline config tools supplement.
func MergeToolsFromDir(toolsDir string, inlineTools []ToolConfig) ([]ToolConfig, error) {
	dirTools, err := LoadToolsFromDir(toolsDir)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]bool, len(dirTools))
	for _, tool := range dirTools {
		existing[tool.Name] = true
	}
	merged := append([]ToolConfig(nil), dirTools...)
	for _, tool := range inlineTools {
		if !existing[tool.Name] {
			merged = append(merged, tool)
		}
	}
	return merged, nil
}

// loadInlineSecurityToolsFromYAML reads security.tools from config.yaml (excluding tools_dir scan results).
func loadInlineSecurityToolsFromYAML(configPath string) ([]ToolConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}
	var partial struct {
		Security struct {
			Tools []ToolConfig `yaml:"tools"`
		} `yaml:"security"`
	}
	if err := yaml.Unmarshal(data, &partial); err != nil {
		return nil, fmt.Errorf("failed to parse configuration file: %w", err)
	}
	if partial.Security.Tools == nil {
		return []ToolConfig{}, nil
	}
	return partial.Security.Tools, nil
}

// ReloadSecurityToolsFromDir reloads tools from tools_dir and updates cfg.Security.Tools (used for ApplyConfig hot-reload).
func ReloadSecurityToolsFromDir(cfg *Config, configPath string) error {
	if cfg == nil || strings.TrimSpace(cfg.Security.ToolsDir) == "" {
		return nil
	}
	inlineTools, err := loadInlineSecurityToolsFromYAML(configPath)
	if err != nil {
		return err
	}
	toolsDir := ResolveToolsDir(cfg.Security.ToolsDir, configPath)
	merged, err := MergeToolsFromDir(toolsDir, inlineTools)
	if err != nil {
		return fmt.Errorf("load tool config from tools directory failed: %w", err)
	}
	cfg.Security.Tools = merged
	return nil
}

// LoadToolsFromDir loads all tool configuration files from a directory
func LoadToolsFromDir(dir string) ([]ToolConfig, error) {
	var tools []ToolConfig

	// check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return tools, nil // return empty list if directory does not exist, no error
	}

	// read all .yaml and .yml files in the directory
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read tools directory failed: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		filePath := filepath.Join(dir, name)
		tool, err := LoadToolFromFile(filePath)
		if err != nil {
			// log error but continue loading other files
			fmt.Printf("warning: load tool configuration file %s failed: %v\n", filePath, err)
			continue
		}

		tools = append(tools, *tool)
	}

	return tools, nil
}

// LoadToolFromFile loads a tool config from a single file
func LoadToolFromFile(path string) (*ToolConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file failed: %w", err)
	}

	var tool ToolConfig
	if err := yaml.Unmarshal(data, &tool); err != nil {
		return nil, fmt.Errorf("parse tool config failed: %w", err)
	}

	// validate required fields
	if tool.Name == "" {
		return nil, fmt.Errorf("tool name cannot be empty")
	}
	if tool.Command == "" {
		return nil, fmt.Errorf("tool command cannot be empty")
	}

	return &tool, nil
}

// LoadRolesFromDir loads all role configuration files from a directory
func LoadRolesFromDir(dir string) (map[string]RoleConfig, error) {
	roles := make(map[string]RoleConfig)

	// check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return roles, nil // return empty map if directory does not exist, no error
	}

	// read all .yaml and .yml files in the directory
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read roles directory failed: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		filePath := filepath.Join(dir, name)
		role, err := LoadRoleFromFile(filePath)
		if err != nil {
			// log error but continue loading other files
			fmt.Printf("warning: load role configuration file %s failed: %v\n", filePath, err)
			continue
		}

		// use role name as key
		roleName := role.Name
		if roleName == "" {
			// if role name is empty, use the filename (without extension) as the name
			roleName = strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
			role.Name = roleName
		}

		roles[roleName] = *role
	}

	return roles, nil
}

// LoadRoleFromFile loads a role config from a single file
func LoadRoleFromFile(path string) (*RoleConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file failed: %w", err)
	}

	var role RoleConfig
	if err := yaml.Unmarshal(data, &role); err != nil {
		return nil, fmt.Errorf("parse role config failed: %w", err)
	}

	// Handle icon field: if it contains Unicode escape format (\U0001F3C6), convert to actual Unicode character
	// Go's yaml library may not automatically parse \U escape sequences, so manual conversion is needed
	if role.Icon != "" {
		icon := role.Icon
		// strip any surrounding quotes
		icon = strings.Trim(icon, `"`)

		// check for Unicode escape format \U0001F3C6 (8-digit hex) or \uXXXX (4-digit hex)
		if len(icon) >= 3 && icon[0] == '\\' {
			if icon[1] == 'U' && len(icon) >= 10 {
				// \U0001F3C6 format (8-digit hex)
				if codePoint, err := strconv.ParseInt(icon[2:10], 16, 32); err == nil {
					role.Icon = string(rune(codePoint))
				}
			} else if icon[1] == 'u' && len(icon) >= 6 {
				// \uXXXX format (4-digit hex)
				if codePoint, err := strconv.ParseInt(icon[2:6], 16, 32); err == nil {
					role.Icon = string(rune(codePoint))
				}
			}
		}
	}

	// validate required fields
	if role.Name == "" {
		// if name is empty, try to derive from filename
		baseName := filepath.Base(path)
		role.Name = strings.TrimSuffix(strings.TrimSuffix(baseName, ".yaml"), ".yml")
	}

	return &role, nil
}

func Default() *Config {
	strictRobotIdentity := true
	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Log: LogConfig{
			Level:  "info",
			Output: "stdout",
		},
		MCP: MCPConfig{
			Enabled: false,
			Host:    "127.0.0.1",
			Port:    8081,
		},
		AI: AIConfig{
			DefaultChannel: "default",
			Channels: map[string]AIChannelConfig{
				"default": {
					Name:                "Default",
					Provider:            "openai_compatible",
					BaseURL:             "https://api.openai.com/v1",
					Model:               "gpt-4",
					MaxTotalTokens:      120000,
					MaxCompletionTokens: DefaultMaxCompletionTokens,
				},
			},
		},
		OpenAI: OpenAIConfig{},
		Agent: AgentConfig{
			MaxIterations:                      30,  // default maximum iterations
			ToolTimeoutMinutes:                 10,  // single tool execution default max 10 minutes to avoid abnormal long occupation
			ToolWaitTimeoutSeconds:             60,  // external MCP tool waits at most 60 seconds per round; after timeout returns execution_id for continued waiting
			ExternalMCPMaxConcurrentPerServer:  2,   // default max 2 concurrent tool executions per external MCP server
			ExternalMCPMaxConcurrentTotal:      16,  // global default max 16 concurrent external MCP tool executions
			ExternalMCPCircuitFailureThreshold: 3,   // temporarily open circuit after 3 consecutive failures for a single server
			ExternalMCPCircuitCooldownSeconds:  60,  // default circuit breaker cooldown 60 seconds
			ShellNoOutputTimeoutSeconds:        300, // execute/exec idle termination after no new output (seconds); -1 to disable
		},
		Security: SecurityConfig{
			Tools:    []ToolConfig{}, // tool config should be loaded from config.yaml or the tools/ directory
			ToolsDir: "tools",        // default tools directory
		},
		Database: DatabaseConfig{
			Path:            "data/conversations.db",
			KnowledgeDBPath: "data/knowledge.db", // default knowledge base database path
		},
		Auth: AuthConfig{
			SessionDurationHours: 12,
		},
		Audit: func() AuditConfig {
			on := true
			return AuditConfig{
				RetentionDays:  90,
				MaxDetailBytes: 8192,
				Enabled:        &on,
			}
		}(),
		Monitor: func() MonitorConfig {
			days := 90
			return MonitorConfig{RetentionDays: &days}
		}(),
		Robots: RobotsConfig{
			Session: RobotSessionConfig{
				StrictUserIdentity: &strictRobotIdentity,
			},
		},
		Knowledge: KnowledgeConfig{
			Enabled:  true,
			BasePath: "knowledge_base",
			Embedding: EmbeddingConfig{
				Provider: "openai",
				Model:    "text-embedding-3-small",
				BaseURL:  "https://api.openai.com/v1",
			},
			Retrieval: RetrievalConfig{
				TopK:                5,
				SimilarityThreshold: 0.65,
				MultiQuery:          MultiQueryConfig{MaxQueries: 4},
				Rerank:              RerankConfig{},
				PostRetrieve: PostRetrieveConfig{
					PrefetchTopK: 20,
				},
			},
			Indexing: IndexingConfig{
				ChunkStrategy:         "markdown_then_recursive",
				RequestTimeoutSeconds: 120,
				ChunkSize:             768, // increased to 768 for better context retention
				ChunkOverlap:          50,
				MaxChunksPerItem:      20, // limit to 20 chunks per knowledge item to avoid consuming too many quota
				BatchSize:             64,
				PreferSourceFile:      false,
				MaxRPM:                100, // default 100 RPM to avoid 429 errors
				RateLimitDelayMs:      600, // 600ms interval, corresponding to 100 RPM
				MaxRetries:            3,
				RetryDelayMs:          1000,
				SubIndexes:            nil,
			},
		},
	}
}

// C2Config is the built-in C2 module toggle (same semantics as knowledge base enabled: when disabled, listeners are not initialized and C2 MCP tools are not registered).
type C2Config struct {
	// Enabled being nil means not configured, treated as true (for backward compatibility with old config.yaml)
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

// EnabledEffective returns whether C2 is enabled; defaults to true when not explicitly configured.
func (c C2Config) EnabledEffective() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// C2Public returns the C2 status for the frontend (scalars only).
type C2Public struct {
	Enabled bool `json:"enabled"`
}

// Public converts the internal config to an API response.
func (c C2Config) Public() C2Public {
	return C2Public{Enabled: c.EnabledEffective()}
}

// C2APIUpdate is the settings page/API payload for updating the C2 toggle.
type C2APIUpdate struct {
	Enabled bool `json:"enabled"`
}

// KnowledgeConfig knowledge base configuration
type KnowledgeConfig struct {
	Enabled   bool            `yaml:"enabled" json:"enabled"`     // enabledKnowledge retrieval
	BasePath  string          `yaml:"base_path" json:"base_path"` // knowledge base path
	Embedding EmbeddingConfig `yaml:"embedding" json:"embedding"`
	Retrieval RetrievalConfig `yaml:"retrieval" json:"retrieval"`
	Indexing  IndexingConfig  `yaml:"indexing,omitempty" json:"indexing,omitempty"` // index build config
}

// IndexingConfig is the index build config (controls behavior during knowledge base index construction)
type IndexingConfig struct {
	// ChunkStrategy: "markdown_then_recursive" (default, Eino Markdown heading split then recursive) or "recursive" (recursive split only)
	ChunkStrategy string `yaml:"chunk_strategy,omitempty" json:"chunk_strategy,omitempty"`
	// RequestTimeoutSeconds is the embedding HTTP client timeout (seconds); 0 means use default 120
	RequestTimeoutSeconds int `yaml:"request_timeout_seconds,omitempty" json:"request_timeout_seconds,omitempty"`
	// chunking config
	ChunkSize        int `yaml:"chunk_size,omitempty" json:"chunk_size,omitempty"`                   // maximum tokens per chunk (estimated), default 512
	ChunkOverlap     int `yaml:"chunk_overlap,omitempty" json:"chunk_overlap,omitempty"`             // overlapping tokens between chunks, default 50
	MaxChunksPerItem int `yaml:"max_chunks_per_item,omitempty" json:"max_chunks_per_item,omitempty"` // max chunks per knowledge item, 0 means unlimited

	// PreferSourceFile: when true, prefer using Eino FileLoader to read the original from file_path before indexing (disk takes precedence when content differs from the store)
	PreferSourceFile bool `yaml:"prefer_source_file,omitempty" json:"prefer_source_file,omitempty"`

	// rate limit config (to avoid API rate limiting)
	RateLimitDelayMs int `yaml:"rate_limit_delay_ms,omitempty" json:"rate_limit_delay_ms,omitempty"` // request interval (ms); 0 means no fixed delay
	MaxRPM           int `yaml:"max_rpm,omitempty" json:"max_rpm,omitempty"`                         // max requests per minute; 0 means unlimited

	// retry config (for handling transient errors)
	MaxRetries   int `yaml:"max_retries,omitempty" json:"max_retries,omitempty"`       // max retries, default 3
	RetryDelayMs int `yaml:"retry_delay_ms,omitempty" json:"retry_delay_ms,omitempty"` // retry interval (ms), default 1000

	// BatchSize: embedding batch size (SQLite index writes); 0 means default 64
	BatchSize int `yaml:"batch_size,omitempty" json:"batch_size,omitempty"`
	// SubIndexes passed to Eino indexer.WithSubIndexes (logical partition tags, propagated with Document metadata)
	SubIndexes []string `yaml:"sub_indexes,omitempty" json:"sub_indexes,omitempty"`
}

// EmbeddingConfig embedding configuration
type EmbeddingConfig struct {
	Provider string `yaml:"provider" json:"provider"` // embedding model provider
	Model    string `yaml:"model" json:"model"`       // modelname
	BaseURL  string `yaml:"base_url" json:"base_url"` // API Base URL
	APIKey   string `yaml:"api_key" json:"api_key"`   // API Key (inherited from OpenAI config)
}

// PostRetrieveConfig is post-retrieval processing: fixed normalised deduplication of body content (best practice) and context budget truncation; PrefetchTopK fetches more candidates and then converges to top_k.
type PostRetrieveConfig struct {
	// PrefetchTopK: max candidates retained per MultiQuery variant during vector retrieval; 0 uses the built-in default max(top_k*4, 20).
	PrefetchTopK int `yaml:"prefetch_top_k,omitempty" json:"prefetch_top_k,omitempty"`
	// MaxContextChars: max total Unicode characters of returned document content (whole chunks; no mid-chunk truncation); 0 means unlimited.
	MaxContextChars int `yaml:"max_context_chars,omitempty" json:"max_context_chars,omitempty"`
	// MaxContextTokens: max total tokens of returned document content (tiktoken, mapped by embedding model name; falls back to cl100k_base on failure); 0 means unlimited.
	MaxContextTokens int `yaml:"max_context_tokens,omitempty" json:"max_context_tokens,omitempty"`
}

// MultiQueryConfig is the Eino MultiQuery query rewriting config (always enabled; no off switch).
type MultiQueryConfig struct {
	// MaxQueries: max retrieval variants generated by LLM (including the original query's semantic coverage); 0 means default 4.
	MaxQueries int `yaml:"max_queries,omitempty" json:"max_queries,omitempty"`
}

func (c MultiQueryConfig) MaxQueriesEffective() int {
	if c.MaxQueries <= 0 {
		return 4
	}
	if c.MaxQueries > 8 {
		return 8
	}
	return c.MaxQueries
}

// RerankConfig is the retrieval re-ranking config (always enabled); supports dashscope and Cohere-compatible HTTP APIs.
type RerankConfig struct {
	// Provider: dashscope | cohere; empty means auto-detect from base_url.
	Provider string `yaml:"provider,omitempty" json:"provider,omitempty"`
	Model    string `yaml:"model,omitempty" json:"model,omitempty"`
	BaseURL  string `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	APIKey   string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
}

func (c RerankConfig) ProviderEffective(baseURL string) string {
	p := strings.TrimSpace(strings.ToLower(c.Provider))
	if p != "" {
		return p
	}
	u := strings.ToLower(baseURL)
	if strings.Contains(u, "dashscope") {
		return "dashscope"
	}
	return "cohere"
}

func (c RerankConfig) ModelEffective(provider string) string {
	if m := strings.TrimSpace(c.Model); m != "" {
		return m
	}
	if provider == "dashscope" {
		return "gte-rerank"
	}
	return "rerank-multilingual-v3.0"
}

// RetrievalConfig retrieval configuration
type RetrievalConfig struct {
	TopK                int     `yaml:"top_k" json:"top_k"`                               // retrieval top-K
	SimilarityThreshold float64 `yaml:"similarity_threshold" json:"similarity_threshold"` // cosine similarity threshold
	// SubIndexFilter: when non-empty, only keeps rows where sub_indexes contains this tag (one of comma-separated values); old rows with empty sub_indexes are still returned.
	SubIndexFilter string           `yaml:"sub_index_filter,omitempty" json:"sub_index_filter,omitempty"`
	MultiQuery     MultiQueryConfig `yaml:"multi_query" json:"multi_query"`
	Rerank         RerankConfig     `yaml:"rerank" json:"rerank"`
	// PostRetrieve: post-retrieval processing (dedup, budget truncation); re-ranking is performed after MultiQuery fusion.
	PostRetrieve PostRetrieveConfig `yaml:"post_retrieve,omitempty" json:"post_retrieve,omitempty"`
}

// RolesConfig is the roles configuration (deprecated; use map[string]RoleConfig instead).
// Retained for backward compatibility; prefer using map[string]RoleConfig directly.
type RolesConfig struct {
	Roles map[string]RoleConfig `yaml:"roles,omitempty" json:"roles,omitempty"`
}

// RoleConfig is the configuration for a single role.
type RoleConfig struct {
	Name            string   `yaml:"name" json:"name"`                                             // role name
	Description     string   `yaml:"description" json:"description"`                               // role description
	UserPrompt      string   `yaml:"user_prompt" json:"user_prompt"`                               // user prompt (prepended to user message)
	Icon            string   `yaml:"icon,omitempty" json:"icon,omitempty"`                         // role icon (optional)
	Tools           []string `yaml:"tools,omitempty" json:"tools,omitempty"`                       // associated tool list (toolKey format, e.g. "toolName" or "mcpName::toolName")
	MCPs            []string `yaml:"mcps,omitempty" json:"mcps,omitempty"`                         // backward compat: associated MCP server list (deprecated; use tools instead)
	WorkflowID      string   `yaml:"workflow_id,omitempty" json:"workflow_id,omitempty"`           // optional: bound workflow ID
	WorkflowVersion string   `yaml:"workflow_version,omitempty" json:"workflow_version,omitempty"` // latest or a specific version; empty is equivalent to latest
	WorkflowPolicy  string   `yaml:"workflow_policy,omitempty" json:"workflow_policy,omitempty"`   // auto | off; empty with non-empty workflow_id defaults to auto
	Enabled         bool     `yaml:"enabled" json:"enabled"`                                       // enabled
}
