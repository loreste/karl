package internal

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// Helper to create a mock RTP packet
func createMockRTPPacket(ssrc uint32, seq uint16, timestamp uint32, payloadSize int) []byte {
	packet := make([]byte, 12+payloadSize)
	// RTP header
	packet[0] = 0x80 // V=2, P=0, X=0, CC=0
	packet[1] = 0x00 // M=0, PT=0
	binary.BigEndian.PutUint16(packet[2:4], seq)
	binary.BigEndian.PutUint32(packet[4:8], timestamp)
	binary.BigEndian.PutUint32(packet[8:12], ssrc)
	// Fill payload with some data
	for i := 12; i < len(packet); i++ {
		packet[i] = byte(i % 256)
	}
	return packet
}

func TestNewLoopProtector(t *testing.T) {
	lp := NewLoopProtector()
	if lp == nil {
		t.Fatal("NewLoopProtector returned nil")
	}
	defer lp.Stop()

	if lp.maxEntries != 100000 {
		t.Errorf("Expected maxEntries 100000, got %d", lp.maxEntries)
	}
	if lp.ttl != 5*time.Second {
		t.Errorf("Expected ttl 5s, got %v", lp.ttl)
	}
}

func TestLoopProtector_IsLoop_ShortPacket(t *testing.T) {
	lp := NewLoopProtector()
	defer lp.Stop()

	// Packet too short to be RTP
	shortPacket := []byte{0x80, 0x00, 0x00}
	isLoop := lp.IsLoop(shortPacket, nil, nil)
	if isLoop {
		t.Error("Short packet should not be detected as loop")
	}
}

func TestLoopProtector_IsLoop_FirstPacket(t *testing.T) {
	lp := NewLoopProtector()
	defer lp.Stop()

	packet := createMockRTPPacket(12345, 1, 100, 160)
	srcAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}

	isLoop := lp.IsLoop(packet, srcAddr, nil)
	if isLoop {
		t.Error("First packet should not be detected as loop")
	}
}

func TestLoopProtector_IsLoop_SameSourceNotLoop(t *testing.T) {
	lp := NewLoopProtector()
	defer lp.Stop()

	packet := createMockRTPPacket(12345, 1, 100, 160)
	srcAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}

	// First packet
	lp.IsLoop(packet, srcAddr, nil)

	// Same packet from same source - not a loop
	isLoop := lp.IsLoop(packet, srcAddr, nil)
	if isLoop {
		t.Error("Same packet from same source should not be a loop")
	}
}

func TestLoopProtector_IsLoop_DifferentSourceIsLoop(t *testing.T) {
	lp := NewLoopProtector()
	defer lp.Stop()

	packet := createMockRTPPacket(12345, 1, 100, 160)
	srcAddr1 := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}
	srcAddr2 := &net.UDPAddr{IP: net.ParseIP("192.168.1.200"), Port: 6000}

	// First packet from source 1
	lp.IsLoop(packet, srcAddr1, nil)

	// Same packet from different source - potential loop
	// Need multiple occurrences to trigger (tolerance > 2)
	lp.IsLoop(packet, srcAddr2, nil)
	lp.IsLoop(packet, srcAddr2, nil)
	isLoop := lp.IsLoop(packet, srcAddr2, nil)
	if !isLoop {
		t.Error("Same packet from different source should be detected as loop after threshold")
	}
}

func TestLoopProtector_IsLoop_DifferentPackets(t *testing.T) {
	lp := NewLoopProtector()
	defer lp.Stop()

	srcAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}

	// Different packets should not trigger loop detection
	for i := uint16(1); i <= 10; i++ {
		packet := createMockRTPPacket(12345, i, uint32(i*160), 160)
		isLoop := lp.IsLoop(packet, srcAddr, nil)
		if isLoop {
			t.Errorf("Packet %d should not be detected as loop", i)
		}
	}
}

