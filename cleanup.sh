#!/bin/bash
# Cleanup script for Karl Media Server
# Removes test and development files that aren't needed in production

# Stop any running Karl instances
echo "Stopping any running Karl instances..."
pkill -f "./karl" || true

# Remove test files
echo "Removing test files..."
rm -f test_client.go
rm -f verify.go
rm -f send_rtp.py

# Create a .gitignore file if it doesn't exist
if [ ! -f .gitignore ]; then
  echo "Creating .gitignore file..."
  cat > .gitignore << EOL
# Binaries for programs and plugins
*.exe
*.exe~
*.dll
*.so
*.dylib
karl

# Test binary, built with 'go test -c'
*.test

# Output of the go coverage tool
*.out

# Dependency directories
vendor/

# Go workspace file
go.work

# IDE files
.idea/
.vscode/
*.swp
*.swo

# Log files
logs/*.log

# Runtime files
run/

# Local development files
*.local.json
*.local.go
EOL
fi

echo "Clean up completed. The following files have been removed:"
echo "- test_client.go - Test client for sending RTP packets"
echo "- verify.go - Verification utility"
echo "- send_rtp.py - Python script for sending test RTP packets"
echo ""
echo "For production deployment, make sure to:"
echo "1. Update configuration in config/config.json"
echo "2. Set up proper permissions for runtime directories"
echo "3. Configure your database credentials"
echo ""
echo "See PRODUCTION-READY.md for detailed deployment instructions."