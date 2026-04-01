package ng_protocol

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// Integration tests for OpenSIPS/Kamailio compatibility
// These tests verify that Karl's NG protocol implementation is compatible
// with the rtpengine module used by OpenSIPS and Kamailio
//
// To run these tests, start a Karl server with NG protocol enabled:
//   go run . -config config.json
//
// Then run with:
//   go test -v ./internal/ng_protocol -run TestIntegration
//
// Use -short flag to skip integration tests:
//   go test -short ./...

const integrationTestServer = "127.0.0.1:22222"

// skipIfServerUnavailable checks if the NG protocol server is running
// and skips the test if it's not available
func skipIfServerUnavailable(t *testing.T) {
	t.Helper()
	conn, err := net.DialTimeout("udp", integrationTestServer, 100*time.Millisecond)
	if err != nil {
		t.Skipf("NG protocol server not available at %s: %v", integrationTestServer, err)
	}
	// Send a quick ping to verify the server responds
	conn.SetDeadline(time.Now().Add(200 * time.Millisecond))
	_, _ = conn.Write([]byte("test_probe d4:ping4:pinge"))
	buf := make([]byte, 256)
	_, err = conn.Read(buf)
	conn.Close()
	if err != nil {
		t.Skipf("NG protocol server not responding at %s (use -short to skip integration tests)", integrationTestServer)
	}
}

// TestClient simulates an OpenSIPS/Kamailio rtpengine client
type TestClient struct {
	t          *testing.T
	conn       *net.UDPConn
	serverAddr *net.UDPAddr
	cookieSeq  int
	mu         sync.Mutex
}

// NewTestClient creates a new test client
func NewTestClient(t *testing.T, serverAddr string) *TestClient {
	addr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		t.Fatalf("Failed to resolve server address: %v", err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}

	return &TestClient{
		t:          t,
		conn:       conn,
		serverAddr: addr,
		cookieSeq:  0,
	}
}

// Close closes the client connection
func (c *TestClient) Close() {
	c.conn.Close()
}

// nextCookie generates a unique cookie for each request
func (c *TestClient) nextCookie() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cookieSeq++
	return fmt.Sprintf("test_%d_%d", time.Now().UnixNano(), c.cookieSeq)
}

// SendCommand sends an NG protocol command and returns the response
func (c *TestClient) SendCommand(params map[string]interface{}) (string, map[string]interface{}, error) {
	cookie := c.nextCookie()

	// Encode the command
	encoder := NewEncoder()
	encoded, err := encoder.Encode(params)
	if err != nil {
		return "", nil, fmt.Errorf("failed to encode command: %w", err)
	}

	// Build the message: cookie + space + bencode
	msg := make([]byte, 0, len(cookie)+1+len(encoded))
	msg = append(msg, []byte(cookie)...)
	msg = append(msg, ' ')
	msg = append(msg, encoded...)

	// Send the message
	c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = c.conn.Write(msg)
	if err != nil {
		return "", nil, fmt.Errorf("failed to send command: %w", err)
	}

	// Read the response
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 65536)
	n, err := c.conn.Read(buf)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse the response
	respCookie, respParams, err := c.parseResponse(buf[:n])
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Verify cookie matches
	if respCookie != cookie {
		return "", nil, fmt.Errorf("cookie mismatch: sent %s, got %s", cookie, respCookie)
	}

	return respCookie, respParams, nil
}

// parseResponse parses an NG protocol response
func (c *TestClient) parseResponse(data []byte) (string, map[string]interface{}, error) {
	spaceIdx := bytes.IndexByte(data, ' ')
	if spaceIdx == -1 {
		return "", nil, fmt.Errorf("no space in response")
	}

	cookie := string(data[:spaceIdx])
	bencodeData := data[spaceIdx+1:]

	decoder := NewDecoder(bencodeData)
	decoded, err := decoder.Decode()
	if err != nil {
		return "", nil, err
	}

	dict, ok := decoded.(BencodeDict)
	if !ok {
		return "", nil, fmt.Errorf("response is not a dictionary")
	}

	// Convert BencodeDict to map[string]interface{}
	result := make(map[string]interface{})
	for k, v := range dict {
		result[k] = v
	}

	return cookie, result, nil
}

// Ping sends a ping command
func (c *TestClient) Ping() (string, error) {
	_, resp, err := c.SendCommand(map[string]interface{}{
		"command": "ping",
	})
	if err != nil {
		return "", err
	}
	result, _ := resp["result"].(string)
	return result, nil
}

