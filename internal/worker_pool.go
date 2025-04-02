package internal

import (
	"encoding/binary"
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// WorkerPool settings
var (
	workerPoolSize = runtime.NumCPU() * 2    // Number of concurrent workers (adjust as needed)
	rtpJobs        = make(chan []byte, 1000) // Buffered channel for incoming RTP packets
	wg             sync.WaitGroup

	// Metrics counters
	packetsProcessed  atomic.Uint64
	packetErrors      atomic.Uint64
	bytesProcessed    atomic.Uint64
	transcodingErrors atomic.Uint64
	forwardingErrors  atomic.Uint64

	// Debug settings
	debugLogging = false

	// RTP handler registry (mapping SSRC to handlers)
	rtpHandlers     = make(map[uint32]RTPPacketHandler)
	rtpHandlersLock sync.RWMutex
)

// RTPPacket represents a parsed RTP packet
type RTPPacket struct {
	Version        uint8
	Padding        bool
	Extension      bool
	CSRCCount      uint8
	Marker         bool
	PayloadType    uint8
	SequenceNumber uint16
	Timestamp      uint32
	SSRC           uint32
	CSRC           []uint32
	ExtensionData  []byte
	Payload        []byte
	Received       time.Time
}

// RTPPacketHandler defines the interface for RTP packet processing
type RTPPacketHandler interface {
	Handle(*RTPPacket) error
}

// RegisterRTPHandler registers a handler for a specific SSRC
func RegisterRTPHandler(ssrc uint32, handler RTPPacketHandler) {
	rtpHandlersLock.Lock()
	defer rtpHandlersLock.Unlock()
	rtpHandlers[ssrc] = handler
}

// UnregisterRTPHandler removes a handler for a specific SSRC
func UnregisterRTPHandler(ssrc uint32) {
	rtpHandlersLock.Lock()
	defer rtpHandlersLock.Unlock()
	delete(rtpHandlers, ssrc)
}

// InitWorkerPool initializes a pool of workers to process RTP packets concurrently
func InitWorkerPool() {
	log.Printf("Initializing RTP worker pool with %d workers", workerPoolSize)

	for i := 0; i < workerPoolSize; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for packet := range rtpJobs {
				processRTPPacket(packet, workerID)
			}
		}(i)
	}
}

// processRTPPacket handles an RTP packet (can include transcoding, forwarding, etc.)
func processRTPPacket(packet []byte, workerID int) {
	// Capture packet for debugging if PCAP logging is enabled
	if IsPCAPEnabled() {
		CapturePacket(packet)
	}

	// Parse the RTP packet
	rtpPacket, err := ParseRTPPacket(packet)
	if err != nil {
		log.Printf("Worker %d failed to parse RTP packet: %v", workerID, err)
		return
	}

	// Update metrics
	UpdateRTPMetrics(rtpPacket)

	// Check if this packet should be processed for transcoding
	if ShouldTranscodePacket(rtpPacket) {
		// Perform audio transcoding if needed
		if err := TranscodeRTPPacket(rtpPacket); err != nil {
			log.Printf("Worker %d transcoding error: %v", workerID, err)
		}
	}

	// Check if packet needs to be forwarded to another destination
	if ShouldForwardPacket(rtpPacket) {
		if err := ForwardRTPPacket(rtpPacket); err != nil {
			log.Printf("Worker %d forwarding error: %v", workerID, err)
		}
	}

	// Check for RTCP feedback messages and update statistics
	HandleRTCPFeedback(rtpPacket)

	// Log detailed packet info at debug level only
	if IsDebugLoggingEnabled() {
		log.Printf("Worker %d processed RTP packet: SSRC=%d, seq=%d, ts=%d, pt=%d, size=%d bytes",
			workerID, rtpPacket.SSRC, rtpPacket.SequenceNumber, rtpPacket.Timestamp,
			rtpPacket.PayloadType, len(packet))
	}
}

// AddRTPJob sends an RTP packet to the worker pool for processing
func AddRTPJob(packet []byte) {
	select {
	case rtpJobs <- append([]byte(nil), packet...): // Copy packet before sending to avoid data race
	default:
		log.Println("RTP job queue is full, packet dropped")
	}
}

// StopWorkerPool shuts down the worker pool gracefully
func StopWorkerPool() {
	close(rtpJobs)
	wg.Wait()
	log.Println("RTP worker pool stopped")
}

// EnableDebugLogging enables or disables debug-level logging
func EnableDebugLogging(enable bool) {
	debugLogging = enable
}

// IsDebugLoggingEnabled returns whether debug logging is enabled
func IsDebugLoggingEnabled() bool {
	return debugLogging
}

// GetMetrics returns current worker pool metrics
func GetMetrics() map[string]uint64 {
	return map[string]uint64{
		"packets_processed":  packetsProcessed.Load(),
		"packet_errors":      packetErrors.Load(),
		"bytes_processed":    bytesProcessed.Load(),
		"transcoding_errors": transcodingErrors.Load(),
		"forwarding_errors":  forwardingErrors.Load(),
	}
}

