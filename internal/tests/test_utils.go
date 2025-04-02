package tests

import (
	"log"
	"net"
	"time"
)

// CreateTestSRTPKeys creates valid SRTP key and salt for testing
func CreateTestSRTPKeys() ([]byte, []byte) {
	// SRTP requires specific key lengths
	srtpKey := make([]byte, 16)  // 16-byte key
	srtpSalt := make([]byte, 14) // 14-byte salt

	// Fill with test data
	for i := range srtpKey {
		srtpKey[i] = byte(i)
	}
	for i := range srtpSalt {
		srtpSalt[i] = byte(i + 100)
	}

	return srtpKey, srtpSalt
}

// CreateTestRTPPacket creates a valid RTP packet for testing
func CreateTestRTPPacket(ssrc uint32, seqNum uint16, ts uint32, payload []byte) []byte {
	// Create minimal RTP header + payload
	header := []byte{
		0x80, byte(seqNum >> 8), byte(seqNum),
		byte(ts >> 24), byte(ts >> 16), byte(ts >> 8), byte(ts),
		byte(ssrc >> 24), byte(ssrc >> 16), byte(ssrc >> 8), byte(ssrc),
	}

	// Combine header and payload
	packet := make([]byte, len(header)+len(payload))
	copy(packet, header)
	copy(packet[len(header):], payload)

	return packet
}

// CreateTestUDPListener creates a temporary UDP listener for testing
func CreateTestUDPListener() (*net.UDPConn, string, error) {
	// Listen on a random port
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, "", err
	}

	// Get the assigned address
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	addrStr := localAddr.String()

	log.Printf("Created test UDP listener on %s", addrStr)
	return conn, addrStr, nil
}

// WaitForCondition polls a condition function until it returns true or times out
func WaitForCondition(condition func() bool, timeout time.Duration) bool {
	start := time.Now()
	for {
		if condition() {
			return true
		}

		if time.Since(start) > timeout {
			return false
		}

		time.Sleep(50 * time.Millisecond)
	}
}
