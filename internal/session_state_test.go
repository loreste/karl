package internal

import (
	"sync"
	"testing"
	"time"
)

// Test types to avoid conflicts with main package types
type testSessionState int

const (
	testStateInitial testSessionState = iota
	testStatePending
	testStateEarlyMedia
	testStateConfirmed
	testStateHold
	testStateTerminated
)

type testLegDirection int

const (
	testLegCaller testLegDirection = iota
	testLegCallee
)

type testLeg struct {
	Tag        string
	Label      string
	Direction  testLegDirection
	LinkedTag  string
	SSRC       uint32
}

type testSessionStats struct {
	PacketsSent     int64
	PacketsReceived int64
	BytesSent       int64
	BytesReceived   int64
}

type testStateMachine struct {
	mu              sync.RWMutex
	callID          string
	state           testSessionState
	legs            map[string]*testLeg
	labels          map[string]string
	ssrcMap         map[uint32]string
	mediaTimeout    time.Duration
	lastActivity    time.Time
	recording       bool
	recordingPaused bool
	stats           testSessionStats
}

func newTestStateMachine(callID string) *testStateMachine {
	return &testStateMachine{
		callID:       callID,
		state:        testStateInitial,
		legs:         make(map[string]*testLeg),
		labels:       make(map[string]string),
		ssrcMap:      make(map[uint32]string),
		mediaTimeout: 30 * time.Second,
		lastActivity: time.Now(),
	}
}

func (sm *testStateMachine) getState() testSessionState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state
}

func (sm *testStateMachine) processOffer(fromTag string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state == testStateInitial {
		sm.state = testStatePending
	}

	if _, exists := sm.legs[fromTag]; !exists {
		sm.legs[fromTag] = &testLeg{
			Tag:       fromTag,
			Direction: testLegCaller,
		}
	}

	sm.lastActivity = time.Now()
	return nil
}

func (sm *testStateMachine) processAnswer(fromTag, toTag string, earlyMedia bool) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state == testStatePending || sm.state == testStateEarlyMedia {
		if earlyMedia {
			sm.state = testStateEarlyMedia
		} else {
			sm.state = testStateConfirmed
		}
	}

	if _, exists := sm.legs[toTag]; !exists {
		sm.legs[toTag] = &testLeg{
			Tag:       toTag,
			Direction: testLegCallee,
		}
	}

	if callerLeg, exists := sm.legs[fromTag]; exists {
		callerLeg.LinkedTag = toTag
	}
	if calleeLeg, exists := sm.legs[toTag]; exists {
		calleeLeg.LinkedTag = fromTag
	}

	sm.lastActivity = time.Now()
	return nil
}

func (sm *testStateMachine) processDelete() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state = testStateTerminated
	return nil
}

func (sm *testStateMachine) processHold() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.state == testStateConfirmed {
		sm.state = testStateHold
	}
	return nil
}

func (sm *testStateMachine) processResume() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.state == testStateHold {
		sm.state = testStateConfirmed
	}
	return nil
}

func (sm *testStateMachine) getLeg(tag string) *testLeg {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.legs[tag]
}

func (sm *testStateMachine) setLabel(tag, label string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if leg, exists := sm.legs[tag]; exists {
		leg.Label = label
		sm.labels[label] = tag
	}
}

func (sm *testStateMachine) getLegByLabel(label string) *testLeg {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if tag, exists := sm.labels[label]; exists {
		return sm.legs[tag]
	}
	return nil
}

func (sm *testStateMachine) setMediaTimeout(d time.Duration) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.mediaTimeout = d
}

func (sm *testStateMachine) updateActivity() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.lastActivity = time.Now()
}

func (sm *testStateMachine) isTimedOut() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return time.Since(sm.lastActivity) > sm.mediaTimeout
}

func (sm *testStateMachine) registerSSRC(tag string, ssrc uint32) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if leg, exists := sm.legs[tag]; exists && leg.SSRC != 0 {
		delete(sm.ssrcMap, leg.SSRC)
	}

	if leg, exists := sm.legs[tag]; exists {
		leg.SSRC = ssrc
		sm.ssrcMap[ssrc] = tag
	}
}

func (sm *testStateMachine) getLegBySSRC(ssrc uint32) *testLeg {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if tag, exists := sm.ssrcMap[ssrc]; exists {
		return sm.legs[tag]
	}
	return nil
}

func (sm *testStateMachine) startRecording() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.recording = true
	sm.recordingPaused = false
	return nil
}

