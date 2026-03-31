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
	isNewSession := session == nil
	if isNewSession {
		// Create new session
		session = h.sessionRegistry.CreateSession(req.CallID, req.FromTag)
		log.Printf("Created new session %s for call-id %s", session.ID, req.CallID)
	}

	// Apply session-level flags
	h.applySessionFlags(session, flags)

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
	if flags.RecordCall || flags.StartRecording {
		session.SetFlag("record", true)
		session.SetMetadata("record-start", "pending")
		if flags.RecordingFile != "" {
			session.SetMetadata("recording-file", flags.RecordingFile)
		}
		if flags.RecordingPath != "" {
			session.SetMetadata("recording-path", flags.RecordingPath)
		}
	}

	// Handle WebRTC flag
	if flags.WebRTCEnabled {
		session.SetFlag("webrtc", true)
	}

	// Handle SIPREC metadata
	if flags.SIPREC {
		for k, v := range flags.RecordingMetadata {
			session.SetMetadata("siprec-"+k, v)
		}
	}

	// Store via-branch if provided
	if flags.ViaBranch != "" || req.ViaBranch != "" {
		viaBranch := flags.ViaBranch
		if viaBranch == "" {
			viaBranch = req.ViaBranch
		}
		session.Lock()
		session.ViaBranch = viaBranch
		session.Unlock()
	}

	return &ng.NGResponse{
		Result:  ng.ResultOK,
		SDP:     modifiedSDP,
		Streams: streams,
		CallID:  req.CallID,
		FromTag: req.FromTag,
	}, nil
}

// applySessionFlags applies parsed flags to the session
func (h *OfferHandler) applySessionFlags(session *internal.MediaSession, flags *ng.ParsedFlags) {
	session.ApplySessionFlags(
		flags.TOS,
		flags.MediaTimeout,
		flags.DeleteDelay,
		flags.SIPREC,
		flags.T38Support,
		flags.T38Gateway,
		flags.ICELite,
		flags.TrickleICE,
		flags.ICEForce,
		flags.ICERemove,
		flags.DTLSOff,
		flags.DTLSPassive,
		flags.DTLSActive,
		flags.SDESOff,
		flags.SDESOnly,
		flags.LoopProtect,
		flags.AlwaysTranscode,
	)

	// Store transcode codecs
	if len(flags.TranscodeCodecs) > 0 {
		session.Lock()
		session.TranscodeCodecs = flags.TranscodeCodecs
		session.Unlock()
	}
}

