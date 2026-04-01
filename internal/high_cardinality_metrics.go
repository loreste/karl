package internal

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// HighCardinalityMetricsConfig configures high-cardinality metrics
type HighCardinalityMetricsConfig struct {
	// Enabled controls whether per-call metrics are collected
	Enabled bool
	// MaxCalls limits the number of concurrent calls tracked
	MaxCalls int
	// MetricsTTL is how long to keep metrics after call ends
	MetricsTTL time.Duration
	// SamplingRate for high-volume metrics (0.0-1.0)
	SamplingRate float64
	// ExportInterval for batch export
	ExportInterval time.Duration
}

// DefaultHighCardinalityMetricsConfig returns default configuration
func DefaultHighCardinalityMetricsConfig() *HighCardinalityMetricsConfig {
	return &HighCardinalityMetricsConfig{
		Enabled:        false, // Off by default
		MaxCalls:       10000,
		MetricsTTL:     5 * time.Minute,
		SamplingRate:   1.0,
		ExportInterval: 10 * time.Second,
	}
}

// CallMetrics holds per-call metrics
type CallMetrics struct {
	CallID    string
	FromTag   string
	ToTag     string
	StartTime time.Time
	EndTime   time.Time

	// Packet counts
	PacketsReceived atomic.Int64
	PacketsSent     atomic.Int64
	PacketsLost     atomic.Int64
	PacketsDropped  atomic.Int64

	// Byte counts
	BytesReceived atomic.Int64
	BytesSent     atomic.Int64

	// Quality metrics
	JitterSum      atomic.Int64  // Microseconds sum for averaging
	JitterCount    atomic.Int64
	LatencySum     atomic.Int64  // Microseconds sum for averaging
	LatencyCount   atomic.Int64
	MaxJitter      atomic.Int64  // Microseconds
	MaxLatency     atomic.Int64  // Microseconds

	// RTCP metrics
	RTCPPacketsReceived atomic.Int64
	RTCPPacketsSent     atomic.Int64
	RTCPBytes           atomic.Int64

	// Error counts
	DecryptErrors   atomic.Int64
	EncryptErrors   atomic.Int64
	SequenceErrors  atomic.Int64
	TimestampErrors atomic.Int64

	// Codec info
	Codec      string
	SampleRate int
	Channels   int

	// Recording
	RecordingEnabled bool
	RecordingBytes   atomic.Int64

	// Last update
	lastUpdate atomic.Int64
}

// NewCallMetrics creates new call metrics
func NewCallMetrics(callID, fromTag, toTag string) *CallMetrics {
	cm := &CallMetrics{
		CallID:    callID,
		FromTag:   fromTag,
		ToTag:     toTag,
		StartTime: time.Now(),
	}
	cm.lastUpdate.Store(time.Now().UnixNano())
	return cm
}

// RecordPacketReceived records a received packet
func (cm *CallMetrics) RecordPacketReceived(bytes int) {
	cm.PacketsReceived.Add(1)
	cm.BytesReceived.Add(int64(bytes))
	cm.lastUpdate.Store(time.Now().UnixNano())
}

// RecordPacketSent records a sent packet
func (cm *CallMetrics) RecordPacketSent(bytes int) {
	cm.PacketsSent.Add(1)
	cm.BytesSent.Add(int64(bytes))
	cm.lastUpdate.Store(time.Now().UnixNano())
}

// RecordPacketLost records a lost packet
func (cm *CallMetrics) RecordPacketLost() {
	cm.PacketsLost.Add(1)
	cm.lastUpdate.Store(time.Now().UnixNano())
}

// RecordPacketDropped records a dropped packet
func (cm *CallMetrics) RecordPacketDropped() {
	cm.PacketsDropped.Add(1)
	cm.lastUpdate.Store(time.Now().UnixNano())
}

// RecordJitter records jitter measurement in microseconds
func (cm *CallMetrics) RecordJitter(jitterUs int64) {
	cm.JitterSum.Add(jitterUs)
	cm.JitterCount.Add(1)
	// Update max
	for {
		old := cm.MaxJitter.Load()
		if jitterUs <= old {
			break
		}
		if cm.MaxJitter.CompareAndSwap(old, jitterUs) {
			break
		}
	}
	cm.lastUpdate.Store(time.Now().UnixNano())
}

