package internal

import (
	"testing"
)

func TestDecodeSDESKey(t *testing.T) {
	b64 := GenerateSDESKeyPair()
	key, salt, err := DecodeSDESKey(b64)
	if err != nil {
		t.Fatalf("DecodeSDESKey failed: %v", err)
	}
	if len(key) != 16 {
		t.Errorf("expected 16-byte key, got %d", len(key))
	}
	if len(salt) != 14 {
		t.Errorf("expected 14-byte salt, got %d", len(salt))
	}
}

func TestNewLegCrypto(t *testing.T) {
	peerKey := GenerateSDESKeyPair()
	ourKey := GenerateSDESKeyPair()

	lc, err := NewLegCrypto(peerKey, ourKey)
	if err != nil {
		t.Fatalf("NewLegCrypto failed: %v", err)
	}
	if lc.DecryptCtx == nil {
		t.Error("expected DecryptCtx to be set")
	}
	if lc.EncryptCtx == nil {
		t.Error("expected EncryptCtx to be set")
	}
}

func TestNewLegCrypto_EmptyKeys(t *testing.T) {
	lc, err := NewLegCrypto("", "")
	if err != nil {
		t.Fatalf("NewLegCrypto with empty keys should not error: %v", err)
	}
	if lc.DecryptCtx != nil || lc.EncryptCtx != nil {
		t.Error("expected nil contexts for empty keys")
	}
}

func TestCalculateMOS(t *testing.T) {
	// Perfect conditions
	mos := CalculateMOS(0, 0, 0)
	if mos < 4.0 {
		t.Errorf("perfect conditions should give MOS > 4.0, got %.2f", mos)
	}

	// High packet loss
	mos = CalculateMOS(20, 10, 100)
	if mos > 3.5 {
		t.Errorf("20%% loss should give MOS < 3.5, got %.2f", mos)
	}

	// Terrible conditions
	mos = CalculateMOS(50, 100, 500)
	if mos > 2.0 {
		t.Errorf("terrible conditions should give MOS < 2.0, got %.2f", mos)
	}

	// MOS always in [1.0, 4.5]
	for _, loss := range []float64{0, 1, 5, 10, 50, 100} {
		m := CalculateMOS(loss, 50, 200)
		if m < 1.0 || m > 4.5 {
			t.Errorf("MOS %.2f out of range [1.0, 4.5] for loss=%.0f%%", m, loss)
		}
	}
}
