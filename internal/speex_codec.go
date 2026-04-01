package internal

import (
	"encoding/binary"
	"errors"
	"math"
	"sync"
)

// Speex codec constants
const (
	// Narrowband (8 kHz)
	SpeexNBSampleRate   = 8000
	SpeexNBFrameSamples = 160 // 20ms frames

	// Wideband (16 kHz)
	SpeexWBSampleRate   = 16000
	SpeexWBFrameSamples = 320

	// Ultra-wideband (32 kHz)
	SpeexUWBSampleRate   = 32000
	SpeexUWBFrameSamples = 640
)

// Speex mode definitions
type SpeexMode int

const (
	SpeexModeNarrowband SpeexMode = iota
	SpeexModeWideband
	SpeexModeUltraWideband
)

// Speex quality settings (0-10)
type SpeexQuality int

const (
	SpeexQuality0 SpeexQuality = iota // ~2.15 kbps
	SpeexQuality1                      // ~3.95 kbps
	SpeexQuality2                      // ~5.95 kbps
	SpeexQuality3                      // ~8.00 kbps
	SpeexQuality4                      // ~11.0 kbps
	SpeexQuality5                      // ~15.0 kbps
	SpeexQuality6                      // ~18.2 kbps
	SpeexQuality7                      // ~24.6 kbps
	SpeexQuality8                      // ~34.2 kbps
	SpeexQuality9                      // ~42.2 kbps
	SpeexQuality10                     // ~42.2 kbps (same as 9)
)

// Approximate frame sizes for different quality levels (narrowband)
var speexNBFrameSizes = map[SpeexQuality]int{
	SpeexQuality0:  6,
	SpeexQuality1:  10,
	SpeexQuality2:  15,
	SpeexQuality3:  20,
	SpeexQuality4:  28,
	SpeexQuality5:  38,
	SpeexQuality6:  46,
	SpeexQuality7:  62,
	SpeexQuality8:  86,
	SpeexQuality9:  106,
	SpeexQuality10: 106,
}

// Speex errors
var (
	ErrSpeexInvalidFrame    = errors.New("invalid Speex frame")
	ErrSpeexDecoderNotReady = errors.New("Speex decoder not initialized")
	ErrSpeexEncoderNotReady = errors.New("Speex encoder not initialized")
	ErrSpeexInvalidMode     = errors.New("invalid Speex mode")
)

// SpeexConfig configures the Speex codec
type SpeexConfig struct {
	// Mode is the Speex mode (narrowband, wideband, ultra-wideband)
	Mode SpeexMode
	// Quality is the encoding quality (0-10)
	Quality SpeexQuality
	// EnableVBR enables Variable Bit Rate
	EnableVBR bool
	// EnableVAD enables Voice Activity Detection
	EnableVAD bool
	// EnableDTX enables Discontinuous Transmission
	EnableDTX bool
	// Complexity is the encoding complexity (1-10)
	Complexity int
	// EnableAGC enables Automatic Gain Control
	EnableAGC bool
	// EnableDenoising enables noise suppression
	EnableDenoising bool
	// EnablePLC enables Packet Loss Concealment
	EnablePLC bool
}

// DefaultSpeexConfig returns default Speex configuration
func DefaultSpeexConfig() *SpeexConfig {
	return &SpeexConfig{
		Mode:            SpeexModeNarrowband,
		Quality:         SpeexQuality8,
		EnableVBR:       true,
		EnableVAD:       true,
		EnableDTX:       true,
		Complexity:      5,
		EnableAGC:       false,
		EnableDenoising: true,
		EnablePLC:       true,
	}
}

// SpeexEncoder encodes PCM to Speex
type SpeexEncoder struct {
	config      *SpeexConfig
	mu          sync.Mutex
	initialized bool
	frameCount  int64

	// Encoder state
	prevSamples []float64
	lpcCoeffs   []float64
	excitation  []float64
	vadState    *speexVADState
}

type speexVADState struct {
	energy       float64
	noiseLevel   float64
	speechProb   float64
	hangover     int
	activeFrames int
}

// NewSpeexEncoder creates a new Speex encoder
func NewSpeexEncoder(config *SpeexConfig) (*SpeexEncoder, error) {
	if config == nil {
		config = DefaultSpeexConfig()
	}

	enc := &SpeexEncoder{
		config:      config,
		initialized: true,
		lpcCoeffs:   make([]float64, 11), // LPC order 10
		vadState: &speexVADState{
			noiseLevel: 0.01,
		},
	}

	frameSamples := enc.getFrameSamples()
	enc.prevSamples = make([]float64, frameSamples)
	enc.excitation = make([]float64, frameSamples)

	return enc, nil
}

