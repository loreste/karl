package internal

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	ng "karl/internal/ng_protocol"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// NGSocketListener metrics
var (
	ngMessagesReceived = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "karl_ng_messages_received_total",
			Help: "Total number of NG protocol messages received",
		},
	)

	ngMessagesSent = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "karl_ng_messages_sent_total",
			Help: "Total number of NG protocol messages sent",
		},
	)

	ngParseErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "karl_ng_parse_errors_total",
			Help: "Total number of NG protocol parse errors",
		},
	)

	ngConnectionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "karl_ng_connections_active",
			Help: "Number of active NG protocol connections (for TCP mode)",
		},
	)
)

// NGCommandHandler is a function that handles an NG protocol command
type NGCommandHandler func(req *ng.NGRequest) (*ng.NGResponse, error)

// NGSocketListener handles NG protocol communication via Unix socket or UDP
type NGSocketListener struct {
	config          *Config
	sessionRegistry *SessionRegistry
	handlers        map[string]NGCommandHandler

	// Socket connections
	unixListener net.Listener
	udpConn      *net.UDPConn

	// State management
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.RWMutex
	running    bool
	startTime  time.Time
}

// NewNGSocketListener creates a new NG protocol socket listener
func NewNGSocketListener(config *Config, sessionRegistry *SessionRegistry) *NGSocketListener {
	ctx, cancel := context.WithCancel(context.Background())

	l := &NGSocketListener{
		config:          config,
		sessionRegistry: sessionRegistry,
		handlers:        make(map[string]NGCommandHandler),
		ctx:             ctx,
		cancel:          cancel,
		startTime:       time.Now(),
	}

	// Register built-in command handlers
	l.registerBuiltinHandlers()

	return l
}

// registerBuiltinHandlers registers all NG protocol command handlers
func (l *NGSocketListener) registerBuiltinHandlers() {
	// Ping
	l.handlers[ng.CmdPing] = func(req *ng.NGRequest) (*ng.NGResponse, error) {
		return &ng.NGResponse{Result: ng.ResultPong}, nil
	}

	// Offer
	l.handlers[ng.CmdOffer] = l.handleOffer

	// Answer
	l.handlers[ng.CmdAnswer] = l.handleAnswer

	// Delete
	l.handlers[ng.CmdDelete] = l.handleDelete

	// Query
	l.handlers[ng.CmdQuery] = l.handleQuery

	// List
	l.handlers[ng.CmdList] = l.handleList

	// Statistics
	l.handlers[ng.CmdStatistics] = l.handleStatistics

	// Recording commands
	l.handlers[ng.CmdStartRecording] = l.handleStartRecording
	l.handlers[ng.CmdStopRecording] = l.handleStopRecording
	l.handlers[ng.CmdPauseRecording] = l.handlePauseRecording

	// DTMF commands
	l.handlers[ng.CmdBlockDTMF] = l.handleBlockDTMF
	l.handlers[ng.CmdUnblockDTMF] = l.handleUnblockDTMF
	l.handlers[ng.CmdPlayDTMF] = l.handlePlayDTMF

	// Media control commands
	l.handlers[ng.CmdBlockMedia] = l.handleBlockMedia
	l.handlers[ng.CmdUnblockMedia] = l.handleUnblockMedia
	l.handlers[ng.CmdSilenceMedia] = l.handleSilenceMedia
	l.handlers[ng.CmdStartForward] = l.handleStartForwarding
	l.handlers[ng.CmdStopForward] = l.handleStopForwarding
	l.handlers[ng.CmdPlayMedia] = l.handlePlayMedia
	l.handlers[ng.CmdStopMedia] = l.handleStopMedia
}

// RegisterHandler registers a custom command handler
func (l *NGSocketListener) RegisterHandler(command string, handler NGCommandHandler) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.handlers[command] = handler
}

