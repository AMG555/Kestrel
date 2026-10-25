package database

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// NormalizeTargetForFingerprint cleans and normalizes a target URL/endpoint for deduplication.
func NormalizeTargetForFingerprint(rawTarget string) string {
	raw := strings.TrimSpace(rawTarget)
	if raw == "" {
		return "unknown"
	}

	// If target has a scheme, parse as URL
	parsed, err := url.Parse(raw)
	if err == nil && parsed.Host != "" {
		host := strings.ToLower(parsed.Hostname())
		path := strings.TrimRight(parsed.EscapedPath(), "/")
		if path == "" {
			path = "/"
		}

		// Sort query keys without keeping volatile session IDs/tokens
		var queryKeys []string
		q := parsed.Query()
		for k := range q {
			lowerK := strings.ToLower(k)
			// exclude common volatile tokens from fingerprinting
			if lowerK == "token" || lowerK == "session" || lowerK == "csrf" || lowerK == "t" || lowerK == "_" || lowerK == "timestamp" || lowerK == "nonce" {
				continue
			}
			queryKeys = append(queryKeys, lowerK)
		}
		sort.Strings(queryKeys)

		if len(queryKeys) > 0 {
			return fmt.Sprintf("%s%s?%s", host, path, strings.Join(queryKeys, "&"))
		}
		return fmt.Sprintf("%s%s", host, path)
	}

	// Fallback for raw paths or non-URL targets: strip scheme, normalize path
	raw = strings.ToLower(raw)
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "http://")
	return strings.TrimRight(raw, "/")
}

// ComputeFindingFingerprint calculates a deterministic SHA-256 fingerprint for a vulnerability.
func ComputeFindingFingerprint(v *Vulnerability) string {
	if v == nil {
		return ""
	}

	normTarget := NormalizeTargetForFingerprint(v.Target)
	normType := strings.ToLower(strings.TrimSpace(v.Type))
	if normType == "" {
		normType = strings.ToLower(strings.TrimSpace(v.Title))
	}

	// Create canonical signature
	canonical := fmt.Sprintf("%s|%s", normType, normTarget)
	sum := sha256.Sum256([]byte(canonical))
	return fmt.Sprintf("%x", sum)
}
