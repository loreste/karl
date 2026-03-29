package ng_protocol

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics for NG protocol
var (
	ngCommandsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "karl_ng_commands_total",
			Help: "Total number of NG protocol commands processed",
		},
		[]string{"command", "result"},
	)

	ngCommandDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "karl_ng_command_duration_seconds",
			Help:    "Duration of NG protocol command processing",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"command"},
	)

	ngActiveCalls = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "karl_ng_active_calls",
			Help: "Number of currently active calls",
		},
	)
)

// CommandHandler is a function that handles an NG protocol command
type CommandHandler func(req *NGRequest) (*NGResponse, error)

// CommandMiddleware is middleware that wraps a command handler
type CommandMiddleware func(CommandHandler) CommandHandler

// CommandRegistry manages command handlers
type CommandRegistry struct {
	handlers    map[string]CommandHandler
	middlewares []CommandMiddleware
	mu          sync.RWMutex
}

// NewCommandRegistry creates a new command registry
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		handlers:    make(map[string]CommandHandler),
		middlewares: make([]CommandMiddleware, 0),
	}
}

// Register registers a handler for a command
func (r *CommandRegistry) Register(command string, handler CommandHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[command] = handler
}

// Use adds middleware to the registry
func (r *CommandRegistry) Use(middleware CommandMiddleware) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.middlewares = append(r.middlewares, middleware)
}

// GetHandler returns the handler for a command with middleware applied
func (r *CommandRegistry) GetHandler(command string) (CommandHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	handler, ok := r.handlers[command]
	if !ok {
		return nil, false
	}

	// Apply middleware in reverse order
	for i := len(r.middlewares) - 1; i >= 0; i-- {
		handler = r.middlewares[i](handler)
	}

	return handler, true
}

// Handler is the main NG protocol handler
type Handler struct {
	registry     *CommandRegistry
	sessionMgr   SessionManager
	sdpProcessor SDPProcessor
	recorder     RecordingManager
}

// SessionManager interface for session operations
type SessionManager interface {
	CreateSession(callID, fromTag string) Session
	GetSession(sessionID string) (Session, bool)
	GetSessionByTags(callID, fromTag, toTag string) Session
	GetSessionByCallID(callID string) []Session
	DeleteSession(sessionID string) error
	UpdateSessionState(sessionID string, state string) error
	ListSessions() []Session
	GetActiveCount() int
	GetStats() map[string]interface{}
	AllocateMediaPorts(localIP string, minPort, maxPort int) (int, int, interface{}, interface{}, error)
}

// Session interface for session operations
type Session interface {
	GetID() string
	GetCallID() string
	GetFromTag() string
	GetToTag() string
	GetState() string
	GetStats() interface{}
	SetFlag(name string, value bool)
	GetFlag(name string) bool
	SetMetadata(key, value string)
	GetMetadata(key string) string
}

// SDPProcessor interface for SDP operations
type SDPProcessor interface {
	ProcessOffer(sdp string, session Session, flags *ParsedFlags) (string, []StreamInfo, error)
	ProcessAnswer(sdp string, session Session, flags *ParsedFlags) (string, []StreamInfo, error)
}

// RecordingManager interface for recording operations
type RecordingManager interface {
	StartRecording(sessionID string, opts RecordingOptions) (string, error)
	StopRecording(sessionID string) error
	PauseRecording(sessionID string) error
	ResumeRecording(sessionID string) error
	GetRecordingStatus(sessionID string) (string, error)
}

// NewHandler creates a new NG protocol handler
func NewHandler(sessionMgr SessionManager, sdpProcessor SDPProcessor, recorder RecordingManager) *Handler {
	h := &Handler{
		registry:     NewCommandRegistry(),
		sessionMgr:   sessionMgr,
		sdpProcessor: sdpProcessor,
		recorder:     recorder,
	}

	// Add logging middleware
	h.registry.Use(LoggingMiddleware)

	// Add metrics middleware
	h.registry.Use(MetricsMiddleware)

	// Register built-in commands
	h.registerBuiltinCommands()

	return h
}

