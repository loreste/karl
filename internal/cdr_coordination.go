package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// CDRCoordinatorConfig configures the CDR coordinator
type CDRCoordinatorConfig struct {
	// NodeID is this node's identifier
	NodeID string
	// RedisPrefix is the prefix for Redis keys
	RedisPrefix string
	// FlushInterval is how often to flush CDRs
	FlushInterval time.Duration
	// BatchSize is the maximum batch size
	BatchSize int
	// RetryAttempts is the number of retry attempts
	RetryAttempts int
	// RetryDelay is the delay between retries
	RetryDelay time.Duration
	// CDRTTL is how long CDRs are kept in Redis
	CDRTTL time.Duration
	// EnableDeduplication enables CDR deduplication
	EnableDeduplication bool
	// DeduplicationWindow is the time window for deduplication
	DeduplicationWindow time.Duration
}

// DefaultCDRCoordinatorConfig returns default configuration
func DefaultCDRCoordinatorConfig() *CDRCoordinatorConfig {
	return &CDRCoordinatorConfig{
		RedisPrefix:         "cdr:",
		FlushInterval:       5 * time.Second,
		BatchSize:           100,
		RetryAttempts:       3,
		RetryDelay:          1 * time.Second,
		CDRTTL:              24 * time.Hour,
		EnableDeduplication: true,
		DeduplicationWindow: 5 * time.Minute,
	}
}

// CDRCoordinator coordinates CDR generation across cluster nodes
type CDRCoordinator struct {
	config  *CDRCoordinatorConfig
	cluster *RedisSessionStore

	mu              sync.Mutex
	pendingCDRs     []*DistributedCDR
	processedIDs    map[string]time.Time
	exporters       []DistributedCDRExporter
	aggregators     map[string]*CDRAggregator

	stopChan chan struct{}
	doneChan chan struct{}
}

// DistributedCDR represents a CDR that can be coordinated across nodes
type DistributedCDR struct {
	// Unique CDR identifier
	ID string `json:"id"`
	// Call ID this CDR belongs to
	CallID string `json:"call_id"`
	// Node that generated this CDR
	OriginNode string `json:"origin_node"`
	// CDR type (call, leg, interim, final)
	Type CDRType `json:"type"`
	// Timestamp when CDR was generated
	Timestamp time.Time `json:"timestamp"`
	// Call start time
	StartTime time.Time `json:"start_time"`
	// Call end time (zero for interim CDRs)
	EndTime time.Time `json:"end_time,omitempty"`
	// Duration in seconds
	Duration float64 `json:"duration"`
	// Caller information
	Caller *CDRParty `json:"caller"`
	// Callee information
	Callee *CDRParty `json:"callee"`
	// Media statistics
	MediaStats *CDRMediaStats `json:"media_stats,omitempty"`
	// Recording information
	Recording *CDRRecording `json:"recording,omitempty"`
	// Quality metrics
	Quality *CDRQuality `json:"quality,omitempty"`
	// Custom metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	// Sequence number for ordering
	Sequence int64 `json:"sequence"`
	// Is this CDR finalized
	Finalized bool `json:"finalized"`
	// Nodes that have seen this CDR
	SeenBy []string `json:"seen_by,omitempty"`
}

// CDRType represents the type of CDR
type CDRType string

const (
	CDRTypeCall    CDRType = "call"
	CDRTypeLeg     CDRType = "leg"
	CDRTypeInterim CDRType = "interim"
	CDRTypeFinal   CDRType = "final"
)

// CDRParty represents a call party
type CDRParty struct {
	URI       string `json:"uri"`
	Tag       string `json:"tag"`
	Label     string `json:"label,omitempty"`
	Address   string `json:"address"`
	Port      int    `json:"port"`
	UserAgent string `json:"user_agent,omitempty"`
}

// CDRMediaStats contains media statistics
type CDRMediaStats struct {
	PacketsSent     int64   `json:"packets_sent"`
	PacketsReceived int64   `json:"packets_received"`
	BytesSent       int64   `json:"bytes_sent"`
	BytesReceived   int64   `json:"bytes_received"`
	PacketsLost     int64   `json:"packets_lost"`
	Jitter          float64 `json:"jitter_ms"`
	Latency         float64 `json:"latency_ms"`
	Codec           string  `json:"codec"`
	PayloadType     int     `json:"payload_type"`
	SSRC            uint32  `json:"ssrc"`
}

