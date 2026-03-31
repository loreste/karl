package internal

import (
	"fmt"
	"sync"
	"time"
)

// LegState represents the SDP negotiation state of a leg
type LegState string

const (
	// LegStateInit is the initial state before any offer/answer exchange
	LegStateInit LegState = "init"
	// LegStateOfferPending is when we've sent an offer awaiting answer
	LegStateOfferPending LegState = "offer_pending"
	// LegStateAnswerPending is when we've received an offer and need to send answer
	LegStateAnswerPending LegState = "answer_pending"
	// LegStateEstablished is when offer/answer is complete
	LegStateEstablished LegState = "established"
	// LegStateReOfferPending is during a re-INVITE negotiation
	LegStateReOfferPending LegState = "reoffer_pending"
	// LegStateReAnswerPending is awaiting re-answer
	LegStateReAnswerPending LegState = "reanswer_pending"
	// LegStateHold is when the leg is on hold
	LegStateHold LegState = "hold"
	// LegStateTerminated is when the leg is terminated
	LegStateTerminated LegState = "terminated"
)

// MediaDirection represents the direction of media flow
type MediaDirection string

const (
	MediaDirectionSendRecv MediaDirection = "sendrecv"
	MediaDirectionSendOnly MediaDirection = "sendonly"
	MediaDirectionRecvOnly MediaDirection = "recvonly"
	MediaDirectionInactive MediaDirection = "inactive"
)

// LegStateMachine tracks the state of a single leg in a call
type LegStateMachine struct {
	mu sync.RWMutex

	// Identity
	Tag   string
	Label string

	// State
	State          LegState
	Direction      MediaDirection
	PreviousState  LegState
	PreviousDirection MediaDirection

	// SDP tracking
	LocalSDP     string  // Last local SDP
	RemoteSDP    string  // Last remote SDP
	OfferSource  string  // Who originated the current offer: "local" or "remote"

	// Timestamps
	CreatedAt        time.Time
	LastStateChange  time.Time
	EstablishedAt    time.Time
	HoldStartedAt    time.Time

	// Hold detection
	IsOnHold         bool
	HoldInitiator    string // "local" or "remote"

	// Early media tracking
	EarlyMediaActive bool
	EarlyMediaStartedAt time.Time

	// SSRC tracking for asymmetric RTP
	LocalSSRC    uint32
	RemoteSSRC   uint32
	SSRCHistory  []SSRCChange

	// Payload type mapping
	PTMap        map[uint8]uint8 // Remote PT -> Local PT
	ReversePTMap map[uint8]uint8 // Local PT -> Remote PT
}

// SSRCChange records a change in SSRC
type SSRCChange struct {
	OldSSRC   uint32
	NewSSRC   uint32
	Timestamp time.Time
	Reason    string
}

// NewLegStateMachine creates a new leg state machine
func NewLegStateMachine(tag, label string) *LegStateMachine {
	return &LegStateMachine{
		Tag:             tag,
		Label:           label,
		State:           LegStateInit,
		Direction:       MediaDirectionSendRecv,
		CreatedAt:       time.Now(),
		LastStateChange: time.Now(),
		PTMap:           make(map[uint8]uint8),
		ReversePTMap:    make(map[uint8]uint8),
		SSRCHistory:     make([]SSRCChange, 0),
	}
}

// ProcessOffer processes an incoming offer
func (lsm *LegStateMachine) ProcessOffer(sdp string, source string) error {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()

	switch lsm.State {
	case LegStateInit:
		lsm.setState(LegStateAnswerPending)
	case LegStateEstablished:
		lsm.setState(LegStateReAnswerPending)
	case LegStateHold:
		// Re-INVITE while on hold (possibly resuming)
		lsm.setState(LegStateReAnswerPending)
	default:
		return fmt.Errorf("cannot process offer in state %s", lsm.State)
	}

	lsm.RemoteSDP = sdp
	lsm.OfferSource = source
	lsm.detectHoldFromSDP(sdp)

	return nil
}

// ProcessAnswer processes an incoming answer
func (lsm *LegStateMachine) ProcessAnswer(sdp string) error {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()

	switch lsm.State {
	case LegStateOfferPending:
		lsm.setState(LegStateEstablished)
		if lsm.EstablishedAt.IsZero() {
			lsm.EstablishedAt = time.Now()
		}
	case LegStateReOfferPending:
		// Re-INVITE complete, check if coming out of hold
		if lsm.IsOnHold && !lsm.detectHoldFromDirection(lsm.Direction) {
			lsm.IsOnHold = false
			lsm.setState(LegStateEstablished)
		} else {
			lsm.setState(LegStateEstablished)
		}
	default:
		return fmt.Errorf("cannot process answer in state %s", lsm.State)
	}

	lsm.RemoteSDP = sdp
	lsm.detectHoldFromSDP(sdp)

	return nil
}

