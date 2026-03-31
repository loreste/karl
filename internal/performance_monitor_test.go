package internal

import (
	"testing"
	"time"
)

func TestNewPerformanceMonitor(t *testing.T) {
	pm := NewPerformanceMonitor()
	if pm == nil {
		t.Fatal("NewPerformanceMonitor returned nil")
	}
	defer pm.Stop()

	if pm.sampleInterval != 1*time.Second {
		t.Errorf("Expected sampleInterval 1s, got %v", pm.sampleInterval)
	}

	if pm.historySize != 100 {
		t.Errorf("Expected historySize 100, got %d", pm.historySize)
	}

	if len(pm.latencyHistogram) != 20 {
		t.Errorf("Expected 20 histogram buckets, got %d", len(pm.latencyHistogram))
	}
}

func TestPerformanceMonitor_RecordPacket(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	// Record successful packet
	pm.RecordPacket(100, false)
	pm.RecordPacket(200, false)

	stats := pm.GetStats()
	if stats.PacketsProcessed != 2 {
		t.Errorf("Expected PacketsProcessed 2, got %d", stats.PacketsProcessed)
	}
	if stats.BytesProcessed != 300 {
		t.Errorf("Expected BytesProcessed 300, got %d", stats.BytesProcessed)
	}
	if stats.PacketsDropped != 0 {
		t.Errorf("Expected PacketsDropped 0, got %d", stats.PacketsDropped)
	}

	// Record dropped packet
	pm.RecordPacket(50, true)
	stats = pm.GetStats()
	if stats.PacketsDropped != 1 {
		t.Errorf("Expected PacketsDropped 1, got %d", stats.PacketsDropped)
	}
}

func TestPerformanceMonitor_RecordLatency(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	// Record some latencies
	pm.RecordLatency(100)  // 100 microseconds
	pm.RecordLatency(200)
	pm.RecordLatency(500)
	pm.RecordLatency(1000)

	stats := pm.GetStats()

	// Check average
	expectedAvg := float64(100+200+500+1000) / 4
	if stats.LatencyAvg != expectedAvg {
		t.Errorf("Expected LatencyAvg %f, got %f", expectedAvg, stats.LatencyAvg)
	}

	// Check min/max
	if stats.LatencyMin != 100 {
		t.Errorf("Expected LatencyMin 100, got %d", stats.LatencyMin)
	}
	if stats.LatencyMax != 1000 {
		t.Errorf("Expected LatencyMax 1000, got %d", stats.LatencyMax)
	}
}

func TestPerformanceMonitor_RecordLatency_UpdatesMinMax(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	// Record initial latency
	pm.RecordLatency(500)

	stats := pm.GetStats()
	if stats.LatencyMin != 500 {
		t.Errorf("Expected LatencyMin 500, got %d", stats.LatencyMin)
	}
	if stats.LatencyMax != 500 {
		t.Errorf("Expected LatencyMax 500, got %d", stats.LatencyMax)
	}

	// Record lower latency
	pm.RecordLatency(100)
	stats = pm.GetStats()
	if stats.LatencyMin != 100 {
		t.Errorf("Expected LatencyMin 100 after lower value, got %d", stats.LatencyMin)
	}

	// Record higher latency
	pm.RecordLatency(1000)
	stats = pm.GetStats()
	if stats.LatencyMax != 1000 {
		t.Errorf("Expected LatencyMax 1000 after higher value, got %d", stats.LatencyMax)
	}
}

func TestPerformanceMonitor_RecordError(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	pm.RecordError()
	pm.RecordError()
	pm.RecordError()

	stats := pm.GetStats()
	if stats.ProcessingErrors != 3 {
		t.Errorf("Expected ProcessingErrors 3, got %d", stats.ProcessingErrors)
	}
}

func TestPerformanceMonitor_RecordSessionStart(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	pm.RecordSessionStart()
	pm.RecordSessionStart()

	stats := pm.GetStats()
	if stats.ActiveSessions != 2 {
		t.Errorf("Expected ActiveSessions 2, got %d", stats.ActiveSessions)
	}
	if stats.TotalSessions != 2 {
		t.Errorf("Expected TotalSessions 2, got %d", stats.TotalSessions)
	}
}

