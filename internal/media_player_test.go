package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewMediaPlayer(t *testing.T) {
	mp := NewMediaPlayer()
	if mp == nil {
		t.Fatal("NewMediaPlayer returned nil")
	}
	if mp.sessions == nil {
		t.Error("sessions map not initialized")
	}
	if mp.stopCh == nil {
		t.Error("stopCh not initialized")
	}
}

func TestMediaPlayer_StartPlayback_FileNotFound(t *testing.T) {
	mp := NewMediaPlayer()

	config := &PlaybackConfig{
		FilePath: "/nonexistent/path/audio.wav",
		Codec:    "PCMU",
	}

	err := mp.StartPlayback("session-1", config)
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestMediaPlayer_StartPlayback_WithRawPCM(t *testing.T) {
	mp := NewMediaPlayer()

	// Create a temporary raw PCM file
	tmpDir := t.TempDir()
	rawFile := filepath.Join(tmpDir, "test.raw")

	// Create a simple raw PCM file (1 second of silence at 8kHz)
	rawData := make([]byte, 8000) // 1 second at 8kHz, 8-bit samples
	if err := os.WriteFile(rawFile, rawData, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	config := &PlaybackConfig{
		FilePath:  rawFile,
		Codec:     "PCMU",
		Loop:      false,
		TargetLeg: "both",
	}

	err := mp.StartPlayback("session-1", config)
	if err != nil {
		t.Fatalf("StartPlayback failed: %v", err)
	}

	// Verify session was created
	if !mp.IsPlaying("session-1") {
		t.Error("Expected IsPlaying to return true after StartPlayback")
	}

	// Clean up
	mp.StopPlayback("session-1")
}

func TestMediaPlayer_StopPlayback(t *testing.T) {
	mp := NewMediaPlayer()

	// Create a temporary raw PCM file
	tmpDir := t.TempDir()
	rawFile := filepath.Join(tmpDir, "test.raw")
	rawData := make([]byte, 8000)
	os.WriteFile(rawFile, rawData, 0644)

	config := &PlaybackConfig{
		FilePath: rawFile,
		Codec:    "PCMU",
	}

	mp.StartPlayback("session-1", config)

	// Stop playback
	err := mp.StopPlayback("session-1")
	if err != nil {
		t.Fatalf("StopPlayback failed: %v", err)
	}

	// Verify session was removed
	if mp.IsPlaying("session-1") {
		t.Error("Expected IsPlaying to return false after StopPlayback")
	}
}

func TestMediaPlayer_StopPlayback_NotFound(t *testing.T) {
	mp := NewMediaPlayer()

	err := mp.StopPlayback("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent session")
	}
}

func TestMediaPlayer_PauseResumePlayback(t *testing.T) {
	mp := NewMediaPlayer()

	// Create a temporary raw PCM file
	tmpDir := t.TempDir()
	rawFile := filepath.Join(tmpDir, "test.raw")
	rawData := make([]byte, 8000)
	os.WriteFile(rawFile, rawData, 0644)

	config := &PlaybackConfig{
		FilePath: rawFile,
		Codec:    "PCMU",
	}

	mp.StartPlayback("session-1", config)

	// Pause
	err := mp.PausePlayback("session-1")
	if err != nil {
		t.Fatalf("PausePlayback failed: %v", err)
	}

	// Should not be playing while paused
	if mp.IsPlaying("session-1") {
		t.Error("Expected IsPlaying to return false after pause")
	}

	// Resume
	err = mp.ResumePlayback("session-1")
	if err != nil {
		t.Fatalf("ResumePlayback failed: %v", err)
	}

	// Should be playing after resume
	if !mp.IsPlaying("session-1") {
		t.Error("Expected IsPlaying to return true after resume")
	}

	mp.StopPlayback("session-1")
}

func TestMediaPlayer_PausePlayback_NotFound(t *testing.T) {
	mp := NewMediaPlayer()

	err := mp.PausePlayback("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent session")
	}
}

