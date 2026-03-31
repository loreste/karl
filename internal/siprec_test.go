package internal

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

func TestNewSIPRECRecorder(t *testing.T) {
	config := &RecordingConfig{
		Enabled:  true,
		BasePath: "/tmp/recordings",
		Format:   "wav",
		Mode:     "mixed",
	}

	sr := NewSIPRECRecorder(config)
	if sr == nil {
		t.Fatal("NewSIPRECRecorder returned nil")
	}
}

func TestSIPRECRecorder_StartRecording(t *testing.T) {
	config := &RecordingConfig{
		Enabled:  true,
		BasePath: "/tmp/recordings",
		Format:   "wav",
		Mode:     "mixed",
	}

	sr := NewSIPRECRecorder(config)

	metadata := map[string]string{
		"user":     "alice",
		"calledTo": "bob",
	}

	session, err := sr.StartRecording("call-123", metadata)
	if err != nil {
		t.Fatalf("StartRecording failed: %v", err)
	}

	if session == nil {
		t.Fatal("StartRecording returned nil session")
	}

	if session.CallID != "call-123" {
		t.Errorf("Expected CallID call-123, got %s", session.CallID)
	}

	if session.Status != SIPRECStatusActive {
		t.Errorf("Expected status active, got %s", session.Status)
	}

	if session.Recording == nil {
		t.Fatal("Recording info is nil")
	}

	if !session.Recording.Active {
		t.Error("Recording should be active")
	}

	if !strings.Contains(session.Recording.FilePath, "call-123") {
		t.Errorf("FilePath should contain call-id: %s", session.Recording.FilePath)
	}
}

func TestSIPRECRecorder_AddParticipant(t *testing.T) {
	sr := NewSIPRECRecorder(&RecordingConfig{
		BasePath: "/tmp",
		Format:   "wav",
		Mode:     "mixed",
	})

	session, _ := sr.StartRecording("call-123", nil)

	participant := ParticipantInfo{
		ID:   "p1",
		AOR:  "sip:alice@example.com",
		Name: "Alice",
		Role: "caller",
	}

	err := sr.AddParticipant(session.ID, participant)
	if err != nil {
		t.Fatalf("AddParticipant failed: %v", err)
	}

	// Verify participant was added
	s, exists := sr.GetSession(session.ID)
	if !exists {
		t.Fatal("Session not found")
	}

	if len(s.ParticipantInfo) != 1 {
		t.Errorf("Expected 1 participant, got %d", len(s.ParticipantInfo))
	}

	if s.ParticipantInfo[0].Name != "Alice" {
		t.Errorf("Expected name Alice, got %s", s.ParticipantInfo[0].Name)
	}
}

func TestSIPRECRecorder_AddParticipant_InvalidSession(t *testing.T) {
	sr := NewSIPRECRecorder(&RecordingConfig{
		BasePath: "/tmp",
		Format:   "wav",
		Mode:     "mixed",
	})

	err := sr.AddParticipant("nonexistent", ParticipantInfo{})
	if err == nil {
		t.Error("Expected error for nonexistent session")
	}
}

func TestSIPRECRecorder_AddMediaStream(t *testing.T) {
	sr := NewSIPRECRecorder(&RecordingConfig{
		BasePath: "/tmp",
		Format:   "wav",
		Mode:     "mixed",
	})

	session, _ := sr.StartRecording("call-123", nil)

	stream := MediaStreamInfo{
		ID:            "s1",
		Type:          "audio",
		Label:         "caller-audio",
		ParticipantID: "p1",
		Direction:     "sendonly",
		Codec:         "PCMU",
		SampleRate:    8000,
		Channels:      1,
	}

	err := sr.AddMediaStream(session.ID, stream)
	if err != nil {
		t.Fatalf("AddMediaStream failed: %v", err)
	}

	s, _ := sr.GetSession(session.ID)
	if len(s.MediaStreams) != 1 {
		t.Errorf("Expected 1 media stream, got %d", len(s.MediaStreams))
	}
}

func TestSIPRECRecorder_PauseRecording(t *testing.T) {
	sr := NewSIPRECRecorder(&RecordingConfig{
		BasePath: "/tmp",
		Format:   "wav",
		Mode:     "mixed",
	})

	session, _ := sr.StartRecording("call-123", nil)

	err := sr.PauseRecording(session.ID)
	if err != nil {
		t.Fatalf("PauseRecording failed: %v", err)
	}

	s, _ := sr.GetSession(session.ID)
	if s.Status != SIPRECStatusPaused {
		t.Errorf("Expected status paused, got %s", s.Status)
	}

	if s.Recording.Active {
		t.Error("Recording should not be active after pause")
	}
}

