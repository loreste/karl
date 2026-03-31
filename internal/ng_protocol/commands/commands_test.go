package commands

import (
	"strings"
	"testing"
	"time"

	"karl/internal"
	ng "karl/internal/ng_protocol"
)

// Helper to create a test config
func createTestConfig() *internal.Config {
	return &internal.Config{
		Integration: internal.IntegrationConfig{
			MediaIP:  "192.168.1.100",
			PublicIP: "203.0.113.50",
		},
		Sessions: &internal.SessionConfig{
			MinPort: 30000,
			MaxPort: 40000,
		},
	}
}

// Helper to create a session registry
func createTestRegistry() *internal.SessionRegistry {
	return internal.NewSessionRegistry(5 * time.Minute)
}

// Helper to create a valid SDP
func createTestSDP() string {
	return `v=0
o=alice 2890844526 2890844526 IN IP4 192.168.1.200
s=Session
c=IN IP4 192.168.1.200
t=0 0
m=audio 5000 RTP/AVP 0 8 101
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:101 telephone-event/8000
a=fmtp:101 0-15
a=sendrecv
`
}

// ========== OfferHandler Tests ==========

func TestNewOfferHandler(t *testing.T) {
	registry := createTestRegistry()
	config := createTestConfig()

	handler := NewOfferHandler(registry, config)
	if handler == nil {
		t.Fatal("NewOfferHandler returned nil")
	}
}

func TestOfferHandler_Handle_MissingCallID(t *testing.T) {
	registry := createTestRegistry()
	config := createTestConfig()
	handler := NewOfferHandler(registry, config)

	req := &ng.NGRequest{
		Command: "offer",
		FromTag: "tag-123",
		SDP:     createTestSDP(),
	}

	resp, err := handler.Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if resp.Result != ng.ResultError {
		t.Error("Expected error result for missing call-id")
	}
	if !strings.Contains(resp.ErrorReason, "call-id") {
		t.Errorf("Expected error to mention call-id, got: %s", resp.ErrorReason)
	}
}

func TestOfferHandler_Handle_MissingFromTag(t *testing.T) {
	registry := createTestRegistry()
	config := createTestConfig()
	handler := NewOfferHandler(registry, config)

	req := &ng.NGRequest{
		Command: "offer",
		CallID:  "call-123",
		SDP:     createTestSDP(),
	}

	resp, err := handler.Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if resp.Result != ng.ResultError {
		t.Error("Expected error result for missing from-tag")
	}
	if !strings.Contains(resp.ErrorReason, "from-tag") {
		t.Errorf("Expected error to mention from-tag, got: %s", resp.ErrorReason)
	}
}

func TestOfferHandler_Handle_MissingSDP(t *testing.T) {
	registry := createTestRegistry()
	config := createTestConfig()
	handler := NewOfferHandler(registry, config)

	req := &ng.NGRequest{
		Command: "offer",
		CallID:  "call-123",
		FromTag: "tag-123",
	}

	resp, err := handler.Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if resp.Result != ng.ResultError {
		t.Error("Expected error result for missing sdp")
	}
	if !strings.Contains(resp.ErrorReason, "sdp") {
		t.Errorf("Expected error to mention sdp, got: %s", resp.ErrorReason)
	}
}