// GetWorkerPoolMetrics is an alias for GetMetrics that can be used
// by other packages without causing import cycles
func GetWorkerPoolMetrics() map[string]uint64 {
	return GetMetrics()
}

// ParseRTPPacket parses a raw RTP packet into a structured RTPPacket
func ParseRTPPacket(data []byte) (*RTPPacket, error) {
	if len(data) < 12 {
		packetErrors.Add(1)
		return nil, fmt.Errorf("packet too short for RTP header")
	}

	// Parse header fields
	headerByte := data[0]
	version := (headerByte >> 6) & 0x03
	hasPadding := (headerByte & 0x20) != 0
	hasExtension := (headerByte & 0x10) != 0
	csrcCount := headerByte & 0x0F

	markerAndPT := data[1]
	marker := (markerAndPT & 0x80) != 0
	payloadType := markerAndPT & 0x7F

	sequenceNumber := binary.BigEndian.Uint16(data[2:4])
	timestamp := binary.BigEndian.Uint32(data[4:8])
	ssrc := binary.BigEndian.Uint32(data[8:12])

	// Initialize packet
	packet := &RTPPacket{
		Version:        version,
		Padding:        hasPadding,
		Extension:      hasExtension,
		CSRCCount:      csrcCount,
		Marker:         marker,
		PayloadType:    payloadType,
		SequenceNumber: sequenceNumber,
		Timestamp:      timestamp,
		SSRC:           ssrc,
		Received:       time.Now(),
	}

	// Calculate header size
	headerSize := 12 + 4*int(csrcCount)

	// Check if packet is long enough for header + CSRC
	if len(data) < headerSize {
		packetErrors.Add(1)
		return nil, fmt.Errorf("packet too short for CSRC list")
	}

	// Extract CSRC list
	if csrcCount > 0 {
		packet.CSRC = make([]uint32, csrcCount)
		for i := uint8(0); i < csrcCount; i++ {
			offset := 12 + 4*i
			packet.CSRC[i] = binary.BigEndian.Uint32(data[offset : offset+4])
		}
	}

	// Handle extension header if present
	if hasExtension {
		// Check if packet is long enough for extension header
		if len(data) < headerSize+4 {
			packetErrors.Add(1)
			return nil, fmt.Errorf("packet too short for extension header")
		}

		extHeaderOffset := headerSize
		extLength := int(binary.BigEndian.Uint16(data[extHeaderOffset+2:extHeaderOffset+4])) * 4

		// Check if packet is long enough for extension data
		if len(data) < headerSize+4+extLength {
			packetErrors.Add(1)
			return nil, fmt.Errorf("packet too short for extension data")
		}

		packet.ExtensionData = data[extHeaderOffset+4 : extHeaderOffset+4+extLength]
		headerSize += 4 + extLength
	}

	// Handle padding
	payloadEnd := len(data)
	if hasPadding && len(data) > 0 {
		paddingSize := int(data[len(data)-1])
		if paddingSize > 0 && len(data) >= paddingSize {
			payloadEnd -= paddingSize
		}
	}

	// Extract payload
	if headerSize < payloadEnd {
		packet.Payload = data[headerSize:payloadEnd]
	} else {
		packet.Payload = []byte{} // Empty payload
	}

	// Update metrics
	packetsProcessed.Add(1)
	bytesProcessed.Add(uint64(len(data)))

	return packet, nil
}

// UpdateRTPMetrics updates metrics for the processed RTP packet
func UpdateRTPMetrics(packet *RTPPacket) {
	// Update Prometheus metrics here
	// This would be integrated with the metrics.go implementation

	// Example: rtpPacketsTotal.WithLabelValues(fmt.Sprintf("%d", packet.PayloadType)).Inc()
}

// ShouldTranscodePacket determines if a packet needs transcoding
func ShouldTranscodePacket(packet *RTPPacket) bool {
	// Check if this packet's payload type requires transcoding
	// This would be based on configured transcoding rules

	// For now, a simple implementation that checks for common audio codecs
	switch packet.PayloadType {
	case 0, 8: // PCMU, PCMA
		return true
	case 111, 96, 97, 98, 99, 100, 101, 102: // Typical dynamic payload types for Opus
		return true
	default:
		return false
	}
}

// TranscodeRTPPacket performs transcoding on an RTP packet's payload
func TranscodeRTPPacket(packet *RTPPacket) error {
	// Identify source and target codecs based on payload type
	var srcCodec, dstCodec string

	switch packet.PayloadType {
	case 0:
		srcCodec = "PCMU" // G.711 μ-law
		dstCodec = "opus"
	case 8:
		srcCodec = "PCMA" // G.711 A-law
		dstCodec = "opus"
	case 111, 96, 97, 98, 99, 100, 101, 102:
		srcCodec = "opus" // Assuming dynamic types are Opus
		dstCodec = "PCMU" // Target G.711 μ-law
	default:
		return fmt.Errorf("unsupported codec for transcoding: %d", packet.PayloadType)
	}

	// Perform the actual transcoding using the codec_converter.go implementations
	transcodedPayload, err := TranscodeAudio(packet.Payload, srcCodec, dstCodec)
	if err != nil {
		transcodingErrors.Add(1)
		return fmt.Errorf("failed to transcode audio: %w", err)
	}

	// Update the packet with the transcoded payload
	packet.Payload = transcodedPayload

	// Update payload type to reflect the new codec
	if srcCodec == "PCMU" || srcCodec == "PCMA" {
		packet.PayloadType = 111 // Example dynamic type for Opus
	} else {
		packet.PayloadType = 0 // PCMU
	}

	return nil
}

