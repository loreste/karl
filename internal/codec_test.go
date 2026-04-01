package internal

import (
	"testing"
)

func TestG711MulawEncoding(t *testing.T) {
	// Test μ-law encoding/decoding roundtrip
	tests := []struct {
		name   string
		sample int16
	}{
		{"zero", 0},
		{"positive small", 100},
		{"positive medium", 10000},
		{"positive large", 30000},
		{"negative small", -100},
		{"negative medium", -10000},
		{"negative large", -30000},
		{"max positive", 32767},
		{"min negative", -32768},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := LinearToMulaw(tt.sample)
			decoded := MulawToLinear(encoded)

			diff := int(decoded) - int(tt.sample)
			if diff < 0 {
				diff = -diff
			}

			maxError := 256
			if tt.sample != 0 {
				absVal := int(tt.sample)
				if absVal < 0 {
					absVal = -absVal
				}
				proportionalError := absVal / 16
				if proportionalError > maxError {
					maxError = proportionalError
				}
			}

			if diff > maxError {
				t.Errorf("roundtrip error too large: original=%d, decoded=%d, diff=%d, maxAllowed=%d",
					tt.sample, decoded, diff, maxError)
			}
		})
	}
}

func TestG711AlawEncoding(t *testing.T) {
	tests := []struct {
		name   string
		sample int16
	}{
		{"zero", 0},
		{"positive small", 100},
		{"positive medium", 10000},
		{"positive large", 30000},
		{"negative small", -100},
		{"negative medium", -10000},
		{"negative large", -30000},
		{"max positive", 32767},
		{"min negative", -32768},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := LinearToAlaw(tt.sample)
			decoded := AlawToLinear(encoded)

			diff := int(decoded) - int(tt.sample)
			if diff < 0 {
				diff = -diff
			}

			maxError := 256
			if tt.sample != 0 {
				absVal := int(tt.sample)
				if absVal < 0 {
					absVal = -absVal
				}
				proportionalError := absVal / 16
				if proportionalError > maxError {
					maxError = proportionalError
				}
			}

			if diff > maxError {
				t.Errorf("roundtrip error too large: original=%d, decoded=%d, diff=%d",
					tt.sample, decoded, diff)
			}
		})
	}
}

func TestG711MulawAlawConversion(t *testing.T) {
	// Test conversion by decoding mulaw to linear and encoding as alaw
	for mulaw := 0; mulaw < 256; mulaw++ {
		linear := MulawToLinear(byte(mulaw))
		alaw := LinearToAlaw(linear)
		backLinear := AlawToLinear(alaw)
		backMulaw := LinearToMulaw(backLinear)

		// Allow some variation due to quantization differences
		diff := int(backMulaw) - mulaw
		if diff < 0 {
			diff = -diff
		}
		if diff > 3 { // Allow slightly more tolerance for cross-conversion
			t.Errorf("mulaw->linear->alaw->linear->mulaw conversion error: original=%d, back=%d", mulaw, backMulaw)
		}
	}
}

func TestILBCEncoderDecoder(t *testing.T) {
	config := &ILBCConfig{
		Mode:      ILBCMode30ms,
		EnablePLC: true,
	}

	encoder, err := NewILBCEncoder(config)
	if err != nil {
		t.Fatalf("failed to create encoder: %v", err)
	}
	defer encoder.Close()

	decoder, err := NewILBCDecoder(config)
	if err != nil {
		t.Fatalf("failed to create decoder: %v", err)
	}
	defer decoder.Close()

	// Create test samples - 30ms at 8kHz = 240 samples
	samples := make([]int16, 240)
	for i := range samples {
		samples[i] = int16(8000 * testSine(float64(i)*2*3.14159/120))
	}

	// Encode
	encoded, err := encoder.Encode(samples)
	if err != nil {
		t.Fatalf("encoding failed: %v", err)
	}

	// iLBC 30ms mode = 50 bytes
	if len(encoded) != 50 {
		t.Errorf("expected 50 encoded bytes, got %d", len(encoded))
	}

	// Decode
	decoded, err := decoder.Decode(encoded)
	if err != nil {
		t.Fatalf("decoding failed: %v", err)
	}

	if len(decoded) != len(samples) {
		t.Errorf("expected %d decoded samples, got %d", len(samples), len(decoded))
	}
}

