package internal

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// RateLimiterConfig holds rate limiter configuration
type RateLimiterConfig struct {
	// Global limits
	GlobalRequestsPerSecond int // Max requests per second globally (0 = unlimited)
	GlobalBurstSize         int // Burst allowance for global limit

	// Per-IP limits
	PerIPRequestsPerSecond int // Max requests per second per IP (0 = unlimited)
	PerIPBurstSize         int // Burst allowance per IP

	// Per-call-id limits
	PerCallRequestsPerSecond int // Max requests per second per call-id
	PerCallBurstSize         int // Burst allowance per call-id

	// Cleanup settings
	CleanupInterval time.Duration // How often to clean up stale entries
	EntryTTL        time.Duration // How long to keep entries after last use

	// Blocking behavior
	BlockDuration time.Duration // How long to block after exceeding limits
}

// DefaultRateLimiterConfig returns sensible defaults
func DefaultRateLimiterConfig() *RateLimiterConfig {
	return &RateLimiterConfig{
		GlobalRequestsPerSecond:  10000, // 10k requests/sec globally
		GlobalBurstSize:          1000,
		PerIPRequestsPerSecond:   100, // 100 requests/sec per IP
		PerIPBurstSize:           50,
		PerCallRequestsPerSecond: 10, // 10 requests/sec per call
		PerCallBurstSize:         5,
		CleanupInterval:          1 * time.Minute,
		EntryTTL:                 5 * time.Minute,
		BlockDuration:            10 * time.Second,
	}
}

// TokenBucket implements a token bucket rate limiter
type TokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket
func NewTokenBucket(maxTokens float64, refillRate float64) *TokenBucket {
	return &TokenBucket{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed and consumes a token if so
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}

// refill adds tokens based on elapsed time
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.lastRefill = now

	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
}

// RateLimitEntry tracks rate limiting for a specific key
type RateLimitEntry struct {
	bucket      *TokenBucket
	lastAccess  time.Time
	blocked     bool
	blockedAt   time.Time
	requests    atomic.Uint64
	rejected    atomic.Uint64
}

// RateLimiter provides rate limiting for NG protocol requests
type RateLimiter struct {
	config       *RateLimiterConfig
	globalBucket *TokenBucket
	ipBuckets    map[string]*RateLimitEntry
	callBuckets  map[string]*RateLimitEntry
	blockedIPs   map[string]time.Time
	mu           sync.RWMutex
	stopCh       chan struct{}
	stats        *RateLimiterStats
}

// RateLimiterStats tracks rate limiter statistics
type RateLimiterStats struct {
	TotalRequests   atomic.Uint64
	AllowedRequests atomic.Uint64
	RejectedGlobal  atomic.Uint64
	RejectedPerIP   atomic.Uint64
	RejectedPerCall atomic.Uint64
	RejectedBlocked atomic.Uint64
	ActiveIPs       atomic.Int64
	ActiveCalls     atomic.Int64
	BlockedIPs      atomic.Int64
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config *RateLimiterConfig) *RateLimiter {
	if config == nil {
		config = DefaultRateLimiterConfig()
	}

	rl := &RateLimiter{
		config:      config,
		ipBuckets:   make(map[string]*RateLimitEntry),
		callBuckets: make(map[string]*RateLimitEntry),
		blockedIPs:  make(map[string]time.Time),
		stopCh:      make(chan struct{}),
		stats:       &RateLimiterStats{},
	}

	// Create global bucket if limit is set
	if config.GlobalRequestsPerSecond > 0 {
		rl.globalBucket = NewTokenBucket(
			float64(config.GlobalBurstSize),
			float64(config.GlobalRequestsPerSecond),
		)
	}

	// Start cleanup goroutine
	go rl.cleanupLoop()

	return rl
}

// Allow checks if a request from the given IP for the given call-id is allowed
func (rl *RateLimiter) Allow(ip string, callID string) bool {
	rl.stats.TotalRequests.Add(1)

	// Check if IP is blocked
	if rl.isBlocked(ip) {
		rl.stats.RejectedBlocked.Add(1)
		return false
	}

	// Check global limit
	if rl.globalBucket != nil && !rl.globalBucket.Allow() {
		rl.stats.RejectedGlobal.Add(1)
		return false
	}

	// Check per-IP limit
	if rl.config.PerIPRequestsPerSecond > 0 {
		if !rl.checkIPLimit(ip) {
			rl.stats.RejectedPerIP.Add(1)
			rl.maybeBlockIP(ip)
			return false
		}
	}

	// Check per-call limit
	if rl.config.PerCallRequestsPerSecond > 0 && callID != "" {
		if !rl.checkCallLimit(callID) {
			rl.stats.RejectedPerCall.Add(1)
			return false
		}
	}

	rl.stats.AllowedRequests.Add(1)
	return true
}

