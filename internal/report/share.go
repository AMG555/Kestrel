package report

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"kestrel/internal/database"
)

var (
	ErrShareTokenInvalid = errors.New("invalid share token")
	ErrShareTokenExpired = errors.New("share token expired")
)

// CreateShareToken generates a signed HMAC-SHA256 permalink token.
func CreateShareToken(projectID string, expiresAt time.Time, secret []byte) (string, string, error) {
	if len(secret) == 0 {
		return "", "", errors.New("empty signing secret")
	}
	nonceBytes := make([]byte, 8)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)

	payload := fmt.Sprintf("%s:%d:%s", projectID, expiresAt.Unix(), nonce)
	payloadEnc := base64.RawURLEncoding.EncodeToString([]byte(payload))

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payloadEnc))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	token := payloadEnc + "." + sig
	tokenHash := database.HashShareToken(token)

	return token, tokenHash, nil
}

// VerifyShareToken validates the signature and expiration of a share permalink token.
func VerifyShareToken(token string, secret []byte) (string, time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", time.Time{}, ErrShareTokenInvalid
	}

	payloadEnc, sigEnc := parts[0], parts[1]

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payloadEnc))
	expectedSig := mac.Sum(nil)

	actualSig, err := base64.RawURLEncoding.DecodeString(sigEnc)
	if err != nil || !hmac.Equal(actualSig, expectedSig) {
		return "", time.Time{}, ErrShareTokenInvalid
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadEnc)
	if err != nil {
		return "", time.Time{}, ErrShareTokenInvalid
	}

	fields := strings.Split(string(payloadBytes), ":")
	if len(fields) != 3 {
		return "", time.Time{}, ErrShareTokenInvalid
	}

	projectID := fields[0]
	expUnix, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return "", time.Time{}, ErrShareTokenInvalid
	}

	expiresAt := time.Unix(expUnix, 0)
	if time.Now().After(expiresAt) {
		return "", time.Time{}, ErrShareTokenExpired
	}

	return projectID, expiresAt, nil
}

// HTMLReportOptions holds data for rendering a read-only report permalink.
type HTMLReportOptions struct {
	Project            *database.Project
	Vulns              []*database.Vulnerability
	BaseURL            string
	ShareToken         string
	PassphraseRequired bool
}

