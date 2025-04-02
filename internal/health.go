package internal

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	Status  HealthStatus     `json:"status"`
	Details map[string]string `json:"details,omitempty"`
	Message string           `json:"message,omitempty"`
	LastChecked time.Time    `json:"lastChecked"`
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
	systemHealth.Uptime = fmt.Sprintf("%s", time.Since(startTime).Round(time.Second))
	
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
		
		for {
			select {
			case <-ticker.C:
				RunHealthChecks()
			}
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
		json.NewEncoder(w).Encode(systemHealth)
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
			w.Write([]byte(`{"status":"DOWN"}`))
		} else if status == StatusDegraded {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"DEGRADED"}`))
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"UP"}`))
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
		Status:     status,
		Message:    message,
		LastChecked: time.Now(),
		Details:    make(map[string]string),
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