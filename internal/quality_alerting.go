package internal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// alertIDCounter ensures unique alert IDs even when generated in rapid succession
var alertIDCounter uint64

// AlertSeverity represents the severity level of an alert
type AlertSeverity string

const (
	AlertSeverityInfo     AlertSeverity = "info"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityCritical AlertSeverity = "critical"
)

// AlertType represents the type of quality alert
type AlertType string

const (
	AlertTypePacketLoss     AlertType = "packet_loss"
	AlertTypeJitter         AlertType = "jitter"
	AlertTypeLatency        AlertType = "latency"
	AlertTypeMOS            AlertType = "mos"
	AlertTypeCodecMismatch  AlertType = "codec_mismatch"
	AlertTypeMediaTimeout   AlertType = "media_timeout"
	AlertTypeRTPGap         AlertType = "rtp_gap"
	AlertTypeDTMFFailure    AlertType = "dtmf_failure"
	AlertTypeRecordingError AlertType = "recording_error"
	AlertTypeResourceLimit  AlertType = "resource_limit"
)

// QualityAlert represents a quality alert
type QualityAlert struct {
	ID          string
	Type        AlertType
	Severity    AlertSeverity
	CallID      string
	SessionID   string
	Message     string
	Value       float64
	Threshold   float64
	Timestamp   time.Time
	Metadata    map[string]interface{}
	Acknowledged bool
	AckedAt     time.Time
	AckedBy     string
}

// AlertThreshold defines thresholds for quality metrics
type AlertThreshold struct {
	MetricName    string
	WarningValue  float64
	CriticalValue float64
	Duration      time.Duration // How long the condition must persist
	Enabled       bool
}

// DefaultAlertThresholds returns default quality thresholds
func DefaultAlertThresholds() map[AlertType]*AlertThreshold {
	return map[AlertType]*AlertThreshold{
		AlertTypePacketLoss: {
			MetricName:    "packet_loss_percent",
			WarningValue:  1.0,  // 1% packet loss
			CriticalValue: 5.0,  // 5% packet loss
			Duration:      10 * time.Second,
			Enabled:       true,
		},
		AlertTypeJitter: {
			MetricName:    "jitter_ms",
			WarningValue:  30.0,  // 30ms jitter
			CriticalValue: 50.0,  // 50ms jitter
			Duration:      10 * time.Second,
			Enabled:       true,
		},
		AlertTypeLatency: {
			MetricName:    "latency_ms",
			WarningValue:  150.0, // 150ms latency
			CriticalValue: 300.0, // 300ms latency
			Duration:      10 * time.Second,
			Enabled:       true,
		},
		AlertTypeMOS: {
			MetricName:    "mos",
			WarningValue:  3.5, // MOS below 3.5
			CriticalValue: 3.0, // MOS below 3.0
			Duration:      30 * time.Second,
			Enabled:       true,
		},
	}
}

// AlertHandler is called when an alert is triggered
type AlertHandler func(alert *QualityAlert)

// QualityAlerter monitors quality metrics and triggers alerts
type QualityAlerter struct {
	config     *QualityAlerterConfig
	thresholds map[AlertType]*AlertThreshold
	handlers   []AlertHandler

	// Active alerts
	activeAlerts map[string]*QualityAlert
	alertsMu     sync.RWMutex

	// Alert history
	alertHistory    []*QualityAlert
	historyMu       sync.RWMutex
	maxHistory      int
	maxActiveAlerts int

	// Metric tracking for duration-based alerts
	metricState map[string]*metricState
	metricMu    sync.RWMutex

	// Alert suppression
	suppressions map[string]time.Time
	suppressMu   sync.RWMutex

	stopChan chan struct{}
	doneChan chan struct{}
}

type metricState struct {
	lastValue     float64
	violationStart time.Time
	isViolating   bool
}

// QualityAlerterConfig holds alerter configuration
type QualityAlerterConfig struct {
	CheckInterval      time.Duration
	MaxActiveAlerts    int
	MaxAlertHistory    int
	SuppressionPeriod  time.Duration
	AggregationWindow  time.Duration
}

// DefaultQualityAlerterConfig returns default configuration
func DefaultQualityAlerterConfig() *QualityAlerterConfig {
	return &QualityAlerterConfig{
		CheckInterval:     5 * time.Second,
		MaxActiveAlerts:   1000,
		MaxAlertHistory:   10000,
		SuppressionPeriod: 5 * time.Minute,
		AggregationWindow: 30 * time.Second,
	}
}

