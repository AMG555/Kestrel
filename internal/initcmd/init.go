package initcmd

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kestrel/internal/config"
	"kestrel/internal/database"
	"kestrel/internal/toolguard"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// InitOptions contains parameters to initialize a new pentest engagement.
type InitOptions struct {
	Name        string
	Target      string
	OutDir      string
	Description string
	Actor       string
}

// InitResult summarizes the created engagement artifacts.
type InitResult struct {
	ProjectID      string                    `json:"project_id"`
	ProjectName    string                    `json:"project_name"`
	ScopeFile      string                    `json:"scope_file"`
	CheckpointFile string                    `json:"checkpoint_file"`
	HeadHash       string                    `json:"head_hash"`
	Seal           string                    `json:"seal"`
	Checkpoint     *database.AuditCheckpoint `json:"checkpoint"`
}

// RunEngagementInit creates scope.yaml, registers the project in the database,
// appends the initial audit trail entry, and exports a cryptographic anchor checkpoint.
func RunEngagementInit(cfg *config.Config, opts InitOptions) (*InitResult, error) {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "Engagement-" + time.Now().UTC().Format("2006-01-02")
	}

	outDir := strings.TrimSpace(opts.OutDir)
	if outDir == "" {
		outDir = "."
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output directory %s: %w", outDir, err)
	}

	target := strings.TrimSpace(opts.Target)
	inScopeCIDRs := []string{}
	inScopeDomains := []string{}
	inScopeURLs := []string{}

	if target != "" {
		parseTargetIntoScope(target, &inScopeCIDRs, &inScopeDomains, &inScopeURLs)
	}

	// 1. Generate scope.yaml
	failClosed := true
	blockIMDS := true
	allowLocal := false
	scopeCfg := toolguard.ScopeConfigFile{
		Version:    1,
		TargetName: name,
		InScope: toolguard.ScopeRuleGroup{
			CIDRs:   inScopeCIDRs,
			Domains: inScopeDomains,
			URLs:    inScopeURLs,
		},
		OutOfScope: toolguard.ScopeRuleGroup{
			CIDRs: []string{"169.254.169.254/32"},
		},
		Options: toolguard.ScopeOptions{
			FailClosed:         &failClosed,
			BlockCloudMetadata: &blockIMDS,
			AllowLocalhost:     &allowLocal,
		},
	}

	scopeData, err := yaml.Marshal(&scopeCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal scope configuration: %w", err)
	}
	scopePath := filepath.Join(outDir, "scope.yaml")
	if err := os.WriteFile(scopePath, scopeData, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write scope file %s: %w", scopePath, err)
	}

	// 2. Open or create database
	dbPath := strings.TrimSpace(cfg.Database.Path)
	if dbPath == "" {
		dbPath = "data/conversations.db"
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}
	db, err := database.NewDB(dbPath, zap.NewNop())
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// 3. Register project
	projectID := uuid.NewString()
	desc := opts.Description
	if desc == "" {
		desc = fmt.Sprintf("Engagement initialized with scope: %s", target)
	}
	scopeJSONBytes, _ := json.Marshal(scopeCfg)
	proj := &database.Project{
		ID:          projectID,
		Name:        name,
		Description: desc,
		ScopeJSON:   string(scopeJSONBytes),
		Status:      "active",
	}
	if _, err := db.CreateProject(proj); err != nil {
		return nil, fmt.Errorf("failed to register project: %w", err)
	}

	// 4. Record initial audit log entry
	actor := opts.Actor
	if actor == "" {
		actor = "operator"
	}
	if err := db.AppendAuditLog(&database.AuditLog{
		ID:           fmt.Sprintf("audit-init-%d", time.Now().UnixNano()),
		CreatedAt:    time.Now().UTC(),
		Level:        "info",
		Category:     "engagement",
		Action:       "engagement_initialized",
		Result:       "success",
		Actor:        actor,
		ResourceType: "project",
		ResourceID:   projectID,
		Message:      fmt.Sprintf("Engagement %q initialized with scope target %q", name, target),
	}); err != nil {
		return nil, fmt.Errorf("failed to append initial audit log: %w", err)
	}

	// 5. Generate external cryptographic checkpoint receipt
	cp, err := db.CreateAuditCheckpoint()
	if err != nil {
		return nil, fmt.Errorf("failed to generate audit checkpoint: %w", err)
	}

	cpData, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format checkpoint: %w", err)
	}
	cpPath := filepath.Join(outDir, "audit-checkpoint.json")
	if err := os.WriteFile(cpPath, cpData, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write checkpoint file %s: %w", cpPath, err)
	}

	return &InitResult{
		ProjectID:      projectID,
		ProjectName:    name,
		ScopeFile:      scopePath,
		CheckpointFile: cpPath,
		HeadHash:       cp.HeadHash,
		Seal:           cp.Seal,
		Checkpoint:     cp,
	}, nil
}

func parseTargetIntoScope(target string, cidrs *[]string, domains *[]string, urls *[]string) {
	// 1. Check if CIDR
	if _, _, err := net.ParseCIDR(target); err == nil {
		*cidrs = append(*cidrs, target)
		return
	}

	// 2. Check if IP
	if ip := net.ParseIP(target); ip != nil {
		if ip.To4() != nil {
			*cidrs = append(*cidrs, target+"/32")
		} else {
			*cidrs = append(*cidrs, target+"/128")
		}
		return
	}

	// 3. Check if URL
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		*urls = append(*urls, target)
		parsed, err := url.Parse(target)
		if err == nil && parsed.Hostname() != "" {
			host := parsed.Hostname()
			if ip := net.ParseIP(host); ip != nil {
				*cidrs = append(*cidrs, host+"/32")
			} else {
				*domains = append(*domains, host)
				if strings.Count(host, ".") >= 1 && !strings.HasPrefix(host, "*.") {
					*domains = append(*domains, "*."+host)
				}
			}
		}
		return
	}

	// 4. Default as domain
	*domains = append(*domains, target)
	if strings.Count(target, ".") >= 1 && !strings.HasPrefix(target, "*.") {
		*domains = append(*domains, "*."+target)
	}
}
