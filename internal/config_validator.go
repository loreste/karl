package internal

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("config validation failed for '%s': %s (value: %v)", e.Field, e.Message, e.Value)
}

// ValidationResult holds the results of configuration validation
type ValidationResult struct {
	Valid    bool
	Errors   []ValidationError
	Warnings []string
}

// AddError adds an error to the validation result
func (r *ValidationResult) AddError(field string, value interface{}, message string) {
	r.Valid = false
	r.Errors = append(r.Errors, ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
	})
}

// AddWarning adds a warning to the validation result
func (r *ValidationResult) AddWarning(message string) {
	r.Warnings = append(r.Warnings, message)
}

// Error returns a combined error message
func (r *ValidationResult) Error() error {
	if r.Valid {
		return nil
	}
	var msgs []string
	for _, e := range r.Errors {
		msgs = append(msgs, e.Error())
	}
	return errors.New(strings.Join(msgs, "; "))
}

// ConfigValidator validates configuration at startup
type ConfigValidator struct {
	validators []func(*ServerConfig, *ValidationResult)
}

// NewConfigValidator creates a new config validator
func NewConfigValidator() *ConfigValidator {
	cv := &ConfigValidator{
		validators: make([]func(*ServerConfig, *ValidationResult), 0),
	}

	// Register default validators
	cv.RegisterValidator(validateNetworkConfig)
	cv.RegisterValidator(validatePortRanges)
	cv.RegisterValidator(validateTimeouts)
	cv.RegisterValidator(validatePaths)
	cv.RegisterValidator(validateRedisConfig)
	cv.RegisterValidator(validateSecurityConfig)
	cv.RegisterValidator(validateResourceLimits)
	cv.RegisterValidator(validateInterfaceConfig)

	return cv
}

// RegisterValidator registers a custom validator function
func (cv *ConfigValidator) RegisterValidator(fn func(*ServerConfig, *ValidationResult)) {
	cv.validators = append(cv.validators, fn)
}

// Validate runs all validators against the configuration
func (cv *ConfigValidator) Validate(config *ServerConfig) *ValidationResult {
	result := &ValidationResult{Valid: true}

	if config == nil {
		result.AddError("config", nil, "configuration is nil")
		return result
	}

	for _, validator := range cv.validators {
		validator(config, result)
	}

	return result
}

// ServerConfig represents the main server configuration for validation
type ServerConfig struct {
	// Network settings
	ListenAddr     string
	NGListenAddr   string
	HTTPListenAddr string
	Interfaces     []InterfaceConfig

	// Port ranges
	RTPPortMin int
	RTPPortMax int

	// Timeouts
	SessionTimeout     time.Duration
	MediaTimeout       time.Duration
	DrainTimeout       time.Duration
	ShutdownTimeout    time.Duration
	HealthCheckTimeout time.Duration

	// Paths
	RecordingPath string
	LogPath       string
	TLSCertPath   string
	TLSKeyPath    string

	// Redis
	RedisEnabled  bool
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// Security
	TLSEnabled        bool
	AuthEnabled       bool
	AllowedIPs        []string
	MaxRequestSize    int64
	RateLimitEnabled  bool
	RateLimitRequests int
	RateLimitWindow   time.Duration

	// Resource limits
	MaxSessions        int
	MaxCallsPerSecond  int
	WorkerPoolSize     int
	BufferSize         int
	MaxMemoryMB        int
	MaxCPUPercent      int

	// Logging
	LogLevel  string
	LogFormat string
}

// InterfaceConfig represents a network interface configuration
type InterfaceConfig struct {
	Name        string
	Address     string
	AdvertiseIP string
	Port        int
	Internal    bool
}

// validateNetworkConfig validates network-related configuration
func validateNetworkConfig(config *ServerConfig, result *ValidationResult) {
	// Validate listen addresses
	if config.ListenAddr != "" {
		if !isValidAddress(config.ListenAddr) {
			result.AddError("ListenAddr", config.ListenAddr, "invalid listen address format")
		}
	}

	if config.NGListenAddr != "" {
		if !isValidAddress(config.NGListenAddr) {
			result.AddError("NGListenAddr", config.NGListenAddr, "invalid NG protocol listen address format")
		}
	}

	if config.HTTPListenAddr != "" {
		if !isValidAddress(config.HTTPListenAddr) {
			result.AddError("HTTPListenAddr", config.HTTPListenAddr, "invalid HTTP listen address format")
		}
	}
}

