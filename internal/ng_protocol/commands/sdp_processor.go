package commands

import (
	"regexp"
	"strconv"
	"strings"

	"karl/internal"
)

// ParsedSDP contains parsed SDP information
type ParsedSDP struct {
	// Session level
	SessionID      int64
	SessionVersion int64
	SessionName    string
	ConnectionIP   string

	// Media level
	MediaType    string
	MediaPort    int
	Protocol     string
	Codecs       []CodecInfo

	// ICE
	HasICE   bool
	ICEUfrag string
	ICEPwd   string
	ICELite  bool

	// DTLS
	HasDTLS     bool
	Fingerprint string
	Setup       string

	// SRTP (SDES)
	HasSRTP     bool
	CryptoSuite string
	CryptoKey   string

	// Other attributes
	Direction string
	RTCPMux   bool
	RTCPPort  int
	SSRC      uint32
}

// CodecInfo holds codec information from SDP
type CodecInfo struct {
	PayloadType uint8
	Name        string
	ClockRate   uint32
	Channels    int
	Fmtp        string
}

// SDPProcessorImpl handles SDP parsing and manipulation
type SDPProcessorImpl struct {
	config *internal.Config
}

// NewSDPProcessor creates a new SDP processor
func NewSDPProcessor(config *internal.Config) *SDPProcessorImpl {
	return &SDPProcessorImpl{config: config}
}

// Parse parses an SDP string
func (p *SDPProcessorImpl) Parse(sdp string) (*ParsedSDP, error) {
	parsed := &ParsedSDP{
		Codecs:    make([]CodecInfo, 0),
		Direction: "sendrecv",
	}

	lines := strings.Split(strings.ReplaceAll(sdp, "\r\n", "\n"), "\n")
	inMedia := false
	payloadTypes := []int{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 2 {
			continue
		}

		lineType := line[0]
		if line[1] != '=' {
			continue
		}
		value := line[2:]

		switch lineType {
		case 'o':
			// o=<username> <sess-id> <sess-version> <nettype> <addrtype> <unicast-address>
			parts := strings.Fields(value)
			if len(parts) >= 3 {
				parsed.SessionID, _ = strconv.ParseInt(parts[1], 10, 64)
				parsed.SessionVersion, _ = strconv.ParseInt(parts[2], 10, 64)
			}

		case 's':
			parsed.SessionName = value

		case 'c':
			// c=IN IP4 <ip>
			parts := strings.Fields(value)
			if len(parts) >= 3 {
				parsed.ConnectionIP = parts[2]
				// Handle cases like "192.168.1.1/127"
				if idx := strings.Index(parsed.ConnectionIP, "/"); idx != -1 {
					parsed.ConnectionIP = parsed.ConnectionIP[:idx]
				}
			}

		case 'm':
			// m=<media> <port> <proto> <fmt> ...
			inMedia = true
			parts := strings.Fields(value)
			if len(parts) >= 4 {
				parsed.MediaType = parts[0]
				parsed.MediaPort, _ = strconv.Atoi(parts[1])
				parsed.Protocol = parts[2]

				// Parse payload types
				payloadTypes = make([]int, 0, len(parts)-3)
				for _, pt := range parts[3:] {
					if ptInt, err := strconv.Atoi(pt); err == nil {
						payloadTypes = append(payloadTypes, ptInt)
					}
				}
			}

		case 'a':
			p.parseAttribute(value, parsed, &payloadTypes, inMedia)
		}
	}

	// Fill in codec info for static payload types without rtpmap
	p.fillStaticCodecs(parsed, payloadTypes)

	return parsed, nil
}

