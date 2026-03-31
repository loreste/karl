package ng_protocol

import (
	"net"
	"time"
)

// Command names supported by the NG protocol
const (
	CmdPing           = "ping"
	CmdOffer          = "offer"
	CmdAnswer         = "answer"
	CmdDelete         = "delete"
	CmdQuery          = "query"
	CmdList           = "list"
	CmdStatistics     = "statistics"
	CmdStartRecording = "start recording"
	CmdStopRecording  = "stop recording"
	CmdPauseRecording = "pause recording"
	CmdBlockDTMF      = "block DTMF"
	CmdUnblockDTMF    = "unblock DTMF"
	CmdPlayDTMF       = "play DTMF"
	CmdBlockMedia     = "block media"
	CmdUnblockMedia   = "unblock media"
	CmdSilenceMedia   = "silence media"
	CmdStartForward   = "start forwarding"
	CmdStopForward    = "stop forwarding"
	CmdPlayMedia      = "play media"
	CmdStopMedia      = "stop media"
)

// Result codes for NG protocol responses
const (
	ResultOK             = "ok"
	ResultPong           = "pong"
	ResultError          = "error"
	ResultCallNotFound   = "call not found"
	ResultSyntaxError    = "syntax error"
	ResultUnknownCommand = "unknown command"
	ResultTimeout        = "timeout"
)

// Error reasons
const (
	ErrReasonNotFound     = "Call-ID not found"
	ErrReasonInvalidSDP   = "Invalid SDP"
	ErrReasonMediaError   = "Media allocation error"
	ErrReasonInternal     = "Internal error"
	ErrReasonTimeout      = "Operation timed out"
	ErrReasonUnsupported  = "Unsupported operation"
	ErrReasonMissingParam = "Missing required parameter"
)

// Direction flags
const (
	DirectionExternal = "external"
	DirectionInternal = "internal"
)

// ICE options
const (
	ICERemove   = "remove"
	ICEForce    = "force"
	ICEDefault  = "default"
	ICEOptional = "optional"
)

// Transport protocols
const (
	TransportRTPAVP   = "RTP/AVP"
	TransportRTPSAVP  = "RTP/SAVP"
	TransportRTPSAVPF = "RTP/SAVPF"
	TransportUDPTLS   = "UDP/TLS/RTP/SAVPF"
)

// NGRequest represents a parsed NG protocol request
type NGRequest struct {
	Cookie     string
	Command    string
	CallID     string
	FromTag    string
	ToTag      string
	ViaBranch  string
	SDP        string
	Flags      []string
	Replace    []string
	Direction  []string
	ReceivedFrom *net.UDPAddr
	Timestamp  time.Time

	// Call control options
	ICE              string
	DTLS             string
	SDES             []string
	Transport        string
	MediaAddress     string
	AddressFamily    string

	// Recording options
	RecordCall      bool
	RecordingMeta   map[string]string

	// Media manipulation
	Codec           []string
	Transcode       []string
	Ptime           int

	// Advanced options
	Label           string
	SetLabel        string
	FromLabel       string
	ToLabel         string

	// DTMF options
	DTMFDigit       string
	DTMFDuration    int

	// Forwarding options
	ForwardAddress  string
	ForwardPort     int

	// Raw parameters for extension
	RawParams       BencodeDict
}

// NGResponse represents an NG protocol response
type NGResponse struct {
	Result    string
	ErrorReason string
	SDP       string

	// Session info
	CallID    string
	FromTag   string
	ToTag     string

	// Media info
	Streams   []StreamInfo

	// Statistics
	Stats     *CallStats

	// Query response
	Created   int64
	LastSignal int64

	// Additional fields
	Warning   string
	Tag       map[string]TagInfo

	// Raw data for extension
	Extra     map[string]interface{}
}

// StreamInfo represents media stream information
type StreamInfo struct {
	LocalIP     string
	LocalPort   int
	LocalRTCPPort int
	MediaType   string
	Protocol    string
	Index       int
	Flags       []string

	// ICE candidates
	ICECandidates []ICECandidate
	ICEUfrag      string
	ICEPwd        string

	// SRTP info
	CryptoSuite   string
	SRTPKey       string
	Fingerprint   string
	FingerprintHash string
	Setup         string
}

