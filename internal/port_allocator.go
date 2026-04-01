package internal

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Port allocation errors
var (
	ErrNoPortsAvailable    = errors.New("no ports available in range")
	ErrPortInUse           = errors.New("port already in use")
	ErrPortOutOfRange      = errors.New("port out of allowed range")
	ErrPortPoolExhausted   = errors.New("port pool exhausted")
	ErrPortAllocationLimit = errors.New("port allocation limit reached")
)

// PortAllocatorConfig holds configuration for port allocation
type PortAllocatorConfig struct {
	MinPort        int
	MaxPort        int
	ReserveCount   int           // Number of port pairs to pre-allocate
	ReuseDelay     time.Duration // Time before a released port can be reused
	MaxAllocations int           // Maximum simultaneous allocations per session
	EvenOnly       bool          // Only allocate even ports (for RTP)
}

// DefaultPortAllocatorConfig returns sensible defaults optimized for performance
func DefaultPortAllocatorConfig() *PortAllocatorConfig {
	return &PortAllocatorConfig{
		MinPort:        10000,
		MaxPort:        60000,
		ReserveCount:   500, // Pre-allocate more pairs for faster allocation
		ReuseDelay:     2 * time.Second,
		MaxAllocations: 100,
		EvenOnly:       true,
	}
}

// portPair represents a pre-allocated RTP/RTCP port pair
type portPair struct {
	rtp  int
	rtcp int
}

// PortAllocator manages port allocation with exhaustion protection
type PortAllocator struct {
	config *PortAllocatorConfig

	// Lockless port pair pool using channel
	pairPool chan portPair

	// Port tracking with sharded locks for reduced contention
	shards     [16]*portShard
	shardMask  uint32

	// Released ports waiting for reuse
	released   sync.Map // port -> releaseTime

	// Metrics (lock-free)
	totalAllocated atomic.Int64
	totalReleased  atomic.Int64
	totalFailed    atomic.Int64
	currentInUse   atomic.Int64
	peakInUse      atomic.Int64
	poolHits       atomic.Int64
	poolMisses     atomic.Int64

	// Per-session tracking with sharded lock
	sessionShards [16]*sessionShard

	// Next port hint for faster scanning
	nextPort atomic.Int32

	// State
	closed   atomic.Bool
	stopCh   chan struct{}
	refillWg sync.WaitGroup
}

// portShard handles a subset of ports with its own lock
type portShard struct {
	mu        sync.RWMutex
	allocated map[int]portInfo
}

// sessionShard handles a subset of sessions
type sessionShard struct {
	mu    sync.RWMutex
	ports map[string][]int
}

type portInfo struct {
	port        int
	sessionID   string
	allocatedAt time.Time
	conn        net.PacketConn
}

// NewPortAllocator creates a new high-performance port allocator
func NewPortAllocator(config *PortAllocatorConfig) *PortAllocator {
	if config == nil {
		config = DefaultPortAllocatorConfig()
	}

	pa := &PortAllocator{
		config:    config,
		pairPool:  make(chan portPair, config.ReserveCount),
		shardMask: 15,
		stopCh:    make(chan struct{}),
	}

	// Initialize shards
	for i := 0; i < 16; i++ {
		pa.shards[i] = &portShard{
			allocated: make(map[int]portInfo),
		}
		pa.sessionShards[i] = &sessionShard{
			ports: make(map[string][]int),
		}
	}

	// Set starting port
	start := config.MinPort
	if config.EvenOnly && start%2 != 0 {
		start++
	}
	pa.nextPort.Store(int32(start))

	// Start pool refiller
	pa.refillWg.Add(1)
	go pa.poolRefiller()

	// Start released port cleaner
	go pa.releasedCleaner()

	return pa
}

