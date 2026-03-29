package main

import (
	"log"
	"os"
)

// getRunDir returns the run directory from environment or default
func getRunDir() string {
	if dir := os.Getenv("KARL_RUN_DIR"); dir != "" {
		return dir
	}
	return "./run/karl"
}

// ensureRunDir ensures that the run directory exists with correct permissions
func ensureRunDir() error {
	runDir := getRunDir()
	if _, err := os.Stat(runDir); os.IsNotExist(err) {
		log.Printf("Directory %s does not exist, creating...", runDir)
		if err := os.MkdirAll(runDir, 0775); err != nil {
			return err
		}
		log.Printf("Created directory: %s", runDir)
	}
	return nil
}

func main() {
	log.Println("Starting Karl RTP Engine...")

	// Ensure run directory exists before starting
	if err := ensureRunDir(); err != nil {
		log.Fatalf("Failed to create run directory: %v", err)
	}

	// Initialize Karl server
	server := NewKarlServer()

	// Start the server (loads config, initializes services)
	if err := server.Start(); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}

	log.Println("Karl Media Server started successfully")

	// Keep the service running until shutdown
	server.WaitForShutdown()
	log.Println("Karl Media Server has been shut down.")
}
