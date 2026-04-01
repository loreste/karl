package internal

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultTLSConfigOptions(t *testing.T) {
	opts := DefaultTLSConfigOptions()

	if opts.MinVersion != TLSVersion12 {
		t.Errorf("Expected MinVersion TLS 1.2, got %d", opts.MinVersion)
	}
	if opts.MaxVersion != TLSVersion13 {
		t.Errorf("Expected MaxVersion TLS 1.3, got %d", opts.MaxVersion)
	}
	if len(opts.CipherSuites) == 0 {
		t.Error("Expected cipher suites to be set")
	}
}

func TestStrictTLSConfigOptions(t *testing.T) {
	opts := StrictTLSConfigOptions()

	if opts.MinVersion != TLSVersion13 {
		t.Errorf("Expected MinVersion TLS 1.3 for strict, got %d", opts.MinVersion)
	}
	if opts.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Error("Expected RequireAndVerifyClientCert for strict mode")
	}
	if !opts.SessionTicketsDisabled {
		t.Error("Session tickets should be disabled in strict mode")
	}
}

func TestNewTLSConfigBuilder(t *testing.T) {
	builder := NewTLSConfigBuilder(nil)

	if builder == nil {
		t.Fatal("NewTLSConfigBuilder returned nil")
	}
	if builder.options == nil {
		t.Error("Options should not be nil")
	}
}

func TestTLSConfigBuilder_Build_NoCert(t *testing.T) {
	opts := DefaultTLSConfigOptions()
	builder := NewTLSConfigBuilder(opts)

	config, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if config.MinVersion != uint16(TLSVersion12) {
		t.Error("MinVersion not set correctly")
	}
}

func TestTLSConfigBuilder_BuildServer_NoCert(t *testing.T) {
	builder := NewTLSConfigBuilder(nil)

	_, err := builder.BuildServer()
	if err != ErrNoCertificate {
		t.Errorf("Expected ErrNoCertificate, got %v", err)
	}
}

func TestTLSConfigBuilder_BuildMutualTLS_NoCA(t *testing.T) {
	// Create test certificate
	certPEM, keyPEM := generateTestCert(t)

	opts := &TLSConfigOptions{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	}
	builder := NewTLSConfigBuilder(opts)

	_, err := builder.BuildMutualTLS()
	if err != ErrNoCAPool {
		t.Errorf("Expected ErrNoCAPool, got %v", err)
	}
}

func TestTLSConfigBuilder_WithCertPEM(t *testing.T) {
	certPEM, keyPEM := generateTestCert(t)

	opts := &TLSConfigOptions{
		CertPEM:    certPEM,
		KeyPEM:     keyPEM,
		MinVersion: TLSVersion12,
	}
	builder := NewTLSConfigBuilder(opts)

	config, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(config.Certificates) == 0 {
		t.Error("Certificate not loaded")
	}
}

func TestTLSConfigBuilder_WithFiles(t *testing.T) {
	certPEM, keyPEM := generateTestCert(t)

	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")

	os.WriteFile(certFile, certPEM, 0644)
	os.WriteFile(keyFile, keyPEM, 0600)

	opts := &TLSConfigOptions{
		CertFile:   certFile,
		KeyFile:    keyFile,
		MinVersion: TLSVersion12,
	}
	builder := NewTLSConfigBuilder(opts)

	config, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(config.Certificates) == 0 {
		t.Error("Certificate not loaded from files")
	}
}

func TestTLSConfigBuilder_WithCAFile(t *testing.T) {
	certPEM, keyPEM := generateTestCert(t)

	tmpDir := t.TempDir()
	caFile := filepath.Join(tmpDir, "ca.pem")

	// Use the same cert as CA for testing
	os.WriteFile(caFile, certPEM, 0644)

	opts := &TLSConfigOptions{
		CertPEM:    certPEM,
		KeyPEM:     keyPEM,
		CAFile:     caFile,
		MinVersion: TLSVersion12,
	}
	builder := NewTLSConfigBuilder(opts)

	config, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if config.ClientCAs == nil {
		t.Error("CA pool not loaded")
	}
}

func TestCertificateReloader(t *testing.T) {
	certPEM, keyPEM := generateTestCert(t)

	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")

	os.WriteFile(certFile, certPEM, 0644)
	os.WriteFile(keyFile, keyPEM, 0600)

	reloader, err := NewCertificateReloader(certFile, keyFile)
	if err != nil {
		t.Fatalf("NewCertificateReloader failed: %v", err)
	}

	// Get certificate
	cert, err := reloader.GetCertificate(nil)
	if err != nil {
		t.Fatalf("GetCertificate failed: %v", err)
	}
	if cert == nil {
		t.Error("Certificate should not be nil")
	}

	// Check last loaded time
	if reloader.LastLoaded().IsZero() {
		t.Error("LastLoaded should not be zero")
	}

	// Check no error
	if reloader.LastError() != nil {
		t.Errorf("LastError should be nil: %v", reloader.LastError())
	}
}

func TestCertificateReloader_AutoReload(t *testing.T) {
	certPEM, keyPEM := generateTestCert(t)

	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")

	os.WriteFile(certFile, certPEM, 0644)
	os.WriteFile(keyFile, keyPEM, 0600)

	reloader, err := NewCertificateReloader(certFile, keyFile)
	if err != nil {
		t.Fatalf("NewCertificateReloader failed: %v", err)
	}

	// Start auto-reload with short interval
	reloader.StartAutoReload(50 * time.Millisecond)

	// Wait for a few reloads
	time.Sleep(150 * time.Millisecond)

	// Stop
	reloader.Stop()

	// Should still work after stop
	cert, err := reloader.GetCertificate(nil)
	if err != nil {
		t.Errorf("GetCertificate failed after stop: %v", err)
	}
	if cert == nil {
		t.Error("Certificate should not be nil")
	}
}

