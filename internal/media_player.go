package internal

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// MediaPlayer handles audio file playback into RTP streams
type MediaPlayer struct {
	sessions map[string]*PlaybackSession
	mu       sync.RWMutex
	stopCh   chan struct{}
}

// PlaybackSession represents an active media playback
type PlaybackSession struct {
	SessionID    string
	FilePath     string
	Codec        string
	SampleRate   int
	Channels     int
	Loop         bool
	BlendOriginal bool // Mix with original audio instead of replacing

	// Playback state
	file         *os.File
	audioData    []byte
	position     int
	playing      bool
	paused       bool
	seqNum       uint16
	timestamp    uint32
	ssrc         uint32
	startTime    time.Time

	// Target leg
	TargetLeg    string // "caller", "callee", or "both"

	mu           sync.Mutex
	stopCh       chan struct{}
}

// PlaybackConfig holds playback configuration
type PlaybackConfig struct {
	FilePath      string
	Codec         string // PCMU, PCMA, or auto-detect
	Loop          bool
	BlendOriginal bool
	TargetLeg     string
	SSRC          uint32
}

// NewMediaPlayer creates a new media player
func NewMediaPlayer() *MediaPlayer {
	return &MediaPlayer{
		sessions: make(map[string]*PlaybackSession),
		stopCh:   make(chan struct{}),
	}
}

// StartPlayback starts playing media into a session
func (mp *MediaPlayer) StartPlayback(sessionID string, config *PlaybackConfig) error {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	// Stop any existing playback for this session
	if existing, ok := mp.sessions[sessionID]; ok {
		existing.Stop()
		delete(mp.sessions, sessionID)
	}

	// Load the audio file
	audioData, sampleRate, channels, codec, err := mp.loadAudioFile(config.FilePath)
	if err != nil {
		return fmt.Errorf("failed to load audio file: %w", err)
	}

	// Override codec if specified
	if config.Codec != "" {
		codec = config.Codec
	}

	ps := &PlaybackSession{
		SessionID:     sessionID,
		FilePath:      config.FilePath,
		Codec:         codec,
		SampleRate:    sampleRate,
		Channels:      channels,
		Loop:          config.Loop,
		BlendOriginal: config.BlendOriginal,
		TargetLeg:     config.TargetLeg,
		audioData:     audioData,
		playing:       true,
		seqNum:        1000,
		timestamp:     0,
		ssrc:          config.SSRC,
		startTime:     time.Now(),
		stopCh:        make(chan struct{}),
	}

	if ps.ssrc == 0 {
		ps.ssrc = uint32(time.Now().UnixNano() & 0xFFFFFFFF)
	}

	mp.sessions[sessionID] = ps
	return nil
}

// StopPlayback stops media playback for a session
func (mp *MediaPlayer) StopPlayback(sessionID string) error {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	ps, ok := mp.sessions[sessionID]
	if !ok {
		return fmt.Errorf("no playback session found: %s", sessionID)
	}

	ps.Stop()
	delete(mp.sessions, sessionID)
	return nil
}

// PausePlayback pauses media playback
func (mp *MediaPlayer) PausePlayback(sessionID string) error {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	ps, ok := mp.sessions[sessionID]
	if !ok {
		return fmt.Errorf("no playback session found: %s", sessionID)
	}

	ps.Pause()
	return nil
}

// ResumePlayback resumes paused playback
func (mp *MediaPlayer) ResumePlayback(sessionID string) error {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	ps, ok := mp.sessions[sessionID]
	if !ok {
		return fmt.Errorf("no playback session found: %s", sessionID)
	}

	ps.Resume()
	return nil
}

// GetNextPacket returns the next RTP packet for playback
func (mp *MediaPlayer) GetNextPacket(sessionID string) ([]byte, bool) {
	mp.mu.RLock()
	ps, ok := mp.sessions[sessionID]
	mp.mu.RUnlock()

	if !ok {
		return nil, false
	}

	return ps.GetNextPacket()
}

