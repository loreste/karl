# Karl Media Server - Development Guide

This document provides guidance for developers who want to contribute to the Karl Media Server project.

## Table of Contents

- [Development Environment Setup](#development-environment-setup)
- [Building and Testing](#building-and-testing)
- [Code Structure](#code-structure)
- [Coding Guidelines](#coding-guidelines)
- [Pull Request Process](#pull-request-process)
- [Adding New Features](#adding-new-features)

## Development Environment Setup

### Prerequisites

- Go 1.23.2 or higher
- Git
- MySQL/MariaDB
- Redis (optional)
- Prometheus (optional)

### Setting Up Your Environment

1. **Clone the repository**:
   ```bash
   git clone https://github.com/karlmediaserver/karl.git
   cd karl
   ```

2. **Install Go dependencies**:
   ```bash
   go mod download
   ```

3. **Set up database**:
   ```bash
   # Create the database
   mysql -u root -p -e "CREATE DATABASE rtpdb;"
   
   # Import the schema
   mysql -u root -p rtpdb < mysql_schema.sql
   ```

4. **Configure development settings**:
   ```bash
   # Copy the example config
   cp config/config.json config/dev-config.json
   
   # Edit with your development settings
   # Especially modify database connection strings and run directories
   ```

5. **Build the project**:
   ```bash
   go build -o karl
   ```

## Building and Testing

### Build Commands

```bash
# Build the application
go build -o karl

# Build with race detection
go build -race -o karl

# Build for a specific platform
GOOS=linux GOARCH=amd64 go build -o karl-linux-amd64
```

### Testing

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run a specific test file
go test ./internal/tests/rtp_control_test.go

# Run tests with verbose output
go test -v ./...
```

### Linting and Formatting

```bash
# Format code
gofmt -s -w .

# Lint code with golangci-lint
golangci-lint run
```

## Code Structure

The Karl Media Server codebase is organized as follows:

```
├── config/             # Configuration files
├── internal/           # Internal packages (not intended for external use)
│   ├── tests/          # Test files
│   ├── codec_*.go      # Codec handling
│   ├── rtp_*.go        # RTP packet handling
│   ├── webrtc_*.go     # WebRTC functionality
│   └── ...
├── logs/               # Log files
├── static/             # Static web assets
├── *.go                # Main application files
└── mysql_schema.sql    # Database schema
```

### Key Components

- `main.go`: Application entry point
- `server.go`: Core server implementation
- `config.go`: Configuration handling
- `services.go`: Service initialization
- `webrtc.go`: WebRTC handling

## Coding Guidelines

### Go Style Guide

Karl Media Server follows the official Go style guide and common best practices:

1. **Formatting**: Use `gofmt` to format your code
2. **Naming**:
   - Use CamelCase for exported names (public)
   - Use camelCase for non-exported names (private)
   - Keep names short but descriptive
3. **Comments**:
   - Document all exported functions, types, and constants
   - Begin comments with the name of the item being documented
4. **Imports**:
   - Group standard library imports first, then third-party, then local
   - Sort each group alphabetically
5. **Error Handling**:
   - Always check errors
   - Use `fmt.Errorf("context: %w", err)` for error wrapping
   - Return errors rather than panicking

### Testing

- Write unit tests for all new functionality
- Aim for at least 80% code coverage
- Use table-driven tests where appropriate
- Test both success and failure cases

### Log Levels

Use appropriate log levels:

- **Error (1)**: Only critical errors that affect functionality
- **Warning (2)**: Issues that don't break functionality but are concerning
- **Info (3)**: General operational information
- **Debug (4)**: Detailed information for debugging
- **Trace (5)**: Very verbose debugging information

## Pull Request Process

1. **Fork the Repository**: Create your own fork of the project

2. **Create a Branch**: Create a feature branch with a descriptive name
   ```bash
   git checkout -b feature/your-feature-name
   ```

3. **Make Changes**: Implement your feature or fix

4. **Write Tests**: Add tests for your changes

5. **Run Tests and Lint**:
   ```bash
   go test ./...
   gofmt -s -w .
   ```

6. **Commit Changes**: Use clear commit messages
   ```bash
   git commit -m "Add feature: description of your feature"
   ```

7. **Push to Your Fork**:
   ```bash
   git push origin feature/your-feature-name
   ```

8. **Create a Pull Request**: Submit a PR from your fork to the main repository

9. **Code Review**: Address any feedback from reviewers

## Adding New Features

### Feature Implementation Process

1. **Discuss**: Start with a discussion or issue to get feedback
2. **Design**: Create a design document for significant features
3. **Implement**: Code the feature with tests
4. **Document**: Update documentation to reflect the new feature
5. **Review**: Get peer review before merging

### Working with Media Components

When adding features related to media handling:

1. **Ensure Thread Safety**: Use mutexes for shared resources
2. **Profile Performance**: Media code should be highly optimized
3. **Handle Edge Cases**: Media packets can be malformed
4. **Error Recovery**: Media code should recover gracefully from errors

### Best Practices for RTP/SRTP Features

1. **Packet Validation**: Always validate packet structure before processing
2. **Buffer Management**: Be careful with buffer allocation and reuse
3. **Timing**: RTP timing is critical - don't block processing threads
4. **Metrics**: Add metrics for all new media handling features

### Example: Adding a New Codec

```go
// In internal/codec_table.go
const (
    // Add your new codec
    CodecVP9 = "VP9"
)

// In internal/codec_converter.go
// Implement your codec conversion functions
func VP9ToVP8(payload []byte) ([]byte, error) {
    // Implement conversion
}

// Don't forget to add to the TranscodeVideo function
func TranscodeVideo(payload []byte, inputCodec, outputCodec string) ([]byte, error) {
    switch {
    // Add your new case
    case inputCodec == CodecVP9 && outputCodec == CodecVP8:
        return VP9ToVP8(payload)
    // ... other cases
    }
}

// Add tests in internal/tests/codec_converter_test.go
func TestVP9ToVP8Conversion(t *testing.T) {
    // Test your conversion
}
```

## Performance Considerations

1. **Memory Allocation**: Minimize allocations in hot paths
2. **Concurrency**: Use goroutines wisely and protect shared state
3. **Buffering**: Configure appropriate buffer sizes for media
4. **Metrics**: Use metrics to identify bottlenecks
5. **Profiling**: Use Go's profiling tools to identify performance issues
   ```bash
   go tool pprof karl profile.out
   ```

---

Thank you for contributing to Karl Media Server! Your efforts help make this project better for everyone.