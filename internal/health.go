package internal

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// HealthStatus represents the health of a component
type HealthStatus string

const (
	// StatusUp indicates the component is healthy
	StatusUp HealthStatus = "UP"

	// StatusDown indicates the component is unhealthy
	StatusDown HealthStatus = "DOWN"

	// StatusDegraded indicates the component is functioning but degraded
	StatusDegraded HealthStatus = "DEGRADED"
)

// ComponentHealth represents the health of a specific component
type ComponentHealth struct {
	Status      HealthStatus      `json:"status"`
	Details     map[string]string `json:"details,omitempty"`
	Message     string            `json:"message,omitempty"`
	LastChecked time.Time         `json:"lastChecked"`
}

// SystemHealth represents the overall health of the system
type SystemHealth struct {
	Status     HealthStatus               `json:"status"`
	Components map[string]ComponentHealth `json:"components"`
	Version    string                     `json:"version"`
	Uptime     string                     `json:"uptime"`
}

var (
	// healthMutex protects the health state
	healthMutex sync.RWMutex

	// systemHealth stores the current health state
	systemHealth SystemHealth

	// startTime is when the system was started
	startTime time.Time

	// healthChecks is a map of health check functions
	healthChecks map[string]func() ComponentHealth

	// WorkerMetricsGetter is the external access point to worker pool metrics
	WorkerMetricsGetter func() map[string]uint64
)

// Initialize the health system
func init() {
	startTime = time.Now()

	systemHealth = SystemHealth{
		Status:     StatusUp,
		Components: make(map[string]ComponentHealth),
		Version:    ConfigVersion,
	}

	healthChecks = make(map[string]func() ComponentHealth)
}

// RegisterHealthCheck registers a health check for a component
func RegisterHealthCheck(component string, check func() ComponentHealth) {
	healthMutex.Lock()
	defer healthMutex.Unlock()

	healthChecks[component] = check
	log.Printf("Registered health check for component: %s", component)
}

// RunHealthChecks executes all registered health checks
func RunHealthChecks() {
	healthMutex.Lock()
	defer healthMutex.Unlock()

	// Update uptime
	systemHealth.Uptime = time.Since(startTime).Round(time.Second).String()

	// Track overall status
	overallStatus := StatusUp

	// Run all health checks
	for component, check := range healthChecks {
		health := check()
		systemHealth.Components[component] = health

		// Update overall status based on component status
		if health.Status == StatusDown {
			overallStatus = StatusDown
		} else if health.Status == StatusDegraded && overallStatus != StatusDown {
			overallStatus = StatusDegraded
		}
	}

	systemHealth.Status = overallStatus
}

// StartHealthChecker starts a goroutine to periodically run health checks
func StartHealthChecker(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			RunHealthChecks()
		}
	}()

	log.Printf("Started health checker with interval: %s", interval)
}

// HealthHandler creates an HTTP handler for health checks
func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If detailed check is requested, run health checks first
		if r.URL.Query().Get("check") == "true" {
			RunHealthChecks()
		}

		healthMutex.RLock()
		defer healthMutex.RUnlock()

		w.Header().Set("Content-Type", "application/json")

		// Set status code based on health
		if systemHealth.Status == StatusDown {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else if systemHealth.Status == StatusDegraded {
			w.WriteHeader(http.StatusOK) // Still 200 but with degraded status
		} else {
			w.WriteHeader(http.StatusOK)
		}

		// Return full health report
		_ = json.NewEncoder(w).Encode(systemHealth)
	}
}

