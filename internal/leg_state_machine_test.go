package internal

import (
	"testing"
	"time"
)

func TestNewLegStateMachine(t *testing.T) {
	lsm := NewLegStateMachine("tag1", "caller")

	if lsm.Tag != "tag1" {
		t.Errorf("Expected tag 'tag1', got %s", lsm.Tag)
	}
	if lsm.Label != "caller" {
		t.Errorf("Expected label 'caller', got %s", lsm.Label)
	}
	if lsm.State != LegStateInit {
		t.Errorf("Expected state 'init', got %s", lsm.State)
	}
	if lsm.Direction != MediaDirectionSendRecv {
		t.Errorf("Expected direction 'sendrecv', got %s", lsm.Direction)
	}
}

func TestLegStateMachine_OfferAnswerFlow(t *testing.T) {
	lsm := NewLegStateMachine("tag1", "caller")

	// Initial state
	if lsm.GetState() != LegStateInit {
		t.Errorf("Initial state should be init")
	}

	// Process incoming offer
	err := lsm.ProcessOffer("v=0\r\no=- 123 456 IN IP4 1.2.3.4\r\n", "remote")
	if err != nil {
		t.Fatalf("ProcessOffer failed: %v", err)
	}
	if lsm.GetState() != LegStateAnswerPending {
		t.Errorf("State should be answer_pending after offer")
	}

	// Send answer
	err = lsm.SendAnswer("v=0\r\no=- 123 456 IN IP4 5.6.7.8\r\n")
	if err != nil {
		t.Fatalf("SendAnswer failed: %v", err)
	}
	if lsm.GetState() != LegStateEstablished {
		t.Errorf("State should be established after answer")
	}
}

func TestLegStateMachine_SendOfferFlow(t *testing.T) {
	lsm := NewLegStateMachine("tag1", "caller")

	// Send offer
	err := lsm.SendOffer("v=0\r\no=- 123 456 IN IP4 1.2.3.4\r\n")
	if err != nil {
		t.Fatalf("SendOffer failed: %v", err)
	}
	if lsm.GetState() != LegStateOfferPending {
		t.Errorf("State should be offer_pending after sending offer")
	}

	// Receive answer
	err = lsm.ProcessAnswer("v=0\r\no=- 123 456 IN IP4 5.6.7.8\r\n")
	if err != nil {
		t.Fatalf("ProcessAnswer failed: %v", err)
	}
	if lsm.GetState() != LegStateEstablished {
		t.Errorf("State should be established after answer")
	}
}

func TestLegStateMachine_ReInvite(t *testing.T) {
	lsm := NewLegStateMachine("tag1", "caller")

	// Establish call
	lsm.SendOffer("v=0\r\n")
	lsm.ProcessAnswer("v=0\r\n")

	if lsm.GetState() != LegStateEstablished {
		t.Fatal("Call should be established")
	}

	// Re-INVITE: send new offer
	err := lsm.SendOffer("v=0\r\n...new offer...")
	if err != nil {
		t.Fatalf("Re-INVITE SendOffer failed: %v", err)
	}
	if lsm.GetState() != LegStateReOfferPending {
		t.Errorf("State should be reoffer_pending")
	}

	// Receive re-answer
	err = lsm.ProcessAnswer("v=0\r\n...new answer...")
	if err != nil {
		t.Fatalf("Re-INVITE ProcessAnswer failed: %v", err)
	}
	if lsm.GetState() != LegStateEstablished {
		t.Errorf("State should be established after re-answer")
	}
}

