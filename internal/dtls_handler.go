package internal

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pion/dtls/v2"
)

var (
	ErrNilConnection         = errors.New("nil connection")
	ErrHandshakeIncomplete   = errors.New("DTLS handshake not complete")
	ErrInvalidKeyingMaterial = errors.New("invalid keying material")
)

// DTLSError represents a custom error type for DTLS operations
type DTLSError struct {
	Op  string
	Err error
}

func (e *DTLSError) Error() string {
	return fmt.Sprintf("dtls %s: %v", e.Op, e.Err)
}

func (e *DTLSError) Unwrap() error {
	return e.Err
}

// DTLSMetrics holds timing and performance metrics for the DTLS session
type DTLSMetrics struct {
	HandshakeStartTime   time.Time
	HandshakeEndTime     time.Time
	CipherSuiteName      string
	KeyExtractionSuccess bool
}

// DTLSConfig holds configuration options for DTLS session
type DTLSConfig struct {
	CertFile           string
	KeyFile            string
	Address            string
	HandshakeTimeout   time.Duration
	InsecureSkipVerify bool
	LogKeys            bool
	MTU                int // Maximum Transmission Unit
	RetransmitInterval time.Duration
}

// DefaultDTLSConfig returns a DTLSConfig with sensible defaults
func DefaultDTLSConfig() DTLSConfig {
	return DTLSConfig{
		HandshakeTimeout:   30 * time.Second,
		InsecureSkipVerify: true, // Required for self-signed WebRTC certs
		LogKeys:            false,
		MTU:                1200, // Default WebRTC MTU
		RetransmitInterval: time.Second,
	}
}

// DTLSSession holds DTLS connection and extracted SRTP keys
type DTLSSession struct {
	Conn     *dtls.Conn
	SRTPKey  []byte
	SRTPSalt []byte
	metrics  DTLSMetrics
	mu       sync.Mutex // Protects concurrent access
}

// Metrics returns the current session metrics
func (d *DTLSSession) Metrics() DTLSMetrics {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.metrics
}

// Close properly cleans up the DTLS session and zeros sensitive material
func (d *DTLSSession) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var errs []error

	if d.Conn != nil {
		if err := d.Conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("conn close: %w", err))
		}
		d.Conn = nil
	}

	// Zero sensitive material
	if d.SRTPKey != nil {
		for i := range d.SRTPKey {
			d.SRTPKey[i] = 0
		}
		d.SRTPKey = nil
	}
	if d.SRTPSalt != nil {
		for i := range d.SRTPSalt {
			d.SRTPSalt[i] = 0
		}
		d.SRTPSalt = nil
	}

	if len(errs) > 0 {
		return fmt.Errorf("multiple close errors: %v", errs)
	}
	return nil
}

// StartDTLSSession initializes a DTLS-SRTP session with default configuration
func StartDTLSSession(ctx context.Context, certFile, keyFile, addr string) (*DTLSSession, error) {
	config := DefaultDTLSConfig()
	config.CertFile = certFile
	config.KeyFile = keyFile
	config.Address = addr
	return StartDTLSSessionWithConfig(ctx, config)
}

