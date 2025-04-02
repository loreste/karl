package internal

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

const (
	maxBufferSize = 100
	maxJitter     = 100 * time.Millisecond
)

// RTPTranscoder handles transcoding between WebRTC and SIP codecs
type RTPTranscoder struct {
	mu            sync.RWMutex
	trackPairs    map[string]*trackPair
	peerConn      *webrtc.PeerConnection
	packetBuffers map[string]*PacketBuffer
	dtmfEnabled   bool
	vadEnabled    bool
	stats         *TranscoderStats
}

// PacketBuffer handles packet reordering and jitter buffer
type PacketBuffer struct {
	mu          sync.Mutex
	packets     []*rtp.Packet
	maxSize     int
	initialized bool
	lastSeq     uint16
	lastTS      uint32
}

// TranscoderStats tracks transcoding statistics
type TranscoderStats struct {
	PacketsReceived uint64
	PacketsDropped  uint64
	LastError       error
	LastErrorTime   time.Time
}

// trackPair represents an input/output track pair for transcoding
type trackPair struct {
	inputTrack  *webrtc.TrackRemote
	outputTrack *webrtc.TrackLocalStaticRTP
	ssrc        webrtc.SSRC
	sequenceNum uint16
	timestamp   uint32
	payloadType uint8
	codec       string
}

// NewRTPTranscoder creates a new transcoder instance
func NewRTPTranscoder(pc *webrtc.PeerConnection) *RTPTranscoder {
	return &RTPTranscoder{
		trackPairs:    make(map[string]*trackPair),
		peerConn:      pc,
		packetBuffers: make(map[string]*PacketBuffer),
		stats:         &TranscoderStats{},
	}
}

// AddTrackPair creates a new track pair for transcoding
func (t *RTPTranscoder) AddTrackPair(inputTrack *webrtc.TrackRemote) (*webrtc.TrackLocalStaticRTP, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	codec := getPreferredCodec(inputTrack.Codec())
	outputTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:    codec,
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

	t.packetBuffers[inputTrack.ID()] = &PacketBuffer{
		packets: make([]*rtp.Packet, maxBufferSize),
		maxSize: maxBufferSize,
	}

	pair := &trackPair{
		inputTrack:  inputTrack,
		outputTrack: outputTrack,
		ssrc:        inputTrack.SSRC(),
		codec:       codec,
	}
	t.trackPairs[inputTrack.ID()] = pair

	go t.processTrack(pair)
	log.Printf("Added track pair - Input: %s (%s), Output: %s (%s)",
		inputTrack.ID(), inputTrack.Codec().MimeType, outputTrack.ID(), codec)

	return outputTrack, nil
}

// processTrack handles the actual transcoding of RTP packets
func (t *RTPTranscoder) processTrack(pair *trackPair) {
	buffer := make([]byte, 1500)
	packetBuffer := t.packetBuffers[pair.inputTrack.ID()]

	for {
		n, _, err := pair.inputTrack.Read(buffer)
		if err != nil {
			t.handleError(fmt.Errorf("track read error: %v", err))
			return
		}

		t.stats.PacketsReceived++

		packet := &rtp.Packet{}
		if err := packet.Unmarshal(buffer[:n]); err != nil {
			t.stats.PacketsDropped++
			t.handleError(fmt.Errorf("packet unmarshal error: %v", err))
			continue
		}

		// DTMF detection if enabled
		if t.dtmfEnabled && isDTMFPacket(packet) {
			t.handleDTMF(packet)
			continue
		}

		// VAD processing if enabled
		if t.vadEnabled {
			// Convert RTP payload to PCM samples first
			pcmSamples, err := DecodePCMUToPCM(packet.Payload)
			if err != nil {
				t.handleError(fmt.Errorf("VAD conversion error: %v", err))
				continue
			}
			if !IsVoiceActive(pcmSamples) {
				continue
			}
		}

		// Handle packet ordering
		if !packetBuffer.initialized {
			packetBuffer.mu.Lock()
			packetBuffer.lastSeq = packet.SequenceNumber
			packetBuffer.lastTS = packet.Timestamp
			packetBuffer.initialized = true
			packetBuffer.mu.Unlock()
		}

		// Jitter buffer management
		t.handleJitterBuffer(packetBuffer, packet, pair)
	}
}