// RecordLatency records latency measurement in microseconds
func (cm *CallMetrics) RecordLatency(latencyUs int64) {
	cm.LatencySum.Add(latencyUs)
	cm.LatencyCount.Add(1)
	// Update max
	for {
		old := cm.MaxLatency.Load()
		if latencyUs <= old {
			break
		}
		if cm.MaxLatency.CompareAndSwap(old, latencyUs) {
			break
		}
	}
	cm.lastUpdate.Store(time.Now().UnixNano())
}

// RecordRTCP records RTCP packet
func (cm *CallMetrics) RecordRTCP(bytes int, sent bool) {
	if sent {
		cm.RTCPPacketsSent.Add(1)
	} else {
		cm.RTCPPacketsReceived.Add(1)
	}
	cm.RTCPBytes.Add(int64(bytes))
	cm.lastUpdate.Store(time.Now().UnixNano())
}

// RecordError records an error
func (cm *CallMetrics) RecordError(errorType string) {
	switch errorType {
	case "decrypt":
		cm.DecryptErrors.Add(1)
	case "encrypt":
		cm.EncryptErrors.Add(1)
	case "sequence":
		cm.SequenceErrors.Add(1)
	case "timestamp":
		cm.TimestampErrors.Add(1)
	}
	cm.lastUpdate.Store(time.Now().UnixNano())
}

// RecordRecording records recording bytes
func (cm *CallMetrics) RecordRecording(bytes int) {
	cm.RecordingBytes.Add(int64(bytes))
	cm.lastUpdate.Store(time.Now().UnixNano())
}

// GetAverageJitter returns average jitter in microseconds
func (cm *CallMetrics) GetAverageJitter() float64 {
	count := cm.JitterCount.Load()
	if count == 0 {
		return 0
	}
	return float64(cm.JitterSum.Load()) / float64(count)
}

// GetAverageLatency returns average latency in microseconds
func (cm *CallMetrics) GetAverageLatency() float64 {
	count := cm.LatencyCount.Load()
	if count == 0 {
		return 0
	}
	return float64(cm.LatencySum.Load()) / float64(count)
}

// GetPacketLossRate returns packet loss rate (0.0-1.0)
func (cm *CallMetrics) GetPacketLossRate() float64 {
	total := cm.PacketsReceived.Load() + cm.PacketsLost.Load()
	if total == 0 {
		return 0
	}
	return float64(cm.PacketsLost.Load()) / float64(total)
}

// Duration returns call duration
func (cm *CallMetrics) Duration() time.Duration {
	end := cm.EndTime
	if end.IsZero() {
		end = time.Now()
	}
	return end.Sub(cm.StartTime)
}

// ToMap converts metrics to a map for export
func (cm *CallMetrics) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"call_id":              cm.CallID,
		"from_tag":             cm.FromTag,
		"to_tag":               cm.ToTag,
		"start_time":           cm.StartTime.Unix(),
		"duration_ms":          cm.Duration().Milliseconds(),
		"packets_received":     cm.PacketsReceived.Load(),
		"packets_sent":         cm.PacketsSent.Load(),
		"packets_lost":         cm.PacketsLost.Load(),
		"packets_dropped":      cm.PacketsDropped.Load(),
		"bytes_received":       cm.BytesReceived.Load(),
		"bytes_sent":           cm.BytesSent.Load(),
		"avg_jitter_us":        cm.GetAverageJitter(),
		"max_jitter_us":        cm.MaxJitter.Load(),
		"avg_latency_us":       cm.GetAverageLatency(),
		"max_latency_us":       cm.MaxLatency.Load(),
		"packet_loss_rate":     cm.GetPacketLossRate(),
		"rtcp_received":        cm.RTCPPacketsReceived.Load(),
		"rtcp_sent":            cm.RTCPPacketsSent.Load(),
		"rtcp_bytes":           cm.RTCPBytes.Load(),
		"decrypt_errors":       cm.DecryptErrors.Load(),
		"encrypt_errors":       cm.EncryptErrors.Load(),
		"sequence_errors":      cm.SequenceErrors.Load(),
		"timestamp_errors":     cm.TimestampErrors.Load(),
		"codec":                cm.Codec,
		"sample_rate":          cm.SampleRate,
		"channels":             cm.Channels,
		"recording_enabled":    cm.RecordingEnabled,
		"recording_bytes":      cm.RecordingBytes.Load(),
	}
}

