#!/bin/bash

# Stop any existing processes (more aggressive approach)
echo "Stopping any running instances of Karl..."
pkill -9 -f "karl" || echo "No Karl processes running"
sleep 1

# Check Go version
echo "Go version:"
go version

# Run go mod tidy to ensure dependencies are correct
echo "Running go mod tidy..."
go mod tidy

# Check if there are any compilation errors without creating a binary
echo "Checking for compilation errors..."
go build -o /dev/null
RESULT=$?

if [ $RESULT -eq 0 ]; then
    echo "✅ Compilation successful!"
else
    echo "❌ Compilation failed with code $RESULT"
    # Print more detailed errors
    echo -e "\nDetailed error information:"
    go build -v 2>&1
fi

exit $RESULT