// Package mcp external MCP client - implemented using the official go-sdk to ensure protocol compatibility
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"kestrel/internal/config"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

const (
	clientName    = "Kestrel"
	clientVersion = "1.0.0"
)

// sdkClient is an external MCP client based on the official MCP Go SDK, implementing the ExternalMCPClient interface
type sdkClient struct {
	session *mcp.ClientSession
	client  *mcp.Client
	logger  *zap.Logger
	mu      sync.RWMutex
	status  string // "disconnected", "connecting", "connected", "error"
}

// newSDKClientFromSession constructs from an already-connected session (for internal use by createSDKClient)
func newSDKClientFromSession(session *mcp.ClientSession, client *mcp.Client, logger *zap.Logger) *sdkClient {
	return &sdkClient{
		session: session,
		client:  client,
		logger:  logger,
		status:  "connected",
	}
}

// lazySDKClient lazy connection: calls the official SDK to establish a connection only when Initialize() is called, externally implements ExternalMCPClient
type lazySDKClient struct {
	serverCfg     config.ExternalMCPServerConfig
	logger        *zap.Logger
	sessionCancel context.CancelFunc
	inner         ExternalMCPClient // connected SDK client
	mu            sync.RWMutex
	status        string
}

func newLazySDKClient(serverCfg config.ExternalMCPServerConfig, logger *zap.Logger) *lazySDKClient {
	return &lazySDKClient{
		serverCfg: serverCfg,
		logger:    logger,
		status:    "connecting",
	}
}

func (c *lazySDKClient) setStatus(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = s
}

func (c *lazySDKClient) GetStatus() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.inner != nil {
		return c.inner.GetStatus()
	}
	return c.status
}

func (c *lazySDKClient) IsConnected() bool {
	c.mu.RLock()
	inner := c.inner
	c.mu.RUnlock()
	if inner != nil {
		return inner.IsConnected()
	}
	return false
}

