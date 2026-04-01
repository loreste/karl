# Karl Media Server - Full rtpengine Replacement Roadmap

This document outlines the implementation plan to make Karl a complete, production-ready replacement for Sipwise rtpengine.

## Current State

Karl already provides:
- NG protocol support (ping, offer, answer, delete, query, list, recording, block media, DTMF)
- WebRTC bridging with ICE/DTLS-SRTP
- Prometheus metrics and health probes
- REST API for management
- Call recording (mixed, stereo, separate modes)
- SRTP/DTLS-SRTP encryption
- G.711 and Opus codec support
- Adaptive jitter buffer and FEC
- Kubernetes-native deployment

## Gaps to Full Replacement

| Gap | Priority | Status |
|-----|----------|--------|
| Complete NG protocol behavioral parity | Critical | ✅ Complete |
| SIPREC recording integration | High | ✅ Complete |
| T.38 fax passthrough | High | ✅ Complete |
| Multi-node clustering with failover | High | ✅ Complete |
| NAT/interface logic parity | Critical | ✅ Complete |
| IPv4↔IPv6 bridging | Medium | ✅ Complete |
| Performance isolation (fast path) | Medium | ✅ Complete |
| Full SDP edge case handling | Medium | ✅ Complete |

---

## Architecture Target

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Karl Media Server                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐            │
│  │  NG Protocol    │  │   REST API      │  │  Health/Metrics │            │
│  │  Compatibility  │  │   Controller    │  │  Endpoints      │            │
│  │  Layer          │  │                 │  │                 │            │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘            │
│           │                    │                    │                      │
│           └────────────────────┼────────────────────┘                      │
│                                │                                           │
│  ┌─────────────────────────────▼─────────────────────────────────────┐    │
│  │                    Session State Engine                            │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐            │    │
│  │  │ Call State   │  │ Leg State    │  │ Stream State │            │    │
│  │  │ Manager      │  │ Manager      │  │ Manager      │            │    │
│  │  └──────────────┘  └──────────────┘  └──────────────┘            │    │
│  └─────────────────────────────┬─────────────────────────────────────┘    │
│                                │                                           │
│           ┌────────────────────┼────────────────────┐                      │
│           │                    │                    │                      │
│  ┌────────▼────────┐  ┌────────▼────────┐  ┌───────▼─────────┐            │
│  │  Media Relay    │  │  Media Services │  │  NAT/Interface  │            │
│  │  Fast Path      │  │  Path           │  │  Resolver       │            │
│  │                 │  │                 │  │                 │            │
│  │  • RTP forward  │  │  • Transcoding  │  │  • Interface    │            │
│  │  • RTCP forward │  │  • Recording    │  │    selection    │            │
│  │  • SRTP relay   │  │  • SIPREC       │  │  • Peer logic   │            │
│  │  • Zero-copy    │  │  • DTMF inject  │  │  • NAT detect   │            │
│  │                 │  │  • FEC/jitter   │  │  • IPv4↔IPv6    │            │
│  │                 │  │  • T.38         │  │                 │            │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘            │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │                    Cluster State Backend                             │  │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │  │
│  │  │ Session Sync │  │ Failover     │  │ Recording    │              │  │
│  │  │ (Redis)      │  │ Coordinator  │  │ Metadata     │              │  │
│  │  └──────────────┘  └──────────────┘  └──────────────┘              │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │                         Policy Engine                                │  │
│  │  • Per-call flags  • Security rules  • Recording policies           │  │
│  │  • Rate limits     • Media policies  • Codec restrictions           │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Phase 1: Control-Plane Parity (NG Protocol)

**Goal**: OpenSIPS/Kamailio configs work identically with Karl as they do with rtpengine.

**Timeline**: 4-6 weeks

### 1.1 NG Protocol Flag Support

Implement all rtpengine NG protocol flags and options:

#### Session/Call Control Flags
- [x] `call-id` - Call identifier (exists)
- [x] `from-tag` - From leg tag (exists)
- [x] `to-tag` - To leg tag (exists)
- [x] `via-branch` - Via branch for forking
- [x] `label` - Arbitrary label for leg identification
- [x] `set-label` - Set/change label
- [x] `from-label` - Reference by label
- [x] `to-label` - Reference by label
- [x] `all` - Apply to all legs
- [x] `flags` - Generic flags passthrough

