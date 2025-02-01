package internal

import (
	"encoding/json"
	"log"
	"net"
	"os"
)

// RTPengineCommand represents an incoming command from OpenSIPS/Kamailio
type RTPengineCommand struct {
	Command   string `json:"command"`
	CallID    string `json:"call_id"`
	OfferSDP  string `json:"offer_sdp,omitempty"`
	AnswerSDP string `json:"answer_sdp,omitempty"`
}

// RTPengineResponse represents the response back to OpenSIPS/Kamailio
type RTPengineResponse struct {
	Result string `json:"result"`
	SDP    string `json:"sdp,omitempty"`
	Error  string `json:"error,omitempty"`
}

// StartRTPengineSocketListener listens for commands from OpenSIPS/Kamailio
func StartRTPengineSocketListener(socketPath string) {
	// Remove existing socket file if it exists
	if _, err := os.Stat(socketPath); err == nil {
		os.Remove(socketPath)
	}

	// Listen on Unix socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("❌ Failed to start RTPengine socket listener: %v", err)
	}
	defer listener.Close()

	log.Printf("📡 RTPengine socket listener started at %s", socketPath)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("❌ Error accepting connection: %v", err)
			continue
		}

		go handleRTPengineCommand(conn)
	}
}

// handleRTPengineCommand processes incoming RTPengine commands
func handleRTPengineCommand(conn net.Conn) {
	defer conn.Close()

	var command RTPengineCommand
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&command); err != nil {
		log.Printf("❌ Failed to parse RTPengine command: %v", err)
		sendErrorResponse(conn, "Invalid command format")
		return
	}

	log.Printf("📩 Received RTPengine command: %+v", command)

	switch command.Command {
	case "offer":
		handleOffer(conn, command)
	case "answer":
		handleAnswer(conn, command)
	default:
		sendErrorResponse(conn, "Unknown command")
	}
}

// handleOffer processes an SDP offer from OpenSIPS/Kamailio
func handleOffer(conn net.Conn, command RTPengineCommand) {
	log.Printf("🔄 Processing SDP Offer for Call ID: %s", command.CallID)

	// Modify SDP (handle NAT, SRTP settings, etc.)
	modifiedSDP := processSDP(command.OfferSDP)

	response := RTPengineResponse{
		Result: "ok",
		SDP:    modifiedSDP,
	}

	sendResponse(conn, response)
}

// handleAnswer processes an SDP answer from OpenSIPS/Kamailio
func handleAnswer(conn net.Conn, command RTPengineCommand) {
	log.Printf("🔄 Processing SDP Answer for Call ID: %s", command.CallID)

	// Modify SDP for RTP forwarding
	modifiedSDP := processSDP(command.AnswerSDP)

	response := RTPengineResponse{
		Result: "ok",
		SDP:    modifiedSDP,
	}

	sendResponse(conn, response)
}

// processSDP modifies SDP based on NAT and SRTP settings
func processSDP(sdp string) string {
	// Implement SDP modifications as needed
	return sdp
}

// sendResponse sends a JSON response back to OpenSIPS/Kamailio
func sendResponse(conn net.Conn, response RTPengineResponse) {
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(response); err != nil {
		log.Printf("❌ Failed to send RTPengine response: %v", err)
	}
}

// sendErrorResponse sends an error response
func sendErrorResponse(conn net.Conn, errorMsg string) {
	response := RTPengineResponse{
		Result: "error",
		Error:  errorMsg,
	}
	sendResponse(conn, response)
}
