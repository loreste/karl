package recording

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Recording metrics
var (
	activeRecordings = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "karl_active_recordings",
			Help: "Number of active recordings",
		},
	)

	recordingBytesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "karl_recording_bytes_total",
			Help: "Total bytes recorded",
		},
	)

	recordingPacketsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "karl_recording_packets_total",
			Help: "Total packets recorded",
		},
	)

	recordingErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "karl_recording_errors_total",
			Help: "Total recording errors",
		},
	)
)

// RecordingFormat represents the output format
type RecordingFormat string

const (
	FormatWAV RecordingFormat = "wav"
	FormatPCM RecordingFormat = "pcm"
)

// RecordingMode represents how to record the call
type RecordingMode string

const (
	ModeMixed    RecordingMode = "mixed"    // Both legs mixed to mono
	ModeStereo   RecordingMode = "stereo"   // Caller=left, Callee=right
	ModeSeparate RecordingMode = "separate" // Separate files for each leg
)

// RecordingStatus represents recording state
type RecordingStatus string

const (
	StatusPending   RecordingStatus = "pending"
	StatusRecording RecordingStatus = "recording"
	StatusPaused    RecordingStatus = "paused"
	StatusCompleted RecordingStatus = "completed"
	StatusFailed    RecordingStatus = "failed"
)

// RecordingConfig holds recording configuration
type RecordingConfig struct {
	BasePath      string
	Format        RecordingFormat
	Mode          RecordingMode
	SampleRate    int
	BitsPerSample int
	Channels      int
	MaxFileSize   int64 // Max file size before rotation
	RetentionDays int
}

// DefaultRecordingConfig returns default configuration
func DefaultRecordingConfig() *RecordingConfig {
	return &RecordingConfig{
		BasePath:      "/var/lib/karl/recordings",
		Format:        FormatWAV,
		Mode:          ModeMixed,
		SampleRate:    8000,
		BitsPerSample: 16,
		Channels:      1,
		MaxFileSize:   100 * 1024 * 1024, // 100MB
		RetentionDays: 30,
	}
}

// Recording represents an active or completed recording
type Recording struct {
	ID          string
	SessionID   string
	CallID      string
	Status      RecordingStatus
	Format      RecordingFormat
	Mode        RecordingMode
	StartTime   time.Time
	EndTime     time.Time
	Duration    time.Duration
	FilePath    string
	FileSize    int64
	SampleRate  int
	Channels    int
	Metadata    map[string]string

	// Internal state
	file        *os.File
	writer      *WAVWriter
	mu          sync.Mutex
	packetCount uint64
	byteCount   uint64
}

// Recorder handles recording of media streams
type Recorder struct {
	config      *RecordingConfig
	recordings  map[string]*Recording
	sessionRecs map[string]string // sessionID -> recordingID
	mu          sync.RWMutex
	stopChan    chan struct{}
}

// NewRecorder creates a new recorder
func NewRecorder(config *RecordingConfig) *Recorder {
	if config == nil {
		config = DefaultRecordingConfig()
	}

	// Ensure base path exists
	if err := os.MkdirAll(config.BasePath, 0755); err != nil {
		log.Printf("Warning: failed to create recording base path %s: %v", config.BasePath, err)
	}

	return &Recorder{
		config:      config,
		recordings:  make(map[string]*Recording),
		sessionRecs: make(map[string]string),
		stopChan:    make(chan struct{}),
	}
}

// Start starts the recorder service
func (r *Recorder) Start() error {
	log.Printf("Recording service started, base path: %s", r.config.BasePath)
	return nil
}

// Stop stops the recorder service
func (r *Recorder) Stop() error {
	close(r.stopChan)

	// Stop all active recordings
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, rec := range r.recordings {
		if rec.Status == StatusRecording {
			_ = r.stopRecordingInternal(rec)
		}
	}

	log.Println("Recording service stopped")
	return nil
}

// StartRecording starts a new recording
func (r *Recorder) StartRecording(sessionID, callID string, metadata map[string]string) (*Recording, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if already recording
	if recID, exists := r.sessionRecs[sessionID]; exists {
		if rec, ok := r.recordings[recID]; ok && rec.Status == StatusRecording {
			return nil, errors.New("session already being recorded")
		}
	}

	// Generate file path
	now := time.Now()
	dateDir := now.Format("2006/01/02")
	fileName := fmt.Sprintf("%s_%s.wav", callID, now.Format("150405"))
	filePath := filepath.Join(r.config.BasePath, dateDir, fileName)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Create recording
	rec := &Recording{
		ID:         uuid.New().String(),
		SessionID:  sessionID,
		CallID:     callID,
		Status:     StatusRecording,
		Format:     r.config.Format,
		Mode:       r.config.Mode,
		StartTime:  now,
		FilePath:   filePath,
		SampleRate: r.config.SampleRate,
		Channels:   r.config.Channels,
		Metadata:   metadata,
	}

	// Open file
	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	rec.file = file

	// Create WAV writer
	rec.writer = NewWAVWriter(file, r.config.SampleRate, r.config.BitsPerSample, r.config.Channels)
	if err := rec.writer.WriteHeader(); err != nil {
		file.Close()
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to write WAV header: %w", err)
	}

	r.recordings[rec.ID] = rec
	r.sessionRecs[sessionID] = rec.ID

	activeRecordings.Inc()
	log.Printf("Started recording %s for session %s", rec.ID, sessionID)

	return rec, nil
}

