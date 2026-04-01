package internal

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestDrainState_String(t *testing.T) {
	tests := []struct {
		state    DrainState
		expected string
	}{
		{DrainStateNormal, "normal"},
		{DrainStateDraining, "draining"},
		{DrainStateDrained, "drained"},
		{DrainState(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("DrainState(%d).String() = %s, expected %s", tt.state, got, tt.expected)
		}
	}
}

func TestDefaultGracefulShutdownConfig(t *testing.T) {
	config := DefaultGracefulShutdownConfig()

	if config.DrainTimeout != 30*time.Second {
		t.Errorf("Expected DrainTimeout 30s, got %v", config.DrainTimeout)
	}
	if config.ShutdownTimeout != 10*time.Second {
		t.Errorf("Expected ShutdownTimeout 10s, got %v", config.ShutdownTimeout)
	}
	if config.HealthCheckGrace != 5*time.Second {
		t.Errorf("Expected HealthCheckGrace 5s, got %v", config.HealthCheckGrace)
	}
}

func TestNewGracefulShutdownManager(t *testing.T) {
	manager := NewGracefulShutdownManager(nil)

	if manager == nil {
		t.Fatal("NewGracefulShutdownManager returned nil")
	}
	if manager.config == nil {
		t.Error("config should not be nil")
	}
	if manager.GetState() != DrainStateNormal {
		t.Error("Initial state should be Normal")
	}
}

func TestGracefulShutdownManager_ConnectionTracking(t *testing.T) {
	config := &GracefulShutdownConfig{
		DrainTimeout:       1 * time.Second,
		ShutdownTimeout:    1 * time.Second,
		HealthCheckGrace:   0,
		NewConnRejectDelay: 0,
	}
	manager := NewGracefulShutdownManager(config)

	// Initially should accept connections
	if !manager.IsAcceptingConnections() {
		t.Error("Should accept connections initially")
	}

	// Increment connections
	if !manager.IncrementConnections() {
		t.Error("IncrementConnections should return true")
	}
	if manager.GetActiveConnections() != 1 {
		t.Errorf("Expected 1 active connection, got %d", manager.GetActiveConnections())
	}

	// Decrement connections
	manager.DecrementConnections()
	if manager.GetActiveConnections() != 0 {
		t.Errorf("Expected 0 active connections, got %d", manager.GetActiveConnections())
	}
}

func TestGracefulShutdownManager_ConnectionRejectionDuringDrain(t *testing.T) {
	config := &GracefulShutdownConfig{
		DrainTimeout:       1 * time.Second,
		ShutdownTimeout:    1 * time.Second,
		HealthCheckGrace:   0,
		NewConnRejectDelay: 0,
	}
	manager := NewGracefulShutdownManager(config)

	// Start draining
	err := manager.StartDrain()
	if err != nil {
		t.Fatalf("StartDrain failed: %v", err)
	}

	// Should reject new connections
	if manager.IncrementConnections() {
		t.Error("Should reject connections during drain")
	}

	if !manager.IsDraining() {
		t.Error("Should be draining")
	}

	if manager.IsAcceptingConnections() {
		t.Error("Should not accept connections during drain")
	}
}

func TestGracefulShutdownManager_DrainWithNoConnections(t *testing.T) {
	config := &GracefulShutdownConfig{
		DrainTimeout:       1 * time.Second,
		ShutdownTimeout:    1 * time.Second,
		HealthCheckGrace:   0,
		NewConnRejectDelay: 0,
	}
	manager := NewGracefulShutdownManager(config)

	err := manager.StartDrain()
	if err != nil {
		t.Fatalf("StartDrain failed: %v", err)
	}

	// Should complete quickly with no connections
	select {
	case <-manager.WaitForDrain():
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Drain should complete quickly with no connections")
	}

	if manager.GetState() != DrainStateDrained {
		t.Errorf("Expected state Drained, got %s", manager.GetState().String())
	}
}

func TestGracefulShutdownManager_DrainWithConnections(t *testing.T) {
	config := &GracefulShutdownConfig{
		DrainTimeout:       2 * time.Second,
		ShutdownTimeout:    1 * time.Second,
		HealthCheckGrace:   0,
		NewConnRejectDelay: 0,
	}
	manager := NewGracefulShutdownManager(config)

	// Add a connection
	manager.IncrementConnections()

	err := manager.StartDrain()
	if err != nil {
		t.Fatalf("StartDrain failed: %v", err)
	}

	// Remove connection after a short delay
	go func() {
		time.Sleep(500 * time.Millisecond)
		manager.DecrementConnections()
	}()

	// Should complete when connection is removed
	select {
	case <-manager.WaitForDrain():
		// Success
	case <-time.After(3 * time.Second):
		t.Error("Drain should complete when connections are removed")
	}
}

func TestGracefulShutdownManager_DrainTimeout(t *testing.T) {
	config := &GracefulShutdownConfig{
		DrainTimeout:       500 * time.Millisecond,
		ShutdownTimeout:    1 * time.Second,
		HealthCheckGrace:   0,
		NewConnRejectDelay: 0,
	}
	manager := NewGracefulShutdownManager(config)

	// Add a connection that won't be removed
	manager.IncrementConnections()

	err := manager.StartDrain()
	if err != nil {
		t.Fatalf("StartDrain failed: %v", err)
	}

	// Should timeout
	select {
	case <-manager.WaitForDrain():
		// Success - timeout reached
	case <-time.After(2 * time.Second):
		t.Error("Drain should timeout")
	}

	if manager.GetState() != DrainStateDrained {
		t.Errorf("Expected state Drained after timeout, got %s", manager.GetState().String())
	}
}

func TestGracefulShutdownManager_RegisterShutdownCallback(t *testing.T) {
	manager := NewGracefulShutdownManager(nil)

	callOrder := make([]int, 0)
	var mu sync.Mutex

	// Register callbacks with different priorities
	manager.RegisterShutdownCallback("high", 10, func(ctx context.Context) error {
		mu.Lock()
		callOrder = append(callOrder, 10)
		mu.Unlock()
		return nil
	})

	manager.RegisterShutdownCallback("low", 1, func(ctx context.Context) error {
		mu.Lock()
		callOrder = append(callOrder, 1)
		mu.Unlock()
		return nil
	})

	manager.RegisterShutdownCallback("medium", 5, func(ctx context.Context) error {
		mu.Lock()
		callOrder = append(callOrder, 5)
		mu.Unlock()
		return nil
	})

	// Trigger shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	manager.Shutdown(ctx)

	// Verify callbacks were called in priority order
	if len(callOrder) != 3 {
		t.Fatalf("Expected 3 callbacks, got %d", len(callOrder))
	}
	if callOrder[0] != 1 || callOrder[1] != 5 || callOrder[2] != 10 {
		t.Errorf("Callbacks not called in priority order: %v", callOrder)
	}
}

func TestGracefulShutdownManager_RegisterDrainHook(t *testing.T) {
	config := &GracefulShutdownConfig{
		DrainTimeout:       1 * time.Second,
		ShutdownTimeout:    1 * time.Second,
		HealthCheckGrace:   0,
		NewConnRejectDelay: 0,
	}
	manager := NewGracefulShutdownManager(config)

	hookCalled := false
	manager.RegisterDrainHook("test-hook", func() error {
		hookCalled = true
		return nil
	})

	manager.StartDrain()

	// Wait for drain
	<-manager.WaitForDrain()

	if !hookCalled {
		t.Error("Drain hook should have been called")
	}
}

func TestGracefulShutdownManager_GetStats(t *testing.T) {
	manager := NewGracefulShutdownManager(nil)

	manager.IncrementConnections()
	manager.RegisterShutdownCallback("test", 1, func(ctx context.Context) error { return nil })
	manager.RegisterDrainHook("test-hook", func() error { return nil })

	stats := manager.GetStats()

	if stats["state"] != "normal" {
		t.Errorf("Expected state 'normal', got %v", stats["state"])
	}
	if stats["active_connections"].(int64) != 1 {
		t.Errorf("Expected 1 active connection, got %v", stats["active_connections"])
	}
	if stats["callbacks_count"].(int) != 1 {
		t.Errorf("Expected 1 callback, got %v", stats["callbacks_count"])
	}
	if stats["drain_hooks_count"].(int) != 1 {
		t.Errorf("Expected 1 drain hook, got %v", stats["drain_hooks_count"])
	}
}

func TestGracefulShutdownManager_DoubleStart(t *testing.T) {
	config := &GracefulShutdownConfig{
		DrainTimeout:       1 * time.Second,
		ShutdownTimeout:    1 * time.Second,
		HealthCheckGrace:   0,
		NewConnRejectDelay: 0,
	}
	manager := NewGracefulShutdownManager(config)

	// First drain should succeed
	err := manager.StartDrain()
	if err != nil {
		t.Fatalf("First StartDrain failed: %v", err)
	}

	// Second drain should fail
	err = manager.StartDrain()
	if err == nil {
		t.Error("Second StartDrain should return error")
	}
}

func TestConnectionTracker(t *testing.T) {
	manager := NewGracefulShutdownManager(nil)
	tracker := NewConnectionTracker(manager)

	// Track connection
	if !tracker.Track() {
		t.Error("Track should return true")
	}
	if manager.GetActiveConnections() != 1 {
		t.Error("Should have 1 active connection")
	}

	// Double track should return true
	if !tracker.Track() {
		t.Error("Double Track should return true")
	}
	if manager.GetActiveConnections() != 1 {
		t.Error("Should still have 1 active connection (no double count)")
	}

	// Untrack
	tracker.Untrack()
	if manager.GetActiveConnections() != 0 {
		t.Error("Should have 0 active connections after untrack")
	}

	// Double untrack should be safe
	tracker.Untrack()
	if manager.GetActiveConnections() != 0 {
		t.Error("Should still have 0 active connections")
	}
}

func TestConnectionTracker_RejectDuringDrain(t *testing.T) {
	config := &GracefulShutdownConfig{
		DrainTimeout:       1 * time.Second,
		ShutdownTimeout:    1 * time.Second,
		HealthCheckGrace:   0,
		NewConnRejectDelay: 0,
	}
	manager := NewGracefulShutdownManager(config)
	tracker := NewConnectionTracker(manager)

	manager.StartDrain()

	if tracker.Track() {
		t.Error("Track should return false during drain")
	}
}

func TestSessionDrainManager(t *testing.T) {
	shutdownMgr := NewGracefulShutdownManager(nil)
	sessionMgr := NewSessionDrainManager(shutdownMgr)

	// Register session
	sessionMgr.RegisterSession("session-1")
	if sessionMgr.GetActiveSessions() != 1 {
		t.Error("Should have 1 active session")
	}
	if shutdownMgr.GetActiveConnections() != 1 {
		t.Error("Shutdown manager should have 1 connection")
	}

	// Update activity
	sessionMgr.UpdateActivity("session-1")

	// Mark draining
	sessionMgr.MarkDraining("session-1")
	if sessionMgr.GetDrainingSessions() != 1 {
		t.Error("Should have 1 draining session")
	}

	// Unregister
	sessionMgr.UnregisterSession("session-1")
	if sessionMgr.GetActiveSessions() != 0 {
		t.Error("Should have 0 active sessions")
	}
	if shutdownMgr.GetActiveConnections() != 0 {
		t.Error("Shutdown manager should have 0 connections")
	}
}

func TestSessionDrainManager_DrainAllSessions(t *testing.T) {
	shutdownMgr := NewGracefulShutdownManager(nil)
	sessionMgr := NewSessionDrainManager(shutdownMgr)

	sessionMgr.RegisterSession("session-1")
	sessionMgr.RegisterSession("session-2")
	sessionMgr.RegisterSession("session-3")

	if sessionMgr.GetDrainingSessions() != 0 {
		t.Error("No sessions should be draining initially")
	}

	sessionMgr.DrainAllSessions()

	if sessionMgr.GetDrainingSessions() != 3 {
		t.Errorf("All 3 sessions should be draining, got %d", sessionMgr.GetDrainingSessions())
	}
}

func TestSessionDrainManager_GetStats(t *testing.T) {
	shutdownMgr := NewGracefulShutdownManager(nil)
	sessionMgr := NewSessionDrainManager(shutdownMgr)

	sessionMgr.RegisterSession("session-1")
	sessionMgr.RegisterSession("session-2")
	sessionMgr.MarkDraining("session-1")

	stats := sessionMgr.GetStats()

	if stats["total_sessions"].(int) != 2 {
		t.Errorf("Expected 2 total sessions, got %v", stats["total_sessions"])
	}
	if stats["draining_sessions"].(int) != 1 {
		t.Errorf("Expected 1 draining session, got %v", stats["draining_sessions"])
	}
}

func TestGracefulShutdownManager_Shutdown(t *testing.T) {
	config := &GracefulShutdownConfig{
		DrainTimeout:       500 * time.Millisecond,
		ShutdownTimeout:    500 * time.Millisecond,
		HealthCheckGrace:   0,
		NewConnRejectDelay: 0,
	}
	manager := NewGracefulShutdownManager(config)

	callbackCalled := false
	manager.RegisterShutdownCallback("test", 1, func(ctx context.Context) error {
		callbackCalled = true
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := manager.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown returned error: %v", err)
	}

	if !callbackCalled {
		t.Error("Shutdown callback should have been called")
	}

	// Done channel should be closed
	select {
	case <-manager.Done():
		// Success
	default:
		t.Error("Done channel should be closed after shutdown")
	}
}

func TestGracefulShutdownManager_ConcurrentAccess(t *testing.T) {
	manager := NewGracefulShutdownManager(nil)

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				manager.IncrementConnections()
				manager.GetActiveConnections()
				manager.GetState()
				manager.GetStats()
				manager.DecrementConnections()
			}
		}()
	}

	wg.Wait()

	if manager.GetActiveConnections() != 0 {
		t.Errorf("Expected 0 connections after concurrent access, got %d", manager.GetActiveConnections())
	}
}

func TestGetShutdownManager(t *testing.T) {
	// First call should create the manager
	m1 := GetShutdownManager()
	if m1 == nil {
		t.Fatal("GetShutdownManager returned nil")
	}

	// Second call should return the same instance
	m2 := GetShutdownManager()
	if m1 != m2 {
		t.Error("GetShutdownManager should return same instance")
	}
}
