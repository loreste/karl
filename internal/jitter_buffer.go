package internal

import (
	"container/heap"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Jitter buffer metrics
var (
	jitterBufferSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "karl_jitter_buffer_size",
			Help: "Current jitter buffer size in packets",
		},
		[]string{"session_id"},
	)

	jitterBufferLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "karl_jitter_buffer_latency_seconds",
			Help:    "Jitter buffer latency",
			Buckets: []float64{0.01, 0.02, 0.05, 0.1, 0.2, 0.5},
		},
		[]string{"session_id"},
	)

	jitterBufferPacketsDropped = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "karl_jitter_buffer_packets_dropped_total",
			Help: "Total packets dropped by jitter buffer",
		},
		[]string{"session_id", "reason"},
	)

	jitterBufferPacketsRecovered = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "karl_jitter_buffer_packets_recovered_total",
			Help: "Total packets recovered from reordering",
		},
		[]string{"session_id"},
	)
)

// JitterBufferInternalConfig holds jitter buffer runtime configuration with time.Duration types
type JitterBufferInternalConfig struct {
	MinDelay      time.Duration
	MaxDelay      time.Duration
	TargetDelay   time.Duration
	AdaptiveMode  bool
	MaxSize       int
}

// DefaultJitterBufferInternalConfig returns default jitter buffer configuration
func DefaultJitterBufferInternalConfig() *JitterBufferInternalConfig {
	return &JitterBufferInternalConfig{
		MinDelay:     20 * time.Millisecond,
		MaxDelay:     200 * time.Millisecond,
		TargetDelay:  50 * time.Millisecond,
		AdaptiveMode: true,
		MaxSize:      100,
	}
}

// ToInternalConfig converts JitterBufferConfig to JitterBufferInternalConfig
func ToJitterBufferInternalConfig(cfg *JitterBufferConfig) *JitterBufferInternalConfig {
	if cfg == nil {
		return DefaultJitterBufferInternalConfig()
	}
	return &JitterBufferInternalConfig{
		MinDelay:     time.Duration(cfg.MinDelay) * time.Millisecond,
		MaxDelay:     time.Duration(cfg.MaxDelay) * time.Millisecond,
		TargetDelay:  time.Duration(cfg.TargetDelay) * time.Millisecond,
		AdaptiveMode: cfg.AdaptiveMode,
		MaxSize:      cfg.MaxSize,
	}
}

// BufferedPacket represents a packet in the jitter buffer
type BufferedPacket struct {
	SequenceNumber uint16
	Timestamp      uint32
	Payload        []byte
	ReceivedAt     time.Time
	index          int // Used by heap
}

// PacketHeap implements a min-heap for packets ordered by sequence number
type PacketHeap []*BufferedPacket

func (h PacketHeap) Len() int           { return len(h) }
func (h PacketHeap) Less(i, j int) bool { return seqLess(h[i].SequenceNumber, h[j].SequenceNumber) }
func (h PacketHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *PacketHeap) Push(x interface{}) {
	n := len(*h)
	item := x.(*BufferedPacket)
	item.index = n
	*h = append(*h, item)
}

func (h *PacketHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	item.index = -1
	*h = old[0 : n-1]
	return item
}

// seqLess compares sequence numbers with wraparound handling
func seqLess(a, b uint16) bool {
	// Handle wraparound: if difference is more than half the range, the "larger" one is actually smaller
	diff := int16(a - b)
	return diff < 0
}

// JitterBuffer implements an adaptive jitter buffer
type JitterBuffer struct {
	config       *JitterBufferInternalConfig
	sessionID    string
	clockRate    uint32

	// Packet storage
	packets      PacketHeap
	packetMap    map[uint16]*BufferedPacket
	mu           sync.Mutex

	// Sequence tracking
	nextExpected uint16
	initialized  bool

	// Timing
	currentDelay time.Duration
	lastPlayTime time.Time

	// Statistics
	packetsIn      uint64
	packetsOut     uint64
	packetsDropped uint64
	packetsLate    uint64
	reordered      uint64

	// Adaptive parameters
	jitterEstimate float64
	avgDelay       float64
}

// NewJitterBuffer creates a new jitter buffer
func NewJitterBuffer(sessionID string, clockRate uint32, config *JitterBufferInternalConfig) *JitterBuffer {
	if config == nil {
		config = DefaultJitterBufferInternalConfig()
	}

	jb := &JitterBuffer{
		config:       config,
		sessionID:    sessionID,
		clockRate:    clockRate,
		packets:      make(PacketHeap, 0, config.MaxSize),
		packetMap:    make(map[uint16]*BufferedPacket),
		currentDelay: config.TargetDelay,
	}

	heap.Init(&jb.packets)

	return jb
}

// NewJitterBufferFromConfig creates a jitter buffer from external config
func NewJitterBufferFromConfig(sessionID string, clockRate uint32, config *JitterBufferConfig) *JitterBuffer {
	return NewJitterBuffer(sessionID, clockRate, ToJitterBufferInternalConfig(config))
}