// ICECandidate represents an ICE candidate
type ICECandidate struct {
	Foundation  string
	Component   int
	Protocol    string
	Priority    uint32
	IP          string
	Port        int
	Type        string
	RelatedIP   string
	RelatedPort int
}

// CallStats represents call statistics
type CallStats struct {
	CreatedAt     time.Time
	Duration      time.Duration

	// Packet counts
	PacketsSent   uint64
	PacketsRecv   uint64
	BytesSent     uint64
	BytesRecv     uint64

	// Quality metrics
	PacketLoss    float64
	Jitter        float64
	RTT           float64
	MOS           float64

	// Per-leg stats
	Legs          []LegStats
}

// LegStats represents per-leg statistics
type LegStats struct {
	Tag           string
	SSRC          uint32
	PacketsSent   uint64
	PacketsRecv   uint64
	BytesSent     uint64
	BytesRecv     uint64
	PacketLoss    float64
	Jitter        float64
	RTT           float64
}

// TagInfo represents tag-specific information
type TagInfo struct {
	Tag         string
	Label       string
	InDialogue  bool
	MediaCount  int
	Created     int64
	Medias      []MediaInfo
}

// MediaInfo represents media stream info for a tag
type MediaInfo struct {
	Index      int
	Type       string
	Protocol   string
	LocalIP    string
	LocalPort  int
	Streams    []RTPStreamInfo
}

// RTPStreamInfo represents RTP stream details
type RTPStreamInfo struct {
	LocalPort     int
	LocalRTCPPort int
	SSRC          uint32
	PayloadType   int
	Codec         string
	ClockRate     int

	// Stats
	PacketsSent   uint64
	PacketsRecv   uint64
	BytesSent     uint64
	BytesRecv     uint64
	LastPacketAt  time.Time
}

