package internal

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackpressureStrategy_String(t *testing.T) {
	tests := []struct {
		strategy BackpressureStrategy
		expected string
	}{
		{StrategyBlock, "block"},
		{StrategyDrop, "drop"},
		{StrategyDropOldest, "drop_oldest"},
		{StrategyReject, "reject"},
		{BackpressureStrategy(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.strategy.String(); got != tt.expected {
			t.Errorf("BackpressureStrategy(%d).String() = %s, expected %s", tt.strategy, got, tt.expected)
		}
	}
}

func TestDefaultBackpressureConfig(t *testing.T) {
	config := DefaultBackpressureConfig()

	if config.MaxQueueSize != 10000 {
		t.Errorf("Expected MaxQueueSize 10000, got %d", config.MaxQueueSize)
	}
	if config.Strategy != StrategyDropOldest {
		t.Errorf("Expected Strategy DropOldest, got %s", config.Strategy.String())
	}
}

func TestNewBackpressureController(t *testing.T) {
	bp := NewBackpressureController(nil)

	if bp == nil {
		t.Fatal("NewBackpressureController returned nil")
	}
	if bp.queueSize.Load() != 0 {
		t.Errorf("Expected initial queue size 0, got %d", bp.queueSize.Load())
	}
}

func TestBackpressureController_AcquireRelease(t *testing.T) {
	config := &BackpressureConfig{
		MaxQueueSize: 10,
		Strategy:     StrategyReject,
	}
	bp := NewBackpressureController(config)
	ctx := context.Background()

	// Acquire should succeed
	err := bp.Acquire(ctx)
	if err != nil {
		t.Errorf("Acquire should succeed, got %v", err)
	}

	if bp.GetQueueSize() != 1 {
		t.Errorf("Expected queue size 1, got %d", bp.GetQueueSize())
	}

	// Release
	bp.Release()

	if bp.GetQueueSize() != 0 {
		t.Errorf("Expected queue size 0 after release, got %d", bp.GetQueueSize())
	}
}

func TestBackpressureController_RejectStrategy(t *testing.T) {
	config := &BackpressureConfig{
		MaxQueueSize: 2,
		Strategy:     StrategyReject,
	}
	bp := NewBackpressureController(config)
	ctx := context.Background()

	// Fill the queue
	bp.Acquire(ctx)
	bp.Acquire(ctx)

	// Third should be rejected
	err := bp.Acquire(ctx)
	if err != ErrBackpressure {
		t.Errorf("Expected ErrBackpressure, got %v", err)
	}
}

func TestBackpressureController_DropStrategy(t *testing.T) {
	config := &BackpressureConfig{
		MaxQueueSize: 2,
		Strategy:     StrategyDrop,
	}
	bp := NewBackpressureController(config)
	ctx := context.Background()

	// Fill the queue
	bp.Acquire(ctx)
	bp.Acquire(ctx)

	// Third should be dropped
	err := bp.Acquire(ctx)
	if err != ErrBackpressure {
		t.Errorf("Expected ErrBackpressure, got %v", err)
	}

	stats := bp.GetStats()
	if stats["total_dropped"].(int64) != 1 {
		t.Errorf("Expected 1 dropped, got %v", stats["total_dropped"])
	}
}

func TestBackpressureController_DropOldestStrategy(t *testing.T) {
	config := &BackpressureConfig{
		MaxQueueSize: 2,
		Strategy:     StrategyDropOldest,
	}
	bp := NewBackpressureController(config)
	ctx := context.Background()

	// Fill beyond capacity (drop_oldest always accepts)
	bp.Acquire(ctx)
	bp.Acquire(ctx)
	bp.Acquire(ctx) // Still succeeds

	// Check ShouldDrop
	if !bp.ShouldDrop() {
		t.Error("ShouldDrop should return true when over capacity")
	}

	// Notify dropped
	bp.NotifyDropped()

	if bp.GetQueueSize() != 2 {
		t.Errorf("Expected queue size 2 after drop, got %d", bp.GetQueueSize())
	}
}

func TestBackpressureController_BlockStrategy(t *testing.T) {
	config := &BackpressureConfig{
		MaxQueueSize: 1,
		Strategy:     StrategyBlock,
		BlockTimeout: 100 * time.Millisecond,
	}
	bp := NewBackpressureController(config)
	ctx := context.Background()

	// Fill the queue
	bp.Acquire(ctx)

	// Start a goroutine that will release after a delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		bp.Release()
	}()

	// This should block then succeed
	err := bp.Acquire(ctx)
	if err != nil {
		t.Errorf("Acquire should succeed after release, got %v", err)
	}
}

func TestBackpressureController_BlockStrategyTimeout(t *testing.T) {
	config := &BackpressureConfig{
		MaxQueueSize: 1,
		Strategy:     StrategyBlock,
		BlockTimeout: 50 * time.Millisecond,
	}
	bp := NewBackpressureController(config)
	ctx := context.Background()

	// Fill the queue
	bp.Acquire(ctx)

	// This should timeout
	start := time.Now()
	err := bp.Acquire(ctx)
	elapsed := time.Since(start)

	if err != ErrBackpressure {
		t.Errorf("Expected ErrBackpressure after timeout, got %v", err)
	}
	if elapsed < 50*time.Millisecond {
		t.Errorf("Should have waited for timeout, elapsed: %v", elapsed)
	}
}

func TestBackpressureController_BlockStrategyContextCancel(t *testing.T) {
	config := &BackpressureConfig{
		MaxQueueSize: 1,
		Strategy:     StrategyBlock,
		BlockTimeout: time.Hour,
	}
	bp := NewBackpressureController(config)
	ctx, cancel := context.WithCancel(context.Background())

	// Fill the queue
	bp.Acquire(ctx)

	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// This should return when context is cancelled
	err := bp.Acquire(ctx)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestBackpressureController_HighLowWaterMarks(t *testing.T) {
	config := &BackpressureConfig{
		MaxQueueSize:  10,
		Strategy:      StrategyReject,
		HighWaterMark: 0.8,
		LowWaterMark:  0.6,
	}
	bp := NewBackpressureController(config)
	ctx := context.Background()

	// Add 8 items (80% = high water mark)
	for i := 0; i < 8; i++ {
		bp.Acquire(ctx)
	}

	if !bp.IsUnderPressure() {
		t.Error("Should be under pressure at high water mark")
	}

	// Release 3 items (50% < low water mark)
	bp.Release()
	bp.Release()
	bp.Release()

	if bp.IsUnderPressure() {
		t.Error("Should not be under pressure below low water mark")
	}
}

func TestBackpressureController_GetQueueUtilization(t *testing.T) {
	config := &BackpressureConfig{
		MaxQueueSize: 10,
		Strategy:     StrategyReject,
	}
	bp := NewBackpressureController(config)
	ctx := context.Background()

	// Add 5 items
	for i := 0; i < 5; i++ {
		bp.Acquire(ctx)
	}

	util := bp.GetQueueUtilization()
	if util != 0.5 {
		t.Errorf("Expected utilization 0.5, got %f", util)
	}
}

func TestBackpressureController_GetStats(t *testing.T) {
	bp := NewBackpressureController(nil)
	ctx := context.Background()

	bp.Acquire(ctx)
	bp.Release()

	stats := bp.GetStats()

	if stats["queue_size"].(int64) != 0 {
		t.Errorf("Expected queue_size 0, got %v", stats["queue_size"])
	}
	if stats["total_enqueued"].(int64) != 1 {
		t.Errorf("Expected total_enqueued 1, got %v", stats["total_enqueued"])
	}
	if stats["total_dequeued"].(int64) != 1 {
		t.Errorf("Expected total_dequeued 1, got %v", stats["total_dequeued"])
	}
}

func TestBackpressureController_Close(t *testing.T) {
	config := &BackpressureConfig{
		MaxQueueSize: 1,
		Strategy:     StrategyBlock,
	}
	bp := NewBackpressureController(config)
	ctx := context.Background()

	bp.Acquire(ctx)

	// Start a blocking acquire
	done := make(chan error)
	go func() {
		done <- bp.Acquire(ctx)
	}()

	// Close should unblock
	time.Sleep(50 * time.Millisecond)
	bp.Close()

	select {
	case err := <-done:
		if err != ErrQueueClosed {
			t.Errorf("Expected ErrQueueClosed, got %v", err)
		}
	case <-time.After(time.Second):
		t.Error("Close should have unblocked the acquire")
	}
}

func TestBackpressureController_ConcurrentAccess(t *testing.T) {
	config := &BackpressureConfig{
		MaxQueueSize: 100,
		Strategy:     StrategyReject,
	}
	bp := NewBackpressureController(config)
	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if bp.Acquire(ctx) == nil {
					bp.Release()
				}
				bp.IsUnderPressure()
				bp.GetStats()
			}
		}()
	}

	wg.Wait()

	if bp.GetQueueSize() != 0 {
		t.Errorf("Expected queue size 0, got %d", bp.GetQueueSize())
	}
}