func TestLegStateMachine_ReInviteReceived(t *testing.T) {
	lsm := NewLegStateMachine("tag1", "caller")

	// Establish call
	lsm.ProcessOffer("v=0\r\n", "remote")
	lsm.SendAnswer("v=0\r\n")

	if lsm.GetState() != LegStateEstablished {
		t.Fatal("Call should be established")
	}

	// Receive re-INVITE
	err := lsm.ProcessOffer("v=0\r\n...re-invite...", "remote")
	if err != nil {
		t.Fatalf("Re-INVITE ProcessOffer failed: %v", err)
	}
	if lsm.GetState() != LegStateReAnswerPending {
		t.Errorf("State should be reanswer_pending")
	}

	// Send answer to re-INVITE
	err = lsm.SendAnswer("v=0\r\n...re-answer...")
	if err != nil {
		t.Fatalf("Re-INVITE SendAnswer failed: %v", err)
	}
	if lsm.GetState() != LegStateEstablished {
		t.Errorf("State should be established")
	}
}

func TestLegStateMachine_HoldResume(t *testing.T) {
	lsm := NewLegStateMachine("tag1", "caller")

	// Establish call
	lsm.SendOffer("v=0\r\n")
	lsm.ProcessAnswer("v=0\r\na=sendrecv\r\n")

	if lsm.IsOnHold {
		t.Error("Call should not be on hold initially")
	}

	// Put on hold
	lsm.SetHold("local")

	if !lsm.IsOnHold {
		t.Error("Call should be on hold")
	}
	if lsm.GetState() != LegStateHold {
		t.Errorf("State should be hold, got %s", lsm.GetState())
	}
	if lsm.HoldInitiator != "local" {
		t.Errorf("Hold initiator should be 'local'")
	}

	// Resume
	lsm.Resume()

	if lsm.IsOnHold {
		t.Error("Call should not be on hold after resume")
	}
	if lsm.GetState() != LegStateEstablished {
		t.Errorf("State should be established after resume")
	}
}

func TestLegStateMachine_DetectHoldFromSDP(t *testing.T) {
	lsm := NewLegStateMachine("tag1", "caller")

	// Establish with sendonly (remote hold)
	lsm.ProcessOffer("v=0\r\na=sendonly\r\n", "remote")

	if !lsm.IsOnHold {
		t.Error("Should detect hold from sendonly")
	}
	if lsm.GetDirection() != MediaDirectionSendOnly {
		t.Errorf("Direction should be sendonly")
	}
}

func TestLegStateMachine_DetectHoldFromInactive(t *testing.T) {
	lsm := NewLegStateMachine("tag1", "caller")

	// Establish with inactive (full hold)
	lsm.ProcessOffer("v=0\r\na=inactive\r\n", "remote")

	if !lsm.IsOnHold {
		t.Error("Should detect hold from inactive")
	}
	if lsm.GetDirection() != MediaDirectionInactive {
		t.Errorf("Direction should be inactive")
	}
}

func TestLegStateMachine_EarlyMedia(t *testing.T) {
	lsm := NewLegStateMachine("tag1", "caller")

	// Send offer
	lsm.SendOffer("v=0\r\n")

	// Enable early media
	lsm.EnableEarlyMedia()

	if !lsm.EarlyMediaActive {
		t.Error("Early media should be active")
	}

	// Should be able to send media with early media even though not established
	if !lsm.CanSendMedia() {
		t.Error("Should be able to send media with early media")
	}

	// Disable early media
	lsm.DisableEarlyMedia()

	if lsm.EarlyMediaActive {
		t.Error("Early media should be disabled")
	}
}

func TestLegStateMachine_EarlyMediaOnlyInOfferPending(t *testing.T) {
	lsm := NewLegStateMachine("tag1", "caller")

	// Try to enable early media before offer
	lsm.EnableEarlyMedia()

	if lsm.EarlyMediaActive {
		t.Error("Early media should not be enabled before offer")
	}
}

