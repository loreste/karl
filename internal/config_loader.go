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
		for _, server := range cfg.WebRTC.StunServers {
			if _, err := net.ResolveUDPAddr("udp", server); err != nil {
				return fmt.Errorf("invalid STUN server address: %s", server)
			}
		}
	}

	if cfg.Database.RedisEnabled && cfg.Database.RedisAddr == "" {
		return fmt.Errorf("Redis enabled but address not specified")
	}

	return nil
}

// WatchConfig monitors for configuration changes
func WatchConfig(filePath string) {
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

	updateTransportSettings(newConfig.Transport)
	updateWebRTCSettings(newConfig.WebRTC)
	updateRTPSettings(newConfig.RTPSettings)
	updateIntegrationSettings(newConfig.Integration)
	UpdateAlertThresholds(newConfig.AlertSettings)

	log.Println("✅ Configuration applied successfully")
	return nil
}

func updateTransportSettings(transport TransportConfig) {
	if transport.UDPEnabled {
		StartRTPUDPListener(strconv.Itoa(transport.UDPPort))
	} else {
		StopRTPListener(strconv.Itoa(transport.UDPPort))
	}

	if transport.TCPEnabled {
		StartRTPTCPListener(strconv.Itoa(transport.TCPPort))
	} else {
		StopRTPListener(strconv.Itoa(transport.TCPPort))
	}

	if transport.TLSEnabled {
		StartRTPTLSListener(
			strconv.Itoa(transport.TLSPort),
			transport.TLSCert,
			transport.TLSKey,
		)
	} else {
		StopRTPListener(strconv.Itoa(transport.TLSPort))
	}
}

func updateWebRTCSettings(webrtc WebRTCConfig) {
	if !webrtc.Enabled {
		return
	}

	StartWebRTCSession()

	if webrtc.RecordingEnabled {
		os.MkdirAll(webrtc.RecordingPath, 0755)
	}
}

func updateRTPSettings(settings RTPSettings) {
	if settings.EnablePCAP {
		InitPCAPCapture()
	}

	if settings.FECEnabled {
		initializeFEC()
	}

	if settings.RTCPInterval > 0 {
		updateRTCPInterval(settings.RTCPInterval)
	}
}

func updateIntegrationSettings(integration IntegrationConfig) {
	RegisterWithSIPProxy(integration.OpenSIPSIp, integration.OpenSIPSPort)
	RegisterWithSIPProxy(integration.KamailioIp, integration.KamailioPort)

	if integration.FailoverEnabled && integration.BackupMediaIP != "" {
		setupFailover(integration.MediaIP, integration.BackupMediaIP)
	}
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
