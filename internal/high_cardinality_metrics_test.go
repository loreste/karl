package internal

import (
	"sync"
	"testing"
	"time"
)

func TestDefaultHighCardinalityMetricsConfig(t *testing.T) {
	config := DefaultHighCardinalityMetricsConfig()

	if config.Enabled {
		t.Error("expected Enabled=false by default")
	}
	if config.MaxCalls != 10000 {
		t.Errorf("expected MaxCalls=10000, got %d", config.MaxCalls)
	}
	if config.MetricsTTL != 5*time.Minute {
		t.Errorf("expected MetricsTTL=5m, got %v", config.MetricsTTL)
	}
	if config.SamplingRate != 1.0 {
		t.Errorf("expected SamplingRate=1.0, got %f", config.SamplingRate)
	}
}

func TestNewCallMetrics(t *testing.T) {
	cm := NewCallMetrics("call-123", "from-tag", "to-tag")

	if cm.CallID != "call-123" {
		t.Errorf("expected CallID=call-123, got %s", cm.CallID)
	}
	if cm.FromTag != "from-tag" {
		t.Errorf("expected FromTag=from-tag, got %s", cm.FromTag)
	}
	if cm.ToTag != "to-tag" {
		t.Errorf("expected ToTag=to-tag, got %s", cm.ToTag)
	}
	if cm.StartTime.IsZero() {
		t.Error("expected non-zero StartTime")
	}
}

func TestCallMetrics_RecordPackets(t *testing.T) {
	cm := NewCallMetrics("call-123", "from", "to")

	cm.RecordPacketReceived(100)
	cm.RecordPacketReceived(200)
	cm.RecordPacketSent(150)
	cm.RecordPacketLost()
	cm.RecordPacketDropped()

	if cm.PacketsReceived.Load() != 2 {
		t.Errorf("expected PacketsReceived=2, got %d", cm.PacketsReceived.Load())
	}
	if cm.BytesReceived.Load() != 300 {
		t.Errorf("expected BytesReceived=300, got %d", cm.BytesReceived.Load())
	}
	if cm.PacketsSent.Load() != 1 {
		t.Errorf("expected PacketsSent=1, got %d", cm.PacketsSent.Load())
	}
	if cm.BytesSent.Load() != 150 {
		t.Errorf("expected BytesSent=150, got %d", cm.BytesSent.Load())
	}
	if cm.PacketsLost.Load() != 1 {
		t.Errorf("expected PacketsLost=1, got %d", cm.PacketsLost.Load())
	}
	if cm.PacketsDropped.Load() != 1 {
		t.Errorf("expected PacketsDropped=1, got %d", cm.PacketsDropped.Load())
	}
}

func TestCallMetrics_RecordJitter(t *testing.T) {
	cm := NewCallMetrics("call-123", "from", "to")

	cm.RecordJitter(1000)
	cm.RecordJitter(2000)
	cm.RecordJitter(3000)

	avgJitter := cm.GetAverageJitter()
	if avgJitter != 2000 {
		t.Errorf("expected avgJitter=2000, got %f", avgJitter)
	}

	maxJitter := cm.MaxJitter.Load()
	if maxJitter != 3000 {
		t.Errorf("expected maxJitter=3000, got %d", maxJitter)
	}
}

func TestCallMetrics_RecordLatency(t *testing.T) {
	cm := NewCallMetrics("call-123", "from", "to")

	cm.RecordLatency(5000)
	cm.RecordLatency(10000)
	cm.RecordLatency(15000)

	avgLatency := cm.GetAverageLatency()
	if avgLatency != 10000 {
		t.Errorf("expected avgLatency=10000, got %f", avgLatency)
	}

	maxLatency := cm.MaxLatency.Load()
	if maxLatency != 15000 {
		t.Errorf("expected maxLatency=15000, got %d", maxLatency)
	}
}

