package internal

import (
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// CDRFormat represents the output format for CDRs
type CDRFormat int

const (
	CDRFormatJSON CDRFormat = iota
	CDRFormatCSV
	CDRFormatSyslog
)

func (f CDRFormat) String() string {
	switch f {
	case CDRFormatJSON:
		return "json"
	case CDRFormatCSV:
		return "csv"
	case CDRFormatSyslog:
		return "syslog"
	default:
		return "unknown"
	}
}

// CDR represents a Call Detail Record
type CDR struct {
	// Identifiers
	ID        string `json:"id"`
	CallID    string `json:"call_id"`
	FromTag   string `json:"from_tag"`
	ToTag     string `json:"to_tag"`
	SessionID string `json:"session_id,omitempty"`

	// Timing
	StartTime    time.Time `json:"start_time"`
	AnswerTime   time.Time `json:"answer_time,omitempty"`
	EndTime      time.Time `json:"end_time"`
	SetupTime    int64     `json:"setup_time_ms,omitempty"`     // Time to answer
	Duration     int64     `json:"duration_ms"`                 // Total duration
	TalkTime     int64     `json:"talk_time_ms,omitempty"`      // Time after answer

	// Call info
	CallerNumber  string `json:"caller_number,omitempty"`
	CalleeNumber  string `json:"callee_number,omitempty"`
	Direction     string `json:"direction,omitempty"` // inbound, outbound, internal

	// Media info
	Codec           string `json:"codec,omitempty"`
	SamplingRate    int    `json:"sampling_rate,omitempty"`
	PacketsRx       uint64 `json:"packets_rx"`
	PacketsTx       uint64 `json:"packets_tx"`
	BytesRx         uint64 `json:"bytes_rx"`
	BytesTx         uint64 `json:"bytes_tx"`
	PacketsLost     uint64 `json:"packets_lost"`
	PacketsLostPct  float64 `json:"packets_lost_pct"`
	Jitter          float64 `json:"jitter_ms"`
	MOS             float64 `json:"mos,omitempty"`
	RFactor         float64 `json:"r_factor,omitempty"`

	// Status
	DisconnectCause string `json:"disconnect_cause"`
	DisconnectCode  int    `json:"disconnect_code"`
	Status          string `json:"status"` // completed, failed, cancelled

	// Recording
	RecordingEnabled bool   `json:"recording_enabled"`
	RecordingFile    string `json:"recording_file,omitempty"`

	// Network
	LocalIP     string `json:"local_ip,omitempty"`
	RemoteIP    string `json:"remote_ip,omitempty"`
	LocalPort   int    `json:"local_port,omitempty"`
	RemotePort  int    `json:"remote_port,omitempty"`
	Transport   string `json:"transport,omitempty"` // UDP, TCP, TLS

	// Custom fields
	CustomFields map[string]interface{} `json:"custom_fields,omitempty"`
}

// CalculateDurations calculates derived time fields
func (c *CDR) CalculateDurations() {
	if !c.EndTime.IsZero() && !c.StartTime.IsZero() {
		c.Duration = c.EndTime.Sub(c.StartTime).Milliseconds()
	}
	if !c.AnswerTime.IsZero() && !c.StartTime.IsZero() {
		c.SetupTime = c.AnswerTime.Sub(c.StartTime).Milliseconds()
	}
	if !c.AnswerTime.IsZero() && !c.EndTime.IsZero() {
		c.TalkTime = c.EndTime.Sub(c.AnswerTime).Milliseconds()
	}
}

// CalculatePacketLoss calculates packet loss percentage
func (c *CDR) CalculatePacketLoss() {
	total := c.PacketsRx + c.PacketsLost
	if total > 0 {
		c.PacketsLostPct = float64(c.PacketsLost) / float64(total) * 100
	}
}

// ToCSVRow returns CSV row data
func (c *CDR) ToCSVRow() []string {
	return []string{
		c.ID,
		c.CallID,
		c.FromTag,
		c.ToTag,
		c.StartTime.Format(time.RFC3339),
		c.EndTime.Format(time.RFC3339),
		fmt.Sprintf("%d", c.Duration),
		fmt.Sprintf("%d", c.TalkTime),
		c.CallerNumber,
		c.CalleeNumber,
		c.Codec,
		fmt.Sprintf("%d", c.PacketsRx),
		fmt.Sprintf("%d", c.PacketsTx),
		fmt.Sprintf("%d", c.BytesRx),
		fmt.Sprintf("%d", c.BytesTx),
		fmt.Sprintf("%d", c.PacketsLost),
		fmt.Sprintf("%.2f", c.PacketsLostPct),
		fmt.Sprintf("%.2f", c.Jitter),
		fmt.Sprintf("%.2f", c.MOS),
		c.DisconnectCause,
		fmt.Sprintf("%d", c.DisconnectCode),
		c.Status,
		c.LocalIP,
		c.RemoteIP,
	}
}

// CSVHeader returns the CSV header row
func CSVHeader() []string {
	return []string{
		"id",
		"call_id",
		"from_tag",
		"to_tag",
		"start_time",
		"end_time",
		"duration_ms",
		"talk_time_ms",
		"caller_number",
		"callee_number",
		"codec",
		"packets_rx",
		"packets_tx",
		"bytes_rx",
		"bytes_tx",
		"packets_lost",
		"packets_lost_pct",
		"jitter_ms",
		"mos",
		"disconnect_cause",
		"disconnect_code",
		"status",
		"local_ip",
		"remote_ip",
	}
}

// CDRExporterConfig holds configuration for CDR export
type CDRExporterConfig struct {
	Format          CDRFormat
	OutputPath      string        // File path or URL
	BufferSize      int           // Number of CDRs to buffer
	FlushInterval   time.Duration // How often to flush
	RotateSize      int64         // Rotate file at this size (bytes)
	RotateInterval  time.Duration // Rotate file at this interval
	MaxFiles        int           // Maximum rotated files to keep
	IncludeHeader   bool          // Include header in CSV
	CompressRotated bool          // Compress rotated files
}

// DefaultCDRExporterConfig returns sensible defaults
func DefaultCDRExporterConfig() *CDRExporterConfig {
	return &CDRExporterConfig{
		Format:         CDRFormatJSON,
		BufferSize:     1000,
		FlushInterval:  time.Minute,
		RotateSize:     100 * 1024 * 1024, // 100MB
		RotateInterval: 24 * time.Hour,
		MaxFiles:       7,
		IncludeHeader:  true,
	}
}

// CDRExporter exports CDRs to various destinations
type CDRExporter struct {
	config *CDRExporterConfig

	buffer  []*CDR
	bufferMu sync.Mutex

	file       *os.File
	csvWriter  *csv.Writer
	fileMu     sync.Mutex

	// Current file info
	currentSize   atomic.Int64
	fileCreatedAt time.Time

	// Metrics
	exported  atomic.Int64
	dropped   atomic.Int64
	errors    atomic.Int64

	// State
	stopCh chan struct{}
	closed atomic.Bool
}

// NewCDRExporter creates a new CDR exporter
func NewCDRExporter(config *CDRExporterConfig) (*CDRExporter, error) {
	if config == nil {
		config = DefaultCDRExporterConfig()
	}

	exporter := &CDRExporter{
		config: config,
		buffer: make([]*CDR, 0, config.BufferSize),
		stopCh: make(chan struct{}),
	}

	if config.OutputPath != "" {
		if err := exporter.openFile(); err != nil {
			return nil, err
		}
	}

	// Start background flusher
	go exporter.flushLoop()

	return exporter, nil
}

// openFile opens the output file
func (e *CDRExporter) openFile() error {
	e.fileMu.Lock()
	defer e.fileMu.Unlock()

	// Close existing file if open
	if e.file != nil {
		if e.csvWriter != nil {
			e.csvWriter.Flush()
		}
		e.file.Close()
	}

	// Ensure directory exists
	dir := filepath.Dir(e.config.OutputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Open file
	f, err := os.OpenFile(e.config.OutputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}

	e.file = f
	e.fileCreatedAt = time.Now()
	e.currentSize.Store(0)

	// Get current file size
	if info, err := f.Stat(); err == nil {
		e.currentSize.Store(info.Size())
	}

	// Initialize CSV writer if needed
	if e.config.Format == CDRFormatCSV {
		e.csvWriter = csv.NewWriter(f)
		// Write header if file is empty and header enabled
		if e.currentSize.Load() == 0 && e.config.IncludeHeader {
			e.csvWriter.Write(CSVHeader())
			e.csvWriter.Flush()
		}
	}

	return nil
}

// Export exports a CDR
func (e *CDRExporter) Export(cdr *CDR) error {
	if e.closed.Load() {
		return fmt.Errorf("exporter is closed")
	}

	// Calculate derived fields
	cdr.CalculateDurations()
	cdr.CalculatePacketLoss()

	e.bufferMu.Lock()
	e.buffer = append(e.buffer, cdr)
	shouldFlush := len(e.buffer) >= e.config.BufferSize
	e.bufferMu.Unlock()

	if shouldFlush {
		return e.Flush()
	}

	return nil
}

// Flush writes buffered CDRs to output
func (e *CDRExporter) Flush() error {
	e.bufferMu.Lock()
	if len(e.buffer) == 0 {
		e.bufferMu.Unlock()
		return nil
	}
	cdrs := e.buffer
	e.buffer = make([]*CDR, 0, e.config.BufferSize)
	e.bufferMu.Unlock()

	// Check for rotation
	if e.shouldRotate() {
		if err := e.rotate(); err != nil {
			e.errors.Add(1)
			// Continue with current file
		}
	}

	e.fileMu.Lock()
	defer e.fileMu.Unlock()

	if e.file == nil {
		return fmt.Errorf("no output file")
	}

	var bytesWritten int64
	var err error

	switch e.config.Format {
	case CDRFormatJSON:
		bytesWritten, err = e.writeJSON(cdrs)
	case CDRFormatCSV:
		bytesWritten, err = e.writeCSV(cdrs)
	case CDRFormatSyslog:
		bytesWritten, err = e.writeSyslog(cdrs)
	default:
		err = fmt.Errorf("unknown format: %d", e.config.Format)
	}

	if err != nil {
		e.errors.Add(int64(len(cdrs)))
		return err
	}

	e.currentSize.Add(bytesWritten)
	e.exported.Add(int64(len(cdrs)))
	return nil
}

// writeJSON writes CDRs as JSON lines
func (e *CDRExporter) writeJSON(cdrs []*CDR) (int64, error) {
	var totalBytes int64
	encoder := json.NewEncoder(e.file)

	for _, cdr := range cdrs {
		data, err := json.Marshal(cdr)
		if err != nil {
			return totalBytes, err
		}
		n, err := e.file.Write(append(data, '\n'))
		totalBytes += int64(n)
		if err != nil {
			return totalBytes, err
		}
	}

	// Use encoder to suppress warning
	_ = encoder

	return totalBytes, nil
}

// writeCSV writes CDRs as CSV rows
func (e *CDRExporter) writeCSV(cdrs []*CDR) (int64, error) {
	if e.csvWriter == nil {
		return 0, fmt.Errorf("CSV writer not initialized")
	}

	for _, cdr := range cdrs {
		if err := e.csvWriter.Write(cdr.ToCSVRow()); err != nil {
			return 0, err
		}
	}
	e.csvWriter.Flush()

	return 0, e.csvWriter.Error() // CSV writer doesn't track bytes
}

// writeSyslog writes CDRs in syslog format
func (e *CDRExporter) writeSyslog(cdrs []*CDR) (int64, error) {
	var totalBytes int64

	for _, cdr := range cdrs {
		msg := fmt.Sprintf("<%d>%s karl-cdr[%s]: call_id=%s from=%s to=%s duration=%dms status=%s\n",
			14, // facility=1 (user), severity=6 (info)
			time.Now().Format(time.RFC3339),
			cdr.ID,
			cdr.CallID,
			cdr.CallerNumber,
			cdr.CalleeNumber,
			cdr.Duration,
			cdr.Status,
		)
		n, err := e.file.WriteString(msg)
		totalBytes += int64(n)
		if err != nil {
			return totalBytes, err
		}
	}

	return totalBytes, nil
}

// shouldRotate checks if file should be rotated
func (e *CDRExporter) shouldRotate() bool {
	if e.config.RotateSize > 0 && e.currentSize.Load() >= e.config.RotateSize {
		return true
	}
	if e.config.RotateInterval > 0 && time.Since(e.fileCreatedAt) >= e.config.RotateInterval {
		return true
	}
	return false
}

// rotate rotates the current file
func (e *CDRExporter) rotate() error {
	e.fileMu.Lock()
	defer e.fileMu.Unlock()

	if e.file == nil {
		return nil
	}

	// Flush CSV writer
	if e.csvWriter != nil {
		e.csvWriter.Flush()
	}

	// Close current file
	e.file.Close()

	// Rename to timestamped name
	rotatedName := fmt.Sprintf("%s.%s", e.config.OutputPath, time.Now().Format("20060102-150405"))
	if err := os.Rename(e.config.OutputPath, rotatedName); err != nil {
		return err
	}

	// Cleanup old files
	e.cleanupOldFiles()

	// Open new file
	e.file = nil
	e.csvWriter = nil
	return e.openFileLocked()
}

// openFileLocked opens file (must hold fileMu)
func (e *CDRExporter) openFileLocked() error {
	// Ensure directory exists
	dir := filepath.Dir(e.config.OutputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	f, err := os.OpenFile(e.config.OutputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	e.file = f
	e.fileCreatedAt = time.Now()
	e.currentSize.Store(0)

	if e.config.Format == CDRFormatCSV {
		e.csvWriter = csv.NewWriter(f)
		if e.config.IncludeHeader {
			e.csvWriter.Write(CSVHeader())
			e.csvWriter.Flush()
		}
	}

	return nil
}

// cleanupOldFiles removes old rotated files
func (e *CDRExporter) cleanupOldFiles() {
	dir := filepath.Dir(e.config.OutputPath)
	base := filepath.Base(e.config.OutputPath)
	pattern := base + ".*"

	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return
	}

	if len(matches) <= e.config.MaxFiles {
		return
	}

	// Sort by modification time and remove oldest
	// Simple approach: remove files exceeding MaxFiles
	for i := 0; i < len(matches)-e.config.MaxFiles; i++ {
		os.Remove(matches[i])
	}
}

// flushLoop periodically flushes the buffer
func (e *CDRExporter) flushLoop() {
	ticker := time.NewTicker(e.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.Flush()
		}
	}
}

// GetStats returns export statistics
func (e *CDRExporter) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"format":       e.config.Format.String(),
		"exported":     e.exported.Load(),
		"dropped":      e.dropped.Load(),
		"errors":       e.errors.Load(),
		"current_size": e.currentSize.Load(),
		"buffer_size":  len(e.buffer),
	}
}