func TestNewRecordingBackpressure(t *testing.T) {
	rb := NewRecordingBackpressure(nil)

	if rb == nil {
		t.Fatal("NewRecordingBackpressure returned nil")
	}
}

func TestRecordingBackpressure_AcquireReleaseBuffer(t *testing.T) {
	rb := NewRecordingBackpressure(nil)
	ctx := context.Background()

	buf, err := rb.AcquireBuffer(ctx, "session1")
	if err != nil {
		t.Errorf("AcquireBuffer should succeed, got %v", err)
	}
	if buf == nil {
		t.Error("Buffer should not be nil")
	}

	rb.ReleaseBuffer(buf, "session1")
}

func TestRecordingBackpressure_SessionTracking(t *testing.T) {
	rb := NewRecordingBackpressure(nil)
	ctx := context.Background()

	rb.AcquireBuffer(ctx, "session1")
	rb.AcquireBuffer(ctx, "session2")

	stats := rb.GetStats()
	if stats["session_count"].(int) != 2 {
		t.Errorf("Expected 2 sessions, got %v", stats["session_count"])
	}

	rb.RemoveSession("session1")

	stats = rb.GetStats()
	if stats["session_count"].(int) != 1 {
		t.Errorf("Expected 1 session after removal, got %v", stats["session_count"])
	}
}

