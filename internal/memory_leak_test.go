package internal

import (
	"runtime"
	"runtime/debug"
	"sync"
	"testing"
	"time"
)

// TestMemoryLeak_SessionManager tests that session creation/deletion doesn't leak memory
func TestMemoryLeak_SessionManager(t *testing.T) {
	// Force GC and get baseline
	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Create and delete many sessions
	iterations := 1000
	for i := 0; i < iterations; i++ {
		sm := newTestStateMachine("test-call-" + string(rune(i)))
		sm.processOffer("from-tag")
		sm.processAnswer("from-tag", "to-tag", false)
		// Let it go out of scope
	}

	// Force GC
	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// Allow some variance but check for significant leaks
	// Heap should not grow significantly
	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	maxAllowedGrowth := int64(10 * 1024 * 1024) // 10 MB

	if heapGrowth > maxAllowedGrowth {
		t.Errorf("Potential memory leak: heap grew by %d bytes after %d session cycles",
			heapGrowth, iterations)
	}
}

// TestMemoryLeak_BufferPool tests that buffer pool doesn't leak
func TestMemoryLeak_BufferPool(t *testing.T) {
	pool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, 2048)
		},
	}

	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Get and put many buffers
	iterations := 10000
	for i := 0; i < iterations; i++ {
		buf := pool.Get().([]byte)
		// Use the buffer
		for j := 0; j < len(buf); j++ {
			buf[j] = byte(j % 256)
		}
		pool.Put(buf)
	}

	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	maxAllowedGrowth := int64(5 * 1024 * 1024) // 5 MB

	if heapGrowth > maxAllowedGrowth {
		t.Errorf("Buffer pool may be leaking: heap grew by %d bytes", heapGrowth)
	}
}

// TestMemoryLeak_Codecs tests that codec encode/decode doesn't leak
func TestMemoryLeak_Codecs(t *testing.T) {
	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	iterations := 1000
	samples := make([]int16, 160)
	for i := range samples {
		samples[i] = int16(i * 100)
	}

	for i := 0; i < iterations; i++ {
		// Test G.711 μ-law
		for j := range samples {
			encoded := LinearToMulaw(samples[j])
			_ = MulawToLinear(encoded)
		}

		// Test G.711 A-law
		for j := range samples {
			encoded := LinearToAlaw(samples[j])
			_ = AlawToLinear(encoded)
		}
	}

	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	maxAllowedGrowth := int64(2 * 1024 * 1024) // 2 MB

	if heapGrowth > maxAllowedGrowth {
		t.Errorf("Codec operations may be leaking: heap grew by %d bytes", heapGrowth)
	}
}

// TestMemoryLeak_ILBCCodec tests iLBC encoder/decoder for memory leaks
func TestMemoryLeak_ILBCCodec(t *testing.T) {
	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	config := &ILBCConfig{Mode: ILBCMode30ms, EnablePLC: true}
	iterations := 500

	samples := make([]int16, ILBC30FrameSamples)
	for i := range samples {
		samples[i] = int16(i * 50)
	}

	for i := 0; i < iterations; i++ {
		encoder, _ := NewILBCEncoder(config)
		decoder, _ := NewILBCDecoder(config)

		encoded, _ := encoder.Encode(samples)
		_, _ = decoder.Decode(encoded)

		encoder.Close()
		decoder.Close()
	}

	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	maxAllowedGrowth := int64(5 * 1024 * 1024) // 5 MB

	if heapGrowth > maxAllowedGrowth {
		t.Errorf("iLBC codec may be leaking: heap grew by %d bytes", heapGrowth)
	}
}

// TestMemoryLeak_V21Detector tests V.21 detector for memory leaks
func TestMemoryLeak_V21Detector(t *testing.T) {
	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	iterations := 100
	samples := make([]int16, 8000) // 1 second of audio
	for i := range samples {
		samples[i] = int16(1000 * testSine(2*3.14159*1100*float64(i)/8000))
	}

	for i := 0; i < iterations; i++ {
		config := DefaultV21DetectorConfig()
		detector := NewV21Detector(config)
		detector.AddHandler(func(d *V21Detection) {})
		detector.ProcessSamples(samples)
		detector.Reset()
	}

	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	maxAllowedGrowth := int64(5 * 1024 * 1024) // 5 MB

	if heapGrowth > maxAllowedGrowth {
		t.Errorf("V21 detector may be leaking: heap grew by %d bytes", heapGrowth)
	}
}

// TestGCLeak_Goroutines tests that goroutines are properly cleaned up
func TestGCLeak_Goroutines(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	// Create and run operations that spawn goroutines
	iterations := 100
	for i := 0; i < iterations; i++ {
		var wg sync.WaitGroup
		wg.Add(5)
		for j := 0; j < 5; j++ {
			go func() {
				defer wg.Done()
				time.Sleep(time.Millisecond)
			}()
		}
		wg.Wait()
	}

	// Give goroutines time to fully exit
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()

	// Allow for some background goroutines but not a leak
	goroutineLeak := finalGoroutines - initialGoroutines
	maxAllowedLeak := 10

	if goroutineLeak > maxAllowedLeak {
		t.Errorf("Goroutine leak detected: started with %d, ended with %d (leak of %d)",
			initialGoroutines, finalGoroutines, goroutineLeak)
	}
}

