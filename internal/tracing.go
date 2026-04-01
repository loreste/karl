package internal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// SpanKind represents the type of span
type SpanKind int

const (
	SpanKindInternal SpanKind = iota
	SpanKindServer
	SpanKindClient
	SpanKindProducer
	SpanKindConsumer
)

func (k SpanKind) String() string {
	switch k {
	case SpanKindInternal:
		return "internal"
	case SpanKindServer:
		return "server"
	case SpanKindClient:
		return "client"
	case SpanKindProducer:
		return "producer"
	case SpanKindConsumer:
		return "consumer"
	default:
		return "unknown"
	}
}

// SpanStatus represents the status of a span
type SpanStatus int

const (
	SpanStatusUnset SpanStatus = iota
	SpanStatusOK
	SpanStatusError
)

func (s SpanStatus) String() string {
	switch s {
	case SpanStatusUnset:
		return "unset"
	case SpanStatusOK:
		return "ok"
	case SpanStatusError:
		return "error"
	default:
		return "unknown"
	}
}

// TraceID represents a unique trace identifier
type TraceID [16]byte

// SpanID represents a unique span identifier
type SpanID [8]byte

// String returns hex representation of TraceID
func (t TraceID) String() string {
	return fmt.Sprintf("%x", t[:])
}

// String returns hex representation of SpanID
func (s SpanID) String() string {
	return fmt.Sprintf("%x", s[:])
}

// SpanContext contains identifying trace information
type SpanContext struct {
	TraceID    TraceID
	SpanID     SpanID
	TraceFlags byte
	TraceState string
	Remote     bool
}

// IsValid returns true if the span context is valid
func (sc SpanContext) IsValid() bool {
	return sc.TraceID != TraceID{} && sc.SpanID != SpanID{}
}

// Span represents a unit of work in a trace
type Span struct {
	name       string
	context    SpanContext
	parentID   SpanID
	kind       SpanKind
	startTime  time.Time
	endTime    time.Time
	status     SpanStatus
	statusMsg  string
	attributes map[string]interface{}
	events     []SpanEvent
	links      []SpanLink
	ended      atomic.Bool
	mu         sync.RWMutex
	tracer     *Tracer
}

// SpanEvent represents an event within a span
type SpanEvent struct {
	Name       string
	Timestamp  time.Time
	Attributes map[string]interface{}
}

// SpanLink represents a link to another span
type SpanLink struct {
	Context    SpanContext
	Attributes map[string]interface{}
}

// SetName sets the span name
func (s *Span) SetName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.name = name
}

// SetStatus sets the span status
func (s *Span) SetStatus(status SpanStatus, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
	s.statusMsg = message
}

// SetAttribute sets an attribute on the span
func (s *Span) SetAttribute(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attributes == nil {
		s.attributes = make(map[string]interface{})
	}
	s.attributes[key] = value
}

// SetAttributes sets multiple attributes on the span
func (s *Span) SetAttributes(attrs map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attributes == nil {
		s.attributes = make(map[string]interface{})
	}
	for k, v := range attrs {
		s.attributes[k] = v
	}
}

// AddEvent adds an event to the span
func (s *Span) AddEvent(name string, attrs map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, SpanEvent{
		Name:       name,
		Timestamp:  time.Now(),
		Attributes: attrs,
	})
}

// RecordError records an error as an event
func (s *Span) RecordError(err error, attrs map[string]interface{}) {
	if err == nil {
		return
	}
	if attrs == nil {
		attrs = make(map[string]interface{})
	}
	attrs["exception.type"] = fmt.Sprintf("%T", err)
	attrs["exception.message"] = err.Error()
	s.AddEvent("exception", attrs)
	s.SetStatus(SpanStatusError, err.Error())
}

// End ends the span
func (s *Span) End() {
	if !s.ended.CompareAndSwap(false, true) {
		return // Already ended
	}
	s.mu.Lock()
	s.endTime = time.Now()
	s.mu.Unlock()

	// Only export if tracer is enabled and exporter is set
	if s.tracer != nil && s.tracer.config.Enabled && s.tracer.exporter != nil {
		s.tracer.exporter.ExportSpan(s)
	}
}

