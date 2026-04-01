package internal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// PCAP capture errors
var (
	ErrCaptureAlreadyRunning = errors.New("capture already running")
	ErrCaptureNotRunning     = errors.New("capture not running")
	ErrCaptureStopped        = errors.New("capture stopped")
	ErrMaxPacketsReached     = errors.New("maximum packets reached")
	ErrMaxSizeReached        = errors.New("maximum file size reached")
)

// PCAPLinkType represents the link layer type
type PCAPLinkType uint32

const (
	LinkTypeNull     PCAPLinkType = 0
	LinkTypeEthernet PCAPLinkType = 1
	LinkTypeRaw      PCAPLinkType = 101 // Raw IP
	LinkTypeLinuxSLL PCAPLinkType = 113
)

// PCAPCaptureConfig holds capture configuration
type PCAPCaptureConfig struct {
	OutputPath     string
	MaxPackets     int64
	MaxFileSize    int64
	MaxDuration    time.Duration
	SnapLen        uint32
	LinkType       PCAPLinkType
	BufferSize     int
	RotateSize     int64
	RotateInterval time.Duration
	Filter         PacketFilter
}

// DefaultPCAPCaptureConfig returns default configuration
func DefaultPCAPCaptureConfig() *PCAPCaptureConfig {
	return &PCAPCaptureConfig{
		MaxPackets:  0, // Unlimited
		MaxFileSize: 100 * 1024 * 1024, // 100MB
		MaxDuration: 0, // Unlimited
		SnapLen:     65535,
		LinkType:    LinkTypeRaw,
		BufferSize:  1000,
	}
}

// PacketFilter filters packets for capture
type PacketFilter func(packet *CapturedPacket) bool

// CapturedPacket represents a captured packet
type CapturedPacket struct {
	Timestamp   time.Time
	CaptureLen  uint32
	OrigLen     uint32
	Data        []byte
	CallID      string
	SessionID   string
	Direction   string // "inbound" or "outbound"
	SrcIP       string
	DstIP       string
	SrcPort     uint16
	DstPort     uint16
	Protocol    string // "RTP", "RTCP", "STUN", etc.
}

// PCAPCapture handles packet capture
type PCAPCapture struct {
	config *PCAPCaptureConfig

	mu        sync.Mutex
	running   bool
	file      *os.File
	writer    *pcapFileWriter

	packetCount atomic.Int64
	byteCount   atomic.Int64
	startTime   time.Time

	packetChan chan *CapturedPacket
	stopChan   chan struct{}
	doneChan   chan struct{}
}

// pcapFileWriter writes PCAP format
type pcapFileWriter struct {
	w        io.Writer
	snapLen  uint32
	linkType PCAPLinkType
}

// PCAP file header constants
const (
	pcapMagicNumber    = 0xa1b2c3d4
	pcapVersionMajor   = 2
	pcapVersionMinor   = 4
	pcapHeaderSize     = 24
	pcapPacketHdrSize  = 16
)

// NewPCAPCapture creates a new PCAP capture
func NewPCAPCapture(config *PCAPCaptureConfig) *PCAPCapture {
	if config == nil {
		config = DefaultPCAPCaptureConfig()
	}

	bufferSize := config.BufferSize
	if bufferSize <= 0 {
		bufferSize = 1000
	}

	return &PCAPCapture{
		config:     config,
		packetChan: make(chan *CapturedPacket, bufferSize),
		stopChan:   make(chan struct{}),
		doneChan:   make(chan struct{}),
	}
}

// Start starts the packet capture
func (pc *PCAPCapture) Start() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.running {
		return ErrCaptureAlreadyRunning
	}

	// Create output directory
	dir := filepath.Dir(pc.config.OutputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Open output file
	file, err := os.Create(pc.config.OutputPath)
	if err != nil {
		return fmt.Errorf("failed to create capture file: %w", err)
	}

	pc.file = file
	pc.writer = &pcapFileWriter{
		w:        file,
		snapLen:  pc.config.SnapLen,
		linkType: pc.config.LinkType,
	}

	// Write PCAP header
	if err := pc.writer.writeHeader(); err != nil {
		pc.file.Close()
		return fmt.Errorf("failed to write PCAP header: %w", err)
	}

	pc.running = true
	pc.startTime = time.Now()
	pc.stopChan = make(chan struct{})
	pc.doneChan = make(chan struct{})

	go pc.captureLoop()

	return nil
}

