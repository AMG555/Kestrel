package config

import "strings"

// VisionConfig holds independent vision model and analyze_image tool parameters; when enabled, registers the MCP tool analyze_image.
type VisionConfig struct {
	Enabled         bool     `yaml:"enabled" json:"enabled"`
	APIKey          string   `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	BaseURL         string   `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	Model           string   `yaml:"model,omitempty" json:"model,omitempty"`
	Provider        string   `yaml:"provider,omitempty" json:"provider,omitempty"`
	TimeoutSeconds  int      `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty"`
	MaxImageBytes   int64    `yaml:"max_image_bytes,omitempty" json:"max_image_bytes,omitempty"`
	MaxDimension    int      `yaml:"max_dimension,omitempty" json:"max_dimension,omitempty"`
	JPEGQuality     int      `yaml:"jpeg_quality,omitempty" json:"jpeg_quality,omitempty"`
	MaxPayloadBytes          int64 `yaml:"max_payload_bytes,omitempty" json:"max_payload_bytes,omitempty"`
	SkipPreprocessBelowBytes int64 `yaml:"skip_preprocess_below_bytes,omitempty" json:"skip_preprocess_below_bytes,omitempty"` // 0=always compress; default: pass-through original if <2MB and long edge <=max_dimension
	Detail string `yaml:"detail,omitempty" json:"detail,omitempty"` // low | high | auto
}

func (v VisionConfig) TimeoutSecondsEffective() int {
	if v.TimeoutSeconds <= 0 {
		return 60
	}
	return v.TimeoutSeconds
}

func (v VisionConfig) MaxImageBytesEffective() int64 {
	if v.MaxImageBytes <= 0 {
		return 5 * 1024 * 1024
	}
	return v.MaxImageBytes
}

func (v VisionConfig) MaxDimensionEffective() int {
	if v.MaxDimension <= 0 {
		return 2048
	}
	return v.MaxDimension
}

func (v VisionConfig) JPEGQualityEffective() int {
	if v.JPEGQuality <= 0 || v.JPEGQuality > 100 {
		return 82
	}
	return v.JPEGQuality
}

func (v VisionConfig) MaxPayloadBytesEffective() int64 {
	if v.MaxPayloadBytes <= 0 {
		return 512 * 1024
	}
	return v.MaxPayloadBytes
}

// SkipPreprocessBelowBytesEffective returns the threshold below which the original image can be passed through directly if the long edge is <=max_dimension and <=max_payload; 0 means always compress.
func (v VisionConfig) SkipPreprocessBelowBytesEffective() int64 {
	if v.SkipPreprocessBelowBytes < 0 {
		return 0
	}
	return v.SkipPreprocessBelowBytes
}

func (v VisionConfig) DetailEffective() string {
	d := strings.ToLower(strings.TrimSpace(v.Detail))
	switch d {
	case "high", "low", "auto":
		return d
	default:
		return "low"
	}
}

// OpenAICfgEffective merges the main openai config with vision overrides, for use by VL ChatModel.
// When vision.api_key / base_url / provider are empty or omitted, the main (openai) fields are inherited; vision.model is required (validated by Ready).
func (v VisionConfig) OpenAICfgEffective(main OpenAIConfig) OpenAIConfig {
	out := main
	if k := strings.TrimSpace(v.APIKey); k != "" {
		out.APIKey = k
	}
	if u := strings.TrimSpace(v.BaseURL); u != "" {
		out.BaseURL = u
	}
	if m := strings.TrimSpace(v.Model); m != "" {
		out.Model = m
	}
	if p := strings.TrimSpace(v.Provider); p != "" {
		out.Provider = p
	}
	out.Reasoning.Mode = "off"
	return out
}

// Ready returns true when enabled and the model name is non-empty.
func (v VisionConfig) Ready() bool {
	return v.Enabled && strings.TrimSpace(v.Model) != ""
}
