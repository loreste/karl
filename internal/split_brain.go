package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// SplitBrainConfig configures split brain detection and handling
type SplitBrainConfig struct {
	// QuorumSize is the minimum number of nodes required for quorum
	QuorumSize int
	// HeartbeatInterval is the interval between heartbeats
	HeartbeatInterval time.Duration
	// FailureThreshold is number of missed heartbeats before node is considered failed
	FailureThreshold int
	// RecoveryGracePeriod is how long to wait after partition heals before resuming
	RecoveryGracePeriod time.Duration
	// FencingEnabled enables node fencing on split brain
	FencingEnabled bool
	// FencingTimeout is the maximum time to wait for fencing
	FencingTimeout time.Duration
	// ConsensusTimeout is the timeout for consensus operations
	ConsensusTimeout time.Duration
}

// DefaultSplitBrainConfig returns default configuration
func DefaultSplitBrainConfig() *SplitBrainConfig {
	return &SplitBrainConfig{
		QuorumSize:          2,
		HeartbeatInterval:   1 * time.Second,
		FailureThreshold:    3,
		RecoveryGracePeriod: 5 * time.Second,
		FencingEnabled:      true,
		FencingTimeout:      10 * time.Second,
		ConsensusTimeout:    5 * time.Second,
	}
}

// PartitionState represents the current partition state
type PartitionState int

const (
	// PartitionStateNormal indicates no partition detected
	PartitionStateNormal PartitionState = iota
	// PartitionStateSuspected indicates potential partition
	PartitionStateSuspected
	// PartitionStatePartitioned indicates confirmed partition
	PartitionStatePartitioned
	// PartitionStateRecovering indicates partition is healing
	PartitionStateRecovering
	// PartitionStateFenced indicates this node is fenced
	PartitionStateFenced
)

func (ps PartitionState) String() string {
	switch ps {
	case PartitionStateNormal:
		return "normal"
	case PartitionStateSuspected:
		return "suspected"
	case PartitionStatePartitioned:
		return "partitioned"
	case PartitionStateRecovering:
		return "recovering"
	case PartitionStateFenced:
		return "fenced"
	default:
		return "unknown"
	}
}

// SplitBrainDetector handles split brain detection and resolution
type SplitBrainDetector struct {
	config  *SplitBrainConfig
	nodeID  string
	cluster *RedisSessionStore

	mu               sync.RWMutex
	state            PartitionState
	knownNodes       map[string]*NodeInfo
	reachableNodes   map[string]bool
	lastHeartbeat    map[string]time.Time
	missedHeartbeats map[string]int

	partitionStart time.Time
	recoveryStart  time.Time
	fenced         bool

	handlers []SplitBrainHandler
	stopChan chan struct{}
	doneChan chan struct{}
}

// NodeInfo represents information about a cluster node
type NodeInfo struct {
	ID            string
	Address       string
	LastSeen      time.Time
	State         string
	SessionCount  int
	PartitionID   string
	JoinedAt      time.Time
	Version       string
	Capabilities  []string
	Healthy       bool
	FencedByNode  string
	FencedAt      time.Time
}

// SplitBrainHandler is called when split brain events occur
type SplitBrainHandler func(event *SplitBrainEvent)

// SplitBrainEvent represents a split brain event
type SplitBrainEvent struct {
	Type           SplitBrainEventType
	Timestamp      time.Time
	NodeID         string
	PartitionState PartitionState
	ReachableNodes []string
	UnreachableNodes []string
	IsQuorum       bool
	Message        string
	Metadata       map[string]interface{}
}

// SplitBrainEventType represents the type of split brain event
type SplitBrainEventType int

const (
	SplitBrainEventPartitionDetected SplitBrainEventType = iota
	SplitBrainEventPartitionSuspected
	SplitBrainEventPartitionRecovered
	SplitBrainEventQuorumLost
	SplitBrainEventQuorumRegained
	SplitBrainEventNodeFenced
	SplitBrainEventNodeUnfenced
	SplitBrainEventNodeJoined
	SplitBrainEventNodeLeft
)