// Offer sends an offer command (OpenSIPS/Kamailio format)
func (c *TestClient) Offer(callID, fromTag, sdp string, flags []string, options map[string]interface{}) (map[string]interface{}, error) {
	params := map[string]interface{}{
		"command":  "offer",
		"call-id":  callID,
		"from-tag": fromTag,
		"sdp":      sdp,
	}

	if len(flags) > 0 {
		flagList := make([]interface{}, len(flags))
		for i, f := range flags {
			flagList[i] = f
		}
		params["flags"] = flagList
	}

	for k, v := range options {
		params[k] = v
	}

	_, resp, err := c.SendCommand(params)
	return resp, err
}

// Answer sends an answer command
func (c *TestClient) Answer(callID, fromTag, toTag, sdp string, flags []string, options map[string]interface{}) (map[string]interface{}, error) {
	params := map[string]interface{}{
		"command":  "answer",
		"call-id":  callID,
		"from-tag": fromTag,
		"to-tag":   toTag,
		"sdp":      sdp,
	}

	if len(flags) > 0 {
		flagList := make([]interface{}, len(flags))
		for i, f := range flags {
			flagList[i] = f
		}
		params["flags"] = flagList
	}

	for k, v := range options {
		params[k] = v
	}

	_, resp, err := c.SendCommand(params)
	return resp, err
}

// Delete sends a delete command
func (c *TestClient) Delete(callID, fromTag, toTag string, flags []string) (map[string]interface{}, error) {
	params := map[string]interface{}{
		"command":  "delete",
		"call-id":  callID,
		"from-tag": fromTag,
	}

	if toTag != "" {
		params["to-tag"] = toTag
	}

	if len(flags) > 0 {
		flagList := make([]interface{}, len(flags))
		for i, f := range flags {
			flagList[i] = f
		}
		params["flags"] = flagList
	}

	_, resp, err := c.SendCommand(params)
	return resp, err
}

// Query sends a query command
func (c *TestClient) Query(callID, fromTag, toTag string) (map[string]interface{}, error) {
	params := map[string]interface{}{
		"command":  "query",
		"call-id":  callID,
		"from-tag": fromTag,
	}

	if toTag != "" {
		params["to-tag"] = toTag
	}

	_, resp, err := c.SendCommand(params)
	return resp, err
}

// List sends a list command
func (c *TestClient) List(limit int) (map[string]interface{}, error) {
	params := map[string]interface{}{
		"command": "list",
	}

	if limit > 0 {
		params["limit"] = int64(limit)
	}

	_, resp, err := c.SendCommand(params)
	return resp, err
}

// StartRecording sends a start recording command
func (c *TestClient) StartRecording(callID, fromTag string) (map[string]interface{}, error) {
	params := map[string]interface{}{
		"command":  "start recording",
		"call-id":  callID,
		"from-tag": fromTag,
	}

	_, resp, err := c.SendCommand(params)
	return resp, err
}

// StopRecording sends a stop recording command
func (c *TestClient) StopRecording(callID, fromTag string) (map[string]interface{}, error) {
	params := map[string]interface{}{
		"command":  "stop recording",
		"call-id":  callID,
		"from-tag": fromTag,
	}

	_, resp, err := c.SendCommand(params)
	return resp, err
}

// BlockMedia sends a block media command
func (c *TestClient) BlockMedia(callID, fromTag string) (map[string]interface{}, error) {
	params := map[string]interface{}{
		"command":  "block media",
		"call-id":  callID,
		"from-tag": fromTag,
	}

	_, resp, err := c.SendCommand(params)
	return resp, err
}

// UnblockMedia sends an unblock media command
func (c *TestClient) UnblockMedia(callID, fromTag string) (map[string]interface{}, error) {
	params := map[string]interface{}{
		"command":  "unblock media",
		"call-id":  callID,
		"from-tag": fromTag,
	}

	_, resp, err := c.SendCommand(params)
	return resp, err
}

// PlayDTMF sends a play DTMF command
func (c *TestClient) PlayDTMF(callID, fromTag, digit string, duration int) (map[string]interface{}, error) {
	params := map[string]interface{}{
		"command":  "play DTMF",
		"call-id":  callID,
		"from-tag": fromTag,
		"digit":    digit,
	}

	if duration > 0 {
		params["duration"] = int64(duration)
	}

	_, resp, err := c.SendCommand(params)
	return resp, err
}

