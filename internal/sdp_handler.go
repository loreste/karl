package internal

import (
	"fmt"
	"log"
	"net"
	"strings"
)

// ModifySDP ensures the correct IP (Public or Private) is sent in SDP
func ModifySDP(sdp string, remoteIP string) string {
	configMutex.RLock()
	publicIP := config.Integration.PublicIP
	privateIP := config.Integration.MediaIP
	configMutex.RUnlock()

	// Determine if the call is internal or external
	isExternal := IsExternalCall(remoteIP)

	// Select appropriate media IP
	mediaIP := privateIP
	if isExternal {
		mediaIP = publicIP
	}

	log.Printf("Modifying SDP, setting c=IN IP4 %s", mediaIP)

	// Replace c=IN IP4 line with correct media IP
	lines := strings.Split(sdp, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "c=IN IP4") {
			lines[i] = fmt.Sprintf("c=IN IP4 %s", mediaIP)
		}

		// Ensure proper SRTP or RTP selection
		if strings.HasPrefix(line, "a=crypto") && !config.RTPSettings.Encryption {
			lines[i] = "; Removed a=crypto (SRTP disabled)"
		}
	}

	return strings.Join(lines, "\n")
}

// IsExternalCall detects whether a remote IP is internal or external
func IsExternalCall(remoteIP string) bool {
	privateRanges := []string{
		"10.", "192.168.", "172.16.", "172.17.", "172.18.", "172.19.",
		"172.20.", "172.21.", "172.22.", "172.23.", "172.24.",
		"172.25.", "172.26.", "172.27.", "172.28.", "172.29.",
		"172.30.", "172.31.",
	}

	for _, prefix := range privateRanges {
		if strings.HasPrefix(remoteIP, prefix) {
			return false // Internal call
		}
	}
	return true // External call, use Public IP
}

// ExtractSDPValues parses SDP for key media parameters
func ExtractSDPValues(sdp string) map[string]string {
	values := make(map[string]string)
	lines := strings.Split(sdp, "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, "c=IN IP4") {
			values["connection_ip"] = strings.TrimSpace(strings.Split(line, " ")[2])
		} else if strings.HasPrefix(line, "a=fingerprint:") {
			values["dtls_fingerprint"] = strings.TrimSpace(strings.SplitN(line, " ", 2)[1])
		} else if strings.HasPrefix(line, "a=setup:") {
			values["dtls_setup"] = strings.TrimSpace(strings.Split(line, ":")[1])
		} else if strings.HasPrefix(line, "a=crypto:") {
			values["srtp_crypto"] = strings.TrimSpace(strings.SplitN(line, " ", 2)[1])
		} else if strings.HasPrefix(line, "m=audio") {
			values["audio_port"] = strings.Fields(line)[1]
		} else if strings.HasPrefix(line, "m=video") {
			values["video_port"] = strings.Fields(line)[1]
		}
	}

	return values
}

// EnsureSDPCompatibility adapts SDP for WebRTC-SIP interop
func EnsureSDPCompatibility(sdp string, isWebRTC bool) string {
	lines := strings.Split(sdp, "\n")

	for i, line := range lines {
		// WebRTC uses RTP/SAVPF while SIP uses RTP/AVP
		if strings.HasPrefix(line, "m=audio") || strings.HasPrefix(line, "m=video") {
			if isWebRTC {
				lines[i] = strings.Replace(lines[i], "RTP/AVP", "RTP/SAVPF", 1)
			} else {
				lines[i] = strings.Replace(lines[i], "RTP/SAVPF", "RTP/AVP", 1)
			}
		}

		// WebRTC uses DTLS-SRTP, SIP might use SDES-SRTP or RTP
		if strings.HasPrefix(line, "a=setup:actpass") && !isWebRTC {
			lines[i] = "a=setup:passive" // SIP acts as passive DTLS endpoint
		}

		// Ensure correct handling of a=fingerprint (WebRTC only)
		if strings.HasPrefix(line, "a=fingerprint") && !isWebRTC {
			lines[i] = "; Removed DTLS fingerprint for SIP"
		}
	}

	return strings.Join(lines, "\n")
}

// GetLocalIP detects the correct local IP address
func GetLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("unable to determine local IP")
}
