package internal

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestDefaultT38Config(t *testing.T) {
	config := DefaultT38Config()

	if config == nil {
		t.Fatal("DefaultT38Config returned nil")
	}

	if !config.Enabled {
		t.Error("Enabled should be true by default")
	}

	if config.GatewayMode {
		t.Error("GatewayMode should be false by default (passthrough)")
	}

	if config.MaxBitRate != 14400 {
		t.Errorf("Expected MaxBitRate 14400, got %d", config.MaxBitRate)
	}

	if config.RateMgmt != "transferredTCF" {
		t.Errorf("Expected RateMgmt transferredTCF, got %s", config.RateMgmt)
	}

	if config.UDPECMode != "t38UDPRedundancy" {
		t.Errorf("Expected UDPECMode t38UDPRedundancy, got %s", config.UDPECMode)
	}

	if config.UDPECDepth != 3 {
		t.Errorf("Expected UDPECDepth 3, got %d", config.UDPECDepth)
	}
}

func TestNewT38Gateway(t *testing.T) {
	gw := NewT38Gateway(nil)
	if gw == nil {
		t.Fatal("NewT38Gateway returned nil")
	}

	// Should use default config
	if gw.config.MaxBitRate != 14400 {
		t.Error("Should use default config when nil is passed")
	}
}

func TestNewT38Gateway_WithConfig(t *testing.T) {
	config := &T38Config{
		Enabled:     true,
		GatewayMode: true,
		MaxBitRate:  9600,
	}

	gw := NewT38Gateway(config)
	if gw.config.MaxBitRate != 9600 {
		t.Error("Should use provided config")
	}
}

func TestT38Gateway_CreateSession(t *testing.T) {
	gw := NewT38Gateway(nil)

	localIP := net.ParseIP("192.168.1.100")
	session, err := gw.CreateSession("call-123", localIP, 5000)

	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if session == nil {
		t.Fatal("CreateSession returned nil session")
	}

	if session.CallID != "call-123" {
		t.Errorf("Expected CallID call-123, got %s", session.CallID)
	}

	if !session.LocalIP.Equal(localIP) {
		t.Errorf("Expected LocalIP %v, got %v", localIP, session.LocalIP)
	}

	if session.LocalPort != 5000 {
		t.Errorf("Expected LocalPort 5000, got %d", session.LocalPort)
	}

	if session.State != T38StateSetup {
		t.Errorf("Expected state setup, got %s", session.State)
	}

	if session.Direction != T38DirectionBoth {
		t.Errorf("Expected direction both, got %s", session.Direction)
	}

	if session.Stats == nil {
		t.Error("Stats should be initialized")
	}
}

func TestT38Gateway_SetRemoteEndpoint(t *testing.T) {
	gw := NewT38Gateway(nil)

	localIP := net.ParseIP("192.168.1.100")
	session, _ := gw.CreateSession("call-123", localIP, 5000)

	remoteIP := net.ParseIP("192.168.1.200")
	err := gw.SetRemoteEndpoint(session.ID, remoteIP, 6000)

	if err != nil {
		t.Fatalf("SetRemoteEndpoint failed: %v", err)
	}

	s, _ := gw.GetSession(session.ID)
	if !s.RemoteIP.Equal(remoteIP) {
		t.Errorf("Expected RemoteIP %v, got %v", remoteIP, s.RemoteIP)
	}

	if s.RemotePort != 6000 {
		t.Errorf("Expected RemotePort 6000, got %d", s.RemotePort)
	}

	if s.State != T38StateActive {
		t.Errorf("Expected state active after setting remote, got %s", s.State)
	}
}

func TestT38Gateway_SetRemoteEndpoint_InvalidSession(t *testing.T) {
	gw := NewT38Gateway(nil)

	err := gw.SetRemoteEndpoint("nonexistent", net.ParseIP("192.168.1.200"), 6000)
	if err == nil {
		t.Error("Expected error for nonexistent session")
	}
}

func TestT38Gateway_GetSession(t *testing.T) {
	gw := NewT38Gateway(nil)

	// Non-existent session
	_, exists := gw.GetSession("nonexistent")
	if exists {
		t.Error("Expected false for nonexistent session")
	}

	// Existing session
	session, _ := gw.CreateSession("call-123", net.ParseIP("192.168.1.100"), 5000)
	s, exists := gw.GetSession(session.ID)
	if !exists {
		t.Error("Expected true for existing session")
	}
	if s.ID != session.ID {
		t.Error("Session IDs should match")
	}
}

func TestT38Gateway_GetSessionByCallID(t *testing.T) {
	gw := NewT38Gateway(nil)

	localIP := net.ParseIP("192.168.1.100")
	gw.CreateSession("call-123", localIP, 5000)
	gw.CreateSession("call-456", localIP, 5002)
	gw.CreateSession("call-123", localIP, 5004) // Another for same call

	sessions := gw.GetSessionByCallID("call-123")
	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions for call-123, got %d", len(sessions))
	}
}

