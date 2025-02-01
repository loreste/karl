package internal

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v3"
)

var (
	ErrNoActiveSession = errors.New("no active WebRTC session")
	ErrMaxRetries      = errors.New("maximum reconnection attempts reached")
)

// Stats holds WebRTC performance metrics
type Stats struct {
	Timestamp       time.Time
	ConnectionState string
	CurrentRTT      float64
	JitterMS        float64
	PacketsLost     uint32
	PacketsSent     uint32
	BytesReceived   uint64
	BytesSent       uint64
	AudioLevel      float64
}

// WebRTCStats handles real-time monitoring of WebRTC performance
type WebRTCStats struct {
	peerConnection *webrtc.PeerConnection
	stopChan       chan struct{}
	stopped        atomic.Bool
	statsMutex     sync.RWMutex
	reconnects     atomic.Int32
	lastStats      *Stats
	onStatsUpdate  func(*Stats)
	onReconnect    func()
	config         *StatsConfig
}

// StatsConfig holds configuration for WebRTC stats collection
type StatsConfig struct {
	MonitoringInterval    time.Duration
	MaxReconnectAttempts  int
	BaseReconnectDelay    time.Duration
	EnableDetailedLogging bool
}

// DefaultStatsConfig returns a default configuration
func DefaultStatsConfig() *StatsConfig {
	return &StatsConfig{
		MonitoringInterval:    2 * time.Second,
		MaxReconnectAttempts:  5,
		BaseReconnectDelay:    time.Second,
		EnableDetailedLogging: true,
	}
}

// NewWebRTCStats initializes WebRTC statistics collection
func NewWebRTCStats(peerConnection *webrtc.PeerConnection, config *StatsConfig) *WebRTCStats {
	if config == nil {
		config = DefaultStatsConfig()
	}

	return &WebRTCStats{
		peerConnection: peerConnection,
		stopChan:       make(chan struct{}),
		lastStats:      &Stats{},
		config:         config,
	}
}

// SetStatsCallback sets a callback function for stats updates
func (s *WebRTCStats) SetStatsCallback(callback func(*Stats)) {
	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()
	s.onStatsUpdate = callback
}

// SetReconnectCallback sets a callback function for reconnection events
func (s *WebRTCStats) SetReconnectCallback(callback func()) {
	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()
	s.onReconnect = callback
}

// GetLastStats returns the most recently collected stats
func (s *WebRTCStats) GetLastStats() *Stats {
	s.statsMutex.RLock()
	defer s.statsMutex.RUnlock()
	if s.lastStats == nil {
		return &Stats{}
	}
	statsCopy := *s.lastStats
	return &statsCopy
}

// StartMonitoring begins collecting WebRTC stats
func (s *WebRTCStats) StartMonitoring(ctx context.Context) error {
	if s.peerConnection == nil {
		return ErrNoActiveSession
	}

	if !s.stopped.CompareAndSwap(false, true) {
		return errors.New("monitoring already started")
	}

	go func() {
		ticker := time.NewTicker(s.config.MonitoringInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := s.collectStats(); err != nil {
					if s.config.EnableDetailedLogging {
						log.Printf("❌ Error collecting stats: %v", err)
					}
				}
			case <-ctx.Done():
				log.Println("🛑 Context cancelled, stopping WebRTC stats monitoring")
				return
			case <-s.stopChan:
				log.Println("🛑 Stopping WebRTC stats monitoring")
				return
			}
		}
	}()

	return nil
}

// collectStats fetches and processes WebRTC statistics
func (s *WebRTCStats) collectStats() error {
	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()

	if s.peerConnection == nil {
		return ErrNoActiveSession
	}

	stats := &Stats{
		Timestamp: time.Now(),
	}

	// Get connection state
	stats.ConnectionState = s.peerConnection.ConnectionState().String()

	// Get stats report
	stats.ConnectionState = s.peerConnection.ConnectionState().String()
	report := s.peerConnection.GetStats()

	// Process each stat
	for _, stat := range report {
		switch v := stat.(type) {
		case *webrtc.OutboundRTPStreamStats:
			stats.PacketsSent = v.PacketsSent
			stats.BytesSent = v.BytesSent

		case *webrtc.InboundRTPStreamStats:
			stats.JitterMS = float64(v.Jitter)        // Already in milliseconds
			stats.PacketsLost = uint32(v.PacketsLost) // Convert int32 to uint32
			stats.BytesReceived = v.BytesReceived
		}
	}

	// Check for disconnection
	if s.peerConnection.ConnectionState() == webrtc.PeerConnectionStateDisconnected ||
		s.peerConnection.ConnectionState() == webrtc.PeerConnectionStateFailed {
		if s.config.EnableDetailedLogging {
			log.Println("❌ Connection Failed/Disconnected - Triggering reconnection")
		}
		go s.retryWebRTCConnection()
	}

	s.lastStats = stats

	if s.onStatsUpdate != nil {
		s.onStatsUpdate(stats)
	}

	if s.config.EnableDetailedLogging {
		log.Printf("📊 WebRTC Stats - RTT: %.2fms, Jitter: %.2fms, Lost: %d, Sent: %d",
			stats.CurrentRTT,
			stats.JitterMS,
			stats.PacketsLost,
			stats.PacketsSent)
	}

	return nil
}

// retryWebRTCConnection attempts to re-establish the WebRTC connection
func (s *WebRTCStats) retryWebRTCConnection() {
	for i := 1; i <= s.config.MaxReconnectAttempts; i++ {
		if s.config.EnableDetailedLogging {
			log.Printf("🔄 [Retry %d/%d] Attempting WebRTC reconnection...",
				i, s.config.MaxReconnectAttempts)
		}

		s.statsMutex.Lock()
		if s.peerConnection != nil {
			if err := s.peerConnection.Close(); err != nil {
				log.Printf("⚠️ Error closing peer connection: %v", err)
			}
		}
		s.statsMutex.Unlock()

		time.Sleep(100 * time.Millisecond)

		if s.onReconnect != nil {
			s.onReconnect()
		}

		s.reconnects.Add(1)

		if s.config.EnableDetailedLogging {
			log.Printf("✅ WebRTC reconnection attempt complete. Total reconnects: %d",
				s.reconnects.Load())
		}

		backoff := s.config.BaseReconnectDelay * time.Duration(1<<uint(i-1))
		time.Sleep(backoff)
	}

	log.Println("🚨 Maximum WebRTC reconnection attempts reached")
}

// StopMonitoring stops WebRTC stats collection
func (s *WebRTCStats) StopMonitoring() {
	if s.stopped.CompareAndSwap(true, false) {
		close(s.stopChan)
	}
}

// GetReconnectCount returns the number of reconnection attempts
func (s *WebRTCStats) GetReconnectCount() int32 {
	return s.reconnects.Load()
}

// UpdatePeerConnection updates the monitored peer connection
func (s *WebRTCStats) UpdatePeerConnection(pc *webrtc.PeerConnection) {
	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()
	s.peerConnection = pc
}