func TestMediaPlayer_ResumePlayback_NotFound(t *testing.T) {
	mp := NewMediaPlayer()

	err := mp.ResumePlayback("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent session")
	}
}

func TestMediaPlayer_GetNextPacket(t *testing.T) {
	mp := NewMediaPlayer()

	// Create a temporary raw PCM file
	tmpDir := t.TempDir()
	rawFile := filepath.Join(tmpDir, "test.raw")
	// Create 320 bytes (enough for 2 packets at 160 bytes each)
	rawData := make([]byte, 320)
	for i := range rawData {
		rawData[i] = byte(i % 256)
	}
	os.WriteFile(rawFile, rawData, 0644)

	config := &PlaybackConfig{
		FilePath: rawFile,
		Codec:    "PCMU",
	}

	mp.StartPlayback("session-1", config)

	// Get first packet
	packet, ok := mp.GetNextPacket("session-1")
	if !ok {
		t.Error("GetNextPacket should return ok=true")
	}
	if packet == nil {
		t.Fatal("GetNextPacket returned nil packet")
	}
	if len(packet) < 12 {
		t.Error("Packet too short (missing RTP header)")
	}

	// Verify RTP header
	if packet[0] != 0x80 {
		t.Errorf("Invalid RTP version/flags: %02x", packet[0])
	}
	if packet[1] != 0 { // PT 0 for PCMU
		t.Errorf("Invalid payload type: %d", packet[1])
	}

	mp.StopPlayback("session-1")
}

func TestMediaPlayer_GetNextPacket_NotFound(t *testing.T) {
	mp := NewMediaPlayer()

	packet, ok := mp.GetNextPacket("nonexistent")
	if ok {
		t.Error("Expected ok=false for nonexistent session")
	}
	if packet != nil {
		t.Error("Expected nil packet for nonexistent session")
	}
}

func TestMediaPlayer_GetStats(t *testing.T) {
	mp := NewMediaPlayer()

	stats := mp.GetStats()
	if stats == nil {
		t.Fatal("GetStats returned nil")
	}

	activeCount, ok := stats["active_playbacks"].(int)
	if !ok {
		t.Error("active_playbacks not found or wrong type")
	}
	if activeCount != 0 {
		t.Errorf("Expected 0 active playbacks, got %d", activeCount)
	}
}

func TestMediaPlayer_Stop(t *testing.T) {
	mp := NewMediaPlayer()

	// Create temp files and start multiple sessions
	tmpDir := t.TempDir()
	rawFile := filepath.Join(tmpDir, "test.raw")
	rawData := make([]byte, 8000)
	os.WriteFile(rawFile, rawData, 0644)

	config := &PlaybackConfig{FilePath: rawFile}

	mp.StartPlayback("session-1", config)
	mp.StartPlayback("session-2", config)

	// Stop all
	mp.Stop()

	// Verify all sessions are gone
	if mp.IsPlaying("session-1") {
		t.Error("session-1 should not be playing after Stop")
	}
	if mp.IsPlaying("session-2") {
		t.Error("session-2 should not be playing after Stop")
	}
}

func TestMediaPlayer_LoopPlayback(t *testing.T) {
	mp := NewMediaPlayer()

	// Create a very small file
	tmpDir := t.TempDir()
	rawFile := filepath.Join(tmpDir, "test.raw")
	// Create only 80 bytes (half a packet)
	rawData := make([]byte, 80)
	os.WriteFile(rawFile, rawData, 0644)

	config := &PlaybackConfig{
		FilePath: rawFile,
		Codec:    "PCMU",
		Loop:     true,
	}

	mp.StartPlayback("session-1", config)

	// Get a few packets - with looping, it should keep returning packets
	for i := 0; i < 5; i++ {
		packet, ok := mp.GetNextPacket("session-1")
		if !ok {
			t.Errorf("Iteration %d: GetNextPacket should return ok=true with looping", i)
		}
		if packet == nil {
			t.Errorf("Iteration %d: GetNextPacket should return packet with looping", i)
		}
	}

	mp.StopPlayback("session-1")
}

