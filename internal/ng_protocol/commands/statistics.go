package commands

import (
	"runtime"
	"time"

	"karl/internal"
	ng "karl/internal/ng_protocol"
)

// StatisticsHandler handles the statistics command
type StatisticsHandler struct {
	sessionRegistry *internal.SessionRegistry
	startTime       time.Time
}

// NewStatisticsHandler creates a new statistics handler
func NewStatisticsHandler(registry *internal.SessionRegistry) *StatisticsHandler {
	return &StatisticsHandler{
		sessionRegistry: registry,
		startTime:       time.Now(),
	}
}

// Handle processes a statistics request
func (h *StatisticsHandler) Handle(req *ng.NGRequest) (*ng.NGResponse, error) {
	sessions := h.sessionRegistry.ListSessions()

	// Calculate aggregate stats
	var (
		activeCalls        int
		totalCalls         int
		totalDuration      time.Duration
		totalPacketsSent   uint64
		totalPacketsRecv   uint64
		totalBytesSent     uint64
		totalBytesRecv     uint64
		totalPacketsLost   uint64
		totalJitter        float64
		jitterCount        int
		totalMOS           float64
		mosCount           int
	)

	for _, session := range sessions {
		session.Lock()
		totalCalls++

		if session.State == internal.SessionStateActive {
			activeCalls++
			if !session.Stats.ConnectTime.IsZero() {
				totalDuration += time.Since(session.Stats.ConnectTime)
			}
		} else if session.Stats.Duration > 0 {
			totalDuration += session.Stats.Duration
		}

		if session.CallerLeg != nil {
			totalPacketsSent += session.CallerLeg.PacketsSent
			totalPacketsRecv += session.CallerLeg.PacketsRecv
			totalBytesSent += session.CallerLeg.BytesSent
			totalBytesRecv += session.CallerLeg.BytesRecv
			totalPacketsLost += uint64(session.CallerLeg.PacketsLost)
			if session.CallerLeg.Jitter > 0 {
				totalJitter += session.CallerLeg.Jitter
				jitterCount++
			}
		}

		if session.CalleeLeg != nil {
			totalPacketsSent += session.CalleeLeg.PacketsSent
			totalPacketsRecv += session.CalleeLeg.PacketsRecv
			totalBytesSent += session.CalleeLeg.BytesSent
			totalBytesRecv += session.CalleeLeg.BytesRecv
			totalPacketsLost += uint64(session.CalleeLeg.PacketsLost)
			if session.CalleeLeg.Jitter > 0 {
				totalJitter += session.CalleeLeg.Jitter
				jitterCount++
			}
		}

		if session.Stats.MOS > 0 {
			totalMOS += session.Stats.MOS
			mosCount++
		}

		session.Unlock()
	}

	// Calculate averages
	var avgJitter, avgMOS, avgDuration float64
	if jitterCount > 0 {
		avgJitter = totalJitter / float64(jitterCount)
	}
	if mosCount > 0 {
		avgMOS = totalMOS / float64(mosCount)
	}
	if totalCalls > 0 {
		avgDuration = float64(totalDuration.Seconds()) / float64(totalCalls)
	}

	// Get memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Build response
	response := &ng.NGResponse{
		Result: ng.ResultOK,
		Extra: map[string]interface{}{
			// Call statistics
			"current-calls":       activeCalls,
			"total-calls":         totalCalls,
			"total-duration":      int64(totalDuration.Seconds()),
			"average-duration":    avgDuration,

			// Packet statistics
			"packets-sent":        totalPacketsSent,
			"packets-received":    totalPacketsRecv,
			"bytes-sent":          totalBytesSent,
			"bytes-received":      totalBytesRecv,
			"packets-lost":        totalPacketsLost,

			// Quality metrics
			"average-jitter":      avgJitter,
			"average-mos":         avgMOS,

			// System info
			"uptime":              int64(time.Since(h.startTime).Seconds()),
			"goroutines":          runtime.NumGoroutine(),
			"memory-alloc":        memStats.Alloc,
			"memory-total-alloc":  memStats.TotalAlloc,
			"memory-sys":          memStats.Sys,
			"gc-pause-total":      memStats.PauseTotalNs,
		},
	}

	return response, nil
}

// GetAggregateStats returns aggregate statistics
func (h *StatisticsHandler) GetAggregateStats() *ng.AggregateStats {
	sessions := h.sessionRegistry.ListSessions()

	stats := &ng.AggregateStats{
		CurrentCalls: h.sessionRegistry.GetActiveCount(),
		TotalCalls:   uint64(len(sessions)),
		Uptime:       time.Since(h.startTime),
	}

	for _, session := range sessions {
		session.Lock()

		if session.State == internal.SessionStateActive {
			if !session.Stats.ConnectTime.IsZero() {
				stats.TotalDuration += time.Since(session.Stats.ConnectTime)
			}
		} else {
			stats.TotalDuration += session.Stats.Duration
		}

		if session.CallerLeg != nil {
			stats.PacketsSent += session.CallerLeg.PacketsSent
			stats.PacketsRecv += session.CallerLeg.PacketsRecv
			stats.BytesSent += session.CallerLeg.BytesSent
			stats.BytesRecv += session.CallerLeg.BytesRecv
			stats.PacketsLost += uint64(session.CallerLeg.PacketsLost)
		}

		if session.CalleeLeg != nil {
			stats.PacketsSent += session.CalleeLeg.PacketsSent
			stats.PacketsRecv += session.CalleeLeg.PacketsRecv
			stats.BytesSent += session.CalleeLeg.BytesSent
			stats.BytesRecv += session.CalleeLeg.BytesRecv
			stats.PacketsLost += uint64(session.CalleeLeg.PacketsLost)
		}

		session.Unlock()
	}

	if stats.TotalCalls > 0 {
		stats.AvgCallDuration = stats.TotalDuration / time.Duration(stats.TotalCalls)
	}

	return stats
}

// GetRealtimeStats returns real-time statistics
func (h *StatisticsHandler) GetRealtimeStats() map[string]interface{} {
	sessions := h.sessionRegistry.ListSessions()

	var activeStreams int
	var currentBandwidth uint64

	for _, session := range sessions {
		session.Lock()
		if session.State == internal.SessionStateActive {
			if session.CallerLeg != nil {
				activeStreams++
			}
			if session.CalleeLeg != nil {
				activeStreams++
			}
		}
		session.Unlock()
	}

	return map[string]interface{}{
		"active-calls":      h.sessionRegistry.GetActiveCount(),
		"active-streams":    activeStreams,
		"current-bandwidth": currentBandwidth,
		"uptime":            time.Since(h.startTime).Seconds(),
	}
}
