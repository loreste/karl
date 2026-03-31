package internal

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// PerformanceMonitor tracks system performance metrics
type PerformanceMonitor struct {
	// Packet processing metrics
	packetsProcessed  uint64
	packetsDropped    uint64
	bytesProcessed    uint64
	processingErrors  uint64

	// Latency tracking
	latencySum    uint64  // Total latency in microseconds
	latencyCount  uint64  // Number of latency samples
	latencyMax    uint64  // Maximum latency seen
	latencyMin    uint64  // Minimum latency seen

	// Session metrics
	activeSessions    int64
	totalSessions     uint64
	sessionErrors     uint64

	// Memory metrics
	lastGCPause       uint64
	totalGCPauses     uint64
	heapAlloc         uint64
	heapInUse         uint64

	// CPU metrics
	goroutines        int64
	cpuUsagePercent   float64

	// Timing
	startTime         time.Time
	lastUpdate        time.Time

	// Rate calculation
	lastPacketCount   uint64
	lastByteCount     uint64
	lastRateCalcTime  time.Time
	packetsPerSecond  float64
	bytesPerSecond    float64

	// Configuration
	sampleInterval    time.Duration
	historySize       int
	latencyHistogram  []uint64  // Buckets for latency distribution

	mu sync.RWMutex
	stopCh chan struct{}
}

// PerformanceStats represents a snapshot of performance statistics
type PerformanceStats struct {
	// Uptime
	Uptime           time.Duration `json:"uptime"`

	// Packet metrics
	PacketsProcessed uint64        `json:"packets_processed"`
	PacketsDropped   uint64        `json:"packets_dropped"`
	BytesProcessed   uint64        `json:"bytes_processed"`
	ProcessingErrors uint64        `json:"processing_errors"`
	PacketsPerSecond float64       `json:"packets_per_second"`
	BytesPerSecond   float64       `json:"bytes_per_second"`

	// Latency metrics (microseconds)
	LatencyAvg       float64       `json:"latency_avg_us"`
	LatencyMax       uint64        `json:"latency_max_us"`
	LatencyMin       uint64        `json:"latency_min_us"`
	LatencyP50       uint64        `json:"latency_p50_us"`
	LatencyP95       uint64        `json:"latency_p95_us"`
	LatencyP99       uint64        `json:"latency_p99_us"`

	// Session metrics
	ActiveSessions   int64         `json:"active_sessions"`
	TotalSessions    uint64        `json:"total_sessions"`
	SessionErrors    uint64        `json:"session_errors"`

	// Memory metrics
	HeapAlloc        uint64        `json:"heap_alloc_bytes"`
	HeapInUse        uint64        `json:"heap_in_use_bytes"`
	GCPauseTotalNs   uint64        `json:"gc_pause_total_ns"`
	LastGCPauseNs    uint64        `json:"last_gc_pause_ns"`
	GCCount          uint32        `json:"gc_count"`

	// Runtime metrics
	Goroutines       int           `json:"goroutines"`
	NumCPU           int           `json:"num_cpu"`
}

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor() *PerformanceMonitor {
	pm := &PerformanceMonitor{
		startTime:        time.Now(),
		lastUpdate:       time.Now(),
		lastRateCalcTime: time.Now(),
		sampleInterval:   1 * time.Second,
		historySize:      100,
		latencyHistogram: make([]uint64, 20), // 20 buckets
		latencyMin:       ^uint64(0),         // Max uint64 as initial min
		stopCh:           make(chan struct{}),
	}

	// Start background monitoring
	go pm.monitorLoop()

	return pm
}

// RecordPacket records a processed packet
func (pm *PerformanceMonitor) RecordPacket(bytes int, dropped bool) {
	if dropped {
		atomic.AddUint64(&pm.packetsDropped, 1)
	} else {
		atomic.AddUint64(&pm.packetsProcessed, 1)
		atomic.AddUint64(&pm.bytesProcessed, uint64(bytes))
	}
}