// processOfferSDP processes the SDP offer and returns modified SDP
func (h *OfferHandler) processOfferSDP(session *internal.MediaSession, sdp string, req *ng.NGRequest, flags *ng.ParsedFlags) (string, []ng.StreamInfo, error) {
	// Parse the SDP
	parsedSDP, err := h.sdpProcessor.Parse(sdp)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse SDP: %w", err)
	}

	// Determine local media IP based on interface selection
	localIP := h.selectMediaIP(flags, req)

	// Determine port range from config
	minPort := 30000
	maxPort := 40000
	if h.config.Sessions != nil && h.config.Sessions.MinPort > 0 {
		minPort = h.config.Sessions.MinPort
	}
	if h.config.Sessions != nil && h.config.Sessions.MaxPort > 0 {
		maxPort = h.config.Sessions.MaxPort
	}

	// Allocate media ports
	rtpPort, rtcpPort, rtpConn, rtcpConn, err := h.sessionRegistry.AllocateMediaPorts(localIP, minPort, maxPort)
	if err != nil {
		return "", nil, fmt.Errorf("failed to allocate media ports: %w", err)
	}

	// Determine label for this leg
	legLabel := flags.Label
	if legLabel == "" && flags.SetLabel != "" {
		legLabel = flags.SetLabel
	}
	if legLabel == "" {
		legLabel = req.FromTag // Default to tag as label
	}

	// Create caller leg with rtpengine-compatible fields
	callerLeg := &internal.CallLeg{
		Tag:           req.FromTag,
		Label:         legLabel,
		LocalIP:       net.ParseIP(localIP),
		LocalPort:     rtpPort,
		LocalRTCPPort: rtcpPort,
		Conn:          rtpConn,
		RTCPConn:      rtcpConn,
		MediaType:     internal.MediaAudio,
		Transport:     internal.TransportRTP,

		// rtpengine flags
		Interface:     flags.Interface,
		AddressFamily: flags.AddressFamily,
		Symmetric:     flags.Symmetric,
		StrictSource:  flags.StrictSource,
		MediaHandover: flags.MediaHandover,
		PortLatching:  flags.PortLatching && !flags.NoPortLatching,
		MediaBlocked:  flags.BlockMedia,
		DTMFBlocked:   flags.BlockDTMF,
		Silenced:      flags.SilenceMedia,
		T38Enabled:    flags.T38Support,
		T38Gateway:    flags.T38Gateway,
	}

	// Set direction on leg
	callerLeg.Direction = h.determineDirection(flags, parsedSDP)

	// Extract remote address from SDP
	if parsedSDP.ConnectionIP != "" && parsedSDP.MediaPort > 0 {
		callerLeg.IP = net.ParseIP(parsedSDP.ConnectionIP)
		callerLeg.Port = parsedSDP.MediaPort
		callerLeg.RTCPPort = parsedSDP.MediaPort + 1
	}

	// Handle ICE based on flags
	h.handleICE(callerLeg, parsedSDP, flags)

	// Handle SRTP/DTLS based on flags
	h.handleSRTPDTLS(callerLeg, parsedSDP, flags)

	// Process codecs with filtering
	callerLeg.Codecs = h.processCodecs(parsedSDP.Codecs, flags)

	// Set caller leg on session
	if err := h.sessionRegistry.SetCallerLeg(session.ID, callerLeg); err != nil {
		return "", nil, fmt.Errorf("failed to set caller leg: %w", err)
	}

	// Also store by label if set
	if legLabel != "" {
		session.SetLegByLabel(legLabel, callerLeg)
	}

	// Build modified SDP
	modifiedSDP := h.buildModifiedSDP(parsedSDP, localIP, rtpPort, flags)

	// Build stream info for response
	streams := h.buildStreamInfo(callerLeg, localIP, rtpPort, rtcpPort, flags, parsedSDP)

	return modifiedSDP, streams, nil
}

// selectMediaIP selects the media IP based on flags and interface configuration
func (h *OfferHandler) selectMediaIP(flags *ng.ParsedFlags, req *ng.NGRequest) string {
	// First check for explicit media-address flag
	if flags.MediaAddress != "" {
		return flags.MediaAddress
	}

	// Check interface-based selection
	if flags.Interface != "" || flags.FromInterface != "" {
		iface := flags.Interface
		if flags.FromInterface != "" {
			iface = flags.FromInterface
		}

		// Try to match interface from config
		if h.config.Integration.Interfaces != nil {
			if ifaceConfig, ok := h.config.Integration.Interfaces[iface]; ok {
				if ifaceConfig.Address != "" {
					return ifaceConfig.Address
				}
			}
		}
	}

	// Use received-from if trust-address is set and SIP-source-address flag
	if flags.TrustAddress && flags.SIPSourceAddress && req.ReceivedFrom != nil {
		return req.ReceivedFrom.IP.String()
	}

	// Fall back to config
	if h.config.Integration.MediaIP != "" {
		return h.config.Integration.MediaIP
	}

	return "127.0.0.1"
}

// determineDirection determines the media direction based on flags and SDP
func (h *OfferHandler) determineDirection(flags *ng.ParsedFlags, parsedSDP *ParsedSDP) string {
	if flags.Inactive {
		return "inactive"
	}
	if flags.SendOnly {
		return "sendonly"
	}
	if flags.RecvOnly {
		return "recvonly"
	}
	if flags.OriginalSendrecv {
		return parsedSDP.Direction
	}
	return "sendrecv"
}

// handleICE configures ICE on the leg based on flags
func (h *OfferHandler) handleICE(leg *internal.CallLeg, parsedSDP *ParsedSDP, flags *ng.ParsedFlags) {
	// ICE removal
	if flags.ICERemove {
		leg.ICECredentials = nil
		return
	}

	// ICE force - add ICE even if not in original SDP
	if flags.ICEForce || parsedSDP.HasICE {
		leg.ICECredentials = &internal.ICECredentials{
			Username: parsedSDP.ICEUfrag,
			Password: parsedSDP.ICEPwd,
			Lite:     flags.ICELite || parsedSDP.ICELite,
		}
	}
}

