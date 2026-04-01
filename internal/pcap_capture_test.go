package internal

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestDefaultPCAPCaptureConfig(t *testing.T) {
	config := DefaultPCAPCaptureConfig()

	if config.MaxPackets != 0 {
		t.Errorf("expected MaxPackets=0 (unlimited), got %d", config.MaxPackets)
	}
	if config.MaxFileSize != 100*1024*1024 {
		t.Errorf("expected MaxFileSize=100MB, got %d", config.MaxFileSize)
	}
	if config.MaxDuration != 0 {
		t.Errorf("expected MaxDuration=0 (unlimited), got %v", config.MaxDuration)
	}
	if config.SnapLen != 65535 {
		t.Errorf("expected SnapLen=65535, got %d", config.SnapLen)
	}
	if config.LinkType != LinkTypeRaw {
		t.Errorf("expected LinkType=LinkTypeRaw, got %d", config.LinkType)
	}
	if config.BufferSize != 1000 {
		t.Errorf("expected BufferSize=1000, got %d", config.BufferSize)
	}
}

func TestNewPCAPCapture(t *testing.T) {
	t.Run("with nil config", func(t *testing.T) {
		capture := NewPCAPCapture(nil)
		if capture == nil {
			t.Fatal("expected non-nil capture")
		}
		if capture.config == nil {
			t.Fatal("expected non-nil config")
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		config := &PCAPCaptureConfig{
			MaxPackets: 100,
			BufferSize: 500,
		}
		capture := NewPCAPCapture(config)
		if capture.config.MaxPackets != 100 {
			t.Errorf("expected MaxPackets=100, got %d", capture.config.MaxPackets)
		}
	})

	t.Run("with zero buffer size", func(t *testing.T) {
		config := &PCAPCaptureConfig{
			BufferSize: 0,
		}
		capture := NewPCAPCapture(config)
		// Should use default buffer size internally
		if capture.packetChan == nil {
			t.Fatal("expected non-nil packet channel")
		}
	})
}

func TestPCAPCapture_StartStop(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.pcap")

	config := &PCAPCaptureConfig{
		OutputPath: outputPath,
		SnapLen:    65535,
		LinkType:   LinkTypeRaw,
		BufferSize: 100,
	}

	capture := NewPCAPCapture(config)

	// Test start
	if err := capture.Start(); err != nil {
		t.Fatalf("failed to start capture: %v", err)
	}

	if !capture.IsRunning() {
		t.Error("expected capture to be running")
	}

	// Test double start
	if err := capture.Start(); err != ErrCaptureAlreadyRunning {
		t.Errorf("expected ErrCaptureAlreadyRunning, got %v", err)
	}

	// Test stop
	if err := capture.Stop(); err != nil {
		t.Fatalf("failed to stop capture: %v", err)
	}

	if capture.IsRunning() {
		t.Error("expected capture to not be running")
	}

	// Verify file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("expected PCAP file to be created")
	}
}

func TestPCAPCapture_StopNotRunning(t *testing.T) {
	capture := NewPCAPCapture(nil)

	if err := capture.Stop(); err != ErrCaptureNotRunning {
		t.Errorf("expected ErrCaptureNotRunning, got %v", err)
	}
}

func TestPCAPCapture_CapturePacket(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.pcap")

	config := &PCAPCaptureConfig{
		OutputPath: outputPath,
		SnapLen:    65535,
		LinkType:   LinkTypeRaw,
		BufferSize: 100,
	}

	capture := NewPCAPCapture(config)

	// Test capture when not running
	packet := &CapturedPacket{
		Data: []byte{0x01, 0x02, 0x03},
	}
	if err := capture.CapturePacket(packet); err != ErrCaptureNotRunning {
		t.Errorf("expected ErrCaptureNotRunning, got %v", err)
	}

	// Start capture
	if err := capture.Start(); err != nil {
		t.Fatalf("failed to start capture: %v", err)
	}
	defer capture.Stop()

	// Capture packets
	for i := 0; i < 10; i++ {
		pkt := &CapturedPacket{
			Timestamp:  time.Now(),
			Data:       []byte{byte(i), 0x02, 0x03, 0x04},
			SrcIP:      "192.168.1.1",
			DstIP:      "192.168.1.2",
			SrcPort:    10000,
			DstPort:    20000,
			Protocol:   "RTP",
		}
		if err := capture.CapturePacket(pkt); err != nil {
			t.Errorf("failed to capture packet %d: %v", i, err)
		}
	}

	// Give time for packets to be written
	time.Sleep(100 * time.Millisecond)

	stats := capture.GetStats()
	if stats.PacketCount < 1 {
		t.Errorf("expected at least 1 packet captured, got %d", stats.PacketCount)
	}
}

