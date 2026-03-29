package internal

import (
	"encoding/binary"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// RTCP metrics
var (
	rtcpSRSent = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "karl_rtcp_sr_sent_total",
			Help: "Total number of RTCP Sender Reports sent",
		},
	)

	rtcpRRSent = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "karl_rtcp_rr_sent_total",
			Help: "Total number of RTCP Receiver Reports sent",
		},
	)

	rtcpSRRecv = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "karl_rtcp_sr_received_total",
			Help: "Total number of RTCP Sender Reports received",
		},
	)

	rtcpRRRecv = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "karl_rtcp_rr_received_total",
			Help: "Total number of RTCP Receiver Reports received",
		},
	)

	rtcpRTTSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "karl_rtcp_rtt_seconds",
			Help:    "Round-trip time measured via RTCP",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		},
	)

	rtcpJitterSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "karl_rtcp_jitter_seconds",
			Help:    "Interarrival jitter measured via RTCP",
			Buckets: []float64{0.001, 0.005, 0.01, 0.02, 0.05, 0.1, 0.2},
		},
	)

	rtcpPacketLoss = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "karl_rtcp_packet_loss_fraction",
			Help:    "Packet loss fraction from RTCP reports",
			Buckets: []float64{0.001, 0.005, 0.01, 0.02, 0.05, 0.1, 0.2, 0.5},
		},
	)
)

// RTCPInternalConfig holds RTCP runtime configuration with time.Duration types
type RTCPInternalConfig struct {
	Enabled     bool
	Interval    time.Duration
	ReducedSize bool
	MuxEnabled  bool
}

// ToRTCPInternalConfig converts RTCPConfig (int seconds) to RTCPInternalConfig (time.Duration)
func ToRTCPInternalConfig(cfg *RTCPConfig) *RTCPInternalConfig {
	if cfg == nil {
		return &RTCPInternalConfig{
			Enabled:  true,
			Interval: 5 * time.Second,
		}
	}
	return &RTCPInternalConfig{
		Enabled:     cfg.Enabled,
		Interval:    time.Duration(cfg.Interval) * time.Second,
		ReducedSize: cfg.ReducedSize,
		MuxEnabled:  cfg.MuxEnabled,
	}
}

// RTCPSessionHandler handles RTCP for a single session leg
type RTCPSessionHandler struct {
	ssrc          uint32
	cname         string
	conn          *net.UDPConn
	remoteAddr    *net.UDPAddr
	clockRate     uint32

	// Sender state
	packetsSent   uint32
	octetsSent    uint32
	lastSRNTP     uint64
	lastSRTime    time.Time

	// Receiver state
	packetsRecv     uint32
	packetsLost     int32
	lastSeq         uint16
	highestSeq      uint16
	seqCycles       uint32
	jitter          float64
	lastSRRecvNTP   uint64
	lastSRRecvTime  time.Time
	lastArrivalTime time.Time
	lastTimestamp   uint32

	// Calculated metrics
	rtt           time.Duration
	fractionLost  uint8

	mu sync.RWMutex
}

// RTCPHandler manages RTCP for all sessions
type RTCPHandler struct {
	config       *RTCPInternalConfig
	sessions     map[string]*RTCPSessionHandler
	mu           sync.RWMutex
	stopChan     chan struct{}
	wg           sync.WaitGroup
	running      bool
}

// NewRTCPHandler creates a new RTCP handler from internal config
func NewRTCPHandler(config *RTCPInternalConfig) *RTCPHandler {
	if config == nil {
		config = &RTCPInternalConfig{
			Enabled:  true,
			Interval: 5 * time.Second,
		}
	}
	if config.Interval == 0 {
		config.Interval = 5 * time.Second
	}

	return &RTCPHandler{
		config:   config,
		sessions: make(map[string]*RTCPSessionHandler),
		stopChan: make(chan struct{}),
	}
}

// NewRTCPHandlerFromConfig creates a new RTCP handler from external config
func NewRTCPHandlerFromConfig(config *RTCPConfig) *RTCPHandler {
	return NewRTCPHandler(ToRTCPInternalConfig(config))
}

