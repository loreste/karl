package internal

import "time"

// Version information
const (
	ConfigVersion   = "1.0.0"
	MinJitterBuffer = 10    // Minimum acceptable jitter buffer (ms)
	MaxJitterBuffer = 200   // Maximum acceptable jitter buffer (ms)
	MinBandwidth    = 64    // Minimum bandwidth in kbps
	MaxBandwidth    = 10000 // Maximum bandwidth in kbps
)

// SRTPConfig defines secure RTP settings
type SRTPConfig struct {
	Key  string `json:"srtp_key"`
	Salt string `json:"srtp_salt"`
}

// DatabaseConfig defines MySQL and Redis settings
type DatabaseConfig struct {
	MySQLDSN             string `json:"mysql_dsn"`
	RedisEnabled         bool   `json:"redis_enabled"`
	RedisAddr            string `json:"redis_addr"`
	RedisCleanupInterval int    `json:"redis_cleanup_interval"`
	MaxConnections       int    `json:"max_connections"`
	ConnectionTimeout    int    `json:"connection_timeout"`
}

// TransportConfig holds networking settings
type TransportConfig struct {
	UDPEnabled  bool   `json:"udp_enabled"`
	UDPPort     int    `json:"udp_port"`
	TCPEnabled  bool   `json:"tcp_enabled"`
	TCPPort     int    `json:"tcp_port"`
	TLSEnabled  bool   `json:"tls_enabled"`
	TLSPort     int    `json:"tls_port"`
	TLSCert     string `json:"tls_cert"`
	TLSKey      string `json:"tls_key"`
	IPv6Enabled bool   `json:"ipv6_enabled"`
	MTU         int    `json:"mtu"`
}

// RTPSettings defines RTP media handling configurations
type RTPSettings struct {
	MaxBandwidth        int    `json:"max_bandwidth"`
	MinJitterBuffer     int    `json:"min_jitter_buffer"`
	PacketLossThreshold int    `json:"packet_loss_threshold"`
	Encryption          bool   `json:"encryption"`
	EnablePCAP          bool   `json:"enable_pcap"`
	DTMFEnabled         bool   `json:"dtmf_enabled"`
	FECEnabled          bool   `json:"fec_enabled"`     // Forward Error Correction
	FECDestination      string `json:"fec_destination"` // FEC packet destination IP
	FECPort             int    `json:"fec_port"`        // FEC packet destination port
	REDEnabled          bool   `json:"red_enabled"`     // Redundant Encoding
	RTCPInterval        int    `json:"rtcp_interval"`   // RTCP report interval in seconds
	VADEnabled          bool   `json:"vad_enabled"`     // Voice Activity Detection
	PLIInterval         int    `json:"pli_interval"`    // Picture Loss Indication interval
}

// TURNServer represents a TURN server configuration
type TURNServer struct {
	URL        string `json:"url"`
	Username   string `json:"username"`
	Credential string `json:"credential"`
	Weight     int    `json:"weight"` // For load balancing
	Region     string `json:"region"` // Geographic region
}

// WebRTCConfig holds WebRTC settings
type WebRTCConfig struct {
	Enabled          bool         `json:"enabled"`
	WebRTCPort       int          `json:"webrtc_port"`
	StunServers      []string     `json:"stun_servers"`
	TurnServers      []TURNServer `json:"turn_servers"`
	MaxBitrate       int          `json:"max_bitrate"`
	StartBitrate     int          `json:"start_bitrate"`
	BWEstimation     bool         `json:"bw_estimation"`
	TCCEnabled       bool         `json:"tcc_enabled"` // Transport-CC feedback
	RecordingEnabled bool         `json:"recording_enabled"`
	RecordingPath    string       `json:"recording_path"`
}

// IntegrationConfig defines SIP proxy settings
type IntegrationConfig struct {
	OpenSIPSIp        string `json:"opensips_ip"`
	OpenSIPSPort      int    `json:"opensips_port"`
	KamailioIp        string `json:"kamailio_ip"`
	KamailioPort      int    `json:"kamailio_port"`
	RTPengineSocket   string `json:"rtpengine_socket"`
	MediaIP           string `json:"media_ip"`
	PublicIP          string `json:"public_ip"`
	BackupMediaIP     string `json:"backup_media_ip"`
	FailoverEnabled   bool   `json:"failover_enabled"`
	KeepAliveInterval int    `json:"keepalive_interval"`
}

