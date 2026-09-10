package dhcpd

import (
	"net"
	"time"
)

const LeaseTime = 12 * time.Hour

func DNSServers() []net.IP {
	return []net.IP{
		net.IPv4(1, 1, 1, 1),
		net.IPv4(8, 8, 8, 8),
	}
}

type SubnetConfig struct {
	Network        *net.IPNet
	InterfaceIP    net.IP
	VPCRoute       *net.IPNet
	DefaultGateway net.IP
}

type Host struct {
	MAC          net.HardwareAddr
	IP           net.IP
	VM           string
	DefaultRoute bool
}
