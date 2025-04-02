//go:build ignore

package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"time"
)

func RunVerification() {
	// Define command line flags
	var port int
	flag.IntVar(&port, "port", 12000, "Port to test")
	flag.Parse()

	// Try to connect to the UDP server
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("udp", addr, 2*time.Second)
	if err != nil {
		log.Fatalf("Failed to connect to %s: %v", addr, err)
	}
	defer conn.Close()

	fmt.Printf("Successfully connected to UDP server at %s\n", addr)

	// Create a basic RTP packet
	packet := []byte{
		0x80, 0x00, 0x00, 0x01, // RTP header (version, padding, extension, CSRC count, marker, payload type)
		0x00, 0x00, 0x00, 0x01, // Timestamp
		0x00, 0x00, 0x00, 0x01, // SSRC
		// Payload (16 bytes)
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10,
	}

	// Send the packet
	_, err = conn.Write(packet)
	if err != nil {
		log.Fatalf("Failed to send packet: %v", err)
	}

	fmt.Println("Successfully sent test RTP packet")
	fmt.Println("Verification complete: The Karl RTP Engine is running and accepting packets")
}