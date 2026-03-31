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
| Performance isolation (fast path) | Medium | In Progress |
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
- [ ] `play-media` - Play media file
- [ ] `stop-media` - Stop media playback

#### Quality/Timeout
- [x] `media-timeout` - Media timeout
- [x] `session-timeout` - Session timeout
- [x] `TOS` - Set TOS/DSCP
- [x] `delete-delay` - Delay before delete
- [ ] `delay-buffer` - Delay buffer config
- [ ] `frequency` - RTCP interval

### 1.2 Behavioral Semantics

Beyond flag support, implement correct behavioral semantics:

- [ ] **Per-leg state machine** - Track offer/answer state per leg
- [ ] **Label resolution** - Resolve legs by label, not just tags
- [ ] **Forked call handling** - Support multiple to-tags
- [ ] **Re-INVITE semantics** - Proper mid-call SDP renegotiation
- [ ] **UPDATE handling** - SIP UPDATE method support
- [ ] **Early media** - Correct early media behavior
- [ ] **Hold/resume** - Proper hold detection and handling
- [ ] **Asymmetric RTP** - Support different send/receive paths
- [ ] **SSRC changes** - Handle SSRC changes mid-call
- [ ] **PT remapping** - Payload type remapping

### 1.3 Response Format Parity

Ensure responses match rtpengine format exactly:

- [ ] `query` response format with stats
- [ ] `list` response format with call details
- [ ] Error responses with correct reason strings
- [ ] Statistics field names and units

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

- [ ] **Strict RFC 3550 compliance** - Validate all RTP/RTCP
- [ ] **SSRC collision detection** - Handle collisions properly
- [ ] **Sequence number handling** - Correct wrap-around
- [ ] **Timestamp handling** - Proper timestamp maintenance
- [ ] **CSRC handling** - Contributor source handling
- [ ] **Extension header support** - RTP extension headers
- [ ] **Padding support** - RTP padding handling
- [ ] **Marker bit handling** - Proper marker semantics

### 2.2 SRTP Handling

- [ ] **AES-CM encryption** - Counter mode
- [ ] **AES-GCM encryption** - GCM mode
- [ ] **HMAC-SHA1 auth** - Authentication
- [ ] **Key derivation** - Proper key derivation
- [ ] **ROC handling** - Roll-over counter
- [ ] **MKI support** - Master Key Identifier
- [ ] **Crypto suite negotiation** - All standard suites
- [ ] **SRTP↔RTP gateway** - Clean transcryption

### 2.3 NAT/Interface Logic

Implement rtpengine-compatible interface handling:

- [ ] **Named interfaces** - Define interfaces in config
- [ ] **Interface selection** - Per-call interface choice
- [ ] **Peer-based routing** - Select interface by peer
- [ ] **Direction hints** - internal/external awareness
- [ ] **NAT detection** - Automatic NAT detection
- [ ] **Symmetric NAT** - Handle symmetric NAT
- [ ] **Address learning** - Learn addresses from packets
- [ ] **Port latching** - First-packet port latching
- [ ] **Multiple NICs** - Support multiple interfaces

### 2.4 IPv4↔IPv6 Bridging

- [ ] **Dual-stack support** - IPv4 and IPv6 simultaneously
- [ ] **IPv4↔IPv6 translation** - Bridge between families
- [ ] **Address family selection** - Per-leg AF choice
- [ ] **DNS resolution** - A/AAAA lookup
- [ ] **Happy eyeballs** - Fast fallback

### 2.5 ICE Handling

- [ ] **ICE-full** - Complete ICE implementation
- [ ] **ICE-lite** - ICE-lite mode
- [ ] **Trickle ICE** - Incremental candidates
- [ ] **ICE restart** - Handle ICE restarts
- [ ] **Candidate gathering** - Host/srflx/relay
- [ ] **Connectivity checks** - STUN binding
- [ ] **Nomination** - Regular/aggressive
- [ ] **ICE removal** - Clean ICE stripping

### 2.6 Media Fast Path

Create an optimized path for simple forwarding:

- [ ] **Zero-copy forwarding** - Minimize copies
- [ ] **Batch processing** - Process packets in batches
- [ ] **Lock-free lookup** - Minimize contention
- [ ] **Socket sharding** - Per-core sockets
- [ ] **CPU affinity** - Pin workers to cores
- [ ] **NUMA awareness** - Memory locality
- [ ] **Bypass transcoding** - Skip when not needed
- [ ] **Bypass recording** - Skip when not enabled
- [ ] **Bypass analytics** - Skip when not needed

---

## Phase 3: Enterprise Parity

**Goal**: Feature parity with rtpengine enterprise features.

**Timeline**: 6-8 weeks

### 3.1 SIPREC Integration

Implement RFC 7865/7866 SIPREC:

- [ ] **SRS (Session Recording Server) mode** - Act as SRS
- [ ] **SRC (Session Recording Client) mode** - Initiate recording
- [ ] **Recording metadata** - Full metadata support
- [ ] **XML body handling** - SIPREC XML parsing
- [ ] **Participant info** - Participant metadata
- [ ] **Stream correlation** - Correlate streams
- [ ] **Recording session** - Separate recording dialog
- [ ] **Selective recording** - Policy-based recording
- [ ] **Recording pause/resume** - Mid-call control
- [ ] **Recording storage** - Multiple backends

### 3.2 T.38 Fax Support

- [ ] **T.38 detection** - Detect T.38 in SDP
- [ ] **T.38 passthrough** - Transparent relay
- [ ] **T.38↔audio** - Optional gateway mode
- [ ] **IFP handling** - Internet Fax Protocol
- [ ] **Error correction** - FEC for T.38
- [ ] **Rate adaptation** - Bitrate handling
- [ ] **V.21 detection** - Fax tone detection
- [ ] **Re-INVITE handling** - Audio↔T.38 switch

### 3.3 Advanced Transcoding

- [ ] **G.722** - Wideband codec
- [ ] **G.729** - Low bitrate (licensing required)
- [ ] **AMR** - Mobile codec
- [ ] **AMR-WB** - Wideband AMR
- [ ] **iLBC** - Internet low bitrate
- [ ] **Speex** - Legacy codec
- [ ] **VP8/VP9** - Video codecs (future)
- [ ] **H.264** - Video codec (future)
- [ ] **Transcode chaining** - Multiple transcodes
- [ ] **Bitrate adaptation** - Dynamic bitrate

### 3.4 Multi-Node Clustering

- [ ] **Distributed session state** - Redis/etcd backend
- [ ] **Session ownership** - Clear ownership model
- [ ] **Ownership transfer** - On node failure
- [ ] **Idempotent commands** - Safe retries
- [ ] **Consistent hashing** - Sticky placement
- [ ] **Health monitoring** - Node health checks
- [ ] **Drain mode** - Graceful shutdown
- [ ] **Split brain handling** - Partition tolerance
- [ ] **CDR coordination** - Distributed CDRs
- [ ] **Recording coordination** - Shared recording state

### 3.5 Failover Semantics

- [ ] **Call preservation** - Survive node failure
- [ ] **Port re-allocation** - Consistent ports
- [ ] **Media recovery** - Resume media flow
- [ ] **State recovery** - Recover from backend
- [ ] **Proxy notification** - Inform SIP proxy
- [ ] **Recording continuity** - No recording gaps
- [ ] **Statistics continuity** - Preserve stats

---

## Phase 4: Operational Maturity

**Goal**: Production-hardened for large-scale deployment.

**Timeline**: 4 weeks

### 4.1 Performance Engineering

- [ ] **Benchmark suite** - Reproducible benchmarks
- [ ] **Profiling integration** - pprof endpoints
- [ ] **Memory optimization** - Reduce allocations
- [ ] **GC tuning** - Optimize GC behavior
- [ ] **Buffer pooling** - Reuse buffers
- [ ] **Connection pooling** - Reuse connections
- [ ] **Batch RTCP** - Batch RTCP processing
- [ ] **Async recording** - Non-blocking recording

### 4.2 Operational Safety

- [ ] **Port exhaustion protection** - Pre-allocation
- [ ] **Memory limits** - Bounded memory usage
- [ ] **Rate limiting** - NG protocol rate limits
- [ ] **Backpressure** - Recording backpressure
- [ ] **Circuit breakers** - Dependency failures
- [ ] **Config validation** - Startup validation
- [ ] **Hot reload** - Config reload without restart
- [ ] **Graceful drain** - Drain before shutdown

### 4.3 Observability

- [ ] **High-cardinality metrics** - Per-call optional
- [ ] **Distributed tracing** - OpenTelemetry
- [ ] **Structured logging** - JSON with context
- [ ] **Audit logging** - Control plane audit
- [ ] **CDR export** - Multiple formats
- [ ] **Call flow logging** - Debug mode
- [ ] **PCAP capture** - On-demand capture
- [ ] **Quality alerting** - Automatic alerts

### 4.4 Security Hardening

- [ ] **Input validation** - All inputs validated
- [ ] **DoS protection** - Rate limits, size limits
- [ ] **Authentication** - API authentication
- [ ] **Authorization** - Per-operation authz
- [ ] **TLS everywhere** - All control plane
- [ ] **Secrets management** - No plaintext secrets
- [ ] **Audit trail** - Security audit log

---

## Phase 5: Testing & Validation

**Goal**: Proven correct under real-world conditions.

**Timeline**: Ongoing

### 5.1 Unit Tests

- [ ] NG protocol parser
- [ ] SDP parser/generator
- [ ] RTP/RTCP handling
- [ ] SRTP crypto
- [ ] ICE state machine
- [ ] Session state machine
- [ ] Codec transcoding
- [ ] Recording output

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