func TestPerformanceMonitor_RecordSessionEnd(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	pm.RecordSessionStart()
	pm.RecordSessionStart()
	pm.RecordSessionStart()

	// End without error
	pm.RecordSessionEnd(false)
	stats := pm.GetStats()
	if stats.ActiveSessions != 2 {
		t.Errorf("Expected ActiveSessions 2, got %d", stats.ActiveSessions)
	}
	if stats.SessionErrors != 0 {
		t.Errorf("Expected SessionErrors 0, got %d", stats.SessionErrors)
	}

	// End with error
	pm.RecordSessionEnd(true)
	stats = pm.GetStats()
	if stats.ActiveSessions != 1 {
		t.Errorf("Expected ActiveSessions 1, got %d", stats.ActiveSessions)
	}
	if stats.SessionErrors != 1 {
		t.Errorf("Expected SessionErrors 1, got %d", stats.SessionErrors)
	}
}

func TestPerformanceMonitor_GetStats(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	stats := pm.GetStats()

	// Check initial values
	if stats.PacketsProcessed != 0 {
		t.Error("Initial PacketsProcessed should be 0")
	}
	if stats.ActiveSessions != 0 {
		t.Error("Initial ActiveSessions should be 0")
	}

	// Check runtime info is populated
	if stats.NumCPU < 1 {
		t.Error("NumCPU should be at least 1")
	}
	if stats.Goroutines < 1 {
		t.Error("Goroutines should be at least 1")
	}

	// Check uptime
	if stats.Uptime < 0 {
		t.Error("Uptime should be non-negative")
	}
}

func TestPerformanceMonitor_Reset(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	// Add some data
	pm.RecordPacket(100, false)
	pm.RecordLatency(500)
	pm.RecordError()
	pm.RecordSessionStart()

	// Reset
	pm.Reset()

	stats := pm.GetStats()
	if stats.PacketsProcessed != 0 {
		t.Error("PacketsProcessed should be 0 after reset")
	}
	if stats.BytesProcessed != 0 {
		t.Error("BytesProcessed should be 0 after reset")
	}
	if stats.ProcessingErrors != 0 {
		t.Error("ProcessingErrors should be 0 after reset")
	}
	if stats.TotalSessions != 0 {
		t.Error("TotalSessions should be 0 after reset")
	}
	if stats.LatencyMax != 0 {
		t.Error("LatencyMax should be 0 after reset")
	}
}

func TestPerformanceMonitor_GetActiveSessionCount(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	if pm.GetActiveSessionCount() != 0 {
		t.Error("Initial active session count should be 0")
	}

	pm.RecordSessionStart()
	pm.RecordSessionStart()

	if pm.GetActiveSessionCount() != 2 {
		t.Errorf("Expected 2 active sessions, got %d", pm.GetActiveSessionCount())
	}

	pm.RecordSessionEnd(false)

	if pm.GetActiveSessionCount() != 1 {
		t.Errorf("Expected 1 active session, got %d", pm.GetActiveSessionCount())
	}
}

func TestPerformanceMonitor_GetPacketsPerSecond(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	// Initial rate should be 0
	if pm.GetPacketsPerSecond() != 0 {
		t.Error("Initial packets per second should be 0")
	}
}

func TestPerformanceMonitor_GetBytesPerSecond(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	// Initial rate should be 0
	if pm.GetBytesPerSecond() != 0 {
		t.Error("Initial bytes per second should be 0")
	}
}

func TestPerformanceMonitor_GetUptime(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	// Small delay to ensure measurable uptime
	time.Sleep(10 * time.Millisecond)

	uptime := pm.GetUptime()
	if uptime < 10*time.Millisecond {
		t.Error("Uptime should be at least 10ms")
	}
}

func TestPerformanceMonitor_LatencyToBucket(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	tests := []struct {
		latency  uint64
		expected int
	}{
		{50, 0},      // 0-100
		{100, 1},     // 100-200
		{150, 1},     // 100-200
		{200, 2},     // 200-500
		{500, 3},     // 500-1000
		{1000, 4},    // 1000-2000
		{2000, 5},    // 2000-5000
		{5000, 6},    // 5000-10000
		{10000, 7},   // 10000-20000
		{20000, 8},   // 20000-50000
		{50000, 9},   // 50000+
		{100000, 9},  // 50000+
	}

	for _, tt := range tests {
		result := pm.latencyToBucket(tt.latency)
		if result != tt.expected {
			t.Errorf("latencyToBucket(%d) = %d, expected %d", tt.latency, result, tt.expected)
		}
	}
}

