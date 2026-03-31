package internal

import (
	"net"
	"strings"
	"sync"
)

// InterfaceSelector handles network interface selection for media routing
type InterfaceSelector struct {
	interfaces    map[string]*InterfaceInfo
	defaultIface  string
	internalNets  []*net.IPNet
	externalNets  []*net.IPNet
	peerRules     []PeerRule
	mu            sync.RWMutex
}

// InterfaceInfo holds interface configuration
type InterfaceInfo struct {
	Name          string
	LocalAddress  string   // Address to bind to
	AdvertiseAddr string   // Address to advertise in SDP (for NAT)
	Port          int      // Optional port override
	LocalAddrs    []string // Additional local addresses
	IsInternal    bool     // Whether this is an internal interface
}

// PeerRule defines routing rules based on peer address
type PeerRule struct {
	Network   *net.IPNet
	Interface string
}

// NewInterfaceSelector creates a new interface selector
func NewInterfaceSelector(config *Config) *InterfaceSelector {
	is := &InterfaceSelector{
		interfaces:   make(map[string]*InterfaceInfo),
		internalNets: make([]*net.IPNet, 0),
		externalNets: make([]*net.IPNet, 0),
		peerRules:    make([]PeerRule, 0),
	}

	// Initialize from config
	if config.Integration.Interfaces != nil {
		for name, ifaceCfg := range config.Integration.Interfaces {
			is.interfaces[name] = &InterfaceInfo{
				Name:          name,
				LocalAddress:  ifaceCfg.Address,
				AdvertiseAddr: ifaceCfg.AdvertiseAddr,
				Port:          ifaceCfg.Port,
				IsInternal:    strings.Contains(strings.ToLower(name), "internal"),
			}
		}
	}

	// Set up default interfaces based on config
	if config.Integration.MediaIP != "" {
		is.interfaces["default"] = &InterfaceInfo{
			Name:          "default",
			LocalAddress:  config.Integration.MediaIP,
			AdvertiseAddr: config.Integration.PublicIP,
		}
		is.defaultIface = "default"
	}

	// Set up internal/external based on config
	if config.Integration.MediaIP != "" {
		is.interfaces["internal"] = &InterfaceInfo{
			Name:          "internal",
			LocalAddress:  config.Integration.MediaIP,
			AdvertiseAddr: config.Integration.MediaIP,
			IsInternal:    true,
		}
	}
	if config.Integration.PublicIP != "" {
		is.interfaces["external"] = &InterfaceInfo{
			Name:          "external",
			LocalAddress:  config.Integration.MediaIP,
			AdvertiseAddr: config.Integration.PublicIP,
			IsInternal:    false,
		}
	}

	// Add common private networks as internal
	internalCIDRs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"fc00::/7",  // IPv6 ULA
		"fe80::/10", // IPv6 link-local
	}
	for _, cidr := range internalCIDRs {
		if _, ipnet, err := net.ParseCIDR(cidr); err == nil {
			is.internalNets = append(is.internalNets, ipnet)
		}
	}

	return is
}

// SelectInterface selects the appropriate interface based on direction and peer
func (is *InterfaceSelector) SelectInterface(interfaceName string, direction []string, peerAddr net.IP) *InterfaceInfo {
	is.mu.RLock()
	defer is.mu.RUnlock()

	// If explicit interface name is provided, use it
	if interfaceName != "" {
		if iface, ok := is.interfaces[interfaceName]; ok {
			return iface
		}
	}

	// Check peer-based rules
	if peerAddr != nil {
		for _, rule := range is.peerRules {
			if rule.Network.Contains(peerAddr) {
				if iface, ok := is.interfaces[rule.Interface]; ok {
					return iface
				}
			}
		}
	}

	// Check direction-based selection
	if len(direction) >= 2 {
		// direction[0] = from direction, direction[1] = to direction
		// e.g., ["internal", "external"] means from internal to external
		toDir := direction[1]
		if iface, ok := is.interfaces[toDir]; ok {
			return iface
		}
	} else if len(direction) == 1 {
		// Single direction specified
		if iface, ok := is.interfaces[direction[0]]; ok {
			return iface
		}
	}

	// Auto-detect based on peer address
	if peerAddr != nil {
		if is.isInternal(peerAddr) {
			if iface, ok := is.interfaces["internal"]; ok {
				return iface
			}
		} else {
			if iface, ok := is.interfaces["external"]; ok {
				return iface
			}
		}
	}

	// Fall back to default
	if is.defaultIface != "" {
		if iface, ok := is.interfaces[is.defaultIface]; ok {
			return iface
		}
	}

	// Last resort - return first available
	for _, iface := range is.interfaces {
		return iface
	}

	return nil
}

