package internal

import (
	"crypto/sha256"
	"encoding/binary"
	"net"
	"sync"
	"time"
)

// LoopProtector detects and prevents RTP media loops
type LoopProtector struct {
	signatures map[uint64]loopEntry
	maxEntries int
	ttl        time.Duration
	mu         sync.RWMutex
	stopped    bool
	cleanupCh  chan struct{}
}

type loopEntry struct {
	timestamp time.Time
	count     int
	srcAddr   string
	ssrc      uint32
}

// NewLoopProtector creates a new loop protector
func NewLoopProtector() *LoopProtector {
	lp := &LoopProtector{
		signatures: make(map[uint64]loopEntry),
		maxEntries: 100000,
		ttl:        5 * time.Second,
		cleanupCh:  make(chan struct{}),
	}
	go lp.cleanupLoop()
	return lp
}

// IsLoop checks if a packet is part of a loop
func (lp *LoopProtector) IsLoop(packet []byte, srcAddr net.Addr, dstAddr net.Addr) bool {
	if len(packet) < 12 {
		return false
	}

	// Extract RTP fields
	ssrc := binary.BigEndian.Uint32(packet[8:12])
	seq := binary.BigEndian.Uint16(packet[2:4])
	timestamp := binary.BigEndian.Uint32(packet[4:8])

	// Generate packet signature
	sig := lp.generateSignature(ssrc, seq, timestamp, packet)

	lp.mu.Lock()
	defer lp.mu.Unlock()

	entry, exists := lp.signatures[sig]
	now := time.Now()

	if exists && now.Sub(entry.timestamp) < lp.ttl {
		// Packet seen recently - potential loop
		srcStr := ""
		if srcAddr != nil {
			srcStr = srcAddr.String()
		}
		// Only flag as loop if from different source
		if entry.srcAddr != srcStr {
			entry.count++
			lp.signatures[sig] = entry
			return entry.count > 2 // Allow some tolerance
		}
	}

	// Record packet
	srcStr := ""
	if srcAddr != nil {
		srcStr = srcAddr.String()
	}
	lp.signatures[sig] = loopEntry{
		timestamp: now,
		count:     1,
		srcAddr:   srcStr,
		ssrc:      ssrc,
	}

	return false
}

// generateSignature creates a unique signature for a packet
func (lp *LoopProtector) generateSignature(ssrc uint32, seq uint16, timestamp uint32, payload []byte) uint64 {
	// Use first 32 bytes of payload for signature
	payloadLen := len(payload)
	if payloadLen > 44 { // 12 header + 32 payload
		payloadLen = 44
	}

	h := sha256.New()
	binary.Write(h, binary.BigEndian, ssrc)
	binary.Write(h, binary.BigEndian, seq)
	binary.Write(h, binary.BigEndian, timestamp)
	h.Write(payload[12:payloadLen])

	sum := h.Sum(nil)
	return binary.BigEndian.Uint64(sum[:8])
}

// cleanupLoop periodically removes old entries
func (lp *LoopProtector) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lp.cleanup()
		case <-lp.cleanupCh:
			return
		}
	}
}

// cleanup removes expired entries
func (lp *LoopProtector) cleanup() {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	if lp.stopped {
		return
	}

	now := time.Now()
	for sig, entry := range lp.signatures {
		if now.Sub(entry.timestamp) > lp.ttl {
			delete(lp.signatures, sig)
		}
	}

	// If too many entries, remove oldest
	if len(lp.signatures) > lp.maxEntries {
		oldest := time.Now()
		var oldestSig uint64
		for sig, entry := range lp.signatures {
			if entry.timestamp.Before(oldest) {
				oldest = entry.timestamp
				oldestSig = sig
			}
		}
		delete(lp.signatures, oldestSig)
	}
}

// Stop stops the loop protector
func (lp *LoopProtector) Stop() {
	lp.mu.Lock()
	lp.stopped = true
	lp.mu.Unlock()
	close(lp.cleanupCh)
}

// Reset clears all entries
func (lp *LoopProtector) Reset() {
	lp.mu.Lock()
	defer lp.mu.Unlock()
	lp.signatures = make(map[uint64]loopEntry)
}

