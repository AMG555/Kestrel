package importer

import (
	"encoding/xml"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"kestrel/internal/database"
)

type nmapRun struct {
	XMLName xml.Name   `xml:"nmaprun"`
	Hosts   []nmapHost `xml:"host"`
}

type nmapHost struct {
	Status    nmapStatus    `xml:"status"`
	Addresses []nmapAddress `xml:"address"`
	Hostnames nmapHostnames `xml:"hostnames"`
	Ports     nmapPorts     `xml:"ports"`
}

type nmapStatus struct {
	State string `xml:"state,attr"`
}

type nmapAddress struct {
	Addr     string `xml:"addr,attr"`
	AddrType string `xml:"addrtype,attr"`
}

type nmapHostnames struct {
	Hostnames []nmapHostname `xml:"hostname"`
}

type nmapHostname struct {
	Name string `xml:"name,attr"`
}

type nmapPorts struct {
	Ports []nmapPort `xml:"port"`
}

type nmapPort struct {
	Protocol string       `xml:"protocol,attr"`
	PortID   string       `xml:"portid,attr"`
	State    nmapState    `xml:"state"`
	Service  nmapService  `xml:"service"`
	Scripts  []nmapScript `xml:"script"`
}

type nmapState struct {
	State string `xml:"state,attr"`
}

type nmapService struct {
	Name      string `xml:"name,attr"`
	Product   string `xml:"product,attr"`
	Version   string `xml:"version,attr"`
	ExtraInfo string `xml:"extrainfo,attr"`
}

type nmapScript struct {
	ID     string `xml:"id,attr"`
	Output string `xml:"output,attr"`
}

// ParseNmap parses standard Nmap XML (-oX) into Assets and potential script Vulnerabilities.
func ParseNmap(data []byte, projectID string) ([]*database.Asset, []*database.Vulnerability, error) {
	var run nmapRun
	if err := xml.Unmarshal(data, &run); err != nil {
		return nil, nil, fmt.Errorf("failed to parse Nmap XML: %w", err)
	}

	var assets []*database.Asset
	var vulns []*database.Vulnerability
	now := time.Now().UTC()

	for _, host := range run.Hosts {
		if host.Status.State != "up" && len(host.Ports.Ports) == 0 {
			continue
		}

		// Extract IP
		var ip string
		for _, addr := range host.Addresses {
			if addr.AddrType == "ipv4" || addr.AddrType == "ipv6" {
				ip = addr.Addr
				break
			}
		}

		// Extract hostname
		var hostname string
		if len(host.Hostnames.Hostnames) > 0 {
			hostname = host.Hostnames.Hostnames[0].Name
		}

		hostIdentifier := ip
		if hostIdentifier == "" {
			hostIdentifier = hostname
		}
		if hostIdentifier == "" {
			continue
		}

		for _, port := range host.Ports.Ports {
			if port.State.State != "open" {
				continue
			}

			pNum, err := strconv.Atoi(port.PortID)
			if err != nil {
				continue
			}

			serverInfo := strings.TrimSpace(fmt.Sprintf("%s %s %s", port.Service.Product, port.Service.Version, port.Service.ExtraInfo))

			title := ""
			for _, script := range port.Scripts {
				if script.ID == "http-title" {
					title = strings.TrimSpace(script.Output)
					break
				}
			}

			protocol := strings.ToLower(port.Protocol)
			if protocol == "" {
				protocol = "tcp"
			}

			domain := ""
			if hostname != "" && net.ParseIP(hostname) == nil {
				domain = hostname
			}

			asset := &database.Asset{
				ProjectID:   projectID,
				Host:        hostIdentifier,
				IP:          ip,
				Port:        pNum,
				Domain:      domain,
				Protocol:    protocol,
				Title:       title,
				Server:      serverInfo,
				Source:      "nmap",
				SourceQuery: "nmap -oX",
				Status:      "active",
				FirstSeenAt: now,
				LastSeenAt:  now,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			assets = append(assets, asset)

			// Inspect vulnerability-bearing NSE scripts
			for _, script := range port.Scripts {
				if isVulnerabilityScript(script.ID, script.Output) {
					v := &database.Vulnerability{
						ProjectID:   projectID,
						Title:       fmt.Sprintf("%s on %s:%d", script.ID, hostIdentifier, pNum),
						Description: fmt.Sprintf("Nmap NSE script %q reported potential vulnerability.", script.ID),
						Severity:    inferScriptSeverity(script.ID, script.Output),
						Status:      "open",
						Target:      fmt.Sprintf("%s:%d", hostIdentifier, pNum),
						Evidence:    strings.TrimSpace(script.Output),
						Type:        "network_service",
						CreatedAt:   now,
						UpdatedAt:   now,
					}
					vulns = append(vulns, v)
				}
			}
		}
	}

	return assets, vulns, nil
}

func isVulnerabilityScript(id, output string) bool {
	idLower := strings.ToLower(id)
	outLower := strings.ToLower(output)
	if strings.Contains(idLower, "vuln") || strings.Contains(idLower, "exploit") ||
		strings.Contains(idLower, "heartbleed") || strings.Contains(idLower, "poodle") ||
		strings.Contains(idLower, "ms08-067") || strings.Contains(idLower, "ms17-010") {
		return strings.Contains(outLower, "vulnerable") || strings.Contains(outLower, "state: vulnerable") || strings.Contains(outLower, "cve-")
	}
	return false
}

func inferScriptSeverity(id, output string) string {
	lower := strings.ToLower(id + " " + output)
	if strings.Contains(lower, "remote code execution") || strings.Contains(lower, "rce") || strings.Contains(lower, "ms17-010") {
		return "critical"
	}
	if strings.Contains(lower, "critical") {
		return "critical"
	}
	if strings.Contains(lower, "high") {
		return "high"
	}
	if strings.Contains(lower, "medium") {
		return "medium"
	}
	return "high"
}
