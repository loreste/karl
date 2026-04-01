package ng_protocol

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// MockNGServer provides a mock NG protocol server for testing
type MockNGServer struct {
	conn     *net.UDPConn
	addr     *net.UDPAddr
	sessions map[string]*MockSession
	mu       sync.RWMutex
	running  bool
	stopCh   chan struct{}
	wg       sync.WaitGroup

	// Counters for testing
	offerCount  int
	answerCount int
	deleteCount int
	queryCount  int
}

// MockSession represents a mock call session
type MockSession struct {
	CallID       string
	FromTag      string
	ToTag        string
	SDP          string
	State        string
	Created      time.Time
	Recording    bool
	MediaBlocked bool
	Labels       map[string]string
}

// NewMockNGServer creates a new mock server
func NewMockNGServer(port int) (*MockNGServer, error) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	// Get the actual address (important when port=0)
	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return &MockNGServer{
		conn:     conn,
		addr:     localAddr,
		sessions: make(map[string]*MockSession),
		stopCh:   make(chan struct{}),
	}, nil
}

// Start starts the mock server
func (s *MockNGServer) Start() {
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	s.wg.Add(1)
	go s.serve()
}

// Stop stops the mock server
func (s *MockNGServer) Stop() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)
	s.conn.Close()
	s.wg.Wait()
}

// Addr returns the server address
func (s *MockNGServer) Addr() string {
	return s.addr.String()
}

// serve handles incoming requests
func (s *MockNGServer) serve() {
	defer s.wg.Done()

	buf := make([]byte, 65536)
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		s.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, remoteAddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}

		response := s.handleRequest(buf[:n])
		s.conn.WriteToUDP(response, remoteAddr)
	}
}

// handleRequest processes a single request
func (s *MockNGServer) handleRequest(data []byte) []byte {
	msg, err := ParseMessage(data, nil)
	if err != nil {
		return s.errorResponse(string(data[:min(32, len(data))]), "parse error")
	}

	command := DictGetString(msg.Data, "command")
	switch command {
	case "ping":
		return s.handlePing(msg)
	case "offer":
		return s.handleOffer(msg)
	case "answer":
		return s.handleAnswer(msg)
	case "delete":
		return s.handleDelete(msg)
	case "query":
		return s.handleQuery(msg)
	case "list":
		return s.handleList(msg)
	case "start recording":
		return s.handleStartRecording(msg)
	case "stop recording":
		return s.handleStopRecording(msg)
	case "block media":
		return s.handleBlockMedia(msg)
	case "unblock media":
		return s.handleUnblockMedia(msg)
	case "play DTMF":
		return s.handlePlayDTMF(msg)
	default:
		return s.errorResponse(msg.Cookie, "unknown command")
	}
}

func (s *MockNGServer) handlePing(msg *NGMessage) []byte {
	resp, _ := BuildResponse(msg.Cookie, &NGResponse{
		Result: ResultPong,
	})
	return resp
}

func (s *MockNGServer) handleOffer(msg *NGMessage) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	callID := DictGetString(msg.Data, "call-id")
	fromTag := DictGetString(msg.Data, "from-tag")
	sdp := DictGetString(msg.Data, "sdp")

	if callID == "" {
		return s.errorResponse(msg.Cookie, "missing call-id")
	}
	if fromTag == "" {
		return s.errorResponse(msg.Cookie, "missing from-tag")
	}

	sessionKey := callID + ":" + fromTag
	session, exists := s.sessions[sessionKey]
	if !exists {
		session = &MockSession{
			CallID:  callID,
			FromTag: fromTag,
			SDP:     sdp,
			State:   "offer",
			Created: time.Now(),
			Labels:  make(map[string]string),
		}
		s.sessions[sessionKey] = session
	} else {
		session.SDP = sdp
	}

	// Handle label
	if label := DictGetString(msg.Data, "label"); label != "" {
		session.Labels[fromTag] = label
	}

	s.offerCount++

	// Generate response SDP
	responseSDP := s.generateResponseSDP(sdp, msg.Data)

	resp, _ := BuildResponse(msg.Cookie, &NGResponse{
		Result: ResultOK,
		SDP:    responseSDP,
		Streams: []StreamInfo{
			{
				LocalIP:       "127.0.0.1",
				LocalPort:     30000 + s.offerCount*2,
				LocalRTCPPort: 30001 + s.offerCount*2,
				MediaType:     "audio",
				Protocol:      "RTP/AVP",
				Index:         0,
			},
		},
	})
	return resp
}