// HighCardinalityMetrics manages per-call metrics
type HighCardinalityMetrics struct {
	config *HighCardinalityMetricsConfig

	mu      sync.RWMutex
	calls   map[string]*CallMetrics
	expired []*CallMetrics // Recently expired for export

	// Prometheus collectors (optional)
	callsActive      prometheus.Gauge
	packetsTotal     *prometheus.CounterVec
	bytesTotal       *prometheus.CounterVec
	errorsTotal      *prometheus.CounterVec
	jitterHistogram  *prometheus.HistogramVec
	latencyHistogram *prometheus.HistogramVec

	// Export callback
	exportFn func([]*CallMetrics)

	stopChan chan struct{}
	doneChan chan struct{}
}

// NewHighCardinalityMetrics creates a new high-cardinality metrics collector
func NewHighCardinalityMetrics(config *HighCardinalityMetricsConfig) *HighCardinalityMetrics {
	if config == nil {
		config = DefaultHighCardinalityMetricsConfig()
	}

	hcm := &HighCardinalityMetrics{
		config:   config,
		calls:    make(map[string]*CallMetrics),
		expired:  make([]*CallMetrics, 0),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	// Initialize Prometheus metrics
	hcm.callsActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "karl_hc_calls_active",
		Help: "Number of active calls being tracked",
	})

	hcm.packetsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "karl_hc_packets_total",
		Help: "Total packets by call and direction",
	}, []string{"call_id", "direction"})

	hcm.bytesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "karl_hc_bytes_total",
		Help: "Total bytes by call and direction",
	}, []string{"call_id", "direction"})

	hcm.errorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "karl_hc_errors_total",
		Help: "Total errors by call and type",
	}, []string{"call_id", "error_type"})

	hcm.jitterHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "karl_hc_jitter_microseconds",
		Help:    "Jitter distribution by call",
		Buckets: []float64{100, 500, 1000, 5000, 10000, 50000, 100000},
	}, []string{"call_id"})

	hcm.latencyHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "karl_hc_latency_microseconds",
		Help:    "Latency distribution by call",
		Buckets: []float64{1000, 5000, 10000, 50000, 100000, 500000, 1000000},
	}, []string{"call_id"})

	return hcm
}

// Start starts the metrics collector
func (hcm *HighCardinalityMetrics) Start() {
	go hcm.cleanupLoop()
}

// Stop stops the metrics collector
func (hcm *HighCardinalityMetrics) Stop() {
	close(hcm.stopChan)
	<-hcm.doneChan
}

// IsEnabled returns whether high-cardinality metrics are enabled
func (hcm *HighCardinalityMetrics) IsEnabled() bool {
	return hcm.config.Enabled
}

// SetEnabled enables or disables high-cardinality metrics
func (hcm *HighCardinalityMetrics) SetEnabled(enabled bool) {
	hcm.config.Enabled = enabled
}

// SetExportCallback sets the callback for expired metrics
func (hcm *HighCardinalityMetrics) SetExportCallback(fn func([]*CallMetrics)) {
	hcm.mu.Lock()
	defer hcm.mu.Unlock()
	hcm.exportFn = fn
}

// GetOrCreateCallMetrics gets or creates metrics for a call
func (hcm *HighCardinalityMetrics) GetOrCreateCallMetrics(callID, fromTag, toTag string) *CallMetrics {
	if !hcm.config.Enabled {
		return nil
	}

	key := callID + ":" + fromTag + ":" + toTag

	hcm.mu.RLock()
	if cm, ok := hcm.calls[key]; ok {
		hcm.mu.RUnlock()
		return cm
	}
	hcm.mu.RUnlock()

	hcm.mu.Lock()
	defer hcm.mu.Unlock()

	// Double-check
	if cm, ok := hcm.calls[key]; ok {
		return cm
	}

	// Check limit
	if len(hcm.calls) >= hcm.config.MaxCalls {
		return nil
	}

	cm := NewCallMetrics(callID, fromTag, toTag)
	hcm.calls[key] = cm
	hcm.callsActive.Inc()

	return cm
}

// GetCallMetrics gets metrics for a call
func (hcm *HighCardinalityMetrics) GetCallMetrics(callID, fromTag, toTag string) *CallMetrics {
	if !hcm.config.Enabled {
		return nil
	}

	key := callID + ":" + fromTag + ":" + toTag

	hcm.mu.RLock()
	defer hcm.mu.RUnlock()
	return hcm.calls[key]
}

