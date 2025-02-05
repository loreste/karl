package internal

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"

	"github.com/pion/webrtc/v3"
)

var (
	transcoder   *RTPTranscoder
	statsMonitor *WebRTCStats
	sessions     int32
)

// StartWebRTCSession initializes a new WebRTC PeerConnection
func StartWebRTCSession() (*webrtc.PeerConnection, error) {
	configMutex.RLock()
	if !config.WebRTC.Enabled {
		configMutex.RUnlock()
		return nil, fmt.Errorf("WebRTC is disabled in configuration")
	}
	stunServers := config.WebRTC.StunServers
	turnServers := config.WebRTC.TurnServers
	configMutex.RUnlock()

	// Create WebRTC configuration with STUN/TURN servers
	var iceServers []webrtc.ICEServer
	for _, stun := range stunServers {
		iceServers = append(iceServers, webrtc.ICEServer{URLs: []string{stun}})
	}
	for _, turn := range turnServers {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs:       []string{turn.URL},
			Username:   turn.Username,
			Credential: turn.Credential,
		})
	}

	webrtcConfig := webrtc.Configuration{
		ICEServers: iceServers,
	}

	// Create a new WebRTC PeerConnection
	peerConnection, err := webrtc.NewPeerConnection(webrtcConfig)
	if err != nil {
		atomic.AddInt32(&sessions, -1)
		log.Printf("Failed to create WebRTC session: %v", err)
		return nil, err
	}

	// Initialize stats monitoring
	statsMonitor = NewWebRTCStats(peerConnection, DefaultStatsConfig())
	statsMonitor.SetStatsCallback(func(stats *Stats) {
		// Update metrics based on stats
		if stats.PacketsLost > 0 {
			IncrementDroppedPackets()
		}
		SetPacketLoss(float64(stats.PacketsLost))
		SetJitter(stats.JitterMS)
		SetBandwidthUsage(int(stats.BytesSent))
	})

	// Start stats monitoring
	ctx := context.Background()
	if err := statsMonitor.StartMonitoring(ctx); err != nil {
		log.Printf("Failed to start stats monitoring: %v", err)
	}

	// Initialize transcoder with the peer connection
	transcoder = NewRTPTranscoder(peerConnection)

	// Set up track handling for transcoding
	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("New track received: %s (ID: %s)", track.Codec().MimeType, track.ID())

		// Check if it's an audio track that needs transcoding
		if track.Kind() == webrtc.RTPCodecTypeAudio {
			outputTrack, err := transcoder.AddTrackPair(track)
			if err != nil {
				log.Printf("Failed to create track pair: %v", err)
				return
			}

			// Add the transcoded track to the peer connection
			if _, err := peerConnection.AddTrack(outputTrack); err != nil {
				log.Printf("Failed to add transcoded track: %v", err)
				return
			}

			log.Printf("Added transcoded track for: %s", track.ID())
		}
	})

	// Set up ICE handling
	peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			log.Printf("New ICE candidate: %s", candidate.ToJSON().Candidate)
		}
	})

	// Set up connection state handling
	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("WebRTC connection state changed to: %s", state.String())

		switch state {
		case webrtc.PeerConnectionStateDisconnected:
			log.Println("WebRTC disconnected - cleaning up transcoder")
			if transcoder != nil {
				transcoder.Close()
			}
			atomic.AddInt32(&sessions, -1)
		case webrtc.PeerConnectionStateFailed:
			log.Println("WebRTC failed - cleaning up transcoder")
			if transcoder != nil {
				transcoder.Close()
			}
			atomic.AddInt32(&sessions, -1)
		case webrtc.PeerConnectionStateConnected:
			log.Println("WebRTC connected successfully")
			atomic.AddInt32(&sessions, 1)
		}
	})

	log.Println("WebRTC session initialized successfully")
	return peerConnection, nil
}

// HandleWebRTCOffer processes a WebRTC SDP offer and returns an SDP answer
func HandleWebRTCOffer(offer webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	peerConnection, err := StartWebRTCSession()
	if err != nil {
		return nil, err
	}

	// Set the remote SDP offer
	err = peerConnection.SetRemoteDescription(offer)
	if err != nil {
		log.Printf("Failed to set remote SDP offer: %v", err)
		return nil, err
	}

	// Create an SDP answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		log.Printf("Failed to create SDP answer: %v", err)
		return nil, err
	}

	// Set the local SDP answer
	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		log.Printf("Failed to set local SDP answer: %v", err)
		return nil, err
	}

	log.Println("Generated SDP answer for WebRTC session")
	return &answer, nil
}

// GetTranscodedTrack retrieves a transcoded track by input track ID
func GetTranscodedTrack(trackID string) (*webrtc.TrackLocalStaticRTP, bool) {
	if transcoder != nil {
		if pair, exists := transcoder.GetTrackPair(trackID); exists {
			return pair.outputTrack, true
		}
	}
	return nil, false
}

// CleanupWebRTCSession cleans up the WebRTC session and monitoring
func CleanupWebRTCSession() {
	if statsMonitor != nil {
		statsMonitor.StopMonitoring()
		statsMonitor = nil
	}

	if transcoder != nil {
		transcoder.Close()
		transcoder = nil
	}

	atomic.StoreInt32(&sessions, 0)
	log.Println("WebRTC session cleaned up")
}

// GetActiveSessionCount returns the number of active WebRTC sessions
func GetActiveSessionCount() int32 {
	return atomic.LoadInt32(&sessions)
}