// SendOffer marks that we're sending an offer
func (lsm *LegStateMachine) SendOffer(sdp string) error {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()

	switch lsm.State {
	case LegStateInit:
		lsm.setState(LegStateOfferPending)
	case LegStateEstablished:
		lsm.setState(LegStateReOfferPending)
	case LegStateHold:
		lsm.setState(LegStateReOfferPending)
	default:
		return fmt.Errorf("cannot send offer in state %s", lsm.State)
	}

	lsm.LocalSDP = sdp
	lsm.OfferSource = "local"

	return nil
}

// SendAnswer marks that we're sending an answer
func (lsm *LegStateMachine) SendAnswer(sdp string) error {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()

	switch lsm.State {
	case LegStateAnswerPending:
		lsm.setState(LegStateEstablished)
		if lsm.EstablishedAt.IsZero() {
			lsm.EstablishedAt = time.Now()
		}
	case LegStateReAnswerPending:
		lsm.setState(LegStateEstablished)
	default:
		return fmt.Errorf("cannot send answer in state %s", lsm.State)
	}

	lsm.LocalSDP = sdp

	return nil
}

// EnableEarlyMedia enables early media for this leg
func (lsm *LegStateMachine) EnableEarlyMedia() {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()

	if !lsm.EarlyMediaActive && lsm.State == LegStateOfferPending {
		lsm.EarlyMediaActive = true
		lsm.EarlyMediaStartedAt = time.Now()
	}
}

// DisableEarlyMedia disables early media
func (lsm *LegStateMachine) DisableEarlyMedia() {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()
	lsm.EarlyMediaActive = false
}

// SetHold puts the leg on hold
func (lsm *LegStateMachine) SetHold(initiator string) {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()

	if !lsm.IsOnHold {
		lsm.PreviousState = lsm.State
		lsm.PreviousDirection = lsm.Direction
		lsm.IsOnHold = true
		lsm.HoldInitiator = initiator
		lsm.HoldStartedAt = time.Now()
		lsm.setState(LegStateHold)
	}
}

// Resume takes the leg off hold
func (lsm *LegStateMachine) Resume() {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()

	if lsm.IsOnHold {
		lsm.IsOnHold = false
		lsm.HoldInitiator = ""
		lsm.setState(LegStateEstablished)
		lsm.Direction = MediaDirectionSendRecv
	}
}

// Terminate marks the leg as terminated
func (lsm *LegStateMachine) Terminate() {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()
	lsm.setState(LegStateTerminated)
}

// UpdateSSRC updates the SSRC, tracking changes
func (lsm *LegStateMachine) UpdateSSRC(ssrc uint32, isLocal bool, reason string) {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()

	var oldSSRC uint32
	if isLocal {
		oldSSRC = lsm.LocalSSRC
		if oldSSRC != 0 && oldSSRC != ssrc {
			lsm.SSRCHistory = append(lsm.SSRCHistory, SSRCChange{
				OldSSRC:   oldSSRC,
				NewSSRC:   ssrc,
				Timestamp: time.Now(),
				Reason:    reason,
			})
		}
		lsm.LocalSSRC = ssrc
	} else {
		oldSSRC = lsm.RemoteSSRC
		if oldSSRC != 0 && oldSSRC != ssrc {
			lsm.SSRCHistory = append(lsm.SSRCHistory, SSRCChange{
				OldSSRC:   oldSSRC,
				NewSSRC:   ssrc,
				Timestamp: time.Now(),
				Reason:    reason,
			})
		}
		lsm.RemoteSSRC = ssrc
	}
}

// SetPayloadTypeMapping sets the payload type mapping
func (lsm *LegStateMachine) SetPayloadTypeMapping(remotePT, localPT uint8) {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()
	lsm.PTMap[remotePT] = localPT
	lsm.ReversePTMap[localPT] = remotePT
}

// MapPayloadType maps a remote PT to local PT
func (lsm *LegStateMachine) MapPayloadType(remotePT uint8) uint8 {
	lsm.mu.RLock()
	defer lsm.mu.RUnlock()
	if localPT, ok := lsm.PTMap[remotePT]; ok {
		return localPT
	}
	return remotePT // No mapping, return as-is
}

