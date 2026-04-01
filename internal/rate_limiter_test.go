package internal

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestDefaultRateLimiterConfig(t *testing.T) {
	config := DefaultRateLimiterConfig()

	if config.GlobalRequestsPerSecond != 10000 {
		t.Errorf("Expected GlobalRequestsPerSecond 10000, got %d", config.GlobalRequestsPerSecond)
	}
	if config.PerIPRequestsPerSecond != 100 {
		t.Errorf("Expected PerIPRequestsPerSecond 100, got %d", config.PerIPRequestsPerSecond)
	}
	if config.PerCallRequestsPerSecond != 10 {
		t.Errorf("Expected PerCallRequestsPerSecond 10, got %d", config.PerCallRequestsPerSecond)
	}
}

func TestNewTokenBucket(t *testing.T) {
	tb := NewTokenBucket(10, 5)

	if tb.maxTokens != 10 {
		t.Errorf("Expected maxTokens 10, got %f", tb.maxTokens)
	}
	if tb.refillRate != 5 {
		t.Errorf("Expected refillRate 5, got %f", tb.refillRate)
	}
	if tb.tokens != 10 {
		t.Errorf("Expected initial tokens 10, got %f", tb.tokens)
	}
}

func TestTokenBucket_Allow(t *testing.T) {
	tb := NewTokenBucket(5, 10)

	// Should allow 5 requests immediately (burst)
	for i := 0; i < 5; i++ {
		if !tb.Allow() {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 6th request should be denied
	if tb.Allow() {
		t.Error("6th request should be denied")
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	tb := NewTokenBucket(5, 100) // 100 tokens/sec

	// Use all tokens
	for i := 0; i < 5; i++ {
		tb.Allow()
	}

	// Wait for refill
	time.Sleep(60 * time.Millisecond)

	// Should have some tokens now
	if !tb.Allow() {
		t.Error("Should have refilled some tokens")
	}
}

func TestNewRateLimiter(t *testing.T) {
	config := &RateLimiterConfig{
		GlobalRequestsPerSecond:  100,
		GlobalBurstSize:          10,
		PerIPRequestsPerSecond:   10,
		PerIPBurstSize:           5,
		PerCallRequestsPerSecond: 5,
		PerCallBurstSize:         2,
		CleanupInterval:          1 * time.Minute,
		EntryTTL:                 5 * time.Minute,
		BlockDuration:            10 * time.Second,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	if rl.globalBucket == nil {
		t.Error("Global bucket should be created")
	}
	if rl.ipBuckets == nil {
		t.Error("IP buckets map should be initialized")
	}
	if rl.callBuckets == nil {
		t.Error("Call buckets map should be initialized")
	}
}

func TestNewRateLimiter_DefaultConfig(t *testing.T) {
	rl := NewRateLimiter(nil)
	defer rl.Stop()

	if rl.config.GlobalRequestsPerSecond != 10000 {
		t.Errorf("Expected default global limit 10000, got %d", rl.config.GlobalRequestsPerSecond)
	}
}

func TestRateLimiter_AllowBasic(t *testing.T) {
	config := &RateLimiterConfig{
		GlobalRequestsPerSecond:  1000,
		GlobalBurstSize:          100,
		PerIPRequestsPerSecond:   100,
		PerIPBurstSize:           50,
		PerCallRequestsPerSecond: 0, // Disable call limit
		CleanupInterval:          1 * time.Hour,
		EntryTTL:                 1 * time.Hour,
		BlockDuration:            10 * time.Second,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	// Should allow requests
	for i := 0; i < 10; i++ {
		if !rl.Allow("192.168.1.1", "") {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiter_PerIPLimit(t *testing.T) {
	config := &RateLimiterConfig{
		GlobalRequestsPerSecond:  10000,
		GlobalBurstSize:          1000,
		PerIPRequestsPerSecond:   10,
		PerIPBurstSize:           5,
		PerCallRequestsPerSecond: 0,
		CleanupInterval:          1 * time.Hour,
		EntryTTL:                 1 * time.Hour,
		BlockDuration:            1 * time.Hour, // Long block to avoid auto-blocking
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	// Use up burst for IP1
	for i := 0; i < 5; i++ {
		rl.Allow("192.168.1.1", "")
	}

	// IP1 should be rate limited
	if rl.Allow("192.168.1.1", "") {
		t.Error("IP1 should be rate limited after burst")
	}

	// IP2 should still be allowed
	if !rl.Allow("192.168.1.2", "") {
		t.Error("IP2 should be allowed")
	}
}

func TestRateLimiter_PerCallLimit(t *testing.T) {
	config := &RateLimiterConfig{
		GlobalRequestsPerSecond:  10000,
		GlobalBurstSize:          1000,
		PerIPRequestsPerSecond:   0, // Disable IP limit
		PerCallRequestsPerSecond: 5,
		PerCallBurstSize:         3,
		CleanupInterval:          1 * time.Hour,
		EntryTTL:                 1 * time.Hour,
		BlockDuration:            10 * time.Second,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	// Use up burst for call1
	for i := 0; i < 3; i++ {
		rl.Allow("192.168.1.1", "call-1")
	}

	// call1 should be rate limited
	if rl.Allow("192.168.1.1", "call-1") {
		t.Error("call-1 should be rate limited after burst")
	}

	// call2 should still be allowed
	if !rl.Allow("192.168.1.1", "call-2") {
		t.Error("call-2 should be allowed")
	}
}

func TestRateLimiter_BlockIP(t *testing.T) {
	config := &RateLimiterConfig{
		GlobalRequestsPerSecond: 10000,
		GlobalBurstSize:         1000,
		PerIPRequestsPerSecond:  0,
		CleanupInterval:         1 * time.Hour,
		EntryTTL:                1 * time.Hour,
		BlockDuration:           1 * time.Second,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	// Block IP
	rl.BlockIP("192.168.1.100")

	// Should be blocked
	if rl.Allow("192.168.1.100", "") {
		t.Error("Blocked IP should be denied")
	}

	// Other IPs should be allowed
	if !rl.Allow("192.168.1.101", "") {
		t.Error("Non-blocked IP should be allowed")
	}

	// Wait for block to expire
	time.Sleep(1100 * time.Millisecond)

	// Should be unblocked now
	if !rl.Allow("192.168.1.100", "") {
		t.Error("IP should be unblocked after duration")
	}
}

func TestRateLimiter_UnblockIP(t *testing.T) {
	config := &RateLimiterConfig{
		GlobalRequestsPerSecond: 10000,
		GlobalBurstSize:         1000,
		PerIPRequestsPerSecond:  0,
		CleanupInterval:         1 * time.Hour,
		EntryTTL:                1 * time.Hour,
		BlockDuration:           1 * time.Hour,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	rl.BlockIP("192.168.1.100")
	if rl.Allow("192.168.1.100", "") {
		t.Error("Should be blocked")
	}

	rl.UnblockIP("192.168.1.100")
	if !rl.Allow("192.168.1.100", "") {
		t.Error("Should be unblocked")
	}
}

func TestRateLimiter_GetStats(t *testing.T) {
	config := DefaultRateLimiterConfig()
	rl := NewRateLimiter(config)
	defer rl.Stop()

	// Make some requests
	rl.Allow("192.168.1.1", "call-1")
	rl.Allow("192.168.1.2", "call-2")

	stats := rl.GetStats()

	if stats["total_requests"].(uint64) != 2 {
		t.Errorf("Expected total_requests 2, got %v", stats["total_requests"])
	}
	if stats["allowed_requests"].(uint64) != 2 {
		t.Errorf("Expected allowed_requests 2, got %v", stats["allowed_requests"])
	}
}

func TestRateLimiter_GetBlockedIPs(t *testing.T) {
	config := DefaultRateLimiterConfig()
	rl := NewRateLimiter(config)
	defer rl.Stop()

	rl.BlockIP("192.168.1.1")
	rl.BlockIP("192.168.1.2")

	blocked := rl.GetBlockedIPs()
	if len(blocked) != 2 {
		t.Errorf("Expected 2 blocked IPs, got %d", len(blocked))
	}
}

func TestRateLimiter_Reset(t *testing.T) {
	config := DefaultRateLimiterConfig()
	rl := NewRateLimiter(config)
	defer rl.Stop()

	rl.Allow("192.168.1.1", "call-1")
	rl.BlockIP("192.168.1.2")

	rl.Reset()

	stats := rl.GetStats()
	if stats["active_ips"].(int64) != 0 {
		t.Error("Active IPs should be 0 after reset")
	}
	if stats["blocked_ips"].(int64) != 0 {
		t.Error("Blocked IPs should be 0 after reset")
	}
}

func TestRateLimiter_Cleanup(t *testing.T) {
	config := &RateLimiterConfig{
		GlobalRequestsPerSecond: 10000,
		GlobalBurstSize:         1000,
		PerIPRequestsPerSecond:  100,
		PerIPBurstSize:          50,
		CleanupInterval:         50 * time.Millisecond,
		EntryTTL:                100 * time.Millisecond,
		BlockDuration:           100 * time.Millisecond,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	rl.Allow("192.168.1.1", "")
	rl.BlockIP("192.168.1.2")

	stats := rl.GetStats()
	if stats["active_ips"].(int64) != 1 {
		t.Error("Should have 1 active IP")
	}

	// Wait for cleanup
	time.Sleep(200 * time.Millisecond)

	stats = rl.GetStats()
	if stats["active_ips"].(int64) != 0 {
		t.Error("Should have 0 active IPs after cleanup")
	}
	if stats["blocked_ips"].(int64) != 0 {
		t.Error("Should have 0 blocked IPs after cleanup")
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	config := &RateLimiterConfig{
		GlobalRequestsPerSecond:  100000,
		GlobalBurstSize:          10000,
		PerIPRequestsPerSecond:   1000,
		PerIPBurstSize:           500,
		PerCallRequestsPerSecond: 100,
		PerCallBurstSize:         50,
		CleanupInterval:          1 * time.Hour,
		EntryTTL:                 1 * time.Hour,
		BlockDuration:            1 * time.Hour,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ip := "192.168.1." + string(rune('0'+id%10))
			callID := "call-" + string(rune('0'+id%5))

			for j := 0; j < 100; j++ {
				rl.Allow(ip, callID)
			}
		}(i)
	}

	wg.Wait()
}

func TestIPRateLimiter(t *testing.T) {
	rl := NewIPRateLimiter(10, 5)

	// Should allow burst
	for i := 0; i < 5; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// Should be rate limited
	if rl.Allow("192.168.1.1") {
		t.Error("Should be rate limited after burst")
	}

	// Different IP should be allowed
	if !rl.Allow("192.168.1.2") {
		t.Error("Different IP should be allowed")
	}
}

func TestIPRateLimiter_AllowAddr(t *testing.T) {
	rl := NewIPRateLimiter(10, 5)

	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 5060}

	for i := 0; i < 5; i++ {
		if !rl.AllowAddr(addr) {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	if rl.AllowAddr(addr) {
		t.Error("Should be rate limited")
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		addr     net.Addr
		expected string
	}{
		{&net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 5060}, "192.168.1.1"},
		{&net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 80}, "10.0.0.1"},
	}

	for _, tt := range tests {
		result := extractIP(tt.addr)
		if result != tt.expected {
			t.Errorf("extractIP(%v) = %s, expected %s", tt.addr, result, tt.expected)
		}
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	config := &RateLimiterConfig{
		GlobalRequestsPerSecond: 10000,
		GlobalBurstSize:         1000,
		PerIPRequestsPerSecond:  5,
		PerIPBurstSize:          3,
		CleanupInterval:         1 * time.Hour,
		EntryTTL:                1 * time.Hour,
		BlockDuration:           1 * time.Hour,
	}

	rl := NewRateLimiter(config)
	defer rl.Stop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := RateLimitMiddleware(rl)
	wrappedHandler := middleware(handler)

	// Make requests within limit
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d should return 200, got %d", i+1, w.Code)
		}
	}

	// Exceed limit
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Should return 429, got %d", w.Code)
	}
}

func TestExtractIPFromRequest(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expected   string
	}{
		{
			name:       "X-Forwarded-For",
			headers:    map[string]string{"X-Forwarded-For": "10.0.0.1, 192.168.1.1"},
			remoteAddr: "127.0.0.1:12345",
			expected:   "10.0.0.1",
		},
		{
			name:       "X-Real-IP",
			headers:    map[string]string{"X-Real-IP": "10.0.0.2"},
			remoteAddr: "127.0.0.1:12345",
			expected:   "10.0.0.2",
		},
		{
			name:       "RemoteAddr",
			headers:    map[string]string{},
			remoteAddr: "192.168.1.100:54321",
			expected:   "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			result := extractIPFromRequest(req)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestRateLimitError(t *testing.T) {
	err := RateLimitError("192.168.1.1")
	if err == nil {
		t.Error("Expected non-nil error")
	}
	if err.Error() != "rate limit exceeded for IP 192.168.1.1" {
		t.Errorf("Unexpected error message: %s", err.Error())
	}
}
