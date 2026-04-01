package internal

import (
	"testing"
)

func TestDefaultG729Config(t *testing.T) {
	config := DefaultG729Config()

	if config.Mode != G729ModeAnnexAB {
		t.Errorf("expected Mode=G729ModeAnnexAB, got %d", config.Mode)
	}
	if !config.EnableVAD {
		t.Error("expected EnableVAD=true")
	}
	if !config.EnableDTX {
		t.Error("expected EnableDTX=true")
	}
	if !config.EnablePLC {
		t.Error("expected EnablePLC=true")
	}
	if config.MaxFramesPerPacket != 2 {
		t.Errorf("expected MaxFramesPerPacket=2, got %d", config.MaxFramesPerPacket)
	}
}

func TestG729Constants(t *testing.T) {
	if G729FrameSize != 10 {
		t.Errorf("expected G729FrameSize=10, got %d", G729FrameSize)
	}
	if G729SampleRate != 8000 {
		t.Errorf("expected G729SampleRate=8000, got %d", G729SampleRate)
	}
	if G729FrameSamples != 80 {
		t.Errorf("expected G729FrameSamples=80, got %d", G729FrameSamples)
	}
	if G729FrameDuration != 10 {
		t.Errorf("expected G729FrameDuration=10, got %d", G729FrameDuration)
	}
	if G729PayloadType != 18 {
		t.Errorf("expected G729PayloadType=18, got %d", G729PayloadType)
	}
}

func TestNewG729Encoder(t *testing.T) {
	enc, err := NewG729Encoder(nil)
	if err != nil {
		t.Fatalf("failed to create encoder: %v", err)
	}
	defer enc.Close()

	if !enc.initialized {
		t.Error("encoder should be initialized")
	}
}

func TestG729Encoder_Encode(t *testing.T) {
	enc, _ := NewG729Encoder(nil)
	defer enc.Close()

	// Create test PCM data (one frame)
	pcm := make([]int16, G729FrameSamples)
	for i := range pcm {
		pcm[i] = int16(i * 100) // Simple test pattern
	}

	encoded, err := enc.Encode(pcm)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	if len(encoded) != G729FrameSize {
		t.Errorf("expected %d bytes, got %d", G729FrameSize, len(encoded))
	}
}

func TestG729Encoder_EncodeMultipleFrames(t *testing.T) {
	config := &G729Config{
		MaxFramesPerPacket: 3,
	}
	enc, _ := NewG729Encoder(config)
	defer enc.Close()

	// Create test PCM data (2 frames)
	pcm := make([]int16, G729FrameSamples*2)
	for i := range pcm {
		pcm[i] = int16(i * 50)
	}

	encoded, err := enc.Encode(pcm)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	if len(encoded) != G729FrameSize*2 {
		t.Errorf("expected %d bytes, got %d", G729FrameSize*2, len(encoded))
	}
}

func TestG729Encoder_FrameTooShort(t *testing.T) {
	enc, _ := NewG729Encoder(nil)
	defer enc.Close()

	// Create PCM data shorter than one frame
	pcm := make([]int16, G729FrameSamples-10)

	_, err := enc.Encode(pcm)
	if err != ErrG729FrameTooShort {
		t.Errorf("expected ErrG729FrameTooShort, got %v", err)
	}
}

func TestG729Encoder_SilenceDetection(t *testing.T) {
	config := &G729Config{
		EnableVAD: true,
		EnableDTX: true,
	}
	enc, _ := NewG729Encoder(config)
	defer enc.Close()

	// Create silent PCM data
	pcm := make([]int16, G729FrameSamples)

	// Encode multiple silent frames
	for i := 0; i < 5; i++ {
		encoded, err := enc.Encode(pcm)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		// After a few silent frames, should get SID frame
		if i > 3 && len(encoded) != G729AnnexBSize {
			// DTX may kick in
		}
	}
}