// NewQualityAlerter creates a new quality alerter
func NewQualityAlerter(config *QualityAlerterConfig) *QualityAlerter {
	if config == nil {
		config = DefaultQualityAlerterConfig()
	}

	// Apply defaults for unset values
	maxHistory := config.MaxAlertHistory
	if maxHistory <= 0 {
		maxHistory = 10000 // Default
	}
	maxActiveAlerts := config.MaxActiveAlerts
	if maxActiveAlerts <= 0 {
		maxActiveAlerts = 1000 // Default
	}

	return &QualityAlerter{
		config:          config,
		thresholds:      DefaultAlertThresholds(),
		handlers:        make([]AlertHandler, 0),
		activeAlerts:    make(map[string]*QualityAlert),
		alertHistory:    make([]*QualityAlert, 0, maxHistory),
		maxHistory:      maxHistory,
		maxActiveAlerts: maxActiveAlerts,
		metricState:     make(map[string]*metricState),
		suppressions:    make(map[string]time.Time),
		stopChan:        make(chan struct{}),
		doneChan:        make(chan struct{}),
	}
}

// AddHandler adds an alert handler
func (qa *QualityAlerter) AddHandler(handler AlertHandler) {
	qa.handlers = append(qa.handlers, handler)
}

// SetThreshold sets or updates an alert threshold
func (qa *QualityAlerter) SetThreshold(alertType AlertType, threshold *AlertThreshold) {
	qa.thresholds[alertType] = threshold
}

// GetThreshold gets an alert threshold
func (qa *QualityAlerter) GetThreshold(alertType AlertType) *AlertThreshold {
	return qa.thresholds[alertType]
}

// CheckMetric checks a metric value against thresholds
func (qa *QualityAlerter) CheckMetric(alertType AlertType, callID, sessionID string, value float64, metadata map[string]interface{}) {
	threshold, exists := qa.thresholds[alertType]
	if !exists || !threshold.Enabled {
		return
	}

	// Create unique key for this metric instance
	metricKey := fmt.Sprintf("%s:%s:%s", alertType, callID, sessionID)

	qa.metricMu.Lock()
	state, exists := qa.metricState[metricKey]
	if !exists {
		state = &metricState{}
		qa.metricState[metricKey] = state
	}

	state.lastValue = value

	// Check if value violates threshold (for MOS, lower is worse)
	var isViolating bool
	var severity AlertSeverity
	var thresholdValue float64

	if alertType == AlertTypeMOS {
		// For MOS, lower values are worse
		if value < threshold.CriticalValue {
			isViolating = true
			severity = AlertSeverityCritical
			thresholdValue = threshold.CriticalValue
		} else if value < threshold.WarningValue {
			isViolating = true
			severity = AlertSeverityWarning
			thresholdValue = threshold.WarningValue
		}
	} else {
		// For other metrics, higher values are worse
		if value >= threshold.CriticalValue {
			isViolating = true
			severity = AlertSeverityCritical
			thresholdValue = threshold.CriticalValue
		} else if value >= threshold.WarningValue {
			isViolating = true
			severity = AlertSeverityWarning
			thresholdValue = threshold.WarningValue
		}
	}

	now := time.Now()

	if isViolating {
		if !state.isViolating {
			state.isViolating = true
			state.violationStart = now
		}
		// Check if violation has persisted long enough (or Duration is 0 for immediate)
		if now.Sub(state.violationStart) >= threshold.Duration {
			qa.metricMu.Unlock()
			// Violation persisted long enough, trigger alert
			qa.triggerAlert(alertType, severity, callID, sessionID, value, thresholdValue, metadata)
			return
		}
	} else {
		state.isViolating = false
		state.violationStart = time.Time{}
	}
	qa.metricMu.Unlock()
}

// triggerAlert creates and dispatches an alert
func (qa *QualityAlerter) triggerAlert(alertType AlertType, severity AlertSeverity, callID, sessionID string, value, threshold float64, metadata map[string]interface{}) {
	// Check suppression
	alertKey := fmt.Sprintf("%s:%s:%s", alertType, callID, sessionID)

	qa.suppressMu.RLock()
	suppressUntil, suppressed := qa.suppressions[alertKey]
	qa.suppressMu.RUnlock()

	if suppressed && time.Now().Before(suppressUntil) {
		return
	}

	alert := &QualityAlert{
		ID:        generateAlertID(),
		Type:      alertType,
		Severity:  severity,
		CallID:    callID,
		SessionID: sessionID,
		Message:   formatAlertMessage(alertType, severity, value, threshold),
		Value:     value,
		Threshold: threshold,
		Timestamp: time.Now(),
		Metadata:  metadata,
	}

	// Add to active alerts
	qa.alertsMu.Lock()
	if len(qa.activeAlerts) < qa.maxActiveAlerts {
		qa.activeAlerts[alert.ID] = alert
	}
	qa.alertsMu.Unlock()

	// Add to history
	qa.historyMu.Lock()
	if qa.maxHistory > 0 {
		if len(qa.alertHistory) >= qa.maxHistory {
			// Remove oldest
			qa.alertHistory = qa.alertHistory[1:]
		}
		qa.alertHistory = append(qa.alertHistory, alert)
	}
	qa.historyMu.Unlock()

	// Set suppression
	qa.suppressMu.Lock()
	qa.suppressions[alertKey] = time.Now().Add(qa.config.SuppressionPeriod)
	qa.suppressMu.Unlock()

	// Notify handlers
	for _, handler := range qa.handlers {
		go handler(alert)
	}
}