// registerBuiltinCommands registers the built-in NG protocol commands
func (h *Handler) registerBuiltinCommands() {
	// Ping command
	h.registry.Register(CmdPing, h.handlePing)

	// List command
	h.registry.Register(CmdList, h.handleList)

	// Statistics command
	h.registry.Register(CmdStatistics, h.handleStatistics)
}

// RegisterCommand allows external registration of commands
func (h *Handler) RegisterCommand(command string, handler CommandHandler) {
	h.registry.Register(command, handler)
}

// Handle processes an NG protocol request
func (h *Handler) Handle(req *NGRequest) ([]byte, error) {
	handler, ok := h.registry.GetHandler(req.Command)
	if !ok {
		return ErrorResponse(req.Cookie, ErrReasonUnsupported)
	}

	resp, err := handler(req)
	if err != nil {
		log.Printf("Error handling command %s: %v", req.Command, err)
		return ErrorResponse(req.Cookie, err.Error())
	}

	return BuildResponse(req.Cookie, resp)
}

// handlePing handles the ping command
func (h *Handler) handlePing(req *NGRequest) (*NGResponse, error) {
	return &NGResponse{Result: ResultPong}, nil
}

// handleList handles the list command
func (h *Handler) handleList(req *NGRequest) (*NGResponse, error) {
	sessions := h.sessionMgr.ListSessions()

	calls := make([]interface{}, 0, len(sessions))
	for _, s := range sessions {
		calls = append(calls, map[string]interface{}{
			"call-id":  s.GetCallID(),
			"from-tag": s.GetFromTag(),
			"to-tag":   s.GetToTag(),
			"state":    s.GetState(),
		})
	}

	return &NGResponse{
		Result: ResultOK,
		Extra: map[string]interface{}{
			"calls": calls,
		},
	}, nil
}

// handleStatistics handles the statistics command
func (h *Handler) handleStatistics(req *NGRequest) (*NGResponse, error) {
	stats := h.sessionMgr.GetStats()

	return &NGResponse{
		Result: ResultOK,
		Extra:  stats,
	}, nil
}

// LoggingMiddleware logs command execution
func LoggingMiddleware(next CommandHandler) CommandHandler {
	return func(req *NGRequest) (*NGResponse, error) {
		start := time.Now()
		log.Printf("NG command: %s, call-id: %s, from-tag: %s", req.Command, req.CallID, req.FromTag)

		resp, err := next(req)

		duration := time.Since(start)
		if err != nil {
			log.Printf("NG command %s failed in %v: %v", req.Command, duration, err)
		} else {
			log.Printf("NG command %s completed in %v with result: %s", req.Command, duration, resp.Result)
		}

		return resp, err
	}
}

// MetricsMiddleware records metrics for command execution
func MetricsMiddleware(next CommandHandler) CommandHandler {
	return func(req *NGRequest) (*NGResponse, error) {
		timer := prometheus.NewTimer(ngCommandDuration.WithLabelValues(req.Command))
		defer timer.ObserveDuration()

		resp, err := next(req)

		result := "error"
		if err == nil && resp != nil {
			result = resp.Result
		}
		ngCommandsTotal.WithLabelValues(req.Command, result).Inc()

		return resp, err
	}
}

// ValidationMiddleware validates required parameters
func ValidationMiddleware(requiredParams ...string) CommandMiddleware {
	return func(next CommandHandler) CommandHandler {
		return func(req *NGRequest) (*NGResponse, error) {
			for _, param := range requiredParams {
				switch param {
				case "call-id":
					if req.CallID == "" {
						return &NGResponse{
							Result:      ResultError,
							ErrorReason: fmt.Sprintf("Missing required parameter: %s", param),
						}, nil
					}
				case "from-tag":
					if req.FromTag == "" {
						return &NGResponse{
							Result:      ResultError,
							ErrorReason: fmt.Sprintf("Missing required parameter: %s", param),
						}, nil
					}
				case "sdp":
					if req.SDP == "" {
						return &NGResponse{
							Result:      ResultError,
							ErrorReason: fmt.Sprintf("Missing required parameter: %s", param),
						}, nil
					}
				}
			}
			return next(req)
		}
	}
}

// UpdateActiveCallsMetric updates the active calls gauge
func UpdateActiveCallsMetric(count int) {
	ngActiveCalls.Set(float64(count))
}
