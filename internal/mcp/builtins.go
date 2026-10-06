package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// registerBuiltins registers the three default read-only recon tools:
//   - subdomain_enum  — passive subdomain enumeration via crt.sh
//   - http_probe      — HTTP/HTTPS service probing
//   - dns_lookup      — DNS record lookup
//
// Exploitation, credential-dumping, and remote-shell classes are deliberately
// excluded. Operators wishing to add higher-risk tool classes must register them
// explicitly with their own controls in place.
func (r *Registry) registerBuiltins() {
	r.RegisterTool(&ToolDefinition{
		Name:        "subdomain_enum",
		Description: "Passively enumerate subdomains for a given domain using certificate transparency logs (crt.sh). Read-only — makes no connections to the target.",
		Class:       ToolClassReadOnly,
		ServerID:    "builtin",
		Parameters: mustSchema(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"domain": map[string]interface{}{
					"type":        "string",
					"description": "Root domain to enumerate (e.g. example.com)",
				},
			},
			"required": []string{"domain"},
		}),
	}, subdomainEnumHandler)

	r.RegisterTool(&ToolDefinition{
		Name:        "http_probe",
		Description: "Probe one or more URLs/hosts for HTTP/HTTPS service availability. Returns status code, title, and server header. Does not submit forms or follow redirect chains beyond one hop.",
		Class:       ToolClassReadOnly,
		ServerID:    "builtin",
		Parameters: mustSchema(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"targets": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "List of URLs or host:port pairs to probe",
				},
				"timeout_seconds": map[string]interface{}{
					"type":        "integer",
					"description": "Per-target request timeout in seconds (default: 10)",
					"default":     10,
				},
			},
			"required": []string{"targets"},
		}),
	}, httpProbeHandler)

	r.RegisterTool(&ToolDefinition{
		Name:        "dns_lookup",
		Description: "Perform DNS record lookups (A, AAAA, MX, TXT, NS, CNAME) for a hostname. Read-only.",
		Class:       ToolClassReadOnly,
		ServerID:    "builtin",
		Parameters: mustSchema(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"hostname": map[string]interface{}{
					"type":        "string",
					"description": "Hostname to query",
				},
				"record_types": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Record types to query (default: [\"A\",\"AAAA\",\"MX\",\"TXT\",\"NS\"])",
				},
			},
			"required": []string{"hostname"},
		}),
	}, dnsLookupHandler)
}

// --- Handlers ---