func (sm *testStateMachine) pauseRecording() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.recording {
		sm.recordingPaused = true
	}
	return nil
}

func (sm *testStateMachine) resumeRecording() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.recording {
		sm.recordingPaused = false
	}
	return nil
}

func (sm *testStateMachine) stopRecording() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.recording = false
	sm.recordingPaused = false
	return nil
}

func (sm *testStateMachine) isRecording() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.recording
}

func (sm *testStateMachine) isRecordingPaused() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.recordingPaused
}

func (sm *testStateMachine) incrementPacketCount(sent bool, count int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sent {
		sm.stats.PacketsSent += count
	} else {
		sm.stats.PacketsReceived += count
	}
}

func (sm *testStateMachine) incrementByteCount(sent bool, count int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sent {
		sm.stats.BytesSent += count
	} else {
		sm.stats.BytesReceived += count
	}
}

func (sm *testStateMachine) getStats() testSessionStats {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.stats
}

type testSessionAction struct {
	action     string
	fromTag    string
	toTag      string
	earlyMedia bool
}

func TestSessionStateMachine(t *testing.T) {
	tests := []struct {
		name      string
		actions   []testSessionAction
		wantState testSessionState
	}{
		{
			name: "basic offer-answer flow",
			actions: []testSessionAction{
				{action: "offer", fromTag: "tag1"},
				{action: "answer", fromTag: "tag1", toTag: "tag2"},
			},
			wantState: testStateConfirmed,
		},
		{
			name: "early media",
			actions: []testSessionAction{
				{action: "offer", fromTag: "tag1"},
				{action: "answer", fromTag: "tag1", toTag: "tag2", earlyMedia: true},
			},
			wantState: testStateEarlyMedia,
		},
		{
			name: "delete before answer",
			actions: []testSessionAction{
				{action: "offer", fromTag: "tag1"},
				{action: "delete"},
			},
			wantState: testStateTerminated,
		},
		{
			name: "re-INVITE",
			actions: []testSessionAction{
				{action: "offer", fromTag: "tag1"},
				{action: "answer", fromTag: "tag1", toTag: "tag2"},
				{action: "offer", fromTag: "tag1"},
				{action: "answer", fromTag: "tag1", toTag: "tag2"},
			},
			wantState: testStateConfirmed,
		},
		{
			name: "hold",
			actions: []testSessionAction{
				{action: "offer", fromTag: "tag1"},
				{action: "answer", fromTag: "tag1", toTag: "tag2"},
				{action: "hold"},
			},
			wantState: testStateHold,
		},
		{
			name: "hold and resume",
			actions: []testSessionAction{
				{action: "offer", fromTag: "tag1"},
				{action: "answer", fromTag: "tag1", toTag: "tag2"},
				{action: "hold"},
				{action: "resume"},
			},
			wantState: testStateConfirmed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := newTestStateMachine("test-call-id")

			for _, action := range tt.actions {
				switch action.action {
				case "offer":
					sm.processOffer(action.fromTag)
				case "answer":
					sm.processAnswer(action.fromTag, action.toTag, action.earlyMedia)
				case "delete":
					sm.processDelete()
				case "hold":
					sm.processHold()
				case "resume":
					sm.processResume()
				}
			}

			if sm.getState() != tt.wantState {
				t.Errorf("got state %v, want %v", sm.getState(), tt.wantState)
			}
		})
	}
}

func TestSessionStateConcurrency(t *testing.T) {
	sm := newTestStateMachine("concurrent-test")
	sm.processOffer("tag1")

	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sm.getState()
			_ = sm.getLeg("tag1")
		}()
	}

	wg.Wait()
}

func TestLegStateTracking(t *testing.T) {
	sm := newTestStateMachine("leg-test")

	sm.processOffer("caller-tag")

	callerLeg := sm.getLeg("caller-tag")
	if callerLeg == nil {
		t.Fatal("caller leg not found")
	}
	if callerLeg.Tag != "caller-tag" {
		t.Errorf("caller tag mismatch: got %s", callerLeg.Tag)
	}
	if callerLeg.Direction != testLegCaller {
		t.Errorf("caller direction wrong: got %v", callerLeg.Direction)
	}

	sm.processAnswer("caller-tag", "callee-tag", false)

	calleeLeg := sm.getLeg("callee-tag")
	if calleeLeg == nil {
		t.Fatal("callee leg not found")
	}
	if calleeLeg.Direction != testLegCallee {
		t.Errorf("callee direction wrong: got %v", calleeLeg.Direction)
	}

	if callerLeg.LinkedTag != "callee-tag" {
		t.Errorf("caller not linked to callee")
	}
	if calleeLeg.LinkedTag != "caller-tag" {
		t.Errorf("callee not linked to caller")
	}
}