// StopRecording stops a recording
func (r *Recorder) StopRecording(recordingID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.recordings[recordingID]
	if !ok {
		return errors.New("recording not found")
	}

	return r.stopRecordingInternal(rec)
}

// StopRecordingBySession stops recording by session ID
func (r *Recorder) StopRecordingBySession(sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	recID, ok := r.sessionRecs[sessionID]
	if !ok {
		return errors.New("no recording for session")
	}

	rec, ok := r.recordings[recID]
	if !ok {
		return errors.New("recording not found")
	}

	return r.stopRecordingInternal(rec)
}

// stopRecordingInternal stops a recording (must hold lock)
func (r *Recorder) stopRecordingInternal(rec *Recording) error {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	if rec.Status != StatusRecording && rec.Status != StatusPaused {
		return nil
	}

	// Finalize WAV file
	if rec.writer != nil {
		if err := rec.writer.Finalize(); err != nil {
			log.Printf("Error finalizing WAV: %v", err)
		}
	}

	// Close file
	if rec.file != nil {
		rec.file.Close()
		rec.file = nil
	}

	// Get file size
	if info, err := os.Stat(rec.FilePath); err == nil {
		rec.FileSize = info.Size()
	}

	rec.EndTime = time.Now()
	rec.Duration = rec.EndTime.Sub(rec.StartTime)
	rec.Status = StatusCompleted

	activeRecordings.Dec()
	log.Printf("Stopped recording %s, duration: %v, size: %d bytes",
		rec.ID, rec.Duration, rec.FileSize)

	return nil
}