func (t SplitBrainEventType) String() string {
	switch t {
	case SplitBrainEventPartitionDetected:
		return "partition_detected"
	case SplitBrainEventPartitionSuspected:
		return "partition_suspected"
	case SplitBrainEventPartitionRecovered:
		return "partition_recovered"
	case SplitBrainEventQuorumLost:
		return "quorum_lost"
	case SplitBrainEventQuorumRegained:
		return "quorum_regained"
	case SplitBrainEventNodeFenced:
		return "node_fenced"
	case SplitBrainEventNodeUnfenced:
		return "node_unfenced"
	case SplitBrainEventNodeJoined:
		return "node_joined"
	case SplitBrainEventNodeLeft:
		return "node_left"
	default:
		return "unknown"
	}
}

// NewSplitBrainDetector creates a new split brain detector
func NewSplitBrainDetector(nodeID string, cluster *RedisSessionStore, config *SplitBrainConfig) *SplitBrainDetector {
	if config == nil {
		config = DefaultSplitBrainConfig()
	}

	return &SplitBrainDetector{
		config:           config,
		nodeID:           nodeID,
		cluster:          cluster,
		state:            PartitionStateNormal,
		knownNodes:       make(map[string]*NodeInfo),
		reachableNodes:   make(map[string]bool),
		lastHeartbeat:    make(map[string]time.Time),
		missedHeartbeats: make(map[string]int),
		handlers:         make([]SplitBrainHandler, 0),
		stopChan:         make(chan struct{}),
		doneChan:         make(chan struct{}),
	}
}

// Start begins split brain detection
func (sbd *SplitBrainDetector) Start() {
	go sbd.heartbeatLoop()
	go sbd.detectionLoop()
}

// Stop stops split brain detection
func (sbd *SplitBrainDetector) Stop() {
	close(sbd.stopChan)
	<-sbd.doneChan
}

// AddHandler adds a split brain event handler
func (sbd *SplitBrainDetector) AddHandler(handler SplitBrainHandler) {
	sbd.mu.Lock()
	defer sbd.mu.Unlock()
	sbd.handlers = append(sbd.handlers, handler)
}

// GetState returns the current partition state
func (sbd *SplitBrainDetector) GetState() PartitionState {
	sbd.mu.RLock()
	defer sbd.mu.RUnlock()
	return sbd.state
}

// IsFenced returns whether this node is fenced
func (sbd *SplitBrainDetector) IsFenced() bool {
	sbd.mu.RLock()
	defer sbd.mu.RUnlock()
	return sbd.fenced
}

// HasQuorum returns whether this partition has quorum
func (sbd *SplitBrainDetector) HasQuorum() bool {
	sbd.mu.RLock()
	defer sbd.mu.RUnlock()
	return sbd.hasQuorum()
}

func (sbd *SplitBrainDetector) hasQuorum() bool {
	reachableCount := 1 // Include self
	for _, reachable := range sbd.reachableNodes {
		if reachable {
			reachableCount++
		}
	}
	return reachableCount >= sbd.config.QuorumSize
}

// GetReachableNodes returns list of reachable node IDs
func (sbd *SplitBrainDetector) GetReachableNodes() []string {
	sbd.mu.RLock()
	defer sbd.mu.RUnlock()

	nodes := []string{sbd.nodeID} // Include self
	for nodeID, reachable := range sbd.reachableNodes {
		if reachable {
			nodes = append(nodes, nodeID)
		}
	}
	return nodes
}

// GetUnreachableNodes returns list of unreachable node IDs
func (sbd *SplitBrainDetector) GetUnreachableNodes() []string {
	sbd.mu.RLock()
	defer sbd.mu.RUnlock()

	var nodes []string
	for nodeID, reachable := range sbd.reachableNodes {
		if !reachable {
			nodes = append(nodes, nodeID)
		}
	}
	return nodes
}

// RegisterNode registers a node in the cluster
func (sbd *SplitBrainDetector) RegisterNode(info *NodeInfo) {
	sbd.mu.Lock()
	defer sbd.mu.Unlock()

	sbd.knownNodes[info.ID] = info
	sbd.reachableNodes[info.ID] = true
	sbd.lastHeartbeat[info.ID] = time.Now()
	sbd.missedHeartbeats[info.ID] = 0

	sbd.emitEvent(&SplitBrainEvent{
		Type:           SplitBrainEventNodeJoined,
		Timestamp:      time.Now(),
		NodeID:         info.ID,
		PartitionState: sbd.state,
		Message:        fmt.Sprintf("Node %s joined cluster", info.ID),
	})
}