func TestCallMetrics_RecordRTCP(t *testing.T) {
	cm := NewCallMetrics("call-123", "from", "to")

	cm.RecordRTCP(50, true)
	cm.RecordRTCP(60, false)

	if cm.RTCPPacketsSent.Load() != 1 {
		t.Errorf("expected RTCPPacketsSent=1, got %d", cm.RTCPPacketsSent.Load())
	}
	if cm.RTCPPacketsReceived.Load() != 1 {
		t.Errorf("expected RTCPPacketsReceived=1, got %d", cm.RTCPPacketsReceived.Load())
	}
	if cm.RTCPBytes.Load() != 110 {
		t.Errorf("expected RTCPBytes=110, got %d", cm.RTCPBytes.Load())
	}
}

func TestCallMetrics_RecordError(t *testing.T) {
	cm := NewCallMetrics("call-123", "from", "to")

	cm.RecordError("decrypt")
	cm.RecordError("encrypt")
	cm.RecordError("sequence")
	cm.RecordError("timestamp")
	cm.RecordError("unknown") // Should be ignored

	if cm.DecryptErrors.Load() != 1 {
		t.Errorf("expected DecryptErrors=1, got %d", cm.DecryptErrors.Load())
	}
	if cm.EncryptErrors.Load() != 1 {
		t.Errorf("expected EncryptErrors=1, got %d", cm.EncryptErrors.Load())
	}
	if cm.SequenceErrors.Load() != 1 {
		t.Errorf("expected SequenceErrors=1, got %d", cm.SequenceErrors.Load())
	}
	if cm.TimestampErrors.Load() != 1 {
		t.Errorf("expected TimestampErrors=1, got %d", cm.TimestampErrors.Load())
	}
}

func TestCallMetrics_GetPacketLossRate(t *testing.T) {
	cm := NewCallMetrics("call-123", "from", "to")

	// No packets
	if rate := cm.GetPacketLossRate(); rate != 0 {
		t.Errorf("expected 0 loss rate with no packets, got %f", rate)
	}

	// 10 received, 0 lost
	for i := 0; i < 10; i++ {
		cm.RecordPacketReceived(100)
	}
	if rate := cm.GetPacketLossRate(); rate != 0 {
		t.Errorf("expected 0 loss rate, got %f", rate)
	}

	// Add 10 lost packets
	for i := 0; i < 10; i++ {
		cm.RecordPacketLost()
	}
	rate := cm.GetPacketLossRate()
	if rate != 0.5 {
		t.Errorf("expected 0.5 loss rate, got %f", rate)
	}
}

func TestCallMetrics_ToMap(t *testing.T) {
	cm := NewCallMetrics("call-123", "from", "to")
	cm.Codec = "opus"
	cm.SampleRate = 48000
	cm.Channels = 2

	cm.RecordPacketReceived(100)
	cm.RecordJitter(1000)

	m := cm.ToMap()

	if m["call_id"] != "call-123" {
		t.Errorf("expected call_id=call-123, got %v", m["call_id"])
	}
	if m["codec"] != "opus" {
		t.Errorf("expected codec=opus, got %v", m["codec"])
	}
	if m["packets_received"].(int64) != 1 {
		t.Errorf("expected packets_received=1, got %v", m["packets_received"])
	}
}

func TestNewHighCardinalityMetrics(t *testing.T) {
	hcm := NewHighCardinalityMetrics(nil)

	if hcm.config.Enabled {
		t.Error("expected disabled by default")
	}
	if hcm.calls == nil {
		t.Error("expected non-nil calls map")
	}
}

func TestHighCardinalityMetrics_GetOrCreateCallMetrics(t *testing.T) {
	config := &HighCardinalityMetricsConfig{
		Enabled:  true,
		MaxCalls: 100,
	}
	hcm := NewHighCardinalityMetrics(config)

	// Create new
	cm := hcm.GetOrCreateCallMetrics("call-1", "from", "to")
	if cm == nil {
		t.Fatal("expected non-nil CallMetrics")
	}
	if cm.CallID != "call-1" {
		t.Errorf("expected CallID=call-1, got %s", cm.CallID)
	}

	// Get existing
	cm2 := hcm.GetOrCreateCallMetrics("call-1", "from", "to")
	if cm2 != cm {
		t.Error("expected same CallMetrics instance")
	}

	// Create different call
	cm3 := hcm.GetOrCreateCallMetrics("call-2", "from", "to")
	if cm3 == cm {
		t.Error("expected different CallMetrics instance")
	}

	if hcm.GetActiveCallCount() != 2 {
		t.Errorf("expected 2 active calls, got %d", hcm.GetActiveCallCount())
	}
}

