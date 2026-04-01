package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RecordingContinuityConfig configures recording continuity
type RecordingContinuityConfig struct {
	// RedisPrefix for recording state keys
	RedisPrefix string
	// CheckpointInterval is how often to save checkpoints
	CheckpointInterval time.Duration
	// BufferDuration is how much audio to buffer for seamless handoff
	BufferDuration time.Duration
	// MaxBufferSize is the maximum buffer size in bytes
	MaxBufferSize int64
	// SharedStoragePath is the path to shared storage
	SharedStoragePath string
	// EnableSharedStorage enables writing to shared storage
	EnableSharedStorage bool
	// LocalBufferPath is the path for local buffers
	LocalBufferPath string
	// SyncTimeout is the timeout for sync operations
	SyncTimeout time.Duration
}

// DefaultRecordingContinuityConfig returns default configuration
func DefaultRecordingContinuityConfig() *RecordingContinuityConfig {
	return &RecordingContinuityConfig{
		RedisPrefix:         "recording:state:",
		CheckpointInterval:  5 * time.Second,
		BufferDuration:      10 * time.Second,
		MaxBufferSize:       10 * 1024 * 1024, // 10MB
		SharedStoragePath:   "/var/lib/karl/recordings/shared",
		EnableSharedStorage: false,
		LocalBufferPath:     "/var/lib/karl/recordings/buffer",
		SyncTimeout:         30 * time.Second,
	}
}

// RecordingContinuityManager manages recording continuity during failover
type RecordingContinuityManager struct {
	config   *RecordingContinuityConfig
	cluster  *RedisSessionStore
	nodeID   string

	mu              sync.RWMutex
	activeRecordings map[string]*ContinuousRecording
	checkpoints     map[string]*RecordingCheckpoint

	stopChan chan struct{}
	doneChan chan struct{}
}

// ContinuousRecording represents a recording with continuity support
type ContinuousRecording struct {
	SessionID     string
	CallID        string
	StartTime     time.Time
	NodeID        string
	State         RecordingState
	FilePath      string
	Format        string
	Channels      int
	SampleRate    int
	BytesWritten  int64
	Duration      time.Duration
	LastChunk     time.Time
	Buffer        *RecordingBuffer
	Checkpoints   []*RecordingCheckpoint
	mu            sync.Mutex
}

// RecordingState represents the recording state
type RecordingState int

const (
	RecordingStateIdle RecordingState = iota
	RecordingStateActive
	RecordingStatePaused
	RecordingStateHandoff
	RecordingStateFinalizing
	RecordingStateComplete
	RecordingStateFailed
)

