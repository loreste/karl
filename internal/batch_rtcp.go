package internal

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"
)

// RTCPBatchConfig configures RTCP batch processing
type RTCPBatchConfig struct {
	// BatchSize is the maximum number of packets per batch
	BatchSize int
	// BatchTimeout is the maximum time to wait before processing
	BatchTimeout time.Duration
	// BufferSize is the channel buffer size
	BufferSize int
	// NumWorkers is the number of processing workers
	NumWorkers int
	// EnableCompound enables compound RTCP generation
	EnableCompound bool
}

// DefaultRTCPBatchConfig returns default configuration
func DefaultRTCPBatchConfig() *RTCPBatchConfig {
	return &RTCPBatchConfig{
		BatchSize:      100,
		BatchTimeout:   10 * time.Millisecond,
		BufferSize:     1000,
		NumWorkers:     4,
		EnableCompound: true,
	}
}

// RTCPPacketInfo holds information about an RTCP packet
type RTCPPacketInfo struct {
	Data      []byte
	Timestamp time.Time
	SessionID string
	Direction string // "inbound" or "outbound"
	SrcSSRC   uint32
	DstSSRC   uint32
}

// RTCPBatch represents a batch of RTCP packets
type RTCPBatch struct {
	Packets   []*RTCPPacketInfo
	StartTime time.Time
	EndTime   time.Time
	SessionID string
}

// RTCPBatchHandler processes a batch of RTCP packets
type RTCPBatchHandler func(batch *RTCPBatch)

// RTCPBatchProcessor processes RTCP packets in batches
type RTCPBatchProcessor struct {
	config  *RTCPBatchConfig
	handler RTCPBatchHandler

	packetChan chan *RTCPPacketInfo
	stopChan   chan struct{}
	doneChan   chan struct{}

	// Batches by session
	mu       sync.Mutex
	batches  map[string]*RTCPBatch
	lastSend map[string]time.Time

	// Stats
	packetsReceived atomic.Int64
	batchesCreated  atomic.Int64
	packetsDropped  atomic.Int64
}

// NewRTCPBatchProcessor creates a new RTCP batch processor
func NewRTCPBatchProcessor(config *RTCPBatchConfig, handler RTCPBatchHandler) *RTCPBatchProcessor {
	if config == nil {
		config = DefaultRTCPBatchConfig()
	}

	return &RTCPBatchProcessor{
		config:     config,
		handler:    handler,
		packetChan: make(chan *RTCPPacketInfo, config.BufferSize),
		stopChan:   make(chan struct{}),
		doneChan:   make(chan struct{}),
		batches:    make(map[string]*RTCPBatch),
		lastSend:   make(map[string]time.Time),
	}
}

// Start starts the batch processor
func (p *RTCPBatchProcessor) Start() {
	for i := 0; i < p.config.NumWorkers; i++ {
		go p.worker()
	}
	go p.timeoutChecker()
}

// Stop stops the batch processor
func (p *RTCPBatchProcessor) Stop() {
	close(p.stopChan)

	// Wait for workers to finish
	for i := 0; i < p.config.NumWorkers; i++ {
		<-p.doneChan
	}

	// Flush remaining batches
	p.mu.Lock()
	for sessionID, batch := range p.batches {
		if len(batch.Packets) > 0 {
			batch.EndTime = time.Now()
			p.handler(batch)
		}
		delete(p.batches, sessionID)
	}
	p.mu.Unlock()
}

// AddPacket adds an RTCP packet to be processed
func (p *RTCPBatchProcessor) AddPacket(packet *RTCPPacketInfo) bool {
	p.packetsReceived.Add(1)

	select {
	case p.packetChan <- packet:
		return true
	default:
		p.packetsDropped.Add(1)
		return false
	}
}

func (p *RTCPBatchProcessor) worker() {
	defer func() { p.doneChan <- struct{}{} }()

	for {
		select {
		case <-p.stopChan:
			return

		case packet := <-p.packetChan:
			p.processPacket(packet)
		}
	}
}

