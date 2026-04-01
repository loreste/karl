package internal

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// DNSRecord represents a resolved DNS record
type DNSRecord struct {
	Host     string
	Port     uint16
	Priority uint16
	Weight   uint16
	IP       net.IP
	Family   string // "ipv4" or "ipv6"
	TTL      time.Duration
}

// DNSResolver handles DNS resolution for SIP and media endpoints
type DNSResolver struct {
	resolver    *net.Resolver
	cache       map[string]*cacheEntry
	cacheLock   sync.RWMutex
	cacheMaxAge time.Duration
	preferIPv6  bool
}

type cacheEntry struct {
	records   []*DNSRecord
	expiresAt time.Time
}

// NAPTRRecord represents a parsed NAPTR record
type NAPTRRecord struct {
	Order       uint16
	Preference  uint16
	Flags       string
	Service     string
	Regexp      string
	Replacement string
}

// NewDNSResolver creates a new DNS resolver
func NewDNSResolver() *DNSResolver {
	return &DNSResolver{
		resolver: &net.Resolver{
			PreferGo: true, // Use Go's pure DNS resolver for better control
		},
		cache:       make(map[string]*cacheEntry),
		cacheMaxAge: 5 * time.Minute,
		preferIPv6:  false,
	}
}

// SetPreferIPv6 configures IPv6 preference
func (r *DNSResolver) SetPreferIPv6(prefer bool) {
	r.cacheLock.Lock()
	r.preferIPv6 = prefer
	r.cacheLock.Unlock()
}

// SetCacheMaxAge sets the maximum cache age
func (r *DNSResolver) SetCacheMaxAge(d time.Duration) {
	r.cacheLock.Lock()
	r.cacheMaxAge = d
	r.cacheLock.Unlock()
}

// getPreferIPv6 returns IPv6 preference safely
func (r *DNSResolver) getPreferIPv6() bool {
	r.cacheLock.RLock()
	defer r.cacheLock.RUnlock()
	return r.preferIPv6
}

// getCacheMaxAge returns cache max age safely
func (r *DNSResolver) getCacheMaxAge() time.Duration {
	r.cacheLock.RLock()
	defer r.cacheLock.RUnlock()
	return r.cacheMaxAge
}

// ResolveSRV performs SRV record lookup and returns resolved addresses
func (r *DNSResolver) ResolveSRV(ctx context.Context, service, proto, domain string) ([]*DNSRecord, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("srv:%s.%s.%s", service, proto, domain)
	if records := r.getFromCache(cacheKey); records != nil {
		return records, nil
	}

	// Perform SRV lookup
	_, srvRecords, err := r.resolver.LookupSRV(ctx, service, proto, domain)
	if err != nil {
		// Fall back to A/AAAA lookup on the domain itself
		return r.ResolveHost(ctx, domain, 5060)
	}

	var records []*DNSRecord

	// Sort SRV records by priority, then weight
	sort.Slice(srvRecords, func(i, j int) bool {
		if srvRecords[i].Priority != srvRecords[j].Priority {
			return srvRecords[i].Priority < srvRecords[j].Priority
		}
		return srvRecords[i].Weight > srvRecords[j].Weight
	})

	// Resolve each SRV target
	for _, srv := range srvRecords {
		hostRecords, err := r.ResolveHost(ctx, strings.TrimSuffix(srv.Target, "."), srv.Port)
		if err != nil {
			continue
		}

		for _, hr := range hostRecords {
			hr.Priority = srv.Priority
			hr.Weight = srv.Weight
			records = append(records, hr)
		}
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("no records found for SRV %s.%s.%s", service, proto, domain)
	}

	// Cache the results
	r.addToCache(cacheKey, records)

	return records, nil
}

