package internal

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSpanKind_String(t *testing.T) {
	tests := []struct {
		kind     SpanKind
		expected string
	}{
		{SpanKindInternal, "internal"},
		{SpanKindServer, "server"},
		{SpanKindClient, "client"},
		{SpanKindProducer, "producer"},
		{SpanKindConsumer, "consumer"},
		{SpanKind(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.expected {
			t.Errorf("SpanKind(%d).String() = %s, expected %s", tt.kind, got, tt.expected)
		}
	}
}

func TestSpanStatus_String(t *testing.T) {
	tests := []struct {
		status   SpanStatus
		expected string
	}{
		{SpanStatusUnset, "unset"},
		{SpanStatusOK, "ok"},
		{SpanStatusError, "error"},
		{SpanStatus(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.expected {
			t.Errorf("SpanStatus(%d).String() = %s, expected %s", tt.status, got, tt.expected)
		}
	}
}

func TestTraceID_String(t *testing.T) {
	var id TraceID
	id[0] = 0xab
	id[1] = 0xcd

	s := id.String()
	if len(s) != 32 {
		t.Errorf("Expected 32 character hex string, got %d", len(s))
	}
}

func TestSpanID_String(t *testing.T) {
	var id SpanID
	id[0] = 0xab
	id[1] = 0xcd

	s := id.String()
	if len(s) != 16 {
		t.Errorf("Expected 16 character hex string, got %d", len(s))
	}
}

func TestSpanContext_IsValid(t *testing.T) {
	// Empty context is invalid
	var sc SpanContext
	if sc.IsValid() {
		t.Error("Empty SpanContext should be invalid")
	}

	// Context with IDs is valid
	sc.TraceID[0] = 1
	sc.SpanID[0] = 1
	if !sc.IsValid() {
		t.Error("SpanContext with IDs should be valid")
	}
}

func TestDefaultTracerConfig(t *testing.T) {
	config := DefaultTracerConfig()

	if config.ServiceName != "karl-media-server" {
		t.Errorf("Expected ServiceName 'karl-media-server', got %s", config.ServiceName)
	}
	if config.SampleRate != 1.0 {
		t.Errorf("Expected SampleRate 1.0, got %f", config.SampleRate)
	}
	if !config.Enabled {
		t.Error("Expected Enabled to be true")
	}
}

func TestNewTracer(t *testing.T) {
	tracer := NewTracer(nil)

	if tracer == nil {
		t.Fatal("NewTracer returned nil")
	}
	if tracer.config == nil {
		t.Error("Tracer config should not be nil")
	}
}

func TestTracer_Start(t *testing.T) {
	tracer := NewTracer(nil)
	exporter := NewInMemoryExporter()
	tracer.SetExporter(exporter)

	ctx := context.Background()
	ctx, span := tracer.Start(ctx, "test-span")

	if span == nil {
		t.Fatal("Start returned nil span")
	}
	if span.name != "test-span" {
		t.Errorf("Expected name 'test-span', got %s", span.name)
	}

	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Errorf("Expected 1 exported span, got %d", len(spans))
	}
}

func TestTracer_StartWithOptions(t *testing.T) {
	tracer := NewTracer(nil)

	ctx := context.Background()
	ctx, span := tracer.Start(ctx, "test-span",
		WithSpanKind(SpanKindServer),
		WithAttributes(map[string]interface{}{"key": "value"}),
	)

	if span.kind != SpanKindServer {
		t.Errorf("Expected SpanKindServer, got %s", span.kind.String())
	}
	if span.attributes["key"] != "value" {
		t.Errorf("Expected attribute key=value, got %v", span.attributes["key"])
	}

	span.End()
}

func TestTracer_ParentChild(t *testing.T) {
	tracer := NewTracer(nil)
	exporter := NewInMemoryExporter()
	tracer.SetExporter(exporter)

	// Start parent span
	ctx := context.Background()
	ctx, parent := tracer.Start(ctx, "parent")

	// Start child span
	_, child := tracer.Start(ctx, "child")

	// Child should have same trace ID as parent
	if child.context.TraceID != parent.context.TraceID {
		t.Error("Child should have same trace ID as parent")
	}

	// Child parent ID should be parent's span ID
	if child.parentID != parent.context.SpanID {
		t.Error("Child parent ID should be parent's span ID")
	}

	child.End()
	parent.End()
}

func TestSpan_SetAttribute(t *testing.T) {
	tracer := NewTracer(nil)
	_, span := tracer.Start(context.Background(), "test")

	span.SetAttribute("string", "value")
	span.SetAttribute("int", 42)
	span.SetAttribute("bool", true)

	if span.attributes["string"] != "value" {
		t.Error("String attribute not set correctly")
	}
	if span.attributes["int"] != 42 {
		t.Error("Int attribute not set correctly")
	}
	if span.attributes["bool"] != true {
		t.Error("Bool attribute not set correctly")
	}

	span.End()
}

func TestSpan_SetAttributes(t *testing.T) {
	tracer := NewTracer(nil)
	_, span := tracer.Start(context.Background(), "test")

	span.SetAttributes(map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
	})

	if len(span.attributes) < 2 {
		t.Error("Attributes not set correctly")
	}

	span.End()
}

func TestSpan_AddEvent(t *testing.T) {
	tracer := NewTracer(nil)
	_, span := tracer.Start(context.Background(), "test")

	span.AddEvent("test-event", map[string]interface{}{"key": "value"})

	if len(span.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(span.events))
	}
	if span.events[0].Name != "test-event" {
		t.Errorf("Expected event name 'test-event', got %s", span.events[0].Name)
	}

	span.End()
}

func TestSpan_RecordError(t *testing.T) {
	tracer := NewTracer(nil)
	_, span := tracer.Start(context.Background(), "test")

	err := errors.New("test error")
	span.RecordError(err, nil)

	if span.status != SpanStatusError {
		t.Error("Status should be Error after RecordError")
	}
	if len(span.events) != 1 {
		t.Error("Should have 1 exception event")
	}
	if span.events[0].Name != "exception" {
		t.Error("Event should be named 'exception'")
	}

	span.End()
}

func TestSpan_RecordError_Nil(t *testing.T) {
	tracer := NewTracer(nil)
	_, span := tracer.Start(context.Background(), "test")

	span.RecordError(nil, nil)

	if len(span.events) != 0 {
		t.Error("Should not record nil error")
	}

	span.End()
}

func TestSpan_SetStatus(t *testing.T) {
	tracer := NewTracer(nil)
	_, span := tracer.Start(context.Background(), "test")

	span.SetStatus(SpanStatusOK, "success")

	if span.status != SpanStatusOK {
		t.Error("Status not set correctly")
	}
	if span.statusMsg != "success" {
		t.Error("Status message not set correctly")
	}

	span.End()
}

func TestSpan_End(t *testing.T) {
	tracer := NewTracer(nil)
	_, span := tracer.Start(context.Background(), "test")

	if !span.IsRecording() {
		t.Error("Span should be recording before End")
	}

	span.End()

	if span.IsRecording() {
		t.Error("Span should not be recording after End")
	}

	// Double end should be safe
	span.End()
}

func TestSpan_Duration(t *testing.T) {
	tracer := NewTracer(nil)
	_, span := tracer.Start(context.Background(), "test")

	time.Sleep(50 * time.Millisecond)
	span.End()

	duration := span.Duration()
	if duration < 50*time.Millisecond {
		t.Errorf("Duration should be at least 50ms, got %v", duration)
	}
}

func TestSpan_ToMap(t *testing.T) {
	tracer := NewTracer(nil)
	_, span := tracer.Start(context.Background(), "test")
	span.SetAttribute("key", "value")
	span.AddEvent("event", nil)
	span.End()

	m := span.ToMap()

	if m["name"] != "test" {
		t.Error("Name not in map")
	}
	if m["trace_id"] == "" {
		t.Error("trace_id not in map")
	}
	if m["span_id"] == "" {
		t.Error("span_id not in map")
	}
	if m["attributes"] == nil {
		t.Error("attributes not in map")
	}
}

func TestSpanFromContext(t *testing.T) {
	tracer := NewTracer(nil)
	ctx := context.Background()

	// No span in context
	if SpanFromContext(ctx) != nil {
		t.Error("Should return nil for context without span")
	}

	// With span in context
	ctx, span := tracer.Start(ctx, "test")
	extracted := SpanFromContext(ctx)

	if extracted != span {
		t.Error("Should extract same span from context")
	}

	span.End()
}

func TestInMemoryExporter(t *testing.T) {
	exporter := NewInMemoryExporter()

	tracer := NewTracer(nil)
	tracer.SetExporter(exporter)

	_, span1 := tracer.Start(context.Background(), "span1")
	span1.End()

	_, span2 := tracer.Start(context.Background(), "span2")
	span2.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Errorf("Expected 2 spans, got %d", len(spans))
	}

	exporter.Reset()

	spans = exporter.GetSpans()
	if len(spans) != 0 {
		t.Error("Spans should be cleared after Reset")
	}
}

