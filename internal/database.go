package internal

import (
	"database/sql"
	"encoding/json"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// RTPDatabase represents the MySQL database connection
type RTPDatabase struct {
	db *sql.DB
}

// NewRTPDatabase initializes a connection to MySQL
func NewRTPDatabase(dsn string) (*RTPDatabase, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Printf("Failed to connect to MySQL: %v", err)
		return nil, err
	}

	// Test the connection
	if err = db.Ping(); err != nil {
		log.Printf("MySQL connection test failed: %v", err)
		return nil, err
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	log.Println("Connected to MySQL successfully")
	return &RTPDatabase{db: db}, nil
}

// InitSchema initializes the database schema
func (r *RTPDatabase) InitSchema() error {
	schemas := []string{
		// Sessions table
		`CREATE TABLE IF NOT EXISTS sessions (
			id VARCHAR(36) PRIMARY KEY,
			call_id VARCHAR(255) NOT NULL,
			from_tag VARCHAR(255) NOT NULL,
			to_tag VARCHAR(255),
			via_branch VARCHAR(255),
			state VARCHAR(20) NOT NULL DEFAULT 'new',
			caller_ip VARCHAR(45),
			caller_port INT,
			callee_ip VARCHAR(45),
			callee_port INT,
			start_time DATETIME NOT NULL,
			connect_time DATETIME,
			end_time DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			INDEX idx_call_id (call_id),
			INDEX idx_state (state),
			INDEX idx_start_time (start_time)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		// CDR (Call Detail Records) table
		`CREATE TABLE IF NOT EXISTS cdr (
			id VARCHAR(36) PRIMARY KEY,
			session_id VARCHAR(36),
			call_id VARCHAR(255) NOT NULL,
			from_tag VARCHAR(255),
			to_tag VARCHAR(255),
			caller_id VARCHAR(255),
			callee_id VARCHAR(255),
			caller_ip VARCHAR(45),
			callee_ip VARCHAR(45),
			start_time DATETIME NOT NULL,
			connect_time DATETIME,
			end_time DATETIME,
			duration INT DEFAULT 0,
			billable_duration INT DEFAULT 0,
			termination_cause VARCHAR(100),
			packets_sent BIGINT DEFAULT 0,
			packets_recv BIGINT DEFAULT 0,
			bytes_sent BIGINT DEFAULT 0,
			bytes_recv BIGINT DEFAULT 0,
			packets_lost INT DEFAULT 0,
			avg_jitter DECIMAL(10,6) DEFAULT 0,
			max_jitter DECIMAL(10,6) DEFAULT 0,
			packet_loss_rate DECIMAL(5,4) DEFAULT 0,
			rtt DECIMAL(10,6) DEFAULT 0,
			mos DECIMAL(3,2) DEFAULT 0,
			recording_path VARCHAR(500),
			metadata JSON,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_session_id (session_id),
			INDEX idx_call_id (call_id),
			INDEX idx_start_time (start_time),
			INDEX idx_end_time (end_time)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		// Recordings table
		`CREATE TABLE IF NOT EXISTS recordings (
			id VARCHAR(36) PRIMARY KEY,
			session_id VARCHAR(36),
			call_id VARCHAR(255),
			file_path VARCHAR(500) NOT NULL,
			format VARCHAR(20) NOT NULL DEFAULT 'wav',
			mode VARCHAR(20) NOT NULL DEFAULT 'mixed',
			status VARCHAR(20) NOT NULL DEFAULT 'pending',
			start_time DATETIME NOT NULL,
			end_time DATETIME,
			duration INT DEFAULT 0,
			file_size BIGINT DEFAULT 0,
			metadata JSON,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_session_id (session_id),
			INDEX idx_call_id (call_id),
			INDEX idx_status (status),
			INDEX idx_start_time (start_time)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		// API Keys table
		`CREATE TABLE IF NOT EXISTS api_keys (
			id VARCHAR(36) PRIMARY KEY,
			key_hash VARCHAR(64) UNIQUE NOT NULL,
			name VARCHAR(100) NOT NULL,
			permissions JSON NOT NULL,
			rate_limit INT DEFAULT 60,
			enabled BOOLEAN DEFAULT TRUE,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_used DATETIME,
			INDEX idx_key_hash (key_hash),
			INDEX idx_enabled (enabled)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		// RTP sessions table (existing, updated)
		`CREATE TABLE IF NOT EXISTS rtp_sessions (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			call_id VARCHAR(255) NOT NULL,
			ssrc INT UNSIGNED NOT NULL,
			codec VARCHAR(50),
			packet_loss INT DEFAULT 0,
			jitter DECIMAL(10,6) DEFAULT 0,
			start_time DATETIME NOT NULL,
			end_time DATETIME,
			packets_sent BIGINT DEFAULT 0,
			packets_recv BIGINT DEFAULT 0,
			bytes_sent BIGINT DEFAULT 0,
			bytes_recv BIGINT DEFAULT 0,
			INDEX idx_call_id (call_id),
			INDEX idx_ssrc (ssrc),
			INDEX idx_start_time (start_time)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}

	for _, schema := range schemas {
		if _, err := r.db.Exec(schema); err != nil {
			log.Printf("Schema creation error: %v", err)
			// Continue with other schemas
		}
	}

	log.Println("Database schema initialized")
	return nil
}

// InsertRTPStats logs RTP session statistics into the MySQL database
func (r *RTPDatabase) InsertRTPStats(callID string, ssrc uint32, codec string, packetLoss int, jitter float64) error {
	query := `
        INSERT INTO rtp_sessions (call_id, ssrc, codec, packet_loss, jitter, start_time)
        VALUES (?, ?, ?, ?, ?, NOW())
    `
	_, err := r.db.Exec(query, callID, ssrc, codec, packetLoss, jitter)
	if err != nil {
		log.Printf("Failed to insert RTP stats: %v", err)
		return err
	}

	log.Printf("RTP stats logged: Call ID=%s, SSRC=%d, Codec=%s", callID, ssrc, codec)
	return nil
}

// GetActiveSessions retrieves active RTP sessions
func (r *RTPDatabase) GetActiveSessions() ([]string, error) {
	query := `SELECT call_id FROM rtp_sessions WHERE end_time IS NULL`
	rows, err := r.db.Query(query)
	if err != nil {
		log.Printf("Failed to fetch active RTP sessions: %v", err)
		return nil, err
	}
	defer rows.Close()

	var sessions []string
	for rows.Next() {
		var callID string
		if err := rows.Scan(&callID); err != nil {
			log.Printf("Failed to scan RTP session: %v", err)
			continue
		}
		sessions = append(sessions, callID)
	}

	log.Printf("Active RTP Sessions: %d", len(sessions))
	return sessions, nil
}

// Session database operations

// SessionRecord represents a session in the database
type SessionRecord struct {
	ID          string
	CallID      string
	FromTag     string
	ToTag       string
	ViaBranch   string
	State       string
	CallerIP    string
	CallerPort  int
	CalleeIP    string
	CalleePort  int
	StartTime   time.Time
	ConnectTime *time.Time
	EndTime     *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// InsertSession inserts a new session
func (r *RTPDatabase) InsertSession(session *SessionRecord) error {
	query := `
		INSERT INTO sessions (id, call_id, from_tag, to_tag, via_branch, state,
			caller_ip, caller_port, callee_ip, callee_port, start_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query,
		session.ID, session.CallID, session.FromTag, session.ToTag, session.ViaBranch,
		session.State, session.CallerIP, session.CallerPort, session.CalleeIP,
		session.CalleePort, session.StartTime)
	return err
}

// UpdateSession updates a session
func (r *RTPDatabase) UpdateSession(session *SessionRecord) error {
	query := `
		UPDATE sessions SET
			to_tag = ?, state = ?, callee_ip = ?, callee_port = ?,
			connect_time = ?, end_time = ?
		WHERE id = ?
	`
	_, err := r.db.Exec(query,
		session.ToTag, session.State, session.CalleeIP, session.CalleePort,
		session.ConnectTime, session.EndTime, session.ID)
	return err
}

// GetSession retrieves a session by ID
func (r *RTPDatabase) GetSession(id string) (*SessionRecord, error) {
	query := `
		SELECT id, call_id, from_tag, to_tag, via_branch, state,
			caller_ip, caller_port, callee_ip, callee_port,
			start_time, connect_time, end_time, created_at, updated_at
		FROM sessions WHERE id = ?
	`
	session := &SessionRecord{}
	err := r.db.QueryRow(query, id).Scan(
		&session.ID, &session.CallID, &session.FromTag, &session.ToTag,
		&session.ViaBranch, &session.State, &session.CallerIP, &session.CallerPort,
		&session.CalleeIP, &session.CalleePort, &session.StartTime,
		&session.ConnectTime, &session.EndTime, &session.CreatedAt, &session.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return session, nil
}

// DeleteSession deletes a session
func (r *RTPDatabase) DeleteSession(id string) error {
	_, err := r.db.Exec("DELETE FROM sessions WHERE id = ?", id)
	return err
}

// CDR database operations

// CDRRecord represents a Call Detail Record
type CDRRecord struct {
	ID               string
	SessionID        string
	CallID           string
	FromTag          string
	ToTag            string
	CallerID         string
	CalleeID         string
	CallerIP         string
	CalleeIP         string
	StartTime        time.Time
	ConnectTime      *time.Time
	EndTime          *time.Time
	Duration         int
	BillableDuration int
	TerminationCause string
	PacketsSent      uint64
	PacketsRecv      uint64
	BytesSent        uint64
	BytesRecv        uint64
	PacketsLost      int
	AvgJitter        float64
	MaxJitter        float64
	PacketLossRate   float64
	RTT              float64
	MOS              float64
	RecordingPath    string
	Metadata         map[string]string
}

// InsertCDR inserts a new CDR
func (r *RTPDatabase) InsertCDR(cdr *CDRRecord) error {
	metadataJSON, _ := json.Marshal(cdr.Metadata)

	query := `
		INSERT INTO cdr (id, session_id, call_id, from_tag, to_tag,
			caller_id, callee_id, caller_ip, callee_ip,
			start_time, connect_time, end_time, duration, billable_duration,
			termination_cause, packets_sent, packets_recv, bytes_sent, bytes_recv,
			packets_lost, avg_jitter, max_jitter, packet_loss_rate, rtt, mos,
			recording_path, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query,
		cdr.ID, cdr.SessionID, cdr.CallID, cdr.FromTag, cdr.ToTag,
		cdr.CallerID, cdr.CalleeID, cdr.CallerIP, cdr.CalleeIP,
		cdr.StartTime, cdr.ConnectTime, cdr.EndTime, cdr.Duration, cdr.BillableDuration,
		cdr.TerminationCause, cdr.PacketsSent, cdr.PacketsRecv, cdr.BytesSent, cdr.BytesRecv,
		cdr.PacketsLost, cdr.AvgJitter, cdr.MaxJitter, cdr.PacketLossRate, cdr.RTT, cdr.MOS,
		cdr.RecordingPath, string(metadataJSON))
	return err
}

// GetCDR retrieves a CDR by ID
func (r *RTPDatabase) GetCDR(id string) (*CDRRecord, error) {
	query := `
		SELECT id, session_id, call_id, from_tag, to_tag,
			caller_id, callee_id, caller_ip, callee_ip,
			start_time, connect_time, end_time, duration, billable_duration,
			termination_cause, packets_sent, packets_recv, bytes_sent, bytes_recv,
			packets_lost, avg_jitter, max_jitter, packet_loss_rate, rtt, mos,
			recording_path, metadata
		FROM cdr WHERE id = ?
	`
	cdr := &CDRRecord{}
	var metadataJSON string
	err := r.db.QueryRow(query, id).Scan(
		&cdr.ID, &cdr.SessionID, &cdr.CallID, &cdr.FromTag, &cdr.ToTag,
		&cdr.CallerID, &cdr.CalleeID, &cdr.CallerIP, &cdr.CalleeIP,
		&cdr.StartTime, &cdr.ConnectTime, &cdr.EndTime, &cdr.Duration, &cdr.BillableDuration,
		&cdr.TerminationCause, &cdr.PacketsSent, &cdr.PacketsRecv, &cdr.BytesSent, &cdr.BytesRecv,
		&cdr.PacketsLost, &cdr.AvgJitter, &cdr.MaxJitter, &cdr.PacketLossRate, &cdr.RTT, &cdr.MOS,
		&cdr.RecordingPath, &metadataJSON)
	if err != nil {
		return nil, err
	}
	// Ignore JSON unmarshal errors for metadata - it's optional
	_ = json.Unmarshal([]byte(metadataJSON), &cdr.Metadata)
	return cdr, nil
}

// ListCDRs lists CDRs with optional filters
func (r *RTPDatabase) ListCDRs(callID string, startFrom, startTo time.Time, limit int) ([]*CDRRecord, error) {
	query := `
		SELECT id, session_id, call_id, from_tag, to_tag,
			caller_id, callee_id, caller_ip, callee_ip,
			start_time, connect_time, end_time, duration, billable_duration,
			termination_cause, packets_sent, packets_recv, bytes_sent, bytes_recv,
			packets_lost, avg_jitter, max_jitter, packet_loss_rate, rtt, mos,
			recording_path
		FROM cdr WHERE 1=1
	`
	args := []interface{}{}

	if callID != "" {
		query += " AND call_id = ?"
		args = append(args, callID)
	}
	if !startFrom.IsZero() {
		query += " AND start_time >= ?"
		args = append(args, startFrom)
	}
	if !startTo.IsZero() {
		query += " AND start_time <= ?"
		args = append(args, startTo)
	}

	query += " ORDER BY start_time DESC"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cdrs []*CDRRecord
	for rows.Next() {
		cdr := &CDRRecord{}
		err := rows.Scan(
			&cdr.ID, &cdr.SessionID, &cdr.CallID, &cdr.FromTag, &cdr.ToTag,
			&cdr.CallerID, &cdr.CalleeID, &cdr.CallerIP, &cdr.CalleeIP,
			&cdr.StartTime, &cdr.ConnectTime, &cdr.EndTime, &cdr.Duration, &cdr.BillableDuration,
			&cdr.TerminationCause, &cdr.PacketsSent, &cdr.PacketsRecv, &cdr.BytesSent, &cdr.BytesRecv,
			&cdr.PacketsLost, &cdr.AvgJitter, &cdr.MaxJitter, &cdr.PacketLossRate, &cdr.RTT, &cdr.MOS,
			&cdr.RecordingPath)
		if err != nil {
			continue
		}
		cdrs = append(cdrs, cdr)
	}

	return cdrs, nil
}

// Recording database operations

// RecordingRecord represents a recording in the database
type RecordingRecord struct {
	ID        string
	SessionID string
	CallID    string
	FilePath  string
	Format    string
	Mode      string
	Status    string
	StartTime time.Time
	EndTime   *time.Time
	Duration  int
	FileSize  int64
	Metadata  map[string]string
	CreatedAt time.Time
}

// InsertRecording inserts a new recording
func (r *RTPDatabase) InsertRecording(rec *RecordingRecord) error {
	metadataJSON, _ := json.Marshal(rec.Metadata)

	query := `
		INSERT INTO recordings (id, session_id, call_id, file_path, format, mode,
			status, start_time, end_time, duration, file_size, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query,
		rec.ID, rec.SessionID, rec.CallID, rec.FilePath, rec.Format, rec.Mode,
		rec.Status, rec.StartTime, rec.EndTime, rec.Duration, rec.FileSize,
		string(metadataJSON))
	return err
}

// UpdateRecording updates a recording
func (r *RTPDatabase) UpdateRecording(rec *RecordingRecord) error {
	query := `
		UPDATE recordings SET
			status = ?, end_time = ?, duration = ?, file_size = ?
		WHERE id = ?
	`
	_, err := r.db.Exec(query, rec.Status, rec.EndTime, rec.Duration, rec.FileSize, rec.ID)
	return err
}

// GetRecording retrieves a recording by ID
func (r *RTPDatabase) GetRecording(id string) (*RecordingRecord, error) {
	query := `
		SELECT id, session_id, call_id, file_path, format, mode,
			status, start_time, end_time, duration, file_size, metadata, created_at
		FROM recordings WHERE id = ?
	`
	rec := &RecordingRecord{}
	var metadataJSON string
	err := r.db.QueryRow(query, id).Scan(
		&rec.ID, &rec.SessionID, &rec.CallID, &rec.FilePath, &rec.Format, &rec.Mode,
		&rec.Status, &rec.StartTime, &rec.EndTime, &rec.Duration, &rec.FileSize,
		&metadataJSON, &rec.CreatedAt)
	if err != nil {
		return nil, err
	}
	// Ignore JSON unmarshal errors for metadata - it's optional
	_ = json.Unmarshal([]byte(metadataJSON), &rec.Metadata)
	return rec, nil
}

// ListRecordings lists recordings with optional filters
func (r *RTPDatabase) ListRecordings(sessionID, status string, limit int) ([]*RecordingRecord, error) {
	query := `
		SELECT id, session_id, call_id, file_path, format, mode,
			status, start_time, end_time, duration, file_size, created_at
		FROM recordings WHERE 1=1
	`
	args := []interface{}{}

	if sessionID != "" {
		query += " AND session_id = ?"
		args = append(args, sessionID)
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}

	query += " ORDER BY start_time DESC"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recordings []*RecordingRecord
	for rows.Next() {
		rec := &RecordingRecord{}
		err := rows.Scan(
			&rec.ID, &rec.SessionID, &rec.CallID, &rec.FilePath, &rec.Format, &rec.Mode,
			&rec.Status, &rec.StartTime, &rec.EndTime, &rec.Duration, &rec.FileSize,
			&rec.CreatedAt)
		if err != nil {
			continue
		}
		recordings = append(recordings, rec)
	}

	return recordings, nil
}

// DeleteRecording deletes a recording
func (r *RTPDatabase) DeleteRecording(id string) error {
	_, err := r.db.Exec("DELETE FROM recordings WHERE id = ?", id)
	return err
}

// Statistics operations

// GetAggregateStats returns aggregate statistics
func (r *RTPDatabase) GetAggregateStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total sessions
	var totalSessions int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&totalSessions); err == nil {
		stats["total_sessions"] = totalSessions
	}

	// Active sessions
	var activeSessions int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE state = 'active'").Scan(&activeSessions); err == nil {
		stats["active_sessions"] = activeSessions
	}

	// Total CDRs
	var totalCDRs int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM cdr").Scan(&totalCDRs); err == nil {
		stats["total_cdrs"] = totalCDRs
	}

	// Average call duration
	var avgDuration float64
	if err := r.db.QueryRow("SELECT COALESCE(AVG(duration), 0) FROM cdr").Scan(&avgDuration); err == nil {
		stats["avg_duration"] = avgDuration
	}

	// Total recordings
	var totalRecordings int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM recordings").Scan(&totalRecordings); err == nil {
		stats["total_recordings"] = totalRecordings
	}

	// Total recording size
	var totalRecordingSize int64
	if err := r.db.QueryRow("SELECT COALESCE(SUM(file_size), 0) FROM recordings").Scan(&totalRecordingSize); err == nil {
		stats["total_recording_size"] = totalRecordingSize
	}

	return stats, nil
}

// Close closes the MySQL database connection
func (r *RTPDatabase) Close() {
	if err := r.db.Close(); err != nil {
		log.Printf("Failed to close MySQL connection: %v", err)
	} else {
		log.Println("MySQL connection closed")
	}
}

// GetDB returns the underlying database connection
func (r *RTPDatabase) GetDB() *sql.DB {
	return r.db
}