// Start starts the NG socket listener
func (l *NGSocketListener) Start() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.running {
		return fmt.Errorf("NG socket listener already running")
	}

	// Check if NG protocol is enabled
	if l.config.NGProtocol == nil || !l.config.NGProtocol.Enabled {
		log.Println("NG protocol is disabled in configuration")
		return nil
	}

	socketPath := l.config.NGProtocol.SocketPath
	if socketPath == "" {
		socketPath = "/var/run/karl/karl.sock"
	}

	// Start Unix socket listener
	if err := l.startUnixListener(socketPath); err != nil {
		return fmt.Errorf("failed to start Unix socket listener: %w", err)
	}

	// Start UDP listener if configured
	if l.config.NGProtocol.UDPPort > 0 {
		if err := l.startUDPListener(l.config.NGProtocol.UDPPort); err != nil {
			log.Printf("Warning: failed to start UDP listener: %v", err)
		}
	}

	l.running = true
	log.Printf("NG socket listener started on %s", socketPath)

	return nil
}

// startUnixListener starts the Unix socket listener
func (l *NGSocketListener) startUnixListener(socketPath string) error {
	// Remove existing socket file
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: could not remove existing socket: %v", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(socketPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create socket directory: %w", err)
		}
	}

	// Create Unix socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to create Unix socket: %w", err)
	}

	// Set socket permissions
	if err := os.Chmod(socketPath, 0666); err != nil {
		log.Printf("Warning: could not set socket permissions: %v", err)
	}

	l.unixListener = listener

	// Start accept loop
	l.wg.Add(1)
	go l.acceptLoop()

	return nil
}

// startUDPListener starts the UDP listener
func (l *NGSocketListener) startUDPListener(port int) error {
	addr := &net.UDPAddr{
		IP:   net.ParseIP("0.0.0.0"),
		Port: port,
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to create UDP socket: %w", err)
	}

	l.udpConn = conn

	// Start UDP read loop
	l.wg.Add(1)
	go l.udpReadLoop()

	log.Printf("NG UDP listener started on port %d", port)

	return nil
}

// acceptLoop handles incoming Unix socket connections
func (l *NGSocketListener) acceptLoop() {
	defer l.wg.Done()

	for {
		select {
		case <-l.ctx.Done():
			return
		default:
		}

		conn, err := l.unixListener.Accept()
		if err != nil {
			select {
			case <-l.ctx.Done():
				return
			default:
				log.Printf("Error accepting connection: %v", err)
				continue
			}
		}

		ngConnectionsActive.Inc()
		l.wg.Add(1)
		go l.handleConnection(conn)
	}
}

// handleConnection handles a single Unix socket connection
func (l *NGSocketListener) handleConnection(conn net.Conn) {
	defer l.wg.Done()
	defer conn.Close()
	defer ngConnectionsActive.Dec()

	// Set read deadline
	if err := conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
		log.Printf("Error setting read deadline: %v", err)
		return
	}

	// Read message
	buf := make([]byte, 65536)
	n, err := conn.Read(buf)
	if err != nil {
		log.Printf("Error reading from connection: %v", err)
		return
	}

	// Process message
	response := l.processMessage(buf[:n], nil)

	// Send response
	if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		log.Printf("Error setting write deadline: %v", err)
		return
	}
	if _, err := conn.Write(response); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

// udpReadLoop handles incoming UDP messages
func (l *NGSocketListener) udpReadLoop() {
	defer l.wg.Done()

	buf := make([]byte, 65536)

	for {
		select {
		case <-l.ctx.Done():
			return
		default:
		}

		_ = l.udpConn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, addr, err := l.udpConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-l.ctx.Done():
				return
			default:
				log.Printf("Error reading UDP: %v", err)
				continue
			}
		}

		// Process message asynchronously
		data := make([]byte, n)
		copy(data, buf[:n])

		l.wg.Add(1)
		go func(data []byte, addr *net.UDPAddr) {
			defer l.wg.Done()
			response := l.processMessage(data, addr)
			if _, err := l.udpConn.WriteToUDP(response, addr); err != nil {
				log.Printf("Error sending UDP response: %v", err)
			}
		}(data, addr)
	}
}

