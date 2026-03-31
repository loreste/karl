package internal

import (
	"fmt"
	"net"
	"strings"
	"sync"
)

// AddressFamily represents IP address family
type AddressFamily string

const (
	AddressFamilyIPv4 AddressFamily = "inet"
	AddressFamilyIPv6 AddressFamily = "inet6"
	AddressFamilyAny  AddressFamily = "any"
)

// IPAddressSelector handles dual-stack IP address selection
type IPAddressSelector struct {
	ipv4Addresses []net.IP
	ipv6Addresses []net.IP
	preferIPv6    bool
	mu            sync.RWMutex
}

// NewIPAddressSelector creates a new IP address selector
func NewIPAddressSelector() *IPAddressSelector {
	selector := &IPAddressSelector{
		ipv4Addresses: make([]net.IP, 0),
		ipv6Addresses: make([]net.IP, 0),
		preferIPv6:    false,
	}

	// Auto-discover local addresses
	selector.discoverLocalAddresses()

	return selector
}

// discoverLocalAddresses discovers local IP addresses
func (s *IPAddressSelector) discoverLocalAddresses() {
	s.mu.Lock()
	defer s.mu.Unlock()

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return
	}

	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		ip := ipnet.IP
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			continue
		}

		if ip4 := ip.To4(); ip4 != nil {
			s.ipv4Addresses = append(s.ipv4Addresses, ip4)
		} else if ip.To16() != nil {
			s.ipv6Addresses = append(s.ipv6Addresses, ip)
		}
	}
}

// SelectAddress selects an appropriate address based on family preference
func (s *IPAddressSelector) SelectAddress(family AddressFamily, peer net.IP) net.IP {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// If peer is specified, match family
	if peer != nil {
		if peer.To4() != nil {
			family = AddressFamilyIPv4
		} else {
			family = AddressFamilyIPv6
		}
	}

	switch family {
	case AddressFamilyIPv4:
		if len(s.ipv4Addresses) > 0 {
			return s.ipv4Addresses[0]
		}
	case AddressFamilyIPv6:
		if len(s.ipv6Addresses) > 0 {
			return s.ipv6Addresses[0]
		}
	case AddressFamilyAny:
		if s.preferIPv6 && len(s.ipv6Addresses) > 0 {
			return s.ipv6Addresses[0]
		}
		if len(s.ipv4Addresses) > 0 {
			return s.ipv4Addresses[0]
		}
		if len(s.ipv6Addresses) > 0 {
			return s.ipv6Addresses[0]
		}
	}

	return nil
}

// AddAddress adds an address to the selector
func (s *IPAddressSelector) AddAddress(ip net.IP) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ip.To4() != nil {
		s.ipv4Addresses = append(s.ipv4Addresses, ip.To4())
	} else {
		s.ipv6Addresses = append(s.ipv6Addresses, ip)
	}
}

// SetPreferIPv6 sets IPv6 preference
func (s *IPAddressSelector) SetPreferIPv6(prefer bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.preferIPv6 = prefer
}

// GetAddresses returns all addresses
func (s *IPAddressSelector) GetAddresses() (ipv4 []net.IP, ipv6 []net.IP) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ipv4 = make([]net.IP, len(s.ipv4Addresses))
	copy(ipv4, s.ipv4Addresses)

	ipv6 = make([]net.IP, len(s.ipv6Addresses))
	copy(ipv6, s.ipv6Addresses)

	return
}

// IsDualStack returns true if both IPv4 and IPv6 are available
func (s *IPAddressSelector) IsDualStack() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.ipv4Addresses) > 0 && len(s.ipv6Addresses) > 0
}

// ParseAddressFamily parses address family string
func ParseAddressFamily(family string) AddressFamily {
	switch strings.ToLower(family) {
	case "inet", "ipv4", "ip4":
		return AddressFamilyIPv4
	case "inet6", "ipv6", "ip6":
		return AddressFamilyIPv6
	default:
		return AddressFamilyAny
	}
}