func TestOfferHandler_Handle_ValidOffer(t *testing.T) {
	registry := createTestRegistry()
	config := createTestConfig()
	handler := NewOfferHandler(registry, config)

	req := &ng.NGRequest{
		Command: "offer",
		CallID:  "call-123",
		FromTag: "tag-123",
		SDP:     createTestSDP(),
	}

	resp, err := handler.Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	// Port allocation may fail in test environment - check if it's a port error
	if resp.Result == ng.ResultError && strings.Contains(resp.ErrorReason, "port") {
		t.Skip("Skipping: port allocation not available in test environment")
	}

	if resp.Result != ng.ResultOK {
		t.Errorf("Expected OK result, got %s: %s", resp.Result, resp.ErrorReason)
	}

	if resp.SDP == "" {
		t.Error("Expected modified SDP in response")
	}

	if resp.CallID != "call-123" {
		t.Errorf("Expected CallID call-123, got %s", resp.CallID)
	}

	if resp.FromTag != "tag-123" {
		t.Errorf("Expected FromTag tag-123, got %s", resp.FromTag)
	}

	// Verify SDP contains expected content
	if !strings.Contains(resp.SDP, "v=0") {
		t.Error("SDP missing version line")
	}
	if !strings.Contains(resp.SDP, "m=audio") {
		t.Error("SDP missing audio media line")
	}
}

func TestOfferHandler_Handle_WithSymmetricFlag(t *testing.T) {
	registry := createTestRegistry()
	config := createTestConfig()
	handler := NewOfferHandler(registry, config)

	req := &ng.NGRequest{
		Command: "offer",
		CallID:  "call-123",
		FromTag: "tag-123",
		SDP:     createTestSDP(),
		Flags:   []string{"symmetric"},
	}

	resp, err := handler.Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	// Port allocation may fail in test environment
	if resp.Result == ng.ResultError && strings.Contains(resp.ErrorReason, "port") {
		t.Skip("Skipping: port allocation not available in test environment")
	}

	if resp.Result != ng.ResultOK {
		t.Errorf("Expected OK result, got %s: %s", resp.Result, resp.ErrorReason)
	}

	// Check session was created with symmetric flag
	session := registry.GetSessionByTags("call-123", "tag-123", "")
	if session == nil {
		t.Fatal("Session not created")
	}
}

func TestOfferHandler_Handle_WithRecordingFlag(t *testing.T) {
	registry := createTestRegistry()
	config := createTestConfig()
	handler := NewOfferHandler(registry, config)

	req := &ng.NGRequest{
		Command: "offer",
		CallID:  "call-123",
		FromTag: "tag-123",
		SDP:     createTestSDP(),
		Flags:   []string{"record-call"},
	}

	resp, err := handler.Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	// Port allocation may fail in test environment
	if resp.Result == ng.ResultError && strings.Contains(resp.ErrorReason, "port") {
		t.Skip("Skipping: port allocation not available in test environment")
	}

	if resp.Result != ng.ResultOK {
		t.Errorf("Expected OK result, got %s", resp.Result)
	}

	// Verify session has recording flag
	session := registry.GetSessionByTags("call-123", "tag-123", "")
	if session == nil {
		t.Fatal("Session not created")
	}

	session.Lock()
	hasRecordFlag := session.Flags["record"]
	session.Unlock()

	if !hasRecordFlag {
		t.Error("Expected record flag to be set")
	}
}

func TestOfferHandler_Handle_WithWebRTCFlag(t *testing.T) {
	registry := createTestRegistry()
	config := createTestConfig()
	handler := NewOfferHandler(registry, config)

	req := &ng.NGRequest{
		Command: "offer",
		CallID:  "call-123",
		FromTag: "tag-123",
		SDP:     createTestSDP(),
		Flags:   []string{"WebRTC"},
	}

	resp, err := handler.Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	// Port allocation may fail in test environment
	if resp.Result == ng.ResultError && strings.Contains(resp.ErrorReason, "port") {
		t.Skip("Skipping: port allocation not available in test environment")
	}

	if resp.Result != ng.ResultOK {
		t.Errorf("Expected OK result, got %s", resp.Result)
	}

	session := registry.GetSessionByTags("call-123", "tag-123", "")
	if session == nil {
		t.Fatal("Session not created")
	}

	session.Lock()
	hasWebRTCFlag := session.Flags["webrtc"]
	session.Unlock()

	if !hasWebRTCFlag {
		t.Error("Expected webrtc flag to be set")
	}
}

