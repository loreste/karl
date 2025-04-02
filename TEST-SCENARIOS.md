# Karl Media Server - Test Scenarios

This document outlines various test scenarios for verifying the functionality of the Karl Media Server.

## Basic Functionality Tests

### 1. Server Startup Test

**Objective**: Verify that the Karl Media Server starts correctly and all services initialize.

**Steps**:
1. Build the server: `go build -o karl`
2. Run the server: `./karl`
3. Check logs for successful initialization

**Expected Results**:
- No errors in the startup sequence
- Log message: "✅ Karl Media Server started successfully"
- Services initialized: RTP Engine, WebRTC, API Server, etc.

### 2. Configuration Loading Test

**Objective**: Verify that the server loads configuration correctly.

**Steps**:
1. Modify `config/config.json` with test values
2. Run the server: `./karl`
3. Check logs for configuration loading

**Expected Results**:
- Log message: "✅ Configuration loaded successfully"
- Server behaves according to the modified configuration

### 3. Metrics Endpoint Test

**Objective**: Verify that the Prometheus metrics endpoint is working.

**Steps**:
1. Run the server: `./karl`
2. Access metrics endpoint: `curl http://localhost:9091/metrics`

**Expected Results**:
- HTTP 200 OK response
- Metrics data in Prometheus format
- Karl-specific metrics like `karl_rtp_packets_total`

## Media Handling Tests

### 4. RTP Packet Processing Test

**Objective**: Verify that the server can receive and process RTP packets.

**Steps**:
1. Run the server: `./karl`
2. Use the test client to send RTP packets: `go run test_e2e.go`

**Expected Results**:
- Log messages showing packet reception
- Metrics showing increased packet count
- No errors in packet processing

### 5. SRTP Transcoding Test

**Objective**: Verify SRTP encryption and decryption.

**Steps**:
1. Configure SRTP in `config/config.json`
2. Run the server: `./karl`
3. Send RTP packets that require SRTP handling

**Expected Results**:
- Log messages showing successful transcoding
- Metrics showing encrypted/decrypted packet counts
- No errors in transcoding process

### 6. Codec Conversion Test

**Objective**: Verify codec conversion between different formats.

**Steps**:
1. Configure the server with specific codec support
2. Send media packets in one codec format
3. Check if conversion to another format occurs

**Expected Results**:
- Log messages showing codec conversion
- Proper handling of different media formats
- No quality loss in the conversion process

## Integration Tests

### 7. SIP Registration Test

**Objective**: Verify SIP proxy registration functionality.

**Steps**:
1. Configure SIP proxy settings in `config/config.json`
2. Run the server: `./karl`
3. Check logs for registration messages

**Expected Results**:
- Log message: "Successfully registered Karl with SIP proxy"
- Periodic registration refresh
- Proper handling of registration failures

### 8. WebRTC Session Test

**Objective**: Verify WebRTC session establishment.

**Steps**:
1. Configure WebRTC in `config/config.json`
2. Run the server: `./karl`
3. Initiate a WebRTC session using a test client

**Expected Results**:
- Log messages showing successful WebRTC session
- ICE connectivity established
- Media streaming through WebRTC

## Performance Tests

### 9. High Load Test

**Objective**: Verify server performance under high load.

**Steps**:
1. Run the server: `./karl`
2. Send a large number of concurrent RTP packets
3. Monitor system resources and packet handling

**Expected Results**:
- Server handles high packet rate without errors
- CPU and memory usage remain within acceptable limits
- No packet drops under normal conditions

### 10. Long-Running Stability Test

**Objective**: Verify server stability over extended periods.

**Steps**:
1. Run the server: `./karl`
2. Send RTP traffic continuously for at least 24 hours
3. Monitor for memory leaks, crashes, or performance degradation

**Expected Results**:
- Server remains stable throughout the test period
- No memory leaks or resource exhaustion
- Consistent performance metrics

## End-to-End Testing Using test_e2e.go

For comprehensive end-to-end testing, use the included `test_e2e.go` program:

```bash
# Stop any running Karl instances
pkill -f "./karl" || true

# Build the Karl server
go build -o karl

# Build the E2E test
go build -o test_e2e test_e2e.go

# Run the test
./test_e2e
```

The E2E test performs the following:
1. Starts a new Karl server instance
2. Verifies the metrics endpoint
3. Sends a series of RTP packets
4. Checks metrics to verify packet processing
5. Shuts down cleanly

## Test Automation

For automated testing, use the provided script:

```bash
./run_e2e_test.sh
```

This script:
1. Stops any running Karl instances
2. Builds the server and test program
3. Runs the E2E test
4. Cleans up test artifacts

## Docker Testing

For testing the Docker deployment:

```bash
# Build the Docker image
docker build -t karl-media-server .

# Run the container
docker run -p 12000:12000/udp -p 9091:9091 karl-media-server

# Test with RTP packets
go run test_e2e.go
```

## Production Readiness Checklist

Before deploying to production, verify:

1. ✅ All tests pass successfully
2. ✅ Configuration is properly secured (no default credentials)
3. ✅ Log levels are set appropriately
4. ✅ Metrics collection is enabled
5. ✅ Error handling is robust
6. ✅ Resource limits are configured correctly