// UnregisterNode removes a node from the cluster
func (sbd *SplitBrainDetector) UnregisterNode(nodeID string) {
	sbd.mu.Lock()
	defer sbd.mu.Unlock()

	delete(sbd.knownNodes, nodeID)
	delete(sbd.reachableNodes, nodeID)
	delete(sbd.lastHeartbeat, nodeID)
	delete(sbd.missedHeartbeats, nodeID)

	sbd.emitEvent(&SplitBrainEvent{
		Type:           SplitBrainEventNodeLeft,
		Timestamp:      time.Now(),
		NodeID:         nodeID,
		PartitionState: sbd.state,
		Message:        fmt.Sprintf("Node %s left cluster", nodeID),
	})
}

func (sbd *SplitBrainDetector) heartbeatLoop() {
	ticker := time.NewTicker(sbd.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sbd.stopChan:
			return
		case <-ticker.C:
			sbd.sendHeartbeat()
		}
	}
}

func (sbd *SplitBrainDetector) sendHeartbeat() {
	if sbd.cluster == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), sbd.config.HeartbeatInterval/2)
	defer cancel()

	heartbeat := &ClusterHeartbeat{
		NodeID:       sbd.nodeID,
		Timestamp:    time.Now(),
		State:        sbd.GetState().String(),
		HasQuorum:    sbd.HasQuorum(),
		SessionCount: 0, // Would be populated from actual session count
	}

	data, err := json.Marshal(heartbeat)
	if err != nil {
		return
	}

	// Publish heartbeat to all nodes
	key := fmt.Sprintf("cluster:heartbeat:%s", sbd.nodeID)
	err = sbd.cluster.client.Set(ctx, key, string(data), sbd.config.HeartbeatInterval*3)
	if err != nil {
		// Log error but don't fail
		return
	}

	// Publish to heartbeat channel for real-time updates
	sbd.cluster.client.Publish(ctx, "cluster:heartbeats", string(data))
}

// ClusterHeartbeat represents a heartbeat message
type ClusterHeartbeat struct {
	NodeID       string         `json:"node_id"`
	Timestamp    time.Time      `json:"timestamp"`
	State        string         `json:"state"`
	HasQuorum    bool           `json:"has_quorum"`
	SessionCount int            `json:"session_count"`
	Reachable    []string       `json:"reachable,omitempty"`
	Unreachable  []string       `json:"unreachable,omitempty"`
}

func (sbd *SplitBrainDetector) detectionLoop() {
	defer close(sbd.doneChan)

	checkTicker := time.NewTicker(sbd.config.HeartbeatInterval)
	defer checkTicker.Stop()

	for {
		select {
		case <-sbd.stopChan:
			return
		case <-checkTicker.C:
			sbd.checkNodes()
			sbd.evaluatePartitionState()
		}
	}
}

func (sbd *SplitBrainDetector) checkNodes() {
	if sbd.cluster == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), sbd.config.HeartbeatInterval)
	defer cancel()

	sbd.mu.Lock()
	defer sbd.mu.Unlock()

	now := time.Now()

	// Check all known nodes
	for nodeID := range sbd.knownNodes {
		if nodeID == sbd.nodeID {
			continue
		}

		key := fmt.Sprintf("cluster:heartbeat:%s", nodeID)
		dataStr, err := sbd.cluster.client.Get(ctx, key)
		if err != nil {
			// Node heartbeat not found or expired
			sbd.missedHeartbeats[nodeID]++
			if sbd.missedHeartbeats[nodeID] >= sbd.config.FailureThreshold {
				if sbd.reachableNodes[nodeID] {
					sbd.reachableNodes[nodeID] = false
					// Node became unreachable
				}
			}
			continue
		}

		var heartbeat ClusterHeartbeat
		if err := json.Unmarshal([]byte(dataStr), &heartbeat); err != nil {
			continue
		}

		// Check if heartbeat is recent
		if now.Sub(heartbeat.Timestamp) < sbd.config.HeartbeatInterval*time.Duration(sbd.config.FailureThreshold) {
			if !sbd.reachableNodes[nodeID] {
				sbd.reachableNodes[nodeID] = true
				// Node became reachable
			}
			sbd.lastHeartbeat[nodeID] = heartbeat.Timestamp
			sbd.missedHeartbeats[nodeID] = 0
		} else {
			sbd.missedHeartbeats[nodeID]++
			if sbd.missedHeartbeats[nodeID] >= sbd.config.FailureThreshold {
				sbd.reachableNodes[nodeID] = false
			}
		}
	}
}

