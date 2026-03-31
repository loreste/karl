package internal

import (
	"net"
	"testing"
)

func TestNewInterfaceSelector(t *testing.T) {
	config := &Config{
		Integration: IntegrationConfig{
			MediaIP:  "192.168.1.100",
			PublicIP: "203.0.113.50",
			Interfaces: map[string]*NetworkInterfaceConfig{
				"lan": {
					Address:       "10.0.0.1",
					AdvertiseAddr: "10.0.0.1",
					Port:          20000,
				},
				"wan": {
					Address:       "192.168.1.100",
					AdvertiseAddr: "203.0.113.50",
					Port:          30000,
				},
			},
		},
	}

	is := NewInterfaceSelector(config)
	if is == nil {
		t.Fatal("NewInterfaceSelector returned nil")
	}

	// Check that default interfaces were created
	names := is.GetInterfaceNames()
	if len(names) == 0 {
		t.Error("No interfaces configured")
	}

	// Check internal networks were set up
	if len(is.internalNets) == 0 {
		t.Error("Internal networks not configured")
	}
}

func TestInterfaceSelector_SelectInterface_ByName(t *testing.T) {
	is := &InterfaceSelector{
		interfaces: make(map[string]*InterfaceInfo),
	}

	is.AddInterface("test", &InterfaceInfo{
		LocalAddress:  "192.168.1.100",
		AdvertiseAddr: "203.0.113.50",
	})

	result := is.SelectInterface("test", nil, nil)
	if result == nil {
		t.Fatal("SelectInterface returned nil for existing interface")
	}

	if result.LocalAddress != "192.168.1.100" {
		t.Errorf("Expected LocalAddress 192.168.1.100, got %s", result.LocalAddress)
	}
}

func TestInterfaceSelector_SelectInterface_ByDirection(t *testing.T) {
	is := &InterfaceSelector{
		interfaces: make(map[string]*InterfaceInfo),
	}

	is.AddInterface("internal", &InterfaceInfo{
		LocalAddress:  "10.0.0.1",
		AdvertiseAddr: "10.0.0.1",
		IsInternal:    true,
	})
	is.AddInterface("external", &InterfaceInfo{
		LocalAddress:  "192.168.1.100",
		AdvertiseAddr: "203.0.113.50",
		IsInternal:    false,
	})

	// Test direction-based selection
	result := is.SelectInterface("", []string{"internal", "external"}, nil)
	if result == nil {
		t.Fatal("SelectInterface returned nil")
	}
	if result.Name != "external" {
		t.Errorf("Expected external interface, got %s", result.Name)
	}
}

func TestInterfaceSelector_SelectInterface_ByPeerAddress(t *testing.T) {
	is := &InterfaceSelector{
		interfaces:   make(map[string]*InterfaceInfo),
		internalNets: make([]*net.IPNet, 0),
	}

	// Add internal network
	_, ipnet, _ := net.ParseCIDR("10.0.0.0/8")
	is.internalNets = append(is.internalNets, ipnet)

	is.AddInterface("internal", &InterfaceInfo{
		LocalAddress:  "10.0.0.1",
		AdvertiseAddr: "10.0.0.1",
		IsInternal:    true,
	})
	is.AddInterface("external", &InterfaceInfo{
		LocalAddress:  "192.168.1.100",
		AdvertiseAddr: "203.0.113.50",
		IsInternal:    false,
	})

	// Test with internal peer
	internalPeer := net.ParseIP("10.0.0.50")
	result := is.SelectInterface("", nil, internalPeer)
	if result == nil {
		t.Fatal("SelectInterface returned nil for internal peer")
	}
	if result.Name != "internal" {
		t.Errorf("Expected internal interface for internal peer, got %s", result.Name)
	}

	// Test with external peer
	externalPeer := net.ParseIP("203.0.113.100")
	result = is.SelectInterface("", nil, externalPeer)
	if result == nil {
		t.Fatal("SelectInterface returned nil for external peer")
	}
	if result.Name != "external" {
		t.Errorf("Expected external interface for external peer, got %s", result.Name)
	}
}

func TestInterfaceSelector_AddPeerRule(t *testing.T) {
	is := &InterfaceSelector{
		interfaces: make(map[string]*InterfaceInfo),
		peerRules:  make([]PeerRule, 0),
	}

	is.AddInterface("custom", &InterfaceInfo{
		LocalAddress:  "172.16.0.1",
		AdvertiseAddr: "172.16.0.1",
	})

	err := is.AddPeerRule("172.16.0.0/12", "custom")
	if err != nil {
		t.Fatalf("AddPeerRule failed: %v", err)
	}

	// Verify rule was added
	if len(is.peerRules) != 1 {
		t.Errorf("Expected 1 peer rule, got %d", len(is.peerRules))
	}

	// Test peer matching
	peer := net.ParseIP("172.20.0.50")
	result := is.SelectInterface("", nil, peer)
	if result == nil {
		t.Fatal("SelectInterface returned nil for peer matching rule")
	}
	if result.Name != "custom" {
		t.Errorf("Expected custom interface for peer rule match, got %s", result.Name)
	}
}

func TestInterfaceSelector_AddPeerRule_InvalidCIDR(t *testing.T) {
	is := &InterfaceSelector{
		peerRules: make([]PeerRule, 0),
	}

	err := is.AddPeerRule("invalid", "test")
	if err == nil {
		t.Error("Expected error for invalid CIDR")
	}
}

