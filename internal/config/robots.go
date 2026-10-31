package config

import (
	"fmt"
	"strings"
)

// RobotsConfig robot configuration (WeCom, DingTalk, Feishu, WeChat iLink, Telegram, Slack, Discord, QQ, etc.)
type RobotsConfig struct {
	Session  RobotSessionConfig  `yaml:"session,omitempty" json:"session,omitempty"`   // robot session isolation strategy
	Wechat   RobotWechatConfig   `yaml:"wechat,omitempty" json:"wechat,omitempty"`     // WeChat (iLink scan code binding)
	Wecom    RobotWecomConfig    `yaml:"wecom,omitempty" json:"wecom,omitempty"`       // WeCom
	Dingtalk RobotDingtalkConfig `yaml:"dingtalk,omitempty" json:"dingtalk,omitempty"` // DingTalk
	Lark     RobotLarkConfig     `yaml:"lark,omitempty" json:"lark,omitempty"`         // Feishu
	Telegram RobotTelegramConfig `yaml:"telegram,omitempty" json:"telegram,omitempty"` // Telegram
	Slack    RobotSlackConfig    `yaml:"slack,omitempty" json:"slack,omitempty"`       // Slack
	Discord  RobotDiscordConfig  `yaml:"discord,omitempty" json:"discord,omitempty"`   // Discord
	QQ       RobotQQConfig       `yaml:"qq,omitempty" json:"qq,omitempty"`             // QQ robot
}

// RobotWechatConfig WeChat iLink robot configuration (personal WeChat ClawBot / iLink protocol)
type RobotWechatConfig struct {
	Enabled       bool                     `yaml:"enabled" json:"enabled"`
	BotToken      string                   `yaml:"bot_token,omitempty" json:"bot_token,omitempty"`
	ILinkBotID    string                   `yaml:"ilink_bot_id,omitempty" json:"ilink_bot_id,omitempty"`
	ILinkUserID   string                   `yaml:"ilink_user_id,omitempty" json:"ilink_user_id,omitempty"`
	BaseURL       string                   `yaml:"base_url,omitempty" json:"base_url,omitempty"`               // default https://ilinkai.weixin.qq.com
	BotType       string                   `yaml:"bot_type,omitempty" json:"bot_type,omitempty"`               // get_bot_qrcode parameter, default 3
	BotAgent      string                   `yaml:"bot_agent,omitempty" json:"bot_agent,omitempty"`             // base_info.bot_agent
	GetUpdatesBuf string                   `yaml:"get_updates_buf,omitempty" json:"get_updates_buf,omitempty"` // long polling cursor (runtime)
	Auth          RobotAuthorizationConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

const (
	RobotAuthModeUserBinding    = "user_binding"
	RobotAuthModeServiceAccount = "service_account"
)

// RobotAuthorizationConfig controls how a verified platform sender becomes
// an RBAC principal. service_account is intentionally fail-closed unless an
// explicit non-admin service user and sender allowlist are both configured.
type RobotAuthorizationConfig struct {
	Mode                 string   `yaml:"mode,omitempty" json:"mode,omitempty"`
	ServiceUserID        string   `yaml:"service_user_id,omitempty" json:"service_user_id,omitempty"`
	AllowedExternalUsers []string `yaml:"allowed_external_users,omitempty" json:"allowed_external_users,omitempty"`
}

func (c RobotAuthorizationConfig) EffectiveMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	if mode == "" {
		return RobotAuthModeUserBinding
	}
	return mode
}

func (c RobotAuthorizationConfig) ExternalUserAllowed(externalUserID string) bool {
	externalUserID = strings.TrimSpace(externalUserID)
	if externalUserID == "" {
		return false
	}
	for _, allowed := range c.AllowedExternalUsers {
		if strings.TrimSpace(allowed) == externalUserID {
			return true
		}
	}
	return false
}

