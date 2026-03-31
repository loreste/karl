package internal

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// T38Gateway handles T.38 fax passthrough and gateway functionality
type T38Gateway struct {
	sessions map[string]*T38Session
	config   *T38Config
	mu       sync.RWMutex
}

// T38Config holds T.38 gateway configuration
type T38Config struct {
	Enabled           bool
	GatewayMode       bool   // true = audio<->T.38 gateway, false = passthrough
	MaxBitRate        int    // Maximum bit rate in bps
	RateMgmt          string // localTCF, transferredTCF
	MaxBuffer         int    // Maximum buffer size
	UDPECMode         string // t38UDPRedundancy, t38UDPFEC
	UDPECDepth        int    // Error correction depth
	FillBitRemoval    bool
	TranscodingMMR    bool
	TranscodingJBIG   bool
}

// T38Session represents an active T.38 session
type T38Session struct {
	ID            string
	CallID        string
	State         T38State
	LocalIP       net.IP
	LocalPort     int
	RemoteIP      net.IP
	RemotePort    int
	Direction     T38Direction
	Config        *T38Config
	Stats         *T38Stats
	CreatedAt     time.Time
	LastActivity  time.Time
	mu            sync.RWMutex
}

// T38State represents the T.38 session state
type T38State string

const (
	T38StateIdle      T38State = "idle"
	T38StateSetup     T38State = "setup"
	T38StateActive    T38State = "active"
	T38StateComplete  T38State = "complete"
	T38StateFailed    T38State = "failed"
)

// T38Direction represents the T.38 direction
type T38Direction string

const (
	T38DirectionSend    T38Direction = "send"
	T38DirectionReceive T38Direction = "receive"
	T38DirectionBoth    T38Direction = "both"
)

// T38Stats holds T.38 session statistics
type T38Stats struct {
	PagesSent     int
	PagesReceived int
	PacketsSent   uint64
	PacketsRecv   uint64
	BytesSent     uint64
	BytesRecv     uint64
	Errors        int
	Retransmits   int
}

// T38IFP represents a T.38 IFP (Internet Fax Protocol) packet
type T38IFP struct {
	Type       T38IFPType
	Data       []byte
	SeqNum     uint16
	Redundant  [][]byte // Redundant data for error correction
}

// T38IFPType represents T.38 IFP data types
type T38IFPType int

const (
	T38IFPPrimaryV21   T38IFPType = 0
	T38IFPPrimaryV27   T38IFPType = 1
	T38IFPPrimaryV29   T38IFPType = 2
	T38IFPPrimaryV17   T38IFPType = 3
	T38IFPPrimaryV8    T38IFPType = 4
	T38IFPPrimaryT30   T38IFPType = 5
	T38IFPPrimaryV34   T38IFPType = 6
	T38IFPRedundancy   T38IFPType = 7
	T38IFPControlData  T38IFPType = 8
)

// NewT38Gateway creates a new T.38 gateway
func NewT38Gateway(config *T38Config) *T38Gateway {
	if config == nil {
		config = DefaultT38Config()
	}
	return &T38Gateway{
		sessions: make(map[string]*T38Session),
		config:   config,
	}
}

// DefaultT38Config returns default T.38 configuration
func DefaultT38Config() *T38Config {
	return &T38Config{
		Enabled:        true,
		GatewayMode:    false, // Passthrough by default
		MaxBitRate:     14400,
		RateMgmt:       "transferredTCF",
		MaxBuffer:      200,
		UDPECMode:      "t38UDPRedundancy",
		UDPECDepth:     3,
		FillBitRemoval: true,
		TranscodingMMR: false,
		TranscodingJBIG: false,
	}
}

// CreateSession creates a new T.38 session
func (gw *T38Gateway) CreateSession(callID string, localIP net.IP, localPort int) (*T38Session, error) {
	gw.mu.Lock()
	defer gw.mu.Unlock()

	sessionID := fmt.Sprintf("t38-%s-%d", callID, time.Now().UnixNano())

	session := &T38Session{
		ID:           sessionID,
		CallID:       callID,
		State:        T38StateSetup,
		LocalIP:      localIP,
		LocalPort:    localPort,
		Direction:    T38DirectionBoth,
		Config:       gw.config,
		Stats:        &T38Stats{},
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
	}

	gw.sessions[sessionID] = session
	return session, nil
}

