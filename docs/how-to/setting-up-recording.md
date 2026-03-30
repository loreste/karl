# How to Set Up Call Recording

This guide covers configuring and managing call recording in Karl Media Server.

## Table of Contents

- [Overview](#overview)
- [Enable Recording](#enable-recording)
- [Recording Modes](#recording-modes)
- [Storage Configuration](#storage-configuration)
- [Trigger Recording](#trigger-recording)
- [Manage Recordings](#manage-recordings)
- [Production Considerations](#production-considerations)
- [Troubleshooting](#troubleshooting)

---

## Overview

Karl supports professional-grade call recording with:

- Multiple recording modes (mixed, stereo, separate)
- WAV output format (16-bit PCM)
- Automatic codec transcoding
- Retention policy management
- REST API for recording control

---

## Enable Recording

### Configuration File

```json
{
  "recording": {
    "enabled": true,
    "base_path": "/var/lib/karl/recordings",
    "format": "wav",
    "mode": "stereo",
    "sample_rate": 8000,
    "bits_per_sample": 16,
    "max_file_size": 104857600,
    "retention_days": 30
  }
}
```

### Environment Variables

```bash
export KARL_RECORDING_ENABLED=true
export KARL_RECORDING_PATH=/var/lib/karl/recordings
```

### Verify Configuration

```bash
# Check recording is enabled
curl http://localhost:8080/api/v1/stats | jq .recording_enabled
```

---

## Recording Modes

### Mixed Mode

Both parties combined into a single mono file.

```json
{
  "recording": {
    "mode": "mixed"
  }
}
```

**Output**: Single mono WAV file
**Use case**: Simple archival, compliance

### Stereo Mode

Caller on left channel, callee on right channel.

```json
{
  "recording": {
    "mode": "stereo"
  }
}
```

**Output**: Single stereo WAV file
**Use case**: Quality analysis, speaker identification

### Separate Mode

Each party in a separate file.

```json
{
  "recording": {
    "mode": "separate"
  }
}
```

**Output**: Two mono WAV files (one per party)
**Use case**: Post-processing, transcription services

---

## Storage Configuration

### Directory Structure

Recordings are stored with this structure:

```
/var/lib/karl/recordings/
├── 2024-01-15/
│   ├── call-abc123_1705312200_stereo.wav
│   ├── call-def456_1705312500_stereo.wav
│   └── call-ghi789_1705313000_stereo.wav
└── 2024-01-16/
    └── ...
```

### File Naming

Format: `{call_id}_{unix_timestamp}_{mode}.wav`

Example: `call-abc123_1705312200_stereo.wav`

### Storage Requirements

| Mode | Bitrate | Per Minute | Per Hour |
|------|---------|------------|----------|
| Mixed (mono) | 128 kbps | ~1 MB | ~60 MB |
| Stereo | 256 kbps | ~2 MB | ~120 MB |
| Separate | 256 kbps | ~2 MB | ~120 MB |

### Retention Policy

Configure automatic cleanup:

```json
{
  "recording": {
    "retention_days": 30
  }
}
```

Recordings older than `retention_days` are automatically deleted.

### Network Storage

For high-availability deployments, mount network storage:

```bash
# NFS mount
mount -t nfs storage.example.com:/recordings /var/lib/karl/recordings

# Or in /etc/fstab
storage.example.com:/recordings /var/lib/karl/recordings nfs defaults 0 0
```

For Kubernetes, use a PersistentVolumeClaim:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: karl-recordings
spec:
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 100Gi
  storageClassName: nfs
```

---

## Trigger Recording

### Method 1: NG Protocol Flag

Add the `record-call` flag when creating a session.

**OpenSIPS**:
```opensips
rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
```

**Kamailio**:
```kamailio
rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
```

### Method 2: REST API

Start recording for an active session:

```bash
# Start recording
curl -X POST http://localhost:8080/api/v1/recording/start \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "abc123",
    "mode": "stereo"
  }'
```

Stop recording:

```bash
# Stop recording
curl -X POST http://localhost:8080/api/v1/recording/stop \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "abc123"
  }'
```

### Method 3: Record All Calls

Configure default recording in your SIP proxy:

**OpenSIPS** (record all calls):
```opensips
route {
    if (is_method("INVITE")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
    }
}
```

### Method 4: Conditional Recording

Record based on criteria:

```opensips
route {
    if (is_method("INVITE")) {
        $var(flags) = "RTP/AVP replace-origin replace-session-connection";

        # Record calls from specific users
        if ($fU =~ "^sales_") {
            $var(flags) = $var(flags) + " record-call";
        }

        # Record calls to specific numbers
        if ($rU =~ "^1800") {
            $var(flags) = $var(flags) + " record-call";
        }

        rtpengine_manage("$var(flags)");
    }
}
```

---

## Manage Recordings

### List Recordings

```bash
# Via API
curl http://localhost:8080/api/v1/recordings

# Response
{
  "recordings": [
    {
      "id": "rec-001",
      "session_id": "abc123",
      "call_id": "call-abc123",
      "file_path": "/var/lib/karl/recordings/2024-01-15/call-abc123_1705312200_stereo.wav",
      "mode": "stereo",
      "duration_seconds": 125,
      "file_size": 3932160,
      "created_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

### Get Recording Details

```bash
curl http://localhost:8080/api/v1/recordings/rec-001
```

### Download Recording

```bash
# Get file path from API, then download
curl http://localhost:8080/api/v1/recordings/rec-001/download -o recording.wav
```

### Delete Recording

```bash
curl -X DELETE http://localhost:8080/api/v1/recordings/rec-001
```

### List Recordings by Date

```bash
# List recordings from specific date
ls -la /var/lib/karl/recordings/2024-01-15/

# Find recordings for a specific call
find /var/lib/karl/recordings -name "*abc123*"
```

---

## Production Considerations

### Storage Planning

Calculate storage needs:

```
Daily storage = calls_per_day × avg_duration_minutes × MB_per_minute

Example:
- 1000 calls/day
- 5 minute average duration
- 2 MB/minute (stereo)
= 1000 × 5 × 2 = 10 GB/day
= 300 GB/month
```

### High Availability

1. **Shared storage**: Use NFS, EFS, or similar for multi-instance deployments
2. **Backup**: Regular backup of recording directory
3. **Monitoring**: Alert on disk space usage

### Compliance

For regulated industries:

1. **Encryption at rest**: Use encrypted filesystem
2. **Access control**: Restrict recording directory permissions
3. **Audit logging**: Track recording access
4. **Retention**: Configure appropriate retention period

### Performance

Recording impact on system:

| Concurrent Recordings | Additional CPU | Additional Memory |
|----------------------|----------------|-------------------|
| 100 | ~5% | ~50 MB |
| 500 | ~15% | ~200 MB |
| 1000 | ~25% | ~400 MB |

---

## Troubleshooting

### Recordings Not Created

1. **Check recording is enabled**:
```bash
grep -A 10 '"recording"' /etc/karl/config.json
```

2. **Verify directory permissions**:
```bash
ls -la /var/lib/karl/recordings
# Should be writable by karl user
```

3. **Check disk space**:
```bash
df -h /var/lib/karl/recordings
```

4. **Verify record-call flag is sent**:
```bash
# Enable debug logging
KARL_LOG_LEVEL=debug ./karl

# Look for recording messages
grep -i recording /var/log/karl.log
```

### Empty Recording Files

1. **Check session has media**:
```bash
curl http://localhost:8080/api/v1/sessions/abc123
# Verify packets_received > 0
```

2. **Verify codec support**:
Karl supports G.711 (PCMU/PCMA) and Opus. Other codecs may not record properly.

3. **Check for errors**:
```bash
grep -i "recording\|codec" /var/log/karl.log
```

### Poor Recording Quality

1. **Sample rate mismatch**: Ensure sample_rate matches your codec
2. **Network issues**: High packet loss affects recording quality
3. **Jitter**: Enable jitter buffer for better quality

### Disk Space Issues

1. **Enable retention policy**:
```json
{
  "recording": {
    "retention_days": 7
  }
}
```

2. **Manual cleanup**:
```bash
# Delete recordings older than 7 days
find /var/lib/karl/recordings -name "*.wav" -mtime +7 -delete
```

3. **Monitor disk usage**:
```bash
# Add to cron
0 * * * * /usr/bin/df -h /var/lib/karl/recordings | mail -s "Karl disk usage" admin@example.com
```

---

## Next Steps

- [Monitor with Prometheus](./monitoring-prometheus.md)
- [Integrate with OpenSIPS](./integrating-opensips.md)
- [Integrate with Kamailio](./integrating-kamailio.md)
