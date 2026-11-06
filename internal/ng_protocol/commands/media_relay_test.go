package commands

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"karl/internal"
	ng "karl/internal/ng_protocol"
)

// buildRTPPacket builds a minimal valid RTP packet with the given payload.
func buildRTPPacket(seq uint16, ts uint32, ssrc uint32, payload []byte) []byte {
	pkt := make([]byte, 12+len(payload))
	pkt[0] = 0x80 // V=2
	pkt[1] = 0    // PT=0 (PCMU)
	binary.BigEndian.PutUint16(pkt[2:4], seq)
	binary.BigEndian.PutUint32(pkt[4:8], ts)
	binary.BigEndian.PutUint32(pkt[8:12], ssrc)
	copy(pkt[12:], payload)
	return pkt
}

// TestFullMediaRelayFlow exercises offer → answer → RTP forwarding → delete.
func TestFullMediaRelayFlow(t *testing.T) {
	registry := createTestRegistry()
	config := createTestConfig()
	// Use localhost and a narrow ephemeral range for the test
	config.Integration.MediaIP = "127.0.0.1"
	config.Integration.PublicIP = ""
	config.Sessions = &internal.SessionConfig{MinPort: 40000, MaxPort: 41000}

	offerHandler := NewOfferHandler(registry, config)
	answerHandler := NewAnswerHandler(registry, config)

	// --- Step 1: Offer ---
	callerSDP := "v=0\r\no=caller 1 1 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 0 RTP/AVP 0\r\na=rtpmap:0 PCMU/8000\r\na=sendrecv\r\n"

	offerResp, err := offerHandler.Handle(&ng.NGRequest{
		Command: "offer",
		CallID:  "test-media-flow",
		FromTag: "caller-tag",
		SDP:     callerSDP,
	})
	if err != nil {
		t.Fatalf("Offer failed: %v", err)
	}
	if offerResp.Result != ng.ResultOK {
		t.Fatalf("Offer not OK: %s", offerResp.ErrorReason)
	}
	if len(offerResp.Streams) == 0 {
		t.Fatal("No streams in offer response")
	}

	callerLegPort := offerResp.Streams[0].LocalPort
	t.Logf("Offer allocated caller-leg local port: %d", callerLegPort)

	// Verify SDP was returned and contains the allocated port
	if !strings.Contains(offerResp.SDP, fmt.Sprintf("m=audio %d", callerLegPort)) {
		t.Errorf("Offer SDP doesn't contain allocated port %d:\n%s", callerLegPort, offerResp.SDP)
	}

	// --- Step 2: Open simulated caller endpoint ---
	// The caller sends packets TO callerLegPort (karl's listening socket for the caller leg)
	callerAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	callerConn, err := net.ListenUDP("udp", callerAddr)
	if err != nil {
		t.Fatalf("Caller listen failed: %v", err)
	}
	defer callerConn.Close()
	callerLocalPort := callerConn.LocalAddr().(*net.UDPAddr).Port
	t.Logf("Simulated caller endpoint on port %d", callerLocalPort)

	// --- Step 3: Open simulated callee endpoint ---
	calleeAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	calleeConn, err := net.ListenUDP("udp", calleeAddr)
	if err != nil {
		t.Fatalf("Callee listen failed: %v", err)
	}
	defer calleeConn.Close()
	calleeLocalPort := calleeConn.LocalAddr().(*net.UDPAddr).Port
	t.Logf("Simulated callee endpoint on port %d", calleeLocalPort)

	// --- Step 4: Answer with callee's SDP (pointing at callee's actual port) ---
	calleeSDP := fmt.Sprintf(
		"v=0\r\no=callee 1 1 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio %d RTP/AVP 0\r\na=rtpmap:0 PCMU/8000\r\na=sendrecv\r\n",
		calleeLocalPort,
	)

	answerResp, err := answerHandler.Handle(&ng.NGRequest{
		Command: "answer",
		CallID:  "test-media-flow",
		FromTag: "caller-tag",
		ToTag:   "callee-tag",
		SDP:     calleeSDP,
	})
	if err != nil {
		t.Fatalf("Answer failed: %v", err)
	}
	if answerResp.Result != ng.ResultOK {
		t.Fatalf("Answer not OK: %s", answerResp.ErrorReason)
	}
	if len(answerResp.Streams) == 0 {
		t.Fatal("No streams in answer response")
	}

	calleeLegPort := answerResp.Streams[0].LocalPort
	t.Logf("Answer allocated callee-leg local port: %d", calleeLegPort)

	// Now we need to fix the caller leg's remote address.
	// The offer SDP had port 0 (we used 0 because caller didn't know its port yet).
	// In real rtpengine, the caller's IP:port comes from the SDP c= and m= lines.
	// We need to update the caller leg with the actual caller address.
	session := registry.GetSessionByTags("test-media-flow", "caller-tag", "callee-tag")
	if session == nil {
		t.Fatal("Session not found after answer")
	}

	session.RLock()
	cl := session.CallerLeg
	session.RUnlock()
	if cl != nil {
		cl.SetRemoteAddr(net.ParseIP("127.0.0.1"), callerLocalPort)
	}

	// Verify relay was started
	session.RLock()
	hasRelay := session.Relay != nil
	session.RUnlock()
	if !hasRelay {
		t.Fatal("MediaRelay not started after answer")
	}

	// Give the relay goroutines a moment to start
	time.Sleep(50 * time.Millisecond)

	// --- Step 5: Send RTP from caller → karl → callee ---
	callerPayload := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	callerPkt := buildRTPPacket(100, 1600, 0xDEADBEEF, callerPayload)

	karlCallerAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", callerLegPort))
	_, err = callerConn.WriteToUDP(callerPkt, karlCallerAddr)
	if err != nil {
		t.Fatalf("Caller send failed: %v", err)
	}

	// Read on callee side — should receive the forwarded packet
	calleeConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1500)
	n, _, readErr := calleeConn.ReadFromUDP(buf)
	if readErr != nil {
		t.Fatalf("Callee did not receive forwarded packet: %v", readErr)
	}
	if n != len(callerPkt) {
		t.Errorf("Callee received %d bytes, expected %d", n, len(callerPkt))
	}
	// Verify RTP payload is intact
	receivedPayload := buf[12:n]
	for i, b := range callerPayload {
		if receivedPayload[i] != b {
			t.Errorf("Payload mismatch at byte %d: got %02x, want %02x", i, receivedPayload[i], b)
		}
	}
	t.Log("Caller → Karl → Callee: packet forwarded correctly")

	// --- Step 6: Send RTP from callee → karl → caller ---
	calleePayload := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	calleePkt := buildRTPPacket(200, 3200, 0xCAFEBABE, calleePayload)

	karlCalleeAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", calleeLegPort))
	_, err = calleeConn.WriteToUDP(calleePkt, karlCalleeAddr)
	if err != nil {
		t.Fatalf("Callee send failed: %v", err)
	}

	// Read on caller side
	callerConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, readErr = callerConn.ReadFromUDP(buf)
	if readErr != nil {
		t.Fatalf("Caller did not receive forwarded packet: %v", readErr)
	}
	if n != len(calleePkt) {
		t.Errorf("Caller received %d bytes, expected %d", n, len(calleePkt))
	}
	receivedPayload = buf[12:n]
	for i, b := range calleePayload {
		if receivedPayload[i] != b {
			t.Errorf("Payload mismatch at byte %d: got %02x, want %02x", i, receivedPayload[i], b)
		}
	}
	t.Log("Callee → Karl → Caller: packet forwarded correctly")

	// --- Step 7: Verify stats ---
	session.RLock()
	callerStats := session.CallerLeg.LoadStats()
	calleeStats := session.CalleeLeg.LoadStats()
	session.RUnlock()
	callerRecv := callerStats.PacketsRecv
	calleeRecv := calleeStats.PacketsRecv

	if callerRecv == 0 {
		t.Error("CallerLeg.PacketsRecv should be > 0")
	}
	if calleeRecv == 0 {
		t.Error("CalleeLeg.PacketsRecv should be > 0")
	}
	t.Logf("Stats: caller recv=%d, callee recv=%d", callerRecv, calleeRecv)

	// --- Step 8: Delete session ---
	deleteHandler := NewDeleteHandler(registry)
	delResp, err := deleteHandler.Handle(&ng.NGRequest{
		Command: "delete",
		CallID:  "test-media-flow",
	})
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if delResp.Result != ng.ResultOK {
		t.Fatalf("Delete not OK: %s", delResp.ErrorReason)
	}

	// Verify session is gone
	sessions := registry.GetSessionByCallID("test-media-flow")
	if len(sessions) != 0 {
		t.Error("Session should be deleted")
	}
	t.Log("Session deleted successfully")
}

