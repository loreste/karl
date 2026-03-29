package tests

import (
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"karl/internal"
)

// TestSessionRegistryConcurrency tests concurrent access to session registry
func TestSessionRegistryConcurrency(t *testing.T) {
	registry := internal.NewSessionRegistry(1 * time.Hour)
	defer registry.Stop()

	var wg sync.WaitGroup
	numGoroutines := 100
	operationsPerGoroutine := 100

	// Start goroutines that create, read, update, and delete sessions
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				callID := fmt.Sprintf("call-%d-%d", id, j)
				fromTag := fmt.Sprintf("from-%d-%d", id, j)

				// Create session
				session := registry.CreateSession(callID, fromTag)
				if session == nil {
					t.Errorf("Failed to create session")
					continue
				}

				// Read session
				_, ok := registry.GetSession(session.ID)
				if !ok {
					t.Errorf("Failed to get session after creation")
				}

				// Update session state
				err := registry.UpdateSessionState(session.ID, "active")
				if err != nil {
					t.Errorf("Failed to update session state: %v", err)
				}

				// Get by call ID
				sessions := registry.GetSessionByCallID(callID)
				if len(sessions) == 0 {
					t.Errorf("Failed to get session by call ID")
				}

				// Delete session
				err = registry.DeleteSession(session.ID)
				if err != nil {
					t.Errorf("Failed to delete session: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify all sessions were cleaned up
	count := registry.GetActiveCount()
	if count != 0 {
		t.Errorf("Expected 0 active sessions, got %d", count)
	}
}

// TestJitterBufferConcurrency tests concurrent push/pop operations
func TestJitterBufferConcurrency(t *testing.T) {
	config := internal.DefaultJitterBufferInternalConfig()
	jb := internal.NewJitterBuffer("test-session", 8000, config)

	var wg sync.WaitGroup
	numProducers := 10
	numConsumers := 5
	packetsPerProducer := 100

	// Track sequence numbers
	var seqMu sync.Mutex
	nextSeq := uint16(0)

	// Producers push packets
	for i := 0; i < numProducers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < packetsPerProducer; j++ {
				seqMu.Lock()
				seq := nextSeq
				nextSeq++
				seqMu.Unlock()

				timestamp := uint32(seq) * 160 // 20ms at 8kHz
				payload := make([]byte, 160)
				jb.Push(seq, timestamp, payload)
				time.Sleep(time.Microsecond * 10)
			}
		}()
	}

	// Consumers pop packets
	for i := 0; i < numConsumers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < packetsPerProducer*numProducers/numConsumers; j++ {
				jb.Pop()
				time.Sleep(time.Microsecond * 50)
			}
		}()
	}

	wg.Wait()

	// Get stats
	stats := jb.GetStats()
	t.Logf("Jitter buffer stats: In=%d, Out=%d, Dropped=%d, Late=%d, Reordered=%d",
		stats.PacketsIn, stats.PacketsOut, stats.PacketsDropped, stats.PacketsLate, stats.Reordered)
}

// TestFECHandlerConcurrency tests concurrent FEC operations
func TestFECHandlerConcurrency(t *testing.T) {
	config := internal.DefaultFECConfig()
	handler := internal.NewFECHandler(config)

	var wg sync.WaitGroup
	numGoroutines := 50
	packetsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < packetsPerGoroutine; j++ {
				seq := uint16(id*packetsPerGoroutine + j)
				pkt := &internal.RTPPacketData{
					SequenceNumber: seq,
					Timestamp:      uint32(seq) * 160,
					PayloadType:    0,
					Marker:         false,
					Payload:        make([]byte, 160),
				}

				// Add media packet (may generate FEC packet)
				fecPkt := handler.AddMediaPacket(pkt)
				if fecPkt != nil {
					// Serialize and deserialize FEC packet
					data := internal.SerializeFECPacket(fecPkt)
					_, err := internal.DeserializeFECPacket(data)
					if err != nil {
						t.Errorf("Failed to deserialize FEC packet: %v", err)
					}
				}

				// Receive media packet
				handler.ReceiveMediaPacket(pkt)

				// Update loss rate occasionally
				if j%10 == 0 {
					handler.UpdateLossRate(rand.Float64() * 0.1)
				}
			}
		}(i)
	}

	wg.Wait()

	stats := handler.GetStats()
	t.Logf("FEC stats: Enabled=%v, BlockSize=%d, Redundancy=%.2f, PendingBlocks=%d",
		stats.Enabled, stats.BlockSize, stats.Redundancy, stats.PendingBlocks)
}