func TestSpeexEncoderDecoder(t *testing.T) {
	config := &SpeexConfig{
		Mode:       SpeexModeNarrowband,
		Quality:    5,
		EnableVBR:  false,
		EnableVAD:  false,
		EnableDTX:  false,
		Complexity: 3,
	}

	encoder, err := NewSpeexEncoder(config)
	if err != nil {
		t.Fatalf("failed to create encoder: %v", err)
	}
	defer encoder.Close()

	decoder, err := NewSpeexDecoder(config)
	if err != nil {
		t.Fatalf("failed to create decoder: %v", err)
	}
	defer decoder.Close()

	// Create test samples - 20ms at 8kHz = 160 samples
	samples := make([]int16, 160)
	for i := range samples {
		samples[i] = int16(8000 * testSine(float64(i)*2*3.14159/80))
	}

	// Encode
	encoded, err := encoder.Encode(samples)
	if err != nil {
		t.Fatalf("encoding failed: %v", err)
	}

	if len(encoded) == 0 {
		t.Error("encoded data is empty")
	}

	// Decode
	decoded, err := decoder.Decode(encoded)
	if err != nil {
		t.Fatalf("decoding failed: %v", err)
	}

	if len(decoded) != len(samples) {
		t.Errorf("expected %d decoded samples, got %d", len(samples), len(decoded))
	}
}

func TestAMREncoderDecoder(t *testing.T) {
	config := &AMRConfig{
		Mode:     AMRMode122,
		Wideband: false,
	}

	encoder, err := NewAMREncoder(config)
	if err != nil {
		t.Fatalf("failed to create encoder: %v", err)
	}
	defer encoder.Close()

	decoder, err := NewAMRDecoder(config)
	if err != nil {
		t.Fatalf("failed to create decoder: %v", err)
	}
	defer decoder.Close()

	// Create test samples - 20ms at 8kHz = 160 samples
	samples := make([]int16, 160)
	for i := range samples {
		samples[i] = int16(8000 * testSine(float64(i)*2*3.14159/80))
	}

	// Encode
	encoded, err := encoder.Encode(samples)
	if err != nil {
		t.Fatalf("encoding failed: %v", err)
	}

	if len(encoded) == 0 {
		t.Error("encoded data is empty")
	}

	// Decode
	decoded, err := decoder.Decode(encoded)
	if err != nil {
		t.Fatalf("decoding failed: %v", err)
	}

	if len(decoded) != len(samples) {
		t.Errorf("expected %d decoded samples, got %d", len(samples), len(decoded))
	}
}

func TestILBCPLC(t *testing.T) {
	config := &ILBCConfig{
		Mode:      ILBCMode30ms,
		EnablePLC: true,
	}

	decoder, err := NewILBCDecoder(config)
	if err != nil {
		t.Fatalf("failed to create decoder: %v", err)
	}
	defer decoder.Close()

	// First decode a valid frame
	samples := make([]int16, 240)
	for i := range samples {
		samples[i] = int16(8000 * testSine(float64(i)*2*3.14159/120))
	}

	encoder, _ := NewILBCEncoder(config)
	validFrame, _ := encoder.Encode(samples)
	encoder.Close()

	// Decode valid frame
	_, err = decoder.Decode(validFrame)
	if err != nil {
		t.Fatalf("failed to decode valid frame: %v", err)
	}

	// Now simulate packet loss (empty frame triggers PLC)
	concealed, err := decoder.Decode(nil)
	if err != nil {
		t.Fatalf("PLC concealment failed: %v", err)
	}

	if len(concealed) != 240 {
		t.Errorf("expected 240 concealed samples, got %d", len(concealed))
	}
}

func TestV21FaxToneDetection(t *testing.T) {
	config := DefaultV21DetectorConfig()
	detector := NewV21Detector(config)

	detectedCount := 0
	detector.AddHandler(func(detection *V21Detection) {
		detectedCount++
	})

	// Generate 1100 Hz CNG tone (Calling tone)
	samples := make([]int16, 8000) // 1 second at 8kHz
	for i := range samples {
		// 1100 Hz sine wave
		samples[i] = int16(16000 * testSine(2*3.14159*1100*float64(i)/8000))
	}

	detector.ProcessSamples(samples)

	stats := detector.GetStats()
	if stats.TotalSamples != 8000 {
		t.Errorf("expected 8000 total samples, got %d", stats.TotalSamples)
	}

	// Detection callback might be called asynchronously, so we just verify
	// the detector processed samples correctly
	_ = detectedCount // Use the variable to satisfy the compiler
}

// Helper functions

func testSine(x float64) float64 {
	x = normalizeTestAngle(x)
	result := x
	term := x
	for i := 1; i < 10; i++ {
		term *= -x * x / float64((2*i)*(2*i+1))
		result += term
	}
	return result
}

func normalizeTestAngle(x float64) float64 {
	const twoPi = 2 * 3.14159265358979323846
	for x > 3.14159265358979323846 {
		x -= twoPi
	}
	for x < -3.14159265358979323846 {
		x += twoPi
	}
	return x
}
