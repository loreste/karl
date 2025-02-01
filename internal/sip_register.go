package internal

import (
	"fmt"
	"log"
	"net"
	"time"
)

// RegisterWithSIPProxy registers Karl as an RTP media relay with OpenSIPS/Kamailio
func RegisterWithSIPProxy(proxyIP string, proxyPort int) {
	proxyAddr := fmt.Sprintf("%s:%d", proxyIP, proxyPort)

	// Create a UDP connection to the SIP proxy
	conn, err := net.Dial("udp", proxyAddr)
	if err != nil {
		log.Printf("Failed to register with SIP proxy %s: %v", proxyAddr, err)
		return
	}
	defer conn.Close()

	// Send a registration message
	registrationMessage := "REGISTER Karl RTP Engine"
	_, err = conn.Write([]byte(registrationMessage))
	if err != nil {
		log.Printf("Failed to send registration to SIP proxy: %v", err)
		return
	}

	log.Printf("Successfully registered Karl with SIP proxy at %s", proxyAddr)
}

// PeriodicallyRegisterWithSIPProxy ensures Karl remains registered with OpenSIPS/Kamailio
func PeriodicallyRegisterWithSIPProxy(proxyIP string, proxyPort int, interval time.Duration) {
	for {
		RegisterWithSIPProxy(proxyIP, proxyPort)
		time.Sleep(interval)
	}
}
