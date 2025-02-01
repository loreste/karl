package internal

import (
	"crypto/tls"
	"log"
	"net"
	"sync"
)

// RTP Transport settings
var (
	rtpListeners = make(map[string]net.Listener)
	rtpMutex     sync.Mutex
)

// StartRTPUDPListener starts a UDP listener for RTP traffic
func StartRTPUDPListener(address string) {
	conn, err := net.ListenPacket("udp", address)
	if err != nil {
		log.Fatalf("Failed to start UDP RTP listener: %v", err)
	}
	defer conn.Close()

	log.Println("RTP UDP listener started on", address)

	buf := make([]byte, 1500)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			log.Println("UDP RTP read error:", err)
			continue
		}

		// Handle incoming RTP packet
		go handleRTPPacket(buf[:n], addr)
	}
}

// StartRTPTCPListener starts a TCP listener for RTP traffic
func StartRTPTCPListener(address string) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("Failed to start TCP RTP listener: %v", err)
	}
	defer listener.Close()

	rtpMutex.Lock()
	rtpListeners[address] = listener
	rtpMutex.Unlock()

	log.Println("RTP TCP listener started on", address)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("TCP RTP accept error:", err)
			continue
		}
		go handleRTPStream(conn)
	}
}

// StartRTPTLSListener starts a TLS listener for encrypted RTP traffic
func StartRTPTLSListener(address, certFile, keyFile string) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Fatalf("Failed to load TLS certificate: %v", err)
	}

	config := &tls.Config{Certificates: []tls.Certificate{cert}}

	listener, err := tls.Listen("tcp", address, config)
	if err != nil {
		log.Fatalf("Failed to start TLS RTP listener: %v", err)
	}
	defer listener.Close()

	rtpMutex.Lock()
	rtpListeners[address] = listener
	rtpMutex.Unlock()

	log.Println("RTP TLS listener started on", address)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("TLS RTP accept error:", err)
			continue
		}
		go handleRTPStream(conn)
	}
}

// handleRTPPacket processes incoming RTP packets
func handleRTPPacket(packet []byte, addr net.Addr) {
	// Capture RTP packets for debugging if PCAP logging is enabled
	CapturePacket(packet)

	// Process RTP packet (this can include transcoding, forwarding, etc.)
	log.Printf("Received RTP packet from %s, size: %d bytes", addr.String(), len(packet))
}

// handleRTPStream handles incoming RTP streams over TCP/TLS
func handleRTPStream(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 1500)

	for {
		n, err := conn.Read(buf)
		if err != nil {
			log.Println("RTP stream read error:", err)
			break
		}

		// Capture RTP packets for debugging if PCAP logging is enabled
		CapturePacket(buf[:n])

		// Process RTP stream packet
		log.Printf("Received RTP stream packet, size: %d bytes", n)
	}
}

// StopRTPListener stops an active RTP listener
func StopRTPListener(address string) {
	rtpMutex.Lock()
	defer rtpMutex.Unlock()

	if listener, exists := rtpListeners[address]; exists {
		listener.Close()
		delete(rtpListeners, address)
		log.Printf("Stopped RTP listener on %s", address)
	}
}
