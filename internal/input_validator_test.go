package internal

import (
	"strings"
	"testing"
)

func TestDefaultInputValidatorConfig(t *testing.T) {
	config := DefaultInputValidatorConfig()

	if config.MaxCallIDLength != 256 {
		t.Errorf("Expected MaxCallIDLength 256, got %d", config.MaxCallIDLength)
	}
	if config.MaxSDPLength != 65536 {
		t.Errorf("Expected MaxSDPLength 65536, got %d", config.MaxSDPLength)
	}
}

func TestNewInputValidator(t *testing.T) {
	v := NewInputValidator(nil)
	if v == nil {
		t.Fatal("NewInputValidator returned nil")
	}
}

func TestInputValidator_ValidateCallID(t *testing.T) {
	v := NewInputValidator(nil)

	tests := []struct {
		name    string
		callID  string
		wantErr bool
	}{
		{"valid simple", "abc123", false},
		{"valid with chars", "call-id_123@host.com", false},
		{"valid uuid", "550e8400-e29b-41d4-a716-446655440000", false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", 300), true},
		{"null bytes", "abc\x00def", true},
		{"control chars", "abc\x07def", true},
		{"spaces", "abc def", true},
		{"unicode", "call-\u0000-id", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateCallID(tt.callID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCallID(%q) error = %v, wantErr %v", tt.callID, err, tt.wantErr)
			}
		})
	}
}

func TestInputValidator_ValidateTag(t *testing.T) {
	v := NewInputValidator(nil)

	tests := []struct {
		name    string
		tag     string
		wantErr bool
	}{
		{"valid simple", "tag123", false},
		{"valid with dash", "tag-123", false},
		{"valid with underscore", "tag_123", false},
		{"empty", "", false}, // Empty allowed
		{"too long", strings.Repeat("a", 200), true},
		{"with space", "tag 123", true},
		{"null bytes", "tag\x00", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateTag(tt.tag)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTag(%q) error = %v, wantErr %v", tt.tag, err, tt.wantErr)
			}
		})
	}
}

func TestInputValidator_ValidateSDP(t *testing.T) {
	v := NewInputValidator(nil)

	validSDP := `v=0
o=- 123 456 IN IP4 192.168.1.1
s=Test Session
c=IN IP4 192.168.1.1
t=0 0
m=audio 49170 RTP/AVP 0
a=rtpmap:0 PCMU/8000
`

	tests := []struct {
		name    string
		sdp     string
		wantErr bool
	}{
		{"valid", validSDP, false},
		{"empty", "", false}, // Empty allowed
		{"too long", strings.Repeat("a", 100000), true},
		{"missing version", "o=- 123\ns=Test\n", true},
		{"missing origin", "v=0\ns=Test\n", true},
		{"missing session", "v=0\no=- 123\n", true},
		{"null bytes", "v=0\x00\no=- 123\ns=Test\n", true},
		{"binary data", "v=0\no=- 123\ns=Test\n\x80\x81\x82", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateSDP(tt.sdp)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSDP() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInputValidator_ValidateIPAddress(t *testing.T) {
	v := NewInputValidator(nil)

	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{"valid IPv4", "192.168.1.1", false},
		{"valid IPv6", "2001:db8::1", false},
		{"valid localhost", "127.0.0.1", false},
		{"valid any", "0.0.0.0", false},
		{"empty", "", true},
		{"invalid", "not-an-ip", true},
		{"invalid partial", "192.168.1", true},
		{"too many octets", "192.168.1.1.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateIPAddress(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIPAddress(%q) error = %v, wantErr %v", tt.addr, err, tt.wantErr)
			}
		})
	}
}

func TestInputValidator_ValidatePort(t *testing.T) {
	v := NewInputValidator(nil)

	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{"valid low", 1, false},
		{"valid mid", 8080, false},
		{"valid high", 65535, false},
		{"zero", 0, true},
		{"negative", -1, true},
		{"too high", 65536, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidatePort(tt.port)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePort(%d) error = %v, wantErr %v", tt.port, err, tt.wantErr)
			}
		})
	}
}

func TestInputValidator_ValidateHostPort(t *testing.T) {
	v := NewInputValidator(nil)

	tests := []struct {
		name     string
		hostPort string
		wantErr  bool
	}{
		{"valid IP:port", "192.168.1.1:8080", false},
		{"valid hostname:port", "localhost:8080", false},
		{"valid IPv6", "[::1]:8080", false},
		{"empty", "", true},
		{"no port", "192.168.1.1", true},
		{"invalid port", "192.168.1.1:abc", true},
		{"port too high", "192.168.1.1:70000", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateHostPort(tt.hostPort)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHostPort(%q) error = %v, wantErr %v", tt.hostPort, err, tt.wantErr)
			}
		})
	}
}

