package toolguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
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
	policy   *ScopeFilePolicy
	resolver *net.Resolver
}

// NewEgressGuard creates an EgressGuard with the given policy.
func NewEgressGuard(policy *ScopeFilePolicy) *EgressGuard {
	return &EgressGuard{
		policy: policy,
		resolver: &net.Resolver{
			PreferGo: true,
		},
	}
}

// Policy returns the underlying scope policy.
func (g *EgressGuard) Policy() *ScopeFilePolicy {
	return g.policy
}

// CheckDestination checks if destination host/port is permitted.
func (g *EgressGuard) CheckDestination(ctx context.Context, hostOrAddr string) error {
	if g == nil || g.policy == nil {
		return nil
	}

	host := hostOrAddr
	if h, _, err := net.SplitHostPort(hostOrAddr); err == nil {
		host = h
	}

	allowed, reason := g.policy.EvaluateDestination(ctx, host, g.resolver)
	if !allowed {
		return fmt.Errorf("%w: %s (%s)", ErrScopeEgressBlocked, hostOrAddr, reason)
	}
	return nil
}

// WrapDialContext wraps a dial function with network egress evaluation.
func (g *EgressGuard) WrapDialContext(baseDial func(ctx context.Context, network, addr string) (net.Conn, error)) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if baseDial == nil {
		dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
		baseDial = dialer.DialContext
	}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if err := g.CheckDestination(ctx, addr); err != nil {
			return nil, err
		}
		return baseDial(ctx, network, addr)
	}
}

// WrapHTTPTransport wraps an http.Transport to enforce egress scope checking on every request.
func (g *EgressGuard) WrapHTTPTransport(base *http.Transport) *http.Transport {
	if base == nil {
		base = http.DefaultTransport.(*http.Transport).Clone()
	} else {
		base = base.Clone()
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
