package internal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ClusterConfig configures the cluster manager
type ClusterConfig struct {
	NodeID            string
	NodeAddress       string
	RedisClient       RedisClient
	SessionTTL        time.Duration
	HeartbeatInterval time.Duration
	FailureThreshold  int
	QuorumSize        int
	ReplicationFactor int
	EnableFencing     bool
}

// DefaultClusterConfig returns default configuration
func DefaultClusterConfig(nodeID, nodeAddress string, client RedisClient) *ClusterConfig {
	return &ClusterConfig{
		NodeID:            nodeID,
		NodeAddress:       nodeAddress,
		RedisClient:       client,
		SessionTTL:        30 * time.Minute,
		HeartbeatInterval: 1 * time.Second,
		FailureThreshold:  3,
		QuorumSize:        2,
		ReplicationFactor: 150,
		EnableFencing:     true,
	}
}

// ClusterManager orchestrates all cluster components
type ClusterManager struct {
	config *ClusterConfig

	// Core components
	sessionStore *RedisSessionStore
	hashRing     *HashRing
	router       *SessionRouter
	splitBrain   *SplitBrainDetector
	portAlloc    *PortAllocator

	// State
	mu       sync.RWMutex
	running  atomic.Bool
	fenced   atomic.Bool
	stopCh   chan struct{}
	doneCh   chan struct{}

	// Metrics
	stats clusterManagerStats
}

type clusterManagerStats struct {
	sessionsRouted    atomic.Int64
	sessionsRerouted  atomic.Int64
	nodeFailures      atomic.Int64
	partitionEvents   atomic.Int64
	takeoversDone     atomic.Int64
	takeoversFailed   atomic.Int64
}

// NewClusterManager creates a new cluster manager
func NewClusterManager(config *ClusterConfig) *ClusterManager {
	if config == nil {
		panic("cluster config required")
	}

	// Create session store
	sessionStore := NewRedisSessionStore(config.RedisClient, config.NodeID, config.SessionTTL)

	// Create hash ring with configured replication factor
	hashConfig := &ConsistentHashConfig{
		ReplicationFactor: config.ReplicationFactor,
		LoadFactor:        1.25,
	}
	hashRing := NewHashRing(hashConfig)

	// Create session router
	router := NewSessionRouter(hashConfig)

	// Create split brain detector
	splitBrainConfig := &SplitBrainConfig{
		QuorumSize:          config.QuorumSize,
		HeartbeatInterval:   config.HeartbeatInterval,
		FailureThreshold:    config.FailureThreshold,
		RecoveryGracePeriod: 5 * time.Second,
		FencingEnabled:      config.EnableFencing,
		FencingTimeout:      10 * time.Second,
		ConsensusTimeout:    5 * time.Second,
	}
	splitBrain := NewSplitBrainDetector(config.NodeID, sessionStore, splitBrainConfig)

	// Get global port allocator
	portAlloc := GetPortAllocator()

	cm := &ClusterManager{
		config:       config,
		sessionStore: sessionStore,
		hashRing:     hashRing,
		router:       router,
		splitBrain:   splitBrain,
		portAlloc:    portAlloc,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}

	// Register split brain event handlers
	splitBrain.AddHandler(cm.handleSplitBrainEvent)

	return cm
}

// Start starts the cluster manager
func (cm *ClusterManager) Start(ctx context.Context) error {
	if !cm.running.CompareAndSwap(false, true) {
		return fmt.Errorf("cluster manager already running")
	}

	// Add self to hash ring
	cm.hashRing.AddNode(&HashNode{
		ID:      cm.config.NodeID,
		Address: cm.config.NodeAddress,
		Weight:  1,
		Healthy: true,
	})
	cm.router.AddNode(&HashNode{
		ID:      cm.config.NodeID,
		Address: cm.config.NodeAddress,
		Weight:  1,
		Healthy: true,
	})

	// Start session store
	if err := cm.sessionStore.Start(ctx); err != nil {
		return fmt.Errorf("failed to start session store: %w", err)
	}

	// Start split brain detector
	cm.splitBrain.Start()

	// Start background tasks
	go cm.monitorLoop(ctx)

	LogInfo("Cluster manager started", map[string]interface{}{
		"node_id":      cm.config.NodeID,
		"node_address": cm.config.NodeAddress,
	})

	return nil
}

