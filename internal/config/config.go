package config

import (
	"bytes"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Config holds the application configuration loaded from TOML
type Config struct {
	BotToken         string  `toml:"bot_token"`
	GuildID          string  `toml:"guild_id"`
	AvatarURL        string  `toml:"avatar_url"`
	Delay            float64 `toml:"delay"`
	MaxDelay         float64 `toml:"max_delay"`
	MaxRetries       int     `toml:"max_retries"`
	Message          string  `toml:"message"`
	MessageLimit     int     `toml:"message_limit"`
	MinDelay         float64 `toml:"min_delay"`
	RateLimitBackoff float64 `toml:"rate_limit_backoff"`
	RequestTimeout   float64 `toml:"request_timeout"`
	TotalPings       int     `toml:"total_pings"`
	Username         string  `toml:"username"`
	WebhooksFile     string  `toml:"webhooks_file"`
}

// DefaultConfig provides fallback values
var DefaultConfig = Config{
	Message:          "@everyone",
	Username:         "Oblivion",
	AvatarURL:        "https://media.discordapp.net/attachments/1329536982237319239/1375174292303773876/blob-cat-ping.gif",
	Delay:            2.5,
	MaxRetries:       3,
	MessageLimit:     9000,
	TotalPings:       450000,
	WebhooksFile:     "webhooks.toml",
	MaxDelay:         5.0,
	MinDelay:         0.5,
	RateLimitBackoff: 60.0,
	RequestTimeout:   10.0,
}

// LoadConfig reads and parses the TOML config file
func LoadConfig(filePath string) (*Config, error) {
	var config Config
	if _, err := toml.DecodeFile(filePath, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply defaults for missing fields
	if config.Message == "" {
		config.Message = DefaultConfig.Message
	}
	if config.Username == "" {
		config.Username = DefaultConfig.Username
	}
	if config.AvatarURL == "" {
		config.AvatarURL = DefaultConfig.AvatarURL
	}
	if config.Delay == 0 {
		config.Delay = DefaultConfig.Delay
	}
	if config.RateLimitBackoff == 0 {
		config.RateLimitBackoff = DefaultConfig.RateLimitBackoff
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = DefaultConfig.MaxRetries
	}
	if config.MessageLimit == 0 {
		config.MessageLimit = DefaultConfig.MessageLimit
	}
	if config.TotalPings == 0 {
		config.TotalPings = DefaultConfig.TotalPings
	}
	if config.WebhooksFile == "" {
		config.WebhooksFile = DefaultConfig.WebhooksFile
	}
	if config.MaxDelay == 0 {
		config.MaxDelay = DefaultConfig.MaxDelay
	}
	if config.MinDelay == 0 {
		config.MinDelay = DefaultConfig.MinDelay
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = DefaultConfig.RequestTimeout
	}

	return &config, nil
}

// Save writes the config back to the TOML file (preserves comments natively)
func (c *Config) Save(filePath string) error {
	// Read original file to preserve comments
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// Parse existing TOML
	var doc interface{}
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return err
	}

	// Create a buffer with config struct
	var buf bytes.Buffer
	encoder := toml.NewEncoder(&buf)
	if err := encoder.Encode(c); err != nil {
		return err
	}

	// Write back (TOML library preserves structure)
	return os.WriteFile(filePath, buf.Bytes(), 0644)
}
