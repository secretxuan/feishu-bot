// Package config provides configuration management for the Feishu bot.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Config represents the application configuration.
type Config struct {
	Feishu FeishuConfig `mapstructure:"feishu"`
	LLM    LLMConfig    `mapstructure:"llm"`
	Redis  RedisConfig  `mapstructure:"redis"`
	Bot    BotConfig    `mapstructure:"bot"`
}

// FeishuConfig holds Feishu (Lark) specific configuration.
type FeishuConfig struct {
	AppID             string `mapstructure:"app_id"`
	AppSecret         string `mapstructure:"app_secret"`
	EscalationGroupID string `mapstructure:"escalation_group_id"`
}

// LLMConfig holds LLM service configuration.
type LLMConfig struct {
	Provider   string `mapstructure:"provider"`
	APIKey     string `mapstructure:"api_key"`
	BaseURL    string `mapstructure:"base_url"`
	Model      string `mapstructure:"model"`
	MaxHistory int    `mapstructure:"max_history"`
}

// RedisConfig holds Redis connection configuration.
type RedisConfig struct {
	Addr       string `mapstructure:"addr"`
	Password   string `mapstructure:"password"`
	DB         int    `mapstructure:"db"`
	Expiration int    `mapstructure:"expiration"`
}

// BotConfig holds bot-specific configuration.
type BotConfig struct {
	EscalationKeywords   []string `mapstructure:"escalation_keywords"`
	ClearContextKeywords []string `mapstructure:"clear_context_keywords"`
	SessionTimeout       int      `mapstructure:"session_timeout"`
}

// Load loads the configuration from the specified config file.
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set config file path
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Read prompts file
	promptsPath := "configs/prompts.yaml"
	if _, err := os.Stat(promptsPath); err == nil {
		v.SetConfigFile(promptsPath)
		_ = v.MergeInConfig()
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Override with environment variables
	overrideFromEnv(&cfg)

	// Normalize strings
	cfg.Feishu.AppID = strings.TrimSpace(cfg.Feishu.AppID)
	cfg.Feishu.AppSecret = strings.TrimSpace(cfg.Feishu.AppSecret)
	cfg.Feishu.EscalationGroupID = strings.TrimSpace(cfg.Feishu.EscalationGroupID)
	cfg.LLM.APIKey = strings.TrimSpace(cfg.LLM.APIKey)
	cfg.LLM.BaseURL = strings.TrimSpace(cfg.LLM.BaseURL)
	cfg.LLM.Model = strings.TrimSpace(cfg.LLM.Model)
	cfg.LLM.Provider = strings.TrimSpace(cfg.LLM.Provider)

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// overrideFromEnv overrides config values from environment variables.
func overrideFromEnv(cfg *Config) {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		cfg.Redis.Addr = addr
	}
	if pw := os.Getenv("REDIS_PASSWORD"); pw != "" {
		cfg.Redis.Password = pw
	}
	if db := os.Getenv("REDIS_DB"); db != "" {
		fmt.Sscanf(db, "%d", &cfg.Redis.DB)
	}

	if appID := os.Getenv("FEISHU_APP_ID"); appID != "" {
		cfg.Feishu.AppID = appID
	}
	if secret := os.Getenv("FEISHU_APP_SECRET"); secret != "" {
		cfg.Feishu.AppSecret = secret
	}
	if groupID := os.Getenv("FEISHU_ESCALATION_GROUP_ID"); groupID != "" {
		cfg.Feishu.EscalationGroupID = groupID
	}

	if provider := os.Getenv("LLM_PROVIDER"); provider != "" {
		cfg.LLM.Provider = provider
	}
	if apiKey := os.Getenv("LLM_API_KEY"); apiKey != "" {
		cfg.LLM.APIKey = apiKey
	}
	if baseURL := os.Getenv("LLM_BASE_URL"); baseURL != "" {
		cfg.LLM.BaseURL = baseURL
	}
	if model := os.Getenv("LLM_MODEL"); model != "" {
		cfg.LLM.Model = model
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Feishu.AppID == "" {
		return fmt.Errorf("feishu.app_id is required")
	}
	if c.Feishu.AppSecret == "" {
		return fmt.Errorf("feishu.app_secret is required")
	}
	if c.Feishu.EscalationGroupID == "" {
		return fmt.Errorf("feishu.escalation_group_id is required")
	}
	if c.LLM.APIKey != "" {
		if c.LLM.BaseURL == "" {
			return fmt.Errorf("llm.base_url is required when llm.api_key is provided")
		}
		if c.LLM.Model == "" {
			return fmt.Errorf("llm.model is required when llm.api_key is provided")
		}
	}
	return nil
}

// IsEscalationKeyword checks if the given content contains an escalation keyword.
func (c *Config) IsEscalationKeyword(content string) bool {
	lowerContent := strings.ToLower(content)
	for _, keyword := range c.Bot.EscalationKeywords {
		if strings.Contains(lowerContent, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

// IsClearContextKeyword checks if the given content contains a clear context keyword.
func (c *Config) IsClearContextKeyword(content string) bool {
	lowerContent := strings.ToLower(content)
	for _, keyword := range c.Bot.ClearContextKeywords {
		if strings.Contains(lowerContent, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}
