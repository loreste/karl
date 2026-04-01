package internal

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int32

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Common errors
var (
	ErrCircuitOpen    = errors.New("circuit breaker is open")
	ErrTooManyRequest = errors.New("too many requests in half-open state")
)

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	Name               string
	MaxFailures        int           // Failures before opening circuit
	Timeout            time.Duration // How long circuit stays open
	HalfOpenMaxCalls   int           // Max calls allowed in half-open state
	SuccessThreshold   int           // Successes needed to close from half-open
	FailureRateWindow  time.Duration // Window for calculating failure rate
	FailureRatePercent float64       // Failure rate percentage to open circuit
}

// DefaultCircuitBreakerConfig returns sensible defaults
func DefaultCircuitBreakerConfig(name string) *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		Name:               name,
		MaxFailures:        5,
		Timeout:            30 * time.Second,
		HalfOpenMaxCalls:   3,
		SuccessThreshold:   2,
		FailureRateWindow:  60 * time.Second,
		FailureRatePercent: 50.0,
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config *CircuitBreakerConfig

	state        atomic.Int32
	failures     atomic.Int64
	successes    atomic.Int64
	lastFailure  atomic.Value // time.Time
	openedAt     atomic.Value // time.Time
	halfOpenCalls atomic.Int64

	// For rate-based tracking
	recentCalls   []callResult
	recentCallsMu sync.RWMutex

	// Callbacks
	onStateChange func(from, to CircuitState)

	mu sync.Mutex
}

type callResult struct {
	timestamp time.Time
	success   bool
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config *CircuitBreakerConfig) *CircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig("default")
	}

	cb := &CircuitBreaker{
		config:      config,
		recentCalls: make([]callResult, 0),
	}

	cb.state.Store(int32(CircuitClosed))
	cb.lastFailure.Store(time.Time{})
	cb.openedAt.Store(time.Time{})

	return cb
}

// OnStateChange sets a callback for state changes
func (cb *CircuitBreaker) OnStateChange(fn func(from, to CircuitState)) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.onStateChange = fn
}

// GetState returns the current circuit state
func (cb *CircuitBreaker) GetState() CircuitState {
	return CircuitState(cb.state.Load())
}

// Allow checks if a request should be allowed
func (cb *CircuitBreaker) Allow() error {
	state := cb.GetState()

	switch state {
	case CircuitClosed:
		return nil

	case CircuitOpen:
		// Check if timeout has passed
		openedAt, ok := cb.openedAt.Load().(time.Time)
		if ok && !openedAt.IsZero() && time.Since(openedAt) > cb.config.Timeout {
			cb.transitionTo(CircuitHalfOpen)
			return cb.Allow()
		}
		return ErrCircuitOpen

	case CircuitHalfOpen:
		// Limit calls in half-open state
		calls := cb.halfOpenCalls.Add(1)
		if calls > int64(cb.config.HalfOpenMaxCalls) {
			cb.halfOpenCalls.Add(-1)
			return ErrTooManyRequest
		}
		return nil
	}

	return nil
}

// Success records a successful call
func (cb *CircuitBreaker) Success() {
	cb.recordCall(true)

	state := cb.GetState()

	switch state {
	case CircuitClosed:
		cb.failures.Store(0)

	case CircuitHalfOpen:
		successes := cb.successes.Add(1)
		if successes >= int64(cb.config.SuccessThreshold) {
			cb.transitionTo(CircuitClosed)
		}
	}
}

// Failure records a failed call
func (cb *CircuitBreaker) Failure() {
	cb.recordCall(false)
	cb.lastFailure.Store(time.Now())

	state := cb.GetState()

	switch state {
	case CircuitClosed:
		failures := cb.failures.Add(1)

		// Check count-based threshold
		if failures >= int64(cb.config.MaxFailures) {
			cb.transitionTo(CircuitOpen)
			return
		}

		// Check rate-based threshold
		if cb.config.FailureRatePercent > 0 {
			rate := cb.getFailureRate()
			if rate >= cb.config.FailureRatePercent {
				cb.transitionTo(CircuitOpen)
			}
		}

	case CircuitHalfOpen:
		// Any failure in half-open state reopens the circuit
		cb.transitionTo(CircuitOpen)
	}
}

// transitionTo transitions to a new state
func (cb *CircuitBreaker) transitionTo(newState CircuitState) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	oldState := CircuitState(cb.state.Load())
	if oldState == newState {
		return
	}

	cb.state.Store(int32(newState))

	switch newState {
	case CircuitClosed:
		cb.failures.Store(0)
		cb.successes.Store(0)
		cb.halfOpenCalls.Store(0)

	case CircuitOpen:
		cb.openedAt.Store(time.Now())
		cb.halfOpenCalls.Store(0)

	case CircuitHalfOpen:
		cb.successes.Store(0)
		cb.halfOpenCalls.Store(0)
	}

	if cb.onStateChange != nil {
		go cb.onStateChange(oldState, newState)
	}
}