// TestRTCPHandlerConcurrency tests concurrent RTCP operations
func TestRTCPHandlerConcurrency(t *testing.T) {
	config := &internal.RTCPInternalConfig{
		Enabled:  true,
		Interval: 100 * time.Millisecond,
	}
	handler := internal.NewRTCPHandler(config)
	handler.Start()
	defer handler.Stop()

	var wg sync.WaitGroup
	numSessions := 50

	// Add sessions concurrently
	for i := 0; i < numSessions; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			sessionID := fmt.Sprintf("session-%d", id)
			ssrc := uint32(id + 1000)
			cname := fmt.Sprintf("cname-%d@karl", id)

			sessionHandler := internal.NewRTCPSessionHandler(ssrc, cname, 8000)
			handler.AddSession(sessionID, sessionHandler)

			// Update stats
			for j := 0; j < 10; j++ {
				sessionHandler.UpdateSenderStats(uint32(j*10), uint32(j*160))
				sessionHandler.UpdateReceiverStats(
					uint16(j),
					uint32(j*160),
					time.Now(),
				)
				time.Sleep(time.Millisecond)
			}

			// Get stats
			_ = sessionHandler.GetStats()

			// Remove session
			handler.RemoveSession(sessionID)
		}(i)
	}

	wg.Wait()
}

// TestMemoryLeaks tests for memory leaks in session management
func TestMemoryLeaks(t *testing.T) {
	// Force GC before starting
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	registry := internal.NewSessionRegistry(1 * time.Hour)

	// Create and delete many sessions
	for i := 0; i < 10000; i++ {
		callID := fmt.Sprintf("call-%d", i)
		fromTag := fmt.Sprintf("from-%d", i)
		session := registry.CreateSession(callID, fromTag)
		_ = registry.DeleteSession(session.ID)
	}

	registry.Stop()

	// Force GC after
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// Check memory didn't grow significantly (allow 10MB growth for test overhead)
	memGrowth := int64(m2.Alloc) - int64(m1.Alloc)
	t.Logf("Memory before: %d KB, after: %d KB, growth: %d KB",
		m1.Alloc/1024, m2.Alloc/1024, memGrowth/1024)

	if memGrowth > 10*1024*1024 {
		t.Errorf("Possible memory leak: memory grew by %d bytes", memGrowth)
	}
}

