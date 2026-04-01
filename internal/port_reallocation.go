package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// PortReallocationConfig configures port reallocation behavior
type PortReallocationConfig struct {
	// RedisPrefix for port allocation keys
	RedisPrefix string
	// PortRangeLow is the low end of the port range
	PortRangeLow int
	// PortRangeHigh is the high end of the port range
	PortRangeHigh int
	// AllocationTTL is how long a port allocation is valid
	AllocationTTL time.Duration
	// RenewalInterval is how often to renew allocations
	RenewalInterval time.Duration
	// MaxRetries for port allocation
	MaxRetries int
	// PreferConsistent tries to reuse same ports
	PreferConsistent bool
	// PortsPerSession is number of ports per media session
	PortsPerSession int
}

// DefaultPortReallocationConfig returns default configuration
func DefaultPortReallocationConfig() *PortReallocationConfig {
	return &PortReallocationConfig{
		RedisPrefix:      "ports:",
		PortRangeLow:     30000,
		PortRangeHigh:    60000,
		AllocationTTL:    5 * time.Minute,
		RenewalInterval:  1 * time.Minute,
		MaxRetries:       10,
		PreferConsistent: true,
		PortsPerSession:  4, // RTP + RTCP for caller and callee
	}
}

// PortReallocationManager manages port allocation with failover support
type PortReallocationManager struct {
	config   *PortReallocationConfig
	cluster  *RedisSessionStore
	nodeID   string

	mu           sync.RWMutex
	allocations  map[string]*PortAllocation
	sessionPorts map[string][]int // session ID -> allocated ports
	portOwners   map[int]string   // port -> session ID

	stopChan chan struct{}
	doneChan chan struct{}
}

