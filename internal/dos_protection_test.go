package internal

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestDefaultDoSProtectionConfig(t *testing.T) {
	config := DefaultDoSProtectionConfig()

	if config.GlobalRequestsPerSecond != 10000 {
		t.Errorf("Expected GlobalRequestsPerSecond 10000, got %d", config.GlobalRequestsPerSecond)
	}
	if config.MaxRequestSize != 1024*1024 {
		t.Errorf("Expected MaxRequestSize 1MB, got %d", config.MaxRequestSize)
	}
	if config.BanDuration != 15*time.Minute {
		t.Errorf("Expected BanDuration 15m, got %v", config.BanDuration)
	}
}

func TestNewDoSProtector(t *testing.T) {
	p := NewDoSProtector(nil)
	defer p.Close()

	if p == nil {
		t.Fatal("NewDoSProtector returned nil")
	}
	if p.globalLimiter == nil {
		t.Error("Global limiter not initialized")
	}
}

func TestDoSProtector_CheckRequest_Basic(t *testing.T) {
	config := &DoSProtectionConfig{
		GlobalRequestsPerSecond:  1000,
		GlobalBurstSize:          100,
		PerIPRequestsPerSecond:   50,
		PerIPBurstSize:           25,
		MaxRequestSize:           1024,
		MaxConcurrentRequests:    1000,
		MaxPendingRequests:       2000,
	}
	p := NewDoSProtector(config)
	defer p.Close()

	ctx := context.Background()

	// Normal request should pass
	err := p.CheckRequest(ctx, "192.168.1.1:1234", 100)
	if err != nil {
		t.Errorf("Normal request should pass: %v", err)
	}

	// Request too large should fail
	err = p.CheckRequest(ctx, "192.168.1.1:1234", 2048)
	if err == nil {
		t.Error("Oversized request should fail")
	}
}

func TestDoSProtector_CheckRequest_RateLimit(t *testing.T) {
	config := &DoSProtectionConfig{
		GlobalRequestsPerSecond:  1000,
		GlobalBurstSize:          5,
		PerIPRequestsPerSecond:   10,
		PerIPBurstSize:           3, // Small burst for testing
		MaxRequestSize:           1024,
		BanThreshold:             100, // High threshold to prevent banning
		ViolationWindow:          time.Minute,
		MaxConcurrentRequests:    1000,
		MaxPendingRequests:       2000,
	}
	p := NewDoSProtector(config)
	defer p.Close()

	ctx := context.Background()

	// Exhaust per-IP burst
	for i := 0; i < 3; i++ {
		err := p.CheckRequest(ctx, "192.168.1.1:1234", 10)
		if err != nil {
			t.Errorf("Request %d should pass: %v", i, err)
		}
	}

	// Next request should be rate limited
	err := p.CheckRequest(ctx, "192.168.1.1:1234", 10)
	if err == nil {
		t.Error("Request should be rate limited after burst exhausted")
	}

	// Different IP should still work
	err = p.CheckRequest(ctx, "192.168.1.2:1234", 10)
	if err != nil {
		t.Errorf("Different IP should not be rate limited: %v", err)
	}
}

func TestDoSProtector_Blacklist(t *testing.T) {
	config := &DoSProtectionConfig{
		GlobalRequestsPerSecond:  1000,
		GlobalBurstSize:          100,
		PerIPRequestsPerSecond:   50,
		PerIPBurstSize:           25,
		MaxRequestSize:           1024,
		BlacklistedIPs:           []string{"10.0.0.1"},
		MaxConcurrentRequests:    1000,
		MaxPendingRequests:       2000,
	}
	p := NewDoSProtector(config)
	defer p.Close()

	ctx := context.Background()

	// Blacklisted IP should fail
	err := p.CheckRequest(ctx, "10.0.0.1:1234", 10)
	if err == nil {
		t.Error("Blacklisted IP should be rejected")
	}
	if err != ErrClientBanned {
		t.Errorf("Expected ErrClientBanned, got %v", err)
	}

	// Non-blacklisted should pass
	err = p.CheckRequest(ctx, "10.0.0.2:1234", 10)
	if err != nil {
		t.Errorf("Non-blacklisted IP should pass: %v", err)
	}
}