func (c *lazySDKClient) Initialize(ctx context.Context) error {
	c.mu.Lock()
	if c.inner != nil {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	sessionCtx, sessionCancel := context.WithCancel(context.Background())
	type connectResult struct {
		inner ExternalMCPClient
		err   error
	}
	resultCh := make(chan connectResult)
	abandoned := make(chan struct{})
	go func() {
		inner, err := createSDKClient(sessionCtx, c.serverCfg, c.logger)
		select {
		case resultCh <- connectResult{inner: inner, err: err}:
		case <-abandoned:
			if inner != nil {
				_ = inner.Close()
			}
			sessionCancel()
		}
	}()

	var result connectResult
	select {
	case result = <-resultCh:
	case <-ctx.Done():
		close(abandoned)
		sessionCancel()
		c.setStatus("error")
		return ctx.Err()
	}

	if err := ctx.Err(); err != nil {
		sessionCancel()
		if result.inner != nil {
			_ = result.inner.Close()
		}
		c.setStatus("error")
		return err
	}

	if result.err != nil {
		sessionCancel()
		c.setStatus("error")
		return result.err
	}

	c.mu.Lock()
	if c.inner != nil {
		c.mu.Unlock()
		sessionCancel()
		if result.inner != nil {
			_ = result.inner.Close()
		}
		return nil
	}
	c.inner = result.inner
	c.sessionCancel = sessionCancel
	c.mu.Unlock()
	c.setStatus("connected")
	return nil
}

func (c *lazySDKClient) ListTools(ctx context.Context) ([]Tool, error) {
	c.mu.RLock()
	inner := c.inner
	c.mu.RUnlock()
	if inner == nil {
		return nil, fmt.Errorf("not connected")
	}
	return inner.ListTools(ctx)
}

func (c *lazySDKClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (*ToolResult, error) {
	c.mu.RLock()
	inner := c.inner
	c.mu.RUnlock()
	if inner == nil {
		return nil, fmt.Errorf("not connected")
	}
	return inner.CallTool(ctx, name, args)
}

func (c *lazySDKClient) Close() error {
	c.mu.Lock()
	inner := c.inner
	sessionCancel := c.sessionCancel
	c.inner = nil
	c.sessionCancel = nil
	c.mu.Unlock()
	c.setStatus("disconnected")
	if sessionCancel != nil {
		sessionCancel()
	}
	if inner != nil {
		return inner.Close()
	}
	return nil
}

// markDisconnected closes the underlying session when a transport-layer disconnect is detected, preventing IsConnected from still returning true.
func (c *lazySDKClient) markDisconnected() {
	c.mu.Lock()
	inner := c.inner
	sessionCancel := c.sessionCancel
	c.inner = nil
	c.sessionCancel = nil
	c.mu.Unlock()
	if sessionCancel != nil {
		sessionCancel()
	}
	if inner != nil {
		_ = inner.Close()
	}
	c.setStatus("disconnected")
}

func (c *sdkClient) setStatus(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = s
}

func (c *sdkClient) GetStatus() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *sdkClient) IsConnected() bool {
	return c.GetStatus() == "connected"
}

func (c *sdkClient) Initialize(ctx context.Context) error {
	// sdkClient is created by createSDKClient only after Connect succeeds, so it is already connected at Initialize time.
	// This method exists only to satisfy the ExternalMCPClient interface; actual connection is established in createSDKClient.
	return nil
}

func (c *sdkClient) ListTools(ctx context.Context) ([]Tool, error) {
	if c.session == nil {
		return nil, fmt.Errorf("not connected")
	}
	res, err := c.session.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	return sdkToolsToOur(res.Tools), nil
}

func (c *sdkClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (*ToolResult, error) {
	if c.session == nil {
		return nil, fmt.Errorf("not connected")
	}
	params := &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	}
	res, err := c.session.CallTool(ctx, params)
	if err != nil {
		return nil, err
	}
	return sdkCallToolResultToOurs(res), nil
}

func (c *sdkClient) Close() error {
	c.setStatus("disconnected")
	if c.session != nil {
		err := c.session.Close()
		c.session = nil
		return err
	}
	return nil
}

// sdkToolsToOur converts the SDK's []*mcp.Tool to our []Tool
func sdkToolsToOur(tools []*mcp.Tool) []Tool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]Tool, 0, len(tools))
	for _, t := range tools {
		if t == nil {
			continue
		}
		schema := make(map[string]interface{})
		if t.InputSchema != nil {
			// SDK InputSchema may be *jsonschema.Schema or map; normalise to map
			if m, ok := t.InputSchema.(map[string]interface{}); ok {
				schema = m
			} else {
				_ = json.Unmarshal(mustJSON(t.InputSchema), &schema)
			}
		}
		desc := t.Description
		shortDesc := desc
		if t.Annotations != nil && t.Annotations.Title != "" {
			shortDesc = t.Annotations.Title
		}
		out = append(out, Tool{
			Name:             t.Name,
			Description:      desc,
			ShortDescription: shortDesc,
			InputSchema:      schema,
		})
	}
	return out
}

// sdkCallToolResultToOurs converts the SDK's *mcp.CallToolResult to our *ToolResult
func sdkCallToolResultToOurs(res *mcp.CallToolResult) *ToolResult {
	if res == nil {
		return &ToolResult{Content: []Content{}}
	}
	content := sdkContentToOurs(res.Content)
	blocked, _ := res.Meta[toolGuardBlockedMetaKey].(bool)
	return &ToolResult{
		Content: content,
		IsError: res.IsError,
		Blocked: blocked,
	}
}

func sdkContentToOurs(list []mcp.Content) []Content {
	if len(list) == 0 {
		return nil
	}
	out := make([]Content, 0, len(list))
	for _, c := range list {
		switch v := c.(type) {
		case *mcp.TextContent:
			out = append(out, Content{Type: "text", Text: v.Text})
		default:
			out = append(out, Content{Type: "text", Text: fmt.Sprintf("%v", c)})
		}
	}
	return out
}

func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

