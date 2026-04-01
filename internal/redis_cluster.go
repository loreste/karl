package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// RedisSessionStore provides Redis-backed session storage for clustering
type RedisSessionStore struct {
	client        RedisClient
	prefix        string
	localNode     string
	ttl           time.Duration
	syncInterval  time.Duration
	mu            sync.RWMutex
	stopCh        chan struct{}
	portAllocator *PortAllocator
	clusterNodes  map[string]time.Time
	nodesMu       sync.RWMutex

	// Performance optimizations
	nodeSessionIndex map[string]map[string]struct{} // nodeID -> set of sessionIDs
	indexMu          sync.RWMutex
	sessionPool      sync.Pool // reuse SessionData objects
	messagePool      sync.Pool // reuse ClusterMessage objects
	jsonEncoder      sync.Pool // reuse json encoders
	workerCount      int
	takeoverCh       chan *takeoverJob
	stats            clusterStats
}

// clusterStats tracks performance metrics
type clusterStats struct {
	sessionsStored   atomic.Int64
	sessionsDeleted  atomic.Int64
	sessionsTakenOver atomic.Int64
	takeoverLatencyNs atomic.Int64
	messagesProcessed atomic.Int64
}

// takeoverJob represents a session takeover task
type takeoverJob struct {
	session *SessionData
	ctx     context.Context
	done    chan error
}

// RedisClient interface for Redis operations (allows mocking)
type RedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Del(ctx context.Context, keys ...string) error
	Keys(ctx context.Context, pattern string) ([]string, error)
	Exists(ctx context.Context, keys ...string) (int64, error)
	Expire(ctx context.Context, key string, expiration time.Duration) error
	Publish(ctx context.Context, channel string, message interface{}) error
	Subscribe(ctx context.Context, channels ...string) (PubSub, error)
	// Batch operations for performance
	MGet(ctx context.Context, keys ...string) ([]string, error)
	MSet(ctx context.Context, pairs ...interface{}) error
	// Scan for non-blocking iteration (preferred over Keys)
	Scan(ctx context.Context, cursor uint64, pattern string, count int64) ([]string, uint64, error)
	// Pipeline for batching commands
	Pipeline(ctx context.Context) RedisPipeline
	// Set operations for O(1) membership
	SAdd(ctx context.Context, key string, members ...interface{}) error
	SRem(ctx context.Context, key string, members ...interface{}) error
	SMembers(ctx context.Context, key string) ([]string, error)
}

// RedisPipeline interface for batched operations
type RedisPipeline interface {
	Get(key string) *PipelineCmd
	Set(key string, value interface{}, expiration time.Duration) *PipelineCmd
	Del(keys ...string) *PipelineCmd
	Exec(ctx context.Context) error
}

// PipelineCmd represents a pipelined command result
type PipelineCmd struct {
	val string
	err error
}

// Value returns the command result
func (c *PipelineCmd) Value() (string, error) {
	return c.val, c.err
}

// PubSub interface for Redis pub/sub
type PubSub interface {
	Receive(ctx context.Context) (interface{}, error)
	Close() error
}

// ClusterMessage represents a message between cluster nodes
type ClusterMessage struct {
	Type      ClusterMsgType `json:"type"`
	NodeID    string         `json:"node_id"`
	SessionID string         `json:"session_id,omitempty"`
	CallID    string         `json:"call_id,omitempty"`
	Data      []byte         `json:"data,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// ClusterMsgType represents cluster message types
type ClusterMsgType string

const (
	MsgTypeSessionCreate   ClusterMsgType = "session_create"
	MsgTypeSessionUpdate   ClusterMsgType = "session_update"
	MsgTypeSessionDelete   ClusterMsgType = "session_delete"
	MsgTypeSessionTransfer ClusterMsgType = "session_transfer"
	MsgTypeHeartbeat       ClusterMsgType = "heartbeat"
	MsgTypeNodeJoin        ClusterMsgType = "node_join"
	MsgTypeNodeLeave       ClusterMsgType = "node_leave"
)

// SessionData represents serializable session data
type SessionData struct {
	ID           string            `json:"id"`
	CallID       string            `json:"call_id"`
	FromTag      string            `json:"from_tag"`
	ToTag        string            `json:"to_tag"`
	ViaBranch    string            `json:"via_branch"`
	State        string            `json:"state"`
	NodeID       string            `json:"node_id"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	Flags        map[string]bool   `json:"flags"`
	Metadata     map[string]string `json:"metadata"`
	CallerLeg    *LegData          `json:"caller_leg,omitempty"`
	CalleeLeg    *LegData          `json:"callee_leg,omitempty"`
}