func TestLegStateMachine_CanSendReceiveMedia(t *testing.T) {
	tests := []struct {
		direction MediaDirection
		canSend   bool
		canRecv   bool
	}{
		{MediaDirectionSendRecv, true, true},
		{MediaDirectionSendOnly, true, false},
		{MediaDirectionRecvOnly, false, true},
		{MediaDirectionInactive, false, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.direction), func(t *testing.T) {
			lsm := NewLegStateMachine("tag1", "caller")
			lsm.SendOffer("v=0\r\n")
			lsm.ProcessAnswer("v=0\r\n")
			lsm.SetDirection(tt.direction)

			if lsm.CanSendMedia() != tt.canSend {
				t.Errorf("CanSendMedia() = %v, expected %v", lsm.CanSendMedia(), tt.canSend)
			}
			if lsm.CanReceiveMedia() != tt.canRecv {
				t.Errorf("CanReceiveMedia() = %v, expected %v", lsm.CanReceiveMedia(), tt.canRecv)
			}
		})
	}
}

func TestLegStateMachine_SSRCTracking(t *testing.T) {
	lsm := NewLegStateMachine("tag1", "caller")

	// Set initial SSRC
	lsm.UpdateSSRC(12345, true, "initial")

	if lsm.LocalSSRC != 12345 {
		t.Errorf("LocalSSRC should be 12345")
	}

	// Change SSRC
	lsm.UpdateSSRC(67890, true, "changed")

	if lsm.LocalSSRC != 67890 {
		t.Errorf("LocalSSRC should be 67890")
	}

	// Check history
	if len(lsm.SSRCHistory) != 1 {
		t.Errorf("Should have 1 SSRC change in history, got %d", len(lsm.SSRCHistory))
	}
	if lsm.SSRCHistory[0].OldSSRC != 12345 || lsm.SSRCHistory[0].NewSSRC != 67890 {
		t.Error("SSRC history entry incorrect")
	}
}

func TestLegStateMachine_PayloadTypeMapping(t *testing.T) {
	lsm := NewLegStateMachine("tag1", "caller")

	// Set mapping: remote PT 96 -> local PT 0
	lsm.SetPayloadTypeMapping(96, 0)

	if lsm.MapPayloadType(96) != 0 {
		t.Errorf("MapPayloadType(96) should return 0")
	}
	if lsm.ReverseMapPayloadType(0) != 96 {
		t.Errorf("ReverseMapPayloadType(0) should return 96")
	}

	// Unmapped should return as-is
	if lsm.MapPayloadType(100) != 100 {
		t.Errorf("MapPayloadType(100) should return 100 (unmapped)")
	}
}

func TestLegStateMachine_Terminate(t *testing.T) {
	lsm := NewLegStateMachine("tag1", "caller")

	// Establish call
	lsm.SendOffer("v=0\r\n")
	lsm.ProcessAnswer("v=0\r\n")

	lsm.Terminate()

	if lsm.GetState() != LegStateTerminated {
		t.Errorf("State should be terminated")
	}

	// Should not be able to send media when terminated
	if lsm.CanSendMedia() {
		t.Error("Should not be able to send media when terminated")
	}
}

func TestLegStateMachine_GetStats(t *testing.T) {
	lsm := NewLegStateMachine("tag1", "caller")
	lsm.SendOffer("v=0\r\n")
	lsm.ProcessAnswer("v=0\r\n")
	lsm.SetPayloadTypeMapping(96, 0)
	lsm.UpdateSSRC(12345, true, "initial")

	stats := lsm.GetStats()

	if stats["tag"] != "tag1" {
		t.Errorf("tag should be 'tag1'")
	}
	if stats["label"] != "caller" {
		t.Errorf("label should be 'caller'")
	}
	if stats["state"] != "established" {
		t.Errorf("state should be 'established'")
	}
	if stats["pt_mappings"].(int) != 1 {
		t.Errorf("Should have 1 PT mapping")
	}
	if stats["local_ssrc"].(uint32) != 12345 {
		t.Errorf("local_ssrc should be 12345")
	}
}

