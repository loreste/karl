package ng_protocol

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// These tests run against a mock server and don't require a real Karl instance

var (
	testServer     *MockNGServer
	testServerOnce sync.Once
	testServerAddr string
	testServerErr  error
)

// setupTestServer sets up a mock server for testing
func setupTestServer(t *testing.T) string {
	testServerOnce.Do(func() {
		testServer, testServerErr = NewMockNGServer(0) // Use any available port
		if testServerErr != nil {
			return
		}
		testServer.Start()
		testServerAddr = testServer.Addr()
	})
	if testServerErr != nil {
		t.Fatalf("Failed to create mock server: %v", testServerErr)
	}
	return testServerAddr
}

// getTestServerAddr returns the test server address
// Uses real server if KARL_TEST_SERVER env var is set
func getTestServerAddr(t *testing.T) string {
	if addr := os.Getenv("KARL_TEST_SERVER"); addr != "" {
		return addr
	}
	return setupTestServer(t)
}

// =============================================================================
// Standalone Integration Tests (run with mock server)
// =============================================================================

func TestStandalone_PingPong(t *testing.T) {
	client := NewTestClient(t, getTestServerAddr(t))
	defer client.Close()

	result, err := client.Ping()
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	if result != "pong" {
		t.Errorf("Expected 'pong', got '%s'", result)
	}
}

