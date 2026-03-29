package internal

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SessionState represents the lifecycle state of a media session
type SessionState string

const (
	SessionStateNew        SessionState = "new"
	SessionStatePending    SessionState = "pending"
	SessionStateActive     SessionState = "active"
	SessionStateHold       SessionState = "hold"
	SessionStateTerminated SessionState = "terminated"
)

// MediaType represents the type of media in a session
type MediaType string

const (
	MediaAudio MediaType = "audio"
	MediaVideo MediaType = "video"
)

// TransportProtocol represents the transport protocol
type TransportProtocol string

const (
	TransportRTP      TransportProtocol = "RTP/AVP"
	TransportRTPS     TransportProtocol = "RTP/SAVP"
	TransportRTPSF    TransportProtocol = "RTP/SAVPF"
	TransportUDPTLSF  TransportProtocol = "UDP/TLS/RTP/SAVPF"
)

// CallLeg represents one side of a call (caller or callee)
type CallLeg struct {
	Tag           string
	IP            net.IP
	Port          int
	RTCPPort      int
	MediaType     MediaType
	Codecs        []CodecInfo
	SSRC          uint32
	Transport     TransportProtocol
	ICECredentials *ICECredentials
	SRTPParams    *SRTPParameters
	LocalIP       net.IP
	LocalPort     int
	LocalRTCPPort int
	Conn          *net.UDPConn
	RTCPConn      *net.UDPConn
	LastActivity  time.Time
	PacketsSent   uint64
	PacketsRecv   uint64
	BytesSent     uint64
	BytesRecv     uint64
	PacketsLost   uint32
	Jitter        float64
}

// ICECredentials holds ICE authentication credentials
type ICECredentials struct {
	Username string
	Password string
	Lite     bool
}

// SRTPParameters holds SRTP encryption parameters
type SRTPParameters struct {
	CryptoSuite string
	MasterKey   []byte
	MasterSalt  []byte
	DTLS        bool
	Fingerprint string
	Setup       string // actpass, active, passive
}

// CodecInfo represents codec information
type CodecInfo struct {
	PayloadType uint8
	Name        string
	ClockRate   uint32
	Channels    int
	Fmtp        string
}

// SessionStats holds session statistics
type SessionStats struct {
	StartTime         time.Time
	ConnectTime       time.Time
	EndTime           time.Time
	Duration          time.Duration
	CallerPacketsSent uint64
	CallerPacketsRecv uint64
	CallerBytesent    uint64
	CallerBytesRecv   uint64
	CalleePacketsSent uint64
	CalleePacketsRecv uint64
	CalleeBytesent    uint64
	CalleeBytesRecv   uint64
	PacketLossRate    float64
	AvgJitter         float64
	MaxJitter         float64
	RTT               float64
	MOS               float64
}

// MediaSession represents an active media session
type MediaSession struct {
	ID           string
	CallID       string
	FromTag      string
	ToTag        string
	ViaBranch    string
	State        SessionState
	CallerLeg    *CallLeg
	CalleeLeg    *CallLeg
	SSRCToLeg    map[uint32]*CallLeg
	Stats        *SessionStats
	JitterBuf    *JitterBuffer
	FECHandler   *FECHandler
	RTCPHandler  *RTCPSessionHandler
	Recording    *SessionRecording
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Flags        map[string]bool
	Metadata     map[string]string
	mu           sync.RWMutex
}

// SessionRecording holds recording state for a session
type SessionRecording struct {
	Active    bool
	RecordID  string
	StartTime time.Time
	FilePath  string
	Format    string
	Mode      string
}

// Lock acquires the session mutex
func (s *MediaSession) Lock() {
	s.mu.Lock()
}

// Unlock releases the session mutex
func (s *MediaSession) Unlock() {
	s.mu.Unlock()
}

// RLock acquires a read lock on the session mutex
func (s *MediaSession) RLock() {
	s.mu.RLock()
}

// RUnlock releases a read lock on the session mutex
func (s *MediaSession) RUnlock() {
	s.mu.RUnlock()
}