func TestInputValidator_ValidateCodec(t *testing.T) {
	v := NewInputValidator(nil)

	tests := []struct {
		name    string
		codec   string
		wantErr bool
	}{
		{"valid opus", "opus", false},
		{"valid pcmu", "PCMU", false},
		{"valid with slash", "opus/48000", false},
		{"valid with dash", "G.711-ulaw", false},
		{"empty", "", false}, // Empty allowed
		{"too long", strings.Repeat("a", 100), true},
		{"starts with number", "8bit", true},
		{"special chars", "codec@!", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateCodec(tt.codec)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCodec(%q) error = %v, wantErr %v", tt.codec, err, tt.wantErr)
			}
		})
	}
}

func TestInputValidator_SanitizeString(t *testing.T) {
	v := NewInputValidator(nil)

	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"normal", "hello world", 100, "hello world"},
		{"truncate", "hello world", 5, "hello"},
		{"null bytes", "hel\x00lo", 100, "hello"},
		{"control chars", "hel\x07lo", 100, "hello"},
		{"preserve newline", "hello\nworld", 100, "hello\nworld"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.SanitizeString(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("SanitizeString(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestInputValidator_SanitizeCallID(t *testing.T) {
	v := NewInputValidator(nil)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"normal", "call-123", "call-123"},
		{"with at", "call@host", "call@host"},
		{"special chars removed", "call<>123", "call123"},
		{"spaces removed", "call 123", "call123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.SanitizeCallID(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeCallID(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRequestValidator_ValidateNGRequest(t *testing.T) {
	v := NewRequestValidator(1024*1024, 65536)

	validSDP := "v=0\no=- 123 456 IN IP4 192.168.1.1\ns=Test\n"

	tests := []struct {
		name      string
		req       *NGRequestValidation
		wantErrs  int
	}{
		{
			name: "valid request",
			req: &NGRequestValidation{
				CallID:  "call-123",
				FromTag: "from-tag",
				ToTag:   "to-tag",
				SDP:     validSDP,
				Address: "192.168.1.1",
			},
			wantErrs: 0,
		},
		{
			name: "invalid call-id",
			req: &NGRequestValidation{
				CallID: strings.Repeat("a", 300),
			},
			wantErrs: 1,
		},
		{
			name: "multiple errors",
			req: &NGRequestValidation{
				CallID:  "call\x00id",
				FromTag: "tag with spaces",
				Address: "not-an-ip",
			},
			wantErrs: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := v.ValidateNGRequest(tt.req)
			if len(errs) != tt.wantErrs {
				t.Errorf("ValidateNGRequest() got %d errors, want %d: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}

func TestSDPValidator_ValidateSDP(t *testing.T) {
	v := NewSDPValidator()

	tests := []struct {
		name    string
		sdp     string
		wantErr bool
	}{
		{"empty", "", false},
		{"normal", "v=0\no=- 123\ns=Test\nm=audio 49170 RTP/AVP 0\n", false},
		{"long line", "a=" + strings.Repeat("x", 5000), true},
		{"too many media", func() string {
			var sb strings.Builder
			for i := 0; i < 60; i++ {
				sb.WriteString("m=audio 49170 RTP/AVP 0\n")
			}
			return sb.String()
		}(), true},
		{"too many candidates", func() string {
			var sb strings.Builder
			for i := 0; i < 150; i++ {
				sb.WriteString("a=candidate:1 1 UDP 123 192.168.1.1 12345 typ host\n")
			}
			return sb.String()
		}(), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateSDP(tt.sdp)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSDP() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBencodeValidator_ValidateBencodeDepth(t *testing.T) {
	v := NewBencodeValidator()

	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{"simple string", "5:hello", false},
		{"simple list", "l5:helloe", false},
		{"nested list", "ll5:helloee", false},
		{"nested dict", "d3:keyd3:subd4:deep5:valueeee", false},
		{"deeply nested", strings.Repeat("l", 25) + strings.Repeat("e", 25), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateBencodeDepth([]byte(tt.data))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBencodeDepth() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGlobalValidators(t *testing.T) {
	// Test global convenience functions
	if err := ValidateCallID("valid-call-id"); err != nil {
		t.Errorf("ValidateCallID failed: %v", err)
	}

	if err := ValidateTag("valid-tag"); err != nil {
		t.Errorf("ValidateTag failed: %v", err)
	}

	validSDP := "v=0\no=- 123 456 IN IP4 192.168.1.1\ns=Test\n"
	if err := ValidateSDP(validSDP); err != nil {
		t.Errorf("ValidateSDP failed: %v", err)
	}

	if err := ValidateIPAddress("192.168.1.1"); err != nil {
		t.Errorf("ValidateIPAddress failed: %v", err)
	}
}

func TestInputValidator_checkBasicSafety(t *testing.T) {
	v := NewInputValidator(nil)

	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"normal", "hello world", nil},
		{"invalid utf8", string([]byte{0xff, 0xfe}), ErrInvalidUTF8},
		{"null byte", "hello\x00world", ErrNullBytes},
		{"control char", "hello\x07world", ErrControlCharacters},
		{"newline ok", "hello\nworld", nil},
		{"tab ok", "hello\tworld", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.checkBasicSafety(tt.input)
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("checkBasicSafety(%q) = nil, want %v", tt.input, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("checkBasicSafety(%q) = %v, want nil", tt.input, err)
			}
		})
	}
}