func (sbd *SplitBrainDetector) evaluatePartitionState() {
	sbd.mu.Lock()
	defer sbd.mu.Unlock()

	oldState := sbd.state
	hasQuorum := sbd.hasQuorum()
	now := time.Now()

	unreachableCount := 0
	for _, reachable := range sbd.reachableNodes {
		if !reachable {
			unreachableCount++
		}
	}

	switch sbd.state {
	case PartitionStateNormal:
		if unreachableCount > 0 {
			sbd.state = PartitionStateSuspected
			sbd.emitEvent(&SplitBrainEvent{
				Type:             SplitBrainEventPartitionSuspected,
				Timestamp:        now,
				NodeID:           sbd.nodeID,
				PartitionState:   sbd.state,
				ReachableNodes:   sbd.getReachableNodesList(),
				UnreachableNodes: sbd.getUnreachableNodesList(),
				IsQuorum:         hasQuorum,
				Message:          fmt.Sprintf("%d nodes unreachable", unreachableCount),
			})
		}

	case PartitionStateSuspected:
		if unreachableCount == 0 {
			// False alarm
			sbd.state = PartitionStateNormal
		} else if !hasQuorum {
			// Lost quorum - partition confirmed
			sbd.state = PartitionStatePartitioned
			sbd.partitionStart = now
			sbd.emitEvent(&SplitBrainEvent{
				Type:             SplitBrainEventPartitionDetected,
				Timestamp:        now,
				NodeID:           sbd.nodeID,
				PartitionState:   sbd.state,
				ReachableNodes:   sbd.getReachableNodesList(),
				UnreachableNodes: sbd.getUnreachableNodesList(),
				IsQuorum:         hasQuorum,
				Message:          "Network partition detected - quorum lost",
			})
			sbd.emitEvent(&SplitBrainEvent{
				Type:           SplitBrainEventQuorumLost,
				Timestamp:      now,
				NodeID:         sbd.nodeID,
				PartitionState: sbd.state,
				IsQuorum:       hasQuorum,
				Message:        "Quorum lost",
			})

			// Fence if enabled and we don't have quorum
			if sbd.config.FencingEnabled && !hasQuorum {
				sbd.fenceNode()
			}
		}

	case PartitionStatePartitioned:
		if hasQuorum {
			sbd.state = PartitionStateRecovering
			sbd.recoveryStart = now
			sbd.emitEvent(&SplitBrainEvent{
				Type:             SplitBrainEventQuorumRegained,
				Timestamp:        now,
				NodeID:           sbd.nodeID,
				PartitionState:   sbd.state,
				ReachableNodes:   sbd.getReachableNodesList(),
				UnreachableNodes: sbd.getUnreachableNodesList(),
				IsQuorum:         hasQuorum,
				Message:          "Quorum regained - beginning recovery",
			})
		}

	case PartitionStateRecovering:
		if !hasQuorum {
			// Back to partitioned
			sbd.state = PartitionStatePartitioned
		} else if now.Sub(sbd.recoveryStart) >= sbd.config.RecoveryGracePeriod {
			// Recovery complete
			sbd.state = PartitionStateNormal
			sbd.emitEvent(&SplitBrainEvent{
				Type:             SplitBrainEventPartitionRecovered,
				Timestamp:        now,
				NodeID:           sbd.nodeID,
				PartitionState:   sbd.state,
				ReachableNodes:   sbd.getReachableNodesList(),
				UnreachableNodes: sbd.getUnreachableNodesList(),
				IsQuorum:         hasQuorum,
				Message:          "Partition recovered",
			})

			// Unfence if we were fenced
			if sbd.fenced {
				sbd.unfenceNode()
			}
		}

	case PartitionStateFenced:
		if hasQuorum {
			sbd.unfenceNode()
			sbd.state = PartitionStateRecovering
			sbd.recoveryStart = now
		}
	}

	if oldState != sbd.state {
		// State changed - log it
	}
}

