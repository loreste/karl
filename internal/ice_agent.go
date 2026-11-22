package internal

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pion/ice/v2"
	"github.com/pion/stun"
)

// ICEAgentConfig configures a per-session ICE agent.
type ICEAgentConfig struct {
	// Controlling indicates if this agent is the ICE controlling agent.
	Controlling bool
	// Lite enables ICE-lite mode (no connectivity checks, host candidates only).
	Lite bool
	// STUNServers for server-reflexive candidate gathering.
	STUNServers []string
	// TURNServers for relay candidate gathering.
	TURNServers []ICETURNServer
	// LocalUfrag and LocalPwd — if empty, generated automatically.
	LocalUfrag string
	LocalPwd   string
	// RemoteUfrag and RemotePwd from the peer's SDP.
	RemoteUfrag string
	RemotePwd   string
	// CandidateTimeout for gathering.
	CandidateTimeout time.Duration
}

// ICETURNServer holds TURN server credentials.
type ICETURNServer struct {
	URL      string
	Username string
	Password string
}

// ICESessionAgent manages ICE negotiation for a single call leg.
type ICESessionAgent struct {
	agent           *ice.Agent
	conn            *ice.Conn // the selected candidate pair connection
	localCandidates []ice.Candidate
	config          ICEAgentConfig

	mu       sync.Mutex
	state    ICESessionState
	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}

	// OnConnected fires when ICE negotiation completes with a valid pair.
	// The net.Conn can be used for media forwarding.
	OnConnected func(conn net.Conn)
}

// ICESessionState tracks ICE agent lifecycle.
type ICESessionState int

const (
	ICEStateNew ICESessionState = iota
	ICEStateGathering
	ICEStateChecking
	ICEStateConnected
	ICEStateFailed
	ICEStateClosed
)

// NewICESessionAgent creates a per-session ICE agent.
func NewICESessionAgent(config ICEAgentConfig) (*ICESessionAgent, error) {
	if config.CandidateTimeout == 0 {
		config.CandidateTimeout = 5 * time.Second
	}

	urls := make([]*stun.URI, 0, len(config.STUNServers)+len(config.TURNServers))
	for _, s := range config.STUNServers {
		u, err := stun.ParseURI(s)
		if err != nil {
			log.Printf("ICE: invalid STUN URI %q: %v", s, err)
			continue
		}
		urls = append(urls, u)
	}
	for _, t := range config.TURNServers {
		u, err := stun.ParseURI(t.URL)
		if err != nil {
			log.Printf("ICE: invalid TURN URI %q: %v", t.URL, err)
			continue
		}
		u.Username = t.Username
		u.Password = t.Password
		urls = append(urls, u)
	}

	agentConfig := &ice.AgentConfig{
		NetworkTypes: []ice.NetworkType{ice.NetworkTypeUDP4},
		Urls:         urls,
		Lite:         config.Lite,
	}
	if config.LocalUfrag != "" && config.LocalPwd != "" {
		agentConfig.LocalUfrag = config.LocalUfrag
		agentConfig.LocalPwd = config.LocalPwd
	}

	agent, err := ice.NewAgent(agentConfig)
	if err != nil {
		return nil, fmt.Errorf("ice.NewAgent: %w", err)
	}

	sa := &ICESessionAgent{
		agent:  agent,
		config: config,
		state:  ICEStateNew,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}

	// Collect local candidates
	if err := agent.OnCandidate(func(c ice.Candidate) {
		if c == nil {
			return // gathering done
		}
		sa.mu.Lock()
		sa.localCandidates = append(sa.localCandidates, c)
		sa.mu.Unlock()
	}); err != nil {
		agent.Close()
		return nil, err
	}

	// Track connection state
	if err := agent.OnConnectionStateChange(func(s ice.ConnectionState) {
		sa.mu.Lock()
		switch s {
		case ice.ConnectionStateChecking:
			sa.state = ICEStateChecking
		case ice.ConnectionStateConnected:
			sa.state = ICEStateConnected
		case ice.ConnectionStateFailed:
			sa.state = ICEStateFailed
		case ice.ConnectionStateClosed:
			sa.state = ICEStateClosed
		}
		sa.mu.Unlock()
		log.Printf("ICE: state changed to %s", s)
	}); err != nil {
		agent.Close()
		return nil, err
	}

	return sa, nil
}

