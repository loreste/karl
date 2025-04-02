//go:build ignore

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

const (
	rtpPort        = 12000
	metricsPort    = 9091
	testDuration   = 10 * time.Second
	packetInterval = 20 * time.Millisecond
)

// RTP packet structure for creating test packets
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
	Payload        []byte
}

// Encode an RTP packet to bytes
func (p *RTPPacket) Encode() []byte {
	buf := new(bytes.Buffer)

	// First byte: version (2 bits), padding (1 bit), extension (1 bit), CSRC count (4 bits)
	b := (p.Version << 6) & 0xC0
	if p.Padding {
		b |= 0x20
	}
	if p.Extension {
		b |= 0x10
	}
	b |= p.CSRCCount & 0x0F
	buf.WriteByte(b)

	// Second byte: marker (1 bit), payload type (7 bits)
	b = p.PayloadType & 0x7F
	if p.Marker {
		b |= 0x80
	}
	buf.WriteByte(b)

	// Sequence number (2 bytes)
	binary.Write(buf, binary.BigEndian, p.SequenceNumber)

	// Timestamp (4 bytes)
	binary.Write(buf, binary.BigEndian, p.Timestamp)

	// SSRC (4 bytes)
	binary.Write(buf, binary.BigEndian, p.SSRC)

	// Payload
	buf.Write(p.Payload)

	return buf.Bytes()
}

// startKarlServer starts the Karl server as a subprocess
func startKarlServer() (*exec.Cmd, error) {
	fmt.Println("Starting Karl Media Server...")
	cmd := exec.Command("./karl")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start Karl server: %w", err)
	}

	// Give it a moment to start up
	time.Sleep(2 * time.Second)
	fmt.Println("Karl server started")
	return cmd, nil
}

// stopKarlServer gracefully stops the Karl server
func stopKarlServer(cmd *exec.Cmd) {
	fmt.Println("Stopping Karl Media Server...")
	if cmd.Process != nil {
		cmd.Process.Signal(syscall.SIGTERM)
		cmd.Wait()
	}
	fmt.Println("Karl server stopped")
}

// sendRTPPackets sends test RTP packets to the Karl server
func sendRTPPackets(done chan struct{}) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", rtpPort))
	if err != nil {
		log.Fatalf("Failed to resolve UDP address: %v", err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Fatalf("Failed to connect to UDP server: %v", err)
	}
	defer conn.Close()

	fmt.Println("Sending RTP packets...")

	seq := uint16(1)
	timestamp := uint32(0)
	packetsSent := 0

	ticker := time.NewTicker(packetInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Create a test RTP packet
			packet := RTPPacket{
				Version:        2,
				Padding:        false,
				Extension:      false,
				CSRCCount:      0,
				Marker:         false,
				PayloadType:    0, // PCMU
				SequenceNumber: seq,
				Timestamp:      timestamp,
				SSRC:           0x12345678,
				Payload:        make([]byte, 160), // 20ms of PCMU at 8kHz
			}

			// Fill the payload with a sine wave pattern
			for i := range packet.Payload {
				packet.Payload[i] = byte(128 + 64*((i/2)%2))
			}

			// Encode and send the packet
			data := packet.Encode()
			if _, err := conn.Write(data); err != nil {
				log.Printf("Failed to send packet: %v", err)
				continue
			}

			packetsSent++
			seq++
			timestamp += 160 // 20ms at 8kHz

			if packetsSent%50 == 0 {
				fmt.Printf("Sent %d packets\n", packetsSent)
			}

		case <-done:
			fmt.Printf("Total packets sent: %d\n", packetsSent)
			return
		}
	}
}

// checkMetricsEndpoint verifies the metrics endpoint is working
func checkMetricsEndpoint() error {
	url := fmt.Sprintf("http://localhost:%d/metrics", metricsPort)
	
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to connect to metrics endpoint: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	
	if !bytes.Contains(body, []byte("karl_rtp_packets_total")) {
		return fmt.Errorf("metrics endpoint is not returning expected metrics")
	}
	
	fmt.Println("Metrics endpoint is working correctly")
	return nil
}

func RunE2ETest() {
	fmt.Println("Karl Media Server - End-to-End Test")
	fmt.Println("===================================")
	
	// Handle interrupts
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	
	// Start Karl server
	cmd, err := startKarlServer()
	if err != nil {
		log.Fatalf("Failed to start Karl server: %v", err)
	}
	
	// Ensure server is stopped on exit
	defer stopKarlServer(cmd)
	
	// Check metrics endpoint
	if err := checkMetricsEndpoint(); err != nil {
		log.Fatalf("Metrics check failed: %v", err)
	}
	
	// Channel to signal packet sender to stop
	done := make(chan struct{})
	
	// Start sending packets
	go sendRTPPackets(done)
	
	// Run the test for the specified duration or until interrupted
	select {
	case <-time.After(testDuration):
		fmt.Printf("Test completed after %s\n", testDuration)
	case <-sigChan:
		fmt.Println("Test interrupted")
	}
	
	// Signal packet sender to stop
	close(done)
	
	// Wait a moment for final packets to be processed
	time.Sleep(1 * time.Second)
	
	// Final metrics check
	fmt.Println("Checking final metrics...")
	if err := checkMetricsEndpoint(); err != nil {
		log.Printf("Final metrics check failed: %v", err)
	}
	
	fmt.Println("Test completed successfully")
}