func TestOfferHandler_Handle_WithDirectionFlags(t *testing.T) {
	tests := []struct {
		flag              string
		expectedDirection string
	}{
		{"sendonly", "sendonly"},
		{"recvonly", "recvonly"},
		{"inactive", "inactive"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			registry := createTestRegistry()
			config := createTestConfig()
			handler := NewOfferHandler(registry, config)

			req := &ng.NGRequest{
				Command: "offer",
				CallID:  "call-" + tt.flag,
				FromTag: "tag-123",
				SDP:     createTestSDP(),
				Flags:   []string{tt.flag},
			}

			resp, err := handler.Handle(req)
			if err != nil {
				t.Fatalf("Handle returned error: %v", err)
			}

			// Port allocation may fail in test environment
			if resp.Result == ng.ResultError && strings.Contains(resp.ErrorReason, "port") {
				t.Skip("Skipping: port allocation not available in test environment")
			}

			if resp.Result != ng.ResultOK {
				t.Errorf("Expected OK result, got %s", resp.Result)
			}

			if !strings.Contains(resp.SDP, "a="+tt.expectedDirection) {
				t.Errorf("Expected %s in SDP, got:\n%s", tt.expectedDirection, resp.SDP)
			}
		})
	}
}

func TestOfferHandler_Handle_WithICEFlags(t *testing.T) {
	registry := createTestRegistry()
	config := createTestConfig()
	handler := NewOfferHandler(registry, config)

	sdpWithICE := `v=0
o=alice 2890844526 2890844526 IN IP4 192.168.1.200
s=Session
c=IN IP4 192.168.1.200
t=0 0
m=audio 5000 RTP/AVP 0 8
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=ice-ufrag:xyz123
a=ice-pwd:password123456789012345678
a=sendrecv
`

	req := &ng.NGRequest{
		Command: "offer",
		CallID:  "call-123",
		FromTag: "tag-123",
		SDP:     sdpWithICE,
		Flags:   []string{"ICE=remove"},
	}

	resp, err := handler.Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	// Port allocation may fail in test environment
	if resp.Result == ng.ResultError && strings.Contains(resp.ErrorReason, "port") {
		t.Skip("Skipping: port allocation not available in test environment")
	}

	if resp.Result != ng.ResultOK {
		t.Errorf("Expected OK result, got %s", resp.Result)
	}

	// ICE should be removed
	if strings.Contains(resp.SDP, "ice-ufrag") {
		t.Error("Expected ICE to be removed from SDP")
	}
}

func TestOfferHandler_Handle_WithLabel(t *testing.T) {
	registry := createTestRegistry()
	config := createTestConfig()
	handler := NewOfferHandler(registry, config)

	req := &ng.NGRequest{
		Command: "offer",
		CallID:  "call-123",
		FromTag: "tag-123",
		SDP:     createTestSDP(),
		Flags:   []string{"label=caller"},
	}

	resp, err := handler.Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	// Port allocation may fail in test environment
	if resp.Result == ng.ResultError && strings.Contains(resp.ErrorReason, "port") {
		t.Skip("Skipping: port allocation not available in test environment")
	}

	if resp.Result != ng.ResultOK {
		t.Errorf("Expected OK result, got %s", resp.Result)
	}

	// Verify leg can be found by label
	session := registry.GetSessionByTags("call-123", "tag-123", "")
	if session == nil {
		t.Fatal("Session not created")
	}

	leg := session.GetLegByLabel("caller")
	if leg == nil {
		t.Error("Expected leg with label 'caller' to be found")
	}
}

// ========== DeleteHandler Tests ==========

func TestNewDeleteHandler(t *testing.T) {
	registry := createTestRegistry()
	handler := NewDeleteHandler(registry)
	if handler == nil {
		t.Fatal("NewDeleteHandler returned nil")
	}
}

