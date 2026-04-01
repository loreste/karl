package internal

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// PprofConfig holds pprof server configuration
type PprofConfig struct {
	Enabled       bool   // Enable pprof endpoints
	Port          string // Port for pprof server (default :6060)
	BlockProfile  int    // Block profile rate (0 = disabled)
	MutexProfile  int    // Mutex profile fraction (0 = disabled)
	GCPercent     int    // GOGC value (default 100)
	MemoryLimitMB int    // Soft memory limit in MB
}

// DefaultPprofConfig returns sensible defaults
func DefaultPprofConfig() *PprofConfig {
	return &PprofConfig{
		Enabled:       true,
		Port:          ":6060",
		BlockProfile:  0,
		MutexProfile:  0,
		GCPercent:     100,
		MemoryLimitMB: 0,
	}
}

// PprofServer manages the pprof HTTP server and buffer pools
type PprofServer struct {
	config   *PprofConfig
	server   *http.Server
	rtpPool  *RTPBufferPool
	rtcpPool *RTPBufferPool
	mu       sync.RWMutex
	running  bool
}

// RTPBufferPool manages a pool of reusable byte buffers for RTP packets
type RTPBufferPool struct {
	pool       sync.Pool
	bufferSize int
	allocated  atomic.Uint64
	reused     atomic.Uint64
	missed     atomic.Uint64
	inUse      atomic.Int64
}

// NewRTPBufferPool creates a new buffer pool
func NewRTPBufferPool(bufferSize, preallocate int) *RTPBufferPool {
	bp := &RTPBufferPool{
		bufferSize: bufferSize,
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, bufferSize)
			},
		},
	}

	// Preallocate buffers
	buffers := make([][]byte, preallocate)
	for i := 0; i < preallocate; i++ {
		buffers[i] = make([]byte, bufferSize)
	}
	for _, buf := range buffers {
		bp.pool.Put(buf)
	}
	bp.allocated.Add(uint64(preallocate))

	return bp
}

// Get retrieves a buffer from the pool
func (bp *RTPBufferPool) Get() []byte {
	buf := bp.pool.Get().([]byte)
	if buf == nil {
		bp.missed.Add(1)
		buf = make([]byte, bp.bufferSize)
		bp.allocated.Add(1)
	} else {
		bp.reused.Add(1)
	}
	bp.inUse.Add(1)
	return buf[:bp.bufferSize]
}

// Put returns a buffer to the pool
func (bp *RTPBufferPool) Put(buf []byte) {
	if buf == nil || cap(buf) < bp.bufferSize {
		return
	}
	bp.inUse.Add(-1)

	// Clear the buffer before returning (security)
	for i := 0; i < bp.bufferSize && i < len(buf); i++ {
		buf[i] = 0
	}

	bp.pool.Put(buf[:bp.bufferSize])
}

// Stats returns buffer pool statistics
func (bp *RTPBufferPool) Stats() map[string]interface{} {
	total := bp.allocated.Load() + bp.reused.Load()
	reuseRatio := float64(0)
	if total > 0 {
		reuseRatio = float64(bp.reused.Load()) / float64(total)
	}

	return map[string]interface{}{
		"buffer_size": bp.bufferSize,
		"allocated":   bp.allocated.Load(),
		"reused":      bp.reused.Load(),
		"missed":      bp.missed.Load(),
		"in_use":      bp.inUse.Load(),
		"reuse_ratio": reuseRatio,
	}
}

// NewPprofServer creates a new pprof server
func NewPprofServer(config *PprofConfig) *PprofServer {
	if config == nil {
		config = DefaultPprofConfig()
	}

	ps := &PprofServer{
		config:   config,
		rtpPool:  NewRTPBufferPool(1500, 1000),
		rtcpPool: NewRTPBufferPool(1500, 500),
	}

	// Apply GC settings
	ps.applyGCSettings()

	return ps
}