// poolRefiller continuously refills the port pair pool
func (pa *PortAllocator) poolRefiller() {
	defer pa.refillWg.Done()

	for {
		select {
		case <-pa.stopCh:
			return
		default:
			// Check if pool needs refilling
			if len(pa.pairPool) < pa.config.ReserveCount/2 {
				pa.refillPool()
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// refillPool adds port pairs to the pool
func (pa *PortAllocator) refillPool() {
	target := pa.config.ReserveCount - len(pa.pairPool)
	if target <= 0 {
		return
	}

	for i := 0; i < target; i++ {
		pair, ok := pa.findAvailablePortPair()
		if !ok {
			return
		}

		select {
		case pa.pairPool <- pair:
		default:
			// Pool full, release the pair
			pa.markPortAvailable(pair.rtp)
			pa.markPortAvailable(pair.rtcp)
			return
		}
	}
}

// releasedCleaner periodically cleans up released ports
func (pa *PortAllocator) releasedCleaner() {
	// Use shorter interval for responsive cleanup
	interval := pa.config.ReuseDelay / 2
	if interval < 50*time.Millisecond {
		interval = 50 * time.Millisecond
	}
	if interval > time.Second {
		interval = time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-pa.stopCh:
			return
		case <-ticker.C:
			now := time.Now()
			pa.released.Range(func(key, value interface{}) bool {
				if releaseTime, ok := value.(time.Time); ok {
					if now.Sub(releaseTime) >= pa.config.ReuseDelay {
						pa.released.Delete(key)
					}
				}
				return true
			})
		}
	}
}

// getShard returns the shard for a port
func (pa *PortAllocator) getShard(port int) *portShard {
	return pa.shards[uint32(port)&pa.shardMask]
}

// getSessionShard returns the session shard for a session ID
func (pa *PortAllocator) getSessionShard(sessionID string) *sessionShard {
	h := fnv1aString(sessionID)
	return pa.sessionShards[h&pa.shardMask]
}

// fnv1aString computes FNV-1a hash of a string
func fnv1aString(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// AllocatePort allocates a single port
func (pa *PortAllocator) AllocatePort(sessionID string) (int, error) {
	if pa.closed.Load() {
		return 0, errors.New("port allocator is closed")
	}

	// Check session limit
	ss := pa.getSessionShard(sessionID)
	ss.mu.RLock()
	count := len(ss.ports[sessionID])
	ss.mu.RUnlock()

	if count >= pa.config.MaxAllocations {
		pa.totalFailed.Add(1)
		return 0, ErrPortAllocationLimit
	}

	// Try to get from pool (just take the RTP port from a pair)
	select {
	case pair := <-pa.pairPool:
		pa.poolHits.Add(1)
		// Return RTCP to pool for single port allocation
		go func() {
			select {
			case pa.pairPool <- portPair{rtp: pair.rtcp, rtcp: pair.rtcp + 1}:
			default:
				pa.markPortAvailable(pair.rtcp)
			}
		}()
		pa.recordAllocation(pair.rtp, sessionID, nil)
		return pair.rtp, nil
	default:
		pa.poolMisses.Add(1)
	}

	// Find a new port
	port, err := pa.findAndAllocatePort(sessionID)
	if err != nil {
		pa.totalFailed.Add(1)
		return 0, err
	}

	return port, nil
}

// AllocatePortPair allocates a pair of consecutive ports (for RTP/RTCP)
func (pa *PortAllocator) AllocatePortPair(sessionID string) (rtpPort, rtcpPort int, err error) {
	if pa.closed.Load() {
		return 0, 0, errors.New("port allocator is closed")
	}

	// Check session limit
	ss := pa.getSessionShard(sessionID)
	ss.mu.RLock()
	count := len(ss.ports[sessionID])
	ss.mu.RUnlock()

	if count+2 > pa.config.MaxAllocations {
		pa.totalFailed.Add(1)
		return 0, 0, ErrPortAllocationLimit
	}

	// Try to get from pool first (fast path)
	select {
	case pair := <-pa.pairPool:
		pa.poolHits.Add(1)
		pa.recordAllocation(pair.rtp, sessionID, nil)
		pa.recordAllocation(pair.rtcp, sessionID, nil)
		return pair.rtp, pair.rtcp, nil
	default:
		pa.poolMisses.Add(1)
	}

	// Slow path: find and allocate directly
	pair, ok := pa.findAvailablePortPair()
	if !ok {
		pa.totalFailed.Add(2)
		return 0, 0, ErrNoPortsAvailable
	}

	pa.recordAllocation(pair.rtp, sessionID, nil)
	pa.recordAllocation(pair.rtcp, sessionID, nil)

	return pair.rtp, pair.rtcp, nil
}

// findAvailablePortPair finds an available port pair
func (pa *PortAllocator) findAvailablePortPair() (portPair, bool) {
	start := int(pa.nextPort.Load())
	end := pa.config.MaxPort - 1

	// Scan from hint
	for port := start; port < end; port += 2 {
		if pa.tryReservePortPair(port) {
			pa.nextPort.Store(int32(port + 2))
			return portPair{rtp: port, rtcp: port + 1}, true
		}
	}

	// Wrap around
	wrapStart := pa.config.MinPort
	if pa.config.EvenOnly && wrapStart%2 != 0 {
		wrapStart++
	}

	for port := wrapStart; port < start && port < end; port += 2 {
		if pa.tryReservePortPair(port) {
			pa.nextPort.Store(int32(port + 2))
			return portPair{rtp: port, rtcp: port + 1}, true
		}
	}

	return portPair{}, false
}

// tryReservePortPair attempts to reserve a port pair
func (pa *PortAllocator) tryReservePortPair(rtpPort int) bool {
	rtcpPort := rtpPort + 1

	// Check released map first (fast)
	if _, ok := pa.released.Load(rtpPort); ok {
		return false
	}
	if _, ok := pa.released.Load(rtcpPort); ok {
		return false
	}

	// Check allocated maps
	rtpShard := pa.getShard(rtpPort)
	rtcpShard := pa.getShard(rtcpPort)

	// Lock in order to prevent deadlock
	if rtpShard == rtcpShard {
		rtpShard.mu.Lock()
		defer rtpShard.mu.Unlock()

		if _, exists := rtpShard.allocated[rtpPort]; exists {
			return false
		}
		if _, exists := rtpShard.allocated[rtcpPort]; exists {
			return false
		}

		// Try to bind
		if !pa.tryBind(rtpPort) || !pa.tryBind(rtcpPort) {
			return false
		}

		// Mark as allocated (temporary, will be set properly in recordAllocation)
		rtpShard.allocated[rtpPort] = portInfo{port: rtpPort, allocatedAt: time.Now()}
		rtpShard.allocated[rtcpPort] = portInfo{port: rtcpPort, allocatedAt: time.Now()}
		return true
	}

	// Different shards - lock both
	rtpShard.mu.Lock()
	rtcpShard.mu.Lock()

	defer rtpShard.mu.Unlock()
	defer rtcpShard.mu.Unlock()

	if _, exists := rtpShard.allocated[rtpPort]; exists {
		return false
	}
	if _, exists := rtcpShard.allocated[rtcpPort]; exists {
		return false
	}

	if !pa.tryBind(rtpPort) || !pa.tryBind(rtcpPort) {
		return false
	}

	rtpShard.allocated[rtpPort] = portInfo{port: rtpPort, allocatedAt: time.Now()}
	rtcpShard.allocated[rtcpPort] = portInfo{port: rtcpPort, allocatedAt: time.Now()}
	return true
}

// tryBind attempts to bind to a port
func (pa *PortAllocator) tryBind(port int) bool {
	conn, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// markPortAvailable marks a port as available (for pool overflow)
func (pa *PortAllocator) markPortAvailable(port int) {
	shard := pa.getShard(port)
	shard.mu.Lock()
	delete(shard.allocated, port)
	shard.mu.Unlock()
}

// findAndAllocatePort finds and allocates a single port
func (pa *PortAllocator) findAndAllocatePort(sessionID string) (int, error) {
	start := int(pa.nextPort.Load())
	step := 1
	if pa.config.EvenOnly {
		step = 2
	}

	// Scan from hint
	for port := start; port <= pa.config.MaxPort; port += step {
		if pa.tryAllocatePort(port, sessionID) {
			pa.nextPort.Store(int32(port + step))
			return port, nil
		}
	}

	// Wrap around
	wrapStart := pa.config.MinPort
	if pa.config.EvenOnly && wrapStart%2 != 0 {
		wrapStart++
	}

	for port := wrapStart; port < start; port += step {
		if pa.tryAllocatePort(port, sessionID) {
			pa.nextPort.Store(int32(port + step))
			return port, nil
		}
	}

	return 0, ErrNoPortsAvailable
}

// tryAllocatePort attempts to allocate a single port
func (pa *PortAllocator) tryAllocatePort(port int, sessionID string) bool {
	if _, ok := pa.released.Load(port); ok {
		return false
	}

	shard := pa.getShard(port)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if _, exists := shard.allocated[port]; exists {
		return false
	}

	if !pa.tryBind(port) {
		return false
	}

	shard.allocated[port] = portInfo{
		port:        port,
		sessionID:   sessionID,
		allocatedAt: time.Now(),
	}

	pa.recordAllocationStats(port, sessionID)
	return true
}

// recordAllocation records a port allocation
func (pa *PortAllocator) recordAllocation(port int, sessionID string, conn net.PacketConn) {
	shard := pa.getShard(port)
	shard.mu.Lock()
	shard.allocated[port] = portInfo{
		port:        port,
		sessionID:   sessionID,
		allocatedAt: time.Now(),
		conn:        conn,
	}
	shard.mu.Unlock()

	pa.recordAllocationStats(port, sessionID)
}

// recordAllocationStats updates stats and session tracking
func (pa *PortAllocator) recordAllocationStats(port int, sessionID string) {
	// Update session tracking
	ss := pa.getSessionShard(sessionID)
	ss.mu.Lock()
	ss.ports[sessionID] = append(ss.ports[sessionID], port)
	ss.mu.Unlock()

	pa.totalAllocated.Add(1)
	current := pa.currentInUse.Add(1)

	// Update peak (lock-free)
	for {
		peak := pa.peakInUse.Load()
		if current <= peak || pa.peakInUse.CompareAndSwap(peak, current) {
			break
		}
	}
}

// AllocateWithConnection allocates a port and returns the bound connection
func (pa *PortAllocator) AllocateWithConnection(sessionID string, network string) (int, net.PacketConn, error) {
	if pa.closed.Load() {
		return 0, nil, errors.New("port allocator is closed")
	}

	ss := pa.getSessionShard(sessionID)
	ss.mu.RLock()
	count := len(ss.ports[sessionID])
	ss.mu.RUnlock()

	if count >= pa.config.MaxAllocations {
		pa.totalFailed.Add(1)
		return 0, nil, ErrPortAllocationLimit
	}

	start := int(pa.nextPort.Load())
	step := 1
	if pa.config.EvenOnly {
		step = 2
	}

	for port := start; port <= pa.config.MaxPort; port += step {
		if _, ok := pa.released.Load(port); ok {
			continue
		}

		shard := pa.getShard(port)
		shard.mu.Lock()

		if _, exists := shard.allocated[port]; exists {
			shard.mu.Unlock()
			continue
		}

		conn, err := net.ListenPacket(network, fmt.Sprintf(":%d", port))
		if err != nil {
			shard.mu.Unlock()
			continue
		}

		shard.allocated[port] = portInfo{
			port:        port,
			sessionID:   sessionID,
			allocatedAt: time.Now(),
			conn:        conn,
		}
		shard.mu.Unlock()

		pa.nextPort.Store(int32(port + step))
		pa.recordAllocationStats(port, sessionID)

		return port, conn, nil
	}

	pa.totalFailed.Add(1)
	return 0, nil, ErrNoPortsAvailable
}

// ReleasePort releases a previously allocated port
func (pa *PortAllocator) ReleasePort(port int) error {
	shard := pa.getShard(port)
	shard.mu.Lock()

	info, exists := shard.allocated[port]
	if !exists {
		shard.mu.Unlock()
		return ErrPortOutOfRange
	}

	if info.conn != nil {
		info.conn.Close()
	}

	delete(shard.allocated, port)
	sessionID := info.sessionID
	shard.mu.Unlock()

	// Mark as released (reuse delay)
	pa.released.Store(port, time.Now())

	// Update session tracking
	if sessionID != "" {
		ss := pa.getSessionShard(sessionID)
		ss.mu.Lock()
		ports := ss.ports[sessionID]
		for i, p := range ports {
			if p == port {
				ss.ports[sessionID] = append(ports[:i], ports[i+1:]...)
				break
			}
		}
		if len(ss.ports[sessionID]) == 0 {
			delete(ss.ports, sessionID)
		}
		ss.mu.Unlock()
	}

	pa.totalReleased.Add(1)
	pa.currentInUse.Add(-1)

	return nil
}

// ReleaseSessionPorts releases all ports for a session
func (pa *PortAllocator) ReleaseSessionPorts(sessionID string) error {
	ss := pa.getSessionShard(sessionID)
	ss.mu.Lock()
	ports := make([]int, len(ss.ports[sessionID]))
	copy(ports, ss.ports[sessionID])
	ss.mu.Unlock()

	var lastErr error
	for _, port := range ports {
		if err := pa.ReleasePort(port); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// GetAvailableCount returns approximate number of available ports
func (pa *PortAllocator) GetAvailableCount() int {
	totalPorts := (pa.config.MaxPort - pa.config.MinPort) / 2
	if !pa.config.EvenOnly {
		totalPorts = pa.config.MaxPort - pa.config.MinPort + 1
	}
	return totalPorts - int(pa.currentInUse.Load())
}

// GetUtilization returns the port pool utilization (0-1)
func (pa *PortAllocator) GetUtilization() float64 {
	totalPorts := (pa.config.MaxPort - pa.config.MinPort) / 2
	if !pa.config.EvenOnly {
		totalPorts = pa.config.MaxPort - pa.config.MinPort + 1
	}
	return float64(pa.currentInUse.Load()) / float64(totalPorts)
}

// IsNearExhaustion checks if the port pool is near exhaustion
func (pa *PortAllocator) IsNearExhaustion(threshold float64) bool {
	return pa.GetUtilization() >= threshold
}

// GetStats returns port allocator statistics
func (pa *PortAllocator) GetStats() map[string]interface{} {
	// Count allocated ports and sessions
	var allocatedCount int
	sessionSet := make(map[string]struct{})

	for i := 0; i < 16; i++ {
		shard := pa.shards[i]
		shard.mu.RLock()
		allocatedCount += len(shard.allocated)
		for _, info := range shard.allocated {
			if info.sessionID != "" {
				sessionSet[info.sessionID] = struct{}{}
			}
		}
		shard.mu.RUnlock()
	}

	return map[string]interface{}{
		"allocated_count":  allocatedCount,
		"session_count":    len(sessionSet),
		"pool_size":        len(pa.pairPool),
		"pool_capacity":    cap(pa.pairPool),
		"pool_hits":        pa.poolHits.Load(),
		"pool_misses":      pa.poolMisses.Load(),
		"total_allocated":  pa.totalAllocated.Load(),
		"total_released":   pa.totalReleased.Load(),
		"total_failed":     pa.totalFailed.Load(),
		"current_in_use":   pa.currentInUse.Load(),
		"peak_in_use":      pa.peakInUse.Load(),
		"utilization":      pa.GetUtilization(),
		"available_count":  pa.GetAvailableCount(),
		"port_range":       fmt.Sprintf("%d-%d", pa.config.MinPort, pa.config.MaxPort),
		"even_only":        pa.config.EvenOnly,
	}
}

// Close closes the port allocator and releases all ports
func (pa *PortAllocator) Close() error {
	if !pa.closed.CompareAndSwap(false, true) {
		return nil
	}

	close(pa.stopCh)
	pa.refillWg.Wait()

	// Drain pool
	for {
		select {
		case <-pa.pairPool:
		default:
			goto drainDone
		}
	}
drainDone:

	// Release all allocated ports
	for i := 0; i < 16; i++ {
		shard := pa.shards[i]
		shard.mu.Lock()
		for port, info := range shard.allocated {
			if info.conn != nil {
				info.conn.Close()
			}
			delete(shard.allocated, port)
		}
		shard.mu.Unlock()
	}

	return nil
}

// PortReservation represents a temporary port reservation
type PortReservation struct {
	allocator *PortAllocator
	ports     []int
	sessionID string
	committed bool
	mu        sync.Mutex
}

// NewPortReservation creates a new reservation for multiple ports
func (pa *PortAllocator) NewPortReservation(sessionID string, count int) (*PortReservation, error) {
	reservation := &PortReservation{
		allocator: pa,
		ports:     make([]int, 0, count),
		sessionID: sessionID,
	}

	for i := 0; i < count; i++ {
		port, err := pa.AllocatePort(sessionID)
		if err != nil {
			reservation.Rollback()
			return nil, err
		}
		reservation.ports = append(reservation.ports, port)
	}

	return reservation, nil
}

// GetPorts returns the reserved ports
func (r *PortReservation) GetPorts() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int{}, r.ports...)
}

// Commit confirms the reservation
func (r *PortReservation) Commit() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.committed = true
}

// Rollback cancels the reservation and releases ports
func (r *PortReservation) Rollback() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.committed {
		return
	}

	for _, port := range r.ports {
		r.allocator.ReleasePort(port)
	}
	r.ports = nil
}

// Global port allocator
var (
	globalPortAllocator     *PortAllocator
	globalPortAllocatorOnce sync.Once
)

// GetPortAllocator returns the global port allocator
func GetPortAllocator() *PortAllocator {
	globalPortAllocatorOnce.Do(func() {
		globalPortAllocator = NewPortAllocator(DefaultPortAllocatorConfig())
	})
	return globalPortAllocator
}

// SetGlobalPortAllocator sets the global port allocator (for testing)
func SetGlobalPortAllocator(pa *PortAllocator) {
	globalPortAllocator = pa
}