func (s *MockNGServer) handleAnswer(msg *NGMessage) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	callID := DictGetString(msg.Data, "call-id")
	fromTag := DictGetString(msg.Data, "from-tag")
	toTag := DictGetString(msg.Data, "to-tag")
	sdp := DictGetString(msg.Data, "sdp")

	if callID == "" {
		return s.errorResponse(msg.Cookie, "missing call-id")
	}
	if fromTag == "" {
		return s.errorResponse(msg.Cookie, "missing from-tag")
	}

	sessionKey := callID + ":" + fromTag
	session, exists := s.sessions[sessionKey]
	if !exists {
		return s.errorResponse(msg.Cookie, "call not found")
	}

	session.ToTag = toTag
	session.State = "answered"

	// Handle label
	if label := DictGetString(msg.Data, "label"); label != "" {
		session.Labels[toTag] = label
	}

	s.answerCount++

	// Generate response SDP
	responseSDP := s.generateResponseSDP(sdp, msg.Data)

	resp, _ := BuildResponse(msg.Cookie, &NGResponse{
		Result: ResultOK,
		SDP:    responseSDP,
		Streams: []StreamInfo{
			{
				LocalIP:       "127.0.0.1",
				LocalPort:     30000 + s.answerCount*2,
				LocalRTCPPort: 30001 + s.answerCount*2,
				MediaType:     "audio",
				Protocol:      "RTP/AVP",
				Index:         0,
			},
		},
	})
	return resp
}

func (s *MockNGServer) handleDelete(msg *NGMessage) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	callID := DictGetString(msg.Data, "call-id")
	fromTag := DictGetString(msg.Data, "from-tag")

	if callID == "" {
		return s.errorResponse(msg.Cookie, "missing call-id")
	}

	sessionKey := callID + ":" + fromTag
	if _, exists := s.sessions[sessionKey]; !exists {
		return s.errorResponse(msg.Cookie, "call not found")
	}

	delete(s.sessions, sessionKey)
	s.deleteCount++

	resp, _ := BuildResponse(msg.Cookie, &NGResponse{
		Result: ResultOK,
	})
	return resp
}

func (s *MockNGServer) handleQuery(msg *NGMessage) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	callID := DictGetString(msg.Data, "call-id")
	fromTag := DictGetString(msg.Data, "from-tag")

	sessionKey := callID + ":" + fromTag
	session, exists := s.sessions[sessionKey]
	if !exists {
		// Try to find by call-id only
		for key, sess := range s.sessions {
			if strings.HasPrefix(key, callID+":") {
				session = sess
				exists = true
				break
			}
		}
	}
	if !exists {
		return s.errorResponse(msg.Cookie, "call not found")
	}

	s.queryCount++

	resp, _ := BuildResponse(msg.Cookie, &NGResponse{
		Result:  ResultOK,
		Created: session.Created.Unix(),
		Stats: &CallStats{
			CreatedAt:   session.Created,
			Duration:    time.Since(session.Created),
			PacketsSent: 1000,
			PacketsRecv: 1000,
			BytesSent:   160000,
			BytesRecv:   160000,
			MOS:         4.2,
		},
	})
	return resp
}

func (s *MockNGServer) handleList(msg *NGMessage) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	calls := make([]interface{}, 0, len(s.sessions))
	for _, session := range s.sessions {
		calls = append(calls, map[string]interface{}{
			"call-id":  session.CallID,
			"from-tag": session.FromTag,
			"to-tag":   session.ToTag,
			"created":  session.Created.Unix(),
		})
	}

	resp, _ := BuildResponse(msg.Cookie, &NGResponse{
		Result: ResultOK,
		Extra: map[string]interface{}{
			"calls": calls,
		},
	})
	return resp
}

func (s *MockNGServer) handleStartRecording(msg *NGMessage) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	callID := DictGetString(msg.Data, "call-id")
	fromTag := DictGetString(msg.Data, "from-tag")

	sessionKey := callID + ":" + fromTag
	session, exists := s.sessions[sessionKey]
	if !exists {
		return s.errorResponse(msg.Cookie, "call not found")
	}

	session.Recording = true

	resp, _ := BuildResponse(msg.Cookie, &NGResponse{
		Result: ResultOK,
	})
	return resp
}

