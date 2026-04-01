package internal

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidationError_Error(t *testing.T) {
	err := ValidationError{
		Field:   "TestField",
		Value:   "testvalue",
		Message: "test message",
	}

	expected := "config validation failed for 'TestField': test message (value: testvalue)"
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}
}

func TestValidationResult_AddError(t *testing.T) {
	result := &ValidationResult{Valid: true}

	result.AddError("field1", "value1", "error message")

	if result.Valid {
		t.Error("Result should be invalid after adding error")
	}
	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(result.Errors))
	}
	if result.Errors[0].Field != "field1" {
		t.Errorf("Expected field 'field1', got %q", result.Errors[0].Field)
	}
}

func TestValidationResult_AddWarning(t *testing.T) {
	result := &ValidationResult{Valid: true}

	result.AddWarning("warning message")

	if !result.Valid {
		t.Error("Warnings should not invalidate result")
	}
	if len(result.Warnings) != 1 {
		t.Errorf("Expected 1 warning, got %d", len(result.Warnings))
	}
}

func TestValidationResult_Error(t *testing.T) {
	result := &ValidationResult{Valid: true}

	if result.Error() != nil {
		t.Error("Valid result should return nil error")
	}

	result.AddError("field1", "value1", "error1")
	result.AddError("field2", "value2", "error2")

	err := result.Error()
	if err == nil {
		t.Error("Invalid result should return error")
	}
}

func TestNewConfigValidator(t *testing.T) {
	validator := NewConfigValidator()

	if validator == nil {
		t.Fatal("NewConfigValidator returned nil")
	}
	if len(validator.validators) == 0 {
		t.Error("Validator should have default validators registered")
	}
}

func TestConfigValidator_ValidateNilConfig(t *testing.T) {
	validator := NewConfigValidator()
	result := validator.Validate(nil)

	if result.Valid {
		t.Error("Nil config should be invalid")
	}
}

func TestConfigValidator_ValidateEmptyConfig(t *testing.T) {
	validator := NewConfigValidator()
	config := &ServerConfig{}

	result := validator.Validate(config)

	// Empty config should be valid (all optional)
	if !result.Valid {
		t.Errorf("Empty config should be valid, errors: %v", result.Errors)
	}
}

func TestValidateNetworkConfig(t *testing.T) {
	tests := []struct {
		name        string
		listenAddr  string
		expectValid bool
	}{
		{"valid host:port", "127.0.0.1:8080", true},
		{"valid :port", ":8080", true},
		{"valid hostname:port", "localhost:8080", true},
		{"invalid no port", "127.0.0.1", false},
		{"invalid port", "127.0.0.1:99999", false},
		{"invalid format", "not-valid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewConfigValidator()
			config := &ServerConfig{ListenAddr: tt.listenAddr}
			result := validator.Validate(config)

			hasAddressError := false
			for _, err := range result.Errors {
				if err.Field == "ListenAddr" {
					hasAddressError = true
					break
				}
			}

			if tt.expectValid && hasAddressError {
				t.Errorf("Expected valid, got error for %q", tt.listenAddr)
			}
			if !tt.expectValid && !hasAddressError {
				t.Errorf("Expected invalid, but got no error for %q", tt.listenAddr)
			}
		})
	}
}

func TestValidatePortRanges(t *testing.T) {
	tests := []struct {
		name        string
		portMin     int
		portMax     int
		expectValid bool
	}{
		{"valid range", 10000, 20000, true},
		{"min too low", 100, 20000, false},
		{"max less than min", 20000, 10000, false},
		{"max too high", 10000, 70000, false},
		{"zero values", 0, 0, true}, // Zero means not configured
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewConfigValidator()
			config := &ServerConfig{
				RTPPortMin: tt.portMin,
				RTPPortMax: tt.portMax,
			}
			result := validator.Validate(config)

			hasPortError := false
			for _, err := range result.Errors {
				if err.Field == "RTPPortMin" || err.Field == "RTPPortMax" {
					hasPortError = true
					break
				}
			}

			if tt.expectValid && hasPortError {
				t.Errorf("Expected valid, got error for range %d-%d", tt.portMin, tt.portMax)
			}
			if !tt.expectValid && !hasPortError {
				t.Errorf("Expected invalid, but got no error for range %d-%d", tt.portMin, tt.portMax)
			}
		})
	}
}

