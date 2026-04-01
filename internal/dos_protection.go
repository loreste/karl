package internal

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// DoS protection errors
var (
	ErrRateLimitExceeded    = errors.New("rate limit exceeded")
	ErrConnectionLimit      = errors.New("connection limit exceeded")
	ErrRequestTooLarge      = errors.New("request too large")
	ErrClientBanned         = errors.New("client is banned")
	ErrResourceExhausted    = errors.New("resource exhausted")
	ErrSlowClientDetected   = errors.New("slow client detected")
)

// DoSProtectionConfig holds configuration for DoS protection
type DoSProtectionConfig struct {
	// Global rate limits
	GlobalRequestsPerSecond int
	GlobalBurstSize         int

	// Per-IP rate limits
	PerIPRequestsPerSecond int
	PerIPBurstSize         int

	// Connection limits
	MaxConnectionsPerIP int
	MaxTotalConnections int

	// Request limits
	MaxRequestSize      int64
	MaxHeaderSize       int
	MaxBodySize         int64

	// Timeout protection
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	SlowClientTimeout time.Duration

	// Ban settings
	BanThreshold     int           // Number of violations before ban
	BanDuration      time.Duration // How long to ban
	ViolationWindow  time.Duration // Window for counting violations

	// Resource limits
	MaxConcurrentRequests int
	MaxPendingRequests    int
	MaxMemoryPerRequest   int64

	// Whitelist/Blacklist
	WhitelistedIPs []string
	BlacklistedIPs []string
}

// DefaultDoSProtectionConfig returns sensible defaults
func DefaultDoSProtectionConfig() *DoSProtectionConfig {
	return &DoSProtectionConfig{
		GlobalRequestsPerSecond: 10000,
		GlobalBurstSize:         1000,
		PerIPRequestsPerSecond:  100,
		PerIPBurstSize:          50,
		MaxConnectionsPerIP:     100,
		MaxTotalConnections:     10000,
		MaxRequestSize:          1024 * 1024,     // 1MB
		MaxHeaderSize:           8 * 1024,        // 8KB
		MaxBodySize:             512 * 1024,      // 512KB
		ReadTimeout:             30 * time.Second,
		WriteTimeout:            30 * time.Second,
		IdleTimeout:             60 * time.Second,
		SlowClientTimeout:       10 * time.Second,
		BanThreshold:            10,
		BanDuration:             15 * time.Minute,
		ViolationWindow:         1 * time.Minute,
		MaxConcurrentRequests:   1000,
		MaxPendingRequests:      5000,
		MaxMemoryPerRequest:     10 * 1024 * 1024, // 10MB
	}
}

// DoSProtector provides comprehensive DoS protection
type DoSProtector struct {
	config *DoSProtectionConfig

	// Global rate limiter
	globalLimiter *TokenBucketLimiter

	// Per-IP state
	ipStateMu sync.RWMutex
	ipStates  map[string]*ipState

	// Connection tracking
	connectionsMu    sync.RWMutex
	connectionsPerIP map[string]int32
	totalConnections atomic.Int32

	// Concurrent request tracking
	concurrentRequests atomic.Int32
	pendingRequests    atomic.Int32

	// Ban list
	bansMu sync.RWMutex
	bans   map[string]time.Time

	// Whitelist/Blacklist
	whitelist map[string]struct{}
	blacklist map[string]struct{}

	// Stats
	stats DoSStats

	// Cleanup
	stopCleanup chan struct{}
	cleanupDone chan struct{}
}

// ipState tracks per-IP state for DoS protection
type ipState struct {
	limiter         *TokenBucketLimiter
	violations      []time.Time
	connections     int32
	lastSeen        time.Time
	mu              sync.Mutex
}

// DoSStats holds DoS protection statistics
type DoSStats struct {
	TotalRequests         atomic.Int64
	RateLimitedRequests   atomic.Int64
	BannedRequests        atomic.Int64
	OversizedRequests     atomic.Int64
	SlowClientRequests    atomic.Int64
	ConnectionsRejected   atomic.Int64
	CurrentConnections    atomic.Int32
	CurrentBannedIPs      atomic.Int32
	ActiveIPs             atomic.Int32
}

