package commands

import (
	"log"
	"time"

	"karl/internal"
	ng "karl/internal/ng_protocol"
)

// DeleteHandler handles the delete command
type DeleteHandler struct {
	sessionRegistry *internal.SessionRegistry
	pendingDeletes  map[string]*time.Timer
}

// NewDeleteHandler creates a new delete handler
func NewDeleteHandler(registry *internal.SessionRegistry) *DeleteHandler {
	return &DeleteHandler{
		sessionRegistry: registry,
		pendingDeletes:  make(map[string]*time.Timer),
	}
}

// Handle processes a delete request
func (h *DeleteHandler) Handle(req *ng.NGRequest) (*ng.NGResponse, error) {
	// Validate required parameters
	if req.CallID == "" {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonMissingParam + ": call-id",
		}, nil
	}

	// Parse flags for delete-delay and label support
	flags := ng.ParseFlags(req.Flags)

	// Find session(s) by call-id
	sessions := h.sessionRegistry.GetSessionByCallID(req.CallID)
	if len(sessions) == 0 {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonNotFound,
		}, nil
	}

	// Filter by label if specified
	if flags.FromLabel != "" || flags.ToLabel != "" {
		filtered := make([]*internal.MediaSession, 0)
		for _, s := range sessions {
			if flags.FromLabel != "" {
				if leg := s.GetLegByLabel(flags.FromLabel); leg != nil {
					filtered = append(filtered, s)
					continue
				}
			}
			if flags.ToLabel != "" {
				if leg := s.GetLegByLabel(flags.ToLabel); leg != nil {
					filtered = append(filtered, s)
					continue
				}
			}
		}
		if len(filtered) > 0 {
			sessions = filtered
		}
	}

	// If from-tag is specified, filter to matching sessions
	if req.FromTag != "" {
		filtered := make([]*internal.MediaSession, 0)
		for _, s := range sessions {
			s.Lock()
			if s.FromTag == req.FromTag {
				if req.ToTag == "" || s.ToTag == req.ToTag {
					filtered = append(filtered, s)
				}
			}
			s.Unlock()
		}
		sessions = filtered
	}

	if len(sessions) == 0 {
		return &ng.NGResponse{
			Result:      ng.ResultError,
			ErrorReason: ng.ErrReasonNotFound,
		}, nil
	}

	// Handle delete-delay if specified
	if flags.DeleteDelay > 0 {
		return h.handleDelayedDelete(req, sessions, flags.DeleteDelay)
	}

	// Collect stats before deletion
	var totalStats *ng.CallStats
	for _, session := range sessions {
		// Update session state to terminated
		if err := h.sessionRegistry.UpdateSessionState(session.ID, string(internal.SessionStateTerminated)); err != nil {
			log.Printf("Warning: failed to update session state: %v", err)
		}

		// Collect stats
		session.Lock()
		if session.Stats != nil {
			stats := &ng.CallStats{
				CreatedAt: session.Stats.StartTime,
				Duration:  session.Stats.Duration,
			}

			if session.CallerLeg != nil {
				stats.PacketsSent += session.CallerLeg.PacketsSent
				stats.PacketsRecv += session.CallerLeg.PacketsRecv
				stats.BytesSent += session.CallerLeg.BytesSent
				stats.BytesRecv += session.CallerLeg.BytesRecv

				stats.Legs = append(stats.Legs, ng.LegStats{
					Tag:         session.CallerLeg.Tag,
					SSRC:        session.CallerLeg.SSRC,
					PacketsSent: session.CallerLeg.PacketsSent,
					PacketsRecv: session.CallerLeg.PacketsRecv,
					BytesSent:   session.CallerLeg.BytesSent,
					BytesRecv:   session.CallerLeg.BytesRecv,
					Jitter:      session.CallerLeg.Jitter,
				})
			}

			if session.CalleeLeg != nil {
				stats.PacketsSent += session.CalleeLeg.PacketsSent
				stats.PacketsRecv += session.CalleeLeg.PacketsRecv
				stats.BytesSent += session.CalleeLeg.BytesSent
				stats.BytesRecv += session.CalleeLeg.BytesRecv

				stats.Legs = append(stats.Legs, ng.LegStats{
					Tag:         session.CalleeLeg.Tag,
					SSRC:        session.CalleeLeg.SSRC,
					PacketsSent: session.CalleeLeg.PacketsSent,
					PacketsRecv: session.CalleeLeg.PacketsRecv,
					BytesSent:   session.CalleeLeg.BytesSent,
					BytesRecv:   session.CalleeLeg.BytesRecv,
					Jitter:      session.CalleeLeg.Jitter,
				})
			}

			// Calculate quality metrics
			if stats.PacketsRecv > 0 {
				stats.PacketLoss = session.Stats.PacketLossRate
				stats.Jitter = session.Stats.AvgJitter
				stats.RTT = session.Stats.RTT
				stats.MOS = session.Stats.MOS
			}

			if totalStats == nil {
				totalStats = stats
			}
		}
		session.Unlock()

		// Delete session
		if err := h.sessionRegistry.DeleteSession(session.ID); err != nil {
			log.Printf("Warning: failed to delete session %s: %v", session.ID, err)
		} else {
			log.Printf("Deleted session %s for call-id %s", session.ID, req.CallID)
		}
	}

	response := &ng.NGResponse{
		Result:  ng.ResultOK,
		CallID:  req.CallID,
		FromTag: req.FromTag,
		ToTag:   req.ToTag,
		Stats:   totalStats,
	}

	return response, nil
}