// NewRTCPSessionHandler creates a new RTCP session handler
func NewRTCPSessionHandler(ssrc uint32, cname string, clockRate uint32) *RTCPSessionHandler {
	return &RTCPSessionHandler{
		ssrc:      ssrc,
		cname:     cname,
		clockRate: clockRate,
	}
}

// Start starts the RTCP handler
func (h *RTCPHandler) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.running || !h.config.Enabled {
		return
	}

	h.running = true
	h.wg.Add(1)
	go h.reportLoop()

	log.Printf("RTCP handler started with interval %v", h.config.Interval)
}

// Stop stops the RTCP handler
func (h *RTCPHandler) Stop() {
	h.mu.Lock()
	if !h.running {
		h.mu.Unlock()
		return
	}
	h.running = false
	h.mu.Unlock()

	close(h.stopChan)
	h.wg.Wait()

	log.Println("RTCP handler stopped")
}

// AddSession adds a session to the RTCP handler
func (h *RTCPHandler) AddSession(sessionID string, handler *RTCPSessionHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessions[sessionID] = handler
}

// RemoveSession removes a session from the RTCP handler
func (h *RTCPHandler) RemoveSession(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.sessions, sessionID)
}

// GetSession gets a session handler
func (h *RTCPHandler) GetSession(sessionID string) (*RTCPSessionHandler, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s, ok := h.sessions[sessionID]
	return s, ok
}

// reportLoop sends periodic RTCP reports
func (h *RTCPHandler) reportLoop() {
	defer h.wg.Done()

	// Calculate interval with randomization per RFC 3550
	interval := h.calculateInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopChan:
			return
		case <-ticker.C:
			h.sendReports()
			// Recalculate interval
			interval = h.calculateInterval()
			ticker.Reset(interval)
		}
	}
}

// calculateInterval calculates RTCP report interval per RFC 3550 Section 6.2
func (h *RTCPHandler) calculateInterval() time.Duration {
	h.mu.RLock()
	numSessions := len(h.sessions)
	h.mu.RUnlock()

	// Base interval
	interval := h.config.Interval

	// Scale interval based on number of sessions
	if numSessions > 0 {
		// Minimum interval is 5 seconds per RFC 3550
		minInterval := 5 * time.Second
		if h.config.ReducedSize {
			minInterval = 360 * time.Millisecond // Reduced minimum for RTCP-RR
		}

		// Add randomization (0.5 to 1.5 times the interval)
		jitter := 0.5 + rand.Float64()
		interval = time.Duration(float64(interval) * jitter)

		if interval < minInterval {
			interval = minInterval
		}
	}

	return interval
}

// sendReports sends RTCP reports for all sessions
func (h *RTCPHandler) sendReports() {
	h.mu.RLock()
	sessions := make([]*RTCPSessionHandler, 0, len(h.sessions))
	for _, s := range h.sessions {
		sessions = append(sessions, s)
	}
	h.mu.RUnlock()

	for _, session := range sessions {
		if err := session.SendReport(); err != nil {
			log.Printf("Failed to send RTCP report: %v", err)
		}
	}
}

// SetConnection sets the RTCP connection for a session
func (s *RTCPSessionHandler) SetConnection(conn *net.UDPConn, remoteAddr *net.UDPAddr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conn = conn
	s.remoteAddr = remoteAddr
}

// UpdateSenderStats updates sender statistics
func (s *RTCPSessionHandler) UpdateSenderStats(packetsSent, octetsSent uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packetsSent = packetsSent
	s.octetsSent = octetsSent
}

