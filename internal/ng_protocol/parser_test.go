package ng_protocol

import (
	"errors"
	"fmt"
	"testing"
)

var (
	errInvalidBencode = errors.New("invalid bencode")
	errInvalidNGMsg   = errors.New("invalid NG message format")
)

func TestParseBencodeDict(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		validate func(t *testing.T, result map[string]interface{})
	}{
		{
			name:    "empty dict",
			input:   "de",
			wantErr: false,
			validate: func(t *testing.T, result map[string]interface{}) {
				if len(result) != 0 {
					t.Errorf("expected empty dict, got %d items", len(result))
				}
			},
		},
		{
			name:    "simple string value",
			input:   "d7:call-id4:teste",
			wantErr: false,
			validate: func(t *testing.T, result map[string]interface{}) {
				if v, ok := result["call-id"]; !ok || v != "test" {
					t.Errorf("expected call-id=test, got %v", result["call-id"])
				}
			},
		},
		{
			name:    "integer value",
			input:   "d4:porti8080ee",
			wantErr: false,
			validate: func(t *testing.T, result map[string]interface{}) {
				if v, ok := result["port"]; !ok || v != int64(8080) {
					t.Errorf("expected port=8080, got %v", result["port"])
				}
			},
		},
		{
			name:    "list value",
			input:   "d5:flagsl3:ICE4:DTLSee",
			wantErr: false,
			validate: func(t *testing.T, result map[string]interface{}) {
				flags, ok := result["flags"].([]interface{})
				if !ok {
					t.Fatalf("expected flags to be a list")
				}
				if len(flags) != 2 {
					t.Errorf("expected 2 flags, got %d", len(flags))
				}
			},
		},
		{
			name:    "nested dict",
			input:   "d3:sdpd4:typei1eee",
			wantErr: false,
			validate: func(t *testing.T, result map[string]interface{}) {
				sdp, ok := result["sdp"].(map[string]interface{})
				if !ok {
					t.Fatalf("expected sdp to be a dict")
				}
				if v, ok := sdp["type"]; !ok || v != int64(1) {
					t.Errorf("expected sdp.type=1, got %v", v)
				}
			},
		},
		{
			name:    "invalid bencode",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "unterminated dict",
			input:   "d7:call-id4:test",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTestBencode([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTestBencode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.validate != nil {
				dict, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected dict result")
				}
				tt.validate(t, dict)
			}
		})
	}
}

func TestEncodeBencodeDict(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]interface{}
		expect string
	}{
		{
			name:   "empty dict",
			input:  map[string]interface{}{},
			expect: "de",
		},
		{
			name:   "string value",
			input:  map[string]interface{}{"result": "ok"},
			expect: "d6:result2:oke",
		},
		{
			name:   "integer value",
			input:  map[string]interface{}{"code": int64(200)},
			expect: "d4:codei200ee",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodeTestBencode(tt.input)
			if string(result) != tt.expect {
				t.Errorf("encodeTestBencode() = %s, want %s", result, tt.expect)
			}
		})
	}
}

func TestParseNGCommand(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantCmd    string
		wantCallID string
		wantErr    bool
	}{
		{
			name:       "ping command",
			input:      "abcd1234 d7:command4:pinge",
			wantCmd:    "ping",
			wantCallID: "",
			wantErr:    false,
		},
		{
			name:       "offer command",
			input:      "cookie123 d7:command5:offer7:call-id8:test-123e",
			wantCmd:    "offer",
			wantCallID: "test-123",
			wantErr:    false,
		},
		{
			name:       "invalid format",
			input:      "nocookie",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cookie, params, err := parseTestNGMessage([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTestNGMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if cookie == "" {
				t.Error("expected non-empty cookie")
			}
			if cmd, ok := params["command"].(string); !ok || cmd != tt.wantCmd {
				t.Errorf("expected command=%s, got %v", tt.wantCmd, params["command"])
			}
			if tt.wantCallID != "" {
				if callID, ok := params["call-id"].(string); !ok || callID != tt.wantCallID {
					t.Errorf("expected call-id=%s, got %v", tt.wantCallID, params["call-id"])
				}
			}
		})
	}
}

