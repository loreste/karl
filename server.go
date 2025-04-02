package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"karl/internal"

	"github.com/pion/webrtc/v3"
)

// KarlServer represents the main server instance
type KarlServer struct {
	config         *internal.Config
	rtpControl     *internal.RTPControl
	iceManager     *internal.ICEManager
	webrtcSession  *webrtc.PeerConnection
	webrtcStats    *internal.WebRTCStats
	srtpTranscoder *internal.SRTPTranscoder
	transcoder     *internal.RTPTranscoder
	rtpSocket      *internal.RTPengineSocketListener
	redisCache     *internal.RTPRedisCache
	database       *internal.RTPDatabase
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
	mu             sync.RWMutex
	isShuttingDown bool
	resources      *internal.ResourceGroup // For tracking all resources
	healthServer   *http.Server            // Health check server
}

// NewKarlServer creates and initializes a new KarlServer instance
func NewKarlServer() *KarlServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &KarlServer{
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start initializes and starts all server components
func (k *KarlServer) Start() error {
	startTime := time.Now()

	// Load configuration
	if err := k.loadConfig(); err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Set up signal handling
	k.setupSignalHandler()

	// Initialize metrics
	internal.InitMetrics()
	err := internal.StartMetricsServer(":9091")
	if err != nil {
		log.Printf("❌ Failed to start metrics server: %v", err)
	} else {
		log.Println("✅ Metrics initialized and server started")
	}

	// Connect worker pool metrics to media handler
	internal.WorkerMetricsGetter = internal.GetWorkerPoolMetrics

	// Initialize resource tracking
	k.resources = internal.NewResourceGroup()

	// Register health checks
	internal.RegisterDefaultHealthChecks()
	internal.StartHealthChecker(30 * time.Second)

	// Initialize all services
	if err := k.initializeServices(); err != nil {
		return fmt.Errorf("failed to initialize services: %w", err)
	}

	// Initialize and start health API
	startHealthAPI := func() {
		mux := http.NewServeMux()

		// Register health check endpoints
		mux.HandleFunc("/health", internal.SimpleHealthHandler())
		mux.HandleFunc("/health/detail", internal.HealthHandler())

		// Create health server with proper timeouts
		k.healthServer = &http.Server{
			Addr:         ":8086",
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		}

		// Start server in a goroutine
		go func() {
			log.Printf("🩺 Starting health check server on %s", k.healthServer.Addr)
			if err := k.healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("❌ Health check server error: %v", err)
			}
		}()

		// Add to resource group for proper cleanup
		k.resources.Add(&internal.HttpServerResource{Server: k.healthServer})
	}
	startHealthAPI()

	// Note: loadConfig() already starts the API server and Unix socket listener
	// SIP registration is initialized in initializeServices()

	// Record startup time
	startupDuration := time.Since(startTime)
	log.Printf("✅ Karl RTP Engine started successfully in %s", startupDuration)

	return nil
}

// setupSignalHandler sets up system signal handling for graceful shutdown
func (k *KarlServer) setupSignalHandler() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-signalChan
		k.mu.Lock()
		if k.isShuttingDown {
			k.mu.Unlock()
			return
		}
		k.isShuttingDown = true
		k.mu.Unlock()

		log.Println("🛑 Shutdown signal received")
		k.Shutdown()
	}()
}

// Shutdown performs a graceful shutdown of all server components
func (k *KarlServer) Shutdown() {
	log.Println("🔄 Starting graceful shutdown...")

	k.mu.Lock()
	if k.isShuttingDown {
		k.mu.Unlock()
		return
	}
	k.isShuttingDown = true
	k.mu.Unlock()

	// Cancel context to stop all operations
	k.cancel()

	k.mu.Lock()
	// Stop WebRTC stats monitoring
	if k.webrtcStats != nil {
		k.webrtcStats.StopMonitoring()
		k.webrtcStats = nil
	}

	// Clean up SRTP transcoder
	if k.srtpTranscoder != nil {
		k.srtpTranscoder.Context = nil // ✅ Reset context instead of calling Close()
		k.srtpTranscoder = nil
	}

	// Clean up RTP transcoder
	if k.transcoder != nil {
		k.transcoder.Close()
	}

	// Close WebRTC session
	if k.webrtcSession != nil {
		if err := k.webrtcSession.Close(); err != nil {
			log.Printf("⚠️ Error closing WebRTC session: %v", err)
		}
		k.webrtcSession = nil
	}

	// Stop RTP control
	if k.rtpControl != nil {
		k.rtpControl.Stop()
		k.rtpControl = nil
	}

	// Close database connections
	if k.database != nil {
		k.database.Close()
	}

	// Close Redis connections
	if k.redisCache != nil {
		k.redisCache.Close()
	}

	// Stop Unix socket listener
	if k.rtpSocket != nil {
		k.rtpSocket.Stop()
	}

	k.mu.Unlock()

	// Stop the worker pool
	internal.StopWorkerPool()

	// Wait with timeout for all goroutines to finish
	done := make(chan struct{})
	go func() {
		k.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("✅ All goroutines completed successfully")
	case <-time.After(5 * time.Second):
		log.Println("⚠️ Shutdown timed out waiting for goroutines")
	}

	log.Println("✅ Graceful shutdown completed")
	os.Exit(0)
}

// GetConfig returns the current configuration
func (k *KarlServer) GetConfig() *internal.Config {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.config
}

// IsShuttingDown returns the current shutdown state
func (k *KarlServer) IsShuttingDown() bool {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.isShuttingDown
}

// WaitForShutdown blocks until the server is shut down
func (k *KarlServer) WaitForShutdown() {
	<-k.ctx.Done()
}

// AddWorker adds a worker to the wait group
func (k *KarlServer) AddWorker() {
	k.wg.Add(1)
}

// WorkerDone marks a worker as done
func (k *KarlServer) WorkerDone() {
	k.wg.Done()
}