package audit

import (
	"encoding/json"
	"strings"
)

var sensitiveKeySubstrings = []string{
	"password", "api_key", "apikey", "secret", "token", "authorization",
	"credential", "private_key", "access_key", "auth_token", "jwt",
}

// SanitizeDetail redacts sensitive keys and truncates serialized size.
func SanitizeDetail(detail map[string]interface{}, maxBytes int) map[string]interface{} {
	if detail == nil {
		return nil
	}
	if maxBytes <= 0 {
		maxBytes = 8192
	}
	out := sanitizeValue("", detail)
	if m, ok := out.(map[string]interface{}); ok {
		b, err := json.Marshal(m)
		if err == nil && len(b) > maxBytes {
			return map[string]interface{}{
				"_truncated": true,
				"_preview":   string(b[:maxBytes]),
			}
		}
		return m
	}
	return map[string]interface{}{"value": out}
}

// SanitizeJSON parses arbitrary JSON, scrubs sensitive keys, and returns clean JSON string.
func SanitizeJSON(raw []byte, maxBytes int) string {
	if len(raw) == 0 {
		return "{}"
	}
	var data interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return "{}"
	}
	scrubbed := sanitizeValue("", data)
	b, err := json.Marshal(scrubbed)
	if err != nil {
		return "{}"
	}
	if maxBytes > 0 && len(b) > maxBytes {
		truncMap := map[string]interface{}{
			"_truncated": true,
			"_preview":   string(b[:maxBytes]),
		}
		tb, _ := json.Marshal(truncMap)
		return string(tb)
	}
	return string(b)
}

func sanitizeValue(key string, v interface{}) interface{} {
	kl := strings.ToLower(key)
	for _, sub := range sensitiveKeySubstrings {
		if strings.Contains(kl, sub) {
			return "***"
		}
	}
	switch t := v.(type) {
	case map[string]interface{}:
		m := make(map[string]interface{}, len(t))
		for k, val := range t {
			m[k] = sanitizeValue(k, val)
		}
		return m
	case []interface{}:
		arr := make([]interface{}, len(t))
		for i, val := range t {
			arr[i] = sanitizeValue(key, val)
		}
		return arr
	default:
		return v
	}
}