// ParsedFlags contains parsed flag options - rtpengine compatible
type ParsedFlags struct {
	// === Media Control ===
	AsymmetricCodecs  bool
	SymmetricCodecs   bool
	Asymmetric        bool // Allow asymmetric RTP
	Symmetric         bool // Force symmetric RTP
	Unidirectional    bool
	StrictSource      bool
	MediaHandover     bool
	Reset             bool // Reset port latching

	// === ICE Handling ===
	ICERemove      bool
	ICEForce       bool
	ICEForceRelay  bool // Force TURN relay
	ICELite        bool
	ICEDefault     bool
	TrickleICE     bool
	GenerateMID    bool // Generate MID attributes

	// === DTLS Control ===
	DTLSOff        bool
	DTLSPassive    bool
	DTLSActive     bool
	DTLSReverse    bool // Reverse DTLS role
	DTLSFingerprint string

	// === SDES/SRTP Control ===
	SDESOff                bool
	SDESOn                 bool
	SDESOnly               bool // SDES only, no DTLS
	SDESUnencryptedSRTP    bool
	SDESUnencryptedSRTCP   bool
	SDESUnauthenticated    bool
	SDESPad                bool
	SDESNoCrypto           []string // Per-crypto SDES control

	// === SDP Manipulation ===
	ReplaceOrigin               bool
	ReplaceSessionConnection    bool
	ReplaceSDPVersion           bool
	ReplaceUsername             bool
	ReplaceSessionName          bool
	TrustAddress                bool
	SIPSourceAddress            bool
	PortLatching                bool
	NoPortLatching              bool

	// === Direction Control ===
	OriginalSendrecv bool
	SendOnly         bool
	RecvOnly         bool
	Inactive         bool
	SymmetricIncoming bool
	DirectMedia       bool

	// === Recording ===
	RecordCall     bool
	StartRecording bool
	StopRecording  bool
	PauseRecording bool

	// === Media Blocking ===
	BlockMedia    bool
	UnblockMedia  bool
	SilenceMedia  bool
	BlockDTMF     bool
	UnblockDTMF   bool

	// === RTP/RTCP Behavior ===
	RTCPMUX           bool
	RTCPMUXDemux      bool
	RTCPMUXAccept     bool
	RTCPMUXOffer      bool
	RTCPMUXRequire    bool
	NoRTCPAttribute   bool
	FullRTCPAttribute bool
	GenerateRTCP      bool

	// === Transport Protocols ===
	RTPAVP    bool
	RTPSAVP   bool
	RTPAVPF   bool
	RTPSAVPF  bool
	UDPTLS    bool

	// === Loop/Echo ===
	LoopProtect bool
	MediaEcho   bool

	// === WebRTC ===
	WebRTCEnabled bool

	// === Quality ===
	TOS       int  // TOS/DSCP value (-1 = not set)
	TOSSet    bool // Whether TOS was explicitly set

	// === Timeout ===
	MediaTimeout   int  // Media timeout in seconds
	SessionTimeout int  // Session timeout
	DeleteDelay    int  // Delay before delete

	// === Buffering ===
	DelayBuffer    int  // Delay buffer in milliseconds for jitter compensation

	// === RTCP ===
	RTCPInterval   int  // RTCP report interval in milliseconds (frequency flag)

	// === T.38 ===
	T38Support   bool
	T38Gateway   bool
	T38FaxUDPEC  bool

	// === Codec Control ===
	AlwaysTranscode  bool
	TranscodeCodecs  []string
	StripCodecs      []string
	StripAllCodecs   bool
	OfferCodecs      []string
	MaskCodecs       []string
	SetCodecs        []string
	ExceptCodecs     []string
	Ptime            int // Packet time
	PtimeReverse     bool

	// === Address Selection ===
	AddressFamily    string // inet, inet6
	MediaAddress     string
	Interface        string
	FromInterface    string
	ToInterface      string
	ReceivedFrom     string

	// === Labels ===
	Label     string
	SetLabel  string
	FromLabel string
	ToLabel   string
	All       bool // Apply to all legs

	// === Metadata ===
	RecordingMetadata map[string]string
	RecordingFile     string
	RecordingPath     string
	RecordingPattern  string
	SIPREC            bool

	// === Via Branch ===
	ViaBranch string

	// === Misc ===
	EarlyMedia bool
}

// SDPManipulation contains SDP manipulation options
type SDPManipulation struct {
	// Codec preferences
	CodecAccept    []string
	CodecExcept    []string
	CodecMask      []string
	CodecConsume   []string
	CodecTranscode []string

	// Address handling
	MediaAddress   string
	AddressFamily  string // inet, inet6

	// Ptime
	Ptime          int

	// Bandwidth
	Bandwidth      int

	// Direction
	SendOnly       bool
	RecvOnly       bool
	Inactive       bool
}

// RecordingOptions contains recording configuration
type RecordingOptions struct {
	Enabled      bool
	Path         string
	Format       string // wav, pcm
	Mode         string // mixed, stereo, separate
	Metadata     map[string]string
}

// ForwardingOptions contains media forwarding config
type ForwardingOptions struct {
	Enabled   bool
	Address   string
	Port      int
	Protocol  string
	SRTP      bool
	SRTPKey   string
}

// MediaManipulation contains media manipulation settings
type MediaManipulation struct {
	Block     bool
	Silence   bool
	DTMFBlock bool
}