// GatherCandidates starts candidate gathering and returns when done or timeout.
func (sa *ICESessionAgent) GatherCandidates(ctx context.Context) error {
	sa.mu.Lock()
	sa.state = ICEStateGathering
	sa.mu.Unlock()

	if err := sa.agent.GatherCandidates(); err != nil {
		return fmt.Errorf("gather candidates: %w", err)
	}

	// Wait for gathering to complete (candidates trickle via OnCandidate,
	// nil candidate signals completion)
	gatherCtx, cancel := context.WithTimeout(ctx, sa.config.CandidateTimeout)
	defer cancel()

	for {
		select {
		case <-gatherCtx.Done():
			return nil // timeout is acceptable — we use what we gathered
		case <-sa.stopCh:
			return fmt.Errorf("agent stopped during gathering")
		case <-time.After(100 * time.Millisecond):
			// Check if we have at least one candidate
			sa.mu.Lock()
			n := len(sa.localCandidates)
			sa.mu.Unlock()
			if n > 0 {
				// Give a brief moment for more candidates, then proceed
				time.Sleep(200 * time.Millisecond)
				return nil
			}
		}
	}
}

// GetLocalCandidates returns gathered local candidates.
func (sa *ICESessionAgent) GetLocalCandidates() []ice.Candidate {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	result := make([]ice.Candidate, len(sa.localCandidates))
	copy(result, sa.localCandidates)
	return result
}

// GetLocalCredentials returns the local ICE ufrag and pwd.
func (sa *ICESessionAgent) GetLocalCredentials() (ufrag, pwd string, err error) {
	return sa.agent.GetLocalUserCredentials()
}

// AddRemoteCandidate adds a remote ICE candidate for connectivity checks.
func (sa *ICESessionAgent) AddRemoteCandidate(candidateStr string) error {
	c, err := ice.UnmarshalCandidate(candidateStr)
	if err != nil {
		return fmt.Errorf("unmarshal candidate: %w", err)
	}
	return sa.agent.AddRemoteCandidate(c)
}

// Connect starts ICE connectivity checks with the remote peer.
// Blocks until a candidate pair is selected or the context is cancelled.
func (sa *ICESessionAgent) Connect(ctx context.Context) (net.Conn, error) {
	sa.mu.Lock()
	sa.state = ICEStateChecking
	sa.mu.Unlock()

	var conn *ice.Conn
	var err error

	if sa.config.Controlling {
		conn, err = sa.agent.Dial(ctx, sa.config.RemoteUfrag, sa.config.RemotePwd)
	} else {
		conn, err = sa.agent.Accept(ctx, sa.config.RemoteUfrag, sa.config.RemotePwd)
	}
	if err != nil {
		sa.mu.Lock()
		sa.state = ICEStateFailed
		sa.mu.Unlock()
		return nil, fmt.Errorf("ICE connect: %w", err)
	}

	sa.mu.Lock()
	sa.conn = conn
	sa.state = ICEStateConnected
	sa.mu.Unlock()

	if sa.OnConnected != nil {
		sa.OnConnected(conn)
	}

	return conn, nil
}

// GetSelectedPair returns the selected candidate pair, or nil.
func (sa *ICESessionAgent) GetSelectedPair() (*ice.CandidatePair, error) {
	return sa.agent.GetSelectedCandidatePair()
}

// State returns the current ICE state.
func (sa *ICESessionAgent) State() ICESessionState {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	return sa.state
}

// Close shuts down the ICE agent.
func (sa *ICESessionAgent) Close() error {
	sa.stopOnce.Do(func() {
		close(sa.stopCh)
	})
	sa.mu.Lock()
	sa.state = ICEStateClosed
	sa.mu.Unlock()
	return sa.agent.Close()
}