// RecordLatency records processing latency
func (pm *PerformanceMonitor) RecordLatency(latencyUs uint64) {
	atomic.AddUint64(&pm.latencySum, latencyUs)
	atomic.AddUint64(&pm.latencyCount, 1)

	// Update max
	for {
		current := atomic.LoadUint64(&pm.latencyMax)
		if latencyUs <= current {
			break
		}
		if atomic.CompareAndSwapUint64(&pm.latencyMax, current, latencyUs) {
			break
		}
	}

	// Update min
	for {
		current := atomic.LoadUint64(&pm.latencyMin)
		if latencyUs >= current {
			break
		}
		if atomic.CompareAndSwapUint64(&pm.latencyMin, current, latencyUs) {
			break
		}
	}

	// Update histogram bucket
	bucket := pm.latencyToBucket(latencyUs)
	if bucket < len(pm.latencyHistogram) {
		atomic.AddUint64(&pm.latencyHistogram[bucket], 1)
	}
}

// latencyToBucket converts latency to histogram bucket
func (pm *PerformanceMonitor) latencyToBucket(latencyUs uint64) int {
	// Buckets: 0-100, 100-200, 200-500, 500-1000, 1000-2000, 2000-5000, 5000-10000, 10000+
	switch {
	case latencyUs < 100:
		return 0
	case latencyUs < 200:
		return 1
	case latencyUs < 500:
		return 2
	case latencyUs < 1000:
		return 3
	case latencyUs < 2000:
		return 4
	case latencyUs < 5000:
		return 5
	case latencyUs < 10000:
		return 6
	case latencyUs < 20000:
		return 7
	case latencyUs < 50000:
		return 8
	default:
		return 9
	}
}

// RecordError records a processing error
func (pm *PerformanceMonitor) RecordError() {
	atomic.AddUint64(&pm.processingErrors, 1)
}

// RecordSessionStart records a new session
func (pm *PerformanceMonitor) RecordSessionStart() {
	atomic.AddInt64(&pm.activeSessions, 1)
	atomic.AddUint64(&pm.totalSessions, 1)
}

// RecordSessionEnd records session termination
func (pm *PerformanceMonitor) RecordSessionEnd(hadError bool) {
	atomic.AddInt64(&pm.activeSessions, -1)
	if hadError {
		atomic.AddUint64(&pm.sessionErrors, 1)
	}
}

// GetStats returns current performance statistics
func (pm *PerformanceMonitor) GetStats() *PerformanceStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	latencyCount := atomic.LoadUint64(&pm.latencyCount)
	latencySum := atomic.LoadUint64(&pm.latencySum)
	latencyMin := atomic.LoadUint64(&pm.latencyMin)
	if latencyMin == ^uint64(0) {
		latencyMin = 0
	}

	var latencyAvg float64
	if latencyCount > 0 {
		latencyAvg = float64(latencySum) / float64(latencyCount)
	}

	// Calculate percentiles from histogram
	p50, p95, p99 := pm.calculatePercentiles()

	return &PerformanceStats{
		Uptime:           time.Since(pm.startTime),
		PacketsProcessed: atomic.LoadUint64(&pm.packetsProcessed),
		PacketsDropped:   atomic.LoadUint64(&pm.packetsDropped),
		BytesProcessed:   atomic.LoadUint64(&pm.bytesProcessed),
		ProcessingErrors: atomic.LoadUint64(&pm.processingErrors),
		PacketsPerSecond: pm.packetsPerSecond,
		BytesPerSecond:   pm.bytesPerSecond,
		LatencyAvg:       latencyAvg,
		LatencyMax:       atomic.LoadUint64(&pm.latencyMax),
		LatencyMin:       latencyMin,
		LatencyP50:       p50,
		LatencyP95:       p95,
		LatencyP99:       p99,
		ActiveSessions:   atomic.LoadInt64(&pm.activeSessions),
		TotalSessions:    atomic.LoadUint64(&pm.totalSessions),
		SessionErrors:    atomic.LoadUint64(&pm.sessionErrors),
		HeapAlloc:        memStats.HeapAlloc,
		HeapInUse:        memStats.HeapInuse,
		GCPauseTotalNs:   memStats.PauseTotalNs,
		LastGCPauseNs:    memStats.PauseNs[(memStats.NumGC+255)%256],
		GCCount:          memStats.NumGC,
		Goroutines:       runtime.NumGoroutine(),
		NumCPU:           runtime.NumCPU(),
	}
}

