package internal

import (
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/srtp/v2"
	"github.com/pion/webrtc/v3"
)

// RTPControl manages RTP forwarding, SRTP handling, and conversions
type RTPControl struct {
	srtpSession  *srtp.Context
	udpConn      *net.UDPConn
	destinations map[string]*net.UDPConn
	mu           sync.RWMutex
	stopped      bool
}

// NewRTPControl initializes RTP handling with SRTP
func NewRTPControl(srtpKey, srtpSalt []byte) (*RTPControl, error) {
	// Initialize SRTP Context if keys are provided
	var srtpSession *srtp.Context
	var err error

	if len(srtpKey) > 0 && len(srtpSalt) > 0 {
		profile := srtp.ProtectionProfileAes128CmHmacSha1_80
		srtpSession, err = srtp.CreateContext(srtpKey, srtpSalt, profile)
		if err != nil {
			return nil, fmt.Errorf("failed to create SRTP context: %w", err)
		}
		log.Println("✅ SRTP context initialized")
	}

	return &RTPControl{
		srtpSession:  srtpSession,
		destinations: make(map[string]*net.UDPConn),
	}, nil
}

// StartRTPListener listens for incoming RTP packets
func (r *RTPControl) StartRTPListener(addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	r.udpConn, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("failed to start UDP listener: %w", err)
	}

	log.Printf("🎧 RTP Listener started on %s", addr)

	// Start packet handling loop
	go r.packetHandlingLoop()

	return nil
}

// packetHandlingLoop continuously reads and processes incoming packets
func (r *RTPControl) packetHandlingLoop() {
	buffer := make([]byte, 1500) // Standard MTU size

	for {
		r.mu.RLock()
		if r.stopped {
			r.mu.RUnlock()
			return
		}
		r.mu.RUnlock()

		n, remoteAddr, err := r.udpConn.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("❌ Error reading UDP packet: %v", err)
			continue
		}

		// Make a copy of the packet data to prevent buffer reuse issues
		packet := make([]byte, n)
		copy(packet, buffer[:n])

		// Process packet in a goroutine
		go r.HandleRTPPacket(packet)

		log.Printf("📦 Received packet from %s, size: %d bytes", remoteAddr, n)
	}
}

// HandleRTPPacket processes an incoming RTP packet
func (r *RTPControl) HandleRTPPacket(packet []byte) error {
	// Parse the RTP packet
	rtpPacket := &rtp.Packet{}
	if err := rtpPacket.Unmarshal(packet); err != nil {
		log.Printf("❌ Failed to unmarshal RTP packet: %v", err)
		return err
	}

	// Update metrics
	IncrementRTPPackets()

	// Capture packet for debugging if enabled
	CapturePacket(packet)

	// Log packet details if detailed logging is enabled
	log.Printf("📦 RTP Packet - SSRC: %d, SeqNum: %d, Timestamp: %d, PayloadType: %d",
		rtpPacket.SSRC,
		rtpPacket.SequenceNumber,
		rtpPacket.Timestamp,
		rtpPacket.PayloadType)

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Handle based on configuration
	if r.srtpSession != nil {
		// If SRTP is enabled, encrypt the packet
		encrypted, err := r.srtpSession.EncryptRTP(nil, rtpPacket.Payload, &rtpPacket.Header)
		if err != nil {
			log.Printf("❌ Failed to encrypt RTP packet: %v", err)
			return err
		}

		// Forward the encrypted packet
		return r.forwardPacket(encrypted)
	}

	// Forward the original packet if SRTP is not enabled
	return r.forwardPacket(packet)
}

// AddDestination adds a new destination for RTP forwarding
func (r *RTPControl) AddDestination(addr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.destinations[addr]; exists {
		return nil // Already exists
	}

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to resolve destination address: %w", err)
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return fmt.Errorf("failed to create UDP connection: %w", err)
	}

	r.destinations[addr] = conn
	log.Printf("✅ Added RTP destination: %s", addr)
	return nil
}

// RemoveDestination removes a forwarding destination
func (r *RTPControl) RemoveDestination(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if conn, exists := r.destinations[addr]; exists {
		conn.Close()
		delete(r.destinations, addr)
		log.Printf("❌ Removed RTP destination: %s", addr)
	}
}

// forwardPacket sends the packet to all configured destinations
func (r *RTPControl) forwardPacket(packet []byte) error {
	var lastErr error

	for addr, conn := range r.destinations {
		_, err := conn.Write(packet)
		if err != nil {
			log.Printf("❌ Failed to forward to %s: %v", addr, err)
			lastErr = err
			IncrementDroppedPackets()
		}
	}

	return lastErr
}

// ConvertToWebRTCTrack converts an RTP stream to a WebRTC track
func (r *RTPControl) ConvertToWebRTCTrack(pc *webrtc.PeerConnection, payloadType uint8) (*webrtc.TrackLocalStaticRTP, error) {
	// Create a new track
	track, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:    "audio/opus", // Adjust based on payload type
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "",
		},
		"audio", // or "video" based on media type
		"rtp-stream",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create WebRTC track: %w", err)
	}

	// Add the track to the peer connection
	_, err = pc.AddTrack(track)
	if err != nil {
		return nil, fmt.Errorf("failed to add track to peer connection: %w", err)
	}

	return track, nil
}

// Stop gracefully shuts down the RTP control
func (r *RTPControl) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stopped = true

	if r.udpConn != nil {
		r.udpConn.Close()
	}

	// Close all destination connections
	for addr, conn := range r.destinations {
		conn.Close()
		log.Printf("Closed connection to %s", addr)
	}

	// Clear the destinations map
	r.destinations = make(map[string]*net.UDPConn)

	log.Println("🛑 RTP Control stopped")
}