func TestHighCardinalityMetrics_Disabled(t *testing.T) {
	config := &HighCardinalityMetricsConfig{
		Enabled: false,
	}
	hcm := NewHighCardinalityMetrics(config)

	cm := hcm.GetOrCreateCallMetrics("call-1", "from", "to")
	if cm != nil {
		t.Error("expected nil when disabled")
	}

	cm = hcm.GetCallMetrics("call-1", "from", "to")
	if cm != nil {
		t.Error("expected nil when disabled")
	}
}

func TestHighCardinalityMetrics_MaxCalls(t *testing.T) {
	config := &HighCardinalityMetricsConfig{
		Enabled:  true,
		MaxCalls: 2,
	}
	hcm := NewHighCardinalityMetrics(config)

	cm1 := hcm.GetOrCreateCallMetrics("call-1", "from", "to")
	cm2 := hcm.GetOrCreateCallMetrics("call-2", "from", "to")
	cm3 := hcm.GetOrCreateCallMetrics("call-3", "from", "to")

	if cm1 == nil || cm2 == nil {
		t.Error("expected first two calls to succeed")
	}
	if cm3 != nil {
		t.Error("expected third call to be rejected due to max limit")
	}
}

func TestHighCardinalityMetrics_EndCall(t *testing.T) {
	config := &HighCardinalityMetricsConfig{
		Enabled:  true,
		MaxCalls: 100,
	}
	hcm := NewHighCardinalityMetrics(config)

	hcm.GetOrCreateCallMetrics("call-1", "from", "to")
	hcm.EndCall("call-1", "from", "to")

	cm := hcm.GetCallMetrics("call-1", "from", "to")
	if cm == nil {
		t.Fatal("expected call to still exist")
	}
	if cm.EndTime.IsZero() {
		t.Error("expected EndTime to be set")
	}
}

func TestHighCardinalityMetrics_RemoveCall(t *testing.T) {
	config := &HighCardinalityMetricsConfig{
		Enabled:  true,
		MaxCalls: 100,
	}
	hcm := NewHighCardinalityMetrics(config)

	hcm.GetOrCreateCallMetrics("call-1", "from", "to")
	cm := hcm.RemoveCall("call-1", "from", "to")

	if cm == nil {
		t.Fatal("expected to get removed CallMetrics")
	}

	if hcm.GetActiveCallCount() != 0 {
		t.Errorf("expected 0 active calls, got %d", hcm.GetActiveCallCount())
	}

	// Should be in expired list
	expired := hcm.GetExpiredMetrics()
	if len(expired) != 1 {
		t.Errorf("expected 1 expired metric, got %d", len(expired))
	}
}

func TestHighCardinalityMetrics_GetAllCallMetrics(t *testing.T) {
	config := &HighCardinalityMetricsConfig{
		Enabled:  true,
		MaxCalls: 100,
	}
	hcm := NewHighCardinalityMetrics(config)

	hcm.GetOrCreateCallMetrics("call-1", "from", "to")
	hcm.GetOrCreateCallMetrics("call-2", "from", "to")
	hcm.GetOrCreateCallMetrics("call-3", "from", "to")

	all := hcm.GetAllCallMetrics()
	if len(all) != 3 {
		t.Errorf("expected 3 metrics, got %d", len(all))
	}
}

func TestHighCardinalityMetrics_GetSummary(t *testing.T) {
	config := &HighCardinalityMetricsConfig{
		Enabled:  true,
		MaxCalls: 100,
	}
	hcm := NewHighCardinalityMetrics(config)

	cm1 := hcm.GetOrCreateCallMetrics("call-1", "from", "to")
	cm2 := hcm.GetOrCreateCallMetrics("call-2", "from", "to")

	cm1.RecordPacketReceived(100)
	cm1.RecordPacketReceived(100)
	cm2.RecordPacketReceived(100)
	cm1.RecordPacketLost()
	cm2.RecordJitter(1000)
	cm2.RecordJitter(2000)

	summary := hcm.GetSummary()

	if summary.ActiveCalls != 2 {
		t.Errorf("expected 2 active calls, got %d", summary.ActiveCalls)
	}
	if summary.TotalPacketsRx != 3 {
		t.Errorf("expected 3 packets rx, got %d", summary.TotalPacketsRx)
	}
	if summary.TotalBytesRx != 300 {
		t.Errorf("expected 300 bytes rx, got %d", summary.TotalBytesRx)
	}
	if summary.TotalPacketsLost != 1 {
		t.Errorf("expected 1 packet lost, got %d", summary.TotalPacketsLost)
	}
}