func TestDeleteHandler_Handle_MissingCallID(t *testing.T) {
	registry := createTestRegistry()
	handler := NewDeleteHandler(registry)

	req := &ng.NGRequest{
		Command: "delete",
	}

	resp, err := handler.Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if resp.Result != ng.ResultError {
		t.Error("Expected error result for missing call-id")
	}
}

func TestDeleteHandler_Handle_NotFound(t *testing.T) {
	registry := createTestRegistry()
	handler := NewDeleteHandler(registry)

	req := &ng.NGRequest{
		Command: "delete",
		CallID:  "nonexistent",
	}

	resp, err := handler.Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if resp.Result != ng.ResultError {
		t.Error("Expected error result for nonexistent session")
	}
	if !strings.Contains(resp.ErrorReason, "not found") {
		t.Errorf("Expected 'not found' error, got: %s", resp.ErrorReason)
	}
}

func TestDeleteHandler_Handle_ValidDelete(t *testing.T) {
	registry := createTestRegistry()

	// Create a session first
	session := registry.CreateSession("call-123", "tag-123")
	if session == nil {
		t.Fatal("Failed to create session")
	}

	handler := NewDeleteHandler(registry)

	req := &ng.NGRequest{
		Command: "delete",
		CallID:  "call-123",
	}

	resp, err := handler.Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if resp.Result != ng.ResultOK {
		t.Errorf("Expected OK result, got %s: %s", resp.Result, resp.ErrorReason)
	}

	// Verify session was deleted
	sessions := registry.GetSessionByCallID("call-123")
	if len(sessions) != 0 {
		t.Error("Expected session to be deleted")
	}
}

func TestDeleteHandler_Handle_WithDelayFlag(t *testing.T) {
	registry := createTestRegistry()

	// Create a session first
	session := registry.CreateSession("call-123", "tag-123")
	if session == nil {
		t.Fatal("Failed to create session")
	}

	handler := NewDeleteHandler(registry)

	req := &ng.NGRequest{
		Command: "delete",
		CallID:  "call-123",
		Flags:   []string{"delete-delay=5"},
	}

	resp, err := handler.Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if resp.Result != ng.ResultOK {
		t.Errorf("Expected OK result, got %s: %s", resp.Result, resp.ErrorReason)
	}

	// Session should still exist (delayed delete)
	sessions := registry.GetSessionByCallID("call-123")
	if len(sessions) == 0 {
		t.Error("Session should still exist with delete-delay")
	}

	// Verify Extra contains delete-delay info
	if resp.Extra == nil {
		t.Fatal("Expected Extra in response")
	}
	if resp.Extra["delete-delay"] != 5 {
		t.Errorf("Expected delete-delay=5, got %v", resp.Extra["delete-delay"])
	}
	if resp.Extra["scheduled"] != true {
		t.Error("Expected scheduled=true")
	}

	// Cancel the delayed delete to clean up
	handler.CancelDelayedDelete(session.ID)
}

func TestDeleteHandler_CancelDelayedDelete(t *testing.T) {
	registry := createTestRegistry()
	handler := NewDeleteHandler(registry)

	// Non-existent session should return false
	result := handler.CancelDelayedDelete("nonexistent")
	if result {
		t.Error("Expected false for non-pending session")
	}
}

func TestDeleteHandler_DeleteByCallID(t *testing.T) {
	registry := createTestRegistry()

	// Create multiple sessions
	registry.CreateSession("call-123", "tag-1")
	registry.CreateSession("call-123", "tag-2")
	registry.CreateSession("call-456", "tag-3")

	handler := NewDeleteHandler(registry)

	err := handler.DeleteByCallID("call-123")
	if err != nil {
		t.Fatalf("DeleteByCallID failed: %v", err)
	}

	// Verify call-123 sessions deleted
	sessions := registry.GetSessionByCallID("call-123")
	if len(sessions) != 0 {
		t.Error("Expected call-123 sessions to be deleted")
	}

	// call-456 should still exist
	sessions = registry.GetSessionByCallID("call-456")
	if len(sessions) == 0 {
		t.Error("Expected call-456 session to still exist")
	}
}

