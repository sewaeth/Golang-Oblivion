package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

// WebhookGroups holds the shard names as keys and lists of webhook URLs as values
type WebhookGroups map[string][]string

// MessagePayload represents the JSON payload for Discord webhook
type MessagePayload struct {
	Content   string `json:"content"`
	Username  string `json:"username,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// LoadWebhookGroups reads and parses the TOML webhooks file
func LoadWebhookGroups(filePath string) (WebhookGroups, error) {
	var data map[string]map[string][]string

	if _, err := toml.DecodeFile(filePath, &data); err != nil {
		return nil, fmt.Errorf("failed to parse webhooks: %w", err)
	}

	// Convert to WebhookGroups format
	groups := make(WebhookGroups)
	for name, section := range data {
		if webhooks, ok := section["webhooks"]; ok {
			groups[name] = webhooks
		}
	}

	if len(groups) == 0 {
		return nil, fmt.Errorf("no webhook groups found")
	}

	return groups, nil
}

// sendWebhook sends a single message to the webhook URL, handling retries and rate limits
func sendWebhook(url string, payload MessagePayload, config *Config, client *http.Client, globalCount *int, mu *sync.Mutex, shardName string) bool {
	for attempt := 0; attempt < config.MaxRetries; attempt++ {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			slog.Error("Failed to marshal webhook payload", "error", err)
			return false
		}

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			slog.Error("Failed to create HTTP request", "error", err, "url", url)
			return false
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			slog.Warn("Webhook request failed", "error", err, "url", url, "attempt", attempt+1)
			if attempt < config.MaxRetries-1 {
				time.Sleep(time.Duration(config.RateLimitBackoff) * time.Second)
			}
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			slog.Error("Failed to read response body", "error", err, "url", url)
			return false
		}
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
				slog.Warn("Rate limited, waiting", "url", url, "wait_duration", wait)
				time.Sleep(wait)
			} else {
				time.Sleep(time.Duration(config.RateLimitBackoff) * time.Second)
			}
			continue
		} else {
			slog.Error("HTTP error from webhook", "status_code", resp.StatusCode, "url", url, "response", string(body))
			return false
		}
	}
	return false
}

// webhookLoop runs the continuous sending loop for a single webhook
func webhookLoop(url string, payload MessagePayload, config *Config, client *http.Client, sem chan struct{}, wg *sync.WaitGroup, stopCh chan struct{}, globalCount *int, mu *sync.Mutex, shardName string) {
	defer wg.Done()

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
				slog.Error("Failed to send webhook after all retries", "url", url)
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

// StartShard starts goroutines for all webhooks in a shard
func StartShard(shardName string, groups WebhookGroups, config *Config, stopCh chan struct{}, globalCount *int, mu *sync.Mutex, wg *sync.WaitGroup) {
	webhooks := groups[shardName]
	payload := MessagePayload{
		Content:   config.Message,
		Username:  config.Username,
		AvatarURL: config.AvatarURL,
	}

	// Create one shared http.Client per shard
	timeout := time.Duration(config.RequestTimeout * float64(time.Second))
	tr := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	client := &http.Client{Transport: tr, Timeout: timeout}

	// Semaphore to limit concurrent requests per shard
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

// RunParallelMode runs selected shards in parallel
func RunParallelMode(selectedShards []string, groups WebhookGroups, config *Config, globalCount *int, mu *sync.Mutex, userStopCh chan struct{}) {
	var wg sync.WaitGroup
	stopChs := []chan struct{}{}
	var closeOnce sync.Once

	for _, shard := range selectedShards {
		stopCh := make(chan struct{})
		stopChs = append(stopChs, stopCh)
		StartShard(shard, groups, config, stopCh, globalCount, mu, &wg)
		fmt.Printf("Started shard: %s\n", shard)
	}

	// Wait for user stop
	select {
	case <-userStopCh:
		fmt.Println("Stopping all shards...")
		closeOnce.Do(func() {
			for _, stopCh := range stopChs {
				close(stopCh)
			}
		})
		wg.Wait()
	}
}

// RunSequentialMode runs shards in sequence, cycling after total_pings
func RunSequentialMode(startingShard string, groups WebhookGroups, config *Config, globalCount *int, mu *sync.Mutex, userStopCh chan struct{}) {
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
		StartShard(currentShard, groups, config, currentStopCh, globalCount, mu, &currentWg)

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		stopped := false
		for !stopped {
			select {
			case <-ticker.C:
				mu.Lock()
				total := *globalCount
				mu.Unlock()
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
				currentWg.Wait()
				mu.Lock()
				*globalCount = 0
				mu.Unlock()
				stopped = true
			}
		}

		currentIdx = (currentIdx + 1) % len(shardNames)
	}
}
