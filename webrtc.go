package main

import (
	"fmt"
	"log"
	"time"

	"karl/internal"

	"github.com/pion/webrtc/v3"
)

// startWebRTC initializes and starts the WebRTC service
func (k *KarlServer) startWebRTC() error {
	k.mu.RLock()
	config := k.config
	k.mu.RUnlock()

	if !config.WebRTC.Enabled {
		log.Println("⚠️ WebRTC is disabled in configuration")
		return nil
	}

	log.Println("🎬 Initializing WebRTC...")

	// Setup ICE servers
	var iceServers []webrtc.ICEServer
	for _, stun := range config.WebRTC.StunServers {
		iceServers = append(iceServers, webrtc.ICEServer{URLs: []string{stun}})
	}
	for _, turn := range config.WebRTC.TurnServers {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs:       []string{turn.URL},
			Username:   turn.Username,
			Credential: turn.Credential,
		})
	}

	// Initialize ICE Manager with proper locking
	k.mu.Lock()
	var err error
	k.iceManager, err = internal.NewICEManager(iceServers)
	k.mu.Unlock()
	if err != nil {
		return fmt.Errorf("❌ Failed to initialize ICE Manager: %w", err)
	}

	// Start WebRTC session
	k.mu.Lock()
	k.webrtcSession, err = internal.StartWebRTCSession()
	k.mu.Unlock()
	if err != nil {
		return fmt.Errorf("❌ Failed to start WebRTC session: %w", err)
	}

	// Initialize SRTP Transcoder
	srtpKey := []byte(config.SRTP.Key)
	srtpSalt := []byte(config.SRTP.Salt)

	k.mu.Lock()
	k.srtpTranscoder, err = internal.NewSRTPTranscoder(srtpKey, srtpSalt)
	k.mu.Unlock()
	if err != nil {
		return fmt.Errorf("❌ Failed to initialize SRTP transcoder: %w", err)
	}

	// Initialize RTP Transcoder
	k.mu.Lock()
	k.transcoder = internal.NewRTPTranscoder(k.webrtcSession)
	k.mu.Unlock()

	// Initialize WebRTC stats monitoring
	statsConfig := &internal.StatsConfig{
		MonitoringInterval:    5 * time.Second,
		MaxReconnectAttempts:  5,
		BaseReconnectDelay:    time.Second,
		EnableDetailedLogging: true,
	}

	k.mu.Lock()
	k.webrtcStats = internal.NewWebRTCStats(k.webrtcSession, statsConfig)
	k.mu.Unlock()

	// Set up stats callback for metrics
	k.webrtcStats.SetStatsCallback(func(stats *internal.Stats) {
		if stats.PacketsLost > 0 {
			log.Printf("⚠️ Packet loss detected: %d packets", stats.PacketsLost)
		}
		// Update Prometheus metrics
		internal.SetPacketLoss(float64(stats.PacketsLost))
		internal.SetJitter(stats.JitterMS)
		internal.SetBandwidthUsage(int(stats.BytesSent))
	})

	// Set up reconnection callback
	k.webrtcStats.SetReconnectCallback(k.handleWebRTCReconnect)

	// Start stats monitoring
	if err := k.webrtcStats.StartMonitoring(k.ctx); err != nil {
		return fmt.Errorf("❌ Failed to start WebRTC monitoring: %w", err)
	}

	// Set up WebRTC callbacks
	k.setupWebRTCCallbacks()

	log.Println("✅ WebRTC initialized successfully")
	return nil
}

// handleWebRTCReconnect handles WebRTC reconnection
func (k *KarlServer) handleWebRTCReconnect() {
	k.mu.RLock()
	if k.isShuttingDown {
		k.mu.RUnlock()
		return
	}
	k.mu.RUnlock()

	log.Println("🔄 Reconnecting WebRTC session...")

	// Create new WebRTC session
	newSession, err := internal.StartWebRTCSession()
	if err != nil {
		log.Printf("❌ Failed to create new WebRTC session: %v", err)
		return
	}

	k.mu.Lock()
	oldSession := k.webrtcSession
	k.webrtcSession = newSession
	k.transcoder = internal.NewRTPTranscoder(newSession)
	k.webrtcStats.UpdatePeerConnection(newSession)
	k.mu.Unlock()

	// Close old session
	if oldSession != nil {
		if err := oldSession.Close(); err != nil {
			log.Printf("⚠️ Error closing old WebRTC session: %v", err)
		}
	}

	// Set up new callbacks
	k.setupWebRTCCallbacks()
}

// setupWebRTCCallbacks sets up WebRTC callbacks
func (k *KarlServer) setupWebRTCCallbacks() {
	k.mu.RLock()
	session := k.webrtcSession
	k.mu.RUnlock()

	if session == nil {
		log.Println("⚠️ Cannot setup callbacks: WebRTC session is nil")
		return
	}

	session.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			log.Printf("🌍 New ICE Candidate: %s", candidate.ToJSON().Candidate)
		}
	})

	session.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("🔄 WebRTC Connection State changed: %s", state.String())

		switch state {
		case webrtc.PeerConnectionStateFailed:
			log.Println("❌ WebRTC connection failed")
		case webrtc.PeerConnectionStateDisconnected:
			log.Println("⚠️ WebRTC disconnected, attempting reconnection...")
			go k.handleWebRTCReconnect()
		case webrtc.PeerConnectionStateConnected:
			log.Println("✅ WebRTC connected")
		}
	})

	session.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("📡 New track received: %s", track.Kind().String())
		k.wg.Add(1)
		go k.handleIncomingTrack(track)
	})
}

// handleIncomingTrack handles incoming WebRTC tracks
func (k *KarlServer) handleIncomingTrack(track *webrtc.TrackRemote) {
	defer k.wg.Done()

	k.mu.RLock()
	transcoder := k.transcoder
	srtpTranscoder := k.srtpTranscoder
	k.mu.RUnlock()

	if track.Kind() == webrtc.RTPCodecTypeAudio && transcoder != nil {
		outputTrack, err := transcoder.AddTrackPair(track)
		if err != nil {
			log.Printf("❌ Failed to create track pair: %v", err)
			return
		}

		k.mu.RLock()
		session := k.webrtcSession
		k.mu.RUnlock()

		if session != nil {
			if _, err := session.AddTrack(outputTrack); err != nil {
				log.Printf("❌ Failed to add transcoded track: %v", err)
				return
			}
		}

		log.Printf("✅ Added transcoded track for: %s", track.ID())
		return
	}

	buffer := make([]byte, 1500)
	for {
		select {
		case <-k.ctx.Done():
			return
		default:
			n, _, err := track.Read(buffer)
			if err != nil {
				log.Printf("❌ Error reading from track: %v", err)
				return
			}

			k.mu.RLock()
			rtpControl := k.rtpControl
			k.mu.RUnlock()

			packet := buffer[:n]

			// Encrypt the packet if SRTP is enabled
			if srtpTranscoder != nil {
				encryptedPacket, err := srtpTranscoder.TranscodeRTPToSRTP(packet)
				if err != nil {
					log.Printf("❌ Error encrypting RTP packet: %v", err)
					continue
				}
				packet = encryptedPacket
			}

			if rtpControl != nil {
				if err := rtpControl.HandleRTPPacket(packet); err != nil {
					log.Printf("❌ Error handling RTP packet: %v", err)
				}
			}
		}
	}
}
