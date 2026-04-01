package internal

import (
	"encoding/binary"
	"errors"
	"sync"
)

// AMR codec constants
const (
	// AMR-NB (Narrowband) constants
	AMRNBSampleRate    = 8000
	AMRNBFrameDuration = 20 // ms
	AMRNBFrameSamples  = 160

	// AMR-WB (Wideband) constants
	AMRWBSampleRate    = 16000
	AMRWBFrameDuration = 20 // ms
	AMRWBFrameSamples  = 320
)

// AMR mode (bit rate) definitions
type AMRMode int

const (
	// AMR-NB modes (narrowband)
	AMRMode475  AMRMode = 0 // 4.75 kbps
	AMRMode515  AMRMode = 1 // 5.15 kbps
	AMRMode590  AMRMode = 2 // 5.90 kbps
	AMRMode670  AMRMode = 3 // 6.70 kbps
	AMRMode740  AMRMode = 4 // 7.40 kbps
	AMRMode795  AMRMode = 5 // 7.95 kbps
	AMRMode102  AMRMode = 6 // 10.2 kbps
	AMRMode122  AMRMode = 7 // 12.2 kbps
	AMRModeSID  AMRMode = 8 // SID (Silence Descriptor)
	AMRModeNoData AMRMode = 15
)

// AMR-WB modes (wideband)
const (
	AMRWBMode660  AMRMode = 0  // 6.60 kbps
	AMRWBMode885  AMRMode = 1  // 8.85 kbps
	AMRWBMode1265 AMRMode = 2  // 12.65 kbps
	AMRWBMode1425 AMRMode = 3  // 14.25 kbps
	AMRWBMode1585 AMRMode = 4  // 15.85 kbps
	AMRWBMode1825 AMRMode = 5  // 18.25 kbps
	AMRWBMode1985 AMRMode = 6  // 19.85 kbps
	AMRWBMode2305 AMRMode = 7  // 23.05 kbps
	AMRWBMode2385 AMRMode = 8  // 23.85 kbps
	AMRWBModeSID  AMRMode = 9  // SID
)

// Frame sizes for AMR-NB modes (in bytes, excluding header)
var amrNBFrameSizes = map[AMRMode]int{
	AMRMode475:  12,
	AMRMode515:  13,
	AMRMode590:  15,
	AMRMode670:  17,
	AMRMode740:  19,
	AMRMode795:  20,
	AMRMode102:  26,
	AMRMode122:  31,
	AMRModeSID:  5,
	AMRModeNoData: 0,
}

// Frame sizes for AMR-WB modes (in bytes, excluding header)
var amrWBFrameSizes = map[AMRMode]int{
	AMRWBMode660:  17,
	AMRWBMode885:  23,
	AMRWBMode1265: 32,
	AMRWBMode1425: 36,
	AMRWBMode1585: 40,
	AMRWBMode1825: 46,
	AMRWBMode1985: 50,
	AMRWBMode2305: 58,
	AMRWBMode2385: 60,
	AMRWBModeSID:  5,
	AMRModeNoData: 0,
}

// AMR errors
var (
	ErrAMRInvalidFrame     = errors.New("invalid AMR frame")
	ErrAMRInvalidMode      = errors.New("invalid AMR mode")
	ErrAMRDecoderNotReady  = errors.New("AMR decoder not initialized")
	ErrAMREncoderNotReady  = errors.New("AMR encoder not initialized")
	ErrAMRUnsupportedMode  = errors.New("unsupported AMR mode")
)

// AMRConfig configures the AMR codec
type AMRConfig struct {
	// Wideband enables AMR-WB instead of AMR-NB
	Wideband bool
	// Mode is the default encoding mode
	Mode AMRMode
	// EnableDTX enables Discontinuous Transmission
	EnableDTX bool
	// OctetAligned uses octet-aligned mode instead of bandwidth-efficient
	OctetAligned bool
}

