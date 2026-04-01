package internal

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Backpressure errors
var (
	ErrBackpressure    = errors.New("backpressure: queue full")
	ErrBackpressureOff = errors.New("backpressure controller is off")
	ErrQueueClosed     = errors.New("queue is closed")
)

// BackpressureStrategy defines how to handle backpressure
type BackpressureStrategy int

const (
	// StrategyBlock blocks the producer until space is available
	StrategyBlock BackpressureStrategy = iota
	// StrategyDrop drops new items when queue is full
	StrategyDrop
	// StrategyDropOldest drops oldest items to make room for new ones
	StrategyDropOldest
	// StrategyReject returns an error immediately
	StrategyReject
)

func (s BackpressureStrategy) String() string {
	switch s {
	case StrategyBlock:
		return "block"
	case StrategyDrop:
		return "drop"
	case StrategyDropOldest:
		return "drop_oldest"
	case StrategyReject:
		return "reject"
	default:
		return "unknown"
	}
}

// BackpressureConfig holds configuration for backpressure control
type BackpressureConfig struct {
	MaxQueueSize     int
	Strategy         BackpressureStrategy
	BlockTimeout     time.Duration
	HighWaterMark    float64 // Percentage (0-1) to start applying backpressure
	LowWaterMark     float64 // Percentage (0-1) to release backpressure
	MetricsInterval  time.Duration
}

// DefaultBackpressureConfig returns sensible defaults
func DefaultBackpressureConfig() *BackpressureConfig {
	return &BackpressureConfig{
		MaxQueueSize:    10000,
		Strategy:        StrategyDropOldest,
		BlockTimeout:    5 * time.Second,
		HighWaterMark:   0.8,
		LowWaterMark:    0.6,
		MetricsInterval: time.Second,
	}
}

// BackpressureController manages backpressure for a component
type BackpressureController struct {
	config *BackpressureConfig

	// Current state
	queueSize     atomic.Int64
	totalEnqueued atomic.Int64
	totalDequeued atomic.Int64
	totalDropped  atomic.Int64
	totalRejected atomic.Int64

	// Pressure state
	underPressure atomic.Bool

	// Signaling
	spaceCh chan struct{}
	closeCh chan struct{}
	closed  atomic.Bool

	mu sync.RWMutex
}

// NewBackpressureController creates a new backpressure controller
func NewBackpressureController(config *BackpressureConfig) *BackpressureController {
	if config == nil {
		config = DefaultBackpressureConfig()
	}

	return &BackpressureController{
		config:  config,
		spaceCh: make(chan struct{}, 1),
		closeCh: make(chan struct{}),
	}
}

// Acquire attempts to acquire a slot in the queue
func (bp *BackpressureController) Acquire(ctx context.Context) error {
	if bp.closed.Load() {
		return ErrQueueClosed
	}

	currentSize := bp.queueSize.Load()
	maxSize := int64(bp.config.MaxQueueSize)

	switch bp.config.Strategy {
	case StrategyBlock:
		return bp.acquireBlocking(ctx)

	case StrategyDrop:
		if currentSize >= maxSize {
			bp.totalDropped.Add(1)
			return ErrBackpressure
		}
		newSize := bp.queueSize.Add(1)
		bp.totalEnqueued.Add(1)
		bp.checkHighWaterMark(newSize, maxSize)
		return nil

	case StrategyDropOldest:
		// Always accept, caller handles dropping
		newSize := bp.queueSize.Add(1)
		bp.totalEnqueued.Add(1)
		bp.checkHighWaterMark(newSize, maxSize)
		return nil

	case StrategyReject:
		if currentSize >= maxSize {
			bp.totalRejected.Add(1)
			return ErrBackpressure
		}
		newSize := bp.queueSize.Add(1)
		bp.totalEnqueued.Add(1)
		bp.checkHighWaterMark(newSize, maxSize)
		return nil

	default:
		return errors.New("unknown backpressure strategy")
	}
}

// checkHighWaterMark checks if we've hit the high water mark
func (bp *BackpressureController) checkHighWaterMark(currentSize, maxSize int64) {
	if float64(currentSize)/float64(maxSize) >= bp.config.HighWaterMark {
		bp.underPressure.Store(true)
	}
}

