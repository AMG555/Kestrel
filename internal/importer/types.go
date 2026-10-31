package importer

import (
	"bytes"
	"strings"
)

// Format constants for supported scan outputs.
const (
	FormatNmap   = "nmap"
	FormatNuclei = "nuclei"
	FormatBurp   = "burp"
)

// ImportResult tracks summary metrics of imported artifacts.
type ImportResult struct {
	Format        string   `json:"format"`
	AssetsCreated int      `json:"assets_created"`
	AssetsUpdated int      `json:"assets_updated"`
	AssetsSkipped int      `json:"assets_skipped"`
	VulnsCreated  int      `json:"vulns_created"`
	VulnsSkipped  int      `json:"vulns_skipped"`
	Errors        []string `json:"errors,omitempty"`
}

// DetectFormat inspects byte content and heuristics to determine the input format.
func DetectFormat(data []byte) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return ""
	}

	// 1. Check for XML formats
	if bytes.HasPrefix(trimmed, []byte("<")) {
		header := string(trimmed[:min(len(trimmed), 1024)])
		if strings.Contains(header, "<nmaprun") {
			return FormatNmap
		}
		if strings.Contains(header, "<issues") || strings.Contains(header, "<issue") {
			return FormatBurp
		}
	}

	// 2. Check for Nuclei JSON or JSONL
	sample := string(trimmed[:min(len(trimmed), 2048)])
	if strings.Contains(sample, `"template-id"`) || strings.Contains(sample, `"templateID"`) ||
		(strings.Contains(sample, `"matcher-name"`) && strings.Contains(sample, `"matched-at"`)) {
		return FormatNuclei
	}

	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