// SessionRegistry manages all active sessions
type SessionRegistry struct {
	sessions      map[string]*MediaSession
	callIDIndex   map[string][]*MediaSession
	fromTagIndex  map[string]*MediaSession
	ssrcIndex     map[uint32]*MediaSession
	mu            sync.RWMutex
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
	sessionTTL    time.Duration
	onSessionEnd  func(*MediaSession)
}

// NewSessionRegistry creates a new session registry
func NewSessionRegistry(sessionTTL time.Duration) *SessionRegistry {
	sr := &SessionRegistry{
		sessions:     make(map[string]*MediaSession),
		callIDIndex:  make(map[string][]*MediaSession),
		fromTagIndex: make(map[string]*MediaSession),
		ssrcIndex:    make(map[uint32]*MediaSession),
		sessionTTL:   sessionTTL,
		stopCleanup:  make(chan struct{}),
	}

	// Start cleanup goroutine
	sr.cleanupTicker = time.NewTicker(30 * time.Second)
	go sr.cleanupLoop()

	return sr
}

// SetOnSessionEnd sets the callback for session termination
func (sr *SessionRegistry) SetOnSessionEnd(callback func(*MediaSession)) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.onSessionEnd = callback
}

// cleanupLoop removes stale sessions
func (sr *SessionRegistry) cleanupLoop() {
	for {
		select {
		case <-sr.cleanupTicker.C:
			sr.cleanupStaleSessions()
		case <-sr.stopCleanup:
			sr.cleanupTicker.Stop()
			return
		}
	}
}

// cleanupStaleSessions removes sessions that have exceeded TTL
func (sr *SessionRegistry) cleanupStaleSessions() {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	now := time.Now()
	for id, session := range sr.sessions {
		session.mu.RLock()
		isStale := session.State == SessionStateTerminated ||
			(session.State != SessionStateActive && now.Sub(session.UpdatedAt) > sr.sessionTTL)
		session.mu.RUnlock()

		if isStale {
			_ = sr.removeSessionLocked(id)
		}
	}
}

// CreateSession creates a new media session
func (sr *SessionRegistry) CreateSession(callID, fromTag string) *MediaSession {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	session := &MediaSession{
		ID:        uuid.New().String(),
		CallID:    callID,
		FromTag:   fromTag,
		State:     SessionStateNew,
		SSRCToLeg: make(map[uint32]*CallLeg),
		Stats:     &SessionStats{StartTime: time.Now()},
		Flags:     make(map[string]bool),
		Metadata:  make(map[string]string),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	sr.sessions[session.ID] = session
	sr.callIDIndex[callID] = append(sr.callIDIndex[callID], session)
	sr.fromTagIndex[fromTag] = session

	return session
}

// GetSession retrieves a session by ID
func (sr *SessionRegistry) GetSession(sessionID string) (*MediaSession, bool) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	session, ok := sr.sessions[sessionID]
	return session, ok
}

// GetSessionByCallID retrieves sessions by Call-ID
func (sr *SessionRegistry) GetSessionByCallID(callID string) []*MediaSession {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	sessions := sr.callIDIndex[callID]
	result := make([]*MediaSession, len(sessions))
	copy(result, sessions)
	return result
}

// GetSessionByTags retrieves a session by Call-ID and tags
func (sr *SessionRegistry) GetSessionByTags(callID, fromTag, toTag string) *MediaSession {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	sessions := sr.callIDIndex[callID]
	for _, session := range sessions {
		session.mu.RLock()
		match := session.FromTag == fromTag && (toTag == "" || session.ToTag == toTag)
		session.mu.RUnlock()
		if match {
			return session
		}
	}
	return nil
}

// GetSessionBySSRC retrieves a session by SSRC
func (sr *SessionRegistry) GetSessionBySSRC(ssrc uint32) (*MediaSession, *CallLeg, bool) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	session, ok := sr.ssrcIndex[ssrc]
	if !ok {
		return nil, nil, false
	}

	session.mu.RLock()
	leg := session.SSRCToLeg[ssrc]
	session.mu.RUnlock()

	return session, leg, true
}