// DefaultAMRConfig returns default AMR configuration
func DefaultAMRConfig() *AMRConfig {
	return &AMRConfig{
		Wideband:     false,
		Mode:         AMRMode122, // 12.2 kbps for best quality
		EnableDTX:    true,
		OctetAligned: true,
	}
}

// DefaultAMRWBConfig returns default AMR-WB configuration
func DefaultAMRWBConfig() *AMRConfig {
	return &AMRConfig{
		Wideband:     true,
		Mode:         AMRWBMode2385, // 23.85 kbps for best quality
		EnableDTX:    true,
		OctetAligned: true,
	}
}

// AMREncoder encodes PCM to AMR
type AMREncoder struct {
	config      *AMRConfig
	mu          sync.Mutex
	initialized bool
	frameCount  int64

	// Encoder state (simplified - real implementation would use opencore-amr)
	prevSamples []int16
	vadState    *vadState
}

type vadState struct {
	energy      float64
	hangover    int
	silentCount int
}

// NewAMREncoder creates a new AMR encoder
func NewAMREncoder(config *AMRConfig) (*AMREncoder, error) {
	if config == nil {
		config = DefaultAMRConfig()
	}

	enc := &AMREncoder{
		config:      config,
		initialized: true,
		vadState:    &vadState{},
	}

	if config.Wideband {
		enc.prevSamples = make([]int16, AMRWBFrameSamples)
	} else {
		enc.prevSamples = make([]int16, AMRNBFrameSamples)
	}

	return enc, nil
}

// Encode encodes PCM samples to AMR
func (e *AMREncoder) Encode(samples []int16) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.initialized {
		return nil, ErrAMREncoderNotReady
	}

	expectedSamples := AMRNBFrameSamples
	if e.config.Wideband {
		expectedSamples = AMRWBFrameSamples
	}

	if len(samples) != expectedSamples {
		return nil, ErrAMRInvalidFrame
	}

	// Determine mode based on VAD if DTX is enabled
	mode := e.config.Mode
	if e.config.EnableDTX {
		if e.detectSilence(samples) {
			mode = AMRModeSID
		}
	}

	// Get frame size for this mode
	frameSizes := amrNBFrameSizes
	if e.config.Wideband {
		frameSizes = amrWBFrameSizes
	}

	frameSize, ok := frameSizes[mode]
	if !ok {
		return nil, ErrAMRInvalidMode
	}

	// Create output frame
	output := make([]byte, frameSize+1) // +1 for header

	// Build header byte (CMR | F | FT | Q)
	// CMR (4 bits) = mode, F (1 bit) = 0 for last frame, FT (4 bits) = frame type, Q (1 bit) = quality
	if e.config.OctetAligned {
		output[0] = byte((mode & 0x0F) << 4) // CMR in high nibble
		output[0] |= 0x04                     // Q bit set (good quality)
	} else {
		output[0] = byte((mode & 0x0F) << 3) // Bandwidth-efficient mode
	}

	// Encode the frame (simplified - real encoding would use ACELP)
	e.encodeFrame(samples, output[1:], mode)

	e.frameCount++
	copy(e.prevSamples, samples)

	return output, nil
}

func (e *AMREncoder) detectSilence(samples []int16) bool {
	// Calculate frame energy
	var energy float64
	for _, s := range samples {
		energy += float64(s) * float64(s)
	}
	energy /= float64(len(samples))
	energy = energy / (32768.0 * 32768.0) // Normalize

	// Simple VAD with hangover
	threshold := 0.001
	if energy < threshold {
		e.vadState.silentCount++
		if e.vadState.silentCount > 10 { // 200ms of silence
			return true
		}
	} else {
		e.vadState.silentCount = 0
		e.vadState.hangover = 5 // 100ms hangover
	}

	if e.vadState.hangover > 0 {
		e.vadState.hangover--
		return false
	}

	return energy < threshold
}