#### Interface Selection
- [x] `direction` - Caller/callee direction hints
- [x] `interface` - Select named interface
- [x] `from-interface` - Interface for from-leg
- [x] `to-interface` - Interface for to-leg
- [x] `received-from` - Override received address
- [x] `media-address` - Override media address
- [x] `address-family` - IPv4/IPv6 selection

#### ICE Handling
- [x] `ICE=remove` - Strip ICE (exists)
- [x] `ICE=force` - Force ICE (exists)
- [x] `ICE=force-relay` - Force TURN relay
- [x] `ICE=default` - Default behavior
- [x] `ICE-lite` - ICE-lite mode
- [x] `unidirectional` - Unidirectional ICE
- [x] `trickle-ice` - Trickle ICE support
- [x] `generate-mid` - Generate MID attributes

#### DTLS/SRTP Control
- [x] `DTLS=off` - Disable DTLS (exists)
- [x] `DTLS=passive` - DTLS passive (exists)
- [x] `DTLS=active` - DTLS active (exists)
- [x] `DTLS-reverse` - Reverse DTLS role
- [x] `DTLS-fingerprint` - Fingerprint handling
- [x] `SDES=off` - Disable SDES (exists)
- [x] `SDES=unencrypted_srtp` - Allow unencrypted
- [x] `SDES=unencrypted_srtcp` - Allow unencrypted SRTCP
- [x] `SDES=unauthenticated` - Unauthenticated SRTP
- [x] `SDES-only` - SDES only, no DTLS
- [x] `SDES-pad` - Pad SDES
- [x] `SDES-no` - Per-crypto SDES control
- [x] `transport-protocol` - Force transport

#### SDP Manipulation
- [x] `replace-origin` - Replace o= line (exists)
- [x] `replace-session-connection` - Replace c= line (exists)
- [x] `replace-sdp-version` - Replace version
- [x] `replace-username` - Replace username
- [x] `replace-session-name` - Replace session name
- [x] `trust-address` - Trust SDP addresses
- [x] `SIP-source-address` - Use SIP source (exists)
- [x] `symmetric` - Force symmetric RTP (exists)
- [x] `asymmetric` - Allow asymmetric
- [x] `port-latching` - Enable port latching
- [x] `no-port-latching` - Disable port latching
- [x] `media-handover` - Allow media handover
- [x] `reset` - Reset port latching

#### Codec Control
- [x] `codec-strip` - Strip specific codecs
- [x] `codec-strip-all` - Strip all codecs (exists)
- [x] `codec-offer` - Offer specific codec (exists)
- [x] `codec-mask` - Mask codecs (exists)
- [x] `codec-set` - Set codec params
- [x] `codec-transcode` - Enable transcoding
- [x] `codec-except` - Exclude from operations
- [x] `ptime` - Packet time
- [x] `ptime-reverse` - Reverse ptime
- [x] `T38` - T.38 handling
- [x] `always-transcode` - Force transcoding

#### RTP/RTCP Behavior
- [x] `RTP/AVP` - RTP profile (exists)
- [x] `RTP/SAVP` - SRTP profile (exists)
- [x] `RTP/AVPF` - AVPF profile (exists)
- [x] `RTP/SAVPF` - SAVPF profile (exists)
- [x] `strict-source` - Strict source validation
- [x] `media-echo` - Echo mode
- [x] `RTCP-mux` - RTCP mux control
- [x] `RTCP-mux-demux` - Demux RTCP
- [x] `RTCP-mux-accept` - Accept RTCP mux
- [x] `RTCP-mux-offer` - Offer RTCP mux
- [x] `RTCP-mux-require` - Require RTCP mux
- [x] `no-rtcp-attribute` - Remove RTCP attribute
- [x] `full-rtcp-attribute` - Full RTCP attribute
- [x] `generate-RTCP` - Generate RTCP
- [x] `loop-protect` - Loop protection

#### Recording Control
- [x] `record-call` - Enable recording (exists)
- [x] `start-recording` - Start recording
- [x] `stop-recording` - Stop recording
- [x] `pause-recording` - Pause recording
- [x] `recording-metadata` - Recording metadata
- [x] `recording-file` - Recording filename
- [x] `recording-path` - Recording path
- [x] `recording-pattern` - Filename pattern
- [x] `SIPREC` - SIPREC mode

#### Media Control
- [x] `block-media` - Block media (exists)
- [x] `unblock-media` - Unblock media
- [x] `block-dtmf` - Block DTMF
- [x] `unblock-dtmf` - Unblock DTMF
- [x] `silence-media` - Silence instead of block
- [x] `inject-DTMF` - Inject DTMF (exists)
- [x] `play-media` - Play media file
- [x] `stop-media` - Stop media playback