// ResolveNAPTR performs NAPTR record lookup for SIP
func (r *DNSResolver) ResolveNAPTR(ctx context.Context, domain string) ([]*NAPTRRecord, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("naptr:%s", domain)
	r.cacheLock.RLock()
	if entry, ok := r.cache[cacheKey]; ok && entry.expiresAt.After(time.Now()) {
		r.cacheLock.RUnlock()
		// Return cached NAPTR records - convert from DNSRecord
		var naptrRecords []*NAPTRRecord
		for _, rec := range entry.records {
			naptrRecords = append(naptrRecords, &NAPTRRecord{
				Order:       rec.Priority,
				Preference:  rec.Weight,
				Service:     rec.Host,
				Replacement: rec.Host,
			})
		}
		return naptrRecords, nil
	}
	r.cacheLock.RUnlock()

	// Use net package to lookup NAPTR (Go standard library doesn't have direct NAPTR support)
	// We'll use LookupTXT to check for ENUM-style records or fall back to SRV
	txtRecords, err := r.resolver.LookupTXT(ctx, domain)
	if err != nil {
		// NAPTR not available, return empty - caller should use SRV fallback
		return nil, nil
	}

	var naptrRecords []*NAPTRRecord

	// Parse TXT records that might contain NAPTR-like data
	for _, txt := range txtRecords {
		if strings.HasPrefix(txt, "v=sip") {
			// SIP service indicator
			naptrRecords = append(naptrRecords, &NAPTRRecord{
				Order:       10,
				Preference:  10,
				Flags:       "s",
				Service:     "SIP+D2U",
				Replacement: "_sip._udp." + domain,
			})
		}
	}

	// If no NAPTR records found, return standard SIP NAPTR entries
	if len(naptrRecords) == 0 {
		naptrRecords = []*NAPTRRecord{
			{
				Order:       10,
				Preference:  10,
				Flags:       "s",
				Service:     "SIP+D2U",
				Replacement: "_sip._udp." + domain,
			},
			{
				Order:       20,
				Preference:  10,
				Flags:       "s",
				Service:     "SIP+D2T",
				Replacement: "_sip._tcp." + domain,
			},
			{
				Order:       30,
				Preference:  10,
				Flags:       "s",
				Service:     "SIPS+D2T",
				Replacement: "_sips._tcp." + domain,
			},
		}
	}

	// Sort by order, then preference
	sort.Slice(naptrRecords, func(i, j int) bool {
		if naptrRecords[i].Order != naptrRecords[j].Order {
			return naptrRecords[i].Order < naptrRecords[j].Order
		}
		return naptrRecords[i].Preference < naptrRecords[j].Preference
	})

	return naptrRecords, nil
}

// ResolveSIPURI resolves a SIP URI to addresses using NAPTR/SRV/A lookup chain
func (r *DNSResolver) ResolveSIPURI(ctx context.Context, domain string, transport string) ([]*DNSRecord, error) {
	// Step 1: Try NAPTR lookup
	naptrRecords, err := r.ResolveNAPTR(ctx, domain)
	if err == nil && len(naptrRecords) > 0 {
		// Find matching service for transport
		var targetService string
		switch strings.ToLower(transport) {
		case "udp", "":
			targetService = "SIP+D2U"
		case "tcp":
			targetService = "SIP+D2T"
		case "tls":
			targetService = "SIPS+D2T"
		case "ws":
			targetService = "SIP+D2W"
		case "wss":
			targetService = "SIPS+D2W"
		}

		for _, naptr := range naptrRecords {
			if strings.Contains(naptr.Service, targetService) || targetService == "" {
				// Parse replacement to get SRV name
				parts := strings.SplitN(naptr.Replacement, ".", 3)
				if len(parts) >= 3 {
					service := strings.TrimPrefix(parts[0], "_")
					proto := strings.TrimPrefix(parts[1], "_")
					srvDomain := strings.Join(parts[2:], ".")
					return r.ResolveSRV(ctx, service, proto, srvDomain)
				}
			}
		}
	}

	// Step 2: Try SRV lookup directly
	var srvService, srvProto string
	switch strings.ToLower(transport) {
	case "udp", "":
		srvService, srvProto = "sip", "udp"
	case "tcp":
		srvService, srvProto = "sip", "tcp"
	case "tls":
		srvService, srvProto = "sips", "tcp"
	default:
		srvService, srvProto = "sip", "udp"
	}

	records, err := r.ResolveSRV(ctx, srvService, srvProto, domain)
	if err == nil && len(records) > 0 {
		return records, nil
	}

	// Step 3: Fall back to A/AAAA lookup
	port := uint16(5060)
	if transport == "tls" {
		port = 5061
	}
	return r.ResolveHost(ctx, domain, port)
}

