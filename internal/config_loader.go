package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	config      *Config
	configMutex sync.RWMutex
)

// LoadConfig reads and validates the configuration
func LoadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var newConfig Config
	if err := json.Unmarshal(data, &newConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	newConfig.LastUpdated = time.Now()
	if newConfig.Version == "" {
		newConfig.Version = ConfigVersion
	}

	if err := ValidateConfig(&newConfig); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Apply environment variable overrides
	ApplyEnvironmentOverrides(&newConfig)

	if newConfig.Integration.PublicIP == "" {
		detectedIP, err := GetPublicIP()
		if err != nil {
			log.Println("⚠️ Failed to detect public IP:", err)

			// Fallback to local IP if public IP detection fails
			localIP := GetLocalIP()
			if localIP != "" {
				newConfig.Integration.PublicIP = localIP
				log.Println("🌍 Using local IP as fallback:", localIP)
			}
		} else {
			newConfig.Integration.PublicIP = detectedIP
			log.Println("🌍 Auto-detected public IP:", detectedIP)
		}
	}

	return &newConfig, nil
}

// ValidateConfig performs comprehensive configuration validation
func ValidateConfig(cfg *Config) error {
	if cfg.Version == "" {
		cfg.Version = ConfigVersion
	}

	if cfg.Transport.UDPEnabled && (cfg.Transport.UDPPort < 1024 || cfg.Transport.UDPPort > 65535) {
		return fmt.Errorf("invalid UDP port: %d", cfg.Transport.UDPPort)
	}

	if cfg.Transport.TLSEnabled {
		if _, err := os.Stat(cfg.Transport.TLSCert); err != nil {
			return fmt.Errorf("TLS cert file not found: %s", cfg.Transport.TLSCert)
		}
		if _, err := os.Stat(cfg.Transport.TLSKey); err != nil {
			return fmt.Errorf("TLS key file not found: %s", cfg.Transport.TLSKey)
		}
	}

	if cfg.RTPSettings.MinJitterBuffer < MinJitterBuffer || cfg.RTPSettings.MinJitterBuffer > MaxJitterBuffer {
		return fmt.Errorf("invalid jitter buffer size: %d", cfg.RTPSettings.MinJitterBuffer)
	}

	if cfg.RTPSettings.MaxBandwidth < MinBandwidth || cfg.RTPSettings.MaxBandwidth > MaxBandwidth {
		return fmt.Errorf("invalid bandwidth: %d", cfg.RTPSettings.MaxBandwidth)
	}

	if cfg.WebRTC.Enabled {
		// Skip strict STUN server validation for now
		// STUN servers are specified as URIs, not raw IP:port
		log.Println("WebRTC enabled with STUN servers:", cfg.WebRTC.StunServers)
	}

	if cfg.Database.RedisEnabled && cfg.Database.RedisAddr == "" {
		return fmt.Errorf("Redis enabled but address not specified")
	}

	return nil
}

// WatchConfig monitors for configuration changes
func WatchConfig(filePath string) error {
	lastMod := time.Now()

	for {
		time.Sleep(5 * time.Second)

		info, err := os.Stat(filePath)
		if err != nil {
			log.Printf("❌ Error checking config file: %v", err)
			continue
		}

		if info.ModTime().After(lastMod) {
			log.Println("📝 Configuration file changed, reloading...")

			newConfig, err := LoadConfig(filePath)
			if err != nil {
				log.Printf("❌ Failed to reload config: %v", err)
				continue
			}

			configMutex.Lock()
			config = newConfig
			configMutex.Unlock()

			if err := ApplyNewConfig(*newConfig); err != nil {
				log.Printf("❌ Failed to apply new config: %v", err)
				continue
			}

			lastMod = info.ModTime()
			log.Println("✅ Configuration updated successfully")
		}
	}
}

// ApplyNewConfig applies the configuration dynamically
func ApplyNewConfig(newConfig Config) error {
	log.Println("⚙️ Applying new configurations dynamically...")

	if err := updateTransportSettings(newConfig.Transport); err != nil {
		return fmt.Errorf("failed to update transport settings: %w", err)
	}

	if err := updateWebRTCSettings(newConfig.WebRTC); err != nil {
		return fmt.Errorf("failed to update WebRTC settings: %w", err)
	}

	if err := updateRTPSettings(newConfig.RTPSettings); err != nil {
		return fmt.Errorf("failed to update RTP settings: %w", err)
	}

	if err := updateIntegrationSettings(newConfig.Integration); err != nil {
		return fmt.Errorf("failed to update integration settings: %w", err)
	}

	UpdateAlertThresholds(newConfig.AlertSettings)

	log.Println("✅ Configuration applied successfully")
	return nil
}

