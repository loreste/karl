# KARL MEDIA SERVER - DEVELOPER GUIDE

## Build Commands
- Build: `go build -o karl`
- Run: `go run .`
- Test: `go test ./...`
- Test specific file: `go test ./internal/file_name.go`
- Test with coverage: `go test -cover ./...`
- Lint: `gofmt -s -w .`
- Format: `go fmt ./...`

## Code Style Guidelines
- **Imports**: Group stdlib first, third-party next, project imports last; alphabetize within groups
- **Formatting**: Use tabs for indentation; run `go fmt` before commits
- **Types**: Use defined structs with clear purpose; implement interfaces when appropriate
- **Naming**: CamelCase for exported (public) items; camelCase for unexported (private) items
- **Error Handling**: Return errors with context using `fmt.Errorf("msg: %w", err)`; log errors before returning
- **Logging**: Use emoji prefixes for log messages (✅ ❌ 🚀 📡); include descriptive context
- **Concurrency**: Use mutexes for thread safety; context for cancellation; goroutines wisely
- **Comments**: Document all exported functions, types, and packages

## Architecture Notes
Karl is a media server handling WebRTC, SIP, RTP/SRTP communications with transcoding capabilities.