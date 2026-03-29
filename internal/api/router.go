package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"karl/internal"
	"karl/internal/auth"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// API metrics
var (
	apiRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "karl_api_requests_total",
			Help: "Total API requests",
		},
		[]string{"endpoint", "method", "status"},
	)

	apiRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "karl_api_request_duration_seconds",
			Help:    "API request duration",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"endpoint"},
	)
)

// Router is the main API router
type Router struct {
	config          *internal.Config
	sessionRegistry *internal.SessionRegistry
	authenticator   *auth.Authenticator
	rateLimiter     *auth.RateLimiter

	mux    *http.ServeMux
	server *http.Server
	mu     sync.RWMutex
}

// NewRouter creates a new API router
func NewRouter(config *internal.Config, sessionRegistry *internal.SessionRegistry) *Router {
	r := &Router{
		config:          config,
		sessionRegistry: sessionRegistry,
		mux:             http.NewServeMux(),
	}

	// Initialize authenticator if auth is enabled
	if config.API != nil && config.API.AuthEnabled {
		r.authenticator = auth.NewAuthenticator(config.Database.MySQLDSN)
	}

	// Initialize rate limiter
	rateLimit := 60
	if config.API != nil && config.API.RateLimitPerMin > 0 {
		rateLimit = config.API.RateLimitPerMin
	}
	r.rateLimiter = auth.NewRateLimiter(rateLimit, time.Minute)

	// Register routes
	r.registerRoutes()

	return r
}

// registerRoutes registers all API routes
func (r *Router) registerRoutes() {
	// Health and metrics (no auth)
	r.mux.HandleFunc("/api/v1/health", r.wrap(r.handleHealth, nil))
	r.mux.HandleFunc("/api/v1/metrics", promhttp.Handler().ServeHTTP)

	// Session endpoints
	r.mux.HandleFunc("/api/v1/sessions", r.wrap(r.handleSessions, []string{"session:read", "session:write"}))
	r.mux.HandleFunc("/api/v1/sessions/", r.wrap(r.handleSessionByID, []string{"session:read", "session:delete"}))

	// Statistics endpoints
	r.mux.HandleFunc("/api/v1/stats", r.wrap(r.handleStats, []string{"stats:read"}))
	r.mux.HandleFunc("/api/v1/stats/", r.wrap(r.handleStatsByCallID, []string{"stats:read"}))

	// Recording endpoints
	r.mux.HandleFunc("/api/v1/recording/start", r.wrap(r.handleStartRecording, []string{"recording:write"}))
	r.mux.HandleFunc("/api/v1/recording/stop", r.wrap(r.handleStopRecording, []string{"recording:write"}))
	r.mux.HandleFunc("/api/v1/recordings", r.wrap(r.handleListRecordings, []string{"recording:read"}))
	r.mux.HandleFunc("/api/v1/recordings/", r.wrap(r.handleRecordingByID, []string{"recording:read"}))

	// Real-time endpoints
	r.mux.HandleFunc("/api/v1/active-calls", r.wrap(r.handleActiveCalls, []string{"session:read"}))
	r.mux.HandleFunc("/api/v1/streams", r.wrap(r.handleStreams, []string{"session:read"}))
}

// wrap wraps a handler with middleware
func (r *Router) wrap(handler http.HandlerFunc, requiredPerms []string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()

		// Rate limiting
		clientIP := getClientIP(req)
		if !r.rateLimiter.Allow(clientIP) {
			r.errorResponse(w, http.StatusTooManyRequests, "rate limit exceeded")
			apiRequestsTotal.WithLabelValues(req.URL.Path, req.Method, "429").Inc()
			return
		}

		// Authentication
		if r.authenticator != nil && len(requiredPerms) > 0 {
			apiKey := extractAPIKey(req)
			if apiKey == "" {
				r.errorResponse(w, http.StatusUnauthorized, "missing API key")
				apiRequestsTotal.WithLabelValues(req.URL.Path, req.Method, "401").Inc()
				return
			}

			permissions, err := r.authenticator.ValidateKey(apiKey)
			if err != nil {
				r.errorResponse(w, http.StatusUnauthorized, "invalid API key")
				apiRequestsTotal.WithLabelValues(req.URL.Path, req.Method, "401").Inc()
				return
			}

			// Check required permissions
			for _, perm := range requiredPerms {
				if !hasPermission(permissions, perm) {
					r.errorResponse(w, http.StatusForbidden, "insufficient permissions")
					apiRequestsTotal.WithLabelValues(req.URL.Path, req.Method, "403").Inc()
					return
				}
			}
		}

		// Create response writer wrapper to capture status
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		// Call handler
		handler(rw, req)

		// Record metrics
		duration := time.Since(start)
		apiRequestDuration.WithLabelValues(req.URL.Path).Observe(duration.Seconds())
		apiRequestsTotal.WithLabelValues(req.URL.Path, req.Method, fmt.Sprintf("%d", rw.status)).Inc()

		// Log request
		log.Printf("API %s %s %d %v", req.Method, req.URL.Path, rw.status, duration)
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// extractAPIKey extracts API key from request
func extractAPIKey(req *http.Request) string {
	// Check Authorization header
	auth := req.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	// Check X-API-Key header
	if key := req.Header.Get("X-API-Key"); key != "" {
		return key
	}

	// Check query parameter
	return req.URL.Query().Get("api_key")
}

// hasPermission checks if permissions include required permission
func hasPermission(permissions []string, required string) bool {
	// Check for wildcard
	for _, p := range permissions {
		if p == "*" || p == required {
			return true
		}
		// Check category wildcard (e.g., "session:*" matches "session:read")
		if strings.HasSuffix(p, ":*") {
			category := strings.TrimSuffix(p, ":*")
			if strings.HasPrefix(required, category+":") {
				return true
			}
		}
	}
	return false
}

// getClientIP extracts client IP from request
func getClientIP(req *http.Request) string {
	// Check X-Forwarded-For header
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	// Check X-Real-IP header
	if xri := req.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Use remote address
	ip := req.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

// JSON response helpers

// jsonResponse sends a JSON response
func (r *Router) jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			log.Printf("Error encoding JSON response: %v", err)
		}
	}
}

// errorResponse sends an error response
func (r *Router) errorResponse(w http.ResponseWriter, status int, message string) {
	r.jsonResponse(w, status, ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	})
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// SuccessResponse represents a success response
type SuccessResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
}

// Start starts the API server
func (r *Router) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	addr := ":8080"
	if r.config.API != nil && r.config.API.Address != "" {
		addr = r.config.API.Address
	}

	r.server = &http.Server{
		Addr:         addr,
		Handler:      r.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("API server starting on %s", addr)
		if err := r.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("API server error: %v", err)
		}
	}()

	return nil
}

// Stop stops the API server
func (r *Router) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := r.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown API server: %w", err)
	}

	log.Println("API server stopped")
	return nil
}

// SetSessionRegistry sets the session registry (for dependency injection)
func (r *Router) SetSessionRegistry(registry *internal.SessionRegistry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessionRegistry = registry
}

// SetAuthenticator sets the authenticator
func (r *Router) SetAuthenticator(authenticator *auth.Authenticator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.authenticator = authenticator
}
