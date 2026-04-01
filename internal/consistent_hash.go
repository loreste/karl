package internal

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"
	"sync"
)

// ConsistentHashConfig configures the consistent hash ring
type ConsistentHashConfig struct {
	// ReplicationFactor is the number of virtual nodes per physical node
	ReplicationFactor int
	// LoadFactor is the max load factor before rebalancing (1.0 = 100%)
	LoadFactor float64
}

// DefaultConsistentHashConfig returns default configuration
func DefaultConsistentHashConfig() *ConsistentHashConfig {
	return &ConsistentHashConfig{
		ReplicationFactor: 150,
		LoadFactor:        1.25,
	}
}

// HashRing implements consistent hashing for session distribution
type HashRing struct {
	config *ConsistentHashConfig

	mu           sync.RWMutex
	nodes        map[string]*HashNode
	ring         []uint32
	ringNodes    map[uint32]string // hash -> node ID
	nodeLoad     map[string]int64  // node ID -> current load
	totalLoad    int64
}

// HashNode represents a node in the hash ring
type HashNode struct {
	ID       string
	Address  string
	Weight   int
	Healthy  bool
	Metadata map[string]string
}

// NewHashRing creates a new consistent hash ring
func NewHashRing(config *ConsistentHashConfig) *HashRing {
	if config == nil {
		config = DefaultConsistentHashConfig()
	}

	return &HashRing{
		config:    config,
		nodes:     make(map[string]*HashNode),
		ring:      make([]uint32, 0),
		ringNodes: make(map[uint32]string),
		nodeLoad:  make(map[string]int64),
	}
}

// AddNode adds a node to the hash ring
func (hr *HashRing) AddNode(node *HashNode) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if _, exists := hr.nodes[node.ID]; exists {
		return
	}

	hr.nodes[node.ID] = node
	hr.nodeLoad[node.ID] = 0

	// Add virtual nodes
	replicas := hr.config.ReplicationFactor
	if node.Weight > 0 {
		replicas = replicas * node.Weight
	}

	for i := 0; i < replicas; i++ {
		hash := hr.hashKey(node.ID, i)
		hr.ring = append(hr.ring, hash)
		hr.ringNodes[hash] = node.ID
	}

	// Sort ring
	sort.Slice(hr.ring, func(i, j int) bool {
		return hr.ring[i] < hr.ring[j]
	})
}

// RemoveNode removes a node from the hash ring
func (hr *HashRing) RemoveNode(nodeID string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	node, exists := hr.nodes[nodeID]
	if !exists {
		return
	}

	// Remove virtual nodes
	replicas := hr.config.ReplicationFactor
	if node.Weight > 0 {
		replicas = replicas * node.Weight
	}

	hashesToRemove := make(map[uint32]bool)
	for i := 0; i < replicas; i++ {
		hash := hr.hashKey(nodeID, i)
		hashesToRemove[hash] = true
		delete(hr.ringNodes, hash)
	}

	// Rebuild ring without removed hashes
	newRing := make([]uint32, 0, len(hr.ring)-len(hashesToRemove))
	for _, h := range hr.ring {
		if !hashesToRemove[h] {
			newRing = append(newRing, h)
		}
	}
	hr.ring = newRing

	hr.totalLoad -= hr.nodeLoad[nodeID]
	delete(hr.nodes, nodeID)
	delete(hr.nodeLoad, nodeID)
}

// GetNode returns the node responsible for a key
func (hr *HashRing) GetNode(key string) *HashNode {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	if len(hr.ring) == 0 {
		return nil
	}

	hash := hr.hashString(key)
	idx := hr.search(hash)
	nodeID := hr.ringNodes[hr.ring[idx]]

	return hr.nodes[nodeID]
}

// GetNodeWithLoad returns the node for a key, considering load balancing
func (hr *HashRing) GetNodeWithLoad(key string) *HashNode {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	if len(hr.ring) == 0 {
		return nil
	}

	hash := hr.hashString(key)
	idx := hr.search(hash)

	// Find first healthy node that's not overloaded
	avgLoad := float64(hr.totalLoad) / float64(len(hr.nodes))
	maxLoad := int64(avgLoad * hr.config.LoadFactor)
	if maxLoad < 1 {
		maxLoad = 1
	}

	// Try up to len(ring) times to find a suitable node
	for i := 0; i < len(hr.ring); i++ {
		nodeID := hr.ringNodes[hr.ring[(idx+i)%len(hr.ring)]]
		node := hr.nodes[nodeID]

		if node.Healthy && hr.nodeLoad[nodeID] < maxLoad {
			return node
		}
	}

	// Fallback to any healthy node
	for _, node := range hr.nodes {
		if node.Healthy {
			return node
		}
	}

	// Fallback to primary
	return hr.nodes[hr.ringNodes[hr.ring[idx]]]
}