// ParseFlags parses flag strings into structured options - rtpengine compatible
func ParseFlags(flags []string) *ParsedFlags {
	pf := &ParsedFlags{
		TOS:            -1, // Not set
		MediaTimeout:   -1,
		SessionTimeout: -1,
		DeleteDelay:    -1,
		DelayBuffer:    -1,
		RTCPInterval:   -1,
		Ptime:          -1,
	}

	for _, flag := range flags {
		// Handle flags with values (e.g., "ICE=remove", "TOS=184", "media-timeout=60")
		if idx := indexOf(flag, "="); idx > 0 {
			key := flag[:idx]
			value := flag[idx+1:]
			parseKeyValueFlag(pf, key, value)
			continue
		}

		// Handle simple boolean flags
		switch flag {
		// === Media Control ===
		case "asymmetric-codecs":
			pf.AsymmetricCodecs = true
		case "symmetric-codecs":
			pf.SymmetricCodecs = true
		case "asymmetric":
			pf.Asymmetric = true
		case "symmetric":
			pf.Symmetric = true
		case "unidirectional":
			pf.Unidirectional = true
		case "strict-source":
			pf.StrictSource = true
		case "media-handover":
			pf.MediaHandover = true
		case "reset":
			pf.Reset = true

		// === ICE ===
		case "no-ice":
			pf.ICERemove = true
		case "force-ice":
			pf.ICEForce = true
		case "ICE-lite", "ice-lite":
			pf.ICELite = true
		case "trickle-ice":
			pf.TrickleICE = true
		case "generate-mid":
			pf.GenerateMID = true

		// === DTLS ===
		case "DTLS-passive", "dtls-passive":
			pf.DTLSPassive = true
		case "DTLS-active", "dtls-active":
			pf.DTLSActive = true
		case "DTLS-off", "dtls-off", "no-dtls":
			pf.DTLSOff = true
		case "DTLS-reverse", "dtls-reverse":
			pf.DTLSReverse = true

		// === SDES/SRTP ===
		case "SDES-off", "sdes-off", "no-sdes":
			pf.SDESOff = true
		case "SDES-on", "sdes-on":
			pf.SDESOn = true
		case "SDES-only", "sdes-only":
			pf.SDESOnly = true
		case "SDES-unencrypted_srtp", "unencrypted-srtp":
			pf.SDESUnencryptedSRTP = true
		case "SDES-unencrypted_srtcp", "unencrypted-srtcp":
			pf.SDESUnencryptedSRTCP = true
		case "SDES-unauthenticated":
			pf.SDESUnauthenticated = true
		case "SDES-pad":
			pf.SDESPad = true

		// === SDP Manipulation ===
		case "replace-origin":
			pf.ReplaceOrigin = true
		case "replace-session-connection":
			pf.ReplaceSessionConnection = true
		case "replace-sdp-version":
			pf.ReplaceSDPVersion = true
		case "replace-username":
			pf.ReplaceUsername = true
		case "replace-session-name":
			pf.ReplaceSessionName = true
		case "trust-address":
			pf.TrustAddress = true
		case "SIP-source-address", "sip-source-address":
			pf.SIPSourceAddress = true
		case "port-latching":
			pf.PortLatching = true
		case "no-port-latching":
			pf.NoPortLatching = true

		// === Direction ===
		case "original-sendrecv":
			pf.OriginalSendrecv = true
		case "sendonly", "send-only":
			pf.SendOnly = true
		case "recvonly", "recv-only":
			pf.RecvOnly = true
		case "inactive":
			pf.Inactive = true
		case "symmetric-incoming":
			pf.SymmetricIncoming = true
		case "direct-media":
			pf.DirectMedia = true

		// === Recording ===
		case "record-call":
			pf.RecordCall = true
		case "start-recording":
			pf.StartRecording = true
		case "stop-recording":
			pf.StopRecording = true
		case "pause-recording":
			pf.PauseRecording = true
		case "SIPREC", "siprec":
			pf.SIPREC = true

		// === Media Blocking ===
		case "block-media":
			pf.BlockMedia = true
		case "unblock-media":
			pf.UnblockMedia = true
		case "silence-media":
			pf.SilenceMedia = true
		case "block-dtmf":
			pf.BlockDTMF = true
		case "unblock-dtmf":
			pf.UnblockDTMF = true

		// === RTCP ===
		case "rtcp-mux":
			pf.RTCPMUX = true
		case "rtcp-mux-demux":
			pf.RTCPMUXDemux = true
		case "rtcp-mux-accept":
			pf.RTCPMUXAccept = true
		case "rtcp-mux-offer":
			pf.RTCPMUXOffer = true
		case "rtcp-mux-require":
			pf.RTCPMUXRequire = true
		case "no-rtcp-attribute":
			pf.NoRTCPAttribute = true
		case "full-rtcp-attribute":
			pf.FullRTCPAttribute = true
		case "generate-rtcp":
			pf.GenerateRTCP = true

		// === Transport Protocols ===
		case "RTP/AVP":
			pf.RTPAVP = true
		case "RTP/SAVP":
			pf.RTPSAVP = true
		case "RTP/AVPF":
			pf.RTPAVPF = true
		case "RTP/SAVPF":
			pf.RTPSAVPF = true
		case "UDP/TLS/RTP/SAVPF", "UDP/TLS/RTP/SAVP":
			pf.UDPTLS = true

		// === Loop/Echo ===
		case "loop-protect":
			pf.LoopProtect = true
		case "media-echo":
			pf.MediaEcho = true

		// === WebRTC ===
		case "webrtc":
			pf.WebRTCEnabled = true

		// === T.38 ===
		case "T.38", "t38", "T38":
			pf.T38Support = true
		case "T.38-gateway", "t38-gateway":
			pf.T38Gateway = true
		case "T.38-fax-udp-ec":
			pf.T38FaxUDPEC = true

		// === Codec ===
		case "always-transcode":
			pf.AlwaysTranscode = true
		case "codec-strip-all", "strip-all-codecs":
			pf.StripAllCodecs = true
		case "ptime-reverse":
			pf.PtimeReverse = true

		// === Labels ===
		case "all":
			pf.All = true

		// === Misc ===
		case "early-media":
			pf.EarlyMedia = true
		}
	}

	return pf
}