// handleSRTPDTLS configures SRTP/DTLS on the leg based on flags
func (h *OfferHandler) handleSRTPDTLS(leg *internal.CallLeg, parsedSDP *ParsedSDP, flags *ng.ParsedFlags) {
	// Handle DTLS
	if flags.DTLSOff {
		// DTLS explicitly disabled, check for SDES
		if !flags.SDESOff && parsedSDP.HasSRTP {
			leg.SRTPParams = &internal.SRTPParameters{
				CryptoSuite: parsedSDP.CryptoSuite,
			}
			leg.Transport = internal.TransportRTPS
		}
		return
	}

	// DTLS enabled
	if parsedSDP.HasDTLS {
		setup := parsedSDP.Setup
		if flags.DTLSPassive {
			setup = "passive"
		} else if flags.DTLSActive {
			setup = "active"
		} else if flags.DTLSReverse {
			// Reverse the DTLS role
			switch parsedSDP.Setup {
			case "active":
				setup = "passive"
			case "passive":
				setup = "active"
			}
		}

		leg.SRTPParams = &internal.SRTPParameters{
			DTLS:        true,
			Fingerprint: parsedSDP.Fingerprint,
			Setup:       setup,
		}
		leg.Transport = internal.TransportUDPTLSF
		return
	}

	// SDES handling
	if flags.SDESOnly {
		// Force SDES even without DTLS
		if parsedSDP.HasSRTP {
			leg.SRTPParams = &internal.SRTPParameters{
				CryptoSuite: parsedSDP.CryptoSuite,
			}
			leg.Transport = internal.TransportRTPS
		}
		return
	}

	// Default: use what's in SDP
	if parsedSDP.HasSRTP && !flags.SDESOff {
		leg.SRTPParams = &internal.SRTPParameters{
			CryptoSuite: parsedSDP.CryptoSuite,
		}
		leg.Transport = internal.TransportRTPS
	}
}

// processCodecs filters and processes codecs based on flags
func (h *OfferHandler) processCodecs(codecs []CodecInfo, flags *ng.ParsedFlags) []internal.CodecInfo {
	result := make([]internal.CodecInfo, 0, len(codecs))

	for _, c := range codecs {
		// Check if codec should be stripped
		if flags.StripAllCodecs {
			continue
		}
		if h.codecInList(c.Name, flags.StripCodecs) {
			continue
		}

		// Check if codec should be masked (renamed)
		// For now, just include it
		result = append(result, internal.CodecInfo{
			PayloadType: c.PayloadType,
			Name:        c.Name,
			ClockRate:   c.ClockRate,
			Channels:    c.Channels,
			Fmtp:        c.Fmtp,
		})
	}

	return result
}

// codecInList checks if a codec name is in a list (case-insensitive)
func (h *OfferHandler) codecInList(name string, list []string) bool {
	name = strings.ToLower(name)
	for _, item := range list {
		if strings.ToLower(item) == name {
			return true
		}
	}
	return false
}

// buildStreamInfo constructs the stream info for the response
func (h *OfferHandler) buildStreamInfo(leg *internal.CallLeg, localIP string, rtpPort, rtcpPort int, flags *ng.ParsedFlags, parsedSDP *ParsedSDP) []ng.StreamInfo {
	streams := []ng.StreamInfo{
		{
			LocalIP:       localIP,
			LocalPort:     rtpPort,
			LocalRTCPPort: rtcpPort,
			MediaType:     "audio",
			Protocol:      string(leg.Transport),
			Index:         0,
		},
	}

	// Add ICE info if applicable
	if !flags.ICERemove && (flags.ICEForce || parsedSDP.HasICE) {
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

		// Add RTCP candidate if not muxed
		if !flags.RTCPMUX && !parsedSDP.RTCPMux {
			streams[0].ICECandidates = append(streams[0].ICECandidates, ng.ICECandidate{
				Foundation: "1",
				Component:  2,
				Protocol:   "UDP",
				Priority:   2130706430,
				IP:         localIP,
				Port:       rtcpPort,
				Type:       "host",
			})
		}
	}

	// Add DTLS info if applicable
	if leg.SRTPParams != nil && leg.SRTPParams.DTLS {
		streams[0].Setup = "actpass"
		streams[0].Fingerprint = "sha-256 XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX"
		streams[0].FingerprintHash = "sha-256"
	}

	return streams
}