// CDRRecording contains recording information
type CDRRecording struct {
	Enabled   bool      `json:"enabled"`
	Path      string    `json:"path,omitempty"`
	Format    string    `json:"format,omitempty"`
	Duration  float64   `json:"duration,omitempty"`
	Size      int64     `json:"size,omitempty"`
	StartTime time.Time `json:"start_time,omitempty"`
	EndTime   time.Time `json:"end_time,omitempty"`
}

// CDRQuality contains quality metrics
type CDRQuality struct {
	MOS          float64 `json:"mos,omitempty"`
	RFactor      float64 `json:"r_factor,omitempty"`
	PacketLoss   float64 `json:"packet_loss_pct"`
	Jitter       float64 `json:"jitter_ms"`
	Latency      float64 `json:"latency_ms"`
	BurstDensity float64 `json:"burst_density,omitempty"`
	GapDensity   float64 `json:"gap_density,omitempty"`
}

// DistributedCDRExporter exports distributed CDRs to external systems
type DistributedCDRExporter interface {
	Export(ctx context.Context, cdr *DistributedCDR) error
	BatchExport(ctx context.Context, cdrs []*DistributedCDR) error
	Name() string
}

// CDRAggregator aggregates partial CDRs
type CDRAggregator struct {
	CallID       string
	PartialCDRs  []*DistributedCDR
	LastUpdated  time.Time
	TotalPackets int64
	TotalBytes   int64
	mu           sync.Mutex
}

// NewCDRCoordinator creates a new CDR coordinator
func NewCDRCoordinator(cluster *RedisSessionStore, config *CDRCoordinatorConfig) *CDRCoordinator {
	if config == nil {
		config = DefaultCDRCoordinatorConfig()
	}

	return &CDRCoordinator{
		config:       config,
		cluster:      cluster,
		pendingCDRs:  make([]*DistributedCDR, 0),
		processedIDs: make(map[string]time.Time),
		exporters:    make([]DistributedCDRExporter, 0),
		aggregators:  make(map[string]*CDRAggregator),
		stopChan:     make(chan struct{}),
		doneChan:     make(chan struct{}),
	}
}

// Start starts the CDR coordinator
func (cc *CDRCoordinator) Start() {
	go cc.flushLoop()
	go cc.cleanupLoop()
	go cc.subscribeLoop()
}

// Stop stops the CDR coordinator
func (cc *CDRCoordinator) Stop() {
	close(cc.stopChan)
	<-cc.doneChan

	// Final flush
	cc.flush()
}

// AddExporter adds a CDR exporter
func (cc *CDRCoordinator) AddExporter(exporter DistributedCDRExporter) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.exporters = append(cc.exporters, exporter)
}

// RecordCDR records a CDR
func (cc *CDRCoordinator) RecordCDR(cdr *DistributedCDR) error {
	cdr.OriginNode = cc.config.NodeID
	cdr.Timestamp = time.Now()

	if cdr.ID == "" {
		cdr.ID = generateDistributedCDRID()
	}

	// Check for duplicates
	if cc.config.EnableDeduplication && cc.isDuplicate(cdr) {
		return nil
	}

	// Store locally
	cc.mu.Lock()
	cc.pendingCDRs = append(cc.pendingCDRs, cdr)
	cc.processedIDs[cdr.ID] = time.Now()
	cc.mu.Unlock()

	// Store in Redis for coordination
	if cc.cluster != nil {
		if err := cc.storeCDR(cdr); err != nil {
			// Log error but don't fail - local storage succeeded
			_ = err
		}

		// Publish for other nodes
		cc.publishCDR(cdr)
	}

	// If batch is full, trigger immediate flush
	cc.mu.Lock()
	shouldFlush := len(cc.pendingCDRs) >= cc.config.BatchSize
	cc.mu.Unlock()

	if shouldFlush {
		go cc.flush()
	}

	return nil
}

// RecordInterimCDR records an interim CDR for long calls
func (cc *CDRCoordinator) RecordInterimCDR(callID string, stats *CDRMediaStats) error {
	cdr := &DistributedCDR{
		ID:         generateDistributedCDRID(),
		CallID:     callID,
		Type:       CDRTypeInterim,
		Timestamp:  time.Now(),
		MediaStats: stats,
		Finalized:  false,
	}

	return cc.RecordCDR(cdr)
}