// TestMediaRelayPortLatching verifies that symmetric/port-latching works.
func TestMediaRelayPortLatching(t *testing.T) {
	registry := createTestRegistry()
	config := createTestConfig()
	config.Integration.MediaIP = "127.0.0.1"
	config.Integration.PublicIP = ""
	config.Sessions = &internal.SessionConfig{MinPort: 41000, MaxPort: 42000}

	offerHandler := NewOfferHandler(registry, config)
	answerHandler := NewAnswerHandler(registry, config)

	// Offer with symmetric flag
	offerResp, err := offerHandler.Handle(&ng.NGRequest{
		Command: "offer",
		CallID:  "test-latching",
		FromTag: "from-latch",
		SDP:     "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 0 RTP/AVP 0\r\na=rtpmap:0 PCMU/8000\r\na=sendrecv\r\n",
		Flags:   []string{"symmetric"},
	})
	if err != nil || offerResp.Result != ng.ResultOK {
		t.Fatalf("Offer failed: %v / %s", err, offerResp.ErrorReason)
	}
	callerLegPort := offerResp.Streams[0].LocalPort

	// Open a callee endpoint
	calleeConn, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	defer calleeConn.Close()
	calleePort := calleeConn.LocalAddr().(*net.UDPAddr).Port

	// Answer
	answerResp, err := answerHandler.Handle(&ng.NGRequest{
		Command: "answer",
		CallID:  "test-latching",
		FromTag: "from-latch",
		ToTag:   "to-latch",
		SDP:     fmt.Sprintf("v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio %d RTP/AVP 0\r\na=rtpmap:0 PCMU/8000\r\na=sendrecv\r\n", calleePort),
		Flags:   []string{"symmetric"},
	})
	if err != nil || answerResp.Result != ng.ResultOK {
		t.Fatalf("Answer failed: %v / %s", err, answerResp.ErrorReason)
	}

	time.Sleep(50 * time.Millisecond)

	// Open a caller endpoint on an ephemeral port (simulating NAT)
	callerConn, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	defer callerConn.Close()

	// Send a packet from caller → karl (this should latch the caller's real address)
	pkt := buildRTPPacket(1, 160, 0x11111111, []byte{0xFF})
	karlAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", callerLegPort))
	callerConn.WriteToUDP(pkt, karlAddr)

	// The callee should receive it
	calleeConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1500)
	n, _, readErr := calleeConn.ReadFromUDP(buf)
	if readErr != nil {
		t.Fatalf("Callee didn't receive latched packet: %v", readErr)
	}
	if n != len(pkt) {
		t.Errorf("Got %d bytes, want %d", n, len(pkt))
	}

	// Now send from callee → karl → should reach caller via latched address
	calleeLegPort := answerResp.Streams[0].LocalPort
	karlCalleeAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", calleeLegPort))
	replyPkt := buildRTPPacket(1, 160, 0x22222222, []byte{0xAA})
	calleeConn.WriteToUDP(replyPkt, karlCalleeAddr)

	callerConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, readErr = callerConn.ReadFromUDP(buf)
	if readErr != nil {
		t.Fatalf("Caller didn't receive reply via latched address: %v", readErr)
	}
	t.Logf("Port latching works: caller received %d bytes on ephemeral port", n)

	// Cleanup
	registry.DeleteSession(registry.GetSessionByTags("test-latching", "from-latch", "to-latch").ID)
}