// RenderHTMLReport generates a standalone, accessible, mobile-responsive HTML assessment report.
func RenderHTMLReport(opts HTMLReportOptions) []byte {
	projectName := "Target Assessment"
	if opts.Project != nil && strings.TrimSpace(opts.Project.Name) != "" {
		projectName = opts.Project.Name
	}

	if opts.PassphraseRequired {
		return renderPassphraseChallenge(projectName, opts.ShareToken)
	}

	var critical, high, medium, low, info int
	for _, v := range opts.Vulns {
		switch strings.ToLower(strings.TrimSpace(v.Severity)) {
		case "critical":
			critical++
		case "high":
			high++
		case "medium":
			medium++
		case "low":
			low++
		default:
			info++
		}
	}
	total := len(opts.Vulns)
	summaryText := fmt.Sprintf("%d Critical, %d High, %d Medium findings (%d total)", critical, high, medium, total)

	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	sb.WriteString("<meta charset=\"utf-8\">\n")
	sb.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	sb.WriteString(fmt.Sprintf("<title>Security Assessment Report — %s | Kestrel</title>\n", html.EscapeString(projectName)))

	// OpenGraph and Twitter Meta Tags for Discord, Slack, X, LinkedIn auto-unfurl
	sb.WriteString(fmt.Sprintf("<meta property=\"og:title\" content=\"Security Assessment: %s\">\n", html.EscapeString(projectName)))
	sb.WriteString(fmt.Sprintf("<meta property=\"og:description\" content=\"Kestrel Security Report: %s\">\n", html.EscapeString(summaryText)))
	sb.WriteString("<meta property=\"og:type\" content=\"article\">\n")
	sb.WriteString("<meta name=\"twitter:card\" content=\"summary\">\n")
	sb.WriteString(fmt.Sprintf("<meta name=\"twitter:title\" content=\"Security Assessment: %s\">\n", html.EscapeString(projectName)))
	sb.WriteString(fmt.Sprintf("<meta name=\"twitter:description\" content=\"Kestrel Security Report: %s\">\n", html.EscapeString(summaryText)))

	// CSS Styling
	sb.WriteString(`<style>
:root {
  --bg: #0d1117; --surface: #161b22; --surface2: #21262d;
  --border: #30363d; --text: #e6edf3; --muted: #8b949e;
  --accent: #58a6ff; --danger: #f85149; --warn: #f0883e;
  --yellow: #e3b341; --success: #3fb950;
}
* { box-sizing: border-box; margin: 0; padding: 0; }
body { background: var(--bg); color: var(--text); font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; line-height: 1.5; padding: 24px; }
.container { max-width: 1040px; margin: 0 auto; }
.header { display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid var(--border); padding-bottom: 20px; margin-bottom: 24px; flex-wrap: wrap; gap: 12px; }
.logo-title { display: flex; align-items: center; gap: 10px; }
.badge { display: inline-block; padding: 2px 8px; border-radius: 12px; font-size: 12px; font-weight: 600; text-transform: uppercase; }
.badge-critical { background: #f8514920; color: var(--danger); border: 1px solid var(--danger); }
.badge-high { background: #f0883e20; color: var(--warn); border: 1px solid var(--warn); }
.badge-medium { background: #e3b34120; color: var(--yellow); border: 1px solid var(--yellow); }
.badge-low { background: #58a6ff20; color: var(--accent); border: 1px solid var(--accent); }
.badge-info { background: #8b949e20; color: var(--muted); border: 1px solid var(--border); }
.stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 12px; margin-bottom: 28px; }
.stat-card { background: var(--surface); border: 1px solid var(--border); border-radius: 8px; padding: 16px; text-align: center; }
.stat-num { font-size: 26px; font-weight: 700; margin-top: 4px; }
.card { background: var(--surface); border: 1px solid var(--border); border-radius: 8px; padding: 20px; margin-bottom: 16px; }
.card-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; gap: 8px; }
.target-box { background: var(--surface2); padding: 6px 12px; border-radius: 6px; font-family: monospace; font-size: 13px; margin: 8px 0; word-break: break-all; }
.evidence-box { background: #00000040; border-left: 3px solid var(--accent); padding: 10px 14px; font-family: monospace; font-size: 12px; margin-top: 10px; overflow-x: auto; white-space: pre-wrap; }
.footer { border-top: 1px solid var(--border); margin-top: 40px; padding-top: 24px; text-align: center; color: var(--muted); font-size: 13px; }
.cta-box { background: linear-gradient(135deg, #161b22 0%, #1f242c 100%); border: 1px solid var(--accent); border-radius: 8px; padding: 20px; margin: 32px 0; text-align: center; }
.btn-copy { background: var(--accent); color: #000; border: none; padding: 8px 16px; border-radius: 6px; font-weight: 600; cursor: pointer; font-size: 13px; margin-top: 10px; }
.btn-copy:hover { opacity: 0.9; }
</style>
<script>
function copyCLI() {
  const code = 'curl -sSL https://raw.githubusercontent.com/AMG555/Kestrel/main/run.sh | bash';
  navigator.clipboard.writeText(code).then(() => {
    const b = document.getElementById('copyBtn');
    b.innerText = 'Copied!';
    setTimeout(() => { b.innerText = 'Copy Quickstart Command'; }, 2000);
  });
}
</script>
</head>
<body>
<div class="container">
  <div class="header">
    <div class="logo-title">
      <span style="font-size:24px;">⚔</span>
      <h1 style="font-size:20px; font-weight:700;">Kestrel Security Assessment</h1>
    </div>
    <div style="font-size:13px; color:var(--muted);">
      Target: <strong style="color:var(--text);">` + html.EscapeString(projectName) + `</strong>
    </div>
  </div>

  <div class="stats-grid">
    <div class="stat-card">
      <div style="font-size:12px; color:var(--muted);">CRITICAL</div>
      <div class="stat-num" style="color:var(--danger);">` + strconv.Itoa(critical) + `</div>
    </div>
    <div class="stat-card">
      <div style="font-size:12px; color:var(--muted);">HIGH</div>
      <div class="stat-num" style="color:var(--warn);">` + strconv.Itoa(high) + `</div>
    </div>
    <div class="stat-card">
      <div style="font-size:12px; color:var(--muted);">MEDIUM</div>
      <div class="stat-num" style="color:var(--yellow);">` + strconv.Itoa(medium) + `</div>
    </div>
    <div class="stat-card">
      <div style="font-size:12px; color:var(--muted);">LOW / INFO</div>
      <div class="stat-num" style="color:var(--accent);">` + strconv.Itoa(low+info) + `</div>
    </div>
    <div class="stat-card">
      <div style="font-size:12px; color:var(--muted);">TOTAL FINDINGS</div>
      <div class="stat-num">` + strconv.Itoa(total) + `</div>
    </div>
  </div>

  <h2 style="font-size:16px; font-weight:600; margin-bottom:14px;">Assessment Findings</h2>
`)

	if len(opts.Vulns) == 0 {
		sb.WriteString(`<div class="card" style="text-align:center; padding:32px; color:var(--success);">
      ✓ No confirmed vulnerabilities detected. Target posture verified.
    </div>`)
	} else {
		for _, v := range opts.Vulns {
			sevClass := "badge-info"
			switch strings.ToLower(v.Severity) {
			case "critical":
				sevClass = "badge-critical"
			case "high":
				sevClass = "badge-high"
			case "medium":
				sevClass = "badge-medium"
			case "low":
				sevClass = "badge-low"
			}

			sb.WriteString(`<div class="card">
        <div class="card-header">
          <div style="font-size:16px; font-weight:600;">` + html.EscapeString(v.Title) + `</div>
          <span class="badge ` + sevClass + `">` + html.EscapeString(v.Severity) + `</span>
        </div>`)

			if v.Target != "" {
				sb.WriteString(`<div class="target-box">Target: ` + html.EscapeString(v.Target) + `</div>`)
			}

			if v.Description != "" {
				sb.WriteString(`<p style="color:var(--text); font-size:14px; margin-top:8px;">` + html.EscapeString(v.Description) + `</p>`)
			}

			if v.Evidence != "" {
				sb.WriteString(`<div class="evidence-box"><strong>Reproduction / Evidence:</strong>` + "\n" + html.EscapeString(v.Evidence) + `</div>`)
			}

			if v.Recommendation != "" {
				sb.WriteString(`<div style="margin-top:10px; font-size:13px; color:var(--muted);"><strong style="color:var(--success);">Remediation:</strong> ` + html.EscapeString(v.Recommendation) + `</div>`)
			}

			sb.WriteString(`</div>`)
		}
	}

	// Growth mechanism CTA & attribution footer
	sb.WriteString(`
  <div class="cta-box">
    <h3 style="font-size:16px; font-weight:700; margin-bottom:6px;">Automate Security Posture Verification with Kestrel</h3>
    <p style="color:var(--muted); font-size:13px; max-width:600px; margin:0 auto 12px;">
      Autonomous agentic penetration testing, continuous reconnaissance, and verifiable SARIF quality gates.
    </p>
    <div style="background:var(--surface2); padding:10px; border-radius:6px; font-family:monospace; font-size:12px; display:inline-block;">
      curl -sSL https://raw.githubusercontent.com/AMG555/Kestrel/main/run.sh | bash
    </div>
    <br>
    <button id="copyBtn" class="btn-copy" onclick="copyCLI()">Copy Quickstart Command</button>
  </div>

  <div class="footer">
    Report rendered by <strong>⚔ Kestrel Security Platform</strong>. Append-only, cryptographically verified findings.
  </div>
</div>
</body>
</html>`)

	return []byte(sb.String())
}