func TestT38Gateway_ProcessPacket(t *testing.T) {
	gw := NewT38Gateway(nil)

	localIP := net.ParseIP("192.168.1.100")
	session, _ := gw.CreateSession("call-123", localIP, 5000)

	// Create a simple T.38 packet (2 byte header + data)
	packet := []byte{0x10, 0x01, 0xDE, 0xAD, 0xBE, 0xEF}

	ifp, err := gw.ProcessPacket(session.ID, packet)
	if err != nil {
		t.Fatalf("ProcessPacket failed: %v", err)
	}

	if ifp == nil {
		t.Fatal("ProcessPacket returned nil IFP")
	}

	// Check stats were updated
	stats, _ := gw.GetStats(session.ID)
	if stats.PacketsRecv != 1 {
		t.Errorf("Expected PacketsRecv 1, got %d", stats.PacketsRecv)
	}
	if stats.BytesRecv != uint64(len(packet)) {
		t.Errorf("Expected BytesRecv %d, got %d", len(packet), stats.BytesRecv)
	}
}

func TestT38Gateway_ProcessPacket_InvalidSession(t *testing.T) {
	gw := NewT38Gateway(nil)

	_, err := gw.ProcessPacket("nonexistent", []byte{0x10, 0x01})
	if err == nil {
		t.Error("Expected error for nonexistent session")
	}
}

func TestT38Gateway_ProcessPacket_ShortPacket(t *testing.T) {
	gw := NewT38Gateway(nil)

	session, _ := gw.CreateSession("call-123", net.ParseIP("192.168.1.100"), 5000)

	// Packet too short
	_, err := gw.ProcessPacket(session.ID, []byte{0x10})
	if err == nil {
		t.Error("Expected error for packet too short")
	}

	// Verify error was counted
	stats, _ := gw.GetStats(session.ID)
	if stats.Errors != 1 {
		t.Errorf("Expected Errors 1, got %d", stats.Errors)
	}
}

func TestT38Gateway_SendPacket(t *testing.T) {
	gw := NewT38Gateway(nil)

	session, _ := gw.CreateSession("call-123", net.ParseIP("192.168.1.100"), 5000)

	ifp := &T38IFP{
		Type:   T38IFPPrimaryV21,
		SeqNum: 1,
		Data:   []byte{0xDE, 0xAD, 0xBE, 0xEF},
	}

	err := gw.SendPacket(session.ID, ifp)
	if err != nil {
		t.Fatalf("SendPacket failed: %v", err)
	}

	// Check stats were updated
	stats, _ := gw.GetStats(session.ID)
	if stats.PacketsSent != 1 {
		t.Errorf("Expected PacketsSent 1, got %d", stats.PacketsSent)
	}
}

func TestT38Gateway_SendPacket_InvalidSession(t *testing.T) {
	gw := NewT38Gateway(nil)

	err := gw.SendPacket("nonexistent", &T38IFP{})
	if err == nil {
		t.Error("Expected error for nonexistent session")
	}
}

func TestT38Gateway_CompleteSession(t *testing.T) {
	gw := NewT38Gateway(nil)

	session, _ := gw.CreateSession("call-123", net.ParseIP("192.168.1.100"), 5000)

	err := gw.CompleteSession(session.ID)
	if err != nil {
		t.Fatalf("CompleteSession failed: %v", err)
	}

	s, _ := gw.GetSession(session.ID)
	if s.State != T38StateComplete {
		t.Errorf("Expected state complete, got %s", s.State)
	}
}

func TestT38Gateway_CompleteSession_InvalidSession(t *testing.T) {
	gw := NewT38Gateway(nil)

	err := gw.CompleteSession("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent session")
	}
}

func TestT38Gateway_RemoveSession(t *testing.T) {
	gw := NewT38Gateway(nil)

	session, _ := gw.CreateSession("call-123", net.ParseIP("192.168.1.100"), 5000)
	gw.RemoveSession(session.ID)

	_, exists := gw.GetSession(session.ID)
	if exists {
		t.Error("Session should be removed")
	}
}

func TestT38Gateway_GetStats(t *testing.T) {
	gw := NewT38Gateway(nil)

	session, _ := gw.CreateSession("call-123", net.ParseIP("192.168.1.100"), 5000)

	stats, err := gw.GetStats(session.ID)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats == nil {
		t.Fatal("GetStats returned nil")
	}

	// Initial stats should be zero
	if stats.PacketsSent != 0 || stats.PacketsRecv != 0 {
		t.Error("Initial stats should be zero")
	}
}

func TestT38Gateway_GetStats_InvalidSession(t *testing.T) {
	gw := NewT38Gateway(nil)

	_, err := gw.GetStats("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent session")
	}
}

