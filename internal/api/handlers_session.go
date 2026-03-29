package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"karl/internal"
)

// SessionResponse represents a session in API responses
type SessionResponse struct {
	ID          string            `json:"id"`
	CallID      string            `json:"call_id"`
	FromTag     string            `json:"from_tag"`
	ToTag       string            `json:"to_tag"`
	State       string            `json:"state"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Duration    float64           `json:"duration_seconds,omitempty"`
	CallerLeg   *LegResponse      `json:"caller_leg,omitempty"`
	CalleeLeg   *LegResponse      `json:"callee_leg,omitempty"`
	Stats       *SessionStatsResp `json:"stats,omitempty"`
	Flags       map[string]bool   `json:"flags,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// LegResponse represents a call leg in API responses
type LegResponse struct {
	Tag          string   `json:"tag"`
	IP           string   `json:"ip"`
	Port         int      `json:"port"`
	LocalIP      string   `json:"local_ip"`
	LocalPort    int      `json:"local_port"`
	MediaType    string   `json:"media_type"`
	Transport    string   `json:"transport"`
	SSRC         uint32   `json:"ssrc"`
	Codecs       []string `json:"codecs"`
	PacketsSent  uint64   `json:"packets_sent"`
	PacketsRecv  uint64   `json:"packets_recv"`
	BytesSent    uint64   `json:"bytes_sent"`
	BytesRecv    uint64   `json:"bytes_recv"`
	LastActivity string   `json:"last_activity"`
}

// SessionStatsResp represents session statistics in API responses
type SessionStatsResp struct {
	StartTime      time.Time `json:"start_time"`
	ConnectTime    time.Time `json:"connect_time,omitempty"`
	Duration       float64   `json:"duration_seconds"`
	PacketLossRate float64   `json:"packet_loss_rate"`
	AvgJitter      float64   `json:"avg_jitter_ms"`
	MaxJitter      float64   `json:"max_jitter_ms"`
	RTT            float64   `json:"rtt_ms"`
	MOS            float64   `json:"mos"`
}

// SessionListResponse represents a list of sessions
type SessionListResponse struct {
	Sessions []SessionResponse `json:"sessions"`
	Total    int               `json:"total"`
	Active   int               `json:"active"`
}

// handleSessions handles GET/POST /api/v1/sessions
func (r *Router) handleSessions(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.listSessions(w, req)
	case http.MethodPost:
		r.createSession(w, req)
	default:
		r.errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// listSessions returns all sessions
func (r *Router) listSessions(w http.ResponseWriter, req *http.Request) {
	sessions := r.sessionRegistry.ListSessions()

	response := SessionListResponse{
		Sessions: make([]SessionResponse, 0, len(sessions)),
		Total:    len(sessions),
		Active:   r.sessionRegistry.GetActiveCount(),
	}

	// Query parameters for filtering
	state := req.URL.Query().Get("state")
	callID := req.URL.Query().Get("call_id")

	for _, session := range sessions {
		session.Lock()

		// Apply filters
		if state != "" && string(session.State) != state {
			session.Unlock()
			continue
		}
		if callID != "" && session.CallID != callID {
			session.Unlock()
			continue
		}

		resp := sessionToResponse(session)
		session.Unlock()

		response.Sessions = append(response.Sessions, resp)
	}

	r.jsonResponse(w, http.StatusOK, response)
}

// CreateSessionRequest represents a create session request
type CreateSessionRequest struct {
	CallID   string            `json:"call_id"`
	FromTag  string            `json:"from_tag"`
	ToTag    string            `json:"to_tag,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// createSession creates a new session
func (r *Router) createSession(w http.ResponseWriter, req *http.Request) {
	var createReq CreateSessionRequest
	if err := json.NewDecoder(req.Body).Decode(&createReq); err != nil {
		r.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if createReq.CallID == "" || createReq.FromTag == "" {
		r.errorResponse(w, http.StatusBadRequest, "call_id and from_tag are required")
		return
	}

	// Create session
	session := r.sessionRegistry.CreateSession(createReq.CallID, createReq.FromTag)

	// Set metadata
	for k, v := range createReq.Metadata {
		session.SetMetadata(k, v)
	}

	session.Lock()
	resp := sessionToResponse(session)
	session.Unlock()

	r.jsonResponse(w, http.StatusCreated, resp)
}

// handleSessionByID handles GET/DELETE /api/v1/sessions/{id}
func (r *Router) handleSessionByID(w http.ResponseWriter, req *http.Request) {
	// Extract session ID from path
	path := req.URL.Path
	sessionID := strings.TrimPrefix(path, "/api/v1/sessions/")
	sessionID = strings.TrimSuffix(sessionID, "/")

	if sessionID == "" {
		r.errorResponse(w, http.StatusBadRequest, "session ID required")
		return
	}

	switch req.Method {
	case http.MethodGet:
		r.getSession(w, req, sessionID)
	case http.MethodDelete:
		r.deleteSession(w, req, sessionID)
	default:
		r.errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// getSession returns a single session
func (r *Router) getSession(w http.ResponseWriter, req *http.Request, sessionID string) {
	session, ok := r.sessionRegistry.GetSession(sessionID)
	if !ok {
		r.errorResponse(w, http.StatusNotFound, "session not found")
		return
	}

	session.Lock()
	resp := sessionToResponse(session)
	session.Unlock()

	r.jsonResponse(w, http.StatusOK, resp)
}

// deleteSession terminates and removes a session
func (r *Router) deleteSession(w http.ResponseWriter, req *http.Request, sessionID string) {
	_, ok := r.sessionRegistry.GetSession(sessionID)
	if !ok {
		r.errorResponse(w, http.StatusNotFound, "session not found")
		return
	}

	// Update state to terminated
	_ = r.sessionRegistry.UpdateSessionState(sessionID, string(internal.SessionStateTerminated))

	// Delete session
	if err := r.sessionRegistry.DeleteSession(sessionID); err != nil {
		r.errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	r.jsonResponse(w, http.StatusOK, SuccessResponse{
		Success: true,
		Message: "session deleted",
	})
}

// handleActiveCalls handles GET /api/v1/active-calls
func (r *Router) handleActiveCalls(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessions := r.sessionRegistry.ListSessions()

	activeCalls := make([]SessionResponse, 0)
	for _, session := range sessions {
		session.Lock()
		if session.State == internal.SessionStateActive {
			activeCalls = append(activeCalls, sessionToResponse(session))
		}
		session.Unlock()
	}

	r.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"active_calls": activeCalls,
		"count":        len(activeCalls),
	})
}

// handleStreams handles GET /api/v1/streams
func (r *Router) handleStreams(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessions := r.sessionRegistry.ListSessions()

	streams := make([]map[string]interface{}, 0)
	for _, session := range sessions {
		session.Lock()

		if session.State == internal.SessionStateActive {
			if session.CallerLeg != nil {
				stream := map[string]interface{}{
					"session_id":   session.ID,
					"call_id":      session.CallID,
					"direction":    "caller",
					"tag":          session.CallerLeg.Tag,
					"ssrc":         session.CallerLeg.SSRC,
					"local_ip":     session.CallerLeg.LocalIP.String(),
					"local_port":   session.CallerLeg.LocalPort,
					"remote_ip":    session.CallerLeg.IP.String(),
					"remote_port":  session.CallerLeg.Port,
					"media_type":   session.CallerLeg.MediaType,
					"packets_sent": session.CallerLeg.PacketsSent,
					"packets_recv": session.CallerLeg.PacketsRecv,
				}
				streams = append(streams, stream)
			}

			if session.CalleeLeg != nil {
				stream := map[string]interface{}{
					"session_id":   session.ID,
					"call_id":      session.CallID,
					"direction":    "callee",
					"tag":          session.CalleeLeg.Tag,
					"ssrc":         session.CalleeLeg.SSRC,
					"local_ip":     session.CalleeLeg.LocalIP.String(),
					"local_port":   session.CalleeLeg.LocalPort,
					"remote_ip":    session.CalleeLeg.IP.String(),
					"remote_port":  session.CalleeLeg.Port,
					"media_type":   session.CalleeLeg.MediaType,
					"packets_sent": session.CalleeLeg.PacketsSent,
					"packets_recv": session.CalleeLeg.PacketsRecv,
				}
				streams = append(streams, stream)
			}
		}

		session.Unlock()
	}

	r.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"streams": streams,
		"count":   len(streams),
	})
}

// sessionToResponse converts a MediaSession to SessionResponse
func sessionToResponse(session *internal.MediaSession) SessionResponse {
	resp := SessionResponse{
		ID:        session.ID,
		CallID:    session.CallID,
		FromTag:   session.FromTag,
		ToTag:     session.ToTag,
		State:     string(session.State),
		CreatedAt: session.CreatedAt,
		UpdatedAt: session.UpdatedAt,
		Flags:     session.Flags,
		Metadata:  session.Metadata,
	}

	// Calculate duration
	if session.State == internal.SessionStateActive && !session.Stats.ConnectTime.IsZero() {
		resp.Duration = time.Since(session.Stats.ConnectTime).Seconds()
	} else if session.Stats.Duration > 0 {
		resp.Duration = session.Stats.Duration.Seconds()
	}

	// Add caller leg
	if session.CallerLeg != nil {
		resp.CallerLeg = legToResponse(session.CallerLeg)
	}

	// Add callee leg
	if session.CalleeLeg != nil {
		resp.CalleeLeg = legToResponse(session.CalleeLeg)
	}

	// Add stats
	if session.Stats != nil {
		resp.Stats = &SessionStatsResp{
			StartTime:      session.Stats.StartTime,
			ConnectTime:    session.Stats.ConnectTime,
			Duration:       session.Stats.Duration.Seconds(),
			PacketLossRate: session.Stats.PacketLossRate,
			AvgJitter:      session.Stats.AvgJitter * 1000, // Convert to ms
			MaxJitter:      session.Stats.MaxJitter * 1000,
			RTT:            session.Stats.RTT * 1000,
			MOS:            session.Stats.MOS,
		}
	}

	return resp
}

// legToResponse converts a CallLeg to LegResponse
func legToResponse(leg *internal.CallLeg) *LegResponse {
	codecs := make([]string, len(leg.Codecs))
	for i, c := range leg.Codecs {
		codecs[i] = c.Name
	}

	var remoteIP, localIP string
	if leg.IP != nil {
		remoteIP = leg.IP.String()
	}
	if leg.LocalIP != nil {
		localIP = leg.LocalIP.String()
	}

	return &LegResponse{
		Tag:          leg.Tag,
		IP:           remoteIP,
		Port:         leg.Port,
		LocalIP:      localIP,
		LocalPort:    leg.LocalPort,
		MediaType:    string(leg.MediaType),
		Transport:    string(leg.Transport),
		SSRC:         leg.SSRC,
		Codecs:       codecs,
		PacketsSent:  leg.PacketsSent,
		PacketsRecv:  leg.PacketsRecv,
		BytesSent:    leg.BytesSent,
		BytesRecv:    leg.BytesRecv,
		LastActivity: leg.LastActivity.Format(time.RFC3339),
	}
}
