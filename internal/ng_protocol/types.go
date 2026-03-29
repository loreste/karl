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

// ParsedFlags contains parsed flag options
type ParsedFlags struct {
	// Media control
	AsymmetricCodecs    bool
	SymmetricCodecs     bool
	ResetMediaStreams   bool
	StrictSource        bool
	MediaHandover       bool

	// ICE/DTLS
	ICERemove           bool
	ICEForce            bool
	ICELite             bool
	DTLSPassive         bool
	TrickleICE          bool

	// Recording
	RecordCall          bool

	// Direction
	TrustAddress        bool
	DirectMedia         bool

	// SDP manipulation
	OriginalSendrecv    bool
	SymmetricIncoming   bool

	// Loop detection
	LoopProtect         bool

	// WebRTC
	WebRTCEnabled       bool

	// Quality
	RTCPMUX             bool
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

// ParseFlags parses flag strings into structured options
func ParseFlags(flags []string) *ParsedFlags {
	pf := &ParsedFlags{}

	for _, flag := range flags {
		switch flag {
		case "asymmetric-codecs", "asymmetric":
			pf.AsymmetricCodecs = true
		case "symmetric-codecs", "symmetric":
			pf.SymmetricCodecs = true
		case "reset":
			pf.ResetMediaStreams = true
		case "strict-source":
			pf.StrictSource = true
		case "media-handover":
			pf.MediaHandover = true
		case "ICE=remove", "no-ice":
			pf.ICERemove = true
		case "ICE=force", "force-ice":
			pf.ICEForce = true
		case "ICE-lite":
			pf.ICELite = true
		case "DTLS-passive":
			pf.DTLSPassive = true
		case "trickle-ice":
			pf.TrickleICE = true
		case "record-call":
			pf.RecordCall = true
		case "trust-address":
			pf.TrustAddress = true
		case "direct-media":
			pf.DirectMedia = true
		case "original-sendrecv":
			pf.OriginalSendrecv = true
		case "symmetric-incoming":
			pf.SymmetricIncoming = true
		case "loop-protect":
			pf.LoopProtect = true
		case "webrtc":
			pf.WebRTCEnabled = true
		case "rtcp-mux":
			pf.RTCPMUX = true
		}
	}

	return pf
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
