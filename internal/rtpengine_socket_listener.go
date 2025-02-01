package internal

import (
	"log"
	"net"
	"os"
)

// RTPengineSocketListener listens for commands from OpenSIPS/Kamailio
type RTPengineSocketListener struct {
	socketPath string
	listener   net.Listener
}

// NewRTPengineSocketListener initializes a new Unix socket listener
func NewRTPengineSocketListener(socketPath string) *RTPengineSocketListener {
	return &RTPengineSocketListener{socketPath: socketPath}
}

// Start begins listening for RTP commands
func (r *RTPengineSocketListener) Start() error {
	// Ensure no existing socket
	if _, err := os.Stat(r.socketPath); err == nil {
		os.Remove(r.socketPath)
	}

	// Start listening on a Unix socket
	listener, err := net.Listen("unix", r.socketPath)
	if err != nil {
		log.Fatalf("❌ Failed to start RTPengine socket: %v", err)
		return err
	}

	r.listener = listener
	log.Printf("✅ RTPengine socket listening at %s", r.socketPath)

	go r.handleConnections()
	return nil
}

// Stop stops the listener
func (r *RTPengineSocketListener) Stop() {
	if r.listener != nil {
		r.listener.Close()
		log.Println("🛑 RTPengine socket listener stopped.")
	}
}

// handleConnections processes incoming commands from SIP proxies
func (r *RTPengineSocketListener) handleConnections() {
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			log.Printf("❌ Error accepting connection: %v", err)
			continue
		}

		go r.handleCommand(conn)
	}
}

// handleCommand processes SIP/RTP commands
func (r *RTPengineSocketListener) handleCommand(conn net.Conn) {
	defer conn.Close()

	// Example: Read command from SIP proxy
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		log.Printf("❌ Error reading from RTPengine socket: %v", err)
		return
	}

	command := string(buffer[:n])
	log.Printf("📡 Received RTP command: %s", command)

	// Example: Send response
	conn.Write([]byte("OK\n"))
}
