# Karl Media Server - Test Suite

This directory contains tests for the Karl Media Server components.

## Running Tests

To run all tests:

```bash
go test ./internal/tests/...
```

To run a specific test:

```bash
go test ./internal/tests/codec_converter_test.go
```

To run tests with coverage:

```bash
go test -cover ./internal/tests/...
```

## Test Structure

The test suite is organized by component:

- **rtp_control_test.go**: Tests for RTP packet handling
- **srtp_transcoding_test.go**: Tests for SRTP encryption/decryption
- **codec_converter_test.go**: Tests for audio codec conversion

## Test Utilities

Common test utilities are available in `test_utils.go`:

- `CreateTestSRTPKeys()`: Creates valid SRTP keys for testing
- `CreateTestRTPPacket()`: Creates a test RTP packet
- `CreateTestUDPListener()`: Sets up a UDP listener for testing
- `WaitForCondition()`: Helper for waiting on asynchronous operations

## Writing New Tests

When adding new tests:

1. Follow the Go testing conventions
2. Use table-driven tests where appropriate
3. Test both success and failure cases
4. Use the test utilities for common operations
5. Keep tests focused on one component

Example of a good test:

```go
func TestSomeFunction(t *testing.T) {
    tests := []struct{
        name     string
        input    []byte
        expected []byte
        wantErr  bool
    }{
        {
            name:     "Valid input",
            input:    []byte{0x01, 0x02},
            expected: []byte{0x03, 0x04},
            wantErr:  false,
        },
        {
            name:     "Invalid input",
            input:    []byte{},
            expected: nil,
            wantErr:  true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test code here
        })
    }
}
```

## Continuous Integration

The test suite is designed to be run in CI environments. The tests use mock dependencies and don't require external services like databases.

For integration tests that require external dependencies, use build tags:

```go
//go:build integration
// +build integration

package tests

// Integration test code here
```

## Test Coverage Goals

- Unit tests: >80% coverage
- Critical components: >90% coverage
- Error handling paths: 100% coverage