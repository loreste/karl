package recording

import (
	"encoding/binary"
	"math"
)

// AudioMixer mixes audio from multiple sources
type AudioMixer struct {
	sampleRate    int
	bitsPerSample int
	channels      int
}

// NewAudioMixer creates a new audio mixer
func NewAudioMixer(sampleRate, bitsPerSample, channels int) *AudioMixer {
	return &AudioMixer{
		sampleRate:    sampleRate,
		bitsPerSample: bitsPerSample,
		channels:      channels,
	}
}

// MixMono mixes two audio streams into a mono output
func (m *AudioMixer) MixMono(caller, callee []byte) []byte {
	// Determine output length (use shorter of the two)
	length := len(caller)
	if len(callee) < length {
		length = len(callee)
	}

	// Ensure even length for 16-bit samples
	if m.bitsPerSample == 16 {
		length = (length / 2) * 2
	}

	output := make([]byte, length)

	if m.bitsPerSample == 16 {
		m.mix16bitMono(caller, callee, output)
	} else {
		m.mix8bitMono(caller, callee, output)
	}

	return output
}

// mix16bitMono mixes 16-bit audio to mono
func (m *AudioMixer) mix16bitMono(caller, callee, output []byte) {
	samples := len(output) / 2

	for i := 0; i < samples; i++ {
		offset := i * 2

		// Get samples (little-endian signed 16-bit)
		var callerSample, calleeSample int16
		if offset+1 < len(caller) {
			callerSample = int16(binary.LittleEndian.Uint16(caller[offset:]))
		}
		if offset+1 < len(callee) {
			calleeSample = int16(binary.LittleEndian.Uint16(callee[offset:]))
		}

		// Mix with headroom to prevent clipping
		mixed := (int32(callerSample) + int32(calleeSample)) / 2

		// Clamp to int16 range
		if mixed > math.MaxInt16 {
			mixed = math.MaxInt16
		} else if mixed < math.MinInt16 {
			mixed = math.MinInt16
		}

		binary.LittleEndian.PutUint16(output[offset:], uint16(int16(mixed)))
	}
}

// mix8bitMono mixes 8-bit audio to mono
func (m *AudioMixer) mix8bitMono(caller, callee, output []byte) {
	for i := 0; i < len(output); i++ {
		var callerSample, calleeSample int16
		if i < len(caller) {
			callerSample = int16(caller[i]) - 128 // Convert unsigned to signed
		}
		if i < len(callee) {
			calleeSample = int16(callee[i]) - 128
		}

		mixed := (callerSample + calleeSample) / 2
		output[i] = byte(mixed + 128) // Convert back to unsigned
	}
}

// MixStereo creates stereo output with caller on left and callee on right
func (m *AudioMixer) MixStereo(caller, callee []byte) []byte {
	// For stereo, input samples map to L/R channels
	inputSamples := len(caller)
	if len(callee) < inputSamples {
		inputSamples = len(callee)
	}

	if m.bitsPerSample == 16 {
		inputSamples = (inputSamples / 2) * 2
	}

	// Output is twice the size (L + R)
	output := make([]byte, inputSamples*2)

	if m.bitsPerSample == 16 {
		m.mix16bitStereo(caller, callee, output)
	} else {
		m.mix8bitStereo(caller, callee, output)
	}

	return output
}

// mix16bitStereo creates 16-bit stereo output
func (m *AudioMixer) mix16bitStereo(caller, callee, output []byte) {
	inputSamples := len(output) / 4 // Each stereo sample is 4 bytes

	for i := 0; i < inputSamples; i++ {
		inputOffset := i * 2
		outputOffset := i * 4

		// Left channel (caller)
		if inputOffset+1 < len(caller) {
			output[outputOffset] = caller[inputOffset]
			output[outputOffset+1] = caller[inputOffset+1]
		}

		// Right channel (callee)
		if inputOffset+1 < len(callee) {
			output[outputOffset+2] = callee[inputOffset]
			output[outputOffset+3] = callee[inputOffset+1]
		}
	}
}

// mix8bitStereo creates 8-bit stereo output
func (m *AudioMixer) mix8bitStereo(caller, callee, output []byte) {
	inputSamples := len(output) / 2

	for i := 0; i < inputSamples; i++ {
		outputOffset := i * 2

		// Left channel (caller)
		if i < len(caller) {
			output[outputOffset] = caller[i]
		} else {
			output[outputOffset] = 128 // Silence
		}

		// Right channel (callee)
		if i < len(callee) {
			output[outputOffset+1] = callee[i]
		} else {
			output[outputOffset+1] = 128 // Silence
		}
	}
}

