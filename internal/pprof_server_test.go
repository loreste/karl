package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestDefaultPprofConfig(t *testing.T) {
	config := DefaultPprofConfig()

	if !config.Enabled {
		t.Error("Expected Enabled true by default")
	}
	if config.Port != ":6060" {
		t.Errorf("Expected Port :6060, got %s", config.Port)
	}
	if config.GCPercent != 100 {
		t.Errorf("Expected GCPercent 100, got %d", config.GCPercent)
	}
}

func TestNewRTPBufferPool(t *testing.T) {
	bp := NewRTPBufferPool(1500, 10)

	if bp.bufferSize != 1500 {
		t.Errorf("Expected bufferSize 1500, got %d", bp.bufferSize)
	}
	if bp.allocated.Load() != 10 {
		t.Errorf("Expected allocated 10, got %d", bp.allocated.Load())
	}
}

func TestRTPBufferPool_GetPut(t *testing.T) {
	bp := NewRTPBufferPool(1500, 5)

	buf := bp.Get()
	if len(buf) != 1500 {
		t.Errorf("Expected buffer length 1500, got %d", len(buf))
	}
	if bp.inUse.Load() != 1 {
		t.Errorf("Expected inUse 1, got %d", bp.inUse.Load())
	}

	bp.Put(buf)
	if bp.inUse.Load() != 0 {
		t.Errorf("Expected inUse 0, got %d", bp.inUse.Load())
	}
}

func TestRTPBufferPool_Reuse(t *testing.T) {
	bp := NewRTPBufferPool(1500, 5)

	// Drain preallocated buffers
	buffers := make([][]byte, 5)
	for i := 0; i < 5; i++ {
		buffers[i] = bp.Get()
	}

	// Put them back
	for _, buf := range buffers {
		bp.Put(buf)
	}

	initialReused := bp.reused.Load()

	// Get again - should reuse
	buf := bp.Get()
	bp.Put(buf)

	if bp.reused.Load() <= initialReused {
		t.Error("Expected reused count to increase")
	}
}

func TestRTPBufferPool_Stats(t *testing.T) {
	bp := NewRTPBufferPool(1500, 5)

	for i := 0; i < 3; i++ {
		buf := bp.Get()
		bp.Put(buf)
	}

	stats := bp.Stats()

	if stats["buffer_size"] != 1500 {
		t.Errorf("Expected buffer_size 1500, got %v", stats["buffer_size"])
	}
	if _, ok := stats["reuse_ratio"].(float64); !ok {
		t.Error("Expected reuse_ratio to be float64")
	}
}

func TestRTPBufferPool_ConcurrentAccess(t *testing.T) {
	bp := NewRTPBufferPool(1500, 100)

	var wg sync.WaitGroup
	numGoroutines := 50
	numOps := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				buf := bp.Get()
				bp.Put(buf)
			}
		}()
	}

	wg.Wait()

	if bp.inUse.Load() != 0 {
		t.Errorf("Expected inUse 0 after all operations, got %d", bp.inUse.Load())
	}
}

func TestRTPBufferPool_PutNil(t *testing.T) {
	bp := NewRTPBufferPool(1500, 5)

	// Should not panic
	bp.Put(nil)

	// Should not panic with small buffer
	smallBuf := make([]byte, 100)
	bp.Put(smallBuf)
}

func TestRTPBufferPool_BufferClearing(t *testing.T) {
	bp := NewRTPBufferPool(100, 0)

	buf := bp.Get()
	for i := range buf {
		buf[i] = 0xFF
	}

	bp.Put(buf)

	buf2 := bp.Get()
	for i := range buf2 {
		if buf2[i] != 0 {
			t.Errorf("Buffer not cleared at index %d: %d", i, buf2[i])
			break
		}
	}
}

func TestNewPprofServer(t *testing.T) {
	config := &PprofConfig{
		Enabled: false, // Don't actually start server
		Port:    ":16060",
	}

	ps := NewPprofServer(config)
	if ps == nil {
		t.Fatal("NewPprofServer returned nil")
	}

	if ps.rtpPool == nil {
		t.Error("rtpPool not initialized")
	}
	if ps.rtcpPool == nil {
		t.Error("rtcpPool not initialized")
	}
}

func TestNewPprofServer_DefaultConfig(t *testing.T) {
	ps := NewPprofServer(nil)
	if ps == nil {
		t.Fatal("NewPprofServer returned nil")
	}

	if ps.config.Port != ":6060" {
		t.Errorf("Expected default port :6060, got %s", ps.config.Port)
	}
}