// TestMediaRelayBlocking verifies that media blocking drops packets.
func TestMediaRelayBlocking(t *testing.T) {
	registry := createTestRegistry()
	config := createTestConfig()
	config.Integration.MediaIP = "127.0.0.1"
	config.Integration.PublicIP = ""
	config.Sessions = &internal.SessionConfig{MinPort: 42000, MaxPort: 43000}

	offerHandler := NewOfferHandler(registry, config)
	answerHandler := NewAnswerHandler(registry, config)

	offerResp, err := offerHandler.Handle(&ng.NGRequest{
		Command: "offer",
		CallID:  "test-block",
		FromTag: "from-block",
		SDP:     "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 0 RTP/AVP 0\r\na=rtpmap:0 PCMU/8000\r\na=sendrecv\r\n",
		Flags:   []string{"symmetric"},
	})
	if err != nil || offerResp.Result != ng.ResultOK {
		t.Fatalf("Offer failed: %v / %s", err, offerResp.ErrorReason)
	}
	callerLegPort := offerResp.Streams[0].LocalPort

	calleeConn, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	defer calleeConn.Close()
	calleePort := calleeConn.LocalAddr().(*net.UDPAddr).Port

	answerResp, err := answerHandler.Handle(&ng.NGRequest{
		Command: "answer",
		CallID:  "test-block",
		FromTag: "from-block",
		ToTag:   "to-block",
		SDP:     fmt.Sprintf("v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio %d RTP/AVP 0\r\na=rtpmap:0 PCMU/8000\r\na=sendrecv\r\n", calleePort),
	})
	if err != nil || answerResp.Result != ng.ResultOK {
		t.Fatalf("Answer failed: %v / %s", err, answerResp.ErrorReason)
	}

	time.Sleep(50 * time.Millisecond)

	// Block media on the caller leg
	session := registry.GetSessionByTags("test-block", "from-block", "to-block")
	session.CallerLeg.SetMediaBlocked(true)

	// Send from caller → karl
	callerConn, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	defer callerConn.Close()
	pkt := buildRTPPacket(1, 160, 0x33333333, []byte{0xDD})
	karlAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", callerLegPort))
	callerConn.WriteToUDP(pkt, karlAddr)

	// Callee should NOT receive anything (blocked)
	calleeConn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	buf := make([]byte, 1500)
	_, _, readErr := calleeConn.ReadFromUDP(buf)
	if readErr == nil {
		t.Error("Callee should NOT have received a packet (media is blocked)")
	} else {
		t.Log("Media blocking works: callee did not receive blocked packet")
	}

	registry.DeleteSession(session.ID)
}

