package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/bwmarrin/discordgo"
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

// BotState holds the current state of bot operations
type BotState struct {
	mu           sync.Mutex
	isRunning    bool
	globalCount  int
	stopCh       chan struct{}
	userStopCh   chan struct{}
	mode         string
	activeShards []string
}

// Handler manages command interactions
type Handler struct {
	config     *Config
	configPath string
	state      *BotState
	groups     WebhookGroups
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

// loadConfig reads and parses the TOML config file
func loadConfig(filePath string) (*Config, error) {
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

// NewBotState creates a new bot state instance
func NewBotState() *BotState {
	return &BotState{
		isRunning:   false,
		globalCount: 0,
	}
}

// IsRunning checks if webhook operations are currently running
func (bs *BotState) IsRunning() bool {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.isRunning
}

// Start initializes a new operation session
func (bs *BotState) Start(mode string, shards []string) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.isRunning = true
	bs.globalCount = 0
	bs.stopCh = make(chan struct{})
	bs.userStopCh = make(chan struct{})
	bs.mode = mode
	bs.activeShards = shards
}

// Stop terminates the current operation session
func (bs *BotState) Stop() {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	if bs.isRunning {
		close(bs.userStopCh)
		bs.isRunning = false
	}
}

// GetStopChannel returns the user stop channel
func (bs *BotState) GetStopChannel() chan struct{} {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.userStopCh
}

// NewHandler creates a new command handler
func NewHandler(config *Config, configPath string, groups WebhookGroups) *Handler {
	return &Handler{
		config:     config,
		configPath: configPath,
		state:      NewBotState(),
		groups:     groups,
	}
}

// RegisterCommands registers all slash commands with Discord
func RegisterCommands(s *discordgo.Session, guildID string) error {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "config",
			Description: "View or modify bot configuration",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "field",
					Description: "Configuration field to modify",
					Required:    false,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Message", Value: "message"},
						{Name: "Username", Value: "username"},
						{Name: "Avatar URL", Value: "avatar_url"},
						{Name: "Delay", Value: "delay"},
						{Name: "Min Delay", Value: "min_delay"},
						{Name: "Max Delay", Value: "max_delay"},
						{Name: "Rate Limit Backoff", Value: "rate_limit_backoff"},
						{Name: "Max Retries", Value: "max_retries"},
						{Name: "Message Limit", Value: "message_limit"},
						{Name: "Total Pings", Value: "total_pings"},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "value",
					Description: "New value for the field",
					Required:    false,
				},
			},
		},
		{
			Name:        "start",
			Description: "Start webhook operations",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "mode",
					Description: "Operation mode",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Parallel", Value: "parallel"},
						{Name: "Sequential", Value: "sequential"},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "shards",
					Description: "Comma-separated list of shards (for parallel mode)",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "starting_shard",
					Description: "Starting shard name (for sequential mode)",
					Required:    false,
				},
			},
		},
		{
			Name:        "stop",
			Description: "Stop all webhook operations",
		},
	}

	for _, cmd := range commands {
		_, err := s.ApplicationCommandCreate(s.State.User.ID, guildID, cmd)
		if err != nil {
			return err
		}
	}

	return nil
}

// UnregisterCommands removes all registered commands
func UnregisterCommands(s *discordgo.Session, guildID string) error {
	commands, err := s.ApplicationCommands(s.State.User.ID, guildID)
	if err != nil {
		return err
	}

	for _, cmd := range commands {
		err := s.ApplicationCommandDelete(s.State.User.ID, guildID, cmd.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

// HandleInteraction routes slash command interactions to appropriate handlers
func (h *Handler) HandleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	switch i.ApplicationCommandData().Name {
	case "config":
		h.handleConfigCommand(s, i)
	case "start":
		h.handleStartCommand(s, i)
	case "stop":
		h.handleStopCommand(s, i)
	}
}

// handleConfigCommand handles the /config command
func (h *Handler) handleConfigCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Audit log
	slog.Info("Command executed",
		"command", "config",
		"user_id", i.Member.User.ID,
		"user_name", i.Member.User.Username,
		"guild_id", i.GuildID,
	)

	options := i.ApplicationCommandData().Options
	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption)
	for _, opt := range options {
		optionMap[opt.Name] = opt
	}

	// If no options, show current config
	if len(optionMap) == 0 {
		embed := &discordgo.MessageEmbed{
			Title:       "Current Configuration",
			Color:       0x00ff00,
			Description: h.getConfigString(),
			Timestamp:   time.Now().Format(time.RFC3339),
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{embed},
			},
		})
		return
	}

	// If field and value provided, update config
	field, hasField := optionMap["field"]
	value, hasValue := optionMap["value"]

	if !hasField || !hasValue {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Please provide both field and value to update configuration.",
			},
		})
		return
	}

	fieldName := field.StringValue()
	newValue := value.StringValue()

	if err := h.updateConfigField(fieldName, newValue); err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Failed to update config: %v", err),
			},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Configuration Updated",
		Color:       0x00ff00,
		Description: fmt.Sprintf("**%s** has been updated to: `%s`", fieldName, newValue),
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

