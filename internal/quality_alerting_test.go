package internal

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestDefaultAlertThresholds(t *testing.T) {
	thresholds := DefaultAlertThresholds()

	if thresholds[AlertTypePacketLoss] == nil {
		t.Error("Missing packet loss threshold")
	}
	if thresholds[AlertTypeJitter] == nil {
		t.Error("Missing jitter threshold")
	}
	if thresholds[AlertTypeLatency] == nil {
		t.Error("Missing latency threshold")
	}
	if thresholds[AlertTypeMOS] == nil {
		t.Error("Missing MOS threshold")
	}
}

func TestDefaultQualityAlerterConfig(t *testing.T) {
	config := DefaultQualityAlerterConfig()

	if config.CheckInterval != 5*time.Second {
		t.Error("CheckInterval not set correctly")
	}
	if config.MaxActiveAlerts != 1000 {
		t.Error("MaxActiveAlerts not set correctly")
	}
}

func TestNewQualityAlerter(t *testing.T) {
	qa := NewQualityAlerter(nil)

	if qa == nil {
		t.Fatal("NewQualityAlerter returned nil")
	}
	if len(qa.thresholds) == 0 {
		t.Error("Thresholds not initialized")
	}
}

func TestQualityAlerter_AddHandler(t *testing.T) {
	qa := NewQualityAlerter(nil)

	qa.AddHandler(func(alert *QualityAlert) {
		// Handler added successfully
	})

	if len(qa.handlers) != 1 {
		t.Error("Handler not added")
	}
}

func TestQualityAlerter_SetThreshold(t *testing.T) {
	qa := NewQualityAlerter(nil)

	customThreshold := &AlertThreshold{
		MetricName:    "custom_metric",
		WarningValue:  10.0,
		CriticalValue: 20.0,
		Enabled:       true,
	}

	qa.SetThreshold(AlertTypePacketLoss, customThreshold)

	threshold := qa.GetThreshold(AlertTypePacketLoss)
	if threshold.WarningValue != 10.0 {
		t.Error("Threshold not updated")
	}
}

func TestQualityAlerter_CheckMetric_Warning(t *testing.T) {
	config := &QualityAlerterConfig{
		SuppressionPeriod: 0, // Disable suppression for testing
	}
	qa := NewQualityAlerter(config)

	// Set threshold with no duration requirement
	qa.SetThreshold(AlertTypePacketLoss, &AlertThreshold{
		MetricName:    "packet_loss",
		WarningValue:  1.0,
		CriticalValue: 5.0,
		Duration:      0, // Immediate
		Enabled:       true,
	})

	var receivedAlert *QualityAlert
	var mu sync.Mutex
	qa.AddHandler(func(alert *QualityAlert) {
		mu.Lock()
		receivedAlert = alert
		mu.Unlock()
	})

	// Check metric with warning value
	qa.CheckMetric(AlertTypePacketLoss, "call-123", "session-456", 2.0, nil)

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if receivedAlert == nil {
		t.Error("Expected warning alert to be triggered")
	} else if receivedAlert.Severity != AlertSeverityWarning {
		t.Errorf("Expected warning severity, got %s", receivedAlert.Severity)
	}
	mu.Unlock()
}

func TestQualityAlerter_CheckMetric_Critical(t *testing.T) {
	config := &QualityAlerterConfig{
		SuppressionPeriod: 0,
	}
	qa := NewQualityAlerter(config)

	qa.SetThreshold(AlertTypePacketLoss, &AlertThreshold{
		MetricName:    "packet_loss",
		WarningValue:  1.0,
		CriticalValue: 5.0,
		Duration:      0,
		Enabled:       true,
	})

	var receivedAlert *QualityAlert
	var mu sync.Mutex
	qa.AddHandler(func(alert *QualityAlert) {
		mu.Lock()
		receivedAlert = alert
		mu.Unlock()
	})

	// Check metric with critical value
	qa.CheckMetric(AlertTypePacketLoss, "call-123", "session-456", 10.0, nil)

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if receivedAlert == nil {
		t.Error("Expected critical alert to be triggered")
	} else if receivedAlert.Severity != AlertSeverityCritical {
		t.Errorf("Expected critical severity, got %s", receivedAlert.Severity)
	}
	mu.Unlock()
}