// Reset clears the session data for reuse
func (s *SessionData) Reset() {
	s.ID = ""
	s.CallID = ""
	s.FromTag = ""
	s.ToTag = ""
	s.ViaBranch = ""
	s.State = ""
	s.NodeID = ""
	s.CreatedAt = time.Time{}
	s.UpdatedAt = time.Time{}
	s.Flags = nil
	s.Metadata = nil
	s.CallerLeg = nil
	s.CalleeLeg = nil
}

// LegData represents serializable leg data
type LegData struct {
	Tag           string `json:"tag"`
	Label         string `json:"label"`
	IP            string `json:"ip"`
	Port          int    `json:"port"`
	RTCPPort      int    `json:"rtcp_port"`
	LocalIP       string `json:"local_ip"`
	LocalPort     int    `json:"local_port"`
	LocalRTCPPort int    `json:"local_rtcp_port"`
	Interface     string `json:"interface"`
	Direction     string `json:"direction"`
}

// NewRedisSessionStore creates a new Redis session store
func NewRedisSessionStore(client RedisClient, nodeID string, ttl time.Duration) *RedisSessionStore {
	workerCount := runtime.NumCPU() * 2
	if workerCount < 4 {
		workerCount = 4
	}

	store := &RedisSessionStore{
		client:           client,
		prefix:           "karl:session:",
		localNode:        nodeID,
		ttl:              ttl,
		syncInterval:     5 * time.Second,
		stopCh:           make(chan struct{}),
		portAllocator:    GetPortAllocator(),
		clusterNodes:     make(map[string]time.Time),
		nodeSessionIndex: make(map[string]map[string]struct{}),
		workerCount:      workerCount,
		takeoverCh:       make(chan *takeoverJob, 1000),
		sessionPool: sync.Pool{
			New: func() interface{} {
				return &SessionData{}
			},
		},
		messagePool: sync.Pool{
			New: func() interface{} {
				return &ClusterMessage{}
			},
		},
	}

	// Start takeover workers
	for i := 0; i < workerCount; i++ {
		go store.takeoverWorker()
	}

	return store
}

// takeoverWorker processes session takeover jobs
func (rs *RedisSessionStore) takeoverWorker() {
	for {
		select {
		case <-rs.stopCh:
			return
		case job := <-rs.takeoverCh:
			err := rs.executeSessionTakeover(job.ctx, job.session)
			if job.done != nil {
				job.done <- err
			}
		}
	}
}

// SetPortAllocator sets a custom port allocator
func (rs *RedisSessionStore) SetPortAllocator(pa *PortAllocator) {
	rs.portAllocator = pa
}

// Start starts the Redis session store
func (rs *RedisSessionStore) Start(ctx context.Context) error {
	go rs.heartbeatLoop(ctx)
	go rs.subscribeLoop(ctx)
	go rs.indexMaintenanceLoop(ctx)
	return rs.announceJoin(ctx)
}

// Stop stops the Redis session store
func (rs *RedisSessionStore) Stop(ctx context.Context) error {
	close(rs.stopCh)
	return rs.announceLeave(ctx)
}

// indexMaintenanceLoop periodically cleans up stale index entries
func (rs *RedisSessionStore) indexMaintenanceLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-rs.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			rs.cleanupStaleNodes()
		}
	}
}

// cleanupStaleNodes removes nodes that haven't sent heartbeats
func (rs *RedisSessionStore) cleanupStaleNodes() {
	rs.nodesMu.Lock()
	threshold := time.Now().Add(-60 * time.Second)
	var staleNodes []string
	for nodeID, lastSeen := range rs.clusterNodes {
		if lastSeen.Before(threshold) {
			staleNodes = append(staleNodes, nodeID)
		}
	}
	for _, nodeID := range staleNodes {
		delete(rs.clusterNodes, nodeID)
	}
	rs.nodesMu.Unlock()

	// Clean up index for stale nodes
	rs.indexMu.Lock()
	for _, nodeID := range staleNodes {
		delete(rs.nodeSessionIndex, nodeID)
	}
	rs.indexMu.Unlock()
}