// EndCall marks a call as ended
func (hcm *HighCardinalityMetrics) EndCall(callID, fromTag, toTag string) {
	if !hcm.config.Enabled {
		return
	}

	key := callID + ":" + fromTag + ":" + toTag

	hcm.mu.Lock()
	defer hcm.mu.Unlock()

	if cm, ok := hcm.calls[key]; ok {
		cm.EndTime = time.Now()
		cm.lastUpdate.Store(time.Now().UnixNano())
	}
}

// RemoveCall removes a call from tracking
func (hcm *HighCardinalityMetrics) RemoveCall(callID, fromTag, toTag string) *CallMetrics {
	if !hcm.config.Enabled {
		return nil
	}

	key := callID + ":" + fromTag + ":" + toTag

	hcm.mu.Lock()
	defer hcm.mu.Unlock()

	if cm, ok := hcm.calls[key]; ok {
		delete(hcm.calls, key)
		hcm.callsActive.Dec()
		hcm.expired = append(hcm.expired, cm)
		return cm
	}
	return nil
}

// GetAllCallMetrics returns all current call metrics
func (hcm *HighCardinalityMetrics) GetAllCallMetrics() []*CallMetrics {
	hcm.mu.RLock()
	defer hcm.mu.RUnlock()

	result := make([]*CallMetrics, 0, len(hcm.calls))
	for _, cm := range hcm.calls {
		result = append(result, cm)
	}
	return result
}

// GetActiveCallCount returns the number of active calls
func (hcm *HighCardinalityMetrics) GetActiveCallCount() int {
	hcm.mu.RLock()
	defer hcm.mu.RUnlock()
	return len(hcm.calls)
}

// GetExpiredMetrics returns and clears expired metrics
func (hcm *HighCardinalityMetrics) GetExpiredMetrics() []*CallMetrics {
	hcm.mu.Lock()
	defer hcm.mu.Unlock()

	expired := hcm.expired
	hcm.expired = make([]*CallMetrics, 0)
	return expired
}

// Collect implements prometheus.Collector
func (hcm *HighCardinalityMetrics) Collect(ch chan<- prometheus.Metric) {
	hcm.callsActive.Collect(ch)
	hcm.packetsTotal.Collect(ch)
	hcm.bytesTotal.Collect(ch)
	hcm.errorsTotal.Collect(ch)
	hcm.jitterHistogram.Collect(ch)
	hcm.latencyHistogram.Collect(ch)
}

// Describe implements prometheus.Collector
func (hcm *HighCardinalityMetrics) Describe(ch chan<- *prometheus.Desc) {
	hcm.callsActive.Describe(ch)
	hcm.packetsTotal.Describe(ch)
	hcm.bytesTotal.Describe(ch)
	hcm.errorsTotal.Describe(ch)
	hcm.jitterHistogram.Describe(ch)
	hcm.latencyHistogram.Describe(ch)
}

func (hcm *HighCardinalityMetrics) cleanupLoop() {
	defer close(hcm.doneChan)

	ticker := time.NewTicker(hcm.config.ExportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-hcm.stopChan:
			// Final export
			hcm.exportExpired()
			return

		case <-ticker.C:
			hcm.cleanup()
			hcm.exportExpired()
		}
	}
}

func (hcm *HighCardinalityMetrics) cleanup() {
	hcm.mu.Lock()
	defer hcm.mu.Unlock()

	now := time.Now()
	ttlNanos := hcm.config.MetricsTTL.Nanoseconds()

	for key, cm := range hcm.calls {
		lastUpdate := cm.lastUpdate.Load()
		if !cm.EndTime.IsZero() && now.UnixNano()-lastUpdate > ttlNanos {
			delete(hcm.calls, key)
			hcm.callsActive.Dec()
			hcm.expired = append(hcm.expired, cm)
		}
	}
}

func (hcm *HighCardinalityMetrics) exportExpired() {
	hcm.mu.Lock()
	if hcm.exportFn == nil || len(hcm.expired) == 0 {
		hcm.mu.Unlock()
		return
	}
	expired := hcm.expired
	hcm.expired = make([]*CallMetrics, 0)
	exportFn := hcm.exportFn
	hcm.mu.Unlock()

	exportFn(expired)
}

