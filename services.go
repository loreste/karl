package main

import (
	"fmt"
	"log"
	"time"

	"karl/internal"
	"karl/internal/api"
	"karl/internal/recording"
)

// initializeServices initializes all service components
func (k *KarlServer) initializeServices() error {
	// Initialize Worker Pool
	internal.InitWorkerPool()

	// Initialize Session Registry
	if err := k.initializeSessionRegistry(); err != nil {
		return err
	}

	// Initialize RTP Engine
	if err := k.startRTPEngine(); err != nil {
		return err
	}

	// Initialize RTCP Handler
	if err := k.initializeRTCPHandler(); err != nil {
		return err
	}

	// Initialize FEC Handler
	k.initializeFECHandler()

	// Initialize WebRTC
	if err := k.startWebRTC(); err != nil {
		return err
	}

	// Initialize Database connections
	if err := k.initializeDatabases(); err != nil {
		return err
	}

	// Initialize NG Socket Listener
	if err := k.initializeNGSocketListener(); err != nil {
		log.Printf("Warning: NG socket listener not started: %v", err)
	}

	// Initialize Unix Socket Listener (legacy)
	k.initializeUnixSocketListener()

	// Initialize REST API
	if err := k.initializeRESTAPI(); err != nil {
		log.Printf("Warning: REST API not started: %v", err)
	}

	// Initialize Recording System
	if err := k.initializeRecording(); err != nil {
		log.Printf("Warning: Recording system not started: %v", err)
	}

	// Initialize API endpoints
	k.initializeAPIServer()

	// Start SIP registration with cancelable context
	k.startSIPRegistration()

	log.Println("All services initialized successfully")
	return nil
}

// initializeSessionRegistry initializes the session registry
func (k *KarlServer) initializeSessionRegistry() error {
	k.mu.RLock()
	config := k.config
	k.mu.RUnlock()

	sessionTTL := 1 * time.Hour
	if config.Sessions != nil && config.Sessions.SessionTTL > 0 {
		sessionTTL = time.Duration(config.Sessions.SessionTTL) * time.Second
	}

	k.sessionRegistry = internal.NewSessionRegistry(sessionTTL)

	// Set callback for session termination metrics
	k.sessionRegistry.SetOnSessionEnd(func(session *internal.MediaSession) {
		session.Lock()
		if session.Stats.Duration > 0 {
			internal.RecordSessionDuration(session.Stats.Duration)
		}
		session.Unlock()
		internal.SetActiveSessionCount(k.sessionRegistry.GetActiveCount())
	})

	log.Println("Session registry initialized")
	return nil
}

// initializeRTCPHandler initializes the RTCP handler
func (k *KarlServer) initializeRTCPHandler() error {
	k.mu.RLock()
	config := k.config
	k.mu.RUnlock()

	rtcpConfig := &internal.RTCPInternalConfig{
		Enabled:     true,
		Interval:    5 * time.Second,
		ReducedSize: false,
		MuxEnabled:  true,
	}

	if config.RTCP != nil {
		rtcpConfig.Enabled = config.RTCP.Enabled
		if config.RTCP.Interval > 0 {
			rtcpConfig.Interval = time.Duration(config.RTCP.Interval) * time.Second
		}
		rtcpConfig.ReducedSize = config.RTCP.ReducedSize
		rtcpConfig.MuxEnabled = config.RTCP.MuxEnabled
	}

	k.rtcpHandler = internal.NewRTCPHandler(rtcpConfig)
	k.rtcpHandler.Start()

	log.Println("RTCP handler initialized")
	return nil
}

// initializeFECHandler initializes the FEC handler
func (k *KarlServer) initializeFECHandler() {
	k.mu.RLock()
	config := k.config
	k.mu.RUnlock()

	fecConfig := internal.DefaultFECConfig()

	if config.FEC != nil {
		fecConfig.Enabled = config.FEC.Enabled
		if config.FEC.BlockSize > 0 {
			fecConfig.BlockSize = config.FEC.BlockSize
		}
		if config.FEC.Redundancy > 0 {
			fecConfig.Redundancy = config.FEC.Redundancy
		}
		fecConfig.AdaptiveMode = config.FEC.AdaptiveMode
		if config.FEC.MaxRedundancy > 0 {
			fecConfig.MaxRedundancy = config.FEC.MaxRedundancy
		}
		if config.FEC.MinRedundancy > 0 {
			fecConfig.MinRedundancy = config.FEC.MinRedundancy
		}
	}

	k.fecHandler = internal.NewFECHandler(fecConfig)
	log.Println("FEC handler initialized")
}

// initializeNGSocketListener initializes the NG protocol socket listener
func (k *KarlServer) initializeNGSocketListener() error {
	k.mu.RLock()
	config := k.config
	k.mu.RUnlock()

	if config.NGProtocol == nil || !config.NGProtocol.Enabled {
		log.Println("NG protocol disabled in configuration")
		return nil
	}

	k.ngListener = internal.NewNGSocketListener(config, k.sessionRegistry)
	if err := k.ngListener.Start(); err != nil {
		return fmt.Errorf("failed to start NG socket listener: %w", err)
	}

	log.Println("NG socket listener initialized")
	return nil
}

// initializeRESTAPI initializes the REST API
func (k *KarlServer) initializeRESTAPI() error {
	k.mu.RLock()
	config := k.config
	k.mu.RUnlock()

	if config.API == nil || !config.API.Enabled {
		log.Println("REST API disabled in configuration")
		return nil
	}

	router := api.NewRouter(config, k.sessionRegistry)
	if err := router.Start(); err != nil {
		return fmt.Errorf("failed to start REST API: %w", err)
	}

	log.Println("REST API initialized")
	return nil
}

// initializeRecording initializes the recording system
func (k *KarlServer) initializeRecording() error {
	k.mu.RLock()
	config := k.config
	k.mu.RUnlock()

	if config.Recording == nil || !config.Recording.Enabled {
		log.Println("Recording disabled in configuration")
		return nil
	}

	recConfig := &recording.RecordingConfig{
		BasePath:      config.Recording.BasePath,
		Format:        recording.RecordingFormat(config.Recording.Format),
		Mode:          recording.RecordingMode(config.Recording.Mode),
		SampleRate:    config.Recording.SampleRate,
		BitsPerSample: config.Recording.BitsPerSample,
		MaxFileSize:   config.Recording.MaxFileSize,
		RetentionDays: config.Recording.RetentionDays,
	}

	manager := recording.NewManager(recConfig)
	if err := manager.Start(); err != nil {
		return fmt.Errorf("failed to start recording manager: %w", err)
	}

	log.Println("Recording system initialized")
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