func TestInMemoryExporter_ExportSpans(t *testing.T) {
	exporter := NewInMemoryExporter()

	spans := []*Span{
		{name: "span1"},
		{name: "span2"},
	}
	exporter.ExportSpans(spans)

	if len(exporter.GetSpans()) != 2 {
		t.Error("ExportSpans should add all spans")
	}
}

func TestInMemoryExporter_Shutdown(t *testing.T) {
	exporter := NewInMemoryExporter()
	err := exporter.Shutdown(context.Background())
	if err != nil {
		t.Errorf("Shutdown should not error, got %v", err)
	}
}

func TestLogExporter(t *testing.T) {
	exporter := NewLogExporter(nil)

	tracer := NewTracer(nil)
	tracer.SetExporter(exporter)

	_, span := tracer.Start(context.Background(), "test")
	span.End()

	// Just verify it doesn't panic
	err := exporter.Shutdown(context.Background())
	if err != nil {
		t.Errorf("Shutdown should not error, got %v", err)
	}
}

func TestRatioSampler(t *testing.T) {
	sampler := &ratioSampler{ratio: 0.5}

	// Test determinism - same trace ID should always give same result
	var id TraceID
	id[0] = 0x12
	id[1] = 0x34

	result1 := sampler.ShouldSample(id, "test")
	result2 := sampler.ShouldSample(id, "test")

	if result1 != result2 {
		t.Error("Sampler should be deterministic for same trace ID")
	}

	// Test ratio 1.0 always samples
	sampler100 := &ratioSampler{ratio: 1.0}
	if !sampler100.ShouldSample(id, "test") {
		t.Error("Ratio 1.0 should always sample")
	}

	// Test ratio 0 never samples
	sampler0 := &ratioSampler{ratio: 0}
	if sampler0.ShouldSample(id, "test") {
		t.Error("Ratio 0 should never sample")
	}
}