func TestRecordingBackpressure_ShouldDropOldest(t *testing.T) {
	config := &BackpressureConfig{
		MaxQueueSize: 2,
		Strategy:     StrategyDropOldest,
	}
	rb := NewRecordingBackpressure(config)
	ctx := context.Background()

	rb.AcquireBuffer(ctx, "session1")
	rb.AcquireBuffer(ctx, "session1")
	rb.AcquireBuffer(ctx, "session1")

	if !rb.ShouldDropOldest() {
		t.Error("ShouldDropOldest should return true when over capacity")
	}
}

func TestRecordingBackpressure_NotifyDropped(t *testing.T) {
	rb := NewRecordingBackpressure(nil)

	rb.NotifyDropped("session1")

	stats := rb.GetStats()
	if stats["total_dropped"].(int64) != 1 {
		t.Errorf("Expected 1 dropped, got %v", stats["total_dropped"])
	}
}

func TestRecordingBackpressure_Close(t *testing.T) {
	rb := NewRecordingBackpressure(nil)
	rb.Close()

	ctx := context.Background()
	_, err := rb.AcquireBuffer(ctx, "session1")
	if err != ErrQueueClosed {
		t.Errorf("Expected ErrQueueClosed after close, got %v", err)
	}
}

func TestNewMediaBackpressure(t *testing.T) {
	mb := NewMediaBackpressure(100, 1000)

	if mb == nil {
		t.Fatal("NewMediaBackpressure returned nil")
	}
}

func TestMediaBackpressure_AllowPacket(t *testing.T) {
	mb := NewMediaBackpressure(5, 10)

	// Should allow initially
	for i := 0; i < 5; i++ {
		if !mb.AllowPacket("stream1") {
			t.Errorf("Packet %d should be allowed", i)
		}
	}

	// Should throttle after limit
	if mb.AllowPacket("stream1") {
		t.Error("Should be throttled after limit")
	}
}