func TestDoSProtector_Whitelist(t *testing.T) {
	config := &DoSProtectionConfig{
		GlobalRequestsPerSecond:  1,
		GlobalBurstSize:          1, // Very restrictive
		PerIPRequestsPerSecond:   1,
		PerIPBurstSize:           1,
		MaxRequestSize:           1024,
		WhitelistedIPs:           []string{"192.168.1.100"},
		MaxConcurrentRequests:    1000,
		MaxPendingRequests:       2000,
	}
	p := NewDoSProtector(config)
	defer p.Close()

	ctx := context.Background()

	// Exhaust the rate limit for a normal IP
	p.CheckRequest(ctx, "192.168.1.1:1234", 10)

	// Whitelisted IP should still work even with exhausted rate limit
	for i := 0; i < 5; i++ {
		err := p.CheckRequest(ctx, "192.168.1.100:1234", 10)
		if err != nil {
			t.Errorf("Whitelisted IP should bypass rate limit: %v", err)
		}
	}
}

func TestDoSProtector_Ban(t *testing.T) {
	config := &DoSProtectionConfig{
		GlobalRequestsPerSecond:  1000,
		GlobalBurstSize:          100,
		PerIPRequestsPerSecond:   100,
		PerIPBurstSize:           50,
		MaxRequestSize:           100, // Small for testing
		BanThreshold:             3,   // Ban after 3 violations
		BanDuration:              time.Hour,
		ViolationWindow:          time.Minute,
		MaxConcurrentRequests:    1000,
		MaxPendingRequests:       2000,
	}
	p := NewDoSProtector(config)
	defer p.Close()

	ctx := context.Background()
	testIP := "192.168.1.50"

	// Trigger violations with oversized requests
	for i := 0; i < 3; i++ {
		p.CheckRequest(ctx, testIP+":1234", 200)
	}

	// IP should now be banned
	err := p.CheckRequest(ctx, testIP+":1234", 10)
	if err != ErrClientBanned {
		t.Errorf("Expected ErrClientBanned after violations, got %v", err)
	}

	// Verify in banned list
	banned := p.GetBannedIPs()
	found := false
	for _, ip := range banned {
		if ip == testIP {
			found = true
			break
		}
	}
	if !found {
		t.Error("IP should be in banned list")
	}

	// Unban and verify
	p.Unban(testIP)
	err = p.CheckRequest(ctx, testIP+":1234", 10)
	if err != nil {
		t.Errorf("Unbanned IP should be allowed: %v", err)
	}
}

func TestDoSProtector_ConnectionTracking(t *testing.T) {
	config := &DoSProtectionConfig{
		MaxConnectionsPerIP: 3,
		MaxTotalConnections: 100,
		BanThreshold:        100,
		ViolationWindow:     time.Minute,
	}
	p := NewDoSProtector(config)
	defer p.Close()

	testIP := "192.168.1.1:1234"

	// Track connections up to limit
	for i := 0; i < 3; i++ {
		err := p.TrackConnection(testIP)
		if err != nil {
			t.Errorf("Connection %d should be allowed: %v", i, err)
		}
	}

	// Next connection should fail
	err := p.TrackConnection(testIP)
	if err != ErrConnectionLimit {
		t.Errorf("Expected ErrConnectionLimit, got %v", err)
	}

	// Release a connection
	p.ReleaseConnection(testIP)

	// Now should work again
	err = p.TrackConnection(testIP)
	if err != nil {
		t.Errorf("Connection after release should work: %v", err)
	}
}

func TestDoSProtector_TotalConnectionLimit(t *testing.T) {
	config := &DoSProtectionConfig{
		MaxConnectionsPerIP: 100,
		MaxTotalConnections: 3,
		BanThreshold:        100,
		ViolationWindow:     time.Minute,
	}
	p := NewDoSProtector(config)
	defer p.Close()

	// Track connections from different IPs
	for i := 0; i < 3; i++ {
		err := p.TrackConnection("192.168.1." + string(rune('0'+i)) + ":1234")
		if err != nil {
			t.Errorf("Connection %d should be allowed: %v", i, err)
		}
	}

	// Total limit reached
	err := p.TrackConnection("192.168.1.99:1234")
	if err != ErrConnectionLimit {
		t.Errorf("Expected ErrConnectionLimit for total limit, got %v", err)
	}
}

func TestDoSProtector_ConcurrentRequests(t *testing.T) {
	config := &DoSProtectionConfig{
		GlobalRequestsPerSecond: 10000,
		GlobalBurstSize:         1000,
		PerIPRequestsPerSecond:  1000,
		PerIPBurstSize:          500,
		MaxConcurrentRequests:   10,
		MaxPendingRequests:      20,
	}
	p := NewDoSProtector(config)
	defer p.Close()

	// Start max concurrent requests
	for i := 0; i < 10; i++ {
		if !p.BeginRequest() {
			t.Errorf("Request %d should be allowed", i)
		}
	}

	// Next should fail
	if p.BeginRequest() {
		t.Error("Should reject when at max concurrent requests")
	}

	// End a request
	p.EndRequest()

	// Now should work
	if !p.BeginRequest() {
		t.Error("Should allow after ending a request")
	}
}

