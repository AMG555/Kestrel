package importer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"kestrel/internal/database"
)

type nucleiResult struct {
	TemplateID string          `json:"template-id"`
	Info       nucleiInfo      `json:"info"`
	Type       string          `json:"type"`
	Host       string          `json:"host"`
	MatchedAt  string          `json:"matched-at"`
	Extracted  []string        `json:"extracted-results"`
	CurlCmd    string          `json:"curl-command"`
	Timestamp  string          `json:"timestamp"`
	IP         string          `json:"ip"`
	Port       json.RawMessage `json:"port"`
}

type nucleiClassification struct {
	CVEID       interface{} `json:"cve-id"`
	CWEID       interface{} `json:"cwe-id"`
	CVSSMetrics string      `json:"cvss-metrics"`
	CVSSScore   float64     `json:"cvss-score"`
	EPSSScore   float64     `json:"epss-score"`
}

type nucleiInfo struct {
	Name           string                `json:"name"`
	Author         interface{}           `json:"author"`
	Severity       string                `json:"severity"`
	Description    string                `json:"description"`
	Remediation    string                `json:"remediation"`
	Tags           interface{}           `json:"tags"`
	Classification *nucleiClassification `json:"classification"`
}

// ParseNuclei parses Nuclei JSON or JSONL stream output into Assets and Vulnerabilities.
func ParseNuclei(data []byte, projectID string) ([]*database.Asset, []*database.Vulnerability, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil, nil
	}

	var results []nucleiResult

	// Check if JSON array
	if bytes.HasPrefix(trimmed, []byte("[")) {
		if err := json.Unmarshal(trimmed, &results); err != nil {
			return nil, nil, fmt.Errorf("failed to parse Nuclei JSON array: %w", err)
		}
	} else {
		// Treat as JSONL
		scanner := bufio.NewScanner(bytes.NewReader(trimmed))
		// Increase buffer to handle large responses in JSONL
		buf := make([]byte, 1024*1024)
		scanner.Buffer(buf, 10*1024*1024)

		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			var item nucleiResult
			if err := json.Unmarshal(line, &item); err != nil {
				continue
			}
			results = append(results, item)
		}
		if err := scanner.Err(); err != nil && len(results) == 0 {
			return nil, nil, fmt.Errorf("failed to read Nuclei JSONL stream: %w", err)
		}
	}

	now := time.Now().UTC()
	var assets []*database.Asset
	var vulns []*database.Vulnerability
	assetSeen := make(map[string]bool)

	for _, res := range results {
		sev := strings.ToLower(strings.TrimSpace(res.Info.Severity))
		if sev == "" {
			sev = "info"
		}

		targetStr := res.MatchedAt
		if targetStr == "" {
			targetStr = res.Host
		}

		title := strings.TrimSpace(res.Info.Name)
		if title == "" {
			title = res.TemplateID
		}

		evidence := res.CurlCmd
		if len(res.Extracted) > 0 {
			if evidence != "" {
				evidence += "\n\nExtracted:\n" + strings.Join(res.Extracted, "\n")
			} else {
				evidence = "Extracted:\n" + strings.Join(res.Extracted, "\n")
			}
		}

		var cveStr, cweStr, cvssVector string
		var cvssScore, epssScore float64
		var isKEV bool

		tags := parseStringOrSlice(res.Info.Tags)
		for _, t := range tags {
			lt := strings.ToLower(t)
			if lt == "kev" || lt == "cisa" || lt == "cisa-kev" || lt == "known-exploited" {
				isKEV = true
				break
			}
		}

		if res.Info.Classification != nil {
			cves := parseStringOrSlice(res.Info.Classification.CVEID)
			if len(cves) > 0 {
				cveStr = strings.Join(cves, ", ")
			}
			cwes := parseStringOrSlice(res.Info.Classification.CWEID)
			if len(cwes) > 0 {
				cweStr = strings.Join(cwes, ", ")
			}
			cvssScore = res.Info.Classification.CVSSScore
			cvssVector = res.Info.Classification.CVSSMetrics
			epssScore = res.Info.Classification.EPSSScore
			if epssScore >= 0.90 {
				isKEV = true
			}
		}

		v := &database.Vulnerability{
			ProjectID:      projectID,
			Title:          title,
			Description:    res.Info.Description,
			Severity:       sev,
			Status:         "suspected",
			Target:         targetStr,
			Evidence:       evidence,
			Recommendation: res.Info.Remediation,
			Type:           res.Type,
			CVE:            cveStr,
			CWE:            cweStr,
			CVSSScore:      cvssScore,
			CVSSVector:     cvssVector,
			EPSSScore:      epssScore,
			KnownExploited: isKEV,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		vulns = append(vulns, v)

		// Parse Host/Port for Asset
		rawHost := res.Host
		if rawHost == "" {
			rawHost = res.MatchedAt
		}
		parsedURL, err := url.Parse(rawHost)
		var hostPart, ipPart, domainPart string
		portNum := 0
		protocol := res.Type
		if protocol == "" {
			protocol = "tcp"
		}

		if err == nil && parsedURL.Host != "" {
			h, p, splitErr := net.SplitHostPort(parsedURL.Host)
			if splitErr == nil {
				hostPart = h
				portNum, _ = strconv.Atoi(p)
			} else {
				hostPart = parsedURL.Host
				if parsedURL.Scheme == "https" {
					portNum = 443
				} else if parsedURL.Scheme == "http" {
					portNum = 80
				}
			}
			if parsedURL.Scheme != "" {
				protocol = parsedURL.Scheme
			}
		} else {
			hostPart = rawHost
		}

		if net.ParseIP(hostPart) != nil {
			ipPart = hostPart
		} else if hostPart != "" {
			domainPart = hostPart
		}
		if res.IP != "" {
			ipPart = res.IP
		}

		dedup := fmt.Sprintf("%s:%d:%s", hostPart, portNum, protocol)
		if hostPart != "" && !assetSeen[dedup] {
			assetSeen[dedup] = true
			a := &database.Asset{
				ProjectID:   projectID,
				Host:        hostPart,
				IP:          ipPart,
				Domain:      domainPart,
				Port:        portNum,
				Protocol:    protocol,
				Source:      "nuclei",
				SourceQuery: "nuclei",
				Status:      "active",
				FirstSeenAt: now,
				LastSeenAt:  now,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			assets = append(assets, a)
		}
	}

	return assets, vulns, nil
}

func parseStringOrSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		if strings.TrimSpace(val) == "" {
			return nil
		}
		parts := strings.Split(val, ",")
		var out []string
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []interface{}:
		var out []string
		for _, item := range val {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case []string:
		return val
	default:
		return nil
	}
}
