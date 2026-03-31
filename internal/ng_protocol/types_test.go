package ng_protocol

import (
	"testing"
)

func TestParseFlags_BasicFlags(t *testing.T) {
	tests := []struct {
		name     string
		flags    []string
		expected func(*ParsedFlags) bool
	}{
		{
			name:  "symmetric flag",
			flags: []string{"symmetric"},
			expected: func(pf *ParsedFlags) bool {
				return pf.Symmetric == true
			},
		},
		{
			name:  "asymmetric flag",
			flags: []string{"asymmetric"},
			expected: func(pf *ParsedFlags) bool {
				return pf.Asymmetric == true
			},
		},
		{
			name:  "record-call flag",
			flags: []string{"record-call"},
			expected: func(pf *ParsedFlags) bool {
				return pf.RecordCall == true
			},
		},
		{
			name:  "webrtc flag",
			flags: []string{"webrtc"},
			expected: func(pf *ParsedFlags) bool {
				return pf.WebRTCEnabled == true
			},
		},
		{
			name:  "strict-source flag",
			flags: []string{"strict-source"},
			expected: func(pf *ParsedFlags) bool {
				return pf.StrictSource == true
			},
		},
		{
			name:  "port-latching flag",
			flags: []string{"port-latching"},
			expected: func(pf *ParsedFlags) bool {
				return pf.PortLatching == true
			},
		},
		{
			name:  "no-port-latching flag",
			flags: []string{"no-port-latching"},
			expected: func(pf *ParsedFlags) bool {
				return pf.NoPortLatching == true
			},
		},
		{
			name:  "loop-protect flag",
			flags: []string{"loop-protect"},
			expected: func(pf *ParsedFlags) bool {
				return pf.LoopProtect == true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf := ParseFlags(tt.flags)
			if !tt.expected(pf) {
				t.Errorf("ParseFlags(%v) did not set expected flag", tt.flags)
			}
		})
	}
}

func TestParseFlags_ICEFlags(t *testing.T) {
	tests := []struct {
		name     string
		flags    []string
		expected func(*ParsedFlags) bool
	}{
		{
			name:  "ICE=remove",
			flags: []string{"ICE=remove"},
			expected: func(pf *ParsedFlags) bool {
				return pf.ICERemove == true
			},
		},
		{
			name:  "ICE=force",
			flags: []string{"ICE=force"},
			expected: func(pf *ParsedFlags) bool {
				return pf.ICEForce == true
			},
		},
		{
			name:  "ICE=force-relay",
			flags: []string{"ICE=force-relay"},
			expected: func(pf *ParsedFlags) bool {
				return pf.ICEForceRelay == true
			},
		},
		{
			name:  "ICE=lite",
			flags: []string{"ICE=lite"},
			expected: func(pf *ParsedFlags) bool {
				return pf.ICELite == true
			},
		},
		{
			name:  "ice-lite flag",
			flags: []string{"ice-lite"},
			expected: func(pf *ParsedFlags) bool {
				return pf.ICELite == true
			},
		},
		{
			name:  "no-ice flag",
			flags: []string{"no-ice"},
			expected: func(pf *ParsedFlags) bool {
				return pf.ICERemove == true
			},
		},
		{
			name:  "trickle-ice flag",
			flags: []string{"trickle-ice"},
			expected: func(pf *ParsedFlags) bool {
				return pf.TrickleICE == true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf := ParseFlags(tt.flags)
			if !tt.expected(pf) {
				t.Errorf("ParseFlags(%v) did not set expected ICE flag", tt.flags)
			}
		})
	}
}

func TestParseFlags_DTLSFlags(t *testing.T) {
	tests := []struct {
		name     string
		flags    []string
		expected func(*ParsedFlags) bool
	}{
		{
			name:  "DTLS=off",
			flags: []string{"DTLS=off"},
			expected: func(pf *ParsedFlags) bool {
				return pf.DTLSOff == true
			},
		},
		{
			name:  "DTLS=passive",
			flags: []string{"DTLS=passive"},
			expected: func(pf *ParsedFlags) bool {
				return pf.DTLSPassive == true
			},
		},
		{
			name:  "DTLS=active",
			flags: []string{"DTLS=active"},
			expected: func(pf *ParsedFlags) bool {
				return pf.DTLSActive == true
			},
		},
		{
			name:  "dtls-passive flag",
			flags: []string{"dtls-passive"},
			expected: func(pf *ParsedFlags) bool {
				return pf.DTLSPassive == true
			},
		},
		{
			name:  "no-dtls flag",
			flags: []string{"no-dtls"},
			expected: func(pf *ParsedFlags) bool {
				return pf.DTLSOff == true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf := ParseFlags(tt.flags)
			if !tt.expected(pf) {
				t.Errorf("ParseFlags(%v) did not set expected DTLS flag", tt.flags)
			}
		})
	}
}