// RobotSessionConfig robot session isolation strategy
type RobotSessionConfig struct {
	StrictUserIdentity *bool `yaml:"strict_user_identity,omitempty" json:"strict_user_identity,omitempty"` // when true, only real user identifiers are allowed; session/group ID fallback is not permitted
}

// StrictUserIdentityEnabled returns whether strict user identity mode is enabled; defaults to true when not configured.
func (c RobotSessionConfig) StrictUserIdentityEnabled() bool {
	if c.StrictUserIdentity == nil {
		return true
	}
	return *c.StrictUserIdentity
}

// RobotWecomConfig WeComrobot configuration
type RobotWecomConfig struct {
	Enabled        bool                     `yaml:"enabled" json:"enabled"`
	Token          string                   `yaml:"token" json:"token"`                       // callback URL verification token
	EncodingAESKey string                   `yaml:"encoding_aes_key" json:"encoding_aes_key"` // EncodingAESKey
	CorpID         string                   `yaml:"corp_id" json:"corp_id"`                   // enterprise ID
	Secret         string                   `yaml:"secret" json:"secret"`                     // application secret
	AgentID        int64                    `yaml:"agent_id" json:"agent_id"`                 // application AgentId
	Auth           RobotAuthorizationConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// ValidateWecomConfig validate WeCom robot configuration; when enabled, token must be configured, otherwise callbacks cannot prevent forgery.
func ValidateWecomConfig(w RobotWecomConfig) error {
	if !w.Enabled {
		return nil
	}
	if strings.TrimSpace(w.Token) == "" {
		return fmt.Errorf("robots.wecom.token must be configured when robots.wecom.enabled is true")
	}
	return nil
}

// RobotDingtalkConfig DingTalkrobot configuration
type RobotDingtalkConfig struct {
	Enabled                     bool                     `yaml:"enabled" json:"enabled"`
	ClientID                    string                   `yaml:"client_id" json:"client_id"`                                           // application Key (AppKey)
	ClientSecret                string                   `yaml:"client_secret" json:"client_secret"`                                   // application secret
	AllowConversationIDFallback bool                     `yaml:"allow_conversation_id_fallback" json:"allow_conversation_id_fallback"` // whether to allow fallback to session ID when sender_id is missing
	Auth                        RobotAuthorizationConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// RobotLarkConfig Feishurobot configuration
type RobotLarkConfig struct {
	Enabled             bool                     `yaml:"enabled" json:"enabled"`
	AppID               string                   `yaml:"app_id" json:"app_id"`                                 // application App ID
	AppSecret           string                   `yaml:"app_secret" json:"app_secret"`                         // application App Secret
	VerifyToken         string                   `yaml:"verify_token" json:"verify_token"`                     // event subscription Verification Token (optional)
	AllowChatIDFallback bool                     `yaml:"allow_chat_id_fallback" json:"allow_chat_id_fallback"` // whether to allow fallback to chat_id when user ID is missing
	Auth                RobotAuthorizationConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// RobotTelegramConfig is the Telegram robot configuration (Bot API long polling)
type RobotTelegramConfig struct {
	Enabled            bool                     `yaml:"enabled" json:"enabled"`
	BotToken           string                   `yaml:"bot_token" json:"bot_token"`
	BotUsername        string                   `yaml:"bot_username,omitempty" json:"bot_username,omitempty"` // optional, for group chat @ recognition; if empty, getMe is called at startup
	AllowGroupMessages bool                     `yaml:"allow_group_messages" json:"allow_group_messages"`     // in group chats, only respond when @ mentioned
	UpdateOffset       int64                    `yaml:"update_offset,omitempty" json:"update_offset,omitempty"`
	Auth               RobotAuthorizationConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// RobotSlackConfig is the Slack robot configuration (Socket Mode, no public callback required)
type RobotSlackConfig struct {
	Enabled  bool                     `yaml:"enabled" json:"enabled"`
	BotToken string                   `yaml:"bot_token" json:"bot_token"` // xoxb-
	AppToken string                   `yaml:"app_token" json:"app_token"` // xapp- (connections:write)
	Auth     RobotAuthorizationConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// RobotDiscordConfig Discord robot configuration (Gateway WebSocket)
type RobotDiscordConfig struct {
	Enabled            bool                     `yaml:"enabled" json:"enabled"`
	BotToken           string                   `yaml:"bot_token" json:"bot_token"`
	AllowGuildMessages bool                     `yaml:"allow_guild_messages" json:"allow_guild_messages"` // in server channels, only respond when @ mentioned
	Auth               RobotAuthorizationConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// RobotQQConfig is the QQ robot configuration (QQ open platform WebSocket)
type RobotQQConfig struct {
	Enabled      bool                     `yaml:"enabled" json:"enabled"`
	AppID        string                   `yaml:"app_id" json:"app_id"`
	ClientSecret string                   `yaml:"client_secret" json:"client_secret"`
	Sandbox      bool                     `yaml:"sandbox" json:"sandbox"` // sandbox environment (pre-launch testing)
	Auth         RobotAuthorizationConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

func (c RobotsConfig) AuthorizationFor(platform string) RobotAuthorizationConfig {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "wechat":
		return c.Wechat.Auth
	case "wecom":
		return c.Wecom.Auth
	case "dingtalk":
		return c.Dingtalk.Auth
	case "lark":
		return c.Lark.Auth
	case "telegram":
		return c.Telegram.Auth
	case "slack":
		return c.Slack.Auth
	case "discord":
		return c.Discord.Auth
	case "qq":
		return c.QQ.Auth
	default:
		return RobotAuthorizationConfig{}
	}
}

func ValidateRobotAuthorization(c RobotAuthorizationConfig, path string) error {
	switch c.EffectiveMode() {
	case RobotAuthModeUserBinding:
		return nil
	case RobotAuthModeServiceAccount:
		serviceUserID := strings.TrimSpace(c.ServiceUserID)
		if serviceUserID == "" {
			return fmt.Errorf("%s.auth.service_user_id cannot be empty", path)
		}
		if len(c.AllowedExternalUsers) == 0 {
			return fmt.Errorf("%s.auth.allowed_external_users must configure at least one real sender", path)
		}
		seen := map[string]bool{}
		for _, userID := range c.AllowedExternalUsers {
			userID = strings.TrimSpace(userID)
			if userID == "" || userID == "*" {
				return fmt.Errorf("%s.auth.allowed_external_users does not allow null values or wildcards", path)
			}
			if seen[userID] {
				return fmt.Errorf("%s.auth.allowed_external_users contains duplicate users", path)
			}
			seen[userID] = true
		}
		return nil
	default:
		return fmt.Errorf("%s.auth.mode only supports user_binding or service_account", path)
	}
}

func ValidateRobotsAuthorization(c RobotsConfig) error {
	items := []struct {
		path string
		auth RobotAuthorizationConfig
	}{
		{"robots.wechat", c.Wechat.Auth}, {"robots.wecom", c.Wecom.Auth},
		{"robots.dingtalk", c.Dingtalk.Auth}, {"robots.lark", c.Lark.Auth},
		{"robots.telegram", c.Telegram.Auth}, {"robots.slack", c.Slack.Auth},
		{"robots.discord", c.Discord.Auth}, {"robots.qq", c.QQ.Auth},
	}
	for _, item := range items {
		if err := ValidateRobotAuthorization(item.auth, item.path); err != nil {
			return err
		}
	}
	return nil
}

func (c RobotsConfig) ServiceAccountUserIDs() map[string]string {
	out := map[string]string{}
	for _, platform := range []string{"wechat", "wecom", "dingtalk", "lark", "telegram", "slack", "discord", "qq"} {
		auth := c.AuthorizationFor(platform)
		if auth.EffectiveMode() == RobotAuthModeServiceAccount {
			out[platform] = strings.TrimSpace(auth.ServiceUserID)
		}
	}
	return out
}