func TestHighCardinalityMetrics_SetExportCallback(t *testing.T) {
	config := &HighCardinalityMetricsConfig{
		Enabled:        true,
		MaxCalls:       100,
		ExportInterval: 50 * time.Millisecond,
	}
	hcm := NewHighCardinalityMetrics(config)

	var exported []*CallMetrics
	var mu sync.Mutex

	hcm.SetExportCallback(func(metrics []*CallMetrics) {
		mu.Lock()
		exported = append(exported, metrics...)
		mu.Unlock()
	})

	hcm.Start()
	defer hcm.Stop()

	hcm.GetOrCreateCallMetrics("call-1", "from", "to")
	hcm.RemoveCall("call-1", "from", "to")

	// Wait for export
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	count := len(exported)
	mu.Unlock()

	if count != 1 {
		t.Errorf("expected 1 exported metric, got %d", count)
	}
}

func TestHighCardinalityMetrics_ConcurrentAccess(t *testing.T) {
	config := &HighCardinalityMetricsConfig{
		Enabled:  true,
		MaxCalls: 10000,
	}
	hcm := NewHighCardinalityMetrics(config)

	var wg sync.WaitGroup
	numGoroutines := 10
	callsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < callsPerGoroutine; j++ {
				callID := string(rune('A' + id))
				cm := hcm.GetOrCreateCallMetrics(callID, "from", "to")
				if cm != nil {
					cm.RecordPacketReceived(100)
					cm.RecordJitter(1000)
				}
			}
		}(i)
	}

	wg.Wait()

	// All calls should be tracked (one per goroutine since same call ID)
	if hcm.GetActiveCallCount() != numGoroutines {
		t.Errorf("expected %d active calls, got %d", numGoroutines, hcm.GetActiveCallCount())
	}
}

func TestHighCardinalityMetrics_SetEnabled(t *testing.T) {
	hcm := NewHighCardinalityMetrics(nil)

	if hcm.IsEnabled() {
		t.Error("expected disabled by default")
	}

	hcm.SetEnabled(true)
	if !hcm.IsEnabled() {
		t.Error("expected enabled after SetEnabled(true)")
	}

	hcm.SetEnabled(false)
	if hcm.IsEnabled() {
		t.Error("expected disabled after SetEnabled(false)")
	}
}

func TestCallMetrics_Duration(t *testing.T) {
	cm := NewCallMetrics("call-1", "from", "to")

	// Duration during call
	time.Sleep(10 * time.Millisecond)
	duration := cm.Duration()
	if duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", duration)
	}

	// Duration after call end
	cm.EndTime = cm.StartTime.Add(100 * time.Millisecond)
	duration = cm.Duration()
	if duration != 100*time.Millisecond {
		t.Errorf("expected duration = 100ms, got %v", duration)
	}
}

func TestCallMetrics_Recording(t *testing.T) {
	cm := NewCallMetrics("call-1", "from", "to")
	cm.RecordingEnabled = true

	cm.RecordRecording(1000)
	cm.RecordRecording(2000)

	if cm.RecordingBytes.Load() != 3000 {
		t.Errorf("expected 3000 recording bytes, got %d", cm.RecordingBytes.Load())
	}
}

func TestCallMetrics_ZeroValues(t *testing.T) {
	cm := NewCallMetrics("call-1", "from", "to")

	// All calculations should be safe with zero values
	if cm.GetAverageJitter() != 0 {
		t.Error("expected 0 avg jitter with no samples")
	}
	if cm.GetAverageLatency() != 0 {
		t.Error("expected 0 avg latency with no samples")
	}
	if cm.GetPacketLossRate() != 0 {
		t.Error("expected 0 packet loss with no packets")
	}
}
