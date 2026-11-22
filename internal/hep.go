package internal

import (
	"encoding/binary"
	"net"
	"sync"
	"time"
)

// HEP protocol constants (HEPv3 / EEP — RFC draft)
const (
	hepVersion    = 3
	hepHeaderLen  = 6 // "HEP3" + length (2 bytes)
	hepProtoSIP   = 0x01
	hepProtoRTCP  = 0x05
	hepProtoLog   = 0x64

	// Chunk type IDs
	chunkIPProtoFamily = 0x0001
	chunkIPProtoID     = 0x0002
	chunkSrcIPv4       = 0x0003
	chunkDstIPv4       = 0x0004
	chunkSrcIPv6       = 0x0005
	chunkDstIPv6       = 0x0006
	chunkSrcPort       = 0x0007
	chunkDstPort       = 0x0008
	chunkTimeSec       = 0x0009
	chunkTimeUSec      = 0x000a
	chunkProtoType     = 0x000b
	chunkCaptureID     = 0x000c
	chunkPayload       = 0x000f
	chunkCorrelationID = 0x0011
)

// HEPConfig holds Homer/HEP exporter configuration.
type HEPConfig struct {
	Enabled     bool   `json:"hep_enabled"`
	ServerAddr  string `json:"hep_server"`   // e.g. "homer:9060"
	CaptureID   uint32 `json:"hep_capture_id"`
	Proto       string `json:"hep_proto"`     // "udp" (default) or "tcp"
}

// HEPExporter sends HEPv3 packets to a Homer capture server.
type HEPExporter struct {
	config HEPConfig
	conn   net.Conn
	mu     sync.Mutex
}

// NewHEPExporter creates and connects a HEP exporter.
func NewHEPExporter(config HEPConfig) (*HEPExporter, error) {
	proto := config.Proto
	if proto == "" {
		proto = "udp"
	}
	conn, err := net.DialTimeout(proto, config.ServerAddr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	return &HEPExporter{config: config, conn: conn}, nil
}

// SendSIP sends a SIP message as HEPv3.
func (h *HEPExporter) SendSIP(srcIP, dstIP net.IP, srcPort, dstPort uint16, payload []byte, correlationID string) error {
	return h.send(srcIP, dstIP, srcPort, dstPort, hepProtoSIP, payload, correlationID)
}

// SendRTCP sends an RTCP report as HEPv3.
func (h *HEPExporter) SendRTCP(srcIP, dstIP net.IP, srcPort, dstPort uint16, payload []byte, correlationID string) error {
	return h.send(srcIP, dstIP, srcPort, dstPort, hepProtoRTCP, payload, correlationID)
}

func (h *HEPExporter) send(srcIP, dstIP net.IP, srcPort, dstPort uint16, protoType byte, payload []byte, correlationID string) error {
	var chunks []byte
	isIPv6 := srcIP.To4() == nil

	var ipFamily byte = 2 // AF_INET
	if isIPv6 {
		ipFamily = 10 // AF_INET6
	}

	chunks = append(chunks, makeChunkU8(chunkIPProtoFamily, ipFamily)...)
	chunks = append(chunks, makeChunkU8(chunkIPProtoID, 17)...) // UDP
	if isIPv6 {
		chunks = append(chunks, makeChunk(chunkSrcIPv6, srcIP.To16())...)
		chunks = append(chunks, makeChunk(chunkDstIPv6, dstIP.To16())...)
	} else {
		chunks = append(chunks, makeChunk(chunkSrcIPv4, srcIP.To4())...)
		chunks = append(chunks, makeChunk(chunkDstIPv4, dstIP.To4())...)
	}
	chunks = append(chunks, makeChunkU16(chunkSrcPort, srcPort)...)
	chunks = append(chunks, makeChunkU16(chunkDstPort, dstPort)...)

	now := time.Now()
	chunks = append(chunks, makeChunkU32(chunkTimeSec, uint32(now.Unix()))...)
	chunks = append(chunks, makeChunkU32(chunkTimeUSec, uint32(now.Nanosecond()/1000))...)
	chunks = append(chunks, makeChunkU8(chunkProtoType, protoType)...)
	chunks = append(chunks, makeChunkU32(chunkCaptureID, h.config.CaptureID)...)

	if correlationID != "" {
		chunks = append(chunks, makeChunk(chunkCorrelationID, []byte(correlationID))...)
	}
	chunks = append(chunks, makeChunk(chunkPayload, payload)...)

	// HEP3 header: "HEP3" + total length (2 bytes)
	totalLen := 4 + 2 + len(chunks)
	msg := make([]byte, totalLen)
	copy(msg, "HEP3")
	binary.BigEndian.PutUint16(msg[4:6], uint16(totalLen))
	copy(msg[6:], chunks)

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.conn.Write(msg)
	return err
}

// Close closes the HEP connection.
func (h *HEPExporter) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conn != nil {
		return h.conn.Close()
	}
	return nil
}

func makeChunk(vendorType uint16, data []byte) []byte {
	// vendor (2) + type (2) + length (2) + data
	chunk := make([]byte, 6+len(data))
	binary.BigEndian.PutUint16(chunk[0:2], 0x0000) // generic vendor
	binary.BigEndian.PutUint16(chunk[2:4], vendorType)
	binary.BigEndian.PutUint16(chunk[4:6], uint16(6+len(data)))
	copy(chunk[6:], data)
	return chunk
}

func makeChunkU8(vendorType uint16, val byte) []byte {
	return makeChunk(vendorType, []byte{val})
}

func makeChunkU16(vendorType uint16, val uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, val)
	return makeChunk(vendorType, b)
}

func makeChunkU32(vendorType uint16, val uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, val)
	return makeChunk(vendorType, b)
}
