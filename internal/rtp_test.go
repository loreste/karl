package internal

import (
	"encoding/binary"
	"errors"
	"testing"
)

var (
	errRTPTooShort    = errors.New("rtp packet too short")
	errRTPInvalidVer  = errors.New("invalid RTP version")
)

// TestRTPHeader represents an RTP packet header for testing
type TestRTPHeader struct {
	Version        uint8
	Padding        bool
	Extension      bool
	CSRCCount      uint8
	Marker         bool
	PayloadType    uint8
	SequenceNumber uint16
	Timestamp      uint32
	SSRC           uint32
	CSRC           []uint32
}

func TestRTPHeaderParsing(t *testing.T) {
	tests := []struct {
		name     string
		packet   []byte
		wantErr  bool
		validate func(t *testing.T, header *TestRTPHeader)
	}{
		{
			name: "basic RTP packet",
			packet: []byte{
				0x80, 0x00,
				0x00, 0x01,
				0x00, 0x00, 0x00, 0x64,
				0x12, 0x34, 0x56, 0x78,
				0x01, 0x02, 0x03, 0x04,
			},
			validate: func(t *testing.T, h *TestRTPHeader) {
				if h.Version != 2 {
					t.Errorf("expected version 2, got %d", h.Version)
				}
				if h.SequenceNumber != 1 {
					t.Errorf("expected seq 1, got %d", h.SequenceNumber)
				}
				if h.SSRC != 0x12345678 {
					t.Errorf("expected SSRC 0x12345678, got 0x%X", h.SSRC)
				}
			},
		},
		{
			name: "RTP with marker bit",
			packet: []byte{
				0x80, 0x80,
				0x00, 0x02,
				0x00, 0x00, 0x00, 0xC8,
				0xAB, 0xCD, 0xEF, 0x01,
			},
			validate: func(t *testing.T, h *TestRTPHeader) {
				if !h.Marker {
					t.Error("expected marker bit set")
				}
			},
		},
		{
			name: "RTP with CSRC",
			packet: []byte{
				0x82, 0x00,
				0x00, 0x03,
				0x00, 0x00, 0x01, 0x00,
				0x11, 0x22, 0x33, 0x44,
				0xAA, 0xBB, 0xCC, 0xDD,
				0x11, 0x22, 0x33, 0x44,
			},
			validate: func(t *testing.T, h *TestRTPHeader) {
				if h.CSRCCount != 2 {
					t.Errorf("expected 2 CSRCs, got %d", h.CSRCCount)
				}
				if len(h.CSRC) != 2 {
					t.Fatalf("expected 2 CSRC entries, got %d", len(h.CSRC))
				}
				if h.CSRC[0] != 0xAABBCCDD {
					t.Errorf("expected CSRC[0] 0xAABBCCDD, got 0x%X", h.CSRC[0])
				}
			},
		},
		{
			name:    "too short packet",
			packet:  []byte{0x80, 0x00, 0x00},
			wantErr: true,
		},
		{
			name: "wrong version",
			packet: []byte{
				0x40, 0x00,
				0x00, 0x01,
				0x00, 0x00, 0x00, 0x64,
				0x12, 0x34, 0x56, 0x78,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header, err := parseTestRTPHeader(tt.packet)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTestRTPHeader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, header)
			}
		})
	}
}

func TestRTPHeaderSerialization(t *testing.T) {
	header := &TestRTPHeader{
		Version:        2,
		Padding:        false,
		Extension:      false,
		CSRCCount:      0,
		Marker:         true,
		PayloadType:    0,
		SequenceNumber: 12345,
		Timestamp:      67890,
		SSRC:           0xDEADBEEF,
	}

	packet := serializeTestRTPHeader(header)
	parsed, err := parseTestRTPHeader(packet)
	if err != nil {
		t.Fatalf("failed to parse serialized header: %v", err)
	}

	if parsed.SequenceNumber != header.SequenceNumber {
		t.Errorf("seq mismatch: got %d, want %d", parsed.SequenceNumber, header.SequenceNumber)
	}
	if parsed.SSRC != header.SSRC {
		t.Errorf("SSRC mismatch: got 0x%X, want 0x%X", parsed.SSRC, header.SSRC)
	}
}

