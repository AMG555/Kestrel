package audit

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeDetailRedaction(t *testing.T) {
	raw := map[string]interface{}{
		"username": "admin",
		"password": "SuperSecretPassword123!",
		"api_key":  "sk-proj-xyz987654321",
		"config": map[string]interface{}{
			"secret_token": "bearer-abc-token",
			"host":         "10.0.0.1",
			"nested": []interface{}{
				map[string]interface{}{
					"private_key": "-----BEGIN RSA PRIVATE KEY-----",
					"port":        8080,
				},
			},
		},
	}

	sanitized := SanitizeDetail(raw, 4096)
	if sanitized["username"] != "admin" {
		t.Errorf("expected username to be untouched, got %v", sanitized["username"])
	}
	if sanitized["password"] != "***" {
		t.Errorf("expected password to be redacted, got %v", sanitized["password"])
	}
	if sanitized["api_key"] != "***" {
		t.Errorf("expected api_key to be redacted, got %v", sanitized["api_key"])
	}

	cfg, ok := sanitized["config"].(map[string]interface{})
	if !ok {
		t.Fatalf("config is not a map")
	}
	if cfg["secret_token"] != "***" {
		t.Errorf("expected secret_token to be redacted, got %v", cfg["secret_token"])
	}
	if cfg["host"] != "10.0.0.1" {
		t.Errorf("expected host to be untouched, got %v", cfg["host"])
	}

	nested, ok := cfg["nested"].([]interface{})
	if !ok || len(nested) == 0 {
		t.Fatalf("nested array invalid")
	}
	firstElem, ok := nested[0].(map[string]interface{})
	if !ok {
		t.Fatalf("nested[0] is not a map")
	}
	if firstElem["private_key"] != "***" {
		t.Errorf("expected private_key to be redacted, got %v", firstElem["private_key"])
	}
	if firstElem["port"] != 8080 {
		t.Errorf("expected port to be 8080, got %v", firstElem["port"])
	}
}

func TestSanitizeDetailTruncation(t *testing.T) {
	raw := map[string]interface{}{
		"large_payload": strings.Repeat("A", 1000),
	}
	sanitized := SanitizeDetail(raw, 50)
	if sanitized["_truncated"] != true {
		t.Errorf("expected _truncated to be true")
	}
	if _, ok := sanitized["_preview"]; !ok {
		t.Errorf("expected _preview to be present")
	}
}

func TestSanitizeJSON(t *testing.T) {
	input := `{"user":"tester","token":"eyJhbGciOi..."}`
	output := SanitizeJSON([]byte(input), 1024)
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(output), &m); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if m["user"] != "tester" {
		t.Errorf("user was altered")
	}
	if m["token"] != "***" {
		t.Errorf("token was not redacted")
	}
}
