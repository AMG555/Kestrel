// Package agents loads Markdown agent definitions from the agents/ directory (sub-agent + optional primary agent orchestrator.md / kind: orchestrator).
package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"kestrel/internal/config"

	"gopkg.in/yaml.v3"
)

// OrchestratorMarkdownFilename fixed filename: if present, treated as Deep primary agent definition and excluded from sub-agent list.
const OrchestratorMarkdownFilename = "orchestrator.md"

// OrchestratorPlanExecuteMarkdownFilename dedicated Markdown filename for plan_execute mode primary agent (planning side).
const OrchestratorPlanExecuteMarkdownFilename = "orchestrator-plan-execute.md"

// OrchestratorSupervisorMarkdownFilename dedicated Markdown filename for supervisor mode primary agent.
const OrchestratorSupervisorMarkdownFilename = "orchestrator-supervisor.md"

// FrontMatter corresponds to Markdown file header fields (consistent with documentation examples).
type FrontMatter struct {
	Name          string      `yaml:"name"`
	ID            string      `yaml:"id"`
	Description   string      `yaml:"description"`
	Tools         interface{} `yaml:"tools"` // string "A, B" or []string
	MaxIterations int         `yaml:"max_iterations"`
	BindRole      string      `yaml:"bind_role,omitempty"`
	Kind          string      `yaml:"kind,omitempty"` // orchestrator = primary agent (can also use filename orchestrator.md)
}

// OrchestratorMarkdown is the primary agent (Deep coordinator) definition parsed from the agents directory.
type OrchestratorMarkdown struct {
	Filename    string
	EinoName    string // written to deep.Config.Name / streaming event filter
	DisplayName string
	Description string
	Instruction string
}

// MarkdownDirLoad is the result of one scan of the agents directory (sub-agents exclude primary agent file).
type MarkdownDirLoad struct {
	SubAgents               []config.MultiAgentSubConfig
	Orchestrator            *OrchestratorMarkdown // Deep primary agent
	OrchestratorPlanExecute *OrchestratorMarkdown // plan_execute planning primary agent
	OrchestratorSupervisor  *OrchestratorMarkdown // supervisor primary agent
	FileEntries             []FileAgent           // includes primary agent and all sub-agents, for management API listing
}

// OrchestratorMarkdownKind returns primary agent type by fixed filename: deep, plan_execute, supervisor; otherwise returns empty.
func OrchestratorMarkdownKind(filename string) string {
	base := filepath.Base(strings.TrimSpace(filename))
	switch {
	case strings.EqualFold(base, OrchestratorPlanExecuteMarkdownFilename):
		return "plan_execute"
	case strings.EqualFold(base, OrchestratorSupervisorMarkdownFilename):
		return "supervisor"
	case strings.EqualFold(base, OrchestratorMarkdownFilename):
		return "deep"
	default:
		return ""
	}
}

// IsOrchestratorMarkdown determines if the file occupies the **Deep** primary agent slot: orchestrator.md or kind: orchestrator (excludes plan_execute / supervisor dedicated filenames).
func IsOrchestratorMarkdown(filename string, fm FrontMatter) bool {
	base := filepath.Base(strings.TrimSpace(filename))
	switch OrchestratorMarkdownKind(base) {
	case "plan_execute", "supervisor":
		return false
	}
	if strings.EqualFold(base, OrchestratorMarkdownFilename) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(fm.Kind), "orchestrator")
}

// IsOrchestratorLikeMarkdown returns whether the file should be displayed as a 'primary agent class' file in the frontend/API.
func IsOrchestratorLikeMarkdown(filename string, kind string) bool {
	if OrchestratorMarkdownKind(filename) != "" {
		return true
	}
	return IsOrchestratorMarkdown(filename, FrontMatter{Kind: kind})
}

