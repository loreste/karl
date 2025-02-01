package internal

import (
	"fmt"
	"log"

	"github.com/pion/rtp"
	"github.com/pion/srtp/v2"
)

// SetSRTPContext initializes the SRTP context in the transcoder
func (t *SRTPTranscoder) SetSRTPContext(srtpKey, srtpSalt []byte) error {
	if len(srtpKey) == 0 || len(srtpSalt) == 0 {
		return fmt.Errorf("❌ SRTP key or salt is empty")
	}

	// Create SRTP context
	srtpContext, err := srtp.CreateContext(srtpKey, srtpSalt, srtp.ProtectionProfileAes128CmHmacSha1_80)
	if err != nil {
		log.Printf("❌ Failed to create SRTP context: %v", err)
		return err
	}

	t.Context = srtpContext
	log.Println("✅ SRTP Context successfully initialized")
	return nil
}

// SRTPTranscoder handles SRTP/RTP encryption & decryption
type SRTPTranscoder struct {
	Context *srtp.Context // ✅ Exported field (fixes `context undefined` issue)
}

// NewSRTPTranscoder initializes SRTP transcoder
func NewSRTPTranscoder(srtpKey, srtpSalt []byte) (*SRTPTranscoder, error) {
	if len(srtpKey) == 0 || len(srtpSalt) == 0 {
		return nil, fmt.Errorf("❌ SRTP key or salt is empty")
	}

	// Create SRTP context for encryption & decryption
	srtpContext, err := srtp.CreateContext(srtpKey, srtpSalt, srtp.ProtectionProfileAes128CmHmacSha1_80)
	if err != nil {
		log.Printf("❌ Failed to create SRTP context: %v", err)
		return nil, err
	}

	log.Println("✅ SRTP Context successfully initialized")
	return &SRTPTranscoder{Context: srtpContext}, nil
}

// TranscodeRTPToSRTP encrypts an RTP packet for SRTP transmission
func (t *SRTPTranscoder) TranscodeRTPToSRTP(packet []byte) ([]byte, error) {
	if t.Context == nil {
		return nil, fmt.Errorf("❌ SRTP context is not initialized")
	}

	// Parse RTP packet
	rtpPacket := &rtp.Packet{}
	if err := rtpPacket.Unmarshal(packet); err != nil {
		log.Printf("❌ Failed to unmarshal RTP packet: %v", err)
		return nil, err
	}

	// Buffer for encrypted SRTP payload
	encryptedPayload := make([]byte, len(packet))

	// Encrypt RTP → SRTP
	encryptedPayload, err := t.Context.EncryptRTP(encryptedPayload[:0], packet, &rtpPacket.Header)
	if err != nil {
		log.Printf("❌ SRTP encryption error: %v", err)
		return nil, err
	}

	log.Printf("✅ Transcoded RTP → SRTP (SSRC=%d, Seq=%d, TS=%d)",
		rtpPacket.SSRC,
		rtpPacket.SequenceNumber,
		rtpPacket.Timestamp)

	return encryptedPayload, nil
}

// TranscodeSRTPToRTP decrypts an SRTP packet for RTP transmission
func (t *SRTPTranscoder) TranscodeSRTPToRTP(encryptedPayload []byte) (*rtp.Packet, error) {
	if t.Context == nil {
		return nil, fmt.Errorf("❌ SRTP context is not initialized")
	}

	// Buffer for decrypted RTP payload
	decryptedPayload := make([]byte, len(encryptedPayload))

	// Decrypt SRTP → RTP
	_, err := t.Context.DecryptRTP(decryptedPayload[:0], encryptedPayload, nil)
	if err != nil {
		log.Printf("❌ SRTP decryption error: %v", err)
		return nil, err
	}

	// Parse the decrypted RTP packet
	rtpPacket := &rtp.Packet{}
	err = rtpPacket.Unmarshal(decryptedPayload)
	if err != nil {
		log.Printf("❌ Failed to parse RTP after SRTP decryption: %v", err)
		return nil, err
	}

	log.Printf("✅ Transcoded SRTP → RTP (SSRC=%d, Seq=%d, TS=%d)",
		rtpPacket.SSRC,
		rtpPacket.SequenceNumber,
		rtpPacket.Timestamp)

	return rtpPacket, nil
}

// GetContext returns the underlying SRTP context
func (t *SRTPTranscoder) GetContext() *srtp.Context {
	return t.Context
}
