package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Async recording errors
var (
	ErrRecordingQueueFull   = errors.New("recording queue is full")
	ErrRecordingClosed      = errors.New("recording is closed")
	ErrRecordingNotStarted  = errors.New("recording not started")
)

// AsyncRecordingConfig configures async recording
type AsyncRecordingConfig struct {
	// QueueSize is the maximum number of pending write operations
	QueueSize int
	// NumWorkers is the number of write workers
	NumWorkers int
	// BatchSize is the number of frames per batch write
	BatchSize int
	// FlushInterval is how often to flush to disk
	FlushInterval time.Duration
	// MaxBufferedBytes is the maximum bytes to buffer before blocking
	MaxBufferedBytes int64
	// WriteTimeout is the timeout for write operations
	WriteTimeout time.Duration
}

// DefaultAsyncRecordingConfig returns default configuration
func DefaultAsyncRecordingConfig() *AsyncRecordingConfig {
	return &AsyncRecordingConfig{
		QueueSize:        10000,
		NumWorkers:       2,
		BatchSize:        50,
		FlushInterval:    100 * time.Millisecond,
		MaxBufferedBytes: 10 * 1024 * 1024, // 10MB
		WriteTimeout:     5 * time.Second,
	}
}

// RecordingFrame represents a frame to be recorded
type RecordingFrame struct {
	Data       []byte
	Timestamp  time.Time
	StreamID   string
	SequenceNo uint32
	Duration   time.Duration
}

// AsyncRecorder provides non-blocking recording
type AsyncRecorder struct {
	config *AsyncRecordingConfig

	mu        sync.Mutex
	writers   map[string]*asyncWriter
	queue     chan *recordingOp
	stopChan  chan struct{}
	doneChan  chan struct{}

	// Stats
	framesQueued   atomic.Int64
	framesWritten  atomic.Int64
	framesDropped  atomic.Int64
	bytesQueued    atomic.Int64
	bytesWritten   atomic.Int64
	flushes        atomic.Int64
	errors         atomic.Int64
}

type recordingOp struct {
	frame    *RecordingFrame
	writer   *asyncWriter
	callback func(error)
}

type asyncWriter struct {
	path     string
	file     *os.File
	buffer   []byte
	bufLen   int
	mu       sync.Mutex
	closed   bool
	lastFlush time.Time
}

// NewAsyncRecorder creates a new async recorder
func NewAsyncRecorder(config *AsyncRecordingConfig) *AsyncRecorder {
	if config == nil {
		config = DefaultAsyncRecordingConfig()
	}

	return &AsyncRecorder{
		config:   config,
		writers:  make(map[string]*asyncWriter),
		queue:    make(chan *recordingOp, config.QueueSize),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
}

// Start starts the async recorder
func (ar *AsyncRecorder) Start() {
	for i := 0; i < ar.config.NumWorkers; i++ {
		go ar.worker()
	}
	go ar.flusher()
}

// Stop stops the async recorder and flushes remaining data
func (ar *AsyncRecorder) Stop() {
	close(ar.stopChan)

	// Wait for workers
	for i := 0; i < ar.config.NumWorkers; i++ {
		<-ar.doneChan
	}

	// Close all writers
	ar.mu.Lock()
	for _, w := range ar.writers {
		w.flush()
		w.close()
	}
	ar.writers = nil
	ar.mu.Unlock()
}

// OpenRecording opens a new recording file
func (ar *AsyncRecorder) OpenRecording(path string) error {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	if _, exists := ar.writers[path]; exists {
		return nil // Already open
	}

	// Create directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	ar.writers[path] = &asyncWriter{
		path:      path,
		file:      file,
		buffer:    make([]byte, 0, 64*1024), // 64KB initial buffer
		lastFlush: time.Now(),
	}

	return nil
}

// CloseRecording closes a recording file
func (ar *AsyncRecorder) CloseRecording(path string) error {
	ar.mu.Lock()
	w, exists := ar.writers[path]
	if !exists {
		ar.mu.Unlock()
		return nil
	}
	delete(ar.writers, path)
	ar.mu.Unlock()

	w.flush()
	return w.close()
}

// WriteFrame queues a frame for writing
func (ar *AsyncRecorder) WriteFrame(path string, frame *RecordingFrame) error {
	return ar.WriteFrameWithCallback(path, frame, nil)
}