// acquireBlocking blocks until space is available
func (bp *BackpressureController) acquireBlocking(ctx context.Context) error {
	maxSize := int64(bp.config.MaxQueueSize)
	// Try non-blocking first
	currentSize := bp.queueSize.Load()
	if currentSize < maxSize {
		newSize := bp.queueSize.Add(1)
		bp.totalEnqueued.Add(1)
		bp.checkHighWaterMark(newSize, maxSize)
		return nil
	}

	// Set up timeout
	var timeoutCh <-chan time.Time
	if bp.config.BlockTimeout > 0 {
		timer := time.NewTimer(bp.config.BlockTimeout)
		defer timer.Stop()
		timeoutCh = timer.C
	}

	// Wait for space
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-bp.closeCh:
			return ErrQueueClosed
		case <-timeoutCh:
			bp.totalRejected.Add(1)
			return ErrBackpressure
		case <-bp.spaceCh:
			currentSize := bp.queueSize.Load()
			if currentSize < maxSize {
				newSize := bp.queueSize.Add(1)
				bp.totalEnqueued.Add(1)
				bp.checkHighWaterMark(newSize, maxSize)
				return nil
			}
			// Space was taken by another goroutine, try again
		}
	}
}

// Release releases a slot in the queue
func (bp *BackpressureController) Release() {
	newSize := bp.queueSize.Add(-1)
	bp.totalDequeued.Add(1)

	// Check low water mark
	if float64(newSize)/float64(bp.config.MaxQueueSize) <= bp.config.LowWaterMark {
		bp.underPressure.Store(false)
	}

	// Signal waiters
	select {
	case bp.spaceCh <- struct{}{}:
	default:
	}
}

// ShouldDrop returns true if oldest items should be dropped
func (bp *BackpressureController) ShouldDrop() bool {
	if bp.config.Strategy != StrategyDropOldest {
		return false
	}
	return bp.queueSize.Load() > int64(bp.config.MaxQueueSize)
}

// NotifyDropped records that an item was dropped
func (bp *BackpressureController) NotifyDropped() {
	bp.queueSize.Add(-1)
	bp.totalDropped.Add(1)
}

// IsUnderPressure returns true if backpressure is active
func (bp *BackpressureController) IsUnderPressure() bool {
	return bp.underPressure.Load()
}

// GetQueueSize returns the current queue size
func (bp *BackpressureController) GetQueueSize() int64 {
	return bp.queueSize.Load()
}

// GetQueueUtilization returns the queue utilization (0-1)
func (bp *BackpressureController) GetQueueUtilization() float64 {
	return float64(bp.queueSize.Load()) / float64(bp.config.MaxQueueSize)
}

// GetStats returns backpressure statistics
func (bp *BackpressureController) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"queue_size":       bp.queueSize.Load(),
		"max_queue_size":   bp.config.MaxQueueSize,
		"utilization":      bp.GetQueueUtilization(),
		"under_pressure":   bp.underPressure.Load(),
		"total_enqueued":   bp.totalEnqueued.Load(),
		"total_dequeued":   bp.totalDequeued.Load(),
		"total_dropped":    bp.totalDropped.Load(),
		"total_rejected":   bp.totalRejected.Load(),
		"strategy":         bp.config.Strategy.String(),
		"high_water_mark":  bp.config.HighWaterMark,
		"low_water_mark":   bp.config.LowWaterMark,
	}
}

// Close closes the backpressure controller
func (bp *BackpressureController) Close() {
	if bp.closed.CompareAndSwap(false, true) {
		close(bp.closeCh)
	}
}

// RecordingBackpressure manages backpressure for recording operations
type RecordingBackpressure struct {
	bp           *BackpressureController
	bufferPool   *sync.Pool
	maxBufferSize int

	// Per-session tracking
	sessions   map[string]*sessionBackpressure
	sessionsMu sync.RWMutex
}

type sessionBackpressure struct {
	queueSize  atomic.Int64
	dropped    atomic.Int64
	lastDrop   time.Time
	lastDropMu sync.Mutex
}