// recordCall records a call result for rate calculation
func (cb *CircuitBreaker) recordCall(success bool) {
	cb.recentCallsMu.Lock()
	defer cb.recentCallsMu.Unlock()

	now := time.Now()
	cutoff := now.Add(-cb.config.FailureRateWindow)

	// Add new result
	cb.recentCalls = append(cb.recentCalls, callResult{
		timestamp: now,
		success:   success,
	})

	// Remove old results
	newCalls := make([]callResult, 0)
	for _, call := range cb.recentCalls {
		if call.timestamp.After(cutoff) {
			newCalls = append(newCalls, call)
		}
	}
	cb.recentCalls = newCalls
}

// getFailureRate calculates the failure rate in the recent window
func (cb *CircuitBreaker) getFailureRate() float64 {
	cb.recentCallsMu.RLock()
	defer cb.recentCallsMu.RUnlock()

	if len(cb.recentCalls) == 0 {
		return 0
	}

	failures := 0
	for _, call := range cb.recentCalls {
		if !call.success {
			failures++
		}
	}

	return float64(failures) / float64(len(cb.recentCalls)) * 100
}

// Execute wraps a function with circuit breaker logic
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if err := cb.Allow(); err != nil {
		return err
	}

	err := fn()

	if err != nil {
		cb.Failure()
	} else {
		cb.Success()
	}

	return err
}

// ExecuteWithContext wraps a function with circuit breaker and context support
func (cb *CircuitBreaker) ExecuteWithContext(ctx context.Context, fn func(context.Context) error) error {
	if err := cb.Allow(); err != nil {
		return err
	}

	// Check context before executing
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	err := fn(ctx)

	if err != nil {
		cb.Failure()
	} else {
		cb.Success()
	}

	return err
}

// GetStats returns circuit breaker statistics
func (cb *CircuitBreaker) GetStats() map[string]interface{} {
	lastFailure, _ := cb.lastFailure.Load().(time.Time)
	openedAt, _ := cb.openedAt.Load().(time.Time)

	stats := map[string]interface{}{
		"name":          cb.config.Name,
		"state":         cb.GetState().String(),
		"failures":      cb.failures.Load(),
		"successes":     cb.successes.Load(),
		"failure_rate":  cb.getFailureRate(),
		"half_open_calls": cb.halfOpenCalls.Load(),
	}

	if !lastFailure.IsZero() {
		stats["last_failure"] = lastFailure.Format(time.RFC3339)
		stats["since_last_failure"] = time.Since(lastFailure).String()
	}

	if !openedAt.IsZero() && cb.GetState() == CircuitOpen {
		stats["opened_at"] = openedAt.Format(time.RFC3339)
		stats["time_until_retry"] = (cb.config.Timeout - time.Since(openedAt)).String()
	}

	return stats
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state.Store(int32(CircuitClosed))
	cb.failures.Store(0)
	cb.successes.Store(0)
	cb.halfOpenCalls.Store(0)
	cb.lastFailure.Store(time.Time{})
	cb.openedAt.Store(time.Time{})

	cb.recentCallsMu.Lock()
	cb.recentCalls = make([]callResult, 0)
	cb.recentCallsMu.Unlock()
}

// CircuitBreakerRegistry manages multiple circuit breakers
type CircuitBreakerRegistry struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
}

// NewCircuitBreakerRegistry creates a new registry
func NewCircuitBreakerRegistry() *CircuitBreakerRegistry {
	return &CircuitBreakerRegistry{
		breakers: make(map[string]*CircuitBreaker),
	}
}

// Get gets or creates a circuit breaker by name
func (r *CircuitBreakerRegistry) Get(name string) *CircuitBreaker {
	r.mu.RLock()
	cb, exists := r.breakers[name]
	r.mu.RUnlock()

	if exists {
		return cb
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double check after acquiring write lock
	if cb, exists = r.breakers[name]; exists {
		return cb
	}

	cb = NewCircuitBreaker(DefaultCircuitBreakerConfig(name))
	r.breakers[name] = cb
	return cb
}

// GetWithConfig gets or creates a circuit breaker with specific config
func (r *CircuitBreakerRegistry) GetWithConfig(config *CircuitBreakerConfig) *CircuitBreaker {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cb, exists := r.breakers[config.Name]; exists {
		return cb
	}

	cb := NewCircuitBreaker(config)
	r.breakers[config.Name] = cb
	return cb
}

// GetAllStats returns stats for all circuit breakers
func (r *CircuitBreakerRegistry) GetAllStats() map[string]map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[string]map[string]interface{})
	for name, cb := range r.breakers {
		stats[name] = cb.GetStats()
	}
	return stats
}