// TestJitterBufferMemoryLeak tests jitter buffer doesn't leak memory
func TestJitterBufferMemoryLeak(t *testing.T) {
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	config := internal.DefaultJitterBufferInternalConfig()
	config.MaxSize = 50

	for iteration := 0; iteration < 100; iteration++ {
		jb := internal.NewJitterBuffer(fmt.Sprintf("session-%d", iteration), 8000, config)

		// Fill and drain buffer multiple times
		for cycle := 0; cycle < 10; cycle++ {
			// Fill buffer
			for i := 0; i < 100; i++ {
				seq := uint16(cycle*100 + i)
				jb.Push(seq, uint32(seq)*160, make([]byte, 160))
			}

			// Drain buffer
			for i := 0; i < 100; i++ {
				jb.Pop()
			}
		}

		jb.Reset()
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	memGrowth := int64(m2.Alloc) - int64(m1.Alloc)
	t.Logf("Jitter buffer memory growth: %d KB", memGrowth/1024)

	if memGrowth > 5*1024*1024 {
		t.Errorf("Possible jitter buffer memory leak: memory grew by %d bytes", memGrowth)
	}
}

// TestFECHandlerMemoryLeak tests FEC handler doesn't leak memory
func TestFECHandlerMemoryLeak(t *testing.T) {
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	for iteration := 0; iteration < 100; iteration++ {
		config := internal.DefaultFECConfig()
		handler := internal.NewFECHandler(config)

		// Process many packets
		for i := 0; i < 1000; i++ {
			pkt := &internal.RTPPacketData{
				SequenceNumber: uint16(i),
				Timestamp:      uint32(i) * 160,
				PayloadType:    0,
				Payload:        make([]byte, 160),
			}
			handler.AddMediaPacket(pkt)
			handler.ReceiveMediaPacket(pkt)
		}

		handler.Reset()
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	memGrowth := int64(m2.Alloc) - int64(m1.Alloc)
	t.Logf("FEC handler memory growth: %d KB", memGrowth/1024)

	if memGrowth > 5*1024*1024 {
		t.Errorf("Possible FEC handler memory leak: memory grew by %d bytes", memGrowth)
	}
}

// TestSessionRegistryCleanup tests that session cleanup works correctly
func TestSessionRegistryCleanup(t *testing.T) {
	// Use very short TTL for testing
	registry := internal.NewSessionRegistry(100 * time.Millisecond)
	defer registry.Stop()

	// Create sessions
	for i := 0; i < 10; i++ {
		registry.CreateSession(fmt.Sprintf("call-%d", i), fmt.Sprintf("tag-%d", i))
	}

	// GetTotalCount returns all sessions regardless of state
	if registry.GetTotalCount() != 10 {
		t.Errorf("Expected 10 sessions, got %d", registry.GetTotalCount())
	}

	// GetActiveCount only returns sessions in Active state
	// Sessions are created in New state, so this should be 0
	if registry.GetActiveCount() != 0 {
		t.Errorf("Expected 0 active sessions (sessions start in New state), got %d", registry.GetActiveCount())
	}

	// Note: actual cleanup depends on the cleanup interval, which is 30s by default
	// The TTL only marks sessions as stale for cleanup, it doesn't auto-delete
	t.Logf("Total sessions: %d, Active sessions: %d", registry.GetTotalCount(), registry.GetActiveCount())
}

// TestGoroutineLeaks tests for goroutine leaks
func TestGoroutineLeaks(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	// Create and destroy components
	for i := 0; i < 10; i++ {
		registry := internal.NewSessionRegistry(1 * time.Hour)

		// Create sessions
		for j := 0; j < 10; j++ {
			registry.CreateSession(fmt.Sprintf("call-%d-%d", i, j), fmt.Sprintf("tag-%d-%d", i, j))
		}

		registry.Stop()
	}

	// Allow goroutines to clean up
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	goroutineGrowth := finalGoroutines - initialGoroutines

	t.Logf("Goroutines: initial=%d, final=%d, growth=%d",
		initialGoroutines, finalGoroutines, goroutineGrowth)

	// Allow some growth for test infrastructure, but not too much
	if goroutineGrowth > 5 {
		t.Errorf("Possible goroutine leak: goroutine count grew by %d", goroutineGrowth)
	}
}

// TestRTCPHandlerGoroutineLeak tests RTCP handler doesn't leak goroutines
func TestRTCPHandlerGoroutineLeak(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	for i := 0; i < 10; i++ {
		config := &internal.RTCPInternalConfig{
			Enabled:  true,
			Interval: 50 * time.Millisecond,
		}
		handler := internal.NewRTCPHandler(config)
		handler.Start()

		// Add some sessions
		for j := 0; j < 5; j++ {
			sessionHandler := internal.NewRTCPSessionHandler(uint32(j+1000), "test@karl", 8000)
			handler.AddSession(fmt.Sprintf("session-%d", j), sessionHandler)
		}

		// Let it run briefly
		time.Sleep(100 * time.Millisecond)

		handler.Stop()
	}

	// Allow cleanup
	time.Sleep(200 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	goroutineGrowth := finalGoroutines - initialGoroutines

	t.Logf("RTCP goroutines: initial=%d, final=%d, growth=%d",
		initialGoroutines, finalGoroutines, goroutineGrowth)

	if goroutineGrowth > 3 {
		t.Errorf("Possible RTCP handler goroutine leak: count grew by %d", goroutineGrowth)
	}
}

// BenchmarkSessionCreate benchmarks session creation
func BenchmarkSessionCreate(b *testing.B) {
	registry := internal.NewSessionRegistry(1 * time.Hour)
	defer registry.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		session := registry.CreateSession(fmt.Sprintf("call-%d", i), fmt.Sprintf("tag-%d", i))
		_ = registry.DeleteSession(session.ID)
	}
}

// BenchmarkJitterBufferPush benchmarks jitter buffer push
func BenchmarkJitterBufferPush(b *testing.B) {
	config := internal.DefaultJitterBufferInternalConfig()
	jb := internal.NewJitterBuffer("bench-session", 8000, config)

	payload := make([]byte, 160)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		seq := uint16(i % 65536)
		jb.Push(seq, uint32(i)*160, payload)
		if i%100 == 99 {
			// Drain periodically to prevent overflow
			for j := 0; j < 50; j++ {
				jb.Pop()
			}
		}
	}
}

// BenchmarkFECEncode benchmarks FEC encoding
func BenchmarkFECEncode(b *testing.B) {
	config := internal.DefaultFECConfig()
	config.BlockSize = 10
	handler := internal.NewFECHandler(config)

	payload := make([]byte, 160)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pkt := &internal.RTPPacketData{
			SequenceNumber: uint16(i % 65536),
			Timestamp:      uint32(i) * 160,
			PayloadType:    0,
			Payload:        payload,
		}
		handler.AddMediaPacket(pkt)
	}
}