// Close closes the exporter
func (e *CDRExporter) Close() error {
	if !e.closed.CompareAndSwap(false, true) {
		return nil
	}

	close(e.stopCh)

	// Final flush
	e.Flush()

	e.fileMu.Lock()
	defer e.fileMu.Unlock()

	if e.csvWriter != nil {
		e.csvWriter.Flush()
	}
	if e.file != nil {
		return e.file.Close()
	}
	return nil
}

// CDRBuilder helps build CDRs
type CDRBuilder struct {
	cdr *CDR
}

// NewCDRBuilder creates a new CDR builder
func NewCDRBuilder() *CDRBuilder {
	return &CDRBuilder{
		cdr: &CDR{
			ID:        generateCDRID(),
			StartTime: time.Now(),
		},
	}
}

// generateCDRID generates a unique CDR ID
func generateCDRID() string {
	// Generate random bytes for uniqueness
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("cdr-%d-%s", time.Now().UnixNano(), hex.EncodeToString(b))
}

// WithCallID sets the call ID
func (b *CDRBuilder) WithCallID(callID string) *CDRBuilder {
	b.cdr.CallID = callID
	return b
}

// WithTags sets from and to tags
func (b *CDRBuilder) WithTags(fromTag, toTag string) *CDRBuilder {
	b.cdr.FromTag = fromTag
	b.cdr.ToTag = toTag
	return b
}

