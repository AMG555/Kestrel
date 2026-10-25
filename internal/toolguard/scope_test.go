package toolguard

import (
	"testing"
)

func TestExtractHost(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"https://example.com/api/v1/users", "example.com"},
		{"http://sub.domain.co.uk:8080/path?id=1", "sub.domain.co.uk"},
		{"192.168.1.50:9000", "192.168.1.50"},
		{"[2001:db8::1]:80", "2001:db8::1"},
		{"target.local/admin", "target.local"},
		{"   api.acme.org   ", "api.acme.org"},
	}

	for _, c := range cases {
		got := ExtractHost(c.input)
		if got != c.expected {
			t.Errorf("ExtractHost(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}

func TestCloudMetadataProtection(t *testing.T) {
	badTargets := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://169.254.169.254:80",
		"169.254.169.254",
		"http://metadata.google.internal/computeMetadata/v1/",
		"169.254.169.250",
	}

	scope := ProjectScope{
		Enabled: true,
		Targets: []string{"*"}, // Even if wildcards are permitted, IMDS MUST be blocked!
	}

	for _, tgt := range badTargets {
		res := ValidateTargetScope(tgt, scope)
		if res.Allowed {
			t.Errorf("Security flaw: Cloud metadata endpoint %s was permitted!", tgt)
		}
	}
}

func TestDomainScopeMatching(t *testing.T) {
	scope := ProjectScope{
		Enabled: true,
		Targets: []string{"*.acme.corp", "portal.partner.com"},
		Exclude: []string{"admin.acme.corp", "internal.acme.corp"},
	}

	// Allowed targets
	allowed := []string{
		"https://api.acme.corp/v1",
		"http://staging.sub.acme.corp",
		"portal.partner.com:443",
	}

	for _, a := range allowed {
		res := ValidateTargetScope(a, scope)
		if !res.Allowed {
			t.Errorf("Expected %s to be allowed, got blocked: %s", a, res.Reason)
		}
	}

	// Excluded targets
	excluded := []string{
		"http://admin.acme.corp/login",
		"https://internal.acme.corp/hr",
	}

	for _, e := range excluded {
		res := ValidateTargetScope(e, scope)
		if res.Allowed {
			t.Errorf("Expected %s to be blocked by exclude rule, but was allowed!", e)
		}
	}

	// Out of scope targets
	outOfScope := []string{
		"https://google.com",
		"https://acme.corp.attacker.com",
		"https://evil-acme.corp",
		"https://other.partner.com",
	}

	for _, o := range outOfScope {
		res := ValidateTargetScope(o, scope)
		if res.Allowed {
			t.Errorf("Expected %s to be out of scope, but was allowed!", o)
		}
	}
}

func TestCIDRScopeMatching(t *testing.T) {
	scope := ProjectScope{
		Enabled: true,
		Targets: []string{"192.168.10.0/24", "10.0.5.25"},
		Exclude: []string{"192.168.10.1"}, // Exclude gateway
	}

	// Allowed
	res1 := ValidateTargetScope("192.168.10.55", scope)
	if !res1.Allowed {
		t.Errorf("Expected 192.168.10.55 to be allowed, got: %s", res1.Reason)
	}

	res2 := ValidateTargetScope("http://10.0.5.25:8080/dashboard", scope)
	if !res2.Allowed {
		t.Errorf("Expected 10.0.5.25 to be allowed, got: %s", res2.Reason)
	}

	// Excluded gateway
	res3 := ValidateTargetScope("192.168.10.1", scope)
	if res3.Allowed {
		t.Errorf("Expected gateway 192.168.10.1 to be blocked by exclude rule!")
	}

	// Out of scope IP
	res4 := ValidateTargetScope("192.168.20.1", scope)
	if res4.Allowed {
		t.Errorf("Expected 192.168.20.1 to be out of scope!")
	}
}