// StoreSession stores a session in Redis with optimized indexing
func (rs *RedisSessionStore) StoreSession(ctx context.Context, session *MediaSession) error {
	data := rs.sessionToData(session)
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// Use pipeline for atomic multi-key operations
	key := rs.prefix + session.ID
	callIDKey := rs.prefix + "callid:" + session.CallID
	nodeIndexKey := rs.prefix + "node:" + rs.localNode

	// Try pipeline if available, fall back to individual ops
	if pipe := rs.client.Pipeline(ctx); pipe != nil {
		pipe.Set(key, jsonData, rs.ttl)
		pipe.Set(callIDKey, session.ID, rs.ttl)
		if err := pipe.Exec(ctx); err != nil {
			return fmt.Errorf("failed to store session: %w", err)
		}
	} else {
		if err := rs.client.Set(ctx, key, jsonData, rs.ttl); err != nil {
			return fmt.Errorf("failed to store session: %w", err)
		}
		if err := rs.client.Set(ctx, callIDKey, session.ID, rs.ttl); err != nil {
			return fmt.Errorf("failed to store call-id index: %w", err)
		}
	}

	// Update node->session index in Redis (O(1) lookup for takeover)
	rs.client.SAdd(ctx, nodeIndexKey, session.ID)

	// Update local index
	rs.indexMu.Lock()
	if rs.nodeSessionIndex[rs.localNode] == nil {
		rs.nodeSessionIndex[rs.localNode] = make(map[string]struct{})
	}
	rs.nodeSessionIndex[rs.localNode][session.ID] = struct{}{}
	rs.indexMu.Unlock()

	rs.stats.sessionsStored.Add(1)

	// Publish async - don't block on cluster notification
	go rs.publishSessionCreate(ctx, session.ID, session.CallID, jsonData)

	return nil
}

// publishSessionCreate publishes session creation asynchronously
func (rs *RedisSessionStore) publishSessionCreate(ctx context.Context, sessionID, callID string, jsonData []byte) {
	msg := rs.acquireMessage()
	msg.Type = MsgTypeSessionCreate
	msg.NodeID = rs.localNode
	msg.SessionID = sessionID
	msg.CallID = callID
	msg.Data = jsonData
	msg.Timestamp = time.Now()
	rs.publishMessage(ctx, msg)
	rs.releaseMessage(msg)
}

// acquireMessage gets a message from the pool
func (rs *RedisSessionStore) acquireMessage() *ClusterMessage {
	return rs.messagePool.Get().(*ClusterMessage)
}

// releaseMessage returns a message to the pool
func (rs *RedisSessionStore) releaseMessage(msg *ClusterMessage) {
	msg.Data = nil
	rs.messagePool.Put(msg)
}