// UpdateSessionState updates the session state (accepts string to match interface)
func (sr *SessionRegistry) UpdateSessionState(sessionID string, state string) error {
	return sr.UpdateSessionStateTyped(sessionID, SessionState(state))
}

// UpdateSessionStateTyped updates the session state with typed SessionState
func (sr *SessionRegistry) UpdateSessionStateTyped(sessionID string, state SessionState) error {
	sr.mu.RLock()
	session, ok := sr.sessions[sessionID]
	sr.mu.RUnlock()

	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.mu.Lock()
	oldState := session.State
	session.State = state
	session.UpdatedAt = time.Now()

	if state == SessionStateActive && session.Stats.ConnectTime.IsZero() {
		session.Stats.ConnectTime = time.Now()
	}
	if state == SessionStateTerminated {
		session.Stats.EndTime = time.Now()
		if !session.Stats.ConnectTime.IsZero() {
			session.Stats.Duration = session.Stats.EndTime.Sub(session.Stats.ConnectTime)
		}
	}
	session.mu.Unlock()

	// Trigger callback on termination
	if state == SessionStateTerminated && oldState != SessionStateTerminated {
		sr.mu.RLock()
		callback := sr.onSessionEnd
		sr.mu.RUnlock()
		if callback != nil {
			go callback(session)
		}
	}

	return nil
}

// SetCallerLeg sets the caller leg for a session
func (sr *SessionRegistry) SetCallerLeg(sessionID string, leg *CallLeg) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	session, ok := sr.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.mu.Lock()
	session.CallerLeg = leg
	if leg.SSRC != 0 {
		session.SSRCToLeg[leg.SSRC] = leg
		sr.ssrcIndex[leg.SSRC] = session
	}
	session.UpdatedAt = time.Now()
	session.mu.Unlock()

	return nil
}

// SetCalleeLeg sets the callee leg for a session
func (sr *SessionRegistry) SetCalleeLeg(sessionID string, leg *CallLeg) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	session, ok := sr.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.mu.Lock()
	session.CalleeLeg = leg
	session.ToTag = leg.Tag
	if leg.SSRC != 0 {
		session.SSRCToLeg[leg.SSRC] = leg
		sr.ssrcIndex[leg.SSRC] = session
	}
	session.UpdatedAt = time.Now()
	session.mu.Unlock()

	return nil
}

// RegisterSSRC registers an SSRC for a session leg
func (sr *SessionRegistry) RegisterSSRC(sessionID string, ssrc uint32, isCaller bool) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	session, ok := sr.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	var leg *CallLeg
	if isCaller {
		leg = session.CallerLeg
	} else {
		leg = session.CalleeLeg
	}

	if leg == nil {
		return fmt.Errorf("leg not found for session: %s", sessionID)
	}

	leg.SSRC = ssrc
	session.SSRCToLeg[ssrc] = leg
	sr.ssrcIndex[ssrc] = session

	return nil
}

// DeleteSession removes a session
func (sr *SessionRegistry) DeleteSession(sessionID string) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	return sr.removeSessionLocked(sessionID)
}

// removeSessionLocked removes a session (caller must hold write lock)
func (sr *SessionRegistry) removeSessionLocked(sessionID string) error {
	session, ok := sr.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Remove from callID index
	sessions := sr.callIDIndex[session.CallID]
	for i, s := range sessions {
		if s.ID == sessionID {
			sr.callIDIndex[session.CallID] = append(sessions[:i], sessions[i+1:]...)
			break
		}
	}
	if len(sr.callIDIndex[session.CallID]) == 0 {
		delete(sr.callIDIndex, session.CallID)
	}

	// Remove from fromTag index
	delete(sr.fromTagIndex, session.FromTag)

	// Remove SSRC mappings
	for ssrc := range session.SSRCToLeg {
		delete(sr.ssrcIndex, ssrc)
	}

	// Close connections
	if session.CallerLeg != nil {
		if session.CallerLeg.Conn != nil {
			session.CallerLeg.Conn.Close()
		}
		if session.CallerLeg.RTCPConn != nil {
			session.CallerLeg.RTCPConn.Close()
		}
	}
	if session.CalleeLeg != nil {
		if session.CalleeLeg.Conn != nil {
			session.CalleeLeg.Conn.Close()
		}
		if session.CalleeLeg.RTCPConn != nil {
			session.CalleeLeg.RTCPConn.Close()
		}
	}

	delete(sr.sessions, sessionID)
	return nil
}

