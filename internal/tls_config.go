package internal

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// TLS configuration errors
var (
	ErrNoCertificate       = errors.New("no certificate provided")
	ErrCertificateExpired  = errors.New("certificate has expired")
	ErrTLSCertInvalid      = errors.New("certificate is invalid")
	ErrNoCAPool            = errors.New("no CA certificate pool")
	ErrInvalidTLSVersion   = errors.New("invalid TLS version")
)

// TLSVersion represents a TLS protocol version
type TLSVersion uint16

const (
	TLSVersionAuto TLSVersion = 0
	TLSVersion10   TLSVersion = tls.VersionTLS10
	TLSVersion11   TLSVersion = tls.VersionTLS11
	TLSVersion12   TLSVersion = tls.VersionTLS12
	TLSVersion13   TLSVersion = tls.VersionTLS13
)

// TLSConfigOptions holds TLS configuration options
type TLSConfigOptions struct {
	// Certificate settings
	CertFile string
	KeyFile  string
	CAFile   string

	// Certificate data (alternative to files)
	CertPEM []byte
	KeyPEM  []byte
	CAPEM   []byte

	// TLS versions
	MinVersion TLSVersion
	MaxVersion TLSVersion

	// Client authentication
	ClientAuth tls.ClientAuthType

	// Cipher suites
	CipherSuites []uint16

	// Curve preferences
	CurvePreferences []tls.CurveID

	// Server name for SNI
	ServerName string

	// Skip certificate verification (NOT recommended for production)
	InsecureSkipVerify bool

	// Session settings
	SessionTicketsDisabled bool

	// ALPN protocols
	NextProtos []string

	// Certificate reload interval (0 = no auto-reload)
	ReloadInterval time.Duration
}

// DefaultTLSConfigOptions returns secure default TLS options
func DefaultTLSConfigOptions() *TLSConfigOptions {
	return &TLSConfigOptions{
		MinVersion: TLSVersion12,
		MaxVersion: TLSVersion13,
		ClientAuth: tls.NoClientCert,
		CipherSuites: []uint16{
			// TLS 1.3 cipher suites are automatically included
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
		},
	}
}

// StrictTLSConfigOptions returns the most secure TLS options
func StrictTLSConfigOptions() *TLSConfigOptions {
	opts := DefaultTLSConfigOptions()
	opts.MinVersion = TLSVersion13
	opts.ClientAuth = tls.RequireAndVerifyClientCert
	opts.SessionTicketsDisabled = true
	return opts
}

// TLSConfigBuilder builds TLS configurations
type TLSConfigBuilder struct {
	options *TLSConfigOptions
	err     error
}

// NewTLSConfigBuilder creates a new TLS config builder
func NewTLSConfigBuilder(options *TLSConfigOptions) *TLSConfigBuilder {
	if options == nil {
		options = DefaultTLSConfigOptions()
	}
	return &TLSConfigBuilder{
		options: options,
	}
}

// Build creates the TLS configuration
func (b *TLSConfigBuilder) Build() (*tls.Config, error) {
	if b.err != nil {
		return nil, b.err
	}

	config := &tls.Config{
		MinVersion:             uint16(b.options.MinVersion),
		MaxVersion:             uint16(b.options.MaxVersion),
		ClientAuth:             b.options.ClientAuth,
		CipherSuites:           b.options.CipherSuites,
		CurvePreferences:       b.options.CurvePreferences,
		InsecureSkipVerify:     b.options.InsecureSkipVerify,
		SessionTicketsDisabled: b.options.SessionTicketsDisabled,
		NextProtos:             b.options.NextProtos,
		ServerName:             b.options.ServerName,
	}

	// Load certificate
	if err := b.loadCertificate(config); err != nil {
		return nil, err
	}

	// Load CA pool for client verification
	if err := b.loadCAPool(config); err != nil {
		return nil, err
	}

	return config, nil
}

