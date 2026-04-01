package internal

import (
	"math"
	"sync"
	"time"
)

// V21DetectorConfig configures the V.21 fax tone detector
type V21DetectorConfig struct {
	// SampleRate is the audio sample rate (typically 8000 Hz)
	SampleRate int
	// MinDuration is the minimum tone duration to trigger detection
	MinDuration time.Duration
	// Threshold is the energy threshold for tone detection
	Threshold float64
	// GoertzelN is the number of samples for Goertzel algorithm
	GoertzelN int
	// EnableCNG enables Calling Tone (CNG) detection at 1100 Hz
	EnableCNG bool
	// EnableCED enables Called Station Identification (CED) detection at 2100 Hz
	EnableCED bool
	// EnableV21Channel1 enables V.21 channel 1 (980/1180 Hz)
	EnableV21Channel1 bool
	// EnableV21Channel2 enables V.21 channel 2 (1650/1850 Hz)
	EnableV21Channel2 bool
}

// DefaultV21DetectorConfig returns default configuration
func DefaultV21DetectorConfig() *V21DetectorConfig {
	return &V21DetectorConfig{
		SampleRate:        8000,
		MinDuration:       300 * time.Millisecond,
		Threshold:         0.3,
		GoertzelN:         205, // ~25.6ms at 8000 Hz
		EnableCNG:         true,
		EnableCED:         true,
		EnableV21Channel1: true,
		EnableV21Channel2: true,
	}
}

// V21ToneType represents the type of detected tone
type V21ToneType int

const (
	V21ToneNone V21ToneType = iota
	V21ToneCNG              // 1100 Hz Calling Tone
	V21ToneCED              // 2100 Hz Called Station ID
	V21ToneChannel1Mark     // 980 Hz V.21 Channel 1 Mark
	V21ToneChannel1Space    // 1180 Hz V.21 Channel 1 Space
	V21ToneChannel2Mark     // 1650 Hz V.21 Channel 2 Mark
	V21ToneChannel2Space    // 1850 Hz V.21 Channel 2 Space
	V21TonePreamble         // V.21 preamble detected
)

func (t V21ToneType) String() string {
	switch t {
	case V21ToneNone:
		return "none"
	case V21ToneCNG:
		return "CNG"
	case V21ToneCED:
		return "CED"
	case V21ToneChannel1Mark:
		return "V21_CH1_MARK"
	case V21ToneChannel1Space:
		return "V21_CH1_SPACE"
	case V21ToneChannel2Mark:
		return "V21_CH2_MARK"
	case V21ToneChannel2Space:
		return "V21_CH2_SPACE"
	case V21TonePreamble:
		return "V21_PREAMBLE"
	default:
		return "unknown"
	}
}

// V21Detection represents a detected V.21 tone
type V21Detection struct {
	Type       V21ToneType
	Timestamp  time.Time
	Duration   time.Duration
	Frequency  float64
	Confidence float64
}

// V21DetectionHandler is called when a V.21 tone is detected
type V21DetectionHandler func(detection *V21Detection)

// V21Detector detects V.21 fax tones in audio streams
type V21Detector struct {
	config   *V21DetectorConfig
	handlers []V21DetectionHandler

	mu              sync.Mutex
	goertzelFilters map[float64]*GoertzelFilter
	toneStates      map[V21ToneType]*toneState
	sampleBuffer    []float64
	bufferPos       int
	totalSamples    int64

	// Detection state
	currentTone    V21ToneType
	toneStartTime  time.Time
	toneStartSample int64
}

type toneState struct {
	detecting   bool
	startTime   time.Time
	startSample int64
	energy      float64
}

// GoertzelFilter implements the Goertzel algorithm for single-frequency detection
type GoertzelFilter struct {
	targetFreq float64
	sampleRate int
	n          int
	coeff      float64
	s0, s1, s2 float64
	sampleCount int
}

// NewGoertzelFilter creates a new Goertzel filter for a specific frequency
func NewGoertzelFilter(targetFreq float64, sampleRate, n int) *GoertzelFilter {
	k := float64(n) * targetFreq / float64(sampleRate)
	coeff := 2 * math.Cos(2*math.Pi*k/float64(n))

	return &GoertzelFilter{
		targetFreq: targetFreq,
		sampleRate: sampleRate,
		n:          n,
		coeff:      coeff,
	}
}

// Process processes a single sample and returns true if N samples have been processed
func (gf *GoertzelFilter) Process(sample float64) bool {
	gf.s0 = sample + gf.coeff*gf.s1 - gf.s2
	gf.s2 = gf.s1
	gf.s1 = gf.s0
	gf.sampleCount++

	if gf.sampleCount >= gf.n {
		return true
	}
	return false
}

// GetMagnitude returns the magnitude of the target frequency
func (gf *GoertzelFilter) GetMagnitude() float64 {
	// Compute magnitude squared
	magSq := gf.s1*gf.s1 + gf.s2*gf.s2 - gf.coeff*gf.s1*gf.s2
	return math.Sqrt(magSq) / float64(gf.n)
}

