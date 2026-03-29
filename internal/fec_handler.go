package internal

import (
	"encoding/binary"
	"errors"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// FEC metrics
var (
	fecPacketsSent = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "karl_fec_packets_sent_total",
			Help: "Total FEC packets sent",
		},
	)

	fecRecoveriesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "karl_fec_recoveries_total",
			Help: "Total packets recovered using FEC",
		},
	)

	fecRecoveryFailures = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "karl_fec_recovery_failures_total",
			Help: "Total FEC recovery failures",
		},
	)

	fecRedundancyRatio = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "karl_fec_redundancy_ratio",
			Help: "Current FEC redundancy ratio",
		},
	)
)

// DefaultFECConfig returns default FEC configuration
func DefaultFECConfig() *FECConfig {
	return &FECConfig{
		Enabled:       true,
		BlockSize:     48,
		Redundancy:    0.30, // 30% redundancy
		AdaptiveMode:  true,
		MaxRedundancy: 0.50,
		MinRedundancy: 0.10,
	}
}

// FECPacket represents a Forward Error Correction packet
type FECPacket struct {
	SequenceBase uint16   // Base sequence number of protected packets
	ProtectedSeq []uint16 // List of protected sequence numbers
	PayloadXOR   []byte   // XOR of all protected payloads
	LengthXOR    uint16   // XOR of all payload lengths
	TimestampXOR uint32   // XOR of all timestamps
	PTMarkerXOR  byte     // XOR of PT and Marker bits
}

// FECHandler handles Forward Error Correction for RTP streams
type FECHandler struct {
	config *FECConfig

	// Encoding state
	encodingBlock    []*RTPPacketData
	encodingBlockSeq uint16
	fecSeqNum        uint16

	// Decoding state
	decodingBlocks  map[uint16]*FECDecodingBlock
	receivedPackets map[uint16]*RTPPacketData

	// Adaptive mode state
	lossHistory     []float64
	currentLossRate float64

	mu sync.Mutex
}

// RTPPacketData holds RTP packet data for FEC
type RTPPacketData struct {
	SequenceNumber uint16
	Timestamp      uint32
	PayloadType    uint8
	Marker         bool
	Payload        []byte
}

// FECDecodingBlock holds state for decoding an FEC block
type FECDecodingBlock struct {
	FECPacket       *FECPacket
	ReceivedPackets map[uint16]*RTPPacketData
	MissingPackets  []uint16
	Complete        bool
}

// NewFECHandler creates a new FEC handler
func NewFECHandler(config *FECConfig) *FECHandler {
	if config == nil {
		config = DefaultFECConfig()
	}

	return &FECHandler{
		config:          config,
		encodingBlock:   make([]*RTPPacketData, 0, config.BlockSize),
		decodingBlocks:  make(map[uint16]*FECDecodingBlock),
		receivedPackets: make(map[uint16]*RTPPacketData),
		lossHistory:     make([]float64, 0, 100),
	}
}

// AddMediaPacket adds a media packet to the encoding block
func (h *FECHandler) AddMediaPacket(pkt *RTPPacketData) *FECPacket {
	if !h.config.Enabled {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Start new block if empty
	if len(h.encodingBlock) == 0 {
		h.encodingBlockSeq = pkt.SequenceNumber
	}

	h.encodingBlock = append(h.encodingBlock, pkt)

	// Generate FEC packet when block is complete
	if len(h.encodingBlock) >= h.config.BlockSize {
		fecPkt := h.generateFECPacket()
		h.encodingBlock = make([]*RTPPacketData, 0, h.config.BlockSize)
		return fecPkt
	}

	return nil
}

// generateFECPacket generates an FEC packet from the current block
func (h *FECHandler) generateFECPacket() *FECPacket {
	if len(h.encodingBlock) == 0 {
		return nil
	}

	// Find max payload length
	maxLen := 0
	for _, pkt := range h.encodingBlock {
		if len(pkt.Payload) > maxLen {
			maxLen = len(pkt.Payload)
		}
	}

	fec := &FECPacket{
		SequenceBase: h.encodingBlockSeq,
		ProtectedSeq: make([]uint16, len(h.encodingBlock)),
		PayloadXOR:   make([]byte, maxLen),
	}

	// XOR all packets together
	for i, pkt := range h.encodingBlock {
		fec.ProtectedSeq[i] = pkt.SequenceNumber
		fec.LengthXOR ^= uint16(len(pkt.Payload))
		fec.TimestampXOR ^= pkt.Timestamp

		// XOR PT and Marker
		ptMarker := pkt.PayloadType
		if pkt.Marker {
			ptMarker |= 0x80
		}
		fec.PTMarkerXOR ^= ptMarker

		// XOR payload
		for j := 0; j < len(pkt.Payload); j++ {
			fec.PayloadXOR[j] ^= pkt.Payload[j]
		}
	}

	h.fecSeqNum++
	fecPacketsSent.Inc()
	fecRedundancyRatio.Set(float64(1) / float64(h.config.BlockSize))

	return fec
}

// ReceiveMediaPacket receives a media packet for potential recovery
func (h *FECHandler) ReceiveMediaPacket(pkt *RTPPacketData) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Store received packet
	h.receivedPackets[pkt.SequenceNumber] = pkt

	// Clean old packets outside window
	h.cleanupOldPackets(pkt.SequenceNumber)

	// Try to complete any pending FEC blocks
	h.tryCompleteBlocks(pkt.SequenceNumber)
}