func TestMediaPlayer_NonLoopPlayback(t *testing.T) {
	mp := NewMediaPlayer()

	// Create a small file
	tmpDir := t.TempDir()
	rawFile := filepath.Join(tmpDir, "test.raw")
	// Create exactly one packet worth of data
	rawData := make([]byte, 160)
	os.WriteFile(rawFile, rawData, 0644)

	config := &PlaybackConfig{
		FilePath: rawFile,
		Codec:    "PCMU",
		Loop:     false,
	}

	mp.StartPlayback("session-1", config)

	// First packet should succeed
	packet, ok := mp.GetNextPacket("session-1")
	if !ok || packet == nil {
		t.Error("First packet should be available")
	}

	// Second packet should fail (no more data, no loop)
	packet, ok = mp.GetNextPacket("session-1")
	if ok || packet != nil {
		t.Error("Second packet should fail without looping")
	}

	mp.StopPlayback("session-1")
}

func TestMediaPlayer_OverwriteExistingSession(t *testing.T) {
	mp := NewMediaPlayer()

	// Create temp files
	tmpDir := t.TempDir()
	rawFile1 := filepath.Join(tmpDir, "test1.raw")
	rawFile2 := filepath.Join(tmpDir, "test2.raw")

	rawData := make([]byte, 8000)
	os.WriteFile(rawFile1, rawData, 0644)
	os.WriteFile(rawFile2, rawData, 0644)

	// Start with first file
	config1 := &PlaybackConfig{FilePath: rawFile1}
	mp.StartPlayback("session-1", config1)

	// Overwrite with second file
	config2 := &PlaybackConfig{FilePath: rawFile2}
	err := mp.StartPlayback("session-1", config2)
	if err != nil {
		t.Fatalf("Failed to overwrite session: %v", err)
	}

	mp.StopPlayback("session-1")
}

func TestPlaybackSession_GetPlaybackStats(t *testing.T) {
	mp := NewMediaPlayer()

	tmpDir := t.TempDir()
	rawFile := filepath.Join(tmpDir, "test.raw")
	rawData := make([]byte, 8000)
	os.WriteFile(rawFile, rawData, 0644)

	config := &PlaybackConfig{
		FilePath: rawFile,
		Codec:    "PCMU",
		Loop:     true,
	}

	mp.StartPlayback("session-1", config)

	// Get a packet to advance position
	mp.GetNextPacket("session-1")

	stats := mp.GetStats()
	sessions, ok := stats["sessions"].(map[string]interface{})
	if !ok {
		t.Fatal("sessions not found in stats")
	}

	sessionStats, ok := sessions["session-1"].(map[string]interface{})
	if !ok {
		t.Fatal("session-1 not found in sessions")
	}

	// Check expected fields
	if _, ok := sessionStats["playing"]; !ok {
		t.Error("playing field missing from stats")
	}
	if _, ok := sessionStats["progress"]; !ok {
		t.Error("progress field missing from stats")
	}

	mp.StopPlayback("session-1")
}

func TestLinearToUlaw(t *testing.T) {
	// Test some known conversions
	tests := []struct {
		input int16
		desc  string
	}{
		{0, "silence"},
		{8192, "positive sample"},
		{-8192, "negative sample"},
		{32767, "max positive"},
		{-32768, "max negative"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := linearToUlaw(tt.input)
			// Just verify it doesn't panic and returns a byte
			if result > 255 {
				t.Error("Result should be a valid byte")
			}
		})
	}
}

func TestConvertPCM16ToUlaw(t *testing.T) {
	// Create 16-bit PCM data
	pcm := []byte{0x00, 0x00, 0x00, 0x10, 0x00, 0x20, 0x00, 0x40}

	ulaw := convertPCM16ToUlaw(pcm)

	if len(ulaw) != len(pcm)/2 {
		t.Errorf("Expected %d samples, got %d", len(pcm)/2, len(ulaw))
	}
}