func TestLegStateMachine_InvalidTransitions(t *testing.T) {
	// Test invalid ProcessOffer
	lsm := NewLegStateMachine("tag1", "caller")
	lsm.SendOffer("v=0\r\n") // Now in offer_pending

	err := lsm.ProcessOffer("v=0\r\n", "remote")
	if err == nil {
		t.Error("Should not allow ProcessOffer in offer_pending state")
	}

	// Test invalid SendAnswer
	lsm2 := NewLegStateMachine("tag2", "caller")
	err = lsm2.SendAnswer("v=0\r\n") // Never received offer
	if err == nil {
		t.Error("Should not allow SendAnswer in init state")
	}

	// Test invalid ProcessAnswer
	lsm3 := NewLegStateMachine("tag3", "caller")
	err = lsm3.ProcessAnswer("v=0\r\n") // Never sent offer
	if err == nil {
		t.Error("Should not allow ProcessAnswer in init state")
	}
}

// ForkedCallManager tests

func TestNewForkedCallManager(t *testing.T) {
	fcm := NewForkedCallManager("call-123", "from-tag-1")

	if fcm.CallID != "call-123" {
		t.Errorf("CallID should be 'call-123'")
	}
	if fcm.FromTag != "from-tag-1" {
		t.Errorf("FromTag should be 'from-tag-1'")
	}
}

func TestForkedCallManager_AddGetFork(t *testing.T) {
	fcm := NewForkedCallManager("call-123", "from-tag-1")

	// Add forks
	lsm1 := fcm.AddFork("to-tag-1", "via-1", "dest1")
	lsm2 := fcm.AddFork("to-tag-2", "via-2", "dest2")

	if lsm1 == nil || lsm2 == nil {
		t.Fatal("AddFork should return state machine")
	}

	// Get by to-tag
	if fcm.GetFork("to-tag-1") != lsm1 {
		t.Error("GetFork should return correct leg")
	}
	if fcm.GetFork("to-tag-2") != lsm2 {
		t.Error("GetFork should return correct leg")
	}
	if fcm.GetFork("nonexistent") != nil {
		t.Error("GetFork should return nil for nonexistent")
	}

	// Get by via-branch
	if fcm.GetForkByViaBranch("via-1") != lsm1 {
		t.Error("GetForkByViaBranch should return correct leg")
	}
}

func TestForkedCallManager_SelectWinner(t *testing.T) {
	fcm := NewForkedCallManager("call-123", "from-tag-1")

	// Establish call on all forks
	lsm1 := fcm.AddFork("to-tag-1", "via-1", "dest1")
	lsm2 := fcm.AddFork("to-tag-2", "via-2", "dest2")
	lsm3 := fcm.AddFork("to-tag-3", "via-3", "dest3")

	lsm1.SendOffer("v=0\r\n")
	lsm2.SendOffer("v=0\r\n")
	lsm3.SendOffer("v=0\r\n")

	// Select winner
	err := fcm.SelectWinner("to-tag-2")
	if err != nil {
		t.Fatalf("SelectWinner failed: %v", err)
	}

	if fcm.SelectedLeg != "to-tag-2" {
		t.Errorf("SelectedLeg should be 'to-tag-2'")
	}

	// Winner should be returned
	winner := fcm.GetWinner()
	if winner != lsm2 {
		t.Error("GetWinner should return selected leg")
	}

	// Other legs should be terminated
	if lsm1.GetState() != LegStateTerminated {
		t.Error("Losing leg 1 should be terminated")
	}
	if lsm3.GetState() != LegStateTerminated {
		t.Error("Losing leg 3 should be terminated")
	}
}

func TestForkedCallManager_SelectWinnerInvalid(t *testing.T) {
	fcm := NewForkedCallManager("call-123", "from-tag-1")

	err := fcm.SelectWinner("nonexistent")
	if err == nil {
		t.Error("Should error for nonexistent to-tag")
	}
}

func TestForkedCallManager_GetAllForks(t *testing.T) {
	fcm := NewForkedCallManager("call-123", "from-tag-1")

	fcm.AddFork("to-tag-1", "", "dest1")
	fcm.AddFork("to-tag-2", "", "dest2")

	forks := fcm.GetAllForks()
	if len(forks) != 2 {
		t.Errorf("Should have 2 forks, got %d", len(forks))
	}
}