func (sbd *SplitBrainDetector) fenceNode() {
	if sbd.fenced {
		return
	}

	sbd.fenced = true
	sbd.state = PartitionStateFenced

	sbd.emitEvent(&SplitBrainEvent{
		Type:           SplitBrainEventNodeFenced,
		Timestamp:      time.Now(),
		NodeID:         sbd.nodeID,
		PartitionState: sbd.state,
		IsQuorum:       false,
		Message:        "Node fenced due to quorum loss",
	})
}

func (sbd *SplitBrainDetector) unfenceNode() {
	if !sbd.fenced {
		return
	}

	sbd.fenced = false

	sbd.emitEvent(&SplitBrainEvent{
		Type:           SplitBrainEventNodeUnfenced,
		Timestamp:      time.Now(),
		NodeID:         sbd.nodeID,
		PartitionState: sbd.state,
		IsQuorum:       sbd.hasQuorum(),
		Message:        "Node unfenced",
	})
}

func (sbd *SplitBrainDetector) getReachableNodesList() []string {
	nodes := []string{sbd.nodeID}
	for nodeID, reachable := range sbd.reachableNodes {
		if reachable {
			nodes = append(nodes, nodeID)
		}
	}
	return nodes
}

func (sbd *SplitBrainDetector) getUnreachableNodesList() []string {
	var nodes []string
	for nodeID, reachable := range sbd.reachableNodes {
		if !reachable {
			nodes = append(nodes, nodeID)
		}
	}
	return nodes
}

func (sbd *SplitBrainDetector) emitEvent(event *SplitBrainEvent) {
	for _, handler := range sbd.handlers {
		go handler(event)
	}
}

// GetStats returns split brain detector statistics
func (sbd *SplitBrainDetector) GetStats() *SplitBrainStats {
	sbd.mu.RLock()
	defer sbd.mu.RUnlock()

	stats := &SplitBrainStats{
		NodeID:          sbd.nodeID,
		State:           sbd.state.String(),
		HasQuorum:       sbd.hasQuorum(),
		IsFenced:        sbd.fenced,
		TotalNodes:      len(sbd.knownNodes) + 1, // Include self
		ReachableNodes:  1,                        // Self
		UnreachableNodes: 0,
		QuorumSize:      sbd.config.QuorumSize,
	}

	for _, reachable := range sbd.reachableNodes {
		if reachable {
			stats.ReachableNodes++
		} else {
			stats.UnreachableNodes++
		}
	}

	if !sbd.partitionStart.IsZero() {
		stats.PartitionDuration = time.Since(sbd.partitionStart)
	}

	return stats
}

// SplitBrainStats contains split brain detector statistics
type SplitBrainStats struct {
	NodeID            string
	State             string
	HasQuorum         bool
	IsFenced          bool
	TotalNodes        int
	ReachableNodes    int
	UnreachableNodes  int
	QuorumSize        int
	PartitionDuration time.Duration
}

// FenceAction represents an action to take when fenced
type FenceAction int

const (
	FenceActionNone FenceAction = iota
	FenceActionRejectWrites
	FenceActionDrainSessions
	FenceActionShutdown
)

// FencingPolicy configures how to respond to fencing
type FencingPolicy struct {
	// Action to take when fenced
	Action FenceAction
	// Whether to transfer sessions before fencing
	TransferSessions bool
	// Maximum time to wait for session transfer
	TransferTimeout time.Duration
}

// DefaultFencingPolicy returns the default fencing policy
func DefaultFencingPolicy() *FencingPolicy {
	return &FencingPolicy{
		Action:           FenceActionRejectWrites,
		TransferSessions: true,
		TransferTimeout:  30 * time.Second,
	}
}
