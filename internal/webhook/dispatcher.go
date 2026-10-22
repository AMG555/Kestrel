package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"kestrel/internal/database"
)

var severityRank = map[string]int{
	"info":     0,
	"low":      1,
	"medium":   2,
	"high":     3,
	"critical": 4,
}

// Payload is the standard JSON structure sent to generic webhook receivers.
type Payload struct {
	Event         string                 `json:"event"`
	Timestamp     string                 `json:"timestamp"`
	Vulnerability VulnerabilitySummary   `json:"vulnerability"`
	Extra         map[string]interface{} `json:"extra,omitempty"`
}

// VulnerabilitySummary summarizes findings for alerting.
type VulnerabilitySummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	Type        string `json:"type"`
	Target      string `json:"target"`
	Impact      string `json:"impact"`
	Remediation string `json:"remediation,omitempty"`
}

// ShouldAlert checks if finding meets the minimum severity threshold.
func ShouldAlert(findingSeverity, minSeverity string) bool {
	fRank, ok1 := severityRank[strings.ToLower(strings.TrimSpace(findingSeverity))]
	if !ok1 {
		fRank = 0
	}
	mRank, ok2 := severityRank[strings.ToLower(strings.TrimSpace(minSeverity))]
	if !ok2 {
		mRank = 3 // default high
	}
	return fRank >= mRank
}

// DispatchVulnerabilityAlert sends an HTTP POST notification to the target webhook.
func DispatchVulnerabilityAlert(ctx context.Context, webhookURL, minSeverity string, v *database.Vulnerability) error {
	if strings.TrimSpace(webhookURL) == "" || v == nil {
		return nil
	}

	if !ShouldAlert(v.Severity, minSeverity) {
		return nil
	}

	var reqBody []byte
	var err error

	// Custom format for Slack / Discord if detected
	lowerURL := strings.ToLower(webhookURL)
	if strings.Contains(lowerURL, "hooks.slack.com") || strings.Contains(lowerURL, "discord.com/api/webhooks") {
		slackMsg := map[string]interface{}{
			"text": fmt.Sprintf("🚨 *[Kestrel Alert]* %s Vulnerability Found: *%s*\n• *Target:* `%s`\n• *Type:* `%s`\n• *Severity:* `%s`",
				strings.ToUpper(v.Severity), v.Title, v.Target, v.Type, strings.ToUpper(v.Severity)),
		}
		reqBody, err = json.Marshal(slackMsg)
	} else {
		payload := Payload{
			Event:     "vulnerability.detected",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Vulnerability: VulnerabilitySummary{
				ID:          v.ID,
				Title:       v.Title,
				Severity:    v.Severity,
				Type:        v.Type,
				Target:      v.Target,
				Impact:      v.Impact,
				Remediation: v.Recommendation,
			},
		}
		reqBody, err = json.Marshal(payload)
	}

	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Kestrel-Webhook-Alerts/1.0")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook dispatch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook responded with non-2xx status: %d", resp.StatusCode)
	}

	return nil
}

// DispatchAsync dispatches the webhook in a separate goroutine without blocking.
func DispatchAsync(webhookURL, minSeverity string, v *database.Vulnerability) {
	if strings.TrimSpace(webhookURL) == "" || v == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_ = DispatchVulnerabilityAlert(ctx, webhookURL, minSeverity, v)
	}()
}