// SpanContext returns the span's context
func (s *Span) SpanContext() SpanContext {
	return s.context
}

// IsRecording returns true if the span is recording events
func (s *Span) IsRecording() bool {
	return !s.ended.Load()
}

// Duration returns the span duration
func (s *Span) Duration() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.endTime.IsZero() {
		return time.Since(s.startTime)
	}
	return s.endTime.Sub(s.startTime)
}

// ToMap converts span to a map for export
func (s *Span) ToMap() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events := make([]map[string]interface{}, len(s.events))
	for i, e := range s.events {
		events[i] = map[string]interface{}{
			"name":       e.Name,
			"timestamp":  e.Timestamp.Format(time.RFC3339Nano),
			"attributes": e.Attributes,
		}
	}

	return map[string]interface{}{
		"name":        s.name,
		"trace_id":    s.context.TraceID.String(),
		"span_id":     s.context.SpanID.String(),
		"parent_id":   s.parentID.String(),
		"kind":        s.kind.String(),
		"start_time":  s.startTime.Format(time.RFC3339Nano),
		"end_time":    s.endTime.Format(time.RFC3339Nano),
		"duration_ms": s.Duration().Milliseconds(),
		"status":      s.status.String(),
		"status_msg":  s.statusMsg,
		"attributes":  s.attributes,
		"events":      events,
	}
}

// TracerConfig holds tracer configuration
type TracerConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	SampleRate     float64
	Enabled        bool
}

// DefaultTracerConfig returns sensible defaults
func DefaultTracerConfig() *TracerConfig {
	return &TracerConfig{
		ServiceName:    "karl-media-server",
		ServiceVersion: "1.0.0",
		Environment:    "production",
		SampleRate:     1.0,
		Enabled:        true,
	}
}

// Tracer creates spans for distributed tracing
type Tracer struct {
	config   *TracerConfig
	exporter SpanExporter
	idGen    IDGenerator
	sampler  Sampler

	// Metrics
	spansCreated atomic.Int64
	spansExported atomic.Int64
}

// SpanExporter exports spans to a backend
type SpanExporter interface {
	ExportSpan(span *Span)
	ExportSpans(spans []*Span)
	Shutdown(ctx context.Context) error
}

// IDGenerator generates trace and span IDs
type IDGenerator interface {
	NewTraceID() TraceID
	NewSpanID() SpanID
}

// Sampler decides whether to sample a trace
type Sampler interface {
	ShouldSample(traceID TraceID, name string) bool
}

// NewTracer creates a new tracer
func NewTracer(config *TracerConfig) *Tracer {
	if config == nil {
		config = DefaultTracerConfig()
	}

	return &Tracer{
		config:  config,
		idGen:   &randomIDGenerator{},
		sampler: &ratioSampler{ratio: config.SampleRate},
	}
}

// SetExporter sets the span exporter
func (t *Tracer) SetExporter(exporter SpanExporter) {
	t.exporter = exporter
}

// Start starts a new span
func (t *Tracer) Start(ctx context.Context, name string, opts ...SpanOption) (context.Context, *Span) {
	if !t.config.Enabled {
		return ctx, &Span{name: name, tracer: t}
	}

	options := &spanOptions{kind: SpanKindInternal}
	for _, opt := range opts {
		opt(options)
	}

	// Get parent span from context
	var parentCtx SpanContext
	var parentID SpanID
	if parent := SpanFromContext(ctx); parent != nil {
		parentCtx = parent.SpanContext()
		parentID = parentCtx.SpanID
	}

	// Generate IDs
	var traceID TraceID
	if parentCtx.IsValid() {
		traceID = parentCtx.TraceID
	} else {
		traceID = t.idGen.NewTraceID()
	}
	spanID := t.idGen.NewSpanID()

	// Check sampling
	if !t.sampler.ShouldSample(traceID, name) && !parentCtx.IsValid() {
		return ctx, &Span{name: name, tracer: t}
	}

	span := &Span{
		name:      name,
		context:   SpanContext{TraceID: traceID, SpanID: spanID},
		parentID:  parentID,
		kind:      options.kind,
		startTime: time.Now(),
		tracer:    t,
	}

	if options.attributes != nil {
		span.attributes = options.attributes
	}

	// Add service attributes
	span.SetAttribute("service.name", t.config.ServiceName)
	span.SetAttribute("service.version", t.config.ServiceVersion)
	span.SetAttribute("deployment.environment", t.config.Environment)

	t.spansCreated.Add(1)

	return ContextWithSpan(ctx, span), span
}