// ResolveHost performs A and AAAA lookups with happy eyeballs support
func (r *DNSResolver) ResolveHost(ctx context.Context, host string, port uint16) ([]*DNSRecord, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("host:%s:%d", host, port)
	if records := r.getFromCache(cacheKey); records != nil {
		return records, nil
	}

	// Check if host is already an IP address
	if ip := net.ParseIP(host); ip != nil {
		family := "ipv4"
		if ip.To4() == nil {
			family = "ipv6"
		}
		return []*DNSRecord{{
			Host:   host,
			Port:   port,
			IP:     ip,
			Family: family,
		}}, nil
	}

	// Perform parallel A and AAAA lookups (happy eyeballs style)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var records []*DNSRecord

	// IPv4 lookup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ips, err := r.resolver.LookupIP(ctx, "ip4", host)
		if err != nil {
			return
		}
		mu.Lock()
		for _, ip := range ips {
			records = append(records, &DNSRecord{
				Host:   host,
				Port:   port,
				IP:     ip,
				Family: "ipv4",
			})
		}
		mu.Unlock()
	}()

	// IPv6 lookup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ips, err := r.resolver.LookupIP(ctx, "ip6", host)
		if err != nil {
			return
		}
		mu.Lock()
		for _, ip := range ips {
			records = append(records, &DNSRecord{
				Host:   host,
				Port:   port,
				IP:     ip,
				Family: "ipv6",
			})
		}
		mu.Unlock()
	}()

	wg.Wait()

	if len(records) == 0 {
		return nil, fmt.Errorf("no addresses found for %s", host)
	}

	// Sort records based on preference
	r.sortByPreference(records)

	// Cache the results
	r.addToCache(cacheKey, records)

	return records, nil
}

// ResolveWithHappyEyeballs performs resolution with RFC 6555 happy eyeballs algorithm
// It prefers IPv6 but starts IPv4 quickly if IPv6 is slow
func (r *DNSResolver) ResolveWithHappyEyeballs(ctx context.Context, host string, port uint16) (*DNSRecord, error) {
	records, err := r.ResolveHost(ctx, host, port)
	if err != nil {
		return nil, err
	}

	// Happy eyeballs: try to connect to addresses in preference order
	// with a small delay between starting IPv4 vs IPv6 attempts

	var ipv4Records, ipv6Records []*DNSRecord
	for _, rec := range records {
		if rec.Family == "ipv6" {
			ipv6Records = append(ipv6Records, rec)
		} else {
			ipv4Records = append(ipv4Records, rec)
		}
	}

	// Create result channel
	type connResult struct {
		record *DNSRecord
		err    error
	}
	resultCh := make(chan connResult, 1)

	// Start IPv6 first if we prefer it, otherwise start IPv4
	var firstSet, secondSet []*DNSRecord
	if r.getPreferIPv6() && len(ipv6Records) > 0 {
		firstSet, secondSet = ipv6Records, ipv4Records
	} else if len(ipv4Records) > 0 {
		firstSet, secondSet = ipv4Records, ipv6Records
	} else {
		firstSet = ipv6Records // Only IPv6 available
	}

	// Cancellable context for attempts
	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Try first set immediately
	for _, rec := range firstSet {
		go func(record *DNSRecord) {
			if r.testConnectivity(attemptCtx, record) {
				select {
				case resultCh <- connResult{record: record}:
				default:
				}
			}
		}(rec)
	}

	// Wait 250ms then try second set (happy eyeballs delay)
	if len(secondSet) > 0 {
		go func() {
			select {
			case <-time.After(250 * time.Millisecond):
				for _, rec := range secondSet {
					go func(record *DNSRecord) {
						if r.testConnectivity(attemptCtx, record) {
							select {
							case resultCh <- connResult{record: record}:
							default:
							}
						}
					}(rec)
				}
			case <-attemptCtx.Done():
				return
			}
		}()
	}

	// Wait for first successful result or timeout
	select {
	case result := <-resultCh:
		return result.record, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		// If connectivity tests fail, just return the first record
		if len(records) > 0 {
			return records[0], nil
		}
		return nil, fmt.Errorf("connection timeout for %s", host)
	}
}

