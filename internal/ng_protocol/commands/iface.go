package commands

import (
	"context"
	"log"
	"net"

	"karl/internal"
	ng "karl/internal/ng_protocol"
)

// selectMediaIPs selects the bind and advertise IPs based on flags, direction, and interface configuration.
// bindIP is used for socket allocation; advertiseIP is used in SDP and stream info responses.
func selectMediaIPs(ifaceSelector *internal.InterfaceSelector, config *internal.Config, flags *ng.ParsedFlags, req *ng.NGRequest, fromSide bool) (bindIP, advertiseIP string) {
	// Explicit media-address overrides everything
	if flags.MediaAddress != "" {
		return flags.MediaAddress, flags.MediaAddress
	}

	// Determine explicit interface name from flags
	var ifaceName string
	if fromSide {
		if flags.FromInterface != "" {
			ifaceName = flags.FromInterface
		} else if flags.Interface != "" {
			ifaceName = flags.Interface
		}
	} else {
		if flags.ToInterface != "" {
			ifaceName = flags.ToInterface
		} else if flags.Interface != "" {
			ifaceName = flags.Interface
		}
	}

	// Use InterfaceSelector which handles direction and peer-based selection
	var peerAddr net.IP
	if req.ReceivedFrom != nil {
		peerAddr = req.ReceivedFrom.IP
	}

	iface := ifaceSelector.SelectInterface(ifaceName, req.Direction, peerAddr)
	if iface != nil && iface.LocalAddress != "" {
		bindIP = iface.LocalAddress
		advertiseIP = iface.AdvertiseAddr
		if advertiseIP == "" {
			advertiseIP = bindIP
		}
		return bindIP, advertiseIP
	}

	// Use received-from if trust-address is set and SIP-source-address flag
	if flags.TrustAddress && flags.SIPSourceAddress && req.ReceivedFrom != nil {
		ip := req.ReceivedFrom.IP.String()
		return ip, ip
	}

	// Fall back to config
	if config.Integration.MediaIP != "" {
		bindIP = config.Integration.MediaIP
		advertiseIP = config.Integration.PublicIP
		if advertiseIP == "" {
			advertiseIP = bindIP
		}
		return bindIP, advertiseIP
	}

	return "127.0.0.1", "127.0.0.1"
}

// setupICEAgent creates an ICE session agent for a call leg when ICE is negotiated.
// controlling should be true for the offer side, false for the answer side.
func setupICEAgent(leg *internal.CallLeg, parsedSDP *ParsedSDP, flags *ng.ParsedFlags, config *internal.Config, controlling bool) {
	if flags.ICERemove || flags.ICELite {
		return
	}
	if !flags.ICEForce && !parsedSDP.HasICE {
		return
	}

	iceConfig := internal.ICEAgentConfig{
		Controlling: controlling,
		Lite:        flags.ICELite,
		RemoteUfrag: parsedSDP.ICEUfrag,
		RemotePwd:   parsedSDP.ICEPwd,
	}
	if config.WebRTC.Enabled {
		for _, s := range config.WebRTC.StunServers {
			iceConfig.STUNServers = append(iceConfig.STUNServers, s)
		}
	}

	agent, err := internal.NewICESessionAgent(iceConfig)
	if err != nil {
		log.Printf("Warning: failed to create ICE agent: %v", err)
		return
	}
	leg.ICEAgent = agent
	go agent.GatherCandidates(context.Background())
}
