package internal

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSLogLevel_String(t *testing.T) {
	tests := []struct {
		level    SLogLevel
		expected string
	}{
		{SLogLevelDebug, "DEBUG"},
		{SLogLevelInfo, "INFO"},
		{SLogLevelWarn, "WARN"},
		{SLogLevelError, "ERROR"},
		{SLogLevelFatal, "FATAL"},
		{SLogLevel(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.expected {
			t.Errorf("SLogLevel(%d).String() = %s, expected %s", tt.level, got, tt.expected)
		}
	}
}

func TestDefaultStructuredLoggerConfig(t *testing.T) {
	config := DefaultStructuredLoggerConfig()

	if config.Level != SLogLevelInfo {
		t.Errorf("Expected level INFO, got %s", config.Level.String())
	}
	if config.Format != LogFormatJSON {
		t.Error("Expected format JSON")
	}
	if !config.IncludeCaller {
		t.Error("Expected IncludeCaller true")
	}
}

func TestNewStructuredLogger(t *testing.T) {
	logger := NewStructuredLogger(nil)

	if logger == nil {
		t.Fatal("NewStructuredLogger returned nil")
	}
	if logger.config == nil {
		t.Error("config should not be nil")
	}
	if logger.fields == nil {
		t.Error("fields should not be nil")
	}
}

func TestStructuredLogger_JSONOutput(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	logger := NewStructuredLogger(config)
	logger.Info("test message")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v\nOutput: %s", err, buf.String())
	}

	if entry.Level != "INFO" {
		t.Errorf("Expected level INFO, got %s", entry.Level)
	}
	if entry.Message != "test message" {
		t.Errorf("Expected message 'test message', got %s", entry.Message)
	}
	if entry.Timestamp == "" {
		t.Error("Expected timestamp to be set")
	}
}

func TestStructuredLogger_TextOutput(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatText,
		Output:        &buf,
		IncludeCaller: false,
	}

	logger := NewStructuredLogger(config)
	logger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "[INFO]") {
		t.Errorf("Expected [INFO] in output: %s", output)
	}
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected 'test message' in output: %s", output)
	}
}

func TestStructuredLogger_WithFields(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	logger := NewStructuredLogger(config)
	loggerWithFields := logger.WithFields(map[string]interface{}{
		"key1": "value1",
		"key2": 123,
	})

	loggerWithFields.Info("test with fields")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if entry.Fields["key1"] != "value1" {
		t.Errorf("Expected key1=value1, got %v", entry.Fields["key1"])
	}
	if entry.Fields["key2"].(float64) != 123 {
		t.Errorf("Expected key2=123, got %v", entry.Fields["key2"])
	}
}

func TestStructuredLogger_WithField(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	logger := NewStructuredLogger(config)
	loggerWithField := logger.WithField("single_key", "single_value")
	loggerWithField.Info("test")

	var entry LogEntry
	json.Unmarshal(buf.Bytes(), &entry)

	if entry.Fields["single_key"] != "single_value" {
		t.Errorf("Expected single_key=single_value, got %v", entry.Fields["single_key"])
	}
}

func TestStructuredLogger_WithError(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	logger := NewStructuredLogger(config)
	loggerWithError := logger.WithError(RateLimitError("test"))
	loggerWithError.Error("error occurred")

	var entry LogEntry
	json.Unmarshal(buf.Bytes(), &entry)

	if entry.Fields["error"] == nil {
		t.Error("Expected error field to be set")
	}
}

func TestStructuredLogger_WithError_Nil(t *testing.T) {
	logger := NewStructuredLogger(nil)
	loggerWithError := logger.WithError(nil)

	// Should return same logger instance
	if loggerWithError == nil {
		t.Error("WithError(nil) should return logger, not nil")
	}
}

func TestStructuredLogger_WithCallID(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	logger := NewStructuredLogger(config)
	loggerWithCallID := logger.WithCallID("call-123")
	loggerWithCallID.Info("test")

	var entry LogEntry
	json.Unmarshal(buf.Bytes(), &entry)

	if entry.Fields["call_id"] != "call-123" {
		t.Errorf("Expected call_id=call-123, got %v", entry.Fields["call_id"])
	}
}

