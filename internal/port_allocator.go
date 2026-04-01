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
	ReserveCount   int           // Number of ports to pre-allocate
	ReuseDelay     time.Duration // Time before a released port can be reused
	MaxAllocations int           // Maximum simultaneous allocations per session
	EvenOnly       bool          // Only allocate even ports (for RTP)
}

// DefaultPortAllocatorConfig returns sensible defaults
func DefaultPortAllocatorConfig() *PortAllocatorConfig {
	return &PortAllocatorConfig{
		MinPort:        10000,
		MaxPort:        60000,
		ReserveCount:   100,
		ReuseDelay:     5 * time.Second,
		MaxAllocations: 100,
		EvenOnly:       true, // RTP ports are typically even
	}
}

// PortAllocator manages port allocation with exhaustion protection
type PortAllocator struct {
	config *PortAllocatorConfig

	// Port tracking
	allocated   map[int]portInfo
	released    map[int]time.Time
	allocatedMu sync.RWMutex

	// Pre-allocated port pool
	portPool   []int
	poolMu     sync.Mutex

	// Metrics
	totalAllocated atomic.Int64
	totalReleased  atomic.Int64
	totalFailed    atomic.Int64
	currentInUse   atomic.Int64
	peakInUse      atomic.Int64

	// Per-session tracking
	sessionPorts   map[string][]int
	sessionPortsMu sync.RWMutex

	// State
	closed atomic.Bool
}

type portInfo struct {
	port        int
	sessionID   string
	allocatedAt time.Time
	conn        net.PacketConn
}

// NewPortAllocator creates a new port allocator
func NewPortAllocator(config *PortAllocatorConfig) *PortAllocator {
	if config == nil {
		config = DefaultPortAllocatorConfig()
	}

	pa := &PortAllocator{
		config:       config,
		allocated:    make(map[int]portInfo),
		released:     make(map[int]time.Time),
		portPool:     make([]int, 0, config.ReserveCount),
		sessionPorts: make(map[string][]int),
	}

	// Pre-allocate ports if configured
	if config.ReserveCount > 0 {
		go pa.preAllocatePorts()
	}

	return pa
}

// preAllocatePorts pre-allocates ports for faster allocation
func (pa *PortAllocator) preAllocatePorts() {
	pa.poolMu.Lock()
	defer pa.poolMu.Unlock()

	for i := 0; i < pa.config.ReserveCount && !pa.closed.Load(); i++ {
		port, err := pa.findAvailablePort()
		if err != nil {
			break
		}
		pa.portPool = append(pa.portPool, port)
	}
}

// AllocatePort allocates a single port
func (pa *PortAllocator) AllocatePort(sessionID string) (int, error) {
	if pa.closed.Load() {
		return 0, errors.New("port allocator is closed")
	}

	// Check session allocation limit
	pa.sessionPortsMu.RLock()
	sessionCount := len(pa.sessionPorts[sessionID])
	pa.sessionPortsMu.RUnlock()

	if sessionCount >= pa.config.MaxAllocations {
		pa.totalFailed.Add(1)
		return 0, ErrPortAllocationLimit
	}

	// Try to get from pre-allocated pool first
	port, err := pa.getFromPool()
	if err == nil {
		pa.recordAllocation(port, sessionID, nil)
		return port, nil
	}

	// Find and allocate a new port
	port, err = pa.findAndAllocatePort(sessionID)
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

	// Check session allocation limit (need 2 ports)
	pa.sessionPortsMu.RLock()
	sessionCount := len(pa.sessionPorts[sessionID])
	pa.sessionPortsMu.RUnlock()

	if sessionCount+2 > pa.config.MaxAllocations {
		pa.totalFailed.Add(1)
		return 0, 0, ErrPortAllocationLimit
	}

	pa.allocatedMu.Lock()
	defer pa.allocatedMu.Unlock()

	// Find consecutive even/odd port pair
	start := pa.config.MinPort
	if pa.config.EvenOnly && start%2 != 0 {
		start++
	}

	for port := start; port < pa.config.MaxPort-1; port += 2 {
		if pa.isPortAvailable(port) && pa.isPortAvailable(port+1) {
			// Try to bind both ports
			if pa.tryBindPort(port) && pa.tryBindPort(port+1) {
				pa.recordAllocationLocked(port, sessionID, nil)
				pa.recordAllocationLocked(port+1, sessionID, nil)
				return port, port + 1, nil
			}
		}
	}

	pa.totalFailed.Add(2)
	return 0, 0, ErrNoPortsAvailable
}