func TestSequenceNumberWrapAround(t *testing.T) {
	tests := []struct {
		prev    uint16
		curr    uint16
		isNewer bool
	}{
		{1, 2, true},
		{65534, 65535, true},
		{65535, 0, true},
		{65535, 1, true},
		{100, 99, false},
		{0, 65535, false},
		{1, 65534, false},
		{32767, 32768, true},
		{32768, 32767, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := isSeqNewer(tt.curr, tt.prev)
			if result != tt.isNewer {
				t.Errorf("isSeqNewer(%d, %d) = %v, want %v", tt.curr, tt.prev, result, tt.isNewer)
			}
		})
	}
}

func TestRTCPIntervalCalculation(t *testing.T) {
	tests := []struct {
		members   int
		senders   int
		bandwidth float64
		minValid  float64
		maxValid  float64
	}{
		{members: 2, senders: 1, bandwidth: 64000, minValid: 2.5, maxValid: 5.0},
		{members: 10, senders: 2, bandwidth: 64000, minValid: 2.5, maxValid: 10.0},
		{members: 100, senders: 10, bandwidth: 64000, minValid: 2.5, maxValid: 50.0},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			interval := calcRTCPInterval(tt.members, tt.senders, tt.bandwidth, false)
			if interval < tt.minValid || interval > tt.maxValid {
				t.Errorf("interval %v not in range [%v, %v]", interval, tt.minValid, tt.maxValid)
			}
		})
	}
}

// Helper functions

func parseTestRTPHeader(data []byte) (*TestRTPHeader, error) {
	if len(data) < 12 {
		return nil, errRTPTooShort
	}

	h := &TestRTPHeader{
		Version:        (data[0] >> 6) & 0x03,
		Padding:        (data[0] & 0x20) != 0,
		Extension:      (data[0] & 0x10) != 0,
		CSRCCount:      data[0] & 0x0F,
		Marker:         (data[1] & 0x80) != 0,
		PayloadType:    data[1] & 0x7F,
		SequenceNumber: binary.BigEndian.Uint16(data[2:4]),
		Timestamp:      binary.BigEndian.Uint32(data[4:8]),
		SSRC:           binary.BigEndian.Uint32(data[8:12]),
	}

	if h.Version != 2 {
		return nil, errRTPInvalidVer
	}

	if h.CSRCCount > 0 {
		expectedLen := 12 + int(h.CSRCCount)*4
		if len(data) < expectedLen {
			return nil, errRTPTooShort
		}
		h.CSRC = make([]uint32, h.CSRCCount)
		for i := uint8(0); i < h.CSRCCount; i++ {
			offset := 12 + i*4
			h.CSRC[i] = binary.BigEndian.Uint32(data[offset : offset+4])
		}
	}

	return h, nil
}

func serializeTestRTPHeader(h *TestRTPHeader) []byte {
	headerLen := 12 + len(h.CSRC)*4
	data := make([]byte, headerLen)

	data[0] = (h.Version << 6)
	if h.Padding {
		data[0] |= 0x20
	}
	if h.Extension {
		data[0] |= 0x10
	}
	data[0] |= uint8(len(h.CSRC)) & 0x0F

	data[1] = h.PayloadType & 0x7F
	if h.Marker {
		data[1] |= 0x80
	}

	binary.BigEndian.PutUint16(data[2:4], h.SequenceNumber)
	binary.BigEndian.PutUint32(data[4:8], h.Timestamp)
	binary.BigEndian.PutUint32(data[8:12], h.SSRC)

	for i, csrc := range h.CSRC {
		offset := 12 + i*4
		binary.BigEndian.PutUint32(data[offset:], csrc)
	}

	return data
}

func isSeqNewer(seq, ref uint16) bool {
	diff := seq - ref
	return diff > 0 && diff < 0x8000
}

func calcRTCPInterval(members, senders int, bandwidth float64, initial bool) float64 {
	const minInterval = 2.5
	const senderFraction = 0.25
	const rtcpFraction = 0.05

	rtcpBw := bandwidth * rtcpFraction

	var n int
	if float64(senders) <= float64(members)*senderFraction {
		n = senders
		rtcpBw *= senderFraction
	} else {
		n = members - senders
		rtcpBw *= (1 - senderFraction)
	}

	if n == 0 {
		n = 1
	}

	avgSize := 100.0
	interval := (float64(n) * avgSize) / rtcpBw

	if interval < minInterval {
		interval = minInterval
	}

	if initial {
		interval /= 2
	}

	return interval
}