func TestQualityAlerter_CheckMetric_MOS(t *testing.T) {
	config := &QualityAlerterConfig{
		SuppressionPeriod: 0,
	}
	qa := NewQualityAlerter(config)

	qa.SetThreshold(AlertTypeMOS, &AlertThreshold{
		MetricName:    "mos",
		WarningValue:  3.5,
		CriticalValue: 3.0,
		Duration:      0,
		Enabled:       true,
	})

	var receivedAlert *QualityAlert
	var mu sync.Mutex
	qa.AddHandler(func(alert *QualityAlert) {
		mu.Lock()
		receivedAlert = alert
		mu.Unlock()
	})

	// Check MOS below critical (lower is worse for MOS)
	qa.CheckMetric(AlertTypeMOS, "call-123", "session-456", 2.5, nil)

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if receivedAlert == nil {
		t.Error("Expected MOS alert to be triggered")
	} else if receivedAlert.Severity != AlertSeverityCritical {
		t.Errorf("Expected critical severity for low MOS, got %s", receivedAlert.Severity)
	}
	mu.Unlock()
}

func TestQualityAlerter_CheckMetric_Disabled(t *testing.T) {
	qa := NewQualityAlerter(nil)

	qa.SetThreshold(AlertTypePacketLoss, &AlertThreshold{
		WarningValue: 1.0,
		Enabled:      false,
	})

	var alertTriggered bool
	qa.AddHandler(func(alert *QualityAlert) {
		alertTriggered = true
	})

	qa.CheckMetric(AlertTypePacketLoss, "call-123", "session-456", 10.0, nil)

	time.Sleep(50 * time.Millisecond)

	if alertTriggered {
		t.Error("Alert should not be triggered when threshold is disabled")
	}
}

func TestQualityAlerter_TriggerCustomAlert(t *testing.T) {
	qa := NewQualityAlerter(nil)

	var mu sync.Mutex
	var receivedAlert *QualityAlert
	qa.AddHandler(func(alert *QualityAlert) {
		mu.Lock()
		receivedAlert = alert
		mu.Unlock()
	})

	qa.TriggerCustomAlert(AlertTypeMediaTimeout, AlertSeverityCritical,
		"call-123", "session-456", "Media timeout detected",
		map[string]interface{}{"timeout": 30})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	alert := receivedAlert
	mu.Unlock()

	if alert == nil {
		t.Fatal("Custom alert not triggered")
	}
	if alert.Type != AlertTypeMediaTimeout {
		t.Error("Alert type mismatch")
	}
	if alert.Message != "Media timeout detected" {
		t.Error("Alert message mismatch")
	}
}

func TestQualityAlerter_GetActiveAlerts(t *testing.T) {
	qa := NewQualityAlerter(nil)

	qa.TriggerCustomAlert(AlertTypeMediaTimeout, AlertSeverityWarning,
		"call-1", "session-1", "Alert 1", nil)
	qa.TriggerCustomAlert(AlertTypeRTPGap, AlertSeverityCritical,
		"call-2", "session-2", "Alert 2", nil)

	time.Sleep(50 * time.Millisecond)

	alerts := qa.GetActiveAlerts()
	if len(alerts) != 2 {
		t.Errorf("Expected 2 active alerts, got %d", len(alerts))
	}
}

func TestQualityAlerter_GetActiveAlertsByCall(t *testing.T) {
	qa := NewQualityAlerter(nil)

	qa.TriggerCustomAlert(AlertTypeMediaTimeout, AlertSeverityWarning,
		"call-target", "session-1", "Alert 1", nil)
	qa.TriggerCustomAlert(AlertTypeRTPGap, AlertSeverityCritical,
		"call-target", "session-2", "Alert 2", nil)
	qa.TriggerCustomAlert(AlertTypeJitter, AlertSeverityWarning,
		"call-other", "session-3", "Alert 3", nil)

	time.Sleep(50 * time.Millisecond)

	alerts := qa.GetActiveAlertsByCall("call-target")
	if len(alerts) != 2 {
		t.Errorf("Expected 2 alerts for call-target, got %d", len(alerts))
	}
}

