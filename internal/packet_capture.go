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
	pcapFile   *os.File
	pcapWriter *pcapgo.Writer
)

// InitPCAPCapture initializes packet capture and creates a PCAP file
func InitPCAPCapture() {
	var err error
	pcapFile, err = os.Create("logs/karl_capture.pcap")
	if err != nil {
		log.Fatalf("Failed to create PCAP file: %v", err)
	}

	pcapWriter = pcapgo.NewWriter(pcapFile)
	pcapWriter.WriteFileHeader(65536, layers.LinkTypeEthernet)

	log.Println("Packet capture initialized: logs/karl_capture.pcap")
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
