package internal

import (
	"errors"
	"strings"
	"testing"
)

var errInvalidTestSDP = errors.New("invalid SDP")

// Test types to avoid conflicts with main package types
type testSDPSession struct {
	Version       int
	Origin        testSDPOrigin
	SessionName   string
	Connection    testSDPConnection
	Bandwidth     map[string]int
	MediaSections []testSDPMedia
}

type testSDPOrigin struct {
	Username       string
	SessionID      int64
	SessionVersion int64
	NetType        string
	AddrType       string
	Address        string
}

type testSDPConnection struct {
	NetType  string
	AddrType string
	Address  string
}

type testSDPMedia struct {
	Type         string
	Port         int
	Protocol     string
	Formats      []string
	Direction    string
	Ptime        int
	MaxPtime     int
	RTCPMux      bool
	RTCPPort     int
	RTCPAddress  string
	ICEUfrag     string
	ICEPwd       string
	Candidates   []testICECandidate
	CryptoSuites []testCryptoSuite
	Codecs       []testCodec
	Connection   *testSDPConnection
	Bandwidth    map[string]int
}

type testICECandidate struct {
	Foundation string
	Component  int
	Protocol   string
	Priority   uint32
	Address    string
	Port       int
	Type       string
}

type testCryptoSuite struct {
	Tag           int
	CryptoSuite   string
	KeyParameters string
}

type testCodec struct {
	PayloadType int
	Name        string
	ClockRate   int
	FMTP        string
}

func TestSDPParsing(t *testing.T) {
	tests := []struct {
		name     string
		sdp      string
		validate func(t *testing.T, parsed *testSDPSession)
		wantErr  bool
	}{
		{
			name: "basic audio SDP",
			sdp: `v=0
o=- 123456 123456 IN IP4 192.168.1.1
s=-
c=IN IP4 192.168.1.1
t=0 0
m=audio 5000 RTP/AVP 0 8 101
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:101 telephone-event/8000
`,
			validate: func(t *testing.T, parsed *testSDPSession) {
				if parsed.Origin.Address != "192.168.1.1" {
					t.Errorf("expected origin address 192.168.1.1, got %s", parsed.Origin.Address)
				}
				if len(parsed.MediaSections) != 1 {
					t.Fatalf("expected 1 media section, got %d", len(parsed.MediaSections))
				}
				m := parsed.MediaSections[0]
				if m.Type != "audio" {
					t.Errorf("expected audio media type, got %s", m.Type)
				}
				if m.Port != 5000 {
					t.Errorf("expected port 5000, got %d", m.Port)
				}
			},
		},
		{
			name: "SDP with ICE candidates",
			sdp: `v=0
o=- 123456 123456 IN IP4 192.168.1.1
s=-
t=0 0
m=audio 5000 RTP/AVP 0
a=ice-ufrag:user1
a=ice-pwd:pass1234567890123456
a=candidate:1 1 UDP 2130706431 192.168.1.1 5000 typ host
`,
			validate: func(t *testing.T, parsed *testSDPSession) {
				m := parsed.MediaSections[0]
				if m.ICEUfrag != "user1" {
					t.Errorf("expected ice-ufrag user1, got %s", m.ICEUfrag)
				}
				if len(m.Candidates) != 1 {
					t.Errorf("expected 1 candidate, got %d", len(m.Candidates))
				}
			},
		},
		{
			name: "SDP with SRTP",
			sdp: `v=0
o=- 123456 123456 IN IP4 192.168.1.1
s=-
t=0 0
m=audio 5000 RTP/SAVP 0
a=crypto:1 AES_CM_128_HMAC_SHA1_80 inline:YUJDZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0
`,
			validate: func(t *testing.T, parsed *testSDPSession) {
				m := parsed.MediaSections[0]
				if m.Protocol != "RTP/SAVP" {
					t.Errorf("expected RTP/SAVP, got %s", m.Protocol)
				}
				if len(m.CryptoSuites) == 0 {
					t.Error("expected crypto suite")
				}
			},
		},
		{
			name: "SDP with hold (sendonly)",
			sdp: `v=0
o=- 123456 123456 IN IP4 192.168.1.1
s=-
t=0 0
m=audio 5000 RTP/AVP 0
a=sendonly
`,
			validate: func(t *testing.T, parsed *testSDPSession) {
				m := parsed.MediaSections[0]
				if m.Direction != "sendonly" {
					t.Errorf("expected sendonly, got %s", m.Direction)
				}
			},
		},
		{
			name: "SDP with RTCP-mux",
			sdp: `v=0
o=- 123456 123456 IN IP4 192.168.1.1
s=-
t=0 0
m=audio 5000 RTP/AVP 0
a=rtcp-mux
`,
			validate: func(t *testing.T, parsed *testSDPSession) {
				m := parsed.MediaSections[0]
				if !m.RTCPMux {
					t.Error("expected RTCP-mux to be enabled")
				}
			},
		},
		{
			name: "SDP with multiple media sections",
			sdp: `v=0
o=- 123456 123456 IN IP4 192.168.1.1
s=-
t=0 0
m=audio 5000 RTP/AVP 0
m=video 5002 RTP/AVP 96
`,
			validate: func(t *testing.T, parsed *testSDPSession) {
				if len(parsed.MediaSections) != 2 {
					t.Fatalf("expected 2 media sections, got %d", len(parsed.MediaSections))
				}
				if parsed.MediaSections[0].Type != "audio" {
					t.Error("expected first section to be audio")
				}
				if parsed.MediaSections[1].Type != "video" {
					t.Error("expected second section to be video")
				}
			},
		},
		{
			name:    "invalid SDP - no version",
			sdp:     "o=- 123456 123456 IN IP4 192.168.1.1\n",
			wantErr: true,
		},
		{
			name:    "invalid SDP - empty",
			sdp:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parseTestSDP(tt.sdp)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTestSDP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, parsed)
			}
		})
	}
}