// TokenBucketLimiter implements a token bucket rate limiter
type TokenBucketLimiter struct {
	rate       float64 // tokens per second
	burst      int     // max tokens
	tokens     float64
	lastUpdate time.Time
	mu         sync.Mutex
}

// NewTokenBucketLimiter creates a new token bucket limiter
func NewTokenBucketLimiter(rate float64, burst int) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		rate:       rate,
		burst:      burst,
		tokens:     float64(burst),
		lastUpdate: time.Now(),
	}
}

// Allow checks if a request is allowed
func (l *TokenBucketLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(l.lastUpdate).Seconds()
	l.lastUpdate = now

	// Add tokens
	l.tokens += elapsed * l.rate
	if l.tokens > float64(l.burst) {
		l.tokens = float64(l.burst)
	}

	// Check if we have a token
	if l.tokens >= 1 {
		l.tokens--
		return true
	}

	return false
}

// AllowN checks if n requests are allowed
func (l *TokenBucketLimiter) AllowN(n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(l.lastUpdate).Seconds()
	l.lastUpdate = now

	// Add tokens
	l.tokens += elapsed * l.rate
	if l.tokens > float64(l.burst) {
		l.tokens = float64(l.burst)
	}

	// Check if we have enough tokens
	if l.tokens >= float64(n) {
		l.tokens -= float64(n)
		return true
	}

	return false
}

// Available returns available tokens
func (l *TokenBucketLimiter) Available() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(l.lastUpdate).Seconds()

	tokens := l.tokens + elapsed*l.rate
	if tokens > float64(l.burst) {
		tokens = float64(l.burst)
	}
	return tokens
}

// NewDoSProtector creates a new DoS protector
func NewDoSProtector(config *DoSProtectionConfig) *DoSProtector {
	if config == nil {
		config = DefaultDoSProtectionConfig()
	}

	p := &DoSProtector{
		config:           config,
		globalLimiter:    NewTokenBucketLimiter(float64(config.GlobalRequestsPerSecond), config.GlobalBurstSize),
		ipStates:         make(map[string]*ipState),
		connectionsPerIP: make(map[string]int32),
		bans:             make(map[string]time.Time),
		whitelist:        make(map[string]struct{}),
		blacklist:        make(map[string]struct{}),
		stopCleanup:      make(chan struct{}),
		cleanupDone:      make(chan struct{}),
	}

	// Initialize whitelist
	for _, ip := range config.WhitelistedIPs {
		p.whitelist[ip] = struct{}{}
	}

	// Initialize blacklist
	for _, ip := range config.BlacklistedIPs {
		p.blacklist[ip] = struct{}{}
	}

	// Start cleanup goroutine
	go p.cleanupLoop()

	return p
}

// CheckRequest validates a request against DoS protection rules
func (p *DoSProtector) CheckRequest(ctx context.Context, clientIP string, requestSize int64) error {
	p.stats.TotalRequests.Add(1)

	// Extract just the IP (without port)
	ip := dosExtractIP(clientIP)

	// Check blacklist first
	if _, blacklisted := p.blacklist[ip]; blacklisted {
		p.stats.BannedRequests.Add(1)
		return ErrClientBanned
	}

	// Whitelisted IPs bypass most checks
	_, whitelisted := p.whitelist[ip]

	// Check ban list
	if !whitelisted && p.isBanned(ip) {
		p.stats.BannedRequests.Add(1)
		return ErrClientBanned
	}

	// Check request size
	if requestSize > p.config.MaxRequestSize {
		p.stats.OversizedRequests.Add(1)
		p.recordViolation(ip)
		return fmt.Errorf("%w: size %d exceeds max %d", ErrRequestTooLarge, requestSize, p.config.MaxRequestSize)
	}

	// Check concurrent requests
	current := p.concurrentRequests.Load()
	if current >= int32(p.config.MaxConcurrentRequests) {
		return fmt.Errorf("%w: max concurrent requests reached", ErrResourceExhausted)
	}

	// Check global rate limit
	if !whitelisted && !p.globalLimiter.Allow() {
		p.stats.RateLimitedRequests.Add(1)
		return ErrRateLimitExceeded
	}

	// Check per-IP rate limit
	if !whitelisted {
		if err := p.checkIPRateLimit(ip); err != nil {
			p.stats.RateLimitedRequests.Add(1)
			p.recordViolation(ip)
			return err
		}
	}

	return nil
}

