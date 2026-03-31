package commands

import (
	"time"

	"karl/internal"
	ng "karl/internal/ng_protocol"
)

// QueryHandler handles the query command
type QueryHandler struct {
	sessionRegistry *internal.SessionRegistry
}

// NewQueryHandler creates a new query handler
func NewQueryHandler(registry *internal.SessionRegistry) *QueryHandler {
	return &QueryHandler{
		sessionRegistry: registry,
	}
}

// Handle processes a query request
func (h *QueryHandler) Handle(req *ng.NGRequest) (*ng.NGResponse, error) {
	// Validate required parameters
	if req.CallID == "" {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonMissingParam + ": call-id",
		}, nil
	}

	// Parse flags for label support
	flags := ng.ParseFlags(req.Flags)

	// Find session by call-id and tags
	var session *internal.MediaSession

	// Try label-based lookup first
	if flags.FromLabel != "" || flags.ToLabel != "" {
		sessions := h.sessionRegistry.GetSessionByCallID(req.CallID)
		for _, s := range sessions {
			if flags.FromLabel != "" {
				if leg := s.GetLegByLabel(flags.FromLabel); leg != nil {
					session = s
					break
				}
			}
			if flags.ToLabel != "" {
				if leg := s.GetLegByLabel(flags.ToLabel); leg != nil {
					session = s
					break
				}
			}
		}
	}

	// Try tag-based lookup
	if session == nil && req.FromTag != "" {
		session = h.sessionRegistry.GetSessionByTags(req.CallID, req.FromTag, req.ToTag)
	}

	if session == nil {
		// Try to find any session with this call-id
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

	// Build response with session info
	session.Lock()
	defer session.Unlock()

	response := &ng.NGResponse{
		Result:     ng.ResultOK,
		CallID:     session.CallID,
		FromTag:    session.FromTag,
		ToTag:      session.ToTag,
		Created:    session.CreatedAt.Unix(),
		LastSignal: session.UpdatedAt.Unix(),
	}

	// Build stats
	stats := &ng.CallStats{
		CreatedAt: session.CreatedAt,
	}

	if !session.Stats.ConnectTime.IsZero() {
		if session.State == internal.SessionStateActive {
			stats.Duration = time.Since(session.Stats.ConnectTime)
		} else {
			stats.Duration = session.Stats.Duration
		}
	}

	// Add caller leg stats
	if session.CallerLeg != nil {
		callerStats := ng.LegStats{
			Tag:         session.CallerLeg.Tag,
			SSRC:        session.CallerLeg.SSRC,
			PacketsSent: session.CallerLeg.PacketsSent,
			PacketsRecv: session.CallerLeg.PacketsRecv,
			BytesSent:   session.CallerLeg.BytesSent,
			BytesRecv:   session.CallerLeg.BytesRecv,
			PacketLoss:  float64(session.CallerLeg.PacketsLost),
			Jitter:      session.CallerLeg.Jitter,
		}
		stats.Legs = append(stats.Legs, callerStats)
		stats.PacketsSent += callerStats.PacketsSent
		stats.PacketsRecv += callerStats.PacketsRecv
		stats.BytesSent += callerStats.BytesSent
		stats.BytesRecv += callerStats.BytesRecv
	}

	// Add callee leg stats
	if session.CalleeLeg != nil {
		calleeStats := ng.LegStats{
			Tag:         session.CalleeLeg.Tag,
			SSRC:        session.CalleeLeg.SSRC,
			PacketsSent: session.CalleeLeg.PacketsSent,
			PacketsRecv: session.CalleeLeg.PacketsRecv,
			BytesSent:   session.CalleeLeg.BytesSent,
			BytesRecv:   session.CalleeLeg.BytesRecv,
			PacketLoss:  float64(session.CalleeLeg.PacketsLost),
			Jitter:      session.CalleeLeg.Jitter,
		}
		stats.Legs = append(stats.Legs, calleeStats)
		stats.PacketsSent += calleeStats.PacketsSent
		stats.PacketsRecv += calleeStats.PacketsRecv
		stats.BytesSent += calleeStats.BytesSent
		stats.BytesRecv += calleeStats.BytesRecv
	}

	// Calculate aggregate quality metrics
	stats.PacketLoss = session.Stats.PacketLossRate
	stats.Jitter = session.Stats.AvgJitter
	stats.RTT = session.Stats.RTT
	stats.MOS = session.Stats.MOS

	response.Stats = stats

	// Build tag info
	response.Tag = make(map[string]ng.TagInfo)

	if session.CallerLeg != nil {
		medias := []ng.MediaInfo{
			{
				Index:     0,
				Type:      string(session.CallerLeg.MediaType),
				Protocol:  string(session.CallerLeg.Transport),
				LocalIP:   session.CallerLeg.LocalIP.String(),
				LocalPort: session.CallerLeg.LocalPort,
			},
		}
		response.Tag[session.FromTag] = ng.TagInfo{
			Tag:        session.FromTag,
			InDialogue: true,
			Created:    session.CreatedAt.Unix(),
			MediaCount: 1,
			Medias:     medias,
		}
	}

	if session.CalleeLeg != nil {
		medias := []ng.MediaInfo{
			{
				Index:     0,
				Type:      string(session.CalleeLeg.MediaType),
				Protocol:  string(session.CalleeLeg.Transport),
				LocalIP:   session.CalleeLeg.LocalIP.String(),
				LocalPort: session.CalleeLeg.LocalPort,
			},
		}
		response.Tag[session.ToTag] = ng.TagInfo{
			Tag:        session.ToTag,
			InDialogue: true,
			Created:    session.CreatedAt.Unix(),
			MediaCount: 1,
			Medias:     medias,
		}
	}

	// Add extra info with rtpengine-compatible fields
	response.Extra = map[string]interface{}{
		"state":    string(session.State),
		"flags":    session.Flags,
		"metadata": session.Metadata,
	}

	// Add session-level rtpengine fields
	if session.TOS >= 0 {
		response.Extra["tos"] = session.TOS
	}
	if session.MediaTimeout >= 0 {
		response.Extra["media-timeout"] = session.MediaTimeout
	}
	if session.SIPREC {
		response.Extra["siprec"] = true
	}
	if session.T38Enabled {
		response.Extra["t38"] = true
	}
	if session.ICELite {
		response.Extra["ice-lite"] = true
	}
	if session.Recording != nil && session.Recording.Active {
		response.Extra["recording"] = true
		response.Extra["recording-file"] = session.Recording.FilePath
	}

	// Add labeled legs info
	if len(session.Legs) > 0 {
		labels := make(map[string]interface{})
		for label, leg := range session.Legs {
			labels[label] = map[string]interface{}{
				"tag":        leg.Tag,
				"interface":  leg.Interface,
				"direction":  leg.Direction,
				"local-ip":   leg.LocalIP.String(),
				"local-port": leg.LocalPort,
			}
		}
		response.Extra["labels"] = labels
	}

	return response, nil
}

