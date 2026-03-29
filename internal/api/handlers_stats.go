package api

import (
	"net/http"
	"runtime"
	"strings"
	"time"

	"karl/internal"
)

// AggregateStatsResponse represents aggregate statistics
type AggregateStatsResponse struct {
	CurrentCalls     int     `json:"current_calls"`
	TotalCalls       int     `json:"total_calls"`
	TotalDuration    float64 `json:"total_duration_seconds"`
	AvgCallDuration  float64 `json:"avg_call_duration_seconds"`
	PacketsSent      uint64  `json:"packets_sent"`
	PacketsRecv      uint64  `json:"packets_received"`
	BytesSent        uint64  `json:"bytes_sent"`
	BytesRecv        uint64  `json:"bytes_received"`
	PacketsLost      uint64  `json:"packets_lost"`
	AvgJitter        float64 `json:"avg_jitter_ms"`
	AvgMOS           float64 `json:"avg_mos"`
	Uptime           float64 `json:"uptime_seconds"`
	Goroutines       int     `json:"goroutines"`
	MemoryAlloc      uint64  `json:"memory_alloc_bytes"`
	MemorySys        uint64  `json:"memory_sys_bytes"`
}

// CallStatsResponse represents call-specific statistics
type CallStatsResponse struct {
	CallID        string        `json:"call_id"`
	SessionID     string        `json:"session_id"`
	State         string        `json:"state"`
	CreatedAt     time.Time     `json:"created_at"`
	Duration      float64       `json:"duration_seconds"`
	PacketsSent   uint64        `json:"packets_sent"`
	PacketsRecv   uint64        `json:"packets_received"`
	BytesSent     uint64        `json:"bytes_sent"`
	BytesRecv     uint64        `json:"bytes_received"`
	PacketLoss    float64       `json:"packet_loss_percent"`
	Jitter        float64       `json:"jitter_ms"`
	RTT           float64       `json:"rtt_ms"`
	MOS           float64       `json:"mos"`
	Legs          []LegStats    `json:"legs"`
}

// LegStats represents per-leg statistics
type LegStats struct {
	Tag          string  `json:"tag"`
	Direction    string  `json:"direction"`
	SSRC         uint32  `json:"ssrc"`
	PacketsSent  uint64  `json:"packets_sent"`
	PacketsRecv  uint64  `json:"packets_received"`
	BytesSent    uint64  `json:"bytes_sent"`
	BytesRecv    uint64  `json:"bytes_received"`
	PacketsLost  uint32  `json:"packets_lost"`
	Jitter       float64 `json:"jitter_ms"`
}

var serverStartTime = time.Now()

// handleStats handles GET /api/v1/stats
func (r *Router) handleStats(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessions := r.sessionRegistry.ListSessions()

	// Calculate aggregate stats
	stats := AggregateStatsResponse{
		CurrentCalls: r.sessionRegistry.GetActiveCount(),
		TotalCalls:   len(sessions),
		Uptime:       time.Since(serverStartTime).Seconds(),
		Goroutines:   runtime.NumGoroutine(),
	}

	// Get memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	stats.MemoryAlloc = memStats.Alloc
	stats.MemorySys = memStats.Sys

	var (
		totalDuration time.Duration
		totalJitter   float64
		totalMOS      float64
		jitterCount   int
		mosCount      int
	)

	for _, session := range sessions {
		session.Lock()

		// Duration
		if session.State == internal.SessionStateActive {
			if !session.Stats.ConnectTime.IsZero() {
				totalDuration += time.Since(session.Stats.ConnectTime)
			}
		} else {
			totalDuration += session.Stats.Duration
		}

		// Packet/byte counts
		if session.CallerLeg != nil {
			stats.PacketsSent += session.CallerLeg.PacketsSent
			stats.PacketsRecv += session.CallerLeg.PacketsRecv
			stats.BytesSent += session.CallerLeg.BytesSent
			stats.BytesRecv += session.CallerLeg.BytesRecv
			stats.PacketsLost += uint64(session.CallerLeg.PacketsLost)
			if session.CallerLeg.Jitter > 0 {
				totalJitter += session.CallerLeg.Jitter
				jitterCount++
			}
		}

		if session.CalleeLeg != nil {
			stats.PacketsSent += session.CalleeLeg.PacketsSent
			stats.PacketsRecv += session.CalleeLeg.PacketsRecv
			stats.BytesSent += session.CalleeLeg.BytesSent
			stats.BytesRecv += session.CalleeLeg.BytesRecv
			stats.PacketsLost += uint64(session.CalleeLeg.PacketsLost)
			if session.CalleeLeg.Jitter > 0 {
				totalJitter += session.CalleeLeg.Jitter
				jitterCount++
			}
		}

		// MOS
		if session.Stats.MOS > 0 {
			totalMOS += session.Stats.MOS
			mosCount++
		}

		session.Unlock()
	}

	stats.TotalDuration = totalDuration.Seconds()
	if stats.TotalCalls > 0 {
		stats.AvgCallDuration = stats.TotalDuration / float64(stats.TotalCalls)
	}
	if jitterCount > 0 {
		stats.AvgJitter = (totalJitter / float64(jitterCount)) * 1000 // Convert to ms
	}
	if mosCount > 0 {
		stats.AvgMOS = totalMOS / float64(mosCount)
	}

	r.jsonResponse(w, http.StatusOK, stats)
}