func TestLabelResolution(t *testing.T) {
	sm := newTestStateMachine("label-test")

	sm.processOffer("tag1")
	sm.setLabel("tag1", "caller-label")
	sm.processAnswer("tag1", "tag2", false)
	sm.setLabel("tag2", "callee-label")

	leg := sm.getLegByLabel("caller-label")
	if leg == nil {
		t.Fatal("leg not found by label")
	}
	if leg.Tag != "tag1" {
		t.Errorf("wrong leg resolved: got tag %s", leg.Tag)
	}

	leg = sm.getLegByLabel("callee-label")
	if leg == nil {
		t.Fatal("callee leg not found by label")
	}
	if leg.Tag != "tag2" {
		t.Errorf("wrong callee leg resolved: got tag %s", leg.Tag)
	}
}

func TestSessionTimeout(t *testing.T) {
	sm := newTestStateMachine("timeout-test")
	sm.setMediaTimeout(100 * time.Millisecond)

	sm.processOffer("tag1")
	sm.processAnswer("tag1", "tag2", false)

	sm.updateActivity()

	if sm.isTimedOut() {
		t.Error("should not be timed out immediately")
	}

	time.Sleep(150 * time.Millisecond)

	if !sm.isTimedOut() {
		t.Error("should be timed out after waiting")
	}
}

func TestSSRCTracking(t *testing.T) {
	sm := newTestStateMachine("ssrc-test")

	sm.processOffer("tag1")
	sm.processAnswer("tag1", "tag2", false)

	sm.registerSSRC("tag1", 0x12345678)

	leg := sm.getLegBySSRC(0x12345678)
	if leg == nil {
		t.Fatal("leg not found by SSRC")
	}
	if leg.Tag != "tag1" {
		t.Errorf("wrong leg: %s", leg.Tag)
	}

	sm.registerSSRC("tag1", 0xDEADBEEF)

	leg = sm.getLegBySSRC(0x12345678)
	if leg != nil {
		t.Error("old SSRC should not resolve")
	}

	leg = sm.getLegBySSRC(0xDEADBEEF)
	if leg == nil {
		t.Fatal("new SSRC should resolve")
	}
}

func TestRecordingState(t *testing.T) {
	sm := newTestStateMachine("recording-test")

	sm.processOffer("tag1")
	sm.processAnswer("tag1", "tag2", false)

	if err := sm.startRecording(); err != nil {
		t.Fatalf("start recording failed: %v", err)
	}

	if !sm.isRecording() {
		t.Error("should be recording")
	}

	if err := sm.pauseRecording(); err != nil {
		t.Fatalf("pause recording failed: %v", err)
	}

	if !sm.isRecordingPaused() {
		t.Error("should be paused")
	}

	if err := sm.resumeRecording(); err != nil {
		t.Fatalf("resume recording failed: %v", err)
	}

	if sm.isRecordingPaused() {
		t.Error("should not be paused")
	}

	if err := sm.stopRecording(); err != nil {
		t.Fatalf("stop recording failed: %v", err)
	}

	if sm.isRecording() {
		t.Error("should not be recording")
	}
}

func TestSessionStatistics(t *testing.T) {
	sm := newTestStateMachine("stats-test")

	sm.processOffer("tag1")
	sm.processAnswer("tag1", "tag2", false)

	sm.incrementPacketCount(true, 100)
	sm.incrementPacketCount(false, 50)
	sm.incrementByteCount(true, 16000)
	sm.incrementByteCount(false, 8000)

	stats := sm.getStats()

	if stats.PacketsSent != 100 {
		t.Errorf("packets sent: got %d, want 100", stats.PacketsSent)
	}
	if stats.PacketsReceived != 50 {
		t.Errorf("packets received: got %d, want 50", stats.PacketsReceived)
	}
	if stats.BytesSent != 16000 {
		t.Errorf("bytes sent: got %d, want 16000", stats.BytesSent)
	}
	if stats.BytesReceived != 8000 {
		t.Errorf("bytes received: got %d, want 8000", stats.BytesReceived)
	}
}
