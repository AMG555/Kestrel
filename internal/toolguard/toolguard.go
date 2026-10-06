// Package toolguard applies configurable blocking rules before tool execution.
// It inspects tool names and arguments, including JSON string values and common
// percent-encoded forms. It is not an exhaustive target authorization check.
package toolguard

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
)

const (
	MaxRules         = 100
	MaxIDLength      = 128
	MaxNameLength    = 200
	MaxPatternLength = 4096
	MaxMessageLength = 4096

	// governmentDomainPattern matches .gov, .gov.cn, *.gov.* etc. with boundary
	// anchoring so that words like "governance" are not incorrectly matched.
	governmentDomainPattern = `(?i)(?:^|[^\p{L}\p{M}\p{N}_.-])(?P<match>(?:(?:[\p{L}\p{M}\p{N}_*-]+\.)+gov(?:\.[\p{L}\p{M}\p{N}_*-]+)*|gov(?:\.[\p{L}\p{M}\p{N}_*-]+)+|\.gov(?:\.[\p{L}\p{M}\p{N}_*-]+)*)\.?)(?:$|[^\p{L}\p{M}\p{N}_.-])`
	defaultMessage          = "Detected {match} — tool call blocked by rule \"{rule}\". Verify the target is within your authorized scope."
)

// Config holds toolguard settings.
type Config struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Rules   []Rule `json:"rules"   yaml:"rules"`
}

// Rule is a single blocking rule.
type Rule struct {
	ID      string `json:"id"      yaml:"id"`
	Name    string `json:"name"    yaml:"name"`
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Pattern string `json:"pattern" yaml:"pattern"`
	Message string `json:"message" yaml:"message"`
}

// Match is returned when a blocking rule fires.
type Match struct {
	RuleID      string `json:"rule_id"`
	RuleName    string `json:"rule_name"`
	MatchedText string `json:"matched_text"`
	Message     string `json:"message"`
}

// DefaultConfig enables a conservative government-domain blocking rule.
func DefaultConfig() Config {
	return Config{
		Enabled: true,
		Rules: []Rule{{
			ID:      "government-domains",
			Name:    "Government Domain Protection",
			Enabled: true,
			Pattern: governmentDomainPattern,
			Message: "Detected {match} — access to government domains is blocked by security policy. Verify your authorized scope.",
		}},
	}
}

type compiledRule struct {
	rule       Rule
	pattern    *regexp.Regexp
	matchGroup int
}

// Policy is an immutable validated snapshot, safe for concurrent Check calls.
type Policy struct {
	config Config
	rules  []compiledRule
}

// Compile validates all rules and returns an immutable Policy ready for use.
// Disabled rules are validated but not loaded into the active rule set.
func Compile(config Config) (*Policy, error) {
	if len(config.Rules) > MaxRules {
		return nil, fmt.Errorf("toolguard: at most %d rules allowed", MaxRules)
	}
	policy := &Policy{config: cloneConfig(config)}
	ids := make(map[string]struct{}, len(config.Rules))
	for i, rule := range policy.config.Rules {
		prefix := fmt.Sprintf("toolguard rule %d", i+1)
		if strings.TrimSpace(rule.ID) == "" || len(rule.ID) > MaxIDLength {
			return nil, fmt.Errorf("%s: id must be non-empty and at most %d bytes", prefix, MaxIDLength)
		}
		if rule.ID != strings.TrimSpace(rule.ID) {
			return nil, fmt.Errorf("%s: id must not have surrounding whitespace", prefix)
		}
		if _, exists := ids[rule.ID]; exists {
			return nil, fmt.Errorf("%s: duplicate id %q", prefix, rule.ID)
		}
		ids[rule.ID] = struct{}{}
		if strings.TrimSpace(rule.Name) == "" || len(rule.Name) > MaxNameLength {
			return nil, fmt.Errorf("%s: name must be non-empty and at most %d bytes", prefix, MaxNameLength)
		}
		if strings.TrimSpace(rule.Pattern) == "" || len(rule.Pattern) > MaxPatternLength {
			return nil, fmt.Errorf("%s: pattern must be non-empty and at most %d bytes", prefix, MaxPatternLength)
		}
		if len(rule.Message) > MaxMessageLength {
			return nil, fmt.Errorf("%s: message must be at most %d bytes", prefix, MaxMessageLength)
		}
		compiled, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return nil, fmt.Errorf("%s (%s): invalid regexp: %w", prefix, rule.ID, err)
		}
		if rule.Enabled {
			policy.rules = append(policy.rules, compiledRule{
				rule:       rule,
				pattern:    compiled,
				matchGroup: compiled.SubexpIndex("match"),
			})
		}
	}
	return policy, nil
}