// AllowIP checks if a request from the given IP is allowed (without call-id check)
func (rl *RateLimiter) AllowIP(ip string) bool {
	return rl.Allow(ip, "")
}

// checkIPLimit checks the per-IP rate limit
func (rl *RateLimiter) checkIPLimit(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry, exists := rl.ipBuckets[ip]
	if !exists {
		entry = &RateLimitEntry{
			bucket: NewTokenBucket(
				float64(rl.config.PerIPBurstSize),
				float64(rl.config.PerIPRequestsPerSecond),
			),
			lastAccess: time.Now(),
		}
		rl.ipBuckets[ip] = entry
		rl.stats.ActiveIPs.Add(1)
	}

	entry.lastAccess = time.Now()
	entry.requests.Add(1)

	if !entry.bucket.Allow() {
		entry.rejected.Add(1)
		return false
	}
	return true
}

// checkCallLimit checks the per-call rate limit
func (rl *RateLimiter) checkCallLimit(callID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry, exists := rl.callBuckets[callID]
	if !exists {
		entry = &RateLimitEntry{
			bucket: NewTokenBucket(
				float64(rl.config.PerCallBurstSize),
				float64(rl.config.PerCallRequestsPerSecond),
			),
			lastAccess: time.Now(),
		}
		rl.callBuckets[callID] = entry
		rl.stats.ActiveCalls.Add(1)
	}

	entry.lastAccess = time.Now()
	entry.requests.Add(1)

	if !entry.bucket.Allow() {
		entry.rejected.Add(1)
		return false
	}
	return true
}

// isBlocked checks if an IP is currently blocked
func (rl *RateLimiter) isBlocked(ip string) bool {
	rl.mu.RLock()
	blockedAt, blocked := rl.blockedIPs[ip]
	rl.mu.RUnlock()

	if !blocked {
		return false
	}

	// Check if block has expired
	if time.Since(blockedAt) > rl.config.BlockDuration {
		rl.mu.Lock()
		delete(rl.blockedIPs, ip)
		rl.stats.BlockedIPs.Add(-1)
		rl.mu.Unlock()
		return false
	}

	return true
}

// maybeBlockIP blocks an IP if it has exceeded limits too many times
func (rl *RateLimiter) maybeBlockIP(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry, exists := rl.ipBuckets[ip]
	if !exists {
		return
	}

	// Block if rejection rate is high
	total := entry.requests.Load()
	rejected := entry.rejected.Load()
	if total > 10 && float64(rejected)/float64(total) > 0.5 {
		rl.blockedIPs[ip] = time.Now()
		rl.stats.BlockedIPs.Add(1)
	}
}

// BlockIP manually blocks an IP
func (rl *RateLimiter) BlockIP(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if _, exists := rl.blockedIPs[ip]; !exists {
		rl.stats.BlockedIPs.Add(1)
	}
	rl.blockedIPs[ip] = time.Now()
}

// UnblockIP manually unblocks an IP
func (rl *RateLimiter) UnblockIP(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if _, exists := rl.blockedIPs[ip]; exists {
		delete(rl.blockedIPs, ip)
		rl.stats.BlockedIPs.Add(-1)
	}
}

// cleanupLoop periodically cleans up stale entries
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopCh:
			return
		}
	}
}

// cleanup removes stale entries
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	ttl := rl.config.EntryTTL

	// Cleanup IP entries
	for ip, entry := range rl.ipBuckets {
		if now.Sub(entry.lastAccess) > ttl {
			delete(rl.ipBuckets, ip)
			rl.stats.ActiveIPs.Add(-1)
		}
	}

	// Cleanup call entries
	for callID, entry := range rl.callBuckets {
		if now.Sub(entry.lastAccess) > ttl {
			delete(rl.callBuckets, callID)
			rl.stats.ActiveCalls.Add(-1)
		}
	}

	// Cleanup expired blocks
	for ip, blockedAt := range rl.blockedIPs {
		if now.Sub(blockedAt) > rl.config.BlockDuration {
			delete(rl.blockedIPs, ip)
			rl.stats.BlockedIPs.Add(-1)
		}
	}
}