func TestT38Gateway_GetActiveCount(t *testing.T) {
	gw := NewT38Gateway(nil)

	if gw.GetActiveCount() != 0 {
		t.Error("Initial active count should be 0")
	}

	localIP := net.ParseIP("192.168.1.100")
	s1, _ := gw.CreateSession("call-1", localIP, 5000)
	s2, _ := gw.CreateSession("call-2", localIP, 5002)

	// Set remote to make active
	gw.SetRemoteEndpoint(s1.ID, net.ParseIP("192.168.1.200"), 6000)
	gw.SetRemoteEndpoint(s2.ID, net.ParseIP("192.168.1.200"), 6002)

	if gw.GetActiveCount() != 2 {
		t.Errorf("Expected 2 active, got %d", gw.GetActiveCount())
	}

	gw.CompleteSession(s1.ID)
	if gw.GetActiveCount() != 1 {
		t.Errorf("Expected 1 active after complete, got %d", gw.GetActiveCount())
	}
}

func TestT38Gateway_Cleanup(t *testing.T) {
	gw := NewT38Gateway(nil)

	localIP := net.ParseIP("192.168.1.100")
	s1, _ := gw.CreateSession("call-1", localIP, 5000)
	s2, _ := gw.CreateSession("call-2", localIP, 5002)
	gw.CreateSession("call-3", localIP, 5004) // Stays in setup

	gw.CompleteSession(s1.ID)
	gw.CompleteSession(s2.ID)

	// Sessions are too new, cleanup shouldn't remove them
	removed := gw.Cleanup(24 * time.Hour)
	if removed != 0 {
		t.Errorf("Expected 0 removed (sessions too new), got %d", removed)
	}
}

func TestBuildT38SDP(t *testing.T) {
	config := &T38Config{
		MaxBitRate:     14400,
		RateMgmt:       "transferredTCF",
		MaxBuffer:      200,
		UDPECMode:      "t38UDPRedundancy",
		FillBitRemoval: true,
	}

	sdp := BuildT38SDP(config, "192.168.1.100", 5000)

	// Verify SDP contains expected attributes
	if !strings.Contains(sdp, "m=image 5000 udptl t38") {
		t.Error("Missing m=image line")
	}

	if !strings.Contains(sdp, "a=T38MaxBitRate:14400") {
		t.Error("Missing T38MaxBitRate")
	}

	if !strings.Contains(sdp, "a=T38FaxRateManagement:transferredTCF") {
		t.Error("Missing T38FaxRateManagement")
	}

	if !strings.Contains(sdp, "a=T38FaxUdpEC:t38UDPRedundancy") {
		t.Error("Missing T38FaxUdpEC")
	}

	if !strings.Contains(sdp, "a=T38FaxFillBitRemoval") {
		t.Error("Missing T38FaxFillBitRemoval")
	}
}

func TestBuildT38SDP_NilConfig(t *testing.T) {
	sdp := BuildT38SDP(nil, "192.168.1.100", 5000)

	// Should use default config
	if !strings.Contains(sdp, "a=T38MaxBitRate:14400") {
		t.Error("Should use default MaxBitRate")
	}
}

func TestParseT38SDP(t *testing.T) {
	sdp := `m=image 5000 udptl t38
a=T38MaxBitRate:14400
a=T38FaxRateManagement:transferredTCF`

	config := ParseT38SDP(sdp)
	if config == nil {
		t.Fatal("ParseT38SDP returned nil")
	}

	// Current implementation returns default config
	// Future implementation should parse the SDP
	if config.MaxBitRate != 14400 {
		t.Errorf("Expected default MaxBitRate, got %d", config.MaxBitRate)
	}
}

func TestT38State_Constants(t *testing.T) {
	if T38StateIdle != "idle" {
		t.Error("T38StateIdle mismatch")
	}
	if T38StateSetup != "setup" {
		t.Error("T38StateSetup mismatch")
	}
	if T38StateActive != "active" {
		t.Error("T38StateActive mismatch")
	}
	if T38StateComplete != "complete" {
		t.Error("T38StateComplete mismatch")
	}
	if T38StateFailed != "failed" {
		t.Error("T38StateFailed mismatch")
	}
}

func TestT38Direction_Constants(t *testing.T) {
	if T38DirectionSend != "send" {
		t.Error("T38DirectionSend mismatch")
	}
	if T38DirectionReceive != "receive" {
		t.Error("T38DirectionReceive mismatch")
	}
	if T38DirectionBoth != "both" {
		t.Error("T38DirectionBoth mismatch")
	}
}

func TestT38IFPType_Constants(t *testing.T) {
	if T38IFPPrimaryV21 != 0 {
		t.Error("T38IFPPrimaryV21 should be 0")
	}
	if T38IFPPrimaryV27 != 1 {
		t.Error("T38IFPPrimaryV27 should be 1")
	}
	if T38IFPControlData != 8 {
		t.Error("T38IFPControlData should be 8")
	}
}