// checkIPRateLimit checks rate limit for a specific IP
func (p *DoSProtector) checkIPRateLimit(ip string) error {
	state := p.getOrCreateIPState(ip)

	state.mu.Lock()
	defer state.mu.Unlock()

	state.lastSeen = time.Now()

	if !state.limiter.Allow() {
		return ErrRateLimitExceeded
	}

	return nil
}

// getOrCreateIPState gets or creates state for an IP
func (p *DoSProtector) getOrCreateIPState(ip string) *ipState {
	p.ipStateMu.RLock()
	state, exists := p.ipStates[ip]
	p.ipStateMu.RUnlock()

	if exists {
		return state
	}

	p.ipStateMu.Lock()
	defer p.ipStateMu.Unlock()

	// Double-check after acquiring write lock
	if state, exists = p.ipStates[ip]; exists {
		return state
	}

	state = &ipState{
		limiter:    NewTokenBucketLimiter(float64(p.config.PerIPRequestsPerSecond), p.config.PerIPBurstSize),
		violations: make([]time.Time, 0, p.config.BanThreshold),
		lastSeen:   time.Now(),
	}
	p.ipStates[ip] = state
	p.stats.ActiveIPs.Add(1)

	return state
}

// recordViolation records a violation for an IP
func (p *DoSProtector) recordViolation(ip string) {
	state := p.getOrCreateIPState(ip)

	state.mu.Lock()
	defer state.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-p.config.ViolationWindow)

	// Remove old violations
	valid := state.violations[:0]
	for _, t := range state.violations {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	state.violations = valid

	// Add new violation
	state.violations = append(state.violations, now)

	// Check if should ban
	if len(state.violations) >= p.config.BanThreshold {
		p.ban(ip)
		state.violations = nil
	}
}

// ban adds an IP to the ban list
func (p *DoSProtector) ban(ip string) {
	p.bansMu.Lock()
	defer p.bansMu.Unlock()

	if _, exists := p.bans[ip]; !exists {
		p.stats.CurrentBannedIPs.Add(1)
	}
	p.bans[ip] = time.Now().Add(p.config.BanDuration)
}

// isBanned checks if an IP is banned
func (p *DoSProtector) isBanned(ip string) bool {
	p.bansMu.RLock()
	expiry, exists := p.bans[ip]
	p.bansMu.RUnlock()

	if !exists {
		return false
	}

	if time.Now().After(expiry) {
		// Expired, remove from ban list
		p.bansMu.Lock()
		delete(p.bans, ip)
		p.stats.CurrentBannedIPs.Add(-1)
		p.bansMu.Unlock()
		return false
	}

	return true
}

// Unban removes an IP from the ban list
func (p *DoSProtector) Unban(ip string) {
	p.bansMu.Lock()
	defer p.bansMu.Unlock()

	if _, exists := p.bans[ip]; exists {
		delete(p.bans, ip)
		p.stats.CurrentBannedIPs.Add(-1)
	}
}

// TrackConnection tracks a new connection
func (p *DoSProtector) TrackConnection(ip string) error {
	ip = dosExtractIP(ip)

	// Check blacklist
	if _, blacklisted := p.blacklist[ip]; blacklisted {
		return ErrClientBanned
	}

	// Check if banned
	if p.isBanned(ip) {
		return ErrClientBanned
	}

	// Check total connections
	if p.totalConnections.Load() >= int32(p.config.MaxTotalConnections) {
		p.stats.ConnectionsRejected.Add(1)
		return ErrConnectionLimit
	}

	// Check per-IP connections
	p.connectionsMu.Lock()
	current := p.connectionsPerIP[ip]
	if current >= int32(p.config.MaxConnectionsPerIP) {
		p.connectionsMu.Unlock()
		p.stats.ConnectionsRejected.Add(1)
		p.recordViolation(ip)
		return ErrConnectionLimit
	}
	p.connectionsPerIP[ip] = current + 1
	p.connectionsMu.Unlock()

	p.totalConnections.Add(1)
	p.stats.CurrentConnections.Add(1)

	return nil
}