// ReceiveFECPacket receives an FEC packet for potential recovery
func (h *FECHandler) ReceiveFECPacket(fec *FECPacket) []*RTPPacketData {
	if !h.config.Enabled || fec == nil {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Create or update decoding block
	block := &FECDecodingBlock{
		FECPacket:       fec,
		ReceivedPackets: make(map[uint16]*RTPPacketData),
		MissingPackets:  make([]uint16, 0),
	}

	// Check which packets we have and which are missing
	for _, seq := range fec.ProtectedSeq {
		if pkt, ok := h.receivedPackets[seq]; ok {
			block.ReceivedPackets[seq] = pkt
		} else {
			block.MissingPackets = append(block.MissingPackets, seq)
		}
	}

	// Store the block
	h.decodingBlocks[fec.SequenceBase] = block

	// Try to recover missing packets
	return h.tryRecoverPackets(block)
}

// tryRecoverPackets attempts to recover missing packets using FEC
func (h *FECHandler) tryRecoverPackets(block *FECDecodingBlock) []*RTPPacketData {
	// Can only recover if exactly one packet is missing
	if len(block.MissingPackets) != 1 {
		if len(block.MissingPackets) > 1 {
			fecRecoveryFailures.Inc()
		}
		return nil
	}

	missingSeq := block.MissingPackets[0]
	fec := block.FECPacket

	// Recover the missing packet by XORing FEC with all received packets
	recoveredPayload := make([]byte, len(fec.PayloadXOR))
	copy(recoveredPayload, fec.PayloadXOR)

	recoveredLengthXOR := fec.LengthXOR
	recoveredTimestampXOR := fec.TimestampXOR
	recoveredPTMarkerXOR := fec.PTMarkerXOR

	for seq, pkt := range block.ReceivedPackets {
		if seq == missingSeq {
			continue
		}

		recoveredLengthXOR ^= uint16(len(pkt.Payload))
		recoveredTimestampXOR ^= pkt.Timestamp

		ptMarker := pkt.PayloadType
		if pkt.Marker {
			ptMarker |= 0x80
		}
		recoveredPTMarkerXOR ^= ptMarker

		for j := 0; j < len(pkt.Payload) && j < len(recoveredPayload); j++ {
			recoveredPayload[j] ^= pkt.Payload[j]
		}
	}

	// Construct recovered packet
	recoveredPkt := &RTPPacketData{
		SequenceNumber: missingSeq,
		Timestamp:      recoveredTimestampXOR,
		PayloadType:    recoveredPTMarkerXOR & 0x7F,
		Marker:         (recoveredPTMarkerXOR & 0x80) != 0,
		Payload:        recoveredPayload[:recoveredLengthXOR],
	}

	// Store recovered packet
	h.receivedPackets[missingSeq] = recoveredPkt
	block.ReceivedPackets[missingSeq] = recoveredPkt
	block.MissingPackets = nil
	block.Complete = true

	fecRecoveriesTotal.Inc()

	return []*RTPPacketData{recoveredPkt}
}

// tryCompleteBlocks checks if any FEC blocks can now be completed
func (h *FECHandler) tryCompleteBlocks(newSeq uint16) {
	for baseSeq, block := range h.decodingBlocks {
		if block.Complete {
			continue
		}

		// Check if we now have more packets
		newMissing := make([]uint16, 0)
		for _, seq := range block.MissingPackets {
			if pkt, ok := h.receivedPackets[seq]; ok {
				block.ReceivedPackets[seq] = pkt
			} else {
				newMissing = append(newMissing, seq)
			}
		}
		block.MissingPackets = newMissing

		// Try to recover
		if len(newMissing) == 1 {
			h.tryRecoverPackets(block)
		} else if len(newMissing) == 0 {
			block.Complete = true
		}

		// Clean up old blocks
		if seqDistance(baseSeq, newSeq) > 256 {
			delete(h.decodingBlocks, baseSeq)
		}
	}
}

// cleanupOldPackets removes old packets outside the window
func (h *FECHandler) cleanupOldPackets(currentSeq uint16) {
	windowSize := uint16(256)

	for seq := range h.receivedPackets {
		if seqDistance(seq, currentSeq) > windowSize {
			delete(h.receivedPackets, seq)
		}
	}
}

// seqDistance calculates distance between sequence numbers with wrap-around handling
func seqDistance(a, b uint16) uint16 {
	if b >= a {
		return b - a
	}
	// Handle wrap-around: calculate using uint32 to avoid overflow
	return uint16(uint32(65536) - uint32(a) + uint32(b))
}

// UpdateLossRate updates the current packet loss rate for adaptive FEC
func (h *FECHandler) UpdateLossRate(lossRate float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.currentLossRate = lossRate
	h.lossHistory = append(h.lossHistory, lossRate)
	if len(h.lossHistory) > 100 {
		h.lossHistory = h.lossHistory[1:]
	}

	if h.config.AdaptiveMode {
		h.adjustRedundancy()
	}
}

// adjustRedundancy adjusts FEC redundancy based on loss rate
func (h *FECHandler) adjustRedundancy() {
	if len(h.lossHistory) < 10 {
		return
	}

	// Calculate average loss rate
	var avgLoss float64
	for _, loss := range h.lossHistory {
		avgLoss += loss
	}
	avgLoss /= float64(len(h.lossHistory))

	// Adjust redundancy based on loss
	// Higher loss = more redundancy
	targetRedundancy := h.config.MinRedundancy + avgLoss*(h.config.MaxRedundancy-h.config.MinRedundancy)

	// Clamp to min/max
	if targetRedundancy < h.config.MinRedundancy {
		targetRedundancy = h.config.MinRedundancy
	}
	if targetRedundancy > h.config.MaxRedundancy {
		targetRedundancy = h.config.MaxRedundancy
	}

	// Update block size based on redundancy
	// redundancy = 1 / blockSize, so blockSize = 1 / redundancy
	newBlockSize := int(1.0 / targetRedundancy)
	if newBlockSize < 2 {
		newBlockSize = 2
	}
	if newBlockSize > 100 {
		newBlockSize = 100
	}

	h.config.BlockSize = newBlockSize
	h.config.Redundancy = targetRedundancy

	fecRedundancyRatio.Set(targetRedundancy)
}

// GetStats returns FEC statistics
type FECStats struct {
	Enabled            bool
	BlockSize          int
	Redundancy         float64
	PacketsSent        uint64
	RecoveriesTotal    uint64
	RecoveryFailures   uint64
	CurrentLossRate    float64
	PendingBlocks      int
	ReceivedPackets    int
}

// GetStats returns current FEC statistics
func (h *FECHandler) GetStats() FECStats {
	h.mu.Lock()
	defer h.mu.Unlock()

	return FECStats{
		Enabled:         h.config.Enabled,
		BlockSize:       h.config.BlockSize,
		Redundancy:      h.config.Redundancy,
		CurrentLossRate: h.currentLossRate,
		PendingBlocks:   len(h.decodingBlocks),
		ReceivedPackets: len(h.receivedPackets),
	}
}

// SetEnabled enables or disables FEC
func (h *FECHandler) SetEnabled(enabled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.config.Enabled = enabled
}

// SetBlockSize sets the FEC block size
func (h *FECHandler) SetBlockSize(blockSize int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if blockSize < 2 {
		blockSize = 2
	}
	if blockSize > 100 {
		blockSize = 100
	}
	h.config.BlockSize = blockSize
	h.config.Redundancy = 1.0 / float64(blockSize)
	fecRedundancyRatio.Set(h.config.Redundancy)
}

// Reset resets the FEC handler state
func (h *FECHandler) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.encodingBlock = make([]*RTPPacketData, 0, h.config.BlockSize)
	h.decodingBlocks = make(map[uint16]*FECDecodingBlock)
	h.receivedPackets = make(map[uint16]*RTPPacketData)
	h.lossHistory = make([]float64, 0, 100)
	h.currentLossRate = 0
}

