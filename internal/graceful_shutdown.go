package internal

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// DrainState represents the current drain state
type DrainState int32

const (
	DrainStateNormal DrainState = iota
	DrainStateDraining
	DrainStateDrained
)

func (s DrainState) String() string {
	switch s {
	case DrainStateNormal:
		return "normal"
	case DrainStateDraining:
		return "draining"
	case DrainStateDrained:
		return "drained"
	default:
		return "unknown"
	}
}

// GracefulShutdownConfig holds shutdown configuration
type GracefulShutdownConfig struct {
	DrainTimeout       time.Duration // Max time to wait for connections to drain
	ShutdownTimeout    time.Duration // Max time for shutdown after drain
	HealthCheckGrace   time.Duration // Time to wait after marking unhealthy
	NewConnRejectDelay time.Duration // Delay before rejecting new connections
}

// DefaultGracefulShutdownConfig returns sensible defaults
func DefaultGracefulShutdownConfig() *GracefulShutdownConfig {
	return &GracefulShutdownConfig{
		DrainTimeout:       30 * time.Second,
		ShutdownTimeout:    10 * time.Second,
		HealthCheckGrace:   5 * time.Second,
		NewConnRejectDelay: 1 * time.Second,
	}
}

// GracefulShutdownManager manages graceful shutdown and drain operations
type GracefulShutdownManager struct {
	config        *GracefulShutdownConfig
	state         atomic.Int32
	activeConns   atomic.Int64
	drainStart    time.Time
	shutdownStart time.Time
	callbacks     []ShutdownCallback
	drainHooks    []DrainHook
	mu            sync.RWMutex
	drainCh       chan struct{}
	shutdownCh    chan struct{}
	doneCh        chan struct{}
}

// ShutdownCallback is called during shutdown
type ShutdownCallback struct {
	Name     string
	Priority int // Lower priority runs first
	Callback func(ctx context.Context) error
}

// DrainHook is called when drain starts
type DrainHook struct {
	Name string
	Hook func() error
}

// NewGracefulShutdownManager creates a new shutdown manager
func NewGracefulShutdownManager(config *GracefulShutdownConfig) *GracefulShutdownManager {
	if config == nil {
		config = DefaultGracefulShutdownConfig()
	}

	return &GracefulShutdownManager{
		config:     config,
		callbacks:  make([]ShutdownCallback, 0),
		drainHooks: make([]DrainHook, 0),
		drainCh:    make(chan struct{}),
		shutdownCh: make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
}

// RegisterShutdownCallback registers a callback to be called during shutdown
func (m *GracefulShutdownManager) RegisterShutdownCallback(name string, priority int, callback func(ctx context.Context) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callbacks = append(m.callbacks, ShutdownCallback{
		Name:     name,
		Priority: priority,
		Callback: callback,
	})

	// Sort by priority (lower first)
	for i := len(m.callbacks) - 1; i > 0; i-- {
		if m.callbacks[i].Priority < m.callbacks[i-1].Priority {
			m.callbacks[i], m.callbacks[i-1] = m.callbacks[i-1], m.callbacks[i]
		}
	}
}

// RegisterDrainHook registers a hook to be called when drain starts
func (m *GracefulShutdownManager) RegisterDrainHook(name string, hook func() error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drainHooks = append(m.drainHooks, DrainHook{Name: name, Hook: hook})
}

// IncrementConnections increments active connection count
func (m *GracefulShutdownManager) IncrementConnections() bool {
	state := DrainState(m.state.Load())
	if state != DrainStateNormal {
		return false // Reject new connections during drain
	}
	m.activeConns.Add(1)
	return true
}

// DecrementConnections decrements active connection count
func (m *GracefulShutdownManager) DecrementConnections() {
	m.activeConns.Add(-1)
}

// GetActiveConnections returns the current active connection count
func (m *GracefulShutdownManager) GetActiveConnections() int64 {
	return m.activeConns.Load()
}

// GetState returns the current drain state
func (m *GracefulShutdownManager) GetState() DrainState {
	return DrainState(m.state.Load())
}

// IsDraining returns true if the server is draining
func (m *GracefulShutdownManager) IsDraining() bool {
	state := DrainState(m.state.Load())
	return state == DrainStateDraining || state == DrainStateDrained
}

// IsAcceptingConnections returns true if new connections are accepted
func (m *GracefulShutdownManager) IsAcceptingConnections() bool {
	return DrainState(m.state.Load()) == DrainStateNormal
}

// StartDrain initiates the drain process
func (m *GracefulShutdownManager) StartDrain() error {
	if !m.state.CompareAndSwap(int32(DrainStateNormal), int32(DrainStateDraining)) {
		return fmt.Errorf("already draining or drained")
	}

	m.mu.Lock()
	m.drainStart = time.Now()
	drainHooks := make([]DrainHook, len(m.drainHooks))
	copy(drainHooks, m.drainHooks)
	m.mu.Unlock()

	log.Printf("Starting graceful drain, active connections: %d", m.activeConns.Load())

	// Execute drain hooks
	for _, hook := range drainHooks {
		if err := hook.Hook(); err != nil {
			log.Printf("Drain hook %s failed: %v", hook.Name, err)
		}
	}

	// Wait for health check grace period
	time.Sleep(m.config.HealthCheckGrace)

	// Start drain monitor
	go m.drainMonitor()

	return nil
}

// drainMonitor monitors the drain process
func (m *GracefulShutdownManager) drainMonitor() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(m.config.DrainTimeout)

	for {
		select {
		case <-ticker.C:
			conns := m.activeConns.Load()
			if conns <= 0 {
				log.Println("All connections drained")
				m.state.Store(int32(DrainStateDrained))
				close(m.drainCh)
				return
			}
			log.Printf("Draining... %d connections remaining", conns)

		case <-timeout:
			conns := m.activeConns.Load()
			log.Printf("Drain timeout reached with %d connections remaining", conns)
			m.state.Store(int32(DrainStateDrained))
			close(m.drainCh)
			return
		}
	}
}

