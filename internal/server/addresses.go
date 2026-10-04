package server

import (
	"net"
	"slices"
)

// UpdateAddresses refreshes the detected LAN addresses without rotating the
// session. Invalid addresses are ignored; an empty snapshot uses localhost.
// The first valid IPv4 address remains the default join address.
func (s *Server) UpdateAddresses(addresses []NetworkAddress) {
	normalized, allowedHosts := normalizeAddresses(addresses)
	s.mu.Lock()
	if s.closed || slices.Equal(s.config.Addresses, normalized) {
		s.mu.Unlock()
		return
	}
	s.config.Addresses = normalized
	s.allowedHosts = allowedHosts
	s.mu.Unlock()
	s.broadcast()
}

func normalizeAddresses(addresses []NetworkAddress) ([]NetworkAddress, map[string]bool) {
	allowedHosts := map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true}
	normalized := make([]NetworkAddress, 0, len(addresses))
	for _, address := range addresses {
		ip := net.ParseIP(address.IP)
		if ip == nil || ip.To4() == nil || !ip.IsGlobalUnicast() {
			continue
		}
		address.IP = ip.To4().String()
		if allowedHosts[address.IP] {
			continue
		}
		address.Name = cleanLabel(address.Name, "Jaringan lokal")
		allowedHosts[address.IP] = true
		normalized = append(normalized, address)
	}
	return normalized, allowedHosts
}