// ReverseMapPayloadType maps a local PT to remote PT
func (lsm *LegStateMachine) ReverseMapPayloadType(localPT uint8) uint8 {
	lsm.mu.RLock()
	defer lsm.mu.RUnlock()
	if remotePT, ok := lsm.ReversePTMap[localPT]; ok {
		return remotePT
	}
	return localPT
}

// SetDirection sets the media direction
func (lsm *LegStateMachine) SetDirection(dir MediaDirection) {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()
	lsm.PreviousDirection = lsm.Direction
	lsm.Direction = dir

	// Detect hold from direction
	if lsm.detectHoldFromDirection(dir) && !lsm.IsOnHold {
		lsm.IsOnHold = true
		lsm.HoldStartedAt = time.Now()
	}
}

// GetState returns the current state
func (lsm *LegStateMachine) GetState() LegState {
	lsm.mu.RLock()
	defer lsm.mu.RUnlock()
	return lsm.State
}

// GetDirection returns the current direction
func (lsm *LegStateMachine) GetDirection() MediaDirection {
	lsm.mu.RLock()
	defer lsm.mu.RUnlock()
	return lsm.Direction
}

// IsEstablished checks if the leg is established
func (lsm *LegStateMachine) IsEstablished() bool {
	lsm.mu.RLock()
	defer lsm.mu.RUnlock()
	return lsm.State == LegStateEstablished || lsm.State == LegStateHold
}

// CanSendMedia checks if the leg can send media
func (lsm *LegStateMachine) CanSendMedia() bool {
	lsm.mu.RLock()
	defer lsm.mu.RUnlock()

	// Can send if established and direction allows
	if lsm.State != LegStateEstablished && lsm.State != LegStateHold && !lsm.EarlyMediaActive {
		return false
	}
	return lsm.Direction == MediaDirectionSendRecv || lsm.Direction == MediaDirectionSendOnly
}

// CanReceiveMedia checks if the leg can receive media
func (lsm *LegStateMachine) CanReceiveMedia() bool {
	lsm.mu.RLock()
	defer lsm.mu.RUnlock()

	if lsm.State != LegStateEstablished && lsm.State != LegStateHold && !lsm.EarlyMediaActive {
		return false
	}
	return lsm.Direction == MediaDirectionSendRecv || lsm.Direction == MediaDirectionRecvOnly
}

// GetStats returns state machine statistics
func (lsm *LegStateMachine) GetStats() map[string]interface{} {
	lsm.mu.RLock()
	defer lsm.mu.RUnlock()

	stats := map[string]interface{}{
		"tag":           lsm.Tag,
		"label":         lsm.Label,
		"state":         string(lsm.State),
		"direction":     string(lsm.Direction),
		"on_hold":       lsm.IsOnHold,
		"early_media":   lsm.EarlyMediaActive,
		"created_at":    lsm.CreatedAt,
		"local_ssrc":    lsm.LocalSSRC,
		"remote_ssrc":   lsm.RemoteSSRC,
		"ssrc_changes":  len(lsm.SSRCHistory),
		"pt_mappings":   len(lsm.PTMap),
	}

	if !lsm.EstablishedAt.IsZero() {
		stats["established_at"] = lsm.EstablishedAt
		stats["time_to_establish"] = lsm.EstablishedAt.Sub(lsm.CreatedAt).Milliseconds()
	}

	if lsm.IsOnHold && !lsm.HoldStartedAt.IsZero() {
		stats["hold_duration"] = time.Since(lsm.HoldStartedAt).Seconds()
		stats["hold_initiator"] = lsm.HoldInitiator
	}

	return stats
}

// Internal helpers

func (lsm *LegStateMachine) setState(newState LegState) {
	lsm.PreviousState = lsm.State
	lsm.State = newState
	lsm.LastStateChange = time.Now()
}

func (lsm *LegStateMachine) detectHoldFromSDP(sdp string) {
	// Parse direction from SDP and detect hold
	// This is a simple check - production would parse full SDP
	if containsString(sdp, "a=sendonly") {
		lsm.Direction = MediaDirectionSendOnly
		lsm.IsOnHold = true
		lsm.HoldInitiator = "remote"
	} else if containsString(sdp, "a=recvonly") {
		lsm.Direction = MediaDirectionRecvOnly
	} else if containsString(sdp, "a=inactive") {
		lsm.Direction = MediaDirectionInactive
		lsm.IsOnHold = true
		lsm.HoldInitiator = "remote"
	} else {
		lsm.Direction = MediaDirectionSendRecv
		// If we were on hold and now have sendrecv, resume
		if lsm.IsOnHold {
			lsm.IsOnHold = false
			lsm.HoldInitiator = ""
		}
	}
}

