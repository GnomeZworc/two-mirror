package netif

import (
	"fmt"
	"net"
)

const (
	locallyAdministered byte = 0x02

	RoleRouter      byte = 0x02
	RoleHostGateway byte = 0x03
)

func HostMAC(role byte, vtep net.IP) (net.HardwareAddr, error) {
	ip := vtep.To4()
	if ip == nil {
		return nil, fmt.Errorf("vtep %v is not an IPv4 address", vtep)
	}
	return net.HardwareAddr{locallyAdministered, role, ip[0], ip[1], ip[2], ip[3]}, nil
}