func TestPprofServer_BufferPools(t *testing.T) {
	config := &PprofConfig{
		Enabled: false,
	}

	ps := NewPprofServer(config)

	// Test RTP buffer
	rtpBuf := ps.GetRTPBuffer()
	if len(rtpBuf) != 1500 {
		t.Errorf("Expected RTP buffer size 1500, got %d", len(rtpBuf))
	}
	ps.PutRTPBuffer(rtpBuf)

	// Test RTCP buffer
	rtcpBuf := ps.GetRTCPBuffer()
	if len(rtcpBuf) != 1500 {
		t.Errorf("Expected RTCP buffer size 1500, got %d", len(rtcpBuf))
	}
	ps.PutRTCPBuffer(rtcpBuf)
}

func TestPprofServer_PoolStatsHandler(t *testing.T) {
	config := &PprofConfig{
		Enabled: false,
	}

	ps := NewPprofServer(config)

	req := httptest.NewRequest(http.MethodGet, "/debug/pools", nil)
	w := httptest.NewRecorder()

	ps.poolStatsHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Errorf("Failed to decode JSON: %v", err)
	}

	if _, ok := result["rtp_pool"]; !ok {
		t.Error("Missing rtp_pool in response")
	}
	if _, ok := result["rtcp_pool"]; !ok {
		t.Error("Missing rtcp_pool in response")
	}
}

func TestPprofServer_GCHandler(t *testing.T) {
	config := &PprofConfig{
		Enabled: false,
	}

	ps := NewPprofServer(config)

	// GET should fail
	req := httptest.NewRequest(http.MethodGet, "/debug/gc", nil)
	w := httptest.NewRecorder()
	ps.gcHandler(w, req)

	if w.Result().StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405 for GET, got %d", w.Result().StatusCode)
	}

	// POST should work
	req = httptest.NewRequest(http.MethodPost, "/debug/gc", nil)
	w = httptest.NewRecorder()
	ps.gcHandler(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for POST, got %d", w.Result().StatusCode)
	}
}

func TestPprofServer_MemoryHandler(t *testing.T) {
	config := &PprofConfig{
		Enabled: false,
	}

	ps := NewPprofServer(config)

	req := httptest.NewRequest(http.MethodGet, "/debug/memory", nil)
	w := httptest.NewRecorder()

	ps.memoryHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Errorf("Failed to decode JSON: %v", err)
	}

	expectedFields := []string{"alloc_mb", "heap_alloc_mb", "num_gc"}
	for _, field := range expectedFields {
		if _, ok := result[field]; !ok {
			t.Errorf("Missing field %s in response", field)
		}
	}
}

func TestPprofServer_RuntimeHandler(t *testing.T) {
	config := &PprofConfig{
		Enabled: false,
	}

	ps := NewPprofServer(config)

	req := httptest.NewRequest(http.MethodGet, "/debug/runtime", nil)
	w := httptest.NewRecorder()

	ps.runtimeHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Errorf("Failed to decode JSON: %v", err)
	}

	expectedFields := []string{"goroutines", "num_cpu", "gomaxprocs", "go_version"}
	for _, field := range expectedFields {
		if _, ok := result[field]; !ok {
			t.Errorf("Missing field %s in response", field)
		}
	}
}

func TestPprofServer_StartStop(t *testing.T) {
	config := &PprofConfig{
		Enabled: false, // Don't start HTTP server in test
	}

	ps := NewPprofServer(config)

	err := ps.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	ps.mu.RLock()
	running := ps.running
	ps.mu.RUnlock()
	if !running {
		t.Error("Expected running to be true")
	}

	err = ps.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Double stop should not error
	err = ps.Stop()
	if err != nil {
		t.Fatalf("Double stop failed: %v", err)
	}
}

func TestPprofServer_StartWithServer(t *testing.T) {
	config := &PprofConfig{
		Enabled: true,
		Port:    ":16061", // Use non-standard port to avoid conflicts
	}

	ps := NewPprofServer(config)
	err := ps.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Verify pprof endpoints are accessible
	resp, err := http.Get("http://localhost:16061/debug/pprof/")
	if err != nil {
		t.Logf("pprof server may not be reachable: %v", err)
	} else {
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 from pprof, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	}

	// Test custom endpoint
	resp, err = http.Get("http://localhost:16061/debug/memory")
	if err == nil {
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 from memory, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	}

	ps.Stop()
}

func TestPprofServer_ConcurrentAccess(t *testing.T) {
	config := &PprofConfig{
		Enabled: false,
	}

	ps := NewPprofServer(config)
	ps.Start()
	defer ps.Stop()

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				rtpBuf := ps.GetRTPBuffer()
				ps.PutRTPBuffer(rtpBuf)

				rtcpBuf := ps.GetRTCPBuffer()
				ps.PutRTCPBuffer(rtcpBuf)
			}
		}()
	}

	wg.Wait()
}