// WriteFrameWithCallback queues a frame with completion callback
func (ar *AsyncRecorder) WriteFrameWithCallback(path string, frame *RecordingFrame, callback func(error)) error {
	ar.mu.Lock()
	w, exists := ar.writers[path]
	ar.mu.Unlock()

	if !exists {
		return ErrRecordingNotStarted
	}

	// Check if we're over the buffer limit
	if ar.config.MaxBufferedBytes > 0 && ar.bytesQueued.Load() > ar.config.MaxBufferedBytes {
		ar.framesDropped.Add(1)
		return ErrRecordingQueueFull
	}

	op := &recordingOp{
		frame:    frame,
		writer:   w,
		callback: callback,
	}

	select {
	case ar.queue <- op:
		ar.framesQueued.Add(1)
		ar.bytesQueued.Add(int64(len(frame.Data)))
		return nil
	default:
		ar.framesDropped.Add(1)
		return ErrRecordingQueueFull
	}
}

func (ar *AsyncRecorder) worker() {
	defer func() { ar.doneChan <- struct{}{} }()

	batch := make([]*recordingOp, 0, ar.config.BatchSize)

	for {
		// Get first item
		select {
		case <-ar.stopChan:
			// Process remaining items
			for {
				select {
				case op := <-ar.queue:
					ar.processOp(op)
				default:
					return
				}
			}

		case op := <-ar.queue:
			batch = append(batch, op)

			// Collect batch
			collectLoop:
			for len(batch) < ar.config.BatchSize {
				select {
				case op := <-ar.queue:
					batch = append(batch, op)
				default:
					break collectLoop
				}
			}

			// Process batch
			for _, op := range batch {
				ar.processOp(op)
			}
			batch = batch[:0]
		}
	}
}

func (ar *AsyncRecorder) processOp(op *recordingOp) {
	err := op.writer.write(op.frame.Data)

	ar.bytesQueued.Add(-int64(len(op.frame.Data)))

	if err != nil {
		ar.errors.Add(1)
	} else {
		ar.framesWritten.Add(1)
		ar.bytesWritten.Add(int64(len(op.frame.Data)))
	}

	if op.callback != nil {
		op.callback(err)
	}
}

func (ar *AsyncRecorder) flusher() {
	ticker := time.NewTicker(ar.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ar.stopChan:
			return
		case <-ticker.C:
			ar.flushAll()
		}
	}
}

func (ar *AsyncRecorder) flushAll() {
	ar.mu.Lock()
	writers := make([]*asyncWriter, 0, len(ar.writers))
	for _, w := range ar.writers {
		writers = append(writers, w)
	}
	ar.mu.Unlock()

	for _, w := range writers {
		w.flush()
		ar.flushes.Add(1)
	}
}

func (w *asyncWriter) write(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrRecordingClosed
	}

	w.buffer = append(w.buffer, data...)
	w.bufLen += len(data)

	return nil
}

func (w *asyncWriter) flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed || w.bufLen == 0 {
		return nil
	}

	_, err := w.file.Write(w.buffer[:w.bufLen])
	if err != nil {
		return err
	}

	w.buffer = w.buffer[:0]
	w.bufLen = 0
	w.lastFlush = time.Now()

	return nil
}

func (w *asyncWriter) close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	w.closed = true
	return w.file.Close()
}

// Stats returns recorder statistics
func (ar *AsyncRecorder) Stats() *AsyncRecorderStats {
	return &AsyncRecorderStats{
		FramesQueued:   ar.framesQueued.Load(),
		FramesWritten:  ar.framesWritten.Load(),
		FramesDropped:  ar.framesDropped.Load(),
		BytesQueued:    ar.bytesQueued.Load(),
		BytesWritten:   ar.bytesWritten.Load(),
		Flushes:        ar.flushes.Load(),
		Errors:         ar.errors.Load(),
	}
}

// AsyncRecorderStats holds recorder statistics
type AsyncRecorderStats struct {
	FramesQueued   int64
	FramesWritten  int64
	FramesDropped  int64
	BytesQueued    int64
	BytesWritten   int64
	Flushes        int64
	Errors         int64
}

// BufferedRecordingWriter wraps an io.Writer with buffering
type BufferedRecordingWriter struct {
	w         io.Writer
	buffer    []byte
	bufLen    int
	mu        sync.Mutex
	flushSize int
}

// NewBufferedRecordingWriter creates a buffered recording writer
func NewBufferedRecordingWriter(w io.Writer, bufferSize int) *BufferedRecordingWriter {
	return &BufferedRecordingWriter{
		w:         w,
		buffer:    make([]byte, bufferSize),
		flushSize: bufferSize,
	}
}