// Check returns the first blocking Match, or nil if the call is allowed.
// Args must not be mutated while Check is running.
func (p *Policy) Check(toolName string, args map[string]interface{}) *Match {
	if p == nil || !p.config.Enabled || len(p.rules) == 0 {
		return nil
	}
	candidates := candidateTexts(toolName, args)
	for _, rule := range p.rules {
		for _, candidate := range candidates {
			indices := rule.pattern.FindStringSubmatchIndex(candidate)
			if indices == nil {
				continue
			}
			start, end := indices[0], indices[1]
			if g := rule.matchGroup; g > 0 && indices[g*2] >= 0 {
				start, end = indices[g*2], indices[g*2+1]
			}
			matched := candidate[start:end]
			msg := rule.rule.Message
			if strings.TrimSpace(msg) == "" {
				msg = defaultMessage
			}
			// Single-pass replacement prevents matched text from introducing
			// additional template substitutions.
			msg = strings.NewReplacer(
				"{match}", matched,
				"{tool}", toolName,
				"{rule}", rule.rule.Name,
			).Replace(msg)
			return &Match{
				RuleID:      rule.rule.ID,
				RuleName:    rule.rule.Name,
				MatchedText: matched,
				Message:     msg,
			}
		}
	}
	return nil
}

// ── Manager — live atomic policy swapping ────────────────────────────────────

// Manager holds a live policy that can be atomically updated.
type Manager struct {
	policy atomic.Pointer[Policy]
}

// NewManager creates a Manager initialized with the provided config.
func NewManager(config Config) (*Manager, error) {
	m := &Manager{}
	if err := m.Update(config); err != nil {
		return nil, err
	}
	return m, nil
}

// Update validates and atomically replaces the active policy.
// On validation failure, the existing policy is preserved.
func (m *Manager) Update(config Config) error {
	policy, err := Compile(config)
	if err != nil {
		return err
	}
	m.policy.Store(policy)
	return nil
}

// Config returns a deep copy of the active policy's configuration.
func (m *Manager) Config() Config {
	if m == nil {
		return Config{}
	}
	if p := m.policy.Load(); p != nil {
		return cloneConfig(p.config)
	}
	return Config{}
}

// Check delegates to the active policy.
func (m *Manager) Check(toolName string, args map[string]interface{}) *Match {
	if m == nil {
		return nil
	}
	return m.policy.Load().Check(toolName, args)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func cloneConfig(config Config) Config {
	if config.Rules != nil {
		rules := make([]Rule, len(config.Rules))
		copy(rules, config.Rules)
		config.Rules = rules
	}
	return config
}

// candidateTexts collects every string value reachable from the tool call.
// Values are percent-decoded up to 3 rounds to cover common nested escaping.
func candidateTexts(toolName string, args map[string]interface{}) []string {
	var candidates []string
	seen := make(map[string]struct{})

	add := func(value string) {
		for round := 0; round <= 3; round++ {
			if _, exists := seen[value]; !exists {
				seen[value] = struct{}{}
				candidates = append(candidates, value)
			}
			decoded := decodePercentEscapes(value)
			if decoded == value {
				break
			}
			value = decoded
		}
	}
	add(toolName)

	var walk func(interface{}, int)
	walk = func(value interface{}, depth int) {
		if depth == 0 {
			return
		}
		switch v := value.(type) {
		case string:
			add(v)
		case map[string]interface{}:
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				add(k)
				walk(v[k], depth-1)
			}
		case []interface{}:
			for _, item := range v {
				walk(item, depth-1)
			}
		}
	}

	// Marshal+unmarshal normalizes json.RawMessage, typed slices, and Unicode escapes.
	if raw, err := json.Marshal(args); err == nil {
		var decoded interface{}
		if json.Unmarshal(raw, &decoded) == nil {
			walk(decoded, 10001)
		}
		add(string(raw))
	} else {
		walk(args, 128)
	}
	return candidates
}

// decodePercentEscapes decodes valid %XX triplets while ignoring stray percent signs.
func decodePercentEscapes(value string) string {
	if !strings.Contains(value, "%") {
		return value
	}
	var out strings.Builder
	out.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] == '%' && i+2 < len(value) {
			hi, okH := hexVal(value[i+1])
			lo, okL := hexVal(value[i+2])
			if okH && okL {
				out.WriteByte(hi<<4 | lo)
				i += 2
				continue
			}
		}
		out.WriteByte(value[i])
	}
	return out.String()
}

func hexVal(b byte) (byte, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	default:
		return 0, false
	}
}
