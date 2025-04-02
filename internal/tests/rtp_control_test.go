package tests

import (
	"karl/internal"
	"testing"
)

func TestRTPControl(t *testing.T) {
	// Create valid SRTP key and salt (using correct binary length)
	srtpKey := make([]byte, 16) // 16-byte key
	srtpSalt := make([]byte, 14) // 14-byte salt

	// Create a new RTP control
	rtpControl, err := internal.NewRTPControl(srtpKey, srtpSalt)
	if err != nil {
		t.Fatalf("Failed to create RTP control: %v", err)
	}

	// Test adding a destination with random port
	testAddr := "127.0.0.1:5000"
	err = rtpControl.AddDestination(testAddr)
	if err != nil {
		t.Fatalf("Failed to add destination: %v", err)
	}

	// Test removing a destination
	rtpControl.RemoveDestination(testAddr)
	
	// Verify stats are initialized to zero
	packetsReceived, packetsDropped, bytesReceived, bytesSent := rtpControl.GetStats()
	if packetsReceived != 0 || packetsDropped != 0 || bytesReceived != 0 || bytesSent != 0 {
		t.Errorf("Expected all stats to be zero initially")
	}

	// Stop the RTP control
	rtpControl.Stop()
}