// TriggerCustomAlert triggers a custom alert
func (qa *QualityAlerter) TriggerCustomAlert(alertType AlertType, severity AlertSeverity, callID, sessionID, message string, metadata map[string]interface{}) {
	alert := &QualityAlert{
		ID:        generateAlertID(),
		Type:      alertType,
		Severity:  severity,
		CallID:    callID,
		SessionID: sessionID,
		Message:   message,
		Timestamp: time.Now(),
		Metadata:  metadata,
	}

	qa.alertsMu.Lock()
	if len(qa.activeAlerts) < qa.maxActiveAlerts {
		qa.activeAlerts[alert.ID] = alert
	}
	qa.alertsMu.Unlock()

	qa.historyMu.Lock()
	if qa.maxHistory > 0 {
		if len(qa.alertHistory) >= qa.maxHistory {
			qa.alertHistory = qa.alertHistory[1:]
		}
		qa.alertHistory = append(qa.alertHistory, alert)
	}
	qa.historyMu.Unlock()

	for _, handler := range qa.handlers {
		go handler(alert)
	}
}

// AcknowledgeAlert acknowledges an active alert
func (qa *QualityAlerter) AcknowledgeAlert(alertID, ackedBy string) error {
	qa.alertsMu.Lock()
	defer qa.alertsMu.Unlock()

	alert, exists := qa.activeAlerts[alertID]
	if !exists {
		return fmt.Errorf("alert not found: %s", alertID)
	}

	alert.Acknowledged = true
	alert.AckedAt = time.Now()
	alert.AckedBy = ackedBy

	return nil
}

// ClearAlert removes an active alert
func (qa *QualityAlerter) ClearAlert(alertID string) {
	qa.alertsMu.Lock()
	defer qa.alertsMu.Unlock()
	delete(qa.activeAlerts, alertID)
}

// ClearCallAlerts clears all alerts for a call
func (qa *QualityAlerter) ClearCallAlerts(callID string) {
	qa.alertsMu.Lock()
	defer qa.alertsMu.Unlock()

	for id, alert := range qa.activeAlerts {
		if alert.CallID == callID {
			delete(qa.activeAlerts, id)
		}
	}

	// Clear metric state
	qa.metricMu.Lock()
	for key := range qa.metricState {
		if containsCallID(key, callID) {
			delete(qa.metricState, key)
		}
	}
	qa.metricMu.Unlock()
}

// GetActiveAlerts returns all active alerts
func (qa *QualityAlerter) GetActiveAlerts() []*QualityAlert {
	qa.alertsMu.RLock()
	defer qa.alertsMu.RUnlock()

	alerts := make([]*QualityAlert, 0, len(qa.activeAlerts))
	for _, alert := range qa.activeAlerts {
		alerts = append(alerts, alert)
	}
	return alerts
}

// GetActiveAlertsByCall returns active alerts for a specific call
func (qa *QualityAlerter) GetActiveAlertsByCall(callID string) []*QualityAlert {
	qa.alertsMu.RLock()
	defer qa.alertsMu.RUnlock()

	var alerts []*QualityAlert
	for _, alert := range qa.activeAlerts {
		if alert.CallID == callID {
			alerts = append(alerts, alert)
		}
	}
	return alerts
}

// GetAlertHistory returns alert history
func (qa *QualityAlerter) GetAlertHistory(limit int) []*QualityAlert {
	qa.historyMu.RLock()
	defer qa.historyMu.RUnlock()

	if limit <= 0 || limit > len(qa.alertHistory) {
		limit = len(qa.alertHistory)
	}

	// Return most recent alerts
	start := len(qa.alertHistory) - limit
	if start < 0 {
		start = 0
	}

	result := make([]*QualityAlert, limit)
	copy(result, qa.alertHistory[start:])
	return result
}

