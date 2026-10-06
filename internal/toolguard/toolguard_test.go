package toolguard

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

// ── DefaultConfig / government domain protection ─────────────────────────────

func TestDefaultGovernmentDomainBlocked(t *testing.T) {
	policy, err := Compile(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}

	blocked := []struct {
		input string
		want  string
	}{
		{"https://agency.gov/login", "agency.gov"},
		{"https://www.agency.gov.cn:443/login", "www.agency.gov.cn"},
		{"curl https://EXAMPLE.GOV.UK/a", "EXAMPLE.GOV.UK"},
		{"*.gov", "*.gov"},
		{"*.gov.*", "*.gov.*"},
		{".gov", ".gov"},
		{".gov.*", ".gov.*"},
		{"https://gov.cn/", "gov.cn"},
		{"https://agency.gov./", "agency.gov."},
		{"https://agency%2egov/a", "agency.gov"},
		{"https://agency%252Egov/a", "agency.gov"},
		{"echo 100% && curl https://agency%2egov/a", "agency.gov"},
	}
	for _, tc := range blocked {
		t.Run("blocked:"+tc.input, func(t *testing.T) {
			m := policy.Check("http_request", map[string]interface{}{"target": tc.input})
			if m == nil {
				t.Fatalf("Check(%q) = nil, want blocked", tc.input)
			}
			if m.MatchedText != tc.want {
				t.Fatalf("MatchedText = %q, want %q", m.MatchedText, tc.want)
			}
		})
	}
}

func TestDefaultGovernmentDomainAllowed(t *testing.T) {
	policy, err := Compile(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}

	allowed := []string{
		"https://example.com/",
		"https://government.example/",
		"https://agency.govt/",
		"https://agency.gov-example.com/",
		"https://agency.gov_cn/",
		"governance",
		"gov",
		".government",
		".govx",
	}
	for _, input := range allowed {
		t.Run("allowed:"+input, func(t *testing.T) {
			if m := policy.Check("http_request", map[string]interface{}{"target": input}); m != nil {
				t.Fatalf("Check(%q) = %+v, want nil", input, m)
			}
		})
	}
}

// ── Nested arguments and JSON escapes ────────────────────────────────────────

func TestNestedArgumentsBlocked(t *testing.T) {
	policy, _ := Compile(DefaultConfig())

	cases := []map[string]interface{}{
		{"targets": []interface{}{map[string]interface{}{"target": "https://agency.gov"}}},
		{"targets": []string{"https://agency.gov"}},
		{"https://agency.gov": true},
		{"payload": json.RawMessage(`{"target":"https://agency\u002egov"}`)},
		{"payload": json.RawMessage(`{"https://agency\u002egov":true}`)},
	}
	for _, args := range cases {
		if m := policy.Check("request", args); m == nil || m.MatchedText != "agency.gov" {
			t.Fatalf("Check(%v) = %+v, want agency.gov blocked", args, m)
		}
	}
}

func TestDeepNestedJSONBlocked(t *testing.T) {
	policy, _ := Compile(DefaultConfig())
	payload := strings.Repeat("[", 200) + `"https://agency\u002egov"` + strings.Repeat("]", 200)
	m := policy.Check("request", map[string]interface{}{"payload": json.RawMessage(payload)})
	if m == nil || m.MatchedText != "agency.gov" {
		t.Fatalf("deep JSON value not checked: %+v", m)
	}
}

// ── Rule ordering and coverage ───────────────────────────────────────────────

func TestRuleOrderingFirstWins(t *testing.T) {
	config := Config{Enabled: true, Rules: []Rule{
		{ID: "first",  Name: "First",  Enabled: true, Pattern: "payload-risk", Message: "{rule}/{tool}/{match}"},
		{ID: "second", Name: "Second", Enabled: true, Pattern: "tool-risk"},
	}}
	policy, _ := Compile(config)

	// "first" pattern is in the payload — should win even though the tool name matches "second".
	m := policy.Check("tool-risk", map[string]interface{}{"value": "payload-risk"})
	if m == nil || m.RuleID != "first" || m.Message != "First/tool-risk/payload-risk" {
		t.Fatalf("rule order wrong: %+v", m)
	}

	// Tool name itself triggers "second".
	m = policy.Check("tool-risk", nil)
	if m == nil || m.RuleID != "second" {
		t.Fatalf("tool name not checked: %+v", m)
	}
}

func TestRuleSerializedJSONValue(t *testing.T) {
	config := Config{Enabled: true, Rules: []Rule{
		{ID: "port", Name: "Port", Enabled: true, Pattern: `"port":443`},
	}}
	policy, _ := Compile(config)
	m := policy.Check("request", map[string]interface{}{"port": 443})
	if m == nil || m.MatchedText != `"port":443` {
		t.Fatalf("serialized arguments not checked: %+v", m)
	}
}

func TestDeterministicFieldTraversal(t *testing.T) {
	config := Config{Enabled: true, Rules: []Rule{
		{ID: "risk", Name: "Risk", Enabled: true, Pattern: `^risk.+$`},
	}}
	policy, _ := Compile(config)
	m := policy.Check("request", map[string]interface{}{"z": "risk-z", "a": "risk-a"})
	if m == nil || m.MatchedText != "risk-a" {
		t.Fatalf("field traversal not deterministic: %+v", m)
	}
}

// ── Template substitution safety ─────────────────────────────────────────────

