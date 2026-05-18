package dhcp

import (
	"net"
)

type Config struct {
	Network        *net.IPNet
	VPCGateway     net.IP     // next-hop for VPCRoute (option 121)
	VPCRoute       *net.IPNet // if non-nil, emit dhcp-option=121,VPCRoute,VPCGateway
	DefaultGateway net.IP     // if non-nil, emit dhcp-option=3,DefaultGateway
	Name           string
	ConfDir        string
}
