package commands

import (
	"fmt"
	"log"
	"net"
	"strings"

	"karl/internal"
	ng "karl/internal/ng_protocol"
)

// OfferHandler handles the offer command
type OfferHandler struct {
	sessionRegistry *internal.SessionRegistry
	sdpProcessor    *SDPProcessorImpl
	config          *internal.Config
}

// NewOfferHandler creates a new offer handler
func NewOfferHandler(registry *internal.SessionRegistry, config *internal.Config) *OfferHandler {
	return &OfferHandler{
		sessionRegistry: registry,
		sdpProcessor:    NewSDPProcessor(config),
		config:          config,
	}
}

// Handle processes an offer request
func (h *OfferHandler) Handle(req *ng.NGRequest) (*ng.NGResponse, error) {
	// Validate required parameters
	if req.CallID == "" {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonMissingParam + ": call-id",
		}, nil
	}
	if req.FromTag == "" {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonMissingParam + ": from-tag",
		}, nil
	}
	if req.SDP == "" {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonMissingParam + ": sdp",
		}, nil
	}

	// Parse flags
	flags := ng.ParseFlags(req.Flags)

	// Check if session already exists
	session := h.sessionRegistry.GetSessionByTags(req.CallID, req.FromTag, req.ToTag)
	if session == nil {
		// Create new session
		session = h.sessionRegistry.CreateSession(req.CallID, req.FromTag)
		log.Printf("Created new session %s for call-id %s", session.ID, req.CallID)
	}

	// Update session state
	_ = h.sessionRegistry.UpdateSessionState(session.ID, string(internal.SessionStatePending))

	// Process SDP offer
	modifiedSDP, streams, err := h.processOfferSDP(session, req.SDP, req, flags)
	if err != nil {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: fmt.Sprintf("%s: %v", ng.ErrReasonInvalidSDP, err),
		}, nil
	}

	// Handle recording flag
	if flags.RecordCall {
		session.SetFlag("record", true)
		session.SetMetadata("record-start", "pending")
	}

	// Handle WebRTC flag
	if flags.WebRTCEnabled {
		session.SetFlag("webrtc", true)
	}

	return &ng.NGResponse{
		Result:  ng.ResultOK,
		SDP:     modifiedSDP,
		Streams: streams,
		CallID:  req.CallID,
		FromTag: req.FromTag,
	}, nil
}

// processOfferSDP processes the SDP offer and returns modified SDP
func (h *OfferHandler) processOfferSDP(session *internal.MediaSession, sdp string, req *ng.NGRequest, flags *ng.ParsedFlags) (string, []ng.StreamInfo, error) {
	// Parse the SDP
	parsedSDP, err := h.sdpProcessor.Parse(sdp)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse SDP: %w", err)
	}

	// Determine local media IP
	localIP := h.config.Integration.MediaIP
	if localIP == "" {
		localIP = "127.0.0.1"
	}

	// Determine port range
	minPort := 30000
	maxPort := 40000
	// Port range can be configured in sessions config if needed

	// Allocate media ports
	rtpPort, rtcpPort, rtpConn, rtcpConn, err := h.sessionRegistry.AllocateMediaPorts(localIP, minPort, maxPort)
	if err != nil {
		return "", nil, fmt.Errorf("failed to allocate media ports: %w", err)
	}

	// Create caller leg
	callerLeg := &internal.CallLeg{
		Tag:           req.FromTag,
		LocalIP:       net.ParseIP(localIP),
		LocalPort:     rtpPort,
		LocalRTCPPort: rtcpPort,
		Conn:          rtpConn,
		RTCPConn:      rtcpConn,
		MediaType:     internal.MediaAudio,
		Transport:     internal.TransportRTP,
	}

	// Extract remote address from SDP
	if parsedSDP.ConnectionIP != "" && parsedSDP.MediaPort > 0 {
		callerLeg.IP = net.ParseIP(parsedSDP.ConnectionIP)
		callerLeg.Port = parsedSDP.MediaPort
		callerLeg.RTCPPort = parsedSDP.MediaPort + 1
	}

	// Handle ICE
	if !flags.ICERemove && parsedSDP.HasICE {
		callerLeg.ICECredentials = &internal.ICECredentials{
			Username: parsedSDP.ICEUfrag,
			Password: parsedSDP.ICEPwd,
			Lite:     flags.ICELite,
		}
	}

	// Handle SRTP/DTLS
	if parsedSDP.HasDTLS {
		callerLeg.SRTPParams = &internal.SRTPParameters{
			DTLS:        true,
			Fingerprint: parsedSDP.Fingerprint,
			Setup:       parsedSDP.Setup,
		}
		callerLeg.Transport = internal.TransportUDPTLSF
	} else if parsedSDP.HasSRTP {
		callerLeg.SRTPParams = &internal.SRTPParameters{
			CryptoSuite: parsedSDP.CryptoSuite,
		}
		callerLeg.Transport = internal.TransportRTPS
	}

	// Copy codec info
	callerLeg.Codecs = make([]internal.CodecInfo, len(parsedSDP.Codecs))
	for i, c := range parsedSDP.Codecs {
		callerLeg.Codecs[i] = internal.CodecInfo{
			PayloadType: c.PayloadType,
			Name:        c.Name,
			ClockRate:   c.ClockRate,
			Channels:    c.Channels,
			Fmtp:        c.Fmtp,
		}
	}

	// Set caller leg on session
	if err := h.sessionRegistry.SetCallerLeg(session.ID, callerLeg); err != nil {
		return "", nil, fmt.Errorf("failed to set caller leg: %w", err)
	}

	// Build modified SDP
	modifiedSDP := h.buildModifiedSDP(parsedSDP, localIP, rtpPort, flags)

	// Build stream info for response
	streams := []ng.StreamInfo{
		{
			LocalIP:       localIP,
			LocalPort:     rtpPort,
			LocalRTCPPort: rtcpPort,
			MediaType:     "audio",
			Protocol:      string(callerLeg.Transport),
			Index:         0,
		},
	}

	// Add ICE info if applicable
	if !flags.ICERemove {
		iceUfrag, icePwd := generateICECredentials()
		streams[0].ICEUfrag = iceUfrag
		streams[0].ICEPwd = icePwd

		// Add ICE candidates
		streams[0].ICECandidates = []ng.ICECandidate{
			{
				Foundation: "1",
				Component:  1,
				Protocol:   "UDP",
				Priority:   2130706431,
				IP:         localIP,
				Port:       rtpPort,
				Type:       "host",
			},
		}
	}

	// Add DTLS info if applicable
	if callerLeg.SRTPParams != nil && callerLeg.SRTPParams.DTLS {
		streams[0].Setup = "actpass"
		streams[0].Fingerprint = "sha-256 XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX"
		streams[0].FingerprintHash = "sha-256"
	}

	return modifiedSDP, streams, nil
}

