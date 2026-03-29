package main

import (
	"fmt"
	"log"

	"karl/internal"
)

// loadConfig loads and initializes the configuration
func (k *KarlServer) loadConfig() error {
	log.Println("Loading configuration...")

	// Get config path from environment or use default
	configPath := internal.GetConfigPath()
	log.Printf("Using config file: %s", configPath)

	config, err := internal.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	k.mu.Lock()
	k.config = config
	k.mu.Unlock()

	// Start config watcher
	go func() { _ = internal.WatchConfig(configPath) }()

	log.Println("Configuration loaded successfully")

	// Ensure Unix Socket Listener is started here
	k.startUnixSocketListener()

	return nil
}



// startUnixSocketListener initializes the Unix socket listener
func (k *KarlServer) startUnixSocketListener() {
	k.mu.RLock()
	socketPath := k.config.Integration.RTPengineSocket
	k.mu.RUnlock()

	k.rtpSocket = internal.NewRTPengineSocketListener(socketPath)
	if err := k.rtpSocket.Start(); err != nil {
		log.Printf("Failed to start Unix socket listener: %v", err)
		return
	}

	log.Printf("Unix socket listener started on %s", socketPath)
}