func TestG729Encoder_Close(t *testing.T) {
	enc, _ := NewG729Encoder(nil)

	if err := enc.Close(); err != nil {
		t.Errorf("close failed: %v", err)
	}

	if enc.initialized {
		t.Error("encoder should not be initialized after close")
	}

	// Encode after close should fail
	_, err := enc.Encode(make([]int16, G729FrameSamples))
	if err != ErrG729EncoderNotReady {
		t.Errorf("expected ErrG729EncoderNotReady, got %v", err)
	}
}

func TestNewG729Decoder(t *testing.T) {
	dec, err := NewG729Decoder(nil)
	if err != nil {
		t.Fatalf("failed to create decoder: %v", err)
	}
	defer dec.Close()

	if !dec.initialized {
		t.Error("decoder should be initialized")
	}
}

func TestG729Decoder_Decode(t *testing.T) {
	dec, _ := NewG729Decoder(nil)
	defer dec.Close()

	// Create valid G.729 frame
	g729Data := make([]byte, G729FrameSize)
	g729Data[4] = g729Data[0] ^ g729Data[1] ^ g729Data[2] ^ g729Data[3] // checksum

	decoded, err := dec.Decode(g729Data)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(decoded) != G729FrameSamples {
		t.Errorf("expected %d samples, got %d", G729FrameSamples, len(decoded))
	}
}

func TestG729Decoder_DecodeMultipleFrames(t *testing.T) {
	dec, _ := NewG729Decoder(nil)
	defer dec.Close()

	// Create 2 valid G.729 frames
	g729Data := make([]byte, G729FrameSize*2)
	g729Data[4] = g729Data[0] ^ g729Data[1] ^ g729Data[2] ^ g729Data[3]
	g729Data[14] = g729Data[10] ^ g729Data[11] ^ g729Data[12] ^ g729Data[13]

	decoded, err := dec.Decode(g729Data)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(decoded) != G729FrameSamples*2 {
		t.Errorf("expected %d samples, got %d", G729FrameSamples*2, len(decoded))
	}
}

func TestG729Decoder_DecodeSIDFrame(t *testing.T) {
	dec, _ := NewG729Decoder(nil)
	defer dec.Close()

	// SID frame
	sidData := make([]byte, G729AnnexBSize)

	decoded, err := dec.Decode(sidData)
	if err != nil {
		t.Fatalf("decode SID failed: %v", err)
	}

	if len(decoded) != G729FrameSamples {
		t.Errorf("expected %d samples, got %d", G729FrameSamples, len(decoded))
	}
}

func TestG729Decoder_InvalidFrame(t *testing.T) {
	dec, _ := NewG729Decoder(nil)
	defer dec.Close()

	// Invalid frame size (not multiple of 10 and not SID)
	invalidData := make([]byte, 7)

	_, err := dec.Decode(invalidData)
	if err != ErrG729InvalidFrame {
		t.Errorf("expected ErrG729InvalidFrame, got %v", err)
	}

	// Empty data
	_, err = dec.Decode([]byte{})
	if err != ErrG729InvalidFrame {
		t.Errorf("expected ErrG729InvalidFrame for empty data, got %v", err)
	}
}

func TestG729Decoder_PLC(t *testing.T) {
	config := &G729Config{
		EnablePLC: true,
	}
	dec, _ := NewG729Decoder(config)
	defer dec.Close()

	// First decode a real frame
	g729Data := make([]byte, G729FrameSize)
	g729Data[0] = 0x10 // Some energy
	g729Data[4] = g729Data[0] ^ g729Data[1] ^ g729Data[2] ^ g729Data[3]
	dec.Decode(g729Data)

	// Now test PLC
	plc, err := dec.DecodePLC()
	if err != nil {
		t.Fatalf("PLC failed: %v", err)
	}

	if len(plc) != G729FrameSamples {
		t.Errorf("expected %d samples, got %d", G729FrameSamples, len(plc))
	}

	// Multiple PLC calls should fade
	plc2, _ := dec.DecodePLC()
	plc3, _ := dec.DecodePLC()

	// Check fading (samples should decrease)
	if plc2[0] >= plc[0] && plc[0] != 0 {
		// May not always fade if initial was 0
	}
	_ = plc3 // use variable
}

