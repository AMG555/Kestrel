package mcp

// This file extends builtins.go with additional read-only recon tools:
//   - whois_lookup      — passive WHOIS data via whois.arin.net / rdap
//   - ssl_cert_check    — TLS certificate info (expiry, SANs, issuer)
//   - tech_fingerprint  — passive HTTP header + HTML fingerprinting
//   - port_scan         — TCP connect scan (read-only, no exploitation)

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// registerExtendedBuiltins registers the additional read-only tools.
// Called from registerBuiltins() in builtins.go.
func (r *Registry) registerExtendedBuiltins() {
	r.RegisterTool(&ToolDefinition{
		Name:        "whois_lookup",
		Description: "Perform a passive WHOIS / RDAP lookup for a domain or IP address. Returns registrant, registrar, creation/expiry dates, and nameservers. Read-only.",
		Class:       ToolClassReadOnly,
		ServerID:    "builtin",
		Parameters: mustSchema(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"target": map[string]interface{}{
					"type":        "string",
					"description": "Domain name or IP address to look up",
				},
			},
			"required": []string{"target"},
		}),
	}, whoisLookupHandler)

	r.RegisterTool(&ToolDefinition{
		Name:        "ssl_cert_check",
		Description: "Retrieve and inspect the TLS certificate of a host: expiry date, Subject Alternative Names, issuer, and whether it is currently valid. Read-only.",
		Class:       ToolClassReadOnly,
		ServerID:    "builtin",
		Parameters: mustSchema(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host": map[string]interface{}{
					"type":        "string",
					"description": "Hostname (and optional :port — defaults to 443)",
				},
			},
			"required": []string{"host"},
		}),
	}, sslCertCheckHandler)

	r.RegisterTool(&ToolDefinition{
		Name:        "tech_fingerprint",
		Description: "Passive technology fingerprinting for a URL. Analyses HTTP response headers and HTML meta-tags to identify web frameworks, CDNs, and server software. Read-only.",
		Class:       ToolClassReadOnly,
		ServerID:    "builtin",
		Parameters: mustSchema(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url": map[string]interface{}{
					"type":        "string",
					"description": "Full URL to fingerprint (e.g. https://example.com)",
				},
			},
			"required": []string{"url"},
		}),
	}, techFingerprintHandler)

	r.RegisterTool(&ToolDefinition{
		Name:        "port_scan",
		Description: "TCP connect scan for a host across a set of common ports. Reports open/closed/filtered status. Read-only — no exploitation or banner grabbing beyond initial connect.",
		Class:       ToolClassReadOnly,
		ServerID:    "builtin",
		Parameters: mustSchema(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"host": map[string]interface{}{
					"type":        "string",
					"description": "Hostname or IP address to scan",
				},
				"ports": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "integer"},
					"description": "List of TCP port numbers (default: common web/infra ports)",
				},
				"timeout_ms": map[string]interface{}{
					"type":        "integer",
					"description": "Per-port connect timeout in milliseconds (default: 2000)",
				},
			},
			"required": []string{"host"},
		}),
	}, portScanHandler)
}

// ─── whois_lookup ─────────────────────────────────────────────────────────────

