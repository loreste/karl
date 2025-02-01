package main

import (
	"context"
	"fmt"
	"log"
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
	// Load configuration
	if err := k.loadConfig(); err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Set up signal handling
	k.setupSignalHandler()

	// Initialize metrics
	k.startMetrics()

	// Initialize all services
	if err := k.initializeServices(); err != nil {
		return fmt.Errorf("failed to initialize services: %w", err)
	}

	// Start SIP registration
	k.startSIPRegistration()

	log.Println("✅ Karl RTP Engine started successfully")
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