func TestMediaBackpressure_GlobalLimit(t *testing.T) {
	mb := NewMediaBackpressure(5, 8)

	// Add packets to two streams
	for i := 0; i < 5; i++ {
		mb.AllowPacket("stream1")
	}
	for i := 0; i < 3; i++ {
		mb.AllowPacket("stream2")
	}

	// Global limit reached
	if mb.AllowPacket("stream2") {
		t.Error("Should be rejected at global limit")
	}
}

func TestMediaBackpressure_PacketProcessed(t *testing.T) {
	mb := NewMediaBackpressure(4, 10) // Larger limit to allow throttle release

	// Fill to limit (4 packets)
	for i := 0; i < 4; i++ {
		mb.AllowPacket("stream1")
	}

	// 5th packet exceeds limit and triggers throttle
	if mb.AllowPacket("stream1") {
		t.Error("Should be throttled at limit")
	}

	// After 5th packet, we have 5 packets but throttle is set
	// Process 4 packets to get to 1 packet remaining
	// Throttle releases when count < maxPacketsPerStream/2 = 4/2 = 2
	// So we need to get below 2 (i.e., to 1 or 0)
	mb.PacketProcessed("stream1") // 5->4
	mb.PacketProcessed("stream1") // 4->3
	mb.PacketProcessed("stream1") // 3->2
	mb.PacketProcessed("stream1") // 2->1, now 1 < 2, throttle released

	// Should allow again (below threshold)
	if !mb.AllowPacket("stream1") {
		t.Error("Should allow after processing")
	}
}

func TestMediaBackpressure_RemoveStream(t *testing.T) {
	mb := NewMediaBackpressure(5, 10)

	mb.AllowPacket("stream1")
	mb.AllowPacket("stream1")

	mb.RemoveStream("stream1")

	stats := mb.GetStats()
	if stats["stream_count"].(int) != 0 {
		t.Errorf("Expected 0 streams, got %v", stats["stream_count"])
	}
}

func TestMediaBackpressure_GetStats(t *testing.T) {
	mb := NewMediaBackpressure(5, 10)

	mb.AllowPacket("stream1")
	mb.AllowPacket("stream2")

	stats := mb.GetStats()

	if stats["total_packets"].(int64) != 2 {
		t.Errorf("Expected 2 total_packets, got %v", stats["total_packets"])
	}
	if stats["stream_count"].(int) != 2 {
		t.Errorf("Expected 2 streams, got %v", stats["stream_count"])
	}
}

func TestMediaBackpressure_ConcurrentAccess(t *testing.T) {
	mb := NewMediaBackpressure(100, 1000)

	var wg sync.WaitGroup
	var allowed atomic.Int64
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(streamID string) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if mb.AllowPacket(streamID) {
					allowed.Add(1)
					mb.PacketProcessed(streamID)
				}
			}
		}(string(rune('a' + i%10)))
	}

	wg.Wait()
}

func TestNewAdaptiveBackpressure(t *testing.T) {
	ab := NewAdaptiveBackpressure(nil)

	if ab == nil {
		t.Fatal("NewAdaptiveBackpressure returned nil")
	}
}

func TestAdaptiveBackpressure_UpdateSystemMetrics(t *testing.T) {
	config := &BackpressureConfig{
		MaxQueueSize: 100,
		Strategy:     StrategyReject,
	}
	ab := NewAdaptiveBackpressure(config)

	// Initially full capacity
	if ab.GetEffectiveCapacity() != 100 {
		t.Errorf("Expected initial capacity 100, got %d", ab.GetEffectiveCapacity())
	}

	// High CPU usage should reduce capacity
	ab.UpdateSystemMetrics(90, 50)

	capacity := ab.GetEffectiveCapacity()
	if capacity >= 100 {
		t.Errorf("Capacity should be reduced under high CPU, got %d", capacity)
	}
}

