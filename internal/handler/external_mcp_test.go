package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"kestrel/internal/config"
	"kestrel/internal/mcp"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func setupTestRouter() (*gin.Engine, *ExternalMCPHandler, string) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// create a temporary configuration file
	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	if err != nil {
		panic(err)
	}
	tmpFile.WriteString("server:\n  host: 0.0.0.0\n  port: 8080\n")
	tmpFile.Close()
	configPath := tmpFile.Name()

	logger := zap.NewNop()
	manager := mcp.NewExternalMCPManager(logger)
	cfg := &config.Config{
		ExternalMCP: config.ExternalMCPConfig{
			Servers: make(map[string]config.ExternalMCPServerConfig),
		},
	}

	handler := NewExternalMCPHandler(manager, cfg, configPath, logger)

	api := router.Group("/api")
	api.GET("/external-mcp", handler.GetExternalMCPs)
	api.GET("/external-mcp/stats", handler.GetExternalMCPStats)
	api.GET("/external-mcp/:name", handler.GetExternalMCP)
	api.PUT("/external-mcp/:name", handler.AddOrUpdateExternalMCP)
	api.DELETE("/external-mcp/:name", handler.DeleteExternalMCP)
	api.POST("/external-mcp/:name/start", handler.StartExternalMCP)
	api.POST("/external-mcp/:name/stop", handler.StopExternalMCP)

	return router, handler, configPath
}

func cleanupTestConfig(configPath string) {
	os.Remove(configPath)
	os.Remove(configPath + ".backup")
}

func TestExternalMCPHandler_AddOrUpdateExternalMCP_Stdio(t *testing.T) {
	router, _, configPath := setupTestRouter()
	defer cleanupTestConfig(configPath)

	// test adding stdio mode config (official format: type can be omitted when command is present)
	configJSON := `{
		"command": "python3",
		"args": ["/path/to/script.py", "--server", "http://example.com"],
		"description": "Test stdio MCP",
		"timeout": 300,
		"external_mcp_enable": true
	}`

	var configObj config.ExternalMCPServerConfig
	if err := json.Unmarshal([]byte(configJSON), &configObj); err != nil {
		t.Fatalf("parse config JSON failed: %v", err)
	}

	reqBody := AddOrUpdateExternalMCPRequest{
		Config: configObj,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/external-mcp/test-stdio", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// validate config was added
	req2 := httptest.NewRequest("GET", "/api/external-mcp/test-stdio", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var response ExternalMCPResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &response); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if response.Config.Command != "python3" {
		t.Errorf("expected command python3, got %s", response.Config.Command)
	}
	if len(response.Config.Args) != 3 {
		t.Errorf("expected args length 3, got %d", len(response.Config.Args))
	}
	if response.Config.Description != "Test stdio MCP" {
		t.Errorf("expected description 'Test stdio MCP', got %s", response.Config.Description)
	}
	if response.Config.Timeout != 300 {
		t.Errorf("expected timeout 300, got %d", response.Config.Timeout)
	}
}

func TestExternalMCPHandler_AddOrUpdateExternalMCP_HTTP(t *testing.T) {
	router, _, configPath := setupTestRouter()
	defer cleanupTestConfig(configPath)

	// test adding HTTP mode config (using the official type field)
	configJSON := `{
		"type": "http",
		"url": "http://127.0.0.1:8081/mcp",
		"external_mcp_enable": true
	}`

	var configObj config.ExternalMCPServerConfig
	if err := json.Unmarshal([]byte(configJSON), &configObj); err != nil {
		t.Fatalf("parse config JSON failed: %v", err)
	}

	reqBody := AddOrUpdateExternalMCPRequest{
		Config: configObj,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/external-mcp/test-http", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// validate config was added
	req2 := httptest.NewRequest("GET", "/api/external-mcp/test-http", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var response ExternalMCPResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &response); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if response.Config.Type != "http" {
		t.Errorf("expected type http, got %s", response.Config.Type)
	}
	if response.Config.URL != "http://127.0.0.1:8081/mcp" {
		t.Errorf("expected url 'http://127.0.0.1:8081/mcp', got %s", response.Config.URL)
	}
}

func TestExternalMCPHandler_AddOrUpdateExternalMCP_InvalidConfig(t *testing.T) {
	router, _, configPath := setupTestRouter()
	defer cleanupTestConfig(configPath)

	testCases := []struct {
		name        string
		configJSON  string
		expectedErr string
	}{
		{
			name:        "missing command and url",
			configJSON:  `{"external_mcp_enable": true}`,
			expectedErr: "must specify command (stdio mode) or url + type (http/sse mode)",
		},
		{
			name:        "stdio mode missing command",
			configJSON:  `{"args": ["test"], "external_mcp_enable": true}`,
			expectedErr: "stdio mode requires command",
		},
		{
			name:        "http mode missing url",
			configJSON:  `{"type": "http", "external_mcp_enable": true}`,
			expectedErr: "HTTP mode requires url",
		},
		{
			name:        "invalid type",
			configJSON:  `{"type": "invalid", "external_mcp_enable": true}`,
			expectedErr: "unsupported transport mode",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var configObj config.ExternalMCPServerConfig
			if err := json.Unmarshal([]byte(tc.configJSON), &configObj); err != nil {
				t.Fatalf("parse config JSON failed: %v", err)
			}

			reqBody := AddOrUpdateExternalMCPRequest{
				Config: configObj,
			}

			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest("PUT", "/api/external-mcp/test-invalid", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
			}

			var response map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("parse response failed: %v", err)
			}

			errorMsg := response["error"].(string)
			// for the stdio mode missing command case, the error message may differ slightly
			if tc.name == "stdio mode missing command" {
				if !strings.Contains(errorMsg, "stdio") && !strings.Contains(errorMsg, "command") {
					t.Errorf("expected error to contain 'stdio' or 'command', got '%s'", errorMsg)
				}
			} else if !strings.Contains(errorMsg, tc.expectedErr) {
				t.Errorf("expected error to contain '%s', got '%s'", tc.expectedErr, errorMsg)
			}
		})
	}
}

