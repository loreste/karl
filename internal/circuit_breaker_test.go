package internal

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		state    CircuitState
		expected string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("CircuitState(%d).String() = %s, expected %s", tt.state, got, tt.expected)
		}
	}
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test")

	if config.Name != "test" {
		t.Errorf("Expected name 'test', got %s", config.Name)
	}
	if config.MaxFailures != 5 {
		t.Errorf("Expected MaxFailures 5, got %d", config.MaxFailures)
	}
	if config.Timeout != 30*time.Second {
		t.Errorf("Expected Timeout 30s, got %v", config.Timeout)
	}
}

func TestNewCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	if cb == nil {
		t.Fatal("NewCircuitBreaker returned nil")
	}
	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected initial state Closed, got %s", cb.GetState().String())
	}
}

func TestCircuitBreaker_AllowWhenClosed(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	err := cb.Allow()
	if err != nil {
		t.Errorf("Allow should return nil when closed, got %v", err)
	}
}

func TestCircuitBreaker_SuccessResetFailures(t *testing.T) {
	config := &CircuitBreakerConfig{
		Name:               "test",
		MaxFailures:        10, // High threshold so we stay closed
		Timeout:            time.Second,
		FailureRatePercent: 0, // Disable rate-based checking
	}
	cb := NewCircuitBreaker(config)

	// Record some failures
	cb.Failure()
	cb.Failure()

	if cb.failures.Load() != 2 {
		t.Errorf("Expected 2 failures, got %d", cb.failures.Load())
	}

	// Success should reset failures when circuit is closed
	cb.Success()

	if cb.failures.Load() != 0 {
		t.Errorf("Expected 0 failures after success, got %d", cb.failures.Load())
	}
}

func TestCircuitBreaker_OpenAfterMaxFailures(t *testing.T) {
	config := &CircuitBreakerConfig{
		Name:        "test",
		MaxFailures: 3,
		Timeout:     time.Second,
	}
	cb := NewCircuitBreaker(config)

	// Record failures
	cb.Failure()
	cb.Failure()
	cb.Failure()

	if cb.GetState() != CircuitOpen {
		t.Errorf("Expected state Open after max failures, got %s", cb.GetState().String())
	}
}

