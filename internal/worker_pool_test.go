package internal

import (
	"encoding/binary"
	"sync"
	"testing"
	"time"
)

func TestParseRTPPacket_Valid(t *testing.T) {
	// Create a valid RTP packet
	// V=2, P=0, X=0, CC=0, M=0, PT=0, Seq=1234, TS=5678, SSRC=0xDEADBEEF
	packet := make([]byte, 172) // 12 byte header + 160 byte payload
	packet[0] = 0x80            // V=2, P=0, X=0, CC=0
	packet[1] = 0               // M=0, PT=0 (PCMU)
	binary.BigEndian.PutUint16(packet[2:4], 1234)
	binary.BigEndian.PutUint32(packet[4:8], 5678)
	binary.BigEndian.PutUint32(packet[8:12], 0xDEADBEEF)

	// Fill payload
	for i := 12; i < 172; i++ {
		packet[i] = byte(i % 256)
	}

	rtpPacket, err := ParseRTPPacket(packet)
	if err != nil {
		t.Fatalf("ParseRTPPacket failed: %v", err)
	}

	if rtpPacket.Version != 2 {
		t.Errorf("Expected version 2, got %d", rtpPacket.Version)
	}
	if rtpPacket.Padding {
		t.Error("Expected no padding")
	}
	if rtpPacket.Extension {
		t.Error("Expected no extension")
	}
	if rtpPacket.CSRCCount != 0 {
		t.Errorf("Expected CSRC count 0, got %d", rtpPacket.CSRCCount)
	}
	if rtpPacket.Marker {
		t.Error("Expected no marker")
	}
	if rtpPacket.PayloadType != 0 {
		t.Errorf("Expected payload type 0, got %d", rtpPacket.PayloadType)
	}
	if rtpPacket.SequenceNumber != 1234 {
		t.Errorf("Expected sequence number 1234, got %d", rtpPacket.SequenceNumber)
	}
	if rtpPacket.Timestamp != 5678 {
		t.Errorf("Expected timestamp 5678, got %d", rtpPacket.Timestamp)
	}
	if rtpPacket.SSRC != 0xDEADBEEF {
		t.Errorf("Expected SSRC 0xDEADBEEF, got 0x%X", rtpPacket.SSRC)
	}
	if len(rtpPacket.Payload) != 160 {
		t.Errorf("Expected payload length 160, got %d", len(rtpPacket.Payload))
	}
}

func TestParseRTPPacket_WithMarker(t *testing.T) {
	packet := make([]byte, 12)
	packet[0] = 0x80
	packet[1] = 0x80 | 8 // Marker=1, PT=8 (PCMA)
	binary.BigEndian.PutUint16(packet[2:4], 100)
	binary.BigEndian.PutUint32(packet[4:8], 200)
	binary.BigEndian.PutUint32(packet[8:12], 0x12345678)

	rtpPacket, err := ParseRTPPacket(packet)
	if err != nil {
		t.Fatalf("ParseRTPPacket failed: %v", err)
	}

	if !rtpPacket.Marker {
		t.Error("Expected marker bit to be set")
	}
	if rtpPacket.PayloadType != 8 {
		t.Errorf("Expected payload type 8, got %d", rtpPacket.PayloadType)
	}
}

func TestParseRTPPacket_WithPadding(t *testing.T) {
	// Create packet with padding
	packet := make([]byte, 20)
	packet[0] = 0xA0 // V=2, P=1, X=0, CC=0
	packet[1] = 0
	binary.BigEndian.PutUint16(packet[2:4], 1)
	binary.BigEndian.PutUint32(packet[4:8], 1)
	binary.BigEndian.PutUint32(packet[8:12], 1)

	// Add 4 bytes payload + 4 bytes padding (last byte indicates padding length)
	packet[12] = 0x01
	packet[13] = 0x02
	packet[14] = 0x03
	packet[15] = 0x04
	packet[16] = 0x00
	packet[17] = 0x00
	packet[18] = 0x00
	packet[19] = 0x04 // 4 bytes of padding

	rtpPacket, err := ParseRTPPacket(packet)
	if err != nil {
		t.Fatalf("ParseRTPPacket failed: %v", err)
	}

	if !rtpPacket.Padding {
		t.Error("Expected padding flag to be set")
	}
	// Payload should be 4 bytes (excluding padding)
	if len(rtpPacket.Payload) != 4 {
		t.Errorf("Expected payload length 4, got %d", len(rtpPacket.Payload))
	}
}