// Standard SDP for testing
const testSDPOffer = `v=0
o=- 123456789 987654321 IN IP4 192.168.1.100
s=-
c=IN IP4 192.168.1.100
t=0 0
m=audio 10000 RTP/AVP 0 8 101
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:101 telephone-event/8000
a=fmtp:101 0-16
a=sendrecv
`

const testSDPAnswer = `v=0
o=- 987654321 123456789 IN IP4 192.168.1.200
s=-
c=IN IP4 192.168.1.200
t=0 0
m=audio 20000 RTP/AVP 0 8 101
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:101 telephone-event/8000
a=fmtp:101 0-16
a=sendrecv
`

const testWebRTCSDP = `v=0
o=- 123456789 987654321 IN IP4 0.0.0.0
s=-
t=0 0
a=group:BUNDLE audio
a=msid-semantic: WMS
m=audio 9 UDP/TLS/RTP/SAVPF 111 103 104 9 0 8 106 105 13 110 112 113 126
c=IN IP4 0.0.0.0
a=rtcp:9 IN IP4 0.0.0.0
a=ice-ufrag:abcd
a=ice-pwd:efghijklmnopqrstuvwxyz
a=ice-options:trickle
a=fingerprint:sha-256 AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99
a=setup:actpass
a=mid:audio
a=extmap:1 urn:ietf:params:rtp-hdrext:ssrc-audio-level
a=sendrecv
a=rtcp-mux
a=rtpmap:111 opus/48000/2
a=rtcp-fb:111 transport-cc
a=fmtp:111 minptime=10;useinbandfec=1
a=rtpmap:103 ISAC/16000
a=rtpmap:104 ISAC/32000
a=rtpmap:9 G722/8000
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
`

// =============================================================================
// Integration Test Functions
// =============================================================================

// TestIntegration_PingPong tests the basic ping/pong functionality
func TestIntegration_PingPong(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	result, err := client.Ping()
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	if result != "pong" {
		t.Errorf("Expected 'pong', got '%s'", result)
	}
}

// TestIntegration_BasicCallFlow tests a complete offer/answer/delete flow
func TestIntegration_BasicCallFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	callID := fmt.Sprintf("test-call-%d", time.Now().UnixNano())
	fromTag := "from-tag-12345"
	toTag := "to-tag-67890"

	// Step 1: Send offer
	offerResp, err := client.Offer(callID, fromTag, testSDPOffer, nil, nil)
	if err != nil {
		t.Fatalf("Offer failed: %v", err)
	}

	result, _ := offerResp["result"].(string)
	if result != "ok" {
		errorReason, _ := offerResp["error-reason"].(string)
		t.Fatalf("Offer returned %s: %s", result, errorReason)
	}

	// Verify SDP in response
	sdp, _ := offerResp["sdp"].(string)
	if sdp == "" {
		t.Error("No SDP in offer response")
	}

	// Verify SDP contains required elements
	if !strings.Contains(sdp, "m=audio") {
		t.Error("SDP missing audio media line")
	}

	// Step 2: Send answer
	answerResp, err := client.Answer(callID, fromTag, toTag, testSDPAnswer, nil, nil)
	if err != nil {
		t.Fatalf("Answer failed: %v", err)
	}

	result, _ = answerResp["result"].(string)
	if result != "ok" {
		errorReason, _ := answerResp["error-reason"].(string)
		t.Fatalf("Answer returned %s: %s", result, errorReason)
	}

	// Step 3: Query the call
	queryResp, err := client.Query(callID, fromTag, toTag)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	result, _ = queryResp["result"].(string)
	if result != "ok" {
		t.Errorf("Query returned %s", result)
	}

	// Step 4: Delete the call
	deleteResp, err := client.Delete(callID, fromTag, toTag, nil)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	result, _ = deleteResp["result"].(string)
	if result != "ok" {
		errorReason, _ := deleteResp["error-reason"].(string)
		t.Errorf("Delete returned %s: %s", result, errorReason)
	}
}

