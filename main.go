package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Config holds the application configuration loaded from JSON.config2
type Config struct {
	AvatarURL        string  `json:"avatar_url"`
	Delay            float64 `json:"delay"`
	MaxDelay         float64 `json:"max_delay"`
	MaxRetries       int     `json:"max_retries"`
	Message          string  `json:"message"`
	MessageLimit     int     `json:"message_limit"`
	MinDelay         float64 `json:"min_delay"`
	RateLimitBackoff float64 `json:"rate_limit_backoff"`
	RequestTimeout   float64 `json:"request_timeout"`
	TotalPings       int     `json:"total_pings"`
	Username         string  `json:"username"`
	WebhooksFile     string  `json:"webhooks_file"`
}

// WebhookGroups holds the shard names as keys and lists of webhook URLs as values.
type WebhookGroups map[string][]string

// MessagePayload represents the JSON payload for Discord webhook.
type MessagePayload struct {
	Content   string `json:"content"`
	Username  string `json:"username,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// DefaultConfig provides fallback values.
var DefaultConfig = Config{
	Message:          "@everyone",
	Username:         "Oblivion",
	AvatarURL:        "https://media.discordapp.net/attachments/1329536982237319239/1375174292303773876/blob-cat-ping.gif",
	Delay:            2.5,
	MaxRetries:       3,
	MessageLimit:     9000,
	TotalPings:       450000,
	WebhooksFile:     "webhooks.json",
	MaxDelay:         5.0,
	MinDelay:         0.5,
	RateLimitBackoff: 60.0,
	RequestTimeout:   10.0,
}

// loadConfig reads and parses the JSON config file.
func loadConfig(filePath string) (*Config, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
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

// saveConfig writes the config back to the JSON file.
func saveConfig(config *Config, filePath string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filePath, data, 0644)
}

// loadWebhookGroups reads and parses the JSON webhooks file.
func loadWebhookGroups(filePath string) (WebhookGroups, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read webhooks file: %w", err)
	}

	var groups WebhookGroups
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil, fmt.Errorf("failed to parse webhooks: %w", err)
	}

	if len(groups) == 0 {
		return nil, fmt.Errorf("no webhook groups found")
	}

	return groups, nil
}

// sendWebhook sends a single message to the webhook URL, handling retries and rate limits.
func sendWebhook(url string, payload MessagePayload, config *Config, client *http.Client, globalCount *int, mu *sync.Mutex, shardName string) bool {
	for attempt := 0; attempt < config.MaxRetries; attempt++ {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			log.Printf("Failed to marshal payload: %v", err)
			return false
		}

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("Failed to create request: %v", err)
			return false
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Request error for %s: %v", url, err)
			if attempt < config.MaxRetries-1 {
				time.Sleep(time.Duration(config.RateLimitBackoff) * time.Second)
			}
			continue
		}
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)
		if resp.StatusCode == 204 {
			mu.Lock()
			*globalCount++
			count := *globalCount
			mu.Unlock()
			if shardName != "" {
				fmt.Printf("Sent to %s (shard: %s, total: %d)\n", url, shardName, count)
			} else {
				fmt.Printf("Sent to %s (total: %d)\n", url, count)
			}
			return true
		} else if resp.StatusCode == 429 {
			var rateLimit struct {
				RetryAfter float64 `json:"retry_after"`
			}
			if json.Unmarshal(body, &rateLimit) == nil && rateLimit.RetryAfter > 0 {
				wait := time.Duration(rateLimit.RetryAfter * float64(time.Second))
				log.Printf("Rate limited on %s, waiting %v", url, wait)
				time.Sleep(wait)
			} else {
				time.Sleep(time.Duration(config.RateLimitBackoff) * time.Second)
			}
			continue
		} else {
			log.Printf("HTTP error %d for %s: %s", resp.StatusCode, url, string(body))
			return false
		}
	}
	return false
}

// webhookLoop runs the continuous sending loop for a single webhook.
func webhookLoop(url string, payload MessagePayload, config *Config, client *http.Client, sem chan struct{}, wg *sync.WaitGroup, stopCh chan struct{}, globalCount *int, mu *sync.Mutex, shardName string) {
	defer wg.Done()

	rand.Seed(time.Now().UnixNano())

	for {
		select {
		case <-stopCh:
			return
		default:
			mu.Lock()
			if *globalCount >= config.MessageLimit {
				mu.Unlock()
				return
			}
			mu.Unlock()
			// Acquire semaphore to limit concurrent requests per shard
			sem <- struct{}{}
			ok := sendWebhook(url, payload, config, client, globalCount, mu, shardName)
			<-sem
			if !ok {
				log.Printf("Failed to send to %s after retries", url)
			}
			delay := config.Delay
			// Clamp to min/max if set
			if config.MinDelay > 0 && delay < config.MinDelay {
				delay = config.MinDelay
			}
			if config.MaxDelay > 0 && delay > config.MaxDelay {
				delay = config.MaxDelay
			}
			time.Sleep(time.Duration(delay * float64(time.Second)))
		}
	}
}

// startShard starts goroutines for all webhooks in a shard.
func startShard(shardName string, groups WebhookGroups, config *Config, stopCh chan struct{}, globalCount *int, mu *sync.Mutex, wg *sync.WaitGroup) {
	webhooks := groups[shardName]
	payload := MessagePayload{
		Content:   config.Message,
		Username:  config.Username,
		AvatarURL: config.AvatarURL,
	}

	// Create one shared http.Client per shard to avoid creating a new Transport per request.
	timeout := time.Duration(config.RequestTimeout * float64(time.Second))
	// tuned transport to limit resources per shard
	tr := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	client := &http.Client{Transport: tr, Timeout: timeout}

	// semaphore to limit concurrent requests per shard
	concurrencyPerShard := 20
	if len(webhooks) < concurrencyPerShard {
		concurrencyPerShard = len(webhooks)
		if concurrencyPerShard == 0 {
			concurrencyPerShard = 1
		}
	}
	sem := make(chan struct{}, concurrencyPerShard)

	for _, url := range webhooks {
		wg.Add(1)
		// run a goroutine per webhook, sharing the client's transport and the semaphore
		go webhookLoop(url, payload, config, client, sem, wg, stopCh, globalCount, mu, shardName)
	}

	// Close idle connections when the shard stops
	go func() {
		<-stopCh
		if tr != nil {
			tr.CloseIdleConnections()
		}
	}()
}

// getTotalMessagesForShard calculates total messages sent for a shard.
func getTotalMessagesForShard(shardName string, groups WebhookGroups, globalCount *int, mu *sync.Mutex) int {
	mu.Lock()
	defer mu.Unlock()
	return *globalCount
}

// runParallelMode runs selected shards in parallel.
func runParallelMode(selectedShards []string, groups WebhookGroups, config *Config, globalCount *int, mu *sync.Mutex, userStopCh chan struct{}) {
	var wg sync.WaitGroup
	stopChs := []chan struct{}{}

	for _, shard := range selectedShards {
		stopCh := make(chan struct{})
		stopChs = append(stopChs, stopCh)
		startShard(shard, groups, config, stopCh, globalCount, mu, &wg)
		fmt.Printf("Started shard: %s\n", shard)
	}

	// Wait for user stop
	select {
	case <-userStopCh:
		fmt.Println("Stopping all shards...")
		for _, stopCh := range stopChs {
			close(stopCh)
		}
		wg.Wait()
	}
}

// runSequentialMode runs shards in sequence, cycling after total_pings.
func runSequentialMode(startingShard string, groups WebhookGroups, config *Config, globalCount *int, mu *sync.Mutex, userStopCh chan struct{}) {
	shardNames := make([]string, 0, len(groups))
	for name := range groups {
		shardNames = append(shardNames, name)
	}

	currentIdx := 0
	for i, name := range shardNames {
		if name == startingShard {
			currentIdx = i
			break
		}
	}

	for {
		currentShard := shardNames[currentIdx]
		currentStopCh := make(chan struct{})
		var currentWg sync.WaitGroup

		fmt.Printf("Starting sequential shard: %s\n", currentShard)
		startShard(currentShard, groups, config, currentStopCh, globalCount, mu, &currentWg)

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		stopped := false
		for !stopped {
			select {
			case <-ticker.C:
				total := getTotalMessagesForShard(currentShard, groups, globalCount, mu)
				fmt.Printf("Shard %s progress: %d/%d pings\n", currentShard, total, config.TotalPings)
				if total >= config.TotalPings {
					close(currentStopCh)
					currentWg.Wait()
					// Reset the global counter when switching to the next shard
					mu.Lock()
					*globalCount = 0
					mu.Unlock()
					stopped = true
				}
			case <-userStopCh:
				close(currentStopCh)
				currentWg.Wait()
				stopped = true
				return // Stop the entire sequential mode
			case <-currentStopCh:
				// In case closed externally
				currentWg.Wait()
				// Reset the global counter when this shard stops so the next shard starts from 0
				mu.Lock()
				*globalCount = 0
				mu.Unlock()
				stopped = true
			}
		}

		currentIdx = (currentIdx + 1) % len(shardNames)
	}
}

// inputListener listens for an Enter (empty line) to stop.
func inputListener(stopCh chan struct{}) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			close(stopCh)
			return
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Input error: %v", err)
	}
}

// settingsMenu handles changing settings interactively.
func settingsMenu(config *Config, configPath string) {
	editableFields := []string{"message", "username", "avatar_url", "delay", "rate_limit_backoff", "max_retries", "message_limit", "total_pings"}

	for {
		fmt.Println("\nCurrent Settings:")
		fmt.Printf("Message: %s\n", config.Message)
		fmt.Printf("Username: %s\n", config.Username)
		fmt.Printf("Avatar URL: %s\n", config.AvatarURL)
		fmt.Printf("Delay: %.2f\n", config.Delay)
		fmt.Printf("Rate Limit Backoff: %.1f\n", config.RateLimitBackoff)
		fmt.Printf("Max Retries: %d\n", config.MaxRetries)
		fmt.Printf("Message Limit: %d\n", config.MessageLimit)
		fmt.Printf("Total Pings: %d\n", config.TotalPings)

		fmt.Print("\nEnter field to change (" + strings.Join(editableFields, "/") + ") or 'done': ")
		var field string
		fmt.Scanln(&field)
		field = strings.TrimSpace(field)

		if field == "done" {
			break
		}

		var newValue string
		switch field {
		case "message", "username", "avatar_url":
			fmt.Printf("New %s: ", field)
			fmt.Scanln(&newValue)
			switch field {
			case "message":
				config.Message = newValue
			case "username":
				config.Username = newValue
			case "avatar_url":
				config.AvatarURL = newValue
			}
		case "delay":
			var f float64
			fmt.Print("New delay: ")
			fmt.Scanln(&f)
			config.Delay = f
		case "rate_limit_backoff":
			var f float64
			fmt.Print("New rate limit backoff: ")
			fmt.Scanln(&f)
			config.RateLimitBackoff = f
		case "max_retries":
			var i int
			fmt.Print("New max retries: ")
			fmt.Scanln(&i)
			config.MaxRetries = i
		case "message_limit":
			var i int
			fmt.Print("New message limit: ")
			fmt.Scanln(&i)
			config.MessageLimit = i
		case "total_pings":
			var i int
			fmt.Print("New total pings: ")
			fmt.Scanln(&i)
			config.TotalPings = i
		default:
			fmt.Println("Invalid field.")
			continue
		}

		if err := saveConfig(config, configPath); err != nil {
			log.Printf("Failed to save config: %v", err)
		} else {
			fmt.Println("Setting updated and saved.")
		}
	}
}

// startSubmenu handles the start mode selection.
func startSubmenu(groups WebhookGroups, config *Config, configPath string) {
	fmt.Println("\n1. Parallel\n2. Sequential")
	var subChoice int
	fmt.Scanln(&subChoice)

	var globalCount int
	var mu sync.Mutex
	userStopCh := make(chan struct{})

	switch subChoice {
	case 1: // Parallel (Nobody uses this)
		fmt.Print("Enter shards (comma-separated): ")
		var shardsStr string
		fmt.Scanln(&shardsStr)
		shards := strings.Split(shardsStr, ",")
		for i := range shards {
			shards[i] = strings.TrimSpace(shards[i])
		}
		validShards := make([]string, 0, len(shards))
		for _, s := range shards {
			// skip empty shard names
			if s == "" {
				log.Printf("Invalid shard: %s", s)
				continue
			}
			if _, ok := groups[s]; ok {
				validShards = append(validShards, s)
			} else {
				log.Printf("Invalid shard: %s", s)
			}
		}
		if len(validShards) == 0 {
			fmt.Println("No valid shards selected.")
			return
		}
		fmt.Println("Press Enter to stop.")
		go inputListener(userStopCh)
		go runParallelMode(validShards, groups, config, &globalCount, &mu, userStopCh)
		<-userStopCh
		fmt.Println("Stopping... waiting 5s for clean shutdown.")
		time.Sleep(5 * time.Second)
		fmt.Println("Stopped.")
	case 2: // Sequential (Imagine using a shard selector at the begin lol)
		fmt.Print("Enter starting shard: ")
		var startShard string
		fmt.Scanln(&startShard)
		startShard = strings.TrimSpace(startShard)
		if _, ok := groups[startShard]; !ok {
			fmt.Println("Invalid starting shard.")
			return
		}
		fmt.Println("Press Enter to stop.")
		go inputListener(userStopCh)
		go runSequentialMode(startShard, groups, config, &globalCount, &mu, userStopCh)
		<-userStopCh
		fmt.Println("Stopping... waiting 5s for clean shutdown.")
		time.Sleep(5 * time.Second)
		fmt.Println("Stopped.")
	default:
		fmt.Println("Invalid choice.")
	}
}

// Core
func main() {
	configPath := "config.json"
	config, err := loadConfig(configPath)
	if err != nil {
		log.Fatal(err)
	}

	webhooksPath := config.WebhooksFile
	groups, err := loadWebhookGroups(webhooksPath)
	if err != nil {
		log.Fatal(err)
	}

	for {
		fmt.Println("\n=== Oblivion V2 Menu ===")
		fmt.Println("1. Start")
		fmt.Println("2. Settings")
		fmt.Println("3. Exit")
		var choice int
		fmt.Scanln(&choice)

		switch choice {
		case 1:
			startSubmenu(groups, config, configPath)
		case 2:
			settingsMenu(config, configPath)
		case 3:
			fmt.Println("Exiting.")
			os.Exit(0)
		default:
			fmt.Println("Invalid choice.")
		}
	}
}