// FormatSDPAddressType returns SDP address type string
func FormatSDPAddressType(ip net.IP) string {
	if ip.To4() != nil {
		return "IP4"
	}
	return "IP6"
}

// FormatSDPConnection formats SDP c= line
func FormatSDPConnection(ip net.IP) string {
	addrType := FormatSDPAddressType(ip)
	return fmt.Sprintf("c=IN %s %s\r\n", addrType, ip.String())
}

// FormatSDPOrigin formats SDP o= line
func FormatSDPOrigin(username string, sessionID, sessionVersion int64, ip net.IP) string {
	addrType := FormatSDPAddressType(ip)
	return fmt.Sprintf("o=%s %d %d IN %s %s\r\n",
		username, sessionID, sessionVersion, addrType, ip.String())
}

// IsIPv4MappedIPv6 checks if an IPv6 address is an IPv4-mapped address
func IsIPv4MappedIPv6(ip net.IP) bool {
	if ip.To4() != nil && len(ip) == 16 {
		// Check for ::ffff:x.x.x.x format
		for i := 0; i < 10; i++ {
			if ip[i] != 0 {
				return false
			}
		}
		return ip[10] == 0xff && ip[11] == 0xff
	}
	return false
}

// ToCanonicalIP converts IPv4-mapped IPv6 addresses to IPv4
func ToCanonicalIP(ip net.IP) net.IP {
	if ip4 := ip.To4(); ip4 != nil {
		return ip4
	}
	return ip
}

// CompareAddressFamily checks if two IPs are in the same family
func CompareAddressFamily(ip1, ip2 net.IP) bool {
	ip1v4 := ip1.To4() != nil
	ip2v4 := ip2.To4() != nil
	return ip1v4 == ip2v4
}

// DualStackListener creates listeners for both IPv4 and IPv6
type DualStackListener struct {
	ipv4Listener *net.UDPConn
	ipv6Listener *net.UDPConn
	mu           sync.RWMutex
}

// NewDualStackListener creates a new dual-stack listener
func NewDualStackListener(port int) (*DualStackListener, error) {
	ds := &DualStackListener{}

	// Try IPv4
	ipv4Addr := &net.UDPAddr{IP: net.IPv4zero, Port: port}
	ipv4Conn, err := net.ListenUDP("udp4", ipv4Addr)
	if err == nil {
		ds.ipv4Listener = ipv4Conn
	}

	// Try IPv6
	ipv6Addr := &net.UDPAddr{IP: net.IPv6zero, Port: port}
	ipv6Conn, err := net.ListenUDP("udp6", ipv6Addr)
	if err == nil {
		ds.ipv6Listener = ipv6Conn
	}

	if ds.ipv4Listener == nil && ds.ipv6Listener == nil {
		return nil, fmt.Errorf("failed to create any listener on port %d", port)
	}

	return ds, nil
}

// Close closes all listeners
func (ds *DualStackListener) Close() error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	var errs []error
	if ds.ipv4Listener != nil {
		if err := ds.ipv4Listener.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if ds.ipv6Listener != nil {
		if err := ds.ipv6Listener.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// GetIPv4Conn returns the IPv4 connection
func (ds *DualStackListener) GetIPv4Conn() *net.UDPConn {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.ipv4Listener
}

// GetIPv6Conn returns the IPv6 connection
func (ds *DualStackListener) GetIPv6Conn() *net.UDPConn {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.ipv6Listener
}

// GetConn returns the appropriate connection for a peer
func (ds *DualStackListener) GetConn(peer net.IP) *net.UDPConn {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	if peer.To4() != nil {
		if ds.ipv4Listener != nil {
			return ds.ipv4Listener
		}
	} else {
		if ds.ipv6Listener != nil {
			return ds.ipv6Listener
		}
	}

	// Fallback to any available
	if ds.ipv4Listener != nil {
		return ds.ipv4Listener
	}
	return ds.ipv6Listener
}

// IsDualStack returns true if both IPv4 and IPv6 are available
func (ds *DualStackListener) IsDualStack() bool {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.ipv4Listener != nil && ds.ipv6Listener != nil
}
