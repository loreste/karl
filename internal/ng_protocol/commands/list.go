package commands

import (
	"time"

	"karl/internal"
	ng "karl/internal/ng_protocol"
)

// ListHandler handles the list command
type ListHandler struct {
	sessionRegistry *internal.SessionRegistry
}

// NewListHandler creates a new list handler
func NewListHandler(registry *internal.SessionRegistry) *ListHandler {
	return &ListHandler{
		sessionRegistry: registry,
	}
}

// Handle processes a list request
func (h *ListHandler) Handle(req *ng.NGRequest) (*ng.NGResponse, error) {
	sessions := h.sessionRegistry.ListSessions()

	calls := make([]interface{}, 0, len(sessions))
	for _, session := range sessions {
		session.Lock()

		var duration time.Duration
		if session.State == internal.SessionStateActive && !session.Stats.ConnectTime.IsZero() {
			duration = time.Since(session.Stats.ConnectTime)
		} else {
			duration = session.Stats.Duration
		}

		call := map[string]interface{}{
			"call-id":     session.CallID,
			"from-tag":    session.FromTag,
			"to-tag":      session.ToTag,
			"state":       string(session.State),
			"created":     session.CreatedAt.Unix(),
			"last-signal": session.UpdatedAt.Unix(),
			"duration":    int64(duration.Seconds()),
		}

		// Add leg info
		if session.CallerLeg != nil {
			call["caller-ip"] = session.CallerLeg.IP.String()
			call["caller-port"] = session.CallerLeg.Port
		}
		if session.CalleeLeg != nil {
			call["callee-ip"] = session.CalleeLeg.IP.String()
			call["callee-port"] = session.CalleeLeg.Port
		}

		calls = append(calls, call)
		session.Unlock()
	}

	return &ng.NGResponse{
		Result: ng.ResultOK,
		Extra: map[string]interface{}{
			"calls": calls,
			"count": len(calls),
		},
	}, nil
}

// ListActive returns only active calls
func (h *ListHandler) ListActive() []ng.CallListEntry {
	sessions := h.sessionRegistry.ListSessions()
	entries := make([]ng.CallListEntry, 0)

	for _, session := range sessions {
		session.Lock()
		if session.State == internal.SessionStateActive {
			var duration time.Duration
			if !session.Stats.ConnectTime.IsZero() {
				duration = time.Since(session.Stats.ConnectTime)
			}

			entries = append(entries, ng.CallListEntry{
				CallID:     session.CallID,
				FromTag:    session.FromTag,
				ToTag:      session.ToTag,
				Created:    session.CreatedAt,
				LastSignal: session.UpdatedAt,
				State:      string(session.State),
				Duration:   duration,
			})
		}
		session.Unlock()
	}

	return entries
}

// ListByState returns calls in a specific state
func (h *ListHandler) ListByState(state internal.SessionState) []ng.CallListEntry {
	sessions := h.sessionRegistry.ListSessions()
	entries := make([]ng.CallListEntry, 0)

	for _, session := range sessions {
		session.Lock()
		if session.State == state {
			entries = append(entries, ng.CallListEntry{
				CallID:     session.CallID,
				FromTag:    session.FromTag,
				ToTag:      session.ToTag,
				Created:    session.CreatedAt,
				LastSignal: session.UpdatedAt,
				State:      string(session.State),
				Duration:   session.Stats.Duration,
			})
		}
		session.Unlock()
	}

	return entries
}