func TestSDPGeneration(t *testing.T) {
	session := &testSDPSession{
		Version: 0,
		Origin: testSDPOrigin{
			Username:  "-",
			SessionID: 123456,
			Address:   "192.168.1.1",
		},
		SessionName: "-",
		Connection: testSDPConnection{
			Address: "192.168.1.1",
		},
		MediaSections: []testSDPMedia{
			{
				Type:     "audio",
				Port:     5000,
				Protocol: "RTP/AVP",
				Formats:  []string{"0", "8"},
			},
		},
	}

	sdp := generateTestSDP(session)

	expected := []string{
		"v=0",
		"m=audio 5000 RTP/AVP 0 8",
	}

	for _, exp := range expected {
		if !strings.Contains(sdp, exp) {
			t.Errorf("expected SDP to contain %q\nGot:\n%s", exp, sdp)
		}
	}
}

func TestSDPCodecParsing(t *testing.T) {
	sdp := `v=0
o=- 123456 123456 IN IP4 192.168.1.1
s=-
t=0 0
m=audio 5000 RTP/AVP 0 8 101
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:101 telephone-event/8000
a=fmtp:101 0-16
`
	parsed, err := parseTestSDP(sdp)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	m := parsed.MediaSections[0]

	expectedCodecs := map[int]string{
		0:   "PCMU",
		8:   "PCMA",
		101: "telephone-event",
	}

	for pt, name := range expectedCodecs {
		found := false
		for _, c := range m.Codecs {
			if c.PayloadType == pt {
				if c.Name != name {
					t.Errorf("PT %d: expected codec %s, got %s", pt, name, c.Name)
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("codec PT %d not found", pt)
		}
	}
}

func TestSDPDirection(t *testing.T) {
	directions := []string{"sendrecv", "sendonly", "recvonly", "inactive"}

	for _, dir := range directions {
		t.Run(dir, func(t *testing.T) {
			sdp := `v=0
o=- 123456 123456 IN IP4 192.168.1.1
s=-
t=0 0
m=audio 5000 RTP/AVP 0
a=` + dir + `
`
			parsed, err := parseTestSDP(sdp)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			m := parsed.MediaSections[0]
			if m.Direction != dir {
				t.Errorf("expected direction %s, got %s", dir, m.Direction)
			}
		})
	}
}

// Helper functions for parsing test SDP
func parseTestSDP(sdp string) (*testSDPSession, error) {
	if sdp == "" {
		return nil, errInvalidTestSDP
	}

	session := &testSDPSession{
		Bandwidth: make(map[string]int),
	}

	lines := strings.Split(sdp, "\n")
	var currentMedia *testSDPMedia
	hasVersion := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, "\r")
		if len(line) < 2 || line[1] != '=' {
			continue
		}

		key := line[0]
		value := line[2:]

		switch key {
		case 'v':
			hasVersion = true
			session.Version = 0

		case 'o':
			session.Origin = parseTestOrigin(value)

		case 's':
			session.SessionName = value

		case 'c':
			conn := parseTestConnection(value)
			if currentMedia != nil {
				currentMedia.Connection = &conn
			} else {
				session.Connection = conn
			}

		case 'm':
			media := parseTestMedia(value)
			session.MediaSections = append(session.MediaSections, media)
			currentMedia = &session.MediaSections[len(session.MediaSections)-1]

		case 'a':
			if currentMedia != nil {
				parseTestMediaAttribute(currentMedia, value)
			}
		}
	}

	if !hasVersion {
		return nil, errInvalidTestSDP
	}

	return session, nil
}

func parseTestOrigin(value string) testSDPOrigin {
	parts := strings.Fields(value)
	origin := testSDPOrigin{}
	if len(parts) >= 6 {
		origin.Username = parts[0]
		origin.SessionID = parseTestInt(parts[1])
		origin.SessionVersion = parseTestInt(parts[2])
		origin.NetType = parts[3]
		origin.AddrType = parts[4]
		origin.Address = parts[5]
	}
	return origin
}

