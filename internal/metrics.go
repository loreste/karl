package internal

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Define Prometheus metrics
var (
	rtpPacketsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "karl_rtp_packets_total",
		Help: "Total number of RTP packets processed",
	})

	rtpPacketsDropped = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "karl_rtp_packets_dropped",
		Help: "Total number of RTP packets dropped due to congestion",
	})

	rtpActiveSessions = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "karl_rtp_active_sessions",
		Help: "Current number of active RTP sessions",
	})

	rtpPacketLoss = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "karl_rtp_packet_loss",
		Help: "Current packet loss percentage in RTP streams",
	})

	rtpJitter = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "karl_rtp_jitter",
		Help: "Current jitter (ms) in RTP streams",
	})

	rtpBandwidthUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "karl_rtp_bandwidth_usage",
		Help: "Current RTP bandwidth usage in kbps",
	})
)

// Initialize and register metrics with Prometheus
func InitMetrics() {
	prometheus.MustRegister(rtpPacketsTotal)
	prometheus.MustRegister(rtpPacketsDropped)
	prometheus.MustRegister(rtpActiveSessions)
	prometheus.MustRegister(rtpPacketLoss)
	prometheus.MustRegister(rtpJitter)
	prometheus.MustRegister(rtpBandwidthUsage)

	// Expose metrics endpoint
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		http.ListenAndServe(":9090", nil)
	}()
}

// Update metrics dynamically
func IncrementRTPPackets() {
	rtpPacketsTotal.Inc()
}

func IncrementDroppedPackets() {
	rtpPacketsDropped.Inc()
}

func SetActiveSessions(count int) {
	rtpActiveSessions.Set(float64(count))
}

func SetPacketLoss(loss float64) {
	rtpPacketLoss.Set(loss)
}

func SetJitter(jitter float64) {
	rtpJitter.Set(jitter)
}

func SetBandwidthUsage(bandwidth int) {
	rtpBandwidthUsage.Set(float64(bandwidth))
}