// validatePortRanges validates RTP port configuration
func validatePortRanges(config *ServerConfig, result *ValidationResult) {
	if config.RTPPortMin > 0 || config.RTPPortMax > 0 {
		// Port range validation
		if config.RTPPortMin < 1024 {
			result.AddError("RTPPortMin", config.RTPPortMin, "port must be >= 1024 (avoid privileged ports)")
		}
		if config.RTPPortMin > 65534 {
			result.AddError("RTPPortMin", config.RTPPortMin, "port must be <= 65534")
		}
		if config.RTPPortMax < config.RTPPortMin {
			result.AddError("RTPPortMax", config.RTPPortMax, "max port must be >= min port")
		}
		if config.RTPPortMax > 65535 {
			result.AddError("RTPPortMax", config.RTPPortMax, "port must be <= 65535")
		}

		// RTP ports should be even (RFC 3550)
		if config.RTPPortMin%2 != 0 {
			result.AddWarning("RTPPortMin should be even per RFC 3550")
		}

		// Reasonable range size
		portRange := config.RTPPortMax - config.RTPPortMin
		if portRange < 100 {
			result.AddWarning("RTP port range is small, may limit concurrent sessions")
		}

		// Each session needs 4 ports (RTP+RTCP for each leg)
		maxSessions := portRange / 4
		if config.MaxSessions > 0 && config.MaxSessions > maxSessions {
			result.AddError("RTPPortRange", portRange,
				fmt.Sprintf("port range too small for %d sessions (need at least %d ports)",
					config.MaxSessions, config.MaxSessions*4))
		}
	}
}

// validateTimeouts validates timeout configuration
func validateTimeouts(config *ServerConfig, result *ValidationResult) {
	if config.SessionTimeout > 0 {
		if config.SessionTimeout < time.Second {
			result.AddError("SessionTimeout", config.SessionTimeout, "timeout too short (minimum 1s)")
		}
		if config.SessionTimeout > 24*time.Hour {
			result.AddWarning("SessionTimeout is very long (>24h)")
		}
	}

	if config.MediaTimeout > 0 {
		if config.MediaTimeout < time.Second {
			result.AddError("MediaTimeout", config.MediaTimeout, "timeout too short (minimum 1s)")
		}
		if config.MediaTimeout > config.SessionTimeout && config.SessionTimeout > 0 {
			result.AddWarning("MediaTimeout exceeds SessionTimeout")
		}
	}

	if config.DrainTimeout > 0 {
		if config.DrainTimeout < time.Second {
			result.AddError("DrainTimeout", config.DrainTimeout, "timeout too short (minimum 1s)")
		}
	}

	if config.ShutdownTimeout > 0 {
		if config.ShutdownTimeout < time.Second {
			result.AddError("ShutdownTimeout", config.ShutdownTimeout, "timeout too short (minimum 1s)")
		}
	}
}

// validatePaths validates file path configuration
func validatePaths(config *ServerConfig, result *ValidationResult) {
	// Validate recording path
	if config.RecordingPath != "" {
		if !filepath.IsAbs(config.RecordingPath) {
			result.AddWarning("RecordingPath is not absolute, may cause issues")
		}

		// Check if directory exists or can be created
		info, err := os.Stat(config.RecordingPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Try to check parent directory
				parent := filepath.Dir(config.RecordingPath)
				if _, err := os.Stat(parent); err != nil {
					result.AddError("RecordingPath", config.RecordingPath, "parent directory does not exist")
				}
			} else {
				result.AddError("RecordingPath", config.RecordingPath, err.Error())
			}
		} else if !info.IsDir() {
			result.AddError("RecordingPath", config.RecordingPath, "path exists but is not a directory")
		}
	}

	// Validate TLS paths
	if config.TLSEnabled {
		if config.TLSCertPath == "" {
			result.AddError("TLSCertPath", "", "TLS enabled but no certificate path specified")
		} else if _, err := os.Stat(config.TLSCertPath); err != nil {
			result.AddError("TLSCertPath", config.TLSCertPath, "certificate file not found")
		}

		if config.TLSKeyPath == "" {
			result.AddError("TLSKeyPath", "", "TLS enabled but no key path specified")
		} else if _, err := os.Stat(config.TLSKeyPath); err != nil {
			result.AddError("TLSKeyPath", config.TLSKeyPath, "key file not found")
		}
	}
}