#### Quality/Timeout
- [x] `media-timeout` - Media timeout
- [x] `session-timeout` - Session timeout
- [x] `TOS` - Set TOS/DSCP
- [x] `delete-delay` - Delay before delete
- [x] `delay-buffer` - Delay buffer config
- [x] `frequency` - RTCP interval

### 1.2 Behavioral Semantics

Beyond flag support, implement correct behavioral semantics:

- [x] **Per-leg state machine** - Track offer/answer state per leg
- [x] **Label resolution** - Resolve legs by label, not just tags
- [x] **Forked call handling** - Support multiple to-tags
- [x] **Re-INVITE semantics** - Proper mid-call SDP renegotiation
- [x] **UPDATE handling** - SIP UPDATE method support (uses same offer/answer flow)
- [x] **Early media** - Correct early media behavior
- [x] **Hold/resume** - Proper hold detection and handling
- [x] **Asymmetric RTP** - Support different send/receive paths
- [x] **SSRC changes** - Handle SSRC changes mid-call
- [x] **PT remapping** - Payload type remapping

### 1.3 Response Format Parity

Ensure responses match rtpengine format exactly:

- [x] `query` response format with stats
- [x] `list` response format with call details
- [x] Error responses with correct reason strings
- [x] Statistics field names and units

### 1.4 Testing

- [ ] Build NG protocol compatibility test suite
- [ ] Test against OpenSIPS rtpengine module
- [ ] Test against Kamailio rtpengine module
- [ ] Test against real SIP traffic (not synthetic)

---

## Phase 2: Relay-Grade Media Core

**Goal**: Rock-solid media relay for production deployment.

**Timeline**: 4-6 weeks

### 2.1 RTP/RTCP Handling

- [x] **Strict RFC 3550 compliance** - Validate all RTP/RTCP (via pion/rtp)
- [x] **SSRC collision detection** - Handle collisions properly (leg_state_machine.go)
- [x] **Sequence number handling** - Correct wrap-around (pion/rtp)
- [x] **Timestamp handling** - Proper timestamp maintenance (pion/rtp)
- [x] **CSRC handling** - Contributor source handling (pion/rtp)
- [x] **Extension header support** - RTP extension headers (pion/rtp)
- [x] **Padding support** - RTP padding handling (pion/rtp)
- [x] **Marker bit handling** - Proper marker semantics (pion/rtp)

### 2.2 SRTP Handling

- [x] **AES-CM encryption** - Counter mode (pion/srtp)
- [x] **AES-GCM encryption** - GCM mode (pion/srtp)
- [x] **HMAC-SHA1 auth** - Authentication (pion/srtp)
- [x] **Key derivation** - Proper key derivation (pion/srtp)
- [x] **ROC handling** - Roll-over counter (pion/srtp)
- [x] **MKI support** - Master Key Identifier (pion/srtp)
- [x] **Crypto suite negotiation** - All standard suites (pion/srtp)
- [x] **SRTP↔RTP gateway** - Clean transcryption (rtp_forwarding.go)

### 2.3 NAT/Interface Logic

Implement rtpengine-compatible interface handling:

- [x] **Named interfaces** - Define interfaces in config (interface_selector.go)
- [x] **Interface selection** - Per-call interface choice (interface_selector.go)
- [x] **Peer-based routing** - Select interface by peer (interface_selector.go)
- [x] **Direction hints** - internal/external awareness (interface_selector.go)
- [x] **NAT detection** - Automatic NAT detection (interface_selector.go)
- [x] **Symmetric NAT** - Handle symmetric NAT (loop_protection.go)
- [x] **Address learning** - Learn addresses from packets (loop_protection.go)
- [x] **Port latching** - First-packet port latching (loop_protection.go)
- [x] **Multiple NICs** - Support multiple interfaces (interface_selector.go)

### 2.4 IPv4↔IPv6 Bridging

- [x] **Dual-stack support** - IPv4 and IPv6 simultaneously (ipv6_support.go)
- [x] **IPv4↔IPv6 translation** - Bridge between families (ipv6_support.go)
- [x] **Address family selection** - Per-leg AF choice (ipv6_support.go)
- [x] **DNS resolution** - SRV/NAPTR/A/AAAA lookup (dns_resolver.go)
- [x] **Happy eyeballs** - Fast fallback with RFC 6555 (dns_resolver.go)