// SimpleHealthHandler returns a simpler health check endpoint
func SimpleHealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		healthMutex.RLock()
		status := systemHealth.Status
		healthMutex.RUnlock()

		w.Header().Set("Content-Type", "application/json")

		if status == StatusDown {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"DOWN"}`))
		} else if status == StatusDegraded {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"DEGRADED"}`))
		} else {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"UP"}`))
		}
	}
}

// GetSystemHealth returns the current system health
func GetSystemHealth() SystemHealth {
	healthMutex.RLock()
	defer healthMutex.RUnlock()

	return systemHealth
}

// CreateComponentHealth creates a component health status
func CreateComponentHealth(status HealthStatus, message string) ComponentHealth {
	return ComponentHealth{
		Status:      status,
		Message:     message,
		LastChecked: time.Now(),
		Details:     make(map[string]string),
	}
}

// Default health checks for common components

// CheckRTPService checks the health of the RTP service
func CheckRTPService() ComponentHealth {
	// Get real metric values from the worker pool metrics
	workerMetrics := WorkerMetricsGetter()
	packetsTotal := float64(workerMetrics["packets_processed"])
	packetsDropped := float64(workerMetrics["packet_errors"])

	if packetsTotal == 0 {
		return CreateComponentHealth(
			StatusUp,
			"RTP service is running but has not processed any packets",
		)
	}

	dropRate := packetsDropped / packetsTotal

	status := StatusUp
	message := "RTP service is healthy"

	if dropRate > 0.1 { // More than 10% packets dropped
		status = StatusDegraded
		message = fmt.Sprintf("High packet drop rate: %.2f%%", dropRate*100)
	}

	if dropRate > 0.3 { // More than 30% packets dropped
		status = StatusDown
		message = fmt.Sprintf("Critical packet drop rate: %.2f%%", dropRate*100)
	}

	health := CreateComponentHealth(status, message)
	health.Details["packetsProcessed"] = fmt.Sprintf("%.0f", packetsTotal)
	health.Details["packetsDropped"] = fmt.Sprintf("%.0f", packetsDropped)
	health.Details["dropRate"] = fmt.Sprintf("%.2f%%", dropRate*100)

	return health
}

// CheckSIPRegistration checks the health of SIP registration
func CheckSIPRegistration() ComponentHealth {
	registrationStatusLock.RLock()
	defer registrationStatusLock.RUnlock()

	// Get config to check which proxies should be registered
	configMutex.RLock()
	if config == nil {
		configMutex.RUnlock()
		return CreateComponentHealth(StatusDown, "Config not loaded")
	}
	opensipsAddr := fmt.Sprintf("%s:%d", config.Integration.OpenSIPSIp, config.Integration.OpenSIPSPort)
	kamailioAddr := fmt.Sprintf("%s:%d", config.Integration.KamailioIp, config.Integration.KamailioPort)
	configMutex.RUnlock()

	// Check if we're registered with the SIP proxies
	opensipsRegistered := registrationStatus[opensipsAddr]
	kamailioRegistered := registrationStatus[kamailioAddr]

	health := CreateComponentHealth(StatusUp, "SIP registrations active")
	health.Details["opensips"] = fmt.Sprintf("%v", opensipsRegistered)
	health.Details["kamailio"] = fmt.Sprintf("%v", kamailioRegistered)

	if !opensipsRegistered && !kamailioRegistered {
		health.Status = StatusDown
		health.Message = "Not registered with any SIP proxy"
		return health
	}

	if !opensipsRegistered || !kamailioRegistered {
		health.Status = StatusDegraded
		health.Message = "Partially registered with SIP proxies"
	}

	return health
}

// RegisterDefaultHealthChecks registers the default health checks
func RegisterDefaultHealthChecks() {
	RegisterHealthCheck("rtp", CheckRTPService)
	RegisterHealthCheck("sip", CheckSIPRegistration)
}

// ReadinessState tracks the readiness of the application
type ReadinessState struct {
	Ready           bool
	DatabaseReady   bool
	RedisReady      bool
	NGListenerReady bool
	Message         string
}

var (
	readinessState ReadinessState
	readinessMu    sync.RWMutex

	// Dependency checkers - set by main application
	DatabaseChecker   func() bool
	RedisChecker      func() bool
	NGListenerChecker func() bool
)

// SetReadinessState updates the readiness state
func SetReadinessState(ready bool, message string) {
	readinessMu.Lock()
	defer readinessMu.Unlock()
	readinessState.Ready = ready
	readinessState.Message = message
}

// SetDatabaseReady updates database readiness
func SetDatabaseReady(ready bool) {
	readinessMu.Lock()
	defer readinessMu.Unlock()
	readinessState.DatabaseReady = ready
}

// SetRedisReady updates redis readiness
func SetRedisReady(ready bool) {
	readinessMu.Lock()
	defer readinessMu.Unlock()
	readinessState.RedisReady = ready
}

// SetNGListenerReady updates NG listener readiness
func SetNGListenerReady(ready bool) {
	readinessMu.Lock()
	defer readinessMu.Unlock()
	readinessState.NGListenerReady = ready
}

// LivenessHandler returns a handler for Kubernetes liveness probes
// Liveness checks if the process is running and not deadlocked
func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// For liveness, we just need to verify the process can respond
		// Check if we're not in a deadlock or crash state
		healthMutex.RLock()
		status := systemHealth.Status
		healthMutex.RUnlock()

		if status == StatusDown {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"DOWN","message":"Service is unhealthy"}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","message":"Service is alive"}`))
	}
}

// ReadinessHandler returns a handler for Kubernetes readiness probes
// Readiness checks if the service is ready to accept traffic
func ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check all critical dependencies
		readinessMu.RLock()
		state := readinessState
		readinessMu.RUnlock()

		// Run dynamic checks if checkers are configured
		if DatabaseChecker != nil {
			state.DatabaseReady = DatabaseChecker()
		}
		if RedisChecker != nil {
			state.RedisReady = RedisChecker()
		}
		if NGListenerChecker != nil {
			state.NGListenerReady = NGListenerChecker()
		}

		// Build response
		response := map[string]interface{}{
			"ready": state.Ready,
			"checks": map[string]bool{
				"database":   state.DatabaseReady,
				"redis":      state.RedisReady,
				"nglistener": state.NGListenerReady,
			},
		}

		// Determine overall readiness
		// Note: Redis and Database are optional, so we don't fail if they're not configured
		isReady := state.Ready && state.NGListenerReady

		if !isReady {
			w.WriteHeader(http.StatusServiceUnavailable)
			response["message"] = "Service is not ready to accept traffic"
		} else {
			w.WriteHeader(http.StatusOK)
			response["message"] = "Service is ready"
		}

		_ = json.NewEncoder(w).Encode(response)
	}
}

// StartupHandler returns a handler for Kubernetes startup probes
// Startup checks if the application has finished initialization
func StartupHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		readinessMu.RLock()
		ready := readinessState.Ready
		message := readinessState.Message
		readinessMu.RUnlock()

		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			response := map[string]interface{}{
				"started": false,
				"message": message,
			}
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		w.WriteHeader(http.StatusOK)
		response := map[string]interface{}{
			"started": true,
			"message": "Application has started successfully",
		}
		_ = json.NewEncoder(w).Encode(response)
	}
}

// GetHealthPort returns the health check port from environment or default
func GetHealthPort() string {
	if port := os.Getenv("KARL_HEALTH_PORT"); port != "" {
		return port
	}
	return ":8086"
}

// GetMetricsPort returns the metrics port from environment or default
func GetMetricsPort() string {
	if port := os.Getenv("KARL_METRICS_PORT"); port != "" {
		return port
	}
	return ":9091"
}

// GetAPIPort returns the API port from environment or default
func GetAPIPort() string {
	if port := os.Getenv("KARL_API_PORT"); port != "" {
		return port
	}
	return ":8080"
}