// DeleteByCallID deletes all sessions for a call-id
func (h *DeleteHandler) DeleteByCallID(callID string) error {
	sessions := h.sessionRegistry.GetSessionByCallID(callID)
	for _, session := range sessions {
		_ = h.sessionRegistry.UpdateSessionState(session.ID, string(internal.SessionStateTerminated))
		_ = h.sessionRegistry.DeleteSession(session.ID)
	}
	return nil
}

// DeleteBySessionID deletes a specific session
func (h *DeleteHandler) DeleteBySessionID(sessionID string) error {
	_ = h.sessionRegistry.UpdateSessionState(sessionID, string(internal.SessionStateTerminated))
	return h.sessionRegistry.DeleteSession(sessionID)
}

// handleDelayedDelete schedules a delayed deletion
func (h *DeleteHandler) handleDelayedDelete(req *ng.NGRequest, sessions []*internal.MediaSession, delaySeconds int) (*ng.NGResponse, error) {
	delay := time.Duration(delaySeconds) * time.Second

	for _, session := range sessions {
		sessionID := session.ID

		// Cancel any existing pending delete
		if timer, exists := h.pendingDeletes[sessionID]; exists {
			timer.Stop()
		}

		// Schedule new delayed delete
		h.pendingDeletes[sessionID] = time.AfterFunc(delay, func() {
			_ = h.sessionRegistry.UpdateSessionState(sessionID, string(internal.SessionStateTerminated))
			_ = h.sessionRegistry.DeleteSession(sessionID)
			delete(h.pendingDeletes, sessionID)
			log.Printf("Delayed delete executed for session %s", sessionID)
		})

		log.Printf("Scheduled delayed delete for session %s in %d seconds", sessionID, delaySeconds)
	}

	return &ng.NGResponse{
		Result:  ng.ResultOK,
		CallID:  req.CallID,
		FromTag: req.FromTag,
		ToTag:   req.ToTag,
		Extra: map[string]interface{}{
			"delete-delay": delaySeconds,
			"scheduled":    true,
		},
	}, nil
}

// CancelDelayedDelete cancels a pending delayed delete
func (h *DeleteHandler) CancelDelayedDelete(sessionID string) bool {
	if timer, exists := h.pendingDeletes[sessionID]; exists {
		timer.Stop()
		delete(h.pendingDeletes, sessionID)
		return true
	}
	return false
}