// Reset resets the filter state
func (gf *GoertzelFilter) Reset() {
	gf.s0 = 0
	gf.s1 = 0
	gf.s2 = 0
	gf.sampleCount = 0
}

// NewV21Detector creates a new V.21 tone detector
func NewV21Detector(config *V21DetectorConfig) *V21Detector {
	if config == nil {
		config = DefaultV21DetectorConfig()
	}

	d := &V21Detector{
		config:          config,
		handlers:        make([]V21DetectionHandler, 0),
		goertzelFilters: make(map[float64]*GoertzelFilter),
		toneStates:      make(map[V21ToneType]*toneState),
		sampleBuffer:    make([]float64, config.GoertzelN),
	}

	// Initialize Goertzel filters for each frequency of interest
	frequencies := []float64{}

	if config.EnableCNG {
		frequencies = append(frequencies, 1100) // CNG tone
	}
	if config.EnableCED {
		frequencies = append(frequencies, 2100) // CED tone
	}
	if config.EnableV21Channel1 {
		frequencies = append(frequencies, 980, 1180) // V.21 channel 1
	}
	if config.EnableV21Channel2 {
		frequencies = append(frequencies, 1650, 1850) // V.21 channel 2
	}

	for _, freq := range frequencies {
		d.goertzelFilters[freq] = NewGoertzelFilter(freq, config.SampleRate, config.GoertzelN)
	}

	// Initialize tone states
	toneTypes := []V21ToneType{
		V21ToneCNG, V21ToneCED,
		V21ToneChannel1Mark, V21ToneChannel1Space,
		V21ToneChannel2Mark, V21ToneChannel2Space,
	}
	for _, tt := range toneTypes {
		d.toneStates[tt] = &toneState{}
	}

	return d
}

// AddHandler adds a detection handler
func (d *V21Detector) AddHandler(handler V21DetectionHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers = append(d.handlers, handler)
}

// ProcessSamples processes audio samples (16-bit linear PCM)
func (d *V21Detector) ProcessSamples(samples []int16) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, sample := range samples {
		// Normalize to [-1, 1]
		normalized := float64(sample) / 32768.0
		d.processSample(normalized)
	}
}

// ProcessFloat32 processes float32 audio samples
func (d *V21Detector) ProcessFloat32(samples []float32) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, sample := range samples {
		d.processSample(float64(sample))
	}
}

func (d *V21Detector) processSample(sample float64) {
	d.totalSamples++

	// Process through all Goertzel filters
	allReady := true
	for _, filter := range d.goertzelFilters {
		if !filter.Process(sample) {
			allReady = false
		}
	}

	// If all filters have processed N samples, analyze the results
	if allReady {
		d.analyzeFilters()

		// Reset all filters
		for _, filter := range d.goertzelFilters {
			filter.Reset()
		}
	}
}

func (d *V21Detector) analyzeFilters() {
	// Get magnitudes for each frequency
	magnitudes := make(map[float64]float64)
	totalEnergy := 0.0

	for freq, filter := range d.goertzelFilters {
		mag := filter.GetMagnitude()
		magnitudes[freq] = mag
		totalEnergy += mag * mag
	}

	totalEnergy = math.Sqrt(totalEnergy)
	if totalEnergy < 0.001 {
		// Too quiet, no tones
		d.endAllTones()
		return
	}

	// Normalize magnitudes
	for freq := range magnitudes {
		magnitudes[freq] /= totalEnergy
	}

	// Detect specific tones
	now := time.Now()

	// CNG detection (1100 Hz)
	if d.config.EnableCNG {
		if mag, ok := magnitudes[1100]; ok && mag > d.config.Threshold {
			d.updateToneState(V21ToneCNG, now, 1100, mag)
		} else {
			d.endTone(V21ToneCNG)
		}
	}

	// CED detection (2100 Hz)
	if d.config.EnableCED {
		if mag, ok := magnitudes[2100]; ok && mag > d.config.Threshold {
			d.updateToneState(V21ToneCED, now, 2100, mag)
		} else {
			d.endTone(V21ToneCED)
		}
	}

	// V.21 Channel 1 detection
	if d.config.EnableV21Channel1 {
		mag980 := magnitudes[980]
		mag1180 := magnitudes[1180]

		if mag980 > d.config.Threshold && mag980 > mag1180 {
			d.updateToneState(V21ToneChannel1Mark, now, 980, mag980)
			d.endTone(V21ToneChannel1Space)
		} else if mag1180 > d.config.Threshold && mag1180 > mag980 {
			d.updateToneState(V21ToneChannel1Space, now, 1180, mag1180)
			d.endTone(V21ToneChannel1Mark)
		} else {
			d.endTone(V21ToneChannel1Mark)
			d.endTone(V21ToneChannel1Space)
		}
	}

	// V.21 Channel 2 detection
	if d.config.EnableV21Channel2 {
		mag1650 := magnitudes[1650]
		mag1850 := magnitudes[1850]

		if mag1650 > d.config.Threshold && mag1650 > mag1850 {
			d.updateToneState(V21ToneChannel2Mark, now, 1650, mag1650)
			d.endTone(V21ToneChannel2Space)
		} else if mag1850 > d.config.Threshold && mag1850 > mag1650 {
			d.updateToneState(V21ToneChannel2Space, now, 1850, mag1850)
			d.endTone(V21ToneChannel2Mark)
		} else {
			d.endTone(V21ToneChannel2Mark)
			d.endTone(V21ToneChannel2Space)
		}
	}
}