func TestParseFlags_SDESFlags(t *testing.T) {
	tests := []struct {
		name     string
		flags    []string
		expected func(*ParsedFlags) bool
	}{
		{
			name:  "SDES=off",
			flags: []string{"SDES=off"},
			expected: func(pf *ParsedFlags) bool {
				return pf.SDESOff == true
			},
		},
		{
			name:  "SDES=on",
			flags: []string{"SDES=on"},
			expected: func(pf *ParsedFlags) bool {
				return pf.SDESOn == true
			},
		},
		{
			name:  "SDES=only",
			flags: []string{"SDES=only"},
			expected: func(pf *ParsedFlags) bool {
				return pf.SDESOnly == true
			},
		},
		{
			name:  "sdes-only flag",
			flags: []string{"sdes-only"},
			expected: func(pf *ParsedFlags) bool {
				return pf.SDESOnly == true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf := ParseFlags(tt.flags)
			if !tt.expected(pf) {
				t.Errorf("ParseFlags(%v) did not set expected SDES flag", tt.flags)
			}
		})
	}
}

func TestParseFlags_DirectionFlags(t *testing.T) {
	tests := []struct {
		name     string
		flags    []string
		expected func(*ParsedFlags) bool
	}{
		{
			name:  "sendonly flag",
			flags: []string{"sendonly"},
			expected: func(pf *ParsedFlags) bool {
				return pf.SendOnly == true
			},
		},
		{
			name:  "recvonly flag",
			flags: []string{"recvonly"},
			expected: func(pf *ParsedFlags) bool {
				return pf.RecvOnly == true
			},
		},
		{
			name:  "inactive flag",
			flags: []string{"inactive"},
			expected: func(pf *ParsedFlags) bool {
				return pf.Inactive == true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf := ParseFlags(tt.flags)
			if !tt.expected(pf) {
				t.Errorf("ParseFlags(%v) did not set expected direction flag", tt.flags)
			}
		})
	}
}

func TestParseFlags_KeyValueFlags(t *testing.T) {
	tests := []struct {
		name     string
		flags    []string
		expected func(*ParsedFlags) bool
	}{
		{
			name:  "TOS value",
			flags: []string{"TOS=184"},
			expected: func(pf *ParsedFlags) bool {
				return pf.TOS == 184 && pf.TOSSet == true
			},
		},
		{
			name:  "media-timeout value",
			flags: []string{"media-timeout=60"},
			expected: func(pf *ParsedFlags) bool {
				return pf.MediaTimeout == 60
			},
		},
		{
			name:  "session-timeout value",
			flags: []string{"session-timeout=3600"},
			expected: func(pf *ParsedFlags) bool {
				return pf.SessionTimeout == 3600
			},
		},
		{
			name:  "delete-delay value",
			flags: []string{"delete-delay=10"},
			expected: func(pf *ParsedFlags) bool {
				return pf.DeleteDelay == 10
			},
		},
		{
			name:  "ptime value",
			flags: []string{"ptime=20"},
			expected: func(pf *ParsedFlags) bool {
				return pf.Ptime == 20
			},
		},
		{
			name:  "interface value",
			flags: []string{"interface=external"},
			expected: func(pf *ParsedFlags) bool {
				return pf.Interface == "external"
			},
		},
		{
			name:  "from-interface value",
			flags: []string{"from-interface=internal"},
			expected: func(pf *ParsedFlags) bool {
				return pf.FromInterface == "internal"
			},
		},
		{
			name:  "to-interface value",
			flags: []string{"to-interface=external"},
			expected: func(pf *ParsedFlags) bool {
				return pf.ToInterface == "external"
			},
		},
		{
			name:  "label value",
			flags: []string{"label=caller"},
			expected: func(pf *ParsedFlags) bool {
				return pf.Label == "caller"
			},
		},
		{
			name:  "set-label value",
			flags: []string{"set-label=participant1"},
			expected: func(pf *ParsedFlags) bool {
				return pf.SetLabel == "participant1"
			},
		},
		{
			name:  "address-family value",
			flags: []string{"address-family=inet6"},
			expected: func(pf *ParsedFlags) bool {
				return pf.AddressFamily == "inet6"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf := ParseFlags(tt.flags)
			if !tt.expected(pf) {
				t.Errorf("ParseFlags(%v) did not set expected key=value flag", tt.flags)
			}
		})
	}
}

