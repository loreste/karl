#!/bin/bash

# Karl Build and Run Script
# For /Users/lanceoreste/karl directory

# Go to the Karl directory
cd /Users/lanceoreste/karl

# 1. Stop any running Karl processes
echo "Stopping any running Karl processes..."
pkill -f "karl" || echo "No Karl processes found running."

# 2. Check Go version
echo "Go version:"
go version

# 3. Compile the application
echo "Building Karl..."
BUILD_OUTPUT=$(go build -v 2>&1)
BUILD_RESULT=$?

# 4. Report build status
if [ $BUILD_RESULT -eq 0 ]; then
    echo "✅ Build successful!"
else
    echo "❌ Build failed with error code: $BUILD_RESULT"
    echo "Build output:"
    echo "$BUILD_OUTPUT"
fi

exit $BUILD_RESULT