// WantsMarkdownOrchestrator determines before saving whether the file will be used as a primary agent (for uniqueness validation).
func WantsMarkdownOrchestrator(filename string, kindField string, raw string) bool {
	base := filepath.Base(strings.TrimSpace(filename))
	if OrchestratorMarkdownKind(base) != "" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(kindField), "orchestrator") {
		return true
	}
	if strings.EqualFold(base, OrchestratorMarkdownFilename) {
		return true
	}
	if strings.TrimSpace(raw) == "" {
		return false
	}
	sub, err := ParseMarkdownSubAgent(filename, raw)
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(sub.Kind), "orchestrator")
}

// SplitFrontMatter separates YAML front matter from body (--- ... ---).
func SplitFrontMatter(content string) (frontYAML string, body string, err error) {
	s := strings.TrimSpace(content)
	if !strings.HasPrefix(s, "---") {
		return "", s, nil
	}
	rest := strings.TrimPrefix(s, "---")
	rest = strings.TrimLeft(rest, "\r\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", "", fmt.Errorf("agents: missing closing --- delimiter")
	}
	fm := strings.TrimSpace(rest[:end])
	body = strings.TrimSpace(rest[end+4:])
	body = strings.TrimLeft(body, "\r\n")
	return fm, body, nil
}

func parseToolsField(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case string:
		return splitToolList(t)
	case []interface{}:
		var out []string
		for _, x := range t {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case []string:
		var out []string
		for _, s := range t {
			if strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func splitToolList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '|'
	})
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// SlugID generates a usable agent id from name (lowercase, hyphenated).
func SlugID(name string) string {
	var b strings.Builder
	name = strings.TrimSpace(strings.ToLower(name))
	lastDash := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) && r < unicode.MaxASCII, unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '_' || r == '/' || r == '.':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "agent"
	}
	return s
}

// sanitizeEinoAgentID normalizes the Deep primary agent's Name in Eino: lowercase ASCII, digits, hyphens, consistent with default kestrel-deep.
func sanitizeEinoAgentID(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) && r < unicode.MaxASCII, unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '-':
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "kestrel-deep"
	}
	return out
}

func parseMarkdownAgentRaw(filename string, content string) (FrontMatter, string, error) {
	var fm FrontMatter
	fmStr, body, err := SplitFrontMatter(content)
	if err != nil {
		return fm, "", err
	}
	if strings.TrimSpace(fmStr) == "" {
		return fm, "", fmt.Errorf("agents: %s none YAML front matter", filename)
	}
	if err := yaml.Unmarshal([]byte(fmStr), &fm); err != nil {
		return fm, "", fmt.Errorf("agents: parsing front matter: %w", err)
	}
	return fm, body, nil
}

func orchestratorFromParsed(filename string, fm FrontMatter, body string) (*OrchestratorMarkdown, error) {
	display := strings.TrimSpace(fm.Name)
	if display == "" {
		display = "Orchestrator"
	}
	rawID := strings.TrimSpace(fm.ID)
	if rawID == "" {
		rawID = SlugID(display)
	}
	eino := sanitizeEinoAgentID(rawID)
	return &OrchestratorMarkdown{
		Filename:    filepath.Base(strings.TrimSpace(filename)),
		EinoName:    eino,
		DisplayName: display,
		Description: strings.TrimSpace(fm.Description),
		Instruction: strings.TrimSpace(body),
	}, nil
}

func orchestratorConfigFromOrchestrator(o *OrchestratorMarkdown) config.MultiAgentSubConfig {
	if o == nil {
		return config.MultiAgentSubConfig{}
	}
	return config.MultiAgentSubConfig{
		ID:          o.EinoName,
		Name:        o.DisplayName,
		Description: o.Description,
		Instruction: o.Instruction,
		Kind:        "orchestrator",
	}
}

