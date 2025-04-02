package main

import (
	"fmt"
	"log"
	"net/http"

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
	go internal.WatchConfig("config/config.json")

	log.Println("✅ Configuration loaded successfully")

	// Ensure API Server and Unix Socket Listener are started here
	k.startAPIServer()
	k.startUnixSocketListener()

	return nil
}

// startAPIServer initializes the HTTP API server
func (k *KarlServer) startAPIServer() {
	log.Println("🌐 Starting API server on :9091")

	// Set up API routes
	mux := internal.SetupRoutes()

	// Start HTTP server
	go func() {
		if err := http.ListenAndServe(":9091", mux); err != nil {
			log.Printf("❌ API server error: %v", err)
		}
	}()
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
