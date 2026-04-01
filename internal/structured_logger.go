package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"
)

// SLogLevel represents the severity of a structured log message
type SLogLevel int

const (
	SLogLevelDebug SLogLevel = iota
	SLogLevelInfo
	SLogLevelWarn
	SLogLevelError
	SLogLevelFatal
)

func (l SLogLevel) String() string {
	switch l {
	case SLogLevelDebug:
		return "DEBUG"
	case SLogLevelInfo:
		return "INFO"
	case SLogLevelWarn:
		return "WARN"
	case SLogLevelError:
		return "ERROR"
	case SLogLevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// LogFormat represents the output format
type LogFormat int

const (
	LogFormatJSON LogFormat = iota
	LogFormatText
)

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Caller    string                 `json:"caller,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// StructuredLoggerConfig holds logger configuration
type StructuredLoggerConfig struct {
	Level           SLogLevel
	Format          LogFormat
	Output          io.Writer
	IncludeCaller   bool
	CallerSkip      int
	TimestampFormat string
}

// DefaultStructuredLoggerConfig returns default configuration
func DefaultStructuredLoggerConfig() *StructuredLoggerConfig {
	return &StructuredLoggerConfig{
		Level:           SLogLevelInfo,
		Format:          LogFormatJSON,
		Output:          os.Stdout,
		IncludeCaller:   true,
		CallerSkip:      3,
		TimestampFormat: time.RFC3339Nano,
	}
}

// StructuredLogger provides structured logging with context
type StructuredLogger struct {
	config     *StructuredLoggerConfig
	fields     map[string]interface{}
	mu         sync.RWMutex
	writeMu    *sync.Mutex // Pointer to shared mutex for derived loggers
}

// NewStructuredLogger creates a new structured logger
func NewStructuredLogger(config *StructuredLoggerConfig) *StructuredLogger {
	if config == nil {
		config = DefaultStructuredLoggerConfig()
	}
	if config.Output == nil {
		config.Output = os.Stdout
	}
	if config.TimestampFormat == "" {
		config.TimestampFormat = time.RFC3339Nano
	}

	return &StructuredLogger{
		config:  config,
		fields:  make(map[string]interface{}),
		writeMu: &sync.Mutex{},
	}
}

// WithFields returns a new logger with additional fields
func (l *StructuredLogger) WithFields(fields map[string]interface{}) *StructuredLogger {
	l.mu.RLock()
	defer l.mu.RUnlock()

	newFields := make(map[string]interface{}, len(l.fields)+len(fields))
	for k, v := range l.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}

	return &StructuredLogger{
		config:  l.config,
		fields:  newFields,
		writeMu: l.writeMu, // Share the mutex for thread-safe writes
	}
}

// WithField returns a new logger with an additional field
func (l *StructuredLogger) WithField(key string, value interface{}) *StructuredLogger {
	return l.WithFields(map[string]interface{}{key: value})
}

// WithError returns a new logger with an error field
func (l *StructuredLogger) WithError(err error) *StructuredLogger {
	if err == nil {
		return l
	}
	return l.WithField("error", err.Error())
}

// WithCallID returns a logger with call-id context
func (l *StructuredLogger) WithCallID(callID string) *StructuredLogger {
	return l.WithField("call_id", callID)
}

// WithSessionID returns a logger with session-id context
func (l *StructuredLogger) WithSessionID(sessionID string) *StructuredLogger {
	return l.WithField("session_id", sessionID)
}

// WithComponent returns a logger with component context
func (l *StructuredLogger) WithComponent(component string) *StructuredLogger {
	return l.WithField("component", component)
}

// log writes a log entry at the specified level
func (l *StructuredLogger) log(level SLogLevel, msg string, fields map[string]interface{}) {
	if level < l.config.Level {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().Format(l.config.TimestampFormat),
		Level:     level.String(),
		Message:   msg,
	}

	// Merge fields
	l.mu.RLock()
	if len(l.fields) > 0 || len(fields) > 0 {
		entry.Fields = make(map[string]interface{}, len(l.fields)+len(fields))
		for k, v := range l.fields {
			entry.Fields[k] = v
		}
		for k, v := range fields {
			entry.Fields[k] = v
		}
	}
	l.mu.RUnlock()

	// Add caller info if enabled
	if l.config.IncludeCaller {
		_, file, line, ok := runtime.Caller(l.config.CallerSkip)
		if ok {
			entry.Caller = fmt.Sprintf("%s:%d", file, line)
		}
	}

	// Format and write
	l.writeMu.Lock()
	defer l.writeMu.Unlock()

	var output []byte
	var err error

	switch l.config.Format {
	case LogFormatJSON:
		output, err = json.Marshal(entry)
		if err != nil {
			output = []byte(fmt.Sprintf(`{"error":"marshal failed","message":"%s"}`, msg))
		}
		output = append(output, '\n')
	case LogFormatText:
		output = l.formatText(entry)
	default:
		output, _ = json.Marshal(entry)
		output = append(output, '\n')
	}

	l.config.Output.Write(output)
}

// formatText formats the entry as human-readable text
func (l *StructuredLogger) formatText(entry LogEntry) []byte {
	var buf []byte

	// Timestamp and level
	buf = append(buf, entry.Timestamp...)
	buf = append(buf, ' ')
	buf = append(buf, '[')
	buf = append(buf, entry.Level...)
	buf = append(buf, ']')
	buf = append(buf, ' ')

	// Message
	buf = append(buf, entry.Message...)

	// Fields
	if len(entry.Fields) > 0 {
		buf = append(buf, ' ')
		first := true
		for k, v := range entry.Fields {
			if !first {
				buf = append(buf, ' ')
			}
			buf = append(buf, k...)
			buf = append(buf, '=')
			buf = append(buf, fmt.Sprintf("%v", v)...)
			first = false
		}
	}

	// Caller
	if entry.Caller != "" {
		buf = append(buf, " ("...)
		buf = append(buf, entry.Caller...)
		buf = append(buf, ')')
	}

	buf = append(buf, '\n')
	return buf
}

// Debug logs a debug message
func (l *StructuredLogger) Debug(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(SLogLevelDebug, msg, f)
}

// Info logs an info message
func (l *StructuredLogger) Info(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(SLogLevelInfo, msg, f)
}

// Warn logs a warning message
func (l *StructuredLogger) Warn(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(SLogLevelWarn, msg, f)
}

// Error logs an error message
func (l *StructuredLogger) Error(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(SLogLevelError, msg, f)
}

// Fatal logs a fatal message and exits
func (l *StructuredLogger) Fatal(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(SLogLevelFatal, msg, f)
	os.Exit(1)
}

// Debugf logs a formatted debug message
func (l *StructuredLogger) Debugf(format string, args ...interface{}) {
	l.Debug(fmt.Sprintf(format, args...))
}

// Infof logs a formatted info message
func (l *StructuredLogger) Infof(format string, args ...interface{}) {
	l.Info(fmt.Sprintf(format, args...))
}

// Warnf logs a formatted warning message
func (l *StructuredLogger) Warnf(format string, args ...interface{}) {
	l.Warn(fmt.Sprintf(format, args...))
}

// Errorf logs a formatted error message
func (l *StructuredLogger) Errorf(format string, args ...interface{}) {
	l.Error(fmt.Sprintf(format, args...))
}

// SetLevel changes the log level
func (l *StructuredLogger) SetLevel(level SLogLevel) {
	l.config.Level = level
}

// SetFormat changes the output format
func (l *StructuredLogger) SetFormat(format LogFormat) {
	l.config.Format = format
}

// SetOutput changes the output writer
func (l *StructuredLogger) SetOutput(w io.Writer) {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()
	l.config.Output = w
}

// CallLogger provides logging specifically for call operations
type CallLogger struct {
	logger *StructuredLogger
}

// NewCallLogger creates a logger for call operations
func NewCallLogger(baseLogger *StructuredLogger, callID, fromTag, toTag string) *CallLogger {
	return &CallLogger{
		logger: baseLogger.WithFields(map[string]interface{}{
			"call_id":  callID,
			"from_tag": fromTag,
			"to_tag":   toTag,
		}),
	}
}

// LogOffer logs an offer operation
func (cl *CallLogger) LogOffer(sdp string, flags map[string]interface{}) {
	cl.logger.Info("Processing offer", map[string]interface{}{
		"operation": "offer",
		"sdp_lines": countLines(sdp),
		"flags":     flags,
	})
}

// LogAnswer logs an answer operation
func (cl *CallLogger) LogAnswer(sdp string, flags map[string]interface{}) {
	cl.logger.Info("Processing answer", map[string]interface{}{
		"operation": "answer",
		"sdp_lines": countLines(sdp),
		"flags":     flags,
	})
}

// LogDelete logs a delete operation
func (cl *CallLogger) LogDelete(reason string) {
	cl.logger.Info("Deleting call", map[string]interface{}{
		"operation": "delete",
		"reason":    reason,
	})
}

// LogMediaStart logs media start
func (cl *CallLogger) LogMediaStart(codec string, rtpPort, rtcpPort int) {
	cl.logger.Info("Media started", map[string]interface{}{
		"operation": "media_start",
		"codec":     codec,
		"rtp_port":  rtpPort,
		"rtcp_port": rtcpPort,
	})
}

// LogMediaStop logs media stop
func (cl *CallLogger) LogMediaStop(duration time.Duration, packetsRx, packetsTx uint64) {
	cl.logger.Info("Media stopped", map[string]interface{}{
		"operation":  "media_stop",
		"duration":   duration.String(),
		"packets_rx": packetsRx,
		"packets_tx": packetsTx,
	})
}

// LogError logs a call error
func (cl *CallLogger) LogError(operation string, err error) {
	cl.logger.Error("Call error", map[string]interface{}{
		"operation": operation,
		"error":     err.Error(),
	})
}

func countLines(s string) int {
	count := 0
	for _, c := range s {
		if c == '\n' {
			count++
		}
	}
	return count
}

// AuditLogger provides audit logging for security events
type AuditLogger struct {
	logger *StructuredLogger
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(baseLogger *StructuredLogger) *AuditLogger {
	return &AuditLogger{
		logger: baseLogger.WithComponent("audit"),
	}
}

// LogAccess logs an access event
func (al *AuditLogger) LogAccess(ip, user, resource, action string, allowed bool) {
	fields := map[string]interface{}{
		"event_type": "access",
		"ip":         ip,
		"user":       user,
		"resource":   resource,
		"action":     action,
		"allowed":    allowed,
	}

	if allowed {
		al.logger.Info("Access granted", fields)
	} else {
		al.logger.Warn("Access denied", fields)
	}
}

// LogConfigChange logs a configuration change
func (al *AuditLogger) LogConfigChange(user, parameter string, oldValue, newValue interface{}) {
	al.logger.Info("Configuration changed", map[string]interface{}{
		"event_type": "config_change",
		"user":       user,
		"parameter":  parameter,
		"old_value":  oldValue,
		"new_value":  newValue,
	})
}

// LogSecurityEvent logs a security-related event
func (al *AuditLogger) LogSecurityEvent(eventType, description string, details map[string]interface{}) {
	fields := map[string]interface{}{
		"event_type":  eventType,
		"description": description,
	}
	for k, v := range details {
		fields[k] = v
	}
	al.logger.Warn("Security event", fields)
}

// Global structured logger
var (
	globalStructuredLogger     *StructuredLogger
	globalStructuredLoggerOnce sync.Once
)

// GetStructuredLogger returns the global structured logger
func GetStructuredLogger() *StructuredLogger {
	globalStructuredLoggerOnce.Do(func() {
		config := DefaultStructuredLoggerConfig()

		// Check environment for log level
		if level := os.Getenv("KARL_LOG_LEVEL"); level != "" {
			switch level {
			case "debug", "DEBUG":
				config.Level = SLogLevelDebug
			case "info", "INFO":
				config.Level = SLogLevelInfo
			case "warn", "WARN":
				config.Level = SLogLevelWarn
			case "error", "ERROR":
				config.Level = SLogLevelError
			}
		}

		// Check environment for log format
		if format := os.Getenv("KARL_LOG_FORMAT"); format != "" {
			switch format {
			case "json", "JSON":
				config.Format = LogFormatJSON
			case "text", "TEXT":
				config.Format = LogFormatText
			}
		}

		globalStructuredLogger = NewStructuredLogger(config)
	})
	return globalStructuredLogger
}

// Convenience functions using global logger
func LogDebug(msg string, fields ...map[string]interface{}) {
	GetStructuredLogger().Debug(msg, fields...)
}

func LogInfo(msg string, fields ...map[string]interface{}) {
	GetStructuredLogger().Info(msg, fields...)
}

func LogWarn(msg string, fields ...map[string]interface{}) {
	GetStructuredLogger().Warn(msg, fields...)
}

func LogError(msg string, fields ...map[string]interface{}) {
	GetStructuredLogger().Error(msg, fields...)
}