func TestPCAPCapture_MaxPacketsLimit(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.pcap")

	config := &PCAPCaptureConfig{
		OutputPath: outputPath,
		MaxPackets: 5,
		SnapLen:    65535,
		LinkType:   LinkTypeRaw,
		BufferSize: 100,
	}

	capture := NewPCAPCapture(config)

	if err := capture.Start(); err != nil {
		t.Fatalf("failed to start capture: %v", err)
	}
	defer capture.Stop()

	// Capture up to limit
	for i := 0; i < 5; i++ {
		pkt := &CapturedPacket{
			Data: []byte{byte(i)},
		}
		capture.CapturePacket(pkt)
	}

	// Give time for processing
	time.Sleep(100 * time.Millisecond)

	// Next packet should hit limit
	pkt := &CapturedPacket{
		Data: []byte{0xFF},
	}
	if err := capture.CapturePacket(pkt); err != ErrMaxPacketsReached {
		t.Errorf("expected ErrMaxPacketsReached, got %v", err)
	}
}

func TestPCAPCapture_Filter(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.pcap")

	config := &PCAPCaptureConfig{
		OutputPath: outputPath,
		SnapLen:    65535,
		LinkType:   LinkTypeRaw,
		BufferSize: 100,
		Filter: func(p *CapturedPacket) bool {
			return p.Protocol == "RTP"
		},
	}

	capture := NewPCAPCapture(config)

	if err := capture.Start(); err != nil {
		t.Fatalf("failed to start capture: %v", err)
	}
	defer capture.Stop()

	// This should be captured (passes filter)
	pkt1 := &CapturedPacket{
		Data:     []byte{0x01},
		Protocol: "RTP",
	}
	capture.CapturePacket(pkt1)

	// This should be filtered out
	pkt2 := &CapturedPacket{
		Data:     []byte{0x02},
		Protocol: "RTCP",
	}
	capture.CapturePacket(pkt2)

	// Give time for processing
	time.Sleep(100 * time.Millisecond)

	stats := capture.GetStats()
	if stats.PacketCount != 1 {
		t.Errorf("expected 1 packet (filtered), got %d", stats.PacketCount)
	}
}

func TestPCAPCapture_GetStats(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.pcap")

	config := &PCAPCaptureConfig{
		OutputPath: outputPath,
		SnapLen:    65535,
		LinkType:   LinkTypeRaw,
		BufferSize: 100,
	}

	capture := NewPCAPCapture(config)

	// Stats before start
	stats := capture.GetStats()
	if stats.Running {
		t.Error("expected Running=false before start")
	}
	if stats.PacketCount != 0 {
		t.Errorf("expected PacketCount=0, got %d", stats.PacketCount)
	}

	// Start and capture
	capture.Start()
	defer capture.Stop()

	stats = capture.GetStats()
	if !stats.Running {
		t.Error("expected Running=true after start")
	}
}

func TestPCAPCapture_ConcurrentCapture(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.pcap")

	config := &PCAPCaptureConfig{
		OutputPath: outputPath,
		SnapLen:    65535,
		LinkType:   LinkTypeRaw,
		BufferSize: 1000,
	}

	capture := NewPCAPCapture(config)

	if err := capture.Start(); err != nil {
		t.Fatalf("failed to start capture: %v", err)
	}

	// Concurrent captures
	var wg sync.WaitGroup
	numGoroutines := 10
	packetsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < packetsPerGoroutine; j++ {
				pkt := &CapturedPacket{
					Data: []byte{byte(id), byte(j)},
				}
				capture.CapturePacket(pkt)
			}
		}(i)
	}

	wg.Wait()
	capture.Stop()

	stats := capture.GetStats()
	// Some packets might be dropped due to buffer, but should have most
	if stats.PacketCount < int64(numGoroutines*packetsPerGoroutine/2) {
		t.Errorf("expected at least %d packets, got %d",
			numGoroutines*packetsPerGoroutine/2, stats.PacketCount)
	}
}

