package internal

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

func TestNewDNSResolver(t *testing.T) {
	r := NewDNSResolver()
	if r == nil {
		t.Fatal("NewDNSResolver returned nil")
	}
	if r.resolver == nil {
		t.Error("resolver not initialized")
	}
	if r.cache == nil {
		t.Error("cache not initialized")
	}
}

func TestDNSResolver_SetPreferIPv6(t *testing.T) {
	r := NewDNSResolver()

	if r.preferIPv6 {
		t.Error("preferIPv6 should default to false")
	}

	r.SetPreferIPv6(true)
	if !r.preferIPv6 {
		t.Error("preferIPv6 should be true after SetPreferIPv6(true)")
	}

	r.SetPreferIPv6(false)
	if r.preferIPv6 {
		t.Error("preferIPv6 should be false after SetPreferIPv6(false)")
	}
}

func TestDNSResolver_SetCacheMaxAge(t *testing.T) {
	r := NewDNSResolver()

	originalAge := r.cacheMaxAge
	newAge := 10 * time.Minute

	r.SetCacheMaxAge(newAge)
	if r.cacheMaxAge != newAge {
		t.Errorf("Expected cache max age %v, got %v", newAge, r.cacheMaxAge)
	}

	// Restore
	r.SetCacheMaxAge(originalAge)
}