// TestIntegration_OfferWithFlags tests offer with various flags
func TestIntegration_OfferWithFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	tests := []struct {
		name  string
		flags []string
	}{
		{"symmetric", []string{"symmetric"}},
		{"ICE-remove", []string{"ICE=remove"}},
		{"trust-address", []string{"trust-address"}},
		{"SIP-source-address", []string{"SIP-source-address"}},
		{"replace-origin", []string{"replace-origin"}},
		{"record-call", []string{"record-call"}},
		{"multiple-flags", []string{"symmetric", "trust-address", "replace-origin"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callID := fmt.Sprintf("test-flags-%s-%d", tt.name, time.Now().UnixNano())
			fromTag := "from-tag-flags"

			resp, err := client.Offer(callID, fromTag, testSDPOffer, tt.flags, nil)
			if err != nil {
				t.Fatalf("Offer failed: %v", err)
			}

			result, _ := resp["result"].(string)
			if result != "ok" {
				errorReason, _ := resp["error-reason"].(string)
				t.Errorf("Offer with flags %v returned %s: %s", tt.flags, result, errorReason)
			}

			// Cleanup
			client.Delete(callID, fromTag, "", nil)
		})
	}
}

// TestIntegration_WebRTCBridging tests WebRTC to SIP bridging
func TestIntegration_WebRTCBridging(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	callID := fmt.Sprintf("webrtc-call-%d", time.Now().UnixNano())
	fromTag := "webrtc-from"
	toTag := "sip-to"

	// WebRTC side offer with ICE and DTLS
	flags := []string{
		"ICE=force",
		"DTLS=passive",
		"SDES-off",
		"RTCP-mux-offer",
	}

	resp, err := client.Offer(callID, fromTag, testWebRTCSDP, flags, nil)
	if err != nil {
		t.Fatalf("WebRTC offer failed: %v", err)
	}

	result, _ := resp["result"].(string)
	if result != "ok" {
		errorReason, _ := resp["error-reason"].(string)
		t.Fatalf("WebRTC offer returned %s: %s", result, errorReason)
	}

	sdp, _ := resp["sdp"].(string)
	if sdp == "" {
		t.Error("No SDP in WebRTC offer response")
	}

	// Verify ICE credentials in response
	if !strings.Contains(sdp, "a=ice-ufrag") {
		t.Error("Response SDP missing ICE ufrag")
	}
	if !strings.Contains(sdp, "a=ice-pwd") {
		t.Error("Response SDP missing ICE pwd")
	}

	// SIP side answer (plain RTP)
	sipFlags := []string{
		"ICE=remove",
		"RTP/AVP",
	}

	answerResp, err := client.Answer(callID, fromTag, toTag, testSDPAnswer, sipFlags, nil)
	if err != nil {
		t.Fatalf("SIP answer failed: %v", err)
	}

	result, _ = answerResp["result"].(string)
	if result != "ok" {
		errorReason, _ := answerResp["error-reason"].(string)
		t.Errorf("SIP answer returned %s: %s", result, errorReason)
	}

	// Cleanup
	client.Delete(callID, fromTag, toTag, nil)
}

// TestIntegration_DirectionFlags tests direction handling
func TestIntegration_DirectionFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	tests := []struct {
		name       string
		directions []string
	}{
		{"internal-external", []string{"internal", "external"}},
		{"external-internal", []string{"external", "internal"}},
		{"pub-priv", []string{"pub", "priv"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callID := fmt.Sprintf("direction-%s-%d", tt.name, time.Now().UnixNano())
			fromTag := "from-direction"

			directionList := make([]interface{}, len(tt.directions))
			for i, d := range tt.directions {
				directionList[i] = d
			}

			resp, err := client.Offer(callID, fromTag, testSDPOffer, nil, map[string]interface{}{
				"direction": directionList,
			})
			if err != nil {
				t.Fatalf("Offer failed: %v", err)
			}

			result, _ := resp["result"].(string)
			if result != "ok" {
				errorReason, _ := resp["error-reason"].(string)
				t.Errorf("Offer with direction %v returned %s: %s", tt.directions, result, errorReason)
			}

			// Cleanup
			client.Delete(callID, fromTag, "", nil)
		})
	}
}