func TestTemplateReplacementNotRecursive(t *testing.T) {
	config := Config{Enabled: true, Rules: []Rule{{
		ID: "tmpl", Name: "Rule", Enabled: true,
		Pattern: `\{tool\}`, Message: "{match}; {tool}; {rule}",
	}}}
	policy, _ := Compile(config)
	m := policy.Check("request", map[string]interface{}{"value": "{tool}"})
	if m == nil || m.Message != "{tool}; request; Rule" {
		t.Fatalf("template was recursive: %+v", m)
	}
}

// ── Percent decoding ──────────────────────────────────────────────────────────

func TestPercentDecodingBudgetPerField(t *testing.T) {
	config := Config{Enabled: true, Rules: []Rule{{
		ID: "domain", Name: "Domain", Enabled: true, Pattern: `^agency\.gov$`,
	}}}
	policy, _ := Compile(config)
	// Field "a" exhausts its budget at an intermediate form; field "b" decodes successfully.
	m := policy.Check("request", map[string]interface{}{
		"a": "agency%2525252egov",
		"b": "agency%2egov",
	})
	if m == nil || m.MatchedText != "agency.gov" {
		t.Fatalf("earlier value suppressed decoding: %+v", m)
	}
}

// ── Disabled settings ─────────────────────────────────────────────────────────

func TestDisabledPolicyAllowsAll(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false
	policy, _ := Compile(config)
	if policy.Check("request", map[string]interface{}{"target": "agency.gov"}) != nil {
		t.Fatal("disabled policy blocked a call")
	}
}

func TestDisabledRuleAllowsAll(t *testing.T) {
	config := DefaultConfig()
	config.Rules[0].Enabled = false
	policy, _ := Compile(config)
	if policy.Check("request", map[string]interface{}{"target": "agency.gov"}) != nil {
		t.Fatal("disabled rule blocked a call")
	}
}

func TestEmptyRulesAllowsAll(t *testing.T) {
	policy, _ := Compile(Config{Enabled: true, Rules: []Rule{}})
	if policy.Check("request", map[string]interface{}{"target": "agency.gov"}) != nil {
		t.Fatal("empty rule list blocked a call")
	}
}

// ── Compile validation ────────────────────────────────────────────────────────

func TestCompileValidationErrors(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Config)
	}{
		{"invalid_regex",        func(c *Config) { c.Rules[0].Pattern = "[" }},
		{"empty_regex",          func(c *Config) { c.Rules[0].Pattern = " " }},
		{"oversized_regex",      func(c *Config) { c.Rules[0].Pattern = strings.Repeat("a", MaxPatternLength+1) }},
		{"empty_id",             func(c *Config) { c.Rules[0].ID = " " }},
		{"padded_id",            func(c *Config) { c.Rules[0].ID = " id" }},
		{"oversized_id",         func(c *Config) { c.Rules[0].ID = strings.Repeat("a", MaxIDLength+1) }},
		{"duplicate_id",         func(c *Config) { c.Rules = append(c.Rules, c.Rules[0]) }},
		{"empty_name",           func(c *Config) { c.Rules[0].Name = " " }},
		{"oversized_name",       func(c *Config) { c.Rules[0].Name = strings.Repeat("a", MaxNameLength+1) }},
		{"oversized_message",    func(c *Config) { c.Rules[0].Message = strings.Repeat("a", MaxMessageLength+1) }},
		{"too_many_rules",       func(c *Config) { c.Rules = make([]Rule, MaxRules+1) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := DefaultConfig()
			tc.change(&config)
			if _, err := Compile(config); err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}

// ── Manager ───────────────────────────────────────────────────────────────────

func TestManagerImmutableSnapshots(t *testing.T) {
	config := DefaultConfig()
	mgr, err := NewManager(config)
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]interface{}{"target": "agency.gov"}

	// External mutation of original config must not affect active policy.
	config.Rules[0].Pattern = "safe"
	if mgr.Check("request", args) == nil {
		t.Fatal("external config mutation changed active policy")
	}

	// Config() copy must not affect active policy.
	snapshot := mgr.Config()
	snapshot.Rules[0].Enabled = false
	if mgr.Check("request", args) == nil {
		t.Fatal("Config() copy mutation changed active policy")
	}

	// Invalid update must preserve existing protection.
	invalid := DefaultConfig()
	invalid.Rules[0].Pattern = "["
	if err := mgr.Update(invalid); err == nil || mgr.Check("request", args) == nil {
		t.Fatal("invalid update broke protection")
	}

	// Valid update disabling protection must take effect.
	disabled := DefaultConfig()
	disabled.Enabled = false
	if err := mgr.Update(disabled); err != nil || mgr.Check("request", args) != nil {
		t.Fatal("valid disable update did not take effect")
	}
}

func TestManagerConcurrentSafety(t *testing.T) {
	mgr, _ := NewManager(DefaultConfig())
	args := map[string]interface{}{"target": "agency.gov"}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if mgr.Check("request", args) == nil {
					t.Error("concurrent update created unprotected window")
					return
				}
				cfg := mgr.Config()
				cfg.Rules[0].Message = "Block {match}"
				if err := mgr.Update(cfg); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestManagerNilSafe(t *testing.T) {
	var mgr *Manager
	if mgr.Check("request", map[string]interface{}{"target": "agency.gov"}) != nil {
		t.Fatal("nil Manager.Check should return nil")
	}
	if cfg := mgr.Config(); cfg.Enabled != false {
		t.Fatal("nil Manager.Config should return empty config")
	}
}