func (p *RTCPBatchProcessor) processPacket(packet *RTCPPacketInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()

	batch, exists := p.batches[packet.SessionID]
	if !exists {
		batch = &RTCPBatch{
			Packets:   make([]*RTCPPacketInfo, 0, p.config.BatchSize),
			StartTime: time.Now(),
			SessionID: packet.SessionID,
		}
		p.batches[packet.SessionID] = batch
	}

	batch.Packets = append(batch.Packets, packet)

	// Check if batch is full
	if len(batch.Packets) >= p.config.BatchSize {
		p.sendBatch(packet.SessionID, batch)
	}
}

func (p *RTCPBatchProcessor) sendBatch(sessionID string, batch *RTCPBatch) {
	batch.EndTime = time.Now()
	p.batchesCreated.Add(1)
	p.lastSend[sessionID] = time.Now()

	// Create new batch
	p.batches[sessionID] = &RTCPBatch{
		Packets:   make([]*RTCPPacketInfo, 0, p.config.BatchSize),
		StartTime: time.Now(),
		SessionID: sessionID,
	}

	// Call handler (outside lock in real impl)
	go p.handler(batch)
}

func (p *RTCPBatchProcessor) timeoutChecker() {
	ticker := time.NewTicker(p.config.BatchTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopChan:
			return
		case <-ticker.C:
			p.checkTimeouts()
		}
	}
}

func (p *RTCPBatchProcessor) checkTimeouts() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	for sessionID, batch := range p.batches {
		if len(batch.Packets) == 0 {
			continue
		}

		lastSend, exists := p.lastSend[sessionID]
		if !exists {
			lastSend = batch.StartTime
		}

		if now.Sub(lastSend) >= p.config.BatchTimeout {
			p.sendBatch(sessionID, batch)
		}
	}
}

// Stats returns processor statistics
func (p *RTCPBatchProcessor) Stats() *RTCPBatchStats {
	return &RTCPBatchStats{
		PacketsReceived: p.packetsReceived.Load(),
		BatchesCreated:  p.batchesCreated.Load(),
		PacketsDropped:  p.packetsDropped.Load(),
	}
}

// RTCPBatchStats holds batch processor statistics
type RTCPBatchStats struct {
	PacketsReceived int64
	BatchesCreated  int64
	PacketsDropped  int64
}

// RTCPCompoundBuilder builds compound RTCP packets
type RTCPCompoundBuilder struct {
	packets [][]byte
	ssrc    uint32
}

// NewRTCPCompoundBuilder creates a new compound RTCP builder
func NewRTCPCompoundBuilder(ssrc uint32) *RTCPCompoundBuilder {
	return &RTCPCompoundBuilder{
		packets: make([][]byte, 0),
		ssrc:    ssrc,
	}
}

// AddSR adds a Sender Report
func (b *RTCPCompoundBuilder) AddSR(ntpTime uint64, rtpTime uint32, packetCount, octetCount uint32) {
	// SR packet format (28 bytes for basic SR without report blocks)
	sr := make([]byte, 28)

	// Header: V=2, P=0, RC=0, PT=200 (SR), Length=6
	sr[0] = 0x80 // V=2, P=0, RC=0
	sr[1] = 200  // PT=SR
	binary.BigEndian.PutUint16(sr[2:4], 6) // Length in 32-bit words minus 1

	binary.BigEndian.PutUint32(sr[4:8], b.ssrc)
	binary.BigEndian.PutUint32(sr[8:12], uint32(ntpTime>>32))  // NTP timestamp MSW
	binary.BigEndian.PutUint32(sr[12:16], uint32(ntpTime))     // NTP timestamp LSW
	binary.BigEndian.PutUint32(sr[16:20], rtpTime)
	binary.BigEndian.PutUint32(sr[20:24], packetCount)
	binary.BigEndian.PutUint32(sr[24:28], octetCount)

	b.packets = append(b.packets, sr)
}

