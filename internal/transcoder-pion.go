package internal

import (
	"fmt"
	"log"
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

// RTPTranscoder handles transcoding between WebRTC and SIP codecs
type RTPTranscoder struct {
	mu            sync.RWMutex
	trackPairs    map[string]*trackPair
	peerConn      *webrtc.PeerConnection
	packetBuffers map[string]*PacketBuffer
}

// PacketBuffer handles packet reordering
type PacketBuffer struct {
	packets     []*rtp.Packet
	maxSize     int
	initialized bool
	lastSeq     uint16
}

// trackPair represents an input/output track pair for transcoding
type trackPair struct {
	inputTrack  *webrtc.TrackRemote
	outputTrack *webrtc.TrackLocalStaticRTP
	ssrc        webrtc.SSRC
	sequenceNum uint16
	timestamp   uint32
}

// NewRTPTranscoder creates a new transcoder instance
func NewRTPTranscoder(pc *webrtc.PeerConnection) *RTPTranscoder {
	return &RTPTranscoder{
		trackPairs:    make(map[string]*trackPair),
		peerConn:      pc,
		packetBuffers: make(map[string]*PacketBuffer),
	}
}

// AddTrackPair creates a new track pair for transcoding
func (t *RTPTranscoder) AddTrackPair(inputTrack *webrtc.TrackRemote) (*webrtc.TrackLocalStaticRTP, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Create output track for G.711
	outputTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypePCMU, // G.711 μ-law
			ClockRate:   8000,
			Channels:    1,
			SDPFmtpLine: "",
		},
		"audio",
		fmt.Sprintf("transcoded-%d", uint32(inputTrack.SSRC())),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create output track: %v", err)
	}

	// Create packet buffer for the track
	t.packetBuffers[inputTrack.ID()] = &PacketBuffer{
		packets: make([]*rtp.Packet, 50), // Buffer 50 packets
		maxSize: 50,
	}

	// Store track pair
	pair := &trackPair{
		inputTrack:  inputTrack,
		outputTrack: outputTrack,
		ssrc:        inputTrack.SSRC(),
	}
	t.trackPairs[inputTrack.ID()] = pair

	// Start processing
	go t.processTrack(pair)

	log.Printf("Added track pair - Input: %s, Output: %s", inputTrack.ID(), outputTrack.ID())
	return outputTrack, nil
}

// processTrack handles the actual transcoding of RTP packets
func (t *RTPTranscoder) processTrack(pair *trackPair) {
	buffer := make([]byte, 1500)
	packetBuffer := t.packetBuffers[pair.inputTrack.ID()]

	for {
		// Read from input track
		n, _, err := pair.inputTrack.Read(buffer)
		if err != nil {
			log.Printf("Error reading from track: %v", err)
			return
		}

		// Parse RTP packet
		packet := &rtp.Packet{}
		if err := packet.Unmarshal(buffer[:n]); err != nil {
			log.Printf("Error parsing RTP packet: %v", err)
			continue
		}

		// Simple packet reordering
		if !packetBuffer.initialized {
			packetBuffer.lastSeq = packet.SequenceNumber
			packetBuffer.initialized = true
		}

		// Check if packet is in sequence
		diff := packet.SequenceNumber - packetBuffer.lastSeq
		if diff > uint16(packetBuffer.maxSize) {
			// Packet too old or too far ahead, process immediately
			t.processPacket(pair, packet)
			packetBuffer.lastSeq = packet.SequenceNumber
		} else {
			// Store packet in buffer
			idx := packet.SequenceNumber % uint16(packetBuffer.maxSize)
			packetBuffer.packets[idx] = packet

			// Process any packets in order
			t.processBufferedPackets(pair, packetBuffer)
		}
	}
}

// processBufferedPackets processes packets that are ready from the buffer
func (t *RTPTranscoder) processBufferedPackets(pair *trackPair, buffer *PacketBuffer) {
	for {
		idx := buffer.lastSeq % uint16(buffer.maxSize)
		packet := buffer.packets[idx]
		if packet == nil {
			break
		}

		if packet.SequenceNumber == buffer.lastSeq {
			t.processPacket(pair, packet)
			buffer.packets[idx] = nil
			buffer.lastSeq++
		} else {
			break
		}
	}
}

// processPacket handles a single RTP packet
func (t *RTPTranscoder) processPacket(pair *trackPair, packet *rtp.Packet) {
	// Create G.711 packet
	g711Packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    0, // G.711 μ-law
			SequenceNumber: pair.sequenceNum,
			Timestamp:      packet.Timestamp / 6, // Convert from 48kHz to 8kHz
			SSRC:           uint32(pair.ssrc),
			Marker:         packet.Marker,
		},
		Payload: packet.Payload, // Note: In a real implementation, you'd transcode the payload here
	}

	// Write to output track
	if err := pair.outputTrack.WriteRTP(g711Packet); err != nil {
		log.Printf("Error writing RTP packet: %v", err)
		return
	}

	pair.sequenceNum++
}

// RemoveTrack removes a track pair and stops processing
func (t *RTPTranscoder) RemoveTrack(trackID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.trackPairs, trackID)
	delete(t.packetBuffers, trackID)
	log.Printf("Removed track pair for ID: %s", trackID)
}

// GetTrackPair retrieves a track pair by input track ID
func (t *RTPTranscoder) GetTrackPair(trackID string) (*trackPair, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	pair, exists := t.trackPairs[trackID]
	return pair, exists
}

// Close cleans up all resources
func (t *RTPTranscoder) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Clean up all track pairs
	for trackID := range t.trackPairs {
		delete(t.trackPairs, trackID)
		delete(t.packetBuffers, trackID)
	}

	log.Println("Transcoder closed and resources cleaned up")
	return nil
}

// GetStats returns current transcoding statistics
func (t *RTPTranscoder) GetStats() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["active_tracks"] = len(t.trackPairs)

	trackStats := make(map[string]interface{})
	for id, pair := range t.trackPairs {
		trackStats[id] = map[string]interface{}{
			"ssrc":         pair.ssrc,
			"sequence_num": pair.sequenceNum,
			"timestamp":    pair.timestamp,
			"input_codec":  pair.inputTrack.Codec().MimeType,
			"output_codec": "audio/pcmu", // G.711 μ-law
		}
	}
	stats["tracks"] = trackStats

	return stats
}
