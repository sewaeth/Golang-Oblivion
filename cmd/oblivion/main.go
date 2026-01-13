package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/seweath/Golang-Oblivion/internal/bot"
	"github.com/seweath/Golang-Oblivion/internal/config"
	"github.com/seweath/Golang-Oblivion/internal/webhook"
)

func main() {
	// Initialize structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting Golang-Oblivion Discord Bot", "version", "2.0")

	// Adjust config path to be relative to where the binary is run, or look in current directory
	// For standard usage, valid config should be in the working directory
	configPath := "config.toml"
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		slog.Error("Failed to load configuration", "error", err, "path", configPath)
		os.Exit(1)
	}

	// Load bot token from environment variable (preferred) or config file (fallback)
	botToken := os.Getenv("OBLIVION_BOT_TOKEN")
	if botToken == "" {
		botToken = cfg.BotToken
	}

	// Validate bot token
	if botToken == "" || botToken == "YOUR_BOT_TOKEN_HERE" {
		slog.Error("Invalid bot token", "message", "Please set OBLIVION_BOT_TOKEN environment variable or update config.toml")
		os.Exit(1)
	}

	// Load webhook groups
	webhooksPath := cfg.WebhooksFile
	groups, err := webhook.LoadWebhookGroups(webhooksPath)
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
	handler := bot.NewHandler(cfg, configPath, groups)

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
	err = bot.RegisterCommands(dg, cfg.GuildID)
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
	err = bot.UnregisterCommands(dg, cfg.GuildID)
	if err != nil {
		slog.Warn("Failed to unregister commands", "error", err)
	}

	slog.Info("Shutdown complete. Goodbye!")
}
