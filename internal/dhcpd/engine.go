package dhcpd

import (
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

func answerable(req *dhcpv4.DHCPv4) bool {
	switch req.MessageType() {
	case dhcpv4.MessageTypeDiscover, dhcpv4.MessageTypeRequest:
		return true
	default:
		return false
	}
}

func (s *Store) Handle(req *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, error) {
	if req == nil {
		return nil, ErrNoRequest
	}
	if !answerable(req) {
		return nil, nil
	}

	subnet, configured := s.Subnet()
	if !configured {
		return nil, nil
	}

	host, known := s.Lookup(req.ClientHWAddr)
	if !known {
		return nil, nil
	}

	return BuildReply(subnet, host, req)
}

func (s *Store) Probe(mac net.HardwareAddr) (*dhcpv4.DHCPv4, error) {
	if len(mac) == 0 {
		return nil, ErrNoMAC
	}

	subnet, configured := s.Subnet()
	if !configured {
		return nil, ErrNotConfigured
	}

	host, known := s.Lookup(mac)
	if !known {
		return nil, nil
	}

	req, err := dhcpv4.New(dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest), dhcpv4.WithHwAddr(mac))
	if err != nil {
		return nil, err
	}

	return BuildReply(subnet, host, req)
}
