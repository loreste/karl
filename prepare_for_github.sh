#!/bin/bash
# Script to prepare the Karl Media Server repository for GitHub

echo "Preparing Karl Media Server repository for GitHub..."

# Stop any running Karl instances
echo "Stopping any running Karl instances..."
pkill -f "./karl" || true
sleep 2

# Create directories for GitHub workflows
echo "Creating GitHub workflow directory..."
mkdir -p .github/workflows

# Create CI workflow
echo "Creating CI workflow file..."
cat > .github/workflows/go.yml << EOL
name: Go

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3

    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.23.2'

    - name: Build
      run: go build -v ./...

    - name: Test
      run: go test -v ./...
      
    - name: Format check
      run: |
        if [ "\$(gofmt -l . | wc -l)" -gt 0 ]; then
          echo "The following files need formatting:"
          gofmt -l .
          exit 1
        fi
EOL

# Create DEV-GUIDE.md if needed
echo "Ensuring DEV-GUIDE.md exists..."
if [ ! -f "DEV-GUIDE.md" ]; then
  touch DEV-GUIDE.md
fi

# List of files to check for AI references
FILES_TO_CHECK=(
  "*.md"
  "*.go"
  "*.sh"
  "*.json"
  "*.yml"
)

# Find and replace references to Claude or AI tools
echo "Removing references to AI tools..."
for pattern in "${FILES_TO_CHECK[@]}"; do
  find . -type f -name "$pattern" -not -path "*/\.*" | while read file; do
    # Skip this script itself
    if [[ "$file" == "./prepare_for_github.sh" ]]; then
      continue
    fi

    # Use sed to remove specific AI references
    sed -i '' 's/Generated with \[.*Code\].*//g' "$file"
    sed -i '' 's/Co-Authored-By:.*//g' "$file"
    sed -i '' 's/AI-assisted//g' "$file"
    sed -i '' 's/AI assisted//g' "$file"
    sed -i '' 's/AI-generated//g' "$file"
  done
done

# Clean up development and test files
echo "Cleaning up development and test files..."
rm -f test_client.go verify.go send_rtp.py

# Create .gitignore if it doesn't exist
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
test_e2e

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

# System files
.DS_Store
Thumbs.db
EOL
fi

# Initialize git if not already a repository
if [ ! -d ".git" ]; then
  echo "Initializing git repository..."
  git init
  
  # Add files to git
  echo "Adding files to git..."
  git add .
  
  # Make the initial commit
  echo "Making initial commit..."
  git commit -m "Initial commit of Karl Media Server"
fi

echo ""
echo "Repository preparation complete!"
echo ""
echo "Next steps:"
echo "1. Create a new repository on GitHub"
echo "2. Run the following commands to push your code:"
echo "   git remote add origin https://github.com/yourusername/karl.git"
echo "   git push -u origin main"
echo ""
echo "Note: Replace 'yourusername' with your actual GitHub username."