func TestQualityAlerter_AcknowledgeAlert(t *testing.T) {
	qa := NewQualityAlerter(nil)

	qa.TriggerCustomAlert(AlertTypeMediaTimeout, AlertSeverityWarning,
		"call-123", "session-456", "Test alert", nil)

	time.Sleep(50 * time.Millisecond)

	alerts := qa.GetActiveAlerts()
	if len(alerts) == 0 {
		t.Fatal("No alerts found")
	}

	alertID := alerts[0].ID
	err := qa.AcknowledgeAlert(alertID, "admin")
	if err != nil {
		t.Fatalf("AcknowledgeAlert failed: %v", err)
	}

	alerts = qa.GetActiveAlerts()
	if !alerts[0].Acknowledged {
		t.Error("Alert should be acknowledged")
	}
	if alerts[0].AckedBy != "admin" {
		t.Error("AckedBy not set correctly")
	}
}

func TestQualityAlerter_AcknowledgeAlert_NotFound(t *testing.T) {
	qa := NewQualityAlerter(nil)

	err := qa.AcknowledgeAlert("nonexistent", "admin")
	if err == nil {
		t.Error("Should return error for nonexistent alert")
	}
}

func TestQualityAlerter_ClearAlert(t *testing.T) {
	qa := NewQualityAlerter(nil)

	qa.TriggerCustomAlert(AlertTypeMediaTimeout, AlertSeverityWarning,
		"call-123", "session-456", "Test alert", nil)

	time.Sleep(50 * time.Millisecond)

	alerts := qa.GetActiveAlerts()
	if len(alerts) == 0 {
		t.Fatal("No alerts found")
	}

	qa.ClearAlert(alerts[0].ID)

	alerts = qa.GetActiveAlerts()
	if len(alerts) != 0 {
		t.Error("Alert should be cleared")
	}
}

func TestQualityAlerter_ClearCallAlerts(t *testing.T) {
	qa := NewQualityAlerter(nil)

	qa.TriggerCustomAlert(AlertTypeMediaTimeout, AlertSeverityWarning,
		"call-to-clear", "session-1", "Alert 1", nil)
	qa.TriggerCustomAlert(AlertTypeRTPGap, AlertSeverityCritical,
		"call-to-clear", "session-2", "Alert 2", nil)
	qa.TriggerCustomAlert(AlertTypeJitter, AlertSeverityWarning,
		"call-to-keep", "session-3", "Alert 3", nil)

	time.Sleep(50 * time.Millisecond)

	qa.ClearCallAlerts("call-to-clear")

	alerts := qa.GetActiveAlerts()
	if len(alerts) != 1 {
		t.Errorf("Expected 1 remaining alert, got %d", len(alerts))
	}
	if alerts[0].CallID != "call-to-keep" {
		t.Error("Wrong alert remaining")
	}
}

func TestQualityAlerter_GetAlertHistory(t *testing.T) {
	config := &QualityAlerterConfig{
		MaxAlertHistory: 100,
	}
	qa := NewQualityAlerter(config)

	for i := 0; i < 5; i++ {
		qa.TriggerCustomAlert(AlertTypeMediaTimeout, AlertSeverityWarning,
			"call", "session", "Alert", nil)
	}

	time.Sleep(50 * time.Millisecond)

	history := qa.GetAlertHistory(3)
	if len(history) != 3 {
		t.Errorf("Expected 3 history entries, got %d", len(history))
	}

	allHistory := qa.GetAlertHistory(0)
	if len(allHistory) != 5 {
		t.Errorf("Expected 5 total history entries, got %d", len(allHistory))
	}
}

func TestQualityAlerter_GetAlertStats(t *testing.T) {
	qa := NewQualityAlerter(nil)

	qa.TriggerCustomAlert(AlertTypeMediaTimeout, AlertSeverityWarning,
		"call-1", "session-1", "Warning", nil)
	qa.TriggerCustomAlert(AlertTypeRTPGap, AlertSeverityCritical,
		"call-2", "session-2", "Critical", nil)

	time.Sleep(50 * time.Millisecond)

	stats := qa.GetAlertStats()

	if stats.ActiveCount != 2 {
		t.Errorf("Expected 2 active, got %d", stats.ActiveCount)
	}
	if stats.UnacknowledgedCount != 2 {
		t.Errorf("Expected 2 unacknowledged, got %d", stats.UnacknowledgedCount)
	}
	if stats.BySeverity[AlertSeverityWarning] != 1 {
		t.Error("Warning count mismatch")
	}
	if stats.BySeverity[AlertSeverityCritical] != 1 {
		t.Error("Critical count mismatch")
	}
}