func TestSIPRECRecorder_PauseRecording_NotActive(t *testing.T) {
	sr := NewSIPRECRecorder(&RecordingConfig{
		BasePath: "/tmp",
		Format:   "wav",
		Mode:     "mixed",
	})

	session, _ := sr.StartRecording("call-123", nil)
	sr.PauseRecording(session.ID)

	// Try to pause again
	err := sr.PauseRecording(session.ID)
	if err == nil {
		t.Error("Expected error when pausing non-active session")
	}
}

func TestSIPRECRecorder_ResumeRecording(t *testing.T) {
	sr := NewSIPRECRecorder(&RecordingConfig{
		BasePath: "/tmp",
		Format:   "wav",
		Mode:     "mixed",
	})

	session, _ := sr.StartRecording("call-123", nil)
	sr.PauseRecording(session.ID)

	err := sr.ResumeRecording(session.ID)
	if err != nil {
		t.Fatalf("ResumeRecording failed: %v", err)
	}

	s, _ := sr.GetSession(session.ID)
	if s.Status != SIPRECStatusActive {
		t.Errorf("Expected status active, got %s", s.Status)
	}

	if !s.Recording.Active {
		t.Error("Recording should be active after resume")
	}
}

func TestSIPRECRecorder_ResumeRecording_NotPaused(t *testing.T) {
	sr := NewSIPRECRecorder(&RecordingConfig{
		BasePath: "/tmp",
		Format:   "wav",
		Mode:     "mixed",
	})

	session, _ := sr.StartRecording("call-123", nil)

	// Try to resume active session
	err := sr.ResumeRecording(session.ID)
	if err == nil {
		t.Error("Expected error when resuming non-paused session")
	}
}

func TestSIPRECRecorder_StopRecording(t *testing.T) {
	sr := NewSIPRECRecorder(&RecordingConfig{
		BasePath: "/tmp",
		Format:   "wav",
		Mode:     "mixed",
	})

	session, _ := sr.StartRecording("call-123", nil)

	// Add a participant
	sr.AddParticipant(session.ID, ParticipantInfo{
		ID:   "p1",
		Name: "Alice",
	})

	err := sr.StopRecording(session.ID)
	if err != nil {
		t.Fatalf("StopRecording failed: %v", err)
	}

	s, _ := sr.GetSession(session.ID)
	if s.Status != SIPRECStatusComplete {
		t.Errorf("Expected status complete, got %s", s.Status)
	}

	if s.Recording.Active {
		t.Error("Recording should not be active after stop")
	}

	// Participant end time should be set
	if s.ParticipantInfo[0].EndTime.IsZero() {
		t.Error("Participant end time should be set after stop")
	}
}

func TestSIPRECRecorder_GetSession(t *testing.T) {
	sr := NewSIPRECRecorder(&RecordingConfig{
		BasePath: "/tmp",
		Format:   "wav",
		Mode:     "mixed",
	})

	// Non-existent session
	_, exists := sr.GetSession("nonexistent")
	if exists {
		t.Error("Expected false for nonexistent session")
	}

	// Existing session
	session, _ := sr.StartRecording("call-123", nil)
	s, exists := sr.GetSession(session.ID)
	if !exists {
		t.Error("Expected true for existing session")
	}
	if s.ID != session.ID {
		t.Error("Session IDs should match")
	}
}

func TestSIPRECRecorder_GetSessionByCallID(t *testing.T) {
	sr := NewSIPRECRecorder(&RecordingConfig{
		BasePath: "/tmp",
		Format:   "wav",
		Mode:     "mixed",
	})

	sr.StartRecording("call-123", nil)
	sr.StartRecording("call-456", nil)
	sr.StartRecording("call-123", nil) // Another session for same call

	sessions := sr.GetSessionByCallID("call-123")
	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions for call-123, got %d", len(sessions))
	}
}

