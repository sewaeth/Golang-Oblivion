package bot

import (
	"sync"
)

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