// calculatePercentiles calculates latency percentiles from histogram
func (pm *PerformanceMonitor) calculatePercentiles() (p50, p95, p99 uint64) {
	total := atomic.LoadUint64(&pm.latencyCount)
	if total == 0 {
		return 0, 0, 0
	}

	target50 := total * 50 / 100
	target95 := total * 95 / 100
	target99 := total * 99 / 100

	var cumulative uint64
	bucketValues := []uint64{50, 150, 350, 750, 1500, 3500, 7500, 15000, 35000, 75000}

	for i := range pm.latencyHistogram {
		count := atomic.LoadUint64(&pm.latencyHistogram[i])
		cumulative += count

		if i < len(bucketValues) {
			if p50 == 0 && cumulative >= target50 {
				p50 = bucketValues[i]
			}
			if p95 == 0 && cumulative >= target95 {
				p95 = bucketValues[i]
			}
			if p99 == 0 && cumulative >= target99 {
				p99 = bucketValues[i]
			}
		}
	}

	return
}

// monitorLoop runs background monitoring
func (pm *PerformanceMonitor) monitorLoop() {
	ticker := time.NewTicker(pm.sampleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pm.updateRates()
		case <-pm.stopCh:
			return
		}
	}
}

// updateRates updates rate calculations
func (pm *PerformanceMonitor) updateRates() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(pm.lastRateCalcTime).Seconds()
	if elapsed <= 0 {
		return
	}

	currentPackets := atomic.LoadUint64(&pm.packetsProcessed)
	currentBytes := atomic.LoadUint64(&pm.bytesProcessed)

	pm.packetsPerSecond = float64(currentPackets-pm.lastPacketCount) / elapsed
	pm.bytesPerSecond = float64(currentBytes-pm.lastByteCount) / elapsed

	pm.lastPacketCount = currentPackets
	pm.lastByteCount = currentBytes
	pm.lastRateCalcTime = now
}

// Stop stops the performance monitor
func (pm *PerformanceMonitor) Stop() {
	close(pm.stopCh)
}

// Reset resets all counters
func (pm *PerformanceMonitor) Reset() {
	atomic.StoreUint64(&pm.packetsProcessed, 0)
	atomic.StoreUint64(&pm.packetsDropped, 0)
	atomic.StoreUint64(&pm.bytesProcessed, 0)
	atomic.StoreUint64(&pm.processingErrors, 0)
	atomic.StoreUint64(&pm.latencySum, 0)
	atomic.StoreUint64(&pm.latencyCount, 0)
	atomic.StoreUint64(&pm.latencyMax, 0)
	atomic.StoreUint64(&pm.latencyMin, ^uint64(0))
	atomic.StoreUint64(&pm.totalSessions, 0)
	atomic.StoreUint64(&pm.sessionErrors, 0)

	for i := range pm.latencyHistogram {
		atomic.StoreUint64(&pm.latencyHistogram[i], 0)
	}

	pm.mu.Lock()
	pm.startTime = time.Now()
	pm.lastPacketCount = 0
	pm.lastByteCount = 0
	pm.lastRateCalcTime = time.Now()
	pm.packetsPerSecond = 0
	pm.bytesPerSecond = 0
	pm.mu.Unlock()
}

// GetActiveSessionCount returns the current active session count
func (pm *PerformanceMonitor) GetActiveSessionCount() int64 {
	return atomic.LoadInt64(&pm.activeSessions)
}

// GetPacketsPerSecond returns current packets per second rate
func (pm *PerformanceMonitor) GetPacketsPerSecond() float64 {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.packetsPerSecond
}

// GetBytesPerSecond returns current bytes per second rate
func (pm *PerformanceMonitor) GetBytesPerSecond() float64 {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.bytesPerSecond
}

// GetUptime returns the monitor uptime
func (pm *PerformanceMonitor) GetUptime() time.Duration {
	return time.Since(pm.startTime)
}