// AlertSettings defines monitoring thresholds
type AlertSettings struct {
	PacketLossThreshold float64 `json:"packet_loss_threshold"`
	JitterThreshold     float64 `json:"jitter_threshold"`
	BandwidthThreshold  int     `json:"bandwidth_threshold"`
	NotifyAdmin         bool    `json:"notify_admin"`
	AdminEmail          string  `json:"admin_email"`
	AlertInterval       int     `json:"alert_interval"` // Minimum time between alerts
	MaxAlertsPerHour    int     `json:"max_alerts_per_hour"`
	SlackWebhook        string  `json:"slack_webhook"`
	PagerDutyKey        string  `json:"pagerduty_key"`
}

// NGProtocolConfig defines NG protocol settings
type NGProtocolConfig struct {
	Enabled    bool   `json:"enabled"`
	SocketPath string `json:"socket_path"`
	UDPPort    int    `json:"udp_port"`
	Timeout    int    `json:"timeout"` // Request timeout in seconds
}

// RecordingConfig defines call recording settings
type RecordingConfig struct {
	Enabled       bool   `json:"enabled"`
	BasePath      string `json:"base_path"`
	Format        string `json:"format"`         // wav, pcm
	Mode          string `json:"mode"`           // mixed, stereo, separate
	SampleRate    int    `json:"sample_rate"`    // 8000, 16000, 48000
	BitsPerSample int    `json:"bits_per_sample"` // 8, 16
	MaxFileSize   int64  `json:"max_file_size"`  // Max file size in bytes before rotation
	RetentionDays int    `json:"retention_days"` // Days to keep recordings
}

// APIConfig defines REST API settings
type APIConfig struct {
	Enabled         bool   `json:"enabled"`
	Address         string `json:"address"` // Listen address (e.g., ":8080")
	AuthEnabled     bool   `json:"auth_enabled"`
	RateLimitPerMin int    `json:"rate_limit_per_min"`
	CORSEnabled     bool   `json:"cors_enabled"`
	CORSOrigins     string `json:"cors_origins"`
	TLSEnabled      bool   `json:"tls_enabled"`
	TLSCert         string `json:"tls_cert"`
	TLSKey          string `json:"tls_key"`
}

// SessionConfig defines session management settings
type SessionConfig struct {
	MaxSessions   int `json:"max_sessions"`
	SessionTTL    int `json:"session_ttl"`     // Session TTL in seconds
	CleanupInterval int `json:"cleanup_interval"` // Cleanup interval in seconds
	MinPort       int `json:"min_port"`        // Minimum RTP port
	MaxPort       int `json:"max_port"`        // Maximum RTP port
}

// JitterBufferConfig defines jitter buffer settings
type JitterBufferConfig struct {
	Enabled      bool `json:"enabled"`
	MinDelay     int  `json:"min_delay"`     // Minimum delay in ms
	MaxDelay     int  `json:"max_delay"`     // Maximum delay in ms
	TargetDelay  int  `json:"target_delay"`  // Target delay in ms
	AdaptiveMode bool `json:"adaptive_mode"` // Enable adaptive jitter buffer
	MaxSize      int  `json:"max_size"`      // Maximum buffer size in packets
}

// RTCPConfig defines RTCP settings
type RTCPConfig struct {
	Enabled     bool `json:"enabled"`
	Interval    int  `json:"interval"`     // Report interval in seconds
	ReducedSize bool `json:"reduced_size"` // Use reduced-size RTCP
	MuxEnabled  bool `json:"mux_enabled"`  // RTCP-mux support
}