// Stop stops the cluster manager
func (cm *ClusterManager) Stop(ctx context.Context) error {
	if !cm.running.CompareAndSwap(true, false) {
		return nil
	}

	close(cm.stopCh)

	// Stop split brain detector
	cm.splitBrain.Stop()

	// Stop session store
	if err := cm.sessionStore.Stop(ctx); err != nil {
		LogError("Failed to stop session store", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Remove self from hash ring
	cm.hashRing.RemoveNode(cm.config.NodeID)
	cm.router.RemoveNode(cm.config.NodeID)

	close(cm.doneCh)

	LogInfo("Cluster manager stopped", map[string]interface{}{
		"node_id": cm.config.NodeID,
	})

	return nil
}

// monitorLoop monitors cluster health
func (cm *ClusterManager) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(cm.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cm.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			cm.updateNodeHealth()
		}
	}
}

// updateNodeHealth updates node health in hash ring based on split brain detector
func (cm *ClusterManager) updateNodeHealth() {
	reachable := cm.splitBrain.GetReachableNodes()
	unreachable := cm.splitBrain.GetUnreachableNodes()

	for _, nodeID := range reachable {
		cm.hashRing.SetNodeHealth(nodeID, true)
		cm.router.SetNodeHealth(nodeID, true)
	}

	for _, nodeID := range unreachable {
		cm.hashRing.SetNodeHealth(nodeID, false)
		cm.router.SetNodeHealth(nodeID, false)
	}
}

// handleSplitBrainEvent handles events from the split brain detector
func (cm *ClusterManager) handleSplitBrainEvent(event *SplitBrainEvent) {
	switch event.Type {
	case SplitBrainEventNodeJoined:
		cm.handleNodeJoined(event)
	case SplitBrainEventNodeLeft:
		cm.handleNodeLeft(event)
	case SplitBrainEventPartitionDetected:
		cm.handlePartitionDetected(event)
	case SplitBrainEventPartitionRecovered:
		cm.handlePartitionRecovered(event)
	case SplitBrainEventNodeFenced:
		cm.handleNodeFenced(event)
	case SplitBrainEventNodeUnfenced:
		cm.handleNodeUnfenced(event)
	case SplitBrainEventQuorumLost:
		cm.handleQuorumLost(event)
	case SplitBrainEventQuorumRegained:
		cm.handleQuorumRegained(event)
	}
}

// handleNodeJoined handles a new node joining the cluster
func (cm *ClusterManager) handleNodeJoined(event *SplitBrainEvent) {
	LogInfo("Cluster: node joined", map[string]interface{}{
		"node_id": event.NodeID,
	})

	// Add node to hash ring
	cm.hashRing.AddNode(&HashNode{
		ID:      event.NodeID,
		Healthy: true,
	})
	cm.router.AddNode(&HashNode{
		ID:      event.NodeID,
		Healthy: true,
	})
}

// handleNodeLeft handles a node leaving the cluster
func (cm *ClusterManager) handleNodeLeft(event *SplitBrainEvent) {
	LogInfo("Cluster: node left, initiating session takeover", map[string]interface{}{
		"node_id": event.NodeID,
	})

	cm.stats.nodeFailures.Add(1)

	// Remove from hash ring first to prevent new sessions routing to it
	cm.hashRing.RemoveNode(event.NodeID)
	cm.router.RemoveNode(event.NodeID)

	// Trigger session takeover using the hash ring for proper distribution
	go cm.takeoverNodeSessions(event.NodeID)
}