// GetSession retrieves a session from Redis
func (rs *RedisSessionStore) GetSession(ctx context.Context, sessionID string) (*SessionData, error) {
	key := rs.prefix + sessionID
	jsonData, err := rs.client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	data := rs.acquireSession()
	if err := json.Unmarshal([]byte(jsonData), data); err != nil {
		rs.releaseSession(data)
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return data, nil
}

// acquireSession gets a session from the pool
func (rs *RedisSessionStore) acquireSession() *SessionData {
	return rs.sessionPool.Get().(*SessionData)
}

// releaseSession returns a session to the pool
func (rs *RedisSessionStore) releaseSession(s *SessionData) {
	s.Reset()
	rs.sessionPool.Put(s)
}

// GetSessionByCallID retrieves a session by call-id
func (rs *RedisSessionStore) GetSessionByCallID(ctx context.Context, callID string) (*SessionData, error) {
	callIDKey := rs.prefix + "callid:" + callID
	sessionID, err := rs.client.Get(ctx, callIDKey)
	if err != nil {
		return nil, fmt.Errorf("call not found: %s", callID)
	}
	return rs.GetSession(ctx, sessionID)
}

// GetSessionsBatch retrieves multiple sessions in a single round-trip
func (rs *RedisSessionStore) GetSessionsBatch(ctx context.Context, sessionIDs []string) ([]*SessionData, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}

	keys := make([]string, len(sessionIDs))
	for i, id := range sessionIDs {
		keys[i] = rs.prefix + id
	}

	values, err := rs.client.MGet(ctx, keys...)
	if err != nil {
		// Fall back to individual gets
		return rs.getSessionsIndividually(ctx, sessionIDs)
	}

	sessions := make([]*SessionData, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		session := rs.acquireSession()
		if err := json.Unmarshal([]byte(v), session); err != nil {
			rs.releaseSession(session)
			continue
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// getSessionsIndividually falls back to individual gets
func (rs *RedisSessionStore) getSessionsIndividually(ctx context.Context, sessionIDs []string) ([]*SessionData, error) {
	sessions := make([]*SessionData, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		session, err := rs.GetSession(ctx, id)
		if err != nil {
			continue
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

// UpdateSession updates a session in Redis
func (rs *RedisSessionStore) UpdateSession(ctx context.Context, session *MediaSession) error {
	data := rs.sessionToData(session)
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	key := rs.prefix + session.ID
	if err := rs.client.Set(ctx, key, jsonData, rs.ttl); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	// Async publish
	go func() {
		msg := rs.acquireMessage()
		msg.Type = MsgTypeSessionUpdate
		msg.NodeID = rs.localNode
		msg.SessionID = session.ID
		msg.CallID = session.CallID
		msg.Data = jsonData
		msg.Timestamp = time.Now()
		rs.publishMessage(ctx, msg)
		rs.releaseMessage(msg)
	}()

	return nil
}

// DeleteSession deletes a session from Redis
func (rs *RedisSessionStore) DeleteSession(ctx context.Context, sessionID, callID string) error {
	key := rs.prefix + sessionID
	callIDKey := rs.prefix + "callid:" + callID
	nodeIndexKey := rs.prefix + "node:" + rs.localNode

	// Batch delete
	if err := rs.client.Del(ctx, key, callIDKey); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// Remove from node index
	rs.client.SRem(ctx, nodeIndexKey, sessionID)

	// Update local index
	rs.indexMu.Lock()
	if rs.nodeSessionIndex[rs.localNode] != nil {
		delete(rs.nodeSessionIndex[rs.localNode], sessionID)
	}
	rs.indexMu.Unlock()

	rs.stats.sessionsDeleted.Add(1)

	// Async publish
	go func() {
		msg := rs.acquireMessage()
		msg.Type = MsgTypeSessionDelete
		msg.NodeID = rs.localNode
		msg.SessionID = sessionID
		msg.CallID = callID
		msg.Timestamp = time.Now()
		rs.publishMessage(ctx, msg)
		rs.releaseMessage(msg)
	}()

	return nil
}

// SessionExists checks if a session exists
func (rs *RedisSessionStore) SessionExists(ctx context.Context, sessionID string) (bool, error) {
	key := rs.prefix + sessionID
	count, err := rs.client.Exists(ctx, key)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// RefreshSession extends session TTL
func (rs *RedisSessionStore) RefreshSession(ctx context.Context, sessionID string) error {
	key := rs.prefix + sessionID
	return rs.client.Expire(ctx, key, rs.ttl)
}

// ListSessions lists all sessions using SCAN (non-blocking)
func (rs *RedisSessionStore) ListSessions(ctx context.Context) ([]string, error) {
	pattern := rs.prefix + "*"
	var sessions []string
	var cursor uint64

	for {
		keys, nextCursor, err := rs.client.Scan(ctx, cursor, pattern, 100)
		if err != nil {
			// Fall back to Keys if Scan not supported
			return rs.listSessionsLegacy(ctx)
		}

		for _, key := range keys {
			// Skip index keys
			suffix := key[len(rs.prefix):]
			if len(suffix) > 6 && suffix[:6] == "callid" {
				continue
			}
			if len(suffix) > 5 && suffix[:5] == "node:" {
				continue
			}
			sessions = append(sessions, suffix)
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return sessions, nil
}

// listSessionsLegacy uses Keys() as fallback
func (rs *RedisSessionStore) listSessionsLegacy(ctx context.Context) ([]string, error) {
	pattern := rs.prefix + "*"
	keys, err := rs.client.Keys(ctx, pattern)
	if err != nil {
		return nil, err
	}

	var sessions []string
	for _, key := range keys {
		suffix := key[len(rs.prefix):]
		if len(suffix) > 6 && suffix[:6] == "callid" {
			continue
		}
		if len(suffix) > 5 && suffix[:5] == "node:" {
			continue
		}
		sessions = append(sessions, suffix)
	}
	return sessions, nil
}

// TransferSession transfers a session to another node
func (rs *RedisSessionStore) TransferSession(ctx context.Context, sessionID, targetNode string) error {
	data, err := rs.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	oldNode := data.NodeID
	data.NodeID = targetNode
	data.UpdatedAt = time.Now()

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	key := rs.prefix + sessionID
	if err := rs.client.Set(ctx, key, jsonData, rs.ttl); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	// Update Redis indexes
	oldIndexKey := rs.prefix + "node:" + oldNode
	newIndexKey := rs.prefix + "node:" + targetNode
	rs.client.SRem(ctx, oldIndexKey, sessionID)
	rs.client.SAdd(ctx, newIndexKey, sessionID)

	// Update local index
	rs.indexMu.Lock()
	if rs.nodeSessionIndex[oldNode] != nil {
		delete(rs.nodeSessionIndex[oldNode], sessionID)
	}
	if rs.nodeSessionIndex[targetNode] == nil {
		rs.nodeSessionIndex[targetNode] = make(map[string]struct{})
	}
	rs.nodeSessionIndex[targetNode][sessionID] = struct{}{}
	rs.indexMu.Unlock()

	msg := rs.acquireMessage()
	msg.Type = MsgTypeSessionTransfer
	msg.NodeID = targetNode
	msg.SessionID = sessionID
	msg.Data = jsonData
	msg.Timestamp = time.Now()
	rs.publishMessage(ctx, msg)
	rs.releaseMessage(msg)

	return nil
}

// sessionToData converts a MediaSession to SessionData
func (rs *RedisSessionStore) sessionToData(session *MediaSession) *SessionData {
	session.RLock()
	defer session.RUnlock()

	data := &SessionData{
		ID:        session.ID,
		CallID:    session.CallID,
		FromTag:   session.FromTag,
		ToTag:     session.ToTag,
		ViaBranch: session.ViaBranch,
		State:     string(session.State),
		NodeID:    rs.localNode,
		CreatedAt: session.CreatedAt,
		UpdatedAt: session.UpdatedAt,
		Flags:     session.Flags,
		Metadata:  session.Metadata,
	}

	if session.CallerLeg != nil {
		data.CallerLeg = rs.legToData(session.CallerLeg)
	}
	if session.CalleeLeg != nil {
		data.CalleeLeg = rs.legToData(session.CalleeLeg)
	}

	return data
}

// legToData converts a CallLeg to LegData
func (rs *RedisSessionStore) legToData(leg *CallLeg) *LegData {
	return &LegData{
		Tag:           leg.Tag,
		Label:         leg.Label,
		IP:            leg.IP.String(),
		Port:          leg.Port,
		RTCPPort:      leg.RTCPPort,
		LocalIP:       leg.LocalIP.String(),
		LocalPort:     leg.LocalPort,
		LocalRTCPPort: leg.LocalRTCPPort,
		Interface:     leg.Interface,
		Direction:     leg.Direction,
	}
}

// publishMessage publishes a cluster message
func (rs *RedisSessionStore) publishMessage(ctx context.Context, msg *ClusterMessage) {
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		return
	}
	rs.client.Publish(ctx, "karl:cluster", jsonMsg)
}

// announceJoin announces node join
func (rs *RedisSessionStore) announceJoin(ctx context.Context) error {
	msg := ClusterMessage{
		Type:      MsgTypeNodeJoin,
		NodeID:    rs.localNode,
		Timestamp: time.Now(),
	}
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return rs.client.Publish(ctx, "karl:cluster", jsonMsg)
}

// announceLeave announces node leave
func (rs *RedisSessionStore) announceLeave(ctx context.Context) error {
	msg := ClusterMessage{
		Type:      MsgTypeNodeLeave,
		NodeID:    rs.localNode,
		Timestamp: time.Now(),
	}
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return rs.client.Publish(ctx, "karl:cluster", jsonMsg)
}

// heartbeatLoop sends periodic heartbeats
func (rs *RedisSessionStore) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(rs.syncInterval)
	defer ticker.Stop()

	msg := &ClusterMessage{
		Type:   MsgTypeHeartbeat,
		NodeID: rs.localNode,
	}

	for {
		select {
		case <-ticker.C:
			msg.Timestamp = time.Now()
			rs.publishMessage(ctx, msg)
		case <-rs.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// subscribeLoop subscribes to cluster messages
func (rs *RedisSessionStore) subscribeLoop(ctx context.Context) {
	pubsub, err := rs.client.Subscribe(ctx, "karl:cluster")
	if err != nil {
		return
	}
	defer pubsub.Close()

	for {
		select {
		case <-rs.stopCh:
			return
		case <-ctx.Done():
			return
		default:
			msg, err := pubsub.Receive(ctx)
			if err != nil {
				continue
			}
			rs.handleClusterMessage(msg)
		}
	}
}

// handleClusterMessage handles incoming cluster messages
func (rs *RedisSessionStore) handleClusterMessage(msg interface{}) {
	var payload string
	switch m := msg.(type) {
	case *redisMessage:
		payload = m.Payload
	case redisMessage:
		payload = m.Payload
	case string:
		payload = m
	default:
		return
	}

	if payload == "" {
		return
	}

	rs.stats.messagesProcessed.Add(1)

	var clusterMsg ClusterMessage
	if err := json.Unmarshal([]byte(payload), &clusterMsg); err != nil {
		return
	}

	if clusterMsg.NodeID == rs.localNode {
		return
	}

	switch clusterMsg.Type {
	case MsgTypeSessionCreate:
		rs.handleRemoteSessionCreate(clusterMsg)
	case MsgTypeSessionUpdate:
		rs.handleRemoteSessionUpdate(clusterMsg)
	case MsgTypeSessionDelete:
		rs.handleRemoteSessionDelete(clusterMsg)
	case MsgTypeSessionTransfer:
		rs.handleSessionTransfer(clusterMsg)
	case MsgTypeHeartbeat:
		rs.handleRemoteHeartbeat(clusterMsg)
	case MsgTypeNodeJoin:
		rs.handleNodeJoin(clusterMsg)
	case MsgTypeNodeLeave:
		rs.handleNodeLeave(clusterMsg)
	}
}

// redisMessage represents a Redis pubsub message
type redisMessage struct {
	Channel string
	Payload string
}

// handleRemoteSessionCreate handles session creation from another node
func (rs *RedisSessionStore) handleRemoteSessionCreate(msg ClusterMessage) {
	// Update local index for fast takeover
	rs.indexMu.Lock()
	if rs.nodeSessionIndex[msg.NodeID] == nil {
		rs.nodeSessionIndex[msg.NodeID] = make(map[string]struct{})
	}
	rs.nodeSessionIndex[msg.NodeID][msg.SessionID] = struct{}{}
	rs.indexMu.Unlock()
}

// handleRemoteSessionUpdate handles session update from another node
func (rs *RedisSessionStore) handleRemoteSessionUpdate(msg ClusterMessage) {
	// No-op for now, Redis is source of truth
}

// handleRemoteSessionDelete handles session deletion from another node
func (rs *RedisSessionStore) handleRemoteSessionDelete(msg ClusterMessage) {
	rs.indexMu.Lock()
	if rs.nodeSessionIndex[msg.NodeID] != nil {
		delete(rs.nodeSessionIndex[msg.NodeID], msg.SessionID)
	}
	rs.indexMu.Unlock()
}

// handleSessionTransfer handles session ownership transfer
func (rs *RedisSessionStore) handleSessionTransfer(msg ClusterMessage) {
	if len(msg.Data) == 0 {
		return
	}

	var session SessionData
	if err := json.Unmarshal(msg.Data, &session); err != nil {
		return
	}

	// Update local index
	rs.indexMu.Lock()
	// Remove from old node's index (we don't know old node, so check all)
	for nodeID, sessions := range rs.nodeSessionIndex {
		if nodeID != session.NodeID {
			delete(sessions, msg.SessionID)
		}
	}
	// Add to new node's index
	if rs.nodeSessionIndex[session.NodeID] == nil {
		rs.nodeSessionIndex[session.NodeID] = make(map[string]struct{})
	}
	rs.nodeSessionIndex[session.NodeID][msg.SessionID] = struct{}{}
	rs.indexMu.Unlock()

	if session.NodeID == rs.localNode {
		LogInfo("Cluster: session transferred to this node", map[string]interface{}{
			"session_id": msg.SessionID,
		})
	}
}

// handleRemoteHeartbeat handles heartbeat from another node
func (rs *RedisSessionStore) handleRemoteHeartbeat(msg ClusterMessage) {
	rs.nodesMu.Lock()
	rs.clusterNodes[msg.NodeID] = msg.Timestamp
	rs.nodesMu.Unlock()
}

// handleNodeJoin handles a new node joining the cluster
func (rs *RedisSessionStore) handleNodeJoin(msg ClusterMessage) {
	rs.nodesMu.Lock()
	rs.clusterNodes[msg.NodeID] = msg.Timestamp
	rs.nodesMu.Unlock()

	LogInfo("Cluster: node joined", map[string]interface{}{
		"node_id": msg.NodeID,
	})
}

// handleNodeLeave handles a node leaving the cluster
func (rs *RedisSessionStore) handleNodeLeave(msg ClusterMessage) {
	LogInfo("Cluster: node left", map[string]interface{}{
		"node_id": msg.NodeID,
	})

	rs.nodesMu.Lock()
	delete(rs.clusterNodes, msg.NodeID)
	rs.nodesMu.Unlock()

	// Trigger fast parallel session takeover
	go rs.takeoverSessionsFromNode(msg.NodeID)
}

// takeoverSessionsFromNode handles session takeover when a node leaves
func (rs *RedisSessionStore) takeoverSessionsFromNode(departedNodeID string) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get sessions from Redis set index (O(1) per session, not O(N) scan)
	sessionIDs, err := rs.getSessionIDsByNodeFast(ctx, departedNodeID)
	if err != nil {
		LogError("Failed to get sessions for takeover", map[string]interface{}{
			"departed_node": departedNodeID,
			"error":         err.Error(),
		})
		return
	}

	if len(sessionIDs) == 0 {
		return
	}

	LogInfo("Starting session takeover", map[string]interface{}{
		"departed_node": departedNodeID,
		"session_count": len(sessionIDs),
	})

	// Get active nodes
	activeNodes := rs.getActiveNodesSorted()

	// Batch fetch all sessions
	sessions, err := rs.GetSessionsBatch(ctx, sessionIDs)
	if err != nil {
		LogError("Failed to batch fetch sessions", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// Filter sessions that belong to this node via consistent hashing
	var myJobs []*takeoverJob
	for _, session := range sessions {
		if session.NodeID != departedNodeID {
			continue // Already taken over or wrong node
		}

		targetNode := rs.selectTakeoverNode(session.ID, activeNodes)
		if targetNode != rs.localNode {
			continue
		}

		job := &takeoverJob{
			session: session,
			ctx:     ctx,
			done:    make(chan error, 1),
		}
		myJobs = append(myJobs, job)
	}

	if len(myJobs) == 0 {
		return
	}

	// Submit all jobs to worker pool
	for _, job := range myJobs {
		select {
		case rs.takeoverCh <- job:
		case <-ctx.Done():
			return
		}
	}

	// Wait for all jobs to complete
	var succeeded, failed int
	for _, job := range myJobs {
		select {
		case err := <-job.done:
			if err != nil {
				failed++
				LogError("Session takeover failed", map[string]interface{}{
					"session_id": job.session.ID,
					"error":      err.Error(),
				})
			} else {
				succeeded++
			}
		case <-ctx.Done():
			return
		}
	}

	elapsed := time.Since(start)
	rs.stats.sessionsTakenOver.Add(int64(succeeded))
	rs.stats.takeoverLatencyNs.Store(elapsed.Nanoseconds())

	LogInfo("Session takeover complete", map[string]interface{}{
		"departed_node": departedNodeID,
		"succeeded":     succeeded,
		"failed":        failed,
		"duration_ms":   elapsed.Milliseconds(),
	})

	// Clean up departed node's index
	nodeIndexKey := rs.prefix + "node:" + departedNodeID
	rs.client.Del(ctx, nodeIndexKey)

	rs.indexMu.Lock()
	delete(rs.nodeSessionIndex, departedNodeID)
	rs.indexMu.Unlock()
}

// getSessionIDsByNodeFast uses Redis set for O(N) where N is sessions for that node
func (rs *RedisSessionStore) getSessionIDsByNodeFast(ctx context.Context, nodeID string) ([]string, error) {
	nodeIndexKey := rs.prefix + "node:" + nodeID

	// Try Redis set first
	sessionIDs, err := rs.client.SMembers(ctx, nodeIndexKey)
	if err == nil && len(sessionIDs) > 0 {
		return sessionIDs, nil
	}

	// Fall back to local index
	rs.indexMu.RLock()
	localIndex := rs.nodeSessionIndex[nodeID]
	if localIndex != nil {
		sessionIDs = make([]string, 0, len(localIndex))
		for id := range localIndex {
			sessionIDs = append(sessionIDs, id)
		}
	}
	rs.indexMu.RUnlock()

	if len(sessionIDs) > 0 {
		return sessionIDs, nil
	}

	// Last resort: scan Redis (expensive)
	return rs.scanSessionsByNode(ctx, nodeID)
}

// scanSessionsByNode scans for sessions owned by a node (fallback)
func (rs *RedisSessionStore) scanSessionsByNode(ctx context.Context, nodeID string) ([]string, error) {
	pattern := rs.prefix + "*"
	var sessionIDs []string
	var cursor uint64

	for {
		keys, nextCursor, err := rs.client.Scan(ctx, cursor, pattern, 500)
		if err != nil {
			// Fall back to Keys
			return rs.scanSessionsByNodeLegacy(ctx, nodeID)
		}

		for _, key := range keys {
			suffix := key[len(rs.prefix):]
			if len(suffix) > 6 && suffix[:6] == "callid" {
				continue
			}
			if len(suffix) > 5 && suffix[:5] == "node:" {
				continue
			}

			// Check ownership
			data, err := rs.client.Get(ctx, key)
			if err != nil {
				continue
			}
			var session SessionData
			if err := json.Unmarshal([]byte(data), &session); err != nil {
				continue
			}
			if session.NodeID == nodeID {
				sessionIDs = append(sessionIDs, session.ID)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return sessionIDs, nil
}

// scanSessionsByNodeLegacy uses Keys() as final fallback
func (rs *RedisSessionStore) scanSessionsByNodeLegacy(ctx context.Context, nodeID string) ([]string, error) {
	keys, err := rs.client.Keys(ctx, rs.prefix+"*")
	if err != nil {
		return nil, err
	}

	var sessionIDs []string
	for _, key := range keys {
		suffix := key[len(rs.prefix):]
		if len(suffix) > 6 && suffix[:6] == "callid" {
			continue
		}
		if len(suffix) > 5 && suffix[:5] == "node:" {
			continue
		}

		data, err := rs.client.Get(ctx, key)
		if err != nil {
			continue
		}
		var session SessionData
		if err := json.Unmarshal([]byte(data), &session); err != nil {
			continue
		}
		if session.NodeID == nodeID {
			sessionIDs = append(sessionIDs, session.ID)
		}
	}

	return sessionIDs, nil
}

// getActiveNodesSorted returns sorted list of active nodes
func (rs *RedisSessionStore) getActiveNodesSorted() []string {
	rs.nodesMu.RLock()
	threshold := time.Now().Add(-30 * time.Second)
	nodes := make([]string, 0, len(rs.clusterNodes)+1)
	nodes = append(nodes, rs.localNode)
	for nodeID, lastSeen := range rs.clusterNodes {
		if lastSeen.After(threshold) {
			nodes = append(nodes, nodeID)
		}
	}
	rs.nodesMu.RUnlock()

	sort.Strings(nodes)
	return nodes
}

// selectTakeoverNode uses consistent hashing to select which node should take over
func (rs *RedisSessionStore) selectTakeoverNode(sessionID string, nodes []string) string {
	if len(nodes) == 0 {
		return rs.localNode
	}
	if len(nodes) == 1 {
		return nodes[0]
	}

	hash := fnv1a(sessionID)
	return nodes[hash%uint32(len(nodes))]
}

// fnv1a computes FNV-1a hash (fast, good distribution)
func fnv1a(s string) uint32 {
	var hash uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		hash ^= uint32(s[i])
		hash *= 16777619
	}
	return hash
}

// executeSessionTakeover performs the actual session takeover
func (rs *RedisSessionStore) executeSessionTakeover(ctx context.Context, session *SessionData) error {
	// Pre-allocate ports before any Redis operations
	var callerRTP, callerRTCP, calleeRTP, calleeRTCP int
	var err error

	if session.CallerLeg != nil {
		callerRTP, callerRTCP, err = rs.portAllocator.AllocatePortPair(session.ID)
		if err != nil {
			return fmt.Errorf("failed to allocate caller ports: %w", err)
		}
	}

	if session.CalleeLeg != nil {
		calleeRTP, calleeRTCP, err = rs.portAllocator.AllocatePortPair(session.ID)
		if err != nil {
			if callerRTP > 0 {
				rs.portAllocator.ReleasePort(callerRTP)
				rs.portAllocator.ReleasePort(callerRTCP)
			}
			return fmt.Errorf("failed to allocate callee ports: %w", err)
		}
	}

	// Update session
	session.NodeID = rs.localNode
	session.UpdatedAt = time.Now()

	if session.CallerLeg != nil {
		session.CallerLeg.LocalPort = callerRTP
		session.CallerLeg.LocalRTCPPort = callerRTCP
	}
	if session.CalleeLeg != nil {
		session.CalleeLeg.LocalPort = calleeRTP
		session.CalleeLeg.LocalRTCPPort = calleeRTCP
	}

	jsonData, err := json.Marshal(session)
	if err != nil {
		rs.releasePortsOnError(callerRTP, callerRTCP, calleeRTP, calleeRTCP)
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// Atomic update in Redis
	key := rs.prefix + session.ID
	if err := rs.client.Set(ctx, key, jsonData, rs.ttl); err != nil {
		rs.releasePortsOnError(callerRTP, callerRTCP, calleeRTP, calleeRTCP)
		return fmt.Errorf("failed to store session: %w", err)
	}

	// Update Redis index
	nodeIndexKey := rs.prefix + "node:" + rs.localNode
	rs.client.SAdd(ctx, nodeIndexKey, session.ID)

	// Update local index
	rs.indexMu.Lock()
	if rs.nodeSessionIndex[rs.localNode] == nil {
		rs.nodeSessionIndex[rs.localNode] = make(map[string]struct{})
	}
	rs.nodeSessionIndex[rs.localNode][session.ID] = struct{}{}
	rs.indexMu.Unlock()

	return nil
}

// releasePortsOnError releases allocated ports on failure
func (rs *RedisSessionStore) releasePortsOnError(ports ...int) {
	for _, p := range ports {
		if p > 0 {
			rs.portAllocator.ReleasePort(p)
		}
	}
}

// GetNodeID returns the local node ID
func (rs *RedisSessionStore) GetNodeID() string {
	return rs.localNode
}

// GetStats returns cluster statistics
func (rs *RedisSessionStore) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"sessions_stored":     rs.stats.sessionsStored.Load(),
		"sessions_deleted":    rs.stats.sessionsDeleted.Load(),
		"sessions_taken_over": rs.stats.sessionsTakenOver.Load(),
		"takeover_latency_ns": rs.stats.takeoverLatencyNs.Load(),
		"messages_processed":  rs.stats.messagesProcessed.Load(),
		"worker_count":        rs.workerCount,
	}
}