func (rs RecordingState) String() string {
	switch rs {
	case RecordingStateIdle:
		return "idle"
	case RecordingStateActive:
		return "active"
	case RecordingStatePaused:
		return "paused"
	case RecordingStateHandoff:
		return "handoff"
	case RecordingStateFinalizing:
		return "finalizing"
	case RecordingStateComplete:
		return "complete"
	case RecordingStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// RecordingCheckpoint represents a checkpoint in the recording
type RecordingCheckpoint struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"session_id"`
	NodeID       string    `json:"node_id"`
	Timestamp    time.Time `json:"timestamp"`
	ByteOffset   int64     `json:"byte_offset"`
	Duration     time.Duration `json:"duration"`
	BufferFile   string    `json:"buffer_file,omitempty"`
	Sequence     int64     `json:"sequence"`
	AudioFormat  string    `json:"audio_format"`
	SampleRate   int       `json:"sample_rate"`
	Channels     int       `json:"channels"`
}

// RecordingBuffer handles buffered audio for seamless handoff
type RecordingBuffer struct {
	mu         sync.Mutex
	data       []byte
	capacity   int64
	writePos   int
	startTime  time.Time
	sampleRate int
	channels   int
	file       *os.File
}

// NewRecordingContinuityManager creates a new recording continuity manager
func NewRecordingContinuityManager(nodeID string, cluster *RedisSessionStore, config *RecordingContinuityConfig) *RecordingContinuityManager {
	if config == nil {
		config = DefaultRecordingContinuityConfig()
	}

	return &RecordingContinuityManager{
		config:           config,
		cluster:          cluster,
		nodeID:           nodeID,
		activeRecordings: make(map[string]*ContinuousRecording),
		checkpoints:      make(map[string]*RecordingCheckpoint),
		stopChan:         make(chan struct{}),
		doneChan:         make(chan struct{}),
	}
}

// Start starts the recording continuity manager
func (rcm *RecordingContinuityManager) Start() error {
	// Create buffer directory
	if err := os.MkdirAll(rcm.config.LocalBufferPath, 0755); err != nil {
		return fmt.Errorf("failed to create buffer directory: %w", err)
	}

	// Create shared storage directory if enabled
	if rcm.config.EnableSharedStorage {
		if err := os.MkdirAll(rcm.config.SharedStoragePath, 0755); err != nil {
			return fmt.Errorf("failed to create shared storage: %w", err)
		}
	}

	go rcm.checkpointLoop()
	go rcm.cleanupLoop()

	return nil
}

// Stop stops the recording continuity manager
func (rcm *RecordingContinuityManager) Stop() {
	close(rcm.stopChan)
	<-rcm.doneChan

	// Finalize all recordings
	rcm.mu.Lock()
	for _, rec := range rcm.activeRecordings {
		rcm.finalizeRecording(rec)
	}
	rcm.mu.Unlock()
}

// StartRecording starts a new recording with continuity support
func (rcm *RecordingContinuityManager) StartRecording(sessionID, callID, filePath, format string, sampleRate, channels int) (*ContinuousRecording, error) {
	rcm.mu.Lock()
	defer rcm.mu.Unlock()

	if _, exists := rcm.activeRecordings[sessionID]; exists {
		return nil, fmt.Errorf("recording already exists for session %s", sessionID)
	}

	buffer, err := rcm.createBuffer(sessionID, sampleRate, channels)
	if err != nil {
		return nil, fmt.Errorf("failed to create buffer: %w", err)
	}

	rec := &ContinuousRecording{
		SessionID:    sessionID,
		CallID:       callID,
		StartTime:    time.Now(),
		NodeID:       rcm.nodeID,
		State:        RecordingStateActive,
		FilePath:     filePath,
		Format:       format,
		SampleRate:   sampleRate,
		Channels:     channels,
		Buffer:       buffer,
		Checkpoints:  make([]*RecordingCheckpoint, 0),
	}

	rcm.activeRecordings[sessionID] = rec

	// Store state in Redis
	if rcm.cluster != nil {
		rcm.storeRecordingState(rec)
	}

	return rec, nil
}

// WriteAudio writes audio data to a recording
func (rcm *RecordingContinuityManager) WriteAudio(sessionID string, data []byte, timestamp time.Time) error {
	rcm.mu.RLock()
	rec, exists := rcm.activeRecordings[sessionID]
	rcm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no recording for session %s", sessionID)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()

	if rec.State != RecordingStateActive {
		return fmt.Errorf("recording not active, state: %s", rec.State)
	}

	// Write to buffer
	if rec.Buffer != nil {
		rec.Buffer.Write(data)
	}

	rec.BytesWritten += int64(len(data))
	rec.LastChunk = timestamp
	rec.Duration = time.Since(rec.StartTime)

	return nil
}

// PauseRecording pauses a recording
func (rcm *RecordingContinuityManager) PauseRecording(sessionID string) error {
	rcm.mu.RLock()
	rec, exists := rcm.activeRecordings[sessionID]
	rcm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no recording for session %s", sessionID)
	}

	rec.mu.Lock()
	rec.State = RecordingStatePaused
	rec.mu.Unlock()

	if rcm.cluster != nil {
		rcm.storeRecordingState(rec)
	}

	return nil
}

// ResumeRecording resumes a paused recording
func (rcm *RecordingContinuityManager) ResumeRecording(sessionID string) error {
	rcm.mu.RLock()
	rec, exists := rcm.activeRecordings[sessionID]
	rcm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no recording for session %s", sessionID)
	}

	rec.mu.Lock()
	rec.State = RecordingStateActive
	rec.mu.Unlock()

	if rcm.cluster != nil {
		rcm.storeRecordingState(rec)
	}

	return nil
}

// StopRecording stops and finalizes a recording
func (rcm *RecordingContinuityManager) StopRecording(sessionID string) error {
	rcm.mu.Lock()
	rec, exists := rcm.activeRecordings[sessionID]
	if !exists {
		rcm.mu.Unlock()
		return fmt.Errorf("no recording for session %s", sessionID)
	}
	delete(rcm.activeRecordings, sessionID)
	rcm.mu.Unlock()

	return rcm.finalizeRecording(rec)
}

// TakeoverRecording takes over a recording from a failed node
func (rcm *RecordingContinuityManager) TakeoverRecording(sessionID string) (*ContinuousRecording, error) {
	// Get recording state from Redis
	state, err := rcm.getRecordingState(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get recording state: %w", err)
	}

	// Get last checkpoint
	checkpoint, err := rcm.getLastCheckpoint(sessionID)
	if err != nil {
		// No checkpoint, start fresh
		checkpoint = nil
	}

	rcm.mu.Lock()
	defer rcm.mu.Unlock()

	// Create buffer
	buffer, err := rcm.createBuffer(sessionID, state.SampleRate, state.Channels)
	if err != nil {
		return nil, fmt.Errorf("failed to create buffer: %w", err)
	}

	rec := &ContinuousRecording{
		SessionID:    sessionID,
		CallID:       state.CallID,
		StartTime:    state.StartTime,
		NodeID:       rcm.nodeID,
		State:        RecordingStateHandoff,
		FilePath:     state.FilePath,
		Format:       state.Format,
		SampleRate:   state.SampleRate,
		Channels:     state.Channels,
		BytesWritten: state.BytesWritten,
		Duration:     state.Duration,
		Buffer:       buffer,
		Checkpoints:  make([]*RecordingCheckpoint, 0),
	}

	// Recover from checkpoint if available
	if checkpoint != nil {
		rec.BytesWritten = checkpoint.ByteOffset
		rec.Duration = checkpoint.Duration
		rec.Checkpoints = append(rec.Checkpoints, checkpoint)

		// Try to recover buffered audio
		if checkpoint.BufferFile != "" {
			if err := rcm.recoverBufferedAudio(rec, checkpoint); err != nil {
				// Log but continue - we can still record new audio
				_ = err
			}
		}
	}

	rec.State = RecordingStateActive
	rcm.activeRecordings[sessionID] = rec

	// Update state in Redis with new owner
	rcm.storeRecordingState(rec)

	return rec, nil
}

// CreateCheckpoint creates a checkpoint for a recording
func (rcm *RecordingContinuityManager) CreateCheckpoint(sessionID string) (*RecordingCheckpoint, error) {
	rcm.mu.RLock()
	rec, exists := rcm.activeRecordings[sessionID]
	rcm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no recording for session %s", sessionID)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()

	// Flush buffer to file
	bufferFile := ""
	if rec.Buffer != nil && rec.Buffer.Len() > 0 {
		bufferFile = rcm.flushBufferToFile(rec)
	}

	checkpoint := &RecordingCheckpoint{
		ID:          fmt.Sprintf("cp-%d", time.Now().UnixNano()),
		SessionID:   sessionID,
		NodeID:      rcm.nodeID,
		Timestamp:   time.Now(),
		ByteOffset:  rec.BytesWritten,
		Duration:    rec.Duration,
		BufferFile:  bufferFile,
		Sequence:    int64(len(rec.Checkpoints) + 1),
		AudioFormat: rec.Format,
		SampleRate:  rec.SampleRate,
		Channels:    rec.Channels,
	}

	rec.Checkpoints = append(rec.Checkpoints, checkpoint)

	// Store checkpoint in Redis
	if rcm.cluster != nil {
		rcm.storeCheckpoint(checkpoint)
	}

	return checkpoint, nil
}

func (rcm *RecordingContinuityManager) createBuffer(sessionID string, sampleRate, channels int) (*RecordingBuffer, error) {
	// Calculate buffer size based on duration
	bytesPerSecond := sampleRate * channels * 2 // 16-bit audio
	bufferSize := int64(float64(bytesPerSecond) * rcm.config.BufferDuration.Seconds())
	if bufferSize > rcm.config.MaxBufferSize {
		bufferSize = rcm.config.MaxBufferSize
	}

	// Create buffer file for persistence
	bufferPath := filepath.Join(rcm.config.LocalBufferPath, fmt.Sprintf("%s.buffer", sessionID))
	file, err := os.OpenFile(bufferPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}

	return &RecordingBuffer{
		data:       make([]byte, bufferSize),
		capacity:   bufferSize,
		startTime:  time.Now(),
		sampleRate: sampleRate,
		channels:   channels,
		file:       file,
	}, nil
}

func (rb *RecordingBuffer) Write(data []byte) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	// Circular buffer write
	for _, b := range data {
		rb.data[rb.writePos] = b
		rb.writePos = (rb.writePos + 1) % len(rb.data)
	}

	// Also write to file for persistence
	if rb.file != nil {
		rb.file.Write(data)
	}
}

func (rb *RecordingBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.writePos
}

func (rb *RecordingBuffer) Close() error {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.file != nil {
		rb.file.Close()
		rb.file = nil
	}
	return nil
}

func (rcm *RecordingContinuityManager) flushBufferToFile(rec *ContinuousRecording) string {
	if rec.Buffer == nil {
		return ""
	}

	// Copy buffer data
	rec.Buffer.mu.Lock()
	data := make([]byte, rec.Buffer.writePos)
	copy(data, rec.Buffer.data[:rec.Buffer.writePos])
	rec.Buffer.mu.Unlock()

	// Write to checkpoint file
	filename := fmt.Sprintf("%s-%d.checkpoint", rec.SessionID, time.Now().UnixNano())
	bufferFilePath := filepath.Join(rcm.config.LocalBufferPath, filename)

	if err := os.WriteFile(bufferFilePath, data, 0644); err != nil {
		return ""
	}

	// Also write to shared storage if enabled
	if rcm.config.EnableSharedStorage {
		sharedPath := filepath.Join(rcm.config.SharedStoragePath, filename)
		os.WriteFile(sharedPath, data, 0644)
	}

	return bufferFilePath
}

func (rcm *RecordingContinuityManager) recoverBufferedAudio(rec *ContinuousRecording, checkpoint *RecordingCheckpoint) error {
	// Try local buffer first
	data, err := os.ReadFile(checkpoint.BufferFile)
	if err != nil {
		// Try shared storage
		if rcm.config.EnableSharedStorage {
			sharedPath := filepath.Join(rcm.config.SharedStoragePath, filepath.Base(checkpoint.BufferFile))
			data, err = os.ReadFile(sharedPath)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	// Write recovered audio to recording
	if rec.Buffer != nil {
		rec.Buffer.Write(data)
	}

	return nil
}

func (rcm *RecordingContinuityManager) finalizeRecording(rec *ContinuousRecording) error {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	rec.State = RecordingStateFinalizing

	// Close buffer
	if rec.Buffer != nil {
		rec.Buffer.Close()
	}

	rec.State = RecordingStateComplete

	// Remove state from Redis
	if rcm.cluster != nil {
		rcm.removeRecordingState(rec.SessionID)
	}

	// Clean up buffer files
	rcm.cleanupBufferFiles(rec.SessionID)

	return nil
}

func (rcm *RecordingContinuityManager) cleanupBufferFiles(sessionID string) {
	pattern := filepath.Join(rcm.config.LocalBufferPath, fmt.Sprintf("%s*", sessionID))
	files, _ := filepath.Glob(pattern)
	for _, f := range files {
		os.Remove(f)
	}
}

func (rcm *RecordingContinuityManager) storeRecordingState(rec *ContinuousRecording) {
	ctx, cancel := context.WithTimeout(context.Background(), rcm.config.SyncTimeout)
	defer cancel()

	state := &RecordingStateData{
		SessionID:    rec.SessionID,
		CallID:       rec.CallID,
		StartTime:    rec.StartTime,
		NodeID:       rec.NodeID,
		State:        int(rec.State),
		FilePath:     rec.FilePath,
		Format:       rec.Format,
		SampleRate:   rec.SampleRate,
		Channels:     rec.Channels,
		BytesWritten: rec.BytesWritten,
		Duration:     rec.Duration,
		LastUpdate:   time.Now(),
	}

	data, err := json.Marshal(state)
	if err != nil {
		return
	}

	key := fmt.Sprintf("%s%s", rcm.config.RedisPrefix, rec.SessionID)
	rcm.cluster.client.Set(ctx, key, string(data), 24*time.Hour)
}

// RecordingStateData represents serialized recording state
type RecordingStateData struct {
	SessionID    string        `json:"session_id"`
	CallID       string        `json:"call_id"`
	StartTime    time.Time     `json:"start_time"`
	NodeID       string        `json:"node_id"`
	State        int           `json:"state"`
	FilePath     string        `json:"file_path"`
	Format       string        `json:"format"`
	SampleRate   int           `json:"sample_rate"`
	Channels     int           `json:"channels"`
	BytesWritten int64         `json:"bytes_written"`
	Duration     time.Duration `json:"duration"`
	LastUpdate   time.Time     `json:"last_update"`
}

func (rcm *RecordingContinuityManager) getRecordingState(sessionID string) (*RecordingStateData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), rcm.config.SyncTimeout)
	defer cancel()

	key := fmt.Sprintf("%s%s", rcm.config.RedisPrefix, sessionID)
	dataStr, err := rcm.cluster.client.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var state RecordingStateData
	if err := json.Unmarshal([]byte(dataStr), &state); err != nil {
		return nil, err
	}

	return &state, nil
}

func (rcm *RecordingContinuityManager) removeRecordingState(sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), rcm.config.SyncTimeout)
	defer cancel()

	key := fmt.Sprintf("%s%s", rcm.config.RedisPrefix, sessionID)
	rcm.cluster.client.Del(ctx, key)
}

func (rcm *RecordingContinuityManager) storeCheckpoint(checkpoint *RecordingCheckpoint) {
	ctx, cancel := context.WithTimeout(context.Background(), rcm.config.SyncTimeout)
	defer cancel()

	data, err := json.Marshal(checkpoint)
	if err != nil {
		return
	}

	key := fmt.Sprintf("%scheckpoint:%s:%s", rcm.config.RedisPrefix, checkpoint.SessionID, checkpoint.ID)
	rcm.cluster.client.Set(ctx, key, string(data), 24*time.Hour)

	// Also store as latest checkpoint
	latestKey := fmt.Sprintf("%scheckpoint:%s:latest", rcm.config.RedisPrefix, checkpoint.SessionID)
	rcm.cluster.client.Set(ctx, latestKey, string(data), 24*time.Hour)
}

func (rcm *RecordingContinuityManager) getLastCheckpoint(sessionID string) (*RecordingCheckpoint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), rcm.config.SyncTimeout)
	defer cancel()

	key := fmt.Sprintf("%scheckpoint:%s:latest", rcm.config.RedisPrefix, sessionID)
	dataStr, err := rcm.cluster.client.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var checkpoint RecordingCheckpoint
	if err := json.Unmarshal([]byte(dataStr), &checkpoint); err != nil {
		return nil, err
	}

	return &checkpoint, nil
}

func (rcm *RecordingContinuityManager) checkpointLoop() {
	ticker := time.NewTicker(rcm.config.CheckpointInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rcm.stopChan:
			return
		case <-ticker.C:
			rcm.createAllCheckpoints()
		}
	}
}

func (rcm *RecordingContinuityManager) createAllCheckpoints() {
	rcm.mu.RLock()
	sessions := make([]string, 0, len(rcm.activeRecordings))
	for sessionID := range rcm.activeRecordings {
		sessions = append(sessions, sessionID)
	}
	rcm.mu.RUnlock()

	for _, sessionID := range sessions {
		rcm.CreateCheckpoint(sessionID)
	}
}

func (rcm *RecordingContinuityManager) cleanupLoop() {
	defer close(rcm.doneChan)

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-rcm.stopChan:
			return
		case <-ticker.C:
			rcm.cleanupOldBuffers()
		}
	}
}

func (rcm *RecordingContinuityManager) cleanupOldBuffers() {
	// Remove buffer files older than 24 hours
	cutoff := time.Now().Add(-24 * time.Hour)

	filepath.Walk(rcm.config.LocalBufferPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.ModTime().Before(cutoff) {
			os.Remove(path)
		}
		return nil
	})
}

// GetActiveRecording returns an active recording
func (rcm *RecordingContinuityManager) GetActiveRecording(sessionID string) *ContinuousRecording {
	rcm.mu.RLock()
	defer rcm.mu.RUnlock()
	return rcm.activeRecordings[sessionID]
}

// GetAllActiveRecordings returns all active recordings
func (rcm *RecordingContinuityManager) GetAllActiveRecordings() []*ContinuousRecording {
	rcm.mu.RLock()
	defer rcm.mu.RUnlock()

	recordings := make([]*ContinuousRecording, 0, len(rcm.activeRecordings))
	for _, rec := range rcm.activeRecordings {
		recordings = append(recordings, rec)
	}
	return recordings
}

// GetStats returns recording continuity statistics
func (rcm *RecordingContinuityManager) GetStats() *RecordingContinuityStats {
	rcm.mu.RLock()
	defer rcm.mu.RUnlock()

	var totalBytes int64
	var totalCheckpoints int

	for _, rec := range rcm.activeRecordings {
		rec.mu.Lock()
		totalBytes += rec.BytesWritten
		totalCheckpoints += len(rec.Checkpoints)
		rec.mu.Unlock()
	}

	return &RecordingContinuityStats{
		ActiveRecordings:  len(rcm.activeRecordings),
		TotalBytesWritten: totalBytes,
		TotalCheckpoints:  totalCheckpoints,
	}
}

// RecordingContinuityStats contains continuity statistics
type RecordingContinuityStats struct {
	ActiveRecordings  int
	TotalBytesWritten int64
	TotalCheckpoints  int
}

// MergeRecordings merges multiple recording segments into one file
func MergeRecordings(segments []string, outputPath string) error {
	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	for _, segment := range segments {
		inFile, err := os.Open(segment)
		if err != nil {
			return fmt.Errorf("failed to open segment %s: %w", segment, err)
		}

		_, err = io.Copy(outFile, inFile)
		inFile.Close()
		if err != nil {
			return fmt.Errorf("failed to copy segment %s: %w", segment, err)
		}
	}

	return nil
}