func subdomainEnumHandler(ctx context.Context, args map[string]interface{}) (string, error) {
	domain, _ := args["domain"].(string)
	if domain == "" {
		return "", fmt.Errorf("domain is required")
	}
	domain = strings.TrimSpace(strings.ToLower(domain))

	url := fmt.Sprintf("https://crt.sh/?q=%%25.%s&output=json", domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "Kestrel/1.0 (security-research)")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("querying crt.sh: %w", err)
	}
	defer resp.Body.Close()

	var entries []struct {
		NameValue string `json:"name_value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return "", fmt.Errorf("decoding crt.sh response: %w", err)
	}

	seen := make(map[string]bool)
	var subdomains []string
	for _, e := range entries {
		for _, name := range strings.Split(e.NameValue, "\n") {
			name = strings.TrimSpace(strings.ToLower(name))
			if name == "" || strings.HasPrefix(name, "*") {
				continue
			}
			if !seen[name] {
				seen[name] = true
				subdomains = append(subdomains, name)
			}
		}
	}

	if len(subdomains) == 0 {
		return fmt.Sprintf("No subdomains found for %s via certificate transparency logs.", domain), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Subdomain enumeration results for %s (%d found):\n\n", domain, len(subdomains)))
	for _, s := range subdomains {
		sb.WriteString("  " + s + "\n")
	}
	return sb.String(), nil
}

func httpProbeHandler(ctx context.Context, args map[string]interface{}) (string, error) {
	rawTargets, _ := args["targets"].([]interface{})
	if len(rawTargets) == 0 {
		return "", fmt.Errorf("targets list is required")
	}
	timeoutSec := 10
	if v, ok := args["timeout_seconds"].(float64); ok && v > 0 {
		timeoutSec = int(v)
	}

	client := &http.Client{
		Timeout: time.Duration(timeoutSec) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 1 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("HTTP probe results (%d targets):\n\n", len(rawTargets)))

	for _, raw := range rawTargets {
		target, _ := raw.(string)
		if target == "" {
			continue
		}
		// Normalise: add scheme if missing.
		if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
			target = "http://" + target
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			sb.WriteString(fmt.Sprintf("  %-50s  ERROR: %v\n", target, err))
			continue
		}
		req.Header.Set("User-Agent", "Kestrel/1.0 (security-research)")

		resp, err := client.Do(req)
		if err != nil {
			sb.WriteString(fmt.Sprintf("  %-50s  UNREACHABLE: %v\n", target, err))
			continue
		}
		resp.Body.Close()

		title := extractTitle(resp)
		server := resp.Header.Get("Server")
		sb.WriteString(fmt.Sprintf("  %-50s  %d  server=%q  title=%q\n", target, resp.StatusCode, server, title))
	}
	return sb.String(), nil
}

func extractTitle(resp *http.Response) string {
	// We already closed the body — just return the redirect location if available.
	if loc := resp.Header.Get("Location"); loc != "" {
		return "[redirect] " + loc
	}
	return ""
}

func dnsLookupHandler(ctx context.Context, args map[string]interface{}) (string, error) {
	hostname, _ := args["hostname"].(string)
	if hostname == "" {
		return "", fmt.Errorf("hostname is required")
	}
	hostname = strings.TrimSpace(hostname)

	recordTypesRaw, _ := args["record_types"].([]interface{})
	recordTypes := []string{"A", "AAAA", "MX", "TXT", "NS"}
	if len(recordTypesRaw) > 0 {
		recordTypes = nil
		for _, rt := range recordTypesRaw {
			if s, ok := rt.(string); ok {
				recordTypes = append(recordTypes, strings.ToUpper(s))
			}
		}
	}

	resolver := &net.Resolver{}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("DNS lookup results for %s:\n\n", hostname))

	for _, rtype := range recordTypes {
		switch rtype {
		case "A", "AAAA":
			addrs, err := resolver.LookupHost(ctx, hostname)
			if err != nil {
				sb.WriteString(fmt.Sprintf("  %-6s  ERROR: %v\n", rtype, err))
				continue
			}
			for _, addr := range addrs {
				ip := net.ParseIP(addr)
				if ip == nil {
					continue
				}
				if (rtype == "A" && ip.To4() != nil) || (rtype == "AAAA" && ip.To4() == nil) {
					sb.WriteString(fmt.Sprintf("  %-6s  %s\n", rtype, addr))
				}
			}
		case "MX":
			mxs, err := resolver.LookupMX(ctx, hostname)
			if err != nil {
				sb.WriteString(fmt.Sprintf("  %-6s  ERROR: %v\n", rtype, err))
				continue
			}
			for _, mx := range mxs {
				sb.WriteString(fmt.Sprintf("  %-6s  %d %s\n", rtype, mx.Pref, mx.Host))
			}
		case "TXT":
			txts, err := resolver.LookupTXT(ctx, hostname)
			if err != nil {
				sb.WriteString(fmt.Sprintf("  %-6s  ERROR: %v\n", rtype, err))
				continue
			}
			for _, txt := range txts {
				sb.WriteString(fmt.Sprintf("  %-6s  %q\n", rtype, txt))
			}
		case "NS":
			nss, err := resolver.LookupNS(ctx, hostname)
			if err != nil {
				sb.WriteString(fmt.Sprintf("  %-6s  ERROR: %v\n", rtype, err))
				continue
			}
			for _, ns := range nss {
				sb.WriteString(fmt.Sprintf("  %-6s  %s\n", rtype, ns.Host))
			}
		case "CNAME":
			cname, err := resolver.LookupCNAME(ctx, hostname)
			if err != nil {
				sb.WriteString(fmt.Sprintf("  %-6s  ERROR: %v\n", rtype, err))
				continue
			}
			sb.WriteString(fmt.Sprintf("  %-6s  %s\n", rtype, cname))
		}
	}
	return sb.String(), nil
}

// mustSchema marshals a schema map to JSON, panicking on error (only for init-time constants).
func mustSchema(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mcp: failed to marshal tool schema: %v", err))
	}
	return json.RawMessage(b)
}
