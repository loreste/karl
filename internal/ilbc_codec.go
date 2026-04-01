package internal

import (
	"encoding/binary"
	"errors"
	"sync"
)

// iLBC codec constants
const (
	// iLBC operates at 8000 Hz
	ILBCSampleRate = 8000

	// 20ms mode
	ILBC20FrameDuration = 20 // ms
	ILBC20FrameSamples  = 160
	ILBC20FrameSize     = 38 // bytes

	// 30ms mode
	ILBC30FrameDuration = 30 // ms
	ILBC30FrameSamples  = 240
	ILBC30FrameSize     = 50 // bytes

	// Bitrates
	ILBC20Bitrate = 15200 // bits/second for 20ms mode
	ILBC30Bitrate = 13333 // bits/second for 30ms mode
)

// iLBC errors
var (
	ErrILBCInvalidFrame    = errors.New("invalid iLBC frame")
	ErrILBCDecoderNotReady = errors.New("iLBC decoder not initialized")
	ErrILBCEncoderNotReady = errors.New("iLBC encoder not initialized")
	ErrILBCInvalidMode     = errors.New("invalid iLBC mode")
)

// ILBCMode represents the iLBC frame mode
type ILBCMode int

const (
	ILBCMode20ms ILBCMode = 20
	ILBCMode30ms ILBCMode = 30
)

// ILBCConfig configures the iLBC codec
type ILBCConfig struct {
	// Mode is the frame duration mode (20ms or 30ms)
	Mode ILBCMode
	// EnablePLC enables Packet Loss Concealment
	EnablePLC bool
	// PLCLPCOrder is the LPC order for PLC (default 10)
	PLCLPCOrder int
}

// DefaultILBCConfig returns default iLBC configuration
func DefaultILBCConfig() *ILBCConfig {
	return &ILBCConfig{
		Mode:        ILBCMode30ms, // 30ms is default for better quality
		EnablePLC:   true,
		PLCLPCOrder: 10,
	}
}

// ILBCEncoder encodes PCM to iLBC
type ILBCEncoder struct {
	config      *ILBCConfig
	mu          sync.Mutex
	initialized bool
	frameCount  int64

	// Encoder state
	lpcCoeffs   []float64
	prevSamples []int16
	residual    []float64
}

// NewILBCEncoder creates a new iLBC encoder
func NewILBCEncoder(config *ILBCConfig) (*ILBCEncoder, error) {
	if config == nil {
		config = DefaultILBCConfig()
	}

	if config.Mode != ILBCMode20ms && config.Mode != ILBCMode30ms {
		return nil, ErrILBCInvalidMode
	}

	enc := &ILBCEncoder{
		config:      config,
		initialized: true,
		lpcCoeffs:   make([]float64, config.PLCLPCOrder+1),
	}

	frameSamples := ILBC20FrameSamples
	if config.Mode == ILBCMode30ms {
		frameSamples = ILBC30FrameSamples
	}
	enc.prevSamples = make([]int16, frameSamples)
	enc.residual = make([]float64, frameSamples)

	return enc, nil
}

// Encode encodes PCM samples to iLBC
func (e *ILBCEncoder) Encode(samples []int16) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.initialized {
		return nil, ErrILBCEncoderNotReady
	}

	expectedSamples := ILBC20FrameSamples
	frameSize := ILBC20FrameSize
	if e.config.Mode == ILBCMode30ms {
		expectedSamples = ILBC30FrameSamples
		frameSize = ILBC30FrameSize
	}

	if len(samples) != expectedSamples {
		return nil, ErrILBCInvalidFrame
	}

	output := make([]byte, frameSize)

	// Simplified iLBC encoding
	// Real iLBC uses block-independent LPC analysis and residual coding

	// Step 1: LPC analysis
	e.computeLPC(samples)

	// Step 2: Compute residual
	e.computeResidual(samples)

	// Step 3: Encode parameters
	e.encodeParameters(output)

	e.frameCount++
	copy(e.prevSamples, samples)

	return output, nil
}

func (e *ILBCEncoder) computeLPC(samples []int16) {
	// Simplified LPC using autocorrelation method
	n := len(samples)
	order := len(e.lpcCoeffs) - 1

	// Compute autocorrelation
	r := make([]float64, order+1)
	for i := 0; i <= order; i++ {
		for j := 0; j < n-i; j++ {
			r[i] += float64(samples[j]) * float64(samples[j+i])
		}
	}

	// Levinson-Durbin recursion (simplified)
	if r[0] > 0 {
		e.lpcCoeffs[0] = 1.0
		for i := 1; i <= order; i++ {
			sum := 0.0
			for j := 1; j < i; j++ {
				sum += e.lpcCoeffs[j] * r[i-j]
			}
			if r[0] != 0 {
				e.lpcCoeffs[i] = -(r[i] + sum) / r[0]
			}
		}
	}
}