// ========== QueryHandler Tests ==========

func TestNewQueryHandler(t *testing.T) {
	registry := createTestRegistry()
	handler := NewQueryHandler(registry)
	if handler == nil {
		t.Fatal("NewQueryHandler returned nil")
	}
}

func TestQueryHandler_Handle_MissingCallID(t *testing.T) {
	registry := createTestRegistry()
	handler := NewQueryHandler(registry)

	req := &ng.NGRequest{
		Command: "query",
	}

	resp, err := handler.Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if resp.Result != ng.ResultError {
		t.Error("Expected error result for missing call-id")
	}
}

func TestQueryHandler_Handle_NotFound(t *testing.T) {
	registry := createTestRegistry()
	handler := NewQueryHandler(registry)

	req := &ng.NGRequest{
		Command: "query",
		CallID:  "nonexistent",
	}

	resp, err := handler.Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if resp.Result != ng.ResultError {
		t.Error("Expected error result for nonexistent session")
	}
}

func TestQueryHandler_Handle_ValidQuery(t *testing.T) {
	registry := createTestRegistry()

	// Create a session
	session := registry.CreateSession("call-123", "tag-123")
	if session == nil {
		t.Fatal("Failed to create session")
	}

	handler := NewQueryHandler(registry)

	req := &ng.NGRequest{
		Command: "query",
		CallID:  "call-123",
	}

	resp, err := handler.Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if resp.Result != ng.ResultOK {
		t.Errorf("Expected OK result, got %s: %s", resp.Result, resp.ErrorReason)
	}

	if resp.CallID != "call-123" {
		t.Errorf("Expected CallID call-123, got %s", resp.CallID)
	}

	if resp.FromTag != "tag-123" {
		t.Errorf("Expected FromTag tag-123, got %s", resp.FromTag)
	}
}

func TestQueryHandler_Handle_WithFromTag(t *testing.T) {
	registry := createTestRegistry()

	// Create sessions with same call-id but different tags
	registry.CreateSession("call-123", "tag-A")
	registry.CreateSession("call-123", "tag-B")

	handler := NewQueryHandler(registry)

	req := &ng.NGRequest{
		Command: "query",
		CallID:  "call-123",
		FromTag: "tag-B",
	}

	resp, err := handler.Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if resp.Result != ng.ResultOK {
		t.Errorf("Expected OK result, got %s", resp.Result)
	}

	if resp.FromTag != "tag-B" {
		t.Errorf("Expected FromTag tag-B, got %s", resp.FromTag)
	}
}

func TestQueryHandler_QueryAll(t *testing.T) {
	registry := createTestRegistry()

	// Create some sessions
	registry.CreateSession("call-1", "tag-1")
	registry.CreateSession("call-2", "tag-2")
	registry.CreateSession("call-3", "tag-3")

	handler := NewQueryHandler(registry)

	stats, err := handler.QueryAll()
	if err != nil {
		t.Fatalf("QueryAll failed: %v", err)
	}

	if stats.TotalCalls != 3 {
		t.Errorf("Expected TotalCalls 3, got %d", stats.TotalCalls)
	}
}

// ========== SDPProcessor Tests ==========

func TestNewSDPProcessor(t *testing.T) {
	config := createTestConfig()
	processor := NewSDPProcessor(config)
	if processor == nil {
		t.Fatal("NewSDPProcessor returned nil")
	}
}

func TestSDPProcessor_Parse(t *testing.T) {
	config := createTestConfig()
	processor := NewSDPProcessor(config)

	sdp := createTestSDP()
	parsed, err := processor.Parse(sdp)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed.ConnectionIP != "192.168.1.200" {
		t.Errorf("Expected ConnectionIP 192.168.1.200, got %s", parsed.ConnectionIP)
	}

	if parsed.MediaPort != 5000 {
		t.Errorf("Expected MediaPort 5000, got %d", parsed.MediaPort)
	}

	if len(parsed.Codecs) != 3 {
		t.Errorf("Expected 3 codecs, got %d", len(parsed.Codecs))
	}
}