// ShouldForwardPacket determines if a packet should be forwarded
func ShouldForwardPacket(packet *RTPPacket) bool {
	// Check if this packet's SSRC has a registered forwarding destination
	rtpHandlersLock.RLock()
	_, hasHandler := rtpHandlers[packet.SSRC]
	rtpHandlersLock.RUnlock()

	return hasHandler
}

// ForwardRTPPacket forwards an RTP packet to its destination
func ForwardRTPPacket(packet *RTPPacket) error {
	// Get handler for this SSRC
	rtpHandlersLock.RLock()
	handler, exists := rtpHandlers[packet.SSRC]
	rtpHandlersLock.RUnlock()

	if !exists {
		return fmt.Errorf("no handler for SSRC %d", packet.SSRC)
	}

	// Use the registered handler to process the packet
	if err := handler.Handle(packet); err != nil {
		forwardingErrors.Add(1)
		return fmt.Errorf("handler failed for SSRC %d: %w", packet.SSRC, err)
	}

	return nil
}

// RTCPFeedbackHandler processes RTCP feedback messages
type RTCPFeedbackHandler struct {
	ssrc           uint32
	lastFeedback   time.Time
	packetLoss     float64
	jitter         float64
	rtt            float64
	mu             sync.RWMutex
	qualityMetrics prometheus.GaugeVec
}

// NewRTCPFeedbackHandler creates a feedback handler for a specific SSRC
func NewRTCPFeedbackHandler(ssrc uint32) *RTCPFeedbackHandler {
	// Create the handler with metrics
	handler := &RTCPFeedbackHandler{
		ssrc:         ssrc,
		lastFeedback: time.Now(),
		qualityMetrics: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "karl",
				Subsystem: "rtcp",
				Name:      "quality_metrics",
				Help:      "RTCP quality metrics (packet loss, jitter, RTT)",
			},
			[]string{"ssrc", "metric"},
		),
	}

	// Register with Prometheus
	prometheus.MustRegister(&handler.qualityMetrics)

	return handler
}

// HandleFeedback processes an RTCP feedback message
func (h *RTCPFeedbackHandler) HandleFeedback(packetLoss, jitter, rtt float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Update metrics
	h.packetLoss = packetLoss
	h.jitter = jitter
	h.rtt = rtt
	h.lastFeedback = time.Now()

	// Update Prometheus metrics
	ssrcStr := fmt.Sprintf("%d", h.ssrc)
	h.qualityMetrics.WithLabelValues(ssrcStr, "packet_loss").Set(packetLoss)
	h.qualityMetrics.WithLabelValues(ssrcStr, "jitter").Set(jitter)
	h.qualityMetrics.WithLabelValues(ssrcStr, "rtt").Set(rtt)

	// Implement congestion control based on feedback
	if packetLoss > 5.0 {
		// High packet loss - reduce bitrate
		log.Printf("⚠️ High packet loss (%.2f%%) for SSRC %d - reducing bitrate",
			packetLoss, h.ssrc)
		// In production would adjust encoder settings
	}
}

// RTCP feedback handlers registry
var (
	rtcpFeedbackHandlers = make(map[uint32]*RTCPFeedbackHandler)
	rtcpFeedbackMu       sync.RWMutex
)

// GetRTCPFeedbackHandler returns a handler for the given SSRC
func GetRTCPFeedbackHandler(ssrc uint32) *RTCPFeedbackHandler {
	rtcpFeedbackMu.RLock()
	handler, exists := rtcpFeedbackHandlers[ssrc]
	rtcpFeedbackMu.RUnlock()

	if exists {
		return handler
	}

	// Create a new handler if one doesn't exist
	rtcpFeedbackMu.Lock()
	defer rtcpFeedbackMu.Unlock()

	// Check again in case another goroutine created it
	handler, exists = rtcpFeedbackHandlers[ssrc]
	if exists {
		return handler
	}

	// Create a new handler
	handler = NewRTCPFeedbackHandler(ssrc)
	rtcpFeedbackHandlers[ssrc] = handler
	return handler
}

// HandleRTCPFeedback processes RTCP feedback for this RTP stream
func HandleRTCPFeedback(packet *RTPPacket) {
	// Get the feedback handler for this SSRC
	handler := GetRTCPFeedbackHandler(packet.SSRC)

	// For now, we're just using simple metrics based on packet sequence
	// In a real implementation, we'd parse actual RTCP packets with feedback

	// Calculate simple jitter metric based on packet arrival time
	jitter := 0.0

	// Process with the handler
	handler.HandleFeedback(0.0, jitter, 0.0)
}