// SerializeFECPacket serializes an FEC packet for transmission
func SerializeFECPacket(fec *FECPacket) []byte {
	// FEC packet format:
	// [2 bytes: sequence base]
	// [2 bytes: number of protected packets]
	// [N * 2 bytes: protected sequence numbers]
	// [2 bytes: length XOR]
	// [4 bytes: timestamp XOR]
	// [1 byte: PT/Marker XOR]
	// [2 bytes: payload length]
	// [M bytes: payload XOR]

	numProtected := len(fec.ProtectedSeq)
	headerSize := 2 + 2 + numProtected*2 + 2 + 4 + 1 + 2
	totalSize := headerSize + len(fec.PayloadXOR)

	buf := make([]byte, totalSize)
	offset := 0

	// Sequence base
	binary.BigEndian.PutUint16(buf[offset:], fec.SequenceBase)
	offset += 2

	// Number of protected packets
	binary.BigEndian.PutUint16(buf[offset:], uint16(numProtected))
	offset += 2

	// Protected sequence numbers
	for _, seq := range fec.ProtectedSeq {
		binary.BigEndian.PutUint16(buf[offset:], seq)
		offset += 2
	}

	// Length XOR
	binary.BigEndian.PutUint16(buf[offset:], fec.LengthXOR)
	offset += 2

	// Timestamp XOR
	binary.BigEndian.PutUint32(buf[offset:], fec.TimestampXOR)
	offset += 4

	// PT/Marker XOR
	buf[offset] = fec.PTMarkerXOR
	offset++

	// Payload length
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(fec.PayloadXOR)))
	offset += 2

	// Payload XOR
	copy(buf[offset:], fec.PayloadXOR)

	return buf
}

