// Package toolrecipe provides indexing, querying, and parameter schemas for curated security tool recipes.
package toolrecipe

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Parameter defines a command-line flag or argument for a tool recipe.
type Parameter struct {
	Name        string      `yaml:"name" json:"name"`
	Type        string      `yaml:"type" json:"type"`
	Description string      `yaml:"description" json:"description"`
	Required    bool        `yaml:"required" json:"required"`
	Flag        string      `yaml:"flag,omitempty" json:"flag,omitempty"`
	Format      string      `yaml:"format,omitempty" json:"format,omitempty"`
	Position    int         `yaml:"position,omitempty" json:"position,omitempty"`
	Template    string      `yaml:"template,omitempty" json:"template,omitempty"`
	Default     interface{} `yaml:"default,omitempty" json:"default,omitempty"`
}

// Recipe represents a declarative YAML tool recipe.
type Recipe struct {
	Name             string      `yaml:"name" json:"name"`
	Command          string      `yaml:"command" json:"command"`
	Args             []string    `yaml:"args,omitempty" json:"args,omitempty"`
	Enabled          bool        `yaml:"enabled" json:"enabled"`
	ShortDescription string      `yaml:"short_description" json:"short_description"`
	Description      string      `yaml:"description" json:"description"`
	Category         string      `yaml:"category,omitempty" json:"category,omitempty"`
	RiskLevel        string      `yaml:"risk_level,omitempty" json:"risk_level,omitempty"`
	Parameters       []Parameter `yaml:"parameters" json:"parameters"`
	FilePath         string      `yaml:"-" json:"file_path,omitempty"`
}

// Registry indexes all loaded tool recipes.
type Registry struct {
	mu      sync.RWMutex
	dir     string
	recipes map[string]*Recipe
}

// NewRegistry initializes and loads recipes from the specified directory.
func NewRegistry(toolsDir string) (*Registry, error) {
	r := &Registry{
		dir:     toolsDir,
		recipes: make(map[string]*Recipe),
	}
	if err := r.Reload(); err != nil {
		return nil, err
	}
	return r, nil
}

// Reload rescans the tools directory and loads all .yaml recipes.
func (r *Registry) Reload() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entries, err := os.ReadDir(r.dir)
	if err != nil {
		if os.IsNotExist(err) {
			r.recipes = make(map[string]*Recipe)
			return nil
		}
		return fmt.Errorf("reading tools directory: %w", err)
	}

	newMap := make(map[string]*Recipe)
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}

		fullPath := filepath.Join(r.dir, entry.Name())
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		var recipe Recipe
		if err := yaml.Unmarshal(data, &recipe); err != nil {
			continue
		}

		if recipe.Name == "" {
			recipe.Name = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
		recipe.FilePath = fullPath

		// Categorize by purpose heuristics if not explicit
		if recipe.Category == "" {
			recipe.Category = inferCategory(recipe.Name, recipe.Description)
		}

		newMap[recipe.Name] = &recipe
	}

	r.recipes = newMap
	return nil
}

// List returns all loaded recipes sorted by name.
func (r *Registry) List() []*Recipe {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*Recipe, 0, len(r.recipes))
	for _, rec := range r.recipes {
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// Get returns a specific recipe by name.
func (r *Registry) Get(name string) (*Recipe, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, ok := r.recipes[name]
	return rec, ok
}

// Count returns the total number of recipes.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.recipes)
}

func inferCategory(name, desc string) string {
	combined := strings.ToLower(name + " " + desc)
	switch {
	case strings.Contains(combined, "recon") || strings.Contains(combined, "subdomain") || strings.Contains(combined, "dns") || strings.Contains(combined, "whois") || strings.Contains(combined, "fingerprint"):
		return "reconnaissance"
	case strings.Contains(combined, "scan") || strings.Contains(combined, "port") || strings.Contains(combined, "nmap") || strings.Contains(combined, "service"):
		return "scanning"
	case strings.Contains(combined, "fuzz") || strings.Contains(combined, "dirsearch") || strings.Contains(combined, "gobuster") || strings.Contains(combined, "ffuf"):
		return "content_discovery"
	case strings.Contains(combined, "vuln") || strings.Contains(combined, "nuclei") || strings.Contains(combined, "cve") || strings.Contains(combined, "injection") || strings.Contains(combined, "sqlmap"):
		return "vulnerability_audit"
	case strings.Contains(combined, "cloud") || strings.Contains(combined, "aws") || strings.Contains(combined, "kube") || strings.Contains(combined, "container"):
		return "cloud_container"
	case strings.Contains(combined, "binary") || strings.Contains(combined, "pwn") || strings.Contains(combined, "reverse") || strings.Contains(combined, "ghidra") || strings.Contains(combined, "radare2"):
		return "binary_reversing"
	case strings.Contains(combined, "password") || strings.Contains(combined, "hash") || strings.Contains(combined, "hydra") || strings.Contains(combined, "brute"):
		return "credential_assessment"
	default:
		return "general_security"
	}
}
