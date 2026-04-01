package internal

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultRTCPBatchConfig(t *testing.T) {
	config := DefaultRTCPBatchConfig()

	if config.BatchSize != 100 {
		t.Errorf("expected BatchSize=100, got %d", config.BatchSize)
	}
	if config.BatchTimeout != 10*time.Millisecond {
		t.Errorf("expected BatchTimeout=10ms, got %v", config.BatchTimeout)
	}
	if config.BufferSize != 1000 {
		t.Errorf("expected BufferSize=1000, got %d", config.BufferSize)
	}
	if config.NumWorkers != 4 {
		t.Errorf("expected NumWorkers=4, got %d", config.NumWorkers)
	}
}

func TestNewRTCPBatchProcessor(t *testing.T) {
	handler := func(batch *RTCPBatch) {}
	processor := NewRTCPBatchProcessor(nil, handler)

	if processor.config.BatchSize != 100 {
		t.Error("expected default config")
	}

	config := &RTCPBatchConfig{BatchSize: 50}
	processor = NewRTCPBatchProcessor(config, handler)
	if processor.config.BatchSize != 50 {
		t.Errorf("expected BatchSize=50, got %d", processor.config.BatchSize)
	}
}

func TestRTCPBatchProcessor_AddPacket(t *testing.T) {
	var receivedBatches []*RTCPBatch
	var mu sync.Mutex

	handler := func(batch *RTCPBatch) {
		mu.Lock()
		receivedBatches = append(receivedBatches, batch)
		mu.Unlock()
	}

	config := &RTCPBatchConfig{
		BatchSize:    5,
		BatchTimeout: 100 * time.Millisecond,
		BufferSize:   100,
		NumWorkers:   2,
	}

	processor := NewRTCPBatchProcessor(config, handler)
	processor.Start()
	defer processor.Stop()

	// Add packets to trigger batch
	for i := 0; i < 5; i++ {
		packet := &RTCPPacketInfo{
			Data:      []byte{0x80, 200, 0, 1, 0, 0, 0, 1},
			Timestamp: time.Now(),
			SessionID: "session-1",
			Direction: "inbound",
			SrcSSRC:   1,
		}
		processor.AddPacket(packet)
	}

	// Wait for batch processing
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	count := len(receivedBatches)
	mu.Unlock()

	if count != 1 {
		t.Errorf("expected 1 batch, got %d", count)
	}

	stats := processor.Stats()
	if stats.PacketsReceived != 5 {
		t.Errorf("expected 5 packets received, got %d", stats.PacketsReceived)
	}
}

func TestRTCPBatchProcessor_Timeout(t *testing.T) {
	var receivedBatches []*RTCPBatch
	var mu sync.Mutex

	handler := func(batch *RTCPBatch) {
		mu.Lock()
		receivedBatches = append(receivedBatches, batch)
		mu.Unlock()
	}

	config := &RTCPBatchConfig{
		BatchSize:    100, // Large batch size
		BatchTimeout: 50 * time.Millisecond,
		BufferSize:   100,
		NumWorkers:   2,
	}

	processor := NewRTCPBatchProcessor(config, handler)
	processor.Start()
	defer processor.Stop()

	// Add fewer packets than batch size
	for i := 0; i < 3; i++ {
		packet := &RTCPPacketInfo{
			Data:      []byte{0x80, 200, 0, 1},
			SessionID: "session-1",
		}
		processor.AddPacket(packet)
	}

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	count := len(receivedBatches)
	mu.Unlock()

	if count < 1 {
		t.Error("expected at least 1 batch from timeout")
	}
}

func TestRTCPBatchProcessor_MultipleSessions(t *testing.T) {
	var batchCount atomic.Int64

	handler := func(batch *RTCPBatch) {
		batchCount.Add(1)
	}

	config := &RTCPBatchConfig{
		BatchSize:    2,
		BatchTimeout: 100 * time.Millisecond,
		BufferSize:   100,
		NumWorkers:   2,
	}

	processor := NewRTCPBatchProcessor(config, handler)
	processor.Start()
	defer processor.Stop()

	// Add packets for different sessions
	sessions := []string{"session-1", "session-2", "session-3"}
	for _, session := range sessions {
		for i := 0; i < 2; i++ {
			packet := &RTCPPacketInfo{
				Data:      []byte{0x80, 200, 0, 1},
				SessionID: session,
			}
			processor.AddPacket(packet)
		}
	}

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	count := batchCount.Load()
	if count < 3 {
		t.Errorf("expected at least 3 batches (one per session), got %d", count)
	}
}

