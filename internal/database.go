package internal

import (
	"database/sql"
	"log"

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
		log.Fatalf("❌ Failed to connect to MySQL: %v", err)
		return nil, err
	}

	// Test the connection
	if err = db.Ping(); err != nil {
		log.Fatalf("❌ MySQL connection test failed: %v", err)
		return nil, err
	}

	log.Println("✅ Connected to MySQL successfully")
	return &RTPDatabase{db: db}, nil
}

// InsertRTPStats logs RTP session statistics into the MySQL database
func (r *RTPDatabase) InsertRTPStats(callID string, ssrc uint32, codec string, packetLoss int, jitter float64) error {
	query := `
        INSERT INTO rtp_sessions (call_id, ssrc, codec, packet_loss, jitter, start_time) 
        VALUES (?, ?, ?, ?, ?, NOW())
    `
	_, err := r.db.Exec(query, callID, ssrc, codec, packetLoss, jitter)
	if err != nil {
		log.Printf("❌ Failed to insert RTP stats: %v", err)
		return err
	}

	log.Printf("✅ RTP stats logged: Call ID=%s, SSRC=%d, Codec=%s", callID, ssrc, codec)
	return nil
}

// GetActiveSessions retrieves active RTP sessions
func (r *RTPDatabase) GetActiveSessions() ([]string, error) {
	query := `SELECT call_id FROM rtp_sessions WHERE end_time IS NULL`
	rows, err := r.db.Query(query)
	if err != nil {
		log.Printf("❌ Failed to fetch active RTP sessions: %v", err)
		return nil, err
	}
	defer rows.Close()

	var sessions []string
	for rows.Next() {
		var callID string
		if err := rows.Scan(&callID); err != nil {
			log.Printf("❌ Failed to scan RTP session: %v", err)
			continue
		}
		sessions = append(sessions, callID)
	}

	log.Printf("📡 Active RTP Sessions: %d", len(sessions))
	return sessions, nil
}

// Close closes the MySQL database connection
func (r *RTPDatabase) Close() {
	if err := r.db.Close(); err != nil {
		log.Printf("❌ Failed to close MySQL connection: %v", err)
	} else {
		log.Println("✅ MySQL connection closed")
	}
}