// GetAlertStats returns alert statistics
func (qa *QualityAlerter) GetAlertStats() *AlertStats {
	qa.alertsMu.RLock()
	activeCount := len(qa.activeAlerts)

	bySeverity := make(map[AlertSeverity]int)
	byType := make(map[AlertType]int)
	unacked := 0

	for _, alert := range qa.activeAlerts {
		bySeverity[alert.Severity]++
		byType[alert.Type]++
		if !alert.Acknowledged {
			unacked++
		}
	}
	qa.alertsMu.RUnlock()

	qa.historyMu.RLock()
	historyCount := len(qa.alertHistory)
	qa.historyMu.RUnlock()

	return &AlertStats{
		ActiveCount:      activeCount,
		HistoryCount:     historyCount,
		UnacknowledgedCount: unacked,
		BySeverity:       bySeverity,
		ByType:           byType,
	}
}

// AlertStats holds alert statistics
type AlertStats struct {
	ActiveCount         int
	HistoryCount        int
	UnacknowledgedCount int
	BySeverity          map[AlertSeverity]int
	ByType              map[AlertType]int
}

// Start starts the alerter background tasks
func (qa *QualityAlerter) Start(ctx context.Context) {
	go qa.cleanupLoop(ctx)
}

// Stop stops the alerter
func (qa *QualityAlerter) Stop() {
	close(qa.stopChan)
	<-qa.doneChan
}

func (qa *QualityAlerter) cleanupLoop(ctx context.Context) {
	defer close(qa.doneChan)

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-qa.stopChan:
			return
		case <-ticker.C:
			qa.cleanup()
		}
	}
}

func (qa *QualityAlerter) cleanup() {
	now := time.Now()

	// Clean up old suppressions
	qa.suppressMu.Lock()
	for key, until := range qa.suppressions {
		if now.After(until) {
			delete(qa.suppressions, key)
		}
	}
	qa.suppressMu.Unlock()

	// Clean up stale metric state (older than 10 minutes)
	qa.metricMu.Lock()
	staleThreshold := now.Add(-10 * time.Minute)
	for key, state := range qa.metricState {
		if state.violationStart.Before(staleThreshold) && !state.isViolating {
			delete(qa.metricState, key)
		}
	}
	qa.metricMu.Unlock()
}

// Helper functions

func generateAlertID() string {
	counter := atomic.AddUint64(&alertIDCounter, 1)
	return fmt.Sprintf("alert-%d-%d", time.Now().UnixNano(), counter)
}

func formatAlertMessage(alertType AlertType, severity AlertSeverity, value, threshold float64) string {
	switch alertType {
	case AlertTypePacketLoss:
		return fmt.Sprintf("Packet loss %.2f%% exceeds %s threshold (%.2f%%)", value, severity, threshold)
	case AlertTypeJitter:
		return fmt.Sprintf("Jitter %.2fms exceeds %s threshold (%.2fms)", value, severity, threshold)
	case AlertTypeLatency:
		return fmt.Sprintf("Latency %.2fms exceeds %s threshold (%.2fms)", value, severity, threshold)
	case AlertTypeMOS:
		return fmt.Sprintf("MOS score %.2f below %s threshold (%.2f)", value, severity, threshold)
	default:
		return fmt.Sprintf("%s alert: value %.2f, threshold %.2f", alertType, value, threshold)
	}
}

func containsCallID(key, callID string) bool {
	return len(key) > len(callID) && key[len(key)-len(callID)-1:len(key)-1] == callID
}

// WebhookAlertHandler sends alerts to a webhook
type WebhookAlertHandler struct {
	URL     string
	Headers map[string]string
}

// NewWebhookAlertHandler creates a new webhook handler
func NewWebhookAlertHandler(url string) *WebhookAlertHandler {
	return &WebhookAlertHandler{
		URL:     url,
		Headers: make(map[string]string),
	}
}

// Handle sends the alert to the webhook
func (h *WebhookAlertHandler) Handle(alert *QualityAlert) {
	// Implementation would use http.Post to send alert as JSON
	// Omitted for brevity - would be similar to existing webhook patterns
}

// LogAlertHandler logs alerts
type LogAlertHandler struct {
	Logger func(format string, args ...interface{})
}

// Handle logs the alert
func (h *LogAlertHandler) Handle(alert *QualityAlert) {
	if h.Logger != nil {
		h.Logger("[%s] %s alert for call %s: %s (value=%.2f, threshold=%.2f)",
			alert.Severity, alert.Type, alert.CallID, alert.Message, alert.Value, alert.Threshold)
	}
}