// FinalizeCDR finalizes a call's CDR
func (cc *CDRCoordinator) FinalizeCDR(callID string, endTime time.Time) error {
	// Get aggregated data
	cc.mu.Lock()
	agg, exists := cc.aggregators[callID]
	if exists {
		agg.mu.Lock()
	}
	cc.mu.Unlock()

	if !exists {
		// No interim CDRs - create final directly
		return nil
	}

	// Merge all partial CDRs
	finalCDR := cc.mergePartialCDRs(agg.PartialCDRs)
	finalCDR.Type = CDRTypeFinal
	finalCDR.EndTime = endTime
	finalCDR.Duration = endTime.Sub(finalCDR.StartTime).Seconds()
	finalCDR.Finalized = true

	agg.mu.Unlock()

	// Remove aggregator
	cc.mu.Lock()
	delete(cc.aggregators, callID)
	cc.mu.Unlock()

	return cc.RecordCDR(finalCDR)
}

func (cc *CDRCoordinator) mergePartialCDRs(partials []*DistributedCDR) *DistributedCDR {
	if len(partials) == 0 {
		return &DistributedCDR{ID: generateDistributedCDRID()}
	}

	// Use first CDR as base
	merged := &DistributedCDR{
		ID:        generateDistributedCDRID(),
		CallID:    partials[0].CallID,
		StartTime: partials[0].StartTime,
		Caller:    partials[0].Caller,
		Callee:    partials[0].Callee,
		Metadata:  make(map[string]interface{}),
	}

	// Aggregate media stats
	stats := &CDRMediaStats{}
	for _, p := range partials {
		if p.MediaStats != nil {
			stats.PacketsSent += p.MediaStats.PacketsSent
			stats.PacketsReceived += p.MediaStats.PacketsReceived
			stats.BytesSent += p.MediaStats.BytesSent
			stats.BytesReceived += p.MediaStats.BytesReceived
			stats.PacketsLost += p.MediaStats.PacketsLost
		}

		// Merge metadata
		for k, v := range p.Metadata {
			merged.Metadata[k] = v
		}

		// Find earliest start time
		if !p.StartTime.IsZero() && (merged.StartTime.IsZero() || p.StartTime.Before(merged.StartTime)) {
			merged.StartTime = p.StartTime
		}
	}

	merged.MediaStats = stats

	// Calculate quality metrics
	if stats.PacketsSent > 0 {
		merged.Quality = &CDRQuality{
			PacketLoss: float64(stats.PacketsLost) / float64(stats.PacketsSent) * 100,
		}
	}

	return merged
}

func (cc *CDRCoordinator) isDuplicate(cdr *DistributedCDR) bool {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if processedAt, exists := cc.processedIDs[cdr.ID]; exists {
		if time.Since(processedAt) < cc.config.DeduplicationWindow {
			return true
		}
	}
	return false
}

func (cc *CDRCoordinator) storeCDR(cdr *DistributedCDR) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := json.Marshal(cdr)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s%s", cc.config.RedisPrefix, cdr.ID)
	return cc.cluster.client.Set(ctx, key, string(data), cc.config.CDRTTL)
}

func (cc *CDRCoordinator) publishCDR(cdr *DistributedCDR) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := json.Marshal(cdr)
	if err != nil {
		return
	}

	cc.cluster.client.Publish(ctx, "cdr:events", string(data))
}

func (cc *CDRCoordinator) flushLoop() {
	ticker := time.NewTicker(cc.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cc.stopChan:
			return
		case <-ticker.C:
			cc.flush()
		}
	}
}

func (cc *CDRCoordinator) flush() {
	cc.mu.Lock()
	if len(cc.pendingCDRs) == 0 {
		cc.mu.Unlock()
		return
	}

	cdrs := cc.pendingCDRs
	cc.pendingCDRs = make([]*DistributedCDR, 0, cc.config.BatchSize)
	exporters := cc.exporters
	cc.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Export to all exporters
	for _, exporter := range exporters {
		if err := exporter.BatchExport(ctx, cdrs); err != nil {
			// Retry individual CDRs
			for _, cdr := range cdrs {
				for attempt := 0; attempt < cc.config.RetryAttempts; attempt++ {
					if err := exporter.Export(ctx, cdr); err == nil {
						break
					}
					time.Sleep(cc.config.RetryDelay)
				}
			}
		}
	}
}

func (cc *CDRCoordinator) cleanupLoop() {
	defer close(cc.doneChan)

	ticker := time.NewTicker(cc.config.DeduplicationWindow)
	defer ticker.Stop()

	for {
		select {
		case <-cc.stopChan:
			return
		case <-ticker.C:
			cc.cleanupProcessedIDs()
			cc.cleanupStaleAggregators()
		}
	}
}