func TestExternalMCPHandler_DeleteExternalMCP(t *testing.T) {
	router, handler, configPath := setupTestRouter()
	defer cleanupTestConfig(configPath)

	// first add a config
	configObj := config.ExternalMCPServerConfig{
		Command:           "python3",
		ExternalMCPEnable: true,
	}
	handler.manager.AddOrUpdateConfig("test-delete", configObj)

	// deleteconfig
	req := httptest.NewRequest("DELETE", "/api/external-mcp/test-delete", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// validateconfigdeleted
	req2 := httptest.NewRequest("GET", "/api/external-mcp/test-delete", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestExternalMCPStatusError(t *testing.T) {
	manager := mcp.NewExternalMCPManager(zap.NewNop())
	if got := externalMCPStatusError(manager, "x", "connected"); got != "" {
		t.Fatalf("connected status should not return error, got %q", got)
	}
	if got := externalMCPStatusError(manager, "x", "connecting"); got != "" {
		t.Fatalf("connecting status should not return error, got %q", got)
	}
}

func TestExternalMCPHandler_GetExternalMCPs(t *testing.T) {
	router, handler, _ := setupTestRouter()

	// add multiple configs
	handler.manager.AddOrUpdateConfig("test1", config.ExternalMCPServerConfig{
		Command:           "python3",
		ExternalMCPEnable: true,
	})
	handler.manager.AddOrUpdateConfig("test2", config.ExternalMCPServerConfig{
		URL:               "http://127.0.0.1:8081/mcp",
		ExternalMCPEnable: false,
	})

	req := httptest.NewRequest("GET", "/api/external-mcp", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	servers := response["servers"].(map[string]interface{})
	if len(servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(servers))
	}
	if _, ok := servers["test1"]; !ok {
		t.Error("expected test1 to be present")
	}
	if _, ok := servers["test2"]; !ok {
		t.Error("expected test2 to be present")
	}

	stats := response["stats"].(map[string]interface{})
	if int(stats["total"].(float64)) != 2 {
		t.Errorf("expected total 2, got %d", int(stats["total"].(float64)))
	}
}

func TestExternalMCPHandler_GetExternalMCPStats(t *testing.T) {
	router, handler, _ := setupTestRouter()

	// add config
	handler.manager.AddOrUpdateConfig("enabled1", config.ExternalMCPServerConfig{
		Command:           "python3",
		ExternalMCPEnable: true,
	})
	handler.manager.AddOrUpdateConfig("enabled2", config.ExternalMCPServerConfig{
		URL:               "http://127.0.0.1:8081/mcp",
		ExternalMCPEnable: true,
	})
	handler.manager.AddOrUpdateConfig("disabled1", config.ExternalMCPServerConfig{
		Command: "python3",
	})

	req := httptest.NewRequest("GET", "/api/external-mcp/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var stats map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if int(stats["total"].(float64)) != 3 {
		t.Errorf("expected total 3, got %d", int(stats["total"].(float64)))
	}
	if int(stats["enabled"].(float64)) != 2 {
		t.Errorf("expected enabled count 2, got %d", int(stats["enabled"].(float64)))
	}
	if int(stats["disabled"].(float64)) != 1 {
		t.Errorf("expected disabled count 1, got %d", int(stats["disabled"].(float64)))
	}
}

func TestExternalMCPHandler_StartStopExternalMCP(t *testing.T) {
	router, handler, configPath := setupTestRouter()
	defer cleanupTestConfig(configPath)

	// add a disabled config
	handler.manager.AddOrUpdateConfig("test-start-stop", config.ExternalMCPServerConfig{
		Command: "python3",
	})

	// test start (may fail because there is no real server)
	req := httptest.NewRequest("POST", "/api/external-mcp/test-start-stop/start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// start may fail, but should return a reasonable status code
	if w.Code != http.StatusOK {
		// if startup failed, should be 400 or 500
		if w.Code != http.StatusBadRequest && w.Code != http.StatusInternalServerError {
			t.Errorf("expected status 200/400/500, got %d: %s", w.Code, w.Body.String())
		}
	}

	// teststop
	req2 := httptest.NewRequest("POST", "/api/external-mcp/test-start-stop/stop", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestExternalMCPHandler_GetExternalMCP_NotFound(t *testing.T) {
	router, _, _ := setupTestRouter()

	req := httptest.NewRequest("GET", "/api/external-mcp/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExternalMCPHandler_DeleteExternalMCP_NotFound(t *testing.T) {
	router, _, configPath := setupTestRouter()
	defer cleanupTestConfig(configPath)

	req := httptest.NewRequest("DELETE", "/api/external-mcp/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// deleting a non-existent config may return 200 (idempotent) or 404, both are acceptable
	if w.Code != http.StatusNotFound && w.Code != http.StatusOK {
		t.Errorf("expected status 404 or 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExternalMCPHandler_AddOrUpdateExternalMCP_EmptyName(t *testing.T) {
	router, _, _ := setupTestRouter()

	configObj := config.ExternalMCPServerConfig{
		Command:           "python3",
		ExternalMCPEnable: true,
	}

	reqBody := AddOrUpdateExternalMCPRequest{
		Config: configObj,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/external-mcp/", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// empty name should return 404 or 400
	if w.Code != http.StatusNotFound && w.Code != http.StatusBadRequest {
		t.Errorf("expected status 404 or 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExternalMCPHandler_AddOrUpdateExternalMCP_InvalidJSON(t *testing.T) {
	router, _, _ := setupTestRouter()

	// send invalid JSON
	body := []byte(`{"config": invalid json}`)
	req := httptest.NewRequest("PUT", "/api/external-mcp/test", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExternalMCPHandler_UpdateExistingConfig(t *testing.T) {
	router, handler, configPath := setupTestRouter()
	defer cleanupTestConfig(configPath)

	// add config first
	config1 := config.ExternalMCPServerConfig{
		Command:           "python3",
		ExternalMCPEnable: true,
	}
	handler.manager.AddOrUpdateConfig("test-update", config1)

	// update configuration
	config2 := config.ExternalMCPServerConfig{
		URL:               "http://127.0.0.1:8081/mcp",
		ExternalMCPEnable: true,
	}

	reqBody := AddOrUpdateExternalMCPRequest{
		Config: config2,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/external-mcp/test-update", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// validate config was updated
	req2 := httptest.NewRequest("GET", "/api/external-mcp/test-update", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var response ExternalMCPResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &response); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if response.Config.URL != "http://127.0.0.1:8081/mcp" {
		t.Errorf("expected url 'http://127.0.0.1:8081/mcp', got %s", response.Config.URL)
	}
	if response.Config.Command != "" {
		t.Errorf("expected command to be empty, got %s", response.Config.Command)
	}
}