func TestParseRTPPacket_WithCSRC(t *testing.T) {
	// Create packet with 2 CSRC entries
	packet := make([]byte, 20+8) // 12 header + 8 CSRC + payload
	packet[0] = 0x82             // V=2, P=0, X=0, CC=2
	packet[1] = 0
	binary.BigEndian.PutUint16(packet[2:4], 1)
	binary.BigEndian.PutUint32(packet[4:8], 1)
	binary.BigEndian.PutUint32(packet[8:12], 0xAAAAAAAA)

	// CSRC entries
	binary.BigEndian.PutUint32(packet[12:16], 0x11111111)
	binary.BigEndian.PutUint32(packet[16:20], 0x22222222)

	rtpPacket, err := ParseRTPPacket(packet)
	if err != nil {
		t.Fatalf("ParseRTPPacket failed: %v", err)
	}

	if rtpPacket.CSRCCount != 2 {
		t.Errorf("Expected CSRC count 2, got %d", rtpPacket.CSRCCount)
	}
	if len(rtpPacket.CSRC) != 2 {
		t.Fatalf("Expected 2 CSRC entries, got %d", len(rtpPacket.CSRC))
	}
	if rtpPacket.CSRC[0] != 0x11111111 {
		t.Errorf("Expected first CSRC 0x11111111, got 0x%X", rtpPacket.CSRC[0])
	}
	if rtpPacket.CSRC[1] != 0x22222222 {
		t.Errorf("Expected second CSRC 0x22222222, got 0x%X", rtpPacket.CSRC[1])
	}
}

func TestParseRTPPacket_WithExtension(t *testing.T) {
	// Create packet with extension header
	packet := make([]byte, 24)
	packet[0] = 0x90 // V=2, P=0, X=1, CC=0
	packet[1] = 0
	binary.BigEndian.PutUint16(packet[2:4], 1)
	binary.BigEndian.PutUint32(packet[4:8], 1)
	binary.BigEndian.PutUint32(packet[8:12], 0xBBBBBBBB)

	// Extension header: profile (2 bytes) + length in 32-bit words (2 bytes)
	binary.BigEndian.PutUint16(packet[12:14], 0xBEDE) // Profile
	binary.BigEndian.PutUint16(packet[14:16], 1)      // Length = 1 word (4 bytes)

	// Extension data (4 bytes)
	packet[16] = 0xAA
	packet[17] = 0xBB
	packet[18] = 0xCC
	packet[19] = 0xDD

	// Payload
	packet[20] = 0x01
	packet[21] = 0x02
	packet[22] = 0x03
	packet[23] = 0x04

	rtpPacket, err := ParseRTPPacket(packet)
	if err != nil {
		t.Fatalf("ParseRTPPacket failed: %v", err)
	}

	if !rtpPacket.Extension {
		t.Error("Expected extension flag to be set")
	}
	if len(rtpPacket.ExtensionData) != 4 {
		t.Errorf("Expected extension data length 4, got %d", len(rtpPacket.ExtensionData))
	}
	if len(rtpPacket.Payload) != 4 {
		t.Errorf("Expected payload length 4, got %d", len(rtpPacket.Payload))
	}
}

func TestParseRTPPacket_TooShort(t *testing.T) {
	// Packet too short for RTP header
	packet := make([]byte, 8)

	_, err := ParseRTPPacket(packet)
	if err == nil {
		t.Error("Expected error for packet too short")
	}
}

func TestParseRTPPacket_TooShortForCSRC(t *testing.T) {
	// Packet claims 2 CSRC but is too short
	packet := make([]byte, 14)
	packet[0] = 0x82 // CC=2 would need 20 bytes minimum
	packet[1] = 0
	binary.BigEndian.PutUint16(packet[2:4], 1)
	binary.BigEndian.PutUint32(packet[4:8], 1)
	binary.BigEndian.PutUint32(packet[8:12], 1)

	_, err := ParseRTPPacket(packet)
	if err == nil {
		t.Error("Expected error for packet too short for CSRC")
	}
}