func TestQualityAlerter_Suppression(t *testing.T) {
	config := &QualityAlerterConfig{
		SuppressionPeriod: 100 * time.Millisecond,
	}
	qa := NewQualityAlerter(config)

	qa.SetThreshold(AlertTypePacketLoss, &AlertThreshold{
		WarningValue:  1.0,
		CriticalValue: 5.0,
		Duration:      0,
		Enabled:       true,
	})

	alertCount := 0
	var mu sync.Mutex
	qa.AddHandler(func(alert *QualityAlert) {
		mu.Lock()
		alertCount++
		mu.Unlock()
	})

	// Trigger multiple alerts in quick succession
	for i := 0; i < 5; i++ {
		qa.CheckMetric(AlertTypePacketLoss, "call-123", "session-456", 10.0, nil)
		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if alertCount != 1 {
		t.Errorf("Expected 1 alert due to suppression, got %d", alertCount)
	}
	mu.Unlock()

	// Wait for suppression to expire
	time.Sleep(150 * time.Millisecond)

	qa.CheckMetric(AlertTypePacketLoss, "call-123", "session-456", 10.0, nil)

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if alertCount != 2 {
		t.Errorf("Expected 2 alerts after suppression expired, got %d", alertCount)
	}
	mu.Unlock()
}

func TestQualityAlerter_StartStop(t *testing.T) {
	qa := NewQualityAlerter(nil)

	ctx, cancel := context.WithCancel(context.Background())
	qa.Start(ctx)

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)

	cancel()
	qa.Stop()

	// Should not panic or hang
}

func TestQualityAlerter_MaxActiveAlerts(t *testing.T) {
	config := &QualityAlerterConfig{
		MaxActiveAlerts: 3,
	}
	qa := NewQualityAlerter(config)

	for i := 0; i < 10; i++ {
		qa.TriggerCustomAlert(AlertTypeMediaTimeout, AlertSeverityWarning,
			"call", "session", "Alert", nil)
	}

	time.Sleep(50 * time.Millisecond)

	alerts := qa.GetActiveAlerts()
	if len(alerts) > 3 {
		t.Errorf("Expected max 3 active alerts, got %d", len(alerts))
	}
}

func TestFormatAlertMessage(t *testing.T) {
	tests := []struct {
		alertType AlertType
		severity  AlertSeverity
		value     float64
		threshold float64
		contains  string
	}{
		{AlertTypePacketLoss, AlertSeverityWarning, 2.5, 1.0, "Packet loss"},
		{AlertTypeJitter, AlertSeverityCritical, 60.0, 50.0, "Jitter"},
		{AlertTypeLatency, AlertSeverityWarning, 200.0, 150.0, "Latency"},
		{AlertTypeMOS, AlertSeverityCritical, 2.8, 3.0, "MOS"},
	}

	for _, tt := range tests {
		msg := formatAlertMessage(tt.alertType, tt.severity, tt.value, tt.threshold)
		if len(msg) == 0 {
			t.Errorf("Empty message for %s", tt.alertType)
		}
		// Just verify it doesn't panic and returns a message
	}
}

func TestLogAlertHandler(t *testing.T) {
	var loggedMessage string
	handler := &LogAlertHandler{
		Logger: func(format string, args ...interface{}) {
			loggedMessage = format
		},
	}

	alert := &QualityAlert{
		Type:      AlertTypePacketLoss,
		Severity:  AlertSeverityWarning,
		CallID:    "call-123",
		Message:   "Test message",
		Value:     5.0,
		Threshold: 1.0,
	}

	handler.Handle(alert)

	if loggedMessage == "" {
		t.Error("Log handler did not log")
	}
}