func renderPassphraseChallenge(projectName, shareToken string) []byte {
	return []byte(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Protected Report — ` + html.EscapeString(projectName) + ` | Kestrel</title>
<style>
body { background: #0d1117; color: #e6edf3; font-family: sans-serif; display: flex; align-items: center; justify-content: center; min-height: 100vh; margin: 0; }
.card { background: #161b22; border: 1px solid #30363d; border-radius: 8px; padding: 32px; width: 340px; text-align: center; }
input { width: 100%; padding: 10px; background: #21262d; border: 1px solid #30363d; color: #fff; border-radius: 6px; margin: 16px 0; box-sizing: border-box; }
button { width: 100%; padding: 10px; background: #58a6ff; color: #000; font-weight: 600; border: none; border-radius: 6px; cursor: pointer; }
</style>
</head>
<body>
<div class="card">
  <div style="font-size:28px; margin-bottom:12px;">🔒</div>
  <h3>Passphrase Protected</h3>
  <p style="color:#8b949e; font-size:13px; margin-top:4px;">Enter password to view ` + html.EscapeString(projectName) + ` assessment report.</p>
  <form method="POST" action="/r/` + html.EscapeString(shareToken) + `/unlock">
    <input type="password" name="passphrase" placeholder="Passphrase" required autofocus>
    <button type="submit">Unlock Report</button>
  </form>
</div>
</body>
</html>`)
}
