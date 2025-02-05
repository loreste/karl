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
	MaxBandwidth        int  `json:"max_bandwidth"`
	MinJitterBuffer     int  `json:"min_jitter_buffer"`
	PacketLossThreshold int  `json:"packet_loss_threshold"`
	Encryption          bool `json:"encryption"`
	EnablePCAP          bool `json:"enable_pcap"`
	DTMFEnabled         bool `json:"dtmf_enabled"`
	FECEnabled          bool `json:"fec_enabled"`   // Forward Error Correction
	REDEnabled          bool `json:"red_enabled"`   // Redundant Encoding
	RTCPInterval        int  `json:"rtcp_interval"` // RTCP report interval in seconds
	VADEnabled          bool `json:"vad_enabled"`   // Voice Activity Detection
	PLIInterval         int  `json:"pli_interval"`  // Picture Loss Indication interval
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

// Config struct holds all settings
type Config struct {
	Version       string            `json:"version"`
	LastUpdated   time.Time         `json:"last_updated"`
	Environment   string            `json:"environment"` // prod, staging, dev
	Transport     TransportConfig   `json:"transport"`
	RTPSettings   RTPSettings       `json:"rtp_settings"`
	WebRTC        WebRTCConfig      `json:"webrtc"`
	Integration   IntegrationConfig `json:"integration"`
	AlertSettings AlertSettings     `json:"alert_settings"`
	Database      DatabaseConfig    `json:"database"`
	SRTP          SRTPConfig        `json:"srtp"`
}
