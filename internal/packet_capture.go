package internal

import (
	"log"
	"os"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
)

// Global variables for PCAP file handling
var (
	pcapFile     *os.File
	pcapWriter   *pcapgo.Writer
	pcapEnabled  bool
)

// InitPCAPCapture initializes packet capture and creates a PCAP file
func InitPCAPCapture() {
	var err error
	
	// Ensure logs directory exists
	if err := os.MkdirAll("logs", 0755); err != nil {
		log.Printf("Failed to create logs directory: %v", err)
		return
	}
	
	pcapFile, err = os.Create("logs/karl_capture.pcap")
	if err != nil {
		log.Printf("Failed to create PCAP file: %v", err)
		return
	}

	pcapWriter = pcapgo.NewWriter(pcapFile)
	if err := pcapWriter.WriteFileHeader(65536, layers.LinkTypeEthernet); err != nil {
		log.Printf("Failed to write PCAP header: %v", err)
		pcapFile.Close()
		pcapFile = nil
		pcapWriter = nil
		return
	}

	pcapEnabled = true
	log.Println("Packet capture initialized: logs/karl_capture.pcap")
}

// IsPCAPEnabled returns whether packet capture is enabled
func IsPCAPEnabled() bool {
	return pcapEnabled
}

// SetPCAPEnabled enables or disables packet capture
func SetPCAPEnabled(enabled bool) {
	// If turning on and not already initialized
	if enabled && !pcapEnabled && pcapWriter == nil {
		InitPCAPCapture()
	} else if !enabled && pcapEnabled {
		ClosePCAPCapture()
		pcapEnabled = false
	}
}

// CapturePacket writes an RTP packet to the PCAP file
func CapturePacket(packet []byte) {
	if pcapWriter == nil {
		return
	}

	pcapWriter.WritePacket(gopacket.CaptureInfo{
		Timestamp:     time.Now(),
		CaptureLength: len(packet),
		Length:        len(packet),
	}, packet)
}

// ClosePCAPCapture properly closes the PCAP file
func ClosePCAPCapture() {
	if pcapFile != nil {
		pcapFile.Close()
		log.Println("PCAP capture file closed.")
	}
}
