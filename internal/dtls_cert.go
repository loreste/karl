package internal

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"
)

var (
	dtlsFingerprint string
	dtlsCertDER     []byte
	dtlsKey         *ecdsa.PrivateKey
	dtlsCertOnce    sync.Once
)

func initDTLSCert() {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		dtlsFingerprint = "sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00"
		return
	}
	dtlsKey = key

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "karl"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		dtlsFingerprint = "sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00"
		return
	}
	dtlsCertDER = der

	hash := sha256.Sum256(der)
	parts := make([]string, len(hash))
	for i, b := range hash {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	dtlsFingerprint = "sha-256 " + strings.Join(parts, ":")
}

// GetDTLSFingerprint returns the SHA-256 fingerprint of the self-signed DTLS certificate.
func GetDTLSFingerprint() string {
	dtlsCertOnce.Do(initDTLSCert)
	return dtlsFingerprint
}
