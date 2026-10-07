// Package robot provides proactive notification integrations for Webhook, Slack, Discord, and Telegram.
package robot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Config defines settings for external webhook and chat notification bots.
type Config struct {
	Enabled              bool   `json:"enabled" yaml:"enabled"`
	WebhookURL           string `json:"webhook_url" yaml:"webhook_url"`
	SlackWebhookURL      string `json:"slack_webhook_url" yaml:"slack_webhook_url"`
	DiscordWebhookURL    string `json:"discord_webhook_url" yaml:"discord_webhook_url"`
	TelegramBotToken     string `json:"telegram_bot_token" yaml:"telegram_bot_token"`
	TelegramChatID       string `json:"telegram_chat_id" yaml:"telegram_chat_id"`
	NotifyOnCriticalVuln bool   `json:"notify_on_critical_vuln" yaml:"notify_on_critical_vuln"`
	NotifyOnHITL         bool   `json:"notify_on_hitl" yaml:"notify_on_hitl"`
	NotifyOnTaskDone     bool   `json:"notify_on_task_done" yaml:"notify_on_task_done"`
}

// Notification represents an outbound alert event.
type Notification struct {
	Event     string                 `json:"event"`     // vuln_alert | hitl_approval | task_complete | test
	Title     string                 `json:"title"`
	Message   string                 `json:"message"`
	Severity  string                 `json:"severity"`  // critical | high | medium | low | info
	Timestamp string                 `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Service manages notification dispatch.
type Service struct {
	mu     sync.RWMutex
	cfg    Config
	client *http.Client
	logger *zap.Logger
}

// NewService creates a new notification service.
func NewService(cfg Config, logger *zap.Logger) *Service {
	return &Service{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// GetConfig returns the current notification configuration.
func (s *Service) GetConfig() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// UpdateConfig updates the notification configuration.
func (s *Service) UpdateConfig(cfg Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
}

// Notify emits a notification to all configured platforms asynchronously.
func (s *Service) Notify(n Notification) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	if !cfg.Enabled && n.Event != "test" {
		return
	}

	if n.Timestamp == "" {
		n.Timestamp = time.Now().Format(time.RFC3339)
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if cfg.WebhookURL != "" {
			_ = s.sendGenericWebhook(ctx, cfg.WebhookURL, n)
		}
		if cfg.SlackWebhookURL != "" {
			_ = s.sendSlackWebhook(ctx, cfg.SlackWebhookURL, n)
		}
		if cfg.DiscordWebhookURL != "" {
			_ = s.sendDiscordWebhook(ctx, cfg.DiscordWebhookURL, n)
		}
		if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
			_ = s.sendTelegramMessage(ctx, cfg.TelegramBotToken, cfg.TelegramChatID, n)
		}
	}()
}

// SendTest sends an immediate test notification to verify integration.
func (s *Service) SendTest(channel string) error {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	n := Notification{
		Event:     "test",
		Title:     "Kestrel Security Operations Alert",
		Message:   fmt.Sprintf("This is a test notification from Kestrel to test channel %q. Delivery successful.", channel),
		Severity:  "info",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch strings.ToLower(channel) {
	case "webhook":
		if cfg.WebhookURL == "" {
			return fmt.Errorf("generic webhook URL not configured")
		}
		return s.sendGenericWebhook(ctx, cfg.WebhookURL, n)
	case "slack":
		if cfg.SlackWebhookURL == "" {
			return fmt.Errorf("slack webhook URL not configured")
		}
		return s.sendSlackWebhook(ctx, cfg.SlackWebhookURL, n)
	case "discord":
		if cfg.DiscordWebhookURL == "" {
			return fmt.Errorf("discord webhook URL not configured")
		}
		return s.sendDiscordWebhook(ctx, cfg.DiscordWebhookURL, n)
	case "telegram":
		if cfg.TelegramBotToken == "" || cfg.TelegramChatID == "" {
			return fmt.Errorf("telegram bot token or chat ID not configured")
		}
		return s.sendTelegramMessage(ctx, cfg.TelegramBotToken, cfg.TelegramChatID, n)
	default:
		s.Notify(n)
		return nil
	}
}

func (s *Service) sendGenericWebhook(ctx context.Context, url string, n Notification) error {
	data, err := json.Marshal(n)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		s.logger.Warn("robot: generic webhook failed", zap.Error(err), zap.String("url", url))
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (s *Service) sendSlackWebhook(ctx context.Context, url string, n Notification) error {
	color := "#36a64f"
	if strings.EqualFold(n.Severity, "critical") {
		color = "#e01e5a"
	} else if strings.EqualFold(n.Severity, "high") {
		color = "#ecb22e"
	}

	payload := map[string]interface{}{
		"text": fmt.Sprintf("*[%s] %s*\n%s", strings.ToUpper(n.Severity), n.Title, n.Message),
		"attachments": []map[string]interface{}{
			{
				"color": color,
				"fields": []map[string]interface{}{
					{"title": "Event", "value": n.Event, "short": true},
					{"title": "Time", "value": n.Timestamp, "short": true},
				},
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		s.logger.Warn("robot: slack webhook failed", zap.Error(err))
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (s *Service) sendDiscordWebhook(ctx context.Context, url string, n Notification) error {
	colorInt := 3066993 // Green
	if strings.EqualFold(n.Severity, "critical") {
		colorInt = 15158332 // Red
	} else if strings.EqualFold(n.Severity, "high") {
		colorInt = 15105570 // Orange
	}

	payload := map[string]interface{}{
		"content": fmt.Sprintf("**⚔ Kestrel Alert: %s**", n.Title),
		"embeds": []map[string]interface{}{
			{
				"title":       n.Title,
				"description": n.Message,
				"color":       colorInt,
				"fields": []map[string]interface{}{
					{"name": "Severity", "value": strings.ToUpper(n.Severity), "inline": true},
					{"name": "Event", "value": n.Event, "inline": true},
				},
				"timestamp": n.Timestamp,
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		s.logger.Warn("robot: discord webhook failed", zap.Error(err))
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (s *Service) sendTelegramMessage(ctx context.Context, botToken, chatID string, n Notification) error {
	text := fmt.Sprintf("⚔ *Kestrel Notification*\n*Title:* %s\n*Severity:* %s\n*Event:* %s\n\n%s",
		n.Title, strings.ToUpper(n.Severity), n.Event, n.Message)

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		s.logger.Warn("robot: telegram message failed", zap.Error(err))
		return err
	}
	defer resp.Body.Close()
	return nil
}
