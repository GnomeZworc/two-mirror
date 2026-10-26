package dhcpd

import (
	"net"
	"time"
)

const LeaseTime = 12 * time.Hour

type SubnetConfig struct {
	Network        *net.IPNet
	InterfaceIP    net.IP
	VPCRoute       *net.IPNet
	DefaultGateway net.IP
	MTU            int
}

func (c SubnetConfig) InterfaceMTU() int {
	if c.MTU == 0 {
		return 1500
	}
	return c.MTU
}

type Host struct {
	MAC          net.HardwareAddr
	IP           net.IP
	VM           string
	DefaultRoute bool
}