func TestParseRTPPacket_TooShortForExtension(t *testing.T) {
	// Packet claims extension but is too short
	packet := make([]byte, 14)
	packet[0] = 0x90 // X=1 but no room for extension header
	packet[1] = 0
	binary.BigEndian.PutUint16(packet[2:4], 1)
	binary.BigEndian.PutUint32(packet[4:8], 1)
	binary.BigEndian.PutUint32(packet[8:12], 1)

	_, err := ParseRTPPacket(packet)
	if err == nil {
		t.Error("Expected error for packet too short for extension")
	}
}

func TestRTPHandlerRegistry(t *testing.T) {
	// Clean up any existing handlers
	rtpHandlersLock.Lock()
	rtpHandlers = make(map[uint32]RTPPacketHandler)
	rtpHandlersLock.Unlock()

	// Create a mock handler
	handler := &mockRTPHandler{handleCalled: false}

	// Register handler
	RegisterRTPHandler(0x12345678, handler)

	// Verify handler is registered
	rtpHandlersLock.RLock()
	_, exists := rtpHandlers[0x12345678]
	rtpHandlersLock.RUnlock()

	if !exists {
		t.Error("Handler should be registered")
	}

	// Unregister handler
	UnregisterRTPHandler(0x12345678)

	// Verify handler is removed
	rtpHandlersLock.RLock()
	_, exists = rtpHandlers[0x12345678]
	rtpHandlersLock.RUnlock()

	if exists {
		t.Error("Handler should be unregistered")
	}
}

type mockRTPHandler struct {
	handleCalled bool
	lastPacket   *RTPPacket
	mu           sync.Mutex
}

func (h *mockRTPHandler) Handle(packet *RTPPacket) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handleCalled = true
	h.lastPacket = packet
	return nil
}

func TestShouldTranscodePacket(t *testing.T) {
	tests := []struct {
		payloadType uint8
		expected    bool
		desc        string
	}{
		{0, true, "PCMU should transcode"},
		{8, true, "PCMA should transcode"},
		{111, true, "Opus dynamic PT should transcode"},
		{96, true, "Dynamic PT 96 should transcode"},
		{13, false, "CN (comfort noise) should not transcode"},
		{18, false, "G.729 should not transcode"},
		{3, false, "GSM should not transcode"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			packet := &RTPPacket{PayloadType: tt.payloadType}
			result := ShouldTranscodePacket(packet)
			if result != tt.expected {
				t.Errorf("ShouldTranscodePacket(PT=%d) = %v, expected %v",
					tt.payloadType, result, tt.expected)
			}
		})
	}
}

func TestShouldForwardPacket(t *testing.T) {
	// Clean up handlers
	rtpHandlersLock.Lock()
	rtpHandlers = make(map[uint32]RTPPacketHandler)
	rtpHandlersLock.Unlock()

	// Create packets
	packetWithHandler := &RTPPacket{SSRC: 0xAAAAAAAA}
	packetWithoutHandler := &RTPPacket{SSRC: 0xBBBBBBBB}

	// Register handler for one SSRC
	RegisterRTPHandler(0xAAAAAAAA, &mockRTPHandler{})

	if !ShouldForwardPacket(packetWithHandler) {
		t.Error("Should forward packet with registered handler")
	}

	if ShouldForwardPacket(packetWithoutHandler) {
		t.Error("Should not forward packet without handler")
	}

	// Cleanup
	UnregisterRTPHandler(0xAAAAAAAA)
}

func TestForwardRTPPacket(t *testing.T) {
	// Clean up handlers
	rtpHandlersLock.Lock()
	rtpHandlers = make(map[uint32]RTPPacketHandler)
	rtpHandlersLock.Unlock()

	// Create mock handler
	handler := &mockRTPHandler{}
	RegisterRTPHandler(0xCCCCCCCC, handler)

	// Create packet
	packet := &RTPPacket{
		SSRC:           0xCCCCCCCC,
		SequenceNumber: 100,
		Timestamp:      1000,
	}

	// Forward packet
	err := ForwardRTPPacket(packet)
	if err != nil {
		t.Errorf("ForwardRTPPacket failed: %v", err)
	}

	handler.mu.Lock()
	if !handler.handleCalled {
		t.Error("Handler should have been called")
	}
	if handler.lastPacket.SequenceNumber != 100 {
		t.Error("Handler received wrong packet")
	}
	handler.mu.Unlock()

	// Cleanup
	UnregisterRTPHandler(0xCCCCCCCC)
}