// TestIntegration_ReINVITE tests re-INVITE scenarios
func TestIntegration_ReINVITE(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	callID := fmt.Sprintf("reinvite-%d", time.Now().UnixNano())
	fromTag := "from-reinvite"
	toTag := "to-reinvite"

	// Initial offer
	_, err := client.Offer(callID, fromTag, testSDPOffer, nil, nil)
	if err != nil {
		t.Fatalf("Initial offer failed: %v", err)
	}

	// Initial answer
	_, err = client.Answer(callID, fromTag, toTag, testSDPAnswer, nil, nil)
	if err != nil {
		t.Fatalf("Initial answer failed: %v", err)
	}

	// Re-INVITE offer (new SDP version)
	reInviteSDP := strings.Replace(testSDPOffer, "987654321", "987654322", 1)
	resp, err := client.Offer(callID, fromTag, reInviteSDP, nil, nil)
	if err != nil {
		t.Fatalf("Re-INVITE offer failed: %v", err)
	}

	result, _ := resp["result"].(string)
	if result != "ok" {
		errorReason, _ := resp["error-reason"].(string)
		t.Errorf("Re-INVITE returned %s: %s", result, errorReason)
	}

	// Re-INVITE answer
	reAnswerSDP := strings.Replace(testSDPAnswer, "123456789", "123456790", 1)
	answerResp, err := client.Answer(callID, fromTag, toTag, reAnswerSDP, nil, nil)
	if err != nil {
		t.Fatalf("Re-INVITE answer failed: %v", err)
	}

	result, _ = answerResp["result"].(string)
	if result != "ok" {
		t.Errorf("Re-INVITE answer returned %s", result)
	}

	// Cleanup
	client.Delete(callID, fromTag, toTag, nil)
}

// TestIntegration_HoldResume tests call hold and resume
func TestIntegration_HoldResume(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	callID := fmt.Sprintf("hold-%d", time.Now().UnixNano())
	fromTag := "from-hold"
	toTag := "to-hold"

	// Initial call setup
	_, err := client.Offer(callID, fromTag, testSDPOffer, nil, nil)
	if err != nil {
		t.Fatalf("Initial offer failed: %v", err)
	}

	_, err = client.Answer(callID, fromTag, toTag, testSDPAnswer, nil, nil)
	if err != nil {
		t.Fatalf("Initial answer failed: %v", err)
	}

	// Hold (sendonly)
	holdSDP := strings.Replace(testSDPOffer, "a=sendrecv", "a=sendonly", 1)
	holdSDP = strings.Replace(holdSDP, "c=IN IP4 192.168.1.100", "c=IN IP4 0.0.0.0", 1)
	resp, err := client.Offer(callID, fromTag, holdSDP, nil, nil)
	if err != nil {
		t.Fatalf("Hold offer failed: %v", err)
	}

	result, _ := resp["result"].(string)
	if result != "ok" {
		t.Errorf("Hold returned %s", result)
	}

	// Resume (sendrecv)
	resp, err = client.Offer(callID, fromTag, testSDPOffer, nil, nil)
	if err != nil {
		t.Fatalf("Resume offer failed: %v", err)
	}

	result, _ = resp["result"].(string)
	if result != "ok" {
		t.Errorf("Resume returned %s", result)
	}

	// Cleanup
	client.Delete(callID, fromTag, toTag, nil)
}

// TestIntegration_Recording tests call recording
func TestIntegration_Recording(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	callID := fmt.Sprintf("record-%d", time.Now().UnixNano())
	fromTag := "from-record"
	toTag := "to-record"

	// Setup call with recording flag
	flags := []string{"record-call"}
	_, err := client.Offer(callID, fromTag, testSDPOffer, flags, nil)
	if err != nil {
		t.Fatalf("Offer with recording failed: %v", err)
	}

	_, err = client.Answer(callID, fromTag, toTag, testSDPAnswer, nil, nil)
	if err != nil {
		t.Fatalf("Answer failed: %v", err)
	}

	// Start recording explicitly
	resp, err := client.StartRecording(callID, fromTag)
	if err != nil {
		t.Fatalf("StartRecording failed: %v", err)
	}

	result, _ := resp["result"].(string)
	if result != "ok" {
		t.Errorf("StartRecording returned %s", result)
	}

	// Stop recording
	resp, err = client.StopRecording(callID, fromTag)
	if err != nil {
		t.Fatalf("StopRecording failed: %v", err)
	}

	result, _ = resp["result"].(string)
	if result != "ok" {
		t.Errorf("StopRecording returned %s", result)
	}

	// Cleanup
	client.Delete(callID, fromTag, toTag, nil)
}