func TestPCAPWriter_WriteHeader(t *testing.T) {
	var buf bytes.Buffer
	w := &pcapFileWriter{
		w:        &buf,
		snapLen:  65535,
		linkType: LinkTypeRaw,
	}

	if err := w.writeHeader(); err != nil {
		t.Fatalf("failed to write header: %v", err)
	}

	// Verify header
	data := buf.Bytes()
	if len(data) != pcapHeaderSize {
		t.Errorf("expected header size %d, got %d", pcapHeaderSize, len(data))
	}

	// Check magic number
	magic := binary.LittleEndian.Uint32(data[0:4])
	if magic != pcapMagicNumber {
		t.Errorf("expected magic %x, got %x", pcapMagicNumber, magic)
	}

	// Check version
	major := binary.LittleEndian.Uint16(data[4:6])
	minor := binary.LittleEndian.Uint16(data[6:8])
	if major != pcapVersionMajor || minor != pcapVersionMinor {
		t.Errorf("expected version %d.%d, got %d.%d",
			pcapVersionMajor, pcapVersionMinor, major, minor)
	}

	// Check snaplen
	snapLen := binary.LittleEndian.Uint32(data[16:20])
	if snapLen != 65535 {
		t.Errorf("expected snapLen 65535, got %d", snapLen)
	}

	// Check link type
	linkType := binary.LittleEndian.Uint32(data[20:24])
	if linkType != uint32(LinkTypeRaw) {
		t.Errorf("expected linkType %d, got %d", LinkTypeRaw, linkType)
	}
}

func TestPCAPWriter_WritePacket(t *testing.T) {
	var buf bytes.Buffer
	w := &pcapFileWriter{
		w:        &buf,
		snapLen:  65535,
		linkType: LinkTypeRaw,
	}

	now := time.Now()
	packet := &CapturedPacket{
		Timestamp: now,
		Data:      []byte{0x01, 0x02, 0x03, 0x04},
	}

	if err := w.writePacket(packet); err != nil {
		t.Fatalf("failed to write packet: %v", err)
	}

	// Verify packet header + data
	data := buf.Bytes()
	expectedSize := pcapPacketHdrSize + len(packet.Data)
	if len(data) != expectedSize {
		t.Errorf("expected size %d, got %d", expectedSize, len(data))
	}

	// Check timestamp seconds
	tsSec := binary.LittleEndian.Uint32(data[0:4])
	if tsSec != uint32(now.Unix()) {
		t.Errorf("expected timestamp %d, got %d", now.Unix(), tsSec)
	}

	// Check capture length
	capLen := binary.LittleEndian.Uint32(data[8:12])
	if capLen != uint32(len(packet.Data)) {
		t.Errorf("expected capLen %d, got %d", len(packet.Data), capLen)
	}

	// Check packet data
	if !bytes.Equal(data[pcapPacketHdrSize:], packet.Data) {
		t.Error("packet data mismatch")
	}
}

func TestPCAPWriter_SnapLength(t *testing.T) {
	var buf bytes.Buffer
	w := &pcapFileWriter{
		w:        &buf,
		snapLen:  4, // Very short snap length
		linkType: LinkTypeRaw,
	}

	packet := &CapturedPacket{
		Timestamp: time.Now(),
		Data:      []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		OrigLen:   8,
	}

	if err := w.writePacket(packet); err != nil {
		t.Fatalf("failed to write packet: %v", err)
	}

	data := buf.Bytes()

	// Check capture length (should be truncated to snapLen)
	capLen := binary.LittleEndian.Uint32(data[8:12])
	if capLen != 4 {
		t.Errorf("expected capLen 4 (truncated), got %d", capLen)
	}

	// Check original length (should be preserved)
	origLen := binary.LittleEndian.Uint32(data[12:16])
	if origLen != 8 {
		t.Errorf("expected origLen 8, got %d", origLen)
	}

	// Only 4 bytes of data should be written
	expectedSize := pcapPacketHdrSize + 4
	if len(data) != expectedSize {
		t.Errorf("expected size %d, got %d", expectedSize, len(data))
	}
}