func TestParseFlags_CodecFlags(t *testing.T) {
	tests := []struct {
		name     string
		flags    []string
		expected func(*ParsedFlags) bool
	}{
		{
			name:  "codec-strip value",
			flags: []string{"codec-strip=G729"},
			expected: func(pf *ParsedFlags) bool {
				return len(pf.StripCodecs) == 1 && pf.StripCodecs[0] == "G729"
			},
		},
		{
			name:  "codec-offer value",
			flags: []string{"codec-offer=opus"},
			expected: func(pf *ParsedFlags) bool {
				return len(pf.OfferCodecs) == 1 && pf.OfferCodecs[0] == "opus"
			},
		},
		{
			name:  "codec-transcode value",
			flags: []string{"codec-transcode=PCMU"},
			expected: func(pf *ParsedFlags) bool {
				return len(pf.TranscodeCodecs) == 1 && pf.TranscodeCodecs[0] == "PCMU"
			},
		},
		{
			name:  "codec-strip-all flag",
			flags: []string{"codec-strip-all"},
			expected: func(pf *ParsedFlags) bool {
				return pf.StripAllCodecs == true
			},
		},
		{
			name:  "always-transcode flag",
			flags: []string{"always-transcode"},
			expected: func(pf *ParsedFlags) bool {
				return pf.AlwaysTranscode == true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf := ParseFlags(tt.flags)
			if !tt.expected(pf) {
				t.Errorf("ParseFlags(%v) did not set expected codec flag", tt.flags)
			}
		})
	}
}

func TestParseFlags_RTCPMuxFlags(t *testing.T) {
	tests := []struct {
		name     string
		flags    []string
		expected func(*ParsedFlags) bool
	}{
		{
			name:  "rtcp-mux flag",
			flags: []string{"rtcp-mux"},
			expected: func(pf *ParsedFlags) bool {
				return pf.RTCPMUX == true
			},
		},
		{
			name:  "rtcp-mux-demux flag",
			flags: []string{"rtcp-mux-demux"},
			expected: func(pf *ParsedFlags) bool {
				return pf.RTCPMUXDemux == true
			},
		},
		{
			name:  "rtcp-mux-accept flag",
			flags: []string{"rtcp-mux-accept"},
			expected: func(pf *ParsedFlags) bool {
				return pf.RTCPMUXAccept == true
			},
		},
		{
			name:  "rtcp-mux-offer flag",
			flags: []string{"rtcp-mux-offer"},
			expected: func(pf *ParsedFlags) bool {
				return pf.RTCPMUXOffer == true
			},
		},
		{
			name:  "rtcp-mux-require flag",
			flags: []string{"rtcp-mux-require"},
			expected: func(pf *ParsedFlags) bool {
				return pf.RTCPMUXRequire == true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf := ParseFlags(tt.flags)
			if !tt.expected(pf) {
				t.Errorf("ParseFlags(%v) did not set expected RTCP-mux flag", tt.flags)
			}
		})
	}
}

func TestParseFlags_TransportFlags(t *testing.T) {
	tests := []struct {
		name     string
		flags    []string
		expected func(*ParsedFlags) bool
	}{
		{
			name:  "RTP/AVP flag",
			flags: []string{"RTP/AVP"},
			expected: func(pf *ParsedFlags) bool {
				return pf.RTPAVP == true
			},
		},
		{
			name:  "RTP/SAVP flag",
			flags: []string{"RTP/SAVP"},
			expected: func(pf *ParsedFlags) bool {
				return pf.RTPSAVP == true
			},
		},
		{
			name:  "RTP/AVPF flag",
			flags: []string{"RTP/AVPF"},
			expected: func(pf *ParsedFlags) bool {
				return pf.RTPAVPF == true
			},
		},
		{
			name:  "RTP/SAVPF flag",
			flags: []string{"RTP/SAVPF"},
			expected: func(pf *ParsedFlags) bool {
				return pf.RTPSAVPF == true
			},
		},
		{
			name:  "UDP/TLS/RTP/SAVPF flag",
			flags: []string{"UDP/TLS/RTP/SAVPF"},
			expected: func(pf *ParsedFlags) bool {
				return pf.UDPTLS == true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf := ParseFlags(tt.flags)
			if !tt.expected(pf) {
				t.Errorf("ParseFlags(%v) did not set expected transport flag", tt.flags)
			}
		})
	}
}