// GetStats returns rate limiter statistics
func (rl *RateLimiter) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_requests":    rl.stats.TotalRequests.Load(),
		"allowed_requests":  rl.stats.AllowedRequests.Load(),
		"rejected_global":   rl.stats.RejectedGlobal.Load(),
		"rejected_per_ip":   rl.stats.RejectedPerIP.Load(),
		"rejected_per_call": rl.stats.RejectedPerCall.Load(),
		"rejected_blocked":  rl.stats.RejectedBlocked.Load(),
		"active_ips":        rl.stats.ActiveIPs.Load(),
		"active_calls":      rl.stats.ActiveCalls.Load(),
		"blocked_ips":       rl.stats.BlockedIPs.Load(),
	}
}

// GetBlockedIPs returns the list of blocked IPs
func (rl *RateLimiter) GetBlockedIPs() []string {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	ips := make([]string, 0, len(rl.blockedIPs))
	for ip := range rl.blockedIPs {
		ips = append(ips, ip)
	}
	return ips
}

// Stop stops the rate limiter
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// Reset resets all rate limiter state
func (rl *RateLimiter) Reset() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.ipBuckets = make(map[string]*RateLimitEntry)
	rl.callBuckets = make(map[string]*RateLimitEntry)
	rl.blockedIPs = make(map[string]time.Time)

	rl.stats.ActiveIPs.Store(0)
	rl.stats.ActiveCalls.Store(0)
	rl.stats.BlockedIPs.Store(0)
}

// IPRateLimiter is a simpler rate limiter for just IP-based limiting
type IPRateLimiter struct {
	requestsPerSecond int
	burstSize         int
	buckets           map[string]*TokenBucket
	mu                sync.RWMutex
}

// NewIPRateLimiter creates a simple IP-based rate limiter
func NewIPRateLimiter(requestsPerSecond, burstSize int) *IPRateLimiter {
	return &IPRateLimiter{
		requestsPerSecond: requestsPerSecond,
		burstSize:         burstSize,
		buckets:           make(map[string]*TokenBucket),
	}
}

// Allow checks if a request from the given IP is allowed
func (rl *IPRateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, exists := rl.buckets[ip]
	if !exists {
		bucket = NewTokenBucket(float64(rl.burstSize), float64(rl.requestsPerSecond))
		rl.buckets[ip] = bucket
	}

	return bucket.Allow()
}

// AllowAddr checks if a request from the given address is allowed
func (rl *IPRateLimiter) AllowAddr(addr net.Addr) bool {
	ip := extractIP(addr)
	return rl.Allow(ip)
}

// extractIP extracts IP address from net.Addr
func extractIP(addr net.Addr) string {
	switch v := addr.(type) {
	case *net.UDPAddr:
		return v.IP.String()
	case *net.TCPAddr:
		return v.IP.String()
	default:
		return addr.String()
	}
}

// Global rate limiter instance
var (
	globalRateLimiter     *RateLimiter
	globalRateLimiterOnce sync.Once
)

// GetRateLimiter returns the global rate limiter instance
func GetRateLimiter() *RateLimiter {
	globalRateLimiterOnce.Do(func() {
		globalRateLimiter = NewRateLimiter(DefaultRateLimiterConfig())
	})
	return globalRateLimiter
}

// RateLimitMiddleware is a middleware function for HTTP handlers
func RateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIPFromRequest(r)
			if !limiter.AllowIP(ip) {
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// extractIPFromRequest extracts IP from HTTP request
func extractIPFromRequest(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list
		if idx := len(xff); idx > 0 {
			for i := 0; i < len(xff); i++ {
				if xff[i] == ',' {
					return xff[:i]
				}
			}
			return xff
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Ensure RateLimitMiddleware returns the correct type
var _ func(http.Handler) http.Handler = RateLimitMiddleware(nil)

// Helper for formatting rate limit error
func RateLimitError(ip string) error {
	return fmt.Errorf("rate limit exceeded for IP %s", ip)
}