func TestForkedCallManager_GetActiveForks(t *testing.T) {
	fcm := NewForkedCallManager("call-123", "from-tag-1")

	fcm.AddFork("to-tag-1", "", "dest1")
	fcm.AddFork("to-tag-2", "", "dest2")
	fcm.AddFork("to-tag-3", "", "dest3")

	// Terminate one
	fcm.GetFork("to-tag-2").Terminate()

	active := fcm.GetActiveForks()
	if len(active) != 2 {
		t.Errorf("Should have 2 active forks, got %d", len(active))
	}
}

func TestForkedCallManager_CancelAllForks(t *testing.T) {
	fcm := NewForkedCallManager("call-123", "from-tag-1")

	lsm1 := fcm.AddFork("to-tag-1", "", "dest1")
	lsm2 := fcm.AddFork("to-tag-2", "", "dest2")

	fcm.CancelAllForks()

	if lsm1.GetState() != LegStateTerminated {
		t.Error("Fork 1 should be terminated")
	}
	if lsm2.GetState() != LegStateTerminated {
		t.Error("Fork 2 should be terminated")
	}
}

func TestForkedCallManager_GetStats(t *testing.T) {
	fcm := NewForkedCallManager("call-123", "from-tag-1")
	fcm.AddFork("to-tag-1", "", "dest1")
	fcm.AddFork("to-tag-2", "", "dest2")

	stats := fcm.GetStats()

	if stats["call_id"] != "call-123" {
		t.Errorf("call_id should be 'call-123'")
	}
	if stats["total_forks"].(int) != 2 {
		t.Errorf("total_forks should be 2")
	}
}

func TestLegStateMachine_Concurrency(t *testing.T) {
	lsm := NewLegStateMachine("tag1", "caller")

	// Establish
	lsm.SendOffer("v=0\r\n")
	lsm.ProcessAnswer("v=0\r\n")

	// Concurrent reads and writes
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				lsm.GetState()
				lsm.GetDirection()
				lsm.CanSendMedia()
				lsm.CanReceiveMedia()
				lsm.UpdateSSRC(uint32(id*1000+j), true, "test")
				lsm.SetPayloadTypeMapping(uint8(j%256), uint8((j+1)%256))
				lsm.GetStats()
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestLegState_Constants(t *testing.T) {
	// Just verify constants are as expected
	if LegStateInit != "init" {
		t.Error("LegStateInit mismatch")
	}
	if LegStateEstablished != "established" {
		t.Error("LegStateEstablished mismatch")
	}
	if LegStateHold != "hold" {
		t.Error("LegStateHold mismatch")
	}
	if LegStateTerminated != "terminated" {
		t.Error("LegStateTerminated mismatch")
	}
}

func TestMediaDirection_Constants(t *testing.T) {
	if MediaDirectionSendRecv != "sendrecv" {
		t.Error("MediaDirectionSendRecv mismatch")
	}
	if MediaDirectionSendOnly != "sendonly" {
		t.Error("MediaDirectionSendOnly mismatch")
	}
	if MediaDirectionRecvOnly != "recvonly" {
		t.Error("MediaDirectionRecvOnly mismatch")
	}
	if MediaDirectionInactive != "inactive" {
		t.Error("MediaDirectionInactive mismatch")
	}
}

func TestLegStateMachine_HoldStartTime(t *testing.T) {
	lsm := NewLegStateMachine("tag1", "caller")
	lsm.SendOffer("v=0\r\n")
	lsm.ProcessAnswer("v=0\r\n")

	before := time.Now()
	lsm.SetHold("local")
	after := time.Now()

	if lsm.HoldStartedAt.Before(before) || lsm.HoldStartedAt.After(after) {
		t.Error("HoldStartedAt should be set when hold is initiated")
	}

	// Check stats include hold duration
	time.Sleep(10 * time.Millisecond)
	stats := lsm.GetStats()
	if _, ok := stats["hold_duration"]; !ok {
		t.Error("Stats should include hold_duration when on hold")
	}
}