// GetNodes returns multiple nodes for replication
func (hr *HashRing) GetNodes(key string, count int) []*HashNode {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	if len(hr.ring) == 0 {
		return nil
	}

	hash := hr.hashString(key)
	idx := hr.search(hash)

	result := make([]*HashNode, 0, count)
	seen := make(map[string]bool)

	for i := 0; i < len(hr.ring) && len(result) < count; i++ {
		nodeID := hr.ringNodes[hr.ring[(idx+i)%len(hr.ring)]]
		if !seen[nodeID] {
			seen[nodeID] = true
			result = append(result, hr.nodes[nodeID])
		}
	}

	return result
}

// IncrementLoad increments the load for a node
func (hr *HashRing) IncrementLoad(nodeID string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if _, exists := hr.nodeLoad[nodeID]; exists {
		hr.nodeLoad[nodeID]++
		hr.totalLoad++
	}
}

// DecrementLoad decrements the load for a node
func (hr *HashRing) DecrementLoad(nodeID string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if load, exists := hr.nodeLoad[nodeID]; exists && load > 0 {
		hr.nodeLoad[nodeID]--
		hr.totalLoad--
	}
}

// SetNodeHealth sets the health status of a node
func (hr *HashRing) SetNodeHealth(nodeID string, healthy bool) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if node, exists := hr.nodes[nodeID]; exists {
		node.Healthy = healthy
	}
}

// GetNodeLoad returns the current load for a node
func (hr *HashRing) GetNodeLoad(nodeID string) int64 {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	return hr.nodeLoad[nodeID]
}