// WaitForDrain waits for the drain to complete
func (m *GracefulShutdownManager) WaitForDrain() <-chan struct{} {
	return m.drainCh
}

// Shutdown performs a complete graceful shutdown
func (m *GracefulShutdownManager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	m.shutdownStart = time.Now()
	m.mu.Unlock()

	log.Println("Starting graceful shutdown...")

	// Start drain if not already started
	if DrainState(m.state.Load()) == DrainStateNormal {
		if err := m.StartDrain(); err != nil {
			log.Printf("Failed to start drain: %v", err)
		}
	}

	// Wait for drain with timeout
	select {
	case <-m.drainCh:
		log.Println("Drain completed")
	case <-ctx.Done():
		log.Println("Shutdown context cancelled during drain")
	}

	// Execute shutdown callbacks
	m.mu.RLock()
	callbacks := make([]ShutdownCallback, len(m.callbacks))
	copy(callbacks, m.callbacks)
	m.mu.RUnlock()

	var lastErr error
	for _, cb := range callbacks {
		cbCtx, cancel := context.WithTimeout(ctx, m.config.ShutdownTimeout)
		if err := cb.Callback(cbCtx); err != nil {
			log.Printf("Shutdown callback %s failed: %v", cb.Name, err)
			lastErr = err
		}
		cancel()
	}

	close(m.doneCh)
	log.Println("Graceful shutdown completed")
	return lastErr
}

// Done returns a channel that's closed when shutdown is complete
func (m *GracefulShutdownManager) Done() <-chan struct{} {
	return m.doneCh
}

// GetStats returns shutdown statistics
func (m *GracefulShutdownManager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := map[string]interface{}{
		"state":              DrainState(m.state.Load()).String(),
		"active_connections": m.activeConns.Load(),
		"callbacks_count":    len(m.callbacks),
		"drain_hooks_count":  len(m.drainHooks),
	}

	if !m.drainStart.IsZero() {
		stats["drain_duration"] = time.Since(m.drainStart).String()
	}
	if !m.shutdownStart.IsZero() {
		stats["shutdown_duration"] = time.Since(m.shutdownStart).String()
	}

	return stats
}

// ConnectionTracker wraps a connection to track it with the shutdown manager
type ConnectionTracker struct {
	manager *GracefulShutdownManager
	tracked bool
	mu      sync.Mutex
}

// NewConnectionTracker creates a new connection tracker
func NewConnectionTracker(manager *GracefulShutdownManager) *ConnectionTracker {
	return &ConnectionTracker{
		manager: manager,
		tracked: false,
	}
}

// Track starts tracking this connection
func (ct *ConnectionTracker) Track() bool {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if ct.tracked {
		return true
	}

	if ct.manager.IncrementConnections() {
		ct.tracked = true
		return true
	}
	return false
}