// ReleaseConnection releases a tracked connection
func (p *DoSProtector) ReleaseConnection(ip string) {
	ip = dosExtractIP(ip)

	p.connectionsMu.Lock()
	if current, exists := p.connectionsPerIP[ip]; exists && current > 0 {
		p.connectionsPerIP[ip] = current - 1
		if current-1 == 0 {
			delete(p.connectionsPerIP, ip)
		}
	}
	p.connectionsMu.Unlock()

	p.totalConnections.Add(-1)
	p.stats.CurrentConnections.Add(-1)
}

// BeginRequest marks the start of request processing
func (p *DoSProtector) BeginRequest() bool {
	// Check pending queue
	if p.config.MaxPendingRequests > 0 {
		pending := p.pendingRequests.Load()
		if pending >= int32(p.config.MaxPendingRequests) {
			return false
		}
	}

	// Check concurrent requests
	if p.config.MaxConcurrentRequests > 0 {
		current := p.concurrentRequests.Load()
		if current >= int32(p.config.MaxConcurrentRequests) {
			return false
		}
	}

	p.pendingRequests.Add(1)
	p.concurrentRequests.Add(1)
	return true
}

// EndRequest marks the end of request processing
func (p *DoSProtector) EndRequest() {
	p.concurrentRequests.Add(-1)
	p.pendingRequests.Add(-1)
}

// CheckSlowClient checks if a client is too slow
func (p *DoSProtector) CheckSlowClient(ctx context.Context, clientIP string, bytesTransferred int64, duration time.Duration) error {
	// Calculate rate (bytes per second)
	if duration == 0 {
		return nil
	}

	// If transferring less than 1KB/s for more than the slow client timeout, flag it
	rate := float64(bytesTransferred) / duration.Seconds()
	if rate < 1024 && duration > p.config.SlowClientTimeout {
		ip := dosExtractIP(clientIP)
		p.stats.SlowClientRequests.Add(1)
		p.recordViolation(ip)
		return ErrSlowClientDetected
	}

	return nil
}

// AddToWhitelist adds an IP to the whitelist
func (p *DoSProtector) AddToWhitelist(ip string) {
	p.ipStateMu.Lock()
	defer p.ipStateMu.Unlock()
	p.whitelist[ip] = struct{}{}
}

// RemoveFromWhitelist removes an IP from the whitelist
func (p *DoSProtector) RemoveFromWhitelist(ip string) {
	p.ipStateMu.Lock()
	defer p.ipStateMu.Unlock()
	delete(p.whitelist, ip)
}

// AddToBlacklist adds an IP to the blacklist
func (p *DoSProtector) AddToBlacklist(ip string) {
	p.ipStateMu.Lock()
	defer p.ipStateMu.Unlock()
	p.blacklist[ip] = struct{}{}
}

// RemoveFromBlacklist removes an IP from the blacklist
func (p *DoSProtector) RemoveFromBlacklist(ip string) {
	p.ipStateMu.Lock()
	defer p.ipStateMu.Unlock()
	delete(p.blacklist, ip)
}

// GetStats returns current DoS protection statistics
func (p *DoSProtector) GetStats() map[string]interface{} {
	p.bansMu.RLock()
	bannedCount := len(p.bans)
	p.bansMu.RUnlock()

	p.ipStateMu.RLock()
	activeIPs := len(p.ipStates)
	p.ipStateMu.RUnlock()

	return map[string]interface{}{
		"total_requests":          p.stats.TotalRequests.Load(),
		"rate_limited_requests":   p.stats.RateLimitedRequests.Load(),
		"banned_requests":         p.stats.BannedRequests.Load(),
		"oversized_requests":      p.stats.OversizedRequests.Load(),
		"slow_client_requests":    p.stats.SlowClientRequests.Load(),
		"connections_rejected":    p.stats.ConnectionsRejected.Load(),
		"current_connections":     p.totalConnections.Load(),
		"current_banned_ips":      bannedCount,
		"active_ips":              activeIPs,
		"concurrent_requests":     p.concurrentRequests.Load(),
		"pending_requests":        p.pendingRequests.Load(),
		"global_tokens_available": p.globalLimiter.Available(),
	}
}

