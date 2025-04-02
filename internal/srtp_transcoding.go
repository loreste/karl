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
	// Check for nil context
	if t.Context == nil {
		return nil, fmt.Errorf("SRTP context not initialized")
	}

	// Validate input packet
	if len(packet) < 12 {
		return nil, fmt.Errorf("RTP packet too short (min 12 bytes required): %d bytes", len(packet))
	}

	// Parse RTP packet
	rtpPacket := &rtp.Packet{}
	if err := rtpPacket.Unmarshal(packet); err != nil {
		IncrementErrorMetric("rtp_unmarshal_error")
		return nil, fmt.Errorf("failed to unmarshal RTP packet: %w", err)
	}

	// Calculate buffer size based on SRTP overhead
	// SRTP adds authentication tag (10 bytes default for AES_CM_128_HMAC_SHA1_80)
	// We allocate extra space to be safe
	srtpOverhead := 20
	bufEncrypted := make([]byte, 0, len(packet)+srtpOverhead)

	// Encrypt RTP → SRTP
	encryptedPayload, err := t.Context.EncryptRTP(bufEncrypted, packet, &rtpPacket.Header)
	if err != nil {
		IncrementErrorMetric("srtp_encryption_error")
		return nil, fmt.Errorf("SRTP encryption error: %w", err)
	}

	// Validate output
	if len(encryptedPayload) < len(packet) {
		IncrementErrorMetric("srtp_invalid_output")
		return nil, fmt.Errorf("SRTP encryption produced invalid output: expected >%d bytes, got %d bytes", 
			len(packet), len(encryptedPayload))
	}

	// Debug logging is useful but should be configurable in production
	if LogLevel >= LogLevelDebug {
		log.Printf("Transcoded RTP → SRTP (SSRC=%d, Seq=%d, TS=%d, Size: %d→%d)",
			rtpPacket.SSRC,
			rtpPacket.SequenceNumber,
			rtpPacket.Timestamp,
			len(packet),
			len(encryptedPayload))
	}

	// Increment success metrics
	IncrementCounter("srtp_packets_encrypted")

	return encryptedPayload, nil
}

// TranscodeSRTPToRTP decrypts an SRTP packet for RTP transmission
func (t *SRTPTranscoder) TranscodeSRTPToRTP(encryptedPayload []byte) (*rtp.Packet, error) {
	// Check for nil context
	if t.Context == nil {
		return nil, fmt.Errorf("SRTP context not initialized")
	}

	// Validate input
	if len(encryptedPayload) < 12 {
		IncrementErrorMetric("srtp_packet_too_short")
		return nil, fmt.Errorf("SRTP packet too short (min 12 bytes required): %d bytes", len(encryptedPayload))
	}

	// Buffer for decrypted RTP payload
	decryptedPayload := make([]byte, 0, len(encryptedPayload))

	// Decrypt SRTP → RTP
	decryptedPayload, err := t.Context.DecryptRTP(decryptedPayload, encryptedPayload, nil)
	if err != nil {
		IncrementErrorMetric("srtp_decryption_error")
		return nil, fmt.Errorf("SRTP decryption error: %w", err)
	}

	// Parse the decrypted RTP packet
	rtpPacket := &rtp.Packet{}
	err = rtpPacket.Unmarshal(decryptedPayload)
	if err != nil {
		IncrementErrorMetric("rtp_parse_error")
		return nil, fmt.Errorf("failed to parse RTP after SRTP decryption: %w", err)
	}

	// Debug logging is useful but should be configurable
	if LogLevel >= LogLevelDebug {
		log.Printf("Transcoded SRTP → RTP (SSRC=%d, Seq=%d, TS=%d, Size: %d→%d)",
			rtpPacket.SSRC,
			rtpPacket.SequenceNumber,
			rtpPacket.Timestamp,
			len(encryptedPayload),
			len(decryptedPayload))
	}

	// Increment success metrics
	IncrementCounter("srtp_packets_decrypted") 

	return rtpPacket, nil
}

// GetContext returns the underlying SRTP context
// This method is kept for backward compatibility
func (t *SRTPTranscoder) GetContext() *srtp.Context {
	return t.Context
}