// UpdateReceiverStats updates receiver statistics from an RTP packet
func (s *RTCPSessionHandler) UpdateReceiverStats(seq uint16, timestamp uint32, arrivalTime time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update sequence number tracking
	if s.packetsRecv == 0 {
		s.highestSeq = seq
		s.lastSeq = seq
	} else {
		// Check for sequence wrap
		diff := int32(seq) - int32(s.highestSeq)
		if diff > 0 {
			// Normal increment
			s.highestSeq = seq
		} else if diff < -32768 {
			// Sequence number wrapped
			s.seqCycles++
			s.highestSeq = seq
		}

		// Calculate expected packets
		expectedSeq := s.highestSeq + uint16(s.seqCycles*65536)
		expected := int32(expectedSeq) - int32(s.lastSeq) + 1
		received := int32(s.packetsRecv) + 1
		s.packetsLost = expected - received
		if s.packetsLost < 0 {
			s.packetsLost = 0
		}
	}

	s.packetsRecv++

	// Calculate jitter per RFC 3550 Appendix A.8
	if s.lastArrivalTime.IsZero() {
		s.lastArrivalTime = arrivalTime
		s.lastTimestamp = timestamp
	} else {
		// Calculate transit time difference
		arrivalDiff := arrivalTime.Sub(s.lastArrivalTime).Seconds() * float64(s.clockRate)
		timestampDiff := float64(int32(timestamp - s.lastTimestamp))
		d := arrivalDiff - timestampDiff
		if d < 0 {
			d = -d
		}

		// Update jitter estimate: J = J + (|D| - J) / 16
		s.jitter += (d - s.jitter) / 16.0

		s.lastArrivalTime = arrivalTime
		s.lastTimestamp = timestamp
	}
}

// ProcessRTCP processes received RTCP packets
func (s *RTCPSessionHandler) ProcessRTCP(data []byte) error {
	packets, err := rtcp.Unmarshal(data)
	if err != nil {
		return err
	}

	for _, pkt := range packets {
		switch p := pkt.(type) {
		case *rtcp.SenderReport:
			s.processSenderReport(p)
		case *rtcp.ReceiverReport:
			s.processReceiverReport(p)
		case *rtcp.Goodbye:
			s.processGoodbye(p)
		case *rtcp.SourceDescription:
			s.processSourceDescription(p)
		}
	}

	return nil
}

// processSenderReport processes an incoming Sender Report
func (s *RTCPSessionHandler) processSenderReport(sr *rtcp.SenderReport) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rtcpSRRecv.Inc()

	// Store SR info for RTT calculation
	s.lastSRRecvNTP = uint64(sr.NTPTime)
	s.lastSRRecvTime = time.Now()

	// Process any receiver reports in the SR
	for _, rr := range sr.Reports {
		s.updateFromReceptionReport(&rr)
	}
}

// processReceiverReport processes an incoming Receiver Report
func (s *RTCPSessionHandler) processReceiverReport(rr *rtcp.ReceiverReport) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rtcpRRRecv.Inc()

	for _, report := range rr.Reports {
		s.updateFromReceptionReport(&report)
	}
}

// updateFromReceptionReport updates stats from a reception report
func (s *RTCPSessionHandler) updateFromReceptionReport(report *rtcp.ReceptionReport) {
	if report.SSRC != s.ssrc {
		return
	}

	// Update fraction lost
	s.fractionLost = report.FractionLost
	rtcpPacketLoss.Observe(float64(report.FractionLost) / 256.0)

	// Update jitter (converted from timestamp units to seconds)
	if s.clockRate > 0 {
		jitterSeconds := float64(report.Jitter) / float64(s.clockRate)
		rtcpJitterSeconds.Observe(jitterSeconds)
	}

	// Calculate RTT from DLSR and LSR
	if report.LastSenderReport != 0 && report.Delay != 0 && !s.lastSRTime.IsZero() {
		// RTT = current time - LSR - DLSR
		now := time.Now()
		dlsr := time.Duration(report.Delay) * time.Second / 65536

		// Calculate time since we sent the SR
		sinceLastSR := now.Sub(s.lastSRTime)
		s.rtt = sinceLastSR - dlsr

		if s.rtt > 0 {
			rtcpRTTSeconds.Observe(s.rtt.Seconds())
		}
	}
}

// processGoodbye processes a BYE packet
func (s *RTCPSessionHandler) processGoodbye(bye *rtcp.Goodbye) {
	// Log the goodbye
	for _, ssrc := range bye.Sources {
		log.Printf("RTCP BYE received from SSRC %d, reason: %s", ssrc, bye.Reason)
	}
}

// processSourceDescription processes an SDES packet
func (s *RTCPSessionHandler) processSourceDescription(sdes *rtcp.SourceDescription) {
	// Extract CNAME for identification
	for _, chunk := range sdes.Chunks {
		for _, item := range chunk.Items {
			if item.Type == rtcp.SDESCNAME {
				log.Printf("RTCP SDES CNAME for SSRC %d: %s", chunk.Source, item.Text)
			}
		}
	}
}