// TestMediaRelayOnPacketHook verifies the recording hook fires.
func TestMediaRelayOnPacketHook(t *testing.T) {
	registry := createTestRegistry()
	config := createTestConfig()
	config.Integration.MediaIP = "127.0.0.1"
	config.Integration.PublicIP = ""
	config.Sessions = &internal.SessionConfig{MinPort: 43000, MaxPort: 44000}

	offerHandler := NewOfferHandler(registry, config)
	answerHandler := NewAnswerHandler(registry, config)

	// Use a mock recording writer to verify the hook
	var hookCalled atomic.Int32
	mockRecorder := &mockRecordingWriter{onWrite: func() { hookCalled.Add(1) }}
	answerHandler.SetRecordingManager(mockRecorder)

	offerResp, _ := offerHandler.Handle(&ng.NGRequest{
		Command: "offer",
		CallID:  "test-hook",
		FromTag: "from-hook",
		SDP:     "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 0 RTP/AVP 0\r\na=rtpmap:0 PCMU/8000\r\na=sendrecv\r\n",
		Flags:   []string{"record-call", "symmetric"},
	})
	if offerResp.Result != ng.ResultOK {
		t.Fatalf("Offer failed: %s", offerResp.ErrorReason)
	}
	callerLegPort := offerResp.Streams[0].LocalPort

	calleeConn, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	defer calleeConn.Close()
	calleePort := calleeConn.LocalAddr().(*net.UDPAddr).Port

	answerResp, _ := answerHandler.Handle(&ng.NGRequest{
		Command: "answer",
		CallID:  "test-hook",
		FromTag: "from-hook",
		ToTag:   "to-hook",
		SDP:     fmt.Sprintf("v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio %d RTP/AVP 0\r\na=rtpmap:0 PCMU/8000\r\na=sendrecv\r\n", calleePort),
		Flags:   []string{"record-call"},
	})
	if answerResp.Result != ng.ResultOK {
		t.Fatalf("Answer failed: %s", answerResp.ErrorReason)
	}

	time.Sleep(50 * time.Millisecond)

	// Send a packet from caller
	callerConn, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	defer callerConn.Close()

	session := registry.GetSessionByTags("test-hook", "from-hook", "to-hook")
	session.CallerLeg.SetRemoteAddr(net.ParseIP("127.0.0.1"), callerConn.LocalAddr().(*net.UDPAddr).Port)

	pkt := buildRTPPacket(1, 160, 0x44444444, []byte{0xEE, 0xFF})
	karlAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", callerLegPort))
	callerConn.WriteToUDP(pkt, karlAddr)

	// Wait for packet to be forwarded and hook to fire
	calleeConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1500)
	calleeConn.ReadFromUDP(buf)

	time.Sleep(50 * time.Millisecond)

	if hookCalled.Load() == 0 {
		t.Error("Recording hook (OnPacket) was not called")
	} else {
		t.Logf("Recording hook fired %d time(s)", hookCalled.Load())
	}

	registry.DeleteSession(session.ID)
}

// mockRecordingWriter is a test double for RecordingWriter.
type mockRecordingWriter struct {
	onWrite func()
}

func (m *mockRecordingWriter) WriteRTPPacket(sessionID string, payload []byte, payloadType uint8, isCaller bool) error {
	if m.onWrite != nil {
		m.onWrite()
	}
	return nil
}
