package internal

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"net"
	"sync"
	"time"
	
	"github.com/prometheus/client_golang/prometheus"
)

// FECConfig holds Forward Error Correction settings
type FECConfig struct {
	enabled     bool
	blockSize   int
	redundancy  float32
	mu          sync.RWMutex
	blockBuffer [][]byte
}

// RTCPConfig holds Real-time Transport Control Protocol settings
type RTCPConfig struct {
	interval   time.Duration
	mu         sync.RWMutex
	lastReport time.Time
	reportChan chan struct{}
}

// FailoverConfig holds media server failover settings
type FailoverConfig struct {
	enabled      bool
	primaryIP    string
	backupIP     string
	mu           sync.RWMutex
	activeIP     string
	healthChecks map[string]bool
}

var (
	fecConfig      *FECConfig
	rtcpConfig     *RTCPConfig
	failoverConfig *FailoverConfig
	
	// Prometheus metrics
	fecSentTotal   prometheus.Counter
	rtcpSentTotal  prometheus.Counter
)

// initializeFEC sets up Forward Error Correction
func initializeFEC() error {
	log.Println("🛠️ Initializing Forward Error Correction...")

	// Initialize FEC config
	fecConfig = &FECConfig{
		enabled:     true,
		blockSize:   48,  // Standard FEC block size for audio
		redundancy:  0.3, // 30% redundancy
		blockBuffer: make([][]byte, 0),
	}
	
	// Initialize FEC metrics
	fecSentTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "karl",
		Subsystem: "fec",
		Name:      "packets_sent_total",
		Help:      "Total number of FEC packets sent",
	})
	
	// Register metrics with Prometheus
	prometheus.MustRegister(fecSentTotal)

	// Start FEC processing goroutine
	go processFECBlocks()

	log.Println("✅ FEC initialized successfully")
	return nil
}

// processFECBlocks handles FEC packet generation
func processFECBlocks() {
	for {
		fecConfig.mu.Lock()
		if len(fecConfig.blockBuffer) >= fecConfig.blockSize {
			// Generate FEC packet
			fecPacket := generateFECPacket(fecConfig.blockBuffer)
			// Send FEC packet
			sendFECPacket(fecPacket)
			// Clear buffer
			fecConfig.blockBuffer = fecConfig.blockBuffer[:0]
		}
		fecConfig.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
}

// generateFECPacket creates a FEC packet from a block of RTP packets
func generateFECPacket(block [][]byte) []byte {
	if len(block) == 0 {
		return nil
	}

	// XOR all packets in the block
	fecData := make([]byte, len(block[0]))
	copy(fecData, block[0])

	for i := 1; i < len(block); i++ {
		for j := 0; j < len(fecData) && j < len(block[i]); j++ {
			fecData[j] ^= block[i][j]
		}
	}

	return fecData
}

// sendFECPacket sends the FEC packet to the network
func sendFECPacket(packet []byte) {
	if packet == nil {
		return
	}
	
	// Get the configured FEC destination from config
	configMutex.RLock()
	fecDest := config.RTPSettings.FECDestination
	fecPort := config.RTPSettings.FECPort
	configMutex.RUnlock()
	
	if fecDest == "" || fecPort == 0 {
		// FEC not configured, can't send
		return
	}
	
	// Create a UDP connection to the FEC destination
	addr := fmt.Sprintf("%s:%d", fecDest, fecPort)
	conn, err := net.Dial("udp", addr)
	if err != nil {
		log.Printf("❌ Failed to create FEC packet connection: %v", err)
		return
	}
	defer conn.Close()
	
	// Send the packet
	_, err = conn.Write(packet)
	if err != nil {
		log.Printf("❌ Failed to send FEC packet: %v", err)
		return
	}
	
	// Update metrics
	if fecSentTotal != nil {
		fecSentTotal.Inc()
	}
}

// updateRTCPInterval updates the RTCP reporting interval
func updateRTCPInterval(interval int) {
	log.Printf("🔄 Updating RTCP interval to %d seconds", interval)

	if rtcpConfig == nil {
		rtcpConfig = &RTCPConfig{
			reportChan: make(chan struct{}),
		}
		
		// Initialize RTCP metrics if not already done
		if rtcpSentTotal == nil {
			rtcpSentTotal = prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: "karl",
				Subsystem: "rtcp",
				Name:      "packets_sent_total",
				Help:      "Total number of RTCP packets sent",
			})
			
			// Register metrics with Prometheus
			prometheus.MustRegister(rtcpSentTotal)
		}
	}

	rtcpConfig.mu.Lock()
	rtcpConfig.interval = time.Duration(interval) * time.Second
	rtcpConfig.mu.Unlock()

	// Start RTCP reporter if not already running
	go rtcpReporter()
}