func (e *SpeexEncoder) getFrameSamples() int {
	switch e.config.Mode {
	case SpeexModeWideband:
		return SpeexWBFrameSamples
	case SpeexModeUltraWideband:
		return SpeexUWBFrameSamples
	default:
		return SpeexNBFrameSamples
	}
}

// Encode encodes PCM samples to Speex
func (e *SpeexEncoder) Encode(samples []int16) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.initialized {
		return nil, ErrSpeexEncoderNotReady
	}

	expectedSamples := e.getFrameSamples()
	if len(samples) != expectedSamples {
		return nil, ErrSpeexInvalidFrame
	}

	// Convert to float
	floatSamples := make([]float64, len(samples))
	for i, s := range samples {
		floatSamples[i] = float64(s) / 32768.0
	}

	// VAD analysis
	isSpeech := e.vadAnalysis(floatSamples)

	// If DTX is enabled and this is silence, encode minimal frame
	if e.config.EnableDTX && !isSpeech {
		return e.encodeSilenceFrame(), nil
	}

	// Estimate frame size based on quality
	frameSize := speexNBFrameSizes[e.config.Quality]
	if e.config.Mode == SpeexModeWideband {
		frameSize = frameSize * 2
	} else if e.config.Mode == SpeexModeUltraWideband {
		frameSize = frameSize * 4
	}

	output := make([]byte, frameSize)

	// Encode frame
	e.encodeFrame(floatSamples, output)

	e.frameCount++
	copy(e.prevSamples, floatSamples)

	return output, nil
}

func (e *SpeexEncoder) vadAnalysis(samples []float64) bool {
	// Calculate frame energy
	var energy float64
	for _, s := range samples {
		energy += s * s
	}
	energy /= float64(len(samples))

	// Update noise estimate (simplified)
	if energy < e.vadState.noiseLevel*2 {
		e.vadState.noiseLevel = e.vadState.noiseLevel*0.99 + energy*0.01
	}

	// Calculate speech probability
	snr := 10.0 * math.Log10(energy/e.vadState.noiseLevel)
	e.vadState.speechProb = 1.0 / (1.0 + math.Exp(-0.5*(snr-10)))

	// Decision with hangover
	isSpeech := e.vadState.speechProb > 0.5
	if isSpeech {
		e.vadState.hangover = 10 // 200ms hangover
		e.vadState.activeFrames++
	} else if e.vadState.hangover > 0 {
		e.vadState.hangover--
		isSpeech = true
	}

	e.vadState.energy = energy
	return isSpeech
}

func (e *SpeexEncoder) encodeSilenceFrame() []byte {
	// Minimal silence frame (SID-like)
	return []byte{0x00, 0x00, 0x00, 0x00}
}

func (e *SpeexEncoder) encodeFrame(samples []float64, output []byte) {
	// LPC analysis
	e.computeLPC(samples)

	// Compute excitation
	e.computeExcitation(samples)

	// Encode LPC coefficients (LSP quantization simplified)
	if len(output) >= 10 {
		for i := 0; i < 5 && i < len(e.lpcCoeffs)-1; i++ {
			quantized := int16(e.lpcCoeffs[i+1] * 16384)
			binary.BigEndian.PutUint16(output[i*2:], uint16(quantized))
		}
	}

	// Encode excitation parameters
	if len(output) >= 14 {
		// Pitch and gain (simplified)
		pitch := e.estimatePitch(samples)
		binary.BigEndian.PutUint16(output[10:12], uint16(pitch))

		gain := e.computeGain(samples)
		binary.BigEndian.PutUint16(output[12:14], uint16(gain*1000))
	}

	// Fill remaining with codebook indices (simplified)
	for i := 14; i < len(output); i++ {
		if i-14 < len(e.excitation) {
			output[i] = byte(int(e.excitation[i-14]*127) & 0xFF)
		}
	}
}

func (e *SpeexEncoder) computeLPC(samples []float64) {
	n := len(samples)
	order := len(e.lpcCoeffs) - 1

	// Autocorrelation
	r := make([]float64, order+1)
	for i := 0; i <= order; i++ {
		for j := 0; j < n-i; j++ {
			r[i] += samples[j] * samples[j+i]
		}
	}

	// Levinson-Durbin
	if r[0] > 1e-10 {
		e.lpcCoeffs[0] = 1.0
		predError := r[0]
		for i := 1; i <= order; i++ {
			lambda := 0.0
			for j := 0; j < i; j++ {
				lambda -= e.lpcCoeffs[j] * r[i-j]
			}
			lambda /= predError

			// Update coefficients
			for j := 1; j < i; j++ {
				e.lpcCoeffs[j] += lambda * e.lpcCoeffs[i-j]
			}
			e.lpcCoeffs[i] = lambda
			predError *= (1 - lambda*lambda)
		}
	}
}

