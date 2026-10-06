package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
	"kestrel/internal/config"
)

// Role constants for chat messages.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Message is a single chat message sent to / received from an LLM.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall is a function call proposed by the LLM.
type ToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"` // "function"
	Function FunctionCallDef `json:"function"`
}

// FunctionCallDef carries the name and JSON-encoded arguments of a tool call.
type FunctionCallDef struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-encoded map
}

// ToolSpec is a tool schema sent to the LLM so it knows which tools it may call.
type ToolSpec struct {
	Type     string          `json:"type"` // "function"
	Function ToolFunctionSpec `json:"function"`
}

// ToolFunctionSpec is the function schema within a ToolSpec.
type ToolFunctionSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// CompletionRequest carries all parameters for a chat completion call.
type CompletionRequest struct {
	Messages    []Message  `json:"messages"`
	Tools       []ToolSpec `json:"tools,omitempty"`
	MaxTokens   int        `json:"max_tokens,omitempty"`
	Temperature float64    `json:"temperature,omitempty"`
	Stream      bool       `json:"stream,omitempty"`
}

// CompletionResponse is the normalised response from any provider.
type CompletionResponse struct {
	Content   string     // assistant text content
	ToolCalls []ToolCall // tool calls the LLM wants to make
	StopReason string    // "stop" | "tool_calls" | "length" | "error"
	InputTokens  int
	OutputTokens int
}

// StreamChunk is a single streaming delta from the LLM.
type StreamChunk struct {
	Delta     string     // text delta
	ToolCalls []ToolCall // incremental tool call info (may be partial)
	Done      bool
}

// Client is the interface every provider must satisfy.
type Client interface {
	// Complete sends a non-streaming completion request.
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
	// Stream sends a streaming completion request, yielding chunks on the returned channel.
	Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error)
	// Provider returns the provider name string.
	Provider() string
}

// Manager holds all configured LLM channels and routes requests.
type Manager struct {
	clients map[string]Client // channel name → client
	def     string            // default channel name
	logger  *zap.Logger
}

// NewManager creates an LLM manager from config.
// If no channels are configured, a stub client is registered so the agent
// can run without real LLM calls (rule-based mode).
func NewManager(cfg *config.AIConfig, logger *zap.Logger) *Manager {
	m := &Manager{
		clients: make(map[string]Client),
		def:     cfg.DefaultChannel,
		logger:  logger,
	}

	for name, ch := range cfg.Channels {
		var client Client
		switch strings.ToLower(ch.Provider) {
		case "openai":
			client = NewOpenAIClient(ch, logger)
		case "anthropic":
			client = NewAnthropicClient(ch, logger)
		case "ollama":
			client = NewOllamaClient(ch, logger)
		default:
			logger.Warn("unknown LLM provider — skipping channel",
				zap.String("channel", name), zap.String("provider", ch.Provider))
			continue
		}
		m.clients[name] = client
		logger.Info("registered LLM channel",
			zap.String("channel", name),
			zap.String("provider", ch.Provider),
			zap.String("model", ch.Model),
		)
	}

	if len(m.clients) == 0 {
		m.clients["stub"] = &StubClient{}
		if m.def == "" {
			m.def = "stub"
		}
		logger.Info("no LLM channels configured — using stub (rule-based) mode")
	}
	return m
}

// Default returns the default channel client.
func (m *Manager) Default() Client {
	if c, ok := m.clients[m.def]; ok {
		return c
	}
	// Fallback: first available.
	for _, c := range m.clients {
		return c
	}
	return &StubClient{}
}

// Get returns a named channel client.
func (m *Manager) Get(name string) (Client, bool) {
	c, ok := m.clients[name]
	return c, ok
}

// IsStub returns true when only the stub client is available.
func (m *Manager) IsStub() bool {
	_, ok := m.clients["stub"]
	return ok && len(m.clients) == 1
}

// ─── OpenAI client ────────────────────────────────────────────────────────────

// OpenAIClient calls the OpenAI Chat Completions API (also compatible with
// Azure OpenAI, OpenRouter, and any OpenAI-compatible endpoint).
type OpenAIClient struct {
	cfg    config.AIChannelConfig
	http   *http.Client
	logger *zap.Logger
}