func (e *AMREncoder) encodeFrame(samples []int16, output []byte, mode AMRMode) {
	// Simplified encoding - real AMR uses ACELP
	// This creates a placeholder that maintains frame structure

	if len(output) == 0 {
		return
	}

	// Calculate basic parameters
	var maxAmp int16
	var avgAmp int64
	for _, s := range samples {
		if s < 0 {
			s = -s
		}
		if s > maxAmp {
			maxAmp = s
		}
		avgAmp += int64(s)
	}
	avgAmp /= int64(len(samples))

	// Store energy and basic parameters
	if len(output) >= 4 {
		binary.BigEndian.PutUint16(output[0:2], uint16(maxAmp))
		binary.BigEndian.PutUint16(output[2:4], uint16(avgAmp))
	}

	// Fill rest with basic encoding
	for i := 4; i < len(output); i++ {
		if i < len(samples)/8 {
			output[i] = byte(samples[i*8] >> 8)
		} else {
			output[i] = 0
		}
	}
}

// Close closes the encoder
func (e *AMREncoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.initialized = false
	return nil
}

// AMRDecoder decodes AMR to PCM
type AMRDecoder struct {
	config      *AMRConfig
	mu          sync.Mutex
	initialized bool
	frameCount  int64
	prevSamples []int16
	plcState    *amrPLCState
}

type amrPLCState struct {
	lostFrames   int
	lastGoodMode AMRMode
	fadeGain     float64
}

// NewAMRDecoder creates a new AMR decoder
func NewAMRDecoder(config *AMRConfig) (*AMRDecoder, error) {
	if config == nil {
		config = DefaultAMRConfig()
	}

	dec := &AMRDecoder{
		config:      config,
		initialized: true,
		plcState:    &amrPLCState{fadeGain: 1.0},
	}

	if config.Wideband {
		dec.prevSamples = make([]int16, AMRWBFrameSamples)
	} else {
		dec.prevSamples = make([]int16, AMRNBFrameSamples)
	}

	return dec, nil
}

// Decode decodes an AMR frame to PCM samples
func (d *AMRDecoder) Decode(frame []byte) ([]int16, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.initialized {
		return nil, ErrAMRDecoderNotReady
	}

	if len(frame) < 1 {
		return nil, ErrAMRInvalidFrame
	}

	// Parse header
	var mode AMRMode
	if d.config.OctetAligned {
		mode = AMRMode((frame[0] >> 4) & 0x0F)
	} else {
		mode = AMRMode((frame[0] >> 3) & 0x0F)
	}

	// Validate mode and get expected frame size
	frameSizes := amrNBFrameSizes
	if d.config.Wideband {
		frameSizes = amrWBFrameSizes
	}

	expectedSize, ok := frameSizes[mode]
	if !ok {
		return nil, ErrAMRInvalidMode
	}

	outputSamples := AMRNBFrameSamples
	if d.config.Wideband {
		outputSamples = AMRWBFrameSamples
	}

	output := make([]int16, outputSamples)

	// Handle SID frame (comfort noise)
	if mode == AMRModeSID || mode == AMRWBModeSID {
		d.generateComfortNoise(output)
		return output, nil
	}

	// Handle no data
	if mode == AMRModeNoData || expectedSize == 0 {
		// PLC
		return d.concealLoss()
	}

	// Validate frame size
	if len(frame) < expectedSize+1 {
		return d.concealLoss()
	}

	// Decode frame
	d.decodeFrame(frame[1:], output, mode)

	d.frameCount++
	d.plcState.lostFrames = 0
	d.plcState.lastGoodMode = mode
	d.plcState.fadeGain = 1.0
	copy(d.prevSamples, output)

	return output, nil
}