// PauseRecording pauses a recording
func (r *Recorder) PauseRecording(recordingID string) error {
	r.mu.RLock()
	rec, ok := r.recordings[recordingID]
	r.mu.RUnlock()

	if !ok {
		return errors.New("recording not found")
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()

	if rec.Status != StatusRecording {
		return errors.New("recording not active")
	}

	rec.Status = StatusPaused
	log.Printf("Paused recording %s", recordingID)

	return nil
}

// ResumeRecording resumes a paused recording
func (r *Recorder) ResumeRecording(recordingID string) error {
	r.mu.RLock()
	rec, ok := r.recordings[recordingID]
	r.mu.RUnlock()

	if !ok {
		return errors.New("recording not found")
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()

	if rec.Status != StatusPaused {
		return errors.New("recording not paused")
	}

	rec.Status = StatusRecording
	log.Printf("Resumed recording %s", recordingID)

	return nil
}

// WriteAudio writes audio data to a recording
func (r *Recorder) WriteAudio(recordingID string, data []byte) error {
	r.mu.RLock()
	rec, ok := r.recordings[recordingID]
	r.mu.RUnlock()

	if !ok {
		return errors.New("recording not found")
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()

	if rec.Status != StatusRecording {
		return nil // Silently ignore if not recording
	}

	if rec.writer == nil {
		return errors.New("writer not initialized")
	}

	n, err := rec.writer.WriteData(data)
	if err != nil {
		recordingErrors.Inc()
		return err
	}

	rec.packetCount++
	rec.byteCount += uint64(n)

	recordingBytesTotal.Add(float64(n))
	recordingPacketsTotal.Inc()

	// Check file size for rotation (not yet implemented)
	// When implemented, this will rotate to a new file when MaxFileSize is exceeded

	return nil
}

// WriteAudioBySession writes audio to a recording by session ID
func (r *Recorder) WriteAudioBySession(sessionID string, data []byte, isCaller bool) error {
	r.mu.RLock()
	recID, ok := r.sessionRecs[sessionID]
	r.mu.RUnlock()

	if !ok {
		return nil // No recording for this session
	}

	return r.WriteAudio(recID, data)
}

// GetRecording returns a recording by ID
func (r *Recorder) GetRecording(recordingID string) (*Recording, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.recordings[recordingID]
	return rec, ok
}

// GetRecordingBySession returns a recording by session ID
func (r *Recorder) GetRecordingBySession(sessionID string) (*Recording, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	recID, ok := r.sessionRecs[sessionID]
	if !ok {
		return nil, false
	}

	rec, ok := r.recordings[recID]
	return rec, ok
}

// ListRecordings returns all recordings
func (r *Recorder) ListRecordings() []*Recording {
	r.mu.RLock()
	defer r.mu.RUnlock()

	recordings := make([]*Recording, 0, len(r.recordings))
	for _, rec := range r.recordings {
		recordings = append(recordings, rec)
	}
	return recordings
}

// DeleteRecording deletes a recording
func (r *Recorder) DeleteRecording(recordingID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.recordings[recordingID]
	if !ok {
		return errors.New("recording not found")
	}

	// Stop if still recording
	if rec.Status == StatusRecording {
		_ = r.stopRecordingInternal(rec)
	}

	// Delete file
	if rec.FilePath != "" {
		os.Remove(rec.FilePath)
	}

	// Remove from maps
	delete(r.recordings, recordingID)
	delete(r.sessionRecs, rec.SessionID)

	return nil
}

// CleanupOldRecordings removes recordings older than retention period
func (r *Recorder) CleanupOldRecordings() (int, error) {
	if r.config.RetentionDays <= 0 {
		return 0, nil
	}

	cutoff := time.Now().AddDate(0, 0, -r.config.RetentionDays)
	count := 0

	r.mu.Lock()
	defer r.mu.Unlock()

	for id, rec := range r.recordings {
		if rec.Status == StatusCompleted && rec.EndTime.Before(cutoff) {
			if rec.FilePath != "" {
				os.Remove(rec.FilePath)
			}
			delete(r.recordings, id)
			delete(r.sessionRecs, rec.SessionID)
			count++
		}
	}

	log.Printf("Cleaned up %d old recordings", count)
	return count, nil
}

// GetStats returns recording statistics
type RecorderStats struct {
	ActiveRecordings   int
	TotalRecordings    int
	TotalBytesRecorded uint64
	TotalPackets       uint64
}

// GetStats returns recorder statistics
func (r *Recorder) GetStats() RecorderStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := RecorderStats{
		TotalRecordings: len(r.recordings),
	}

	for _, rec := range r.recordings {
		if rec.Status == StatusRecording {
			stats.ActiveRecordings++
		}
		stats.TotalBytesRecorded += rec.byteCount
		stats.TotalPackets += rec.packetCount
	}

	return stats
}

// WAVWriter writes WAV format audio
type WAVWriter struct {
	w             io.WriteSeeker
	sampleRate    int
	bitsPerSample int
	channels      int
	dataSize      uint32
	headerWritten bool
}

// NewWAVWriter creates a new WAV writer
func NewWAVWriter(w io.WriteSeeker, sampleRate, bitsPerSample, channels int) *WAVWriter {
	return &WAVWriter{
		w:             w,
		sampleRate:    sampleRate,
		bitsPerSample: bitsPerSample,
		channels:      channels,
	}
}

// WriteHeader writes the WAV header
func (w *WAVWriter) WriteHeader() error {
	// Write placeholder header
	header := make([]byte, 44)

	// RIFF header
	copy(header[0:4], "RIFF")
	// File size - 8 (will be updated in Finalize)
	binary.LittleEndian.PutUint32(header[4:8], 0)
	copy(header[8:12], "WAVE")

	// fmt chunk
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16) // Chunk size
	binary.LittleEndian.PutUint16(header[20:22], 1)  // Audio format (PCM)
	binary.LittleEndian.PutUint16(header[22:24], uint16(w.channels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(w.sampleRate))
	byteRate := w.sampleRate * w.channels * w.bitsPerSample / 8
	binary.LittleEndian.PutUint32(header[28:32], uint32(byteRate))
	blockAlign := w.channels * w.bitsPerSample / 8
	binary.LittleEndian.PutUint16(header[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(header[34:36], uint16(w.bitsPerSample))

	// data chunk
	copy(header[36:40], "data")
	// Data size (will be updated in Finalize)
	binary.LittleEndian.PutUint32(header[40:44], 0)

	_, err := w.w.Write(header)
	if err != nil {
		return err
	}

	w.headerWritten = true
	return nil
}

// WriteData writes audio data
func (w *WAVWriter) WriteData(data []byte) (int, error) {
	if !w.headerWritten {
		return 0, errors.New("header not written")
	}

	n, err := w.w.Write(data)
	if err != nil {
		return n, err
	}

	w.dataSize += uint32(n)
	return n, nil
}

// Finalize updates the header with final sizes
func (w *WAVWriter) Finalize() error {
	if !w.headerWritten {
		return nil
	}

	// Update RIFF size
	if _, err := w.w.Seek(4, io.SeekStart); err != nil {
		return err
	}
	riffSize := make([]byte, 4)
	binary.LittleEndian.PutUint32(riffSize, w.dataSize+36)
	if _, err := w.w.Write(riffSize); err != nil {
		return err
	}

	// Update data size
	if _, err := w.w.Seek(40, io.SeekStart); err != nil {
		return err
	}
	dataSize := make([]byte, 4)
	binary.LittleEndian.PutUint32(dataSize, w.dataSize)
	if _, err := w.w.Write(dataSize); err != nil {
		return err
	}

	return nil
}