// applyGCSettings configures Go runtime GC settings
func (ps *PprofServer) applyGCSettings() {
	if ps.config.GCPercent > 0 {
		debug.SetGCPercent(ps.config.GCPercent)
	}

	if ps.config.MemoryLimitMB > 0 {
		limit := int64(ps.config.MemoryLimitMB) * 1024 * 1024
		debug.SetMemoryLimit(limit)
		log.Printf("Set memory limit to %d MB", ps.config.MemoryLimitMB)
	}

	if ps.config.BlockProfile > 0 {
		runtime.SetBlockProfileRate(ps.config.BlockProfile)
	}
	if ps.config.MutexProfile > 0 {
		runtime.SetMutexProfileFraction(ps.config.MutexProfile)
	}
}

// Start starts the pprof server
func (ps *PprofServer) Start() error {
	ps.mu.Lock()
	if ps.running {
		ps.mu.Unlock()
		return nil
	}
	ps.running = true
	ps.mu.Unlock()

	if !ps.config.Enabled {
		return nil
	}

	mux := http.NewServeMux()

	// Register pprof handlers
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// Additional pprof endpoints
	mux.HandleFunc("/debug/pprof/heap", pprof.Handler("heap").ServeHTTP)
	mux.HandleFunc("/debug/pprof/goroutine", pprof.Handler("goroutine").ServeHTTP)
	mux.HandleFunc("/debug/pprof/allocs", pprof.Handler("allocs").ServeHTTP)
	mux.HandleFunc("/debug/pprof/block", pprof.Handler("block").ServeHTTP)
	mux.HandleFunc("/debug/pprof/mutex", pprof.Handler("mutex").ServeHTTP)
	mux.HandleFunc("/debug/pprof/threadcreate", pprof.Handler("threadcreate").ServeHTTP)

	// Custom endpoints
	mux.HandleFunc("/debug/pools", ps.poolStatsHandler)
	mux.HandleFunc("/debug/gc", ps.gcHandler)
	mux.HandleFunc("/debug/memory", ps.memoryHandler)
	mux.HandleFunc("/debug/runtime", ps.runtimeHandler)

	ps.server = &http.Server{
		Addr:         ps.config.Port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	go func() {
		log.Printf("Starting pprof server on %s", ps.config.Port)
		if err := ps.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("pprof server error: %v", err)
		}
	}()

	return nil
}

// poolStatsHandler returns buffer pool statistics
func (ps *PprofServer) poolStatsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rtpStats := ps.rtpPool.Stats()
	rtcpStats := ps.rtcpPool.Stats()

	fmt.Fprintf(w, `{
  "rtp_pool": {
    "buffer_size": %d,
    "allocated": %d,
    "reused": %d,
    "missed": %d,
    "in_use": %d,
    "reuse_ratio": %.4f
  },
  "rtcp_pool": {
    "buffer_size": %d,
    "allocated": %d,
    "reused": %d,
    "missed": %d,
    "in_use": %d,
    "reuse_ratio": %.4f
  }
}`,
		rtpStats["buffer_size"], rtpStats["allocated"], rtpStats["reused"],
		rtpStats["missed"], rtpStats["in_use"], rtpStats["reuse_ratio"],
		rtcpStats["buffer_size"], rtcpStats["allocated"], rtcpStats["reused"],
		rtcpStats["missed"], rtcpStats["in_use"], rtcpStats["reuse_ratio"],
	)
}