// NewOpenAIClient creates an OpenAI (or compatible) client.
func NewOpenAIClient(cfg config.AIChannelConfig, logger *zap.Logger) *OpenAIClient {
	return &OpenAIClient{
		cfg:    cfg,
		http:   &http.Client{Timeout: 120 * time.Second},
		logger: logger,
	}
}

func (c *OpenAIClient) Provider() string { return "openai" }

func (c *OpenAIClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	baseURL := c.cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	type openAIReq struct {
		Model     string      `json:"model"`
		Messages  []Message   `json:"messages"`
		Tools     []ToolSpec  `json:"tools,omitempty"`
		MaxTokens int         `json:"max_tokens,omitempty"`
		Temp      float64     `json:"temperature,omitempty"`
	}

	body := openAIReq{
		Model:     c.cfg.Model,
		Messages:  req.Messages,
		Tools:     req.Tools,
		MaxTokens: req.MaxTokens,
		Temp:      req.Temperature,
	}
	if body.MaxTokens <= 0 && c.cfg.MaxCompletionTokens > 0 {
		body.MaxTokens = c.cfg.MaxCompletionTokens
	}

	data, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai error %d: %s", resp.StatusCode, string(body))
	}

	var out struct {
		Choices []struct {
			Message    Message `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding openai response: %w", err)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("openai returned no choices")
	}

	choice := out.Choices[0]
	return &CompletionResponse{
		Content:      choice.Message.Content,
		ToolCalls:    choice.Message.ToolCalls,
		StopReason:   choice.FinishReason,
		InputTokens:  out.Usage.PromptTokens,
		OutputTokens: out.Usage.CompletionTokens,
	}, nil
}

func (c *OpenAIClient) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	baseURL := c.cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	type openAIReq struct {
		Model     string     `json:"model"`
		Messages  []Message  `json:"messages"`
		Tools     []ToolSpec `json:"tools,omitempty"`
		MaxTokens int        `json:"max_tokens,omitempty"`
		Stream    bool       `json:"stream"`
	}
	body := openAIReq{
		Model: c.cfg.Model, Messages: req.Messages, Tools: req.Tools,
		MaxTokens: req.MaxTokens, Stream: true,
	}
	data, _ := json.Marshal(body)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("openai stream error %d: %s", resp.StatusCode, string(b))
	}

	ch := make(chan StreamChunk, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")
			if payload == "[DONE]" {
				ch <- StreamChunk{Done: true}
				return
			}
			var event struct {
				Choices []struct {
					Delta struct {
						Content   string     `json:"content"`
						ToolCalls []ToolCall `json:"tool_calls"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				continue
			}
			if len(event.Choices) > 0 {
				d := event.Choices[0].Delta
				ch <- StreamChunk{Delta: d.Content, ToolCalls: d.ToolCalls}
			}
		}
	}()
	return ch, nil
}

// ─── Anthropic client ─────────────────────────────────────────────────────────

// AnthropicClient calls the Anthropic Messages API.
type AnthropicClient struct {
	cfg    config.AIChannelConfig
	http   *http.Client
	logger *zap.Logger
}

// NewAnthropicClient creates an Anthropic client.
func NewAnthropicClient(cfg config.AIChannelConfig, logger *zap.Logger) *AnthropicClient {
	return &AnthropicClient{
		cfg:    cfg,
		http:   &http.Client{Timeout: 120 * time.Second},
		logger: logger,
	}
}

func (c *AnthropicClient) Provider() string { return "anthropic" }