// processMessage processes an NG protocol message and returns the response
func (l *NGSocketListener) processMessage(data []byte, from *net.UDPAddr) []byte {
	ngMessagesReceived.Inc()

	// Parse the message
	msg, err := ng.ParseMessage(data, from)
	if err != nil {
		ngParseErrors.Inc()
		log.Printf("Failed to parse NG message: %v", err)
		resp, _ := ng.ErrorResponse("", ng.ErrReasonInternal)
		return resp
	}

	// Convert to request
	req, err := msg.ToRequest()
	if err != nil {
		ngParseErrors.Inc()
		log.Printf("Failed to convert NG message to request: %v", err)
		resp, _ := ng.ErrorResponse(msg.Cookie, err.Error())
		return resp
	}

	// Find handler
	l.mu.RLock()
	handler, ok := l.handlers[req.Command]
	l.mu.RUnlock()

	if !ok {
		resp, _ := ng.ErrorResponse(req.Cookie, ng.ErrReasonUnsupported)
		return resp
	}

	// Execute handler
	start := time.Now()
	response, err := handler(req)
	duration := time.Since(start)

	log.Printf("NG command: %s, call-id: %s, duration: %v", req.Command, req.CallID, duration)

	if err != nil {
		log.Printf("Error handling NG request: %v", err)
		resp, _ := ng.ErrorResponse(req.Cookie, err.Error())
		return resp
	}

	// Build response
	respBytes, err := ng.BuildResponse(req.Cookie, response)
	if err != nil {
		log.Printf("Error building response: %v", err)
		resp, _ := ng.ErrorResponse(req.Cookie, ng.ErrReasonInternal)
		return resp
	}

	ngMessagesSent.Inc()

	// Update active calls metric
	ng.UpdateActiveCallsMetric(l.sessionRegistry.GetActiveCount())

	return respBytes
}

// Stop stops the NG socket listener
func (l *NGSocketListener) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.running {
		return nil
	}

	log.Println("Stopping NG socket listener...")

	// Cancel context to stop goroutines
	l.cancel()

	// Close listeners
	if l.unixListener != nil {
		l.unixListener.Close()
	}
	if l.udpConn != nil {
		l.udpConn.Close()
	}

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("NG socket listener stopped gracefully")
	case <-time.After(5 * time.Second):
		log.Println("NG socket listener stop timed out")
	}

	l.running = false
	return nil
}

// IsRunning returns whether the listener is running
func (l *NGSocketListener) IsRunning() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.running
}

// GetSessionRegistry returns the session registry
func (l *NGSocketListener) GetSessionRegistry() *SessionRegistry {
	return l.sessionRegistry
}

// Health check
func (l *NGSocketListener) HealthCheck() ComponentHealth {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if !l.running {
		return ComponentHealth{
			Status:  StatusDown,
			Message: "NG socket listener not running",
		}
	}

	return ComponentHealth{
		Status:  StatusUp,
		Message: "NG socket listener is operational",
		Details: map[string]string{
			"uptime":       time.Since(l.startTime).String(),
			"active_calls": fmt.Sprintf("%d", l.sessionRegistry.GetActiveCount()),
		},
		LastChecked: time.Now(),
	}
}

// Command handler implementations

func (l *NGSocketListener) handleOffer(req *ng.NGRequest) (*ng.NGResponse, error) {
	if req.CallID == "" {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonMissingParam + ": call-id"}, nil
	}
	if req.FromTag == "" {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonMissingParam + ": from-tag"}, nil
	}
	if req.SDP == "" {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonMissingParam + ": sdp"}, nil
	}

	// Create or get session
	session := l.sessionRegistry.GetSessionByTags(req.CallID, req.FromTag, req.ToTag)
	if session == nil {
		session = l.sessionRegistry.CreateSession(req.CallID, req.FromTag)
	}

	_ = l.sessionRegistry.UpdateSessionState(session.ID, string(SessionStatePending))

	// TODO: Full SDP processing would go here
	// For now, return a basic response

	return &ng.NGResponse{
		Result:  ng.ResultOK,
		SDP:     req.SDP, // Echo back for now
		CallID:  req.CallID,
		FromTag: req.FromTag,
	}, nil
}