// GetStats returns tracer statistics
func (t *Tracer) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":        t.config.Enabled,
		"service_name":   t.config.ServiceName,
		"sample_rate":    t.config.SampleRate,
		"spans_created":  t.spansCreated.Load(),
		"spans_exported": t.spansExported.Load(),
	}
}

// SpanOption configures a span
type SpanOption func(*spanOptions)

type spanOptions struct {
	kind       SpanKind
	attributes map[string]interface{}
	links      []SpanLink
}

// WithSpanKind sets the span kind
func WithSpanKind(kind SpanKind) SpanOption {
	return func(o *spanOptions) {
		o.kind = kind
	}
}

// WithAttributes sets initial attributes
func WithAttributes(attrs map[string]interface{}) SpanOption {
	return func(o *spanOptions) {
		o.attributes = attrs
	}
}

// Context key for spans
type spanContextKey struct{}

// ContextWithSpan returns a context with the span attached
func ContextWithSpan(ctx context.Context, span *Span) context.Context {
	return context.WithValue(ctx, spanContextKey{}, span)
}

// SpanFromContext extracts a span from the context
func SpanFromContext(ctx context.Context) *Span {
	if span, ok := ctx.Value(spanContextKey{}).(*Span); ok {
		return span
	}
	return nil
}

// randomIDGenerator generates random IDs
type randomIDGenerator struct {
	counter atomic.Uint64
}

func (g *randomIDGenerator) NewTraceID() TraceID {
	var id TraceID
	now := time.Now().UnixNano()
	counter := g.counter.Add(1)
	// Simple ID generation - in production would use crypto/rand
	id[0] = byte(now >> 56)
	id[1] = byte(now >> 48)
	id[2] = byte(now >> 40)
	id[3] = byte(now >> 32)
	id[4] = byte(now >> 24)
	id[5] = byte(now >> 16)
	id[6] = byte(now >> 8)
	id[7] = byte(now)
	id[8] = byte(counter >> 56)
	id[9] = byte(counter >> 48)
	id[10] = byte(counter >> 40)
	id[11] = byte(counter >> 32)
	id[12] = byte(counter >> 24)
	id[13] = byte(counter >> 16)
	id[14] = byte(counter >> 8)
	id[15] = byte(counter)
	return id
}

func (g *randomIDGenerator) NewSpanID() SpanID {
	var id SpanID
	now := time.Now().UnixNano()
	counter := g.counter.Add(1)
	id[0] = byte(now >> 24)
	id[1] = byte(now >> 16)
	id[2] = byte(now >> 8)
	id[3] = byte(now)
	id[4] = byte(counter >> 24)
	id[5] = byte(counter >> 16)
	id[6] = byte(counter >> 8)
	id[7] = byte(counter)
	return id
}

// ratioSampler samples based on a ratio
type ratioSampler struct {
	ratio   float64
	counter atomic.Uint64
}

func (s *ratioSampler) ShouldSample(traceID TraceID, name string) bool {
	if s.ratio >= 1.0 {
		return true
	}
	if s.ratio <= 0 {
		return false
	}
	// Simple deterministic sampling based on trace ID
	val := uint64(traceID[0])<<56 | uint64(traceID[1])<<48 |
		uint64(traceID[2])<<40 | uint64(traceID[3])<<32 |
		uint64(traceID[4])<<24 | uint64(traceID[5])<<16 |
		uint64(traceID[6])<<8 | uint64(traceID[7])
	threshold := uint64(s.ratio * float64(^uint64(0)))
	return val < threshold
}

// InMemoryExporter stores spans in memory for testing
type InMemoryExporter struct {
	spans []*Span
	mu    sync.Mutex
}

// NewInMemoryExporter creates a new in-memory exporter
func NewInMemoryExporter() *InMemoryExporter {
	return &InMemoryExporter{
		spans: make([]*Span, 0),
	}
}