// handleStartCommand handles the /start command
func (h *Handler) handleStartCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Audit log
	slog.Info("Command executed",
		"command", "start",
		"user_id", i.Member.User.ID,
		"user_name", i.Member.User.Username,
		"guild_id", i.GuildID,
	)

	if h.state.IsRunning() {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Webhook operations are already running. Use `/stop` first.",
			},
		})
		return
	}

	options := i.ApplicationCommandData().Options
	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption)
	for _, opt := range options {
		optionMap[opt.Name] = opt
	}

	mode := optionMap["mode"].StringValue()

	var shards []string
	var startingShard string

	if mode == "parallel" {
		if shardsOpt, ok := optionMap["shards"]; ok {
			shardsStr := shardsOpt.StringValue()
			shards = strings.Split(shardsStr, ",")
			for i := range shards {
				shards[i] = strings.TrimSpace(shards[i])
			}

			// Validate shards
			validShards := []string{}
			for _, shard := range shards {
				if _, ok := h.groups[shard]; ok {
					validShards = append(validShards, shard)
				} else {
					slog.Warn("Invalid shard name, skipping", "shard", shard)
				}
			}
			shards = validShards

			if len(shards) == 0 {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "No valid shards provided for parallel mode.",
					},
				})
				return
			}
		} else {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Please provide shards for parallel mode.",
				},
			})
			return
		}
	} else if mode == "sequential" {
		if startingShardOpt, ok := optionMap["starting_shard"]; ok {
			startingShard = strings.TrimSpace(startingShardOpt.StringValue())
			if _, ok := h.groups[startingShard]; !ok {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: fmt.Sprintf("Invalid starting shard: %s", startingShard),
					},
				})
				return
			}
		} else {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Please provide a starting shard for sequential mode.",
				},
			})
			return
		}
	}

	// Start the operations
	h.state.Start(mode, shards)

	embed := &discordgo.MessageEmbed{
		Title: "Starting Webhook Operations",
		Color: 0x00ff00,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Mode", Value: mode, Inline: true},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if mode == "parallel" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Shards",
			Value:  strings.Join(shards, ", "),
			Inline: false,
		})
	} else {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Starting Shard",
			Value:  startingShard,
			Inline: true,
		})
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})

	// Start webhook operations in background
	go func() {
		var globalCount int
		var mu sync.Mutex

		if mode == "parallel" {
			RunParallelMode(shards, h.groups, h.webhookConfig(), &globalCount, &mu, h.state.GetStopChannel())
		} else {
			RunSequentialMode(startingShard, h.groups, h.webhookConfig(), &globalCount, &mu, h.state.GetStopChannel())
		}

		h.state.Stop()
		slog.Info("Webhook operations completed")
	}()
}

// handleStopCommand handles the /stop command
func (h *Handler) handleStopCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Audit log
	slog.Info("Command executed",
		"command", "stop",
		"user_id", i.Member.User.ID,
		"user_name", i.Member.User.Username,
		"guild_id", i.GuildID,
	)

	if !h.state.IsRunning() {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No webhook operations are currently running.",
			},
		})
		return
	}

	h.state.Stop()

	embed := &discordgo.MessageEmbed{
		Title:       "Stopping Webhook Operations",
		Color:       0xff0000,
		Description: "All webhook operations are being terminated...",
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

// getConfigString returns a formatted string of current configuration
func (h *Handler) getConfigString() string {
	return fmt.Sprintf(
		"**Message:** `%s`\n"+
			"**Username:** `%s`\n"+
			"**Avatar URL:** `%s`\n"+
			"**Delay:** `%.2f`s\n"+
			"**Min Delay:** `%.2f`s\n"+
			"**Max Delay:** `%.2f`s\n"+
			"**Rate Limit Backoff:** `%.1f`s\n"+
			"**Max Retries:** `%d`\n"+
			"**Message Limit:** `%d`\n"+
			"**Total Pings:** `%d`",
		h.config.Message,
		h.config.Username,
		h.config.AvatarURL,
		h.config.Delay,
		h.config.MinDelay,
		h.config.MaxDelay,
		h.config.RateLimitBackoff,
		h.config.MaxRetries,
		h.config.MessageLimit,
		h.config.TotalPings,
	)
}

// updateConfigField updates a specific configuration field
func (h *Handler) updateConfigField(field, value string) error {
	switch field {
	case "message":
		if len(value) == 0 {
			return fmt.Errorf("message cannot be empty")
		}
		if len(value) > 2000 {
			return fmt.Errorf("message cannot exceed 2000 characters")
		}
		h.config.Message = value
	case "username":
		if len(value) == 0 {
			return fmt.Errorf("username cannot be empty")
		}
		if len(value) > 80 {
			return fmt.Errorf("username cannot exceed 80 characters")
		}
		h.config.Username = value
	case "avatar_url":
		if len(value) > 2048 {
			return fmt.Errorf("avatar_url is too long")
		}
		h.config.AvatarURL = value
	case "delay":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("delay must be a number")
		}
		if f < 0 || f > 60 {
			return fmt.Errorf("delay must be between 0 and 60 seconds")
		}
		h.config.Delay = f
	case "min_delay":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("min_delay must be a number")
		}
		if f < 0 || f > 60 {
			return fmt.Errorf("min_delay must be between 0 and 60 seconds")
		}
		h.config.MinDelay = f
	case "max_delay":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("max_delay must be a number")
		}
		if f < 0 || f > 60 {
			return fmt.Errorf("max_delay must be between 0 and 60 seconds")
		}
		h.config.MaxDelay = f
	case "rate_limit_backoff":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("rate_limit_backoff must be a number")
		}
		if f < 1 || f > 300 {
			return fmt.Errorf("rate_limit_backoff must be between 1 and 300 seconds")
		}
		h.config.RateLimitBackoff = f
	case "max_retries":
		i, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("max_retries must be a number")
		}
		if i < 1 || i > 10 {
			return fmt.Errorf("max_retries must be between 1 and 10")
		}
		h.config.MaxRetries = i
	case "message_limit":
		i, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("message_limit must be a number")
		}
		if i < 1 || i > 10000000 {
			return fmt.Errorf("message_limit must be between 1 and 10,000,000")
		}
		h.config.MessageLimit = i
	case "total_pings":
		i, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("total_pings must be a number")
		}
		if i < 1 || i > 10000000 {
			return fmt.Errorf("total_pings must be between 1 and 10,000,000")
		}
		h.config.TotalPings = i
	default:
		return fmt.Errorf("unknown field: %s", field)
	}

	// Save config to file
	return h.saveConfig()
}