// parseAttribute parses an SDP attribute line
func (p *SDPProcessorImpl) parseAttribute(value string, parsed *ParsedSDP, payloadTypes *[]int, inMedia bool) {
	parts := strings.SplitN(value, ":", 2)
	attrName := parts[0]
	attrValue := ""
	if len(parts) > 1 {
		attrValue = parts[1]
	}

	switch attrName {
	case "rtpmap":
		// a=rtpmap:<payload type> <encoding name>/<clock rate>[/<channels>]
		codecRegex := regexp.MustCompile(`^(\d+)\s+([^/]+)/(\d+)(?:/(\d+))?`)
		if matches := codecRegex.FindStringSubmatch(attrValue); matches != nil {
			pt, _ := strconv.Atoi(matches[1])
			codec := CodecInfo{
				PayloadType: uint8(pt),
				Name:        matches[2],
				Channels:    1,
			}
			clockRate, _ := strconv.ParseUint(matches[3], 10, 32)
			codec.ClockRate = uint32(clockRate)
			if len(matches) > 4 && matches[4] != "" {
				codec.Channels, _ = strconv.Atoi(matches[4])
			}
			parsed.Codecs = append(parsed.Codecs, codec)
		}

	case "fmtp":
		// a=fmtp:<payload type> <format specific parameters>
		fmtpRegex := regexp.MustCompile(`^(\d+)\s+(.+)`)
		if matches := fmtpRegex.FindStringSubmatch(attrValue); matches != nil {
			pt, _ := strconv.Atoi(matches[1])
			fmtp := matches[2]
			for i := range parsed.Codecs {
				if int(parsed.Codecs[i].PayloadType) == pt {
					parsed.Codecs[i].Fmtp = fmtp
					break
				}
			}
		}

	case "ice-ufrag":
		parsed.HasICE = true
		parsed.ICEUfrag = attrValue

	case "ice-pwd":
		parsed.HasICE = true
		parsed.ICEPwd = attrValue

	case "ice-lite":
		parsed.ICELite = true

	case "fingerprint":
		// a=fingerprint:sha-256 <hash>
		parsed.HasDTLS = true
		parsed.Fingerprint = attrValue

	case "setup":
		parsed.Setup = attrValue

	case "crypto":
		// a=crypto:<tag> <crypto-suite> <key-params>
		parsed.HasSRTP = true
		cryptoRegex := regexp.MustCompile(`^\d+\s+(\S+)\s+inline:(.+)`)
		if matches := cryptoRegex.FindStringSubmatch(attrValue); matches != nil {
			parsed.CryptoSuite = matches[1]
			parsed.CryptoKey = matches[2]
		}

	case "rtcp-mux":
		parsed.RTCPMux = true

	case "rtcp":
		// a=rtcp:<port>
		parsed.RTCPPort, _ = strconv.Atoi(strings.Fields(attrValue)[0])

	case "sendrecv", "sendonly", "recvonly", "inactive":
		parsed.Direction = attrName

	case "ssrc":
		// a=ssrc:<ssrc-id> ...
		ssrcRegex := regexp.MustCompile(`^(\d+)`)
		if matches := ssrcRegex.FindStringSubmatch(attrValue); matches != nil {
			ssrc, _ := strconv.ParseUint(matches[1], 10, 32)
			parsed.SSRC = uint32(ssrc)
		}
	}
}

// fillStaticCodecs adds codec info for well-known static payload types
func (p *SDPProcessorImpl) fillStaticCodecs(parsed *ParsedSDP, payloadTypes []int) {
	// Map of existing payload types
	existing := make(map[uint8]bool)
	for _, c := range parsed.Codecs {
		existing[c.PayloadType] = true
	}

	// Add static payload types that weren't defined via rtpmap
	staticCodecs := map[int]CodecInfo{
		0:  {PayloadType: 0, Name: "PCMU", ClockRate: 8000, Channels: 1},
		3:  {PayloadType: 3, Name: "GSM", ClockRate: 8000, Channels: 1},
		4:  {PayloadType: 4, Name: "G723", ClockRate: 8000, Channels: 1},
		5:  {PayloadType: 5, Name: "DVI4", ClockRate: 8000, Channels: 1},
		6:  {PayloadType: 6, Name: "DVI4", ClockRate: 16000, Channels: 1},
		7:  {PayloadType: 7, Name: "LPC", ClockRate: 8000, Channels: 1},
		8:  {PayloadType: 8, Name: "PCMA", ClockRate: 8000, Channels: 1},
		9:  {PayloadType: 9, Name: "G722", ClockRate: 8000, Channels: 1},
		10: {PayloadType: 10, Name: "L16", ClockRate: 44100, Channels: 2},
		11: {PayloadType: 11, Name: "L16", ClockRate: 44100, Channels: 1},
		12: {PayloadType: 12, Name: "QCELP", ClockRate: 8000, Channels: 1},
		13: {PayloadType: 13, Name: "CN", ClockRate: 8000, Channels: 1},
		14: {PayloadType: 14, Name: "MPA", ClockRate: 90000, Channels: 1},
		15: {PayloadType: 15, Name: "G728", ClockRate: 8000, Channels: 1},
		16: {PayloadType: 16, Name: "DVI4", ClockRate: 11025, Channels: 1},
		17: {PayloadType: 17, Name: "DVI4", ClockRate: 22050, Channels: 1},
		18: {PayloadType: 18, Name: "G729", ClockRate: 8000, Channels: 1},
	}

	for _, pt := range payloadTypes {
		if !existing[uint8(pt)] {
			if codec, ok := staticCodecs[pt]; ok {
				parsed.Codecs = append(parsed.Codecs, codec)
			}
		}
	}
}

