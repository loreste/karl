package internal

import (
	"net"
	"strings"
	"testing"
)

func TestNewIPAddressSelector(t *testing.T) {
	selector := NewIPAddressSelector()
	if selector == nil {
		t.Fatal("NewIPAddressSelector returned nil")
	}
}

func TestIPAddressSelector_AddAddress(t *testing.T) {
	selector := &IPAddressSelector{
		ipv4Addresses: make([]net.IP, 0),
		ipv6Addresses: make([]net.IP, 0),
	}

	// Add IPv4
	selector.AddAddress(net.ParseIP("192.168.1.100"))
	ipv4, ipv6 := selector.GetAddresses()
	if len(ipv4) != 1 {
		t.Errorf("Expected 1 IPv4 address, got %d", len(ipv4))
	}
	if len(ipv6) != 0 {
		t.Errorf("Expected 0 IPv6 addresses, got %d", len(ipv6))
	}

	// Add IPv6
	selector.AddAddress(net.ParseIP("2001:db8::1"))
	ipv4, ipv6 = selector.GetAddresses()
	if len(ipv4) != 1 {
		t.Errorf("Expected 1 IPv4 address, got %d", len(ipv4))
	}
	if len(ipv6) != 1 {
		t.Errorf("Expected 1 IPv6 address, got %d", len(ipv6))
	}
}

func TestIPAddressSelector_SelectAddress_IPv4(t *testing.T) {
	selector := &IPAddressSelector{
		ipv4Addresses: []net.IP{net.ParseIP("192.168.1.100")},
		ipv6Addresses: []net.IP{net.ParseIP("2001:db8::1")},
	}

	addr := selector.SelectAddress(AddressFamilyIPv4, nil)
	if addr == nil {
		t.Fatal("SelectAddress returned nil")
	}

	if addr.To4() == nil {
		t.Error("Expected IPv4 address")
	}
}

func TestIPAddressSelector_SelectAddress_IPv6(t *testing.T) {
	selector := &IPAddressSelector{
		ipv4Addresses: []net.IP{net.ParseIP("192.168.1.100")},
		ipv6Addresses: []net.IP{net.ParseIP("2001:db8::1")},
	}

	addr := selector.SelectAddress(AddressFamilyIPv6, nil)
	if addr == nil {
		t.Fatal("SelectAddress returned nil")
	}

	if addr.To4() != nil {
		t.Error("Expected IPv6 address")
	}
}

func TestIPAddressSelector_SelectAddress_MatchPeer(t *testing.T) {
	selector := &IPAddressSelector{
		ipv4Addresses: []net.IP{net.ParseIP("192.168.1.100")},
		ipv6Addresses: []net.IP{net.ParseIP("2001:db8::1")},
	}

	// IPv4 peer should select IPv4 address
	peer := net.ParseIP("10.0.0.1")
	addr := selector.SelectAddress(AddressFamilyAny, peer)
	if addr == nil {
		t.Fatal("SelectAddress returned nil")
	}
	if addr.To4() == nil {
		t.Error("IPv4 peer should select IPv4 address")
	}

	// IPv6 peer should select IPv6 address
	peer = net.ParseIP("2001:db8::100")
	addr = selector.SelectAddress(AddressFamilyAny, peer)
	if addr == nil {
		t.Fatal("SelectAddress returned nil")
	}
	if addr.To4() != nil {
		t.Error("IPv6 peer should select IPv6 address")
	}
}

func TestIPAddressSelector_SelectAddress_Any(t *testing.T) {
	selector := &IPAddressSelector{
		ipv4Addresses: []net.IP{net.ParseIP("192.168.1.100")},
		ipv6Addresses: []net.IP{net.ParseIP("2001:db8::1")},
		preferIPv6:    false,
	}

	// Default prefers IPv4
	addr := selector.SelectAddress(AddressFamilyAny, nil)
	if addr == nil {
		t.Fatal("SelectAddress returned nil")
	}
	if addr.To4() == nil {
		t.Error("Should prefer IPv4 by default")
	}

	// With preferIPv6 set
	selector.SetPreferIPv6(true)
	addr = selector.SelectAddress(AddressFamilyAny, nil)
	if addr == nil {
		t.Fatal("SelectAddress returned nil")
	}
	if addr.To4() != nil {
		t.Error("Should prefer IPv6 when set")
	}
}

func TestIPAddressSelector_SelectAddress_NoAddresses(t *testing.T) {
	selector := &IPAddressSelector{
		ipv4Addresses: make([]net.IP, 0),
		ipv6Addresses: make([]net.IP, 0),
	}

	addr := selector.SelectAddress(AddressFamilyIPv4, nil)
	if addr != nil {
		t.Error("Expected nil when no addresses available")
	}
}

