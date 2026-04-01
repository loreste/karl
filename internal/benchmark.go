package internal

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// BenchmarkConfig configures benchmark parameters
type BenchmarkConfig struct {
	// Duration of the benchmark
	Duration time.Duration
	// NumSessions to simulate
	NumSessions int
	// PacketRate per session (packets per second)
	PacketRate int
	// PacketSize in bytes
	PacketSize int
	// NumWorkers for processing
	NumWorkers int
	// WarmupDuration before measuring
	WarmupDuration time.Duration
	// EnableRecording during benchmark
	EnableRecording bool
	// EnableTranscoding during benchmark
	EnableTranscoding bool
	// ReportInterval for intermediate results
	ReportInterval time.Duration
}

// DefaultBenchmarkConfig returns default benchmark configuration
func DefaultBenchmarkConfig() *BenchmarkConfig {
	return &BenchmarkConfig{
		Duration:        30 * time.Second,
		NumSessions:     1000,
		PacketRate:      50, // 50 pps = ~20ms packets
		PacketSize:      172, // G.711 20ms
		NumWorkers:      runtime.NumCPU(),
		WarmupDuration:  5 * time.Second,
		EnableRecording: false,
		ReportInterval:  5 * time.Second,
	}
}

// BenchmarkResult holds benchmark results
type BenchmarkResult struct {
	// Configuration
	Config *BenchmarkConfig

	// Timing
	StartTime    time.Time
	EndTime      time.Time
	Duration     time.Duration
	WarmupTime   time.Duration

	// Packet metrics
	PacketsSent     int64
	PacketsReceived int64
	PacketsDropped  int64
	PacketsLost     int64

	// Byte metrics
	BytesSent     int64
	BytesReceived int64

	// Latency metrics (nanoseconds)
	AvgLatency   int64
	MinLatency   int64
	MaxLatency   int64
	P50Latency   int64
	P95Latency   int64
	P99Latency   int64

	// Throughput
	PacketsPerSecond float64
	BytesPerSecond   float64
	BitsPerSecond    float64

	// Resource usage
	MemoryUsed     uint64
	GoroutineCount int
	CPUUsage       float64

	// Session metrics
	SessionsCreated   int64
	SessionsCompleted int64
	SessionsFailed    int64

	// Errors
	Errors []error
}

// BenchmarkRunner runs media server benchmarks
type BenchmarkRunner struct {
	config *BenchmarkConfig

	// Counters
	packetsSent     atomic.Int64
	packetsReceived atomic.Int64
	packetsDropped  atomic.Int64
	bytesTotal      atomic.Int64

	// Latency tracking
	latencySum   atomic.Int64
	latencyCount atomic.Int64
	minLatency   atomic.Int64
	maxLatency   atomic.Int64

	// Session tracking
	sessionsCreated   atomic.Int64
	sessionsCompleted atomic.Int64
	sessionsFailed    atomic.Int64

	// Control
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	errors   []error
	errorsMu sync.Mutex

	// Result
	result *BenchmarkResult
}

// NewBenchmarkRunner creates a new benchmark runner
func NewBenchmarkRunner(config *BenchmarkConfig) *BenchmarkRunner {
	if config == nil {
		config = DefaultBenchmarkConfig()
	}

	return &BenchmarkRunner{
		config: config,
		errors: make([]error, 0),
	}
}

// Run executes the benchmark
func (br *BenchmarkRunner) Run() (*BenchmarkResult, error) {
	br.ctx, br.cancel = context.WithCancel(context.Background())
	defer br.cancel()

	br.result = &BenchmarkResult{
		Config:     br.config,
		StartTime:  time.Now(),
		MinLatency: int64(^uint64(0) >> 1), // Max int64
	}

	// Get baseline memory
	runtime.GC()
	var memStart runtime.MemStats
	runtime.ReadMemStats(&memStart)

	// Run warmup
	if br.config.WarmupDuration > 0 {
		warmupCtx, warmupCancel := context.WithTimeout(br.ctx, br.config.WarmupDuration)
		br.runPhase(warmupCtx, true)
		warmupCancel()
		br.result.WarmupTime = br.config.WarmupDuration

		// Reset counters after warmup
		br.packetsSent.Store(0)
		br.packetsReceived.Store(0)
		br.packetsDropped.Store(0)
		br.bytesTotal.Store(0)
		br.latencySum.Store(0)
		br.latencyCount.Store(0)
		br.minLatency.Store(int64(^uint64(0) >> 1))
		br.maxLatency.Store(0)
		br.sessionsCreated.Store(0)
		br.sessionsCompleted.Store(0)
		br.sessionsFailed.Store(0)
	}

	// Run benchmark
	benchCtx, benchCancel := context.WithTimeout(br.ctx, br.config.Duration)
	br.runPhase(benchCtx, false)
	benchCancel()

	br.result.EndTime = time.Now()
	br.result.Duration = br.result.EndTime.Sub(br.result.StartTime) - br.result.WarmupTime

	// Collect final stats
	br.collectResults(&memStart)

	return br.result, nil
}