func TestLoopProtector_Reset(t *testing.T) {
	lp := NewLoopProtector()
	defer lp.Stop()

	packet := createMockRTPPacket(12345, 1, 100, 160)
	srcAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}

	// Add some entries
	lp.IsLoop(packet, srcAddr, nil)

	stats := lp.Stats()
	if stats["entries"].(int) == 0 {
		t.Error("Expected entries after packet processing")
	}

	// Reset
	lp.Reset()

	stats = lp.Stats()
	if stats["entries"].(int) != 0 {
		t.Error("Expected 0 entries after reset")
	}
}

func TestLoopProtector_Stats(t *testing.T) {
	lp := NewLoopProtector()
	defer lp.Stop()

	stats := lp.Stats()
	if stats["max_entries"].(int) != 100000 {
		t.Errorf("Expected max_entries 100000, got %v", stats["max_entries"])
	}
	if stats["ttl_seconds"].(float64) != 5.0 {
		t.Errorf("Expected ttl_seconds 5.0, got %v", stats["ttl_seconds"])
	}
}

// Tests for SymmetricLatching

func TestNewSymmetricLatching(t *testing.T) {
	sl := NewSymmetricLatching()
	if sl == nil {
		t.Fatal("NewSymmetricLatching returned nil")
	}
}

func TestSymmetricLatching_LatchEndpoint(t *testing.T) {
	sl := NewSymmetricLatching()

	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}

	// First latch should return true (new latch)
	isNew := sl.LatchEndpoint("session1", addr, 12345)
	if !isNew {
		t.Error("First latch should return true")
	}

	// Verify latched
	if !sl.IsLatched("session1") {
		t.Error("Session should be latched")
	}
}

func TestSymmetricLatching_LatchEndpoint_SameAddr(t *testing.T) {
	sl := NewSymmetricLatching()

	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}

	sl.LatchEndpoint("session1", addr, 12345)

	// Same address should not be a new latch
	isNew := sl.LatchEndpoint("session1", addr, 12345)
	if isNew {
		t.Error("Same address should not be a new latch")
	}
}

func TestSymmetricLatching_LatchEndpoint_RelatchOnAddrChange(t *testing.T) {
	sl := NewSymmetricLatching()

	addr1 := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}
	addr2 := &net.UDPAddr{IP: net.ParseIP("192.168.1.200"), Port: 6000}

	sl.LatchEndpoint("session1", addr1, 12345)

	// Different address should trigger re-latch (NAT rebinding)
	isNew := sl.LatchEndpoint("session1", addr2, 12346)
	if !isNew {
		t.Error("Different address should trigger re-latch")
	}

	// Verify new address is latched
	latchedAddr := sl.GetLatchedAddress("session1")
	if latchedAddr == nil {
		t.Fatal("GetLatchedAddress returned nil")
	}
	if !latchedAddr.IP.Equal(addr2.IP) || latchedAddr.Port != addr2.Port {
		t.Errorf("Expected latched address %v, got %v", addr2, latchedAddr)
	}
}

func TestSymmetricLatching_GetLatchedAddress(t *testing.T) {
	sl := NewSymmetricLatching()

	// Non-existent session
	addr := sl.GetLatchedAddress("nonexistent")
	if addr != nil {
		t.Error("Expected nil for non-existent session")
	}

	// Existing session
	expectedAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}
	sl.LatchEndpoint("session1", expectedAddr, 12345)

	addr = sl.GetLatchedAddress("session1")
	if addr == nil {
		t.Fatal("Expected non-nil address for latched session")
	}
	if !addr.IP.Equal(expectedAddr.IP) || addr.Port != expectedAddr.Port {
		t.Errorf("Expected %v, got %v", expectedAddr, addr)
	}
}

func TestSymmetricLatching_IsLatched(t *testing.T) {
	sl := NewSymmetricLatching()

	if sl.IsLatched("nonexistent") {
		t.Error("Non-existent session should not be latched")
	}

	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}
	sl.LatchEndpoint("session1", addr, 12345)

	if !sl.IsLatched("session1") {
		t.Error("Session should be latched")
	}
}

func TestSymmetricLatching_UnlatchSession(t *testing.T) {
	sl := NewSymmetricLatching()

	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}
	sl.LatchEndpoint("session1", addr, 12345)

	sl.UnlatchSession("session1")

	if sl.IsLatched("session1") {
		t.Error("Session should not be latched after unlatch")
	}

	if sl.GetLatchedAddress("session1") != nil {
		t.Error("Unlatched session should return nil address")
	}
}