func subAgentFromFrontMatter(filename string, fm FrontMatter, body string) (config.MultiAgentSubConfig, error) {
	var out config.MultiAgentSubConfig
	name := strings.TrimSpace(fm.Name)
	if name == "" {
		return out, fmt.Errorf("agents: %s missing name field", filename)
	}
	id := strings.TrimSpace(fm.ID)
	if id == "" {
		id = SlugID(name)
	}
	out.ID = id
	out.Name = name
	out.Description = strings.TrimSpace(fm.Description)
	out.Instruction = strings.TrimSpace(body)
	out.RoleTools = parseToolsField(fm.Tools)
	out.MaxIterations = fm.MaxIterations
	out.BindRole = strings.TrimSpace(fm.BindRole)
	out.Kind = strings.TrimSpace(fm.Kind)
	return out, nil
}

func collectMarkdownBasenames(dir string) ([]string, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, nil
	}
	st, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("agents: not a directory: %s", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasPrefix(n, ".") {
			continue
		}
		if !strings.EqualFold(filepath.Ext(n), ".md") {
			continue
		}
		if strings.EqualFold(n, "README.md") {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

// LoadMarkdownAgentsDir scans the agents directory: extracts at most one Deep / plan_execute / supervisor primary agent each, plus remaining sub-agents.
func LoadMarkdownAgentsDir(dir string) (*MarkdownDirLoad, error) {
	out := &MarkdownDirLoad{}
	names, err := collectMarkdownBasenames(dir)
	if err != nil {
		return nil, err
	}
	for _, n := range names {
		p := filepath.Join(dir, n)
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		fm, body, err := parseMarkdownAgentRaw(n, string(b))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		switch OrchestratorMarkdownKind(n) {
		case "plan_execute":
			if out.OrchestratorPlanExecute != nil {
				return nil, fmt.Errorf("agents: only one %s can be defined, already have %s", OrchestratorPlanExecuteMarkdownFilename, out.OrchestratorPlanExecute.Filename)
			}
			orch, err := orchestratorFromParsed(n, fm, body)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", n, err)
			}
			out.OrchestratorPlanExecute = orch
			out.FileEntries = append(out.FileEntries, FileAgent{
				Filename:       n,
				Config:         orchestratorConfigFromOrchestrator(orch),
				IsOrchestrator: true,
			})
			continue
		case "supervisor":
			if out.OrchestratorSupervisor != nil {
				return nil, fmt.Errorf("agents: only one %s can be defined, already have %s", OrchestratorSupervisorMarkdownFilename, out.OrchestratorSupervisor.Filename)
			}
			orch, err := orchestratorFromParsed(n, fm, body)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", n, err)
			}
			out.OrchestratorSupervisor = orch
			out.FileEntries = append(out.FileEntries, FileAgent{
				Filename:       n,
				Config:         orchestratorConfigFromOrchestrator(orch),
				IsOrchestrator: true,
			})
			continue
		}
		if IsOrchestratorMarkdown(n, fm) {
			if out.Orchestrator != nil {
				return nil, fmt.Errorf("agents: only one primary agent (Deep coordinator) can be defined, already have %s, conflicts with %s", out.Orchestrator.Filename, n)
			}
			orch, err := orchestratorFromParsed(n, fm, body)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", n, err)
			}
			out.Orchestrator = orch
			out.FileEntries = append(out.FileEntries, FileAgent{
				Filename:       n,
				Config:         orchestratorConfigFromOrchestrator(orch),
				IsOrchestrator: true,
			})
			continue
		}
		sub, err := subAgentFromFrontMatter(n, fm, body)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		out.SubAgents = append(out.SubAgents, sub)
		out.FileEntries = append(out.FileEntries, FileAgent{Filename: n, Config: sub, IsOrchestrator: false})
	}
	return out, nil
}

