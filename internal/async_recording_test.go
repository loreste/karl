package internal

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestDefaultAsyncRecordingConfig(t *testing.T) {
	config := DefaultAsyncRecordingConfig()

	if config.QueueSize != 10000 {
		t.Errorf("expected QueueSize=10000, got %d", config.QueueSize)
	}
	if config.NumWorkers != 2 {
		t.Errorf("expected NumWorkers=2, got %d", config.NumWorkers)
	}
	if config.BatchSize != 50 {
		t.Errorf("expected BatchSize=50, got %d", config.BatchSize)
	}
	if config.FlushInterval != 100*time.Millisecond {
		t.Errorf("expected FlushInterval=100ms, got %v", config.FlushInterval)
	}
}

func TestNewAsyncRecorder(t *testing.T) {
	recorder := NewAsyncRecorder(nil)
	if recorder.config.QueueSize != 10000 {
		t.Error("expected default config")
	}

	config := &AsyncRecordingConfig{QueueSize: 500}
	recorder = NewAsyncRecorder(config)
	if recorder.config.QueueSize != 500 {
		t.Errorf("expected QueueSize=500, got %d", recorder.config.QueueSize)
	}
}

func TestAsyncRecorder_OpenClose(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.raw")

	recorder := NewAsyncRecorder(nil)
	recorder.Start()
	defer recorder.Stop()

	// Open recording
	err := recorder.OpenRecording(path)
	if err != nil {
		t.Fatalf("failed to open recording: %v", err)
	}

	// Open again should be idempotent
	err = recorder.OpenRecording(path)
	if err != nil {
		t.Errorf("second open should succeed: %v", err)
	}

	// Close recording
	err = recorder.CloseRecording(path)
	if err != nil {
		t.Fatalf("failed to close recording: %v", err)
	}

	// File should exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected file to exist")
	}
}

func TestAsyncRecorder_WriteFrame(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.raw")

	config := &AsyncRecordingConfig{
		QueueSize:     100,
		NumWorkers:    2,
		BatchSize:     5,
		FlushInterval: 50 * time.Millisecond,
	}

	recorder := NewAsyncRecorder(config)
	recorder.Start()
	defer recorder.Stop()

	err := recorder.OpenRecording(path)
	if err != nil {
		t.Fatalf("failed to open recording: %v", err)
	}

	// Write frames
	for i := 0; i < 10; i++ {
		frame := &RecordingFrame{
			Data:      []byte{byte(i), byte(i + 1), byte(i + 2)},
			Timestamp: time.Now(),
		}
		err := recorder.WriteFrame(path, frame)
		if err != nil {
			t.Errorf("failed to write frame %d: %v", i, err)
		}
	}

	// Wait for flush
	time.Sleep(100 * time.Millisecond)

	recorder.CloseRecording(path)

	// Verify file has content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if len(data) != 30 { // 10 frames * 3 bytes
		t.Errorf("expected 30 bytes, got %d", len(data))
	}

	stats := recorder.Stats()
	if stats.FramesWritten != 10 {
		t.Errorf("expected 10 frames written, got %d", stats.FramesWritten)
	}
}

func TestAsyncRecorder_WriteNotStarted(t *testing.T) {
	recorder := NewAsyncRecorder(nil)
	recorder.Start()
	defer recorder.Stop()

	frame := &RecordingFrame{Data: []byte{1, 2, 3}}
	err := recorder.WriteFrame("nonexistent", frame)
	if err != ErrRecordingNotStarted {
		t.Errorf("expected ErrRecordingNotStarted, got %v", err)
	}
}

func TestAsyncRecorder_QueueFull(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.raw")

	config := &AsyncRecordingConfig{
		QueueSize:        5, // Small queue
		NumWorkers:       1,
		BatchSize:        100, // Large batch to slow processing
		FlushInterval:    time.Hour,
		MaxBufferedBytes: 0, // No byte limit
	}

	recorder := NewAsyncRecorder(config)
	recorder.Start()
	defer recorder.Stop()

	recorder.OpenRecording(path)

	// Flood queue
	dropped := 0
	for i := 0; i < 100; i++ {
		frame := &RecordingFrame{Data: make([]byte, 1000)}
		if err := recorder.WriteFrame(path, frame); err == ErrRecordingQueueFull {
			dropped++
		}
	}

	if dropped == 0 {
		t.Error("expected some frames to be dropped")
	}

	stats := recorder.Stats()
	if stats.FramesDropped == 0 {
		t.Error("expected FramesDropped > 0")
	}
}

