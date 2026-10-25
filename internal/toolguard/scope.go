package toolguard

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// CloudMetadataEndpoints defines known cloud provider instance metadata endpoints that must be protected.
var CloudMetadataEndpoints = []string{
	"169.254.169.254",
	"fd00:ec2::254",
	"metadata.google.internal",
	"169.254.169.253",
	"169.254.169.250",
	"instance-data",
}

// ProjectScope specifies target authorization boundaries for security assessments.
type ProjectScope struct {
	Enabled bool     `json:"enabled"`
	Targets []string `json:"targets"` // Authorized domains, wildcards (*.domain.com), CIDRs (10.0.0.0/8)
	Exclude []string `json:"exclude"` // Explicitly prohibited subdomains or IP ranges
}

// ScopeEvaluationResult represents the outcome of target scope validation.
type ScopeEvaluationResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
	Host    string `json:"host"`
}

// ExtractHost parses a raw target string (URL, host:port, or domain) and extracts the canonical hostname or IP.
func ExtractHost(target string) string {
	t := strings.TrimSpace(target)
	if t == "" {
		return ""
	}

	// If it has a scheme, parse as URL
	if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") || strings.HasPrefix(t, "ws://") || strings.HasPrefix(t, "wss://") {
		if u, err := url.Parse(t); err == nil && u.Hostname() != "" {
			return strings.ToLower(u.Hostname())
		}
	}

	// Remove trailing paths / slashes if user passed "example.com/api" without scheme
	if slashIdx := strings.Index(t, "/"); slashIdx != -1 {
		t = t[:slashIdx]
	}

	// If it contains a port like "example.com:8080" or "127.0.0.1:443"
	if h, _, err := net.SplitHostPort(t); err == nil {
		return strings.ToLower(h)
	}

	return strings.ToLower(strings.Trim(t, "[]"))
}

// IsCloudMetadataEndpoint checks whether the target points to known cloud instance metadata services (IMDS).
func IsCloudMetadataEndpoint(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	for _, m := range CloudMetadataEndpoints {
		if h == m {
			return true
		}
	}

	// Check if IP is in the 169.254.0.0/16 IPv4 link-local subnet
	if ip := net.ParseIP(h); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			if ip4[0] == 169 && ip4[1] == 254 {
				return true
			}
		}
	}
	return false
}

// MatchDomainPattern checks whether a candidate host matches a domain or wildcard pattern.
// Examples:
// - Pattern "example.com" matches "example.com" and "sub.example.com"
// - Pattern "*.example.com" matches "sub.example.com" and "deep.sub.example.com"
// - Rejects "attacker-example.com" or "example.com.attacker.com"
func MatchDomainPattern(host, pattern string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	p := strings.ToLower(strings.TrimSpace(pattern))

	if h == "" || p == "" {
		return false
	}

	// Remove leading *. if present
	if strings.HasPrefix(p, "*.") {
		baseDomain := p[2:]
		if h == baseDomain {
			return true
		}
		return strings.HasSuffix(h, "."+baseDomain)
	}

	// Exact match
	if h == p {
		return true
	}

	// Subdomain match (if pattern is "example.com", "sub.example.com" matches)
	if strings.HasSuffix(h, "."+p) {
		return true
	}

	return false
}

// MatchCIDRPattern checks if an IP belongs to a CIDR or matches an IP pattern.
func MatchCIDRPattern(host, pattern string) bool {
	candidateIP := net.ParseIP(host)
	if candidateIP == nil {
		return false
	}

	// If pattern is a CIDR notation like 192.168.1.0/24
	if strings.Contains(pattern, "/") {
		_, ipNet, err := net.ParseCIDR(strings.TrimSpace(pattern))
		if err == nil && ipNet != nil {
			return ipNet.Contains(candidateIP)
		}
	}

	// Single IP match
	patternIP := net.ParseIP(strings.TrimSpace(pattern))
	if patternIP != nil {
		return candidateIP.Equal(patternIP)
	}

	return false
}

// ValidateTargetScope evaluates whether a target is permitted under the given ProjectScope.
func ValidateTargetScope(target string, scope ProjectScope) ScopeEvaluationResult {
	host := ExtractHost(target)
	if host == "" {
		return ScopeEvaluationResult{
			Allowed: true,
			Reason:  "target host could not be parsed; passing to default filter",
			Host:    "",
		}
	}

	// 1. Mandatory Security Rule: Always block cloud instance metadata (IMDS)
	if IsCloudMetadataEndpoint(host) {
		return ScopeEvaluationResult{
			Allowed: false,
			Reason:  fmt.Sprintf("Cloud metadata endpoint (%s) access is strictly forbidden by ToolGuard", host),
			Host:    host,
		}
	}

	if !scope.Enabled {
		return ScopeEvaluationResult{
			Allowed: true,
			Reason:  "project scope enforcement is disabled",
			Host:    host,
		}
	}

	// 2. Check Exclude list first
	for _, excl := range scope.Exclude {
		exclTrim := strings.TrimSpace(excl)
		if exclTrim == "" {
			continue
		}

		if MatchCIDRPattern(host, exclTrim) {
			return ScopeEvaluationResult{
				Allowed: false,
				Reason:  fmt.Sprintf("Target IP %s matches explicitly excluded CIDR/IP rule %s", host, exclTrim),
				Host:    host,
			}
		}

		if MatchDomainPattern(host, exclTrim) {
			return ScopeEvaluationResult{
				Allowed: false,
				Reason:  fmt.Sprintf("Target host %s matches explicitly excluded domain rule %s", host, exclTrim),
				Host:    host,
			}
		}
	}

	// 3. If Targets list is configured, candidate MUST match at least one allowed target
	if len(scope.Targets) > 0 {
		matched := false
		var matchedRule string

		for _, allowed := range scope.Targets {
			allowTrim := strings.TrimSpace(allowed)
			if allowTrim == "" {
				continue
			}

			if MatchCIDRPattern(host, allowTrim) {
				matched = true
				matchedRule = allowTrim
				break
			}

			if MatchDomainPattern(host, allowTrim) {
				matched = true
				matchedRule = allowTrim
				break
			}
		}

		if !matched {
			return ScopeEvaluationResult{
				Allowed: false,
				Reason:  fmt.Sprintf("Target host %s is OUT OF SCOPE. Must match one of authorized targets: %s", host, strings.Join(scope.Targets, ", ")),
				Host:    host,
			}
		}

		return ScopeEvaluationResult{
			Allowed: true,
			Reason:  fmt.Sprintf("Target %s matches authorized scope rule %s", host, matchedRule),
			Host:    host,
		}
	}

	// If no explicit targets specified but not excluded and not metadata, permit
	return ScopeEvaluationResult{
		Allowed: true,
		Reason:  "target permitted (no restrictive target constraints configured)",
		Host:    host,
	}
}