// NewRecordingBackpressure creates a new recording backpressure manager
func NewRecordingBackpressure(config *BackpressureConfig) *RecordingBackpressure {
	if config == nil {
		config = &BackpressureConfig{
			MaxQueueSize:    5000,
			Strategy:        StrategyDropOldest,
			BlockTimeout:    time.Second,
			HighWaterMark:   0.75,
			LowWaterMark:    0.5,
		}
	}

	return &RecordingBackpressure{
		bp:            NewBackpressureController(config),
		bufferPool:    &sync.Pool{New: func() interface{} { return make([]byte, 0, 2048) }},
		maxBufferSize: 2048,
		sessions:      make(map[string]*sessionBackpressure),
	}
}

// AcquireBuffer attempts to acquire a buffer for recording
func (rb *RecordingBackpressure) AcquireBuffer(ctx context.Context, sessionID string) ([]byte, error) {
	// Global backpressure check
	if err := rb.bp.Acquire(ctx); err != nil {
		rb.recordDrop(sessionID)
		return nil, err
	}

	// Session-level tracking
	sb := rb.getOrCreateSession(sessionID)
	sb.queueSize.Add(1)

	// Get buffer from pool
	buf := rb.bufferPool.Get().([]byte)
	return buf[:0], nil
}

// ReleaseBuffer releases a buffer back to the pool
func (rb *RecordingBackpressure) ReleaseBuffer(buf []byte, sessionID string) {
	rb.bp.Release()

	sb := rb.getSession(sessionID)
	if sb != nil {
		sb.queueSize.Add(-1)
	}

	// Only recycle if buffer isn't oversized
	if cap(buf) <= rb.maxBufferSize*2 {
		rb.bufferPool.Put(buf[:0])
	}
}

// ShouldDropOldest checks if oldest packets should be dropped
func (rb *RecordingBackpressure) ShouldDropOldest() bool {
	return rb.bp.ShouldDrop()
}

// NotifyDropped notifies that a packet was dropped
func (rb *RecordingBackpressure) NotifyDropped(sessionID string) {
	rb.bp.NotifyDropped()
	rb.recordDrop(sessionID)
}

func (rb *RecordingBackpressure) recordDrop(sessionID string) {
	sb := rb.getOrCreateSession(sessionID)
	sb.dropped.Add(1)
	sb.lastDropMu.Lock()
	sb.lastDrop = time.Now()
	sb.lastDropMu.Unlock()
}

func (rb *RecordingBackpressure) getOrCreateSession(sessionID string) *sessionBackpressure {
	rb.sessionsMu.RLock()
	sb, exists := rb.sessions[sessionID]
	rb.sessionsMu.RUnlock()

	if exists {
		return sb
	}

	rb.sessionsMu.Lock()
	defer rb.sessionsMu.Unlock()

	// Double check
	if sb, exists = rb.sessions[sessionID]; exists {
		return sb
	}

	sb = &sessionBackpressure{}
	rb.sessions[sessionID] = sb
	return sb
}

func (rb *RecordingBackpressure) getSession(sessionID string) *sessionBackpressure {
	rb.sessionsMu.RLock()
	defer rb.sessionsMu.RUnlock()
	return rb.sessions[sessionID]
}

// RemoveSession removes session tracking
func (rb *RecordingBackpressure) RemoveSession(sessionID string) {
	rb.sessionsMu.Lock()
	delete(rb.sessions, sessionID)
	rb.sessionsMu.Unlock()
}

// IsUnderPressure returns global pressure state
func (rb *RecordingBackpressure) IsUnderPressure() bool {
	return rb.bp.IsUnderPressure()
}

// GetStats returns statistics
func (rb *RecordingBackpressure) GetStats() map[string]interface{} {
	stats := rb.bp.GetStats()

	rb.sessionsMu.RLock()
	sessionStats := make(map[string]map[string]interface{})
	for id, sb := range rb.sessions {
		sb.lastDropMu.Lock()
		lastDrop := sb.lastDrop
		sb.lastDropMu.Unlock()

		sessionStats[id] = map[string]interface{}{
			"queue_size": sb.queueSize.Load(),
			"dropped":    sb.dropped.Load(),
			"last_drop":  lastDrop.Format(time.RFC3339),
		}
	}
	rb.sessionsMu.RUnlock()

	stats["sessions"] = sessionStats
	stats["session_count"] = len(sessionStats)
	return stats
}