// AddRR adds a Receiver Report
func (b *RTCPCompoundBuilder) AddRR() {
	// RR packet format (8 bytes for basic RR without report blocks)
	rr := make([]byte, 8)

	// Header: V=2, P=0, RC=0, PT=201 (RR), Length=1
	rr[0] = 0x80 // V=2, P=0, RC=0
	rr[1] = 201  // PT=RR
	binary.BigEndian.PutUint16(rr[2:4], 1) // Length

	binary.BigEndian.PutUint32(rr[4:8], b.ssrc)

	b.packets = append(b.packets, rr)
}

// AddSDES adds Source Description items
func (b *RTCPCompoundBuilder) AddSDES(cname string) {
	// Calculate padded length
	cnameLen := len(cname)
	// SDES chunk: SSRC (4) + CNAME item (1 type + 1 len + value) + null (1) + padding
	chunkLen := 4 + 2 + cnameLen + 1
	// Pad to 32-bit boundary
	padding := (4 - (chunkLen % 4)) % 4
	totalLen := chunkLen + padding

	sdes := make([]byte, 4+totalLen) // 4 for header

	// Header: V=2, P=0, SC=1, PT=202 (SDES)
	sdes[0] = 0x81 // V=2, P=0, SC=1
	sdes[1] = 202  // PT=SDES
	binary.BigEndian.PutUint16(sdes[2:4], uint16((totalLen/4))) // Length in 32-bit words

	// SSRC
	binary.BigEndian.PutUint32(sdes[4:8], b.ssrc)

	// CNAME item
	sdes[8] = 1                  // CNAME type
	sdes[9] = byte(cnameLen)     // Length
	copy(sdes[10:], []byte(cname))

	b.packets = append(b.packets, sdes)
}

// AddBYE adds a BYE packet
func (b *RTCPCompoundBuilder) AddBYE(reason string) {
	reasonLen := len(reason)
	// BYE: Header (4) + SSRC (4) + optional reason (1 len + value + padding)
	totalLen := 8
	if reasonLen > 0 {
		totalLen += 1 + reasonLen
		// Pad to 32-bit boundary
		padding := (4 - ((1 + reasonLen) % 4)) % 4
		totalLen += padding
	}

	bye := make([]byte, totalLen)

	// Header: V=2, P=0, SC=1, PT=203 (BYE)
	bye[0] = 0x81 // V=2, P=0, SC=1
	bye[1] = 203  // PT=BYE
	binary.BigEndian.PutUint16(bye[2:4], uint16((totalLen-4)/4))

	binary.BigEndian.PutUint32(bye[4:8], b.ssrc)

	if reasonLen > 0 {
		bye[8] = byte(reasonLen)
		copy(bye[9:], []byte(reason))
	}

	b.packets = append(b.packets, bye)
}

// Build creates the compound RTCP packet
func (b *RTCPCompoundBuilder) Build() []byte {
	totalLen := 0
	for _, p := range b.packets {
		totalLen += len(p)
	}

	compound := make([]byte, totalLen)
	offset := 0
	for _, p := range b.packets {
		copy(compound[offset:], p)
		offset += len(p)
	}

	return compound
}

// Clear resets the builder
func (b *RTCPCompoundBuilder) Clear() {
	b.packets = b.packets[:0]
}

// RTCPReportBlock represents a report block in SR/RR
type RTCPReportBlock struct {
	SSRC             uint32
	FractionLost     uint8
	TotalLost        uint32 // 24 bits
	HighestSeq       uint32
	Jitter           uint32
	LastSR           uint32
	DelaySinceLastSR uint32
}

