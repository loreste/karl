package internal

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/pion/srtp/v2"
)

// LegCrypto holds per-leg SRTP encryption/decryption contexts.
// DecryptCtx decrypts incoming packets from the peer (using peer's SDES key).
// EncryptCtx encrypts outgoing packets to the peer (using our generated key).
type LegCrypto struct {
	DecryptCtx *srtp.Context
	EncryptCtx *srtp.Context
}

// NewLegCrypto creates SRTP contexts for a call leg from base64-encoded SDES keys.
func NewLegCrypto(peerKeyB64, ourKeyB64 string) (*LegCrypto, error) {
	lc := &LegCrypto{}

	if peerKeyB64 != "" {
		key, salt, err := DecodeSDESKey(peerKeyB64)
		if err != nil {
			return nil, fmt.Errorf("decode peer SDES key: %w", err)
		}
		ctx, err := srtp.CreateContext(key, salt, srtp.ProtectionProfileAes128CmHmacSha1_80)
		if err != nil {
			return nil, fmt.Errorf("create decrypt context: %w", err)
		}
		lc.DecryptCtx = ctx
	}

	if ourKeyB64 != "" {
		key, salt, err := DecodeSDESKey(ourKeyB64)
		if err != nil {
			return nil, fmt.Errorf("decode our SDES key: %w", err)
		}
		ctx, err := srtp.CreateContext(key, salt, srtp.ProtectionProfileAes128CmHmacSha1_80)
		if err != nil {
			return nil, fmt.Errorf("create encrypt context: %w", err)
		}
		lc.EncryptCtx = ctx
	}

	return lc, nil
}

// DecodeSDESKey decodes a base64 SDES key into AES-128 key (16 bytes) and salt (14 bytes).
func DecodeSDESKey(b64 string) (key, salt []byte, err error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, nil, err
	}
	if len(raw) < 30 {
		return nil, nil, fmt.Errorf("SDES key too short: %d bytes, need 30", len(raw))
	}
	return raw[:16], raw[16:30], nil
}

// GenerateSDESKeyPair produces a random SDES master key+salt and returns both
// the raw bytes and the base64-encoded string for SDP.
func GenerateSDESKeyPair() (b64 string) {
	keySalt := make([]byte, 30+14) // 30-byte key + 14-byte salt
	rand.Read(keySalt)
	return base64.StdEncoding.EncodeToString(keySalt)
}