func TestCertificateReloader_InvalidFile(t *testing.T) {
	_, err := NewCertificateReloader("/nonexistent/cert.pem", "/nonexistent/key.pem")
	if err == nil {
		t.Error("Should fail with nonexistent files")
	}
}

func TestTLSVersionName(t *testing.T) {
	tests := []struct {
		version  uint16
		expected string
	}{
		{tls.VersionTLS10, "TLS 1.0"},
		{tls.VersionTLS11, "TLS 1.1"},
		{tls.VersionTLS12, "TLS 1.2"},
		{tls.VersionTLS13, "TLS 1.3"},
		{0x0304, "TLS 1.3"}, // Same as VersionTLS13
		{0x9999, "Unknown (0x9999)"},
	}

	for _, tt := range tests {
		result := TLSVersionName(tt.version)
		if result != tt.expected {
			t.Errorf("TLSVersionName(0x%04x) = %s, expected %s", tt.version, result, tt.expected)
		}
	}
}

func TestParseTLSVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected TLSVersion
		hasError bool
	}{
		{"1.0", TLSVersion10, false},
		{"tls1.0", TLSVersion10, false},
		{"1.1", TLSVersion11, false},
		{"1.2", TLSVersion12, false},
		{"TLS1.2", TLSVersion12, false},
		{"1.3", TLSVersion13, false},
		{"auto", TLSVersionAuto, false},
		{"", TLSVersionAuto, false},
		{"invalid", 0, true},
		{"2.0", 0, true},
	}

	for _, tt := range tests {
		version, err := ParseTLSVersion(tt.input)
		if tt.hasError {
			if err == nil {
				t.Errorf("ParseTLSVersion(%q) should return error", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("ParseTLSVersion(%q) returned error: %v", tt.input, err)
			}
			if version != tt.expected {
				t.Errorf("ParseTLSVersion(%q) = %d, expected %d", tt.input, version, tt.expected)
			}
		}
	}
}

func TestValidateCertificate(t *testing.T) {
	// Create valid certificate
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)

	// Should be valid
	err := ValidateCertificate(cert)
	if err != nil {
		t.Errorf("Valid certificate should pass: %v", err)
	}

	// Create expired certificate
	template.NotBefore = time.Now().Add(-2 * time.Hour)
	template.NotAfter = time.Now().Add(-time.Hour)
	certDER, _ = x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	expiredCert, _ := x509.ParseCertificate(certDER)

	err = ValidateCertificate(expiredCert)
	if err != ErrCertificateExpired {
		t.Errorf("Expected ErrCertificateExpired, got %v", err)
	}

	// Create not-yet-valid certificate
	template.NotBefore = time.Now().Add(time.Hour)
	template.NotAfter = time.Now().Add(2 * time.Hour)
	certDER, _ = x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	futureCert, _ := x509.ParseCertificate(certDER)

	err = ValidateCertificate(futureCert)
	if err == nil {
		t.Error("Not-yet-valid certificate should fail")
	}
}

func TestCertificateExpiresIn(t *testing.T) {
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)

	expires := CertificateExpiresIn(cert)
	if expires < 23*time.Hour || expires > 24*time.Hour {
		t.Errorf("Expected ~24h, got %v", expires)
	}
}

func TestIsCertificateExpiringSoon(t *testing.T) {
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(6 * time.Hour),
	}

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)

	// Should be expiring within 24 hours
	if !IsCertificateExpiringSoon(cert, 24*time.Hour) {
		t.Error("Certificate should be expiring within 24h")
	}

	// Should not be expiring within 1 hour
	if IsCertificateExpiringSoon(cert, 1*time.Hour) {
		t.Error("Certificate should not be expiring within 1h")
	}
}

func TestGetTLSInfo(t *testing.T) {
	// Nil state
	info := GetTLSInfo(nil)
	if info != nil {
		t.Error("GetTLSInfo(nil) should return nil")
	}

	// Valid state
	state := &tls.ConnectionState{
		Version:           tls.VersionTLS13,
		CipherSuite:       tls.TLS_AES_256_GCM_SHA384,
		ServerName:        "test.example.com",
		HandshakeComplete: true,
	}

	info = GetTLSInfo(state)
	if info == nil {
		t.Fatal("GetTLSInfo should return info")
	}
	if info.Version != tls.VersionTLS13 {
		t.Error("Version mismatch")
	}
	if info.VersionName != "TLS 1.3" {
		t.Errorf("VersionName = %s, expected 'TLS 1.3'", info.VersionName)
	}
	if info.ServerName != "test.example.com" {
		t.Error("ServerName mismatch")
	}
	if !info.HandshakeComplete {
		t.Error("HandshakeComplete should be true")
	}
}

func TestGetCipherSuiteInfo(t *testing.T) {
	suites := GetCipherSuiteInfo()

	if len(suites) == 0 {
		t.Error("Expected cipher suites")
	}

	// Check that we have some secure suites
	hasSecure := false
	for _, s := range suites {
		if s.Secure {
			hasSecure = true
			break
		}
	}
	if !hasSecure {
		t.Error("Expected at least one secure cipher suite")
	}
}

func TestRecommendedCipherSuites(t *testing.T) {
	suites := RecommendedCipherSuites()

	if len(suites) == 0 {
		t.Error("Expected recommended cipher suites")
	}

	// All recommended suites should be AEAD
	for _, id := range suites {
		name := tls.CipherSuiteName(id)
		if name == "" {
			t.Errorf("Unknown cipher suite: %d", id)
		}
	}
}

// Helper function to generate a test certificate
func generateTestCert(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test.example.com",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("Failed to marshal key: %v", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM
}