// Untrack stops tracking this connection
func (ct *ConnectionTracker) Untrack() {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if ct.tracked {
		ct.manager.DecrementConnections()
		ct.tracked = false
	}
}

// SessionDrainManager manages session-level draining
type SessionDrainManager struct {
	shutdownMgr *GracefulShutdownManager
	sessions    map[string]*DrainableSession
	mu          sync.RWMutex
}

// DrainableSession represents a session that can be drained
type DrainableSession struct {
	ID          string
	StartTime   time.Time
	LastActive  time.Time
	Draining    bool
	DrainStart  time.Time
	Connections int
}

// NewSessionDrainManager creates a new session drain manager
func NewSessionDrainManager(shutdownMgr *GracefulShutdownManager) *SessionDrainManager {
	return &SessionDrainManager{
		shutdownMgr: shutdownMgr,
		sessions:    make(map[string]*DrainableSession),
	}
}

// RegisterSession registers a session for drain tracking
func (sdm *SessionDrainManager) RegisterSession(sessionID string) {
	sdm.mu.Lock()
	defer sdm.mu.Unlock()

	sdm.sessions[sessionID] = &DrainableSession{
		ID:         sessionID,
		StartTime:  time.Now(),
		LastActive: time.Now(),
	}
	sdm.shutdownMgr.IncrementConnections()
}

// UnregisterSession removes a session from drain tracking
func (sdm *SessionDrainManager) UnregisterSession(sessionID string) {
	sdm.mu.Lock()
	defer sdm.mu.Unlock()

	if _, exists := sdm.sessions[sessionID]; exists {
		delete(sdm.sessions, sessionID)
		sdm.shutdownMgr.DecrementConnections()
	}
}

// UpdateActivity updates the last active time for a session
func (sdm *SessionDrainManager) UpdateActivity(sessionID string) {
	sdm.mu.Lock()
	defer sdm.mu.Unlock()

	if session, exists := sdm.sessions[sessionID]; exists {
		session.LastActive = time.Now()
	}
}

// MarkDraining marks a session as draining
func (sdm *SessionDrainManager) MarkDraining(sessionID string) {
	sdm.mu.Lock()
	defer sdm.mu.Unlock()

	if session, exists := sdm.sessions[sessionID]; exists {
		session.Draining = true
		session.DrainStart = time.Now()
	}
}

// GetActiveSessions returns the number of active sessions
func (sdm *SessionDrainManager) GetActiveSessions() int {
	sdm.mu.RLock()
	defer sdm.mu.RUnlock()
	return len(sdm.sessions)
}

// GetDrainingSessions returns the number of draining sessions
func (sdm *SessionDrainManager) GetDrainingSessions() int {
	sdm.mu.RLock()
	defer sdm.mu.RUnlock()

	count := 0
	for _, session := range sdm.sessions {
		if session.Draining {
			count++
		}
	}
	return count
}

// DrainAllSessions marks all sessions as draining
func (sdm *SessionDrainManager) DrainAllSessions() {
	sdm.mu.Lock()
	defer sdm.mu.Unlock()

	now := time.Now()
	for _, session := range sdm.sessions {
		if !session.Draining {
			session.Draining = true
			session.DrainStart = now
		}
	}
}

// GetStats returns session drain statistics
func (sdm *SessionDrainManager) GetStats() map[string]interface{} {
	sdm.mu.RLock()
	defer sdm.mu.RUnlock()

	draining := 0
	oldestSession := time.Duration(0)
	now := time.Now()

	for _, session := range sdm.sessions {
		if session.Draining {
			draining++
		}
		age := now.Sub(session.StartTime)
		if age > oldestSession {
			oldestSession = age
		}
	}

	return map[string]interface{}{
		"total_sessions":    len(sdm.sessions),
		"draining_sessions": draining,
		"oldest_session":    oldestSession.String(),
	}
}

// Global shutdown manager
var (
	globalShutdownManager     *GracefulShutdownManager
	globalShutdownManagerOnce sync.Once
)

// GetShutdownManager returns the global shutdown manager
func GetShutdownManager() *GracefulShutdownManager {
	globalShutdownManagerOnce.Do(func() {
		globalShutdownManager = NewGracefulShutdownManager(DefaultGracefulShutdownConfig())
	})
	return globalShutdownManager
}