// Push adds a packet to the jitter buffer
func (jb *JitterBuffer) Push(seq uint16, timestamp uint32, payload []byte) bool {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	now := time.Now()
	jb.packetsIn++

	// Initialize on first packet
	if !jb.initialized {
		jb.nextExpected = seq
		jb.initialized = true
		jb.lastPlayTime = now.Add(jb.currentDelay)
	}

	// Check for duplicate
	if _, exists := jb.packetMap[seq]; exists {
		jitterBufferPacketsDropped.WithLabelValues(jb.sessionID, "duplicate").Inc()
		return false
	}

	// Check if packet is too old
	if seqLess(seq, jb.nextExpected) {
		jb.packetsLate++
		jitterBufferPacketsDropped.WithLabelValues(jb.sessionID, "late").Inc()
		return false
	}

	// Check if buffer is full
	if len(jb.packets) >= jb.config.MaxSize {
		// Drop oldest packet
		oldest := heap.Pop(&jb.packets).(*BufferedPacket)
		delete(jb.packetMap, oldest.SequenceNumber)
		jb.packetsDropped++
		jitterBufferPacketsDropped.WithLabelValues(jb.sessionID, "overflow").Inc()
	}

	// Create and add packet
	pkt := &BufferedPacket{
		SequenceNumber: seq,
		Timestamp:      timestamp,
		Payload:        make([]byte, len(payload)),
		ReceivedAt:     now,
	}
	copy(pkt.Payload, payload)

	heap.Push(&jb.packets, pkt)
	jb.packetMap[seq] = pkt

	// Check for reordering
	if seq != jb.nextExpected && seqLess(jb.nextExpected, seq) {
		jb.reordered++
		jitterBufferPacketsRecovered.WithLabelValues(jb.sessionID).Inc()
	}

	// Update adaptive delay if enabled
	if jb.config.AdaptiveMode {
		jb.updateAdaptiveDelay(pkt)
	}

	// Update metrics
	jitterBufferSize.WithLabelValues(jb.sessionID).Set(float64(len(jb.packets)))

	return true
}

// Pop retrieves the next packet from the jitter buffer
func (jb *JitterBuffer) Pop() (*BufferedPacket, bool) {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	if len(jb.packets) == 0 {
		return nil, false
	}

	now := time.Now()

	// Check if it's time to play out a packet
	if now.Before(jb.lastPlayTime) {
		return nil, false
	}

	// Get the next expected packet if available
	pkt, exists := jb.packetMap[jb.nextExpected]
	if exists {
		// Remove from heap and map
		heap.Remove(&jb.packets, pkt.index)
		delete(jb.packetMap, pkt.SequenceNumber)

		jb.nextExpected++
		jb.packetsOut++

		// Update play time based on packet timing
		if jb.clockRate > 0 {
			jb.lastPlayTime = now.Add(time.Duration(float64(time.Second) / float64(jb.clockRate)))
		} else {
			jb.lastPlayTime = now.Add(20 * time.Millisecond) // Default 20ms for audio
		}

		// Record latency
		latency := now.Sub(pkt.ReceivedAt)
		jitterBufferLatency.WithLabelValues(jb.sessionID).Observe(latency.Seconds())

		jitterBufferSize.WithLabelValues(jb.sessionID).Set(float64(len(jb.packets)))

		return pkt, true
	}

	// Check if we should skip this sequence number
	// (e.g., if we've waited long enough and have later packets)
	if len(jb.packets) > 0 {
		oldestPkt := jb.packets[0]

		// If we've waited too long, consider the packet lost
		waitTime := now.Sub(jb.lastPlayTime)
		if waitTime > jb.config.MaxDelay {
			// Skip to the oldest available packet
			jb.nextExpected = oldestPkt.SequenceNumber

			// Pop and return the oldest packet
			pkt := heap.Pop(&jb.packets).(*BufferedPacket)
			delete(jb.packetMap, pkt.SequenceNumber)

			jb.nextExpected++
			jb.packetsOut++

			if jb.clockRate > 0 {
				jb.lastPlayTime = now.Add(time.Duration(float64(time.Second) / float64(jb.clockRate)))
			} else {
				jb.lastPlayTime = now.Add(20 * time.Millisecond)
			}

			jitterBufferSize.WithLabelValues(jb.sessionID).Set(float64(len(jb.packets)))

			return pkt, true
		}
	}

	return nil, false
}

// PopWithTimeout retrieves the next packet with a timeout
func (jb *JitterBuffer) PopWithTimeout(timeout time.Duration) (*BufferedPacket, bool) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if pkt, ok := jb.Pop(); ok {
			return pkt, true
		}
		time.Sleep(time.Millisecond)
	}

	return nil, false
}

// PopBatch retrieves multiple packets at once
func (jb *JitterBuffer) PopBatch(maxPackets int) []*BufferedPacket {
	packets := make([]*BufferedPacket, 0, maxPackets)

	for i := 0; i < maxPackets; i++ {
		pkt, ok := jb.Pop()
		if !ok {
			break
		}
		packets = append(packets, pkt)
	}

	return packets
}