func TestSIPRECRecorder_GenerateMetadataXML(t *testing.T) {
	sr := NewSIPRECRecorder(&RecordingConfig{
		BasePath: "/tmp",
		Format:   "wav",
		Mode:     "mixed",
	})

	session, _ := sr.StartRecording("call-123", nil)

	// Add participants
	sr.AddParticipant(session.ID, ParticipantInfo{
		ID:   "p1",
		AOR:  "sip:alice@example.com",
		Name: "Alice",
		Role: "caller",
	})
	sr.AddParticipant(session.ID, ParticipantInfo{
		ID:   "p2",
		AOR:  "sip:bob@example.com",
		Name: "Bob",
		Role: "callee",
	})

	// Add media streams
	sr.AddMediaStream(session.ID, MediaStreamInfo{
		ID:            "s1",
		Type:          "audio",
		Label:         "caller-audio",
		ParticipantID: "p1",
		Direction:     "sendonly",
	})
	sr.AddMediaStream(session.ID, MediaStreamInfo{
		ID:            "s2",
		Type:          "audio",
		Label:         "callee-audio",
		ParticipantID: "p2",
		Direction:     "recvonly",
	})

	xmlData, err := sr.GenerateMetadataXML(session.ID)
	if err != nil {
		t.Fatalf("GenerateMetadataXML failed: %v", err)
	}

	// Verify valid XML
	var metadata RecordingMetadataXML
	if err := xml.Unmarshal(xmlData, &metadata); err != nil {
		t.Fatalf("Generated XML is invalid: %v", err)
	}

	// Verify content
	if metadata.XMLNS != "urn:ietf:params:xml:ns:recording:1" {
		t.Errorf("Expected SIPREC namespace, got %s", metadata.XMLNS)
	}

	if len(metadata.Participants) != 2 {
		t.Errorf("Expected 2 participants, got %d", len(metadata.Participants))
	}

	if len(metadata.Streams) != 2 {
		t.Errorf("Expected 2 streams, got %d", len(metadata.Streams))
	}

	if len(metadata.Associations) != 2 {
		t.Errorf("Expected 2 associations, got %d", len(metadata.Associations))
	}
}

func TestSIPRECRecorder_GenerateMetadataXML_InvalidSession(t *testing.T) {
	sr := NewSIPRECRecorder(&RecordingConfig{
		BasePath: "/tmp",
		Format:   "wav",
		Mode:     "mixed",
	})

	_, err := sr.GenerateMetadataXML("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent session")
	}
}

func TestSIPRECRecorder_RemoveSession(t *testing.T) {
	sr := NewSIPRECRecorder(&RecordingConfig{
		BasePath: "/tmp",
		Format:   "wav",
		Mode:     "mixed",
	})

	session, _ := sr.StartRecording("call-123", nil)
	sr.RemoveSession(session.ID)

	_, exists := sr.GetSession(session.ID)
	if exists {
		t.Error("Session should be removed")
	}
}

func TestSIPRECRecorder_GetActiveCount(t *testing.T) {
	sr := NewSIPRECRecorder(&RecordingConfig{
		BasePath: "/tmp",
		Format:   "wav",
		Mode:     "mixed",
	})

	if sr.GetActiveCount() != 0 {
		t.Error("Initial active count should be 0")
	}

	s1, _ := sr.StartRecording("call-1", nil)
	s2, _ := sr.StartRecording("call-2", nil)
	sr.StartRecording("call-3", nil)

	if sr.GetActiveCount() != 3 {
		t.Errorf("Expected 3 active, got %d", sr.GetActiveCount())
	}

	sr.PauseRecording(s1.ID)
	if sr.GetActiveCount() != 2 {
		t.Errorf("Expected 2 active after pause, got %d", sr.GetActiveCount())
	}

	sr.StopRecording(s2.ID)
	if sr.GetActiveCount() != 1 {
		t.Errorf("Expected 1 active after stop, got %d", sr.GetActiveCount())
	}
}

func TestSIPRECRecorder_Cleanup(t *testing.T) {
	sr := NewSIPRECRecorder(&RecordingConfig{
		BasePath: "/tmp",
		Format:   "wav",
		Mode:     "mixed",
	})

	// Create and stop some sessions
	s1, _ := sr.StartRecording("call-1", nil)
	s2, _ := sr.StartRecording("call-2", nil)
	sr.StartRecording("call-3", nil) // This one stays active

	sr.StopRecording(s1.ID)
	sr.StopRecording(s2.ID)

	// Cleanup with 0 duration should remove all completed
	// Note: StartTime is recent, so with actual duration check it may not remove
	// We need to test with maxAge > 0 but sessions are just created
	removed := sr.Cleanup(24 * time.Hour)
	// Sessions are too new, none should be removed
	if removed != 0 {
		t.Errorf("Expected 0 removed (sessions too new), got %d", removed)
	}
}

func TestSIPRECStatus_Constants(t *testing.T) {
	// Verify status constants
	if SIPRECStatusInitial != "initial" {
		t.Error("SIPRECStatusInitial mismatch")
	}
	if SIPRECStatusActive != "active" {
		t.Error("SIPRECStatusActive mismatch")
	}
	if SIPRECStatusPaused != "paused" {
		t.Error("SIPRECStatusPaused mismatch")
	}
	if SIPRECStatusComplete != "complete" {
		t.Error("SIPRECStatusComplete mismatch")
	}
	if SIPRECStatusFailed != "failed" {
		t.Error("SIPRECStatusFailed mismatch")
	}
}
