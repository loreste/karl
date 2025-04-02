package internal

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// SIPRegistrationSettings defines the configuration for SIP registration
type SIPRegistrationSettings struct {
	ProxyIP         string
	ProxyPort       int
	Interval        time.Duration
	RetryCount      int
	RetryBackoff    time.Duration
	KeepAliveEnable bool
}

var (
	// Default settings
	defaultSIPSettings = SIPRegistrationSettings{
		RetryCount:      5,
		RetryBackoff:    time.Second * 2,
		Interval:        time.Second * 30,
		KeepAliveEnable: true,
	}

	// Track registration status
	registrationStatus     map[string]bool
	registrationStatusLock sync.RWMutex
)

func init() {
	registrationStatus = make(map[string]bool)
}

// IsRegisteredWithSIPProxy checks if we're registered with a specific proxy
func IsRegisteredWithSIPProxy(proxyAddr string) bool {
	registrationStatusLock.RLock()
	defer registrationStatusLock.RUnlock()
	return registrationStatus[proxyAddr]
}

// RegisterWithSIPProxy registers Karl as an RTP media relay with OpenSIPS/Kamailio
func RegisterWithSIPProxy(proxyIP string, proxyPort int) error {
	proxyAddr := fmt.Sprintf("%s:%d", proxyIP, proxyPort)

	// Create a UDP connection to the SIP proxy with timeout
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.Dial("udp", proxyAddr)
	if err != nil {
		// Update status
		registrationStatusLock.Lock()
		registrationStatus[proxyAddr] = false
		registrationStatusLock.Unlock()
		
		return fmt.Errorf("failed to connect to SIP proxy %s: %w", proxyAddr, err)
	}
	defer conn.Close()

	// Set read/write timeouts
	deadline := time.Now().Add(5 * time.Second)
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetDeadline(deadline)
	} else if udpConn, ok := conn.(*net.UDPConn); ok {
		udpConn.SetDeadline(deadline)
	}

	// Send a registration message with host information
	localIP := GetLocalIPAddress()
	hostname, _ := net.LookupAddr(localIP)
	registrationMessage := fmt.Sprintf("REGISTER Karl RTP Engine %s", hostname)
	_, err = conn.Write([]byte(registrationMessage))
	if err != nil {
		// Update status
		registrationStatusLock.Lock()
		registrationStatus[proxyAddr] = false
		registrationStatusLock.Unlock()
		
		return fmt.Errorf("failed to send registration to SIP proxy: %w", err)
	}

	// Read response (to confirm registration)
	buffer := make([]byte, 1024)
	_, err = conn.Read(buffer)
	if err != nil {
		// Update status
		registrationStatusLock.Lock()
		registrationStatus[proxyAddr] = false
		registrationStatusLock.Unlock()
		
		return fmt.Errorf("failed to receive response from SIP proxy: %w", err)
	}

	// Update status
	registrationStatusLock.Lock()
	registrationStatus[proxyAddr] = true
	registrationStatusLock.Unlock()

	log.Printf("Successfully registered Karl with SIP proxy at %s", proxyAddr)
	return nil
}

// PeriodicallyRegisterWithSIPProxy ensures Karl remains registered with OpenSIPS/Kamailio
// with retries and exponential backoff
func PeriodicallyRegisterWithSIPProxy(proxyIP string, proxyPort int, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	settings := defaultSIPSettings
	settings.ProxyIP = proxyIP
	settings.ProxyPort = proxyPort
	settings.Interval = interval

	// Initial registration attempt
	registerWithRetries(settings)

	// Periodic registration
	for range ticker.C {
		registerWithRetries(settings)
	}
}

// StartRegistrationService starts the SIP registration service with context for shutdown
func StartRegistrationService(ctx context.Context, proxyIP string, proxyPort int, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	settings := defaultSIPSettings
	settings.ProxyIP = proxyIP
	settings.ProxyPort = proxyPort
	settings.Interval = interval

	// Initial registration attempt
	registerWithRetries(settings)

	// Periodic registration with cancellation support
	for {
		select {
		case <-ticker.C:
			registerWithRetries(settings)
		case <-ctx.Done():
			log.Println("SIP registration service shutting down")
			return
		}
	}
}

// registerWithRetries attempts to register with exponential backoff
func registerWithRetries(settings SIPRegistrationSettings) {
	backoff := settings.RetryBackoff

	// First attempt
	err := RegisterWithSIPProxy(settings.ProxyIP, settings.ProxyPort)
	if err == nil {
		return
	}

	log.Printf("Initial registration with SIP proxy failed: %v, will retry", err)

	// Retry with exponential backoff
	for i := 0; i < settings.RetryCount; i++ {
		time.Sleep(backoff)
		err = RegisterWithSIPProxy(settings.ProxyIP, settings.ProxyPort)
		if err == nil {
			return
		}

		log.Printf("Retry %d/%d failed: %v", i+1, settings.RetryCount, err)
		backoff *= 2 // Exponential backoff
	}

	log.Printf("Failed to register with SIP proxy %s:%d after %d retries",
		settings.ProxyIP, settings.ProxyPort, settings.RetryCount)
}

// GetLocalIPAddress returns the non-loopback local IP of the host
func GetLocalIPAddress() string {
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