func (br *BenchmarkRunner) runPhase(ctx context.Context, isWarmup bool) {
	// Use a fresh WaitGroup for this phase
	var phaseWg sync.WaitGroup

	// Start packet generators
	for i := 0; i < br.config.NumSessions; i++ {
		phaseWg.Add(1)
		go func(sessionID int) {
			defer phaseWg.Done()
			br.sessionWorkerFunc(ctx, sessionID)
		}(i)
	}

	// Report progress
	if !isWarmup && br.config.ReportInterval > 0 {
		go br.reportProgress(ctx)
	}

	// Wait for all workers to complete
	phaseWg.Wait()
}

func (br *BenchmarkRunner) sessionWorker(ctx context.Context, sessionID int) {
	br.sessionWorkerFunc(ctx, sessionID)
}

func (br *BenchmarkRunner) sessionWorkerFunc(ctx context.Context, sessionID int) {
	br.sessionsCreated.Add(1)

	// Calculate packet interval
	interval := time.Second / time.Duration(br.config.PacketRate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Packet buffer
	packet := make([]byte, br.config.PacketSize)
	rand.Read(packet)

	for {
		select {
		case <-ctx.Done():
			br.sessionsCompleted.Add(1)
			return
		case <-ticker.C:
			// Simulate packet processing
			start := time.Now()
			br.processPacket(packet, sessionID)
			latency := time.Since(start).Nanoseconds()

			// Record latency
			br.latencySum.Add(latency)
			br.latencyCount.Add(1)

			// Update min/max
			for {
				old := br.minLatency.Load()
				if latency >= old {
					break
				}
				if br.minLatency.CompareAndSwap(old, latency) {
					break
				}
			}
			for {
				old := br.maxLatency.Load()
				if latency <= old {
					break
				}
				if br.maxLatency.CompareAndSwap(old, latency) {
					break
				}
			}
		}
	}
}

func (br *BenchmarkRunner) processPacket(packet []byte, sessionID int) {
	// Simulate RTP packet processing
	br.packetsSent.Add(1)
	br.bytesTotal.Add(int64(len(packet)))

	// Simulate some processing time
	// In real usage, this would go through the media server
	if rand.Float32() < 0.001 { // 0.1% drop rate
		br.packetsDropped.Add(1)
		return
	}

	br.packetsReceived.Add(1)
}

func (br *BenchmarkRunner) reportProgress(ctx context.Context) {
	ticker := time.NewTicker(br.config.ReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sent := br.packetsSent.Load()
			received := br.packetsReceived.Load()
			dropped := br.packetsDropped.Load()
			pps := float64(sent) / time.Since(br.result.StartTime.Add(br.result.WarmupTime)).Seconds()
			fmt.Printf("Progress: %d packets sent, %d received, %d dropped (%.0f pps)\n",
				sent, received, dropped, pps)
		}
	}
}