// CallMetricsSummary provides aggregated metrics across all calls
type CallMetricsSummary struct {
	ActiveCalls     int
	TotalPacketsRx  int64
	TotalPacketsTx  int64
	TotalPacketsLost int64
	TotalBytesRx    int64
	TotalBytesTx    int64
	AvgJitterUs     float64
	AvgLatencyUs    float64
	AvgPacketLoss   float64
	TotalErrors     int64
}

// GetSummary returns aggregated metrics
func (hcm *HighCardinalityMetrics) GetSummary() *CallMetricsSummary {
	hcm.mu.RLock()
	defer hcm.mu.RUnlock()

	summary := &CallMetricsSummary{
		ActiveCalls: len(hcm.calls),
	}

	var jitterSum, latencySum float64
	var jitterCount, latencyCount int

	for _, cm := range hcm.calls {
		summary.TotalPacketsRx += cm.PacketsReceived.Load()
		summary.TotalPacketsTx += cm.PacketsSent.Load()
		summary.TotalPacketsLost += cm.PacketsLost.Load()
		summary.TotalBytesRx += cm.BytesReceived.Load()
		summary.TotalBytesTx += cm.BytesSent.Load()
		summary.TotalErrors += cm.DecryptErrors.Load() + cm.EncryptErrors.Load() +
			cm.SequenceErrors.Load() + cm.TimestampErrors.Load()

		if avgJitter := cm.GetAverageJitter(); avgJitter > 0 {
			jitterSum += avgJitter
			jitterCount++
		}
		if avgLatency := cm.GetAverageLatency(); avgLatency > 0 {
			latencySum += avgLatency
			latencyCount++
		}
	}

	if jitterCount > 0 {
		summary.AvgJitterUs = jitterSum / float64(jitterCount)
	}
	if latencyCount > 0 {
		summary.AvgLatencyUs = latencySum / float64(latencyCount)
	}

	totalPackets := summary.TotalPacketsRx + summary.TotalPacketsLost
	if totalPackets > 0 {
		summary.AvgPacketLoss = float64(summary.TotalPacketsLost) / float64(totalPackets)
	}

	return summary
}

// RecordMetrics records a batch of metrics to Prometheus
func (hcm *HighCardinalityMetrics) RecordMetrics(cm *CallMetrics) {
	if cm == nil || !hcm.config.Enabled {
		return
	}

	// Use sampling for high-volume metrics
	if hcm.config.SamplingRate < 1.0 {
		// Simple deterministic sampling based on packet count
		if cm.PacketsReceived.Load()%int64(1/hcm.config.SamplingRate) != 0 {
			return
		}
	}

	hcm.packetsTotal.WithLabelValues(cm.CallID, "received").Add(float64(cm.PacketsReceived.Load()))
	hcm.packetsTotal.WithLabelValues(cm.CallID, "sent").Add(float64(cm.PacketsSent.Load()))
	hcm.bytesTotal.WithLabelValues(cm.CallID, "received").Add(float64(cm.BytesReceived.Load()))
	hcm.bytesTotal.WithLabelValues(cm.CallID, "sent").Add(float64(cm.BytesSent.Load()))

	if jitter := cm.GetAverageJitter(); jitter > 0 {
		hcm.jitterHistogram.WithLabelValues(cm.CallID).Observe(jitter)
	}
	if latency := cm.GetAverageLatency(); latency > 0 {
		hcm.latencyHistogram.WithLabelValues(cm.CallID).Observe(latency)
	}
}

// CleanupCallMetrics removes Prometheus metrics for a specific call
func (hcm *HighCardinalityMetrics) CleanupCallMetrics(callID string) {
	hcm.packetsTotal.DeleteLabelValues(callID, "received")
	hcm.packetsTotal.DeleteLabelValues(callID, "sent")
	hcm.bytesTotal.DeleteLabelValues(callID, "received")
	hcm.bytesTotal.DeleteLabelValues(callID, "sent")
	hcm.errorsTotal.DeleteLabelValues(callID, "decrypt")
	hcm.errorsTotal.DeleteLabelValues(callID, "encrypt")
	hcm.errorsTotal.DeleteLabelValues(callID, "sequence")
	hcm.errorsTotal.DeleteLabelValues(callID, "timestamp")
	hcm.jitterHistogram.DeleteLabelValues(callID)
	hcm.latencyHistogram.DeleteLabelValues(callID)
}