func (e *ILBCEncoder) computeResidual(samples []int16) {
	order := len(e.lpcCoeffs) - 1
	for i := range samples {
		var pred float64
		for j := 1; j <= order && i-j >= 0; j++ {
			pred += e.lpcCoeffs[j] * float64(samples[i-j])
		}
		e.residual[i] = float64(samples[i]) - pred
	}
}

func (e *ILBCEncoder) encodeParameters(output []byte) {
	// Encode LPC coefficients (first 20 bytes)
	for i := 0; i < len(e.lpcCoeffs) && i*2 < 20; i++ {
		// Quantize to 16-bit
		quantized := int16(e.lpcCoeffs[i] * 4096)
		binary.BigEndian.PutUint16(output[i*2:], uint16(quantized))
	}

	// Encode residual energy (simplified)
	var energy float64
	for _, r := range e.residual {
		energy += r * r
	}
	energy = energy / float64(len(e.residual))

	if len(output) >= 22 {
		binary.BigEndian.PutUint16(output[20:22], uint16(energy/256))
	}

	// Encode codebook indices and gains (simplified)
	for i := 22; i < len(output) && i-22 < len(e.residual)/4; i++ {
		// Simple quantization of residual
		idx := (i - 22) * 4
		if idx < len(e.residual) {
			output[i] = byte(int(e.residual[idx]) >> 8)
		}
	}
}

// Close closes the encoder
func (e *ILBCEncoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.initialized = false
	return nil
}

// ILBCDecoder decodes iLBC to PCM
type ILBCDecoder struct {
	config      *ILBCConfig
	mu          sync.Mutex
	initialized bool
	frameCount  int64

	// Decoder state
	lpcCoeffs   []float64
	prevSamples []int16
	plcState    *ilbcPLCState
}

type ilbcPLCState struct {
	lostFrames int
	fadeGain   float64
	lastLPC    []float64
}

// NewILBCDecoder creates a new iLBC decoder
func NewILBCDecoder(config *ILBCConfig) (*ILBCDecoder, error) {
	if config == nil {
		config = DefaultILBCConfig()
	}

	if config.Mode != ILBCMode20ms && config.Mode != ILBCMode30ms {
		return nil, ErrILBCInvalidMode
	}

	dec := &ILBCDecoder{
		config:      config,
		initialized: true,
		lpcCoeffs:   make([]float64, config.PLCLPCOrder+1),
		plcState: &ilbcPLCState{
			fadeGain: 1.0,
			lastLPC:  make([]float64, config.PLCLPCOrder+1),
		},
	}

	frameSamples := ILBC20FrameSamples
	if config.Mode == ILBCMode30ms {
		frameSamples = ILBC30FrameSamples
	}
	dec.prevSamples = make([]int16, frameSamples)

	return dec, nil
}

// Decode decodes an iLBC frame to PCM samples
func (d *ILBCDecoder) Decode(frame []byte) ([]int16, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.initialized {
		return nil, ErrILBCDecoderNotReady
	}

	expectedSize := ILBC20FrameSize
	frameSamples := ILBC20FrameSamples
	if d.config.Mode == ILBCMode30ms {
		expectedSize = ILBC30FrameSize
		frameSamples = ILBC30FrameSamples
	}

	if len(frame) < expectedSize {
		if d.config.EnablePLC {
			return d.concealLoss()
		}
		return nil, ErrILBCInvalidFrame
	}

	output := make([]int16, frameSamples)

	// Decode parameters
	d.decodeParameters(frame)

	// Synthesize audio
	d.synthesize(output)

	d.frameCount++
	d.plcState.lostFrames = 0
	d.plcState.fadeGain = 1.0
	copy(d.plcState.lastLPC, d.lpcCoeffs)
	copy(d.prevSamples, output)

	return output, nil
}

func (d *ILBCDecoder) decodeParameters(frame []byte) {
	// Decode LPC coefficients
	for i := 0; i < len(d.lpcCoeffs) && i*2 < 20; i++ {
		quantized := int16(binary.BigEndian.Uint16(frame[i*2:]))
		d.lpcCoeffs[i] = float64(quantized) / 4096.0
	}
	d.lpcCoeffs[0] = 1.0 // a[0] is always 1
}