// SendReport sends an RTCP report (SR or RR)
func (s *RTCPSessionHandler) SendReport() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn == nil || s.remoteAddr == nil {
		return nil
	}

	var packets []rtcp.Packet

	// Determine if we should send SR (sender) or RR (receiver only)
	if s.packetsSent > 0 {
		sr := s.buildSenderReport()
		packets = append(packets, sr)
		rtcpSRSent.Inc()
	} else {
		rr := s.buildReceiverReport()
		packets = append(packets, rr)
		rtcpRRSent.Inc()
	}

	// Add SDES with CNAME
	sdes := &rtcp.SourceDescription{
		Chunks: []rtcp.SourceDescriptionChunk{
			{
				Source: s.ssrc,
				Items: []rtcp.SourceDescriptionItem{
					{
						Type: rtcp.SDESCNAME,
						Text: s.cname,
					},
				},
			},
		},
	}
	packets = append(packets, sdes)

	// Marshal and send
	data, err := rtcp.Marshal(packets)
	if err != nil {
		return err
	}

	_, err = s.conn.WriteToUDP(data, s.remoteAddr)
	return err
}

// buildSenderReport builds an RTCP Sender Report
func (s *RTCPSessionHandler) buildSenderReport() *rtcp.SenderReport {
	now := time.Now()
	ntpTime := toNTPTime(now)

	s.lastSRNTP = ntpTime
	s.lastSRTime = now

	sr := &rtcp.SenderReport{
		SSRC:        s.ssrc,
		NTPTime:     ntpTime,
		RTPTime:     s.calculateRTPTimestamp(now),
		PacketCount: s.packetsSent,
		OctetCount:  s.octetsSent,
	}

	// Add receiver report if we've received packets
	if s.packetsRecv > 0 {
		sr.Reports = append(sr.Reports, s.buildReceptionReport())
	}

	return sr
}

// buildReceiverReport builds an RTCP Receiver Report
func (s *RTCPSessionHandler) buildReceiverReport() *rtcp.ReceiverReport {
	rr := &rtcp.ReceiverReport{
		SSRC: s.ssrc,
	}

	if s.packetsRecv > 0 {
		rr.Reports = append(rr.Reports, s.buildReceptionReport())
	}

	return rr
}

// buildReceptionReport builds a reception report block
func (s *RTCPSessionHandler) buildReceptionReport() rtcp.ReceptionReport {
	// Calculate fraction lost since last RR
	// This is a simplified calculation
	fractionLost := uint8(0)
	if s.packetsLost > 0 && s.packetsRecv > 0 {
		fractionLost = uint8(float64(s.packetsLost) / float64(s.packetsRecv+uint32(s.packetsLost)) * 256)
	}

	// Calculate extended highest sequence number
	extHighSeq := uint32(s.highestSeq) + s.seqCycles*65536

	// Calculate interarrival jitter in timestamp units
	jitterTS := uint32(s.jitter)

	// Calculate LSR and DLSR
	var lsr, dlsr uint32
	if !s.lastSRRecvTime.IsZero() {
		// LSR is middle 32 bits of NTP timestamp from last SR
		lsr = uint32(s.lastSRRecvNTP >> 16)

		// DLSR is delay since last SR in 1/65536 seconds
		delay := time.Since(s.lastSRRecvTime)
		dlsr = uint32(delay.Seconds() * 65536)
	}

	return rtcp.ReceptionReport{
		SSRC:               s.ssrc,
		FractionLost:       fractionLost,
		TotalLost:          uint32(s.packetsLost),
		LastSequenceNumber: extHighSeq,
		Jitter:             jitterTS,
		LastSenderReport:   lsr,
		Delay:              dlsr,
	}
}

// SendBye sends an RTCP BYE packet
func (s *RTCPSessionHandler) SendBye(reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn == nil || s.remoteAddr == nil {
		return nil
	}

	bye := &rtcp.Goodbye{
		Sources: []uint32{s.ssrc},
		Reason:  reason,
	}

	data, err := rtcp.Marshal([]rtcp.Packet{bye})
	if err != nil {
		return err
	}

	_, err = s.conn.WriteToUDP(data, s.remoteAddr)
	return err
}

