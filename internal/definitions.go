package internal

// CandidatePairStats represents stats related to an ICE candidate pair
type CandidatePairStats struct {
	// Timestamp when the stats were collected
	Timestamp float64

	// LocalCandidateID local candidate ID
	LocalCandidateID string

	// RemoteCandidateID remote candidate ID
	RemoteCandidateID string

	// State represents the state of the checklist for the local and remote candidates
	State string

	// Nominated indicates if this pair is the nominated pair
	Nominated bool

	// PacketsSent total packets sent on this candidate pair
	PacketsSent uint32

	// PacketsReceived total packets received on this candidate pair
	PacketsReceived uint32

	// BytesSent total bytes sent on this candidate pair
	BytesSent uint64

	// BytesReceived total bytes received on this candidate pair
	BytesReceived uint64

	// LastPacketSentTimestamp timestamp of the last packet sent
	LastPacketSentTimestamp float64

	// LastPacketReceivedTimestamp timestamp of the last packet received
	LastPacketReceivedTimestamp float64

	// CurrentRoundTripTime current round trip time for this candidate pair
	CurrentRoundTripTime float64

	// AvailableOutgoingBitrate estimated available outgoing bitrate
	AvailableOutgoingBitrate float64

	// AvailableIncomingBitrate estimated available incoming bitrate
	AvailableIncomingBitrate float64

	// CircuitBreakerTriggerCount number of times the circuit breaker was triggered
	CircuitBreakerTriggerCount uint32

	// ResponsesReceived total STUN responses received
	ResponsesReceived uint32

	// RequestsSent total STUN requests sent
	RequestsSent uint32

	// RetransmissionsReceived total retransmissions received
	RetransmissionsReceived uint32

	// RetransmissionsSent total retransmissions sent
	RetransmissionsSent uint32

	// ConsentRequestsSent total consent requests sent
	ConsentRequestsSent uint32

	// ConsentExpiredTimestamp timestamp when consent expired
	ConsentExpiredTimestamp float64

	// Priority computed priority of this candidate pair
	Priority uint64

	// TotalRoundTripTime total round trip time
	TotalRoundTripTime float64

	// Writable indicates if the connection is writable
	Writable bool
}

// TransportStats represents stats about the underlying transport
type TransportStats struct {
	// BytesSent represents the total number of bytes sent
	BytesSent uint64

	// BytesReceived represents the total number of bytes received
	BytesReceived uint64

	// PacketsSent represents the total number of packets sent
	PacketsSent uint32

	// PacketsReceived represents the total number of packets received
	PacketsReceived uint32

	// RTCPPacketsSent represents the total number of RTCP packets sent
	RTCPPacketsSent uint32

	// RTCPPacketsReceived represents the total number of RTCP packets received
	RTCPPacketsReceived uint32

	// ActiveConnection indicates whether this is the active connection
	ActiveConnection bool

	// CurrentRoundTripTime current round trip time
	CurrentRoundTripTime float64
}

// RTPStreamStats represents common stats used by both inbound and outbound RTP streams
type RTPStreamStats struct {
	// SSRC represents the synchronization source identifier
	SSRC uint32

	// Kind represents the kind of media (audio/video)
	Kind string

	// PacketsLost represents the total number of packets lost
	PacketsLost int32

	// Jitter represents the packet jitter in seconds
	Jitter float64

	// LastPacketReceivedTimestamp timestamp when the last packet was received
	LastPacketReceivedTimestamp float64
}

// InboundRTPStreamStats represents stats for an inbound RTP stream
type InboundRTPStreamStats struct {
	RTPStreamStats

	// PacketsReceived represents the total number of packets received
	PacketsReceived uint32

	// BytesReceived represents the total number of bytes received
	BytesReceived uint64

	// FractionLost represents the fraction of packets lost
	FractionLost float64

	// PacketsDiscarded represents the total number of packets discarded
	PacketsDiscarded uint32
}

// OutboundRTPStreamStats represents stats for an outbound RTP stream
type OutboundRTPStreamStats struct {
	RTPStreamStats

	// PacketsSent represents the total number of packets sent
	PacketsSent uint32

	// BytesSent represents the total number of bytes sent
	BytesSent uint64

	// TargetBitrate represents the current target bitrate
	TargetBitrate float64

	// RoundTripTime represents the current round trip time
	RoundTripTime float64
}

// Status values for SIP sessions
const (
	SessionStatusNew      = "new"
	SessionStatusActive   = "active"
	SessionStatusInactive = "inactive"
	SessionStatusClosed   = "closed"
	SessionStatusFailed   = "failed"
)

// Media types
const (
	MediaTypeAudio = "audio"
	MediaTypeVideo = "video"
	MediaTypeData  = "data"
)

// Codec identifiers
const (
	CodecOpus  = "opus"
	CodecG711u = "PCMU"
	CodecG711a = "PCMA"
	CodecVP8   = "VP8"
	CodecH264  = "H264"
)

// RTP/SRTP related constants
const (
	RTPHeaderSize        = 12 // bytes
	SRTPAuthTagSize      = 10 // bytes (for AES-CM-128-HMAC-SHA1-80)
	MaxPacketSize        = 1500
	DefaultJitterBuffer  = 50  // ms
	DefaultPacketTimeout = 200 // ms
)

// Log levels
const (
	LogLevelError = 1
	LogLevelWarn  = 2
	LogLevelInfo  = 3
	LogLevelDebug = 4
	LogLevelTrace = 5
)

// Global settings
var (
	LogLevel = LogLevelInfo // Default log level
)