func TestSymmetricLatching_Reset(t *testing.T) {
	sl := NewSymmetricLatching()

	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}
	sl.LatchEndpoint("session1", addr, 12345)

	sl.Reset("session1")

	// Session still exists but latched flag is false
	if sl.IsLatched("session1") {
		t.Error("Session should not be latched after reset")
	}
}

// Tests for StrictSourceChecker

func TestNewStrictSourceChecker(t *testing.T) {
	ssc := NewStrictSourceChecker()
	if ssc == nil {
		t.Fatal("NewStrictSourceChecker returned nil")
	}
}

func TestStrictSourceChecker_SetExpectedSource(t *testing.T) {
	ssc := NewStrictSourceChecker()

	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}
	ssc.SetExpectedSource("session1", addr)

	// Should now validate sources
	if !ssc.IsValidSource("session1", addr) {
		t.Error("Expected source should be valid")
	}
}

func TestStrictSourceChecker_IsValidSource_NoExpected(t *testing.T) {
	ssc := NewStrictSourceChecker()

	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}

	// No expected source configured, should allow any
	if !ssc.IsValidSource("session1", addr) {
		t.Error("Should allow any source when no expected source is configured")
	}
}

func TestStrictSourceChecker_IsValidSource_MatchingSource(t *testing.T) {
	ssc := NewStrictSourceChecker()

	expectedAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}
	ssc.SetExpectedSource("session1", expectedAddr)

	// Matching source
	if !ssc.IsValidSource("session1", expectedAddr) {
		t.Error("Matching source should be valid")
	}
}

func TestStrictSourceChecker_IsValidSource_MismatchingIP(t *testing.T) {
	ssc := NewStrictSourceChecker()

	expectedAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}
	wrongAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.200"), Port: 5000}

	ssc.SetExpectedSource("session1", expectedAddr)

	if ssc.IsValidSource("session1", wrongAddr) {
		t.Error("Different IP should be invalid")
	}
}

func TestStrictSourceChecker_IsValidSource_MismatchingPort(t *testing.T) {
	ssc := NewStrictSourceChecker()

	expectedAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}
	wrongAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 6000}

	ssc.SetExpectedSource("session1", expectedAddr)

	if ssc.IsValidSource("session1", wrongAddr) {
		t.Error("Different port should be invalid")
	}
}

func TestStrictSourceChecker_RemoveSession(t *testing.T) {
	ssc := NewStrictSourceChecker()

	expectedAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 5000}
	ssc.SetExpectedSource("session1", expectedAddr)

	ssc.RemoveSession("session1")

	// After removal, any source should be allowed
	wrongAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.200"), Port: 6000}
	if !ssc.IsValidSource("session1", wrongAddr) {
		t.Error("After removal, any source should be allowed")
	}
}

func TestLoopProtector_GenerateSignature(t *testing.T) {
	lp := NewLoopProtector()
	defer lp.Stop()

	// Same inputs should produce same signature
	sig1 := lp.generateSignature(12345, 100, 1000, []byte{0x80, 0x00, 0x00, 0x64, 0x00, 0x00, 0x03, 0xE8, 0x00, 0x00, 0x30, 0x39, 0xDE, 0xAD, 0xBE, 0xEF})
	sig2 := lp.generateSignature(12345, 100, 1000, []byte{0x80, 0x00, 0x00, 0x64, 0x00, 0x00, 0x03, 0xE8, 0x00, 0x00, 0x30, 0x39, 0xDE, 0xAD, 0xBE, 0xEF})

	if sig1 != sig2 {
		t.Error("Same inputs should produce same signature")
	}

	// Different inputs should produce different signatures
	sig3 := lp.generateSignature(12346, 100, 1000, []byte{0x80, 0x00, 0x00, 0x64, 0x00, 0x00, 0x03, 0xE8, 0x00, 0x00, 0x30, 0x39, 0xDE, 0xAD, 0xBE, 0xEF})
	if sig1 == sig3 {
		t.Error("Different SSRC should produce different signature")
	}
}
