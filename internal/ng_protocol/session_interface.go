package ng_protocol

import (
	"net"
	"time"
)

// SessionRegistryInterface defines the session registry interface used by NG protocol handlers
type SessionRegistryInterface interface {
	CreateSession(callID, fromTag string) SessionInterface
	GetSession(sessionID string) (SessionInterface, bool)
	GetSessionByTags(callID, fromTag, toTag string) SessionInterface
	GetSessionByCallID(callID string) []SessionInterface
	DeleteSession(sessionID string) error
	UpdateSessionState(sessionID string, state string) error
	SetCallerLeg(sessionID string, leg *CallLegData) error
	SetCalleeLeg(sessionID string, leg *CallLegData) error
	ListSessions() []SessionInterface
	GetActiveCount() int
	AllocateMediaPorts(localIP string, minPort, maxPort int) (int, int, *net.UDPConn, *net.UDPConn, error)
}

// SessionInterface defines a session interface
type SessionInterface interface {
	GetID() string
	GetCallID() string
	GetFromTag() string
	GetToTag() string
	GetState() string
	SetFlag(name string, value bool)
	GetFlag(name string) bool
	SetMetadata(key, value string)
	GetMetadata(key string) string
	Lock()
	Unlock()
}

// CallLegData represents call leg data for the NG protocol
type CallLegData struct {
	Tag           string
	IP            net.IP
	Port          int
	RTCPPort      int
	LocalIP       net.IP
	LocalPort     int
	LocalRTCPPort int
	Conn          *net.UDPConn
	RTCPConn      *net.UDPConn
	MediaType     string
	Transport     string
	SSRC          uint32
	Codecs        []CodecData
	ICECredentials *ICECredentialsData
	SRTPParams    *SRTPParamsData
	LastActivity  time.Time
	PacketsSent   uint64
	PacketsRecv   uint64
	BytesSent     uint64
	BytesRecv     uint64
	PacketsLost   uint32
	Jitter        float64
}

// CodecData represents codec data
type CodecData struct {
	PayloadType uint8
	Name        string
	ClockRate   uint32
	Channels    int
	Fmtp        string
}

// ICECredentialsData holds ICE credentials
type ICECredentialsData struct {
	Username string
	Password string
	Lite     bool
}

// SRTPParamsData holds SRTP parameters
type SRTPParamsData struct {
	CryptoSuite string
	MasterKey   []byte
	MasterSalt  []byte
	DTLS        bool
	Fingerprint string
	Setup       string
}

// ConfigInterface defines the configuration interface
type ConfigInterface interface {
	GetMediaIP() string
	GetMinPort() int
	GetMaxPort() int
}