func TestInterfaceSelector_GetAdvertiseAddress(t *testing.T) {
	is := &InterfaceSelector{
		interfaces: make(map[string]*InterfaceInfo),
	}

	is.AddInterface("default", &InterfaceInfo{
		LocalAddress:  "192.168.1.100",
		AdvertiseAddr: "203.0.113.50",
	})
	is.SetDefaultInterface("default")

	// Should return advertise address
	addr := is.GetAdvertiseAddress("default", nil)
	if addr != "203.0.113.50" {
		t.Errorf("Expected 203.0.113.50, got %s", addr)
	}
}

func TestInterfaceSelector_GetAdvertiseAddress_NoAdvertise(t *testing.T) {
	is := &InterfaceSelector{
		interfaces: make(map[string]*InterfaceInfo),
	}

	is.AddInterface("simple", &InterfaceInfo{
		LocalAddress: "192.168.1.100",
	})
	is.SetDefaultInterface("simple")

	// Should return local address when no advertise address
	addr := is.GetAdvertiseAddress("simple", nil)
	if addr != "192.168.1.100" {
		t.Errorf("Expected 192.168.1.100, got %s", addr)
	}
}

func TestInterfaceSelector_GetLocalAddress(t *testing.T) {
	is := &InterfaceSelector{
		interfaces: make(map[string]*InterfaceInfo),
	}

	is.AddInterface("test", &InterfaceInfo{
		LocalAddress: "10.0.0.1",
	})

	addr := is.GetLocalAddress("test")
	if addr != "10.0.0.1" {
		t.Errorf("Expected 10.0.0.1, got %s", addr)
	}
}

func TestInterfaceSelector_AddInternalNetwork(t *testing.T) {
	is := &InterfaceSelector{
		internalNets: make([]*net.IPNet, 0),
	}

	err := is.AddInternalNetwork("100.64.0.0/10")
	if err != nil {
		t.Fatalf("AddInternalNetwork failed: %v", err)
	}

	if len(is.internalNets) != 1 {
		t.Errorf("Expected 1 internal network, got %d", len(is.internalNets))
	}

	// Verify network is considered internal
	if !is.isInternal(net.ParseIP("100.64.0.1")) {
		t.Error("100.64.0.1 should be internal")
	}
}

func TestInterfaceSelector_IsInternal(t *testing.T) {
	is := &InterfaceSelector{
		internalNets: make([]*net.IPNet, 0),
	}

	// Add common private networks
	cidrs := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
	for _, cidr := range cidrs {
		_, ipnet, _ := net.ParseCIDR(cidr)
		is.internalNets = append(is.internalNets, ipnet)
	}

	tests := []struct {
		ip       string
		expected bool
	}{
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"8.8.8.8", false},
		{"203.0.113.1", false},
	}

	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		result := is.isInternal(ip)
		if result != tt.expected {
			t.Errorf("isInternal(%s) = %v, expected %v", tt.ip, result, tt.expected)
		}
	}
}

func TestResolveDirections(t *testing.T) {
	tests := []struct {
		directions []string
		wantFrom   string
		wantTo     string
	}{
		{nil, "", ""},
		{[]string{}, "", ""},
		{[]string{"internal"}, "internal", "internal"},
		{[]string{"internal", "external"}, "internal", "external"},
		{[]string{"external", "internal"}, "external", "internal"},
	}

	for _, tt := range tests {
		from, to := ResolveDirections(tt.directions)
		if from != tt.wantFrom || to != tt.wantTo {
			t.Errorf("ResolveDirections(%v) = (%s, %s), want (%s, %s)",
				tt.directions, from, to, tt.wantFrom, tt.wantTo)
		}
	}
}

func TestAutoDetectLocalIP(t *testing.T) {
	ip := AutoDetectLocalIP()
	if ip == "" {
		t.Error("AutoDetectLocalIP returned empty string")
	}

	// Should be a valid IP
	parsed := net.ParseIP(ip)
	if parsed == nil {
		t.Errorf("AutoDetectLocalIP returned invalid IP: %s", ip)
	}
}

func TestInterfaceSelector_SetDefaultInterface(t *testing.T) {
	is := &InterfaceSelector{
		interfaces: make(map[string]*InterfaceInfo),
	}

	is.AddInterface("primary", &InterfaceInfo{
		LocalAddress: "192.168.1.100",
	})
	is.AddInterface("secondary", &InterfaceInfo{
		LocalAddress: "192.168.1.101",
	})

	is.SetDefaultInterface("primary")

	// Select without any criteria should return default
	result := is.SelectInterface("", nil, nil)
	if result == nil {
		t.Fatal("SelectInterface returned nil")
	}
	if result.LocalAddress != "192.168.1.100" {
		t.Errorf("Expected primary interface, got %s", result.LocalAddress)
	}
}

func TestInterfaceSelector_GetInterfaceNames(t *testing.T) {
	is := &InterfaceSelector{
		interfaces: make(map[string]*InterfaceInfo),
	}

	is.AddInterface("eth0", &InterfaceInfo{LocalAddress: "10.0.0.1"})
	is.AddInterface("eth1", &InterfaceInfo{LocalAddress: "10.0.0.2"})
	is.AddInterface("wlan0", &InterfaceInfo{LocalAddress: "10.0.0.3"})

	names := is.GetInterfaceNames()
	if len(names) != 3 {
		t.Errorf("Expected 3 interface names, got %d", len(names))
	}
}