func (t *RTPTranscoder) handleDTMF(packet *rtp.Packet) {
	if len(packet.Payload) < 4 {
		return
	}

	eventID := packet.Payload[0]                                         // DTMF digit
	volume := packet.Payload[1]                                          // Volume
	duration := uint16(packet.Payload[2])<<8 | uint16(packet.Payload[3]) // Duration

	log.Printf("DTMF Event: digit=%d, volume=%d, duration=%d", eventID, volume, duration)
}

func (t *RTPTranscoder) handleJitterBuffer(buffer *PacketBuffer, packet *rtp.Packet, pair *trackPair) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	// Calculate position in buffer
	diff := packet.SequenceNumber - buffer.lastSeq
	if diff > uint16(buffer.maxSize) {
		// Packet too old or too far ahead, process immediately
		t.transcodeAndSend(packet, pair)
		buffer.lastSeq = packet.SequenceNumber
	} else {
		// Store packet in buffer
		idx := packet.SequenceNumber % uint16(buffer.maxSize)
		buffer.packets[idx] = packet

		// Process any packets in order
		t.processBufferedPackets(buffer, pair)
	}
}

func (t *RTPTranscoder) processBufferedPackets(buffer *PacketBuffer, pair *trackPair) {
	for {
		idx := buffer.lastSeq % uint16(buffer.maxSize)
		packet := buffer.packets[idx]
		if packet == nil {
			break
		}

		if packet.SequenceNumber == buffer.lastSeq {
			t.transcodeAndSend(packet, pair)
			buffer.packets[idx] = nil
			buffer.lastSeq++
		} else {
			break
		}
	}
}

func (t *RTPTranscoder) transcodeAndSend(packet *rtp.Packet, pair *trackPair) {
	// Transcode based on codec
	transcodedPayload, err := TranscodeAudio(packet.Payload, pair.inputTrack.Codec().MimeType, pair.codec)
	if err != nil {
		t.handleError(fmt.Errorf("transcoding error: %v", err))
		return
	}

	// Create output packet
	outputPacket := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    pair.payloadType,
			SequenceNumber: pair.sequenceNum,
			Timestamp:      packet.Timestamp,
			SSRC:           uint32(pair.ssrc),
			Marker:         packet.Marker,
		},
		Payload: transcodedPayload,
	}

	if err := pair.outputTrack.WriteRTP(outputPacket); err != nil {
		t.handleError(fmt.Errorf("failed to write RTP packet: %v", err))
		return
	}

	pair.sequenceNum++
}

// handleError processes transcoding errors
func (t *RTPTranscoder) handleError(err error) {
	t.mu.Lock()
	t.stats.LastError = err
	t.stats.LastErrorTime = time.Now()
	t.mu.Unlock()
	log.Printf("Transcoding error: %v", err)
}

// Helper functions
func getPreferredCodec(input webrtc.RTPCodecParameters) string {
	switch input.MimeType {
	case webrtc.MimeTypeOpus:
		return webrtc.MimeTypePCMU // Convert Opus to G.711 μ-law
	case webrtc.MimeTypeVP8:
		return webrtc.MimeTypeH264 // Convert VP8 to H.264
	default:
		return input.MimeType // Pass through if no conversion needed
	}
}

func isDTMFPacket(packet *rtp.Packet) bool {
	// Check if packet contains DTMF (RFC 4733)
	return packet.PayloadType == 101
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

	for trackID := range t.trackPairs {
		delete(t.trackPairs, trackID)
		delete(t.packetBuffers, trackID)
	}

	log.Println("Transcoder closed and resources cleaned up")
	return nil
}

// GetStats returns current transcoding statistics
func (t *RTPTranscoder) GetStats() *TranscoderStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return &TranscoderStats{
		PacketsReceived: t.stats.PacketsReceived,
		PacketsDropped:  t.stats.PacketsDropped,
		LastError:       t.stats.LastError,
		LastErrorTime:   t.stats.LastErrorTime,
	}
}