func TestDNSResolver_ResolveHost_IPAddress(t *testing.T) {
	r := NewDNSResolver()
	ctx := context.Background()

	// Test IPv4 address
	records, err := r.ResolveHost(ctx, "192.168.1.1", 5060)
	if err != nil {
		t.Fatalf("ResolveHost failed for IPv4: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("Expected 1 record, got %d", len(records))
	}
	if records[0].IP.String() != "192.168.1.1" {
		t.Errorf("Expected IP 192.168.1.1, got %s", records[0].IP.String())
	}
	if records[0].Family != "ipv4" {
		t.Errorf("Expected family ipv4, got %s", records[0].Family)
	}
	if records[0].Port != 5060 {
		t.Errorf("Expected port 5060, got %d", records[0].Port)
	}

	// Test IPv6 address
	records, err = r.ResolveHost(ctx, "::1", 5060)
	if err != nil {
		t.Fatalf("ResolveHost failed for IPv6: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("Expected 1 record, got %d", len(records))
	}
	if records[0].Family != "ipv6" {
		t.Errorf("Expected family ipv6, got %s", records[0].Family)
	}
}

func TestDNSResolver_ResolveHost_Localhost(t *testing.T) {
	r := NewDNSResolver()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	records, err := r.ResolveHost(ctx, "localhost", 5060)
	if err != nil {
		t.Fatalf("ResolveHost failed for localhost: %v", err)
	}
	if len(records) == 0 {
		t.Error("Expected at least one record for localhost")
	}

	// localhost should resolve to 127.0.0.1 or ::1
	foundLoopback := false
	for _, rec := range records {
		if rec.IP.IsLoopback() {
			foundLoopback = true
			break
		}
	}
	if !foundLoopback {
		t.Error("Expected loopback address for localhost")
	}
}

func TestDNSResolver_ResolveHost_Nonexistent(t *testing.T) {
	r := NewDNSResolver()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.ResolveHost(ctx, "this-domain-should-not-exist-12345.invalid", 5060)
	if err == nil {
		t.Error("Expected error for nonexistent domain")
	}
}

func TestDNSResolver_Caching(t *testing.T) {
	r := NewDNSResolver()
	r.SetCacheMaxAge(1 * time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First resolution - use localhost which requires actual DNS lookup
	records1, err := r.ResolveHost(ctx, "localhost", 5060)
	if err != nil {
		t.Fatalf("First resolution failed: %v", err)
	}

	// Second resolution should come from cache
	records2, err := r.ResolveHost(ctx, "localhost", 5060)
	if err != nil {
		t.Fatalf("Second resolution failed: %v", err)
	}

	if len(records1) != len(records2) {
		t.Error("Cached records differ from original")
	}

	// Verify cache entry exists
	stats := r.GetStats()
	if stats["cache_entries"].(int) == 0 {
		t.Error("Expected cache entries > 0")
	}
}

func TestDNSResolver_ClearCache(t *testing.T) {
	r := NewDNSResolver()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Add something to cache - use localhost which requires actual DNS lookup
	r.ResolveHost(ctx, "localhost", 5060)

	// Verify cache has entries
	stats := r.GetStats()
	if stats["cache_entries"].(int) == 0 {
		t.Error("Expected cache to have entries")
	}

	// Clear cache
	r.ClearCache()

	// Verify cache is empty
	stats = r.GetStats()
	if stats["cache_entries"].(int) != 0 {
		t.Errorf("Expected cache to be empty, got %d entries", stats["cache_entries"].(int))
	}
}

func TestDNSResolver_GetStats(t *testing.T) {
	r := NewDNSResolver()
	r.SetPreferIPv6(true)
	r.SetCacheMaxAge(10 * time.Minute)

	stats := r.GetStats()

	if _, ok := stats["cache_entries"]; !ok {
		t.Error("Missing cache_entries in stats")
	}
	if _, ok := stats["valid_cache_entries"]; !ok {
		t.Error("Missing valid_cache_entries in stats")
	}
	if prefer, ok := stats["prefer_ipv6"].(bool); !ok || !prefer {
		t.Error("prefer_ipv6 should be true")
	}
	if age, ok := stats["cache_max_age_secs"].(float64); !ok || age != 600.0 {
		t.Errorf("Expected cache_max_age_secs 600, got %v", age)
	}
}

func TestDNSResolver_SortByPreference_IPv4First(t *testing.T) {
	r := NewDNSResolver()
	r.SetPreferIPv6(false)

	records := []*DNSRecord{
		{IP: net.ParseIP("::1"), Family: "ipv6", Priority: 10},
		{IP: net.ParseIP("127.0.0.1"), Family: "ipv4", Priority: 10},
		{IP: net.ParseIP("::2"), Family: "ipv6", Priority: 20},
		{IP: net.ParseIP("127.0.0.2"), Family: "ipv4", Priority: 20},
	}

	r.sortByPreference(records)

	if records[0].Family != "ipv4" {
		t.Error("Expected IPv4 first when preferIPv6 is false")
	}
	if records[1].Family != "ipv4" {
		t.Error("Expected IPv4 second when preferIPv6 is false")
	}
}

func TestDNSResolver_SortByPreference_IPv6First(t *testing.T) {
	r := NewDNSResolver()
	r.SetPreferIPv6(true)

	records := []*DNSRecord{
		{IP: net.ParseIP("127.0.0.1"), Family: "ipv4", Priority: 10},
		{IP: net.ParseIP("::1"), Family: "ipv6", Priority: 10},
		{IP: net.ParseIP("127.0.0.2"), Family: "ipv4", Priority: 20},
		{IP: net.ParseIP("::2"), Family: "ipv6", Priority: 20},
	}

	r.sortByPreference(records)

	if records[0].Family != "ipv6" {
		t.Error("Expected IPv6 first when preferIPv6 is true")
	}
	if records[1].Family != "ipv6" {
		t.Error("Expected IPv6 second when preferIPv6 is true")
	}
}

func TestDNSResolver_SortByPreference_ByPriority(t *testing.T) {
	r := NewDNSResolver()
	r.SetPreferIPv6(false)

	records := []*DNSRecord{
		{IP: net.ParseIP("127.0.0.2"), Family: "ipv4", Priority: 20, Weight: 10},
		{IP: net.ParseIP("127.0.0.1"), Family: "ipv4", Priority: 10, Weight: 10},
		{IP: net.ParseIP("127.0.0.3"), Family: "ipv4", Priority: 30, Weight: 10},
	}

	r.sortByPreference(records)

	if records[0].Priority != 10 {
		t.Errorf("Expected priority 10 first, got %d", records[0].Priority)
	}
	if records[1].Priority != 20 {
		t.Errorf("Expected priority 20 second, got %d", records[1].Priority)
	}
	if records[2].Priority != 30 {
		t.Errorf("Expected priority 30 third, got %d", records[2].Priority)
	}
}

func TestDNSResolver_SortByPreference_ByWeight(t *testing.T) {
	r := NewDNSResolver()

	records := []*DNSRecord{
		{IP: net.ParseIP("127.0.0.1"), Family: "ipv4", Priority: 10, Weight: 10},
		{IP: net.ParseIP("127.0.0.2"), Family: "ipv4", Priority: 10, Weight: 50},
		{IP: net.ParseIP("127.0.0.3"), Family: "ipv4", Priority: 10, Weight: 30},
	}

	r.sortByPreference(records)

	// Higher weight should come first
	if records[0].Weight != 50 {
		t.Errorf("Expected weight 50 first, got %d", records[0].Weight)
	}
	if records[1].Weight != 30 {
		t.Errorf("Expected weight 30 second, got %d", records[1].Weight)
	}
}

func TestNAPTRRecord(t *testing.T) {
	record := &NAPTRRecord{
		Order:       10,
		Preference:  20,
		Flags:       "s",
		Service:     "SIP+D2U",
		Regexp:      "",
		Replacement: "_sip._udp.example.com",
	}

	if record.Order != 10 {
		t.Errorf("Expected order 10, got %d", record.Order)
	}
	if record.Preference != 20 {
		t.Errorf("Expected preference 20, got %d", record.Preference)
	}
	if record.Service != "SIP+D2U" {
		t.Errorf("Expected service SIP+D2U, got %s", record.Service)
	}
}

func TestDNSRecord(t *testing.T) {
	record := &DNSRecord{
		Host:     "sip.example.com",
		Port:     5060,
		Priority: 10,
		Weight:   50,
		IP:       net.ParseIP("192.168.1.100"),
		Family:   "ipv4",
		TTL:      5 * time.Minute,
	}

	if record.Host != "sip.example.com" {
		t.Errorf("Expected host sip.example.com, got %s", record.Host)
	}
	if record.Port != 5060 {
		t.Errorf("Expected port 5060, got %d", record.Port)
	}
	if record.IP.String() != "192.168.1.100" {
		t.Errorf("Expected IP 192.168.1.100, got %s", record.IP.String())
	}
}

func TestDNSResolver_ConcurrentAccess(t *testing.T) {
	r := NewDNSResolver()
	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Concurrent resolves
			r.ResolveHost(ctx, "127.0.0.1", uint16(5060+id%10))

			// Concurrent cache clear
			if id%10 == 0 {
				r.ClearCache()
			}

			// Concurrent stats
			r.GetStats()

			// Concurrent preference changes
			r.SetPreferIPv6(id%2 == 0)
		}(i)
	}

	wg.Wait()
}

func TestDNSResolver_ResolveNAPTR(t *testing.T) {
	r := NewDNSResolver()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test with a domain that likely doesn't have NAPTR
	// Should return default SIP NAPTR entries
	records, err := r.ResolveNAPTR(ctx, "example.com")
	if err != nil {
		// NAPTR not available is okay
		t.Skipf("NAPTR lookup not available: %v", err)
	}

	if len(records) == 0 {
		t.Skip("No NAPTR records found, using defaults")
	}

	// Verify records are sorted by order
	for i := 1; i < len(records); i++ {
		if records[i].Order < records[i-1].Order {
			t.Error("NAPTR records not sorted by order")
		}
	}
}

func TestDNSResolver_ResolveSIPURI(t *testing.T) {
	r := NewDNSResolver()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test with localhost
	records, err := r.ResolveSIPURI(ctx, "localhost", "udp")
	if err != nil {
		t.Fatalf("ResolveSIPURI failed: %v", err)
	}

	if len(records) == 0 {
		t.Error("Expected at least one record")
	}
}

func TestDNSResolver_ResolveSIPURI_Transport(t *testing.T) {
	r := NewDNSResolver()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	transports := []string{"udp", "tcp", "tls", ""}

	for _, transport := range transports {
		t.Run("transport_"+transport, func(t *testing.T) {
			_, err := r.ResolveSIPURI(ctx, "localhost", transport)
			if err != nil {
				// May fail due to no SRV records, but should not panic
				t.Logf("ResolveSIPURI with transport %q: %v", transport, err)
			}
		})
	}
}

func TestGetDNSResolver(t *testing.T) {
	// First call should create the resolver
	r1 := GetDNSResolver()
	if r1 == nil {
		t.Fatal("GetDNSResolver returned nil")
	}

	// Second call should return the same instance
	r2 := GetDNSResolver()
	if r1 != r2 {
		t.Error("GetDNSResolver should return same instance")
	}
}

func TestDNSResolver_CacheExpiration(t *testing.T) {
	r := NewDNSResolver()
	r.SetCacheMaxAge(100 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Add to cache - use localhost which requires actual DNS lookup
	_, err := r.ResolveHost(ctx, "localhost", 5060)
	if err != nil {
		t.Skipf("Could not resolve localhost: %v", err)
	}

	// Should be in cache
	stats := r.GetStats()
	if stats["valid_cache_entries"].(int) == 0 {
		t.Error("Expected valid cache entry immediately after resolution")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	stats = r.GetStats()
	if stats["valid_cache_entries"].(int) != 0 {
		t.Error("Expected cache entry to be expired")
	}
}

func TestDNSResolver_ResolveWithHappyEyeballs_IPv4Only(t *testing.T) {
	r := NewDNSResolver()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Resolve an IPv4 address directly
	record, err := r.ResolveWithHappyEyeballs(ctx, "127.0.0.1", 5060)
	if err != nil {
		t.Fatalf("ResolveWithHappyEyeballs failed: %v", err)
	}

	if record.Family != "ipv4" {
		t.Errorf("Expected ipv4 family, got %s", record.Family)
	}
	if record.IP.String() != "127.0.0.1" {
		t.Errorf("Expected 127.0.0.1, got %s", record.IP.String())
	}
}

func TestDNSResolver_TestConnectivity_ValidIP(t *testing.T) {
	r := NewDNSResolver()
	ctx := context.Background()

	record := &DNSRecord{
		IP:     net.ParseIP("127.0.0.1"),
		Port:   53, // DNS port, likely available
		Family: "ipv4",
	}

	// This test may or may not succeed depending on local firewall
	result := r.testConnectivity(ctx, record)
	// Just verify it doesn't panic
	_ = result
}

func TestDNSResolver_TestConnectivity_NilIP(t *testing.T) {
	r := NewDNSResolver()
	ctx := context.Background()

	record := &DNSRecord{
		IP:     nil,
		Port:   5060,
		Family: "ipv4",
	}

	result := r.testConnectivity(ctx, record)
	if result {
		t.Error("Expected false for nil IP")
	}
}