// WithParties sets caller and callee numbers
func (b *CDRBuilder) WithParties(caller, callee string) *CDRBuilder {
	b.cdr.CallerNumber = caller
	b.cdr.CalleeNumber = callee
	return b
}

// WithTiming sets timing information
func (b *CDRBuilder) WithTiming(start, answer, end time.Time) *CDRBuilder {
	b.cdr.StartTime = start
	b.cdr.AnswerTime = answer
	b.cdr.EndTime = end
	return b
}

// WithMedia sets media statistics
func (b *CDRBuilder) WithMedia(codec string, packetsRx, packetsTx, bytesRx, bytesTx uint64) *CDRBuilder {
	b.cdr.Codec = codec
	b.cdr.PacketsRx = packetsRx
	b.cdr.PacketsTx = packetsTx
	b.cdr.BytesRx = bytesRx
	b.cdr.BytesTx = bytesTx
	return b
}

// WithQuality sets quality metrics
func (b *CDRBuilder) WithQuality(packetsLost uint64, jitter, mos float64) *CDRBuilder {
	b.cdr.PacketsLost = packetsLost
	b.cdr.Jitter = jitter
	b.cdr.MOS = mos
	return b
}

// WithStatus sets call status
func (b *CDRBuilder) WithStatus(status, cause string, code int) *CDRBuilder {
	b.cdr.Status = status
	b.cdr.DisconnectCause = cause
	b.cdr.DisconnectCode = code
	return b
}

