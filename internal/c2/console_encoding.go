package c2

import (
	"encoding/base64"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// NormalizeConsoleOutput converts raw implant/Shell console bytes to UTF-8 text.
// osTag comes from the session's os field (e.g. windows / Windows 10); empty values are treated as auto.
func NormalizeConsoleOutput(raw []byte, osTag string) string {
	if len(raw) == 0 {
		return ""
	}
	osTag = strings.ToLower(strings.TrimSpace(osTag))
	isWindows := strings.Contains(osTag, "windows")

	if utf8.Valid(raw) {
		return string(raw)
	}
	if isWindows {
		if out, _, err := transform.Bytes(simplifiedchinese.GB18030.NewDecoder(), raw); err == nil {
			return string(out)
		}
	}
	// Non-Windows or decode failed: fall back to GB18030 (covers GBK)
	if out, _, err := transform.Bytes(simplifiedchinese.GB18030.NewDecoder(), raw); err == nil {
		return string(out)
	}
	return string(raw)
}

// ResolveTaskResultText merges the Output/OutputB64 (and Error/ErrorB64) returned by the beacon, decoding according to the session OS.
func ResolveTaskResultText(plain, b64, sessionOS string) string {
	if strings.TrimSpace(b64) != "" {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
		if err == nil {
			return NormalizeConsoleOutput(raw, sessionOS)
		}
	}
	if plain == "" {
		return ""
	}
	return NormalizeConsoleOutput([]byte(plain), sessionOS)
}