// TestIntegration_MediaControl tests media blocking
func TestIntegration_MediaControl(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	callID := fmt.Sprintf("media-ctrl-%d", time.Now().UnixNano())
	fromTag := "from-media"
	toTag := "to-media"

	// Setup call
	_, err := client.Offer(callID, fromTag, testSDPOffer, nil, nil)
	if err != nil {
		t.Fatalf("Offer failed: %v", err)
	}

	_, err = client.Answer(callID, fromTag, toTag, testSDPAnswer, nil, nil)
	if err != nil {
		t.Fatalf("Answer failed: %v", err)
	}

	// Block media
	resp, err := client.BlockMedia(callID, fromTag)
	if err != nil {
		t.Fatalf("BlockMedia failed: %v", err)
	}

	result, _ := resp["result"].(string)
	if result != "ok" {
		t.Errorf("BlockMedia returned %s", result)
	}

	// Unblock media
	resp, err = client.UnblockMedia(callID, fromTag)
	if err != nil {
		t.Fatalf("UnblockMedia failed: %v", err)
	}

	result, _ = resp["result"].(string)
	if result != "ok" {
		t.Errorf("UnblockMedia returned %s", result)
	}

	// Cleanup
	client.Delete(callID, fromTag, toTag, nil)
}

// TestIntegration_DTMF tests DTMF injection
func TestIntegration_DTMF(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	callID := fmt.Sprintf("dtmf-%d", time.Now().UnixNano())
	fromTag := "from-dtmf"
	toTag := "to-dtmf"

	// Setup call
	_, err := client.Offer(callID, fromTag, testSDPOffer, nil, nil)
	if err != nil {
		t.Fatalf("Offer failed: %v", err)
	}

	_, err = client.Answer(callID, fromTag, toTag, testSDPAnswer, nil, nil)
	if err != nil {
		t.Fatalf("Answer failed: %v", err)
	}

	// Play DTMF digits
	digits := []string{"1", "2", "3", "*", "#"}
	for _, digit := range digits {
		resp, err := client.PlayDTMF(callID, fromTag, digit, 100)
		if err != nil {
			t.Fatalf("PlayDTMF failed for digit %s: %v", digit, err)
		}

		result, _ := resp["result"].(string)
		if result != "ok" {
			t.Errorf("PlayDTMF for digit %s returned %s", digit, result)
		}
	}

	// Cleanup
	client.Delete(callID, fromTag, toTag, nil)
}

// TestIntegration_ListCalls tests list command
func TestIntegration_ListCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	// Create multiple calls
	callIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		callIDs[i] = fmt.Sprintf("list-test-%d-%d", i, time.Now().UnixNano())
		fromTag := fmt.Sprintf("from-%d", i)

		_, err := client.Offer(callIDs[i], fromTag, testSDPOffer, nil, nil)
		if err != nil {
			t.Fatalf("Offer %d failed: %v", i, err)
		}
	}

	// List calls
	resp, err := client.List(10)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	result, _ := resp["result"].(string)
	if result != "ok" {
		t.Errorf("List returned %s", result)
	}

	// Cleanup
	for i, callID := range callIDs {
		fromTag := fmt.Sprintf("from-%d", i)
		client.Delete(callID, fromTag, "", nil)
	}
}

// TestIntegration_ErrorHandling tests error responses
func TestIntegration_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	tests := []struct {
		name          string
		params        map[string]interface{}
		expectResult  string
		expectContain string
	}{
		{
			name: "missing-call-id",
			params: map[string]interface{}{
				"command":  "offer",
				"from-tag": "test",
				"sdp":      testSDPOffer,
			},
			expectResult: "error",
		},
		{
			name: "missing-from-tag",
			params: map[string]interface{}{
				"command": "offer",
				"call-id": "test-123",
				"sdp":     testSDPOffer,
			},
			expectResult: "error",
		},
		{
			name: "delete-nonexistent",
			params: map[string]interface{}{
				"command":  "delete",
				"call-id":  "nonexistent-call-id",
				"from-tag": "nonexistent-tag",
			},
			expectResult: "error",
		},
		{
			name: "query-nonexistent",
			params: map[string]interface{}{
				"command":  "query",
				"call-id":  "nonexistent-call-id",
				"from-tag": "nonexistent-tag",
			},
			expectResult: "error",
		},
		{
			name: "unknown-command",
			params: map[string]interface{}{
				"command": "foobar",
			},
			expectResult: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, resp, err := client.SendCommand(tt.params)
			if err != nil {
				t.Fatalf("SendCommand failed: %v", err)
			}

			result, _ := resp["result"].(string)
			if result != tt.expectResult {
				t.Errorf("Expected result '%s', got '%s'", tt.expectResult, result)
			}
		})
	}
}