func TestAsyncRecorder_MaxBufferedBytes(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.raw")

	config := &AsyncRecordingConfig{
		QueueSize:        10000,
		NumWorkers:       1,
		BatchSize:        1000,
		FlushInterval:    time.Hour,
		MaxBufferedBytes: 1000, // Small limit
	}

	recorder := NewAsyncRecorder(config)
	recorder.Start()
	defer recorder.Stop()

	recorder.OpenRecording(path)

	// Write until buffer limit
	dropped := 0
	for i := 0; i < 100; i++ {
		frame := &RecordingFrame{Data: make([]byte, 100)}
		if err := recorder.WriteFrame(path, frame); err == ErrRecordingQueueFull {
			dropped++
		}
	}

	if dropped == 0 {
		t.Error("expected drops due to buffer limit")
	}
}

func TestAsyncRecorder_Callback(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.raw")

	config := &AsyncRecordingConfig{
		QueueSize:     100,
		NumWorkers:    2,
		BatchSize:     1,
		FlushInterval: 10 * time.Millisecond,
	}

	recorder := NewAsyncRecorder(config)
	recorder.Start()
	defer recorder.Stop()

	recorder.OpenRecording(path)

	var callbackCalled bool
	var mu sync.Mutex

	frame := &RecordingFrame{Data: []byte{1, 2, 3}}
	recorder.WriteFrameWithCallback(path, frame, func(err error) {
		mu.Lock()
		callbackCalled = true
		mu.Unlock()
	})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	called := callbackCalled
	mu.Unlock()

	if !called {
		t.Error("expected callback to be called")
	}
}

func TestAsyncRecorder_ConcurrentWrites(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.raw")

	config := &AsyncRecordingConfig{
		QueueSize:     10000,
		NumWorkers:    4,
		BatchSize:     10,
		FlushInterval: 10 * time.Millisecond,
	}

	recorder := NewAsyncRecorder(config)
	recorder.Start()

	recorder.OpenRecording(path)

	var wg sync.WaitGroup
	numGoroutines := 10
	framesPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < framesPerGoroutine; j++ {
				frame := &RecordingFrame{Data: []byte{byte(id), byte(j)}}
				recorder.WriteFrame(path, frame)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)
	recorder.Stop()

	stats := recorder.Stats()
	total := numGoroutines * framesPerGoroutine
	if stats.FramesWritten+stats.FramesDropped != int64(total) {
		t.Errorf("frame count mismatch: written=%d, dropped=%d, expected total=%d",
			stats.FramesWritten, stats.FramesDropped, total)
	}
}

func TestBufferedRecordingWriter(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.raw")

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	writer := NewBufferedRecordingWriter(file, 100)

	// Write small chunks
	for i := 0; i < 10; i++ {
		data := make([]byte, 10)
		for j := range data {
			data[j] = byte(i*10 + j)
		}
		_, err := writer.Write(data)
		if err != nil {
			t.Errorf("write failed: %v", err)
		}
	}

	// Flush
	if err := writer.Flush(); err != nil {
		t.Errorf("flush failed: %v", err)
	}

	file.Close()

	// Verify file content
	data, _ := os.ReadFile(path)
	if len(data) != 100 {
		t.Errorf("expected 100 bytes, got %d", len(data))
	}
}

func TestBufferedRecordingWriter_LargeWrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.raw")

	file, _ := os.Create(path)
	defer file.Close()

	writer := NewBufferedRecordingWriter(file, 10) // Small buffer

	// Write larger than buffer
	data := make([]byte, 50)
	n, err := writer.Write(data)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 50 {
		t.Errorf("expected 50 bytes written, got %d", n)
	}
}

