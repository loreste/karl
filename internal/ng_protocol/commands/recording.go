package commands

import (
	"karl/internal"
	ng "karl/internal/ng_protocol"
	"karl/internal/recording"
)

// RecordingCommandHandler handles recording-related commands
type RecordingCommandHandler struct {
	sessionRegistry   *internal.SessionRegistry
	recordingManager  *recording.Manager
}

// NewRecordingCommandHandler creates a new recording command handler
func NewRecordingCommandHandler(registry *internal.SessionRegistry, recorder *recording.Manager) *RecordingCommandHandler {
	return &RecordingCommandHandler{
		sessionRegistry:  registry,
		recordingManager: recorder,
	}
}

// HandleStartRecording handles the "start recording" command
func (h *RecordingCommandHandler) HandleStartRecording(req *ng.NGRequest) (*ng.NGResponse, error) {
	// Validate required parameters
	if req.CallID == "" {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonMissingParam + ": call-id",
		}, nil
	}

	// Find session
	session := h.sessionRegistry.GetSessionByTags(req.CallID, req.FromTag, req.ToTag)
	if session == nil {
		sessions := h.sessionRegistry.GetSessionByCallID(req.CallID)
		if len(sessions) > 0 {
			session = sessions[0]
		}
	}

	if session == nil {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonNotFound,
		}, nil
	}

	if h.recordingManager == nil {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: "Recording not available",
		}, nil
	}

	// Get recording options
	format := "wav"
	mode := "mixed"
	if req.RawParams != nil {
		if f := ng.DictGetString(req.RawParams, "format"); f != "" {
			format = f
		}
		if m := ng.DictGetString(req.RawParams, "mode"); m != "" {
			mode = m
		}
	}

	// Start recording
	recordingID, err := h.recordingManager.StartRecording(session.ID, req.CallID, format, mode, req.RecordingMeta)
	if err != nil {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: err.Error(),
		}, nil
	}

	// Mark session as recording
	session.SetFlag("recording", true)
	session.SetMetadata("recording_id", recordingID)

	return &ng.NGResponse{
		Result: ng.ResultOK,
		Extra: map[string]interface{}{
			"recording-id": recordingID,
		},
	}, nil
}

// HandleStopRecording handles the "stop recording" command
func (h *RecordingCommandHandler) HandleStopRecording(req *ng.NGRequest) (*ng.NGResponse, error) {
	if req.CallID == "" {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonMissingParam + ": call-id",
		}, nil
	}

	// Find session
	session := h.sessionRegistry.GetSessionByTags(req.CallID, req.FromTag, req.ToTag)
	if session == nil {
		sessions := h.sessionRegistry.GetSessionByCallID(req.CallID)
		if len(sessions) > 0 {
			session = sessions[0]
		}
	}

	if session == nil {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonNotFound,
		}, nil
	}

	if h.recordingManager == nil {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: "Recording not available",
		}, nil
	}

	// Get recording ID
	recordingID := session.GetMetadata("recording_id")
	if recordingID == "" {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: "No active recording",
		}, nil
	}

	// Stop recording
	if err := h.recordingManager.StopRecording(recordingID); err != nil {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: err.Error(),
		}, nil
	}

	// Update session
	session.SetFlag("recording", false)

	return &ng.NGResponse{
		Result: ng.ResultOK,
		Extra: map[string]interface{}{
			"recording-id": recordingID,
		},
	}, nil
}

// HandlePauseRecording handles the "pause recording" command
func (h *RecordingCommandHandler) HandlePauseRecording(req *ng.NGRequest) (*ng.NGResponse, error) {
	if req.CallID == "" {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonMissingParam + ": call-id",
		}, nil
	}

	session := h.sessionRegistry.GetSessionByTags(req.CallID, req.FromTag, req.ToTag)
	if session == nil {
		sessions := h.sessionRegistry.GetSessionByCallID(req.CallID)
		if len(sessions) > 0 {
			session = sessions[0]
		}
	}

	if session == nil {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonNotFound,
		}, nil
	}

	if h.recordingManager == nil {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: "Recording not available",
		}, nil
	}

	recordingID := session.GetMetadata("recording_id")
	if recordingID == "" {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: "No active recording",
		}, nil
	}

	if err := h.recordingManager.PauseRecording(recordingID); err != nil {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: err.Error(),
		}, nil
	}

	return &ng.NGResponse{
		Result: ng.ResultOK,
	}, nil
}
