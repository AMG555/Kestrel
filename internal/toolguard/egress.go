package toolguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrScopeEgressBlocked is returned when an outbound socket is blocked by network egress policy.
	ErrScopeEgressBlocked = errors.New("connection dropped by network egress scope guard")

	globalGuardMu sync.RWMutex
	globalGuard   *EgressGuard
)

// EgressGuard enforces network-layer outbound connection filtering against a ScopeFilePolicy.
type EgressGuard struct {
	policy      *ScopeFilePolicy
	resolver    *net.Resolver
	exemptMu    sync.RWMutex
	exemptHosts map[string]bool
	allowIMDS   atomic.Bool
}

// NewEgressGuard creates an EgressGuard with the given policy and default plumbing exemptions.
func NewEgressGuard(policy *ScopeFilePolicy) *EgressGuard {
	g := &EgressGuard{
		policy: policy,
		resolver: &net.Resolver{
			PreferGo: true,
		},
		exemptHosts: make(map[string]bool),
	}

	// Default infrastructure plumbing exemptions:
	// LLM providers, local inference, alert bots, DNS/loopback
	g.AddExemptHosts(
		"api.openai.com",
		"api.anthropic.com",
		"generativelanguage.googleapis.com",
		"api.groq.com",
		"api.deepseek.com",
		"localhost",
		"127.0.0.1",
		"::1",
		"api.telegram.org",
		"hooks.slack.com",
		"discord.com",
		"open.feishu.cn",
		"oapi.dingtalk.com",
		"qyapi.weixin.qq.com",
	)

	return g
}

// AddExemptHosts registers hostnames or URLs that represent infrastructure plumbing.
func (g *EgressGuard) AddExemptHosts(hosts ...string) {
	if g == nil {
		return
	}
	g.exemptMu.Lock()
	defer g.exemptMu.Unlock()
	for _, h := range hosts {
		trimmed := strings.ToLower(strings.TrimSpace(h))
		if trimmed == "" {
			continue
		}
		canonical := ExtractHost(trimmed)
		if canonical != "" {
			g.exemptHosts[canonical] = true
		} else {
			g.exemptHosts[trimmed] = true
		}
	}
}

// IsExempt checks if a host is an exempt plumbing service.
func (g *EgressGuard) IsExempt(host string) bool {
	if g == nil {
		return false
	}
	g.exemptMu.RLock()
	defer g.exemptMu.RUnlock()

	canonical := ExtractHost(host)
	if canonical == "" {
		canonical = strings.ToLower(strings.TrimSpace(host))
	}

	if g.exemptHosts[canonical] {
		return true
	}

	for ex := range g.exemptHosts {
		if MatchDomainPattern(canonical, ex) {
			return true
		}
	}
	return false
}

// SetAllowIMDS controls whether cloud instance metadata (IMDS) destinations are permitted.
func (g *EgressGuard) SetAllowIMDS(allow bool) {
	if g == nil {
		return
	}
	g.allowIMDS.Store(allow)
}

// AllowIMDS returns whether cloud instance metadata (IMDS) destinations are permitted.
func (g *EgressGuard) AllowIMDS() bool {
	if g == nil {
		return false
	}
	return g.allowIMDS.Load()
}

// Policy returns the underlying scope policy.
func (g *EgressGuard) Policy() *ScopeFilePolicy {
	return g.policy
}

// CheckDestination checks if destination host/port is permitted.
func (g *EgressGuard) CheckDestination(ctx context.Context, hostOrAddr string) error {
	if g == nil {
		return nil
	}

	host := ExtractHost(hostOrAddr)
	if host == "" {
		host = hostOrAddr
		if h, _, err := net.SplitHostPort(hostOrAddr); err == nil {
			host = h
		}
	}

	// 0. Cloud instance metadata (IMDS) check
	if !g.AllowIMDS() && IsCloudMetadataEndpoint(host) {
		return fmt.Errorf("%w: %s (cloud metadata endpoint IMDS access forbidden)", ErrScopeEgressBlocked, hostOrAddr)
	}

	if g.policy == nil {
		return nil
	}

	// 1. Plumbing exemption check
	if g.IsExempt(host) {
		return nil
	}

	allowed, reason := g.policy.EvaluateDestination(ctx, host, g.resolver)
	if !allowed {
		return fmt.Errorf("%w: %s (%s)", ErrScopeEgressBlocked, hostOrAddr, reason)
	}
	return nil
}