// GetAdvertiseAddress returns the address to advertise in SDP
func (is *InterfaceSelector) GetAdvertiseAddress(interfaceName string, peerAddr net.IP) string {
	iface := is.SelectInterface(interfaceName, nil, peerAddr)
	if iface == nil {
		return ""
	}

	// If we have an advertise address (NAT), use it
	if iface.AdvertiseAddr != "" {
		return iface.AdvertiseAddr
	}

	// Otherwise use local address
	return iface.LocalAddress
}

// GetLocalAddress returns the local address to bind to
func (is *InterfaceSelector) GetLocalAddress(interfaceName string) string {
	is.mu.RLock()
	defer is.mu.RUnlock()

	if interfaceName != "" {
		if iface, ok := is.interfaces[interfaceName]; ok {
			return iface.LocalAddress
		}
	}

	if is.defaultIface != "" {
		if iface, ok := is.interfaces[is.defaultIface]; ok {
			return iface.LocalAddress
		}
	}

	return ""
}

// isInternal checks if an IP is in an internal network
func (is *InterfaceSelector) isInternal(ip net.IP) bool {
	for _, ipnet := range is.internalNets {
		if ipnet.Contains(ip) {
			return true
		}
	}
	return false
}

// AddInterface adds a new interface configuration
func (is *InterfaceSelector) AddInterface(name string, info *InterfaceInfo) {
	is.mu.Lock()
	defer is.mu.Unlock()
	info.Name = name
	is.interfaces[name] = info
}

// AddPeerRule adds a peer-based routing rule
func (is *InterfaceSelector) AddPeerRule(cidr string, interfaceName string) error {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}

	is.mu.Lock()
	defer is.mu.Unlock()

	is.peerRules = append(is.peerRules, PeerRule{
		Network:   ipnet,
		Interface: interfaceName,
	})
	return nil
}

// AddInternalNetwork adds a CIDR to the internal network list
func (is *InterfaceSelector) AddInternalNetwork(cidr string) error {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}

	is.mu.Lock()
	defer is.mu.Unlock()
	is.internalNets = append(is.internalNets, ipnet)
	return nil
}

// SetDefaultInterface sets the default interface
func (is *InterfaceSelector) SetDefaultInterface(name string) {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.defaultIface = name
}

// GetInterfaceNames returns all configured interface names
func (is *InterfaceSelector) GetInterfaceNames() []string {
	is.mu.RLock()
	defer is.mu.RUnlock()

	names := make([]string, 0, len(is.interfaces))
	for name := range is.interfaces {
		names = append(names, name)
	}
	return names
}

// ResolveDirections parses rtpengine direction array and returns interface names
func ResolveDirections(directions []string) (from, to string) {
	if len(directions) == 0 {
		return "", ""
	}
	if len(directions) == 1 {
		return directions[0], directions[0]
	}
	return directions[0], directions[1]
}

// AutoDetectLocalIP attempts to auto-detect a suitable local IP
func AutoDetectLocalIP() string {
	// Try to find a non-loopback, non-link-local address
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}

	var fallback string
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			ip := ipnet.IP
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}
			// Prefer IPv4
			if ip4 := ip.To4(); ip4 != nil {
				// Prefer non-private addresses for external-facing
				if !ip.IsPrivate() {
					return ip4.String()
				}
				if fallback == "" {
					fallback = ip4.String()
				}
			}
		}
	}

	if fallback != "" {
		return fallback
	}
	return "127.0.0.1"
}