func parseTestConnection(value string) testSDPConnection {
	parts := strings.Fields(value)
	conn := testSDPConnection{}
	if len(parts) >= 3 {
		conn.NetType = parts[0]
		conn.AddrType = parts[1]
		conn.Address = parts[2]
	}
	return conn
}

func parseTestMedia(value string) testSDPMedia {
	parts := strings.Fields(value)
	media := testSDPMedia{
		Bandwidth: make(map[string]int),
	}
	if len(parts) >= 4 {
		media.Type = parts[0]
		media.Port = int(parseTestInt(parts[1]))
		media.Protocol = parts[2]
		media.Formats = parts[3:]
	}
	return media
}

func parseTestMediaAttribute(media *testSDPMedia, value string) {
	colonIdx := strings.Index(value, ":")
	var attrName, attrValue string
	if colonIdx >= 0 {
		attrName = value[:colonIdx]
		attrValue = value[colonIdx+1:]
	} else {
		attrName = value
	}

	switch attrName {
	case "rtpmap":
		codec := parseTestRtpmap(attrValue)
		media.Codecs = append(media.Codecs, codec)

	case "fmtp":
		parts := strings.SplitN(attrValue, " ", 2)
		if len(parts) >= 2 {
			pt := int(parseTestInt(parts[0]))
			for i := range media.Codecs {
				if media.Codecs[i].PayloadType == pt {
					media.Codecs[i].FMTP = parts[1]
					break
				}
			}
		}

	case "ice-ufrag":
		media.ICEUfrag = attrValue

	case "ice-pwd":
		media.ICEPwd = attrValue

	case "candidate":
		candidate := parseTestCandidate(attrValue)
		media.Candidates = append(media.Candidates, candidate)

	case "crypto":
		crypto := parseTestCrypto(attrValue)
		media.CryptoSuites = append(media.CryptoSuites, crypto)

	case "rtcp-mux":
		media.RTCPMux = true

	case "ptime":
		media.Ptime = int(parseTestInt(attrValue))

	case "sendrecv", "sendonly", "recvonly", "inactive":
		media.Direction = attrName
	}
}

func parseTestRtpmap(value string) testCodec {
	codec := testCodec{}
	parts := strings.SplitN(value, " ", 2)
	if len(parts) >= 2 {
		codec.PayloadType = int(parseTestInt(parts[0]))
		codecParts := strings.Split(parts[1], "/")
		if len(codecParts) >= 1 {
			codec.Name = codecParts[0]
		}
		if len(codecParts) >= 2 {
			codec.ClockRate = int(parseTestInt(codecParts[1]))
		}
	}
	return codec
}

func parseTestCandidate(value string) testICECandidate {
	candidate := testICECandidate{}
	parts := strings.Fields(value)
	if len(parts) >= 6 {
		candidate.Foundation = parts[0]
		candidate.Component = int(parseTestInt(parts[1]))
		candidate.Protocol = parts[2]
		candidate.Priority = uint32(parseTestInt(parts[3]))
		candidate.Address = parts[4]
		candidate.Port = int(parseTestInt(parts[5]))
	}
	for i := 6; i < len(parts)-1; i += 2 {
		if parts[i] == "typ" {
			candidate.Type = parts[i+1]
		}
	}
	return candidate
}

func parseTestCrypto(value string) testCryptoSuite {
	crypto := testCryptoSuite{}
	parts := strings.Fields(value)
	if len(parts) >= 3 {
		crypto.Tag = int(parseTestInt(parts[0]))
		crypto.CryptoSuite = parts[1]
		crypto.KeyParameters = parts[2]
	}
	return crypto
}

func parseTestInt(s string) int64 {
	var result int64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			result = result*10 + int64(c-'0')
		} else {
			break
		}
	}
	return result
}

func generateTestSDP(s *testSDPSession) string {
	var sb strings.Builder

	sb.WriteString("v=0\r\n")
	sb.WriteString("o=")
	sb.WriteString(s.Origin.Username)
	sb.WriteString(" ")
	sb.WriteString(testItoa(s.Origin.SessionID))
	sb.WriteString(" 1 IN IP4 ")
	sb.WriteString(s.Origin.Address)
	sb.WriteString("\r\n")
	sb.WriteString("s=-\r\n")

	if s.Connection.Address != "" {
		sb.WriteString("c=IN IP4 ")
		sb.WriteString(s.Connection.Address)
		sb.WriteString("\r\n")
	}

	sb.WriteString("t=0 0\r\n")

	for _, m := range s.MediaSections {
		sb.WriteString("m=")
		sb.WriteString(m.Type)
		sb.WriteString(" ")
		sb.WriteString(testItoa(int64(m.Port)))
		sb.WriteString(" ")
		sb.WriteString(m.Protocol)
		for _, f := range m.Formats {
			sb.WriteString(" ")
			sb.WriteString(f)
		}
		sb.WriteString("\r\n")
	}

	return sb.String()
}

func testItoa(i int64) string {
	if i == 0 {
		return "0"
	}
	var result []byte
	for i > 0 {
		result = append([]byte{byte('0' + i%10)}, result...)
		i /= 10
	}
	return string(result)
}