// DeserializeFECPacket deserializes an FEC packet
func DeserializeFECPacket(data []byte) (*FECPacket, error) {
	if len(data) < 11 {
		return nil, ErrInvalidPacket
	}

	fec := &FECPacket{}
	offset := 0

	// Sequence base
	fec.SequenceBase = binary.BigEndian.Uint16(data[offset:])
	offset += 2

	// Number of protected packets
	numProtected := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2

	if len(data) < offset+numProtected*2+9 {
		return nil, ErrInvalidPacket
	}

	// Protected sequence numbers
	fec.ProtectedSeq = make([]uint16, numProtected)
	for i := 0; i < numProtected; i++ {
		fec.ProtectedSeq[i] = binary.BigEndian.Uint16(data[offset:])
		offset += 2
	}

	// Length XOR
	fec.LengthXOR = binary.BigEndian.Uint16(data[offset:])
	offset += 2

	// Timestamp XOR
	fec.TimestampXOR = binary.BigEndian.Uint32(data[offset:])
	offset += 4

	// PT/Marker XOR
	fec.PTMarkerXOR = data[offset]
	offset++

	// Payload length
	payloadLen := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2

	if len(data) < offset+payloadLen {
		return nil, ErrInvalidPacket
	}

	// Payload XOR
	fec.PayloadXOR = make([]byte, payloadLen)
	copy(fec.PayloadXOR, data[offset:offset+payloadLen])

	return fec, nil
}

// ErrInvalidPacket indicates an invalid packet
var ErrInvalidPacket = &KarlError{Op: "fec", Err: errInvalidFECPacket}

var errInvalidFECPacket = errors.New("invalid FEC packet")