// validateRedisConfig validates Redis configuration
func validateRedisConfig(config *ServerConfig, result *ValidationResult) {
	if !config.RedisEnabled {
		return
	}

	if config.RedisAddr == "" {
		result.AddError("RedisAddr", "", "Redis enabled but no address specified")
		return
	}

	// Validate address format
	if !isValidAddress(config.RedisAddr) {
		result.AddError("RedisAddr", config.RedisAddr, "invalid Redis address format")
	}

	// Validate DB number
	if config.RedisDB < 0 || config.RedisDB > 15 {
		result.AddError("RedisDB", config.RedisDB, "Redis DB must be 0-15")
	}
}

// validateSecurityConfig validates security configuration
func validateSecurityConfig(config *ServerConfig, result *ValidationResult) {
	// Validate allowed IPs
	for i, ip := range config.AllowedIPs {
		if !isValidIPOrCIDR(ip) {
			result.AddError(fmt.Sprintf("AllowedIPs[%d]", i), ip, "invalid IP address or CIDR")
		}
	}

	// Validate rate limiting
	if config.RateLimitEnabled {
		if config.RateLimitRequests <= 0 {
			result.AddError("RateLimitRequests", config.RateLimitRequests, "must be positive when rate limiting enabled")
		}
		if config.RateLimitWindow <= 0 {
			result.AddError("RateLimitWindow", config.RateLimitWindow, "must be positive when rate limiting enabled")
		}
	}

	// Validate max request size
	if config.MaxRequestSize > 0 && config.MaxRequestSize < 1024 {
		result.AddWarning("MaxRequestSize is very small (<1KB)")
	}
}

// validateResourceLimits validates resource limit configuration
func validateResourceLimits(config *ServerConfig, result *ValidationResult) {
	if config.MaxSessions > 0 && config.MaxSessions < 10 {
		result.AddWarning("MaxSessions is very low")
	}

	if config.WorkerPoolSize > 0 {
		if config.WorkerPoolSize < 1 {
			result.AddError("WorkerPoolSize", config.WorkerPoolSize, "must be >= 1")
		}
		if config.WorkerPoolSize > 1000 {
			result.AddWarning("WorkerPoolSize is very high, may cause resource issues")
		}
	}

	if config.BufferSize > 0 {
		if config.BufferSize < 1500 {
			result.AddWarning("BufferSize smaller than typical MTU")
		}
		if config.BufferSize > 65535 {
			result.AddError("BufferSize", config.BufferSize, "exceeds maximum UDP packet size")
		}
	}

	if config.MaxMemoryMB > 0 && config.MaxMemoryMB < 128 {
		result.AddWarning("MaxMemoryMB is very low (<128MB)")
	}

	if config.MaxCPUPercent > 0 {
		if config.MaxCPUPercent > 100 {
			result.AddError("MaxCPUPercent", config.MaxCPUPercent, "cannot exceed 100%")
		}
	}
}

// validateInterfaceConfig validates network interface configuration
func validateInterfaceConfig(config *ServerConfig, result *ValidationResult) {
	names := make(map[string]bool)

	for i, iface := range config.Interfaces {
		// Check for duplicate names
		if iface.Name != "" {
			if names[iface.Name] {
				result.AddError(fmt.Sprintf("Interfaces[%d].Name", i), iface.Name, "duplicate interface name")
			}
			names[iface.Name] = true
		}

		// Validate address
		if iface.Address != "" {
			ip := net.ParseIP(iface.Address)
			if ip == nil {
				result.AddError(fmt.Sprintf("Interfaces[%d].Address", i), iface.Address, "invalid IP address")
			}
		}

		// Validate advertise IP
		if iface.AdvertiseIP != "" {
			ip := net.ParseIP(iface.AdvertiseIP)
			if ip == nil {
				result.AddError(fmt.Sprintf("Interfaces[%d].AdvertiseIP", i), iface.AdvertiseIP, "invalid advertise IP address")
			}
		}

		// Validate port
		if iface.Port > 0 && (iface.Port < 1 || iface.Port > 65535) {
			result.AddError(fmt.Sprintf("Interfaces[%d].Port", i), iface.Port, "invalid port number")
		}
	}
}

