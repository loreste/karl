# How to Integrate Karl with Kamailio

This guide covers integrating Karl Media Server with Kamailio for RTP proxying, NAT traversal, and media handling.

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

- Kamailio 5.x or later
- Karl Media Server running and accessible
- `rtpengine` module compiled with Kamailio

Verify the module is available:

```bash
kamailio -l | grep rtpengine
```

---

## Basic Integration

### Step 1: Load the Module

Add to your `kamailio.cfg`:

```kamailio
# Load rtpengine module
loadmodule "rtpengine.so"
```

### Step 2: Configure the Module

```kamailio
# Point to Karl Media Server
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

### Step 3: Use in Routing Logic

```kamailio
request_route {
    # Handle INVITE
    if (is_method("INVITE")) {
        if (has_body("application/sdp")) {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
        t_on_reply("MANAGE_REPLY");
    }

    # Handle BYE
    if (is_method("BYE")) {
        rtpengine_delete();
    }

    # Route the request
    route(RELAY);
}

onreply_route[MANAGE_REPLY] {
    if (has_body("application/sdp")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
    }
}

route[RELAY] {
    if (!t_relay()) {
        sl_reply_error();
    }
    exit;
}
```

### Step 4: Restart Kamailio

```bash
systemctl restart kamailio
```

---

## Configuration Options

### Module Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `rtpengine_sock` | (none) | Karl server address |
| `rtpengine_disable_tout` | 60 | Disable timeout after failure (seconds) |
| `rtpengine_tout_ms` | 1000 | Socket timeout (milliseconds) |
| `rtpengine_retr` | 5 | Retry count |
| `queried_nodes_limit` | 0 | Max nodes to query (0=all) |
| `setid_default` | 0 | Default set ID |

### Example Configuration

```kamailio
# Module parameters
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
modparam("rtpengine", "rtpengine_tout_ms", 2000)
modparam("rtpengine", "rtpengine_retr", 3)
modparam("rtpengine", "rtpengine_disable_tout", 30)
```

---

## Common Use Cases

### NAT Traversal

Handle endpoints behind NAT:

```kamailio
request_route {
    if (is_method("INVITE")) {
        # Detect NAT
        if (nat_uac_test(64)) {
            # Force relay and fix NAT
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection SIP-source-address");
            setbflag(FLB_NATB);
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### WebRTC Gateway

Bridge WebRTC to SIP:

```kamailio
request_route {
    if (is_method("INVITE")) {
        if ($proto == "ws" || $proto == "wss") {
            # WebRTC endpoint
            xlog("L_INFO", "WebRTC call from $fU\n");
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection ICE=remove DTLS=off SDES-off");
        } else if (has_body("application/sdp")) {
            # Regular SIP endpoint
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### Call Recording

Enable recording on demand:

```kamailio
request_route {
    if (is_method("INVITE")) {
        # Record calls from specific numbers
        if ($fU =~ "^record") {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
        } else {
            rtpengine_manage("RTP/AVP replace-origin replace-session-connection");
        }
    }
}
```

### Codec Transcoding

Force codec negotiation:

```kamailio
request_route {
    if (is_method("INVITE")) {
        # Strip all codecs and offer only G.711
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection codec-strip-all codec-offer-PCMU codec-offer-PCMA");
    }
}
```

### SRTP Enforcement

Force SRTP for all calls:

```kamailio
request_route {
    if (is_method("INVITE")) {
        # Force SRTP
        rtpengine_manage("RTP/SAVP replace-origin replace-session-connection SDES-on");
    }
}
```

---

## Advanced Configuration

### Complete kamailio.cfg Example

```kamailio
#!KAMAILIO

####### Global Parameters #########

#!define WITH_NAT
#!define WITH_RTPENGINE

debug=2
log_stderror=no
log_facility=LOG_LOCAL0
fork=yes
children=4

listen=udp:0.0.0.0:5060
listen=tcp:0.0.0.0:5060

####### Modules Section ########

loadmodule "tm.so"
loadmodule "sl.so"
loadmodule "rr.so"
loadmodule "pv.so"
loadmodule "maxfwd.so"
loadmodule "textops.so"
loadmodule "siputils.so"
loadmodule "xlog.so"
loadmodule "sanity.so"
loadmodule "nathelper.so"
loadmodule "rtpengine.so"

####### Module Parameters ########

# Transaction module
modparam("tm", "fr_timer", 5000)
modparam("tm", "fr_inv_timer", 30000)

# Record-Route
modparam("rr", "enable_full_lr", 1)
modparam("rr", "append_fromtag", 1)

# NAT helper
modparam("nathelper", "natping_interval", 30)
modparam("nathelper", "ping_nated_only", 1)
modparam("nathelper", "sipping_bflag", "FLB_NATSIPPING")

# RTPEngine (Karl)
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
modparam("rtpengine", "rtpengine_tout_ms", 2000)
modparam("rtpengine", "rtpengine_retr", 3)

####### Routing Logic ########

# Main request routing
request_route {
    # Per-request initial checks
    route(REQINIT);

    # NAT detection
    route(NATDETECT);

    # CANCEL processing
    if (is_method("CANCEL")) {
        if (t_check_trans()) {
            route(RTPENGINE_DELETE);
            t_relay();
        }
        exit;
    }

    # Handle requests within dialogs
    if (has_totag()) {
        route(WITHINDLG);
        exit;
    }

    # Record routing for dialog-forming requests
    if (is_method("INVITE|SUBSCRIBE")) {
        record_route();
    }

    # Handle INVITE
    if (is_method("INVITE")) {
        route(RTPENGINE_OFFER);
        t_on_reply("MANAGE_REPLY");
        t_on_failure("CALL_FAILURE");
    }

    route(RELAY);
}

# Per-request initial checks
route[REQINIT] {
    if (!mf_process_maxfwd_header("10")) {
        sl_send_reply("483", "Too Many Hops");
        exit;
    }

    if (!sanity_check("1511", "7")) {
        xlog("L_WARN", "Malformed SIP message from $si:$sp\n");
        exit;
    }
}

# NAT detection
route[NATDETECT] {
    force_rport();
    if (nat_uac_test("19")) {
        if (is_method("REGISTER")) {
            fix_nated_register();
        } else {
            if (is_first_hop()) {
                set_contact_alias();
            }
        }
        setflag(FLT_NATS);
    }
}

# Handle in-dialog requests
route[WITHINDLG] {
    if (!loose_route()) {
        if (is_method("ACK")) {
            if (!t_check_trans()) {
                exit;
            }
        }
        sl_send_reply("404", "Not Found");
        exit;
    }

    if (is_method("ACK")) {
        route(NATMANAGE);
    } else if (is_method("BYE")) {
        route(RTPENGINE_DELETE);
    } else if (is_method("INVITE")) {
        # Re-INVITE
        route(RTPENGINE_OFFER);
        t_on_reply("MANAGE_REPLY");
    }

    route(RELAY);
}

# RTPEngine offer handling
route[RTPENGINE_OFFER] {
    if (!has_body("application/sdp")) {
        return;
    }

    $var(rtpflags) = "RTP/AVP replace-origin replace-session-connection";

    if (isflagset(FLT_NATS)) {
        $var(rtpflags) = $var(rtpflags) + " SIP-source-address";
    }

    rtpengine_manage("$var(rtpflags)");
}

# RTPEngine delete
route[RTPENGINE_DELETE] {
    rtpengine_delete();
}

# NAT management
route[NATMANAGE] {
    if (is_request()) {
        if (has_totag()) {
            if (check_route_param("nat=yes")) {
                setbflag(FLB_NATB);
            }
        }
    }

    if (isflagset(FLT_NATS) || isbflagset(FLB_NATB)) {
        if (is_request()) {
            if (!has_totag()) {
                if (t_is_branch_route()) {
                    add_rr_param(";nat=yes");
                }
            }
        }
    }
}

# Relay route
route[RELAY] {
    if (!t_relay()) {
        sl_reply_error();
    }
    exit;
}

# Reply route for managing SDP in responses
onreply_route[MANAGE_REPLY] {
    if (status =~ "[12][0-9][0-9]") {
        route(NATMANAGE);
    }

    if (has_body("application/sdp")) {
        $var(rtpflags) = "RTP/AVP replace-origin replace-session-connection";

        if (isbflagset(FLB_NATB)) {
            $var(rtpflags) = $var(rtpflags) + " SIP-source-address";
        }

        rtpengine_manage("$var(rtpflags)");
    }
}

# Failure route
failure_route[CALL_FAILURE] {
    if (t_was_cancelled()) {
        route(RTPENGINE_DELETE);
        exit;
    }

    # Handle other failure cases as needed
}
```

### Flag Reference

Common rtpengine_manage flags:

| Flag | Description |
|------|-------------|
| `RTP/AVP` | Standard RTP profile |
| `RTP/SAVP` | SRTP profile |
| `RTP/AVPF` | RTP with feedback |
| `RTP/SAVPF` | SRTP with feedback |
| `replace-origin` | Replace SDP origin |
| `replace-session-connection` | Replace session connection |
| `SIP-source-address` | Use SIP source for media |
| `symmetric` | Force symmetric RTP |
| `asymmetric` | Allow asymmetric RTP |
| `ICE=remove` | Remove ICE attributes |
| `ICE=force` | Force ICE |
| `DTLS=off` | Disable DTLS |
| `DTLS=passive` | DTLS passive mode |
| `SDES-off` | Disable SDES |
| `SDES-on` | Enable SDES |
| `record-call` | Enable call recording |
| `media-timeout=N` | Media timeout in seconds |

---

## High Availability

### Multiple Karl Instances

Configure failover:

```kamailio
# Primary and backup servers
modparam("rtpengine", "rtpengine_sock",
    "udp:karl1.example.com:22222 udp:karl2.example.com:22222")
```

### Weighted Load Balancing

```kamailio
# 70/30 distribution
modparam("rtpengine", "rtpengine_sock",
    "7 == udp:karl1.example.com:22222 3 == udp:karl2.example.com:22222")
```

### Multiple Sets

```kamailio
# Define sets for different regions
modparam("rtpengine", "rtpengine_sock", "1 == udp:karl-us.example.com:22222")
modparam("rtpengine", "rtpengine_sock", "2 == udp:karl-eu.example.com:22222")

# Use in routing
request_route {
    if ($rd =~ "\.us\.") {
        set_rtpengine_set("1");
    } else {
        set_rtpengine_set("2");
    }
    rtpengine_manage("...");
}
```

### Database Configuration

Store Karl servers in database:

```kamailio
modparam("rtpengine", "db_url", "mysql://kamailio:password@localhost/kamailio")
modparam("rtpengine", "table_name", "rtpengine")
```

---

## Troubleshooting

### Connection Issues

```bash
# Test Karl connectivity
echo -n "d7:command4:pinge" | nc -u 127.0.0.1 22222

# Check Kamailio logs
tail -f /var/log/kamailio.log | grep rtpengine

# Enable debug
kamctl debug 4
```

### No Audio

1. Verify SDP modification:
```kamailio
request_route {
    xlog("L_INFO", "SDP before: $rb\n");
    rtpengine_manage("...");
    xlog("L_INFO", "SDP after: $rb\n");
}
```

2. Check firewall for UDP ports

3. Verify Karl session creation:
```bash
curl http://127.0.0.1:8080/api/v1/sessions
```

### One-Way Audio

```kamailio
# Force symmetric RTP
rtpengine_manage("RTP/AVP replace-origin replace-session-connection symmetric");
```

### Monitor Sessions

```bash
# Karl API
curl http://127.0.0.1:8080/api/v1/sessions

# Kamailio RPC
kamcmd rtpengine.show_all
```

---

## Next Steps

- [Set Up Call Recording](./setting-up-recording.md)
- [Configure Monitoring](./monitoring-prometheus.md)
- [WebRTC to SIP Bridging](./webrtc-sip-bridging.md)