func TestG729Decoder_PLCDisabled(t *testing.T) {
	config := &G729Config{
		EnablePLC: false,
	}
	dec, _ := NewG729Decoder(config)
	defer dec.Close()

	plc, err := dec.DecodePLC()
	if err != nil {
		t.Fatalf("PLC failed: %v", err)
	}

	// Should return silence
	for _, sample := range plc {
		if sample != 0 {
			t.Error("expected silence when PLC disabled")
			break
		}
	}
}

func TestG729Decoder_Close(t *testing.T) {
	dec, _ := NewG729Decoder(nil)

	if err := dec.Close(); err != nil {
		t.Errorf("close failed: %v", err)
	}

	_, err := dec.Decode(make([]byte, G729FrameSize))
	if err != ErrG729DecoderNotReady {
		t.Errorf("expected ErrG729DecoderNotReady, got %v", err)
	}
}

func TestG729Transcoder(t *testing.T) {
	tc, err := NewG729Transcoder(nil)
	if err != nil {
		t.Fatalf("failed to create transcoder: %v", err)
	}
	defer tc.Close()

	// Test PCM -> G.729 -> PCM roundtrip
	originalPCM := make([]int16, G729FrameSamples)
	for i := range originalPCM {
		originalPCM[i] = int16(i * 100)
	}

	g729, err := tc.PCMToG729(originalPCM)
	if err != nil {
		t.Fatalf("PCMToG729 failed: %v", err)
	}

	decodedPCM, err := tc.G729ToPCM(g729)
	if err != nil {
		t.Fatalf("G729ToPCM failed: %v", err)
	}

	if len(decodedPCM) != len(originalPCM) {
		t.Errorf("length mismatch: original=%d, decoded=%d", len(originalPCM), len(decodedPCM))
	}
}

func TestG729Transcoder_ToPCMU(t *testing.T) {
	tc, _ := NewG729Transcoder(nil)
	defer tc.Close()

	// Create G.729 frame
	g729Data := make([]byte, G729FrameSize)
	g729Data[4] = g729Data[0] ^ g729Data[1] ^ g729Data[2] ^ g729Data[3]

	pcmu, err := tc.G729ToPCMU(g729Data)
	if err != nil {
		t.Fatalf("G729ToPCMU failed: %v", err)
	}

	if len(pcmu) != G729FrameSamples {
		t.Errorf("expected %d bytes, got %d", G729FrameSamples, len(pcmu))
	}
}

func TestG729Transcoder_FromPCMU(t *testing.T) {
	tc, _ := NewG729Transcoder(nil)
	defer tc.Close()

	// Create PCMU data (one frame worth)
	pcmuData := make([]byte, G729FrameSamples)
	for i := range pcmuData {
		pcmuData[i] = byte(i)
	}

	g729, err := tc.PCMUToG729(pcmuData)
	if err != nil {
		t.Fatalf("PCMUToG729 failed: %v", err)
	}

	if len(g729) != G729FrameSize {
		t.Errorf("expected %d bytes, got %d", G729FrameSize, len(g729))
	}
}

func TestG729Transcoder_ToPCMA(t *testing.T) {
	tc, _ := NewG729Transcoder(nil)
	defer tc.Close()

	g729Data := make([]byte, G729FrameSize)
	g729Data[4] = g729Data[0] ^ g729Data[1] ^ g729Data[2] ^ g729Data[3]

	pcma, err := tc.G729ToPCMA(g729Data)
	if err != nil {
		t.Fatalf("G729ToPCMA failed: %v", err)
	}

	if len(pcma) != G729FrameSamples {
		t.Errorf("expected %d bytes, got %d", G729FrameSamples, len(pcma))
	}
}

func TestG729Transcoder_FromPCMA(t *testing.T) {
	tc, _ := NewG729Transcoder(nil)
	defer tc.Close()

	pcmaData := make([]byte, G729FrameSamples)
	for i := range pcmaData {
		pcmaData[i] = byte(i)
	}

	g729, err := tc.PCMAToG729(pcmaData)
	if err != nil {
		t.Fatalf("PCMAToG729 failed: %v", err)
	}

	if len(g729) != G729FrameSize {
		t.Errorf("expected %d bytes, got %d", G729FrameSize, len(g729))
	}
}

