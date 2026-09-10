package dhcpd

import (
	"errors"
	"fmt"
	"net"

	"git.g3e.fr/syonad/two/internal/metadata"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

var (
	ErrNoInterfaceIP = errors.New("interface ip is required: guests would have no route to the metadata server")
	ErrNoNetwork     = errors.New("subnet network is required")
	ErrNoHostIP      = errors.New("host ip is required")
	ErrNoRequest     = errors.New("request is nil")
)

func metadataRoute() *net.IPNet {
	return &net.IPNet{
		IP:   net.ParseIP(metadata.ServiceIP).To4(),
		Mask: net.CIDRMask(32, 32),
	}
}

func defaultRoute() *net.IPNet {
	return &net.IPNet{
		IP:   net.IPv4zero.To4(),
		Mask: net.CIDRMask(0, 32),
	}
}

func Routes(c SubnetConfig, h Host) (dhcpv4.Routes, error) {
	if c.InterfaceIP == nil {
		return nil, ErrNoInterfaceIP
	}

	routes := dhcpv4.Routes{{Dest: metadataRoute(), Router: c.InterfaceIP}}

	if c.VPCRoute != nil {
		routes = append(routes, &dhcpv4.Route{Dest: c.VPCRoute, Router: c.InterfaceIP})
	}
	if h.DefaultRoute && c.DefaultGateway != nil {
		routes = append(routes, &dhcpv4.Route{Dest: defaultRoute(), Router: c.DefaultGateway})
	}
	return routes, nil
}

func replyType(req *dhcpv4.DHCPv4) (dhcpv4.MessageType, error) {
	switch req.MessageType() {
	case dhcpv4.MessageTypeDiscover:
		return dhcpv4.MessageTypeOffer, nil
	case dhcpv4.MessageTypeRequest:
		return dhcpv4.MessageTypeAck, nil
	default:
		return 0, fmt.Errorf("no reply built for message type %s", req.MessageType())
	}
}

func BuildReply(c SubnetConfig, h Host, req *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, error) {
	if req == nil {
		return nil, ErrNoRequest
	}
	if c.Network == nil {
		return nil, ErrNoNetwork
	}
	if h.IP == nil {
		return nil, ErrNoHostIP
	}

	kind, err := replyType(req)
	if err != nil {
		return nil, err
	}

	routes, err := Routes(c, h)
	if err != nil {
		return nil, err
	}

	mods := []dhcpv4.Modifier{
		dhcpv4.WithMessageType(kind),
		dhcpv4.WithServerIP(c.InterfaceIP),
		dhcpv4.WithYourIP(h.IP),
		dhcpv4.WithNetmask(c.Network.Mask),
		dhcpv4.WithLeaseTime(uint32(LeaseTime.Seconds())),
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(c.InterfaceIP)),
		dhcpv4.WithOption(dhcpv4.OptDNS(DNSServers()...)),
		dhcpv4.WithOption(dhcpv4.OptClasslessStaticRoute(routes...)),
	}
	if h.DefaultRoute && c.DefaultGateway != nil {
		mods = append(mods, dhcpv4.WithOption(dhcpv4.OptRouter(c.DefaultGateway)))
	}

	return dhcpv4.NewReplyFromRequest(req, mods...)
}