func (e *SpeexEncoder) computeExcitation(samples []float64) {
	order := len(e.lpcCoeffs) - 1
	for i := range samples {
		pred := 0.0
		for j := 1; j <= order && i-j >= 0; j++ {
			pred += e.lpcCoeffs[j] * samples[i-j]
		}
		e.excitation[i] = samples[i] - pred
	}
}

func (e *SpeexEncoder) estimatePitch(samples []float64) int {
	// Simplified autocorrelation-based pitch estimation
	minPitch := 20  // ~400 Hz
	maxPitch := 150 // ~53 Hz

	maxCorr := 0.0
	bestPitch := minPitch

	for lag := minPitch; lag <= maxPitch && lag < len(samples)/2; lag++ {
		corr := 0.0
		for i := 0; i < len(samples)-lag; i++ {
			corr += samples[i] * samples[i+lag]
		}
		if corr > maxCorr {
			maxCorr = corr
			bestPitch = lag
		}
	}

	return bestPitch
}

func (e *SpeexEncoder) computeGain(samples []float64) float64 {
	var energy float64
	for _, s := range samples {
		energy += s * s
	}
	return math.Sqrt(energy / float64(len(samples)))
}

// Close closes the encoder
func (e *SpeexEncoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.initialized = false
	return nil
}

// SpeexDecoder decodes Speex to PCM
type SpeexDecoder struct {
	config      *SpeexConfig
	mu          sync.Mutex
	initialized bool
	frameCount  int64

	// Decoder state
	prevSamples []float64
	lpcCoeffs   []float64
	plcState    *speexPLCState
}

type speexPLCState struct {
	lostFrames int
	fadeGain   float64
	lastPitch  int
	lastGain   float64
	lastLPC    []float64
}

// NewSpeexDecoder creates a new Speex decoder
func NewSpeexDecoder(config *SpeexConfig) (*SpeexDecoder, error) {
	if config == nil {
		config = DefaultSpeexConfig()
	}

	dec := &SpeexDecoder{
		config:      config,
		initialized: true,
		lpcCoeffs:   make([]float64, 11),
		plcState: &speexPLCState{
			fadeGain: 1.0,
			lastLPC:  make([]float64, 11),
		},
	}

	frameSamples := dec.getFrameSamples()
	dec.prevSamples = make([]float64, frameSamples)

	return dec, nil
}

func (d *SpeexDecoder) getFrameSamples() int {
	switch d.config.Mode {
	case SpeexModeWideband:
		return SpeexWBFrameSamples
	case SpeexModeUltraWideband:
		return SpeexUWBFrameSamples
	default:
		return SpeexNBFrameSamples
	}
}

// Decode decodes a Speex frame to PCM samples
func (d *SpeexDecoder) Decode(frame []byte) ([]int16, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.initialized {
		return nil, ErrSpeexDecoderNotReady
	}

	frameSamples := d.getFrameSamples()

	// Check for silence frame
	if len(frame) <= 4 && isZeroFrame(frame) {
		return d.decodeSilence()
	}

	if len(frame) < 14 {
		if d.config.EnablePLC {
			return d.concealLoss()
		}
		return nil, ErrSpeexInvalidFrame
	}

	// Decode parameters
	d.decodeParameters(frame)

	// Synthesize audio
	floatOutput := make([]float64, frameSamples)
	d.synthesize(frame, floatOutput)

	// Convert to int16
	output := make([]int16, frameSamples)
	for i, s := range floatOutput {
		s = s * 32768.0
		if s > 32767 {
			s = 32767
		} else if s < -32768 {
			s = -32768
		}
		output[i] = int16(s)
	}

	d.frameCount++
	d.plcState.lostFrames = 0
	d.plcState.fadeGain = 1.0
	copy(d.plcState.lastLPC, d.lpcCoeffs)
	copy(d.prevSamples, floatOutput)

	return output, nil
}

func isZeroFrame(frame []byte) bool {
	for _, b := range frame {
		if b != 0 {
			return false
		}
	}
	return true
}

func (d *SpeexDecoder) decodeSilence() ([]int16, error) {
	frameSamples := d.getFrameSamples()
	output := make([]int16, frameSamples)

	// Generate comfort noise
	for i := range output {
		output[i] = int16((i * 13) % 100) - 50
	}

	return output, nil
}