// SetRemoteEndpoint sets the remote endpoint for T.38
func (gw *T38Gateway) SetRemoteEndpoint(sessionID string, remoteIP net.IP, remotePort int) error {
	gw.mu.Lock()
	defer gw.mu.Unlock()

	session, exists := gw.sessions[sessionID]
	if !exists {
		return fmt.Errorf("T.38 session not found: %s", sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	session.RemoteIP = remoteIP
	session.RemotePort = remotePort
	session.State = T38StateActive
	session.LastActivity = time.Now()

	return nil
}

// GetSession returns a T.38 session by ID
func (gw *T38Gateway) GetSession(sessionID string) (*T38Session, bool) {
	gw.mu.RLock()
	defer gw.mu.RUnlock()

	session, exists := gw.sessions[sessionID]
	return session, exists
}

// GetSessionByCallID returns T.38 sessions for a call
func (gw *T38Gateway) GetSessionByCallID(callID string) []*T38Session {
	gw.mu.RLock()
	defer gw.mu.RUnlock()

	var sessions []*T38Session
	for _, session := range gw.sessions {
		if session.CallID == callID {
			sessions = append(sessions, session)
		}
	}
	return sessions
}

// ProcessPacket processes an incoming T.38 packet
func (gw *T38Gateway) ProcessPacket(sessionID string, packet []byte) (*T38IFP, error) {
	gw.mu.RLock()
	session, exists := gw.sessions[sessionID]
	gw.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("T.38 session not found: %s", sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	session.Stats.PacketsRecv++
	session.Stats.BytesRecv += uint64(len(packet))
	session.LastActivity = time.Now()

	// Parse IFP packet
	ifp, err := gw.parseIFP(packet)
	if err != nil {
		session.Stats.Errors++
		return nil, err
	}

	return ifp, nil
}

// parseIFP parses a T.38 IFP packet
func (gw *T38Gateway) parseIFP(packet []byte) (*T38IFP, error) {
	if len(packet) < 2 {
		return nil, fmt.Errorf("T.38 packet too short")
	}

	// Basic IFP parsing (simplified)
	ifp := &T38IFP{
		Type:   T38IFPType(packet[0] >> 4),
		SeqNum: uint16(packet[0]&0x0F)<<8 | uint16(packet[1]),
		Data:   packet[2:],
	}

	return ifp, nil
}

// SendPacket sends a T.38 packet
func (gw *T38Gateway) SendPacket(sessionID string, ifp *T38IFP) error {
	gw.mu.RLock()
	session, exists := gw.sessions[sessionID]
	gw.mu.RUnlock()

	if !exists {
		return fmt.Errorf("T.38 session not found: %s", sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Build packet
	packet := gw.buildIFP(ifp)

	session.Stats.PacketsSent++
	session.Stats.BytesSent += uint64(len(packet))
	session.LastActivity = time.Now()

	return nil
}

// buildIFP builds a T.38 IFP packet
func (gw *T38Gateway) buildIFP(ifp *T38IFP) []byte {
	packet := make([]byte, 2+len(ifp.Data))
	packet[0] = byte(ifp.Type)<<4 | byte(ifp.SeqNum>>8)
	packet[1] = byte(ifp.SeqNum & 0xFF)
	copy(packet[2:], ifp.Data)

	// Add redundancy if configured
	if gw.config.UDPECMode == "t38UDPRedundancy" && len(ifp.Redundant) > 0 {
		for _, redundant := range ifp.Redundant {
			packet = append(packet, redundant...)
		}
	}

	return packet
}

// CompleteSession marks a T.38 session as complete
func (gw *T38Gateway) CompleteSession(sessionID string) error {
	gw.mu.Lock()
	defer gw.mu.Unlock()

	session, exists := gw.sessions[sessionID]
	if !exists {
		return fmt.Errorf("T.38 session not found: %s", sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	session.State = T38StateComplete
	return nil
}

// RemoveSession removes a T.38 session
func (gw *T38Gateway) RemoveSession(sessionID string) {
	gw.mu.Lock()
	defer gw.mu.Unlock()
	delete(gw.sessions, sessionID)
}

// GetStats returns statistics for a session
func (gw *T38Gateway) GetStats(sessionID string) (*T38Stats, error) {
	gw.mu.RLock()
	defer gw.mu.RUnlock()

	session, exists := gw.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("T.38 session not found: %s", sessionID)
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	// Return a copy
	statsCopy := *session.Stats
	return &statsCopy, nil
}

// GetActiveCount returns the number of active T.38 sessions
func (gw *T38Gateway) GetActiveCount() int {
	gw.mu.RLock()
	defer gw.mu.RUnlock()

	count := 0
	for _, session := range gw.sessions {
		session.mu.RLock()
		if session.State == T38StateActive {
			count++
		}
		session.mu.RUnlock()
	}
	return count
}

// BuildT38SDP generates T.38 SDP attributes
func BuildT38SDP(config *T38Config, localIP string, localPort int) string {
	if config == nil {
		config = DefaultT38Config()
	}

	// m=image line for T.38
	sdp := fmt.Sprintf("m=image %d udptl t38\r\n", localPort)

	// T.38 attributes
	sdp += fmt.Sprintf("a=T38MaxBitRate:%d\r\n", config.MaxBitRate)
	sdp += fmt.Sprintf("a=T38FaxRateManagement:%s\r\n", config.RateMgmt)
	sdp += fmt.Sprintf("a=T38FaxMaxBuffer:%d\r\n", config.MaxBuffer)
	sdp += "a=T38FaxMaxDatagram:200\r\n"
	sdp += fmt.Sprintf("a=T38FaxUdpEC:%s\r\n", config.UDPECMode)

	if config.FillBitRemoval {
		sdp += "a=T38FaxFillBitRemoval\r\n"
	}
	if config.TranscodingMMR {
		sdp += "a=T38FaxTranscodingMMR\r\n"
	}
	if config.TranscodingJBIG {
		sdp += "a=T38FaxTranscodingJBIG\r\n"
	}

	return sdp
}

// ParseT38SDP parses T.38 attributes from SDP
func ParseT38SDP(sdp string) *T38Config {
	config := DefaultT38Config()
	// Basic parsing would be implemented here
	// For now return default config
	return config
}

// Cleanup removes old completed sessions
func (gw *T38Gateway) Cleanup(maxAge time.Duration) int {
	gw.mu.Lock()
	defer gw.mu.Unlock()

	now := time.Now()
	removed := 0
	for id, session := range gw.sessions {
		session.mu.RLock()
		shouldRemove := (session.State == T38StateComplete || session.State == T38StateFailed) &&
			now.Sub(session.CreatedAt) > maxAge
		session.mu.RUnlock()

		if shouldRemove {
			delete(gw.sessions, id)
			removed++
		}
	}
	return removed
}