// Stop stops the packet capture
func (pc *PCAPCapture) Stop() error {
	pc.mu.Lock()
	if !pc.running {
		pc.mu.Unlock()
		return ErrCaptureNotRunning
	}
	pc.running = false
	pc.mu.Unlock()

	close(pc.stopChan)
	<-pc.doneChan

	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.file != nil {
		pc.file.Close()
		pc.file = nil
	}

	return nil
}

// IsRunning returns whether capture is running
func (pc *PCAPCapture) IsRunning() bool {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return pc.running
}

// CapturePacket adds a packet to the capture
func (pc *PCAPCapture) CapturePacket(packet *CapturedPacket) error {
	pc.mu.Lock()
	if !pc.running {
		pc.mu.Unlock()
		return ErrCaptureNotRunning
	}
	pc.mu.Unlock()

	// Apply filter
	if pc.config.Filter != nil && !pc.config.Filter(packet) {
		return nil
	}

	// Check limits
	if pc.config.MaxPackets > 0 && pc.packetCount.Load() >= pc.config.MaxPackets {
		return ErrMaxPacketsReached
	}

	if pc.config.MaxFileSize > 0 && pc.byteCount.Load() >= pc.config.MaxFileSize {
		return ErrMaxSizeReached
	}

	if pc.config.MaxDuration > 0 && time.Since(pc.startTime) >= pc.config.MaxDuration {
		return ErrCaptureStopped
	}

	// Send to capture loop
	select {
	case pc.packetChan <- packet:
		return nil
	default:
		// Buffer full, drop packet
		return nil
	}
}

func (pc *PCAPCapture) captureLoop() {
	defer close(pc.doneChan)

	for {
		select {
		case <-pc.stopChan:
			// Drain remaining packets
			for {
				select {
				case pkt := <-pc.packetChan:
					pc.writePacket(pkt)
				default:
					return
				}
			}

		case packet := <-pc.packetChan:
			pc.writePacket(packet)
		}
	}
}

func (pc *PCAPCapture) writePacket(packet *CapturedPacket) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.writer == nil {
		return
	}

	if err := pc.writer.writePacket(packet); err != nil {
		return
	}

	pc.packetCount.Add(1)
	pc.byteCount.Add(int64(len(packet.Data) + pcapPacketHdrSize))
}

// GetStats returns capture statistics
func (pc *PCAPCapture) GetStats() *PCAPCaptureStats {
	pc.mu.Lock()
	running := pc.running
	startTime := pc.startTime
	pc.mu.Unlock()

	var duration time.Duration
	if !startTime.IsZero() {
		duration = time.Since(startTime)
	}

	return &PCAPCaptureStats{
		PacketCount: pc.packetCount.Load(),
		ByteCount:   pc.byteCount.Load(),
		Duration:    duration,
		Running:     running,
	}
}

// PCAPCaptureStats holds capture statistics
type PCAPCaptureStats struct {
	PacketCount int64
	ByteCount   int64
	Duration    time.Duration
	Running     bool
}

// pcapFileWriter methods

func (w *pcapFileWriter) writeHeader() error {
	header := make([]byte, pcapHeaderSize)

	binary.LittleEndian.PutUint32(header[0:4], pcapMagicNumber)
	binary.LittleEndian.PutUint16(header[4:6], pcapVersionMajor)
	binary.LittleEndian.PutUint16(header[6:8], pcapVersionMinor)
	binary.LittleEndian.PutUint32(header[8:12], 0)  // thiszone
	binary.LittleEndian.PutUint32(header[12:16], 0) // sigfigs
	binary.LittleEndian.PutUint32(header[16:20], w.snapLen)
	binary.LittleEndian.PutUint32(header[20:24], uint32(w.linkType))

	_, err := w.w.Write(header)
	return err
}

func (w *pcapFileWriter) writePacket(packet *CapturedPacket) error {
	// Calculate lengths
	captureLen := uint32(len(packet.Data))
	if captureLen > w.snapLen {
		captureLen = w.snapLen
	}

	origLen := packet.OrigLen
	if origLen == 0 {
		origLen = uint32(len(packet.Data))
	}

	ts := packet.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	// Write packet header
	header := make([]byte, pcapPacketHdrSize)
	binary.LittleEndian.PutUint32(header[0:4], uint32(ts.Unix()))
	binary.LittleEndian.PutUint32(header[4:8], uint32(ts.Nanosecond()/1000))
	binary.LittleEndian.PutUint32(header[8:12], captureLen)
	binary.LittleEndian.PutUint32(header[12:16], origLen)

	if _, err := w.w.Write(header); err != nil {
		return err
	}

	// Write packet data
	_, err := w.w.Write(packet.Data[:captureLen])
	return err
}