// IsPlaying checks if a session has active playback
func (mp *MediaPlayer) IsPlaying(sessionID string) bool {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	ps, ok := mp.sessions[sessionID]
	if !ok {
		return false
	}
	return ps.playing && !ps.paused
}

// Stop stops all playback
func (mp *MediaPlayer) Stop() {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	for _, ps := range mp.sessions {
		ps.Stop()
	}
	mp.sessions = make(map[string]*PlaybackSession)
	close(mp.stopCh)
}

// loadAudioFile loads and parses an audio file
func (mp *MediaPlayer) loadAudioFile(filePath string) ([]byte, int, int, string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, 0, "", err
	}
	defer file.Close()

	// Check if it's a WAV file
	header := make([]byte, 44)
	n, err := file.Read(header)
	if err != nil && err != io.EOF {
		return nil, 0, 0, "", err
	}

	if n >= 44 && string(header[0:4]) == "RIFF" && string(header[8:12]) == "WAVE" {
		return mp.loadWAVFile(file, header)
	}

	// Assume raw PCM (G.711 u-law)
	file.Seek(0, 0)
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, 0, 0, "", err
	}

	return data, 8000, 1, "PCMU", nil
}

// loadWAVFile parses a WAV file
func (mp *MediaPlayer) loadWAVFile(file *os.File, header []byte) ([]byte, int, int, string, error) {
	// Parse WAV header
	// Bytes 22-23: Number of channels
	channels := int(binary.LittleEndian.Uint16(header[22:24]))
	// Bytes 24-27: Sample rate
	sampleRate := int(binary.LittleEndian.Uint32(header[24:28]))
	// Bytes 34-35: Bits per sample
	bitsPerSample := int(binary.LittleEndian.Uint16(header[34:36]))

	// Find data chunk
	file.Seek(12, 0) // Skip RIFF header

	for {
		chunkHeader := make([]byte, 8)
		_, err := file.Read(chunkHeader)
		if err != nil {
			return nil, 0, 0, "", fmt.Errorf("failed to find data chunk: %w", err)
		}

		chunkID := string(chunkHeader[0:4])
		chunkSize := binary.LittleEndian.Uint32(chunkHeader[4:8])

		if chunkID == "data" {
			data := make([]byte, chunkSize)
			_, err := io.ReadFull(file, data)
			if err != nil {
				return nil, 0, 0, "", err
			}

			// Determine codec based on format
			codec := "PCMU"
			if bitsPerSample == 16 {
				// Convert 16-bit PCM to G.711 u-law
				data = convertPCM16ToUlaw(data)
			} else if bitsPerSample == 8 {
				// Already 8-bit, check if it's a-law or u-law
				// Default to u-law
				codec = "PCMU"
			}

			return data, sampleRate, channels, codec, nil
		}

		// Skip this chunk
		file.Seek(int64(chunkSize), 1)
	}
}

// convertPCM16ToUlaw converts 16-bit PCM to G.711 u-law
func convertPCM16ToUlaw(pcm []byte) []byte {
	samples := len(pcm) / 2
	ulaw := make([]byte, samples)

	for i := 0; i < samples; i++ {
		// Read 16-bit sample (little-endian)
		sample := int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2]))
		ulaw[i] = linearToUlaw(sample)
	}

	return ulaw
}

// linearToUlaw converts a 16-bit linear sample to u-law
func linearToUlaw(sample int16) byte {
	const BIAS = 0x84
	const CLIP = 32635

	// Get the sign bit
	sign := byte(0)
	if sample < 0 {
		sign = 0x80
		sample = -sample
	}

	// Clip the magnitude
	if sample > CLIP {
		sample = CLIP
	}

	// Add bias for rounding
	sample += BIAS

	// Find the position of the most significant 1 bit
	exponent := 7
	expMask := int16(0x4000)
	for exponent > 0 {
		if (sample & expMask) != 0 {
			break
		}
		exponent--
		expMask >>= 1
	}

	// Get mantissa bits
	mantissa := byte((sample >> (exponent + 3)) & 0x0F)

	// Construct the u-law byte
	return ^(sign | byte(exponent<<4) | mantissa)
}