// buildModifiedSDP creates the modified SDP for the offer response
func (h *OfferHandler) buildModifiedSDP(parsed *ParsedSDP, localIP string, localPort int, flags *ng.ParsedFlags) string {
	var sb strings.Builder

	// Version
	sb.WriteString("v=0\r\n")

	// Origin line - handle replace-origin flag
	sessionID := parsed.SessionID
	sessionVersion := parsed.SessionVersion + 1
	originUsername := parsed.OriginUsername
	if flags.ReplaceOrigin || flags.ReplaceUsername {
		originUsername = "karl"
	}
	if originUsername == "" {
		originUsername = "karl"
	}
	addressFamily := "IP4"
	if flags.AddressFamily == "inet6" {
		addressFamily = "IP6"
	}
	sb.WriteString(fmt.Sprintf("o=%s %d %d IN %s %s\r\n",
		originUsername, sessionID, sessionVersion, addressFamily, localIP))

	// Session name - handle replace-session-name flag
	sessionName := parsed.SessionName
	if flags.ReplaceSessionName || sessionName == "" {
		sessionName = "Karl Media Server"
	}
	sb.WriteString(fmt.Sprintf("s=%s\r\n", sessionName))

	// Connection - handle replace-session-connection flag
	sb.WriteString(fmt.Sprintf("c=IN %s %s\r\n", addressFamily, localIP))

	// Timing
	sb.WriteString("t=0 0\r\n")

	// Determine transport protocol
	protocol := h.determineProtocol(parsed, flags)

	// Filter codecs based on flags
	filteredCodecs := h.filterCodecsForSDP(parsed.Codecs, flags)

	// Build payload type list
	payloadTypes := make([]string, len(filteredCodecs))
	for i, c := range filteredCodecs {
		payloadTypes[i] = fmt.Sprintf("%d", c.PayloadType)
	}

	// Handle port 0 for inactive streams
	mediaPort := localPort
	if flags.Inactive {
		mediaPort = 0
	}

	sb.WriteString(fmt.Sprintf("m=audio %d %s %s\r\n",
		mediaPort, protocol, strings.Join(payloadTypes, " ")))

	// Add rtpmap and fmtp for each codec
	for _, c := range filteredCodecs {
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

	// Ptime handling
	if flags.Ptime > 0 {
		sb.WriteString(fmt.Sprintf("a=ptime:%d\r\n", flags.Ptime))
	} else if parsed.Ptime > 0 {
		sb.WriteString(fmt.Sprintf("a=ptime:%d\r\n", parsed.Ptime))
	}

	// Direction attribute
	direction := h.buildDirection(flags, parsed)
	sb.WriteString(fmt.Sprintf("a=%s\r\n", direction))

	// RTCP attribute - handle rtcp-mux variants
	h.writeRTCPAttributes(&sb, localPort, flags, parsed)

	// ICE attributes if not removed
	if !flags.ICERemove && (flags.ICEForce || parsed.HasICE) {
		h.writeICEAttributes(&sb, localIP, localPort, flags, parsed)
	}

	// DTLS/SRTP attributes
	h.writeSecurityAttributes(&sb, parsed, flags)

	// MID attribute if requested
	if flags.GenerateMID && parsed.MID != "" {
		sb.WriteString(fmt.Sprintf("a=mid:%s\r\n", parsed.MID))
	}

	return sb.String()
}

// determineProtocol determines the SDP transport protocol
func (h *OfferHandler) determineProtocol(parsed *ParsedSDP, flags *ng.ParsedFlags) string {
	// Explicit protocol flags take precedence
	if flags.UDPTLS {
		return "UDP/TLS/RTP/SAVPF"
	}
	if flags.RTPSAVPF {
		return "RTP/SAVPF"
	}
	if flags.RTPSAVP {
		return "RTP/SAVP"
	}
	if flags.RTPAVPF {
		return "RTP/AVPF"
	}
	if flags.RTPAVP {
		return "RTP/AVP"
	}

	// DTLS handling
	if !flags.DTLSOff && parsed.HasDTLS {
		return "UDP/TLS/RTP/SAVPF"
	}

	// SDES handling
	if !flags.SDESOff && parsed.HasSRTP {
		if parsed.HasAVPF {
			return "RTP/SAVPF"
		}
		return "RTP/SAVP"
	}

	// Default
	if parsed.HasAVPF {
		return "RTP/AVPF"
	}
	return "RTP/AVP"
}

// filterCodecsForSDP filters codecs based on flags
func (h *OfferHandler) filterCodecsForSDP(codecs []CodecInfo, flags *ng.ParsedFlags) []CodecInfo {
	if flags.StripAllCodecs {
		return []CodecInfo{}
	}

	result := make([]CodecInfo, 0, len(codecs))
	for _, c := range codecs {
		if h.codecInList(c.Name, flags.StripCodecs) {
			continue
		}
		result = append(result, c)
	}
	return result
}

// buildDirection builds the direction attribute
func (h *OfferHandler) buildDirection(flags *ng.ParsedFlags, parsed *ParsedSDP) string {
	if flags.Inactive {
		return "inactive"
	}
	if flags.SendOnly {
		return "sendonly"
	}
	if flags.RecvOnly {
		return "recvonly"
	}
	if flags.OriginalSendrecv {
		if parsed.Direction != "" {
			return parsed.Direction
		}
	}
	return "sendrecv"
}

// writeRTCPAttributes writes RTCP-related SDP attributes
func (h *OfferHandler) writeRTCPAttributes(sb *strings.Builder, localPort int, flags *ng.ParsedFlags, parsed *ParsedSDP) {
	// RTCP-mux handling
	shouldMux := flags.RTCPMUX || flags.RTCPMUXRequire || flags.RTCPMUXOffer
	shouldMux = shouldMux || (flags.RTCPMUXAccept && parsed.RTCPMux)
	shouldMux = shouldMux || parsed.RTCPMux

	if flags.RTCPMUXDemux {
		shouldMux = false
	}

	if shouldMux {
		sb.WriteString("a=rtcp-mux\r\n")
	} else if !flags.NoRTCPAttribute {
		// Include explicit RTCP port attribute
		sb.WriteString(fmt.Sprintf("a=rtcp:%d\r\n", localPort+1))
	}

	// Full RTCP attribute if requested
	if flags.FullRTCPAttribute {
		sb.WriteString(fmt.Sprintf("a=rtcp:%d IN IP4 0.0.0.0\r\n", localPort+1))
	}
}

// writeICEAttributes writes ICE-related SDP attributes
func (h *OfferHandler) writeICEAttributes(sb *strings.Builder, localIP string, localPort int, flags *ng.ParsedFlags, parsed *ParsedSDP) {
	iceUfrag, icePwd := generateICECredentials()
	sb.WriteString(fmt.Sprintf("a=ice-ufrag:%s\r\n", iceUfrag))
	sb.WriteString(fmt.Sprintf("a=ice-pwd:%s\r\n", icePwd))

	if flags.ICELite {
		sb.WriteString("a=ice-lite\r\n")
	}

	// Add host candidate
	sb.WriteString(fmt.Sprintf("a=candidate:1 1 UDP 2130706431 %s %d typ host\r\n",
		localIP, localPort))
}

// writeSecurityAttributes writes DTLS/SRTP SDP attributes
func (h *OfferHandler) writeSecurityAttributes(sb *strings.Builder, parsed *ParsedSDP, flags *ng.ParsedFlags) {
	// DTLS
	if !flags.DTLSOff && parsed.HasDTLS {
		sb.WriteString("a=fingerprint:sha-256 XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX\r\n")

		setup := "actpass"
		if flags.DTLSPassive {
			setup = "passive"
		} else if flags.DTLSActive {
			setup = "active"
		}
		sb.WriteString(fmt.Sprintf("a=setup:%s\r\n", setup))
		return
	}

	// SDES
	if !flags.SDESOff && parsed.HasSRTP && !parsed.HasDTLS {
		cryptoSuite := parsed.CryptoSuite
		if cryptoSuite == "" {
			cryptoSuite = "AES_CM_128_HMAC_SHA1_80"
		}
		sb.WriteString(fmt.Sprintf("a=crypto:1 %s inline:XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX\r\n",
			cryptoSuite))
	}
}

// generateICECredentials generates ICE credentials
func generateICECredentials() (ufrag, pwd string) {
	// In production, use crypto/rand
	ufrag = "karl" + fmt.Sprintf("%08x", uint32(1234567890))
	pwd = "karlpass" + fmt.Sprintf("%016x", uint64(9876543210123456789))
	return
}
