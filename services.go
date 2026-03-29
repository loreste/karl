package main

import (
	"fmt"
	"log"
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

	// Start SIP registration with cancelable context
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

	// Initialize MySQL if DSN is provided
	if config.Database.MySQLDSN != "" {
		db, err := internal.NewRTPDatabase(config.Database.MySQLDSN)
		if err != nil {
			return fmt.Errorf("❌ Failed to initialize MySQL: %w", err)
		}
		k.database = db
	} else {
		log.Println("⚠️ MySQL database connection disabled (no DSN provided)")
	}

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
	// Skip API server initialization here, it's already started in loadConfig
	log.Println("✅ API server already initialized")
}

// startUnixSocketListener initializes and starts the Unix socket listener
func (k *KarlServer) initializeUnixSocketListener() {
	// Skip Unix socket listener initialization here, it's already started in loadConfig
	log.Println("✅ Unix socket listener already initialized")
}



// startSIPRegistration starts periodic SIP proxy registration
func (k *KarlServer) startSIPRegistration() {
	k.mu.RLock()
	config := k.config
	k.mu.RUnlock()

	interval := 30 * time.Second
	if config.Integration.KeepAliveInterval > 0 {
		interval = time.Duration(config.Integration.KeepAliveInterval) * time.Second
	}

	// Use the new context-aware registration service
	// Register with OpenSIPS
	if config.Integration.OpenSIPSIp != "" && config.Integration.OpenSIPSPort > 0 {
		k.AddWorker() // Track this in the waitgroup
		go func() {
			defer k.WorkerDone()
			internal.StartRegistrationService(
				k.ctx,
				config.Integration.OpenSIPSIp,
				config.Integration.OpenSIPSPort,
				interval,
			)
		}()
		log.Printf("✅ OpenSIPS registration service started for %s:%d",
			config.Integration.OpenSIPSIp,
			config.Integration.OpenSIPSPort)
	}

	// Register with Kamailio
	if config.Integration.KamailioIp != "" && config.Integration.KamailioPort > 0 {
		k.AddWorker() // Track this in the waitgroup
		go func() {
			defer k.WorkerDone()
			internal.StartRegistrationService(
				k.ctx,
				config.Integration.KamailioIp,
				config.Integration.KamailioPort,
				interval,
			)
		}()
		log.Printf("✅ Kamailio registration service started for %s:%d",
			config.Integration.KamailioIp,
			config.Integration.KamailioPort)
	}

	log.Println("✅ SIP registration services started")
}