// AddReportBlock adds a report block to SR/RR
func (b *RTCPCompoundBuilder) AddSRWithReportBlock(ntpTime uint64, rtpTime uint32, packetCount, octetCount uint32, reports []RTCPReportBlock) {
	// SR packet format with report blocks (28 + 24*n bytes)
	reportBlockSize := 24
	numReports := len(reports)
	if numReports > 31 {
		numReports = 31 // Max RC is 5 bits
	}

	srLen := 28 + reportBlockSize*numReports
	sr := make([]byte, srLen)

	// Header: V=2, P=0, RC=n, PT=200 (SR)
	sr[0] = 0x80 | byte(numReports) // V=2, P=0, RC=n
	sr[1] = 200                      // PT=SR
	binary.BigEndian.PutUint16(sr[2:4], uint16((srLen/4)-1)) // Length in 32-bit words minus 1

	binary.BigEndian.PutUint32(sr[4:8], b.ssrc)
	binary.BigEndian.PutUint32(sr[8:12], uint32(ntpTime>>32))
	binary.BigEndian.PutUint32(sr[12:16], uint32(ntpTime))
	binary.BigEndian.PutUint32(sr[16:20], rtpTime)
	binary.BigEndian.PutUint32(sr[20:24], packetCount)
	binary.BigEndian.PutUint32(sr[24:28], octetCount)

	// Add report blocks
	offset := 28
	for i := 0; i < numReports; i++ {
		rb := reports[i]
		binary.BigEndian.PutUint32(sr[offset:offset+4], rb.SSRC)

		// Fraction lost (8 bits) + cumulative lost (24 bits)
		sr[offset+4] = rb.FractionLost
		sr[offset+5] = byte(rb.TotalLost >> 16)
		sr[offset+6] = byte(rb.TotalLost >> 8)
		sr[offset+7] = byte(rb.TotalLost)

		binary.BigEndian.PutUint32(sr[offset+8:offset+12], rb.HighestSeq)
		binary.BigEndian.PutUint32(sr[offset+12:offset+16], rb.Jitter)
		binary.BigEndian.PutUint32(sr[offset+16:offset+20], rb.LastSR)
		binary.BigEndian.PutUint32(sr[offset+20:offset+24], rb.DelaySinceLastSR)

		offset += reportBlockSize
	}

	b.packets = append(b.packets, sr)
}

// ParseRTCPPacketBasic parses basic RTCP packet info
func ParseRTCPPacketBasic(data []byte) (*RTCPPacketParsed, error) {
	if len(data) < 4 {
		return nil, ErrInvalidRTCP
	}

	version := (data[0] >> 6) & 0x03
	if version != 2 {
		return nil, ErrInvalidRTCP
	}

	padding := (data[0] >> 5) & 0x01
	count := data[0] & 0x1F
	packetType := data[1]
	length := binary.BigEndian.Uint16(data[2:4])

	expectedLen := int((length + 1) * 4)
	if len(data) < expectedLen {
		return nil, ErrInvalidRTCP
	}

	parsed := &RTCPPacketParsed{
		Version:    version,
		Padding:    padding == 1,
		Count:      count,
		PacketType: packetType,
		Length:     length,
		Data:       data[:expectedLen],
	}

	// Parse SSRC for SR/RR/SDES/BYE
	if len(data) >= 8 {
		parsed.SSRC = binary.BigEndian.Uint32(data[4:8])
	}

	return parsed, nil
}

// RTCPPacketParsed holds parsed RTCP packet info
type RTCPPacketParsed struct {
	Version    uint8
	Padding    bool
	Count      uint8
	PacketType uint8
	Length     uint16
	SSRC       uint32
	Data       []byte
}

// PacketTypeName returns the packet type name
func (p *RTCPPacketParsed) PacketTypeName() string {
	switch p.PacketType {
	case 200:
		return "SR"
	case 201:
		return "RR"
	case 202:
		return "SDES"
	case 203:
		return "BYE"
	case 204:
		return "APP"
	case 205:
		return "RTPFB"
	case 206:
		return "PSFB"
	case 207:
		return "XR"
	default:
		return "Unknown"
	}
}

// ErrInvalidRTCP indicates an invalid RTCP packet
var ErrInvalidRTCP = errRTCPInvalid

var errRTCPInvalid = &rtcpError{"invalid RTCP packet"}

type rtcpError struct {
	msg string
}

func (e *rtcpError) Error() string {
	return e.msg
}
