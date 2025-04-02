//go:build ignore

package main

import (
	"log"
	"net"
	"time"
)

func main() {
	// Create a test RTP packet
	// Format: RTP header (12 bytes) + payload
	testPacket := []byte{
		0x80, 0x00, 0x00, 0x01, // RTP header (version, padding, extension, CSRC count, marker, payload type)
		0x00, 0x00, 0x00, 0x01, // Timestamp
		0x00, 0x00, 0x00, 0x01, // SSRC
		// Additional payload
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10,
	}

	// Get UDP address
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:12000")
	if err != nil {
		log.Fatalf("Failed to resolve UDP address: %v", err)
	}

	// Create a UDP connection
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Fatalf("Failed to connect to UDP server: %v", err)
	}
	defer conn.Close()

	// Send multiple RTP packets
	for i := 0; i < 5; i++ {
		log.Printf("Sending RTP packet %d", i+1)
		_, err = conn.Write(testPacket)
		if err != nil {
			log.Printf("Failed to send packet: %v", err)
		}
		
		// Increment sequence number for each packet
		testPacket[2]++
		// Increment timestamp
		testPacket[7]++
		
		time.Sleep(100 * time.Millisecond)
	}

	log.Println("Test completed")
}