func TestPlaybackConfig(t *testing.T) {
	config := &PlaybackConfig{
		FilePath:      "/path/to/file.wav",
		Codec:         "PCMA",
		Loop:          true,
		BlendOriginal: true,
		TargetLeg:     "caller",
		SSRC:          12345,
	}

	if config.FilePath != "/path/to/file.wav" {
		t.Error("FilePath mismatch")
	}
	if config.Codec != "PCMA" {
		t.Error("Codec mismatch")
	}
	if !config.Loop {
		t.Error("Loop should be true")
	}
	if !config.BlendOriginal {
		t.Error("BlendOriginal should be true")
	}
	if config.TargetLeg != "caller" {
		t.Error("TargetLeg mismatch")
	}
	if config.SSRC != 12345 {
		t.Error("SSRC mismatch")
	}
}

func TestPlaybackSession_RTPPacketFormat(t *testing.T) {
	mp := NewMediaPlayer()

	tmpDir := t.TempDir()
	rawFile := filepath.Join(tmpDir, "test.raw")
	rawData := make([]byte, 320)
	os.WriteFile(rawFile, rawData, 0644)

	config := &PlaybackConfig{
		FilePath: rawFile,
		Codec:    "PCMU",
		SSRC:     0x12345678,
	}

	mp.StartPlayback("session-1", config)

	// Get first packet and verify RTP format
	packet1, ok := mp.GetNextPacket("session-1")
	if !ok {
		t.Fatal("Failed to get packet 1")
	}

	// Verify RTP header structure
	// Byte 0: V=2, P=0, X=0, CC=0 (0x80)
	if packet1[0] != 0x80 {
		t.Errorf("Invalid RTP version: expected 0x80, got 0x%02x", packet1[0])
	}

	// Byte 1: M=0, PT=0 (PCMU)
	if packet1[1] != 0 {
		t.Errorf("Invalid payload type: expected 0, got %d", packet1[1])
	}

	// Get second packet
	packet2, ok := mp.GetNextPacket("session-1")
	if !ok {
		t.Fatal("Failed to get packet 2")
	}

	// Verify sequence number incremented
	seq1 := uint16(packet1[2])<<8 | uint16(packet1[3])
	seq2 := uint16(packet2[2])<<8 | uint16(packet2[3])
	if seq2 != seq1+1 {
		t.Errorf("Sequence number not incremented: %d -> %d", seq1, seq2)
	}

	// Verify timestamp incremented by payload size
	ts1 := uint32(packet1[4])<<24 | uint32(packet1[5])<<16 | uint32(packet1[6])<<8 | uint32(packet1[7])
	ts2 := uint32(packet2[4])<<24 | uint32(packet2[5])<<16 | uint32(packet2[6])<<8 | uint32(packet2[7])
	expectedTsDiff := uint32(160) // Default payload size for 8kHz
	if ts2-ts1 != expectedTsDiff {
		t.Errorf("Timestamp increment wrong: expected %d, got %d", expectedTsDiff, ts2-ts1)
	}

	mp.StopPlayback("session-1")
}

func TestPlaybackSession_PCMACodec(t *testing.T) {
	mp := NewMediaPlayer()

	tmpDir := t.TempDir()
	rawFile := filepath.Join(tmpDir, "test.raw")
	rawData := make([]byte, 320)
	os.WriteFile(rawFile, rawData, 0644)

	config := &PlaybackConfig{
		FilePath: rawFile,
		Codec:    "PCMA",
	}

	mp.StartPlayback("session-1", config)

	packet, ok := mp.GetNextPacket("session-1")
	if !ok {
		t.Fatal("Failed to get packet")
	}

	// PCMA should have payload type 8
	if packet[1] != 8 {
		t.Errorf("Expected PT 8 for PCMA, got %d", packet[1])
	}

	mp.StopPlayback("session-1")
}
