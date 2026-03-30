# How to Bridge WebRTC to SIP

This guide covers using Karl to bridge WebRTC browser clients to traditional SIP infrastructure.

## Table of Contents

- [Overview](#overview)
- [How It Works](#how-it-works)
- [Prerequisites](#prerequisites)
- [Configuration](#configuration)
- [SIP Proxy Setup](#sip-proxy-setup)
- [Common Scenarios](#common-scenarios)
- [Codec Handling](#codec-handling)
- [Troubleshooting](#troubleshooting)

---

## Overview

Karl acts as a media gateway between WebRTC and SIP, handling:

- **ICE/STUN/TURN**: NAT traversal for WebRTC clients
- **DTLS-SRTP to RTP**: Encryption bridging
- **Codec transcoding**: Opus to G.711 conversion
- **SDP translation**: WebRTC to SIP-compatible SDP

---

## How It Works

```
┌──────────────┐                    ┌──────────────┐                    ┌──────────────┐
│   Browser    │                    │     Karl     │                    │  SIP Phone   │
│   (WebRTC)   │                    │              │                    │              │
└──────┬───────┘                    └──────┬───────┘                    └──────┬───────┘
       │                                   │                                   │
       │  DTLS-SRTP (Opus)                │  RTP (G.711)                      │
       │  ICE candidates                   │  Plain RTP                        │
       │◄─────────────────────────────────►│◄─────────────────────────────────►│
       │                                   │                                   │
```

### Translation Performed

| WebRTC Side | SIP Side |
|-------------|----------|
| DTLS-SRTP | RTP or SRTP |
| ICE candidates | Direct IP |
| Opus codec | G.711 (PCMU/PCMA) |
| Transport-CC | RTCP |
| Bundle | Separate m-lines |

---

## Prerequisites

- Karl Media Server running
- SIP proxy (OpenSIPS or Kamailio) with WebSocket support
- WebRTC-capable browser application
- STUN/TURN server (for NAT traversal)

---

## Configuration

### Karl Configuration

```json
{
  "webrtc": {
    "enabled": true,
    "stun_servers": [
      "stun:stun.l.google.com:19302",
      "stun:stun1.l.google.com:19302"
    ],
    "turn_servers": [
      {
        "url": "turn:turn.example.com:3478",
        "username": "user",
        "credential": "password"
      }
    ],
    "max_bitrate": 2000000,
    "bw_estimation": true,
    "tcc_enabled": true
  },
  "ng_protocol": {
    "enabled": true,
    "udp_port": 22222
  }
}
```

### Environment Variables

```bash
export KARL_WEBRTC_ENABLED=true
export KARL_STUN_SERVERS="stun:stun.l.google.com:19302"
```

---

## SIP Proxy Setup

### OpenSIPS with WebSocket

```opensips
# Load WebSocket module
loadmodule "proto_ws.so"
loadmodule "proto_wss.so"

# Listen on WebSocket
listen=ws:0.0.0.0:80
listen=wss:0.0.0.0:443

# RTPEngine module
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")

route {
    if (is_method("INVITE")) {
        if ($proto == "ws" || $proto == "wss") {
            # WebRTC to SIP
            # Remove ICE, disable DTLS, convert to RTP
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else if ($ru =~ "transport=ws") {
            # SIP to WebRTC (unlikely but handle it)
            rtpengine_manage("RTP/SAVPF replace-origin replace-session-connection ICE=force DTLS=passive");
        } else {
            # SIP to SIP
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }

    if (is_method("BYE")) {
        rtpengine_delete();
    }

    t_relay();
}

onreply_route[MANAGE_REPLY] {
    if (has_body("application/sdp")) {
        if ($proto == "ws" || $proto == "wss") {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### Kamailio with WebSocket

```kamailio
# Load WebSocket modules
loadmodule "xhttp.so"
loadmodule "websocket.so"

# Listen on WebSocket
listen=tcp:0.0.0.0:80
listen=tls:0.0.0.0:443

# RTPEngine module
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")

request_route {
    if (is_method("INVITE")) {
        if ($proto == "ws" || $proto == "wss") {
            # WebRTC client
            $var(rtpflags) = "RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off";
        } else {
            # Standard SIP
            $var(rtpflags) = "RTP/AVP replace-origin replace-session-connection";
        }

        rtpengine_manage("$var(rtpflags)");
        t_on_reply("MANAGE_REPLY");
    }

    if (is_method("BYE")) {
        rtpengine_delete();
    }

    route(RELAY);
}

onreply_route[MANAGE_REPLY] {
    if (has_body("application/sdp")) {
        if ($proto == "ws" || $proto == "wss") {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}

event_route[xhttp:request] {
    if ($hdr(Upgrade) =~ "websocket") {
        if (ws_handle_handshake()) {
            exit;
        }
    }
    xhttp_reply("404", "Not Found", "", "");
}
```

---

## Common Scenarios

### WebRTC Browser to SIP Phone

Browser initiates call to SIP phone:

```
Browser (WebRTC) ──► SIP Proxy ──► Karl ──► SIP Phone
     │                  │           │           │
     │ INVITE (WS)      │           │           │
     │ SDP: Opus, ICE   │           │           │
     │─────────────────►│           │           │
     │                  │ INVITE    │           │
     │                  │ SDP mod   │           │
     │                  │──────────►│ INVITE    │
     │                  │           │ SDP: G.711│
     │                  │           │──────────►│
```

**Flags used**:
```
ICE=remove    # Remove ICE candidates
DTLS=off      # Disable DTLS
SDES-off      # Disable SDES
RTP/AVP       # Use plain RTP
```

### SIP Phone to WebRTC Browser

SIP phone calls WebRTC client:

```
SIP Phone ──► Karl ──► SIP Proxy ──► Browser (WebRTC)
     │           │          │              │
     │ INVITE    │          │              │
     │ SDP: G.711│          │              │
     │──────────►│ INVITE   │              │
     │           │ SDP mod  │ INVITE (WS)  │
     │           │─────────►│ SDP: Opus    │
     │           │          │─────────────►│
```

**Flags used**:
```
ICE=force     # Add ICE candidates
DTLS=passive  # Enable DTLS in passive mode
RTP/SAVPF     # Use SRTP with feedback
```

### WebRTC to WebRTC via SIP Proxy

Both endpoints are WebRTC:

```
Browser A ──► SIP Proxy ──► Karl ──► SIP Proxy ──► Browser B
```

**Flags**: Karl handles ICE and DTLS on both sides.

---

## Codec Handling

### Automatic Transcoding

Karl automatically transcodes between:

| WebRTC Codec | SIP Codec |
|--------------|-----------|
| Opus | PCMU (G.711 μ-law) |
| Opus | PCMA (G.711 A-law) |

### Force Specific Codec

```opensips
# Strip all codecs except G.711
rtpengine_manage("... codec-strip-all codec-offer-PCMU codec-offer-PCMA");
```

### Codec Preference

```opensips
# Prefer PCMU over PCMA
rtpengine_manage("... codec-mask-PCMA");
```

---

## NAT Traversal

### STUN Configuration

STUN servers help discover public IP:

```json
{
  "webrtc": {
    "stun_servers": [
      "stun:stun.l.google.com:19302",
      "stun:stun.example.com:3478"
    ]
  }
}
```

### TURN Configuration

TURN servers relay traffic when direct connection fails:

```json
{
  "webrtc": {
    "turn_servers": [
      {
        "url": "turn:turn.example.com:3478",
        "username": "webrtc",
        "credential": "secret123"
      },
      {
        "url": "turns:turn.example.com:5349",
        "username": "webrtc",
        "credential": "secret123"
      }
    ]
  }
}
```

### ICE Handling Flags

| Flag | Description |
|------|-------------|
| `ICE=remove` | Remove all ICE attributes |
| `ICE=force` | Force ICE negotiation |
| `ICE=force-relay` | Force TURN relay |

---

## Security Considerations

### SRTP Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `DTLS=off` | Disable DTLS-SRTP | WebRTC to plain RTP |
| `DTLS=passive` | DTLS passive role | SIP to WebRTC |
| `DTLS=active` | DTLS active role | Karl initiates DTLS |
| `SDES-off` | Disable SDES | Don't offer SDES |
| `SDES-on` | Enable SDES | Offer SDES keys |

### Recommended Configuration

```opensips
# WebRTC to SIP (remove encryption)
rtpengine_manage("RTP/AVP ... DTLS=off SDES-off");

# SIP to WebRTC (add encryption)
rtpengine_manage("RTP/SAVPF ... DTLS=passive ICE=force");

# WebRTC to WebRTC (keep encryption)
rtpengine_manage("RTP/SAVPF ... DTLS=passive ICE=force");
```

---

## Troubleshooting

### No Audio from WebRTC Client

**Diagnosis**:
```bash
# Check ICE is being handled
grep -i "ICE\|DTLS" /var/log/karl.log

# Verify session SDP
curl http://localhost:8080/api/v1/sessions | jq .
```

**Common Causes**:
1. ICE not removed for SIP-side
2. DTLS handshake failing
3. STUN/TURN not reachable

**Solutions**:
```opensips
# Ensure ICE removal
rtpengine_manage("... ICE=remove");

# Check TURN connectivity
# Browser console should show ICE gathering complete
```

### One-Way Audio

**WebRTC hears SIP but not vice versa**:
- DTLS handshake incomplete
- ICE connectivity check failing

**SIP hears WebRTC but not vice versa**:
- NAT not handled for SIP endpoint
- Firewall blocking return traffic

**Solution**:
```bash
# Enable symmetric RTP
rtpengine_manage("... symmetric");
```

### Codec Negotiation Failure

**Symptoms**: No common codec, call fails

**Diagnosis**:
```bash
# Check offered codecs in logs
grep -i "codec\|offer" /var/log/karl.log
```

**Solution**:
```opensips
# Always transcode to G.711
rtpengine_manage("... transcode-PCMU transcode-PCMA");
```

### DTLS Handshake Timeout

**Symptoms**: Call setup delays, eventual failure

**Diagnosis**:
```bash
# Check DTLS logs
grep -i DTLS /var/log/karl.log
```

**Solutions**:
1. Verify certificate configuration
2. Check UDP connectivity on DTLS ports
3. Ensure firewall allows UDP traffic

### ICE Gathering Slow

**Symptoms**: Long call setup time

**Solutions**:
1. Use closer STUN/TURN servers
2. Reduce ICE candidates gathered
3. Check network latency to STUN servers

---

## WebRTC Client Example

### JavaScript SIP.js Example

```javascript
const ua = new SIP.UA({
  uri: 'sip:user@example.com',
  transportOptions: {
    wsServers: ['wss://proxy.example.com:443']
  },
  sessionDescriptionHandlerFactoryOptions: {
    peerConnectionOptions: {
      iceServers: [
        { urls: 'stun:stun.l.google.com:19302' },
        {
          urls: 'turn:turn.example.com:3478',
          username: 'user',
          credential: 'password'
        }
      ]
    }
  }
});

// Make a call
const session = ua.invite('sip:destination@example.com', {
  sessionDescriptionHandlerOptions: {
    constraints: {
      audio: true,
      video: false
    }
  }
});
```

### JsSIP Example

```javascript
const socket = new JsSIP.WebSocketInterface('wss://proxy.example.com:443');
const configuration = {
  sockets: [socket],
  uri: 'sip:user@example.com',
  password: 'secret'
};

const ua = new JsSIP.UA(configuration);

ua.start();

// Make a call
const eventHandlers = {
  'progress': (e) => console.log('call is in progress'),
  'failed': (e) => console.log('call failed'),
  'ended': (e) => console.log('call ended'),
  'confirmed': (e) => console.log('call confirmed')
};

const options = {
  eventHandlers: eventHandlers,
  mediaConstraints: { audio: true, video: false },
  pcConfig: {
    iceServers: [
      { urls: 'stun:stun.l.google.com:19302' }
    ]
  }
};

ua.call('sip:destination@example.com', options);
```

---

## Next Steps

- [OpenSIPS Integration](./integrating-opensips.md)
- [Kamailio Integration](./integrating-kamailio.md)
- [Troubleshooting Guide](./troubleshooting.md)