// indexOf returns the index of substr in s, or -1 if not found
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// parseKeyValueFlag handles flags with values like "ICE=remove", "TOS=184"
func parseKeyValueFlag(pf *ParsedFlags, key, value string) {
	switch key {
	// ICE handling
	case "ICE":
		switch value {
		case "remove":
			pf.ICERemove = true
		case "force":
			pf.ICEForce = true
		case "force-relay":
			pf.ICEForceRelay = true
		case "default":
			pf.ICEDefault = true
		case "lite":
			pf.ICELite = true
		}

	// DTLS handling
	case "DTLS":
		switch value {
		case "off":
			pf.DTLSOff = true
		case "passive":
			pf.DTLSPassive = true
		case "active":
			pf.DTLSActive = true
		}

	// SDES handling
	case "SDES":
		switch value {
		case "off":
			pf.SDESOff = true
		case "on":
			pf.SDESOn = true
		case "only":
			pf.SDESOnly = true
		}

	// TOS/DSCP
	case "TOS", "tos":
		if v := parseIntValue(value); v >= 0 {
			pf.TOS = v
			pf.TOSSet = true
		}

	// Timeouts
	case "media-timeout":
		if v := parseIntValue(value); v >= 0 {
			pf.MediaTimeout = v
		}
	case "session-timeout":
		if v := parseIntValue(value); v >= 0 {
			pf.SessionTimeout = v
		}
	case "delete-delay":
		if v := parseIntValue(value); v >= 0 {
			pf.DeleteDelay = v
		}

	// Delay buffer (jitter compensation)
	case "delay-buffer":
		if v := parseIntValue(value); v >= 0 {
			pf.DelayBuffer = v
		}

	// RTCP interval (frequency)
	case "frequency", "rtcp-interval":
		if v := parseIntValue(value); v >= 0 {
			pf.RTCPInterval = v
		}

	// Ptime
	case "ptime":
		if v := parseIntValue(value); v > 0 {
			pf.Ptime = v
		}

	// Address selection
	case "address-family":
		pf.AddressFamily = value
	case "media-address":
		pf.MediaAddress = value
	case "interface":
		pf.Interface = value
	case "from-interface":
		pf.FromInterface = value
	case "to-interface":
		pf.ToInterface = value
	case "received-from":
		pf.ReceivedFrom = value

	// Labels
	case "label":
		pf.Label = value
	case "set-label":
		pf.SetLabel = value
	case "from-label":
		pf.FromLabel = value
	case "to-label":
		pf.ToLabel = value
	case "via-branch":
		pf.ViaBranch = value

	// Recording
	case "recording-file":
		pf.RecordingFile = value
	case "recording-path":
		pf.RecordingPath = value
	case "recording-pattern":
		pf.RecordingPattern = value

	// Codec manipulation
	case "codec-strip", "strip-codec":
		pf.StripCodecs = append(pf.StripCodecs, value)
	case "codec-offer", "offer-codec":
		pf.OfferCodecs = append(pf.OfferCodecs, value)
	case "codec-mask", "mask-codec":
		pf.MaskCodecs = append(pf.MaskCodecs, value)
	case "codec-transcode", "transcode":
		pf.TranscodeCodecs = append(pf.TranscodeCodecs, value)
	case "codec-set":
		pf.SetCodecs = append(pf.SetCodecs, value)
	case "codec-except":
		pf.ExceptCodecs = append(pf.ExceptCodecs, value)

	// DTLS fingerprint
	case "DTLS-fingerprint":
		pf.DTLSFingerprint = value

	// SDES per-crypto control
	case "SDES-no":
		pf.SDESNoCrypto = append(pf.SDESNoCrypto, value)
	}
}