func TestAdaptiveBackpressure_MemoryPressure(t *testing.T) {
	config := &BackpressureConfig{
		MaxQueueSize: 100,
		Strategy:     StrategyReject,
	}
	ab := NewAdaptiveBackpressure(config)

	// High memory usage should reduce capacity
	ab.UpdateSystemMetrics(50, 95)

	capacity := ab.GetEffectiveCapacity()
	if capacity >= 100 {
		t.Errorf("Capacity should be reduced under high memory, got %d", capacity)
	}
}

func TestAdaptiveBackpressure_AcquireRelease(t *testing.T) {
	config := &BackpressureConfig{
		MaxQueueSize: 10,
		Strategy:     StrategyReject,
	}
	ab := NewAdaptiveBackpressure(config)
	ctx := context.Background()

	err := ab.Acquire(ctx)
	if err != nil {
		t.Errorf("Acquire should succeed, got %v", err)
	}

	ab.Release()
}

func TestAdaptiveBackpressure_RejectsUnderPressure(t *testing.T) {
	config := &BackpressureConfig{
		MaxQueueSize: 100,
		Strategy:     StrategyReject,
	}
	ab := NewAdaptiveBackpressure(config)
	ctx := context.Background()

	// Simulate extreme pressure
	ab.UpdateSystemMetrics(95, 95)

	// Effective capacity should be reduced from 100
	capacity := ab.GetEffectiveCapacity()
	if capacity >= 100 {
		t.Errorf("Expected reduced capacity, got %d", capacity)
	}

	// Fill to effective capacity
	for i := int64(0); i < capacity; i++ {
		if err := ab.Acquire(ctx); err != nil {
			// If we get an error early, that's fine - capacity was already filled
			break
		}
	}

	// Should reject at or above effective capacity
	err := ab.Acquire(ctx)
	if err != ErrBackpressure {
		t.Errorf("Should reject at effective capacity, got %v", err)
	}
}

func TestAdaptiveBackpressure_GetStats(t *testing.T) {
	ab := NewAdaptiveBackpressure(nil)

	ab.UpdateSystemMetrics(75, 60)

	stats := ab.GetStats()

	if stats["cpu_usage"].(float64) != 75 {
		t.Errorf("Expected cpu_usage 75, got %v", stats["cpu_usage"])
	}
	if stats["memory_usage"].(float64) != 60 {
		t.Errorf("Expected memory_usage 60, got %v", stats["memory_usage"])
	}
	if stats["effective_capacity"] == nil {
		t.Error("Missing effective_capacity in stats")
	}
}

func TestAdaptiveBackpressure_Close(t *testing.T) {
	ab := NewAdaptiveBackpressure(nil)
	ab.Close()

	ctx := context.Background()
	err := ab.Acquire(ctx)
	if err != ErrQueueClosed {
		t.Errorf("Expected ErrQueueClosed after close, got %v", err)
	}
}

func TestBackpressure_IntegrationScenario(t *testing.T) {
	config := &BackpressureConfig{
		MaxQueueSize:  100,
		Strategy:      StrategyDropOldest,
		HighWaterMark: 0.8,
		LowWaterMark:  0.6,
	}
	rb := NewRecordingBackpressure(config)
	ctx := context.Background()

	var wg sync.WaitGroup
	var acquired, dropped atomic.Int64

	// Producer
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(sessionID string) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				buf, err := rb.AcquireBuffer(ctx, sessionID)
				if err != nil {
					dropped.Add(1)
					continue
				}
				acquired.Add(1)

				// Simulate work
				time.Sleep(time.Microsecond)

				// Check if we should drop oldest
				if rb.ShouldDropOldest() {
					rb.NotifyDropped(sessionID)
					dropped.Add(1)
				}

				rb.ReleaseBuffer(buf, sessionID)
			}
		}("session-" + string(rune('a'+i)))
	}

	wg.Wait()

	stats := rb.GetStats()
	t.Logf("Stats: acquired=%d, dropped=%d, final_queue=%v",
		acquired.Load(), dropped.Load(), stats["queue_size"])
}
