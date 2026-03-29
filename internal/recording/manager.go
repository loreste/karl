package recording

import (
	"errors"
	"log"
	"sync"
	"time"

	"karl/internal/api"
)

// Manager manages the recording lifecycle
type Manager struct {
	recorder    *Recorder
	mixer       *AudioMixer
	config      *RecordingConfig
	mu          sync.RWMutex
	stopChan    chan struct{}
	cleanupDone chan struct{}
}

// NewManager creates a new recording manager
func NewManager(config *RecordingConfig) *Manager {
	if config == nil {
		config = DefaultRecordingConfig()
	}

	m := &Manager{
		recorder:    NewRecorder(config),
		mixer:       NewAudioMixer(config.SampleRate, config.BitsPerSample, config.Channels),
		config:      config,
		stopChan:    make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}

	return m
}

// Start starts the recording manager
func (m *Manager) Start() error {
	if err := m.recorder.Start(); err != nil {
		return err
	}

	// Start cleanup goroutine
	go m.cleanupLoop()

	// Register as the API recording manager
	api.SetRecordingManager(m)

	log.Println("Recording manager started")
	return nil
}

// Stop stops the recording manager
func (m *Manager) Stop() error {
	close(m.stopChan)

	// Wait for cleanup to finish
	select {
	case <-m.cleanupDone:
	case <-time.After(5 * time.Second):
		log.Println("Cleanup timeout")
	}

	if err := m.recorder.Stop(); err != nil {
		return err
	}

	log.Println("Recording manager stopped")
	return nil
}

// cleanupLoop periodically cleans up old recordings
func (m *Manager) cleanupLoop() {
	defer close(m.cleanupDone)

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			count, err := m.recorder.CleanupOldRecordings()
			if err != nil {
				log.Printf("Cleanup error: %v", err)
			} else if count > 0 {
				log.Printf("Cleaned up %d old recordings", count)
			}
		case <-m.stopChan:
			return
		}
	}
}

// StartRecording starts a new recording
func (m *Manager) StartRecording(sessionID, callID, format, mode string, metadata map[string]string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Override format/mode if specified
	if format != "" {
		m.config.Format = RecordingFormat(format)
	}
	if mode != "" {
		m.config.Mode = RecordingMode(mode)
	}

	rec, err := m.recorder.StartRecording(sessionID, callID, metadata)
	if err != nil {
		return "", err
	}

	return rec.ID, nil
}

// StopRecording stops a recording
func (m *Manager) StopRecording(recordingID string) error {
	return m.recorder.StopRecording(recordingID)
}

// PauseRecording pauses a recording
func (m *Manager) PauseRecording(recordingID string) error {
	return m.recorder.PauseRecording(recordingID)
}

// ResumeRecording resumes a recording
func (m *Manager) ResumeRecording(recordingID string) error {
	return m.recorder.ResumeRecording(recordingID)
}

// GetRecording returns a recording
func (m *Manager) GetRecording(recordingID string) (*api.RecordingInfo, error) {
	rec, ok := m.recorder.GetRecording(recordingID)
	if !ok {
		return nil, errors.New("recording not found")
	}

	return toRecordingInfo(rec), nil
}

// ListRecordings lists recordings matching filter
func (m *Manager) ListRecordings(filter api.RecordingFilter) ([]*api.RecordingInfo, error) {
	recordings := m.recorder.ListRecordings()
	result := make([]*api.RecordingInfo, 0)

	for _, rec := range recordings {
		// Apply filters
		if filter.SessionID != "" && rec.SessionID != filter.SessionID {
			continue
		}
		if filter.CallID != "" && rec.CallID != filter.CallID {
			continue
		}
		if filter.Status != "" && string(rec.Status) != filter.Status {
			continue
		}
		if !filter.StartFrom.IsZero() && rec.StartTime.Before(filter.StartFrom) {
			continue
		}
		if !filter.StartTo.IsZero() && rec.StartTime.After(filter.StartTo) {
			continue
		}

		result = append(result, toRecordingInfo(rec))
	}

	return result, nil
}

// GetRecordingStatus returns the status of a recording
func (m *Manager) GetRecordingStatus(recordingID string) (string, error) {
	rec, ok := m.recorder.GetRecording(recordingID)
	if !ok {
		return "", errors.New("recording not found")
	}
	return string(rec.Status), nil
}

// WriteAudio writes audio data to a recording
func (m *Manager) WriteAudio(sessionID string, callerData, calleeData []byte) error {
	rec, ok := m.recorder.GetRecordingBySession(sessionID)
	if !ok {
		return nil // No recording for this session
	}

	if rec.Status != StatusRecording {
		return nil
	}

	var audioData []byte

	switch m.config.Mode {
	case ModeMixed:
		// Mix to mono
		audioData = m.mixer.MixMono(callerData, calleeData)
	case ModeStereo:
		// Mix to stereo
		audioData = m.mixer.MixStereo(callerData, calleeData)
	case ModeSeparate:
		// For separate mode, write both
		// This would need separate files - simplified here
		audioData = m.mixer.MixMono(callerData, calleeData)
	default:
		audioData = m.mixer.MixMono(callerData, calleeData)
	}

	return m.recorder.WriteAudio(rec.ID, audioData)
}

// WriteRTPPacket writes an RTP packet to a recording
func (m *Manager) WriteRTPPacket(sessionID string, payload []byte, payloadType uint8, isCaller bool) error {
	rec, ok := m.recorder.GetRecordingBySession(sessionID)
	if !ok {
		return nil // No recording for this session
	}

	if rec.Status != StatusRecording {
		return nil
	}

	// Convert payload based on codec
	var pcmData []byte
	switch payloadType {
	case 0: // PCMU (G.711 u-law)
		pcmData = ConvertG711uToPCM(payload)
	case 8: // PCMA (G.711 a-law)
		pcmData = ConvertG711aToPCM(payload)
	default:
		// For other codecs, we'd need transcoding
		// For now, just use raw payload
		pcmData = payload
	}

	return m.recorder.WriteAudio(rec.ID, pcmData)
}

// GetStats returns recording statistics
func (m *Manager) GetStats() RecorderStats {
	return m.recorder.GetStats()
}

// toRecordingInfo converts Recording to api.RecordingInfo
func toRecordingInfo(rec *Recording) *api.RecordingInfo {
	return &api.RecordingInfo{
		ID:        rec.ID,
		SessionID: rec.SessionID,
		CallID:    rec.CallID,
		Status:    string(rec.Status),
		StartTime: rec.StartTime,
		EndTime:   rec.EndTime,
		Duration:  rec.Duration,
		FilePath:  rec.FilePath,
		FileSize:  rec.FileSize,
		Format:    string(rec.Format),
		Mode:      string(rec.Mode),
		Metadata:  rec.Metadata,
	}
}

// RecordingCallback is a callback for recording events
type RecordingCallback func(event RecordingEvent)

// RecordingEvent represents a recording event
type RecordingEvent struct {
	Type        string
	RecordingID string
	SessionID   string
	CallID      string
	Timestamp   time.Time
	Details     map[string]interface{}
}

// Event types
const (
	EventRecordingStarted   = "recording_started"
	EventRecordingStopped   = "recording_stopped"
	EventRecordingPaused    = "recording_paused"
	EventRecordingResumed   = "recording_resumed"
	EventRecordingError     = "recording_error"
	EventRecordingCompleted = "recording_completed"
)
