package toolguard

import (
	"context"
	"strings"
	"testing"
)

func TestEgressGuard_ExemptionSubdomainAndSpoofingProtection(t *testing.T) {
	guard := NewEgressGuard(nil)

	// Valid exemptions: exact match and genuine subdomains
	validCases := []string{
		"discord.com",
		"cdn.discord.com",
		"sub.api.anthropic.com",
		"api.openai.com",
		"localhost",
		"127.0.0.1",
	}
	for _, host := range validCases {
		if !guard.IsExempt(host) {
			t.Errorf("expected host %q to be exempt, but got false", host)
		}
	}

	// Host spoofing attempts that must NOT be exempt
	invalidCases := []string{
		"evil-discord.com",
		"discord.com.evil.com",
		"notapi.anthropic.com",
		"api.openai.com.attacker.com",
		"127.0.0.1.nip.io",
		"fake-localhost",
	}
	for _, host := range invalidCases {
		if guard.IsExempt(host) {
			t.Errorf("expected host %q to NOT be exempt, but got true", host)
		}
	}
}

func TestEgressGuard_BlocksIMDSEvenWithoutPolicy(t *testing.T) {
	// Guard without a scope policy configured
	guard := NewEgressGuard(nil)

	imdsEndpoints := []string{
		"169.254.169.254",
		"169.254.169.254:80",
		"http://169.254.169.254/latest/meta-data/",
		"metadata.google.internal",
		"instance-data",
		"metadata.azure.com",
	}

	for _, endpoint := range imdsEndpoints {
		err := guard.CheckDestination(context.Background(), endpoint)
		if err == nil {
			t.Errorf("expected IMDS endpoint %q to be blocked with nil policy, got nil", endpoint)
		} else if !strings.Contains(err.Error(), "cloud metadata endpoint IMDS access forbidden") {
			t.Errorf("unexpected error for IMDS endpoint %q: %v", endpoint, err)
		}
	}
}
