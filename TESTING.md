# Karl Media Server - Testing Guide

This document provides guidance on testing the Karl Media Server, including unit tests, integration tests, and performance testing.

## Table of Contents

- [Running Tests](#running-tests)
- [Writing Tests](#writing-tests)
- [Test Coverage](#test-coverage)
- [Integration Testing](#integration-testing)
- [Performance Testing](#performance-testing)
- [Mocking Dependencies](#mocking-dependencies)

## Running Tests

### Basic Test Commands

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests with coverage information
go test -cover ./...

# Run tests for a specific package
go test ./internal/
```

### Running Specific Tests

```bash
# Run a specific test file
go test ./internal/tests/codec_converter_test.go

# Run a specific test function
go test -run TestSRTPTranscoder ./internal/tests/

# Run tests matching a pattern
go test -run "RTP.*" ./internal/tests/
```

### Test Tags

Some tests may require specific tags to run:

```bash
# Run integration tests
go test -tags=integration ./...

# Run performance tests
go test -tags=performance ./...
```

### Setting Timeouts

For longer tests, you might need to increase the timeout:

```bash
go test -timeout 5m ./...
```

## Writing Tests

### Test Structure

Karl Media Server uses the standard Go testing package. Test files should be named with `_test.go` suffix and placed in the same package as the code they test, or in the dedicated `internal/tests` directory.

```go
package tests

import (
	"karl/internal"
	"testing"
)

func TestSomething(t *testing.T) {
	// Test setup
	result := internal.SomeFunction()
	
	// Assertions
	if result != expectedResult {
		t.Errorf("Expected %v, got %v", expectedResult, result)
	}
}
```

### Table-Driven Tests

For functions with multiple input/output cases, use table-driven tests:

```go
func TestPCMUToPCMA(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
		wantErr  bool
	}{
		{
			name:     "Empty input",
			input:    []byte{},
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "Valid conversion",
			input:    []byte{0xFF, 0xFE},
			expected: []byte{0x2A, 0x2A},
			wantErr:  false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := internal.PCMUToPCMA(tt.input)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("PCMUToPCMA() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("PCMUToPCMA() = %v, want %v", got, tt.expected)
			}
		})
	}
}
```

### Test Utilities

Use the utilities in `internal/tests/test_utils.go` to simplify test setup:

```go
// Create test SRTP keys
srtpKey, srtpSalt := tests.CreateTestSRTPKeys()

// Create test RTP packet
packet := tests.CreateTestRTPPacket(ssrc, seqNum, timestamp, payload)

// Set up a test UDP listener
conn, addrStr, err := tests.CreateTestUDPListener()
```

## Test Coverage

### Measuring Coverage

```bash
# Generate coverage profile
go test -coverprofile=coverage.out ./...

# View coverage in browser
go tool cover -html=coverage.out

# View coverage in terminal
go tool cover -func=coverage.out
```

### Coverage Goals

- Aim for at least 80% code coverage
- Focus on covering critical paths and error handling
- Not all code needs to be covered (e.g., some initialization code)

## Integration Testing

Integration tests verify that different components work together correctly.

### Setting Up Integration Tests

Create a file with the `integration` build tag:

```go
//go:build integration
// +build integration

package tests

import (
	"karl/internal"
	"testing"
)

func TestRTPToSIPIntegration(t *testing.T) {
	// Integration test code
}
```

### MySQL Integration Testing

For tests that require a database connection:

```go
func TestDatabaseIntegration(t *testing.T) {
	// Skip if not in integration mode
	if testing.Short() {
		t.Skip("Skipping database integration test in short mode")
	}
	
	// Connect to test database
	db, err := internal.NewRTPDatabase("user:password@tcp(localhost:3306)/test_rtpdb")
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	
	// Perform tests
}
```

### Redis Integration Testing

```go
func TestRedisIntegration(t *testing.T) {
	// Skip if not in integration mode
	if testing.Short() {
		t.Skip("Skipping Redis integration test in short mode")
	}
	
	// Create test config
	config := &internal.Config{
		Database: internal.DatabaseConfig{
			RedisEnabled: true,
			RedisAddr:    "localhost:6379",
		},
	}
	
	// Initialize Redis
	redisCache := internal.NewRTPRedisCache(config)
	if redisCache == nil {
		t.Fatal("Failed to initialize Redis")
	}
	defer redisCache.Close()
	
	// Perform tests
}
```

## Performance Testing

### Benchmarks

Use Go's benchmark functionality for performance testing:

```go
func BenchmarkSRTPTranscoding(b *testing.B) {
	// Setup
	srtpKey, srtpSalt := tests.CreateTestSRTPKeys()
	transcoder, _ := internal.NewSRTPTranscoder(srtpKey, srtpSalt)
	packet := tests.CreateTestRTPPacket(1, 1, 1, make([]byte, 160))
	
	// Reset timer before the loop
	b.ResetTimer()
	
	// Run the benchmark
	for i := 0; i < b.N; i++ {
		_, err := transcoder.TranscodeRTPToSRTP(packet)
		if err != nil {
			b.Fatal(err)
		}
	}
}
```

Run benchmarks with:

```bash
go test -bench=. ./internal/tests/
```

### Load Testing

For more intensive load testing, use dedicated load testing tools along with custom scenarios:

```go
//go:build loadtest
// +build loadtest

package tests

import (
	"karl/internal"
	"sync"
	"testing"
	"time"
)

func TestHighConcurrencyLoad(t *testing.T) {
	// Skip if not in load test mode
	if testing.Short() {
		t.Skip("Skipping load test")
	}
	
	// Setup Karl server with test config
	
	// Create multiple concurrent clients
	const numClients = 1000
	var wg sync.WaitGroup
	wg.Add(numClients)
	
	start := time.Now()
	
	for i := 0; i < numClients; i++ {
		go func(clientID int) {
			defer wg.Done()
			
			// Send RTP packets
			// Record metrics
		}(i)
	}
	
	wg.Wait()
	elapsed := time.Since(start)
	
	t.Logf("Handled %d concurrent clients in %v", numClients, elapsed)
}
```

## Mocking Dependencies

### Creating Test Mocks

For components with external dependencies, create mock implementations:

```go
// MockDatabase implements the database interface for testing
type MockDatabase struct {
	// Mock fields
	Sessions map[string]string
}

// NewMockDatabase creates a mock database
func NewMockDatabase() *MockDatabase {
	return &MockDatabase{
		Sessions: make(map[string]string),
	}
}

// InsertRTPStats implements the RTPDatabase interface
func (m *MockDatabase) InsertRTPStats(callID string, ssrc uint32, codec string, packetLoss int, jitter float64) error {
	// Store in mock data
	m.Sessions[callID] = codec
	return nil
}

// GetActiveSessions implements the RTPDatabase interface
func (m *MockDatabase) GetActiveSessions() ([]string, error) {
	var sessions []string
	for k := range m.Sessions {
		sessions = append(sessions, k)
	}
	return sessions, nil
}
```

### Using Mocks in Tests

```go
func TestRTPControlWithMockDB(t *testing.T) {
	// Create mock database
	mockDB := NewMockDatabase()
	
	// Create system under test
	rtpControl, _ := internal.NewRTPControl(srtpKey, srtpSalt)
	
	// Inject mock
	rtpControl.SetDatabase(mockDB)
	
	// Test functionality
	// ...
	
	// Verify mock interactions
	sessions, _ := mockDB.GetActiveSessions()
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}
}
```

## Continuous Integration

Karl Media Server uses GitHub Actions for continuous integration. The workflow:

1. Runs all unit tests
2. Checks test coverage
3. Performs linting
4. Builds the application for multiple platforms

To run the CI checks locally:

```bash
# Run the verification script
./scripts/verify.sh
```

## Debugging Tests

### Enable Verbose Logging

```bash
# Set log level to debug during tests
export KARL_LOG_LEVEL=4
go test -v ./...
```

### Runtime Analysis

For memory or CPU issues in tests:

```bash
# Memory profiling
go test -memprofile=mem.out ./internal/tests/problematic_test.go

# CPU profiling
go test -cpuprofile=cpu.out ./internal/tests/problematic_test.go

# Analyze the profiles
go tool pprof mem.out
go tool pprof cpu.out
```

---

Remember that good tests are the foundation of reliable software. Take the time to write comprehensive tests that cover both success and failure cases.