func TestForwardRTPPacket_NoHandler(t *testing.T) {
	// Clean up handlers
	rtpHandlersLock.Lock()
	rtpHandlers = make(map[uint32]RTPPacketHandler)
	rtpHandlersLock.Unlock()

	packet := &RTPPacket{SSRC: 0xDDDDDDDD}

	err := ForwardRTPPacket(packet)
	if err == nil {
		t.Error("Expected error when no handler registered")
	}
}

func TestGetMetrics(t *testing.T) {
	metrics := GetMetrics()

	expectedKeys := []string{
		"packets_processed",
		"packet_errors",
		"bytes_processed",
		"transcoding_errors",
		"forwarding_errors",
	}

	for _, key := range expectedKeys {
		if _, exists := metrics[key]; !exists {
			t.Errorf("Expected metric key '%s' not found", key)
		}
	}
}

func TestGetWorkerPoolMetrics(t *testing.T) {
	// GetWorkerPoolMetrics should return same as GetMetrics
	metrics1 := GetMetrics()
	metrics2 := GetWorkerPoolMetrics()

	for key := range metrics1 {
		if metrics2[key] != metrics1[key] {
			t.Errorf("Metric mismatch for '%s': %d vs %d", key, metrics1[key], metrics2[key])
		}
	}
}

func TestDebugLogging(t *testing.T) {
	// Test enable/disable
	originalState := IsDebugLoggingEnabled()

	EnableDebugLogging(true)
	if !IsDebugLoggingEnabled() {
		t.Error("Debug logging should be enabled")
	}

	EnableDebugLogging(false)
	if IsDebugLoggingEnabled() {
		t.Error("Debug logging should be disabled")
	}

	// Restore original state
	EnableDebugLogging(originalState)
}

func TestAddRTPJob_NonBlocking(t *testing.T) {
	// Create a fresh channel for testing
	oldRtpJobs := rtpJobs
	rtpJobs = make(chan []byte, 10)
	defer func() { rtpJobs = oldRtpJobs }()

	// Add a few packets
	for i := 0; i < 5; i++ {
		packet := make([]byte, 12)
		packet[0] = 0x80
		binary.BigEndian.PutUint32(packet[8:12], uint32(i))
		AddRTPJob(packet)
	}

	// Verify packets were queued
	if len(rtpJobs) != 5 {
		t.Errorf("Expected 5 packets in queue, got %d", len(rtpJobs))
	}

	// Drain the channel
	for len(rtpJobs) > 0 {
		<-rtpJobs
	}
}

func TestAddRTPJob_PacketCopy(t *testing.T) {
	// Test that AddRTPJob creates a copy of the packet
	oldRtpJobs := rtpJobs
	rtpJobs = make(chan []byte, 10)
	defer func() { rtpJobs = oldRtpJobs }()

	packet := make([]byte, 12)
	packet[0] = 0x80
	packet[11] = 0xFF

	AddRTPJob(packet)

	// Modify original packet
	packet[11] = 0x00

	// Verify queued packet has original value
	queued := <-rtpJobs
	if queued[11] != 0xFF {
		t.Error("AddRTPJob should copy packet, not reference it")
	}
}