// AllocateWithConnection allocates a port and returns the bound connection
func (pa *PortAllocator) AllocateWithConnection(sessionID string, network string) (int, net.PacketConn, error) {
	if pa.closed.Load() {
		return 0, nil, errors.New("port allocator is closed")
	}

	// Check session allocation limit
	pa.sessionPortsMu.RLock()
	sessionCount := len(pa.sessionPorts[sessionID])
	pa.sessionPortsMu.RUnlock()

	if sessionCount >= pa.config.MaxAllocations {
		pa.totalFailed.Add(1)
		return 0, nil, ErrPortAllocationLimit
	}

	pa.allocatedMu.Lock()
	defer pa.allocatedMu.Unlock()

	start := pa.config.MinPort
	if pa.config.EvenOnly && start%2 != 0 {
		start++
	}

	step := 1
	if pa.config.EvenOnly {
		step = 2
	}

	for port := start; port <= pa.config.MaxPort; port += step {
		if !pa.isPortAvailable(port) {
			continue
		}

		// Try to bind
		addr := fmt.Sprintf(":%d", port)
		conn, err := net.ListenPacket(network, addr)
		if err != nil {
			continue
		}

		pa.recordAllocationLocked(port, sessionID, conn)
		return port, conn, nil
	}

	pa.totalFailed.Add(1)
	return 0, nil, ErrNoPortsAvailable
}

// ReleasePort releases a previously allocated port
func (pa *PortAllocator) ReleasePort(port int) error {
	pa.allocatedMu.Lock()
	defer pa.allocatedMu.Unlock()

	info, exists := pa.allocated[port]
	if !exists {
		return ErrPortOutOfRange
	}

	// Close connection if we hold it
	if info.conn != nil {
		info.conn.Close()
	}

	// Remove from allocated
	delete(pa.allocated, port)

	// Track release time for reuse delay
	pa.released[port] = time.Now()

	// Update session tracking
	pa.sessionPortsMu.Lock()
	if ports, exists := pa.sessionPorts[info.sessionID]; exists {
		for i, p := range ports {
			if p == port {
				pa.sessionPorts[info.sessionID] = append(ports[:i], ports[i+1:]...)
				break
			}
		}
		if len(pa.sessionPorts[info.sessionID]) == 0 {
			delete(pa.sessionPorts, info.sessionID)
		}
	}
	pa.sessionPortsMu.Unlock()

	pa.totalReleased.Add(1)
	pa.currentInUse.Add(-1)

	return nil
}

