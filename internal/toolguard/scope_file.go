package toolguard

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ScopeConfigFile represents the YAML structure of an engagement scope file (scope.yaml).
type ScopeConfigFile struct {
	Version    int            `yaml:"version"`
	TargetName string         `yaml:"target_name"`
	InScope    ScopeRuleGroup `yaml:"in_scope"`
	OutOfScope ScopeRuleGroup `yaml:"out_of_scope"`
	Options    ScopeOptions   `yaml:"options"`
}

// ScopeRuleGroup defines CIDR ranges, domain names, and URLs.
type ScopeRuleGroup struct {
	CIDRs   []string `yaml:"cidrs"`
	Domains []string `yaml:"domains"`
	URLs    []string `yaml:"urls"`
}

// ScopeOptions configures fail-closed behavior and special target handling.
type ScopeOptions struct {
	FailClosed         *bool `yaml:"fail_closed"`
	BlockCloudMetadata *bool `yaml:"block_cloud_metadata"`
	AllowLocalhost     *bool `yaml:"allow_localhost"`
}

// ScopeFilePolicy contains pre-parsed and compiled scope rules for network-layer evaluation.
type ScopeFilePolicy struct {
	TargetName         string
	FailClosed         bool
	BlockCloudMetadata bool
	AllowLocalhost     bool

	InScopeCIDRs   []*net.IPNet
	InScopeDomains []string
	InScopeURLs    []string

	OutOfScopeCIDRs   []*net.IPNet
	OutOfScopeDomains []string
	OutOfScopeURLs    []string
}

// LoadScopeFile reads and compiles a scope.yaml file from disk.
func LoadScopeFile(path string) (*ScopeFilePolicy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read scope file %s: %w", path, err)
	}

	var cfg ScopeConfigFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse scope YAML %s: %w", path, err)
	}

	policy := &ScopeFilePolicy{
		TargetName:         cfg.TargetName,
		FailClosed:         true,
		BlockCloudMetadata: true,
		AllowLocalhost:     true,
	}

	if cfg.Options.FailClosed != nil {
		policy.FailClosed = *cfg.Options.FailClosed
	}
	if cfg.Options.BlockCloudMetadata != nil {
		policy.BlockCloudMetadata = *cfg.Options.BlockCloudMetadata
	}
	if cfg.Options.AllowLocalhost != nil {
		policy.AllowLocalhost = *cfg.Options.AllowLocalhost
	}

	// Parse InScope CIDRs
	for _, raw := range cfg.InScope.CIDRs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		ipNet, err := parseCIDROrSingleIP(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid in_scope CIDR %q: %w", raw, err)
		}
		policy.InScopeCIDRs = append(policy.InScopeCIDRs, ipNet)
	}
	for _, d := range cfg.InScope.Domains {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			policy.InScopeDomains = append(policy.InScopeDomains, d)
		}
	}
	for _, u := range cfg.InScope.URLs {
		if u = strings.TrimSpace(u); u != "" {
			policy.InScopeURLs = append(policy.InScopeURLs, u)
		}
	}

	// Parse OutOfScope CIDRs
	for _, raw := range cfg.OutOfScope.CIDRs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		ipNet, err := parseCIDROrSingleIP(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid out_of_scope CIDR %q: %w", raw, err)
		}
		policy.OutOfScopeCIDRs = append(policy.OutOfScopeCIDRs, ipNet)
	}
	for _, d := range cfg.OutOfScope.Domains {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			policy.OutOfScopeDomains = append(policy.OutOfScopeDomains, d)
		}
	}
	for _, u := range cfg.OutOfScope.URLs {
		if u = strings.TrimSpace(u); u != "" {
			policy.OutOfScopeURLs = append(policy.OutOfScopeURLs, u)
		}
	}

	return policy, nil
}

func parseCIDROrSingleIP(raw string) (*net.IPNet, error) {
	if strings.Contains(raw, "/") {
		_, ipNet, err := net.ParseCIDR(raw)
		return ipNet, err
	}
	ip := net.ParseIP(raw)
	if ip == nil {
		return nil, fmt.Errorf("not a valid IP or CIDR notation: %s", raw)
	}
	mask := net.CIDRMask(32, 32)
	if ip.To4() == nil {
		mask = net.CIDRMask(128, 128)
	}
	return &net.IPNet{IP: ip, Mask: mask}, nil
}

