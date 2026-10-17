package netif

import (
	"fmt"
	"net"
)

const (
	locallyAdministered byte = 0x02

	RoleAnycastGateway byte = 0x01
	RoleRouter         byte = 0x02
	RoleHostGateway    byte = 0x03

	MaxVNI = 1<<24 - 1
)

func HostMAC(role byte, vtep net.IP) (net.HardwareAddr, error) {
	ip := vtep.To4()
	if ip == nil {
		return nil, fmt.Errorf("vtep %v is not an IPv4 address", vtep)
	}
	return net.HardwareAddr{locallyAdministered, role, ip[0], ip[1], ip[2], ip[3]}, nil
}

func AnycastMAC(vni int) (net.HardwareAddr, error) {
	if vni < 1 || vni > MaxVNI {
		return nil, fmt.Errorf("vni %d out of range 1-%d", vni, MaxVNI)
	}
	return net.HardwareAddr{locallyAdministered, RoleAnycastGateway, 0x00, byte(vni >> 16), byte(vni >> 8), byte(vni)}, nil
}