// FECConfig defines Forward Error Correction settings
type FECConfig struct {
	Enabled       bool    `json:"enabled"`
	BlockSize     int     `json:"block_size"`     // Packets per FEC block
	Redundancy    float64 `json:"redundancy"`     // Redundancy ratio (0.0-1.0)
	AdaptiveMode  bool    `json:"adaptive_mode"`  // Adjust based on loss rate
	MaxRedundancy float64 `json:"max_redundancy"` // Maximum redundancy
	MinRedundancy float64 `json:"min_redundancy"` // Minimum redundancy
}

// Config struct holds all settings
type Config struct {
	Version       string              `json:"version"`
	LastUpdated   time.Time           `json:"last_updated"`
	Environment   string              `json:"environment"` // prod, staging, dev
	Transport     TransportConfig     `json:"transport"`
	RTPSettings   RTPSettings         `json:"rtp_settings"`
	WebRTC        WebRTCConfig        `json:"webrtc"`
	Integration   IntegrationConfig   `json:"integration"`
	AlertSettings AlertSettings       `json:"alert_settings"`
	Database      DatabaseConfig      `json:"database"`
	SRTP          SRTPConfig          `json:"srtp"`
	NGProtocol    *NGProtocolConfig   `json:"ng_protocol"`
	Recording     *RecordingConfig    `json:"recording"`
	API           *APIConfig          `json:"api"`
	Sessions      *SessionConfig      `json:"sessions"`
	JitterBuffer  *JitterBufferConfig `json:"jitter_buffer"`
	RTCP          *RTCPConfig         `json:"rtcp"`
	FEC           *FECConfig          `json:"fec"`
}

// GetNGProtocolConfig returns NG protocol config with defaults
func (c *Config) GetNGProtocolConfig() *NGProtocolConfig {
	if c.NGProtocol == nil {
		return &NGProtocolConfig{
			Enabled:    true,
			SocketPath: "/var/run/karl/karl.sock",
			Timeout:    30,
		}
	}
	return c.NGProtocol
}

// GetRecordingConfig returns recording config with defaults
func (c *Config) GetRecordingConfig() *RecordingConfig {
	if c.Recording == nil {
		return &RecordingConfig{
			Enabled:       false,
			BasePath:      "/var/lib/karl/recordings",
			Format:        "wav",
			Mode:          "mixed",
			SampleRate:    8000,
			BitsPerSample: 16,
			MaxFileSize:   100 * 1024 * 1024, // 100MB
			RetentionDays: 30,
		}
	}
	return c.Recording
}

// GetAPIConfig returns API config with defaults
func (c *Config) GetAPIConfig() *APIConfig {
	if c.API == nil {
		return &APIConfig{
			Enabled:         true,
			Address:         ":8080",
			AuthEnabled:     false,
			RateLimitPerMin: 60,
		}
	}
	return c.API
}

// GetSessionConfig returns session config with defaults
func (c *Config) GetSessionConfig() *SessionConfig {
	if c.Sessions == nil {
		return &SessionConfig{
			MaxSessions:     10000,
			SessionTTL:      3600,
			CleanupInterval: 60,
			MinPort:         30000,
			MaxPort:         40000,
		}
	}
	return c.Sessions
}

// GetJitterBufferConfig returns jitter buffer config with defaults
func (c *Config) GetJitterBufferConfig() *JitterBufferConfig {
	if c.JitterBuffer == nil {
		return &JitterBufferConfig{
			Enabled:      true,
			MinDelay:     20,
			MaxDelay:     200,
			TargetDelay:  50,
			AdaptiveMode: true,
			MaxSize:      100,
		}
	}
	return c.JitterBuffer
}

// GetRTCPConfig returns RTCP config with defaults
func (c *Config) GetRTCPConfig() *RTCPConfig {
	if c.RTCP == nil {
		return &RTCPConfig{
			Enabled:     true,
			Interval:    5,
			ReducedSize: false,
			MuxEnabled:  true,
		}
	}
	return c.RTCP
}

// GetFECConfig returns FEC config with defaults
func (c *Config) GetFECConfig() *FECConfig {
	if c.FEC == nil {
		return &FECConfig{
			Enabled:       true,
			BlockSize:     48,
			Redundancy:    0.30,
			AdaptiveMode:  true,
			MaxRedundancy: 0.50,
			MinRedundancy: 0.10,
		}
	}
	return c.FEC
}