func TestCircuitBreaker_AllowBlockedWhenOpen(t *testing.T) {
	config := &CircuitBreakerConfig{
		Name:        "test",
		MaxFailures: 1,
		Timeout:     time.Hour, // Long timeout
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.Failure()

	err := cb.Allow()
	if err != ErrCircuitOpen {
		t.Errorf("Expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreaker_TransitionToHalfOpen(t *testing.T) {
	config := &CircuitBreakerConfig{
		Name:             "test",
		MaxFailures:      1,
		Timeout:          100 * time.Millisecond,
		HalfOpenMaxCalls: 5, // Allow multiple calls in half-open
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.Failure()

	if cb.GetState() != CircuitOpen {
		t.Fatalf("Expected state Open, got %s", cb.GetState().String())
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Next Allow should transition to half-open
	err := cb.Allow()
	if err != nil {
		t.Errorf("Expected Allow to succeed after timeout, got %v", err)
	}

	if cb.GetState() != CircuitHalfOpen {
		t.Errorf("Expected state HalfOpen, got %s", cb.GetState().String())
	}
}

func TestCircuitBreaker_HalfOpenToClosedOnSuccess(t *testing.T) {
	config := &CircuitBreakerConfig{
		Name:             "test",
		MaxFailures:      1,
		Timeout:          100 * time.Millisecond,
		SuccessThreshold: 2,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.Failure()
	time.Sleep(150 * time.Millisecond)
	cb.Allow() // Transition to half-open

	if cb.GetState() != CircuitHalfOpen {
		t.Fatalf("Expected state HalfOpen, got %s", cb.GetState().String())
	}

	// Record successes
	cb.Success()
	cb.Success()

	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected state Closed after successes, got %s", cb.GetState().String())
	}
}

func TestCircuitBreaker_HalfOpenToOpenOnFailure(t *testing.T) {
	config := &CircuitBreakerConfig{
		Name:        "test",
		MaxFailures: 1,
		Timeout:     100 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.Failure()
	time.Sleep(150 * time.Millisecond)
	cb.Allow() // Transition to half-open

	if cb.GetState() != CircuitHalfOpen {
		t.Fatalf("Expected state HalfOpen, got %s", cb.GetState().String())
	}

	// Failure should reopen
	cb.Failure()

	if cb.GetState() != CircuitOpen {
		t.Errorf("Expected state Open after failure in half-open, got %s", cb.GetState().String())
	}
}

func TestCircuitBreaker_HalfOpenMaxCalls(t *testing.T) {
	config := &CircuitBreakerConfig{
		Name:             "test",
		MaxFailures:      1,
		Timeout:          100 * time.Millisecond,
		HalfOpenMaxCalls: 2,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit and wait for half-open
	cb.Failure()
	time.Sleep(150 * time.Millisecond)

	// First two calls should succeed
	if err := cb.Allow(); err != nil {
		t.Errorf("First Allow should succeed, got %v", err)
	}
	if err := cb.Allow(); err != nil {
		t.Errorf("Second Allow should succeed, got %v", err)
	}

	// Third call should fail
	if err := cb.Allow(); err != ErrTooManyRequest {
		t.Errorf("Expected ErrTooManyRequest, got %v", err)
	}
}

func TestCircuitBreaker_Execute(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	// Successful execution
	err := cb.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Execute should succeed, got %v", err)
	}

	// Failed execution
	expectedErr := errors.New("test error")
	err = cb.Execute(func() error {
		return expectedErr
	})
	if err != expectedErr {
		t.Errorf("Execute should return function error, got %v", err)
	}
}

func TestCircuitBreaker_ExecuteBlocked(t *testing.T) {
	config := &CircuitBreakerConfig{
		Name:        "test",
		MaxFailures: 1,
		Timeout:     time.Hour,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.Failure()

	// Execute should return circuit open error
	err := cb.Execute(func() error {
		t.Error("Function should not be called when circuit is open")
		return nil
	})

	if err != ErrCircuitOpen {
		t.Errorf("Expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreaker_ExecuteWithContext(t *testing.T) {
	cb := NewCircuitBreaker(nil)
	ctx := context.Background()

	err := cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("ExecuteWithContext should succeed, got %v", err)
	}
}

func TestCircuitBreaker_ExecuteWithContext_Cancelled(t *testing.T) {
	cb := NewCircuitBreaker(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		t.Error("Function should not be called with cancelled context")
		return nil
	})

	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestCircuitBreaker_OnStateChange(t *testing.T) {
	config := &CircuitBreakerConfig{
		Name:        "test",
		MaxFailures: 1,
		Timeout:     100 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	var transitions []string
	var mu sync.Mutex

	cb.OnStateChange(func(from, to CircuitState) {
		mu.Lock()
		transitions = append(transitions, from.String()+"->"+to.String())
		mu.Unlock()
	})

	// Trigger state changes
	cb.Failure()                     // closed -> open
	time.Sleep(150 * time.Millisecond)
	cb.Allow()                       // open -> half-open
	cb.Success()
	cb.Success()                     // May transition to closed depending on config

	// Give callback time to execute
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(transitions) < 2 {
		t.Errorf("Expected at least 2 transitions, got %d: %v", len(transitions), transitions)
	}
	if transitions[0] != "closed->open" {
		t.Errorf("Expected first transition 'closed->open', got %s", transitions[0])
	}
}

func TestCircuitBreaker_GetStats(t *testing.T) {
	config := &CircuitBreakerConfig{
		Name:               "test-stats",
		MaxFailures:        10, // High threshold so we stay closed
		Timeout:            time.Second,
		FailureRatePercent: 0, // Disable rate-based checking
	}
	cb := NewCircuitBreaker(config)

	cb.Success()
	cb.Success()

	stats := cb.GetStats()

	if stats["name"] != "test-stats" {
		t.Errorf("Expected name 'test-stats', got %v", stats["name"])
	}
	if stats["state"] != "closed" {
		t.Errorf("Expected state 'closed', got %v", stats["state"])
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := &CircuitBreakerConfig{
		Name:        "test",
		MaxFailures: 1,
		Timeout:     time.Hour,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.Failure()

	if cb.GetState() != CircuitOpen {
		t.Fatalf("Expected state Open, got %s", cb.GetState().String())
	}

	// Reset
	cb.Reset()

	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected state Closed after reset, got %s", cb.GetState().String())
	}
	if cb.failures.Load() != 0 {
		t.Errorf("Expected 0 failures after reset, got %d", cb.failures.Load())
	}
}

func TestCircuitBreaker_FailureRateBased(t *testing.T) {
	config := &CircuitBreakerConfig{
		Name:               "test",
		MaxFailures:        100, // High count threshold
		Timeout:            time.Second,
		FailureRateWindow:  time.Second,
		FailureRatePercent: 50.0,
	}
	cb := NewCircuitBreaker(config)

	// 50% failures should open circuit
	cb.Failure()
	cb.Success()
	cb.Failure()
	cb.Success()
	cb.Failure()

	// At this point we have 60% failure rate (3/5)
	if cb.GetState() != CircuitOpen {
		t.Errorf("Expected state Open with >50%% failure rate, got %s", cb.GetState().String())
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cb.Allow()
				if j%2 == 0 {
					cb.Success()
				} else {
					cb.Failure()
				}
				cb.GetState()
				cb.GetStats()
			}
		}(i)
	}

	wg.Wait()
}

func TestNewCircuitBreakerRegistry(t *testing.T) {
	registry := NewCircuitBreakerRegistry()

	if registry == nil {
		t.Fatal("NewCircuitBreakerRegistry returned nil")
	}
}

func TestCircuitBreakerRegistry_Get(t *testing.T) {
	registry := NewCircuitBreakerRegistry()

	cb1 := registry.Get("test1")
	if cb1 == nil {
		t.Fatal("Get returned nil")
	}

	cb2 := registry.Get("test1")
	if cb1 != cb2 {
		t.Error("Get should return same instance for same name")
	}

	cb3 := registry.Get("test2")
	if cb1 == cb3 {
		t.Error("Get should return different instance for different name")
	}
}

func TestCircuitBreakerRegistry_GetWithConfig(t *testing.T) {
	registry := NewCircuitBreakerRegistry()

	config := &CircuitBreakerConfig{
		Name:        "custom",
		MaxFailures: 10,
	}

	cb := registry.GetWithConfig(config)
	if cb == nil {
		t.Fatal("GetWithConfig returned nil")
	}

	// Same name should return same instance
	cb2 := registry.Get("custom")
	if cb != cb2 {
		t.Error("Should return same instance for same name")
	}
}

func TestCircuitBreakerRegistry_GetAllStats(t *testing.T) {
	registry := NewCircuitBreakerRegistry()

	registry.Get("test1")
	registry.Get("test2")

	stats := registry.GetAllStats()

	if len(stats) != 2 {
		t.Errorf("Expected 2 breakers in stats, got %d", len(stats))
	}
	if _, exists := stats["test1"]; !exists {
		t.Error("Missing test1 in stats")
	}
	if _, exists := stats["test2"]; !exists {
		t.Error("Missing test2 in stats")
	}
}

func TestCircuitBreakerRegistry_Reset(t *testing.T) {
	registry := NewCircuitBreakerRegistry()

	config := &CircuitBreakerConfig{
		Name:        "test",
		MaxFailures: 1,
		Timeout:     time.Hour,
	}
	cb := registry.GetWithConfig(config)

	// Open the circuit
	cb.Failure()

	if cb.GetState() != CircuitOpen {
		t.Fatalf("Expected state Open, got %s", cb.GetState().String())
	}

	// Reset via registry
	registry.Reset("test")

	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected state Closed after reset, got %s", cb.GetState().String())
	}
}

func TestCircuitBreakerRegistry_ResetAll(t *testing.T) {
	registry := NewCircuitBreakerRegistry()

	config1 := &CircuitBreakerConfig{Name: "test1", MaxFailures: 1, Timeout: time.Hour}
	config2 := &CircuitBreakerConfig{Name: "test2", MaxFailures: 1, Timeout: time.Hour}

	cb1 := registry.GetWithConfig(config1)
	cb2 := registry.GetWithConfig(config2)

	cb1.Failure()
	cb2.Failure()

	registry.ResetAll()

	if cb1.GetState() != CircuitClosed {
		t.Errorf("cb1 should be Closed, got %s", cb1.GetState().String())
	}
	if cb2.GetState() != CircuitClosed {
		t.Errorf("cb2 should be Closed, got %s", cb2.GetState().String())
	}
}

func TestCircuitBreakerRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewCircuitBreakerRegistry()

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cb := registry.Get("shared")
				cb.Allow()
				cb.Success()
			}
		}(i)
	}

	wg.Wait()
}

func TestGetCircuitBreakerRegistry(t *testing.T) {
	r1 := GetCircuitBreakerRegistry()
	if r1 == nil {
		t.Fatal("GetCircuitBreakerRegistry returned nil")
	}

	r2 := GetCircuitBreakerRegistry()
	if r1 != r2 {
		t.Error("GetCircuitBreakerRegistry should return same instance")
	}
}

func TestWrapWithCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	// Success case
	result, err := WrapWithCircuitBreaker(cb, func() (string, error) {
		return "success", nil
	})
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
	if result != "success" {
		t.Errorf("Expected 'success', got %s", result)
	}

	// Error case
	expectedErr := errors.New("test error")
	_, err = WrapWithCircuitBreaker(cb, func() (string, error) {
		return "", expectedErr
	})
	if err != expectedErr {
		t.Errorf("Expected test error, got %v", err)
	}
}

func TestRedisCircuitBreaker(t *testing.T) {
	rcb := NewRedisCircuitBreaker()

	if rcb == nil {
		t.Fatal("NewRedisCircuitBreaker returned nil")
	}

	if rcb.GetState() != CircuitClosed {
		t.Errorf("Expected initial state Closed, got %s", rcb.GetState().String())
	}

	// Test execute
	err := rcb.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Execute should succeed, got %v", err)
	}

	stats := rcb.GetStats()
	if stats["name"] != CBRedis {
		t.Errorf("Expected name %s, got %v", CBRedis, stats["name"])
	}
}

func TestRecordingCircuitBreaker(t *testing.T) {
	rcb := NewRecordingCircuitBreaker()

	if rcb == nil {
		t.Fatal("NewRecordingCircuitBreaker returned nil")
	}

	if rcb.GetState() != CircuitClosed {
		t.Errorf("Expected initial state Closed, got %s", rcb.GetState().String())
	}

	stats := rcb.GetStats()
	if stats["name"] != CBRecording {
		t.Errorf("Expected name %s, got %v", CBRecording, stats["name"])
	}
}

func TestCircuitBreaker_IntegrationScenario(t *testing.T) {
	config := &CircuitBreakerConfig{
		Name:             "integration",
		MaxFailures:      3,
		Timeout:          200 * time.Millisecond,
		HalfOpenMaxCalls: 2,
		SuccessThreshold: 2,
	}
	cb := NewCircuitBreaker(config)

	// Track calls
	var callCount atomic.Int64
	simulateCall := func(succeed bool) error {
		return cb.Execute(func() error {
			callCount.Add(1)
			if !succeed {
				return errors.New("simulated failure")
			}
			return nil
		})
	}

	// Normal operation
	for i := 0; i < 5; i++ {
		simulateCall(true)
	}
	if cb.GetState() != CircuitClosed {
		t.Errorf("Should stay closed with successful calls")
	}

	// Failures open the circuit
	for i := 0; i < 3; i++ {
		simulateCall(false)
	}
	if cb.GetState() != CircuitOpen {
		t.Errorf("Should be open after failures")
	}

	// Calls should be blocked
	initialCount := callCount.Load()
	simulateCall(true)
	if callCount.Load() != initialCount {
		t.Error("Call should have been blocked")
	}

	// Wait for timeout
	time.Sleep(250 * time.Millisecond)

	// Should transition to half-open and allow limited calls
	simulateCall(true)
	if cb.GetState() != CircuitHalfOpen {
		t.Errorf("Should be half-open after timeout, got %s", cb.GetState().String())
	}

	// Success should close the circuit
	simulateCall(true)
	if cb.GetState() != CircuitClosed {
		t.Errorf("Should be closed after successes in half-open, got %s", cb.GetState().String())
	}
}