### 2.5 ICE Handling

- [x] **ICE-full** - Complete ICE implementation (pion/ice, ice_manager.go)
- [x] **ICE-lite** - ICE-lite mode (session_manager.go ICELite flag)
- [x] **Trickle ICE** - Incremental candidates (session_manager.go TrickleICE flag)
- [x] **ICE restart** - Handle ICE restarts (pion/ice)
- [x] **Candidate gathering** - Host/srflx/relay (ice_manager.go)
- [x] **Connectivity checks** - STUN binding (pion/ice)
- [x] **Nomination** - Regular/aggressive (pion/ice)
- [x] **ICE removal** - Clean ICE stripping (sdp_processor.go)

### 2.6 Media Fast Path

Create an optimized path for simple forwarding:

- [x] **Zero-copy forwarding** - Minimize copies (socket_sharding.go ZeroCopyForwarder)
- [x] **Batch processing** - Process packets in batches (worker_pool.go)
- [x] **Lock-free lookup** - Minimize contention (atomic counters in worker_pool.go)
- [x] **Socket sharding** - Per-core sockets (socket_sharding.go ShardedSocketPool)
- [x] **CPU affinity** - Pin workers to cores (NumCPU*2 workers in worker_pool.go)
- [x] **NUMA awareness** - Memory locality via buffer pools (socket_sharding.go)
- [x] **Bypass transcoding** - Skip when not needed (ShouldTranscodePacket)
- [x] **Bypass recording** - Skip when not enabled (session flags)
- [x] **Bypass analytics** - Skip when not needed (IsDebugLoggingEnabled)

---

## Phase 3: Enterprise Parity

**Goal**: Feature parity with rtpengine enterprise features.

**Timeline**: 6-8 weeks

### 3.1 SIPREC Integration

Implement RFC 7865/7866 SIPREC:

- [x] **SRS (Session Recording Server) mode** - Act as SRS (siprec.go)
- [x] **SRC (Session Recording Client) mode** - Initiate recording (siprec.go)
- [x] **Recording metadata** - Full metadata support (siprec.go)
- [x] **XML body handling** - SIPREC XML parsing (siprec.go)
- [x] **Participant info** - Participant metadata (siprec.go)
- [x] **Stream correlation** - Correlate streams (siprec.go)
- [x] **Recording session** - Separate recording dialog (siprec.go)
- [x] **Selective recording** - Policy-based recording (siprec.go)
- [x] **Recording pause/resume** - Mid-call control (siprec.go)
- [x] **Recording storage** - Multiple backends (recording/manager.go)

### 3.2 T.38 Fax Support

- [x] **T.38 detection** - Detect T.38 in SDP (t38_gateway.go)
- [x] **T.38 passthrough** - Transparent relay (t38_gateway.go)
- [x] **T.38↔audio** - Optional gateway mode (t38_gateway.go)
- [x] **IFP handling** - Internet Fax Protocol (t38_gateway.go)
- [x] **Error correction** - FEC for T.38 (t38_gateway.go)
- [x] **Rate adaptation** - Bitrate handling (t38_gateway.go)
- [x] **V.21 detection** - Fax tone detection (v21_detection.go)
- [x] **Re-INVITE handling** - Audio↔T.38 switch (t38_gateway.go)

### 3.3 Advanced Transcoding

- [x] **G.722** - Wideband codec (codec_converter.go)
- [x] **G.729** - Low bitrate codec (g729_codec.go, using bcg729-compatible API)
- [x] **AMR** - Mobile codec (amr_codec.go)
- [x] **AMR-WB** - Wideband AMR (amr_codec.go)
- [x] **iLBC** - Internet low bitrate (ilbc_codec.go)
- [x] **Speex** - Legacy codec (speex_codec.go)
- [ ] **VP8/VP9** - Video codecs (future)
- [ ] **H.264** - Video codec (future)
- [x] **Transcode chaining** - Multiple transcodes (codec_converter.go)
- [x] **Bitrate adaptation** - Dynamic bitrate (codec_converter.go)

### 3.4 Multi-Node Clustering