// Close closes the recording backpressure manager
func (rb *RecordingBackpressure) Close() {
	rb.bp.Close()
}

// MediaBackpressure provides backpressure for media forwarding
type MediaBackpressure struct {
	// Per-stream tracking
	streams   map[string]*streamBackpressure
	streamsMu sync.RWMutex

	// Global limits
	maxPacketsPerStream int64
	maxTotalPackets     int64
	totalPackets        atomic.Int64

	// Metrics
	totalDropped  atomic.Int64
	totalThrottled atomic.Int64
}

type streamBackpressure struct {
	packets     atomic.Int64
	dropped     atomic.Int64
	throttled   bool
	throttleMu  sync.RWMutex
}

// NewMediaBackpressure creates a new media backpressure controller
func NewMediaBackpressure(maxPacketsPerStream, maxTotalPackets int64) *MediaBackpressure {
	return &MediaBackpressure{
		streams:             make(map[string]*streamBackpressure),
		maxPacketsPerStream: maxPacketsPerStream,
		maxTotalPackets:     maxTotalPackets,
	}
}

// AllowPacket checks if a packet should be processed
func (mb *MediaBackpressure) AllowPacket(streamID string) bool {
	// Check global limit
	total := mb.totalPackets.Load()
	if total >= mb.maxTotalPackets {
		mb.totalDropped.Add(1)
		return false
	}

	// Check per-stream limit
	sb := mb.getOrCreateStream(streamID)

	sb.throttleMu.RLock()
	throttled := sb.throttled
	sb.throttleMu.RUnlock()

	if throttled {
		sb.dropped.Add(1)
		mb.totalThrottled.Add(1)
		return false
	}

	packets := sb.packets.Add(1)
	mb.totalPackets.Add(1)

	if packets > mb.maxPacketsPerStream {
		sb.throttleMu.Lock()
		sb.throttled = true
		sb.throttleMu.Unlock()
		sb.dropped.Add(1)
		mb.totalThrottled.Add(1)
		return false
	}

	return true
}

// PacketProcessed marks a packet as processed
func (mb *MediaBackpressure) PacketProcessed(streamID string) {
	sb := mb.getStream(streamID)
	if sb != nil {
		newCount := sb.packets.Add(-1)
		mb.totalPackets.Add(-1)

		// Release throttle if below threshold
		if newCount < mb.maxPacketsPerStream/2 {
			sb.throttleMu.Lock()
			sb.throttled = false
			sb.throttleMu.Unlock()
		}
	}
}

func (mb *MediaBackpressure) getOrCreateStream(streamID string) *streamBackpressure {
	mb.streamsMu.RLock()
	sb, exists := mb.streams[streamID]
	mb.streamsMu.RUnlock()

	if exists {
		return sb
	}

	mb.streamsMu.Lock()
	defer mb.streamsMu.Unlock()

	if sb, exists = mb.streams[streamID]; exists {
		return sb
	}

	sb = &streamBackpressure{}
	mb.streams[streamID] = sb
	return sb
}

func (mb *MediaBackpressure) getStream(streamID string) *streamBackpressure {
	mb.streamsMu.RLock()
	defer mb.streamsMu.RUnlock()
	return mb.streams[streamID]
}

// RemoveStream removes stream tracking
func (mb *MediaBackpressure) RemoveStream(streamID string) {
	mb.streamsMu.Lock()
	if sb, exists := mb.streams[streamID]; exists {
		mb.totalPackets.Add(-sb.packets.Load())
		delete(mb.streams, streamID)
	}
	mb.streamsMu.Unlock()
}

