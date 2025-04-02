#!/bin/bash
# End-to-End Test Script for Karl Media Server

# Stop any running Karl instances
echo "Stopping any running Karl instances..."
pkill -f "./karl" || true
sleep 2

# Build the Karl server
echo "Building Karl Media Server..."
go build -o karl

# Build the test program
echo "Building E2E test program..."
go build -o test_e2e test_e2e.go

# Run the test
echo "Running E2E test..."
./test_e2e

# Clean up
echo "Cleaning up..."
rm -f test_e2e

echo "E2E test completed"