// rtcpReporter sends RTCP reports at the configured interval
func rtcpReporter() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C

		rtcpConfig.mu.RLock()
		interval := rtcpConfig.interval
		lastReport := rtcpConfig.lastReport
		rtcpConfig.mu.RUnlock()

		if time.Since(lastReport) >= interval {
			sendRTCPReport()

			rtcpConfig.mu.Lock()
			rtcpConfig.lastReport = time.Now()
			rtcpConfig.mu.Unlock()
		}
	}
}

// sendRTCPReport generates and sends an RTCP report
func sendRTCPReport() {
	// Get current statistics
	stats := collectRTCPStats()

	// Create RTCP packet
	rtcpPacket := createRTCPPacket(stats)
	
	// Get destination from active sessions
	destinations := getActiveRTCPDestinations()
	if len(destinations) == 0 {
		// No active destinations, just log that we would have sent a report
		log.Printf("📊 RTCP Report ready but no active destinations - Packet Loss: %.2f%%, Jitter: %.2fms",
			stats.packetLoss, stats.jitter)
		return
	}
	
	// Send to all active destinations
	for _, dest := range destinations {
		if err := sendRTCPPacket(rtcpPacket, dest); err != nil {
			log.Printf("❌ Failed to send RTCP packet to %s: %v", dest, err)
		} else {
			// Update metrics
			if rtcpSentTotal != nil {
				rtcpSentTotal.Inc()
			}
			log.Printf("📊 Sent RTCP Report to %s - Packet Loss: %.2f%%, Jitter: %.2fms",
				dest, stats.packetLoss, stats.jitter)
		}
	}
}

// RTCPStats holds statistics for RTCP reporting
type RTCPStats struct {
	packetLoss float64
	jitter     float64
	rtt        float64
}

// We'll use WorkerMetricsGetter from health.go instead of declaring it here

// createRTCPPacket creates an RTCP packet from the statistics
func createRTCPPacket(stats RTCPStats) []byte {
	// Create a simple RTCP Sender Report (SR) packet
	// RFC 3550 defines the format of RTCP packets
	
	// RTCP header (8 bytes)
	// V=2, P=0, RC=0, PT=200 (SR), length=7 (32-bit words - 1)
	header := []byte{0x80, 0xc8, 0x00, 0x07}
	
	// Sender SSRC (4 bytes) - Get from current session or use dummy value for now
	ssrc := []byte{0x12, 0x34, 0x56, 0x78}
	
	// NTP timestamp (8 bytes)
	// Use current time in NTP format (RFC 5905)
	ntpTime := make([]byte, 8)
	now := time.Now()
	seconds := uint32(now.Unix() + 2208988800) // Seconds since Jan 1, 1900
	fraction := uint32(float64(now.Nanosecond()) * math.Pow(2, 32) / 1e9)
	binary.BigEndian.PutUint32(ntpTime[:4], seconds)
	binary.BigEndian.PutUint32(ntpTime[4:], fraction)
	
	// RTP timestamp (4 bytes) - Convert from NTP time
	rtpTs := make([]byte, 4)
	// 90kHz clock rate is common for video
	rtpTimestamp := uint32(seconds * 90000)
	binary.BigEndian.PutUint32(rtpTs, rtpTimestamp)
	
	// Packet and octet counts (8 bytes)
	packetCount := make([]byte, 4)
	octetCount := make([]byte, 4)
	// Get from statistics
	binary.BigEndian.PutUint32(packetCount, 1000)  // Example value
	binary.BigEndian.PutUint32(octetCount, 160000) // Example value
	
	// Concatenate all parts
	rtcpPacket := append(header, ssrc...)
	rtcpPacket = append(rtcpPacket, ntpTime...)
	rtcpPacket = append(rtcpPacket, rtpTs...)
	rtcpPacket = append(rtcpPacket, packetCount...)
	rtcpPacket = append(rtcpPacket, octetCount...)
	
	// Add Reception Report Block if needed (not included in this simple implementation)
	
	return rtcpPacket
}

// getActiveRTCPDestinations returns a list of active RTCP destinations
func getActiveRTCPDestinations() []string {
	// In a real implementation, get active RTP sessions with RTCP enabled
	// For now, return configured endpoints or use a hardcoded demo value
	
	configMutex.RLock()
	defer configMutex.RUnlock()
	
	destinations := make([]string, 0)
	
	// Check if we have active RTP sessions with SIP proxies
	if config != nil {
		if config.Integration.OpenSIPSIp != "" && config.Integration.OpenSIPSPort > 0 {
			rtcpPort := config.Integration.OpenSIPSPort + 1 // RTCP is usually RTP port + 1
			destinations = append(destinations, fmt.Sprintf("%s:%d", 
				config.Integration.OpenSIPSIp, rtcpPort))
		}
		
		if config.Integration.KamailioIp != "" && config.Integration.KamailioPort > 0 {
			rtcpPort := config.Integration.KamailioPort + 1
			destinations = append(destinations, fmt.Sprintf("%s:%d", 
				config.Integration.KamailioIp, rtcpPort))
		}
	}
	
	return destinations
}