func TestParseFlagsArray(t *testing.T) {
	tests := []struct {
		name   string
		flags  []string
		expect map[string]bool
	}{
		{
			name:  "ICE flags",
			flags: []string{"ICE=force", "DTLS=passive"},
			expect: map[string]bool{
				"ICE=force":    true,
				"DTLS=passive": true,
			},
		},
		{
			name:  "boolean flags",
			flags: []string{"trust-address", "SIP-source-address", "record-call"},
			expect: map[string]bool{
				"trust-address":      true,
				"SIP-source-address": true,
				"record-call":        true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTestFlags(tt.flags)
			for flag, expected := range tt.expect {
				if result[flag] != expected {
					t.Errorf("flag %s: expected %v, got %v", flag, expected, result[flag])
				}
			}
		})
	}
}

func TestResponseFormat(t *testing.T) {
	tests := []struct {
		name     string
		response testNGResponse
		validate func(t *testing.T, encoded []byte)
	}{
		{
			name: "success response",
			response: testNGResponse{
				Result: "ok",
			},
			validate: func(t *testing.T, encoded []byte) {
				parsed, err := parseTestBencode(encoded)
				if err != nil {
					t.Fatalf("failed to parse response: %v", err)
				}
				dict := parsed.(map[string]interface{})
				if dict["result"] != "ok" {
					t.Errorf("expected result=ok, got %v", dict["result"])
				}
			},
		},
		{
			name: "error response",
			response: testNGResponse{
				Result:    "error",
				ErrorCode: 400,
				Reason:    "Bad Request",
			},
			validate: func(t *testing.T, encoded []byte) {
				parsed, err := parseTestBencode(encoded)
				if err != nil {
					t.Fatalf("failed to parse response: %v", err)
				}
				dict := parsed.(map[string]interface{})
				if dict["result"] != "error" {
					t.Errorf("expected result=error, got %v", dict["result"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := tt.response.encode()
			if tt.validate != nil {
				tt.validate(t, encoded)
			}
		})
	}
}

// Test helper types and functions

type testNGResponse struct {
	Result    string
	ErrorCode int
	Reason    string
	SDP       string
}

func (r *testNGResponse) encode() []byte {
	result := make(map[string]interface{})
	result["result"] = r.Result
	if r.ErrorCode != 0 {
		result["error-code"] = int64(r.ErrorCode)
	}
	if r.Reason != "" {
		result["error-reason"] = r.Reason
	}
	if r.SDP != "" {
		result["sdp"] = r.SDP
	}
	return encodeTestBencode(result)
}

func parseTestBencode(data []byte) (interface{}, error) {
	result, _, err := parseTestBencodeValue(data, 0)
	return result, err
}

func parseTestBencodeValue(data []byte, pos int) (interface{}, int, error) {
	if pos >= len(data) {
		return nil, pos, errInvalidBencode
	}

	switch data[pos] {
	case 'd':
		return parseTestBencodeDict(data, pos)
	case 'l':
		return parseTestBencodeList(data, pos)
	case 'i':
		return parseTestBencodeInt(data, pos)
	default:
		if data[pos] >= '0' && data[pos] <= '9' {
			return parseTestBencodeString(data, pos)
		}
		return nil, pos, errInvalidBencode
	}
}

func parseTestBencodeDict(data []byte, pos int) (map[string]interface{}, int, error) {
	if data[pos] != 'd' {
		return nil, pos, errInvalidBencode
	}
	pos++

	result := make(map[string]interface{})
	for pos < len(data) && data[pos] != 'e' {
		key, newPos, err := parseTestBencodeString(data, pos)
		if err != nil {
			return nil, pos, err
		}
		pos = newPos

		value, newPos, err := parseTestBencodeValue(data, pos)
		if err != nil {
			return nil, pos, err
		}
		pos = newPos

		result[key] = value
	}

	if pos >= len(data) || data[pos] != 'e' {
		return nil, pos, errInvalidBencode
	}
	return result, pos + 1, nil
}

func parseTestBencodeList(data []byte, pos int) ([]interface{}, int, error) {
	if data[pos] != 'l' {
		return nil, pos, errInvalidBencode
	}
	pos++

	var result []interface{}
	for pos < len(data) && data[pos] != 'e' {
		value, newPos, err := parseTestBencodeValue(data, pos)
		if err != nil {
			return nil, pos, err
		}
		pos = newPos
		result = append(result, value)
	}

	if pos >= len(data) || data[pos] != 'e' {
		return nil, pos, errInvalidBencode
	}
	return result, pos + 1, nil
}

func parseTestBencodeInt(data []byte, pos int) (int64, int, error) {
	if data[pos] != 'i' {
		return 0, pos, errInvalidBencode
	}
	pos++

	end := pos
	for end < len(data) && data[end] != 'e' {
		end++
	}
	if end >= len(data) {
		return 0, pos, errInvalidBencode
	}

	numStr := string(data[pos:end])
	var num int64
	negative := false
	start := 0
	if len(numStr) > 0 && numStr[0] == '-' {
		negative = true
		start = 1
	}
	for _, c := range numStr[start:] {
		if c < '0' || c > '9' {
			return 0, pos, errInvalidBencode
		}
		num = num*10 + int64(c-'0')
	}
	if negative {
		num = -num
	}

	return num, end + 1, nil
}

func parseTestBencodeString(data []byte, pos int) (string, int, error) {
	colonPos := pos
	for colonPos < len(data) && data[colonPos] != ':' {
		colonPos++
	}
	if colonPos >= len(data) {
		return "", pos, errInvalidBencode
	}

	lenStr := string(data[pos:colonPos])
	var strLen int
	for _, c := range lenStr {
		if c < '0' || c > '9' {
			return "", pos, errInvalidBencode
		}
		strLen = strLen*10 + int(c-'0')
	}

	start := colonPos + 1
	end := start + strLen
	if end > len(data) {
		return "", pos, errInvalidBencode
	}

	return string(data[start:end]), end, nil
}

func encodeTestBencode(v interface{}) []byte {
	switch val := v.(type) {
	case map[string]interface{}:
		return encodeTestBencodeDict(val)
	case []interface{}:
		return encodeTestBencodeList(val)
	case string:
		return encodeTestBencodeString(val)
	case int64:
		return encodeTestBencodeInt(val)
	case int:
		return encodeTestBencodeInt(int64(val))
	default:
		return nil
	}
}

func encodeTestBencodeDict(m map[string]interface{}) []byte {
	result := []byte{'d'}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	for _, k := range keys {
		result = append(result, encodeTestBencodeString(k)...)
		result = append(result, encodeTestBencode(m[k])...)
	}
	result = append(result, 'e')
	return result
}

func encodeTestBencodeList(l []interface{}) []byte {
	result := []byte{'l'}
	for _, v := range l {
		result = append(result, encodeTestBencode(v)...)
	}
	result = append(result, 'e')
	return result
}

func encodeTestBencodeString(s string) []byte {
	return []byte(fmt.Sprintf("%d:%s", len(s), s))
}

func encodeTestBencodeInt(i int64) []byte {
	return []byte(fmt.Sprintf("i%de", i))
}

func parseTestNGMessage(data []byte) (string, map[string]interface{}, error) {
	spaceIdx := -1
	for i, b := range data {
		if b == ' ' {
			spaceIdx = i
			break
		}
	}
	if spaceIdx == -1 {
		return "", nil, errInvalidNGMsg
	}

	cookie := string(data[:spaceIdx])
	bencodeData := data[spaceIdx+1:]

	parsed, err := parseTestBencode(bencodeData)
	if err != nil {
		return "", nil, err
	}

	dict, ok := parsed.(map[string]interface{})
	if !ok {
		return "", nil, errInvalidNGMsg
	}

	return cookie, dict, nil
}

func parseTestFlags(flags []string) map[string]bool {
	result := make(map[string]bool)
	for _, flag := range flags {
		result[flag] = true
	}
	return result
}