// GetStats returns current RTCP statistics
func (s *RTCPSessionHandler) GetStats() RTCPStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return RTCPStats{
		SSRC:          s.ssrc,
		PacketsSent:   s.packetsSent,
		OctetsSent:    s.octetsSent,
		PacketsRecv:   s.packetsRecv,
		PacketsLost:   s.packetsLost,
		FractionLost:  s.fractionLost,
		Jitter:        s.jitter / float64(s.clockRate), // Convert to seconds
		RTT:           s.rtt,
	}
}

// RTCPStats holds RTCP statistics
type RTCPStats struct {
	SSRC         uint32
	PacketsSent  uint32
	OctetsSent   uint32
	PacketsRecv  uint32
	PacketsLost  int32
	FractionLost uint8
	Jitter       float64
	RTT          time.Duration
}

// calculateRTPTimestamp calculates RTP timestamp from wall clock
func (s *RTCPSessionHandler) calculateRTPTimestamp(t time.Time) uint32 {
	// This is a simplified implementation
	// In practice, this should be synchronized with the actual RTP timestamps being sent
	elapsed := t.UnixNano()
	return uint32(elapsed / int64(time.Second/time.Duration(s.clockRate)))
}

// toNTPTime converts a time.Time to NTP timestamp (RFC 5905)
func toNTPTime(t time.Time) uint64 {
	// NTP epoch is January 1, 1900
	// Unix epoch is January 1, 1970
	// Difference is 2208988800 seconds
	const ntpEpochOffset = 2208988800

	secs := uint64(t.Unix()) + ntpEpochOffset
	frac := uint64(t.Nanosecond()) * (1 << 32) / 1e9

	return (secs << 32) | frac
}

// FromNTPTime converts NTP timestamp to time.Time
func FromNTPTime(ntp uint64) time.Time {
	const ntpEpochOffset = 2208988800

	secs := int64((ntp >> 32) - ntpEpochOffset)
	frac := float64(ntp&0xFFFFFFFF) / float64(1<<32)
	nsec := int64(frac * 1e9)

	return time.Unix(secs, nsec)
}

// ParseRTCPPacket parses raw RTCP data
func ParseRTCPPacket(data []byte) ([]rtcp.Packet, error) {
	return rtcp.Unmarshal(data)
}

// IsRTCPPacket checks if the data looks like an RTCP packet
func IsRTCPPacket(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	// RTCP packets have version 2 and payload types 200-207
	version := (data[0] >> 6) & 0x3
	payloadType := data[1]

	return version == 2 && payloadType >= 200 && payloadType <= 207
}

// GetRTCPType returns the RTCP packet type
func GetRTCPType(data []byte) uint8 {
	if len(data) < 2 {
		return 0
	}
	return data[1]
}

// RTCP packet type constants
const (
	RTCPTypeSR   = 200
	RTCPTypeRR   = 201
	RTCPTypeSDES = 202
	RTCPTypeBYE  = 203
	RTCPTypeAPP  = 204
)

// Helper functions for building RTCP packets manually

// BuildCompactSR builds a compact Sender Report
func BuildCompactSR(ssrc uint32, ntpTime uint64, rtpTime, packets, octets uint32) []byte {
	buf := make([]byte, 28)

	// Header: V=2, P=0, RC=0, PT=200, Length=6
	buf[0] = 0x80
	buf[1] = RTCPTypeSR
	binary.BigEndian.PutUint16(buf[2:4], 6) // Length in 32-bit words minus 1

	// SSRC
	binary.BigEndian.PutUint32(buf[4:8], ssrc)

	// NTP timestamp
	binary.BigEndian.PutUint64(buf[8:16], ntpTime)

	// RTP timestamp
	binary.BigEndian.PutUint32(buf[16:20], rtpTime)

	// Packet count
	binary.BigEndian.PutUint32(buf[20:24], packets)

	// Octet count
	binary.BigEndian.PutUint32(buf[24:28], octets)

	return buf
}
