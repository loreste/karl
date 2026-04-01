package internal

import (
	"context"
	"testing"
	"time"
)

func TestDefaultBenchmarkConfig(t *testing.T) {
	config := DefaultBenchmarkConfig()

	if config.Duration != 30*time.Second {
		t.Errorf("expected Duration=30s, got %v", config.Duration)
	}
	if config.NumSessions != 1000 {
		t.Errorf("expected NumSessions=1000, got %d", config.NumSessions)
	}
	if config.PacketRate != 50 {
		t.Errorf("expected PacketRate=50, got %d", config.PacketRate)
	}
	if config.PacketSize != 172 {
		t.Errorf("expected PacketSize=172, got %d", config.PacketSize)
	}
	if config.WarmupDuration != 5*time.Second {
		t.Errorf("expected WarmupDuration=5s, got %v", config.WarmupDuration)
	}
}

func TestNewBenchmarkRunner(t *testing.T) {
	t.Run("with nil config", func(t *testing.T) {
		runner := NewBenchmarkRunner(nil)
		if runner.config == nil {
			t.Fatal("expected non-nil config")
		}
		if runner.config.NumSessions != 1000 {
			t.Error("expected default config values")
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		config := &BenchmarkConfig{
			Duration:    10 * time.Second,
			NumSessions: 100,
			PacketRate:  25,
			PacketSize:  200,
		}
		runner := NewBenchmarkRunner(config)
		if runner.config.NumSessions != 100 {
			t.Errorf("expected NumSessions=100, got %d", runner.config.NumSessions)
		}
	})
}

func TestBenchmarkRunner_Run(t *testing.T) {
	config := &BenchmarkConfig{
		Duration:       500 * time.Millisecond,
		NumSessions:    10,
		PacketRate:     100,
		PacketSize:     100,
		NumWorkers:     2,
		WarmupDuration: 100 * time.Millisecond,
		ReportInterval: 0, // Disable reporting
	}

	runner := NewBenchmarkRunner(config)
	result, err := runner.Run()

	if err != nil {
		t.Fatalf("benchmark failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Verify some packets were processed
	if result.PacketsSent == 0 {
		t.Error("expected PacketsSent > 0")
	}
	if result.PacketsReceived == 0 {
		t.Error("expected PacketsReceived > 0")
	}

	// Verify throughput was calculated
	if result.PacketsPerSecond <= 0 {
		t.Error("expected PacketsPerSecond > 0")
	}

	// Verify sessions were tracked
	if result.SessionsCreated != int64(config.NumSessions) {
		t.Errorf("expected SessionsCreated=%d, got %d", config.NumSessions, result.SessionsCreated)
	}
	if result.SessionsCompleted != int64(config.NumSessions) {
		t.Errorf("expected SessionsCompleted=%d, got %d", config.NumSessions, result.SessionsCompleted)
	}

	// Verify latency was measured
	if result.AvgLatency <= 0 {
		t.Error("expected AvgLatency > 0")
	}
}

func TestBenchmarkResult_Report(t *testing.T) {
	result := &BenchmarkResult{
		Config: &BenchmarkConfig{
			Duration:    10 * time.Second,
			NumSessions: 100,
			PacketRate:  50,
			PacketSize:  172,
			NumWorkers:  4,
		},
		Duration:          10 * time.Second,
		PacketsSent:       50000,
		PacketsReceived:   49950,
		PacketsDropped:    50,
		BytesSent:         8600000,
		BytesReceived:     8590000,
		PacketsPerSecond:  5000,
		BytesPerSecond:    860000,
		BitsPerSecond:     6880000,
		AvgLatency:        1000,
		MinLatency:        500,
		MaxLatency:        5000,
		P50Latency:        1000,
		P95Latency:        2000,
		P99Latency:        3000,
		SessionsCreated:   100,
		SessionsCompleted: 100,
		MemoryUsed:        10485760,
		GoroutineCount:    50,
		Errors:            nil,
	}

	report := result.Report()

	if report == "" {
		t.Fatal("expected non-empty report")
	}

	// Check for key metrics in report
	if !benchContainsString(report, "50000") {
		t.Error("expected report to contain packet count")
	}
	if !benchContainsString(report, "Sessions") {
		t.Error("expected report to contain Sessions section")
	}
	if !benchContainsString(report, "Latency") {
		t.Error("expected report to contain Latency section")
	}
}

func benchContainsString(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || benchContainsString(s[1:], substr)))
}

func TestUDPBenchmark_Setup(t *testing.T) {
	config := &BenchmarkConfig{
		NumWorkers: 2,
		PacketSize: 100,
		PacketRate: 100,
	}

	bench := NewUDPBenchmark(config)
	err := bench.Setup()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer bench.Cleanup()

	if bench.listener == nil {
		t.Error("expected non-nil listener")
	}
	if bench.serverAddr == nil {
		t.Error("expected non-nil server address")
	}
	if len(bench.senders) != config.NumWorkers {
		t.Errorf("expected %d senders, got %d", config.NumWorkers, len(bench.senders))
	}
}

func TestUDPBenchmark_Run(t *testing.T) {
	config := &BenchmarkConfig{
		NumWorkers: 2,
		PacketSize: 100,
		PacketRate: 100,
	}

	bench := NewUDPBenchmark(config)
	err := bench.Setup()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer bench.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	result, err := bench.Run(ctx)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if result.PacketsSent == 0 {
		t.Error("expected PacketsSent > 0")
	}
	if result.Duration <= 0 {
		t.Error("expected Duration > 0")
	}
}

func TestMemoryBenchmark_Run(t *testing.T) {
	bench := NewMemoryBenchmark()
	result := bench.Run(1000, 1024)

	if result.NumAllocations != 1000 {
		t.Errorf("expected NumAllocations=1000, got %d", result.NumAllocations)
	}
	if result.AllocationSize != 1024 {
		t.Errorf("expected AllocationSize=1024, got %d", result.AllocationSize)
	}
	if result.TotalAllocated == 0 {
		t.Error("expected TotalAllocated > 0")
	}
	if result.AllocDuration <= 0 {
		t.Error("expected AllocDuration > 0")
	}
	if result.AllocsPerSecond <= 0 {
		t.Error("expected AllocsPerSecond > 0")
	}
}

func TestBenchmarkRunner_MinimalRun(t *testing.T) {
	// Test with minimal configuration
	config := &BenchmarkConfig{
		Duration:       100 * time.Millisecond,
		NumSessions:    1,
		PacketRate:     10,
		PacketSize:     50,
		NumWorkers:     1,
		WarmupDuration: 0,
		ReportInterval: 0,
	}

	runner := NewBenchmarkRunner(config)
	result, err := runner.Run()

	if err != nil {
		t.Fatalf("minimal benchmark failed: %v", err)
	}

	if result.SessionsCreated != 1 {
		t.Errorf("expected 1 session created, got %d", result.SessionsCreated)
	}
}

func TestBenchmarkRunner_NoWarmup(t *testing.T) {
	config := &BenchmarkConfig{
		Duration:       100 * time.Millisecond,
		NumSessions:    5,
		PacketRate:     50,
		PacketSize:     100,
		NumWorkers:     2,
		WarmupDuration: 0, // No warmup
		ReportInterval: 0,
	}

	runner := NewBenchmarkRunner(config)
	result, err := runner.Run()

	if err != nil {
		t.Fatalf("benchmark failed: %v", err)
	}

	if result.WarmupTime != 0 {
		t.Errorf("expected WarmupTime=0, got %v", result.WarmupTime)
	}
}

func TestBenchmarkResult_Throughput(t *testing.T) {
	config := &BenchmarkConfig{
		Duration:       200 * time.Millisecond,
		NumSessions:    5,
		PacketRate:     100,
		PacketSize:     100,
		NumWorkers:     2,
		WarmupDuration: 0,
		ReportInterval: 0,
	}

	runner := NewBenchmarkRunner(config)
	result, err := runner.Run()

	if err != nil {
		t.Fatalf("benchmark failed: %v", err)
	}

	// Verify throughput calculations
	if result.PacketsPerSecond <= 0 {
		t.Error("expected positive PacketsPerSecond")
	}
	if result.BytesPerSecond <= 0 {
		t.Error("expected positive BytesPerSecond")
	}
	if result.BitsPerSecond != result.BytesPerSecond*8 {
		t.Error("BitsPerSecond should be BytesPerSecond * 8")
	}
}

func BenchmarkPacketProcessing(b *testing.B) {
	runner := NewBenchmarkRunner(nil)

	packet := make([]byte, 172)
	for i := range packet {
		packet[i] = byte(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runner.processPacket(packet, 0)
	}
}

func BenchmarkMemoryAllocation(b *testing.B) {
	bench := NewMemoryBenchmark()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bench.Run(100, 1024)
	}
}
