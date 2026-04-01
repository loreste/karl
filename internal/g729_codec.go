package internal

import (
	"encoding/binary"
	"errors"
	"sync"
)

// G.729 codec constants
const (
	G729FrameSize      = 10      // bytes per frame (80 bits)
	G729SampleRate     = 8000    // Hz
	G729FrameSamples   = 80      // samples per 10ms frame
	G729FrameDuration  = 10      // milliseconds
	G729BitRate        = 8000    // bits per second
	G729AnnexBSize     = 2       // SID frame size for Annex B (VAD)
	G729PayloadType    = 18      // Standard RTP payload type
)

// G.729 codec errors
var (
	ErrG729InvalidFrame     = errors.New("invalid G.729 frame")
	ErrG729EncoderNotReady  = errors.New("G.729 encoder not initialized")
	ErrG729DecoderNotReady  = errors.New("G.729 decoder not initialized")
	ErrG729FrameTooShort    = errors.New("G.729 frame too short")
	ErrG729UnsupportedMode  = errors.New("unsupported G.729 mode")
)

// G729Mode represents G.729 operating modes
type G729Mode int

const (
	G729ModeStandard  G729Mode = iota // Standard G.729
	G729ModeAnnexA                    // G.729 Annex A (reduced complexity)
	G729ModeAnnexB                    // G.729 Annex B (VAD/CNG)
	G729ModeAnnexAB                   // G.729 Annex A + B
)

// G729Config holds G.729 codec configuration
type G729Config struct {
	Mode              G729Mode
	EnableVAD         bool  // Voice Activity Detection (Annex B)
	EnableDTX         bool  // Discontinuous Transmission
	EnablePLC         bool  // Packet Loss Concealment
	MaxFramesPerPacket int  // Multiple frames per RTP packet
}

// DefaultG729Config returns default G.729 configuration
func DefaultG729Config() *G729Config {
	return &G729Config{
		Mode:              G729ModeAnnexAB,
		EnableVAD:         true,
		EnableDTX:         true,
		EnablePLC:         true,
		MaxFramesPerPacket: 2, // 20ms typical
	}
}

// G729Encoder encodes PCM to G.729
type G729Encoder struct {
	config *G729Config
	mu     sync.Mutex

	// Encoder state (would be bcg729 encoder handle in real implementation)
	initialized bool
	frameBuffer []int16

	// VAD state
	vadActive   bool
	silenceFrames int
}

// NewG729Encoder creates a new G.729 encoder
func NewG729Encoder(config *G729Config) (*G729Encoder, error) {
	if config == nil {
		config = DefaultG729Config()
	}

	enc := &G729Encoder{
		config:      config,
		frameBuffer: make([]int16, G729FrameSamples),
		initialized: true,
	}

	return enc, nil
}

// Encode encodes PCM samples to G.729
func (e *G729Encoder) Encode(pcm []int16) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.initialized {
		return nil, ErrG729EncoderNotReady
	}

	if len(pcm) < G729FrameSamples {
		return nil, ErrG729FrameTooShort
	}

	// Calculate number of frames
	numFrames := len(pcm) / G729FrameSamples
	if numFrames > e.config.MaxFramesPerPacket {
		numFrames = e.config.MaxFramesPerPacket
	}

	// Check for silence if VAD enabled
	if e.config.EnableVAD {
		if e.isFrameSilent(pcm[:G729FrameSamples]) {
			e.silenceFrames++
			if e.silenceFrames > 3 && e.config.EnableDTX {
				// Return SID frame (Annex B)
				return e.encodeSIDFrame(), nil
			}
		} else {
			e.silenceFrames = 0
			e.vadActive = true
		}
	}

	// Encode frames
	// In real implementation, this would call bcg729_encode
	output := make([]byte, numFrames*G729FrameSize)
	for i := 0; i < numFrames; i++ {
		frame := pcm[i*G729FrameSamples : (i+1)*G729FrameSamples]
		encoded := e.encodeFrame(frame)
		copy(output[i*G729FrameSize:], encoded)
	}

	return output, nil
}