// GetBannedIPs returns list of currently banned IPs
func (p *DoSProtector) GetBannedIPs() []string {
	p.bansMu.RLock()
	defer p.bansMu.RUnlock()

	ips := make([]string, 0, len(p.bans))
	now := time.Now()
	for ip, expiry := range p.bans {
		if now.Before(expiry) {
			ips = append(ips, ip)
		}
	}
	return ips
}

// cleanupLoop periodically cleans up stale state
func (p *DoSProtector) cleanupLoop() {
	defer close(p.cleanupDone)

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCleanup:
			return
		case <-ticker.C:
			p.cleanup()
		}
	}
}

// cleanup removes stale state
func (p *DoSProtector) cleanup() {
	now := time.Now()
	staleThreshold := now.Add(-5 * time.Minute)

	// Cleanup stale IP states
	p.ipStateMu.Lock()
	for ip, state := range p.ipStates {
		state.mu.Lock()
		if state.lastSeen.Before(staleThreshold) {
			delete(p.ipStates, ip)
			p.stats.ActiveIPs.Add(-1)
		}
		state.mu.Unlock()
	}
	p.ipStateMu.Unlock()

	// Cleanup expired bans
	p.bansMu.Lock()
	for ip, expiry := range p.bans {
		if now.After(expiry) {
			delete(p.bans, ip)
			p.stats.CurrentBannedIPs.Add(-1)
		}
	}
	p.bansMu.Unlock()
}

// Close shuts down the DoS protector
func (p *DoSProtector) Close() {
	close(p.stopCleanup)
	<-p.cleanupDone
}

// dosExtractIP extracts just the IP address from an address string
func dosExtractIP(addr string) string {
	if addr == "" {
		return ""
	}

	// Try to parse as host:port
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return host
	}

	// Already just an IP
	if net.ParseIP(addr) != nil {
		return addr
	}

	return addr
}

// SizeLimitedReader wraps a reader with size limits
type SizeLimitedReader struct {
	R       net.Conn
	N       int64 // max bytes remaining
	Total   int64 // total bytes read
	Timeout time.Duration
}

// Read implements io.Reader with size and timeout limits
func (l *SizeLimitedReader) Read(p []byte) (n int, err error) {
	if l.N <= 0 {
		return 0, ErrRequestTooLarge
	}

	// Set read deadline
	if l.Timeout > 0 {
		l.R.SetReadDeadline(time.Now().Add(l.Timeout))
	}

	if int64(len(p)) > l.N {
		p = p[:l.N]
	}

	n, err = l.R.Read(p)
	l.N -= int64(n)
	l.Total += int64(n)

	return n, err
}

// RequestThrottler provides request-level throttling
type RequestThrottler struct {
	protector *DoSProtector
	clientIP  string
	startTime time.Time
	bytesRead int64
}

// NewRequestThrottler creates a new request throttler
func NewRequestThrottler(protector *DoSProtector, clientIP string) *RequestThrottler {
	return &RequestThrottler{
		protector: protector,
		clientIP:  clientIP,
		startTime: time.Now(),
	}
}

// CheckProgress checks if the request is progressing appropriately
func (t *RequestThrottler) CheckProgress(bytesRead int64) error {
	t.bytesRead = bytesRead
	duration := time.Since(t.startTime)

	return t.protector.CheckSlowClient(context.Background(), t.clientIP, bytesRead, duration)
}

// End marks the end of the request
func (t *RequestThrottler) End() {
	t.protector.EndRequest()
}

// DoSConnectionTracker tracks connections for DoS protection
type DoSConnectionTracker struct {
	protector *DoSProtector
	ip        string
	tracked   bool
}

// NewDoSConnectionTracker creates a new connection tracker
func NewDoSConnectionTracker(protector *DoSProtector, ip string) (*DoSConnectionTracker, error) {
	ct := &DoSConnectionTracker{
		protector: protector,
		ip:        ip,
		tracked:   false,
	}

	if err := protector.TrackConnection(ip); err != nil {
		return nil, err
	}
	ct.tracked = true

	return ct, nil
}

// Release releases the tracked connection
func (ct *DoSConnectionTracker) Release() {
	if ct.tracked {
		ct.protector.ReleaseConnection(ct.ip)
		ct.tracked = false
	}
}