func (l *NGSocketListener) handleAnswer(req *ng.NGRequest) (*ng.NGResponse, error) {
	if req.CallID == "" {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonMissingParam + ": call-id"}, nil
	}
	if req.FromTag == "" {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonMissingParam + ": from-tag"}, nil
	}
	if req.SDP == "" {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonMissingParam + ": sdp"}, nil
	}

	session := l.sessionRegistry.GetSessionByTags(req.CallID, req.FromTag, req.ToTag)
	if session == nil {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonNotFound}, nil
	}

	_ = l.sessionRegistry.UpdateSessionState(session.ID, string(SessionStateActive))

	return &ng.NGResponse{
		Result:  ng.ResultOK,
		SDP:     req.SDP,
		CallID:  req.CallID,
		FromTag: req.FromTag,
		ToTag:   req.ToTag,
	}, nil
}

func (l *NGSocketListener) handleDelete(req *ng.NGRequest) (*ng.NGResponse, error) {
	if req.CallID == "" {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonMissingParam + ": call-id"}, nil
	}

	sessions := l.sessionRegistry.GetSessionByCallID(req.CallID)
	if len(sessions) == 0 {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonNotFound}, nil
	}

	for _, session := range sessions {
		_ = l.sessionRegistry.UpdateSessionState(session.ID, string(SessionStateTerminated))
		_ = l.sessionRegistry.DeleteSession(session.ID)
	}

	return &ng.NGResponse{Result: ng.ResultOK}, nil
}

func (l *NGSocketListener) handleQuery(req *ng.NGRequest) (*ng.NGResponse, error) {
	if req.CallID == "" {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonMissingParam + ": call-id"}, nil
	}

	session := l.sessionRegistry.GetSessionByTags(req.CallID, req.FromTag, req.ToTag)
	if session == nil {
		sessions := l.sessionRegistry.GetSessionByCallID(req.CallID)
		if len(sessions) > 0 {
			session = sessions[0]
		}
	}

	if session == nil {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonNotFound}, nil
	}

	return &ng.NGResponse{
		Result:     ng.ResultOK,
		CallID:     session.CallID,
		FromTag:    session.FromTag,
		ToTag:      session.ToTag,
		Created:    session.CreatedAt.Unix(),
		LastSignal: session.UpdatedAt.Unix(),
	}, nil
}

func (l *NGSocketListener) handleList(req *ng.NGRequest) (*ng.NGResponse, error) {
	sessions := l.sessionRegistry.ListSessions()

	calls := make([]interface{}, 0, len(sessions))
	for _, session := range sessions {
		calls = append(calls, map[string]interface{}{
			"call-id":  session.CallID,
			"from-tag": session.FromTag,
			"to-tag":   session.ToTag,
			"state":    string(session.State),
		})
	}

	return &ng.NGResponse{
		Result: ng.ResultOK,
		Extra: map[string]interface{}{
			"calls": calls,
			"count": len(calls),
		},
	}, nil
}

func (l *NGSocketListener) handleStatistics(req *ng.NGRequest) (*ng.NGResponse, error) {
	stats := l.sessionRegistry.GetStats()
	return &ng.NGResponse{
		Result: ng.ResultOK,
		Extra:  stats,
	}, nil
}

func (l *NGSocketListener) handleStartRecording(req *ng.NGRequest) (*ng.NGResponse, error) {
	session := l.findSession(req)
	if session == nil {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonNotFound}, nil
	}
	session.SetFlag("recording", true)
	return &ng.NGResponse{Result: ng.ResultOK}, nil
}

func (l *NGSocketListener) handleStopRecording(req *ng.NGRequest) (*ng.NGResponse, error) {
	session := l.findSession(req)
	if session == nil {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonNotFound}, nil
	}
	session.SetFlag("recording", false)
	return &ng.NGResponse{Result: ng.ResultOK}, nil
}

func (l *NGSocketListener) handlePauseRecording(req *ng.NGRequest) (*ng.NGResponse, error) {
	session := l.findSession(req)
	if session == nil {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonNotFound}, nil
	}
	session.SetFlag("recording_paused", true)
	return &ng.NGResponse{Result: ng.ResultOK}, nil
}

