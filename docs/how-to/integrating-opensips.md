# How to Integrate Karl with OpenSIPS

This guide covers integrating Karl Media Server with OpenSIPS for RTP proxying, NAT traversal, and media manipulation.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Basic Integration](#basic-integration)
- [Configuration Options](#configuration-options)
- [Common Use Cases](#common-use-cases)
- [Advanced Configuration](#advanced-configuration)
- [High Availability](#high-availability)
- [Troubleshooting](#troubleshooting)

---

## Prerequisites

- OpenSIPS 3.x or later
- Karl Media Server running and accessible
- `rtpengine` module compiled with OpenSIPS

Verify the module is available:

```bash
opensips -C -f /etc/opensips/opensips.cfg 2>&1 | grep rtpengine
```

---

## Basic Integration

### Step 1: Load the Module

Add to your `opensips.cfg`:

```opensips
# Load rtpengine module
loadmodule "rtpengine.so"
```

### Step 2: Configure the Module

```opensips
# Point to Karl Media Server
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

### Step 3: Use in Routing Logic

```opensips
route {
    # Handle INVITE
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            # Engage RTP proxy for the offer
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }

    # Handle BYE
    if (is_method("BYE")) {
        rtpengine_delete();
    }

    # Route the request
    t_relay();
}

# Handle replies
onreply_route[MANAGE_REPLY] {
    if (has_body("application/sdp")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
    }
}
```

### Step 4: Restart OpenSIPS

```bash
systemctl restart opensips
```

---

## Configuration Options

### Module Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `rtpengine_sock` | (none) | Karl server address |
| `rtpengine_tout` | 1 | Socket timeout (seconds) |
| `rtpengine_retr` | 5 | Retry count |
| `queued_msgs_threshold` | 64 | Max queued messages |
| `db_url` | (none) | Database for rtpengine list |

### Example Configuration

```opensips
# Module parameters
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
modparam("rtpengine", "rtpengine_tout", 2)
modparam("rtpengine", "rtpengine_retr", 3)
```

---

## Common Use Cases

### NAT Traversal

Handle calls where one or both endpoints are behind NAT:

```opensips
route {
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            # Detect NAT and apply appropriate flags
            if (nat_uac_test("19")) {
                # Client is behind NAT
                rtpengine_manage("RTP/AVP replace-origin replace-session-connection SIP-source-address");
            } else {
                rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
            }
        }
    }
}
```

### WebRTC to SIP Bridging

Bridge WebRTC clients to SIP endpoints:

```opensips
route {
    if (is_method("INVITE")) {
        if ($ru =~ "transport=ws") {
            # WebRTC client - needs ICE removal and DTLS-SRTP handling
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else {
            # Regular SIP endpoint
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### Call Recording

Enable recording for specific calls:

```opensips
route {
    if (is_method("INVITE")) {
        # Check if recording is required (e.g., based on caller)
        if ($fU == "recorded_user") {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### Codec Filtering

Force specific codecs:

```opensips
route {
    if (is_method("INVITE")) {
        # Only allow G.711 codecs
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection codec-strip-all codec-offer-PCMU codec-offer-PCMA");
    }
}
```

### Media Timeout Handling

Configure session timeouts:

```opensips
route {
    if (is_method("INVITE")) {
        # Set media timeout to 60 seconds
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection media-timeout=60");
    }
}
```

---

## Advanced Configuration

### Complete opensips.cfg Example

```opensips
####### Global Parameters #########
log_level=3
log_stderror=no
log_facility=LOG_LOCAL0

listen=udp:0.0.0.0:5060

####### Modules Section ########

# Core modules
loadmodule "signaling.so"
loadmodule "sl.so"
loadmodule "tm.so"
loadmodule "rr.so"
loadmodule "maxfwd.so"
loadmodule "textops.so"
loadmodule "sipmsgops.so"
loadmodule "mi_fifo.so"

# NAT handling
loadmodule "nathelper.so"

# RTP proxy
loadmodule "rtpengine.so"

####### Module Parameters ########

# Transaction module
modparam("tm", "fr_timeout", 5)
modparam("tm", "fr_inv_timeout", 30)

# Record-Route
modparam("rr", "enable_full_lr", 1)

# NAT helper
modparam("nathelper", "ping_nated_only", 1)
modparam("nathelper", "sipping_bflag", "SIP_PING_FLAG")
modparam("nathelper", "natping_interval", 30)

# RTPEngine (Karl)
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
modparam("rtpengine", "rtpengine_tout", 2)
modparam("rtpengine", "rtpengine_retr", 3)

####### Routing Logic ########

route {
    # Max forwards check
    if (!mf_process_maxfwd_header(10)) {
        sl_send_reply(483, "Too Many Hops");
        exit;
    }

    # Record-Route for dialog-forming requests
    if (is_method("INVITE|SUBSCRIBE")) {
        record_route();
    }

    # Handle sequential requests
    if (has_totag()) {
        if (loose_route()) {
            if (is_method("BYE")) {
                # Delete RTP session on BYE
                rtpengine_delete();
            } else if (is_method("INVITE")) {
                # Re-INVITE handling
                if (has_body("application/sdp")) {
                    route(RTPENGINE);
                }
            }
            route(RELAY);
            exit;
        }
    }

    # Handle CANCEL
    if (is_method("CANCEL")) {
        if (t_check_trans()) {
            rtpengine_delete();
            t_relay();
        }
        exit;
    }

    # Handle initial INVITE
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            route(RTPENGINE);
        }
        t_on_reply("MANAGE_REPLY");
    }

    route(RELAY);
}

route[RTPENGINE] {
    $var(rtpflags) = "RTP/AVP replace-origin replace-session-connection";

    # NAT detection
    if (nat_uac_test("19")) {
        $var(rtpflags) = $var(rtpflags) + " SIP-source-address";
    }

    rtpengine_manage("$var(rtpflags)");
}

route[RELAY] {
    if (!t_relay()) {
        sl_reply_error();
    }
}

onreply_route[MANAGE_REPLY] {
    if (has_body("application/sdp")) {
        route(RTPENGINE);
    }
}

# Failure route - cleanup on call failure
failure_route[CALL_FAILURE] {
    if (t_was_cancelled()) {
        rtpengine_delete();
        exit;
    }
}
```

### Flag Reference

Common rtpengine_manage flags:

| Flag | Description |
|------|-------------|
| `RTP/AVP` | Use RTP/AVP profile |
| `RTP/SAVP` | Use RTP/SAVP (SRTP) profile |
| `replace-origin` | Replace SDP origin |
| `replace-session-connection` | Replace session-level connection |
| `SIP-source-address` | Use SIP source for media |
| `ICE=remove` | Remove ICE candidates |
| `ICE=force` | Force ICE |
| `DTLS=off` | Disable DTLS |
| `SDES-off` | Disable SDES |
| `record-call` | Enable recording |
| `media-timeout=N` | Set media timeout (seconds) |
| `codec-strip-all` | Remove all codecs |
| `codec-offer-X` | Offer codec X |

---

## High Availability

### Multiple Karl Instances

Configure multiple Karl servers:

```opensips
# Primary and backup
modparam("rtpengine", "rtpengine_sock",
    "udp:karl1.example.com:22222 udp:karl2.example.com:22222")
```

### Weighted Distribution

```opensips
# 70% to primary, 30% to secondary
modparam("rtpengine", "rtpengine_sock",
    "7 == udp:karl1.example.com:22222 3 == udp:karl2.example.com:22222")
```

### Database-Backed Configuration

Store Karl servers in database:

```opensips
modparam("rtpengine", "db_url", "mysql://opensips:password@localhost/opensips")
modparam("rtpengine", "db_table", "rtpengine")
```

SQL schema:

```sql
CREATE TABLE rtpengine (
    id INT PRIMARY KEY AUTO_INCREMENT,
    socket VARCHAR(128) NOT NULL,
    set_id INT DEFAULT 0,
    weight INT DEFAULT 1,
    disabled INT DEFAULT 0
);

INSERT INTO rtpengine (socket, set_id, weight) VALUES
    ('udp:karl1.example.com:22222', 0, 10),
    ('udp:karl2.example.com:22222', 0, 10);
```

---

## Troubleshooting

### OpenSIPS Can't Connect to Karl

```bash
# Check Karl is running
echo -n "d7:command4:pinge" | nc -u 127.0.0.1 22222

# Check OpenSIPS logs
tail -f /var/log/opensips.log | grep rtpengine

# Enable debug logging
modparam("rtpengine", "rtpengine_disable", 0)
```

### No Audio in Calls

1. Check SDP is being modified:
```opensips
# Add to your route
xlog("L_INFO", "SDP before: $rb\n");
rtpengine_manage("...");
xlog("L_INFO", "SDP after: $rb\n");
```

2. Verify RTP ports are reachable between endpoints

3. Check Karl logs for session creation

### One-Way Audio

Common causes:
- NAT not detected properly
- Asymmetric routing
- Firewall blocking return traffic

Solution:
```opensips
# Force symmetric RTP
rtpengine_manage("RTP/AVP replace-origin replace-session-connection symmetric");
```

### Recording Not Working

1. Verify recording is enabled in Karl config
2. Check recording directory permissions
3. Confirm the `record-call` flag is being sent:

```opensips
rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
```

### Monitor Active Sessions

```bash
# Via Karl API
curl http://127.0.0.1:8080/api/v1/sessions

# Via OpenSIPS MI
opensips-cli -x mi rtpengine_show_all
```

---

## Next Steps

- [Configure Call Recording](./setting-up-recording.md)
- [Set Up Monitoring](./monitoring-prometheus.md)
- [WebRTC to SIP Bridging](./webrtc-sip-bridging.md)