func TestIPAddressSelector_IsDualStack(t *testing.T) {
	tests := []struct {
		name     string
		ipv4     []net.IP
		ipv6     []net.IP
		expected bool
	}{
		{
			name:     "dual stack",
			ipv4:     []net.IP{net.ParseIP("192.168.1.100")},
			ipv6:     []net.IP{net.ParseIP("2001:db8::1")},
			expected: true,
		},
		{
			name:     "ipv4 only",
			ipv4:     []net.IP{net.ParseIP("192.168.1.100")},
			ipv6:     []net.IP{},
			expected: false,
		},
		{
			name:     "ipv6 only",
			ipv4:     []net.IP{},
			ipv6:     []net.IP{net.ParseIP("2001:db8::1")},
			expected: false,
		},
		{
			name:     "no addresses",
			ipv4:     []net.IP{},
			ipv6:     []net.IP{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := &IPAddressSelector{
				ipv4Addresses: tt.ipv4,
				ipv6Addresses: tt.ipv6,
			}
			if selector.IsDualStack() != tt.expected {
				t.Errorf("IsDualStack() = %v, expected %v", selector.IsDualStack(), tt.expected)
			}
		})
	}
}

func TestParseAddressFamily(t *testing.T) {
	tests := []struct {
		input    string
		expected AddressFamily
	}{
		{"inet", AddressFamilyIPv4},
		{"ipv4", AddressFamilyIPv4},
		{"ip4", AddressFamilyIPv4},
		{"INET", AddressFamilyIPv4},
		{"inet6", AddressFamilyIPv6},
		{"ipv6", AddressFamilyIPv6},
		{"ip6", AddressFamilyIPv6},
		{"INET6", AddressFamilyIPv6},
		{"any", AddressFamilyAny},
		{"unknown", AddressFamilyAny},
		{"", AddressFamilyAny},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseAddressFamily(tt.input)
			if result != tt.expected {
				t.Errorf("ParseAddressFamily(%s) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatSDPAddressType(t *testing.T) {
	tests := []struct {
		ip       net.IP
		expected string
	}{
		{net.ParseIP("192.168.1.100"), "IP4"},
		{net.ParseIP("10.0.0.1"), "IP4"},
		{net.ParseIP("2001:db8::1"), "IP6"},
		{net.ParseIP("::1"), "IP6"},
	}

	for _, tt := range tests {
		t.Run(tt.ip.String(), func(t *testing.T) {
			result := FormatSDPAddressType(tt.ip)
			if result != tt.expected {
				t.Errorf("FormatSDPAddressType(%v) = %s, expected %s", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestFormatSDPConnection(t *testing.T) {
	tests := []struct {
		ip       net.IP
		expected string
	}{
		{net.ParseIP("192.168.1.100"), "c=IN IP4 192.168.1.100\r\n"},
		{net.ParseIP("2001:db8::1"), "c=IN IP6 2001:db8::1\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.ip.String(), func(t *testing.T) {
			result := FormatSDPConnection(tt.ip)
			if result != tt.expected {
				t.Errorf("FormatSDPConnection(%v) = %s, expected %s", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestFormatSDPOrigin(t *testing.T) {
	ip := net.ParseIP("192.168.1.100")
	result := FormatSDPOrigin("karl", 123456, 1, ip)

	if !strings.HasPrefix(result, "o=karl ") {
		t.Error("Origin should start with o=karl")
	}
	if !strings.Contains(result, "IN IP4 192.168.1.100") {
		t.Error("Origin should contain IP4 address")
	}
	if !strings.HasSuffix(result, "\r\n") {
		t.Error("Origin should end with CRLF")
	}
}

func TestIsIPv4MappedIPv6(t *testing.T) {
	// Note: In Go, net.ParseIP stores IPv4 addresses as IPv4-mapped IPv6 (16-byte slice)
	// So we use To4() to get the 4-byte representation for "pure" IPv4 testing
	tests := []struct {
		ip       net.IP
		expected bool
	}{
		{net.ParseIP("192.168.1.100").To4(), false},     // Pure IPv4 (4-byte)
		{net.ParseIP("2001:db8::1"), false},             // Pure IPv6
		{net.ParseIP("::ffff:192.168.1.100"), true},     // IPv4-mapped IPv6
		{net.ParseIP("::1"), false},                     // Loopback
	}

	for _, tt := range tests {
		t.Run(tt.ip.String(), func(t *testing.T) {
			result := IsIPv4MappedIPv6(tt.ip)
			if result != tt.expected {
				t.Errorf("IsIPv4MappedIPv6(%v) = %v, expected %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestToCanonicalIP(t *testing.T) {
	// IPv4-mapped IPv6 should be converted to IPv4
	mapped := net.ParseIP("::ffff:192.168.1.100")
	result := ToCanonicalIP(mapped)
	if result.To4() == nil {
		t.Error("Should convert to IPv4")
	}

	// Pure IPv4 stays IPv4
	ipv4 := net.ParseIP("192.168.1.100")
	result = ToCanonicalIP(ipv4)
	if result.To4() == nil {
		t.Error("IPv4 should stay IPv4")
	}

	// Pure IPv6 stays IPv6
	ipv6 := net.ParseIP("2001:db8::1")
	result = ToCanonicalIP(ipv6)
	if result.To4() != nil {
		t.Error("IPv6 should stay IPv6")
	}
}

func TestCompareAddressFamily(t *testing.T) {
	tests := []struct {
		name     string
		ip1      net.IP
		ip2      net.IP
		expected bool
	}{
		{"both ipv4", net.ParseIP("192.168.1.1"), net.ParseIP("10.0.0.1"), true},
		{"both ipv6", net.ParseIP("2001:db8::1"), net.ParseIP("2001:db8::2"), true},
		{"ipv4 vs ipv6", net.ParseIP("192.168.1.1"), net.ParseIP("2001:db8::1"), false},
		{"ipv6 vs ipv4", net.ParseIP("2001:db8::1"), net.ParseIP("192.168.1.1"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareAddressFamily(tt.ip1, tt.ip2)
			if result != tt.expected {
				t.Errorf("CompareAddressFamily(%v, %v) = %v, expected %v",
					tt.ip1, tt.ip2, result, tt.expected)
			}
		})
	}
}

func TestAddressFamily_Constants(t *testing.T) {
	if AddressFamilyIPv4 != "inet" {
		t.Error("AddressFamilyIPv4 should be inet")
	}
	if AddressFamilyIPv6 != "inet6" {
		t.Error("AddressFamilyIPv6 should be inet6")
	}
	if AddressFamilyAny != "any" {
		t.Error("AddressFamilyAny should be any")
	}
}

func TestDualStackListener_GetConn(t *testing.T) {
	// Test with mock listeners
	ds := &DualStackListener{
		ipv4Listener: nil, // We can't easily create UDP listeners in tests
		ipv6Listener: nil,
	}

	// With no listeners, should return nil
	conn := ds.GetConn(net.ParseIP("192.168.1.100"))
	if conn != nil {
		t.Error("Expected nil when no listeners")
	}
}

func TestDualStackListener_IsDualStack(t *testing.T) {
	ds := &DualStackListener{
		ipv4Listener: nil,
		ipv6Listener: nil,
	}

	if ds.IsDualStack() {
		t.Error("Should not be dual stack with no listeners")
	}
}

func TestIPAddressSelector_GetAddresses_ReturnsCopy(t *testing.T) {
	selector := &IPAddressSelector{
		ipv4Addresses: []net.IP{net.ParseIP("192.168.1.100")},
		ipv6Addresses: []net.IP{net.ParseIP("2001:db8::1")},
	}

	ipv4, ipv6 := selector.GetAddresses()

	// Modify returned slices
	ipv4[0] = net.ParseIP("10.0.0.1")
	ipv6[0] = net.ParseIP("2001:db8::2")

	// Original should be unchanged
	origIpv4, origIpv6 := selector.GetAddresses()
	if origIpv4[0].String() == "10.0.0.1" {
		t.Error("GetAddresses should return a copy, not the original")
	}
	if origIpv6[0].String() == "2001:db8::2" {
		t.Error("GetAddresses should return a copy, not the original")
	}
}

func TestIPAddressSelector_SetPreferIPv6(t *testing.T) {
	selector := &IPAddressSelector{
		ipv4Addresses: []net.IP{net.ParseIP("192.168.1.100")},
		ipv6Addresses: []net.IP{net.ParseIP("2001:db8::1")},
		preferIPv6:    false,
	}

	// Initially prefers IPv4
	addr := selector.SelectAddress(AddressFamilyAny, nil)
	if addr.To4() == nil {
		t.Error("Should prefer IPv4 initially")
	}

	selector.SetPreferIPv6(true)

	// Now prefers IPv6
	addr = selector.SelectAddress(AddressFamilyAny, nil)
	if addr.To4() != nil {
		t.Error("Should prefer IPv6 after setting preference")
	}
}
