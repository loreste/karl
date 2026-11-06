# KARL MEDIA SERVER - DEVELOPER GUIDE

## Build Commands
- Build: `go build -o karl`
- Run: `go run .`
- Test: `go test ./...`
- Test with race detector: `go test -race ./...`
- Test specific package: `go test ./internal/ng_protocol/commands/`
- Test with coverage: `go test -cover ./...`
- Static analysis: `go vet ./...`
- Format: `go fmt ./...`

## Code Style Guidelines
- **Imports**: Group stdlib first, third-party next, project imports last; alphabetize within groups
- **Formatting**: Use tabs for indentation; run `go fmt` before commits
- **Types**: Use defined structs with clear purpose; implement interfaces when appropriate
- **Naming**: CamelCase for exported (public) items; camelCase for unexported (private) items
- **Error Handling**: Return errors with context using `fmt.Errorf("msg: %w", err)`; log errors before returning
- **Concurrency**: Use `sync.Mutex` for shared state on CallLeg fields (IP, Port, LastActivity, blocking flags). Use `atomic` for counters (PacketsSent, PacketsRecv, etc.). Use `session.mu` for session-level fields. Never hold `session.mu` while calling `Relay.Stop()` (deadlock risk).
- **Resource cleanup**: Use `defer` for socket cleanup in offer/answer error paths. Always stop relay before closing sockets.

## Architecture

Karl is an RTP media proxy that acts as a drop-in replacement for rtpengine, handling SIP proxy NG protocol commands and forwarding RTP between call legs.

### Request Flow

```
SIP Proxy (OpenSIPS/Kamailio)
    │
    ▼ NG Protocol (bencode over UDP/Unix socket)
ng_socket_listener.go
    │
    ▼ dispatches by command name
commands/offer.go ─── creates session + caller leg with UDP sockets
commands/answer.go ── creates callee leg, starts MediaRelay
commands/delete.go ── stops relay, closes sockets, frees session
```

### Media Flow

```
Caller ──RTP──▶ [CallerLeg.Conn] ──readLoop──▶ [CalleeLeg.Conn] ──WriteToUDP──▶ Callee
Callee ──RTP──▶ [CalleeLeg.Conn] ──readLoop──▶ [CallerLeg.Conn] ──WriteToUDP──▶ Caller
```

Each session has one `MediaRelay` with two `readLoop` goroutines (one per direction). The relay handles:
- **Port latching**: Learn remote address from first packet (symmetric RTP / NAT traversal)
- **Media handover**: Allow source address change mid-call (phone roaming)
- **Strict source**: Reject packets from unexpected addresses
- **Media blocking / silencing**: Drop or zero-fill packets per flag
- **Recording hook**: `OnPacket` callback sends RTP payload to recording manager
- **Media timeout**: Auto-delete session when both legs are idle

### Key Data Structures

- `MediaSession` (`session_manager.go`): Holds CallID, tags, CallerLeg, CalleeLeg, Relay, flags, stats
- `CallLeg` (`session_manager.go`): IP/Port (remote), LocalIP/LocalPort, Conn (UDP socket), NAT flags, stats counters
- `MediaRelay` (`media_relay.go`): Two readLoop goroutines, stopCh, WaitGroup, OnPacket/OnTimeout hooks
- `SessionRegistry` (`session_manager.go`): Map of sessions indexed by ID, CallID, FromTag, SSRC; TTL cleanup

### Thread Safety Rules

1. `CallLeg.mu` protects: `IP`, `Port`, `LastActivity`, `MediaBlocked`, `Silenced`
2. `CallLeg` stats (`PacketsSent`, `PacketsRecv`, etc.) use `atomic` operations — read with `LoadStats()`
3. `MediaSession.mu` protects session-level fields. Never hold it while calling `Relay.Stop()`.
4. `SessionRegistry.mu` protects the session maps. `removeSessionLocked` stops relay before acquiring `session.mu`.

### Package Structure

```
karl/
├── main.go, server.go, services.go, config.go  # Server lifecycle, wiring
├── internal/
│   ├── media_relay.go          # Per-session RTP forwarding (core data path)
│   ├── session_manager.go      # Session/CallLeg/Registry
│   ├── ng_socket_listener.go   # NG protocol UDP/Unix listener
│   ├── ng_protocol/
│   │   ├── message.go          # Bencode parsing
│   │   ├── handler.go          # Command registry
│   │   ├── types.go            # Flag parsing (100+ rtpengine flags)
│   │   └── commands/           # offer, answer, delete, query, etc.
│   ├── recording/              # WAV recording, audio mixing
│   ├── dtls_cert.go            # Self-signed DTLS certificate
│   ├── codec_converter.go      # G.711 transcoding (others are stubs)
│   ├── rtcp_handler.go         # RTCP quality monitoring
│   ├── fec_handler.go          # Forward error correction
│   ├── jitter_buffer.go        # Adaptive jitter buffer
│   ├── rate_limiter.go         # Per-IP/per-call rate limiting
│   └── ...                     # Health, metrics, config, etc.
```
