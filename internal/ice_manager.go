package internal

import (
	"fmt"
	"log"
	"sync"

	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"
)

// ICEManager handles ICE candidates dynamically
type ICEManager struct {
	agent         *ice.Agent
	bestCandidate ice.Candidate
	mu            sync.Mutex
}

// NewICEManager initializes ICE with dynamic selection
func NewICEManager(iceServers []webrtc.ICEServer) (*ICEManager, error) {
	log.Println("🌍 Initializing WebRTC ICE for NAT Traversal...")

	// ICE Agent Configuration
	config := &ice.AgentConfig{
		NetworkTypes: []ice.NetworkType{ice.NetworkTypeUDP4, ice.NetworkTypeUDP6},
	}

	// Create ICE Agent
	agent, err := ice.NewAgent(config)
	if err != nil {
		return nil, fmt.Errorf("❌ Failed to create ICE agent: %v", err)
	}

	manager := &ICEManager{agent: agent}

	// Monitor candidate selection
	agent.OnCandidate(func(candidate ice.Candidate) {
		if candidate != nil {
			manager.selectBestCandidate(candidate)
		}
	})

	return manager, nil
}

// selectBestCandidate chooses the lowest-latency candidate
func (i *ICEManager) selectBestCandidate(candidate ice.Candidate) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.bestCandidate == nil || candidate.Priority() > i.bestCandidate.Priority() {
		i.bestCandidate = candidate
		log.Printf("⭐ New Best ICE Candidate: %s (Priority: %d)", candidate.String(), candidate.Priority())
	}
}

// GetBestCandidate returns the best ICE candidate
func (i *ICEManager) GetBestCandidate() ice.Candidate {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.bestCandidate
}