func TestValidatePortRanges_OddPortWarning(t *testing.T) {
	validator := NewConfigValidator()
	config := &ServerConfig{
		RTPPortMin: 10001, // Odd port
		RTPPortMax: 20000,
	}
	result := validator.Validate(config)

	hasWarning := false
	for _, w := range result.Warnings {
		if w == "RTPPortMin should be even per RFC 3550" {
			hasWarning = true
			break
		}
	}

	if !hasWarning {
		t.Error("Expected warning for odd RTPPortMin")
	}
}

func TestValidatePortRanges_SmallRangeWarning(t *testing.T) {
	validator := NewConfigValidator()
	config := &ServerConfig{
		RTPPortMin: 10000,
		RTPPortMax: 10050, // Only 50 ports
	}
	result := validator.Validate(config)

	hasWarning := false
	for _, w := range result.Warnings {
		if w == "RTP port range is small, may limit concurrent sessions" {
			hasWarning = true
			break
		}
	}

	if !hasWarning {
		t.Error("Expected warning for small port range")
	}
}

func TestValidateTimeouts(t *testing.T) {
	tests := []struct {
		name          string
		sessionTimeout time.Duration
		expectValid   bool
	}{
		{"valid timeout", 60 * time.Second, true},
		{"too short", 100 * time.Millisecond, false},
		{"zero (not configured)", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewConfigValidator()
			config := &ServerConfig{SessionTimeout: tt.sessionTimeout}
			result := validator.Validate(config)

			hasTimeoutError := false
			for _, err := range result.Errors {
				if err.Field == "SessionTimeout" {
					hasTimeoutError = true
					break
				}
			}

			if tt.expectValid && hasTimeoutError {
				t.Errorf("Expected valid, got error for timeout %v", tt.sessionTimeout)
			}
			if !tt.expectValid && !hasTimeoutError {
				t.Errorf("Expected invalid, but got no error for timeout %v", tt.sessionTimeout)
			}
		})
	}
}

func TestValidatePaths_RecordingPath(t *testing.T) {
	// Create temp dir for testing
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		path        string
		expectValid bool
	}{
		{"existing dir", tmpDir, true},
		{"non-existent with existing parent", filepath.Join(tmpDir, "newdir"), true},
		{"non-existent parent", "/nonexistent/path/to/dir", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewConfigValidator()
			config := &ServerConfig{RecordingPath: tt.path}
			result := validator.Validate(config)

			hasPathError := false
			for _, err := range result.Errors {
				if err.Field == "RecordingPath" {
					hasPathError = true
					break
				}
			}

			if tt.expectValid && hasPathError {
				t.Errorf("Expected valid, got error for path %q", tt.path)
			}
			if !tt.expectValid && !hasPathError {
				t.Errorf("Expected invalid, but got no error for path %q", tt.path)
			}
		})
	}
}

func TestValidatePaths_TLS(t *testing.T) {
	// Create temp cert and key files
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")
	os.WriteFile(certPath, []byte("fake cert"), 0600)
	os.WriteFile(keyPath, []byte("fake key"), 0600)

	tests := []struct {
		name        string
		enabled     bool
		certPath    string
		keyPath     string
		expectValid bool
	}{
		{"TLS disabled", false, "", "", true},
		{"TLS enabled with valid paths", true, certPath, keyPath, true},
		{"TLS enabled missing cert", true, "", keyPath, false},
		{"TLS enabled missing key", true, certPath, "", false},
		{"TLS enabled non-existent cert", true, "/nonexistent/cert.pem", keyPath, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewConfigValidator()
			config := &ServerConfig{
				TLSEnabled:  tt.enabled,
				TLSCertPath: tt.certPath,
				TLSKeyPath:  tt.keyPath,
			}
			result := validator.Validate(config)

			hasTLSError := false
			for _, err := range result.Errors {
				if err.Field == "TLSCertPath" || err.Field == "TLSKeyPath" {
					hasTLSError = true
					break
				}
			}

			if tt.expectValid && hasTLSError {
				t.Errorf("Expected valid, got TLS error")
			}
			if !tt.expectValid && !hasTLSError {
				t.Errorf("Expected invalid, but got no TLS error")
			}
		})
	}
}