// loadCertificate loads the server certificate
func (b *TLSConfigBuilder) loadCertificate(config *tls.Config) error {
	var cert tls.Certificate
	var err error

	if b.options.CertPEM != nil && b.options.KeyPEM != nil {
		cert, err = tls.X509KeyPair(b.options.CertPEM, b.options.KeyPEM)
	} else if b.options.CertFile != "" && b.options.KeyFile != "" {
		cert, err = tls.LoadX509KeyPair(b.options.CertFile, b.options.KeyFile)
	} else {
		// No certificate - might be client-only config
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to load certificate: %w", err)
	}

	config.Certificates = []tls.Certificate{cert}
	return nil
}

// loadCAPool loads the CA certificate pool
func (b *TLSConfigBuilder) loadCAPool(config *tls.Config) error {
	var caPEM []byte

	if b.options.CAPEM != nil {
		caPEM = b.options.CAPEM
	} else if b.options.CAFile != "" {
		var err error
		caPEM, err = os.ReadFile(b.options.CAFile)
		if err != nil {
			return fmt.Errorf("failed to read CA file: %w", err)
		}
	} else {
		// No custom CA pool
		return nil
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return fmt.Errorf("failed to parse CA certificates")
	}

	config.ClientCAs = pool
	config.RootCAs = pool

	return nil
}

// BuildClient creates a client-side TLS configuration
func (b *TLSConfigBuilder) BuildClient() (*tls.Config, error) {
	config, err := b.Build()
	if err != nil {
		return nil, err
	}

	// Client-specific settings
	config.ClientAuth = tls.NoClientCert

	return config, nil
}

// BuildServer creates a server-side TLS configuration
func (b *TLSConfigBuilder) BuildServer() (*tls.Config, error) {
	config, err := b.Build()
	if err != nil {
		return nil, err
	}

	// Verify we have certificates for server
	if len(config.Certificates) == 0 {
		return nil, ErrNoCertificate
	}

	return config, nil
}

// BuildMutualTLS creates a mutual TLS configuration
func (b *TLSConfigBuilder) BuildMutualTLS() (*tls.Config, error) {
	b.options.ClientAuth = tls.RequireAndVerifyClientCert

	config, err := b.Build()
	if err != nil {
		return nil, err
	}

	// Verify we have CA pool for client verification
	if config.ClientCAs == nil {
		return nil, ErrNoCAPool
	}

	return config, nil
}

// CertificateReloader handles automatic certificate reloading
type CertificateReloader struct {
	certFile string
	keyFile  string

	mu          sync.RWMutex
	certificate *tls.Certificate
	lastLoaded  time.Time
	lastError   error

	stopChan chan struct{}
	doneChan chan struct{}
}

// NewCertificateReloader creates a new certificate reloader
func NewCertificateReloader(certFile, keyFile string) (*CertificateReloader, error) {
	cr := &CertificateReloader{
		certFile: certFile,
		keyFile:  keyFile,
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	// Load initial certificate
	if err := cr.reload(); err != nil {
		return nil, err
	}

	return cr, nil
}

// reload reloads the certificate from disk
func (cr *CertificateReloader) reload() error {
	cert, err := tls.LoadX509KeyPair(cr.certFile, cr.keyFile)
	if err != nil {
		cr.mu.Lock()
		cr.lastError = err
		cr.mu.Unlock()
		return err
	}

	cr.mu.Lock()
	cr.certificate = &cert
	cr.lastLoaded = time.Now()
	cr.lastError = nil
	cr.mu.Unlock()

	return nil
}

// GetCertificate returns the current certificate
func (cr *CertificateReloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	if cr.certificate == nil {
		return nil, ErrNoCertificate
	}

	return cr.certificate, nil
}

// StartAutoReload starts automatic certificate reloading
func (cr *CertificateReloader) StartAutoReload(interval time.Duration) {
	go func() {
		defer close(cr.doneChan)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-cr.stopChan:
				return
			case <-ticker.C:
				cr.reload()
			}
		}
	}()
}

// Stop stops auto-reloading
func (cr *CertificateReloader) Stop() {
	close(cr.stopChan)
	<-cr.doneChan
}

// LastLoaded returns when the certificate was last loaded
func (cr *CertificateReloader) LastLoaded() time.Time {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return cr.lastLoaded
}

