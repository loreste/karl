package main

import (
	"fmt"
	"log"
	"os"
)

// Set at build time via: go build -ldflags "-X main.version=1.0.0 -X main.commit=$(git rev-parse --short HEAD) -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
var (
	version   = "0.1.0"
	commit    = "dev"
	buildDate = "unknown"
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
	// Allow overriding version info via environment (for containers/CI)
	if v := os.Getenv("KARL_VERSION"); v != "" {
		version = v
	}
	if c := os.Getenv("KARL_COMMIT"); c != "" {
		commit = c
	}

	// Handle --version / -v flag
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("karl %s (commit: %s, built: %s)\n", version, commit, buildDate)
		return
	}

	log.Printf("Starting Karl Media Server %s (commit: %s)", version, commit)

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