func TestParseFlags_T38Flags(t *testing.T) {
	tests := []struct {
		name     string
		flags    []string
		expected func(*ParsedFlags) bool
	}{
		{
			name:  "T.38 flag",
			flags: []string{"T.38"},
			expected: func(pf *ParsedFlags) bool {
				return pf.T38Support == true
			},
		},
		{
			name:  "t38 flag",
			flags: []string{"t38"},
			expected: func(pf *ParsedFlags) bool {
				return pf.T38Support == true
			},
		},
		{
			name:  "T.38-gateway flag",
			flags: []string{"T.38-gateway"},
			expected: func(pf *ParsedFlags) bool {
				return pf.T38Gateway == true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf := ParseFlags(tt.flags)
			if !tt.expected(pf) {
				t.Errorf("ParseFlags(%v) did not set expected T.38 flag", tt.flags)
			}
		})
	}
}

func TestParseFlags_RecordingFlags(t *testing.T) {
	tests := []struct {
		name     string
		flags    []string
		expected func(*ParsedFlags) bool
	}{
		{
			name:  "record-call flag",
			flags: []string{"record-call"},
			expected: func(pf *ParsedFlags) bool {
				return pf.RecordCall == true
			},
		},
		{
			name:  "start-recording flag",
			flags: []string{"start-recording"},
			expected: func(pf *ParsedFlags) bool {
				return pf.StartRecording == true
			},
		},
		{
			name:  "stop-recording flag",
			flags: []string{"stop-recording"},
			expected: func(pf *ParsedFlags) bool {
				return pf.StopRecording == true
			},
		},
		{
			name:  "pause-recording flag",
			flags: []string{"pause-recording"},
			expected: func(pf *ParsedFlags) bool {
				return pf.PauseRecording == true
			},
		},
		{
			name:  "SIPREC flag",
			flags: []string{"SIPREC"},
			expected: func(pf *ParsedFlags) bool {
				return pf.SIPREC == true
			},
		},
		{
			name:  "recording-file value",
			flags: []string{"recording-file=/tmp/call.wav"},
			expected: func(pf *ParsedFlags) bool {
				return pf.RecordingFile == "/tmp/call.wav"
			},
		},
		{
			name:  "recording-path value",
			flags: []string{"recording-path=/var/recordings"},
			expected: func(pf *ParsedFlags) bool {
				return pf.RecordingPath == "/var/recordings"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf := ParseFlags(tt.flags)
			if !tt.expected(pf) {
				t.Errorf("ParseFlags(%v) did not set expected recording flag", tt.flags)
			}
		})
	}
}

func TestParseFlags_MultipleFlags(t *testing.T) {
	flags := []string{
		"symmetric",
		"ICE=remove",
		"DTLS=passive",
		"rtcp-mux",
		"record-call",
		"TOS=184",
		"interface=external",
	}

	pf := ParseFlags(flags)

	if !pf.Symmetric {
		t.Error("Expected Symmetric to be true")
	}
	if !pf.ICERemove {
		t.Error("Expected ICERemove to be true")
	}
	if !pf.DTLSPassive {
		t.Error("Expected DTLSPassive to be true")
	}
	if !pf.RTCPMUX {
		t.Error("Expected RTCPMUX to be true")
	}
	if !pf.RecordCall {
		t.Error("Expected RecordCall to be true")
	}
	if pf.TOS != 184 {
		t.Errorf("Expected TOS to be 184, got %d", pf.TOS)
	}
	if pf.Interface != "external" {
		t.Errorf("Expected Interface to be 'external', got '%s'", pf.Interface)
	}
}

func TestParseFlags_DefaultValues(t *testing.T) {
	pf := ParseFlags([]string{})

	if pf.TOS != -1 {
		t.Errorf("Expected default TOS to be -1, got %d", pf.TOS)
	}
	if pf.MediaTimeout != -1 {
		t.Errorf("Expected default MediaTimeout to be -1, got %d", pf.MediaTimeout)
	}
	if pf.SessionTimeout != -1 {
		t.Errorf("Expected default SessionTimeout to be -1, got %d", pf.SessionTimeout)
	}
	if pf.DeleteDelay != -1 {
		t.Errorf("Expected default DeleteDelay to be -1, got %d", pf.DeleteDelay)
	}
	if pf.Ptime != -1 {
		t.Errorf("Expected default Ptime to be -1, got %d", pf.Ptime)
	}
}
