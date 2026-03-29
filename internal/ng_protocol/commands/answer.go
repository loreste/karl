package commands

import (
	"fmt"
	"log"
	"net"
	"strings"

	"karl/internal"
	ng "karl/internal/ng_protocol"
)

// AnswerHandler handles the answer command
type AnswerHandler struct {
	sessionRegistry *internal.SessionRegistry
	sdpProcessor    *SDPProcessorImpl
	config          *internal.Config
}

// NewAnswerHandler creates a new answer handler
func NewAnswerHandler(registry *internal.SessionRegistry, config *internal.Config) *AnswerHandler {
	return &AnswerHandler{
		sessionRegistry: registry,
		sdpProcessor:    NewSDPProcessor(config),
		config:          config,
	}
}

// Handle processes an answer request
func (h *AnswerHandler) Handle(req *ng.NGRequest) (*ng.NGResponse, error) {
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

	// Find existing session
	session := h.sessionRegistry.GetSessionByTags(req.CallID, req.FromTag, req.ToTag)
	if session == nil {
		// Try finding by call-id and from-tag only
		sessions := h.sessionRegistry.GetSessionByCallID(req.CallID)
		for _, s := range sessions {
			s.Lock()
			if s.FromTag == req.FromTag {
				session = s
				s.Unlock()
				break
			}
			s.Unlock()
		}
	}

	if session == nil {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonNotFound,
		}, nil
	}

	// Process SDP answer
	modifiedSDP, streams, err := h.processAnswerSDP(session, req.SDP, req, flags)
	if err != nil {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: fmt.Sprintf("%s: %v", ng.ErrReasonInvalidSDP, err),
		}, nil
	}

	// Update session state to active
	if err := h.sessionRegistry.UpdateSessionState(session.ID, string(internal.SessionStateActive)); err != nil {
		log.Printf("Warning: failed to update session state: %v", err)
	}

	// Set to-tag if provided
	if req.ToTag != "" {
		session.Lock()
		session.ToTag = req.ToTag
		session.Unlock()
	}

	// Start recording if flagged
	if session.GetFlag("record") {
		log.Printf("Recording requested for session %s", session.ID)
	}

	return &ng.NGResponse{
		Result:  ng.ResultOK,
		SDP:     modifiedSDP,
		Streams: streams,
		CallID:  req.CallID,
		FromTag: req.FromTag,
		ToTag:   req.ToTag,
	}, nil
}

