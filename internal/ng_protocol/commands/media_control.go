package commands

import (
	"karl/internal"
	ng "karl/internal/ng_protocol"
)

// MediaControlHandler handles media control commands
type MediaControlHandler struct {
	sessionRegistry *internal.SessionRegistry
}

// NewMediaControlHandler creates a new media control handler
func NewMediaControlHandler(registry *internal.SessionRegistry) *MediaControlHandler {
	return &MediaControlHandler{
		sessionRegistry: registry,
	}
}

// HandleBlockMedia handles the "block media" command
func (h *MediaControlHandler) HandleBlockMedia(req *ng.NGRequest) (*ng.NGResponse, error) {
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

	// Set media block flag
	session.SetFlag("media_blocked", true)

	return &ng.NGResponse{
		Result: ng.ResultOK,
	}, nil
}

// HandleUnblockMedia handles the "unblock media" command
func (h *MediaControlHandler) HandleUnblockMedia(req *ng.NGRequest) (*ng.NGResponse, error) {
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

	// Clear media block flag
	session.SetFlag("media_blocked", false)

	return &ng.NGResponse{
		Result: ng.ResultOK,
	}, nil
}

// HandleSilenceMedia handles the "silence media" command
func (h *MediaControlHandler) HandleSilenceMedia(req *ng.NGRequest) (*ng.NGResponse, error) {
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

	// Set silence flag - media will be replaced with silence
	session.SetFlag("media_silenced", true)

	return &ng.NGResponse{
		Result: ng.ResultOK,
	}, nil
}

// HandleStartForwarding handles the "start forwarding" command
func (h *MediaControlHandler) HandleStartForwarding(req *ng.NGRequest) (*ng.NGResponse, error) {
	if req.CallID == "" {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonMissingParam + ": call-id",
		}, nil
	}

	if req.ForwardAddress == "" || req.ForwardPort <= 0 {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonMissingParam + ": forward-address and forward-port",
		}, nil
	}

	session := h.findSession(req)
	if session == nil {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonNotFound,
		}, nil
	}

	// Set forwarding parameters
	session.SetFlag("forwarding", true)
	session.SetMetadata("forward_address", req.ForwardAddress)
	session.SetMetadata("forward_port", string(rune(req.ForwardPort)))

	return &ng.NGResponse{
		Result: ng.ResultOK,
		Extra: map[string]interface{}{
			"forward-address": req.ForwardAddress,
			"forward-port":    req.ForwardPort,
		},
	}, nil
}

// HandleStopForwarding handles the "stop forwarding" command
func (h *MediaControlHandler) HandleStopForwarding(req *ng.NGRequest) (*ng.NGResponse, error) {
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

	// Clear forwarding
	session.SetFlag("forwarding", false)
	session.SetMetadata("forward_address", "")
	session.SetMetadata("forward_port", "")

	return &ng.NGResponse{
		Result: ng.ResultOK,
	}, nil
}

// HandlePlayMedia handles the "play media" command
func (h *MediaControlHandler) HandlePlayMedia(req *ng.NGRequest) (*ng.NGResponse, error) {
	if req.CallID == "" {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonMissingParam + ": call-id",
		}, nil
	}

	// Get file path from raw params
	filePath := ""
	if req.RawParams != nil {
		filePath = ng.DictGetString(req.RawParams, "file")
	}
	if filePath == "" {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonMissingParam + ": file",
		}, nil
	}

	session := h.findSession(req)
	if session == nil {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonNotFound,
		}, nil
	}

	// Set playback parameters
	session.SetFlag("playing_media", true)
	session.SetMetadata("play_file", filePath)

	return &ng.NGResponse{
		Result: ng.ResultOK,
		Extra: map[string]interface{}{
			"file": filePath,
		},
	}, nil
}

// HandleStopMedia handles the "stop media" command
func (h *MediaControlHandler) HandleStopMedia(req *ng.NGRequest) (*ng.NGResponse, error) {
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

	// Clear playback
	session.SetFlag("playing_media", false)
	session.SetMetadata("play_file", "")

	return &ng.NGResponse{
		Result: ng.ResultOK,
	}, nil
}

// findSession finds a session by call-id and tags
func (h *MediaControlHandler) findSession(req *ng.NGRequest) *internal.MediaSession {
	session := h.sessionRegistry.GetSessionByTags(req.CallID, req.FromTag, req.ToTag)
	if session == nil {
		sessions := h.sessionRegistry.GetSessionByCallID(req.CallID)
		if len(sessions) > 0 {
			session = sessions[0]
		}
	}
	return session
}

// MediaFlags holds media manipulation flags for a session
type MediaFlags struct {
	MediaBlocked  bool
	MediaSilenced bool
	DTMFBlocked   bool
	Forwarding    bool
	PlayingMedia  bool
}

// GetMediaFlags extracts media flags from a session
func GetMediaFlags(session *internal.MediaSession) *MediaFlags {
	return &MediaFlags{
		MediaBlocked:  session.GetFlag("media_blocked"),
		MediaSilenced: session.GetFlag("media_silenced"),
		DTMFBlocked:   session.GetFlag("dtmf_blocked"),
		Forwarding:    session.GetFlag("forwarding"),
		PlayingMedia:  session.GetFlag("playing_media"),
	}
}

// ShouldProcessMedia determines if media should be processed
func ShouldProcessMedia(flags *MediaFlags) bool {
	return !flags.MediaBlocked
}

// ShouldForwardMedia determines if media should be forwarded
func ShouldForwardMedia(flags *MediaFlags) bool {
	return flags.Forwarding && !flags.MediaBlocked
}

// ShouldReplaceWithSilence determines if media should be replaced with silence
func ShouldReplaceWithSilence(flags *MediaFlags) bool {
	return flags.MediaSilenced && !flags.MediaBlocked
}