// createSDKClient creates and connects an external MCP client based on config (using the official SDK), returning a *sdkClient that implements ExternalMCPClient.
// Returns (nil, error) if connection fails. ctx is used for connection timeout and cancellation.
func createSDKClient(ctx context.Context, serverCfg config.ExternalMCPServerConfig, logger *zap.Logger) (ExternalMCPClient, error) {
	timeout := time.Duration(serverCfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	transport := serverCfg.GetTransportType()
	if transport == "" {
		return nil, fmt.Errorf("config missing command or url, and no type/transport specified")
	}

	// construct ClientOptions: KeepAlive heartbeat
	var clientOpts *mcp.ClientOptions
	if serverCfg.KeepAlive > 0 {
		clientOpts = &mcp.ClientOptions{
			KeepAlive: time.Duration(serverCfg.KeepAlive) * time.Second,
		}
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    clientName,
		Version: clientVersion,
	}, clientOpts)

	var t mcp.Transport
	switch transport {
	case "stdio":
		if serverCfg.Command == "" {
			return nil, fmt.Errorf("stdio mode requires config command")
		}
		// must use exec.Command instead of CommandContext: after doConnect returns, ctx will be cancelled;
		// using CommandContext(ctx) would immediately kill the child process, causing ListTools and subsequent requests to fail with 0 tools
		cmd := exec.Command(serverCfg.Command, serverCfg.Args...)
		if len(serverCfg.Env) > 0 {
			cmd.Env = append(cmd.Env, envMapToSlice(serverCfg.Env)...)
		}
		ct := &mcp.CommandTransport{Command: cmd}
		if serverCfg.TerminateDuration > 0 {
			ct.TerminateDuration = time.Duration(serverCfg.TerminateDuration) * time.Second
		}
		t = ct
	case "sse":
		if serverCfg.URL == "" {
			return nil, fmt.Errorf("sse mode requires config url")
		}
		// SSE is a long-lived connection (GET stream stays open); cannot set http.Client.Timeout (it would kill the connection after timeout, causing EOF).
		// Timeout is controlled per-request by the context passed to each ListTools/CallTool call.
		httpClient := httpClientForLongLived(serverCfg.Headers)
		t = &mcp.SSEClientTransport{
			Endpoint:   serverCfg.URL,
			HTTPClient: httpClient,
		}
	case "http":
		if serverCfg.URL == "" {
			return nil, fmt.Errorf("http mode requires config url")
		}
		httpClient := httpClientWithTimeoutAndHeaders(timeout, serverCfg.Headers)
		st := &mcp.StreamableClientTransport{
			Endpoint:   serverCfg.URL,
			HTTPClient: httpClient,
		}
		if serverCfg.MaxRetries > 0 {
			st.MaxRetries = serverCfg.MaxRetries
		}
		t = st
	default:
		return nil, fmt.Errorf("unsupported transport mode: %s (supported: stdio, sse, http)", transport)
	}

	session, err := client.Connect(ctx, t, nil)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}

	return newSDKClientFromSession(session, client, logger), nil
}

func envMapToSlice(env map[string]string) []string {
	m := make(map[string]string)
	for _, s := range os.Environ() {
		if i := strings.IndexByte(s, '='); i > 0 {
			m[s[:i]] = s[i+1:]
		}
	}
	for k, v := range env {
		m[k] = v
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

func httpClientWithTimeoutAndHeaders(timeout time.Duration, headers map[string]string) *http.Client {
	transport := http.DefaultTransport
	if len(headers) > 0 {
		transport = &headerRoundTripper{
			headers: headers,
			base:    http.DefaultTransport,
		}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

// httpClientForLongLived creates an HTTP client with no timeout, for use with long-lived transports like SSE.
// SSE's GET stream stays open indefinitely; http.Client.Timeout would force-close the connection after the timeout causing EOF.
// Timeout is controlled by the caller via context.
func httpClientForLongLived(headers map[string]string) *http.Client {
	transport := http.DefaultTransport
	if len(headers) > 0 {
		transport = &headerRoundTripper{
			headers: headers,
			base:    http.DefaultTransport,
		}
	}
	return &http.Client{
		Transport: transport,
		// no Timeout set; SSE long-lived connection timeout is controlled by per-request context
	}
}

type headerRoundTripper struct {
	headers map[string]string
	base    http.RoundTripper
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}
	return h.base.RoundTrip(req)
}