func (lsm *LegStateMachine) detectHoldFromDirection(dir MediaDirection) bool {
	return dir == MediaDirectionSendOnly || dir == MediaDirectionInactive
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ForkedCallManager manages forked calls with multiple to-tags
type ForkedCallManager struct {
	mu sync.RWMutex

	CallID       string
	FromTag      string
	ToTags       map[string]*LegStateMachine // to-tag -> leg state machine
	SelectedLeg  string                      // The "winning" to-tag
	ViaBranches  map[string]string           // via-branch -> to-tag
	CreatedAt    time.Time
}

// NewForkedCallManager creates a manager for forked calls
func NewForkedCallManager(callID, fromTag string) *ForkedCallManager {
	return &ForkedCallManager{
		CallID:      callID,
		FromTag:     fromTag,
		ToTags:      make(map[string]*LegStateMachine),
		ViaBranches: make(map[string]string),
		CreatedAt:   time.Now(),
	}
}

// AddFork adds a new fork (new to-tag)
func (fcm *ForkedCallManager) AddFork(toTag, viaBranch, label string) *LegStateMachine {
	fcm.mu.Lock()
	defer fcm.mu.Unlock()

	lsm := NewLegStateMachine(toTag, label)
	fcm.ToTags[toTag] = lsm
	if viaBranch != "" {
		fcm.ViaBranches[viaBranch] = toTag
	}

	return lsm
}

// GetFork returns a forked leg by to-tag
func (fcm *ForkedCallManager) GetFork(toTag string) *LegStateMachine {
	fcm.mu.RLock()
	defer fcm.mu.RUnlock()
	return fcm.ToTags[toTag]
}

// GetForkByViaBranch returns a forked leg by via-branch
func (fcm *ForkedCallManager) GetForkByViaBranch(viaBranch string) *LegStateMachine {
	fcm.mu.RLock()
	defer fcm.mu.RUnlock()
	if toTag, ok := fcm.ViaBranches[viaBranch]; ok {
		return fcm.ToTags[toTag]
	}
	return nil
}

// SelectWinner selects the "winning" fork (when call is answered)
func (fcm *ForkedCallManager) SelectWinner(toTag string) error {
	fcm.mu.Lock()
	defer fcm.mu.Unlock()

	if _, ok := fcm.ToTags[toTag]; !ok {
		return fmt.Errorf("fork with to-tag %s not found", toTag)
	}

	fcm.SelectedLeg = toTag

	// Terminate all other forks
	for tag, lsm := range fcm.ToTags {
		if tag != toTag {
			lsm.Terminate()
		}
	}

	return nil
}

// GetWinner returns the winning leg
func (fcm *ForkedCallManager) GetWinner() *LegStateMachine {
	fcm.mu.RLock()
	defer fcm.mu.RUnlock()
	if fcm.SelectedLeg != "" {
		return fcm.ToTags[fcm.SelectedLeg]
	}
	return nil
}

// GetAllForks returns all forks
func (fcm *ForkedCallManager) GetAllForks() []*LegStateMachine {
	fcm.mu.RLock()
	defer fcm.mu.RUnlock()

	forks := make([]*LegStateMachine, 0, len(fcm.ToTags))
	for _, lsm := range fcm.ToTags {
		forks = append(forks, lsm)
	}
	return forks
}

// GetActiveForks returns only non-terminated forks
func (fcm *ForkedCallManager) GetActiveForks() []*LegStateMachine {
	fcm.mu.RLock()
	defer fcm.mu.RUnlock()

	forks := make([]*LegStateMachine, 0)
	for _, lsm := range fcm.ToTags {
		if lsm.GetState() != LegStateTerminated {
			forks = append(forks, lsm)
		}
	}
	return forks
}

// CancelAllForks cancels all forked legs (e.g., on CANCEL)
func (fcm *ForkedCallManager) CancelAllForks() {
	fcm.mu.Lock()
	defer fcm.mu.Unlock()

	for _, lsm := range fcm.ToTags {
		lsm.Terminate()
	}
}

// GetStats returns forked call statistics
func (fcm *ForkedCallManager) GetStats() map[string]interface{} {
	fcm.mu.RLock()
	defer fcm.mu.RUnlock()

	forkStats := make(map[string]interface{})
	for toTag, lsm := range fcm.ToTags {
		forkStats[toTag] = lsm.GetStats()
	}

	return map[string]interface{}{
		"call_id":     fcm.CallID,
		"from_tag":    fcm.FromTag,
		"total_forks": len(fcm.ToTags),
		"selected":    fcm.SelectedLeg,
		"forks":       forkStats,
	}
}
