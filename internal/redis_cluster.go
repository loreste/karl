package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// RedisSessionStore provides Redis-backed session storage for clustering
type RedisSessionStore struct {
	client       RedisClient
	prefix       string
	localNode    string
	ttl          time.Duration
	syncInterval time.Duration
	mu           sync.RWMutex
	stopCh       chan struct{}
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
	store := &RedisSessionStore{
		client:       client,
		prefix:       "karl:session:",
		localNode:    nodeID,
		ttl:          ttl,
		syncInterval: 5 * time.Second,
		stopCh:       make(chan struct{}),
	}
	return store
}

// Start starts the Redis session store
func (rs *RedisSessionStore) Start(ctx context.Context) error {
	// Start heartbeat
	go rs.heartbeatLoop(ctx)

	// Subscribe to cluster channel
	go rs.subscribeLoop(ctx)

	// Announce node join
	return rs.announceJoin(ctx)
}

// Stop stops the Redis session store
func (rs *RedisSessionStore) Stop(ctx context.Context) error {
	close(rs.stopCh)
	return rs.announceLeave(ctx)
}

// StoreSession stores a session in Redis
func (rs *RedisSessionStore) StoreSession(ctx context.Context, session *MediaSession) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	data := rs.sessionToData(session)
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// Store session by ID
	key := rs.prefix + session.ID
	if err := rs.client.Set(ctx, key, jsonData, rs.ttl); err != nil {
		return fmt.Errorf("failed to store session: %w", err)
	}

	// Store call-id index
	callIDKey := rs.prefix + "callid:" + session.CallID
	if err := rs.client.Set(ctx, callIDKey, session.ID, rs.ttl); err != nil {
		return fmt.Errorf("failed to store call-id index: %w", err)
	}

	// Publish update message
	msg := ClusterMessage{
		Type:      MsgTypeSessionCreate,
		NodeID:    rs.localNode,
		SessionID: session.ID,
		CallID:    session.CallID,
		Data:      jsonData,
		Timestamp: time.Now(),
	}
	rs.publishMessage(ctx, msg)

	return nil
}

// GetSession retrieves a session from Redis
func (rs *RedisSessionStore) GetSession(ctx context.Context, sessionID string) (*SessionData, error) {
	key := rs.prefix + sessionID
	jsonData, err := rs.client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	var data SessionData
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &data, nil
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

// UpdateSession updates a session in Redis
func (rs *RedisSessionStore) UpdateSession(ctx context.Context, session *MediaSession) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	data := rs.sessionToData(session)
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	key := rs.prefix + session.ID
	if err := rs.client.Set(ctx, key, jsonData, rs.ttl); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	// Publish update message
	msg := ClusterMessage{
		Type:      MsgTypeSessionUpdate,
		NodeID:    rs.localNode,
		SessionID: session.ID,
		CallID:    session.CallID,
		Data:      jsonData,
		Timestamp: time.Now(),
	}
	rs.publishMessage(ctx, msg)

	return nil
}

// DeleteSession deletes a session from Redis
func (rs *RedisSessionStore) DeleteSession(ctx context.Context, sessionID, callID string) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	key := rs.prefix + sessionID
	if err := rs.client.Del(ctx, key); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// Delete call-id index
	callIDKey := rs.prefix + "callid:" + callID
	rs.client.Del(ctx, callIDKey)

	// Publish delete message
	msg := ClusterMessage{
		Type:      MsgTypeSessionDelete,
		NodeID:    rs.localNode,
		SessionID: sessionID,
		CallID:    callID,
		Timestamp: time.Now(),
	}
	rs.publishMessage(ctx, msg)

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

// ListSessions lists all sessions
func (rs *RedisSessionStore) ListSessions(ctx context.Context) ([]string, error) {
	pattern := rs.prefix + "*"
	keys, err := rs.client.Keys(ctx, pattern)
	if err != nil {
		return nil, err
	}

	// Filter out index keys
	var sessions []string
	for _, key := range keys {
		if len(key) > len(rs.prefix) && key[len(rs.prefix):len(rs.prefix)+6] != "callid" {
			sessions = append(sessions, key[len(rs.prefix):])
		}
	}
	return sessions, nil
}

// TransferSession transfers a session to another node
func (rs *RedisSessionStore) TransferSession(ctx context.Context, sessionID, targetNode string) error {
	data, err := rs.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

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

	// Publish transfer message
	msg := ClusterMessage{
		Type:      MsgTypeSessionTransfer,
		NodeID:    targetNode,
		SessionID: sessionID,
		Data:      jsonData,
		Timestamp: time.Now(),
	}
	rs.publishMessage(ctx, msg)

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
func (rs *RedisSessionStore) publishMessage(ctx context.Context, msg ClusterMessage) {
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

	for {
		select {
		case <-ticker.C:
			msg := ClusterMessage{
				Type:      MsgTypeHeartbeat,
				NodeID:    rs.localNode,
				Timestamp: time.Now(),
			}
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
	// Message handling would be implemented here
	// For now, this is a placeholder
}

// GetNodeID returns the local node ID
func (rs *RedisSessionStore) GetNodeID() string {
	return rs.localNode
}