// encodeFrame encodes a single G.729 frame
// This is a simplified implementation - real bcg729 would do actual compression
func (e *G729Encoder) encodeFrame(pcm []int16) []byte {
	frame := make([]byte, G729FrameSize)

	// Simplified encoding: convert PCM to compressed representation
	// Real G.729 uses CELP (Code Excited Linear Prediction)
	// This creates a placeholder that maintains frame structure

	// Calculate basic parameters for the frame
	var energy int64
	for _, sample := range pcm {
		energy += int64(sample) * int64(sample)
	}
	avgEnergy := energy / int64(len(pcm))

	// Pack simplified parameters into frame
	// Real G.729 would have LSP, adaptive codebook, fixed codebook, gains
	binary.LittleEndian.PutUint32(frame[0:4], uint32(avgEnergy>>16))

	// Add checksum-like bytes for frame validation
	var sum byte
	for i := 0; i < 4; i++ {
		sum ^= frame[i]
	}
	frame[4] = sum

	return frame
}

// encodeSIDFrame creates a Silence Insertion Descriptor frame
func (e *G729Encoder) encodeSIDFrame() []byte {
	sid := make([]byte, G729AnnexBSize)
	// SID frame marker
	sid[0] = 0x00
	sid[1] = 0x00
	return sid
}

// isFrameSilent checks if a frame is silence
func (e *G729Encoder) isFrameSilent(pcm []int16) bool {
	var energy int64
	for _, sample := range pcm {
		energy += int64(sample) * int64(sample)
	}
	avgEnergy := energy / int64(len(pcm))
	// Threshold for silence detection
	return avgEnergy < 100
}

// Close releases encoder resources
func (e *G729Encoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.initialized = false
	return nil
}

// G729Decoder decodes G.729 to PCM
type G729Decoder struct {
	config *G729Config
	mu     sync.Mutex

	// Decoder state
	initialized bool

	// PLC state for packet loss concealment
	lastFrame   []int16
	plcBuffer   []int16
	lostFrames  int
}

// NewG729Decoder creates a new G.729 decoder
func NewG729Decoder(config *G729Config) (*G729Decoder, error) {
	if config == nil {
		config = DefaultG729Config()
	}

	dec := &G729Decoder{
		config:      config,
		initialized: true,
		lastFrame:   make([]int16, G729FrameSamples),
		plcBuffer:   make([]int16, G729FrameSamples*4),
	}

	return dec, nil
}

// Decode decodes G.729 to PCM samples
func (d *G729Decoder) Decode(g729Data []byte) ([]int16, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.initialized {
		return nil, ErrG729DecoderNotReady
	}

	if len(g729Data) == 0 {
		return nil, ErrG729InvalidFrame
	}

	// Handle SID frame (Annex B silence)
	if len(g729Data) == G729AnnexBSize {
		return d.decodeSIDFrame(g729Data), nil
	}

	// Calculate number of frames
	if len(g729Data)%G729FrameSize != 0 {
		return nil, ErrG729InvalidFrame
	}

	numFrames := len(g729Data) / G729FrameSize
	output := make([]int16, numFrames*G729FrameSamples)

	for i := 0; i < numFrames; i++ {
		frameData := g729Data[i*G729FrameSize : (i+1)*G729FrameSize]
		decoded := d.decodeFrame(frameData)
		copy(output[i*G729FrameSamples:], decoded)
	}

	// Save for PLC
	copy(d.lastFrame, output[len(output)-G729FrameSamples:])
	d.lostFrames = 0

	return output, nil
}

// DecodePLC performs packet loss concealment
func (d *G729Decoder) DecodePLC() ([]int16, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.initialized {
		return nil, ErrG729DecoderNotReady
	}

	if !d.config.EnablePLC {
		// Return silence
		return make([]int16, G729FrameSamples), nil
	}

	d.lostFrames++

	// Apply fade-out for consecutive losses
	output := make([]int16, G729FrameSamples)
	fadeGain := 1.0
	if d.lostFrames > 1 {
		fadeGain = 0.9 / float64(d.lostFrames)
		if fadeGain < 0.1 {
			fadeGain = 0.1
		}
	}

	for i := 0; i < G729FrameSamples; i++ {
		output[i] = int16(float64(d.lastFrame[i]) * fadeGain)
	}

	return output, nil
}

// decodeFrame decodes a single G.729 frame
// This is a simplified implementation - real bcg729 would do actual decompression
func (d *G729Decoder) decodeFrame(g729Data []byte) []int16 {
	output := make([]int16, G729FrameSamples)

	// Simplified decoding - real G.729 uses CELP synthesis
	// This creates a placeholder with basic reconstruction

	if len(g729Data) < 5 {
		return output
	}

	// Extract energy parameter
	energy := binary.LittleEndian.Uint32(g729Data[0:4])

	// Verify checksum
	var sum byte
	for i := 0; i < 4; i++ {
		sum ^= g729Data[i]
	}
	if sum != g729Data[4] {
		// Bad frame, return silence
		return output
	}

	// Reconstruct PCM (simplified)
	amplitude := int16(energy >> 8)
	if amplitude > 16384 {
		amplitude = 16384
	}

	// Generate basic waveform
	for i := 0; i < G729FrameSamples; i++ {
		// Simple reconstruction - real decoder would use LSP, codebooks
		output[i] = amplitude
		amplitude = -amplitude // Simple oscillation
	}

	return output
}

