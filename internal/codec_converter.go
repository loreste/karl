package internal

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/pion/webrtc/v3"
)

const (
	vadThreshold    = -45.0 // dB threshold for voice activity
	vadFrameSize    = 160   // samples per frame for VAD
	pcmMaxAmplitude = 32767 // maximum amplitude for 16-bit PCM
)

// CodecConverter handles audio codec conversions
type CodecConverter struct {
	sampleRate int
	channels   int
	frameSize  int
}

// NewCodecConverter creates a new codec converter instance
func NewCodecConverter(sampleRate, channels, frameSize int) *CodecConverter {
	return &CodecConverter{
		sampleRate: sampleRate,
		channels:   channels,
		frameSize:  frameSize,
	}
}

// transcodeAudio handles conversion between different audio codecs
func transcodeAudio(payload []byte, inputCodec, outputCodec string) ([]byte, error) {
	switch {
	case inputCodec == webrtc.MimeTypeOpus && outputCodec == webrtc.MimeTypePCMU:
		return opusToPCMU(payload)
	case inputCodec == webrtc.MimeTypePCMU && outputCodec == webrtc.MimeTypeOpus:
		return pcmuToOpus(payload)
	case inputCodec == webrtc.MimeTypePCMA && outputCodec == webrtc.MimeTypePCMU:
		return pcmaTopcmu(payload)
	case inputCodec == webrtc.MimeTypePCMU && outputCodec == webrtc.MimeTypePCMA:
		return pcmuTopcma(payload)
	default:
		return payload, nil
	}
}

// pcmuTopcma converts G.711 μ-law to A-law
func pcmuTopcma(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}

	output := make([]byte, len(payload))
	for i, sample := range payload {
		output[i] = muLawToALawMap[sample]
	}
	return output, nil
}

// pcmaTopcmu converts G.711 A-law to μ-law
func pcmaTopcmu(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}

	output := make([]byte, len(payload))
	for i, sample := range payload {
		output[i] = aLawToMuLawMap[sample]
	}
	return output, nil
}

// opusToPCMU converts Opus to G.711 μ-law
func opusToPCMU(payload []byte) ([]byte, error) {
	pcm, err := decodeToPCM(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Opus: %v", err)
	}
	return encodePCMToPCMU(pcm)
}

// pcmuToOpus converts G.711 μ-law to Opus
func pcmuToOpus(payload []byte) ([]byte, error) {
	pcm, err := decodePCMUToPCM(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode PCM-U: %v", err)
	}
	return encodeToOpus(pcm)
}

// decodeToPCM decodes Opus to PCM
func decodeToPCM(payload []byte) ([]int16, error) {
	// This is a placeholder for actual Opus decoding
	// You would typically use an Opus decoder library here
	pcm := make([]int16, len(payload)/2)
	for i := 0; i < len(payload); i += 2 {
		pcm[i/2] = int16(binary.LittleEndian.Uint16(payload[i:]))
	}
	return pcm, nil
}

// encodeToOpus encodes PCM to Opus
func encodeToOpus(pcm []int16) ([]byte, error) {
	// This is a placeholder for actual Opus encoding
	// You would typically use an Opus encoder library here
	encoded := make([]byte, len(pcm)*2)
	for i, sample := range pcm {
		binary.LittleEndian.PutUint16(encoded[i*2:], uint16(sample))
	}
	return encoded, nil
}

// decodePCMUToPCM converts G.711 μ-law to PCM samples
func decodePCMUToPCM(payload []byte) ([]int16, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}

	pcm := make([]int16, len(payload))
	for i, mu := range payload {
		mu = ^mu // Invert all bits

		// Extract fields
		sign := (mu & 0x80) >> 7
		exponent := (mu & 0x70) >> 4
		mantissa := mu & 0x0F

		// Convert to linear
		magnitude := (int16(mantissa) << 3) + 0x84
		magnitude <<= exponent

		// Apply sign
		if sign == 1 {
			pcm[i] = -magnitude
		} else {
			pcm[i] = magnitude
		}
	}
	return pcm, nil
}

// encodePCMToPCMU converts PCM samples to G.711 μ-law
func encodePCMToPCMU(pcm []int16) ([]byte, error) {
	if len(pcm) == 0 {
		return nil, fmt.Errorf("empty PCM data")
	}

	output := make([]byte, len(pcm))
	for i, sample := range pcm {
		// Handle sign
		sign := sample < 0
		if sign {
			sample = -sample
		}

		// Add bias
		sample += 132

		// Find segment and quantize
		exponent := uint8(0)
		for sample > 15 {
			sample >>= 1
			exponent++
		}

		// Combine fields
		mantissa := uint8(sample)
		mu := (exponent << 4) | mantissa
		if sign {
			mu |= 0x80
		}

		// Invert all bits
		output[i] = ^mu
	}
	return output, nil
}

// isVoiceActive performs voice activity detection
func isVoiceActive(pcm []int16) bool {
	if len(pcm) == 0 {
		return false
	}

	// Calculate RMS energy
	var sumSquares float64
	for _, sample := range pcm {
		amplitude := float64(sample) / pcmMaxAmplitude
		sumSquares += amplitude * amplitude
	}

	rms := math.Sqrt(sumSquares / float64(len(pcm)))
	db := 20 * math.Log10(rms)

	return db > vadThreshold
}

// normalizeAudio normalizes PCM samples
func normalizeAudio(pcm []int16) []int16 {
	if len(pcm) == 0 {
		return pcm
	}

	// Find maximum amplitude
	var maxAmp int16
	for _, sample := range pcm {
		abs := sample
		if abs < 0 {
			abs = -abs
		}
		if abs > maxAmp {
			maxAmp = abs
		}
	}

	if maxAmp == 0 {
		return pcm
	}

	// Normalize if needed
	if maxAmp > pcmMaxAmplitude {
		ratio := float64(pcmMaxAmplitude) / float64(maxAmp)
		normalized := make([]int16, len(pcm))
		for i, sample := range pcm {
			normalized[i] = int16(float64(sample) * ratio)
		}
		return normalized
	}

	return pcm
}

// calculateRMS calculates Root Mean Square of PCM samples
func calculateRMS(pcm []int16) float64 {
	if len(pcm) == 0 {
		return 0
	}

	var sumSquares int64
	for _, sample := range pcm {
		sumSquares += int64(sample) * int64(sample)
	}

	return math.Sqrt(float64(sumSquares) / float64(len(pcm)))
}