// takeoverNodeSessions takes over sessions from a failed node using consistent hashing
func (cm *ClusterManager) takeoverNodeSessions(failedNodeID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get sessions owned by failed node
	sessionIDs, err := cm.sessionStore.getSessionIDsByNodeFast(ctx, failedNodeID)
	if err != nil {
		LogError("Failed to get sessions for takeover", map[string]interface{}{
			"failed_node": failedNodeID,
			"error":       err.Error(),
		})
		return
	}

	if len(sessionIDs) == 0 {
		LogDebug("No sessions to take over", map[string]interface{}{
			"failed_node": failedNodeID,
		})
		return
	}

	LogInfo("Starting consistent hash session takeover", map[string]interface{}{
		"failed_node":   failedNodeID,
		"session_count": len(sessionIDs),
	})

	// Batch fetch sessions
	sessions, err := cm.sessionStore.GetSessionsBatch(ctx, sessionIDs)
	if err != nil {
		LogError("Failed to fetch sessions for takeover", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	var taken, skipped int
	for _, session := range sessions {
		if session.NodeID != failedNodeID {
			skipped++
			continue // Already taken over
		}

		// Use hash ring to determine new owner
		targetNode := cm.hashRing.GetNode(session.ID)
		if targetNode == nil {
			LogError("No target node for session", map[string]interface{}{
				"session_id": session.ID,
			})
			cm.stats.takeoversFailed.Add(1)
			continue
		}

		if targetNode.ID != cm.config.NodeID {
			skipped++ // Another node should take this
			continue
		}

		// This node should take over the session
		if err := cm.sessionStore.executeSessionTakeover(ctx, session); err != nil {
			LogError("Session takeover failed", map[string]interface{}{
				"session_id": session.ID,
				"error":      err.Error(),
			})
			cm.stats.takeoversFailed.Add(1)
			continue
		}

		taken++
		cm.stats.takeoversDone.Add(1)

		// Update hash ring load
		cm.hashRing.IncrementLoad(cm.config.NodeID)
	}

	LogInfo("Session takeover complete", map[string]interface{}{
		"failed_node": failedNodeID,
		"taken":       taken,
		"skipped":     skipped,
		"total":       len(sessions),
	})
}

// handlePartitionDetected handles network partition detection
func (cm *ClusterManager) handlePartitionDetected(event *SplitBrainEvent) {
	LogWarn("Network partition detected", map[string]interface{}{
		"reachable":   event.ReachableNodes,
		"unreachable": event.UnreachableNodes,
		"has_quorum":  event.IsQuorum,
	})
	cm.stats.partitionEvents.Add(1)
}

// handlePartitionRecovered handles partition recovery
func (cm *ClusterManager) handlePartitionRecovered(event *SplitBrainEvent) {
	LogInfo("Network partition recovered", map[string]interface{}{
		"reachable": event.ReachableNodes,
	})
}

// handleNodeFenced handles this node being fenced
func (cm *ClusterManager) handleNodeFenced(event *SplitBrainEvent) {
	LogWarn("This node has been fenced", map[string]interface{}{
		"reason": event.Message,
	})
	cm.fenced.Store(true)
}

// handleNodeUnfenced handles this node being unfenced
func (cm *ClusterManager) handleNodeUnfenced(event *SplitBrainEvent) {
	LogInfo("This node has been unfenced", nil)
	cm.fenced.Store(false)
}

// handleQuorumLost handles quorum loss
func (cm *ClusterManager) handleQuorumLost(event *SplitBrainEvent) {
	LogWarn("Quorum lost - read-only mode", map[string]interface{}{
		"reachable_nodes": event.ReachableNodes,
	})
}

// handleQuorumRegained handles quorum recovery
func (cm *ClusterManager) handleQuorumRegained(event *SplitBrainEvent) {
	LogInfo("Quorum regained - resuming normal operations", map[string]interface{}{
		"reachable_nodes": event.ReachableNodes,
	})
}

// RouteSession routes a session to the appropriate node
func (cm *ClusterManager) RouteSession(sessionID string) (*HashNode, error) {
	if cm.fenced.Load() {
		return nil, fmt.Errorf("node is fenced")
	}

	node := cm.router.RouteSession(sessionID)
	if node == nil {
		return nil, fmt.Errorf("no available nodes")
	}

	cm.stats.sessionsRouted.Add(1)
	return node, nil
}

// IsLocalSession checks if a session should be handled by this node
func (cm *ClusterManager) IsLocalSession(sessionID string) bool {
	node := cm.hashRing.GetNode(sessionID)
	return node != nil && node.ID == cm.config.NodeID
}

// IsFenced returns whether this node is fenced
func (cm *ClusterManager) IsFenced() bool {
	return cm.fenced.Load()
}

// HasQuorum returns whether the cluster has quorum
func (cm *ClusterManager) HasQuorum() bool {
	return cm.splitBrain.HasQuorum()
}

// GetSessionStore returns the session store
func (cm *ClusterManager) GetSessionStore() *RedisSessionStore {
	return cm.sessionStore
}

// GetHashRing returns the hash ring
func (cm *ClusterManager) GetHashRing() *HashRing {
	return cm.hashRing
}

// GetNodeCount returns the number of nodes in the cluster
func (cm *ClusterManager) GetNodeCount() int {
	return cm.hashRing.NodeCount()
}

// GetHealthyNodeCount returns the number of healthy nodes
func (cm *ClusterManager) GetHealthyNodeCount() int {
	return len(cm.hashRing.GetHealthyNodes())
}

// GetStats returns cluster manager statistics
func (cm *ClusterManager) GetStats() map[string]interface{} {
	ringStats := cm.hashRing.Stats()
	storeStats := cm.sessionStore.GetStats()
	sbStats := cm.splitBrain.GetStats()
	portStats := cm.portAlloc.GetStats()

	return map[string]interface{}{
		"node_id":           cm.config.NodeID,
		"running":           cm.running.Load(),
		"fenced":            cm.fenced.Load(),
		"has_quorum":        cm.HasQuorum(),
		"sessions_routed":   cm.stats.sessionsRouted.Load(),
		"sessions_rerouted": cm.stats.sessionsRerouted.Load(),
		"node_failures":     cm.stats.nodeFailures.Load(),
		"partition_events":  cm.stats.partitionEvents.Load(),
		"takeovers_done":    cm.stats.takeoversDone.Load(),
		"takeovers_failed":  cm.stats.takeoversFailed.Load(),
		"hash_ring": map[string]interface{}{
			"node_count":      ringStats.NodeCount,
			"virtual_nodes":   ringStats.VirtualNodes,
			"total_load":      ringStats.TotalLoad,
			"avg_load":        ringStats.AvgLoad,
			"healthy_count":   ringStats.HealthyCount,
			"unhealthy_count": ringStats.UnhealthyCount,
		},
		"session_store": storeStats,
		"split_brain": map[string]interface{}{
			"state":             sbStats.State,
			"has_quorum":        sbStats.HasQuorum,
			"is_fenced":         sbStats.IsFenced,
			"reachable_nodes":   sbStats.ReachableNodes,
			"unreachable_nodes": sbStats.UnreachableNodes,
		},
		"port_allocator": portStats,
	}
}

// RegisterNode registers a new node in the cluster
func (cm *ClusterManager) RegisterNode(nodeID, address string) {
	node := &HashNode{
		ID:      nodeID,
		Address: address,
		Weight:  1,
		Healthy: true,
	}
	cm.hashRing.AddNode(node)
	cm.router.AddNode(node)
	cm.splitBrain.RegisterNode(&NodeInfo{
		ID:       nodeID,
		Address:  address,
		LastSeen: time.Now(),
		State:    "active",
		Healthy:  true,
	})
}

// UnregisterNode unregisters a node from the cluster
func (cm *ClusterManager) UnregisterNode(nodeID string) {
	cm.hashRing.RemoveNode(nodeID)
	cm.router.RemoveNode(nodeID)
	cm.splitBrain.UnregisterNode(nodeID)
}