func TestValidateRedisConfig(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		addr        string
		db          int
		expectValid bool
	}{
		{"Redis disabled", false, "", 0, true},
		{"Redis enabled valid", true, "localhost:6379", 0, true},
		{"Redis enabled no addr", true, "", 0, false},
		{"Redis enabled invalid addr", true, "invalid", 0, false},
		{"Redis enabled invalid db", true, "localhost:6379", 20, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewConfigValidator()
			config := &ServerConfig{
				RedisEnabled: tt.enabled,
				RedisAddr:    tt.addr,
				RedisDB:      tt.db,
			}
			result := validator.Validate(config)

			hasRedisError := false
			for _, err := range result.Errors {
				if err.Field == "RedisAddr" || err.Field == "RedisDB" {
					hasRedisError = true
					break
				}
			}

			if tt.expectValid && hasRedisError {
				t.Errorf("Expected valid, got Redis error")
			}
			if !tt.expectValid && !hasRedisError {
				t.Errorf("Expected invalid, but got no Redis error")
			}
		})
	}
}

func TestValidateSecurityConfig_AllowedIPs(t *testing.T) {
	tests := []struct {
		name        string
		allowedIPs  []string
		expectValid bool
	}{
		{"empty list", []string{}, true},
		{"valid IPs", []string{"192.168.1.1", "10.0.0.0/8"}, true},
		{"invalid IP", []string{"not-an-ip"}, false},
		{"mixed valid/invalid", []string{"192.168.1.1", "invalid"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewConfigValidator()
			config := &ServerConfig{AllowedIPs: tt.allowedIPs}
			result := validator.Validate(config)

			hasIPError := false
			for _, err := range result.Errors {
				if err.Field[:10] == "AllowedIPs" {
					hasIPError = true
					break
				}
			}

			if tt.expectValid && hasIPError {
				t.Errorf("Expected valid, got IP error")
			}
			if !tt.expectValid && !hasIPError {
				t.Errorf("Expected invalid, but got no IP error")
			}
		})
	}
}

func TestValidateResourceLimits(t *testing.T) {
	tests := []struct {
		name           string
		workerPoolSize int
		bufferSize     int
		expectValid    bool
	}{
		{"valid config", 10, 2000, true},
		{"zero values", 0, 0, true},
		{"invalid buffer size", 10, 70000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewConfigValidator()
			config := &ServerConfig{
				WorkerPoolSize: tt.workerPoolSize,
				BufferSize:     tt.bufferSize,
			}
			result := validator.Validate(config)

			hasResourceError := false
			for _, err := range result.Errors {
				if err.Field == "WorkerPoolSize" || err.Field == "BufferSize" {
					hasResourceError = true
					break
				}
			}

			if tt.expectValid && hasResourceError {
				t.Errorf("Expected valid, got resource error")
			}
			if !tt.expectValid && !hasResourceError {
				t.Errorf("Expected invalid, but got no resource error")
			}
		})
	}
}

func TestValidateInterfaceConfig(t *testing.T) {
	tests := []struct {
		name        string
		interfaces  []InterfaceConfig
		expectValid bool
	}{
		{"empty interfaces", []InterfaceConfig{}, true},
		{"valid interface", []InterfaceConfig{
			{Name: "eth0", Address: "192.168.1.1"},
		}, true},
		{"duplicate names", []InterfaceConfig{
			{Name: "eth0", Address: "192.168.1.1"},
			{Name: "eth0", Address: "192.168.1.2"},
		}, false},
		{"invalid address", []InterfaceConfig{
			{Name: "eth0", Address: "not-an-ip"},
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewConfigValidator()
			config := &ServerConfig{Interfaces: tt.interfaces}
			result := validator.Validate(config)

			hasInterfaceError := false
			for _, err := range result.Errors {
				if len(err.Field) >= 10 && err.Field[:10] == "Interfaces" {
					hasInterfaceError = true
					break
				}
			}

			if tt.expectValid && hasInterfaceError {
				t.Errorf("Expected valid, got interface error")
			}
			if !tt.expectValid && !hasInterfaceError {
				t.Errorf("Expected invalid, but got no interface error")
			}
		})
	}
}

