package internal

import (
	"log"

	"github.com/pion/rtp"
	"github.com/pion/srtp/v2"
)

// SRTP Profiles for encryption
var (
	SRTP_AES_CM_128_HMAC_SHA1_80 = srtp.ProtectionProfileAes128CmHmacSha1_80
	SRTP_AES_CM_128_HMAC_SHA1_32 = srtp.ProtectionProfileAes128CmHmacSha1_32
)

// StartSRTPSession initializes an SRTP encryption/decryption context
func StartSRTPSession(masterKey []byte, masterSalt []byte, profile srtp.ProtectionProfile) (*srtp.Context, error) {
	context, err := srtp.CreateContext(masterKey, masterSalt, profile)
	if err != nil {
		log.Printf("Failed to initialize SRTP session: %v", err)
		return nil, err
	}
	log.Println("SRTP session initialized successfully")
	return context, nil
}

// EncryptSRTP encrypts an RTP packet using the SRTP context
func EncryptSRTP(context *srtp.Context, packet *rtp.Packet) ([]byte, error) {
	payload := packet.Payload
	header := &packet.Header

	encryptedPacket, err := context.EncryptRTP(nil, payload, header)
	if err != nil {
		log.Printf("SRTP encryption error: %v", err)
		return nil, err
	}
	return encryptedPacket, nil
}

// DecryptSRTP decrypts an SRTP packet using the SRTP context
func DecryptSRTP(context *srtp.Context, encryptedPacket []byte, header *rtp.Header) ([]byte, error) {
	decryptedPayload, err := context.DecryptRTP(nil, encryptedPacket, header)
	if err != nil {
		log.Printf("SRTP decryption error: %v", err)
		return nil, err
	}
	return decryptedPayload, nil
}
