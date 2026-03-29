package commands

import (
	"karl/internal"
	ng "karl/internal/ng_protocol"
)

// DTMFCommandHandler handles DTMF-related commands
type DTMFCommandHandler struct {
	sessionRegistry *internal.SessionRegistry
}

// NewDTMFCommandHandler creates a new DTMF command handler
func NewDTMFCommandHandler(registry *internal.SessionRegistry) *DTMFCommandHandler {
	return &DTMFCommandHandler{
		sessionRegistry: registry,
	}
}

// HandleBlockDTMF handles the "block DTMF" command
func (h *DTMFCommandHandler) HandleBlockDTMF(req *ng.NGRequest) (*ng.NGResponse, error) {
	if req.CallID == "" {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonMissingParam + ": call-id",
		}, nil
	}

	session := h.findSession(req)
	if session == nil {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonNotFound,
		}, nil
	}

	// Set DTMF block flag
	session.SetFlag("dtmf_blocked", true)

	return &ng.NGResponse{
		Result: ng.ResultOK,
	}, nil
}

// HandleUnblockDTMF handles the "unblock DTMF" command
func (h *DTMFCommandHandler) HandleUnblockDTMF(req *ng.NGRequest) (*ng.NGResponse, error) {
	if req.CallID == "" {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonMissingParam + ": call-id",
		}, nil
	}

	session := h.findSession(req)
	if session == nil {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonNotFound,
		}, nil
	}

	// Clear DTMF block flag
	session.SetFlag("dtmf_blocked", false)

	return &ng.NGResponse{
		Result: ng.ResultOK,
	}, nil
}

// HandlePlayDTMF handles the "play DTMF" command
func (h *DTMFCommandHandler) HandlePlayDTMF(req *ng.NGRequest) (*ng.NGResponse, error) {
	if req.CallID == "" {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonMissingParam + ": call-id",
		}, nil
	}

	if req.DTMFDigit == "" {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonMissingParam + ": digit",
		}, nil
	}

	session := h.findSession(req)
	if session == nil {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonNotFound,
		}, nil
	}

	// Validate DTMF digit
	validDigits := "0123456789*#ABCD"
	valid := false
	for _, c := range validDigits {
		if string(c) == req.DTMFDigit {
			valid = true
			break
		}
	}
	if !valid {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: "Invalid DTMF digit",
		}, nil
	}

	// Duration in milliseconds (default 100ms)
	duration := req.DTMFDuration
	if duration <= 0 {
		duration = 100
	}
	if duration > 5000 {
		duration = 5000 // Max 5 seconds
	}

	// Queue DTMF for transmission
	session.SetMetadata("pending_dtmf", req.DTMFDigit)
	session.SetMetadata("pending_dtmf_duration", string(rune(duration)))

	return &ng.NGResponse{
		Result: ng.ResultOK,
		Extra: map[string]interface{}{
			"digit":    req.DTMFDigit,
			"duration": duration,
		},
	}, nil
}

// findSession finds a session by call-id and tags
func (h *DTMFCommandHandler) findSession(req *ng.NGRequest) *internal.MediaSession {
	session := h.sessionRegistry.GetSessionByTags(req.CallID, req.FromTag, req.ToTag)
	if session == nil {
		sessions := h.sessionRegistry.GetSessionByCallID(req.CallID)
		if len(sessions) > 0 {
			session = sessions[0]
		}
	}
	return session
}

// DTMF event types (RFC 4733)
const (
	DTMFEvent0     = 0
	DTMFEvent1     = 1
	DTMFEvent2     = 2
	DTMFEvent3     = 3
	DTMFEvent4     = 4
	DTMFEvent5     = 5
	DTMFEvent6     = 6
	DTMFEvent7     = 7
	DTMFEvent8     = 8
	DTMFEvent9     = 9
	DTMFEventStar  = 10
	DTMFEventPound = 11
	DTMFEventA     = 12
	DTMFEventB     = 13
	DTMFEventC     = 14
	DTMFEventD     = 15
)

// DTMFDigitToEvent converts a DTMF digit to event code
func DTMFDigitToEvent(digit string) int {
	switch digit {
	case "0":
		return DTMFEvent0
	case "1":
		return DTMFEvent1
	case "2":
		return DTMFEvent2
	case "3":
		return DTMFEvent3
	case "4":
		return DTMFEvent4
	case "5":
		return DTMFEvent5
	case "6":
		return DTMFEvent6
	case "7":
		return DTMFEvent7
	case "8":
		return DTMFEvent8
	case "9":
		return DTMFEvent9
	case "*":
		return DTMFEventStar
	case "#":
		return DTMFEventPound
	case "A":
		return DTMFEventA
	case "B":
		return DTMFEventB
	case "C":
		return DTMFEventC
	case "D":
		return DTMFEventD
	default:
		return -1
	}
}

// DTMFEventToDigit converts event code to DTMF digit
func DTMFEventToDigit(event int) string {
	switch event {
	case DTMFEvent0:
		return "0"
	case DTMFEvent1:
		return "1"
	case DTMFEvent2:
		return "2"
	case DTMFEvent3:
		return "3"
	case DTMFEvent4:
		return "4"
	case DTMFEvent5:
		return "5"
	case DTMFEvent6:
		return "6"
	case DTMFEvent7:
		return "7"
	case DTMFEvent8:
		return "8"
	case DTMFEvent9:
		return "9"
	case DTMFEventStar:
		return "*"
	case DTMFEventPound:
		return "#"
	case DTMFEventA:
		return "A"
	case DTMFEventB:
		return "B"
	case DTMFEventC:
		return "C"
	case DTMFEventD:
		return "D"
	default:
		return ""
	}
}

// BuildDTMFPayload builds RFC 4733 DTMF payload
func BuildDTMFPayload(event int, endOfEvent bool, volume int, duration int) []byte {
	payload := make([]byte, 4)

	// Event ID (0-15)
	payload[0] = byte(event & 0x0F)

	// End of Event flag + Reserved + Volume
	if endOfEvent {
		payload[1] = 0x80 | byte(volume&0x3F)
	} else {
		payload[1] = byte(volume & 0x3F)
	}

	// Duration (16-bit, in timestamp units)
	payload[2] = byte((duration >> 8) & 0xFF)
	payload[3] = byte(duration & 0xFF)

	return payload
}

// ParseDTMFPayload parses RFC 4733 DTMF payload
func ParseDTMFPayload(payload []byte) (event int, endOfEvent bool, volume int, duration int) {
	if len(payload) < 4 {
		return -1, false, 0, 0
	}

	event = int(payload[0] & 0x0F)
	endOfEvent = (payload[1] & 0x80) != 0
	volume = int(payload[1] & 0x3F)
	duration = int(payload[2])<<8 | int(payload[3])

	return
}