// Write writes data to the buffer
func (b *BufferedRecordingWriter) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// If data is larger than buffer, write directly
	if len(p) >= b.flushSize {
		if b.bufLen > 0 {
			if _, err := b.w.Write(b.buffer[:b.bufLen]); err != nil {
				return 0, err
			}
			b.bufLen = 0
		}
		return b.w.Write(p)
	}

	// Check if buffer needs flushing
	if b.bufLen+len(p) > b.flushSize {
		if _, err := b.w.Write(b.buffer[:b.bufLen]); err != nil {
			return 0, err
		}
		b.bufLen = 0
	}

	// Copy to buffer
	copy(b.buffer[b.bufLen:], p)
	b.bufLen += len(p)

	return len(p), nil
}

// Flush flushes the buffer
func (b *BufferedRecordingWriter) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.bufLen == 0 {
		return nil
	}

	_, err := b.w.Write(b.buffer[:b.bufLen])
	b.bufLen = 0
	return err
}

// RecordingPipeline provides a complete async recording pipeline
type RecordingPipeline struct {
	config   *RecordingPipelineConfig
	recorder *AsyncRecorder

	mu        sync.Mutex
	sessions  map[string]*recordingSession

	stopChan chan struct{}
	doneChan chan struct{}
}

// RecordingPipelineConfig configures the recording pipeline
type RecordingPipelineConfig struct {
	OutputDir         string
	FilePrefix        string
	AsyncConfig       *AsyncRecordingConfig
	MaxConcurrent     int
	AutoClose         bool
	AutoCloseTimeout  time.Duration
}

// DefaultRecordingPipelineConfig returns default pipeline config
func DefaultRecordingPipelineConfig() *RecordingPipelineConfig {
	return &RecordingPipelineConfig{
		OutputDir:        "/var/lib/karl/recordings",
		FilePrefix:       "rec_",
		AsyncConfig:      DefaultAsyncRecordingConfig(),
		MaxConcurrent:    1000,
		AutoClose:        true,
		AutoCloseTimeout: 5 * time.Minute,
	}
}

type recordingSession struct {
	callID     string
	path       string
	startTime  time.Time
	lastWrite  time.Time
	frameCount int64
	byteCount  int64
}

// NewRecordingPipeline creates a new recording pipeline
func NewRecordingPipeline(config *RecordingPipelineConfig) *RecordingPipeline {
	if config == nil {
		config = DefaultRecordingPipelineConfig()
	}

	return &RecordingPipeline{
		config:   config,
		recorder: NewAsyncRecorder(config.AsyncConfig),
		sessions: make(map[string]*recordingSession),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
}

// Start starts the recording pipeline
func (rp *RecordingPipeline) Start() error {
	// Ensure output directory exists
	if err := os.MkdirAll(rp.config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	rp.recorder.Start()

	if rp.config.AutoClose {
		go rp.autoCloseLoop()
	}

	return nil
}

// Stop stops the recording pipeline
func (rp *RecordingPipeline) Stop() {
	close(rp.stopChan)
	if rp.config.AutoClose {
		<-rp.doneChan
	}

	// Close all sessions
	rp.mu.Lock()
	for callID := range rp.sessions {
		rp.closeSessionLocked(callID)
	}
	rp.mu.Unlock()

	rp.recorder.Stop()
}

// StartRecording starts recording for a call
func (rp *RecordingPipeline) StartRecording(callID string) (string, error) {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	if _, exists := rp.sessions[callID]; exists {
		return rp.sessions[callID].path, nil
	}

	if len(rp.sessions) >= rp.config.MaxConcurrent {
		return "", fmt.Errorf("max concurrent recordings reached")
	}

	// Generate filename
	filename := fmt.Sprintf("%s%s_%d.raw", rp.config.FilePrefix, callID, time.Now().Unix())
	path := filepath.Join(rp.config.OutputDir, filename)

	if err := rp.recorder.OpenRecording(path); err != nil {
		return "", err
	}

	rp.sessions[callID] = &recordingSession{
		callID:    callID,
		path:      path,
		startTime: time.Now(),
		lastWrite: time.Now(),
	}

	return path, nil
}

// WriteRecording writes a frame for a call
func (rp *RecordingPipeline) WriteRecording(callID string, data []byte, timestamp time.Time) error {
	rp.mu.Lock()
	session, exists := rp.sessions[callID]
	if !exists {
		rp.mu.Unlock()
		return ErrRecordingNotStarted
	}
	session.lastWrite = time.Now()
	session.frameCount++
	session.byteCount += int64(len(data))
	path := session.path
	rp.mu.Unlock()

	frame := &RecordingFrame{
		Data:      data,
		Timestamp: timestamp,
		StreamID:  callID,
	}

	return rp.recorder.WriteFrame(path, frame)
}

// StopRecording stops recording for a call
func (rp *RecordingPipeline) StopRecording(callID string) error {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	return rp.closeSessionLocked(callID)
}

func (rp *RecordingPipeline) closeSessionLocked(callID string) error {
	session, exists := rp.sessions[callID]
	if !exists {
		return nil
	}

	delete(rp.sessions, callID)
	return rp.recorder.CloseRecording(session.path)
}

func (rp *RecordingPipeline) autoCloseLoop() {
	defer func() { rp.doneChan <- struct{}{} }()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rp.stopChan:
			return
		case <-ticker.C:
			rp.checkExpiredSessions()
		}
	}
}

