package dhcp

import (
	"net"
)

const DefaultConfDir = "/etc/dnsmasq.d"

type Config struct {
	Network        *net.IPNet
	InterfaceIP    net.IP     // subnet gateway; next-hop for the metadata and VPC routes
	VPCRoute       *net.IPNet // if non-nil, routed via InterfaceIP in option 121
	DefaultGateway net.IP     // if non-nil, default route via option 3 and 0.0.0.0/0 in option 121
	Name           string
	ConfDir        string
}