// Reset resets a specific circuit breaker
func (r *CircuitBreakerRegistry) Reset(name string) {
	r.mu.RLock()
	cb, exists := r.breakers[name]
	r.mu.RUnlock()

	if exists {
		cb.Reset()
	}
}

// ResetAll resets all circuit breakers
func (r *CircuitBreakerRegistry) ResetAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, cb := range r.breakers {
		cb.Reset()
	}
}

// Global registry
var (
	globalCircuitBreakerRegistry     *CircuitBreakerRegistry
	globalCircuitBreakerRegistryOnce sync.Once
)

// GetCircuitBreakerRegistry returns the global circuit breaker registry
func GetCircuitBreakerRegistry() *CircuitBreakerRegistry {
	globalCircuitBreakerRegistryOnce.Do(func() {
		globalCircuitBreakerRegistry = NewCircuitBreakerRegistry()
	})
	return globalCircuitBreakerRegistry
}

// Predefined circuit breakers for common dependencies
const (
	CBRedis       = "redis"
	CBDatabase    = "database"
	CBRecording   = "recording"
	CBExternalAPI = "external-api"
	CBDNSLookup   = "dns-lookup"
)

// CircuitBreakerMiddleware provides HTTP middleware with circuit breaker
type CircuitBreakerMiddleware struct {
	registry *CircuitBreakerRegistry
}

// NewCircuitBreakerMiddleware creates new middleware
func NewCircuitBreakerMiddleware(registry *CircuitBreakerRegistry) *CircuitBreakerMiddleware {
	if registry == nil {
		registry = GetCircuitBreakerRegistry()
	}
	return &CircuitBreakerMiddleware{registry: registry}
}

// WrapService wraps a service function with circuit breaker
func WrapWithCircuitBreaker[T any](cb *CircuitBreaker, fn func() (T, error)) (T, error) {
	var zero T

	if err := cb.Allow(); err != nil {
		return zero, err
	}

	result, err := fn()

	if err != nil {
		cb.Failure()
	} else {
		cb.Success()
	}

	return result, err
}

// RedisCircuitBreaker provides a specialized circuit breaker for Redis
type RedisCircuitBreaker struct {
	cb *CircuitBreaker
}

// NewRedisCircuitBreaker creates a Redis-specific circuit breaker
func NewRedisCircuitBreaker() *RedisCircuitBreaker {
	config := &CircuitBreakerConfig{
		Name:               CBRedis,
		MaxFailures:        3,
		Timeout:            10 * time.Second,
		HalfOpenMaxCalls:   2,
		SuccessThreshold:   2,
		FailureRateWindow:  30 * time.Second,
		FailureRatePercent: 50.0,
	}

	return &RedisCircuitBreaker{
		cb: NewCircuitBreaker(config),
	}
}

// Execute executes a Redis operation with circuit breaker
func (r *RedisCircuitBreaker) Execute(fn func() error) error {
	return r.cb.Execute(fn)
}

// GetState returns the current state
func (r *RedisCircuitBreaker) GetState() CircuitState {
	return r.cb.GetState()
}

// GetStats returns statistics
func (r *RedisCircuitBreaker) GetStats() map[string]interface{} {
	return r.cb.GetStats()
}

// RecordingCircuitBreaker provides a specialized circuit breaker for recording
type RecordingCircuitBreaker struct {
	cb *CircuitBreaker
}

// NewRecordingCircuitBreaker creates a recording-specific circuit breaker
func NewRecordingCircuitBreaker() *RecordingCircuitBreaker {
	config := &CircuitBreakerConfig{
		Name:               CBRecording,
		MaxFailures:        5,
		Timeout:            60 * time.Second, // Longer timeout for recording
		HalfOpenMaxCalls:   1,
		SuccessThreshold:   3,
		FailureRateWindow:  120 * time.Second,
		FailureRatePercent: 30.0,
	}

	return &RecordingCircuitBreaker{
		cb: NewCircuitBreaker(config),
	}
}

// Execute executes a recording operation with circuit breaker
func (r *RecordingCircuitBreaker) Execute(fn func() error) error {
	return r.cb.Execute(fn)
}

// GetState returns the current state
func (r *RecordingCircuitBreaker) GetState() CircuitState {
	return r.cb.GetState()
}

// GetStats returns statistics
func (r *RecordingCircuitBreaker) GetStats() map[string]interface{} {
	return r.cb.GetStats()
}