// processAnswerSDP processes the SDP answer and returns modified SDP
func (h *AnswerHandler) processAnswerSDP(session *internal.MediaSession, sdp string, req *ng.NGRequest, flags *ng.ParsedFlags) (string, []ng.StreamInfo, error) {
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

	// Allocate media ports for callee
	rtpPort, rtcpPort, rtpConn, rtcpConn, err := h.sessionRegistry.AllocateMediaPorts(localIP, minPort, maxPort)
	if err != nil {
		return "", nil, fmt.Errorf("failed to allocate media ports: %w", err)
	}

	// Create callee leg
	calleeLeg := &internal.CallLeg{
		Tag:           req.ToTag,
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
		calleeLeg.IP = net.ParseIP(parsedSDP.ConnectionIP)
		calleeLeg.Port = parsedSDP.MediaPort
		calleeLeg.RTCPPort = parsedSDP.MediaPort + 1
		if parsedSDP.RTCPPort > 0 {
			calleeLeg.RTCPPort = parsedSDP.RTCPPort
		}
	}

	// Handle ICE
	if !flags.ICERemove && parsedSDP.HasICE {
		calleeLeg.ICECredentials = &internal.ICECredentials{
			Username: parsedSDP.ICEUfrag,
			Password: parsedSDP.ICEPwd,
			Lite:     parsedSDP.ICELite,
		}
	}

	// Handle SRTP/DTLS
	if parsedSDP.HasDTLS {
		calleeLeg.SRTPParams = &internal.SRTPParameters{
			DTLS:        true,
			Fingerprint: parsedSDP.Fingerprint,
			Setup:       parsedSDP.Setup,
		}
		calleeLeg.Transport = internal.TransportUDPTLSF
	} else if parsedSDP.HasSRTP {
		calleeLeg.SRTPParams = &internal.SRTPParameters{
			CryptoSuite: parsedSDP.CryptoSuite,
		}
		calleeLeg.Transport = internal.TransportRTPS
	}

	// Copy codec info
	calleeLeg.Codecs = make([]internal.CodecInfo, len(parsedSDP.Codecs))
	for i, c := range parsedSDP.Codecs {
		calleeLeg.Codecs[i] = internal.CodecInfo{
			PayloadType: c.PayloadType,
			Name:        c.Name,
			ClockRate:   c.ClockRate,
			Channels:    c.Channels,
			Fmtp:        c.Fmtp,
		}
	}

	// Set callee leg on session
	if err := h.sessionRegistry.SetCalleeLeg(session.ID, calleeLeg); err != nil {
		return "", nil, fmt.Errorf("failed to set callee leg: %w", err)
	}

	// Build modified SDP
	modifiedSDP := h.buildModifiedSDP(parsedSDP, localIP, rtpPort, flags, session)

	// Build stream info for response
	streams := []ng.StreamInfo{
		{
			LocalIP:       localIP,
			LocalPort:     rtpPort,
			LocalRTCPPort: rtcpPort,
			MediaType:     "audio",
			Protocol:      string(calleeLeg.Transport),
			Index:         0,
		},
	}

	// Add ICE info if applicable
	if !flags.ICERemove {
		iceUfrag, icePwd := generateICECredentials()
		streams[0].ICEUfrag = iceUfrag
		streams[0].ICEPwd = icePwd

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
	if calleeLeg.SRTPParams != nil && calleeLeg.SRTPParams.DTLS {
		streams[0].Setup = "active"
		streams[0].Fingerprint = "sha-256 XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX"
		streams[0].FingerprintHash = "sha-256"
	}

	return modifiedSDP, streams, nil
}

// buildModifiedSDP creates the modified SDP for the answer response
func (h *AnswerHandler) buildModifiedSDP(parsed *ParsedSDP, localIP string, localPort int, flags *ng.ParsedFlags, session *internal.MediaSession) string {
	var sb strings.Builder

	// Version
	sb.WriteString("v=0\r\n")

	// Origin
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

	// Direction (typically sendrecv for answer)
	sb.WriteString("a=sendrecv\r\n")

	// RTCP-mux if requested or present in offer
	if flags.RTCPMUX || parsed.RTCPMux {
		sb.WriteString("a=rtcp-mux\r\n")
	}

	// ICE attributes if not removed
	if !flags.ICERemove && parsed.HasICE {
		iceUfrag, icePwd := generateICECredentials()
		sb.WriteString(fmt.Sprintf("a=ice-ufrag:%s\r\n", iceUfrag))
		sb.WriteString(fmt.Sprintf("a=ice-pwd:%s\r\n", icePwd))

		// Add candidate
		sb.WriteString(fmt.Sprintf("a=candidate:1 1 UDP 2130706431 %s %d typ host\r\n",
			localIP, localPort))
	}

	// DTLS fingerprint if present
	if parsed.HasDTLS {
		sb.WriteString(fmt.Sprintf("a=fingerprint:sha-256 %s\r\n", "XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX"))
		sb.WriteString("a=setup:active\r\n") // Answer should be active if offer was actpass
	}

	// SRTP crypto if present
	if parsed.HasSRTP && !parsed.HasDTLS {
		sb.WriteString(fmt.Sprintf("a=crypto:1 %s inline:XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX\r\n",
			parsed.CryptoSuite))
	}

	return sb.String()
}
