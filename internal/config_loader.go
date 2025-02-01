package internal

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
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
}

// TransportConfig holds networking settings
type TransportConfig struct {
	UDPEnabled bool   `json:"udp_enabled"`
	UDPPort    int    `json:"udp_port"`
	TCPEnabled bool   `json:"tcp_enabled"`
	TCPPort    int    `json:"tcp_port"`
	TLSEnabled bool   `json:"tls_enabled"`
	TLSPort    int    `json:"tls_port"`
	TLSCert    string `json:"tls_cert"`
	TLSKey     string `json:"tls_key"`
}

// RTPSettings defines RTP media handling configurations
type RTPSettings struct {
	MaxBandwidth        int  `json:"max_bandwidth"`
	MinJitterBuffer     int  `json:"min_jitter_buffer"`
	PacketLossThreshold int  `json:"packet_loss_threshold"`
	Encryption          bool `json:"encryption"`
	EnablePCAP          bool `json:"enable_pcap"` // Enable PCAP recording
}

// WebRTCConfig holds WebRTC settings
type WebRTCConfig struct {
	Enabled     bool     `json:"enabled"`
	WebRTCPort  int      `json:"webrtc_port"`
	StunServers []string `json:"stun_servers"`
	TurnServers []struct {
		URL        string `json:"url"`
		Username   string `json:"username"`
		Credential string `json:"credential"`
	} `json:"turn_servers"`
}

// IntegrationConfig defines SIP proxy settings for OpenSIPS/Kamailio
type IntegrationConfig struct {
	OpenSIPSIp      string `json:"opensips_ip"`
	OpenSIPSPort    int    `json:"opensips_port"`
	KamailioIp      string `json:"kamailio_ip"`
	KamailioPort    int    `json:"kamailio_port"`
	RTPengineSocket string `json:"rtpengine_socket"`
	MediaIP         string `json:"media_ip"`
	PublicIP        string `json:"public_ip"`
}

// AlertSettings defines threshold settings for RTP monitoring
type AlertSettings struct {
	PacketLossThreshold float64 `json:"packet_loss_threshold"`
	JitterThreshold     float64 `json:"jitter_threshold"`
	BandwidthThreshold  int     `json:"bandwidth_threshold"`
	NotifyAdmin         bool    `json:"notify_admin"`
	AdminEmail          string  `json:"admin_email"`
}

// Config struct holds all settings for Karl
type Config struct {
	Transport     TransportConfig   `json:"transport"`
	RTPSettings   RTPSettings       `json:"rtp_settings"`
	WebRTC        WebRTCConfig      `json:"webrtc"`
	Integration   IntegrationConfig `json:"integration"`
	AlertSettings AlertSettings     `json:"alert_settings"`
	Database      DatabaseConfig    `json:"database"`
	SRTP          SRTPConfig        `json:"srtp"`
}

// Global configuration variable and mutex for safe concurrent access
var (
	config      *Config
	configMutex sync.RWMutex
)

// LoadConfig reads the configuration file and returns a pointer to a Config struct
func LoadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var newConfig Config
	if err := json.Unmarshal(data, &newConfig); err != nil {
		return nil, err
	}

	// If PublicIP is empty, auto-detect it
	if newConfig.Integration.PublicIP == "" {
		detectedIP, err := GetPublicIP()
		if err != nil {
			log.Println("⚠️ Failed to detect public IP:", err)
		} else {
			newConfig.Integration.PublicIP = detectedIP
			log.Println("🌍 Auto-detected public IP:", detectedIP)
		}
	}

	return &newConfig, nil
}

// WatchConfig monitors `config.json` for changes and applies updates in real-time
func WatchConfig(filePath string) {
	for {
		time.Sleep(5 * time.Second) // Check for changes every 5 seconds

		newConfig, err := LoadConfig(filePath)
		if err != nil {
			log.Println("❌ Failed to reload config:", err)
			continue
		}

		configMutex.Lock()
		config = newConfig
		configMutex.Unlock()

		ApplyNewConfig(*newConfig)

		log.Println("🔄 Configuration updated dynamically.")
	}
}

// ApplyNewConfig applies the configuration dynamically
func ApplyNewConfig(newConfig Config) {
	log.Println("⚙️ Applying new configurations dynamically...")

	// Convert int ports to strings for function compatibility
	udpPort := strconv.Itoa(newConfig.Transport.UDPPort)
	tcpPort := strconv.Itoa(newConfig.Transport.TCPPort)
	tlsPort := strconv.Itoa(newConfig.Transport.TLSPort)

	// Restart RTP Listeners if needed
	if newConfig.Transport.UDPEnabled {
		go StartRTPUDPListener(udpPort)
	} else {
		StopRTPListener(udpPort)
	}

	if newConfig.Transport.TCPEnabled {
		go StartRTPTCPListener(tcpPort)
	} else {
		StopRTPListener(tcpPort)
	}

	if newConfig.Transport.TLSEnabled {
		go StartRTPTLSListener(tlsPort, newConfig.Transport.TLSCert, newConfig.Transport.TLSKey)
	} else {
		StopRTPListener(tlsPort)
	}

	// Update WebRTC Settings
	if newConfig.WebRTC.Enabled {
		go StartWebRTCSession()
	}

	// Register Karl with OpenSIPS/Kamailio dynamically
	go RegisterWithSIPProxy(newConfig.Integration.OpenSIPSIp, newConfig.Integration.OpenSIPSPort)
	go RegisterWithSIPProxy(newConfig.Integration.KamailioIp, newConfig.Integration.KamailioPort)

	// Apply new RTP alerting thresholds dynamically
	UpdateAlertThresholds(newConfig.AlertSettings)

	log.Println("✅ Dynamic configuration applied successfully.")
}

// GetPublicIP retrieves the system's public IP from an external service
func GetPublicIP() (string, error) {
	resp, err := http.Get("https://api64.ipify.org")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