func whoisLookupHandler(ctx context.Context, args map[string]interface{}) (string, error) {
	target, _ := args["target"].(string)
	if target == "" {
		return "", fmt.Errorf("target is required")
	}
	target = strings.TrimSpace(strings.ToLower(target))

	// Use RDAP for domains (iana bootstrap or rdap.org).
	rdapURL := fmt.Sprintf("https://rdap.org/domain/%s", target)
	if net.ParseIP(target) != nil {
		rdapURL = fmt.Sprintf("https://rdap.org/ip/%s", target)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rdapURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/rdap+json")
	req.Header.Set("User-Agent", "Kestrel/1.0 (security-research)")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("RDAP lookup failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 32768))
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("RDAP returned HTTP %d for %s.\nBody: %s", resp.StatusCode, target, string(body)), nil
	}

	// Extract key fields with simple regex so we don't need a JSON dep.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("RDAP/WHOIS results for %s:\n\n", target))

	for _, pair := range []struct{ label, pattern string }{
		{"Handle", `"handle"\s*:\s*"([^"]+)"`},
		{"Name", `"name"\s*:\s*"([^"]+)"`},
		{"Status", `"status"\s*:\s*\["([^"]+)"`},
		{"Registered", `"registration"\s*:\s*"([^"]+)"`},
		{"Expires", `"expiration"\s*:\s*"([^"]+)"`},
		{"Updated", `"lastChanged"\s*:\s*"([^"]+)"`},
	} {
		if m := regexp.MustCompile(pair.pattern).FindStringSubmatch(string(body)); len(m) > 1 {
			sb.WriteString(fmt.Sprintf("  %-14s %s\n", pair.label+":", m[1]))
		}
	}

	// Nameservers.
	nsRe := regexp.MustCompile(`"ldhName"\s*:\s*"([^"]+)"`)
	nsMatches := nsRe.FindAllStringSubmatch(string(body), -1)
	if len(nsMatches) > 0 {
		sb.WriteString("  Nameservers:\n")
		seen := map[string]bool{}
		for _, m := range nsMatches {
			if !seen[m[1]] {
				seen[m[1]] = true
				sb.WriteString("    " + m[1] + "\n")
			}
		}
	}

	if sb.Len() < 60 {
		sb.WriteString("  (No structured data returned — raw response follows)\n")
		if len(body) > 2000 {
			body = body[:2000]
		}
		sb.WriteString(string(body))
	}
	return sb.String(), nil
}

// ─── ssl_cert_check ───────────────────────────────────────────────────────────

func sslCertCheckHandler(ctx context.Context, args map[string]interface{}) (string, error) {
	host, _ := args["host"].(string)
	if host == "" {
		return "", fmt.Errorf("host is required")
	}
	host = strings.TrimSpace(host)

	addr := host
	if !strings.Contains(host, ":") {
		addr = host + ":443"
	}

	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 10 * time.Second},
		Config: &tls.Config{
			InsecureSkipVerify: true, // we want to inspect even invalid certs
		},
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return "", fmt.Errorf("TLS connect failed: %w", err)
	}
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return "", fmt.Errorf("unexpected connection type")
	}

	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return fmt.Sprintf("No certificates returned by %s", host), nil
	}

	cert := state.PeerCertificates[0]
	now := time.Now()
	daysLeft := int(cert.NotAfter.Sub(now).Hours() / 24)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("TLS certificate for %s:\n\n", host))
	sb.WriteString(fmt.Sprintf("  Subject:     %s\n", cert.Subject.CommonName))
	sb.WriteString(fmt.Sprintf("  Issuer:      %s\n", cert.Issuer.CommonName))
	sb.WriteString(fmt.Sprintf("  Valid From:  %s\n", cert.NotBefore.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("  Valid Until: %s", cert.NotAfter.Format("2006-01-02")))
	if daysLeft < 0 {
		sb.WriteString(fmt.Sprintf("  ⚠ EXPIRED %d days ago", -daysLeft))
	} else if daysLeft < 30 {
		sb.WriteString(fmt.Sprintf("  ⚠ Expires in %d days (soon)", daysLeft))
	} else {
		sb.WriteString(fmt.Sprintf("  (%d days remaining)", daysLeft))
	}
	sb.WriteString("\n")

	if len(cert.DNSNames) > 0 {
		sb.WriteString(fmt.Sprintf("  SANs (%d):   %s\n", len(cert.DNSNames), strings.Join(cert.DNSNames, ", ")))
	}
	if len(cert.IPAddresses) > 0 {
		var ips []string
		for _, ip := range cert.IPAddresses {
			ips = append(ips, ip.String())
		}
		sb.WriteString(fmt.Sprintf("  IP SANs:     %s\n", strings.Join(ips, ", ")))
	}

	// Chain depth.
	if len(state.PeerCertificates) > 1 {
		sb.WriteString(fmt.Sprintf("  Chain depth: %d\n", len(state.PeerCertificates)))
	}

	// TLS version.
	tlsVersions := map[uint16]string{
		tls.VersionTLS10: "TLS 1.0", tls.VersionTLS11: "TLS 1.1",
		tls.VersionTLS12: "TLS 1.2", tls.VersionTLS13: "TLS 1.3",
	}
	if v, ok := tlsVersions[state.Version]; ok {
		sb.WriteString(fmt.Sprintf("  TLS Version: %s\n", v))
	}

	return sb.String(), nil
}

// ─── tech_fingerprint ─────────────────────────────────────────────────────────