// ReleaseSessionPorts releases all ports for a session
func (pa *PortAllocator) ReleaseSessionPorts(sessionID string) error {
	pa.sessionPortsMu.RLock()
	ports := make([]int, len(pa.sessionPorts[sessionID]))
	copy(ports, pa.sessionPorts[sessionID])
	pa.sessionPortsMu.RUnlock()

	var lastErr error
	for _, port := range ports {
		if err := pa.ReleasePort(port); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// getFromPool tries to get a port from the pre-allocated pool
func (pa *PortAllocator) getFromPool() (int, error) {
	pa.poolMu.Lock()
	defer pa.poolMu.Unlock()

	if len(pa.portPool) == 0 {
		return 0, ErrPortPoolExhausted
	}

	port := pa.portPool[len(pa.portPool)-1]
	pa.portPool = pa.portPool[:len(pa.portPool)-1]

	// Verify it's still available
	pa.allocatedMu.RLock()
	available := pa.isPortAvailable(port)
	pa.allocatedMu.RUnlock()

	if !available {
		return 0, ErrPortInUse
	}

	return port, nil
}

// findAvailablePort finds an available port
func (pa *PortAllocator) findAvailablePort() (int, error) {
	pa.allocatedMu.RLock()
	defer pa.allocatedMu.RUnlock()

	start := pa.config.MinPort
	if pa.config.EvenOnly && start%2 != 0 {
		start++
	}

	step := 1
	if pa.config.EvenOnly {
		step = 2
	}

	for port := start; port <= pa.config.MaxPort; port += step {
		if pa.isPortAvailable(port) {
			return port, nil
		}
	}

	return 0, ErrNoPortsAvailable
}

// findAndAllocatePort finds and allocates a port
func (pa *PortAllocator) findAndAllocatePort(sessionID string) (int, error) {
	pa.allocatedMu.Lock()
	defer pa.allocatedMu.Unlock()

	start := pa.config.MinPort
	if pa.config.EvenOnly && start%2 != 0 {
		start++
	}

	step := 1
	if pa.config.EvenOnly {
		step = 2
	}

	for port := start; port <= pa.config.MaxPort; port += step {
		if pa.isPortAvailable(port) {
			// Try to bind
			if pa.tryBindPort(port) {
				pa.recordAllocationLocked(port, sessionID, nil)
				return port, nil
			}
		}
	}

	return 0, ErrNoPortsAvailable
}

// isPortAvailable checks if a port is available for allocation
func (pa *PortAllocator) isPortAvailable(port int) bool {
	// Check if already allocated
	if _, exists := pa.allocated[port]; exists {
		return false
	}

	// Check reuse delay
	if releaseTime, exists := pa.released[port]; exists {
		if time.Since(releaseTime) < pa.config.ReuseDelay {
			return false
		}
		// Clean up old release record
		delete(pa.released, port)
	}

	return true
}

// tryBindPort attempts to bind to a port to verify it's available
func (pa *PortAllocator) tryBindPort(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// recordAllocation records a port allocation
func (pa *PortAllocator) recordAllocation(port int, sessionID string, conn net.PacketConn) {
	pa.allocatedMu.Lock()
	pa.recordAllocationLocked(port, sessionID, conn)
	pa.allocatedMu.Unlock()
}

// recordAllocationLocked records a port allocation (must hold allocatedMu)
func (pa *PortAllocator) recordAllocationLocked(port int, sessionID string, conn net.PacketConn) {
	pa.allocated[port] = portInfo{
		port:        port,
		sessionID:   sessionID,
		allocatedAt: time.Now(),
		conn:        conn,
	}

	pa.sessionPortsMu.Lock()
	pa.sessionPorts[sessionID] = append(pa.sessionPorts[sessionID], port)
	pa.sessionPortsMu.Unlock()

	pa.totalAllocated.Add(1)
	current := pa.currentInUse.Add(1)

	// Update peak
	for {
		peak := pa.peakInUse.Load()
		if current <= peak {
			break
		}
		if pa.peakInUse.CompareAndSwap(peak, current) {
			break
		}
	}
}

// GetAvailableCount returns the number of available ports
func (pa *PortAllocator) GetAvailableCount() int {
	pa.allocatedMu.RLock()
	defer pa.allocatedMu.RUnlock()

	totalPorts := (pa.config.MaxPort - pa.config.MinPort) / 2
	if !pa.config.EvenOnly {
		totalPorts = pa.config.MaxPort - pa.config.MinPort + 1
	}

	return totalPorts - len(pa.allocated)
}

// GetUtilization returns the port pool utilization (0-1)
func (pa *PortAllocator) GetUtilization() float64 {
	pa.allocatedMu.RLock()
	defer pa.allocatedMu.RUnlock()

	totalPorts := (pa.config.MaxPort - pa.config.MinPort) / 2
	if !pa.config.EvenOnly {
		totalPorts = pa.config.MaxPort - pa.config.MinPort + 1
	}

	return float64(len(pa.allocated)) / float64(totalPorts)
}

// IsNearExhaustion checks if the port pool is near exhaustion
func (pa *PortAllocator) IsNearExhaustion(threshold float64) bool {
	return pa.GetUtilization() >= threshold
}

// GetStats returns port allocator statistics
func (pa *PortAllocator) GetStats() map[string]interface{} {
	pa.allocatedMu.RLock()
	allocatedCount := len(pa.allocated)
	pa.allocatedMu.RUnlock()

	pa.sessionPortsMu.RLock()
	sessionCount := len(pa.sessionPorts)
	pa.sessionPortsMu.RUnlock()

	pa.poolMu.Lock()
	poolSize := len(pa.portPool)
	pa.poolMu.Unlock()

	return map[string]interface{}{
		"allocated_count":  allocatedCount,
		"session_count":    sessionCount,
		"pool_size":        poolSize,
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

	pa.allocatedMu.Lock()
	defer pa.allocatedMu.Unlock()

	for port, info := range pa.allocated {
		if info.conn != nil {
			info.conn.Close()
		}
		delete(pa.allocated, port)
	}

	pa.poolMu.Lock()
	pa.portPool = nil
	pa.poolMu.Unlock()

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

	// Allocate requested ports
	for i := 0; i < count; i++ {
		port, err := pa.AllocatePort(sessionID)
		if err != nil {
			// Rollback on failure
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