// TestIntegration_ConcurrentCalls tests concurrent call handling
func TestIntegration_ConcurrentCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	numCalls := 10
	var wg sync.WaitGroup
	errors := make(chan error, numCalls)

	for i := 0; i < numCalls; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			callID := fmt.Sprintf("concurrent-%d-%d", idx, time.Now().UnixNano())
			fromTag := fmt.Sprintf("from-%d", idx)
			toTag := fmt.Sprintf("to-%d", idx)

			// Offer
			resp, err := client.Offer(callID, fromTag, testSDPOffer, nil, nil)
			if err != nil {
				errors <- fmt.Errorf("offer %d: %w", idx, err)
				return
			}
			result, _ := resp["result"].(string)
			if result != "ok" {
				errors <- fmt.Errorf("offer %d: result=%s", idx, result)
				return
			}

			// Answer
			resp, err = client.Answer(callID, fromTag, toTag, testSDPAnswer, nil, nil)
			if err != nil {
				errors <- fmt.Errorf("answer %d: %w", idx, err)
				return
			}
			result, _ = resp["result"].(string)
			if result != "ok" {
				errors <- fmt.Errorf("answer %d: result=%s", idx, result)
				return
			}

			// Delete
			resp, err = client.Delete(callID, fromTag, toTag, nil)
			if err != nil {
				errors <- fmt.Errorf("delete %d: %w", idx, err)
				return
			}
			result, _ = resp["result"].(string)
			if result != "ok" {
				errors <- fmt.Errorf("delete %d: result=%s", idx, result)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	var errList []error
	for err := range errors {
		errList = append(errList, err)
	}

	if len(errList) > 0 {
		for _, err := range errList {
			t.Errorf("Concurrent call error: %v", err)
		}
	}
}

// TestIntegration_LabelSupport tests label-based leg identification
func TestIntegration_LabelSupport(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	callID := fmt.Sprintf("label-%d", time.Now().UnixNano())
	fromTag := "from-label"
	toTag := "to-label"

	// Offer with label
	resp, err := client.Offer(callID, fromTag, testSDPOffer, nil, map[string]interface{}{
		"label": "caller",
	})
	if err != nil {
		t.Fatalf("Offer with label failed: %v", err)
	}

	result, _ := resp["result"].(string)
	if result != "ok" {
		t.Errorf("Offer with label returned %s", result)
	}

	// Answer with label
	resp, err = client.Answer(callID, fromTag, toTag, testSDPAnswer, nil, map[string]interface{}{
		"label": "callee",
	})
	if err != nil {
		t.Fatalf("Answer with label failed: %v", err)
	}

	result, _ = resp["result"].(string)
	if result != "ok" {
		t.Errorf("Answer with label returned %s", result)
	}

	// Cleanup
	client.Delete(callID, fromTag, toTag, nil)
}

// TestIntegration_TranscodeFlags tests transcoding flags
func TestIntegration_TranscodeFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	tests := []struct {
		name      string
		transcode []string
		codec     []string
	}{
		{"transcode-opus", []string{"opus"}, nil},
		{"transcode-g722", []string{"G722"}, nil},
		{"codec-strip", nil, []string{"strip-all", "offer-PCMU"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callID := fmt.Sprintf("transcode-%s-%d", tt.name, time.Now().UnixNano())
			fromTag := "from-transcode"

			options := make(map[string]interface{})
			if len(tt.transcode) > 0 {
				transcodeList := make([]interface{}, len(tt.transcode))
				for i, t := range tt.transcode {
					transcodeList[i] = t
				}
				options["transcode"] = transcodeList
			}
			if len(tt.codec) > 0 {
				codecList := make([]interface{}, len(tt.codec))
				for i, c := range tt.codec {
					codecList[i] = c
				}
				options["codec"] = codecList
			}

			resp, err := client.Offer(callID, fromTag, testSDPOffer, nil, options)
			if err != nil {
				t.Fatalf("Offer failed: %v", err)
			}

			result, _ := resp["result"].(string)
			if result != "ok" {
				errorReason, _ := resp["error-reason"].(string)
				t.Errorf("Offer with transcode returned %s: %s", result, errorReason)
			}

			// Cleanup
			client.Delete(callID, fromTag, "", nil)
		})
	}
}