func techFingerprintHandler(ctx context.Context, args map[string]interface{}) (string, error) {
	url, _ := args["url"].(string)
	if url == "" {
		return "", fmt.Errorf("url is required")
	}
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 2 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Kestrel/1.0; +https://github.com/AMG555/Kestrel)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	htmlBody, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
	html := strings.ToLower(string(htmlBody))

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Technology fingerprint for %s:\n\n", url))
	sb.WriteString(fmt.Sprintf("  HTTP Status: %d\n", resp.StatusCode))

	// Header signals.
	headerChecks := [][2]string{
		{"Server", resp.Header.Get("Server")},
		{"X-Powered-By", resp.Header.Get("X-Powered-By")},
		{"X-Generator", resp.Header.Get("X-Generator")},
		{"X-Frame-Options", resp.Header.Get("X-Frame-Options")},
		{"Strict-Transport-Security", resp.Header.Get("Strict-Transport-Security")},
		{"Content-Security-Policy", func() string {
			v := resp.Header.Get("Content-Security-Policy")
			if len(v) > 80 {
				return v[:80] + "…"
			}
			return v
		}()},
		{"CF-Cache-Status", resp.Header.Get("CF-Cache-Status")},
		{"Via", resp.Header.Get("Via")},
	}
	for _, h := range headerChecks {
		if h[1] != "" {
			sb.WriteString(fmt.Sprintf("  %-28s %s\n", h[0]+":", h[1]))
		}
	}

	// Technology signals from HTML.
	techSignals := []struct {
		name    string
		pattern string
	}{
		{"WordPress", "wp-content"},
		{"Joomla", "joomla"},
		{"Drupal", "drupal.js"},
		{"React", "react.development.js"},
		{"Next.js", "__next_data__"},
		{"Angular", "ng-version"},
		{"Vue.js", "__vue__"},
		{"Bootstrap", "bootstrap.min.css"},
		{"jQuery", "jquery.min.js"},
		{"Cloudflare", "cloudflare"},
		{"Google Analytics", "google-analytics.com"},
		{"Google Tag Manager", "googletagmanager.com"},
	}

	sb.WriteString("\n  Detected technologies:\n")
	found := 0
	for _, sig := range techSignals {
		if strings.Contains(html, sig.pattern) {
			sb.WriteString("    ✓ " + sig.name + "\n")
			found++
		}
	}
	if found == 0 {
		sb.WriteString("    (none detected from passive HTML analysis)\n")
	}

	// Meta generator.
	if m := regexp.MustCompile(`<meta[^>]+name=["']generator["'][^>]+content=["']([^"']+)["']`).FindStringSubmatch(html); len(m) > 1 {
		sb.WriteString(fmt.Sprintf("  Meta generator: %s\n", m[1]))
	}

	return sb.String(), nil
}

// ─── port_scan ────────────────────────────────────────────────────────────────

var defaultPorts = []int{
	21, 22, 23, 25, 53, 80, 110, 135, 143, 443, 445, 465,
	587, 993, 995, 1433, 1521, 3306, 3389, 5432, 5900, 6379,
	8080, 8443, 8888, 9200, 27017,
}

func portScanHandler(ctx context.Context, args map[string]interface{}) (string, error) {
	host, _ := args["host"].(string)
	if host == "" {
		return "", fmt.Errorf("host is required")
	}
	host = strings.TrimSpace(host)

	timeoutMs := 2000
	if v, ok := args["timeout_ms"].(float64); ok && v > 0 {
		timeoutMs = int(v)
	}

	ports := defaultPorts
	if raw, ok := args["ports"].([]interface{}); ok && len(raw) > 0 {
		ports = nil
		for _, p := range raw {
			if f, ok := p.(float64); ok {
				ports = append(ports, int(f))
			}
		}
	}
	if len(ports) > 200 {
		ports = ports[:200] // hard cap
	}

	type result struct {
		port   int
		open   bool
		banner string
	}

	results := make([]result, len(ports))
	sem := make(chan struct{}, 50) // 50 concurrent connects

	for i, port := range ports {
		sem <- struct{}{}
		go func(idx, p int) {
			defer func() { <-sem }()
			addr := fmt.Sprintf("%s:%d", host, p)
			d := &net.Dialer{Timeout: time.Duration(timeoutMs) * time.Millisecond}
			conn, err := d.DialContext(ctx, "tcp", addr)
			if err == nil {
				conn.Close()
				results[idx] = result{port: p, open: true}
			} else {
				results[idx] = result{port: p, open: false}
			}
		}(i, port)
	}

	// Drain semaphore to wait for all goroutines.
	for i := 0; i < cap(sem); i++ {
		sem <- struct{}{}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Port scan results for %s (%d ports):\n\n", host, len(ports)))

	openCount := 0
	for i, p := range ports {
		r := results[i]
		if r.open {
			openCount++
			sb.WriteString(fmt.Sprintf("  %-6d  OPEN\n", p))
		}
	}
	if openCount == 0 {
		sb.WriteString("  No open ports found in scanned range.\n")
	}
	sb.WriteString(fmt.Sprintf("\n  Summary: %d/%d ports open\n", openCount, len(ports)))
	return sb.String(), nil
}
