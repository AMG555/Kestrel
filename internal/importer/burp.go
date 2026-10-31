package importer

import (
	"encoding/xml"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"kestrel/internal/database"
)

type burpIssues struct {
	XMLName xml.Name    `xml:"issues"`
	Issues  []burpIssue `xml:"issue"`
}

type burpIssue struct {
	Name                  string   `xml:"name"`
	Host                  burpHost `xml:"host"`
	Path                  string   `xml:"path"`
	Location              string   `xml:"location"`
	Severity              string   `xml:"severity"`
	Confidence            string   `xml:"confidence"`
	IssueBackground       string   `xml:"issueBackground"`
	RemediationBackground string   `xml:"remediationBackground"`
	IssueDetail           string   `xml:"issueDetail"`
}

type burpHost struct {
	IP    string `xml:"ip,attr"`
	Value string `xml:",chardata"`
}

// ParseBurp parses Burp Suite issues XML into Assets and Vulnerabilities.
func ParseBurp(data []byte, projectID string) ([]*database.Asset, []*database.Vulnerability, error) {
	var root burpIssues
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("failed to parse Burp XML: %w", err)
	}

	var assets []*database.Asset
	var vulns []*database.Vulnerability
	now := time.Now().UTC()
	assetSeen := make(map[string]bool)

	for _, issue := range root.Issues {
		title := strings.TrimSpace(issue.Name)
		if title == "" {
			continue
		}

		rawHost := strings.TrimSpace(issue.Host.Value)
		ipAddr := strings.TrimSpace(issue.Host.IP)

		target := rawHost + issue.Location
		if issue.Location == "" {
			target = rawHost + issue.Path
		}

		sev := strings.ToLower(strings.TrimSpace(issue.Severity))
		switch sev {
		case "information", "informational":
			sev = "info"
		case "low", "medium", "high":
			// matches database enum
		default:
			sev = "info"
		}

		desc := strings.TrimSpace(stripHTML(issue.IssueBackground))
		evidence := strings.TrimSpace(stripHTML(issue.IssueDetail))
		remediation := strings.TrimSpace(stripHTML(issue.RemediationBackground))

		v := &database.Vulnerability{
			ProjectID:      projectID,
			Title:          title,
			Description:    desc,
			Severity:       sev,
			Status:         "open",
			Target:         target,
			Evidence:       evidence,
			Recommendation: remediation,
			Type:           "web",
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		vulns = append(vulns, v)

		// Parse host into asset
		parsed, err := url.Parse(rawHost)
		var hostName, domainPart string
		portNum := 0
		protocol := "http"

		if err == nil && parsed.Host != "" {
			protocol = parsed.Scheme
			h, p, splitErr := net.SplitHostPort(parsed.Host)
			if splitErr == nil {
				hostName = h
				portNum, _ = strconv.Atoi(p)
			} else {
				hostName = parsed.Host
				if protocol == "https" {
					portNum = 443
				} else {
					portNum = 80
				}
			}
		} else {
			hostName = rawHost
		}

		if ipAddr == "" && net.ParseIP(hostName) != nil {
			ipAddr = hostName
		} else if hostName != "" && net.ParseIP(hostName) == nil {
			domainPart = hostName
		}

		dedup := fmt.Sprintf("%s:%d:%s", hostName, portNum, protocol)
		if hostName != "" && !assetSeen[dedup] {
			assetSeen[dedup] = true
			a := &database.Asset{
				ProjectID:   projectID,
				Host:        hostName,
				IP:          ipAddr,
				Domain:      domainPart,
				Port:        portNum,
				Protocol:    protocol,
				Source:      "burp",
				SourceQuery: "burp_export",
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

// stripHTML removes basic HTML tags common in Burp Suite issue descriptions.
func stripHTML(input string) string {
	var b strings.Builder
	inTag := false
	for _, r := range input {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return strings.ReplaceAll(b.String(), "&lt;", "<")
}