// Stop stops the playback session
func (ps *PlaybackSession) Stop() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.playing {
		ps.playing = false
		close(ps.stopCh)
	}

	if ps.file != nil {
		ps.file.Close()
		ps.file = nil
	}
}

// Pause pauses the playback
func (ps *PlaybackSession) Pause() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.paused = true
}

// Resume resumes paused playback
func (ps *PlaybackSession) Resume() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.paused = false
}

// GetNextPacket returns the next RTP packet
func (ps *PlaybackSession) GetNextPacket() ([]byte, bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if !ps.playing || ps.paused {
		return nil, false
	}

	// Determine payload size based on codec and ptime (20ms default)
	// G.711: 8000 Hz, 8 bits/sample = 160 bytes per 20ms
	payloadSize := 160
	if ps.SampleRate == 16000 {
		payloadSize = 320
	}

	// Check if we have enough data
	if ps.position >= len(ps.audioData) {
		if ps.Loop {
			ps.position = 0
		} else {
			ps.playing = false
			return nil, false
		}
	}

	// Calculate how much data to take
	endPos := ps.position + payloadSize
	if endPos > len(ps.audioData) {
		if ps.Loop {
			// Wrap around
			payload := make([]byte, payloadSize)
			firstPart := len(ps.audioData) - ps.position
			copy(payload, ps.audioData[ps.position:])
			copy(payload[firstPart:], ps.audioData[:payloadSize-firstPart])
			ps.position = payloadSize - firstPart
		} else {
			endPos = len(ps.audioData)
		}
	}

	payload := ps.audioData[ps.position:endPos]
	ps.position = endPos

	// Build RTP packet
	packet := make([]byte, 12+len(payload))

	// RTP header
	packet[0] = 0x80 // Version 2, no padding, no extension, no CSRC

	// Payload type based on codec
	switch ps.Codec {
	case "PCMU":
		packet[1] = 0 // PT 0 for PCMU
	case "PCMA":
		packet[1] = 8 // PT 8 for PCMA
	default:
		packet[1] = 0
	}

	// Sequence number
	binary.BigEndian.PutUint16(packet[2:4], ps.seqNum)
	ps.seqNum++

	// Timestamp
	binary.BigEndian.PutUint32(packet[4:8], ps.timestamp)
	ps.timestamp += uint32(payloadSize) // Increment by samples

	// SSRC
	binary.BigEndian.PutUint32(packet[8:12], ps.ssrc)

	// Payload
	copy(packet[12:], payload)

	return packet, true
}

// GetPlaybackStats returns playback statistics
func (ps *PlaybackSession) GetPlaybackStats() map[string]interface{} {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	progress := float64(0)
	if len(ps.audioData) > 0 {
		progress = float64(ps.position) / float64(len(ps.audioData)) * 100
	}

	return map[string]interface{}{
		"file":       ps.FilePath,
		"codec":      ps.Codec,
		"playing":    ps.playing,
		"paused":     ps.paused,
		"loop":       ps.Loop,
		"progress":   progress,
		"duration":   time.Since(ps.startTime).Seconds(),
		"position":   ps.position,
		"total_size": len(ps.audioData),
	}
}

// GetStats returns overall player statistics
func (mp *MediaPlayer) GetStats() map[string]interface{} {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	sessions := make(map[string]interface{})
	for id, ps := range mp.sessions {
		sessions[id] = ps.GetPlaybackStats()
	}

	return map[string]interface{}{
		"active_playbacks": len(mp.sessions),
		"sessions":         sessions,
	}
}