func TestDoSProtector_SlowClient(t *testing.T) {
	config := &DoSProtectionConfig{
		SlowClientTimeout: 100 * time.Millisecond,
		BanThreshold:      100, // High to prevent banning
		ViolationWindow:   time.Minute,
	}
	p := NewDoSProtector(config)
	defer p.Close()

	ctx := context.Background()

	// Fast client should pass
	err := p.CheckSlowClient(ctx, "192.168.1.1", 10000, 1*time.Millisecond)
	if err != nil {
		t.Errorf("Fast client should pass: %v", err)
	}

	// Slow client should be detected
	err = p.CheckSlowClient(ctx, "192.168.1.2", 100, 200*time.Millisecond)
	if err != ErrSlowClientDetected {
		t.Errorf("Expected ErrSlowClientDetected, got %v", err)
	}
}

func TestDoSProtector_GetStats(t *testing.T) {
	p := NewDoSProtector(nil)
	defer p.Close()

	ctx := context.Background()
	p.CheckRequest(ctx, "192.168.1.1:1234", 100)

	stats := p.GetStats()

	if stats["total_requests"].(int64) != 1 {
		t.Error("Total requests should be 1")
	}
	if _, ok := stats["current_connections"]; !ok {
		t.Error("Stats should include current_connections")
	}
	if _, ok := stats["global_tokens_available"]; !ok {
		t.Error("Stats should include global_tokens_available")
	}
}

func TestDoSProtector_DynamicWhiteBlacklist(t *testing.T) {
	p := NewDoSProtector(nil)
	defer p.Close()

	ctx := context.Background()
	testIP := "192.168.1.50"

	// Add to blacklist
	p.AddToBlacklist(testIP)

	err := p.CheckRequest(ctx, testIP+":1234", 10)
	if err != ErrClientBanned {
		t.Error("Dynamically blacklisted IP should be rejected")
	}

	// Remove from blacklist
	p.RemoveFromBlacklist(testIP)

	err = p.CheckRequest(ctx, testIP+":1234", 10)
	if err != nil {
		t.Errorf("Removed from blacklist should be allowed: %v", err)
	}

	// Add to whitelist
	p.AddToWhitelist(testIP)

	// Should bypass rate limits now
	for i := 0; i < 100; i++ {
		err = p.CheckRequest(ctx, testIP+":1234", 10)
		if err != nil {
			t.Errorf("Whitelisted IP should bypass limits: %v", err)
		}
	}
}

func TestDoSProtector_ConcurrentAccess(t *testing.T) {
	p := NewDoSProtector(&DoSProtectionConfig{
		GlobalRequestsPerSecond: 100000,
		GlobalBurstSize:         10000,
		PerIPRequestsPerSecond:  10000,
		PerIPBurstSize:          1000,
		MaxRequestSize:          1024 * 1024,
		MaxConnectionsPerIP:     1000,
		MaxTotalConnections:     10000,
		BanThreshold:            1000,
		ViolationWindow:         time.Minute,
	})
	defer p.Close()

	ctx := context.Background()
	var wg sync.WaitGroup

	// Concurrent requests from multiple IPs
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ip := "192.168.1." + string(rune('0'+idx)) + ":1234"
			for j := 0; j < 100; j++ {
				p.CheckRequest(ctx, ip, 100)
			}
		}(i)
	}

	// Concurrent connection tracking
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ip := "192.168.2." + string(rune('0'+idx)) + ":1234"
			for j := 0; j < 50; j++ {
				p.TrackConnection(ip)
				p.ReleaseConnection(ip)
			}
		}(i)
	}

	wg.Wait()

	// Verify stats are consistent
	stats := p.GetStats()
	if stats["current_connections"].(int32) < 0 {
		t.Error("Current connections should not be negative")
	}
}

func TestTokenBucketLimiter_Allow(t *testing.T) {
	l := NewTokenBucketLimiter(10, 5) // 10 per second, burst of 5

	// Should allow burst
	for i := 0; i < 5; i++ {
		if !l.Allow() {
			t.Errorf("Allow() should return true for request %d", i)
		}
	}

	// Burst exhausted
	if l.Allow() {
		t.Error("Allow() should return false when burst exhausted")
	}

	// Wait for refill
	time.Sleep(200 * time.Millisecond)

	// Should have ~2 tokens now
	if !l.Allow() {
		t.Error("Allow() should return true after refill")
	}
}