func (d *SpeexDecoder) decodeParameters(frame []byte) {
	// Decode LPC coefficients
	d.lpcCoeffs[0] = 1.0
	for i := 0; i < 5 && i*2+2 <= len(frame); i++ {
		quantized := int16(binary.BigEndian.Uint16(frame[i*2:]))
		d.lpcCoeffs[i+1] = float64(quantized) / 16384.0
	}

	// Decode pitch and gain
	if len(frame) >= 14 {
		d.plcState.lastPitch = int(binary.BigEndian.Uint16(frame[10:12]))
		d.plcState.lastGain = float64(binary.BigEndian.Uint16(frame[12:14])) / 1000.0
	}
}

func (d *SpeexDecoder) synthesize(frame []byte, output []float64) {
	order := len(d.lpcCoeffs) - 1

	// Build excitation from coded data
	excitation := make([]float64, len(output))
	for i := 0; i < len(output) && i+14 < len(frame); i++ {
		excitation[i] = float64(int8(frame[14+i])) / 127.0
	}

	// Scale excitation by gain
	gain := d.plcState.lastGain
	if gain < 0.001 {
		gain = 0.001
	}
	for i := range excitation {
		excitation[i] *= gain
	}

	// LPC synthesis
	for i := range output {
		sample := excitation[i]
		for j := 1; j <= order && i-j >= 0; j++ {
			sample -= d.lpcCoeffs[j] * output[i-j]
		}
		output[i] = sample
	}
}

func (d *SpeexDecoder) concealLoss() ([]int16, error) {
	d.plcState.lostFrames++
	frameSamples := d.getFrameSamples()

	// Fade gain
	if d.plcState.lostFrames > 1 {
		d.plcState.fadeGain *= 0.85
	}
	if d.plcState.fadeGain < 0.1 {
		d.plcState.fadeGain = 0.1
	}

	// Generate concealment using pitch repetition
	floatOutput := make([]float64, frameSamples)
	pitch := d.plcState.lastPitch
	if pitch < 20 {
		pitch = 80
	}

	for i := range floatOutput {
		srcIdx := i % pitch
		if srcIdx < len(d.prevSamples) {
			floatOutput[i] = d.prevSamples[srcIdx] * d.plcState.fadeGain
		}
	}

	// Convert to int16
	output := make([]int16, frameSamples)
	for i, s := range floatOutput {
		s = s * 32768.0
		if s > 32767 {
			s = 32767
		} else if s < -32768 {
			s = -32768
		}
		output[i] = int16(s)
	}

	copy(d.prevSamples, floatOutput)
	return output, nil
}

// Close closes the decoder
func (d *SpeexDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.initialized = false
	return nil
}

// SpeexTranscoder handles Speex transcoding
type SpeexTranscoder struct {
	encoder *SpeexEncoder
	decoder *SpeexDecoder
	config  *SpeexConfig
}

// NewSpeexTranscoder creates a new Speex transcoder
func NewSpeexTranscoder(config *SpeexConfig) (*SpeexTranscoder, error) {
	if config == nil {
		config = DefaultSpeexConfig()
	}

	enc, err := NewSpeexEncoder(config)
	if err != nil {
		return nil, err
	}

	dec, err := NewSpeexDecoder(config)
	if err != nil {
		enc.Close()
		return nil, err
	}

	return &SpeexTranscoder{
		encoder: enc,
		decoder: dec,
		config:  config,
	}, nil
}

// PCMToSpeex converts PCM to Speex
func (t *SpeexTranscoder) PCMToSpeex(samples []int16) ([]byte, error) {
	return t.encoder.Encode(samples)
}

// SpeexToPCM converts Speex to PCM
func (t *SpeexTranscoder) SpeexToPCM(frame []byte) ([]int16, error) {
	return t.decoder.Decode(frame)
}

// Close closes the transcoder
func (t *SpeexTranscoder) Close() error {
	var err error
	if e := t.encoder.Close(); e != nil {
		err = e
	}
	if e := t.decoder.Close(); e != nil {
		err = e
	}
	return err
}

// GetSpeexSampleRate returns the sample rate for a given mode
func GetSpeexSampleRate(mode SpeexMode) int {
	switch mode {
	case SpeexModeWideband:
		return SpeexWBSampleRate
	case SpeexModeUltraWideband:
		return SpeexUWBSampleRate
	default:
		return SpeexNBSampleRate
	}
}

// GetSpeexFrameSamples returns the number of samples per frame for a given mode
func GetSpeexFrameSamples(mode SpeexMode) int {
	switch mode {
	case SpeexModeWideband:
		return SpeexWBFrameSamples
	case SpeexModeUltraWideband:
		return SpeexUWBFrameSamples
	default:
		return SpeexNBFrameSamples
	}
}