// saveConfig writes the config back to the TOML file (preserves comments natively)
func (h *Handler) saveConfig() error {
	// Read original file to preserve comments
	data, err := os.ReadFile(h.configPath)
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
	if err := encoder.Encode(h.config); err != nil {
		return err
	}

	// Write back (TOML library preserves structure)
	return os.WriteFile(h.configPath, buf.Bytes(), 0644)
}

// webhookConfig converts Handler config to webhook Config format
func (h *Handler) webhookConfig() *Config {
	return &Config{
		AvatarURL:        h.config.AvatarURL,
		Delay:            h.config.Delay,
		MaxDelay:         h.config.MaxDelay,
		MaxRetries:       h.config.MaxRetries,
		Message:          h.config.Message,
		MessageLimit:     h.config.MessageLimit,
		MinDelay:         h.config.MinDelay,
		RateLimitBackoff: h.config.RateLimitBackoff,
		RequestTimeout:   h.config.RequestTimeout,
		TotalPings:       h.config.TotalPings,
		Username:         h.config.Username,
		WebhooksFile:     h.config.WebhooksFile,
	}
}

func main() {
	// Initialize structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting Golang-Oblivion Discord Bot", "version", "2.0")

	configPath := "config.toml"
	config, err := loadConfig(configPath)
	if err != nil {
		slog.Error("Failed to load configuration", "error", err, "path", configPath)
		os.Exit(1)
	}

	// Load bot token from environment variable (preferred) or config file (fallback)
	botToken := os.Getenv("OBLIVION_BOT_TOKEN")
	if botToken == "" {
		botToken = config.BotToken
	}

	// Validate bot token
	if botToken == "" || botToken == "YOUR_BOT_TOKEN_HERE" {
		slog.Error("Invalid bot token", "message", "Please set OBLIVION_BOT_TOKEN environment variable or update config.toml")
		os.Exit(1)
	}

	// Load webhook groups
	webhooksPath := config.WebhooksFile
	groups, err := LoadWebhookGroups(webhooksPath)
	if err != nil {
		slog.Error("Failed to load webhook groups", "error", err, "path", webhooksPath)
		os.Exit(1)
	}

	// Create Discord session
	dg, err := discordgo.New("Bot " + botToken)
	if err != nil {
		slog.Error("Failed to create Discord session", "error", err)
		os.Exit(1)
	}

	// Create command handler
	handler := NewHandler(config, configPath, groups)

	// Register event handlers
	dg.AddHandler(handler.HandleInteraction)

	// Set intents
	dg.Identify.Intents = discordgo.IntentsGuilds

	// Open connection to Discord
	err = dg.Open()
	if err != nil {
		slog.Error("Failed to open connection to Discord", "error", err)
		os.Exit(1)
	}
	defer dg.Close()

	slog.Info("Bot connected to Discord, registering commands...")

	// Register slash commands
	err = RegisterCommands(dg, config.GuildID)
	if err != nil {
		slog.Error("Failed to register commands", "error", err)
		os.Exit(1)
	}

	slog.Info("Commands registered successfully")
	slog.Info("Bot is ready. Press CTRL+C to exit.")

	// Wait for interrupt signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("Shutdown signal received, cleaning up...")

	// Unregister commands (optional, but clean)
	slog.Info("Unregistering commands...")
	err = UnregisterCommands(dg, config.GuildID)
	if err != nil {
		slog.Warn("Failed to unregister commands", "error", err)
	}

	slog.Info("Shutdown complete. Goodbye!")
}