// ListSessions returns all active sessions
func (sr *SessionRegistry) ListSessions() []*MediaSession {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	sessions := make([]*MediaSession, 0, len(sr.sessions))
	for _, session := range sr.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// GetActiveCount returns the number of active sessions
func (sr *SessionRegistry) GetActiveCount() int {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	count := 0
	for _, session := range sr.sessions {
		session.mu.RLock()
		if session.State == SessionStateActive {
			count++
		}
		session.mu.RUnlock()
	}
	return count
}

// GetTotalCount returns total number of sessions (all states)
func (sr *SessionRegistry) GetTotalCount() int {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return len(sr.sessions)
}

// GetStats returns aggregate statistics
func (sr *SessionRegistry) GetStats() map[string]interface{} {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	stats := map[string]interface{}{
		"total_sessions":  len(sr.sessions),
		"active_sessions": 0,
		"pending_sessions": 0,
		"terminated_sessions": 0,
	}

	for _, session := range sr.sessions {
		session.mu.RLock()
		switch session.State {
		case SessionStateActive:
			stats["active_sessions"] = stats["active_sessions"].(int) + 1
		case SessionStatePending, SessionStateNew:
			stats["pending_sessions"] = stats["pending_sessions"].(int) + 1
		case SessionStateTerminated:
			stats["terminated_sessions"] = stats["terminated_sessions"].(int) + 1
		}
		session.mu.RUnlock()
	}

	return stats
}

// UpdateLegStats updates statistics for a call leg
func (session *MediaSession) UpdateLegStats(ssrc uint32, packetsSent, packetsRecv uint64, bytesSent, bytesRecv uint64) {
	session.mu.Lock()
	defer session.mu.Unlock()

	leg, ok := session.SSRCToLeg[ssrc]
	if !ok {
		return
	}

	leg.PacketsSent = packetsSent
	leg.PacketsRecv = packetsRecv
	leg.BytesSent = bytesSent
	leg.BytesRecv = bytesRecv
	leg.LastActivity = time.Now()
}

// SetFlag sets a session flag
func (session *MediaSession) SetFlag(name string, value bool) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.Flags[name] = value
}

// GetFlag gets a session flag
func (session *MediaSession) GetFlag(name string) bool {
	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.Flags[name]
}

// SetMetadata sets session metadata
func (session *MediaSession) SetMetadata(key, value string) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.Metadata[key] = value
}

// GetMetadata gets session metadata
func (session *MediaSession) GetMetadata(key string) string {
	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.Metadata[key]
}

// Stop stops the session registry
func (sr *SessionRegistry) Stop() {
	close(sr.stopCleanup)

	sr.mu.Lock()
	defer sr.mu.Unlock()

	for id := range sr.sessions {
		_ = sr.removeSessionLocked(id)
	}
}

// AllocateMediaPorts allocates RTP/RTCP port pairs for a session
func (sr *SessionRegistry) AllocateMediaPorts(localIP string, minPort, maxPort int) (rtpPort, rtcpPort int, rtpConn, rtcpConn *net.UDPConn, err error) {
	// Try to find an available port pair
	for port := minPort; port < maxPort; port += 2 {
		rtpAddr := &net.UDPAddr{IP: net.ParseIP(localIP), Port: port}
		rtcpAddr := &net.UDPAddr{IP: net.ParseIP(localIP), Port: port + 1}

		rtpConn, err = net.ListenUDP("udp", rtpAddr)
		if err != nil {
			continue
		}

		rtcpConn, err = net.ListenUDP("udp", rtcpAddr)
		if err != nil {
			rtpConn.Close()
			continue
		}

		return port, port + 1, rtpConn, rtcpConn, nil
	}

	return 0, 0, nil, nil, fmt.Errorf("no available port pair in range %d-%d", minPort, maxPort)
}