// testConnectivity performs a quick connectivity test
func (r *DNSResolver) testConnectivity(ctx context.Context, record *DNSRecord) bool {
	// For RTP/media, we just verify the IP is valid
	// In production, this could do actual UDP connectivity test
	if record.IP == nil {
		return false
	}

	// Try a quick UDP "connect" (doesn't actually send data)
	addr := fmt.Sprintf("%s:%d", record.IP.String(), record.Port)
	if record.Family == "ipv6" {
		addr = fmt.Sprintf("[%s]:%d", record.IP.String(), record.Port)
	}

	conn, err := net.DialTimeout("udp", addr, 100*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// sortByPreference sorts records by IPv4/IPv6 preference
func (r *DNSResolver) sortByPreference(records []*DNSRecord) {
	preferIPv6 := r.getPreferIPv6()
	sort.Slice(records, func(i, j int) bool {
		// Sort by family preference
		if preferIPv6 {
			if records[i].Family == "ipv6" && records[j].Family == "ipv4" {
				return true
			}
			if records[i].Family == "ipv4" && records[j].Family == "ipv6" {
				return false
			}
		} else {
			if records[i].Family == "ipv4" && records[j].Family == "ipv6" {
				return true
			}
			if records[i].Family == "ipv6" && records[j].Family == "ipv4" {
				return false
			}
		}
		// Then by priority
		if records[i].Priority != records[j].Priority {
			return records[i].Priority < records[j].Priority
		}
		// Then by weight (higher weight = preferred)
		return records[i].Weight > records[j].Weight
	})
}

// getFromCache retrieves cached records if still valid
func (r *DNSResolver) getFromCache(key string) []*DNSRecord {
	r.cacheLock.RLock()
	defer r.cacheLock.RUnlock()

	entry, ok := r.cache[key]
	if !ok || entry.expiresAt.Before(time.Now()) {
		return nil
	}

	return entry.records
}

// addToCache adds records to the cache
func (r *DNSResolver) addToCache(key string, records []*DNSRecord) {
	r.cacheLock.Lock()
	defer r.cacheLock.Unlock()

	r.cache[key] = &cacheEntry{
		records:   records,
		expiresAt: time.Now().Add(r.cacheMaxAge),
	}
}

// ClearCache clears all cached DNS records
func (r *DNSResolver) ClearCache() {
	r.cacheLock.Lock()
	defer r.cacheLock.Unlock()
	r.cache = make(map[string]*cacheEntry)
}

// GetStats returns resolver statistics
func (r *DNSResolver) GetStats() map[string]interface{} {
	r.cacheLock.RLock()
	defer r.cacheLock.RUnlock()

	validEntries := 0
	now := time.Now()
	for _, entry := range r.cache {
		if entry.expiresAt.After(now) {
			validEntries++
		}
	}

	return map[string]interface{}{
		"cache_entries":       len(r.cache),
		"valid_cache_entries": validEntries,
		"prefer_ipv6":         r.preferIPv6,
		"cache_max_age_secs":  r.cacheMaxAge.Seconds(),
	}
}

// Global DNS resolver instance
var (
	globalDNSResolver     *DNSResolver
	globalDNSResolverOnce sync.Once
)

// GetDNSResolver returns the global DNS resolver instance
func GetDNSResolver() *DNSResolver {
	globalDNSResolverOnce.Do(func() {
		globalDNSResolver = NewDNSResolver()
	})
	return globalDNSResolver
}
