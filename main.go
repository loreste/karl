package main

import (
	"context"
	"karl/internal"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// ensureRunDir ensures that /var/run/karl exists with correct permissions
func ensureRunDir() error {
	runDir := "/var/run/karl"
	if _, err := os.Stat(runDir); os.IsNotExist(err) {
		log.Printf("📂 Directory %s does not exist, creating...", runDir)
		if err := os.MkdirAll(runDir, 0775); err != nil {
			return err
		}
		log.Printf("✅ Created directory: %s", runDir)
	}
	return nil
}

func main() {
	log.Println("🚀 Starting Karl RTP Engine...")

	// Ensure /var/run/karl exists before starting
	if err := ensureRunDir(); err != nil {
		log.Fatalf("❌ Failed to create /var/run/karl/: %v", err)
	}

	// Initialize Karl server
	server := NewKarlServer()

	// Load configuration
	if err := server.loadConfig(); err != nil {
		log.Fatalf("❌ Error loading config: %v", err)
	}

	// Set up signal handling for graceful shutdown
	server.setupSignalHandler()

	// Initialize Prometheus metrics AFTER configuration is loaded
	internal.InitMetrics()

	// Initialize PCAP capture AFTER configuration is loaded
	internal.InitPCAPCapture()

	// Initialize all services
	if err := server.initializeServices(); err != nil {
		log.Fatalf("❌ Error initializing services: %v", err)
	}

	log.Println("✅ Karl Media server started successfully")

	// Handle shutdown signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-signalChan
		log.Println("🛑 Received shutdown signal, stopping Karl...")
		server.Shutdown()
		cancel()
	}()

	// Keep the service running until shutdown
	<-ctx.Done()
	log.Println("🛑 Karl Media Server has been shut down.")
}
