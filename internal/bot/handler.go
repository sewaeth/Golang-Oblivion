package bot

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/seweath/Golang-Oblivion/internal/config"
	"github.com/seweath/Golang-Oblivion/internal/webhook"
)

// Handler manages command interactions
type Handler struct {
	config     *config.Config
	configPath string
	state      *BotState
	groups     webhook.WebhookGroups
}

// NewHandler creates a new command handler
func NewHandler(cfg *config.Config, configPath string, groups webhook.WebhookGroups) *Handler {
	return &Handler{
		config:     cfg,
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
			webhook.RunParallelMode(shards, h.groups, h.config, &globalCount, &mu, h.state.GetStopChannel())
		} else {
			webhook.RunSequentialMode(startingShard, h.groups, h.config, &globalCount, &mu, h.state.GetStopChannel())
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
	return h.config.Save(h.configPath)
}