// GetStats returns statistics
func (mb *MediaBackpressure) GetStats() map[string]interface{} {
	mb.streamsMu.RLock()
	streamCount := len(mb.streams)
	mb.streamsMu.RUnlock()

	return map[string]interface{}{
		"total_packets":          mb.totalPackets.Load(),
		"max_total_packets":      mb.maxTotalPackets,
		"max_packets_per_stream": mb.maxPacketsPerStream,
		"stream_count":           streamCount,
		"total_dropped":          mb.totalDropped.Load(),
		"total_throttled":        mb.totalThrottled.Load(),
	}
}

// AdaptiveBackpressure provides dynamic backpressure based on system load
type AdaptiveBackpressure struct {
	baseController *BackpressureController

	// System metrics
	cpuUsage     atomic.Int64 // Percentage * 100
	memoryUsage  atomic.Int64 // Percentage * 100

	// Adaptive thresholds
	cpuThreshold    int64
	memoryThreshold int64

	// Scaling
	scaleFactor atomic.Int64 // Percentage of max capacity
	minScale    int64
	maxScale    int64

	mu sync.RWMutex
}

// NewAdaptiveBackpressure creates a new adaptive backpressure controller
func NewAdaptiveBackpressure(config *BackpressureConfig) *AdaptiveBackpressure {
	ab := &AdaptiveBackpressure{
		baseController:   NewBackpressureController(config),
		cpuThreshold:    70,  // 70%
		memoryThreshold: 80,  // 80%
		minScale:        20,  // 20% minimum capacity
		maxScale:        100, // 100% maximum capacity
	}
	ab.scaleFactor.Store(100) // Start at full capacity
	return ab
}

// UpdateSystemMetrics updates CPU and memory usage
func (ab *AdaptiveBackpressure) UpdateSystemMetrics(cpuPercent, memoryPercent float64) {
	ab.cpuUsage.Store(int64(cpuPercent * 100))
	ab.memoryUsage.Store(int64(memoryPercent * 100))

	// Adjust scale factor based on system load
	ab.adjustScale()
}

func (ab *AdaptiveBackpressure) adjustScale() {
	cpu := ab.cpuUsage.Load()
	mem := ab.memoryUsage.Load()

	// Calculate pressure (0-100)
	cpuPressure := int64(0)
	if cpu > ab.cpuThreshold*100 {
		cpuPressure = (cpu - ab.cpuThreshold*100) / 100
	}

	memPressure := int64(0)
	if mem > ab.memoryThreshold*100 {
		memPressure = (mem - ab.memoryThreshold*100) / 100
	}

	// Use max pressure
	pressure := cpuPressure
	if memPressure > pressure {
		pressure = memPressure
	}

	// Calculate new scale (inverse of pressure)
	newScale := ab.maxScale - pressure
	if newScale < ab.minScale {
		newScale = ab.minScale
	}

	ab.scaleFactor.Store(newScale)
}

// GetEffectiveCapacity returns the current effective capacity
func (ab *AdaptiveBackpressure) GetEffectiveCapacity() int64 {
	baseCapacity := int64(ab.baseController.config.MaxQueueSize)
	scale := ab.scaleFactor.Load()
	return baseCapacity * scale / 100
}

// Acquire attempts to acquire with adaptive capacity
func (ab *AdaptiveBackpressure) Acquire(ctx context.Context) error {
	currentSize := ab.baseController.queueSize.Load()
	effectiveCapacity := ab.GetEffectiveCapacity()

	if currentSize >= effectiveCapacity {
		ab.baseController.totalRejected.Add(1)
		return ErrBackpressure
	}

	return ab.baseController.Acquire(ctx)
}

// Release releases a slot
func (ab *AdaptiveBackpressure) Release() {
	ab.baseController.Release()
}

// GetStats returns statistics
func (ab *AdaptiveBackpressure) GetStats() map[string]interface{} {
	stats := ab.baseController.GetStats()
	stats["cpu_usage"] = float64(ab.cpuUsage.Load()) / 100
	stats["memory_usage"] = float64(ab.memoryUsage.Load()) / 100
	stats["scale_factor"] = ab.scaleFactor.Load()
	stats["effective_capacity"] = ab.GetEffectiveCapacity()
	return stats
}

// Close closes the controller
func (ab *AdaptiveBackpressure) Close() {
	ab.baseController.Close()
}