// QueryAll returns stats for all sessions
func (h *QueryHandler) QueryAll() (*ng.AggregateStats, error) {
	sessions := h.sessionRegistry.ListSessions()

	stats := &ng.AggregateStats{
		CurrentCalls: h.sessionRegistry.GetActiveCount(),
		TotalCalls:   uint64(len(sessions)),
	}

	var totalDuration time.Duration
	activeSessions := 0

	for _, session := range sessions {
		session.Lock()

		if session.State == internal.SessionStateActive {
			activeSessions++
			if !session.Stats.ConnectTime.IsZero() {
				totalDuration += time.Since(session.Stats.ConnectTime)
			}
		} else if session.Stats.Duration > 0 {
			totalDuration += session.Stats.Duration
		}

		// Aggregate packet/byte counts
		if session.CallerLeg != nil {
			stats.PacketsSent += session.CallerLeg.PacketsSent
			stats.PacketsRecv += session.CallerLeg.PacketsRecv
			stats.BytesSent += session.CallerLeg.BytesSent
			stats.BytesRecv += session.CallerLeg.BytesRecv
		}
		if session.CalleeLeg != nil {
			stats.PacketsSent += session.CalleeLeg.PacketsSent
			stats.PacketsRecv += session.CalleeLeg.PacketsRecv
			stats.BytesSent += session.CalleeLeg.BytesSent
			stats.BytesRecv += session.CalleeLeg.BytesRecv
		}

		session.Unlock()
	}

	stats.TotalDuration = totalDuration
	if stats.TotalCalls > 0 {
		stats.AvgCallDuration = totalDuration / time.Duration(stats.TotalCalls)
	}

	return stats, nil
}