// LastError returns the last reload error
func (cr *CertificateReloader) LastError() error {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return cr.lastError
}

// TLSConnectionInfo holds information about a TLS connection
type TLSConnectionInfo struct {
	Version            uint16
	VersionName        string
	CipherSuite        uint16
	CipherSuiteName    string
	ServerName         string
	NegotiatedProtocol string
	PeerCertificates   []*x509.Certificate
	VerifiedChains     [][]*x509.Certificate
	HandshakeComplete  bool
}

// GetTLSInfo extracts TLS information from a connection state
func GetTLSInfo(state *tls.ConnectionState) *TLSConnectionInfo {
	if state == nil {
		return nil
	}

	return &TLSConnectionInfo{
		Version:            state.Version,
		VersionName:        TLSVersionName(state.Version),
		CipherSuite:        state.CipherSuite,
		CipherSuiteName:    tls.CipherSuiteName(state.CipherSuite),
		ServerName:         state.ServerName,
		NegotiatedProtocol: state.NegotiatedProtocol,
		PeerCertificates:   state.PeerCertificates,
		VerifiedChains:     state.VerifiedChains,
		HandshakeComplete:  state.HandshakeComplete,
	}
}

// TLSVersionName returns the name of a TLS version
func TLSVersionName(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("Unknown (0x%04x)", version)
	}
}

// ParseTLSVersion parses a TLS version string
func ParseTLSVersion(s string) (TLSVersion, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1.0", "tls1.0", "tlsv1.0":
		return TLSVersion10, nil
	case "1.1", "tls1.1", "tlsv1.1":
		return TLSVersion11, nil
	case "1.2", "tls1.2", "tlsv1.2":
		return TLSVersion12, nil
	case "1.3", "tls1.3", "tlsv1.3":
		return TLSVersion13, nil
	case "", "auto":
		return TLSVersionAuto, nil
	default:
		return 0, fmt.Errorf("%w: %s", ErrInvalidTLSVersion, s)
	}
}

// ValidateCertificate checks if a certificate is valid
func ValidateCertificate(cert *x509.Certificate) error {
	now := time.Now()

	if now.Before(cert.NotBefore) {
		return fmt.Errorf("%w: certificate not yet valid", ErrTLSCertInvalid)
	}

	if now.After(cert.NotAfter) {
		return ErrCertificateExpired
	}

	return nil
}

// CertificateExpiresIn returns when the certificate expires
func CertificateExpiresIn(cert *x509.Certificate) time.Duration {
	return time.Until(cert.NotAfter)
}

// IsCertificateExpiringSoon checks if the certificate expires within the given duration
func IsCertificateExpiringSoon(cert *x509.Certificate, within time.Duration) bool {
	return CertificateExpiresIn(cert) < within
}

// TLSStats holds TLS-related statistics
type TLSStats struct {
	TLS10Connections  int64
	TLS11Connections  int64
	TLS12Connections  int64
	TLS13Connections  int64
	HandshakeErrors   int64
	CertReloads       int64
	CertReloadErrors  int64
}

// CipherSuiteInfo describes a cipher suite
type CipherSuiteInfo struct {
	ID       uint16
	Name     string
	Secure   bool
	TLS13    bool
}

// GetCipherSuiteInfo returns information about supported cipher suites
func GetCipherSuiteInfo() []CipherSuiteInfo {
	var suites []CipherSuiteInfo

	// TLS 1.3 cipher suites (always secure)
	for _, cs := range tls.CipherSuites() {
		isTLS13 := false
		for _, v := range cs.SupportedVersions {
			if v == tls.VersionTLS13 {
				isTLS13 = true
				break
			}
		}
		suites = append(suites, CipherSuiteInfo{
			ID:     cs.ID,
			Name:   cs.Name,
			Secure: !cs.Insecure,
			TLS13:  isTLS13,
		})
	}

	return suites
}

// RecommendedCipherSuites returns the recommended cipher suites
func RecommendedCipherSuites() []uint16 {
	return []uint16{
		// TLS 1.3 cipher suites (automatically used when available)
		// TLS 1.2 AEAD cipher suites
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	}
}