// EvaluateDestination checks whether destination host/IP passes all in-scope and out-of-scope rules.
func (p *ScopeFilePolicy) EvaluateDestination(ctx context.Context, host string, resolver *net.Resolver) (bool, string) {
	if p == nil {
		return true, "no scope policy enforced"
	}

	canonicalHost := ExtractHost(host)
	if canonicalHost == "" {
		if p.FailClosed {
			return false, "empty destination host rejected (fail-closed)"
		}
		return true, "empty destination host permitted"
	}

	// 1. Mandatory Cloud Instance Metadata Service (IMDS) check
	if p.BlockCloudMetadata && IsCloudMetadataEndpoint(canonicalHost) {
		return false, fmt.Sprintf("destination %s matches forbidden cloud metadata endpoint (IMDS)", canonicalHost)
	}

	// Localhost handling
	isLocal := canonicalHost == "localhost" || canonicalHost == "127.0.0.1" || canonicalHost == "::1"
	if isLocal && p.AllowLocalhost {
		// If localhost is allowed, check if explicitly prohibited in out_of_scope
		for _, exclDomain := range p.OutOfScopeDomains {
			if MatchDomainPattern(canonicalHost, exclDomain) {
				return false, fmt.Sprintf("localhost destination %s matches out-of-scope domain %s", canonicalHost, exclDomain)
			}
		}
		return true, "destination is permitted localhost"
	}

	// 2. OutOfScope Domains check
	for _, exclDomain := range p.OutOfScopeDomains {
		if MatchDomainPattern(canonicalHost, exclDomain) {
			return false, fmt.Sprintf("destination %s matches out-of-scope domain rule %s", canonicalHost, exclDomain)
		}
	}

	// 3. Check if domain is explicitly matched in in_scope
	domainMatched := false
	matchedRuleName := ""
	matchedRuleKind := "domain"
	for _, inDomain := range p.InScopeDomains {
		if MatchDomainPattern(canonicalHost, inDomain) {
			domainMatched = true
			matchedRuleName = inDomain
			matchedRuleKind = "domain"
			break
		}
	}
	if !domainMatched {
		for _, inURL := range p.InScopeURLs {
			if ExtractHost(inURL) == canonicalHost {
				domainMatched = true
				matchedRuleName = inURL
				matchedRuleKind = "target URL"
				break
			}
		}
	}

	// 4. Resolve destination IP(s)
	var resolvedIPs []net.IP
	if parsedIP := net.ParseIP(canonicalHost); parsedIP != nil {
		resolvedIPs = append(resolvedIPs, parsedIP)
	} else if resolver != nil {
		dnsCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		addrs, err := resolver.LookupIPAddr(dnsCtx, canonicalHost)
		if err != nil {
			if !domainMatched && p.FailClosed {
				return false, fmt.Sprintf("DNS resolution failed for %s and fail_closed=true: %v", canonicalHost, err)
			}
		} else {
			for _, a := range addrs {
				resolvedIPs = append(resolvedIPs, a.IP)
			}
		}
	}

	// 5. OutOfScope CIDR checks on resolved IPs
	for _, ip := range resolvedIPs {
		if p.BlockCloudMetadata && IsCloudMetadataEndpoint(ip.String()) {
			return false, fmt.Sprintf("resolved IP %s matches forbidden cloud metadata endpoint", ip.String())
		}
		for _, exclCIDR := range p.OutOfScopeCIDRs {
			if exclCIDR.Contains(ip) {
				return false, fmt.Sprintf("destination IP %s matches out-of-scope CIDR %s", ip.String(), exclCIDR.String())
			}
		}
	}

	// 6. InScope checks
	hasInScopeRules := len(p.InScopeDomains) > 0 || len(p.InScopeCIDRs) > 0 || len(p.InScopeURLs) > 0
	if !hasInScopeRules {
		return true, "permitted (no restrictive in-scope allowlist configured)"
	}

	if domainMatched {
		return true, fmt.Sprintf("destination %s matches authorized in-scope %s %s", canonicalHost, matchedRuleKind, matchedRuleName)
	}

	// CIDR match on resolved IPs
	for _, ip := range resolvedIPs {
		for _, inCIDR := range p.InScopeCIDRs {
			if inCIDR.Contains(ip) {
				return true, fmt.Sprintf("destination IP %s matches authorized in-scope CIDR %s", ip.String(), inCIDR.String())
			}
		}
	}

	if p.FailClosed {
		return false, fmt.Sprintf("destination %s (IPs: %v) is not in authorized scope allowlist (fail-closed)", canonicalHost, resolvedIPs)
	}

	return true, "permitted by fallback"
}