// Dynamic configuration update functions
//
// Note: These functions are intentionally simplified because the RTP/transport
// listeners in rtp_transport.go are blocking functions designed to run for the
// lifetime of the server. Full dynamic reconfiguration would require:
// 1. A listener manager that tracks running listeners
// 2. Stopping existing listeners gracefully
// 3. Starting new listeners in goroutines
//
// For now, transport settings changes require a server restart to take effect.
// The functions below log the changes for monitoring but don't restart listeners.
func updateTransportSettings(transport TransportConfig) error {
	log.Printf("Transport settings updated (UDP: %v port %d, TCP: %v port %d, TLS: %v port %d)",
		transport.UDPEnabled, transport.UDPPort,
		transport.TCPEnabled, transport.TCPPort,
		transport.TLSEnabled, transport.TLSPort)

	// Note: Actual transport listener changes require server restart.
	// The listeners (StartRTPUDPListener, etc.) are blocking functions
	// that run for the lifetime of the server.

	return nil
}

func updateWebRTCSettings(webrtc WebRTCConfig) error {
	if !webrtc.Enabled {
		log.Printf("WebRTC settings: disabled")
		return nil
	}

	log.Printf("WebRTC settings updated (ICE servers: %d, recording: %v)",
		len(webrtc.StunServers), webrtc.RecordingEnabled)

	// Ensure recording directory exists if recording is enabled
	if webrtc.RecordingEnabled && webrtc.RecordingPath != "" {
		if err := os.MkdirAll(webrtc.RecordingPath, 0755); err != nil {
			return fmt.Errorf("failed to create recording directory: %w", err)
		}
		log.Printf("WebRTC recording directory: %s", webrtc.RecordingPath)
	}

	return nil
}

func updateRTPSettings(settings RTPSettings) error {
	log.Printf("RTP settings updated (jitter buffer: %dms, bandwidth: %d, FEC: %v, PCAP: %v, RTCP interval: %d)",
		settings.MinJitterBuffer,
		settings.MaxBandwidth, settings.FECEnabled,
		settings.EnablePCAP, settings.RTCPInterval)

	// These settings are applied to new sessions automatically.
	// Existing sessions continue with their original settings.
	// PCAP capture and FEC are initialized at server startup.

	return nil
}

func updateIntegrationSettings(integration IntegrationConfig) error {
	log.Printf("Integration settings updated (Media IP: %s, Public IP: %s)",
		integration.MediaIP, integration.PublicIP)

	// Re-register with SIP proxies if configured
	if integration.OpenSIPSIp != "" && integration.OpenSIPSPort > 0 {
		if err := RegisterWithSIPProxy(integration.OpenSIPSIp, integration.OpenSIPSPort); err != nil {
			log.Printf("Failed to register with OpenSIPS at %s:%d: %v",
				integration.OpenSIPSIp, integration.OpenSIPSPort, err)
		} else {
			log.Printf("Registered with OpenSIPS at %s:%d", integration.OpenSIPSIp, integration.OpenSIPSPort)
		}
	}

	if integration.KamailioIp != "" && integration.KamailioPort > 0 {
		if err := RegisterWithSIPProxy(integration.KamailioIp, integration.KamailioPort); err != nil {
			log.Printf("Failed to register with Kamailio at %s:%d: %v",
				integration.KamailioIp, integration.KamailioPort, err)
		} else {
			log.Printf("Registered with Kamailio at %s:%d", integration.KamailioIp, integration.KamailioPort)
		}
	}

	// Log failover configuration
	if integration.FailoverEnabled && integration.BackupMediaIP != "" {
		log.Printf("Failover configured: primary %s, backup %s",
			integration.MediaIP, integration.BackupMediaIP)
	}

	return nil
}

// GetPublicIP retrieves the system's public IP
func GetPublicIP() (string, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get("https://api64.ipify.org")
	if err != nil {
		return "", fmt.Errorf("failed to get public IP: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	ip := string(body)
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("invalid IP address received: %s", ip)
	}

	return ip, nil
}

// GetLocalIP returns the non-loopback local IP of the host
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}

	for _, address := range addrs {
		// Check the address type and if it's not a loopback
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}

	return ""
}