func TestRecordingPipeline(t *testing.T) {
	tmpDir := t.TempDir()

	config := &RecordingPipelineConfig{
		OutputDir:     tmpDir,
		FilePrefix:    "test_",
		AsyncConfig:   DefaultAsyncRecordingConfig(),
		MaxConcurrent: 100,
		AutoClose:     false,
	}

	pipeline := NewRecordingPipeline(config)
	pipeline.Start()
	defer pipeline.Stop()

	// Start recording
	path, err := pipeline.StartRecording("call-123")
	if err != nil {
		t.Fatalf("failed to start recording: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}

	// Write data
	err = pipeline.WriteRecording("call-123", []byte{1, 2, 3, 4, 5}, time.Now())
	if err != nil {
		t.Errorf("failed to write: %v", err)
	}

	// Get stats
	stats := pipeline.GetSessionStats("call-123")
	if stats == nil {
		t.Fatal("expected non-nil stats")
	}
	if stats.FrameCount != 1 {
		t.Errorf("expected FrameCount=1, got %d", stats.FrameCount)
	}

	// Stop recording
	err = pipeline.StopRecording("call-123")
	if err != nil {
		t.Errorf("failed to stop: %v", err)
	}

	// File should exist
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected file to exist")
	}
}

func TestRecordingPipeline_MaxConcurrent(t *testing.T) {
	tmpDir := t.TempDir()

	config := &RecordingPipelineConfig{
		OutputDir:     tmpDir,
		AsyncConfig:   DefaultAsyncRecordingConfig(),
		MaxConcurrent: 2,
		AutoClose:     false,
	}

	pipeline := NewRecordingPipeline(config)
	pipeline.Start()
	defer pipeline.Stop()

	// Start max recordings
	pipeline.StartRecording("call-1")
	pipeline.StartRecording("call-2")

	// Third should fail
	_, err := pipeline.StartRecording("call-3")
	if err == nil {
		t.Error("expected error for exceeding max concurrent")
	}
}

func TestRecordingPipeline_AutoClose(t *testing.T) {
	tmpDir := t.TempDir()

	config := &RecordingPipelineConfig{
		OutputDir:        tmpDir,
		AsyncConfig:      DefaultAsyncRecordingConfig(),
		AutoClose:        true,
		AutoCloseTimeout: 100 * time.Millisecond,
	}

	pipeline := NewRecordingPipeline(config)
	pipeline.Start()
	defer pipeline.Stop()

	pipeline.StartRecording("call-1")

	// Write initially
	pipeline.WriteRecording("call-1", []byte{1}, time.Now())

	// Wait for auto-close
	time.Sleep(250 * time.Millisecond)

	stats := pipeline.GetStats()
	if stats.ActiveSessions != 0 {
		t.Errorf("expected 0 active sessions after auto-close, got %d", stats.ActiveSessions)
	}
}

func TestAsyncRecordingSession(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "session.raw")

	session := NewAsyncRecordingSession(nil, path)

	err := session.Start()
	if err != nil {
		t.Fatalf("failed to start session: %v", err)
	}

	// Write
	err = session.Write([]byte{1, 2, 3, 4, 5})
	if err != nil {
		t.Errorf("write failed: %v", err)
	}

	// Wait for write
	time.Sleep(50 * time.Millisecond)

	stats := session.Stats()
	if stats.FramesWritten == 0 {
		t.Error("expected frames written")
	}

	err = session.Stop()
	if err != nil {
		t.Errorf("stop failed: %v", err)
	}

	// Write after stop should fail
	err = session.Write([]byte{1})
	if err != ErrRecordingNotStarted {
		t.Errorf("expected ErrRecordingNotStarted, got %v", err)
	}
}

func TestRecordingFrame_Fields(t *testing.T) {
	frame := &RecordingFrame{
		Data:       []byte{1, 2, 3},
		Timestamp:  time.Now(),
		StreamID:   "stream-1",
		SequenceNo: 100,
		Duration:   20 * time.Millisecond,
	}

	if len(frame.Data) != 3 {
		t.Error("data length mismatch")
	}
	if frame.StreamID != "stream-1" {
		t.Error("stream ID mismatch")
	}
	if frame.SequenceNo != 100 {
		t.Error("sequence number mismatch")
	}
	if frame.Duration != 20*time.Millisecond {
		t.Error("duration mismatch")
	}
}

func TestAsyncRecorderStats(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.raw")

	config := &AsyncRecordingConfig{
		QueueSize:     100,
		NumWorkers:    2,
		BatchSize:     5,
		FlushInterval: 10 * time.Millisecond,
	}

	recorder := NewAsyncRecorder(config)
	recorder.Start()
	defer recorder.Stop()

	recorder.OpenRecording(path)

	// Write frames
	for i := 0; i < 20; i++ {
		frame := &RecordingFrame{Data: make([]byte, 100)}
		recorder.WriteFrame(path, frame)
	}

	time.Sleep(100 * time.Millisecond)

	stats := recorder.Stats()
	if stats.FramesQueued == 0 && stats.FramesWritten == 0 {
		t.Error("expected some frame activity")
	}
	if stats.BytesWritten == 0 {
		t.Error("expected bytes written")
	}
	if stats.Flushes == 0 {
		t.Error("expected some flushes")
	}
}