// StartDTLSSessionWithConfig initializes a DTLS-SRTP session with custom configuration
func StartDTLSSessionWithConfig(ctx context.Context, config DTLSConfig) (*DTLSSession, error) {
	// Input validation
	if config.CertFile == "" || config.KeyFile == "" {
		return nil, &DTLSError{Op: "validate", Err: errors.New("certificate and key files required")}
	}
	if config.Address == "" {
		return nil, &DTLSError{Op: "validate", Err: errors.New("address required")}
	}

	log.Println("🔒 Starting DTLS-SRTP handshake...")

	// Load DTLS certificate
	cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		log.Printf("❌ Failed to load DTLS certificate: %v", err)
		return nil, &DTLSError{Op: "certificate_load", Err: err}
	}

	// Configure DTLS
	dtlsConfig := &dtls.Config{
		Certificates:         []tls.Certificate{cert},
		ExtendedMasterSecret: dtls.RequireExtendedMasterSecret,
		// SRTP Protection Profiles
		SRTPProtectionProfiles: []dtls.SRTPProtectionProfile{
			dtls.SRTP_AES128_CM_HMAC_SHA1_80,
			dtls.SRTP_AES128_CM_HMAC_SHA1_32,
		},
		// Connection settings
		MTU:                config.MTU,
		InsecureSkipVerify: config.InsecureSkipVerify,
		FlightInterval:     config.RetransmitInterval,
	}

	// Resolve UDP address
	udpAddr, err := net.ResolveUDPAddr("udp", config.Address)
	if err != nil {
		log.Printf("❌ Failed to resolve UDP address: %v", err)
		return nil, &DTLSError{Op: "address_resolve", Err: err}
	}

	// Create UDP connection
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, &DTLSError{Op: "udp_listen", Err: err}
	}

	// Create session with metrics
	session := &DTLSSession{
		metrics: DTLSMetrics{
			HandshakeStartTime: time.Now(),
		},
	}

	// Create cancelable context for handshake
	ctxWithTimeout, cancel := context.WithTimeout(ctx, config.HandshakeTimeout)
	defer cancel()

	// Start DTLS handshake with timeout
	errChan := make(chan error, 1)
	connChan := make(chan *dtls.Conn, 1)

	go func() {
		conn, err := dtls.Server(udpConn, dtlsConfig)
		if err != nil {
			errChan <- err
			return
		}
		connChan <- conn
	}()

	// Wait for handshake completion or timeout
	select {
	case <-ctxWithTimeout.Done():
		udpConn.Close()
		if ctxWithTimeout.Err() == context.DeadlineExceeded {
			return nil, &DTLSError{Op: "handshake", Err: fmt.Errorf("handshake timeout after %v", config.HandshakeTimeout)}
		}
		return nil, &DTLSError{Op: "context", Err: ctxWithTimeout.Err()}
	case err := <-errChan:
		udpConn.Close()
		return nil, &DTLSError{Op: "handshake", Err: err}
	case conn := <-connChan:
		session.Conn = conn
	}

	session.metrics.HandshakeEndTime = time.Now()

	// Extract SRTP keys
	srtpKey, srtpSalt, err := extractSRTPKeys(session.Conn, config.LogKeys)
	if err != nil {
		session.Close()
		return nil, err
	}

	session.mu.Lock()
	session.SRTPKey = srtpKey
	session.SRTPSalt = srtpSalt
	session.metrics.KeyExtractionSuccess = true
	session.metrics.CipherSuiteName = "DTLS-SRTP"
	session.mu.Unlock()

	log.Println("✅ DTLS-SRTP handshake successful")
	if config.LogKeys {
		log.Printf("🔑 SRTP Key: %x", srtpKey)
		log.Printf("🔑 SRTP Salt: %x", srtpSalt)
	}

	return session, nil
}

const (
	srtpKeyLength  = 16 // AES-128
	srtpSaltLength = 14 // 112 bits
	srtpMasterSize = srtpKeyLength + srtpSaltLength
)

// extractSRTPKeys extracts SRTP keys from DTLS connection state
func extractSRTPKeys(conn *dtls.Conn, logKeys bool) ([]byte, []byte, error) {
	if conn == nil {
		return nil, nil, &DTLSError{Op: "extract_keys", Err: ErrNilConnection}
	}

	if logKeys {
		log.Printf("🔑 Extracting DTLS-SRTP keys")
	}

	// Get connection state and export keying material
	state := conn.ConnectionState()
	masterKey, err := state.ExportKeyingMaterial(
		"EXTRACTOR-dtls_srtp",
		nil,
		srtpMasterSize,
	)
	if err != nil {
		return nil, nil, &DTLSError{Op: "extract_keys", Err: fmt.Errorf("failed to extract keying material: %w", err)}
	}

	if len(masterKey) < srtpMasterSize {
		return nil, nil, &DTLSError{Op: "extract_keys", Err: ErrInvalidKeyingMaterial}
	}

	// Split the keying material into key and salt
	srtpKey := masterKey[:srtpKeyLength]
	srtpSalt := masterKey[srtpKeyLength : srtpKeyLength+srtpSaltLength]

	return srtpKey, srtpSalt, nil
}