func TestStandalone_BasicCallFlow(t *testing.T) {
	client := NewTestClient(t, getTestServerAddr(t))
	defer client.Close()

	callID := fmt.Sprintf("standalone-call-%d", time.Now().UnixNano())
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

func TestStandalone_OfferWithFlags(t *testing.T) {
	client := NewTestClient(t, getTestServerAddr(t))
	defer client.Close()

	tests := []struct {
		name  string
		flags []string
	}{
		{"symmetric", []string{"symmetric"}},
		{"ICE-remove", []string{"ICE=remove"}},
		{"trust-address", []string{"trust-address"}},
		{"record-call", []string{"record-call"}},
		{"multiple-flags", []string{"symmetric", "trust-address", "replace-origin"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callID := fmt.Sprintf("flags-%s-%d", tt.name, time.Now().UnixNano())
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

func TestStandalone_DirectionFlags(t *testing.T) {
	client := NewTestClient(t, getTestServerAddr(t))
	defer client.Close()

	tests := []struct {
		name       string
		directions []string
	}{
		{"internal-external", []string{"internal", "external"}},
		{"external-internal", []string{"external", "internal"}},
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
				t.Errorf("Offer with direction returned %s", result)
			}

			// Cleanup
			client.Delete(callID, fromTag, "", nil)
		})
	}
}

func TestStandalone_ReINVITE(t *testing.T) {
	client := NewTestClient(t, getTestServerAddr(t))
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

	// Re-INVITE offer
	reInviteSDP := strings.Replace(testSDPOffer, "987654321", "987654322", 1)
	resp, err := client.Offer(callID, fromTag, reInviteSDP, nil, nil)
	if err != nil {
		t.Fatalf("Re-INVITE offer failed: %v", err)
	}

	result, _ := resp["result"].(string)
	if result != "ok" {
		t.Errorf("Re-INVITE returned %s", result)
	}

	// Cleanup
	client.Delete(callID, fromTag, toTag, nil)
}

func TestStandalone_Recording(t *testing.T) {
	client := NewTestClient(t, getTestServerAddr(t))
	defer client.Close()

	callID := fmt.Sprintf("record-%d", time.Now().UnixNano())
	fromTag := "from-record"
	toTag := "to-record"

	// Setup call
	_, err := client.Offer(callID, fromTag, testSDPOffer, []string{"record-call"}, nil)
	if err != nil {
		t.Fatalf("Offer with recording failed: %v", err)
	}

	_, err = client.Answer(callID, fromTag, toTag, testSDPAnswer, nil, nil)
	if err != nil {
		t.Fatalf("Answer failed: %v", err)
	}

	// Start recording
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

func TestStandalone_MediaControl(t *testing.T) {
	client := NewTestClient(t, getTestServerAddr(t))
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

func TestStandalone_DTMF(t *testing.T) {
	client := NewTestClient(t, getTestServerAddr(t))
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
	for _, digit := range []string{"1", "2", "3", "*", "#"} {
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

func TestStandalone_ListCalls(t *testing.T) {
	client := NewTestClient(t, getTestServerAddr(t))
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

func TestStandalone_ErrorHandling(t *testing.T) {
	client := NewTestClient(t, getTestServerAddr(t))
	defer client.Close()

	tests := []struct {
		name         string
		params       map[string]interface{}
		expectResult string
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

func TestStandalone_ConcurrentCalls(t *testing.T) {
	serverAddr := getTestServerAddr(t)

	numCalls := 10
	var wg sync.WaitGroup
	errors := make(chan error, numCalls)

	for i := 0; i < numCalls; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Each goroutine gets its own client to avoid response mixing
			client := NewTestClient(t, serverAddr)
			defer client.Close()

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

func TestStandalone_LabelSupport(t *testing.T) {
	client := NewTestClient(t, getTestServerAddr(t))
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

func TestStandalone_ResponseFormat(t *testing.T) {
	client := NewTestClient(t, getTestServerAddr(t))
	defer client.Close()

	callID := fmt.Sprintf("format-%d", time.Now().UnixNano())
	fromTag := "from-format"
	toTag := "to-format"

	// Offer
	offerResp, err := client.Offer(callID, fromTag, testSDPOffer, nil, nil)
	if err != nil {
		t.Fatalf("Offer failed: %v", err)
	}

	// Verify required fields
	if _, ok := offerResp["result"]; !ok {
		t.Error("Offer response missing result field")
	}
	if _, ok := offerResp["sdp"]; !ok {
		t.Error("Offer response missing sdp field")
	}

	// Answer
	answerResp, err := client.Answer(callID, fromTag, toTag, testSDPAnswer, nil, nil)
	if err != nil {
		t.Fatalf("Answer failed: %v", err)
	}

	if _, ok := answerResp["result"]; !ok {
		t.Error("Answer response missing result field")
	}
	if _, ok := answerResp["sdp"]; !ok {
		t.Error("Answer response missing sdp field")
	}

	// Cleanup
	client.Delete(callID, fromTag, toTag, nil)
}

func TestStandalone_BencodeRoundtrip(t *testing.T) {
	// Test that our bencode encoding/decoding is compatible
	testCases := []map[string]interface{}{
		{
			"command": "offer",
			"call-id": "test-123",
		},
		{
			"command":  "offer",
			"call-id":  "test-456",
			"from-tag": "tag-abc",
			"flags":    []interface{}{"symmetric", "record-call"},
		},
		{
			"command":   "answer",
			"call-id":   "test-789",
			"from-tag":  "tag-def",
			"to-tag":    "tag-ghi",
			"direction": []interface{}{"internal", "external"},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			// Encode
			encoder := NewEncoder()
			encoded, err := encoder.Encode(tc)
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}

			// Decode
			decoder := NewDecoder(encoded)
			decoded, err := decoder.Decode()
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}

			dict, ok := decoded.(BencodeDict)
			if !ok {
				t.Fatal("Decoded value is not a dictionary")
			}

			// Verify key fields
			for key, expected := range tc {
				actual := dict[key]
				switch v := expected.(type) {
				case string:
					if actual != v {
						t.Errorf("Key %s: expected %v, got %v", key, expected, actual)
					}
				case []interface{}:
					actualList, ok := actual.(BencodeList)
					if !ok {
						t.Errorf("Key %s: expected list, got %T", key, actual)
						continue
					}
					if len(actualList) != len(v) {
						t.Errorf("Key %s: list length mismatch: expected %d, got %d", key, len(v), len(actualList))
					}
				}
			}
		})
	}
}

// Benchmark tests
func BenchmarkBencodeEncode(b *testing.B) {
	params := map[string]interface{}{
		"command":  "offer",
		"call-id":  "benchmark-call-12345678901234567890",
		"from-tag": "from-tag-abcdefghij",
		"sdp":      testSDPOffer,
		"flags":    []interface{}{"symmetric", "trust-address", "record-call"},
	}

	encoder := NewEncoder()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoder.Encode(params)
	}
}

func BenchmarkBencodeDecode(b *testing.B) {
	params := map[string]interface{}{
		"command":  "offer",
		"call-id":  "benchmark-call-12345678901234567890",
		"from-tag": "from-tag-abcdefghij",
		"sdp":      testSDPOffer,
		"flags":    []interface{}{"symmetric", "trust-address", "record-call"},
	}

	encoder := NewEncoder()
	encoded, _ := encoder.Encode(params)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decoder := NewDecoder(encoded)
		decoder.Decode()
	}
}

func BenchmarkParseMessage(b *testing.B) {
	params := map[string]interface{}{
		"command":  "offer",
		"call-id":  "benchmark-call-12345678901234567890",
		"from-tag": "from-tag-abcdefghij",
		"sdp":      testSDPOffer,
		"flags":    []interface{}{"symmetric", "trust-address", "record-call"},
	}

	encoder := NewEncoder()
	encoded, _ := encoder.Encode(params)
	message := append([]byte("cookie123 "), encoded...)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseMessage(message, nil)
	}
}