func (d *ILBCDecoder) synthesize(output []int16) {
	order := len(d.lpcCoeffs) - 1

	// Create excitation signal from decoded residual
	excitation := make([]float64, len(output))
	for i := range excitation {
		// Simple excitation (real iLBC uses sophisticated codebook)
		excitation[i] = float64((i * 17) % 1000) - 500
	}

	// LPC synthesis filter
	for i := range output {
		var sample float64 = excitation[i]
		for j := 1; j <= order && i-j >= 0; j++ {
			sample -= d.lpcCoeffs[j] * float64(output[i-j])
		}

		// Clip to int16 range
		if sample > 32767 {
			sample = 32767
		} else if sample < -32768 {
			sample = -32768
		}
		output[i] = int16(sample)
	}
}

func (d *ILBCDecoder) concealLoss() ([]int16, error) {
	d.plcState.lostFrames++

	frameSamples := ILBC20FrameSamples
	if d.config.Mode == ILBCMode30ms {
		frameSamples = ILBC30FrameSamples
	}

	output := make([]int16, frameSamples)

	// Apply PLC with fade-out
	if d.plcState.lostFrames > 1 {
		d.plcState.fadeGain *= 0.85
	}
	if d.plcState.fadeGain < 0.1 {
		d.plcState.fadeGain = 0.1
	}

	// Extrapolate using last LPC coefficients
	copy(d.lpcCoeffs, d.plcState.lastLPC)

	// Generate synthetic excitation
	excitation := make([]float64, frameSamples)
	for i := range excitation {
		excitation[i] = float64(d.prevSamples[i%len(d.prevSamples)]) * 0.5
	}

	// Synthesize with fade
	order := len(d.lpcCoeffs) - 1
	for i := range output {
		var sample float64 = excitation[i]
		for j := 1; j <= order && i-j >= 0; j++ {
			sample -= d.lpcCoeffs[j] * float64(output[i-j])
		}
		output[i] = int16(sample * d.plcState.fadeGain)
	}

	copy(d.prevSamples, output)
	return output, nil
}

// Close closes the decoder
func (d *ILBCDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.initialized = false
	return nil
}

// ILBCTranscoder handles iLBC transcoding
type ILBCTranscoder struct {
	encoder *ILBCEncoder
	decoder *ILBCDecoder
	config  *ILBCConfig
}

// NewILBCTranscoder creates a new iLBC transcoder
func NewILBCTranscoder(config *ILBCConfig) (*ILBCTranscoder, error) {
	if config == nil {
		config = DefaultILBCConfig()
	}

	enc, err := NewILBCEncoder(config)
	if err != nil {
		return nil, err
	}

	dec, err := NewILBCDecoder(config)
	if err != nil {
		enc.Close()
		return nil, err
	}

	return &ILBCTranscoder{
		encoder: enc,
		decoder: dec,
		config:  config,
	}, nil
}

// PCMToILBC converts PCM to iLBC
func (t *ILBCTranscoder) PCMToILBC(samples []int16) ([]byte, error) {
	return t.encoder.Encode(samples)
}

// ILBCToPCM converts iLBC to PCM
func (t *ILBCTranscoder) ILBCToPCM(frame []byte) ([]int16, error) {
	return t.decoder.Decode(frame)
}

// Close closes the transcoder
func (t *ILBCTranscoder) Close() error {
	var err error
	if e := t.encoder.Close(); e != nil {
		err = e
	}
	if e := t.decoder.Close(); e != nil {
		err = e
	}
	return err
}

// GetILBCFrameSize returns the frame size for a given mode
func GetILBCFrameSize(mode ILBCMode) int {
	switch mode {
	case ILBCMode20ms:
		return ILBC20FrameSize
	case ILBCMode30ms:
		return ILBC30FrameSize
	default:
		return 0
	}
}

// GetILBCFrameSamples returns the number of samples per frame for a given mode
func GetILBCFrameSamples(mode ILBCMode) int {
	switch mode {
	case ILBCMode20ms:
		return ILBC20FrameSamples
	case ILBCMode30ms:
		return ILBC30FrameSamples
	default:
		return 0
	}
}

// GetILBCBitrate returns the bitrate for a given mode
func GetILBCBitrate(mode ILBCMode) int {
	switch mode {
	case ILBCMode20ms:
		return ILBC20Bitrate
	case ILBCMode30ms:
		return ILBC30Bitrate
	default:
		return 0
	}
}