func (c *AnthropicClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	baseURL := c.cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}

	// Separate system messages from the rest.
	var systemContent string
	var msgs []Message
	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			systemContent += m.Content + "\n"
		} else {
			msgs = append(msgs, m)
		}
	}

	// Build Anthropic tools format.
	type anthropicTool struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"input_schema"`
	}
	var tools []anthropicTool
	for _, t := range req.Tools {
		tools = append(tools, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}

	type anthropicReq struct {
		Model     string          `json:"model"`
		MaxTokens int             `json:"max_tokens"`
		System    string          `json:"system,omitempty"`
		Messages  []Message       `json:"messages"`
		Tools     []anthropicTool `json:"tools,omitempty"`
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = c.cfg.MaxCompletionTokens
	}
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	body := anthropicReq{
		Model: c.cfg.Model, MaxTokens: maxTokens,
		System: strings.TrimSpace(systemContent), Messages: msgs, Tools: tools,
	}
	data, _ := json.Marshal(body)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/messages", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic error %d: %s", resp.StatusCode, string(b))
	}

	var out struct {
		Content []struct {
			Type  string `json:"type"` // "text" | "tool_use"
			Text  string `json:"text,omitempty"`
			ID    string `json:"id,omitempty"`
			Name  string `json:"name,omitempty"`
			Input json.RawMessage `json:"input,omitempty"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	result := &CompletionResponse{
		StopReason:   out.StopReason,
		InputTokens:  out.Usage.InputTokens,
		OutputTokens: out.Usage.OutputTokens,
	}
	for _, block := range out.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "tool_use":
			argsStr := "{}"
			if block.Input != nil {
				argsStr = string(block.Input)
			}
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: FunctionCallDef{Name: block.Name, Arguments: argsStr},
			})
		}
	}
	if len(result.ToolCalls) > 0 {
		result.StopReason = "tool_calls"
	}
	return result, nil
}

func (c *AnthropicClient) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	// Delegate to non-streaming for now; Anthropic streaming is complex.
	resp, err := c.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	ch := make(chan StreamChunk, 4)
	go func() {
		defer close(ch)
		ch <- StreamChunk{Delta: resp.Content, ToolCalls: resp.ToolCalls}
		ch <- StreamChunk{Done: true}
	}()
	return ch, nil
}

// ─── Ollama client ────────────────────────────────────────────────────────────

// OllamaClient calls a local Ollama server (/api/chat).
type OllamaClient struct {
	cfg    config.AIChannelConfig
	http   *http.Client
	logger *zap.Logger
}

// NewOllamaClient creates an Ollama client.
func NewOllamaClient(cfg config.AIChannelConfig, logger *zap.Logger) *OllamaClient {
	return &OllamaClient{
		cfg:    cfg,
		http:   &http.Client{Timeout: 300 * time.Second},
		logger: logger,
	}
}

func (c *OllamaClient) Provider() string { return "ollama" }

func (c *OllamaClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	baseURL := c.cfg.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	type ollamaMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	var msgs []ollamaMsg
	for _, m := range req.Messages {
		msgs = append(msgs, ollamaMsg{Role: m.Role, Content: m.Content})
	}

	body := map[string]interface{}{
		"model":    c.cfg.Model,
		"messages": msgs,
		"stream":   false,
	}
	data, _ := json.Marshal(body)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(b))
	}

	var out struct {
		Message ollamaMsg `json:"message"`
		Done    bool      `json:"done"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	return &CompletionResponse{
		Content:    out.Message.Content,
		StopReason: "stop",
	}, nil
}

func (c *OllamaClient) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	resp, err := c.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	ch := make(chan StreamChunk, 4)
	go func() {
		defer close(ch)
		ch <- StreamChunk{Delta: resp.Content}
		ch <- StreamChunk{Done: true}
	}()
	return ch, nil
}

// ─── Stub client ──────────────────────────────────────────────────────────────

// StubClient returns a canned response when no real LLM is configured.
type StubClient struct{}

func (s *StubClient) Provider() string { return "stub" }

func (s *StubClient) Complete(_ context.Context, req CompletionRequest) (*CompletionResponse, error) {
	return &CompletionResponse{
		Content:    "LLM not configured. Running in rule-based stub mode. Configure an AI channel in config.yaml to enable real LLM responses.",
		StopReason: "stop",
	}, nil
}

func (s *StubClient) Stream(_ context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 2)
	go func() {
		defer close(ch)
		ch <- StreamChunk{Delta: "LLM not configured — running in stub mode."}
		ch <- StreamChunk{Done: true}
	}()
	return ch, nil
}