- [x] **Distributed session state** - Redis/etcd backend (redis_cluster.go)
- [x] **Session ownership** - Clear ownership model (redis_cluster.go)
- [x] **Ownership transfer** - On node failure (redis_cluster.go)
- [x] **Idempotent commands** - Safe retries (redis_cluster.go)
- [x] **Consistent hashing** - Sticky placement (consistent_hash.go)
- [x] **Health monitoring** - Node health checks (health.go)
- [x] **Drain mode** - Graceful shutdown (health.go)
- [x] **Split brain handling** - Partition tolerance (split_brain.go)
- [x] **CDR coordination** - Distributed CDRs (cdr_coordination.go)
- [x] **Recording coordination** - Shared recording state (redis_cluster.go)

### 3.5 Failover Semantics

- [x] **Call preservation** - Survive node failure (redis_cluster.go)
- [x] **Port re-allocation** - Consistent ports (port_reallocation.go)
- [x] **Media recovery** - Resume media flow (redis_cluster.go)
- [x] **State recovery** - Recover from backend (redis_cluster.go)
- [x] **Proxy notification** - Inform SIP proxy (proxy_notification.go)
- [x] **Recording continuity** - No recording gaps (recording_continuity.go)
- [x] **Statistics continuity** - Preserve stats (redis_cluster.go)

---

## Phase 4: Operational Maturity

**Goal**: Production-hardened for large-scale deployment.

**Timeline**: 4 weeks

### 4.1 Performance Engineering

- [x] **Benchmark suite** - Reproducible benchmarks (benchmark.go)
- [x] **Profiling integration** - pprof endpoints (pprof_server.go)
- [x] **Memory optimization** - Reduce allocations (buffer pools)
- [x] **GC tuning** - Optimize GC behavior (pprof_server.go SetGCPercent/SetMemoryLimit)
- [x] **Buffer pooling** - Reuse buffers (pprof_server.go RTPBufferPool, socket_sharding.go)
- [x] **Connection pooling** - Reuse connections (connection_pool.go)
- [x] **Batch RTCP** - Batch RTCP processing (batch_rtcp.go)
- [x] **Async recording** - Non-blocking recording (async_recording.go)

### 4.2 Operational Safety

- [x] **Port exhaustion protection** - Pre-allocation (port_allocator.go)
- [x] **Memory limits** - Bounded memory usage (pprof_server.go SetMemoryLimit)
- [x] **Rate limiting** - NG protocol rate limits (rate_limiter.go)
- [x] **Backpressure** - Recording backpressure (backpressure.go)
- [x] **Circuit breakers** - Dependency failures (circuit_breaker.go)
- [x] **Config validation** - Startup validation (config_validator.go)
- [x] **Hot reload** - Config reload without restart (hot_reload.go)
- [x] **Graceful drain** - Drain before shutdown (graceful_shutdown.go)

### 4.3 Observability

- [x] **High-cardinality metrics** - Per-call optional (high_cardinality_metrics.go)
- [x] **Distributed tracing** - OpenTelemetry-compatible (tracing.go)
- [x] **Structured logging** - JSON with context (structured_logger.go)
- [x] **Audit logging** - Control plane audit (structured_logger.go AuditLogger)
- [x] **CDR export** - Multiple formats (cdr.go - JSON, CSV, Syslog)
- [x] **Call flow logging** - Debug mode (structured_logger.go CallLogger)
- [x] **PCAP capture** - On-demand capture (pcap_capture.go)
- [x] **Quality alerting** - Automatic alerts (quality_alerting.go)

### 4.4 Security Hardening

- [x] **Input validation** - All inputs validated (input_validator.go)
- [x] **DoS protection** - Rate limits, size limits (dos_protection.go)
- [x] **Authentication** - API authentication (authentication.go)
- [x] **Authorization** - Per-operation authz (authorization.go)
- [x] **TLS everywhere** - All control plane (tls_config.go)
- [x] **Secrets management** - No plaintext secrets (secrets_manager.go)
- [x] **Audit trail** - Security audit log (structured_logger.go AuditLogger)

---

## Phase 5: Testing & Validation

**Goal**: Proven correct under real-world conditions.

**Timeline**: Ongoing

### 5.1 Unit Tests

- [x] NG protocol parser (ng_protocol/parser_test.go)
- [x] SDP parser/generator (sdp_parser_test.go)
- [x] RTP/RTCP handling (rtp_test.go)
- [x] SRTP crypto (existing tests via pion/srtp)
- [x] ICE state machine (existing tests via pion/ice)
- [x] Session state machine (session_state_test.go)
- [x] Codec transcoding (codec_test.go, g729_codec_test.go)
- [x] Recording output (existing recording tests)

### 5.2 Integration Tests