// decodeSIDFrame generates comfort noise for SID frame
func (d *G729Decoder) decodeSIDFrame(sidData []byte) []int16 {
	output := make([]int16, G729FrameSamples)

	// Generate comfort noise
	// Real implementation would use parameters from SID frame
	for i := 0; i < G729FrameSamples; i++ {
		// Low-level white noise
		output[i] = int16((i * 17) % 100) - 50
	}

	return output
}

// Close releases decoder resources
func (d *G729Decoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.initialized = false
	return nil
}

// G729Transcoder handles G.729 transcoding
type G729Transcoder struct {
	encoder *G729Encoder
	decoder *G729Decoder
	config  *G729Config
}

// NewG729Transcoder creates a new G.729 transcoder
func NewG729Transcoder(config *G729Config) (*G729Transcoder, error) {
	if config == nil {
		config = DefaultG729Config()
	}

	encoder, err := NewG729Encoder(config)
	if err != nil {
		return nil, err
	}

	decoder, err := NewG729Decoder(config)
	if err != nil {
		encoder.Close()
		return nil, err
	}

	return &G729Transcoder{
		encoder: encoder,
		decoder: decoder,
		config:  config,
	}, nil
}

// G729ToPCM converts G.729 to PCM
func (t *G729Transcoder) G729ToPCM(g729Data []byte) ([]int16, error) {
	return t.decoder.Decode(g729Data)
}

// PCMToG729 converts PCM to G.729
func (t *G729Transcoder) PCMToG729(pcm []int16) ([]byte, error) {
	return t.encoder.Encode(pcm)
}

// G729ToPCMU converts G.729 to G.711 μ-law
func (t *G729Transcoder) G729ToPCMU(g729Data []byte) ([]byte, error) {
	pcm, err := t.decoder.Decode(g729Data)
	if err != nil {
		return nil, err
	}

	output := make([]byte, len(pcm))
	for i, sample := range pcm {
		output[i] = LinearToMulaw(sample)
	}
	return output, nil
}

// PCMUToG729 converts G.711 μ-law to G.729
func (t *G729Transcoder) PCMUToG729(pcmuData []byte) ([]byte, error) {
	pcm := make([]int16, len(pcmuData))
	for i, sample := range pcmuData {
		pcm[i] = MulawToLinear(sample)
	}
	return t.encoder.Encode(pcm)
}

// G729ToPCMA converts G.729 to G.711 A-law
func (t *G729Transcoder) G729ToPCMA(g729Data []byte) ([]byte, error) {
	pcm, err := t.decoder.Decode(g729Data)
	if err != nil {
		return nil, err
	}

	output := make([]byte, len(pcm))
	for i, sample := range pcm {
		output[i] = LinearToAlaw(sample)
	}
	return output, nil
}

// PCMAToG729 converts G.711 A-law to G.729
func (t *G729Transcoder) PCMAToG729(pcmaData []byte) ([]byte, error) {
	pcm := make([]int16, len(pcmaData))
	for i, sample := range pcmaData {
		pcm[i] = AlawToLinear(sample)
	}
	return t.encoder.Encode(pcm)
}