// GetAllNodes returns all nodes in the ring
func (hr *HashRing) GetAllNodes() []*HashNode {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	nodes := make([]*HashNode, 0, len(hr.nodes))
	for _, node := range hr.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// GetHealthyNodes returns all healthy nodes
func (hr *HashRing) GetHealthyNodes() []*HashNode {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	nodes := make([]*HashNode, 0)
	for _, node := range hr.nodes {
		if node.Healthy {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// NodeCount returns the number of nodes in the ring
func (hr *HashRing) NodeCount() int {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	return len(hr.nodes)
}

// Stats returns statistics about the hash ring
func (hr *HashRing) Stats() *HashRingStats {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	stats := &HashRingStats{
		NodeCount:      len(hr.nodes),
		VirtualNodes:   len(hr.ring),
		TotalLoad:      hr.totalLoad,
		NodeLoads:      make(map[string]int64),
		HealthyCount:   0,
		UnhealthyCount: 0,
	}

	for id, load := range hr.nodeLoad {
		stats.NodeLoads[id] = load
	}

	for _, node := range hr.nodes {
		if node.Healthy {
			stats.HealthyCount++
		} else {
			stats.UnhealthyCount++
		}
	}

	if stats.NodeCount > 0 {
		stats.AvgLoad = float64(hr.totalLoad) / float64(stats.NodeCount)
	}

	return stats
}

// HashRingStats holds ring statistics
type HashRingStats struct {
	NodeCount      int
	VirtualNodes   int
	TotalLoad      int64
	AvgLoad        float64
	NodeLoads      map[string]int64
	HealthyCount   int
	UnhealthyCount int
}

func (hr *HashRing) hashKey(nodeID string, replica int) uint32 {
	data := make([]byte, len(nodeID)+4)
	copy(data, nodeID)
	binary.BigEndian.PutUint32(data[len(nodeID):], uint32(replica))

	hash := sha256.Sum256(data)
	return binary.BigEndian.Uint32(hash[:4])
}

func (hr *HashRing) hashString(key string) uint32 {
	hash := sha256.Sum256([]byte(key))
	return binary.BigEndian.Uint32(hash[:4])
}

func (hr *HashRing) search(hash uint32) int {
	idx := sort.Search(len(hr.ring), func(i int) bool {
		return hr.ring[i] >= hash
	})
	if idx >= len(hr.ring) {
		idx = 0
	}
	return idx
}

// SessionRouter uses consistent hashing for session routing
type SessionRouter struct {
	ring   *HashRing
	mu     sync.RWMutex
	sticky map[string]string // session ID -> node ID (for sticky sessions)
}

// NewSessionRouter creates a new session router
func NewSessionRouter(config *ConsistentHashConfig) *SessionRouter {
	return &SessionRouter{
		ring:   NewHashRing(config),
		sticky: make(map[string]string),
	}
}

// AddNode adds a node to the router
func (sr *SessionRouter) AddNode(node *HashNode) {
	sr.ring.AddNode(node)
}

// RemoveNode removes a node from the router
func (sr *SessionRouter) RemoveNode(nodeID string) {
	sr.ring.RemoveNode(nodeID)

	// Clear sticky sessions for removed node
	sr.mu.Lock()
	for sessionID, stickyNodeID := range sr.sticky {
		if stickyNodeID == nodeID {
			delete(sr.sticky, sessionID)
		}
	}
	sr.mu.Unlock()
}

// RouteSession returns the node for a session
func (sr *SessionRouter) RouteSession(sessionID string) *HashNode {
	// Check for sticky session
	sr.mu.RLock()
	if stickyNodeID, exists := sr.sticky[sessionID]; exists {
		sr.mu.RUnlock()
		// Verify node still exists and is healthy
		nodes := sr.ring.GetAllNodes()
		for _, node := range nodes {
			if node.ID == stickyNodeID && node.Healthy {
				return node
			}
		}
		// Node gone or unhealthy, fall through to new routing
	} else {
		sr.mu.RUnlock()
	}

	// Route via consistent hash with load balancing
	node := sr.ring.GetNodeWithLoad(sessionID)
	if node != nil {
		sr.mu.Lock()
		sr.sticky[sessionID] = node.ID
		sr.mu.Unlock()
		sr.ring.IncrementLoad(node.ID)
	}

	return node
}

// EndSession removes a session from routing
func (sr *SessionRouter) EndSession(sessionID string) {
	sr.mu.Lock()
	nodeID, exists := sr.sticky[sessionID]
	delete(sr.sticky, sessionID)
	sr.mu.Unlock()

	if exists {
		sr.ring.DecrementLoad(nodeID)
	}
}

// SetNodeHealth sets node health status
func (sr *SessionRouter) SetNodeHealth(nodeID string, healthy bool) {
	sr.ring.SetNodeHealth(nodeID, healthy)
}

// GetStats returns router statistics
func (sr *SessionRouter) GetStats() *SessionRouterStats {
	sr.mu.RLock()
	stickyCount := len(sr.sticky)
	sr.mu.RUnlock()

	ringStats := sr.ring.Stats()

	return &SessionRouterStats{
		RingStats:     ringStats,
		StickySessions: stickyCount,
	}
}

// SessionRouterStats holds router statistics
type SessionRouterStats struct {
	RingStats      *HashRingStats
	StickySessions int
}

// RendezvousHash implements highest random weight (HRW) hashing
// This is an alternative to consistent hashing with better distribution
type RendezvousHash struct {
	mu    sync.RWMutex
	nodes map[string]*HashNode
}

// NewRendezvousHash creates a new rendezvous hash
func NewRendezvousHash() *RendezvousHash {
	return &RendezvousHash{
		nodes: make(map[string]*HashNode),
	}
}

// AddNode adds a node
func (rh *RendezvousHash) AddNode(node *HashNode) {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	rh.nodes[node.ID] = node
}

// RemoveNode removes a node
func (rh *RendezvousHash) RemoveNode(nodeID string) {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	delete(rh.nodes, nodeID)
}

// GetNode returns the node with highest weight for a key
func (rh *RendezvousHash) GetNode(key string) *HashNode {
	rh.mu.RLock()
	defer rh.mu.RUnlock()

	var maxNode *HashNode
	var maxWeight uint32

	for _, node := range rh.nodes {
		if !node.Healthy {
			continue
		}
		weight := rh.computeWeight(key, node.ID)
		if maxNode == nil || weight > maxWeight {
			maxWeight = weight
			maxNode = node
		}
	}

	return maxNode
}

// GetNodes returns top N nodes by weight
func (rh *RendezvousHash) GetNodes(key string, count int) []*HashNode {
	rh.mu.RLock()
	defer rh.mu.RUnlock()

	type nodeWeight struct {
		node   *HashNode
		weight uint32
	}

	weights := make([]nodeWeight, 0, len(rh.nodes))
	for _, node := range rh.nodes {
		if node.Healthy {
			weights = append(weights, nodeWeight{
				node:   node,
				weight: rh.computeWeight(key, node.ID),
			})
		}
	}

	sort.Slice(weights, func(i, j int) bool {
		return weights[i].weight > weights[j].weight
	})

	result := make([]*HashNode, 0, count)
	for i := 0; i < len(weights) && i < count; i++ {
		result = append(result, weights[i].node)
	}

	return result
}

func (rh *RendezvousHash) computeWeight(key, nodeID string) uint32 {
	combined := key + nodeID
	hash := sha256.Sum256([]byte(combined))
	return binary.BigEndian.Uint32(hash[:4])
}

// NodeCount returns the number of nodes
func (rh *RendezvousHash) NodeCount() int {
	rh.mu.RLock()
	defer rh.mu.RUnlock()
	return len(rh.nodes)
}

// SetNodeHealth sets node health
func (rh *RendezvousHash) SetNodeHealth(nodeID string, healthy bool) {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	if node, exists := rh.nodes[nodeID]; exists {
		node.Healthy = healthy
	}
}
