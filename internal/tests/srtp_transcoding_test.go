package tests

import (
	"karl/internal"
	"testing"
)

func TestSRTPTranscoder(t *testing.T) {
	// Create valid SRTP key and salt (using correct binary length)
	srtpKey := make([]byte, 16) // 16-byte key
	srtpSalt := make([]byte, 14) // 14-byte salt

	// Create a new SRTP transcoder
	transcoder, err := internal.NewSRTPTranscoder(srtpKey, srtpSalt)
	if err != nil {
		t.Fatalf("Failed to create SRTP transcoder: %v", err)
	}

	// Check that the context was created
	if transcoder.Context == nil {
		t.Fatalf("SRTP context is nil")
	}

	// Create a test RTP packet
	testPacket := []byte{
		0x80, 0x00, 0x00, 0x01, // RTP header (version, padding, extension, CSRC count, marker, payload type)
		0x00, 0x00, 0x00, 0x01, // Timestamp
		0x00, 0x00, 0x00, 0x01, // SSRC
		0x01, 0x02, 0x03, 0x04, // Payload
	}

	// Transcode RTP to SRTP
	srtpPacket, err := transcoder.TranscodeRTPToSRTP(testPacket)
	if err != nil {
		t.Fatalf("Failed to transcode RTP to SRTP: %v", err)
	}

	// Check that the packet was encrypted
	if len(srtpPacket) <= len(testPacket) {
		t.Errorf("Expected SRTP packet to be longer than RTP packet, got %d <= %d", len(srtpPacket), len(testPacket))
	}

	// Test GetContext
	ctx := transcoder.GetContext()
	if ctx == nil {
		t.Errorf("GetContext returned nil")
	}
}