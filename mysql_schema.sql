-- Create database
CREATE DATABASE IF NOT EXISTS karl_db;
USE karl_db;

-- 1️⃣ Users Table (For SIP/WebRTC User Registrations)
CREATE TABLE users (
    id INT AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 2️⃣ SIP Registrations Table (For Tracking SIP Accounts)
CREATE TABLE sip_registrations (
    id INT AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(255) NOT NULL,
    domain VARCHAR(255) NOT NULL,
    contact VARCHAR(255) NOT NULL,
    expires TIMESTAMP NOT NULL,
    registered_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

-- 3️⃣ RTP Sessions Table (For Active RTP Streams)
CREATE TABLE rtp_sessions (
    id INT AUTO_INCREMENT PRIMARY KEY,
    call_id VARCHAR(255) NOT NULL,
    ssrc BIGINT NOT NULL,
    local_ip VARCHAR(45) NOT NULL,
    remote_ip VARCHAR(45) NOT NULL,
    local_port INT NOT NULL,
    remote_port INT NOT NULL,
    started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_call_id (call_id),
    INDEX idx_remote_ip (remote_ip)
);

-- 4️⃣ Call Logs Table (For SIP/WebRTC Call History)
CREATE TABLE call_logs (
    id INT AUTO_INCREMENT PRIMARY KEY,
    call_id VARCHAR(255) NOT NULL,
    caller VARCHAR(255) NOT NULL,
    callee VARCHAR(255) NOT NULL,
    start_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    end_time TIMESTAMP NULL,
    status ENUM('in-progress', 'completed', 'failed') DEFAULT 'in-progress',
    duration INT DEFAULT 0,
    recording_url TEXT NULL,
    INDEX idx_caller (caller),
    INDEX idx_callee (callee),
    INDEX idx_call_status (status)
);

-- 5️⃣ WebRTC Statistics Table (For Monitoring WebRTC Calls)
CREATE TABLE webrtc_stats (
    id INT AUTO_INCREMENT PRIMARY KEY,
    session_id VARCHAR(255) NOT NULL,
    bytes_sent BIGINT DEFAULT 0,
    bytes_received BIGINT DEFAULT 0,
    packets_lost INT DEFAULT 0,
    jitter FLOAT DEFAULT 0,
    bandwidth_usage INT DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_session (session_id)
);

-- 6️⃣ RTP Transcoding Table (For Audio/Video Codec Conversions)
CREATE TABLE rtp_transcoding (
    id INT AUTO_INCREMENT PRIMARY KEY,
    call_id VARCHAR(255) NOT NULL,
    input_codec VARCHAR(50) NOT NULL,
    output_codec VARCHAR(50) NOT NULL,
    transcoding_time FLOAT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_transcoding_call (call_id)
);

-- 7️⃣ Redis Session Cleanup Trigger (Optional: Auto-delete Expired RTP Sessions)
DELIMITER //
CREATE TRIGGER delete_expired_sessions
AFTER INSERT ON rtp_sessions
FOR EACH ROW
BEGIN
    DELETE FROM rtp_sessions WHERE last_updated < NOW() - INTERVAL 1 HOUR;
END;
//
DELIMITER ;

-- 8️⃣ Index Optimizations for Faster Queries
CREATE INDEX idx_sip_user ON sip_registrations (username);
CREATE INDEX idx_rtp_remote_ip ON rtp_sessions (remote_ip);
CREATE INDEX idx_call_status ON call_logs (status);

-- 9️⃣ Sample Data for Testing
INSERT INTO users (username, password_hash, email)
VALUES ('alice', 'hashed_password_here', 'alice@example.com');

INSERT INTO call_logs (call_id, caller, callee, status, duration)
VALUES ('abc123', 'alice', 'bob', 'completed', 120);

INSERT INTO rtp_sessions (call_id, ssrc, local_ip, remote_ip, local_port, remote_port)
VALUES ('abc123', 987654321, '192.168.1.100', '203.0.113.45', 5004, 4000);

INSERT INTO webrtc_stats (session_id, bytes_sent, bytes_received, packets_lost, jitter, bandwidth_usage)
VALUES ('session123', 1048576, 2097152, 10, 5.2, 1500);