// PCAPCaptureManager manages multiple captures
type PCAPCaptureManager struct {
	captures map[string]*PCAPCapture
	mu       sync.RWMutex
	basePath string
}

// NewPCAPCaptureManager creates a new capture manager
func NewPCAPCaptureManager(basePath string) *PCAPCaptureManager {
	return &PCAPCaptureManager{
		captures: make(map[string]*PCAPCapture),
		basePath: basePath,
	}
}

// StartCapture starts a named capture
func (m *PCAPCaptureManager) StartCapture(name string, config *PCAPCaptureConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.captures[name]; exists {
		return fmt.Errorf("capture %s already exists", name)
	}

	if config == nil {
		config = DefaultPCAPCaptureConfig()
	}

	if config.OutputPath == "" {
		config.OutputPath = filepath.Join(m.basePath, fmt.Sprintf("%s_%d.pcap", name, time.Now().Unix()))
	}

	capture := NewPCAPCapture(config)
	if err := capture.Start(); err != nil {
		return err
	}

	m.captures[name] = capture
	return nil
}

// StopCapture stops a named capture
func (m *PCAPCaptureManager) StopCapture(name string) error {
	m.mu.Lock()
	capture, exists := m.captures[name]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("capture %s not found", name)
	}
	delete(m.captures, name)
	m.mu.Unlock()

	return capture.Stop()
}

// GetCapture gets a named capture
func (m *PCAPCaptureManager) GetCapture(name string) *PCAPCapture {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.captures[name]
}

// ListCaptures returns all capture names
func (m *PCAPCaptureManager) ListCaptures() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.captures))
	for name := range m.captures {
		names = append(names, name)
	}
	return names
}

// StopAll stops all captures
func (m *PCAPCaptureManager) StopAll() {
	m.mu.Lock()
	captures := make(map[string]*PCAPCapture)
	for k, v := range m.captures {
		captures[k] = v
	}
	m.captures = make(map[string]*PCAPCapture)
	m.mu.Unlock()

	for _, capture := range captures {
		capture.Stop()
	}
}

// CallPCAPCapture captures packets for a specific call
type CallPCAPCapture struct {
	callID   string
	capture  *PCAPCapture
	started  time.Time
}

// NewCallPCAPCapture creates a capture for a specific call
func NewCallPCAPCapture(callID, outputPath string) (*CallPCAPCapture, error) {
	config := &PCAPCaptureConfig{
		OutputPath:  outputPath,
		MaxDuration: 2 * time.Hour, // Max 2 hours per call
		MaxFileSize: 500 * 1024 * 1024, // 500MB max
		SnapLen:     65535,
		LinkType:    LinkTypeRaw,
		Filter: func(p *CapturedPacket) bool {
			return p.CallID == callID
		},
	}

	capture := NewPCAPCapture(config)

	return &CallPCAPCapture{
		callID:  callID,
		capture: capture,
		started: time.Now(),
	}, nil
}

// Start starts the call capture
func (c *CallPCAPCapture) Start() error {
	return c.capture.Start()
}

// Stop stops the call capture
func (c *CallPCAPCapture) Stop() error {
	return c.capture.Stop()
}

// CapturePacket captures a packet for this call
func (c *CallPCAPCapture) CapturePacket(packet *CapturedPacket) error {
	packet.CallID = c.callID
	return c.capture.CapturePacket(packet)
}

// GetStats returns capture stats
func (c *CallPCAPCapture) GetStats() *PCAPCaptureStats {
	return c.capture.GetStats()
}

// CreateRTPPacketCapture creates a captured packet from RTP data
func CreateRTPPacketCapture(data []byte, srcIP, dstIP string, srcPort, dstPort uint16, direction string) *CapturedPacket {
	return &CapturedPacket{
		Timestamp:  time.Now(),
		CaptureLen: uint32(len(data)),
		OrigLen:    uint32(len(data)),
		Data:       data,
		Direction:  direction,
		SrcIP:      srcIP,
		DstIP:      dstIP,
		SrcPort:    srcPort,
		DstPort:    dstPort,
		Protocol:   "RTP",
	}
}