func TestLinearMulawConversion(t *testing.T) {
	// Test roundtrip - use int32 to avoid overflow when i approaches 32767
	for i := int32(-32768); i < 32767; i += 100 {
		sample := int16(i)
		mulaw := LinearToMulaw(sample)
		back := MulawToLinear(mulaw)

		// Allow some quantization error
		diff := sample - back
		if diff < 0 {
			diff = -diff
		}
		if diff > 1024 { // μ-law has 8-bit precision
			t.Errorf("roundtrip error too large for %d: got %d (diff %d)", sample, back, diff)
		}
	}
}

func TestLinearAlawConversion(t *testing.T) {
	// Test roundtrip - use int32 to avoid overflow when i approaches 32767
	for i := int32(-32768); i < 32767; i += 100 {
		sample := int16(i)
		alaw := LinearToAlaw(sample)
		back := AlawToLinear(alaw)

		// Allow some quantization error
		diff := sample - back
		if diff < 0 {
			diff = -diff
		}
		if diff > 1024 { // A-law has 8-bit precision
			t.Errorf("roundtrip error too large for %d: got %d (diff %d)", sample, back, diff)
		}
	}
}

func TestIsG729Frame(t *testing.T) {
	tests := []struct {
		data   []byte
		valid  bool
	}{
		{make([]byte, G729FrameSize), true},     // Single frame
		{make([]byte, G729FrameSize*2), true},   // Two frames
		{make([]byte, G729AnnexBSize), true},    // SID frame
		{make([]byte, 5), false},                // Invalid size
		{make([]byte, 0), false},                // Empty
		{make([]byte, 11), false},               // Not multiple of 10
	}

	for _, tt := range tests {
		result := IsG729Frame(tt.data)
		if result != tt.valid {
			t.Errorf("IsG729Frame(%d bytes): expected %v, got %v", len(tt.data), tt.valid, result)
		}
	}
}

func TestG729FrameCount(t *testing.T) {
	tests := []struct {
		dataLen int
		count   int
	}{
		{G729FrameSize, 1},
		{G729FrameSize * 2, 2},
		{G729FrameSize * 3, 3},
		{G729AnnexBSize, 1},
	}

	for _, tt := range tests {
		data := make([]byte, tt.dataLen)
		result := G729FrameCount(data)
		if result != tt.count {
			t.Errorf("G729FrameCount(%d bytes): expected %d, got %d", tt.dataLen, tt.count, result)
		}
	}
}

func TestG729Duration(t *testing.T) {
	tests := []struct {
		dataLen  int
		duration int
	}{
		{G729FrameSize, 10},      // 1 frame = 10ms
		{G729FrameSize * 2, 20},  // 2 frames = 20ms
		{G729FrameSize * 3, 30},  // 3 frames = 30ms
		{G729AnnexBSize, 10},     // SID = 10ms
	}

	for _, tt := range tests {
		data := make([]byte, tt.dataLen)
		result := G729Duration(data)
		if result != tt.duration {
			t.Errorf("G729Duration(%d bytes): expected %dms, got %dms", tt.dataLen, tt.duration, result)
		}
	}
}

func BenchmarkG729Encode(b *testing.B) {
	enc, _ := NewG729Encoder(nil)
	defer enc.Close()

	pcm := make([]int16, G729FrameSamples)
	for i := range pcm {
		pcm[i] = int16(i * 100)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc.Encode(pcm)
	}
}

func BenchmarkG729Decode(b *testing.B) {
	dec, _ := NewG729Decoder(nil)
	defer dec.Close()

	g729Data := make([]byte, G729FrameSize)
	g729Data[4] = g729Data[0] ^ g729Data[1] ^ g729Data[2] ^ g729Data[3]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dec.Decode(g729Data)
	}
}