func (cc *CDRCoordinator) cleanupProcessedIDs() {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cutoff := time.Now().Add(-cc.config.DeduplicationWindow)
	for id, processedAt := range cc.processedIDs {
		if processedAt.Before(cutoff) {
			delete(cc.processedIDs, id)
		}
	}
}

func (cc *CDRCoordinator) cleanupStaleAggregators() {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	// Remove aggregators that haven't been updated in a long time
	staleThreshold := time.Now().Add(-1 * time.Hour)
	for callID, agg := range cc.aggregators {
		agg.mu.Lock()
		if agg.LastUpdated.Before(staleThreshold) {
			delete(cc.aggregators, callID)
		}
		agg.mu.Unlock()
	}
}

func (cc *CDRCoordinator) subscribeLoop() {
	if cc.cluster == nil {
		return
	}

	ctx := context.Background()
	pubsub, err := cc.cluster.client.Subscribe(ctx, "cdr:events")
	if err != nil {
		return
	}
	defer pubsub.Close()

	for {
		select {
		case <-cc.stopChan:
			return
		default:
			msg, err := pubsub.Receive(ctx)
			if err != nil {
				continue
			}

			// Extract payload from message
			payload, ok := msg.(string)
			if !ok {
				continue
			}

			var cdr DistributedCDR
			if err := json.Unmarshal([]byte(payload), &cdr); err != nil {
				continue
			}

			// Skip CDRs from this node
			if cdr.OriginNode == cc.config.NodeID {
				continue
			}

			// Mark as seen
			cdr.SeenBy = append(cdr.SeenBy, cc.config.NodeID)

			// Process received CDR
			cc.processReceivedCDR(&cdr)
		}
	}
}

func (cc *CDRCoordinator) processReceivedCDR(cdr *DistributedCDR) {
	// Check for duplicates
	if cc.config.EnableDeduplication && cc.isDuplicate(cdr) {
		return
	}

	cc.mu.Lock()
	cc.processedIDs[cdr.ID] = time.Now()

	// Add to aggregator if interim
	if cdr.Type == CDRTypeInterim {
		agg, exists := cc.aggregators[cdr.CallID]
		if !exists {
			agg = &CDRAggregator{
				CallID:      cdr.CallID,
				PartialCDRs: make([]*DistributedCDR, 0),
			}
			cc.aggregators[cdr.CallID] = agg
		}
		agg.mu.Lock()
		agg.PartialCDRs = append(agg.PartialCDRs, cdr)
		agg.LastUpdated = time.Now()
		agg.mu.Unlock()
	}
	cc.mu.Unlock()
}

// GetPendingCount returns the number of pending CDRs
func (cc *CDRCoordinator) GetPendingCount() int {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return len(cc.pendingCDRs)
}

// GetStats returns CDR coordinator statistics
func (cc *CDRCoordinator) GetStats() *CDRCoordinatorStats {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	return &CDRCoordinatorStats{
		PendingCDRs:      len(cc.pendingCDRs),
		ProcessedIDs:     len(cc.processedIDs),
		ActiveAggregators: len(cc.aggregators),
		ExporterCount:    len(cc.exporters),
	}
}

// CDRCoordinatorStats contains coordinator statistics
type CDRCoordinatorStats struct {
	PendingCDRs       int
	ProcessedIDs      int
	ActiveAggregators int
	ExporterCount     int
}

func generateDistributedCDRID() string {
	return fmt.Sprintf("cdr-%d-%d", time.Now().UnixNano(), time.Now().UnixMicro()%10000)
}

// JSONCDRExporter exports CDRs as JSON files
type JSONCDRExporter struct {
	filePath string
	file     *os.File
	mu       sync.Mutex
}

// NewJSONCDRExporter creates a JSON CDR exporter
func NewJSONCDRExporter(filePath string) *JSONCDRExporter {
	return &JSONCDRExporter{
		filePath: filePath,
	}
}

func (e *JSONCDRExporter) Name() string {
	return "json"
}

func (e *JSONCDRExporter) Export(ctx context.Context, cdr *DistributedCDR) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.file == nil {
		f, err := os.OpenFile(e.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		e.file = f
	}

	data, err := json.Marshal(cdr)
	if err != nil {
		return err
	}

	_, err = e.file.Write(append(data, '\n'))
	return err
}

func (e *JSONCDRExporter) BatchExport(ctx context.Context, cdrs []*DistributedCDR) error {
	for _, cdr := range cdrs {
		if err := e.Export(ctx, cdr); err != nil {
			return err
		}
	}
	return nil
}

func (e *JSONCDRExporter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.file != nil {
		err := e.file.Close()
		e.file = nil
		return err
	}
	return nil
}