// ExportSpan exports a single span
func (e *InMemoryExporter) ExportSpan(span *Span) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = append(e.spans, span)
}

// ExportSpans exports multiple spans
func (e *InMemoryExporter) ExportSpans(spans []*Span) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = append(e.spans, spans...)
}

// Shutdown shuts down the exporter
func (e *InMemoryExporter) Shutdown(ctx context.Context) error {
	return nil
}

// GetSpans returns all exported spans
func (e *InMemoryExporter) GetSpans() []*Span {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make([]*Span, len(e.spans))
	copy(result, e.spans)
	return result
}

// Reset clears all spans
func (e *InMemoryExporter) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = e.spans[:0]
}

// LogExporter exports spans to the structured logger
type LogExporter struct {
	logger *StructuredLogger
}

// NewLogExporter creates a new log exporter
func NewLogExporter(logger *StructuredLogger) *LogExporter {
	if logger == nil {
		logger = GetStructuredLogger()
	}
	return &LogExporter{logger: logger}
}

// ExportSpan exports a span to logs
func (e *LogExporter) ExportSpan(span *Span) {
	e.logger.Debug("span", span.ToMap())
}

// ExportSpans exports multiple spans
func (e *LogExporter) ExportSpans(spans []*Span) {
	for _, span := range spans {
		e.ExportSpan(span)
	}
}

// Shutdown shuts down the exporter
func (e *LogExporter) Shutdown(ctx context.Context) error {
	return nil
}

// CallTracer provides tracing specifically for call operations
type CallTracer struct {
	tracer *Tracer
}

// NewCallTracer creates a new call tracer
func NewCallTracer(tracer *Tracer) *CallTracer {
	return &CallTracer{tracer: tracer}
}

// StartCall starts a trace for a call
func (ct *CallTracer) StartCall(ctx context.Context, callID, fromTag, toTag string) (context.Context, *Span) {
	ctx, span := ct.tracer.Start(ctx, "call.process", WithSpanKind(SpanKindServer))
	span.SetAttributes(map[string]interface{}{
		"call.id":       callID,
		"call.from_tag": fromTag,
		"call.to_tag":   toTag,
	})
	return ctx, span
}

// StartOffer starts a trace for an offer operation
func (ct *CallTracer) StartOffer(ctx context.Context, callID string) (context.Context, *Span) {
	ctx, span := ct.tracer.Start(ctx, "call.offer", WithSpanKind(SpanKindServer))
	span.SetAttribute("call.id", callID)
	span.SetAttribute("call.operation", "offer")
	return ctx, span
}

// StartAnswer starts a trace for an answer operation
func (ct *CallTracer) StartAnswer(ctx context.Context, callID string) (context.Context, *Span) {
	ctx, span := ct.tracer.Start(ctx, "call.answer", WithSpanKind(SpanKindServer))
	span.SetAttribute("call.id", callID)
	span.SetAttribute("call.operation", "answer")
	return ctx, span
}

// StartDelete starts a trace for a delete operation
func (ct *CallTracer) StartDelete(ctx context.Context, callID string) (context.Context, *Span) {
	ctx, span := ct.tracer.Start(ctx, "call.delete", WithSpanKind(SpanKindServer))
	span.SetAttribute("call.id", callID)
	span.SetAttribute("call.operation", "delete")
	return ctx, span
}

// StartMediaRelay starts a trace for media relay
func (ct *CallTracer) StartMediaRelay(ctx context.Context, callID string) (context.Context, *Span) {
	ctx, span := ct.tracer.Start(ctx, "media.relay", WithSpanKind(SpanKindInternal))
	span.SetAttribute("call.id", callID)
	return ctx, span
}

// Global tracer
var (
	globalTracer     *Tracer
	globalTracerOnce sync.Once
)

// GetTracer returns the global tracer
func GetTracer() *Tracer {
	globalTracerOnce.Do(func() {
		globalTracer = NewTracer(DefaultTracerConfig())
	})
	return globalTracer
}

// SetGlobalTracer sets the global tracer
func SetGlobalTracer(tracer *Tracer) {
	globalTracer = tracer
}
