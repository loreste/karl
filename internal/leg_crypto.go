package internal

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/pion/srtp/v2"
)

// LegCrypto holds per-leg SRTP/SRTCP encryption/decryption contexts.
// DecryptCtx decrypts incoming packets from the peer (using peer's SDES key).
// EncryptCtx encrypts outgoing packets to the peer (using our generated key).
// SRTCP contexts use the same key material but separate state for RTCP packets.
type LegCrypto struct {
	DecryptCtx     *srtp.Context
	EncryptCtx     *srtp.Context
	SRTCPDecryptCtx *srtp.Context
	SRTCPEncryptCtx *srtp.Context
}

// NewLegCrypto creates SRTP and SRTCP contexts for a call leg from base64-encoded SDES keys.
func NewLegCrypto(peerKeyB64, ourKeyB64 string) (*LegCrypto, error) {
	lc := &LegCrypto{}

	if peerKeyB64 != "" {
		key, salt, err := DecodeSDESKey(peerKeyB64)
		if err != nil {
			return nil, fmt.Errorf("decode peer SDES key: %w", err)
		}
		if lc.DecryptCtx, err = srtp.CreateContext(key, salt, srtp.ProtectionProfileAes128CmHmacSha1_80); err != nil {
			return nil, fmt.Errorf("create SRTP decrypt context: %w", err)
		}
		// SRTCP uses same key material but separate state
		if lc.SRTCPDecryptCtx, err = srtp.CreateContext(key, salt, srtp.ProtectionProfileAes128CmHmacSha1_80); err != nil {
			return nil, fmt.Errorf("create SRTCP decrypt context: %w", err)
		}
	}

	if ourKeyB64 != "" {
		key, salt, err := DecodeSDESKey(ourKeyB64)
		if err != nil {
			return nil, fmt.Errorf("decode our SDES key: %w", err)
		}
		if lc.EncryptCtx, err = srtp.CreateContext(key, salt, srtp.ProtectionProfileAes128CmHmacSha1_80); err != nil {
			return nil, fmt.Errorf("create SRTP encrypt context: %w", err)
		}
		if lc.SRTCPEncryptCtx, err = srtp.CreateContext(key, salt, srtp.ProtectionProfileAes128CmHmacSha1_80); err != nil {
			return nil, fmt.Errorf("create SRTCP encrypt context: %w", err)
		}
	}

	return lc, nil
}

// NewLegCryptoFromKeys creates SRTP/SRTCP contexts from raw key and salt bytes.
// Used when keys come from DTLS-SRTP handshake rather than SDES SDP attributes.
func NewLegCryptoFromKeys(decryptKey, decryptSalt, encryptKey, encryptSalt []byte) (*LegCrypto, error) {
	lc := &LegCrypto{}
	var err error

	if len(decryptKey) > 0 && len(decryptSalt) > 0 {
		if lc.DecryptCtx, err = srtp.CreateContext(decryptKey, decryptSalt, srtp.ProtectionProfileAes128CmHmacSha1_80); err != nil {
			return nil, fmt.Errorf("create SRTP decrypt context: %w", err)
		}
		if lc.SRTCPDecryptCtx, err = srtp.CreateContext(decryptKey, decryptSalt, srtp.ProtectionProfileAes128CmHmacSha1_80); err != nil {
			return nil, fmt.Errorf("create SRTCP decrypt context: %w", err)
		}
	}

	if len(encryptKey) > 0 && len(encryptSalt) > 0 {
		if lc.EncryptCtx, err = srtp.CreateContext(encryptKey, encryptSalt, srtp.ProtectionProfileAes128CmHmacSha1_80); err != nil {
			return nil, fmt.Errorf("create SRTP encrypt context: %w", err)
		}
		if lc.SRTCPEncryptCtx, err = srtp.CreateContext(encryptKey, encryptSalt, srtp.ProtectionProfileAes128CmHmacSha1_80); err != nil {
			return nil, fmt.Errorf("create SRTCP encrypt context: %w", err)
		}
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