func TestStructuredLogger_WithSessionID(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	logger := NewStructuredLogger(config)
	loggerWithSessionID := logger.WithSessionID("session-456")
	loggerWithSessionID.Info("test")

	var entry LogEntry
	json.Unmarshal(buf.Bytes(), &entry)

	if entry.Fields["session_id"] != "session-456" {
		t.Errorf("Expected session_id=session-456, got %v", entry.Fields["session_id"])
	}
}

func TestStructuredLogger_WithComponent(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	logger := NewStructuredLogger(config)
	loggerWithComponent := logger.WithComponent("rtp-handler")
	loggerWithComponent.Info("test")

	var entry LogEntry
	json.Unmarshal(buf.Bytes(), &entry)

	if entry.Fields["component"] != "rtp-handler" {
		t.Errorf("Expected component=rtp-handler, got %v", entry.Fields["component"])
	}
}

func TestStructuredLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelWarn,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	logger := NewStructuredLogger(config)

	// Debug and Info should be filtered
	logger.Debug("debug message")
	logger.Info("info message")

	if buf.Len() > 0 {
		t.Error("Debug and Info should be filtered at WARN level")
	}

	// Warn should pass
	logger.Warn("warn message")
	if buf.Len() == 0 {
		t.Error("Warn should not be filtered")
	}
}

func TestStructuredLogger_AllLevels(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	logger := NewStructuredLogger(config)

	tests := []struct {
		level   string
		logFunc func()
	}{
		{"DEBUG", func() { logger.Debug("debug") }},
		{"INFO", func() { logger.Info("info") }},
		{"WARN", func() { logger.Warn("warn") }},
		{"ERROR", func() { logger.Error("error") }},
	}

	for _, tt := range tests {
		buf.Reset()
		tt.logFunc()

		var entry LogEntry
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Errorf("Failed to parse %s log: %v", tt.level, err)
			continue
		}

		if entry.Level != tt.level {
			t.Errorf("Expected level %s, got %s", tt.level, entry.Level)
		}
	}
}

func TestStructuredLogger_FormattedMethods(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	logger := NewStructuredLogger(config)

	logger.Infof("formatted %s %d", "message", 123)

	var entry LogEntry
	json.Unmarshal(buf.Bytes(), &entry)

	if entry.Message != "formatted message 123" {
		t.Errorf("Expected formatted message, got %s", entry.Message)
	}
}

func TestStructuredLogger_SetLevel(t *testing.T) {
	logger := NewStructuredLogger(nil)

	logger.SetLevel(SLogLevelError)
	if logger.config.Level != SLogLevelError {
		t.Error("SetLevel should change log level")
	}
}

func TestStructuredLogger_SetFormat(t *testing.T) {
	logger := NewStructuredLogger(nil)

	logger.SetFormat(LogFormatText)
	if logger.config.Format != LogFormatText {
		t.Error("SetFormat should change format")
	}
}

func TestStructuredLogger_SetOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(nil)

	logger.SetOutput(&buf)
	logger.Info("test")

	if buf.Len() == 0 {
		t.Error("SetOutput should redirect output")
	}
}

func TestStructuredLogger_ConcurrentAccess(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	logger := NewStructuredLogger(config)

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				logger.WithField("goroutine", id).Info("concurrent log")
			}
		}(i)
	}

	wg.Wait()
}

func TestCallLogger(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	baseLogger := NewStructuredLogger(config)
	callLogger := NewCallLogger(baseLogger, "call-123", "from-tag", "to-tag")

	callLogger.LogOffer("v=0\r\n", map[string]interface{}{"symmetric": true})

	var entry LogEntry
	json.Unmarshal(buf.Bytes(), &entry)

	if entry.Fields["call_id"] != "call-123" {
		t.Error("Expected call_id field")
	}
	if entry.Fields["operation"] != "offer" {
		t.Error("Expected operation=offer")
	}
}

func TestCallLogger_LogAnswer(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	baseLogger := NewStructuredLogger(config)
	callLogger := NewCallLogger(baseLogger, "call-123", "from-tag", "to-tag")

	callLogger.LogAnswer("v=0\r\n", nil)

	var entry LogEntry
	json.Unmarshal(buf.Bytes(), &entry)

	if entry.Fields["operation"] != "answer" {
		t.Error("Expected operation=answer")
	}
}