// Stats returns loop protection statistics
func (lp *LoopProtector) Stats() map[string]interface{} {
	lp.mu.RLock()
	defer lp.mu.RUnlock()

	return map[string]interface{}{
		"entries":     len(lp.signatures),
		"max_entries": lp.maxEntries,
		"ttl_seconds": lp.ttl.Seconds(),
	}
}

// SymmetricLatching handles symmetric RTP with port latching
type SymmetricLatching struct {
	sessions map[string]*latchedEndpoint
	mu       sync.RWMutex
}

type latchedEndpoint struct {
	addr       *net.UDPAddr
	ssrc       uint32
	lastSeen   time.Time
	packetCount uint64
	latched    bool
}

// NewSymmetricLatching creates a new symmetric latching handler
func NewSymmetricLatching() *SymmetricLatching {
	return &SymmetricLatching{
		sessions: make(map[string]*latchedEndpoint),
	}
}

// LatchEndpoint latches to the source of incoming media
func (sl *SymmetricLatching) LatchEndpoint(sessionKey string, addr *net.UDPAddr, ssrc uint32) bool {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	endpoint, exists := sl.sessions[sessionKey]
	if !exists {
		sl.sessions[sessionKey] = &latchedEndpoint{
			addr:       addr,
			ssrc:       ssrc,
			lastSeen:   time.Now(),
			packetCount: 1,
			latched:    true,
		}
		return true // New latch
	}

	// Update existing endpoint
	endpoint.lastSeen = time.Now()
	endpoint.packetCount++

	// Allow re-latch if address changed (NAT rebinding)
	if !endpoint.addr.IP.Equal(addr.IP) || endpoint.addr.Port != addr.Port {
		endpoint.addr = addr
		endpoint.ssrc = ssrc
		return true // Re-latched
	}

	return false // No change
}

// GetLatchedAddress returns the latched address for a session
func (sl *SymmetricLatching) GetLatchedAddress(sessionKey string) *net.UDPAddr {
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	if endpoint, exists := sl.sessions[sessionKey]; exists && endpoint.latched {
		return endpoint.addr
	}
	return nil
}

// IsLatched checks if a session is latched
func (sl *SymmetricLatching) IsLatched(sessionKey string) bool {
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	endpoint, exists := sl.sessions[sessionKey]
	return exists && endpoint.latched
}

// UnlatchSession removes latching for a session
func (sl *SymmetricLatching) UnlatchSession(sessionKey string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	delete(sl.sessions, sessionKey)
}

// Reset resets latching for a session (for media handover)
func (sl *SymmetricLatching) Reset(sessionKey string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	if endpoint, exists := sl.sessions[sessionKey]; exists {
		endpoint.latched = false
	}
}

// StrictSourceChecker validates that media comes from expected source
type StrictSourceChecker struct {
	expectedSources map[string]*net.UDPAddr
	mu              sync.RWMutex
}

// NewStrictSourceChecker creates a new strict source checker
func NewStrictSourceChecker() *StrictSourceChecker {
	return &StrictSourceChecker{
		expectedSources: make(map[string]*net.UDPAddr),
	}
}

// SetExpectedSource sets the expected source for a session
func (ssc *StrictSourceChecker) SetExpectedSource(sessionKey string, addr *net.UDPAddr) {
	ssc.mu.Lock()
	defer ssc.mu.Unlock()
	ssc.expectedSources[sessionKey] = addr
}

// IsValidSource checks if the source matches the expected source
func (ssc *StrictSourceChecker) IsValidSource(sessionKey string, addr *net.UDPAddr) bool {
	ssc.mu.RLock()
	defer ssc.mu.RUnlock()

	expected, exists := ssc.expectedSources[sessionKey]
	if !exists {
		return true // No strict checking if not configured
	}

	// Check IP and port
	return expected.IP.Equal(addr.IP) && expected.Port == addr.Port
}

// RemoveSession removes a session from strict checking
func (ssc *StrictSourceChecker) RemoveSession(sessionKey string) {
	ssc.mu.Lock()
	defer ssc.mu.Unlock()
	delete(ssc.expectedSources, sessionKey)
}