// validateLogConfig validates logging configuration
func validateLogConfig(config *ServerConfig, result *ValidationResult) {
	validLevels := map[string]bool{
		"debug": true, "DEBUG": true,
		"info": true, "INFO": true,
		"warn": true, "WARN": true,
		"warning": true, "WARNING": true,
		"error": true, "ERROR": true,
		"fatal": true, "FATAL": true,
	}

	if config.LogLevel != "" && !validLevels[config.LogLevel] {
		result.AddError("LogLevel", config.LogLevel, "invalid log level")
	}

	validFormats := map[string]bool{
		"json": true, "JSON": true,
		"text": true, "TEXT": true,
	}

	if config.LogFormat != "" && !validFormats[config.LogFormat] {
		result.AddError("LogFormat", config.LogFormat, "invalid log format")
	}
}

// Helper functions

// isValidAddress checks if address is in host:port format
func isValidAddress(addr string) bool {
	// Allow :port format
	if strings.HasPrefix(addr, ":") {
		port := strings.TrimPrefix(addr, ":")
		p, err := strconv.Atoi(port)
		return err == nil && p > 0 && p <= 65535
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}

	// Validate port
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return false
	}

	// Host can be empty, IP, or hostname
	if host != "" {
		// Check if it's a valid IP
		if net.ParseIP(host) == nil {
			// Check if it's a valid hostname
			if !isValidHostname(host) {
				return false
			}
		}
	}

	return true
}

// isValidHostname checks if string is a valid hostname
func isValidHostname(hostname string) bool {
	if len(hostname) > 253 {
		return false
	}

	// RFC 1123 hostname pattern
	pattern := `^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`
	matched, _ := regexp.MatchString(pattern, hostname)
	return matched
}

// isValidIPOrCIDR checks if string is a valid IP address or CIDR notation
func isValidIPOrCIDR(s string) bool {
	// Try parsing as IP
	if net.ParseIP(s) != nil {
		return true
	}

	// Try parsing as CIDR
	_, _, err := net.ParseCIDR(s)
	return err == nil
}

// ValidateConfigFromEnv validates configuration from environment variables
func ValidateConfigFromEnv() *ValidationResult {
	config := &ServerConfig{}

	// Parse environment variables into config
	config.ListenAddr = os.Getenv("KARL_LISTEN_ADDR")
	config.NGListenAddr = os.Getenv("KARL_NG_LISTEN_ADDR")
	config.HTTPListenAddr = os.Getenv("KARL_HTTP_LISTEN_ADDR")

	if port := os.Getenv("KARL_RTP_PORT_MIN"); port != "" {
		config.RTPPortMin, _ = strconv.Atoi(port)
	}
	if port := os.Getenv("KARL_RTP_PORT_MAX"); port != "" {
		config.RTPPortMax, _ = strconv.Atoi(port)
	}

	config.RecordingPath = os.Getenv("KARL_RECORDING_PATH")
	config.LogLevel = os.Getenv("KARL_LOG_LEVEL")
	config.LogFormat = os.Getenv("KARL_LOG_FORMAT")

	config.RedisEnabled = os.Getenv("KARL_REDIS_ENABLED") == "true"
	config.RedisAddr = os.Getenv("KARL_REDIS_ADDR")
	config.RedisPassword = os.Getenv("KARL_REDIS_PASSWORD")
	if db := os.Getenv("KARL_REDIS_DB"); db != "" {
		config.RedisDB, _ = strconv.Atoi(db)
	}

	config.TLSEnabled = os.Getenv("KARL_TLS_ENABLED") == "true"
	config.TLSCertPath = os.Getenv("KARL_TLS_CERT_PATH")
	config.TLSKeyPath = os.Getenv("KARL_TLS_KEY_PATH")

	if maxSessions := os.Getenv("KARL_MAX_SESSIONS"); maxSessions != "" {
		config.MaxSessions, _ = strconv.Atoi(maxSessions)
	}

	// Validate
	validator := NewConfigValidator()
	return validator.Validate(config)
}

// MustValidate validates config and panics if invalid
func MustValidate(config *ServerConfig) {
	validator := NewConfigValidator()
	result := validator.Validate(config)

	if !result.Valid {
		panic(fmt.Sprintf("Configuration validation failed: %v", result.Error()))
	}

	// Log warnings
	for _, w := range result.Warnings {
		LogWarn("Configuration warning: " + w)
	}
}