func TestRTCPFeedbackHandler_BasicFields(t *testing.T) {
	// Test basic handler struct fields without prometheus metrics
	handler := &RTCPFeedbackHandler{
		ssrc:         0x12345678,
		lastFeedback: time.Now(),
		packetLoss:   0.0,
		jitter:       0.0,
		rtt:          0.0,
	}

	// Test direct field updates (avoiding HandleFeedback which uses prometheus)
	handler.mu.Lock()
	handler.packetLoss = 2.5
	handler.jitter = 10.0
	handler.rtt = 50.0
	handler.lastFeedback = time.Now()
	handler.mu.Unlock()

	handler.mu.RLock()
	defer handler.mu.RUnlock()

	if handler.packetLoss != 2.5 {
		t.Errorf("Expected packet loss 2.5, got %f", handler.packetLoss)
	}
	if handler.jitter != 10.0 {
		t.Errorf("Expected jitter 10.0, got %f", handler.jitter)
	}
	if handler.rtt != 50.0 {
		t.Errorf("Expected RTT 50.0, got %f", handler.rtt)
	}
	if handler.lastFeedback.IsZero() {
		t.Error("lastFeedback should be set")
	}
	if handler.ssrc != 0x12345678 {
		t.Errorf("Expected SSRC 0x12345678, got 0x%X", handler.ssrc)
	}
}

func TestRTPPacket_AllPayloadTypes(t *testing.T) {
	// Test parsing with various payload types
	payloadTypes := []uint8{0, 3, 4, 8, 9, 13, 18, 96, 97, 100, 111, 127}

	for _, pt := range payloadTypes {
		t.Run("PT"+string(rune('0'+pt%10)), func(t *testing.T) {
			packet := make([]byte, 12)
			packet[0] = 0x80
			packet[1] = pt

			rtpPacket, err := ParseRTPPacket(packet)
			if err != nil {
				t.Fatalf("Failed to parse packet with PT %d: %v", pt, err)
			}
			if rtpPacket.PayloadType != pt {
				t.Errorf("Expected PT %d, got %d", pt, rtpPacket.PayloadType)
			}
		})
	}
}

func TestRTPPacket_EmptyPayload(t *testing.T) {
	// Minimum valid RTP packet (header only)
	packet := make([]byte, 12)
	packet[0] = 0x80
	packet[1] = 0

	rtpPacket, err := ParseRTPPacket(packet)
	if err != nil {
		t.Fatalf("ParseRTPPacket failed: %v", err)
	}

	if len(rtpPacket.Payload) != 0 {
		t.Errorf("Expected empty payload, got %d bytes", len(rtpPacket.Payload))
	}
}

func TestRTPPacket_ReceivedTime(t *testing.T) {
	packet := make([]byte, 12)
	packet[0] = 0x80

	before := time.Now()
	rtpPacket, err := ParseRTPPacket(packet)
	if err != nil {
		t.Fatalf("ParseRTPPacket failed: %v", err)
	}
	after := time.Now()

	if rtpPacket.Received.Before(before) || rtpPacket.Received.After(after) {
		t.Error("Received time should be between before and after")
	}
}

func TestConcurrentHandlerAccess(t *testing.T) {
	// Clean up handlers
	rtpHandlersLock.Lock()
	rtpHandlers = make(map[uint32]RTPPacketHandler)
	rtpHandlersLock.Unlock()

	// Test concurrent register/unregister operations
	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ssrc := uint32(id)
			handler := &mockRTPHandler{}

			// Register
			RegisterRTPHandler(ssrc, handler)

			// Check
			rtpHandlersLock.RLock()
			_, exists := rtpHandlers[ssrc]
			rtpHandlersLock.RUnlock()

			if !exists {
				t.Errorf("Handler %d not found after registration", id)
			}

			// Unregister
			UnregisterRTPHandler(ssrc)
		}(i)
	}

	wg.Wait()
}

func TestConcurrentPacketParsing(t *testing.T) {
	// Test concurrent parsing
	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			packet := make([]byte, 172)
			packet[0] = 0x80
			packet[1] = byte(id % 128)
			binary.BigEndian.PutUint16(packet[2:4], uint16(id))
			binary.BigEndian.PutUint32(packet[4:8], uint32(id*160))
			binary.BigEndian.PutUint32(packet[8:12], uint32(id))

			rtpPacket, err := ParseRTPPacket(packet)
			if err != nil {
				t.Errorf("Goroutine %d: ParseRTPPacket failed: %v", id, err)
				return
			}
			if rtpPacket.SequenceNumber != uint16(id) {
				t.Errorf("Goroutine %d: wrong sequence number", id)
			}
		}(i)
	}

	wg.Wait()
}
