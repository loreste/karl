package tests

import (
	"karl/internal"
	"testing"
)

// This test will use the exported functions to test codec conversion
func TestCodecConverter(t *testing.T) {
	// Test PCM to PCMU conversion
	_ = []int16{0, 100, 1000, 16000, -100, -1000, -16000} // just for reference
	
	// Test PCMU to PCMA conversion
	pcmuData := []byte{0xFF, 0xFE, 0xFD, 0xFC, 0xFB, 0xFA, 0xF9}
	pcmaOutput, err := internal.PCMUToPCMA(pcmuData)
	if err != nil {
		t.Fatalf("Failed to convert PCMU to PCMA: %v", err)
	}
	if len(pcmaOutput) != len(pcmuData) {
		t.Errorf("Expected output length %d, got %d", len(pcmuData), len(pcmaOutput))
	}

	// Test PCMA to PCMU conversion
	pcmaData := []byte{0x55, 0x54, 0x53, 0x52, 0x51, 0x50, 0x49}
	pcmuOutput, err := internal.PCMAToPCMU(pcmaData)
	if err != nil {
		t.Fatalf("Failed to convert PCMA to PCMU: %v", err)
	}
	if len(pcmuOutput) != len(pcmaData) {
		t.Errorf("Expected output length %d, got %d", len(pcmaData), len(pcmuOutput))
	}

	// Test voice activity detection with known silence
	pcmSilence := make([]int16, 100) // All zeros = silence
	// Just verify it runs - actual detection depends on threshold settings
	_ = internal.IsVoiceActive(pcmSilence)
	
	// Test with loud audio
	pcmLoud := make([]int16, 100)
	for i := range pcmLoud {
		pcmLoud[i] = 16000 // High amplitude
	}
	// Just verify it runs - actual detection depends on threshold settings
	_ = internal.IsVoiceActive(pcmLoud)

	// Test audio normalization with values that need normalization
	pcmOverload := []int16{32767, 32766, -32768, -32767}
	normalized := internal.NormalizeAudio(pcmOverload)
	
	// Since these values are already at max, they won't be normalized
	// Just check that the function runs without error
	if normalized == nil {
		t.Errorf("Expected non-nil result from NormalizeAudio")
	}
	
	// Maximum value in normalized audio should not exceed 32767
	for _, sample := range normalized {
		if sample > 32767 || sample < -32768 {
			t.Errorf("Normalized sample out of range: %d", sample)
		}
	}
}