func TestPCAPCaptureManager(t *testing.T) {
	tmpDir := t.TempDir()

	manager := NewPCAPCaptureManager(tmpDir)

	// Start a capture
	config := &PCAPCaptureConfig{
		SnapLen:    65535,
		LinkType:   LinkTypeRaw,
		BufferSize: 100,
	}

	if err := manager.StartCapture("test1", config); err != nil {
		t.Fatalf("failed to start capture: %v", err)
	}

	// Check it's listed
	names := manager.ListCaptures()
	if len(names) != 1 || names[0] != "test1" {
		t.Errorf("expected [test1], got %v", names)
	}

	// Get capture
	capture := manager.GetCapture("test1")
	if capture == nil {
		t.Fatal("expected non-nil capture")
	}
	if !capture.IsRunning() {
		t.Error("expected capture to be running")
	}

	// Try to start duplicate
	if err := manager.StartCapture("test1", nil); err == nil {
		t.Error("expected error for duplicate capture")
	}

	// Stop capture
	if err := manager.StopCapture("test1"); err != nil {
		t.Fatalf("failed to stop capture: %v", err)
	}

	// Should be removed from list
	names = manager.ListCaptures()
	if len(names) != 0 {
		t.Errorf("expected empty list, got %v", names)
	}

	// Stop non-existent
	if err := manager.StopCapture("nonexistent"); err == nil {
		t.Error("expected error for non-existent capture")
	}
}

func TestPCAPCaptureManager_StopAll(t *testing.T) {
	tmpDir := t.TempDir()

	manager := NewPCAPCaptureManager(tmpDir)

	// Start multiple captures
	for i := 0; i < 3; i++ {
		config := &PCAPCaptureConfig{
			SnapLen:    65535,
			LinkType:   LinkTypeRaw,
			BufferSize: 100,
		}
		name := string(rune('a' + i))
		if err := manager.StartCapture(name, config); err != nil {
			t.Fatalf("failed to start capture %s: %v", name, err)
		}
	}

	if len(manager.ListCaptures()) != 3 {
		t.Errorf("expected 3 captures, got %d", len(manager.ListCaptures()))
	}

	// Stop all
	manager.StopAll()

	if len(manager.ListCaptures()) != 0 {
		t.Errorf("expected 0 captures after StopAll, got %d", len(manager.ListCaptures()))
	}
}

func TestCallPCAPCapture(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "call.pcap")

	callCapture, err := NewCallPCAPCapture("call-123", outputPath)
	if err != nil {
		t.Fatalf("failed to create call capture: %v", err)
	}

	if err := callCapture.Start(); err != nil {
		t.Fatalf("failed to start call capture: %v", err)
	}

	// Capture packet
	pkt := &CapturedPacket{
		Data:     []byte{0x01, 0x02, 0x03},
		Protocol: "RTP",
	}
	if err := callCapture.CapturePacket(pkt); err != nil {
		t.Errorf("failed to capture packet: %v", err)
	}

	// Packet should have CallID set
	if pkt.CallID != "call-123" {
		t.Errorf("expected CallID=call-123, got %s", pkt.CallID)
	}

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	stats := callCapture.GetStats()
	if stats.PacketCount != 1 {
		t.Errorf("expected 1 packet, got %d", stats.PacketCount)
	}

	if err := callCapture.Stop(); err != nil {
		t.Fatalf("failed to stop call capture: %v", err)
	}
}

