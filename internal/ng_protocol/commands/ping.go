package commands

import (
	ng "karl/internal/ng_protocol"
)

// PingHandler handles the ping command
type PingHandler struct{}

// NewPingHandler creates a new ping handler
func NewPingHandler() *PingHandler {
	return &PingHandler{}
}

// Handle processes a ping request
func (h *PingHandler) Handle(req *ng.NGRequest) (*ng.NGResponse, error) {
	return &ng.NGResponse{
		Result: ng.ResultPong,
	}, nil
}