func TestTokenBucketLimiter_AllowN(t *testing.T) {
	l := NewTokenBucketLimiter(100, 10)

	// Should allow 5 at once
	if !l.AllowN(5) {
		t.Error("AllowN(5) should return true")
	}

	// Should allow another 5
	if !l.AllowN(5) {
		t.Error("AllowN(5) should return true")
	}

	// Should not allow 5 more (burst exhausted)
	if l.AllowN(5) {
		t.Error("AllowN(5) should return false when burst exhausted")
	}
}

func TestTokenBucketLimiter_Available(t *testing.T) {
	l := NewTokenBucketLimiter(10, 5)

	available := l.Available()
	if available != 5 {
		t.Errorf("Expected 5 available tokens, got %f", available)
	}

	l.Allow()
	l.Allow()

	available = l.Available()
	if available < 2.9 || available > 3.1 {
		t.Errorf("Expected ~3 available tokens, got %f", available)
	}
}

func TestDosExtractIP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"192.168.1.1:1234", "192.168.1.1"},
		{"192.168.1.1", "192.168.1.1"},
		{"[::1]:8080", "::1"},
		{"::1", "::1"},
		{"", ""},
		{"invalid", "invalid"},
	}

	for _, tt := range tests {
		result := dosExtractIP(tt.input)
		if result != tt.expected {
			t.Errorf("dosExtractIP(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestDoSConnectionTracker(t *testing.T) {
	p := NewDoSProtector(&DoSProtectionConfig{
		MaxConnectionsPerIP: 10,
		MaxTotalConnections: 100,
	})
	defer p.Close()

	// Create tracker
	ct, err := NewDoSConnectionTracker(p, "192.168.1.1:1234")
	if err != nil {
		t.Fatalf("NewConnectionTracker failed: %v", err)
	}

	stats := p.GetStats()
	if stats["current_connections"].(int32) != 1 {
		t.Error("Should have 1 connection tracked")
	}

	// Release
	ct.Release()

	stats = p.GetStats()
	if stats["current_connections"].(int32) != 0 {
		t.Error("Should have 0 connections after release")
	}

	// Double release should be safe
	ct.Release()
}

func TestRequestThrottler(t *testing.T) {
	config := &DoSProtectionConfig{
		SlowClientTimeout:     50 * time.Millisecond,
		MaxConcurrentRequests: 100,
		MaxPendingRequests:    200,
		BanThreshold:          100,
		ViolationWindow:       time.Minute,
	}
	p := NewDoSProtector(config)
	defer p.Close()

	p.BeginRequest()
	throttler := NewRequestThrottler(p, "192.168.1.1:1234")

	// Fast progress should be fine
	err := throttler.CheckProgress(10000)
	if err != nil {
		t.Errorf("Fast progress should not error: %v", err)
	}

	// Simulate slow progress
	time.Sleep(100 * time.Millisecond)
	err = throttler.CheckProgress(50) // Only 50 bytes in 100ms
	if err != ErrSlowClientDetected {
		t.Errorf("Expected ErrSlowClientDetected, got %v", err)
	}

	throttler.End()
}

func TestDoSProtector_Cleanup(t *testing.T) {
	config := &DoSProtectionConfig{
		GlobalRequestsPerSecond:  1000,
		GlobalBurstSize:          100,
		PerIPRequestsPerSecond:   100,
		PerIPBurstSize:           50,
		MaxRequestSize:           1024 * 1024,
		MaxConcurrentRequests:    1000,
		MaxPendingRequests:       2000,
	}
	p := NewDoSProtector(config)

	ctx := context.Background()

	// Generate some state
	for i := 0; i < 10; i++ {
		p.CheckRequest(ctx, "192.168.1."+string(rune('0'+i))+":1234", 100)
	}

	// Run cleanup manually
	p.cleanup()

	// Should still work after cleanup
	err := p.CheckRequest(ctx, "192.168.1.1:1234", 100)
	if err != nil {
		t.Errorf("Request after cleanup should work: %v", err)
	}

	p.Close()
}

func TestSizeLimitedReader(t *testing.T) {
	// This is a placeholder test since SizeLimitedReader requires a net.Conn
	// In a real implementation, you would mock the connection
	t.Skip("Requires net.Conn mock")
}
