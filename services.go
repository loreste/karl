package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"karl/internal"
)

// initializeServices initializes all service components
func (k *KarlServer) initializeServices() error {
	// Initialize Worker Pool
	internal.InitWorkerPool()

	// Initialize RTP Engine
	if err := k.startRTPEngine(); err != nil {
		return err
	}

	// Initialize WebRTC
	if err := k.startWebRTC(); err != nil {
		return err
	}

	// Initialize Database connections
	if err := k.initializeDatabases(); err != nil {
		return err
	}

	// Initialize Unix Socket Listener
	k.initializeUnixSocketListener()

	// Initialize API endpoints
	k.initializeAPIServer()

	// Start SIP registration
	k.startSIPRegistration()

	log.Println("✅ All services initialized successfully")
	return nil
}

// startRTPEngine initializes and starts the RTP engine
func (k *KarlServer) startRTPEngine() error {
	k.mu.RLock()
	config := k.config
	k.mu.RUnlock()

	if config == nil {
		return fmt.Errorf("❌ Configuration not loaded")
	}

	log.Println("🎬 Initializing RTP engine...")

	// Retrieve SRTP key and salt correctly
	srtpKey := []byte(config.SRTP.Key)
	srtpSalt := []byte(config.SRTP.Salt)

	// Initialize SRTP Transcoder
	srtpTranscoder, err := internal.NewSRTPTranscoder(srtpKey, srtpSalt)
	if err != nil {
		return fmt.Errorf("❌ Failed to initialize SRTP transcoder: %w", err)
	}

	// Initialize RTP Control with correct SRTP parameters
	rtpControl, err := internal.NewRTPControl(srtpKey, srtpSalt)
	if err != nil {
		return fmt.Errorf("❌ Failed to initialize RTP Control: %w", err)
	}

	addr := fmt.Sprintf(":%d", config.Transport.UDPPort)
	if err := rtpControl.StartRTPListener(addr); err != nil {
		rtpControl.Stop()
		return fmt.Errorf("❌ RTP Listener failed to start: %w", err)
	}

	k.mu.Lock()
	k.rtpControl = rtpControl
	k.srtpTranscoder = srtpTranscoder
	k.mu.Unlock()

	log.Printf("✅ RTP Engine started on UDP port %d", config.Transport.UDPPort)
	return nil
}

// initializeDatabases initializes database connections
func (k *KarlServer) initializeDatabases() error {
	k.mu.RLock()
	config := k.config
	k.mu.RUnlock()

	if config == nil {
		return fmt.Errorf("❌ Configuration not loaded")
	}

	// Initialize MySQL
	db, err := internal.NewRTPDatabase(config.Database.MySQLDSN)
	if err != nil {
		return fmt.Errorf("❌ Failed to initialize MySQL: %w", err)
	}
	k.database = db

	// Initialize Redis if enabled
	if config.Database.RedisEnabled {
		redisCache := internal.NewRTPRedisCache(config) // Pass entire `config` struct
		if redisCache != nil {
			k.redisCache = redisCache
			log.Println("✅ Redis initialized successfully")

			// Start Redis maintenance routines
			go k.redisCache.AutoCleanupExpiredSessions(
				time.Duration(config.Database.RedisCleanupInterval) * time.Second,
			)
			go k.redisCache.CheckRedisHealth(30 * time.Second)
		}
	}

	return nil
}

// startAPIServer initializes and starts the HTTP API server
func (k *KarlServer) initializeAPIServer() {
	// Set up API routes
	mux := internal.SetupRoutes()

	// Start HTTP server
	go func() {
		log.Println("🌐 Starting API server on :8080")
		if err := http.ListenAndServe(":8080", mux); err != nil {
			log.Printf("❌ API server error: %v", err)
		}
	}()
}

// startUnixSocketListener initializes and starts the Unix socket listener
func (k *KarlServer) initializeUnixSocketListener() {
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

// startMetrics initializes and starts the metrics collection
func (k *KarlServer) startMetrics() {
	// Initialize Prometheus metrics
	internal.InitMetrics()

	// Initialize PCAP capture if enabled
	k.mu.RLock()
	if k.config.RTPSettings.EnablePCAP {
		internal.InitPCAPCapture()
		log.Println("✅ PCAP capture initialized")
	}
	k.mu.RUnlock()

	log.Println("✅ Metrics collection started")
}

// startSIPRegistration starts periodic SIP proxy registration
func (k *KarlServer) startSIPRegistration() {
	k.mu.RLock()
	config := k.config
	k.mu.RUnlock()

	// Register with OpenSIPS
	go internal.PeriodicallyRegisterWithSIPProxy(
		config.Integration.OpenSIPSIp,
		config.Integration.OpenSIPSPort,
		30*time.Second,
	)

	// Register with Kamailio
	go internal.PeriodicallyRegisterWithSIPProxy(
		config.Integration.KamailioIp,
		config.Integration.KamailioPort,
		30*time.Second,
	)

	log.Println("✅ SIP registration started")
}
