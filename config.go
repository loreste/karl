package main

import (
	"fmt"
	"log"

	"karl/internal"
)

// loadConfig loads and initializes the configuration
func (k *KarlServer) loadConfig() error {
	log.Println("🛠 Loading configuration...")

	config, err := internal.LoadConfig("config/config.json")
	if err != nil {
		return fmt.Errorf("❌ Failed to load configuration: %w", err)
	}

	k.mu.Lock()
	k.config = config
	k.mu.Unlock()

	// Start config watcher
	go func() { _ = internal.WatchConfig("config/config.json") }()

	log.Println("✅ Configuration loaded successfully")

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
		log.Printf("❌ Failed to start Unix socket listener: %v", err)
		return
	}

	log.Printf("✅ Unix socket listener started on %s", socketPath)
}