func TestCreateRTPPacketCapture(t *testing.T) {
	data := []byte{0x80, 0x00, 0x01, 0x02}
	packet := CreateRTPPacketCapture(data, "192.168.1.1", "192.168.1.2", 10000, 20000, "inbound")

	if packet.Protocol != "RTP" {
		t.Errorf("expected Protocol=RTP, got %s", packet.Protocol)
	}
	if packet.SrcIP != "192.168.1.1" {
		t.Errorf("expected SrcIP=192.168.1.1, got %s", packet.SrcIP)
	}
	if packet.DstIP != "192.168.1.2" {
		t.Errorf("expected DstIP=192.168.1.2, got %s", packet.DstIP)
	}
	if packet.SrcPort != 10000 {
		t.Errorf("expected SrcPort=10000, got %d", packet.SrcPort)
	}
	if packet.DstPort != 20000 {
		t.Errorf("expected DstPort=20000, got %d", packet.DstPort)
	}
	if packet.Direction != "inbound" {
		t.Errorf("expected Direction=inbound, got %s", packet.Direction)
	}
	if !bytes.Equal(packet.Data, data) {
		t.Error("data mismatch")
	}
	if packet.CaptureLen != uint32(len(data)) {
		t.Errorf("expected CaptureLen=%d, got %d", len(data), packet.CaptureLen)
	}
	if packet.OrigLen != uint32(len(data)) {
		t.Errorf("expected OrigLen=%d, got %d", len(data), packet.OrigLen)
	}
	if packet.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestPCAPCapture_CreateDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "a", "b", "c", "test.pcap")

	config := &PCAPCaptureConfig{
		OutputPath: nestedPath,
		SnapLen:    65535,
		LinkType:   LinkTypeRaw,
		BufferSize: 100,
	}

	capture := NewPCAPCapture(config)

	if err := capture.Start(); err != nil {
		t.Fatalf("failed to start capture: %v", err)
	}
	capture.Stop()

	// Check directory was created
	dir := filepath.Dir(nestedPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("expected directory to be created")
	}
}

func TestLinkTypes(t *testing.T) {
	// Verify link type constants
	if LinkTypeNull != 0 {
		t.Errorf("expected LinkTypeNull=0, got %d", LinkTypeNull)
	}
	if LinkTypeEthernet != 1 {
		t.Errorf("expected LinkTypeEthernet=1, got %d", LinkTypeEthernet)
	}
	if LinkTypeRaw != 101 {
		t.Errorf("expected LinkTypeRaw=101, got %d", LinkTypeRaw)
	}
	if LinkTypeLinuxSLL != 113 {
		t.Errorf("expected LinkTypeLinuxSLL=113, got %d", LinkTypeLinuxSLL)
	}
}

func TestPCAPCapture_ValidPCAPFile(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.pcap")

	config := &PCAPCaptureConfig{
		OutputPath: outputPath,
		SnapLen:    65535,
		LinkType:   LinkTypeRaw,
		BufferSize: 100,
	}

	capture := NewPCAPCapture(config)
	capture.Start()

	// Write some packets
	for i := 0; i < 5; i++ {
		pkt := &CapturedPacket{
			Timestamp: time.Now(),
			Data:      []byte{byte(i), 0x02, 0x03, 0x04},
		}
		capture.CapturePacket(pkt)
	}

	time.Sleep(100 * time.Millisecond)
	capture.Stop()

	// Read and validate PCAP file
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read PCAP file: %v", err)
	}

	if len(data) < pcapHeaderSize {
		t.Fatalf("PCAP file too small: %d bytes", len(data))
	}

	// Validate header
	magic := binary.LittleEndian.Uint32(data[0:4])
	if magic != pcapMagicNumber {
		t.Errorf("invalid magic number: %x", magic)
	}

	// Count packets in file
	offset := pcapHeaderSize
	packetCount := 0
	for offset < len(data) {
		if offset+pcapPacketHdrSize > len(data) {
			break
		}
		capLen := binary.LittleEndian.Uint32(data[offset+8 : offset+12])
		offset += pcapPacketHdrSize + int(capLen)
		packetCount++
	}

	if packetCount < 1 {
		t.Errorf("expected at least 1 packet in file, got %d", packetCount)
	}
}
