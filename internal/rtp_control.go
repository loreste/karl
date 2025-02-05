package internal

import (
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"

	"github.com/pion/rtp"
	"github.com/pion/srtp/v2"
	"github.com/pion/webrtc/v3"
)

// RTPControl manages RTP forwarding, SRTP handling, and conversions
type RTPControl struct {
	srtpSession     *srtp.Context
	udpConn         *net.UDPConn
	destinations    map[string]*net.UDPConn
	mu              sync.RWMutex
	stopped         bool
	packetsReceived uint64
	packetsDropped  uint64
	bytesReceived   uint64
	bytesSent       uint64
}

// NewRTPControl initializes RTP handling with SRTP
func NewRTPControl(srtpKey, srtpSalt []byte) (*RTPControl, error) {
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
			atomic.AddUint64(&r.packetsDropped, 1)
			continue
		}

		atomic.AddUint64(&r.packetsReceived, 1)
		atomic.AddUint64(&r.bytesReceived, uint64(n))

		packet := make([]byte, n)
		copy(packet, buffer[:n])

		go r.HandleRTPPacket(packet)

		if n > 0 {
			log.Printf("📦 Received packet from %s, size: %d bytes", remoteAddr, n)
		}
	}
}

// HandleRTPPacket processes an incoming RTP packet
func (r *RTPControl) HandleRTPPacket(packet []byte) error {
	rtpPacket := &rtp.Packet{}
	if err := rtpPacket.Unmarshal(packet); err != nil {
		atomic.AddUint64(&r.packetsDropped, 1)
		log.Printf("❌ Failed to unmarshal RTP packet: %v", err)
		return err
	}

	IncrementRTPPackets()
	CapturePacket(packet)

	log.Printf("📦 RTP Packet - SSRC: %d, SeqNum: %d, Timestamp: %d, PayloadType: %d",
		rtpPacket.SSRC,
		rtpPacket.SequenceNumber,
		rtpPacket.Timestamp,
		rtpPacket.PayloadType)

	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.srtpSession != nil {
		encrypted, err := r.srtpSession.EncryptRTP(nil, rtpPacket.Payload, &rtpPacket.Header)
		if err != nil {
			atomic.AddUint64(&r.packetsDropped, 1)
			log.Printf("❌ Failed to encrypt RTP packet: %v", err)
			return err
		}
		return r.forwardPacket(encrypted)
	}

	return r.forwardPacket(packet)
}

// AddDestination adds a new destination for RTP forwarding
func (r *RTPControl) AddDestination(addr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.destinations[addr]; exists {
		return nil
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
		n, err := conn.Write(packet)
		if err != nil {
			atomic.AddUint64(&r.packetsDropped, 1)
			log.Printf("❌ Failed to forward to %s: %v", addr, err)
			lastErr = err
			IncrementDroppedPackets()
		} else {
			atomic.AddUint64(&r.bytesSent, uint64(n))
		}
	}

	return lastErr
}

// GetStats returns the current RTP statistics
func (r *RTPControl) GetStats() (uint64, uint64, uint64, uint64) {
	return atomic.LoadUint64(&r.packetsReceived),
		atomic.LoadUint64(&r.packetsDropped),
		atomic.LoadUint64(&r.bytesReceived),
		atomic.LoadUint64(&r.bytesSent)
}

// ConvertToWebRTCTrack converts an RTP stream to a WebRTC track
func (r *RTPControl) ConvertToWebRTCTrack(pc *webrtc.PeerConnection, payloadType uint8) (*webrtc.TrackLocalStaticRTP, error) {
	track, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:    "audio/opus",
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "",
		},
		"audio",
		"rtp-stream",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create WebRTC track: %w", err)
	}

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

	for addr, conn := range r.destinations {
		conn.Close()
		log.Printf("Closed connection to %s", addr)
	}

	r.destinations = make(map[string]*net.UDPConn)
	log.Println("🛑 RTP Control stopped")
}