// TestGCLeak_Channels tests that channels are properly garbage collected
func TestGCLeak_Channels(t *testing.T) {
	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	iterations := 10000
	for i := 0; i < iterations; i++ {
		ch := make(chan int, 100)
		for j := 0; j < 50; j++ {
			ch <- j
		}
		close(ch)
		// Channel goes out of scope and should be GC'd
	}

	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	maxAllowedGrowth := int64(5 * 1024 * 1024) // 5 MB

	if heapGrowth > maxAllowedGrowth {
		t.Errorf("Channel leak detected: heap grew by %d bytes", heapGrowth)
	}
}

// TestGCLeak_Maps tests that maps are properly garbage collected
func TestGCLeak_Maps(t *testing.T) {
	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	iterations := 1000
	for i := 0; i < iterations; i++ {
		m := make(map[string][]byte)
		for j := 0; j < 100; j++ {
			key := string(rune('a' + j%26))
			m[key] = make([]byte, 1024)
		}
		// Map goes out of scope
	}

	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	maxAllowedGrowth := int64(10 * 1024 * 1024) // 10 MB

	if heapGrowth > maxAllowedGrowth {
		t.Errorf("Map leak detected: heap grew by %d bytes", heapGrowth)
	}
}

// TestGCLeak_Finalizers tests that finalizers are being called
func TestGCLeak_Finalizers(t *testing.T) {
	var finalizerCount int
	var mu sync.Mutex

	type testObj struct {
		data []byte
	}

	iterations := 100
	for i := 0; i < iterations; i++ {
		obj := &testObj{data: make([]byte, 1024)}
		runtime.SetFinalizer(obj, func(o *testObj) {
			mu.Lock()
			finalizerCount++
			mu.Unlock()
		})
	}

	// Force several GC cycles
	for i := 0; i < 10; i++ {
		runtime.GC()
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	count := finalizerCount
	mu.Unlock()

	// At least some finalizers should have run
	minExpectedFinalizers := iterations / 2
	if count < minExpectedFinalizers {
		t.Errorf("Finalizers not being called properly: expected at least %d, got %d",
			minExpectedFinalizers, count)
	}
}

// TestMemoryLeak_CircuitBreaker tests circuit breaker for memory leaks
func TestMemoryLeak_CircuitBreaker(t *testing.T) {
	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	iterations := 1000
	for i := 0; i < iterations; i++ {
		cb := NewCircuitBreaker(DefaultCircuitBreakerConfig("test"))
		for j := 0; j < 10; j++ {
			cb.Execute(func() error { return nil })
		}
		// Circuit breaker goes out of scope
	}

	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	maxAllowedGrowth := int64(5 * 1024 * 1024) // 5 MB

	if heapGrowth > maxAllowedGrowth {
		t.Errorf("CircuitBreaker may be leaking: heap grew by %d bytes", heapGrowth)
	}
}

// TestMemoryLeak_RateLimiter tests rate limiter for memory leaks
func TestMemoryLeak_RateLimiter(t *testing.T) {
	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	config := DefaultRateLimiterConfig()
	rl := NewRateLimiter(config)

	iterations := 10000
	for i := 0; i < iterations; i++ {
		rl.Allow("127.0.0.1", "test-call")
	}

	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	maxAllowedGrowth := int64(5 * 1024 * 1024) // 5 MB

	if heapGrowth > maxAllowedGrowth {
		t.Errorf("RateLimiter may be leaking: heap grew by %d bytes", heapGrowth)
	}
}

// TestMemoryLeak_Logger tests structured logger for memory leaks
func TestMemoryLeak_Logger(t *testing.T) {
	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	config := &StructuredLoggerConfig{
		Level:  SLogLevelError, // Suppress output
		Format: LogFormatJSON,
	}
	logger := NewStructuredLogger(config)

	iterations := 10000
	for i := 0; i < iterations; i++ {
		// Create derived loggers
		l := logger.WithField("iter", i).WithField("test", true)
		l.Debug("test message that won't be logged")
	}

	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	maxAllowedGrowth := int64(5 * 1024 * 1024) // 5 MB

	if heapGrowth > maxAllowedGrowth {
		t.Errorf("StructuredLogger may be leaking: heap grew by %d bytes", heapGrowth)
	}
}

// BenchmarkMemoryAllocation measures memory allocation rates
func BenchmarkMemoryAllocation_Sessions(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sm := newTestStateMachine("test-call")
		sm.processOffer("from-tag")
		sm.processAnswer("from-tag", "to-tag", false)
	}
}

func BenchmarkMemoryAllocation_G711(b *testing.B) {
	b.ReportAllocs()
	samples := make([]int16, 160)
	for i := range samples {
		samples[i] = int16(i * 100)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range samples {
			encoded := LinearToMulaw(samples[j])
			_ = MulawToLinear(encoded)
		}
	}
}

func BenchmarkMemoryAllocation_ILBCEncode(b *testing.B) {
	b.ReportAllocs()
	config := &ILBCConfig{Mode: ILBCMode30ms}
	encoder, _ := NewILBCEncoder(config)
	defer encoder.Close()

	samples := make([]int16, ILBC30FrameSamples)
	for i := range samples {
		samples[i] = int16(i * 50)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoder.Encode(samples)
	}
}

func BenchmarkMemoryAllocation_BufferPool(b *testing.B) {
	b.ReportAllocs()
	pool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, 2048)
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := pool.Get().([]byte)
		pool.Put(buf)
	}
}