// WithNetwork sets network information
func (b *CDRBuilder) WithNetwork(localIP, remoteIP string, localPort, remotePort int) *CDRBuilder {
	b.cdr.LocalIP = localIP
	b.cdr.RemoteIP = remoteIP
	b.cdr.LocalPort = localPort
	b.cdr.RemotePort = remotePort
	return b
}

// WithRecording sets recording information
func (b *CDRBuilder) WithRecording(enabled bool, file string) *CDRBuilder {
	b.cdr.RecordingEnabled = enabled
	b.cdr.RecordingFile = file
	return b
}

// WithCustomField adds a custom field
func (b *CDRBuilder) WithCustomField(key string, value interface{}) *CDRBuilder {
	if b.cdr.CustomFields == nil {
		b.cdr.CustomFields = make(map[string]interface{})
	}
	b.cdr.CustomFields[key] = value
	return b
}

// Build returns the completed CDR
func (b *CDRBuilder) Build() *CDR {
	b.cdr.CalculateDurations()
	b.cdr.CalculatePacketLoss()
	return b.cdr
}

// MemoryCDRExporter stores CDRs in memory (for testing)
type MemoryCDRExporter struct {
	cdrs []*CDR
	mu   sync.Mutex
}

// NewMemoryCDRExporter creates a new memory exporter
func NewMemoryCDRExporter() *MemoryCDRExporter {
	return &MemoryCDRExporter{
		cdrs: make([]*CDR, 0),
	}
}

// Export stores a CDR in memory
func (e *MemoryCDRExporter) Export(cdr *CDR) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cdrs = append(e.cdrs, cdr)
	return nil
}

// GetCDRs returns all stored CDRs
func (e *MemoryCDRExporter) GetCDRs() []*CDR {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make([]*CDR, len(e.cdrs))
	copy(result, e.cdrs)
	return result
}

// Reset clears all CDRs
func (e *MemoryCDRExporter) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cdrs = e.cdrs[:0]
}

// WriteCDRToWriter writes a CDR to an io.Writer
func WriteCDRToWriter(w io.Writer, cdr *CDR, format CDRFormat) error {
	switch format {
	case CDRFormatJSON:
		encoder := json.NewEncoder(w)
		return encoder.Encode(cdr)
	case CDRFormatCSV:
		writer := csv.NewWriter(w)
		err := writer.Write(cdr.ToCSVRow())
		writer.Flush()
		return err
	default:
		return fmt.Errorf("unsupported format: %d", format)
	}
}