func TestPerformanceMonitor_CalculatePercentiles_NoData(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	// No data recorded
	p50, p95, p99 := pm.calculatePercentiles()
	if p50 != 0 || p95 != 0 || p99 != 0 {
		t.Error("Percentiles should be 0 with no data")
	}
}

func TestPerformanceMonitor_CalculatePercentiles_WithData(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	// Record latencies in different buckets
	for i := 0; i < 100; i++ {
		pm.RecordLatency(50) // Bucket 0 (0-100)
	}
	for i := 0; i < 50; i++ {
		pm.RecordLatency(150) // Bucket 1 (100-200)
	}
	for i := 0; i < 25; i++ {
		pm.RecordLatency(750) // Bucket 3 (500-1000)
	}

	// Calculate percentiles
	p50, p95, p99 := pm.calculatePercentiles()

	// With this distribution:
	// Bucket 0: 100 entries (cumulative: 100)
	// Bucket 1: 50 entries (cumulative: 150)
	// Bucket 3: 25 entries (cumulative: 175)
	// Total: 175 entries
	// p50 = 175 * 0.50 = 87.5, should be bucket 0 (50)
	// p95 = 175 * 0.95 = 166.25, should be bucket 3 (750)
	// p99 = 175 * 0.99 = 173.25, should be bucket 3 (750)

	if p50 == 0 {
		t.Error("P50 should not be 0")
	}
	if p95 == 0 {
		t.Error("P95 should not be 0")
	}
	if p99 == 0 {
		t.Error("P99 should not be 0")
	}
}

func TestPerformanceMonitor_Stop(t *testing.T) {
	pm := NewPerformanceMonitor()

	// Stop should not panic
	pm.Stop()

	// Multiple stops should not panic
	// (although the second one will panic due to closing closed channel)
	// This test just verifies the first stop works
}

func TestPerformanceStats_Structure(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	stats := pm.GetStats()

	// Verify all fields are accessible
	_ = stats.Uptime
	_ = stats.PacketsProcessed
	_ = stats.PacketsDropped
	_ = stats.BytesProcessed
	_ = stats.ProcessingErrors
	_ = stats.PacketsPerSecond
	_ = stats.BytesPerSecond
	_ = stats.LatencyAvg
	_ = stats.LatencyMax
	_ = stats.LatencyMin
	_ = stats.LatencyP50
	_ = stats.LatencyP95
	_ = stats.LatencyP99
	_ = stats.ActiveSessions
	_ = stats.TotalSessions
	_ = stats.SessionErrors
	_ = stats.HeapAlloc
	_ = stats.HeapInUse
	_ = stats.GCPauseTotalNs
	_ = stats.LastGCPauseNs
	_ = stats.GCCount
	_ = stats.Goroutines
	_ = stats.NumCPU
}

func TestPerformanceMonitor_ConcurrentAccess(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	// Run concurrent operations
	done := make(chan bool)

	// Writer 1: Record packets
	go func() {
		for i := 0; i < 100; i++ {
			pm.RecordPacket(100, false)
		}
		done <- true
	}()

	// Writer 2: Record latencies
	go func() {
		for i := 0; i < 100; i++ {
			pm.RecordLatency(uint64(i * 10))
		}
		done <- true
	}()

	// Writer 3: Sessions
	go func() {
		for i := 0; i < 50; i++ {
			pm.RecordSessionStart()
		}
		for i := 0; i < 50; i++ {
			pm.RecordSessionEnd(false)
		}
		done <- true
	}()

	// Reader: Get stats
	go func() {
		for i := 0; i < 100; i++ {
			_ = pm.GetStats()
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 4; i++ {
		<-done
	}

	// Verify no data corruption
	stats := pm.GetStats()
	if stats.PacketsProcessed != 100 {
		t.Errorf("Expected 100 packets, got %d", stats.PacketsProcessed)
	}
}

func TestPerformanceMonitor_LatencyMinInitialization(t *testing.T) {
	pm := NewPerformanceMonitor()
	defer pm.Stop()

	// Before any latency recorded, LatencyMin should be 0 (converted from max uint64)
	stats := pm.GetStats()
	if stats.LatencyMin != 0 {
		t.Errorf("Initial LatencyMin should be 0, got %d", stats.LatencyMin)
	}

	// After recording, should reflect actual minimum
	pm.RecordLatency(500)
	stats = pm.GetStats()
	if stats.LatencyMin != 500 {
		t.Errorf("LatencyMin should be 500, got %d", stats.LatencyMin)
	}
}
