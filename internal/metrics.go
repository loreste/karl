package internal

import (
	"context"
	"log"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Define Prometheus metrics
var (
	// Mutex for thread-safe metrics operations
	metricsMutex sync.RWMutex
	
	// Server reference for proper shutdown
	metricsServer *http.Server
	
	// System metrics
	goroutinesGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "karl_goroutines",
		Help: "Current number of goroutines",
	})
	
	memoryUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "karl_memory_bytes",
		Help: "Current memory usage in bytes",
	})
	
	// Latency histograms
	operationDurations = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "karl_operation_duration_seconds",
			Help:    "Time taken to complete operations",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
		},
		[]string{"operation"},
	)
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

	// Error metrics
	rtpErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "karl_rtp_errors_total",
			Help: "Total number of errors by type",
		},
		[]string{"type"},
	)

	// Success metrics
	rtpSuccesses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "karl_rtp_successes_total",
			Help: "Total number of successful operations by type",
		},
		[]string{"type"},
	)
)

// Initialize and register metrics with Prometheus
func InitMetrics() {
	// Register all metrics with Prometheus
	prometheus.MustRegister(rtpPacketsTotal)
	prometheus.MustRegister(rtpPacketsDropped)
	prometheus.MustRegister(rtpActiveSessions)
	prometheus.MustRegister(rtpPacketLoss)
	prometheus.MustRegister(rtpJitter)
	prometheus.MustRegister(rtpBandwidthUsage)
	prometheus.MustRegister(rtpErrors)
	prometheus.MustRegister(rtpSuccesses)
	
	// Register system metrics
	prometheus.MustRegister(goroutinesGauge)
	prometheus.MustRegister(memoryUsage)
	prometheus.MustRegister(operationDurations)
	
	// Start system metrics collection
	go collectSystemMetrics()

	// Log metrics initialization
	log.Println("✅ Metrics system initialized")
}

// StartMetricsServer starts the metrics HTTP server with proper timeouts and error handling
func StartMetricsServer(address string) error {
	if address == "" {
		address = ":9091" // Default metrics port
	}
	
	// Create a dedicated mux for metrics
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	
	// Add a health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	
	// Create server with proper timeouts
	server := &http.Server{
		Addr:         address,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	
	// Start server in a goroutine
	go func() {
		log.Printf("🔍 Starting metrics server on %s", address)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("❌ Metrics server error: %v", err)
		}
	}()
	
	return nil
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

// IncrementErrorMetric increments an error counter for specific error types
func IncrementErrorMetric(errorType string) {
	rtpErrors.WithLabelValues(errorType).Inc()
	
	// Log the error based on the log level
	if LogLevel >= LogLevelError {
		log.Printf("ERROR [%s]: Recorded error metric", errorType)
	}
}

// IncrementCounter increments a success counter for specific operation types
func IncrementCounter(operationType string) {
	rtpSuccesses.WithLabelValues(operationType).Inc()
	
	// Log for debug level
	if LogLevel >= LogLevelDebug {
		log.Printf("DEBUG [%s]: Recorded success metric", operationType)
	}
}

// StopMetricsServer gracefully stops the metrics server
func StopMetricsServer() error {
	metricsMutex.Lock()
	defer metricsMutex.Unlock()
	
	if metricsServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		log.Println("🛑 Shutting down metrics server...")
		return metricsServer.Shutdown(ctx)
	}
	return nil
}

// MeasureOperation records the duration of an operation
func MeasureOperation(operation string, start time.Time) {
	duration := time.Since(start).Seconds()
	operationDurations.WithLabelValues(operation).Observe(duration)
}

// collectSystemMetrics periodically updates system metrics
func collectSystemMetrics() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Update goroutine count
			goroutinesGauge.Set(float64(runtime.NumGoroutine()))
			
			// Update memory usage
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)
			memoryUsage.Set(float64(memStats.Alloc))
		}
	}
}