// sendRTCPPacket sends an RTCP packet to the specified destination
func sendRTCPPacket(packet []byte, destination string) error {
	// Create a UDP connection to the destination
	conn, err := net.Dial("udp", destination)
	if err != nil {
		return fmt.Errorf("failed to connect to RTCP destination: %w", err)
	}
	defer conn.Close()
	
	// Send the packet
	_, err = conn.Write(packet)
	if err != nil {
		return fmt.Errorf("failed to send RTCP packet: %w", err)
	}
	
	return nil
}

// collectRTCPStats gathers statistics for RTCP reporting
func collectRTCPStats() RTCPStats {
	// Get actual statistics from the worker pool metrics and other sources
	workerMetrics := WorkerMetricsGetter()
	
	// Calculate packet loss percentage
	packetsProcessed := float64(workerMetrics["packets_processed"])
	packetsDropped := float64(workerMetrics["packet_errors"])
	
	var packetLoss float64
	if packetsProcessed > 0 {
		packetLoss = (packetsDropped / packetsProcessed) * 100.0
	} else {
		packetLoss = 0.0
	}
	
	// Get jitter from RTP sessions (simplified for now)
	// In a real implementation, this would come from actual RTP session measurements
	jitter := 0.0
	
	// Round-trip time
	// This would come from RTCP Receiver Reports in a real implementation
	rtt := 0.0
	
	return RTCPStats{
		packetLoss: packetLoss,
		jitter:     jitter,
		rtt:        rtt,
	}
}

// setupFailover initializes media server failover
func setupFailover(primaryIP, backupIP string) error {
	log.Printf("🔄 Setting up failover: Primary IP: %s, Backup IP: %s", primaryIP, backupIP)

	if failoverConfig == nil {
		failoverConfig = &FailoverConfig{
			enabled:      true,
			primaryIP:    primaryIP,
			backupIP:     backupIP,
			activeIP:     primaryIP,
			healthChecks: make(map[string]bool),
		}
	}

	// Validate IP addresses
	if net.ParseIP(primaryIP) == nil || net.ParseIP(backupIP) == nil {
		return fmt.Errorf("invalid IP address configuration")
	}

	// Start health checking
	go monitorMediaServers()

	log.Println("✅ Failover setup completed")
	return nil
}

// monitorMediaServers continuously checks the health of media servers
func monitorMediaServers() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C

		failoverConfig.mu.Lock()
		primaryHealth := checkServerHealth(failoverConfig.primaryIP)
		backupHealth := checkServerHealth(failoverConfig.backupIP)

		failoverConfig.healthChecks[failoverConfig.primaryIP] = primaryHealth
		failoverConfig.healthChecks[failoverConfig.backupIP] = backupHealth

		// Handle failover logic
		if failoverConfig.activeIP == failoverConfig.primaryIP && !primaryHealth && backupHealth {
			// Failover to backup
			log.Printf("⚠️ Primary server down, failing over to backup: %s", failoverConfig.backupIP)
			failoverConfig.activeIP = failoverConfig.backupIP
			handleFailover(failoverConfig.backupIP)
		} else if failoverConfig.activeIP == failoverConfig.backupIP && primaryHealth {
			// Fail back to primary
			log.Printf("✅ Primary server restored, failing back to: %s", failoverConfig.primaryIP)
			failoverConfig.activeIP = failoverConfig.primaryIP
			handleFailover(failoverConfig.primaryIP)
		}

		failoverConfig.mu.Unlock()
	}
}

// checkServerHealth verifies if a media server is responding
func checkServerHealth(ip string) bool {
	// Send a ping packet
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:5060", ip), time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

// handleFailover performs the actual failover process
func handleFailover(newIP string) {
	// Update RTP destinations
	updateRTPDestinations(newIP)

	// Update SIP registrations
	updateSIPRegistrations(newIP)

	// Notify monitoring systems
	notifyFailover(newIP)
}

// updateRTPDestinations updates all RTP streams to use the new IP
func updateRTPDestinations(newIP string) {
	// Implementation depends on your RTP handling mechanism
	log.Printf("🔄 Updating RTP destinations to %s", newIP)
}

// updateSIPRegistrations updates SIP registrations with the new IP
func updateSIPRegistrations(newIP string) {
	// Implementation depends on your SIP integration
	log.Printf("🔄 Updating SIP registrations to %s", newIP)
}

// notifyFailover sends notifications about the failover event
func notifyFailover(newIP string) {
	log.Printf("📢 Failover notification: Switched to %s", newIP)
	// Implement your notification system here (e.g., email, Slack, monitoring systems)
}