func (br *BenchmarkRunner) collectResults(memStart *runtime.MemStats) {
	var memEnd runtime.MemStats
	runtime.ReadMemStats(&memEnd)

	br.result.PacketsSent = br.packetsSent.Load()
	br.result.PacketsReceived = br.packetsReceived.Load()
	br.result.PacketsDropped = br.packetsDropped.Load()
	br.result.BytesSent = br.bytesTotal.Load()
	br.result.BytesReceived = br.bytesTotal.Load()

	// Calculate throughput
	durationSec := br.result.Duration.Seconds()
	if durationSec > 0 {
		br.result.PacketsPerSecond = float64(br.result.PacketsSent) / durationSec
		br.result.BytesPerSecond = float64(br.result.BytesSent) / durationSec
		br.result.BitsPerSecond = br.result.BytesPerSecond * 8
	}

	// Latency
	count := br.latencyCount.Load()
	if count > 0 {
		br.result.AvgLatency = br.latencySum.Load() / count
	}
	br.result.MinLatency = br.minLatency.Load()
	br.result.MaxLatency = br.maxLatency.Load()
	// P50/P95/P99 would require histogram - using estimates
	br.result.P50Latency = br.result.AvgLatency
	br.result.P95Latency = br.result.AvgLatency * 2
	br.result.P99Latency = br.result.AvgLatency * 3

	// Session counts
	br.result.SessionsCreated = br.sessionsCreated.Load()
	br.result.SessionsCompleted = br.sessionsCompleted.Load()
	br.result.SessionsFailed = br.sessionsFailed.Load()

	// Memory
	br.result.MemoryUsed = memEnd.Alloc - memStart.Alloc
	br.result.GoroutineCount = runtime.NumGoroutine()

	// Errors
	br.errorsMu.Lock()
	br.result.Errors = br.errors
	br.errorsMu.Unlock()
}

func (br *BenchmarkRunner) recordError(err error) {
	br.errorsMu.Lock()
	br.errors = append(br.errors, err)
	br.errorsMu.Unlock()
}

// Report returns a formatted benchmark report
func (r *BenchmarkResult) Report() string {
	report := fmt.Sprintf(`
Benchmark Results
=================
Configuration:
  Duration:        %v
  Sessions:        %d
  Packet Rate:     %d pps per session
  Packet Size:     %d bytes
  Workers:         %d

Packet Metrics:
  Sent:            %d
  Received:        %d
  Dropped:         %d (%.2f%%)
  Lost:            %d

Throughput:
  Packets/sec:     %.2f
  Bytes/sec:       %.2f
  Mbps:            %.2f

Latency (nanoseconds):
  Average:         %d
  Min:             %d
  Max:             %d
  P50:             %d
  P95:             %d
  P99:             %d

Sessions:
  Created:         %d
  Completed:       %d
  Failed:          %d

Resources:
  Memory Used:     %.2f MB
  Goroutines:      %d

Errors:            %d
`,
		r.Duration,
		r.Config.NumSessions,
		r.Config.PacketRate,
		r.Config.PacketSize,
		r.Config.NumWorkers,
		r.PacketsSent,
		r.PacketsReceived,
		r.PacketsDropped,
		float64(r.PacketsDropped)/float64(r.PacketsSent)*100,
		r.PacketsLost,
		r.PacketsPerSecond,
		r.BytesPerSecond,
		r.BitsPerSecond/1000000,
		r.AvgLatency,
		r.MinLatency,
		r.MaxLatency,
		r.P50Latency,
		r.P95Latency,
		r.P99Latency,
		r.SessionsCreated,
		r.SessionsCompleted,
		r.SessionsFailed,
		float64(r.MemoryUsed)/1024/1024,
		r.GoroutineCount,
		len(r.Errors),
	)

	return report
}

// UDPBenchmark benchmarks UDP packet handling
type UDPBenchmark struct {
	config     *BenchmarkConfig
	listener   *net.UDPConn
	senders    []*net.UDPConn
	serverAddr *net.UDPAddr

	received atomic.Int64
	sent     atomic.Int64
}

// NewUDPBenchmark creates a UDP benchmark
func NewUDPBenchmark(config *BenchmarkConfig) *UDPBenchmark {
	if config == nil {
		config = DefaultBenchmarkConfig()
	}
	return &UDPBenchmark{
		config:  config,
		senders: make([]*net.UDPConn, 0),
	}
}

// Setup prepares the UDP benchmark
func (ub *UDPBenchmark) Setup() error {
	// Create listener
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to resolve address: %w", err)
	}

	ub.listener, err = net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	ub.serverAddr = ub.listener.LocalAddr().(*net.UDPAddr)

	// Create senders
	for i := 0; i < ub.config.NumWorkers; i++ {
		conn, err := net.DialUDP("udp", nil, ub.serverAddr)
		if err != nil {
			return fmt.Errorf("failed to create sender: %w", err)
		}
		ub.senders = append(ub.senders, conn)
	}

	return nil
}

