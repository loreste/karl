#!/bin/bash

# Move test files temporarily
mv test_client.go test_client.go.bak 2>/dev/null || true
mv test_e2e.go test_e2e.go.bak 2>/dev/null || true
mv verify.go verify.go.bak 2>/dev/null || true

# Compile the main application
go build -v

# Check build status
if [ $? -eq 0 ]; then
    echo "✅ Build successful!"
else
    echo "❌ Build failed"
    exit 1
fi

# Move test files back
mv test_client.go.bak test_client.go 2>/dev/null || true
mv test_e2e.go.bak test_e2e.go 2>/dev/null || true
mv verify.go.bak verify.go 2>/dev/null || true

echo "✅ Test files restored"