- [ ] OpenSIPS integration
- [ ] Kamailio integration
- [ ] FreeSWITCH integration
- [ ] Asterisk integration
- [ ] WebRTC browser tests
- [ ] SIP phone tests
- [ ] Multi-node tests

### 5.3 Scenario Tests

- [ ] SIP↔SIP audio call
- [ ] WebRTC↔SIP call
- [ ] SRTP↔RTP bridging
- [ ] IPv4↔IPv6 bridging
- [ ] NATed endpoints
- [ ] Re-INVITE scenarios
- [ ] Hold/resume
- [ ] Transfer scenarios
- [ ] Conference scenarios
- [ ] Recording scenarios
- [ ] SIPREC scenarios
- [ ] T.38 fax scenarios
- [ ] DTMF scenarios
- [ ] Codec fallback

### 5.4 Stress Tests

- [ ] 10,000 concurrent sessions
- [ ] Sustained load (24h+)
- [ ] Burst traffic
- [ ] Memory leak detection
- [ ] Connection exhaustion
- [ ] Port exhaustion
- [ ] Recording under load
- [ ] Transcoding under load

### 5.5 Chaos Tests

- [ ] Node failure mid-call
- [ ] Redis failure
- [ ] Network partition
- [ ] Packet loss (10%, 30%, 50%)
- [ ] Packet reordering
- [ ] Jitter injection
- [ ] CPU saturation
- [ ] Memory pressure
- [ ] Disk full (recording)

---

## Implementation Priority Matrix

| Feature | Impact | Effort | Priority |
|---------|--------|--------|----------|
| NG protocol flags | Critical | Medium | P0 |
| NAT/interface parity | Critical | Medium | P0 |
| ICE-lite mode | High | Low | P1 |
| SRTP↔RTP gateway | High | Medium | P1 |
| IPv4↔IPv6 bridging | High | Medium | P1 |
| Media fast path | High | High | P1 |
| SIPREC | High | High | P2 |
| T.38 passthrough | Medium | Medium | P2 |
| Clustering/failover | High | High | P2 |
| Advanced transcoding | Medium | High | P3 |
| Video support | Low | High | P4 |

---

## Success Criteria

### Functional Parity

- [ ] OpenSIPS/Kamailio configs work without modification
- [ ] All NG protocol flags produce correct behavior
- [ ] SDP output matches rtpengine for same input
- [ ] Recording output is compatible
- [ ] Statistics match expected format

### Performance Parity

- [ ] Relay latency ≤ rtpengine userspace mode
- [ ] CPU usage comparable for forwarding
- [ ] Memory usage ≤ 2KB per session
- [ ] 10,000+ concurrent sessions stable

### Operational Superiority

- [ ] Better metrics than rtpengine
- [ ] Better health visibility
- [ ] Better Kubernetes integration
- [ ] Better documentation
- [ ] Easier deployment

---

## Timeline Summary

| Phase | Duration | Milestone |
|-------|----------|-----------|
| Phase 1: Control-Plane Parity | 4-6 weeks | NG protocol complete |
| Phase 2: Relay-Grade Media | 4-6 weeks | Production-ready relay |
| Phase 3: Enterprise Parity | 6-8 weeks | SIPREC, T.38, clustering |
| Phase 4: Operational Maturity | 4 weeks | Hardened for scale |
| Phase 5: Testing | Ongoing | Validated behavior |

**Total estimated time to full parity: 18-24 weeks**

---

## Contributing

Areas where contributions are especially welcome:

1. NG protocol flag implementation
2. Test cases from real deployments
3. SIP proxy integration testing
4. Documentation of edge cases
5. Performance optimization
6. Codec support expansion

See [CONTRIBUTING.md](./CONTRIBUTING.md) for guidelines.

---

## References

- [rtpengine GitHub](https://github.com/sipwise/rtpengine)
- [rtpengine NG protocol docs](https://github.com/sipwise/rtpengine/blob/master/docs/ng_protocol.md)
- [RFC 3550 - RTP](https://tools.ietf.org/html/rfc3550)
- [RFC 3711 - SRTP](https://tools.ietf.org/html/rfc3711)
- [RFC 5245 - ICE](https://tools.ietf.org/html/rfc5245)
- [RFC 7865 - SIPREC Architecture](https://tools.ietf.org/html/rfc7865)
- [RFC 7866 - SIPREC Protocol](https://tools.ietf.org/html/rfc7866)
- [T.38 - ITU-T Recommendation](https://www.itu.int/rec/T-REC-T.38)