// buildModifiedSDP creates the modified SDP for the offer response
func (h *OfferHandler) buildModifiedSDP(parsed *ParsedSDP, localIP string, localPort int, flags *ng.ParsedFlags) string {
	var sb strings.Builder

	// Version
	sb.WriteString("v=0\r\n")

	// Origin (use parsed session ID/version or generate new)
	sb.WriteString(fmt.Sprintf("o=karl %d %d IN IP4 %s\r\n",
		parsed.SessionID, parsed.SessionVersion+1, localIP))

	// Session name
	sb.WriteString("s=Karl Media Server\r\n")

	// Connection
	sb.WriteString(fmt.Sprintf("c=IN IP4 %s\r\n", localIP))

	// Timing
	sb.WriteString("t=0 0\r\n")

	// Media line
	protocol := "RTP/AVP"
	if parsed.HasDTLS {
		protocol = "UDP/TLS/RTP/SAVPF"
	} else if parsed.HasSRTP {
		protocol = "RTP/SAVP"
	}

	// Build payload type list
	payloadTypes := make([]string, len(parsed.Codecs))
	for i, c := range parsed.Codecs {
		payloadTypes[i] = fmt.Sprintf("%d", c.PayloadType)
	}
	sb.WriteString(fmt.Sprintf("m=audio %d %s %s\r\n",
		localPort, protocol, strings.Join(payloadTypes, " ")))

	// Add rtpmap and fmtp for each codec
	for _, c := range parsed.Codecs {
		if c.Channels > 1 {
			sb.WriteString(fmt.Sprintf("a=rtpmap:%d %s/%d/%d\r\n",
				c.PayloadType, c.Name, c.ClockRate, c.Channels))
		} else {
			sb.WriteString(fmt.Sprintf("a=rtpmap:%d %s/%d\r\n",
				c.PayloadType, c.Name, c.ClockRate))
		}
		if c.Fmtp != "" {
			sb.WriteString(fmt.Sprintf("a=fmtp:%d %s\r\n", c.PayloadType, c.Fmtp))
		}
	}

	// Direction
	sb.WriteString("a=sendrecv\r\n")

	// RTCP-mux if requested
	if flags.RTCPMUX || parsed.RTCPMux {
		sb.WriteString("a=rtcp-mux\r\n")
	}

	// ICE attributes if not removed
	if !flags.ICERemove && parsed.HasICE {
		iceUfrag, icePwd := generateICECredentials()
		sb.WriteString(fmt.Sprintf("a=ice-ufrag:%s\r\n", iceUfrag))
		sb.WriteString(fmt.Sprintf("a=ice-pwd:%s\r\n", icePwd))
		if flags.ICELite {
			sb.WriteString("a=ice-lite\r\n")
		}

		// Add candidate
		sb.WriteString(fmt.Sprintf("a=candidate:1 1 UDP 2130706431 %s %d typ host\r\n",
			localIP, localPort))
	}

	// DTLS fingerprint if present
	if parsed.HasDTLS {
		sb.WriteString(fmt.Sprintf("a=fingerprint:sha-256 %s\r\n", "XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX"))
		sb.WriteString("a=setup:actpass\r\n")
	}

	// SRTP crypto if present
	if parsed.HasSRTP && !parsed.HasDTLS {
		sb.WriteString(fmt.Sprintf("a=crypto:1 %s inline:XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX\r\n",
			parsed.CryptoSuite))
	}

	return sb.String()
}

// generateICECredentials generates ICE credentials
func generateICECredentials() (ufrag, pwd string) {
	// In production, use crypto/rand
	ufrag = "karl" + fmt.Sprintf("%08x", uint32(1234567890))
	pwd = "karlpass" + fmt.Sprintf("%016x", uint64(9876543210123456789))
	return
}
