package internal

import (
	"encoding/xml"
	"fmt"
	"sync"
	"time"
)

// SIPRECRecorder handles SIPREC-compliant call recording per RFC 7865/7866
type SIPRECRecorder struct {
	sessions map[string]*SIPRECSession
	config   *RecordingConfig
	mu       sync.RWMutex
}

// SIPRECSession represents an active SIPREC recording session
type SIPRECSession struct {
	ID              string
	CallID          string
	SessionID       string
	ParticipantInfo []ParticipantInfo
	MediaStreams    []MediaStreamInfo
	StartTime       time.Time
	Metadata        map[string]string
	Recording       *SessionRecording
	Status          SIPRECStatus
}

// SIPRECStatus represents the status of a SIPREC session
type SIPRECStatus string

const (
	SIPRECStatusInitial  SIPRECStatus = "initial"
	SIPRECStatusActive   SIPRECStatus = "active"
	SIPRECStatusPaused   SIPRECStatus = "paused"
	SIPRECStatusComplete SIPRECStatus = "complete"
	SIPRECStatusFailed   SIPRECStatus = "failed"
)

// ParticipantInfo represents a participant in a recorded session
type ParticipantInfo struct {
	ID          string
	AOR         string // Address of Record (SIP URI)
	Name        string
	Role        string // caller, callee, conferee
	StartTime   time.Time
	EndTime     time.Time
	MediaLabels []string
}

// MediaStreamInfo represents a media stream in a recorded session
type MediaStreamInfo struct {
	ID            string
	Type          string // audio, video
	Label         string
	ParticipantID string
	Direction     string // sendonly, recvonly, sendrecv
	Codec         string
	SampleRate    int
	Channels      int
}

// RecordingMetadataXML represents SIPREC recording metadata XML (RFC 7865)
type RecordingMetadataXML struct {
	XMLName      xml.Name            `xml:"recording"`
	XMLNS        string              `xml:"xmlns,attr"`
	DataMode     string              `xml:"datamode,attr,omitempty"`
	Session      SessionXML          `xml:"session"`
	Participants []ParticipantXML    `xml:"participant"`
	Streams      []StreamXML         `xml:"stream"`
	Associations []AssociationXML    `xml:"association"`
}

// SessionXML represents session metadata
type SessionXML struct {
	ID             string `xml:"session-id,attr"`
	GroupRef       string `xml:"group-ref,attr,omitempty"`
	AssociateTime  string `xml:"associate-time,attr,omitempty"`
	DisassociateTime string `xml:"disassociate-time,attr,omitempty"`
}

// ParticipantXML represents participant metadata
type ParticipantXML struct {
	ID        string `xml:"participant-id,attr"`
	AOR       string `xml:"aor,omitempty"`
	Name      string `xml:"name,omitempty"`
	StartTime string `xml:"start-time,omitempty"`
	EndTime   string `xml:"end-time,omitempty"`
}

// StreamXML represents stream metadata
type StreamXML struct {
	ID      string `xml:"stream-id,attr"`
	Session string `xml:"session,attr"`
	Label   string `xml:"label,omitempty"`
	Mode    string `xml:"mode,omitempty"`
}

// AssociationXML represents the association between participants and streams
type AssociationXML struct {
	ID          string `xml:"id,attr"`
	Participant string `xml:"participant,attr"`
	Stream      string `xml:"stream,attr"`
}

// NewSIPRECRecorder creates a new SIPREC recorder
func NewSIPRECRecorder(config *RecordingConfig) *SIPRECRecorder {
	return &SIPRECRecorder{
		sessions: make(map[string]*SIPRECSession),
		config:   config,
	}
}

// StartRecording starts a SIPREC recording session
func (sr *SIPRECRecorder) StartRecording(callID string, metadata map[string]string) (*SIPRECSession, error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	sessionID := fmt.Sprintf("siprec-%s-%d", callID, time.Now().UnixNano())

	session := &SIPRECSession{
		ID:        sessionID,
		CallID:    callID,
		SessionID: sessionID,
		StartTime: time.Now(),
		Metadata:  metadata,
		Status:    SIPRECStatusActive,
		ParticipantInfo: make([]ParticipantInfo, 0),
		MediaStreams:    make([]MediaStreamInfo, 0),
	}

	// Create recording info
	session.Recording = &SessionRecording{
		Active:    true,
		RecordID:  sessionID,
		StartTime: time.Now(),
		Format:    sr.config.Format,
		Mode:      sr.config.Mode,
	}

	// Generate file path
	session.Recording.FilePath = fmt.Sprintf("%s/%s_%s.%s",
		sr.config.BasePath,
		callID,
		time.Now().Format("20060102-150405"),
		sr.config.Format,
	)

	sr.sessions[sessionID] = session
	return session, nil
}

// AddParticipant adds a participant to a recording session
func (sr *SIPRECRecorder) AddParticipant(sessionID string, participant ParticipantInfo) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	session, exists := sr.sessions[sessionID]
	if !exists {
		return fmt.Errorf("SIPREC session not found: %s", sessionID)
	}

	participant.StartTime = time.Now()
	session.ParticipantInfo = append(session.ParticipantInfo, participant)
	return nil
}