// PortAllocation represents a port allocation
type PortAllocation struct {
	SessionID    string    `json:"session_id"`
	CallID       string    `json:"call_id"`
	NodeID       string    `json:"node_id"`
	Ports        []int     `json:"ports"`
	AllocatedAt  time.Time `json:"allocated_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	LastRenewed  time.Time `json:"last_renewed"`
	IsFailover   bool      `json:"is_failover"`
	OriginalNode string    `json:"original_node,omitempty"`
}

// NewPortReallocationManager creates a new port reallocation manager
func NewPortReallocationManager(nodeID string, cluster *RedisSessionStore, config *PortReallocationConfig) *PortReallocationManager {
	if config == nil {
		config = DefaultPortReallocationConfig()
	}

	return &PortReallocationManager{
		config:       config,
		cluster:      cluster,
		nodeID:       nodeID,
		allocations:  make(map[string]*PortAllocation),
		sessionPorts: make(map[string][]int),
		portOwners:   make(map[int]string),
		stopChan:     make(chan struct{}),
		doneChan:     make(chan struct{}),
	}
}

// Start starts the port reallocation manager
func (prm *PortReallocationManager) Start() {
	go prm.renewalLoop()
	go prm.cleanupLoop()
}

// Stop stops the port reallocation manager
func (prm *PortReallocationManager) Stop() {
	close(prm.stopChan)
	<-prm.doneChan
}

// AllocatePorts allocates ports for a session
func (prm *PortReallocationManager) AllocatePorts(sessionID, callID string, count int) ([]int, error) {
	prm.mu.Lock()
	defer prm.mu.Unlock()

	// Check if session already has ports
	if existing, exists := prm.sessionPorts[sessionID]; exists {
		return existing, nil
	}

	// Try to get consistent ports if this is a failover
	if prm.config.PreferConsistent && prm.cluster != nil {
		ports, err := prm.tryGetConsistentPorts(sessionID, callID, count)
		if err == nil && len(ports) == count {
			prm.recordAllocation(sessionID, callID, ports, false)
			return ports, nil
		}
	}

	// Allocate new ports
	ports := make([]int, 0, count)
	for i := 0; i < count; i++ {
		port, err := prm.allocatePort(sessionID)
		if err != nil {
			// Rollback allocated ports
			for _, p := range ports {
				prm.releasePortInternal(p)
			}
			return nil, err
		}
		ports = append(ports, port)
	}

	prm.recordAllocation(sessionID, callID, ports, false)

	// Store in Redis for cluster coordination
	if prm.cluster != nil {
		prm.storeAllocation(sessionID, callID, ports)
	}

	return ports, nil
}

// AllocateConsistentPorts allocates ports that will be consistent across failovers
func (prm *PortReallocationManager) AllocateConsistentPorts(sessionID, callID string, preferredPorts []int) ([]int, error) {
	prm.mu.Lock()
	defer prm.mu.Unlock()

	// Try to use preferred ports first
	if len(preferredPorts) > 0 {
		available := make([]int, 0, len(preferredPorts))
		for _, port := range preferredPorts {
			if prm.isPortAvailable(port) {
				available = append(available, port)
			}
		}

		if len(available) == len(preferredPorts) {
			// All preferred ports available
			for _, port := range available {
				prm.portOwners[port] = sessionID
			}
			prm.sessionPorts[sessionID] = available
			prm.recordAllocation(sessionID, callID, available, true)

			if prm.cluster != nil {
				prm.storeAllocation(sessionID, callID, available)
			}

			return available, nil
		}
	}

	// Fall back to regular allocation
	return prm.AllocatePorts(sessionID, callID, len(preferredPorts))
}

// ReleasePorts releases ports for a session
func (prm *PortReallocationManager) ReleasePorts(sessionID string) {
	prm.mu.Lock()
	defer prm.mu.Unlock()

	ports, exists := prm.sessionPorts[sessionID]
	if !exists {
		return
	}

	for _, port := range ports {
		delete(prm.portOwners, port)
	}
	delete(prm.sessionPorts, sessionID)
	delete(prm.allocations, sessionID)

	// Remove from Redis
	if prm.cluster != nil {
		prm.removeAllocation(sessionID)
	}
}

// TakeoverPorts takes over ports from a failed node
func (prm *PortReallocationManager) TakeoverPorts(sessionID string, originalNodeID string) ([]int, error) {
	if prm.cluster == nil {
		return nil, fmt.Errorf("cluster not configured")
	}

	// Get original allocation from Redis
	allocation, err := prm.getRemoteAllocation(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get original allocation: %w", err)
	}

	if allocation == nil {
		return nil, fmt.Errorf("no allocation found for session %s", sessionID)
	}

	prm.mu.Lock()
	defer prm.mu.Unlock()

	// Try to allocate the same ports
	availablePorts := make([]int, 0, len(allocation.Ports))
	for _, port := range allocation.Ports {
		if prm.isPortAvailable(port) {
			availablePorts = append(availablePorts, port)
		}
	}

	var finalPorts []int
	if len(availablePorts) == len(allocation.Ports) {
		// All original ports available
		finalPorts = availablePorts
	} else {
		// Allocate new ports
		finalPorts = make([]int, 0, len(allocation.Ports))
		for i := 0; i < len(allocation.Ports); i++ {
			port, err := prm.allocatePort(sessionID)
			if err != nil {
				// Rollback
				for _, p := range finalPorts {
					prm.releasePortInternal(p)
				}
				return nil, err
			}
			finalPorts = append(finalPorts, port)
		}
	}

	// Record takeover
	for _, port := range finalPorts {
		prm.portOwners[port] = sessionID
	}
	prm.sessionPorts[sessionID] = finalPorts

	// Update allocation
	newAlloc := &PortAllocation{
		SessionID:    sessionID,
		CallID:       allocation.CallID,
		NodeID:       prm.nodeID,
		Ports:        finalPorts,
		AllocatedAt:  time.Now(),
		ExpiresAt:    time.Now().Add(prm.config.AllocationTTL),
		LastRenewed:  time.Now(),
		IsFailover:   true,
		OriginalNode: originalNodeID,
	}
	prm.allocations[sessionID] = newAlloc

	// Update Redis
	prm.storeAllocation(sessionID, allocation.CallID, finalPorts)

	return finalPorts, nil
}

// GetSessionPorts returns ports for a session
func (prm *PortReallocationManager) GetSessionPorts(sessionID string) []int {
	prm.mu.RLock()
	defer prm.mu.RUnlock()
	return prm.sessionPorts[sessionID]
}

func (prm *PortReallocationManager) allocatePort(sessionID string) (int, error) {
	for retry := 0; retry < prm.config.MaxRetries; retry++ {
		// Use hash-based port selection for better distribution
		basePort := prm.config.PortRangeLow
		portRange := prm.config.PortRangeHigh - prm.config.PortRangeLow

		for i := 0; i < portRange; i++ {
			port := basePort + ((hashSessionPort(sessionID, retry) + i) % portRange)
			if prm.isPortAvailable(port) {
				prm.portOwners[port] = sessionID
				return port, nil
			}
		}
	}

	return 0, fmt.Errorf("no available ports in range %d-%d", prm.config.PortRangeLow, prm.config.PortRangeHigh)
}

func (prm *PortReallocationManager) isPortAvailable(port int) bool {
	if port < prm.config.PortRangeLow || port > prm.config.PortRangeHigh {
		return false
	}
	_, occupied := prm.portOwners[port]
	return !occupied
}

func (prm *PortReallocationManager) releasePortInternal(port int) {
	delete(prm.portOwners, port)
}

func (prm *PortReallocationManager) recordAllocation(sessionID, callID string, ports []int, isFailover bool) {
	prm.sessionPorts[sessionID] = ports
	prm.allocations[sessionID] = &PortAllocation{
		SessionID:   sessionID,
		CallID:      callID,
		NodeID:      prm.nodeID,
		Ports:       ports,
		AllocatedAt: time.Now(),
		ExpiresAt:   time.Now().Add(prm.config.AllocationTTL),
		LastRenewed: time.Now(),
		IsFailover:  isFailover,
	}
}

func (prm *PortReallocationManager) tryGetConsistentPorts(sessionID, callID string, count int) ([]int, error) {
	// Generate deterministic ports based on session ID
	ports := make([]int, 0, count)
	baseHash := hashSessionPort(sessionID, 0)
	portRange := prm.config.PortRangeHigh - prm.config.PortRangeLow

	for i := 0; i < count; i++ {
		port := prm.config.PortRangeLow + ((baseHash + i*2) % portRange)
		if prm.isPortAvailable(port) {
			ports = append(ports, port)
			prm.portOwners[port] = sessionID
		}
	}

	if len(ports) == count {
		return ports, nil
	}

	// Rollback partial allocation
	for _, p := range ports {
		delete(prm.portOwners, p)
	}

	return nil, fmt.Errorf("could not allocate consistent ports")
}

func (prm *PortReallocationManager) storeAllocation(sessionID, callID string, ports []int) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allocation := &PortAllocation{
		SessionID:   sessionID,
		CallID:      callID,
		NodeID:      prm.nodeID,
		Ports:       ports,
		AllocatedAt: time.Now(),
		ExpiresAt:   time.Now().Add(prm.config.AllocationTTL),
		LastRenewed: time.Now(),
	}

	data, err := json.Marshal(allocation)
	if err != nil {
		return
	}

	key := fmt.Sprintf("%s%s", prm.config.RedisPrefix, sessionID)
	prm.cluster.client.Set(ctx, key, string(data), prm.config.AllocationTTL)
}

func (prm *PortReallocationManager) removeAllocation(sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := fmt.Sprintf("%s%s", prm.config.RedisPrefix, sessionID)
	prm.cluster.client.Del(ctx, key)
}

func (prm *PortReallocationManager) getRemoteAllocation(sessionID string) (*PortAllocation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := fmt.Sprintf("%s%s", prm.config.RedisPrefix, sessionID)
	dataStr, err := prm.cluster.client.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var allocation PortAllocation
	if err := json.Unmarshal([]byte(dataStr), &allocation); err != nil {
		return nil, err
	}

	return &allocation, nil
}

func (prm *PortReallocationManager) renewalLoop() {
	ticker := time.NewTicker(prm.config.RenewalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-prm.stopChan:
			return
		case <-ticker.C:
			prm.renewAllocations()
		}
	}
}

func (prm *PortReallocationManager) renewAllocations() {
	if prm.cluster == nil {
		return
	}

	prm.mu.Lock()
	defer prm.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for sessionID, allocation := range prm.allocations {
		allocation.LastRenewed = time.Now()
		allocation.ExpiresAt = time.Now().Add(prm.config.AllocationTTL)

		data, err := json.Marshal(allocation)
		if err != nil {
			continue
		}

		key := fmt.Sprintf("%s%s", prm.config.RedisPrefix, sessionID)
		prm.cluster.client.Set(ctx, key, string(data), prm.config.AllocationTTL)
	}
}

func (prm *PortReallocationManager) cleanupLoop() {
	defer close(prm.doneChan)

	ticker := time.NewTicker(prm.config.AllocationTTL / 2)
	defer ticker.Stop()

	for {
		select {
		case <-prm.stopChan:
			return
		case <-ticker.C:
			prm.cleanupExpired()
		}
	}
}

func (prm *PortReallocationManager) cleanupExpired() {
	prm.mu.Lock()
	defer prm.mu.Unlock()

	now := time.Now()
	for sessionID, allocation := range prm.allocations {
		if now.After(allocation.ExpiresAt) {
			for _, port := range allocation.Ports {
				delete(prm.portOwners, port)
			}
			delete(prm.sessionPorts, sessionID)
			delete(prm.allocations, sessionID)
		}
	}
}

// GetStats returns port allocation statistics
func (prm *PortReallocationManager) GetStats() *PortAllocationStats {
	prm.mu.RLock()
	defer prm.mu.RUnlock()

	failoverCount := 0
	for _, alloc := range prm.allocations {
		if alloc.IsFailover {
			failoverCount++
		}
	}

	return &PortAllocationStats{
		TotalSessions:   len(prm.sessionPorts),
		TotalPorts:      len(prm.portOwners),
		FailoverCount:   failoverCount,
		AvailablePorts:  prm.config.PortRangeHigh - prm.config.PortRangeLow - len(prm.portOwners),
		PortRangeLow:    prm.config.PortRangeLow,
		PortRangeHigh:   prm.config.PortRangeHigh,
	}
}

// PortAllocationStats contains port allocation statistics
type PortAllocationStats struct {
	TotalSessions  int
	TotalPorts     int
	FailoverCount  int
	AvailablePorts int
	PortRangeLow   int
	PortRangeHigh  int
}

func hashSessionPort(sessionID string, salt int) int {
	h := 0
	for _, c := range sessionID {
		h = h*31 + int(c)
	}
	return (h + salt*17) & 0x7FFFFFFF
}