func (s *MockNGServer) handleStopRecording(msg *NGMessage) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	callID := DictGetString(msg.Data, "call-id")
	fromTag := DictGetString(msg.Data, "from-tag")

	sessionKey := callID + ":" + fromTag
	session, exists := s.sessions[sessionKey]
	if !exists {
		return s.errorResponse(msg.Cookie, "call not found")
	}

	session.Recording = false

	resp, _ := BuildResponse(msg.Cookie, &NGResponse{
		Result: ResultOK,
	})
	return resp
}

func (s *MockNGServer) handleBlockMedia(msg *NGMessage) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	callID := DictGetString(msg.Data, "call-id")
	fromTag := DictGetString(msg.Data, "from-tag")

	sessionKey := callID + ":" + fromTag
	session, exists := s.sessions[sessionKey]
	if !exists {
		return s.errorResponse(msg.Cookie, "call not found")
	}

	session.MediaBlocked = true

	resp, _ := BuildResponse(msg.Cookie, &NGResponse{
		Result: ResultOK,
	})
	return resp
}

func (s *MockNGServer) handleUnblockMedia(msg *NGMessage) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	callID := DictGetString(msg.Data, "call-id")
	fromTag := DictGetString(msg.Data, "from-tag")

	sessionKey := callID + ":" + fromTag
	session, exists := s.sessions[sessionKey]
	if !exists {
		return s.errorResponse(msg.Cookie, "call not found")
	}

	session.MediaBlocked = false

	resp, _ := BuildResponse(msg.Cookie, &NGResponse{
		Result: ResultOK,
	})
	return resp
}

func (s *MockNGServer) handlePlayDTMF(msg *NGMessage) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	callID := DictGetString(msg.Data, "call-id")
	fromTag := DictGetString(msg.Data, "from-tag")

	sessionKey := callID + ":" + fromTag
	if _, exists := s.sessions[sessionKey]; !exists {
		return s.errorResponse(msg.Cookie, "call not found")
	}

	resp, _ := BuildResponse(msg.Cookie, &NGResponse{
		Result: ResultOK,
	})
	return resp
}

func (s *MockNGServer) errorResponse(cookie, reason string) []byte {
	resp, _ := BuildResponse(cookie, &NGResponse{
		Result:      ResultError,
		ErrorReason: reason,
	})
	return resp
}

// generateResponseSDP generates a modified SDP for the response
func (s *MockNGServer) generateResponseSDP(originalSDP string, params BencodeDict) string {
	// Parse flags
	flags := DictGetStringList(params, "flags")
	flagSet := make(map[string]bool)
	for _, f := range flags {
		flagSet[f] = true
		// Handle flags with values like "ICE=force"
		if idx := strings.Index(f, "="); idx != -1 {
			flagSet[f[:idx]] = true
		}
	}

	lines := strings.Split(originalSDP, "\n")
	var result []string

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}

		// Handle ICE removal
		if flagSet["ICE"] || flagSet["ICE=remove"] {
			if strings.HasPrefix(line, "a=ice-") ||
				strings.HasPrefix(line, "a=candidate:") {
				continue
			}
		}

		// Handle origin replacement
		if (flagSet["replace-origin"] || containsStringInList(flags, "replace-origin")) && strings.HasPrefix(line, "o=") {
			line = "o=karl 1 1 IN IP4 127.0.0.1"
		}

		// Handle connection replacement
		if (flagSet["replace-session-connection"] || containsStringInList(flags, "replace-session-connection")) && strings.HasPrefix(line, "c=") {
			line = "c=IN IP4 127.0.0.1"
		}

		// Modify media port
		if strings.HasPrefix(line, "m=") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				parts[1] = fmt.Sprintf("%d", 30000+s.offerCount*2)
				line = strings.Join(parts, " ")
			}
		}

		result = append(result, line)
	}

	// Add ICE credentials if ICE=force
	if flagSet["ICE=force"] || flagSet["ICE"] {
		result = append(result, "a=ice-ufrag:mockufrag")
		result = append(result, "a=ice-pwd:mocklongenoughpassword")
	}

	return strings.Join(result, "\r\n") + "\r\n"
}

func containsStringInList(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