// AddMediaStream adds a media stream to a recording session
func (sr *SIPRECRecorder) AddMediaStream(sessionID string, stream MediaStreamInfo) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	session, exists := sr.sessions[sessionID]
	if !exists {
		return fmt.Errorf("SIPREC session not found: %s", sessionID)
	}

	session.MediaStreams = append(session.MediaStreams, stream)
	return nil
}

// PauseRecording pauses a recording session
func (sr *SIPRECRecorder) PauseRecording(sessionID string) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	session, exists := sr.sessions[sessionID]
	if !exists {
		return fmt.Errorf("SIPREC session not found: %s", sessionID)
	}

	if session.Status != SIPRECStatusActive {
		return fmt.Errorf("cannot pause session in status: %s", session.Status)
	}

	session.Status = SIPRECStatusPaused
	session.Recording.Active = false
	return nil
}

// ResumeRecording resumes a paused recording session
func (sr *SIPRECRecorder) ResumeRecording(sessionID string) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	session, exists := sr.sessions[sessionID]
	if !exists {
		return fmt.Errorf("SIPREC session not found: %s", sessionID)
	}

	if session.Status != SIPRECStatusPaused {
		return fmt.Errorf("cannot resume session in status: %s", session.Status)
	}

	session.Status = SIPRECStatusActive
	session.Recording.Active = true
	return nil
}

// StopRecording stops a recording session
func (sr *SIPRECRecorder) StopRecording(sessionID string) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	session, exists := sr.sessions[sessionID]
	if !exists {
		return fmt.Errorf("SIPREC session not found: %s", sessionID)
	}

	session.Status = SIPRECStatusComplete
	session.Recording.Active = false

	// Mark all participants as ended
	now := time.Now()
	for i := range session.ParticipantInfo {
		if session.ParticipantInfo[i].EndTime.IsZero() {
			session.ParticipantInfo[i].EndTime = now
		}
	}

	return nil
}

// GetSession returns a SIPREC session by ID
func (sr *SIPRECRecorder) GetSession(sessionID string) (*SIPRECSession, bool) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	session, exists := sr.sessions[sessionID]
	return session, exists
}

// GetSessionByCallID returns SIPREC sessions for a call
func (sr *SIPRECRecorder) GetSessionByCallID(callID string) []*SIPRECSession {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	var sessions []*SIPRECSession
	for _, session := range sr.sessions {
		if session.CallID == callID {
			sessions = append(sessions, session)
		}
	}
	return sessions
}

// GenerateMetadataXML generates SIPREC metadata XML per RFC 7865
func (sr *SIPRECRecorder) GenerateMetadataXML(sessionID string) ([]byte, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	session, exists := sr.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("SIPREC session not found: %s", sessionID)
	}

	metadata := RecordingMetadataXML{
		XMLNS: "urn:ietf:params:xml:ns:recording:1",
		Session: SessionXML{
			ID:            session.SessionID,
			AssociateTime: session.StartTime.Format(time.RFC3339),
		},
		Participants: make([]ParticipantXML, len(session.ParticipantInfo)),
		Streams:      make([]StreamXML, len(session.MediaStreams)),
		Associations: make([]AssociationXML, 0),
	}

	// Add participants
	for i, p := range session.ParticipantInfo {
		metadata.Participants[i] = ParticipantXML{
			ID:        p.ID,
			AOR:       p.AOR,
			Name:      p.Name,
			StartTime: p.StartTime.Format(time.RFC3339),
		}
		if !p.EndTime.IsZero() {
			metadata.Participants[i].EndTime = p.EndTime.Format(time.RFC3339)
		}
	}

	// Add streams
	for i, s := range session.MediaStreams {
		metadata.Streams[i] = StreamXML{
			ID:      s.ID,
			Session: session.SessionID,
			Label:   s.Label,
			Mode:    s.Direction,
		}

		// Add association
		metadata.Associations = append(metadata.Associations, AssociationXML{
			ID:          fmt.Sprintf("assoc-%d", i),
			Participant: s.ParticipantID,
			Stream:      s.ID,
		})
	}

	return xml.MarshalIndent(metadata, "", "  ")
}

// RemoveSession removes a completed session
func (sr *SIPRECRecorder) RemoveSession(sessionID string) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	delete(sr.sessions, sessionID)
}

// GetActiveCount returns the number of active recording sessions
func (sr *SIPRECRecorder) GetActiveCount() int {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	count := 0
	for _, session := range sr.sessions {
		if session.Status == SIPRECStatusActive {
			count++
		}
	}
	return count
}

// Cleanup removes old completed sessions
func (sr *SIPRECRecorder) Cleanup(maxAge time.Duration) int {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	now := time.Now()
	removed := 0
	for id, session := range sr.sessions {
		if session.Status == SIPRECStatusComplete && now.Sub(session.StartTime) > maxAge {
			delete(sr.sessions, id)
			removed++
		}
	}
	return removed
}