func TestRTCPBatchProcessor_BufferFull(t *testing.T) {
	handler := func(batch *RTCPBatch) {
		time.Sleep(100 * time.Millisecond) // Slow handler
	}

	config := &RTCPBatchConfig{
		BatchSize:    100,
		BatchTimeout: time.Second,
		BufferSize:   5, // Small buffer
		NumWorkers:   1,
	}

	processor := NewRTCPBatchProcessor(config, handler)
	processor.Start()
	defer processor.Stop()

	// Flood packets
	dropped := 0
	for i := 0; i < 100; i++ {
		packet := &RTCPPacketInfo{
			Data:      []byte{0x80, 200, 0, 1},
			SessionID: "session-1",
		}
		if !processor.AddPacket(packet) {
			dropped++
		}
	}

	if dropped == 0 {
		t.Error("expected some packets to be dropped")
	}

	stats := processor.Stats()
	if stats.PacketsDropped == 0 {
		t.Error("expected PacketsDropped > 0")
	}
}

func TestRTCPCompoundBuilder(t *testing.T) {
	builder := NewRTCPCompoundBuilder(0x12345678)

	// Add SR
	builder.AddSR(0x0102030405060708, 1000, 100, 10000)

	// Add SDES
	builder.AddSDES("test@example.com")

	// Build compound packet
	compound := builder.Build()

	if len(compound) == 0 {
		t.Fatal("expected non-empty compound packet")
	}

	// Verify first packet is SR (PT=200)
	if compound[1] != 200 {
		t.Errorf("expected PT=200 (SR), got %d", compound[1])
	}
}

func TestRTCPCompoundBuilder_AllTypes(t *testing.T) {
	builder := NewRTCPCompoundBuilder(0xDEADBEEF)

	builder.AddSR(0, 0, 0, 0)
	builder.AddRR()
	builder.AddSDES("cname")
	builder.AddBYE("leaving")

	compound := builder.Build()

	// Should have 4 packets
	if len(compound) < 4*4 { // At least 4 headers
		t.Error("expected compound packet with 4 RTCP packets")
	}
}

func TestRTCPCompoundBuilder_SRWithReportBlocks(t *testing.T) {
	builder := NewRTCPCompoundBuilder(0x12345678)

	reports := []RTCPReportBlock{
		{
			SSRC:             0xAABBCCDD,
			FractionLost:     25,
			TotalLost:        100,
			HighestSeq:       50000,
			Jitter:           500,
			LastSR:           0x11223344,
			DelaySinceLastSR: 1000,
		},
	}

	builder.AddSRWithReportBlock(0, 0, 0, 0, reports)

	compound := builder.Build()

	// SR with 1 report block: 28 + 24 = 52 bytes
	if len(compound) != 52 {
		t.Errorf("expected 52 bytes, got %d", len(compound))
	}

	// Check RC field (should be 1)
	if compound[0]&0x1F != 1 {
		t.Errorf("expected RC=1, got %d", compound[0]&0x1F)
	}
}

func TestRTCPCompoundBuilder_Clear(t *testing.T) {
	builder := NewRTCPCompoundBuilder(0x12345678)

	builder.AddSR(0, 0, 0, 0)
	builder.AddRR()

	builder.Clear()

	compound := builder.Build()
	if len(compound) != 0 {
		t.Error("expected empty after clear")
	}
}

