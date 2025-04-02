package main

import (
	"log"
	"os"
)

// ensureRunDir ensures that the run directory exists with correct permissions
func ensureRunDir() error {
	// For testing, use a local directory
	runDir := "./run/karl"
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

	// Start the server (loads config, initializes services)
	if err := server.Start(); err != nil {
		log.Fatalf("❌ Error starting server: %v", err)
	}

	log.Println("✅ Karl Media server started successfully")

	// Keep the service running until shutdown
	server.WaitForShutdown()
	log.Println("🛑 Karl Media Server has been shut down.")
}
