package auth

import (
	"sync"
	"time"
)

// RateLimiter implements a token bucket rate limiter
type RateLimiter struct {
	rate       int           // Requests per period
	period     time.Duration // Time period
	buckets    map[string]*bucket
	mu         sync.RWMutex
	cleanup    *time.Ticker
	stopChan   chan struct{}
}

// bucket represents a token bucket for a single client
type bucket struct {
	tokens     float64
	lastRefill time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate int, period time.Duration) *RateLimiter {
	rl := &RateLimiter{
		rate:     rate,
		period:   period,
		buckets:  make(map[string]*bucket),
		cleanup:  time.NewTicker(time.Minute),
		stopChan: make(chan struct{}),
	}

	go rl.cleanupLoop()

	return rl
}

// cleanupLoop removes stale buckets
func (rl *RateLimiter) cleanupLoop() {
	for {
		select {
		case <-rl.cleanup.C:
			rl.cleanup_stale()
		case <-rl.stopChan:
			rl.cleanup.Stop()
			return
		}
	}
}

// cleanup_stale removes buckets that haven't been used recently
func (rl *RateLimiter) cleanup_stale() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	staleThreshold := time.Now().Add(-5 * rl.period)
	for key, b := range rl.buckets {
		if b.lastRefill.Before(staleThreshold) {
			delete(rl.buckets, key)
		}
	}
}

// Allow checks if a request is allowed for the given key
func (rl *RateLimiter) Allow(key string) bool {
	return rl.AllowN(key, 1)
}

// AllowN checks if n requests are allowed for the given key
func (rl *RateLimiter) AllowN(key string, n int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Get or create bucket
	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{
			tokens:     float64(rl.rate),
			lastRefill: now,
		}
		rl.buckets[key] = b
	}

	// Refill tokens
	elapsed := now.Sub(b.lastRefill)
	refillRate := float64(rl.rate) / float64(rl.period)
	b.tokens += elapsed.Seconds() * refillRate
	b.lastRefill = now

	// Cap at max tokens
	if b.tokens > float64(rl.rate) {
		b.tokens = float64(rl.rate)
	}

	// Check if enough tokens
	if b.tokens >= float64(n) {
		b.tokens -= float64(n)
		return true
	}

	return false
}

// Remaining returns the number of remaining requests for the given key
func (rl *RateLimiter) Remaining(key string) int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	b, ok := rl.buckets[key]
	if !ok {
		return rl.rate
	}

	// Calculate current tokens
	now := time.Now()
	elapsed := now.Sub(b.lastRefill)
	refillRate := float64(rl.rate) / float64(rl.period)
	tokens := b.tokens + elapsed.Seconds()*refillRate

	if tokens > float64(rl.rate) {
		tokens = float64(rl.rate)
	}

	return int(tokens)
}

// Reset resets the rate limiter for a given key
func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.buckets, key)
}

// ResetAll resets all rate limiters
func (rl *RateLimiter) ResetAll() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.buckets = make(map[string]*bucket)
}

// SetRate updates the rate limit
func (rl *RateLimiter) SetRate(rate int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.rate = rate
}

// GetRate returns the current rate limit
func (rl *RateLimiter) GetRate() int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return rl.rate
}

// Stop stops the rate limiter cleanup goroutine
func (rl *RateLimiter) Stop() {
	close(rl.stopChan)
}

// Stats returns rate limiter statistics
type RateLimiterStats struct {
	Rate          int
	Period        time.Duration
	ActiveBuckets int
}

// GetStats returns rate limiter statistics
func (rl *RateLimiter) GetStats() RateLimiterStats {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	return RateLimiterStats{
		Rate:          rl.rate,
		Period:        rl.period,
		ActiveBuckets: len(rl.buckets),
	}
}

// SlidingWindowRateLimiter implements a sliding window rate limiter
// This provides more accurate rate limiting than token bucket
type SlidingWindowRateLimiter struct {
	rate     int
	window   time.Duration
	requests map[string][]time.Time
	mu       sync.RWMutex
}

// NewSlidingWindowRateLimiter creates a new sliding window rate limiter
func NewSlidingWindowRateLimiter(rate int, window time.Duration) *SlidingWindowRateLimiter {
	return &SlidingWindowRateLimiter{
		rate:     rate,
		window:   window,
		requests: make(map[string][]time.Time),
	}
}

// Allow checks if a request is allowed
func (rl *SlidingWindowRateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	// Get requests for this key
	reqs, ok := rl.requests[key]
	if !ok {
		reqs = make([]time.Time, 0)
	}

	// Remove requests outside the window
	filtered := make([]time.Time, 0, len(reqs))
	for _, t := range reqs {
		if t.After(windowStart) {
			filtered = append(filtered, t)
		}
	}

	// Check if under limit
	if len(filtered) >= rl.rate {
		rl.requests[key] = filtered
		return false
	}

	// Add new request
	filtered = append(filtered, now)
	rl.requests[key] = filtered

	return true
}

// Remaining returns remaining requests in the current window
func (rl *SlidingWindowRateLimiter) Remaining(key string) int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	reqs, ok := rl.requests[key]
	if !ok {
		return rl.rate
	}

	count := 0
	for _, t := range reqs {
		if t.After(windowStart) {
			count++
		}
	}

	remaining := rl.rate - count
	if remaining < 0 {
		remaining = 0
	}

	return remaining
}

// Reset resets the rate limiter for a key
func (rl *SlidingWindowRateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.requests, key)
}

// PerKeyRateLimiter allows different rate limits per key
type PerKeyRateLimiter struct {
	defaultRate int
	period      time.Duration
	keyRates    map[string]int
	buckets     map[string]*bucket
	mu          sync.RWMutex
}

// NewPerKeyRateLimiter creates a new per-key rate limiter
func NewPerKeyRateLimiter(defaultRate int, period time.Duration) *PerKeyRateLimiter {
	return &PerKeyRateLimiter{
		defaultRate: defaultRate,
		period:      period,
		keyRates:    make(map[string]int),
		buckets:     make(map[string]*bucket),
	}
}

// SetKeyRate sets a custom rate for a specific key
func (rl *PerKeyRateLimiter) SetKeyRate(key string, rate int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.keyRates[key] = rate
}

// Allow checks if a request is allowed
func (rl *PerKeyRateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Get rate for this key
	rate, ok := rl.keyRates[key]
	if !ok {
		rate = rl.defaultRate
	}

	now := time.Now()

	// Get or create bucket
	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{
			tokens:     float64(rate),
			lastRefill: now,
		}
		rl.buckets[key] = b
	}

	// Refill tokens
	elapsed := now.Sub(b.lastRefill)
	refillRate := float64(rate) / float64(rl.period)
	b.tokens += elapsed.Seconds() * refillRate
	b.lastRefill = now

	// Cap at max tokens
	if b.tokens > float64(rate) {
		b.tokens = float64(rate)
	}

	// Check if enough tokens
	if b.tokens >= 1 {
		b.tokens -= 1
		return true
	}

	return false
}