// Close releases transcoder resources
func (t *G729Transcoder) Close() error {
	var firstErr error
	if err := t.encoder.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := t.decoder.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// LinearToMulaw converts 16-bit linear PCM to μ-law (ITU-T G.711)
func LinearToMulaw(sample int16) byte {
	const bias = 33
	const clip = 32635

	// Get sign bit and absolute value
	sign := 0
	var absVal int
	if sample < 0 {
		sign = 0x80
		// Handle int16 min value edge case
		if sample == -32768 {
			absVal = 32768
		} else {
			absVal = int(-sample)
		}
	} else {
		absVal = int(sample)
	}

	// Clip
	if absVal > clip {
		absVal = clip
	}

	// Add bias
	absVal += bias

	// Find segment and quantize
	var exponent, mantissa int
	if absVal >= 0x4000 {
		exponent = 7
		mantissa = (absVal >> 10) & 0x0F
	} else if absVal >= 0x2000 {
		exponent = 6
		mantissa = (absVal >> 9) & 0x0F
	} else if absVal >= 0x1000 {
		exponent = 5
		mantissa = (absVal >> 8) & 0x0F
	} else if absVal >= 0x0800 {
		exponent = 4
		mantissa = (absVal >> 7) & 0x0F
	} else if absVal >= 0x0400 {
		exponent = 3
		mantissa = (absVal >> 6) & 0x0F
	} else if absVal >= 0x0200 {
		exponent = 2
		mantissa = (absVal >> 5) & 0x0F
	} else if absVal >= 0x0100 {
		exponent = 1
		mantissa = (absVal >> 4) & 0x0F
	} else {
		exponent = 0
		mantissa = (absVal >> 3) & 0x0F
	}

	// μ-law byte = ~(sign | exponent | mantissa)
	return byte(^(sign | (exponent << 4) | mantissa))
}

// MulawToLinear converts μ-law to 16-bit linear PCM (ITU-T G.711)
func MulawToLinear(mulaw byte) int16 {
	const bias = 33

	// Complement to get original
	mulaw = ^mulaw

	sign := int(mulaw & 0x80)
	exponent := int((mulaw >> 4) & 0x07)
	mantissa := int(mulaw & 0x0F)

	// Reconstruct linear value
	// Formula: ((mantissa << 3) + bias) << exponent
	sample := ((mantissa << 3) + 0x84) << exponent
	sample -= bias

	if sign != 0 {
		return -int16(sample)
	}
	return int16(sample)
}

// LinearToAlaw converts 16-bit linear PCM to A-law (ITU-T G.711)
func LinearToAlaw(sample int16) byte {
	const clip = 32635

	// Get sign bit and absolute value
	sign := 0
	var absVal int
	if sample < 0 {
		sign = 0x80
		if sample == -32768 {
			absVal = 32768
		} else {
			absVal = int(-sample)
		}
	} else {
		absVal = int(sample)
	}

	// Clip
	if absVal > clip {
		absVal = clip
	}

	// Find segment and quantize (16-bit thresholds)
	var exponent, mantissa int
	if absVal >= 16384 {
		exponent = 7
		mantissa = (absVal >> 10) & 0x0F
	} else if absVal >= 8192 {
		exponent = 6
		mantissa = (absVal >> 9) & 0x0F
	} else if absVal >= 4096 {
		exponent = 5
		mantissa = (absVal >> 8) & 0x0F
	} else if absVal >= 2048 {
		exponent = 4
		mantissa = (absVal >> 7) & 0x0F
	} else if absVal >= 1024 {
		exponent = 3
		mantissa = (absVal >> 6) & 0x0F
	} else if absVal >= 512 {
		exponent = 2
		mantissa = (absVal >> 5) & 0x0F
	} else if absVal >= 256 {
		exponent = 1
		mantissa = (absVal >> 4) & 0x0F
	} else {
		exponent = 0
		mantissa = absVal >> 4
	}

	// A-law byte with XOR pattern
	return byte(sign|(exponent<<4)|mantissa) ^ 0x55
}

// AlawToLinear converts A-law to 16-bit linear PCM (ITU-T G.711)
func AlawToLinear(alaw byte) int16 {
	// Undo XOR pattern
	alaw ^= 0x55

	sign := alaw & 0x80
	exponent := int((alaw >> 4) & 0x07)
	mantissa := int(alaw & 0x0F)

	// Reconstruct 16-bit linear value
	var sample int
	if exponent == 0 {
		sample = (mantissa << 4) + 8
	} else {
		sample = ((mantissa << 4) + 0x108) << (exponent - 1)
	}

	if sign != 0 {
		return -int16(sample)
	}
	return int16(sample)
}

// IsG729Frame validates a G.729 frame
func IsG729Frame(data []byte) bool {
	if len(data) == G729AnnexBSize {
		// Valid SID frame
		return true
	}
	if len(data) >= G729FrameSize && len(data)%G729FrameSize == 0 {
		// Valid G.729 frame(s)
		return true
	}
	return false
}

// G729FrameCount returns the number of G.729 frames in data
func G729FrameCount(data []byte) int {
	if len(data) == G729AnnexBSize {
		return 1 // SID frame
	}
	return len(data) / G729FrameSize
}

// G729Duration returns the duration in milliseconds
func G729Duration(data []byte) int {
	return G729FrameCount(data) * G729FrameDuration
}
