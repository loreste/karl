package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Recording handlers - these integrate with the recording system

// RecordingResponse represents a recording in API responses
type RecordingResponse struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	CallID      string    `json:"call_id"`
	Status      string    `json:"status"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time,omitempty"`
	Duration    float64   `json:"duration_seconds"`
	FilePath    string    `json:"file_path,omitempty"`
	FileSize    int64     `json:"file_size_bytes,omitempty"`
	Format      string    `json:"format"`
	Mode        string    `json:"mode"`
}

// StartRecordingRequest represents a start recording request
type StartRecordingRequest struct {
	SessionID string            `json:"session_id"`
	CallID    string            `json:"call_id"`
	Format    string            `json:"format,omitempty"`  // wav, pcm
	Mode      string            `json:"mode,omitempty"`    // mixed, stereo, separate
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// StopRecordingRequest represents a stop recording request
type StopRecordingRequest struct {
	SessionID   string `json:"session_id"`
	RecordingID string `json:"recording_id"`
}

// Recording manager interface for dependency injection
var recordingManager RecordingManagerInterface

// RecordingManagerInterface defines the recording manager interface
type RecordingManagerInterface interface {
	StartRecording(sessionID, callID, format, mode string, metadata map[string]string) (string, error)
	StopRecording(recordingID string) error
	PauseRecording(recordingID string) error
	ResumeRecording(recordingID string) error
	GetRecording(recordingID string) (*RecordingInfo, error)
	ListRecordings(filter RecordingFilter) ([]*RecordingInfo, error)
}

// RecordingInfo holds recording information
type RecordingInfo struct {
	ID        string
	SessionID string
	CallID    string
	Status    string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	FilePath  string
	FileSize  int64
	Format    string
	Mode      string
	Metadata  map[string]string
}

// RecordingFilter holds filter options for listing recordings
type RecordingFilter struct {
	SessionID string
	CallID    string
	Status    string
	StartFrom time.Time
	StartTo   time.Time
}

// SetRecordingManager sets the recording manager
func SetRecordingManager(rm RecordingManagerInterface) {
	recordingManager = rm
}

// handleStartRecording handles POST /api/v1/recording/start
func (r *Router) handleStartRecording(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var startReq StartRecordingRequest
	if err := json.NewDecoder(req.Body).Decode(&startReq); err != nil {
		r.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate request
	if startReq.SessionID == "" && startReq.CallID == "" {
		r.errorResponse(w, http.StatusBadRequest, "session_id or call_id required")
		return
	}

	// Default values
	if startReq.Format == "" {
		startReq.Format = "wav"
	}
	if startReq.Mode == "" {
		startReq.Mode = "mixed"
	}

	// If call_id provided, find session
	sessionID := startReq.SessionID
	if sessionID == "" && startReq.CallID != "" {
		sessions := r.sessionRegistry.GetSessionByCallID(startReq.CallID)
		if len(sessions) > 0 {
			sessionID = sessions[0].ID
		}
	}

	if sessionID == "" {
		r.errorResponse(w, http.StatusNotFound, "session not found")
		return
	}

	// Check recording manager
	if recordingManager == nil {
		r.errorResponse(w, http.StatusServiceUnavailable, "recording not available")
		return
	}

	// Start recording
	recordingID, err := recordingManager.StartRecording(
		sessionID,
		startReq.CallID,
		startReq.Format,
		startReq.Mode,
		startReq.Metadata,
	)
	if err != nil {
		r.errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	r.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"recording_id": recordingID,
		"session_id":   sessionID,
		"status":       "recording",
	})
}

// handleStopRecording handles POST /api/v1/recording/stop
func (r *Router) handleStopRecording(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var stopReq StopRecordingRequest
	if err := json.NewDecoder(req.Body).Decode(&stopReq); err != nil {
		r.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if stopReq.SessionID == "" && stopReq.RecordingID == "" {
		r.errorResponse(w, http.StatusBadRequest, "session_id or recording_id required")
		return
	}

	if recordingManager == nil {
		r.errorResponse(w, http.StatusServiceUnavailable, "recording not available")
		return
	}

	// Stop recording
	recordingID := stopReq.RecordingID
	if recordingID == "" {
		// Find recording by session ID
		recordings, err := recordingManager.ListRecordings(RecordingFilter{
			SessionID: stopReq.SessionID,
			Status:    "recording",
		})
		if err != nil || len(recordings) == 0 {
			r.errorResponse(w, http.StatusNotFound, "active recording not found")
			return
		}
		recordingID = recordings[0].ID
	}

	if err := recordingManager.StopRecording(recordingID); err != nil {
		r.errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	r.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"recording_id": recordingID,
		"status":       "stopped",
	})
}

// handleListRecordings handles GET /api/v1/recordings
func (r *Router) handleListRecordings(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if recordingManager == nil {
		r.errorResponse(w, http.StatusServiceUnavailable, "recording not available")
		return
	}

	// Build filter from query parameters
	filter := RecordingFilter{
		SessionID: req.URL.Query().Get("session_id"),
		CallID:    req.URL.Query().Get("call_id"),
		Status:    req.URL.Query().Get("status"),
	}

	if startFrom := req.URL.Query().Get("start_from"); startFrom != "" {
		if t, err := time.Parse(time.RFC3339, startFrom); err == nil {
			filter.StartFrom = t
		}
	}
	if startTo := req.URL.Query().Get("start_to"); startTo != "" {
		if t, err := time.Parse(time.RFC3339, startTo); err == nil {
			filter.StartTo = t
		}
	}

	// List recordings
	recordings, err := recordingManager.ListRecordings(filter)
	if err != nil {
		r.errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Convert to response format
	responses := make([]RecordingResponse, 0, len(recordings))
	for _, rec := range recordings {
		responses = append(responses, RecordingResponse{
			ID:        rec.ID,
			SessionID: rec.SessionID,
			CallID:    rec.CallID,
			Status:    rec.Status,
			StartTime: rec.StartTime,
			EndTime:   rec.EndTime,
			Duration:  rec.Duration.Seconds(),
			FilePath:  rec.FilePath,
			FileSize:  rec.FileSize,
			Format:    rec.Format,
			Mode:      rec.Mode,
		})
	}

	r.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"recordings": responses,
		"count":      len(responses),
	})
}

// handleRecordingByID handles GET /api/v1/recordings/{id}
func (r *Router) handleRecordingByID(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract recording ID from path
	path := req.URL.Path
	recordingID := strings.TrimPrefix(path, "/api/v1/recordings/")
	recordingID = strings.TrimSuffix(recordingID, "/")

	if recordingID == "" {
		r.errorResponse(w, http.StatusBadRequest, "recording ID required")
		return
	}

	if recordingManager == nil {
		r.errorResponse(w, http.StatusServiceUnavailable, "recording not available")
		return
	}

	// Get recording
	rec, err := recordingManager.GetRecording(recordingID)
	if err != nil {
		r.errorResponse(w, http.StatusNotFound, "recording not found")
		return
	}

	response := RecordingResponse{
		ID:        rec.ID,
		SessionID: rec.SessionID,
		CallID:    rec.CallID,
		Status:    rec.Status,
		StartTime: rec.StartTime,
		EndTime:   rec.EndTime,
		Duration:  rec.Duration.Seconds(),
		FilePath:  rec.FilePath,
		FileSize:  rec.FileSize,
		Format:    rec.Format,
		Mode:      rec.Mode,
	}

	r.jsonResponse(w, http.StatusOK, response)
}