// gcHandler triggers garbage collection
func (ps *PprofServer) gcHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	runtime.GC()
	runtime.ReadMemStats(&after)

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{
  "status": "ok",
  "heap_before_mb": %.2f,
  "heap_after_mb": %.2f,
  "freed_mb": %.2f
}`,
		float64(before.HeapAlloc)/1024/1024,
		float64(after.HeapAlloc)/1024/1024,
		float64(before.HeapAlloc-after.HeapAlloc)/1024/1024,
	)
}

// memoryHandler provides detailed memory information
func (ps *PprofServer) memoryHandler(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{
  "alloc_mb": %.2f,
  "total_alloc_mb": %.2f,
  "sys_mb": %.2f,
  "heap_alloc_mb": %.2f,
  "heap_sys_mb": %.2f,
  "heap_idle_mb": %.2f,
  "heap_inuse_mb": %.2f,
  "heap_objects": %d,
  "stack_inuse_mb": %.2f,
  "gc_pause_total_ms": %.2f,
  "num_gc": %d,
  "gc_cpu_fraction": %.6f
}`,
		float64(m.Alloc)/1024/1024,
		float64(m.TotalAlloc)/1024/1024,
		float64(m.Sys)/1024/1024,
		float64(m.HeapAlloc)/1024/1024,
		float64(m.HeapSys)/1024/1024,
		float64(m.HeapIdle)/1024/1024,
		float64(m.HeapInuse)/1024/1024,
		m.HeapObjects,
		float64(m.StackInuse)/1024/1024,
		float64(m.PauseTotalNs)/1e6,
		m.NumGC,
		m.GCCPUFraction,
	)
}

// runtimeHandler provides runtime information
func (ps *PprofServer) runtimeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{
  "goroutines": %d,
  "num_cpu": %d,
  "gomaxprocs": %d,
  "go_version": "%s",
  "compiler": "%s",
  "arch": "%s",
  "os": "%s"
}`,
		runtime.NumGoroutine(),
		runtime.NumCPU(),
		runtime.GOMAXPROCS(0),
		runtime.Version(),
		runtime.Compiler,
		runtime.GOARCH,
		runtime.GOOS,
	)
}

// GetRTPBuffer gets a buffer from the RTP pool
func (ps *PprofServer) GetRTPBuffer() []byte {
	return ps.rtpPool.Get()
}

// PutRTPBuffer returns a buffer to the RTP pool
func (ps *PprofServer) PutRTPBuffer(buf []byte) {
	ps.rtpPool.Put(buf)
}

// GetRTCPBuffer gets a buffer from the RTCP pool
func (ps *PprofServer) GetRTCPBuffer() []byte {
	return ps.rtcpPool.Get()
}

// PutRTCPBuffer returns a buffer to the RTCP pool
func (ps *PprofServer) PutRTCPBuffer(buf []byte) {
	ps.rtcpPool.Put(buf)
}

// Stop stops the pprof server
func (ps *PprofServer) Stop() error {
	ps.mu.Lock()
	if !ps.running {
		ps.mu.Unlock()
		return nil
	}
	ps.running = false
	ps.mu.Unlock()

	if ps.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return ps.server.Shutdown(ctx)
	}
	return nil
}

// Global pprof server instance
var (
	globalPprofServer     *PprofServer
	globalPprofServerOnce sync.Once
)

// GetPprofServer returns the global pprof server instance
func GetPprofServer() *PprofServer {
	globalPprofServerOnce.Do(func() {
		config := DefaultPprofConfig()

		// Check environment variables
		if port := os.Getenv("KARL_PPROF_PORT"); port != "" {
			config.Port = port
		}
		if gcPercent := os.Getenv("KARL_GC_PERCENT"); gcPercent != "" {
			if val, err := strconv.Atoi(gcPercent); err == nil {
				config.GCPercent = val
			}
		}
		if memLimit := os.Getenv("KARL_MEMORY_LIMIT_MB"); memLimit != "" {
			if val, err := strconv.Atoi(memLimit); err == nil {
				config.MemoryLimitMB = val
			}
		}
		if os.Getenv("KARL_PPROF_DISABLED") == "true" {
			config.Enabled = false
		}

		globalPprofServer = NewPprofServer(config)
		globalPprofServer.Start()
	})
	return globalPprofServer
}

// StartPprofServer is a convenience function to start the global pprof server
func StartPprofServer() *PprofServer {
	return GetPprofServer()
}