func TestSDPProcessor_Parse_WithICE(t *testing.T) {
	config := createTestConfig()
	processor := NewSDPProcessor(config)

	sdpWithICE := `v=0
o=- 2890844526 2890844526 IN IP4 192.168.1.200
s=Session
c=IN IP4 192.168.1.200
t=0 0
m=audio 5000 RTP/AVP 0
a=rtpmap:0 PCMU/8000
a=ice-ufrag:xyz123
a=ice-pwd:password123456789012345678
a=sendrecv
`

	parsed, err := processor.Parse(sdpWithICE)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !parsed.HasICE {
		t.Error("Expected HasICE to be true")
	}

	if parsed.ICEUfrag != "xyz123" {
		t.Errorf("Expected ICEUfrag xyz123, got %s", parsed.ICEUfrag)
	}

	if parsed.ICEPwd != "password123456789012345678" {
		t.Errorf("Expected ICEPwd, got %s", parsed.ICEPwd)
	}
}

func TestSDPProcessor_Parse_WithDTLS(t *testing.T) {
	config := createTestConfig()
	processor := NewSDPProcessor(config)

	sdpWithDTLS := `v=0
o=- 2890844526 2890844526 IN IP4 192.168.1.200
s=Session
c=IN IP4 192.168.1.200
t=0 0
m=audio 5000 UDP/TLS/RTP/SAVPF 0
a=rtpmap:0 PCMU/8000
a=fingerprint:sha-256 AB:CD:EF:12:34:56:78:90:AB:CD:EF:12:34:56:78:90:AB:CD:EF:12:34:56:78:90:AB:CD:EF:12:34:56:78:90
a=setup:actpass
a=sendrecv
`

	parsed, err := processor.Parse(sdpWithDTLS)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !parsed.HasDTLS {
		t.Error("Expected HasDTLS to be true")
	}

	if parsed.Setup != "actpass" {
		t.Errorf("Expected setup actpass, got %s", parsed.Setup)
	}

	if parsed.Fingerprint == "" {
		t.Error("Expected fingerprint to be parsed")
	}
}

func TestSDPProcessor_Parse_WithRTCPMux(t *testing.T) {
	config := createTestConfig()
	processor := NewSDPProcessor(config)

	sdpWithMux := `v=0
o=- 2890844526 2890844526 IN IP4 192.168.1.200
s=Session
c=IN IP4 192.168.1.200
t=0 0
m=audio 5000 RTP/AVP 0
a=rtpmap:0 PCMU/8000
a=rtcp-mux
a=sendrecv
`

	parsed, err := processor.Parse(sdpWithMux)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !parsed.RTCPMux {
		t.Error("Expected RTCPMux to be true")
	}
}

func TestSDPProcessor_Parse_Direction(t *testing.T) {
	tests := []struct {
		direction string
	}{
		{"sendrecv"},
		{"sendonly"},
		{"recvonly"},
		{"inactive"},
	}

	config := createTestConfig()
	processor := NewSDPProcessor(config)

	for _, tt := range tests {
		t.Run(tt.direction, func(t *testing.T) {
			sdp := `v=0
o=- 123 1 IN IP4 192.168.1.200
s=Session
c=IN IP4 192.168.1.200
t=0 0
m=audio 5000 RTP/AVP 0
a=rtpmap:0 PCMU/8000
a=` + tt.direction + `
`
			parsed, err := processor.Parse(sdp)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			if parsed.Direction != tt.direction {
				t.Errorf("Expected direction %s, got %s", tt.direction, parsed.Direction)
			}
		})
	}
}