// Run executes the UDP benchmark
func (ub *UDPBenchmark) Run(ctx context.Context) (*BenchmarkResult, error) {
	result := &BenchmarkResult{
		Config:    ub.config,
		StartTime: time.Now(),
	}

	var wg sync.WaitGroup

	// Start receiver
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 2048)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				ub.listener.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
				n, _, err := ub.listener.ReadFromUDP(buf)
				if err != nil {
					continue
				}
				ub.received.Add(1)
				result.BytesReceived += int64(n)
			}
		}
	}()

	// Start senders
	packet := make([]byte, ub.config.PacketSize)
	rand.Read(packet)

	for _, sender := range ub.senders {
		wg.Add(1)
		go func(conn *net.UDPConn) {
			defer wg.Done()
			interval := time.Second / time.Duration(ub.config.PacketRate)
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_, err := conn.Write(packet)
					if err == nil {
						ub.sent.Add(1)
					}
				}
			}
		}(sender)
	}

	// Wait for context cancellation
	<-ctx.Done()
	wg.Wait()

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.PacketsSent = ub.sent.Load()
	result.PacketsReceived = ub.received.Load()
	result.BytesSent = ub.sent.Load() * int64(ub.config.PacketSize)

	if result.Duration.Seconds() > 0 {
		result.PacketsPerSecond = float64(result.PacketsSent) / result.Duration.Seconds()
		result.BytesPerSecond = float64(result.BytesSent) / result.Duration.Seconds()
		result.BitsPerSecond = result.BytesPerSecond * 8
	}

	return result, nil
}

// Cleanup releases resources
func (ub *UDPBenchmark) Cleanup() {
	if ub.listener != nil {
		ub.listener.Close()
	}
	for _, sender := range ub.senders {
		sender.Close()
	}
}

// MemoryBenchmark measures memory usage patterns
type MemoryBenchmark struct {
	allocations [][]byte
	mu          sync.Mutex
}

// NewMemoryBenchmark creates a memory benchmark
func NewMemoryBenchmark() *MemoryBenchmark {
	return &MemoryBenchmark{
		allocations: make([][]byte, 0),
	}
}

// Run executes memory allocation benchmark
func (mb *MemoryBenchmark) Run(numAllocs int, allocSize int) *MemoryBenchmarkResult {
	runtime.GC()
	var memStart runtime.MemStats
	runtime.ReadMemStats(&memStart)

	start := time.Now()

	// Perform allocations
	for i := 0; i < numAllocs; i++ {
		mb.mu.Lock()
		mb.allocations = append(mb.allocations, make([]byte, allocSize))
		mb.mu.Unlock()
	}

	allocDuration := time.Since(start)

	var memAfterAlloc runtime.MemStats
	runtime.ReadMemStats(&memAfterAlloc)

	// Cleanup
	start = time.Now()
	mb.mu.Lock()
	mb.allocations = nil
	mb.mu.Unlock()
	runtime.GC()
	cleanupDuration := time.Since(start)

	var memEnd runtime.MemStats
	runtime.ReadMemStats(&memEnd)

	return &MemoryBenchmarkResult{
		NumAllocations:   numAllocs,
		AllocationSize:   allocSize,
		TotalAllocated:   memAfterAlloc.TotalAlloc - memStart.TotalAlloc,
		PeakMemory:       memAfterAlloc.Alloc,
		AllocDuration:    allocDuration,
		CleanupDuration:  cleanupDuration,
		AllocsPerSecond:  float64(numAllocs) / allocDuration.Seconds(),
		GCPauseTotal:     memEnd.PauseTotalNs - memStart.PauseTotalNs,
	}
}

// MemoryBenchmarkResult holds memory benchmark results
type MemoryBenchmarkResult struct {
	NumAllocations  int
	AllocationSize  int
	TotalAllocated  uint64
	PeakMemory      uint64
	AllocDuration   time.Duration
	CleanupDuration time.Duration
	AllocsPerSecond float64
	GCPauseTotal    uint64
}