func TestParseRTCPPacketBasic(t *testing.T) {
	tests := []struct {
		name       string
		data       []byte
		expectErr  bool
		expectType uint8
	}{
		{
			name:       "valid SR",
			data:       []byte{0x80, 200, 0, 6, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			expectErr:  false,
			expectType: 200,
		},
		{
			name:       "valid RR",
			data:       []byte{0x80, 201, 0, 1, 0, 0, 0, 1},
			expectErr:  false,
			expectType: 201,
		},
		{
			name:      "too short",
			data:      []byte{0x80, 200},
			expectErr: true,
		},
		{
			name:      "wrong version",
			data:      []byte{0x40, 200, 0, 1, 0, 0, 0, 1}, // Version 1
			expectErr: true,
		},
		{
			name:      "length mismatch",
			data:      []byte{0x80, 200, 0, 10, 0, 0, 0, 1}, // Claims 10 words but only has 2
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseRTCPPacketBasic(tt.data)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if parsed.PacketType != tt.expectType {
				t.Errorf("expected PacketType=%d, got %d", tt.expectType, parsed.PacketType)
			}

			if parsed.Version != 2 {
				t.Errorf("expected Version=2, got %d", parsed.Version)
			}
		})
	}
}

func TestRTCPPacketParsed_PacketTypeName(t *testing.T) {
	tests := []struct {
		packetType uint8
		expected   string
	}{
		{200, "SR"},
		{201, "RR"},
		{202, "SDES"},
		{203, "BYE"},
		{204, "APP"},
		{205, "RTPFB"},
		{206, "PSFB"},
		{207, "XR"},
		{255, "Unknown"},
	}

	for _, tt := range tests {
		parsed := &RTCPPacketParsed{PacketType: tt.packetType}
		name := parsed.PacketTypeName()
		if name != tt.expected {
			t.Errorf("PacketType %d: expected %s, got %s", tt.packetType, tt.expected, name)
		}
	}
}

func TestRTCPBatchProcessor_Stats(t *testing.T) {
	handler := func(batch *RTCPBatch) {}

	config := &RTCPBatchConfig{
		BatchSize:    10,
		BatchTimeout: time.Second,
		BufferSize:   100,
		NumWorkers:   2,
	}

	processor := NewRTCPBatchProcessor(config, handler)
	processor.Start()

	// Add some packets
	for i := 0; i < 15; i++ {
		packet := &RTCPPacketInfo{
			Data:      []byte{0x80, 200, 0, 1},
			SessionID: "session-1",
		}
		processor.AddPacket(packet)
	}

	time.Sleep(50 * time.Millisecond)

	stats := processor.Stats()
	if stats.PacketsReceived != 15 {
		t.Errorf("expected 15 packets received, got %d", stats.PacketsReceived)
	}
	if stats.BatchesCreated < 1 {
		t.Error("expected at least 1 batch created")
	}

	processor.Stop()
}

func TestRTCPBatch_Fields(t *testing.T) {
	batch := &RTCPBatch{
		Packets:   make([]*RTCPPacketInfo, 0),
		StartTime: time.Now(),
		SessionID: "test-session",
	}

	packet := &RTCPPacketInfo{
		Data:      []byte{0x80, 200},
		Timestamp: time.Now(),
		SessionID: "test-session",
		Direction: "outbound",
		SrcSSRC:   12345,
		DstSSRC:   67890,
	}

	batch.Packets = append(batch.Packets, packet)
	batch.EndTime = time.Now()

	if len(batch.Packets) != 1 {
		t.Error("expected 1 packet in batch")
	}
	if batch.SessionID != "test-session" {
		t.Error("session ID mismatch")
	}
}

func BenchmarkRTCPBatchProcessor(b *testing.B) {
	handler := func(batch *RTCPBatch) {}

	config := &RTCPBatchConfig{
		BatchSize:    100,
		BatchTimeout: 10 * time.Millisecond,
		BufferSize:   10000,
		NumWorkers:   4,
	}

	processor := NewRTCPBatchProcessor(config, handler)
	processor.Start()
	defer processor.Stop()

	packet := &RTCPPacketInfo{
		Data:      []byte{0x80, 200, 0, 6, 0, 0, 0, 1},
		SessionID: "bench-session",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processor.AddPacket(packet)
	}
}

func BenchmarkRTCPCompoundBuild(b *testing.B) {
	builder := NewRTCPCompoundBuilder(0x12345678)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.AddSR(0, 0, 0, 0)
		builder.AddSDES("test@example.com")
		builder.Build()
		builder.Clear()
	}
}
