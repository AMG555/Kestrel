package toolguard

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadScopeFile_And_EvaluateDestination(t *testing.T) {
	yamlContent := `
version: 1
target_name: "Client Penetration Test"
in_scope:
  cidrs:
    - "10.0.0.0/8"
    - "192.168.1.50"
  domains:
    - "example.com"
    - "*.staging.internal"
  urls:
    - "http://api.local.test:8080/v1"
out_of_scope:
  cidrs:
    - "10.0.100.0/24"
  domains:
    - "production.example.com"
    - "*.gov"
options:
  fail_closed: true
  block_cloud_metadata: true
  allow_localhost: true
`
	tmpFile := filepath.Join(t.TempDir(), "scope.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write temp scope file: %v", err)
	}

	policy, err := LoadScopeFile(tmpFile)
	if err != nil {
		t.Fatalf("LoadScopeFile failed: %v", err)
	}

	if policy.TargetName != "Client Penetration Test" {
		t.Errorf("expected target name 'Client Penetration Test', got %s", policy.TargetName)
	}
	if !policy.FailClosed {
		t.Errorf("expected fail_closed to be true")
	}

	ctx := context.Background()

	tests := []struct {
		name       string
		dest       string
		wantAllow  bool
		reasonSub  string
	}{
		{
			name:      "In-scope domain direct match",
			dest:      "example.com",
			wantAllow: true,
			reasonSub: "matches authorized in-scope domain",
		},
		{
			name:      "In-scope subdomain match",
			dest:      "sub.example.com",
			wantAllow: true,
			reasonSub: "matches authorized in-scope domain",
		},
		{
			name:      "In-scope wildcard match",
			dest:      "app.staging.internal",
			wantAllow: true,
			reasonSub: "matches authorized in-scope domain",
		},
		{
			name:      "In-scope CIDR match",
			dest:      "10.0.5.21",
			wantAllow: true,
			reasonSub: "matches authorized in-scope CIDR",
		},
		{
			name:      "In-scope single IP notation",
			dest:      "192.168.1.50",
			wantAllow: true,
			reasonSub: "matches authorized in-scope CIDR",
		},
		{
			name:      "In-scope URL host match",
			dest:      "api.local.test:8080",
			wantAllow: true,
			reasonSub: "matches authorized in-scope target URL",
		},
		{
			name:      "Out-of-scope excluded CIDR takes precedence over in-scope",
			dest:      "10.0.100.5",
			wantAllow: false,
			reasonSub: "matches out-of-scope CIDR",
		},
		{
			name:      "Out-of-scope excluded domain takes precedence over in-scope",
			dest:      "production.example.com",
			wantAllow: false,
			reasonSub: "matches out-of-scope domain rule",
		},
		{
			name:      "Out-of-scope wildcard domain blocked",
			dest:      "defense.gov",
			wantAllow: false,
			reasonSub: "matches out-of-scope domain rule",
		},
		{
			name:      "Cloud metadata IP strictly blocked",
			dest:      "169.254.169.254",
			wantAllow: false,
			reasonSub: "cloud metadata endpoint",
		},
		{
			name:      "Unknown target dropped under fail_closed",
			dest:      "unauthorized-target.org",
			wantAllow: false,
			reasonSub: "fail-closed",
		},
		{
			name:      "Localhost allowed by option",
			dest:      "127.0.0.1:3000",
			wantAllow: true,
			reasonSub: "permitted localhost",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			allowed, reason := policy.EvaluateDestination(ctx, tc.dest, nil)
			if allowed != tc.wantAllow {
				t.Fatalf("destination %q: got allowed=%v, want %v (reason: %s)", tc.dest, allowed, tc.wantAllow, reason)
			}
			if tc.reasonSub != "" && !containsIgnoreCase(reason, tc.reasonSub) {
				t.Errorf("destination %q reason %q does not contain %q", tc.dest, reason, tc.reasonSub)
			}
		})
	}
}

func TestEgressGuard_WrapDialContext_DropsBlockedSockets(t *testing.T) {
	policy := &ScopeFilePolicy{
		FailClosed:         true,
		BlockCloudMetadata: true,
		AllowLocalhost:     false,
		InScopeDomains:     []string{"permitted.com"},
		OutOfScopeCIDRs:    []*net.IPNet{mustParseCIDR("10.10.10.0/24")},
	}

	guard := NewEgressGuard(policy)
	mockDialCalled := false
	mockDial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		mockDialCalled = true
		return nil, nil
	}

	wrappedDial := guard.WrapDialContext(mockDial)

	// Blocked target should return error without calling mockDial
	_, err := wrappedDial(context.Background(), "tcp", "10.10.10.5:80")
	if err == nil {
		t.Fatal("expected dial to blocked CIDR to fail")
	}
	if mockDialCalled {
		t.Fatal("expected mockDial NOT to be called for blocked socket")
	}

	// Permitted target should call mockDial
	mockDialCalled = false
	_, _ = wrappedDial(context.Background(), "tcp", "permitted.com:443")
	if !mockDialCalled {
		t.Fatal("expected mockDial to be called for authorized destination")
	}
}

func TestEgressGuard_PlumbingExemptions(t *testing.T) {
	// Policy allows ONLY a single client host, strictly fail-closed
	policy := &ScopeFilePolicy{
		FailClosed:         true,
		BlockCloudMetadata: true,
		AllowLocalhost:     false,
		InScopeDomains:     []string{"target-client.com"},
	}

	guard := NewEgressGuard(policy)
	guard.AddExemptHosts("https://custom-webhook.internal:9000/alerts")

	// Standard LLM endpoints must pass through as exempt
	if err := guard.CheckDestination(context.Background(), "api.openai.com:443"); err != nil {
		t.Fatalf("expected api.openai.com to be exempt, got error: %v", err)
	}
	if err := guard.CheckDestination(context.Background(), "api.anthropic.com:443"); err != nil {
		t.Fatalf("expected api.anthropic.com to be exempt, got error: %v", err)
	}
	if err := guard.CheckDestination(context.Background(), "api.telegram.org:443"); err != nil {
		t.Fatalf("expected api.telegram.org to be exempt, got error: %v", err)
	}
	if err := guard.CheckDestination(context.Background(), "custom-webhook.internal:9000"); err != nil {
		t.Fatalf("expected custom-webhook.internal to be exempt, got error: %v", err)
	}

	// Unauthorized external target must be blocked
	if err := guard.CheckDestination(context.Background(), "unauthorized-external.com:443"); err == nil {
		t.Fatal("expected unauthorized-external.com to be blocked")
	}
}

func containsIgnoreCase(s, sub string) bool {
	return len(s) >= len(sub) && (sub == "" || stringContains(s, sub))
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func mustParseCIDR(cidr string) *net.IPNet {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	return ipNet
}