func (g *EgressGuard) checkAndPinDestination(ctx context.Context, addr string) (string, error) {
	if g == nil {
		return addr, nil
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
		port = ""
	}

	// 1. Plumbing exemption check
	if g.IsExempt(host) {
		return addr, nil
	}

	// 2. Validate destination through policy
	if err := g.CheckDestination(ctx, addr); err != nil {
		return "", err
	}

	// If host is an IP, no DNS rebinding possible
	if ip := net.ParseIP(host); ip != nil {
		return addr, nil
	}

	// 3. DNS rebinding prevention: resolve now and verify resolved IP(s)
	resolver := g.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	dnsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ips, err := resolver.LookupIPAddr(dnsCtx, host)
	if err != nil {
		// If DNS resolution fails here, return original addr and let dialer handle network error
		return addr, nil
	}
	if len(ips) == 0 {
		return addr, nil
	}

	// Verify all resolved IPs against forbidden IMDS
	for _, ipAddr := range ips {
		ipStr := ipAddr.IP.String()
		if !g.AllowIMDS() && IsCloudMetadataEndpoint(ipStr) {
			return "", fmt.Errorf("%w: %s resolves to cloud metadata endpoint %s", ErrScopeEgressBlocked, host, ipStr)
		}
	}

	// Pin dial destination to first validated IP
	if port != "" {
		return net.JoinHostPort(ips[0].IP.String(), port), nil
	}
	return ips[0].IP.String(), nil
}

// WrapDialContext wraps a dial function with network egress evaluation.
func (g *EgressGuard) WrapDialContext(baseDial func(ctx context.Context, network, addr string) (net.Conn, error)) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if baseDial == nil {
		dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
		baseDial = dialer.DialContext
	}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if g == nil {
			return baseDial(ctx, network, addr)
		}
		targetAddr, err := g.checkAndPinDestination(ctx, addr)
		if err != nil {
			return nil, err
		}
		return baseDial(ctx, network, targetAddr)
	}
}

// WrapHTTPTransport wraps an http.Transport to enforce egress scope checking on every request.
func (g *EgressGuard) WrapHTTPTransport(base *http.Transport) *http.Transport {
	if base == nil {
		base = http.DefaultTransport.(*http.Transport).Clone()
	} else {
		base = base.Clone()
	}

	// Protect against proxy bypass: validate request URL host before proxying
	origProxy := base.Proxy
	if origProxy != nil {
		base.Proxy = func(req *http.Request) (*url.URL, error) {
			if g != nil && req != nil && req.URL != nil {
				if err := g.CheckDestination(req.Context(), req.URL.Host); err != nil {
					return nil, err
				}
			}
			return origProxy(req)
		}
	}

	base.DialContext = g.WrapDialContext(base.DialContext)
	return base
}

// SetGlobalEgressGuard sets the process-wide network egress guard.
func SetGlobalEgressGuard(guard *EgressGuard) {
	globalGuardMu.Lock()
	defer globalGuardMu.Unlock()
	globalGuard = guard

	if guard != nil {
		// Hook into http.DefaultTransport
		if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok {
			defaultTransport.DialContext = guard.WrapDialContext(defaultTransport.DialContext)
		}
	}
}

// GetGlobalEgressGuard returns the active process-wide egress guard.
func GetGlobalEgressGuard() *EgressGuard {
	globalGuardMu.RLock()
	defer globalGuardMu.RUnlock()
	return globalGuard
}

// WrapDialContextWithGlobal wraps a dial function to check outbound addresses against the global egress guard.
func WrapDialContextWithGlobal(baseDial func(ctx context.Context, network, addr string) (net.Conn, error)) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if baseDial == nil {
		dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
		baseDial = dialer.DialContext
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if g := GetGlobalEgressGuard(); g != nil {
			targetAddr, err := g.checkAndPinDestination(ctx, addr)
			if err != nil {
				return nil, err
			}
			return baseDial(ctx, network, targetAddr)
		}
		return baseDial(ctx, network, addr)
	}
}