// ParseMarkdownSubAgent parses a single Markdown file into a MultiAgentSubConfig.
func ParseMarkdownSubAgent(filename string, content string) (config.MultiAgentSubConfig, error) {
	fm, body, err := parseMarkdownAgentRaw(filename, content)
	if err != nil {
		return config.MultiAgentSubConfig{}, err
	}
	if OrchestratorMarkdownKind(filename) != "" {
		orch, err := orchestratorFromParsed(filename, fm, body)
		if err != nil {
			return config.MultiAgentSubConfig{}, err
		}
		return orchestratorConfigFromOrchestrator(orch), nil
	}
	if IsOrchestratorMarkdown(filename, fm) {
		orch, err := orchestratorFromParsed(filename, fm, body)
		if err != nil {
			return config.MultiAgentSubConfig{}, err
		}
		return orchestratorConfigFromOrchestrator(orch), nil
	}
	return subAgentFromFrontMatter(filename, fm, body)
}

// LoadMarkdownSubAgents reads all sub-agent .md files under a directory (excludes primary agent orchestrator.md / kind: orchestrator).
func LoadMarkdownSubAgents(dir string) ([]config.MultiAgentSubConfig, error) {
	load, err := LoadMarkdownAgentsDir(dir)
	if err != nil {
		return nil, err
	}
	return load.SubAgents, nil
}

// FileAgent is a single Markdown file and its parsed result.
type FileAgent struct {
	Filename       string
	Config         config.MultiAgentSubConfig
	IsOrchestrator bool
}

// LoadMarkdownAgentFiles lists all .md files under a directory (including primary agent), for management API use.
func LoadMarkdownAgentFiles(dir string) ([]FileAgent, error) {
	load, err := LoadMarkdownAgentsDir(dir)
	if err != nil {
		return nil, err
	}
	return load.FileEntries, nil
}

// MergeYAMLAndMarkdown merges sub_agents from config.yaml with Markdown definitions: Markdown overrides YAML for same id; entries only in Markdown are appended after YAML order.
func MergeYAMLAndMarkdown(yamlSubs []config.MultiAgentSubConfig, mdSubs []config.MultiAgentSubConfig) []config.MultiAgentSubConfig {
	mdByID := make(map[string]config.MultiAgentSubConfig)
	for _, m := range mdSubs {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		mdByID[id] = m
	}
	yamlIDSet := make(map[string]bool)
	for _, y := range yamlSubs {
		yamlIDSet[strings.TrimSpace(y.ID)] = true
	}
	out := make([]config.MultiAgentSubConfig, 0, len(yamlSubs)+len(mdSubs))
	for _, y := range yamlSubs {
		id := strings.TrimSpace(y.ID)
		if id == "" {
			continue
		}
		if m, ok := mdByID[id]; ok {
			out = append(out, m)
		} else {
			out = append(out, y)
		}
	}
	for _, m := range mdSubs {
		id := strings.TrimSpace(m.ID)
		if id == "" || yamlIDSet[id] {
			continue
		}
		out = append(out, m)
	}
	return out
}

// EffectiveSubAgents for use by multi-agent runtime.
func EffectiveSubAgents(yamlSubs []config.MultiAgentSubConfig, agentsDir string) ([]config.MultiAgentSubConfig, error) {
	md, err := LoadMarkdownSubAgents(agentsDir)
	if err != nil {
		return nil, err
	}
	if len(md) == 0 {
		return yamlSubs, nil
	}
	return MergeYAMLAndMarkdown(yamlSubs, md), nil
}

// BuildMarkdownFile serializes config into Markdown that can be written back to disk.
func BuildMarkdownFile(sub config.MultiAgentSubConfig) ([]byte, error) {
	fm := FrontMatter{
		Name:          sub.Name,
		ID:            sub.ID,
		Description:   sub.Description,
		MaxIterations: sub.MaxIterations,
		BindRole:      sub.BindRole,
	}
	if k := strings.TrimSpace(sub.Kind); k != "" {
		fm.Kind = k
	}
	if len(sub.RoleTools) > 0 {
		fm.Tools = sub.RoleTools
	}
	head, err := yaml.Marshal(fm)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.Write(head)
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(sub.Instruction))
	if !strings.HasSuffix(sub.Instruction, "\n") && sub.Instruction != "" {
		b.WriteString("\n")
	}
	return []byte(b.String()), nil
}