// ConvertG711uToPCM converts G.711 u-law to 16-bit PCM
func ConvertG711uToPCM(data []byte) []byte {
	output := make([]byte, len(data)*2)

	for i, sample := range data {
		pcm := ulawToLinear(sample)
		binary.LittleEndian.PutUint16(output[i*2:], uint16(pcm))
	}

	return output
}

// ConvertG711aToPCM converts G.711 a-law to 16-bit PCM
func ConvertG711aToPCM(data []byte) []byte {
	output := make([]byte, len(data)*2)

	for i, sample := range data {
		pcm := alawToLinear(sample)
		binary.LittleEndian.PutUint16(output[i*2:], uint16(pcm))
	}

	return output
}

// ulawToLinear converts u-law sample to linear PCM
func ulawToLinear(sample byte) int16 {
	// u-law decoding table
	ulaw := ^sample
	sign := (ulaw & 0x80) >> 7
	exponent := (ulaw >> 4) & 0x07
	mantissa := ulaw & 0x0F

	magnitude := ((int16(mantissa) << 3) + 0x84) << exponent
	magnitude -= 0x84

	if sign == 0 {
		return magnitude
	}
	return -magnitude
}

// alawToLinear converts a-law sample to linear PCM
func alawToLinear(sample byte) int16 {
	sample ^= 0x55
	sign := sample & 0x80
	exponent := (sample >> 4) & 0x07
	mantissa := sample & 0x0F

	var magnitude int16
	if exponent > 0 {
		magnitude = (int16(mantissa) + 16) << (exponent + 3)
	} else {
		magnitude = (int16(mantissa) << 4) + 8
	}

	if sign != 0 {
		return magnitude
	}
	return -magnitude
}

// ApplyGain applies gain to audio samples
func ApplyGain(data []byte, gain float64, bitsPerSample int) {
	if gain == 1.0 {
		return
	}

	if bitsPerSample == 16 {
		for i := 0; i < len(data)-1; i += 2 {
			sample := int16(binary.LittleEndian.Uint16(data[i:]))
			adjusted := float64(sample) * gain

			// Clamp
			if adjusted > math.MaxInt16 {
				adjusted = math.MaxInt16
			} else if adjusted < math.MinInt16 {
				adjusted = math.MinInt16
			}

			binary.LittleEndian.PutUint16(data[i:], uint16(int16(adjusted)))
		}
	} else {
		for i := 0; i < len(data); i++ {
			sample := int16(data[i]) - 128
			adjusted := float64(sample) * gain

			if adjusted > 127 {
				adjusted = 127
			} else if adjusted < -128 {
				adjusted = -128
			}

			data[i] = byte(int16(adjusted) + 128)
		}
	}
}

// Normalize normalizes audio to target peak level
func Normalize(data []byte, targetPeak float64, bitsPerSample int) {
	// Find current peak
	var maxSample float64

	if bitsPerSample == 16 {
		for i := 0; i < len(data)-1; i += 2 {
			sample := math.Abs(float64(int16(binary.LittleEndian.Uint16(data[i:]))))
			if sample > maxSample {
				maxSample = sample
			}
		}
		if maxSample > 0 {
			gain := (targetPeak * math.MaxInt16) / maxSample
			ApplyGain(data, gain, bitsPerSample)
		}
	} else {
		for i := 0; i < len(data); i++ {
			sample := math.Abs(float64(int16(data[i]) - 128))
			if sample > maxSample {
				maxSample = sample
			}
		}
		if maxSample > 0 {
			gain := (targetPeak * 127) / maxSample
			ApplyGain(data, gain, bitsPerSample)
		}
	}
}

// GenerateSilence generates silence of specified duration
func GenerateSilence(duration float64, sampleRate, bitsPerSample, channels int) []byte {
	samples := int(duration * float64(sampleRate))
	bytesPerSample := bitsPerSample / 8
	size := samples * bytesPerSample * channels

	data := make([]byte, size)

	// For 8-bit audio, silence is 128
	if bitsPerSample == 8 {
		for i := range data {
			data[i] = 128
		}
	}
	// For 16-bit, silence is 0 (already zeroed)

	return data
}