// BuildSDP builds an SDP string from components
func BuildSDP(sessionID, sessionVersion int64, localIP string, port int, codecs []CodecInfo, opts *SDPBuildOptions) string {
	var sb strings.Builder

	// Version
	sb.WriteString("v=0\r\n")

	// Origin
	sb.WriteString("o=karl ")
	sb.WriteString(strconv.FormatInt(sessionID, 10))
	sb.WriteString(" ")
	sb.WriteString(strconv.FormatInt(sessionVersion, 10))
	sb.WriteString(" IN IP4 ")
	sb.WriteString(localIP)
	sb.WriteString("\r\n")

	// Session name
	sb.WriteString("s=Karl Media Server\r\n")

	// Connection
	sb.WriteString("c=IN IP4 ")
	sb.WriteString(localIP)
	sb.WriteString("\r\n")

	// Timing
	sb.WriteString("t=0 0\r\n")

	// Media line
	sb.WriteString("m=")
	if opts != nil && opts.MediaType != "" {
		sb.WriteString(opts.MediaType)
	} else {
		sb.WriteString("audio")
	}
	sb.WriteString(" ")
	sb.WriteString(strconv.Itoa(port))
	sb.WriteString(" ")

	protocol := "RTP/AVP"
	if opts != nil {
		if opts.DTLS {
			protocol = "UDP/TLS/RTP/SAVPF"
		} else if opts.SRTP {
			protocol = "RTP/SAVP"
		}
	}
	sb.WriteString(protocol)

	// Payload types
	for _, c := range codecs {
		sb.WriteString(" ")
		sb.WriteString(strconv.Itoa(int(c.PayloadType)))
	}
	sb.WriteString("\r\n")

	// rtpmap and fmtp for each codec
	for _, c := range codecs {
		sb.WriteString("a=rtpmap:")
		sb.WriteString(strconv.Itoa(int(c.PayloadType)))
		sb.WriteString(" ")
		sb.WriteString(c.Name)
		sb.WriteString("/")
		sb.WriteString(strconv.FormatUint(uint64(c.ClockRate), 10))
		if c.Channels > 1 {
			sb.WriteString("/")
			sb.WriteString(strconv.Itoa(c.Channels))
		}
		sb.WriteString("\r\n")

		if c.Fmtp != "" {
			sb.WriteString("a=fmtp:")
			sb.WriteString(strconv.Itoa(int(c.PayloadType)))
			sb.WriteString(" ")
			sb.WriteString(c.Fmtp)
			sb.WriteString("\r\n")
		}
	}

	// Direction
	direction := "sendrecv"
	if opts != nil && opts.Direction != "" {
		direction = opts.Direction
	}
	sb.WriteString("a=")
	sb.WriteString(direction)
	sb.WriteString("\r\n")

	// RTCP-mux
	if opts != nil && opts.RTCPMux {
		sb.WriteString("a=rtcp-mux\r\n")
	}

	// ICE
	if opts != nil && opts.ICE {
		sb.WriteString("a=ice-ufrag:")
		sb.WriteString(opts.ICEUfrag)
		sb.WriteString("\r\n")
		sb.WriteString("a=ice-pwd:")
		sb.WriteString(opts.ICEPwd)
		sb.WriteString("\r\n")
		if opts.ICELite {
			sb.WriteString("a=ice-lite\r\n")
		}
		for _, candidate := range opts.ICECandidates {
			sb.WriteString("a=candidate:")
			sb.WriteString(candidate)
			sb.WriteString("\r\n")
		}
	}

	// DTLS
	if opts != nil && opts.DTLS {
		sb.WriteString("a=fingerprint:")
		sb.WriteString(opts.FingerprintHash)
		sb.WriteString(" ")
		sb.WriteString(opts.Fingerprint)
		sb.WriteString("\r\n")
		sb.WriteString("a=setup:")
		sb.WriteString(opts.Setup)
		sb.WriteString("\r\n")
	}

	// SRTP crypto
	if opts != nil && opts.SRTP && !opts.DTLS {
		sb.WriteString("a=crypto:1 ")
		sb.WriteString(opts.CryptoSuite)
		sb.WriteString(" inline:")
		sb.WriteString(opts.CryptoKey)
		sb.WriteString("\r\n")
	}

	return sb.String()
}

// SDPBuildOptions contains options for building SDP
type SDPBuildOptions struct {
	MediaType       string
	Direction       string
	RTCPMux         bool
	ICE             bool
	ICEUfrag        string
	ICEPwd          string
	ICELite         bool
	ICECandidates   []string
	DTLS            bool
	Fingerprint     string
	FingerprintHash string
	Setup           string
	SRTP            bool
	CryptoSuite     string
	CryptoKey       string
}