func TestCallLogger_LogDelete(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	baseLogger := NewStructuredLogger(config)
	callLogger := NewCallLogger(baseLogger, "call-123", "from-tag", "to-tag")

	callLogger.LogDelete("BYE received")

	var entry LogEntry
	json.Unmarshal(buf.Bytes(), &entry)

	if entry.Fields["operation"] != "delete" {
		t.Error("Expected operation=delete")
	}
	if entry.Fields["reason"] != "BYE received" {
		t.Error("Expected reason field")
	}
}

func TestCallLogger_LogMediaStart(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	baseLogger := NewStructuredLogger(config)
	callLogger := NewCallLogger(baseLogger, "call-123", "from-tag", "to-tag")

	callLogger.LogMediaStart("PCMU", 10000, 10001)

	var entry LogEntry
	json.Unmarshal(buf.Bytes(), &entry)

	if entry.Fields["codec"] != "PCMU" {
		t.Error("Expected codec=PCMU")
	}
}

func TestCallLogger_LogMediaStop(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	baseLogger := NewStructuredLogger(config)
	callLogger := NewCallLogger(baseLogger, "call-123", "from-tag", "to-tag")

	callLogger.LogMediaStop(5*time.Minute, 10000, 9500)

	var entry LogEntry
	json.Unmarshal(buf.Bytes(), &entry)

	if entry.Fields["operation"] != "media_stop" {
		t.Error("Expected operation=media_stop")
	}
}

func TestCallLogger_LogError(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	baseLogger := NewStructuredLogger(config)
	callLogger := NewCallLogger(baseLogger, "call-123", "from-tag", "to-tag")

	callLogger.LogError("media_start", RateLimitError("test"))

	var entry LogEntry
	json.Unmarshal(buf.Bytes(), &entry)

	if entry.Level != "ERROR" {
		t.Error("Expected ERROR level")
	}
}

func TestAuditLogger_LogAccess(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	baseLogger := NewStructuredLogger(config)
	auditLogger := NewAuditLogger(baseLogger)

	auditLogger.LogAccess("192.168.1.1", "admin", "/api/config", "GET", true)

	var entry LogEntry
	json.Unmarshal(buf.Bytes(), &entry)

	if entry.Fields["event_type"] != "access" {
		t.Error("Expected event_type=access")
	}
	if entry.Fields["allowed"] != true {
		t.Error("Expected allowed=true")
	}
}

func TestAuditLogger_LogConfigChange(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	baseLogger := NewStructuredLogger(config)
	auditLogger := NewAuditLogger(baseLogger)

	auditLogger.LogConfigChange("admin", "log_level", "INFO", "DEBUG")

	var entry LogEntry
	json.Unmarshal(buf.Bytes(), &entry)

	if entry.Fields["event_type"] != "config_change" {
		t.Error("Expected event_type=config_change")
	}
}

func TestAuditLogger_LogSecurityEvent(t *testing.T) {
	var buf bytes.Buffer
	config := &StructuredLoggerConfig{
		Level:         SLogLevelDebug,
		Format:        LogFormatJSON,
		Output:        &buf,
		IncludeCaller: false,
	}

	baseLogger := NewStructuredLogger(config)
	auditLogger := NewAuditLogger(baseLogger)

	auditLogger.LogSecurityEvent("rate_limit", "IP blocked", map[string]interface{}{
		"ip":       "192.168.1.100",
		"requests": 1000,
	})

	var entry LogEntry
	json.Unmarshal(buf.Bytes(), &entry)

	if entry.Level != "WARN" {
		t.Error("Expected WARN level for security events")
	}
	if entry.Fields["ip"] != "192.168.1.100" {
		t.Error("Expected IP in details")
	}
}

func TestCountLines(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"no newlines", 0},
		{"one\nline", 1},
		{"two\nlines\n", 2},
		{"three\nlines\nhere", 2},
	}

	for _, tt := range tests {
		if got := countLines(tt.input); got != tt.expected {
			t.Errorf("countLines(%q) = %d, expected %d", tt.input, got, tt.expected)
		}
	}
}