// ApplyEnvironmentOverrides applies environment variable overrides to the config
// Environment variables take precedence over config file values
func ApplyEnvironmentOverrides(cfg *Config) {
	// NG Protocol settings
	if port := os.Getenv("KARL_NG_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.NGProtocol.UDPPort = p
			log.Printf("NG Protocol port overridden by KARL_NG_PORT: %d", p)
		}
	}

	// Session settings
	if minPort := os.Getenv("KARL_RTP_MIN_PORT"); minPort != "" {
		if p, err := strconv.Atoi(minPort); err == nil {
			cfg.Sessions.MinPort = p
			log.Printf("RTP min port overridden by KARL_RTP_MIN_PORT: %d", p)
		}
	}
	if maxPort := os.Getenv("KARL_RTP_MAX_PORT"); maxPort != "" {
		if p, err := strconv.Atoi(maxPort); err == nil {
			cfg.Sessions.MaxPort = p
			log.Printf("RTP max port overridden by KARL_RTP_MAX_PORT: %d", p)
		}
	}
	if maxSessions := os.Getenv("KARL_MAX_SESSIONS"); maxSessions != "" {
		if s, err := strconv.Atoi(maxSessions); err == nil {
			cfg.Sessions.MaxSessions = s
			log.Printf("Max sessions overridden by KARL_MAX_SESSIONS: %d", s)
		}
	}

	// Recording settings
	if recordingPath := os.Getenv("KARL_RECORDING_PATH"); recordingPath != "" {
		cfg.Recording.BasePath = recordingPath
		log.Printf("Recording path overridden by KARL_RECORDING_PATH: %s", recordingPath)
	}
	if recordingEnabled := os.Getenv("KARL_RECORDING_ENABLED"); recordingEnabled != "" {
		cfg.Recording.Enabled = recordingEnabled == "true" || recordingEnabled == "1"
		log.Printf("Recording enabled overridden by KARL_RECORDING_ENABLED: %v", cfg.Recording.Enabled)
	}

	// Database settings
	if mysqlDSN := os.Getenv("KARL_MYSQL_DSN"); mysqlDSN != "" {
		cfg.Database.MySQLDSN = mysqlDSN
		log.Printf("MySQL DSN overridden by KARL_MYSQL_DSN")
	}
	if redisAddr := os.Getenv("KARL_REDIS_ADDR"); redisAddr != "" {
		cfg.Database.RedisAddr = redisAddr
		log.Printf("Redis address overridden by KARL_REDIS_ADDR: %s", redisAddr)
	}
	if redisEnabled := os.Getenv("KARL_REDIS_ENABLED"); redisEnabled != "" {
		cfg.Database.RedisEnabled = redisEnabled == "true" || redisEnabled == "1"
		log.Printf("Redis enabled overridden by KARL_REDIS_ENABLED: %v", cfg.Database.RedisEnabled)
	}

	// Integration settings
	if mediaIP := os.Getenv("KARL_MEDIA_IP"); mediaIP != "" {
		cfg.Integration.MediaIP = mediaIP
		log.Printf("Media IP overridden by KARL_MEDIA_IP: %s", mediaIP)
	}
	if publicIP := os.Getenv("KARL_PUBLIC_IP"); publicIP != "" {
		cfg.Integration.PublicIP = publicIP
		log.Printf("Public IP overridden by KARL_PUBLIC_IP: %s", publicIP)
	}

	// API settings
	if apiEnabled := os.Getenv("KARL_API_ENABLED"); apiEnabled != "" {
		cfg.API.Enabled = apiEnabled == "true" || apiEnabled == "1"
		log.Printf("API enabled overridden by KARL_API_ENABLED: %v", cfg.API.Enabled)
	}
	if apiAuth := os.Getenv("KARL_API_AUTH_ENABLED"); apiAuth != "" {
		cfg.API.AuthEnabled = apiAuth == "true" || apiAuth == "1"
		log.Printf("API auth enabled overridden by KARL_API_AUTH_ENABLED: %v", cfg.API.AuthEnabled)
	}

	// Transport settings
	if udpPort := os.Getenv("KARL_UDP_PORT"); udpPort != "" {
		if p, err := strconv.Atoi(udpPort); err == nil {
			cfg.Transport.UDPPort = p
			log.Printf("UDP port overridden by KARL_UDP_PORT: %d", p)
		}
	}
}

// GetConfigPath returns the config file path from environment or default
func GetConfigPath() string {
	if path := os.Getenv("KARL_CONFIG_PATH"); path != "" {
		return path
	}
	return "config/config.json"
}