func (l *NGSocketListener) handleBlockDTMF(req *ng.NGRequest) (*ng.NGResponse, error) {
	session := l.findSession(req)
	if session == nil {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonNotFound}, nil
	}
	session.SetFlag("dtmf_blocked", true)
	return &ng.NGResponse{Result: ng.ResultOK}, nil
}

func (l *NGSocketListener) handleUnblockDTMF(req *ng.NGRequest) (*ng.NGResponse, error) {
	session := l.findSession(req)
	if session == nil {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonNotFound}, nil
	}
	session.SetFlag("dtmf_blocked", false)
	return &ng.NGResponse{Result: ng.ResultOK}, nil
}

func (l *NGSocketListener) handlePlayDTMF(req *ng.NGRequest) (*ng.NGResponse, error) {
	session := l.findSession(req)
	if session == nil {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonNotFound}, nil
	}
	if req.DTMFDigit == "" {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonMissingParam + ": digit"}, nil
	}
	session.SetMetadata("pending_dtmf", req.DTMFDigit)
	return &ng.NGResponse{Result: ng.ResultOK}, nil
}

func (l *NGSocketListener) handleBlockMedia(req *ng.NGRequest) (*ng.NGResponse, error) {
	session := l.findSession(req)
	if session == nil {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonNotFound}, nil
	}
	session.SetFlag("media_blocked", true)
	return &ng.NGResponse{Result: ng.ResultOK}, nil
}

func (l *NGSocketListener) handleUnblockMedia(req *ng.NGRequest) (*ng.NGResponse, error) {
	session := l.findSession(req)
	if session == nil {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonNotFound}, nil
	}
	session.SetFlag("media_blocked", false)
	return &ng.NGResponse{Result: ng.ResultOK}, nil
}

func (l *NGSocketListener) handleSilenceMedia(req *ng.NGRequest) (*ng.NGResponse, error) {
	session := l.findSession(req)
	if session == nil {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonNotFound}, nil
	}
	session.SetFlag("media_silenced", true)
	return &ng.NGResponse{Result: ng.ResultOK}, nil
}

func (l *NGSocketListener) handleStartForwarding(req *ng.NGRequest) (*ng.NGResponse, error) {
	session := l.findSession(req)
	if session == nil {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonNotFound}, nil
	}
	session.SetFlag("forwarding", true)
	session.SetMetadata("forward_address", req.ForwardAddress)
	return &ng.NGResponse{Result: ng.ResultOK}, nil
}

func (l *NGSocketListener) handleStopForwarding(req *ng.NGRequest) (*ng.NGResponse, error) {
	session := l.findSession(req)
	if session == nil {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonNotFound}, nil
	}
	session.SetFlag("forwarding", false)
	return &ng.NGResponse{Result: ng.ResultOK}, nil
}

func (l *NGSocketListener) handlePlayMedia(req *ng.NGRequest) (*ng.NGResponse, error) {
	session := l.findSession(req)
	if session == nil {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonNotFound}, nil
	}
	session.SetFlag("playing_media", true)
	return &ng.NGResponse{Result: ng.ResultOK}, nil
}

func (l *NGSocketListener) handleStopMedia(req *ng.NGRequest) (*ng.NGResponse, error) {
	session := l.findSession(req)
	if session == nil {
		return &ng.NGResponse{Result: ng.ResultError, ErrorReason: ng.ErrReasonNotFound}, nil
	}
	session.SetFlag("playing_media", false)
	return &ng.NGResponse{Result: ng.ResultOK}, nil
}

func (l *NGSocketListener) findSession(req *ng.NGRequest) *MediaSession {
	if req.CallID == "" {
		return nil
	}
	session := l.sessionRegistry.GetSessionByTags(req.CallID, req.FromTag, req.ToTag)
	if session == nil {
		sessions := l.sessionRegistry.GetSessionByCallID(req.CallID)
		if len(sessions) > 0 {
			return sessions[0]
		}
	}
	return session
}