func (d *V21Detector) updateToneState(toneType V21ToneType, now time.Time, freq, mag float64) {
	state := d.toneStates[toneType]

	if !state.detecting {
		state.detecting = true
		state.startTime = now
		state.startSample = d.totalSamples
		state.energy = mag
	} else {
		// Update running average energy
		state.energy = state.energy*0.9 + mag*0.1

		// Check if tone has been present long enough
		duration := now.Sub(state.startTime)
		if duration >= d.config.MinDuration {
			d.emitDetection(&V21Detection{
				Type:       toneType,
				Timestamp:  state.startTime,
				Duration:   duration,
				Frequency:  freq,
				Confidence: state.energy,
			})
		}
	}
}

func (d *V21Detector) endTone(toneType V21ToneType) {
	state := d.toneStates[toneType]
	if state.detecting {
		state.detecting = false
	}
}

func (d *V21Detector) endAllTones() {
	for _, state := range d.toneStates {
		state.detecting = false
	}
}

func (d *V21Detector) emitDetection(detection *V21Detection) {
	for _, handler := range d.handlers {
		go handler(detection)
	}
}

// Reset resets the detector state
func (d *V21Detector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, filter := range d.goertzelFilters {
		filter.Reset()
	}

	for _, state := range d.toneStates {
		state.detecting = false
	}

	d.currentTone = V21ToneNone
	d.totalSamples = 0
}

// GetStats returns detector statistics
func (d *V21Detector) GetStats() *V21DetectorStats {
	d.mu.Lock()
	defer d.mu.Unlock()

	activeTones := make([]V21ToneType, 0)
	for toneType, state := range d.toneStates {
		if state.detecting {
			activeTones = append(activeTones, toneType)
		}
	}

	return &V21DetectorStats{
		TotalSamples: d.totalSamples,
		ActiveTones:  activeTones,
	}
}

// V21DetectorStats contains detector statistics
type V21DetectorStats struct {
	TotalSamples int64
	ActiveTones  []V21ToneType
}

// FaxToneDetector wraps V21Detector for easier integration
type FaxToneDetector struct {
	v21      *V21Detector
	callback FaxDetectionCallback
	mu       sync.Mutex
	detected bool
}

// FaxDetectionCallback is called when fax is detected
type FaxDetectionCallback func(isFax bool, toneType V21ToneType)

// NewFaxToneDetector creates a new fax tone detector
func NewFaxToneDetector(callback FaxDetectionCallback) *FaxToneDetector {
	config := DefaultV21DetectorConfig()
	config.MinDuration = 500 * time.Millisecond // Require 500ms of tone

	ftd := &FaxToneDetector{
		v21:      NewV21Detector(config),
		callback: callback,
	}

	ftd.v21.AddHandler(func(detection *V21Detection) {
		ftd.handleDetection(detection)
	})

	return ftd
}

func (ftd *FaxToneDetector) handleDetection(detection *V21Detection) {
	ftd.mu.Lock()
	defer ftd.mu.Unlock()

	// CNG or CED tone indicates fax
	if detection.Type == V21ToneCNG || detection.Type == V21ToneCED {
		if !ftd.detected {
			ftd.detected = true
			if ftd.callback != nil {
				ftd.callback(true, detection.Type)
			}
		}
	}
}

// ProcessAudio processes audio for fax detection
func (ftd *FaxToneDetector) ProcessAudio(samples []int16) {
	ftd.v21.ProcessSamples(samples)
}

// IsFaxDetected returns whether fax has been detected
func (ftd *FaxToneDetector) IsFaxDetected() bool {
	ftd.mu.Lock()
	defer ftd.mu.Unlock()
	return ftd.detected
}

// Reset resets the detector
func (ftd *FaxToneDetector) Reset() {
	ftd.mu.Lock()
	defer ftd.mu.Unlock()
	ftd.detected = false
	ftd.v21.Reset()
}

// DTMFAndFaxDetector combines DTMF and fax detection
type DTMFAndFaxDetector struct {
	faxDetector *FaxToneDetector
	mu          sync.Mutex
}

// NewDTMFAndFaxDetector creates a combined detector
func NewDTMFAndFaxDetector(faxCallback FaxDetectionCallback) *DTMFAndFaxDetector {
	return &DTMFAndFaxDetector{
		faxDetector: NewFaxToneDetector(faxCallback),
	}
}

// ProcessAudio processes audio for both DTMF and fax
func (d *DTMFAndFaxDetector) ProcessAudio(samples []int16) {
	d.faxDetector.ProcessAudio(samples)
}

// IsFaxDetected returns whether fax has been detected
func (d *DTMFAndFaxDetector) IsFaxDetected() bool {
	return d.faxDetector.IsFaxDetected()
}