func (rp *RecordingPipeline) checkExpiredSessions() {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	now := time.Now()
	expired := make([]string, 0)

	for callID, session := range rp.sessions {
		if now.Sub(session.lastWrite) > rp.config.AutoCloseTimeout {
			expired = append(expired, callID)
		}
	}

	for _, callID := range expired {
		rp.closeSessionLocked(callID)
	}
}

// GetSessionStats returns statistics for a recording session
func (rp *RecordingPipeline) GetSessionStats(callID string) *RecordingSessionStats {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	session, exists := rp.sessions[callID]
	if !exists {
		return nil
	}

	return &RecordingSessionStats{
		CallID:     callID,
		Path:       session.path,
		StartTime:  session.startTime,
		LastWrite:  session.lastWrite,
		FrameCount: session.frameCount,
		ByteCount:  session.byteCount,
		Duration:   time.Since(session.startTime),
	}
}

// RecordingSessionStats holds session statistics
type RecordingSessionStats struct {
	CallID     string
	Path       string
	StartTime  time.Time
	LastWrite  time.Time
	FrameCount int64
	ByteCount  int64
	Duration   time.Duration
}

// GetStats returns overall pipeline statistics
func (rp *RecordingPipeline) GetStats() *RecordingPipelineStats {
	rp.mu.Lock()
	activeCount := len(rp.sessions)
	rp.mu.Unlock()

	recStats := rp.recorder.Stats()

	return &RecordingPipelineStats{
		ActiveSessions: activeCount,
		RecorderStats:  recStats,
	}
}

// RecordingPipelineStats holds pipeline statistics
type RecordingPipelineStats struct {
	ActiveSessions int
	RecorderStats  *AsyncRecorderStats
}

// AsyncRecordingSession provides async recording for a single session
type AsyncRecordingSession struct {
	ctx      context.Context
	cancel   context.CancelFunc
	recorder *AsyncRecorder
	path     string

	started   bool
	stopped   bool
	mu        sync.Mutex
}

// NewAsyncRecordingSession creates a new async recording session
func NewAsyncRecordingSession(config *AsyncRecordingConfig, path string) *AsyncRecordingSession {
	ctx, cancel := context.WithCancel(context.Background())

	return &AsyncRecordingSession{
		ctx:      ctx,
		cancel:   cancel,
		recorder: NewAsyncRecorder(config),
		path:     path,
	}
}

// Start starts the recording session
func (s *AsyncRecordingSession) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return nil
	}

	s.recorder.Start()

	if err := s.recorder.OpenRecording(s.path); err != nil {
		s.recorder.Stop()
		return err
	}

	s.started = true
	return nil
}

// Write writes data to the recording
func (s *AsyncRecordingSession) Write(data []byte) error {
	s.mu.Lock()
	if !s.started || s.stopped {
		s.mu.Unlock()
		return ErrRecordingNotStarted
	}
	s.mu.Unlock()

	frame := &RecordingFrame{
		Data:      data,
		Timestamp: time.Now(),
	}

	return s.recorder.WriteFrame(s.path, frame)
}

// Stop stops the recording session
func (s *AsyncRecordingSession) Stop() error {
	s.mu.Lock()
	if !s.started || s.stopped {
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	s.mu.Unlock()

	s.cancel()
	s.recorder.CloseRecording(s.path)
	s.recorder.Stop()

	return nil
}

// Stats returns session statistics
func (s *AsyncRecordingSession) Stats() *AsyncRecorderStats {
	return s.recorder.Stats()
}