func TestIsValidAddress(t *testing.T) {
	tests := []struct {
		addr     string
		expected bool
	}{
		{":8080", true},
		{"localhost:8080", true},
		{"127.0.0.1:8080", true},
		{"0.0.0.0:8080", true},
		{"[::1]:8080", true},
		{"example.com:443", true},
		{"", false},
		{"localhost", false},
		{"localhost:99999", false},
		{"localhost:-1", false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			result := isValidAddress(tt.addr)
			if result != tt.expected {
				t.Errorf("isValidAddress(%q) = %v, expected %v", tt.addr, result, tt.expected)
			}
		})
	}
}

func TestIsValidHostname(t *testing.T) {
	tests := []struct {
		hostname string
		expected bool
	}{
		{"localhost", true},
		{"example.com", true},
		{"sub.example.com", true},
		{"my-server", true},
		{"server-1.example.com", true},
		{"-invalid", false},
		{"invalid-", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			result := isValidHostname(tt.hostname)
			if result != tt.expected {
				t.Errorf("isValidHostname(%q) = %v, expected %v", tt.hostname, result, tt.expected)
			}
		})
	}
}

func TestIsValidIPOrCIDR(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"192.168.1.1", true},
		{"10.0.0.0/8", true},
		{"::1", true},
		{"2001:db8::/32", true},
		{"invalid", false},
		{"192.168.1.1/33", false}, // Invalid CIDR
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isValidIPOrCIDR(tt.input)
			if result != tt.expected {
				t.Errorf("isValidIPOrCIDR(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRegisterValidator(t *testing.T) {
	validator := NewConfigValidator()
	initialCount := len(validator.validators)

	customCalled := false
	validator.RegisterValidator(func(config *ServerConfig, result *ValidationResult) {
		customCalled = true
	})

	if len(validator.validators) != initialCount+1 {
		t.Error("Custom validator not registered")
	}

	validator.Validate(&ServerConfig{})

	if !customCalled {
		t.Error("Custom validator not called")
	}
}

func TestValidateConfigFromEnv(t *testing.T) {
	// Set some environment variables
	os.Setenv("KARL_LISTEN_ADDR", ":8080")
	os.Setenv("KARL_RTP_PORT_MIN", "10000")
	os.Setenv("KARL_RTP_PORT_MAX", "20000")
	defer func() {
		os.Unsetenv("KARL_LISTEN_ADDR")
		os.Unsetenv("KARL_RTP_PORT_MIN")
		os.Unsetenv("KARL_RTP_PORT_MAX")
	}()

	result := ValidateConfigFromEnv()

	if !result.Valid {
		t.Errorf("Expected valid config from env, got errors: %v", result.Errors)
	}
}

func TestMustValidate_Valid(t *testing.T) {
	config := &ServerConfig{
		RTPPortMin: 10000,
		RTPPortMax: 20000,
	}

	// Should not panic
	MustValidate(config)
}

func TestMustValidate_Invalid(t *testing.T) {
	config := &ServerConfig{
		RTPPortMin: 100, // Invalid - below 1024
		RTPPortMax: 20000,
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustValidate should panic on invalid config")
		}
	}()

	MustValidate(config)
}

func TestComplexValidation(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")
	os.WriteFile(certPath, []byte("fake cert"), 0600)
	os.WriteFile(keyPath, []byte("fake key"), 0600)

	config := &ServerConfig{
		ListenAddr:      ":8080",
		NGListenAddr:    ":22222",
		HTTPListenAddr:  ":8081",
		RTPPortMin:      10000,
		RTPPortMax:      20000,
		SessionTimeout:  60 * time.Second,
		MediaTimeout:    30 * time.Second,
		RecordingPath:   tmpDir,
		RedisEnabled:    true,
		RedisAddr:       "localhost:6379",
		RedisDB:         0,
		TLSEnabled:      true,
		TLSCertPath:     certPath,
		TLSKeyPath:      keyPath,
		AllowedIPs:      []string{"192.168.0.0/16", "10.0.0.0/8"},
		MaxSessions:     1000,
		WorkerPoolSize:  16,
		BufferSize:      2048,
		Interfaces: []InterfaceConfig{
			{Name: "internal", Address: "192.168.1.1", Internal: true},
			{Name: "external", Address: "203.0.113.1", AdvertiseIP: "203.0.113.1"},
		},
	}

	validator := NewConfigValidator()
	result := validator.Validate(config)

	if !result.Valid {
		t.Errorf("Complex valid config failed validation: %v", result.Errors)
	}
}