func TestTracer_Disabled(t *testing.T) {
	config := &TracerConfig{Enabled: false}
	tracer := NewTracer(config)
	exporter := NewInMemoryExporter()
	tracer.SetExporter(exporter)

	_, span := tracer.Start(context.Background(), "test")
	span.End()

	// Should not export when disabled
	if len(exporter.GetSpans()) != 0 {
		t.Error("Should not export spans when tracer is disabled")
	}
}

func TestCallTracer(t *testing.T) {
	tracer := NewTracer(nil)
	exporter := NewInMemoryExporter()
	tracer.SetExporter(exporter)

	ct := NewCallTracer(tracer)

	// Test StartCall
	ctx, span := ct.StartCall(context.Background(), "call-123", "from-tag", "to-tag")
	if span.attributes["call.id"] != "call-123" {
		t.Error("call.id not set")
	}
	span.End()

	// Test StartOffer
	_, offerSpan := ct.StartOffer(ctx, "call-123")
	if offerSpan.attributes["call.operation"] != "offer" {
		t.Error("operation not set to offer")
	}
	offerSpan.End()

	// Test StartAnswer
	_, answerSpan := ct.StartAnswer(ctx, "call-123")
	if answerSpan.attributes["call.operation"] != "answer" {
		t.Error("operation not set to answer")
	}
	answerSpan.End()

	// Test StartDelete
	_, deleteSpan := ct.StartDelete(ctx, "call-123")
	if deleteSpan.attributes["call.operation"] != "delete" {
		t.Error("operation not set to delete")
	}
	deleteSpan.End()

	// Test StartMediaRelay
	_, mediaSpan := ct.StartMediaRelay(ctx, "call-123")
	mediaSpan.End()

	if len(exporter.GetSpans()) != 5 {
		t.Errorf("Expected 5 spans, got %d", len(exporter.GetSpans()))
	}
}

func TestTracer_ConcurrentAccess(t *testing.T) {
	tracer := NewTracer(nil)
	exporter := NewInMemoryExporter()
	tracer.SetExporter(exporter)

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				ctx, span := tracer.Start(context.Background(), "concurrent-test")
				span.SetAttribute("key", "value")
				span.AddEvent("event", nil)
				span.End()
				_ = ctx
			}
		}()
	}

	wg.Wait()

	// Should have created all spans without race conditions
	if tracer.spansCreated.Load() != int64(numGoroutines*100) {
		t.Errorf("Expected %d spans created, got %d", numGoroutines*100, tracer.spansCreated.Load())
	}
}

func TestGetTracer(t *testing.T) {
	t1 := GetTracer()
	if t1 == nil {
		t.Fatal("GetTracer returned nil")
	}

	t2 := GetTracer()
	if t1 != t2 {
		t.Error("GetTracer should return same instance")
	}
}

func TestTracer_GetStats(t *testing.T) {
	tracer := NewTracer(nil)

	_, span := tracer.Start(context.Background(), "test")
	span.End()

	stats := tracer.GetStats()

	if stats["enabled"] != true {
		t.Error("enabled should be true")
	}
	if stats["spans_created"].(int64) != 1 {
		t.Error("spans_created should be 1")
	}
}

func TestRandomIDGenerator(t *testing.T) {
	gen := &randomIDGenerator{}

	id1 := gen.NewTraceID()
	id2 := gen.NewTraceID()

	if id1 == id2 {
		t.Error("Generated trace IDs should be unique")
	}

	spanID1 := gen.NewSpanID()
	spanID2 := gen.NewSpanID()

	if spanID1 == spanID2 {
		t.Error("Generated span IDs should be unique")
	}
}
