package internal

import (
	"log"
	"net"

	"github.com/pion/rtp"
	"github.com/pion/srtp/v2"
	"github.com/pion/webrtc/v3"
)

// TranscodeRTPToSRTP encrypts an RTP packet and converts it into SRTP for WebRTC transmission
func TranscodeRTPToSRTP(context *srtp.Context, rtpPacket *rtp.Packet) ([]byte, error) {
	// Serialize RTP packet
	packetBytes, err := rtpPacket.Marshal()
	if err != nil {
		log.Printf("❌ Failed to marshal RTP packet: %v", err)
		return nil, err
	}

	// Buffer to hold encrypted SRTP payload
	encryptedPayload := make([]byte, len(packetBytes))

	// Encrypt RTP to SRTP
	encryptedPayload, err = context.EncryptRTP(encryptedPayload[:0], packetBytes, &rtpPacket.Header)
	if err != nil {
		log.Printf("❌ SRTP encryption error: %v", err)
		return nil, err
	}

	log.Printf("✅ Transcoded RTP → SRTP (SSRC=%d, Seq=%d, TS=%d)",
		rtpPacket.SSRC, rtpPacket.SequenceNumber, rtpPacket.Timestamp)

	return encryptedPayload, nil
}

// TranscodeSRTPToRTP decrypts an incoming SRTP packet and converts it into RTP for SIP transmission
func TranscodeSRTPToRTP(context *srtp.Context, encryptedPayload []byte) (*rtp.Packet, error) {
	// Buffer to hold decrypted RTP payload
	decryptedPayload := make([]byte, len(encryptedPayload))

	// Decrypt SRTP packet
	_, err := context.DecryptRTP(decryptedPayload[:0], encryptedPayload, nil)
	if err != nil {
		log.Printf("❌ SRTP decryption error: %v", err)
		return nil, err
	}

	// Parse the RTP packet after decryption
	rtpPacket := &rtp.Packet{}
	err = rtpPacket.Unmarshal(decryptedPayload)
	if err != nil {
		log.Printf("❌ Failed to parse RTP after SRTP decryption: %v", err)
		return nil, err
	}

	log.Printf("✅ Transcoded SRTP → RTP (SSRC=%d, Seq=%d, TS=%d)",
		rtpPacket.SSRC, rtpPacket.SequenceNumber, rtpPacket.Timestamp)

	return rtpPacket, nil
}

// ForwardRTPWebRTCToSIP decrypts SRTP from WebRTC and forwards RTP to SIP
func ForwardRTPWebRTCToSIP(track *webrtc.TrackRemote, sipEndpoint string, srtpContext *srtp.Context) {
	log.Printf("🔄 Forwarding RTP packets from WebRTC to SIP: %s", sipEndpoint)

	// Create UDP connection to SIP RTP proxy
	conn, err := net.Dial("udp", sipEndpoint)
	if err != nil {
		log.Printf("❌ Failed to connect to SIP RTP endpoint: %v", err)
		return
	}
	defer conn.Close()

	for {
		// Read SRTP packet from WebRTC track
		rtpPacket, _, err := track.ReadRTP()
		if err != nil {
			log.Printf("❌ Failed to read RTP packet from WebRTC: %v", err)
			return
		}

		// Convert SRTP → RTP
		plainRTP, err := TranscodeSRTPToRTP(srtpContext, rtpPacket.Payload)
		if err != nil {
			log.Printf("❌ Failed to transcode SRTP to RTP: %v", err)
			continue
		}

		// Serialize RTP packet
		packetBytes, err := plainRTP.Marshal()
		if err != nil {
			log.Printf("❌ Failed to marshal RTP packet: %v", err)
			continue
		}

		// Send RTP packet to SIP endpoint
		_, err = conn.Write(packetBytes)
		if err != nil {
			log.Printf("❌ Failed to send RTP to SIP: %v", err)
			return
		}

		log.Printf("✅ Forwarded RTP to SIP (SSRC=%d, Seq=%d, TS=%d)",
			plainRTP.SSRC, plainRTP.SequenceNumber, plainRTP.Timestamp)
	}
}

// ForwardRTPSIPToWebRTC receives RTP from SIP, encrypts it as SRTP, and sends to WebRTC
func ForwardRTPSIPToWebRTC(conn *net.UDPConn, track *webrtc.TrackLocalStaticRTP, srtpContext *srtp.Context) {
	log.Println("🔄 Forwarding RTP from SIP to WebRTC")

	buf := make([]byte, 1500)
	for {
		// Read RTP packet from SIP
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("❌ Failed to read RTP from SIP: %v", err)
			continue
		}

		// Parse RTP packet from SIP
		rtpPacket := &rtp.Packet{}
		err = rtpPacket.Unmarshal(buf[:n])
		if err != nil {
			log.Printf("❌ Failed to unmarshal RTP from SIP: %v", err)
			continue
		}

		// Convert RTP → SRTP
		encryptedSRTP, err := TranscodeRTPToSRTP(srtpContext, rtpPacket)
		if err != nil {
			log.Printf("❌ Failed to transcode RTP to SRTP: %v", err)
			continue
		}

		// ✅ Send encrypted SRTP to WebRTC
		n, err = track.Write(encryptedSRTP)
		if err != nil {
			log.Printf("❌ Failed to send SRTP to WebRTC: %v", err)
		} else {
			log.Printf("✅ Forwarded SRTP to WebRTC (Bytes Sent=%d, SSRC=%d, Seq=%d, TS=%d)",
				n, rtpPacket.SSRC, rtpPacket.SequenceNumber, rtpPacket.Timestamp)
		}
	}
}