// handleStatsByCallID handles GET /api/v1/stats/{call_id}
func (r *Router) handleStatsByCallID(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract call ID from path
	path := req.URL.Path
	callID := strings.TrimPrefix(path, "/api/v1/stats/")
	callID = strings.TrimSuffix(callID, "/")

	if callID == "" {
		r.errorResponse(w, http.StatusBadRequest, "call ID required")
		return
	}

	// Find sessions by call ID
	sessions := r.sessionRegistry.GetSessionByCallID(callID)
	if len(sessions) == 0 {
		r.errorResponse(w, http.StatusNotFound, "call not found")
		return
	}

	// Build response for each session
	responses := make([]CallStatsResponse, 0, len(sessions))

	for _, session := range sessions {
		session.Lock()

		var duration time.Duration
		if session.State == internal.SessionStateActive && !session.Stats.ConnectTime.IsZero() {
			duration = time.Since(session.Stats.ConnectTime)
		} else {
			duration = session.Stats.Duration
		}

		resp := CallStatsResponse{
			CallID:    session.CallID,
			SessionID: session.ID,
			State:     string(session.State),
			CreatedAt: session.CreatedAt,
			Duration:  duration.Seconds(),
			Legs:      make([]LegStats, 0),
		}

		// Add caller leg stats
		if session.CallerLeg != nil {
			resp.PacketsSent += session.CallerLeg.PacketsSent
			resp.PacketsRecv += session.CallerLeg.PacketsRecv
			resp.BytesSent += session.CallerLeg.BytesSent
			resp.BytesRecv += session.CallerLeg.BytesRecv

			resp.Legs = append(resp.Legs, LegStats{
				Tag:         session.CallerLeg.Tag,
				Direction:   "caller",
				SSRC:        session.CallerLeg.SSRC,
				PacketsSent: session.CallerLeg.PacketsSent,
				PacketsRecv: session.CallerLeg.PacketsRecv,
				BytesSent:   session.CallerLeg.BytesSent,
				BytesRecv:   session.CallerLeg.BytesRecv,
				PacketsLost: session.CallerLeg.PacketsLost,
				Jitter:      session.CallerLeg.Jitter * 1000,
			})
		}

		// Add callee leg stats
		if session.CalleeLeg != nil {
			resp.PacketsSent += session.CalleeLeg.PacketsSent
			resp.PacketsRecv += session.CalleeLeg.PacketsRecv
			resp.BytesSent += session.CalleeLeg.BytesSent
			resp.BytesRecv += session.CalleeLeg.BytesRecv

			resp.Legs = append(resp.Legs, LegStats{
				Tag:         session.CalleeLeg.Tag,
				Direction:   "callee",
				SSRC:        session.CalleeLeg.SSRC,
				PacketsSent: session.CalleeLeg.PacketsSent,
				PacketsRecv: session.CalleeLeg.PacketsRecv,
				BytesSent:   session.CalleeLeg.BytesSent,
				BytesRecv:   session.CalleeLeg.BytesRecv,
				PacketsLost: session.CalleeLeg.PacketsLost,
				Jitter:      session.CalleeLeg.Jitter * 1000,
			})
		}

		// Session-level quality metrics
		resp.PacketLoss = session.Stats.PacketLossRate * 100
		resp.Jitter = session.Stats.AvgJitter * 1000
		resp.RTT = session.Stats.RTT * 1000
		resp.MOS = session.Stats.MOS

		session.Unlock()

		responses = append(responses, resp)
	}

	r.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"call_id":  callID,
		"sessions": responses,
	})
}

// handleHealth handles GET /api/v1/health
func (r *Router) handleHealth(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"uptime":    time.Since(serverStartTime).String(),
		"version":   "1.0.0",
	}

	// Add component status
	components := map[string]string{
		"session_registry": "up",
		"api_server":       "up",
	}

	if r.sessionRegistry != nil {
		components["active_sessions"] = "up"
	}

	health["components"] = components

	// Add basic metrics
	health["metrics"] = map[string]interface{}{
		"active_calls": r.sessionRegistry.GetActiveCount(),
		"goroutines":   runtime.NumGoroutine(),
	}

	r.jsonResponse(w, http.StatusOK, health)
}
