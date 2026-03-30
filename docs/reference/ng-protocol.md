# NG Protocol Reference

Complete reference for the rtpengine-compatible NG (Next Generation) protocol implemented by Karl.

## Table of Contents

- [Protocol Overview](#protocol-overview)
- [Message Format](#message-format)
- [Commands](#commands)
- [Flags Reference](#flags-reference)
- [Response Format](#response-format)
- [Error Handling](#error-handling)

---

## Protocol Overview

The NG protocol is a UDP-based protocol using bencode encoding for communication between SIP proxies and media servers.

### Connection

- **Default Port**: UDP 22222
- **Encoding**: Bencode
- **Request/Response**: Synchronous

### Basic Flow

```
SIP Proxy                    Karl
    │                          │
    │  ping                    │
    │─────────────────────────►│
    │                          │
    │  pong                    │
    │◄─────────────────────────│
    │                          │
```

---

## Message Format

### Request Format

```
<cookie> <bencoded-dictionary>
```

- **cookie**: Unique identifier to match responses (string)
- **bencoded-dictionary**: Command and parameters

### Example

```
abc123 d7:command5:offer7:call-id10:call-567898:from-tag9:tag-12345 3:sdp...e
```

### Bencode Basics

| Type | Format | Example |
|------|--------|---------|
| String | `<length>:<data>` | `4:ping` |
| Integer | `i<number>e` | `i42e` |
| List | `l<items>e` | `l4:spam4:eggse` |
| Dictionary | `d<key><value>...e` | `d3:cow3:mooe` |

---

## Commands

### ping

Health check command.

**Request**:
```
d7:command4:pinge
```

**Response**:
```
d6:result4:ponge
```

**Use**: Keepalive, health monitoring

---

### offer

Process SDP offer for new or existing call.

**Required Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `command` | string | `offer` |
| `call-id` | string | Unique call identifier |
| `from-tag` | string | SIP From-tag |
| `sdp` | string | SDP offer body |

**Optional Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `to-tag` | string | SIP To-tag (for re-INVITE) |
| `flags` | list | Processing flags |
| `replace` | list | SDP elements to replace |
| `direction` | list | Media direction hints |
| `ICE` | string | ICE handling mode |
| `DTLS` | string | DTLS handling mode |
| `SDES` | string | SDES handling mode |
| `transport-protocol` | string | Force transport protocol |
| `media-address` | string | Override media address |

**Example Request**:
```
d7:command5:offer7:call-id10:call-567898:from-tag9:tag-123453:sdpX:v=0...e
```

**Response**:
```
d6:result2:ok3:sdpX:v=0...e
```

---

### answer

Process SDP answer for existing call.

**Required Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `command` | string | `answer` |
| `call-id` | string | Call identifier |
| `from-tag` | string | SIP From-tag |
| `to-tag` | string | SIP To-tag |
| `sdp` | string | SDP answer body |

**Optional Parameters**:

Same as `offer`.

**Example Request**:
```
d7:command6:answer7:call-id10:call-567898:from-tag9:tag-123456:to-tag9:tag-678903:sdpX:v=0...e
```

**Response**:
```
d6:result2:ok3:sdpX:v=0...e
```

---

### delete

Terminate call and release resources.

**Required Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `command` | string | `delete` |
| `call-id` | string | Call identifier |

**Optional Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `from-tag` | string | Delete specific leg |
| `to-tag` | string | Delete specific leg |
| `flags` | list | Processing flags |

**Example Request**:
```
d7:command6:delete7:call-id10:call-56789e
```

**Response**:
```
d6:result2:oke
```

---

### query

Get real-time statistics for a call.

**Required Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `command` | string | `query` |
| `call-id` | string | Call identifier |

**Response Fields**:

| Field | Type | Description |
|-------|------|-------------|
| `result` | string | `ok` or error |
| `totals` | dict | Aggregate statistics |
| `tags` | dict | Per-tag statistics |
| `created` | int | Creation timestamp |
| `last-signal` | int | Last signaling timestamp |

**Example Response**:
```
d6:result2:ok6:totalsd12:packets-sent i1000e16:packets-receivedi950e...ee
```

---

### list

List all active calls.

**Required Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `command` | string | `list` |

**Optional Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `limit` | int | Max results |

**Response**:
```
d6:result2:ok5:callsl d7:call-id10:call-12345 8:from-tag9:tag-11111 6:to-tag9:tag-22222 5:state6:active e ...ee
```

---

### start recording

Begin call recording.

**Required Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `command` | string | `start recording` |
| `call-id` | string | Call identifier |

**Optional Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `from-tag` | string | Record specific leg |
| `to-tag` | string | Record specific leg |

**Response**:
```
d6:result2:oke
```

---

### stop recording

End call recording.

**Required Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `command` | string | `stop recording` |
| `call-id` | string | Call identifier |

---

### block media

Mute media in one or both directions.

**Required Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `command` | string | `block media` |
| `call-id` | string | Call identifier |

**Optional Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `from-tag` | string | Specific leg |
| `direction` | list | `from-tag`, `to-tag`, or both |

---

### unblock media

Unmute media.

**Parameters**: Same as `block media`.

---

### play DTMF

Inject DTMF tones.

**Required Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `command` | string | `play DTMF` |
| `call-id` | string | Call identifier |
| `dtmf` | string | DTMF digits (0-9, *, #) |

**Optional Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `from-tag` | string | Inject toward this leg |
| `duration` | int | Tone duration (ms) |
| `pause` | int | Inter-digit pause (ms) |

---

### statistics

Get server-wide statistics.

**Required Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `command` | string | `statistics` |

**Response Fields**:

| Field | Description |
|-------|-------------|
| `currentstatistics` | Current counters |
| `totalstatistics` | Lifetime counters |
| `sessions` | Active session count |
| `uptime` | Server uptime (seconds) |

---

## Flags Reference

### Transport Protocol Flags

| Flag | Description |
|------|-------------|
| `RTP/AVP` | Plain RTP (Audio/Video Profile) |
| `RTP/AVPF` | RTP with feedback |
| `RTP/SAVP` | SRTP (Secure AVP) |
| `RTP/SAVPF` | SRTP with feedback |
| `UDP/TLS/RTP/SAVP` | DTLS-SRTP |
| `UDP/TLS/RTP/SAVPF` | DTLS-SRTP with feedback |

### SDP Manipulation Flags

| Flag | Description |
|------|-------------|
| `replace-origin` | Replace SDP origin line |
| `replace-session-connection` | Replace session-level connection |
| `trust-address` | Trust SDP addresses |
| `SIP-source-address` | Use SIP source as media address |
| `symmetric` | Force symmetric RTP |
| `asymmetric` | Allow asymmetric RTP |

### ICE Flags

| Flag | Description |
|------|-------------|
| `ICE=remove` | Remove all ICE attributes |
| `ICE=force` | Force ICE negotiation |
| `ICE=force-relay` | Force TURN relay |

### DTLS Flags

| Flag | Description |
|------|-------------|
| `DTLS=off` | Disable DTLS |
| `DTLS=passive` | DTLS passive role |
| `DTLS=active` | DTLS active role |

### SDES Flags

| Flag | Description |
|------|-------------|
| `SDES-off` | Disable SDES |
| `SDES-on` | Enable SDES |
| `SDES-unencrypted_srtp` | Allow unencrypted SRTP |
| `SDES-unencrypted_srtcp` | Allow unencrypted SRTCP |

### Codec Flags

| Flag | Description |
|------|-------------|
| `codec-strip-all` | Remove all codecs |
| `codec-offer-XXXX` | Add codec XXXX to offer |
| `codec-mask-XXXX` | Remove codec XXXX |
| `transcode-XXXX` | Transcode to codec XXXX |

### Recording Flags

| Flag | Description |
|------|-------------|
| `record-call` | Enable call recording |

### Media Control Flags

| Flag | Description |
|------|-------------|
| `media-timeout=N` | Media timeout in seconds |
| `receive-only` | Receive-only mode |
| `send-only` | Send-only mode |

---

## Response Format

### Success Response

```
d6:result2:ok...e
```

### Error Response

```
d6:result5:error12:error-reason18:Invalid parameterse
```

### Common Error Reasons

| Error | Description |
|-------|-------------|
| `Unknown call-id` | Session not found |
| `Invalid SDP` | Malformed SDP |
| `Port allocation failed` | No available ports |
| `Codec negotiation failed` | No common codecs |
| `Session limit reached` | Max sessions exceeded |

---

## Error Handling

### Timeout Handling

If no response within timeout:
1. Retry the request (up to configured retries)
2. Mark instance as failed
3. Failover to backup instance

### Bencode Parsing Errors

Invalid bencode results in:
```
d6:result5:error12:error-reason14:Parse error: ...e
```

### Session State Errors

Attempting operations on invalid session state:
```
d6:result5:error12:error-reason20:Invalid session statee
```

---

## Testing

### Manual Testing

```bash
# Ping test
echo -n "test123 d7:command4:pinge" | nc -u localhost 22222

# Expected response contains "pong"
```

### OpenSIPS/Kamailio Testing

```bash
# In SIP proxy config, enable debug logging for rtpengine module
# Then make a test call and check logs
```

---

## Next Steps

- [REST API Reference](./rest-api.md)
- [Metrics Reference](./metrics.md)
- [Configuration Reference](../configuration.md)