func (d *AMRDecoder) decodeFrame(frame []byte, output []int16, mode AMRMode) {
	// Simplified decoding - real AMR uses ACELP synthesis
	// This reconstructs basic audio from the placeholder encoding

	if len(frame) < 4 {
		// Not enough data, use silence
		return
	}

	// Extract basic parameters
	maxAmp := int16(binary.BigEndian.Uint16(frame[0:2]))
	avgAmp := int16(binary.BigEndian.Uint16(frame[2:4]))

	// Generate basic waveform
	for i := range output {
		// Simple reconstruction with some variation
		if i < len(frame)-4 && frame[4+i/8] != 0 {
			output[i] = int16(frame[4+i/8]) << 8
		} else {
			// Interpolate
			output[i] = avgAmp
			if i%2 == 0 {
				output[i] = -output[i]
			}
		}

		// Apply gain based on maxAmp
		scale := float64(maxAmp) / 32768.0
		output[i] = int16(float64(output[i]) * scale)
	}
}

func (d *AMRDecoder) generateComfortNoise(output []int16) {
	// Generate low-level comfort noise
	for i := range output {
		// Simple noise generation
		noise := int16((i * 17) % 200) - 100
		output[i] = noise
	}
}

func (d *AMRDecoder) concealLoss() ([]int16, error) {
	d.plcState.lostFrames++

	outputSamples := AMRNBFrameSamples
	if d.config.Wideband {
		outputSamples = AMRWBFrameSamples
	}

	output := make([]int16, outputSamples)

	// Apply fade-out for packet loss concealment
	if d.plcState.lostFrames > 5 {
		d.plcState.fadeGain *= 0.9
	}
	if d.plcState.fadeGain < 0.1 {
		d.plcState.fadeGain = 0.1
	}

	// Repeat last frame with gain reduction
	for i := range output {
		output[i] = int16(float64(d.prevSamples[i]) * d.plcState.fadeGain)
	}

	copy(d.prevSamples, output)
	return output, nil
}

// Close closes the decoder
func (d *AMRDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.initialized = false
	return nil
}

// AMRTranscoder handles AMR to PCM and PCM to AMR transcoding
type AMRTranscoder struct {
	encoder *AMREncoder
	decoder *AMRDecoder
	config  *AMRConfig
}

// NewAMRTranscoder creates a new AMR transcoder
func NewAMRTranscoder(config *AMRConfig) (*AMRTranscoder, error) {
	if config == nil {
		config = DefaultAMRConfig()
	}

	enc, err := NewAMREncoder(config)
	if err != nil {
		return nil, err
	}

	dec, err := NewAMRDecoder(config)
	if err != nil {
		enc.Close()
		return nil, err
	}

	return &AMRTranscoder{
		encoder: enc,
		decoder: dec,
		config:  config,
	}, nil
}

// PCMToAMR converts PCM to AMR
func (t *AMRTranscoder) PCMToAMR(samples []int16) ([]byte, error) {
	return t.encoder.Encode(samples)
}

// AMRToPCM converts AMR to PCM
func (t *AMRTranscoder) AMRToPCM(frame []byte) ([]int16, error) {
	return t.decoder.Decode(frame)
}

// Close closes the transcoder
func (t *AMRTranscoder) Close() error {
	var err error
	if e := t.encoder.Close(); e != nil {
		err = e
	}
	if e := t.decoder.Close(); e != nil {
		err = e
	}
	return err
}

// GetAMRFrameSize returns the frame size for a given mode
func GetAMRFrameSize(mode AMRMode, wideband bool) int {
	if wideband {
		if size, ok := amrWBFrameSizes[mode]; ok {
			return size + 1 // +1 for header
		}
	} else {
		if size, ok := amrNBFrameSizes[mode]; ok {
			return size + 1 // +1 for header
		}
	}
	return 0
}

// ParseAMRMode extracts the mode from an AMR frame header
func ParseAMRMode(header byte, octetAligned bool) AMRMode {
	if octetAligned {
		return AMRMode((header >> 4) & 0x0F)
	}
	return AMRMode((header >> 3) & 0x0F)
}

// IsAMRSIDFrame checks if a frame is a SID frame
func IsAMRSIDFrame(mode AMRMode, wideband bool) bool {
	if wideband {
		return mode == AMRWBModeSID
	}
	return mode == AMRModeSID
}