// updateAdaptiveDelay updates the adaptive delay based on network conditions
func (jb *JitterBuffer) updateAdaptiveDelay(pkt *BufferedPacket) {
	// Calculate inter-arrival jitter
	// This is a simplified version of RFC 3550 jitter calculation

	if jb.packetsIn < 2 {
		return
	}

	// Estimate delay based on packet arrival variance
	delay := time.Since(pkt.ReceivedAt).Seconds()
	jb.avgDelay = jb.avgDelay*0.9 + delay*0.1

	// Estimate jitter
	diff := delay - jb.avgDelay
	if diff < 0 {
		diff = -diff
	}
	jb.jitterEstimate = jb.jitterEstimate*0.9 + diff*0.1

	// Calculate new target delay
	// Target = avg delay + 2 * jitter (for some safety margin)
	targetDelay := time.Duration((jb.avgDelay + 2*jb.jitterEstimate) * float64(time.Second))

	// Clamp to min/max
	if targetDelay < jb.config.MinDelay {
		targetDelay = jb.config.MinDelay
	}
	if targetDelay > jb.config.MaxDelay {
		targetDelay = jb.config.MaxDelay
	}

	// Slowly adapt current delay
	diff2 := float64(targetDelay - jb.currentDelay)
	adjustment := time.Duration(diff2 * 0.1)
	jb.currentDelay += adjustment
}

// Flush clears the jitter buffer
func (jb *JitterBuffer) Flush() {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	jb.packets = make(PacketHeap, 0, jb.config.MaxSize)
	jb.packetMap = make(map[uint16]*BufferedPacket)
	heap.Init(&jb.packets)

	jb.packetsDropped += uint64(len(jb.packets))

	jitterBufferSize.WithLabelValues(jb.sessionID).Set(0)
}

// Reset resets the jitter buffer state
func (jb *JitterBuffer) Reset() {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	jb.packets = make(PacketHeap, 0, jb.config.MaxSize)
	jb.packetMap = make(map[uint16]*BufferedPacket)
	heap.Init(&jb.packets)

	jb.initialized = false
	jb.packetsIn = 0
	jb.packetsOut = 0
	jb.packetsDropped = 0
	jb.packetsLate = 0
	jb.reordered = 0
	jb.jitterEstimate = 0
	jb.avgDelay = 0
	jb.currentDelay = jb.config.TargetDelay

	jitterBufferSize.WithLabelValues(jb.sessionID).Set(0)
}

// Size returns the current buffer size
func (jb *JitterBuffer) Size() int {
	jb.mu.Lock()
	defer jb.mu.Unlock()
	return len(jb.packets)
}

// Stats returns jitter buffer statistics
type JitterBufferStats struct {
	PacketsIn      uint64
	PacketsOut     uint64
	PacketsDropped uint64
	PacketsLate    uint64
	Reordered      uint64
	CurrentSize    int
	CurrentDelay   time.Duration
	JitterEstimate float64
}

// GetStats returns current jitter buffer statistics
func (jb *JitterBuffer) GetStats() JitterBufferStats {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	return JitterBufferStats{
		PacketsIn:      jb.packetsIn,
		PacketsOut:     jb.packetsOut,
		PacketsDropped: jb.packetsDropped,
		PacketsLate:    jb.packetsLate,
		Reordered:      jb.reordered,
		CurrentSize:    len(jb.packets),
		CurrentDelay:   jb.currentDelay,
		JitterEstimate: jb.jitterEstimate,
	}
}

// IsEmpty returns whether the buffer is empty
func (jb *JitterBuffer) IsEmpty() bool {
	jb.mu.Lock()
	defer jb.mu.Unlock()
	return len(jb.packets) == 0
}

// GetCurrentDelay returns the current playout delay
func (jb *JitterBuffer) GetCurrentDelay() time.Duration {
	jb.mu.Lock()
	defer jb.mu.Unlock()
	return jb.currentDelay
}

// SetTargetDelay sets the target delay
func (jb *JitterBuffer) SetTargetDelay(delay time.Duration) {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	if delay < jb.config.MinDelay {
		delay = jb.config.MinDelay
	}
	if delay > jb.config.MaxDelay {
		delay = jb.config.MaxDelay
	}

	jb.config.TargetDelay = delay
}

// SetAdaptiveMode enables or disables adaptive mode
func (jb *JitterBuffer) SetAdaptiveMode(enabled bool) {
	jb.mu.Lock()
	defer jb.mu.Unlock()
	jb.config.AdaptiveMode = enabled
}

// GetNextExpected returns the next expected sequence number
func (jb *JitterBuffer) GetNextExpected() uint16 {
	jb.mu.Lock()
	defer jb.mu.Unlock()
	return jb.nextExpected
}

// HasPacket checks if a specific sequence number is in the buffer
func (jb *JitterBuffer) HasPacket(seq uint16) bool {
	jb.mu.Lock()
	defer jb.mu.Unlock()
	_, exists := jb.packetMap[seq]
	return exists
}

// PeekNext returns the next packet without removing it
func (jb *JitterBuffer) PeekNext() (*BufferedPacket, bool) {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	if pkt, exists := jb.packetMap[jb.nextExpected]; exists {
		return pkt, true
	}
	return nil, false
}