// TestIntegration_ForkedCall tests forked call scenarios (multiple to-tags)
func TestIntegration_ForkedCall(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	callID := fmt.Sprintf("forked-%d", time.Now().UnixNano())
	fromTag := "from-forked"

	// Initial offer
	_, err := client.Offer(callID, fromTag, testSDPOffer, nil, nil)
	if err != nil {
		t.Fatalf("Initial offer failed: %v", err)
	}

	// Multiple answers with different to-tags (forking)
	toTags := []string{"to-fork-1", "to-fork-2", "to-fork-3"}
	for _, toTag := range toTags {
		resp, err := client.Answer(callID, fromTag, toTag, testSDPAnswer, nil, nil)
		if err != nil {
			t.Errorf("Answer for %s failed: %v", toTag, err)
			continue
		}

		result, _ := resp["result"].(string)
		if result != "ok" {
			t.Errorf("Answer for %s returned %s", toTag, result)
		}
	}

	// Delete one fork
	resp, err := client.Delete(callID, fromTag, "to-fork-1", nil)
	if err != nil {
		t.Fatalf("Delete fork failed: %v", err)
	}
	result, _ := resp["result"].(string)
	if result != "ok" {
		t.Errorf("Delete fork returned %s", result)
	}

	// Delete entire call
	client.Delete(callID, fromTag, "", nil)
}

// TestIntegration_ResponseFormat tests response format compatibility with rtpengine
func TestIntegration_ResponseFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	callID := fmt.Sprintf("format-%d", time.Now().UnixNano())
	fromTag := "from-format"
	toTag := "to-format"

	// Offer
	offerResp, err := client.Offer(callID, fromTag, testSDPOffer, nil, nil)
	if err != nil {
		t.Fatalf("Offer failed: %v", err)
	}

	// Verify required fields in offer response
	requiredOfferFields := []string{"result", "sdp"}
	for _, field := range requiredOfferFields {
		if _, ok := offerResp[field]; !ok {
			t.Errorf("Offer response missing required field: %s", field)
		}
	}

	// Answer
	answerResp, err := client.Answer(callID, fromTag, toTag, testSDPAnswer, nil, nil)
	if err != nil {
		t.Fatalf("Answer failed: %v", err)
	}

	// Verify required fields in answer response
	requiredAnswerFields := []string{"result", "sdp"}
	for _, field := range requiredAnswerFields {
		if _, ok := answerResp[field]; !ok {
			t.Errorf("Answer response missing required field: %s", field)
		}
	}

	// Query
	queryResp, err := client.Query(callID, fromTag, toTag)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// Verify result field
	if _, ok := queryResp["result"]; !ok {
		t.Error("Query response missing result field")
	}

	// Cleanup
	client.Delete(callID, fromTag, toTag, nil)
}

// TestIntegration_SDPManipulation tests SDP manipulation flags
func TestIntegration_SDPManipulation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfServerUnavailable(t)

	client := NewTestClient(t, integrationTestServer)
	defer client.Close()

	tests := []struct {
		name    string
		replace []string
		flags   []string
		check   func(t *testing.T, sdp string)
	}{
		{
			name:    "replace-origin",
			flags:   []string{"replace-origin"},
			replace: []string{"origin"},
			check: func(t *testing.T, sdp string) {
				if !strings.Contains(sdp, "o=") {
					t.Error("SDP missing origin line")
				}
			},
		},
		{
			name:    "replace-session-connection",
			flags:   []string{"replace-session-connection"},
			replace: []string{"session-connection"},
			check: func(t *testing.T, sdp string) {
				if !strings.Contains(sdp, "c=") {
					t.Error("SDP missing connection line")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callID := fmt.Sprintf("sdp-manip-%s-%d", tt.name, time.Now().UnixNano())
			fromTag := "from-sdp-manip"

			options := make(map[string]interface{})
			if len(tt.replace) > 0 {
				replaceList := make([]interface{}, len(tt.replace))
				for i, r := range tt.replace {
					replaceList[i] = r
				}
				options["replace"] = replaceList
			}

			resp, err := client.Offer(callID, fromTag, testSDPOffer, tt.flags, options)
			if err != nil {
				t.Fatalf("Offer failed: %v", err)
			}

			result, _ := resp["result"].(string)
			if result != "ok" {
				errorReason, _ := resp["error-reason"].(string)
				t.Errorf("Offer returned %s: %s", result, errorReason)
				return
			}

			sdp, _ := resp["sdp"].(string)
			if tt.check != nil && sdp != "" {
				tt.check(t, sdp)
			}

			// Cleanup
			client.Delete(callID, fromTag, "", nil)
		})
	}
}