// parseIntValue parses a string to int, returns -1 on error
func parseIntValue(s string) int {
	var v int
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		v = v*10 + int(c-'0')
	}
	return v
}

// CallListEntry represents an entry in the call list
type CallListEntry struct {
	CallID     string
	FromTag    string
	ToTag      string
	Created    time.Time
	LastSignal time.Time
	State      string
	Duration   time.Duration
}

// NetworkInterface represents a named network interface for media
type NetworkInterface struct {
	Name          string   // Interface name (e.g., "internal", "external")
	Address       string   // IP address to use
	AdvertiseAddr string   // Address to advertise in SDP (for NAT)
	Port          int      // Base port (optional)
	LocalAddrs    []string // Additional local addresses
}

// InterfaceConfig holds all interface configurations
type InterfaceConfig struct {
	Interfaces map[string]*NetworkInterface
	Default    string // Default interface name
}

// PeerConfig represents peer-based interface selection rules
type PeerConfig struct {
	Address   string // Peer IP/CIDR
	Interface string // Interface to use for this peer
}

// DirectionConfig represents direction-based interface selection
type DirectionConfig struct {
	Internal []string // Internal networks (CIDRs)
	External []string // External networks (CIDRs)
}

// LegInfo represents per-leg information in a call
type LegInfo struct {
	Tag           string
	Label         string
	Interface     string
	MediaAddress  string
	AddressFamily string // inet or inet6
	Codecs        []CodecInfo
	ICEState      string
	DTLSState     string
	Direction     string
	Recording     bool
	BlockMedia    bool
	BlockDTMF     bool
	Created       time.Time
	Stats         *LegStats
}

// CodecInfo represents codec parameters
type CodecInfo struct {
	PayloadType int
	Name        string
	ClockRate   int
	Channels    int
	FMTP        string
	Enabled     bool
}

// SessionFlags holds per-session flags
type SessionFlags struct {
	// Media behavior
	Symmetric         bool
	Asymmetric        bool
	StrictSource      bool
	MediaHandover     bool
	PortLatching      bool

	// ICE
	ICELite           bool

	// Recording
	Recording         bool
	RecordingPath     string
	RecordingMetadata map[string]string

	// Blocking
	MediaBlocked      bool
	DTMFBlocked       bool
	Silenced          bool

	// Quality
	TOS               int
	MediaTimeout      int

	// T.38
	T38Enabled        bool
}

// CallDirection represents call direction for interface selection
type CallDirection int

const (
	CallDirectionUnknown CallDirection = iota
	CallDirectionInbound
	CallDirectionOutbound
	CallDirectionInternal
	CallDirectionExternal
)

// AggregateStats represents aggregate statistics
type AggregateStats struct {
	CurrentCalls     int
	TotalCalls       uint64
	TotalDuration    time.Duration
	AvgCallDuration  time.Duration
	PacketsSent      uint64
	PacketsRecv      uint64
	BytesSent        uint64
	BytesRecv        uint64
	PacketsLost      uint64
	AvgJitter        float64
	AvgMOS           float64
	ErrorCount       uint64
	Uptime           time.Duration
}
