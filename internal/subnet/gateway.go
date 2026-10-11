package subnet

import (
	"fmt"
	"net"
)

const (
	roleHostGateway     = 0x03
	locallyAdministered = 0x02
)

func hostGatewayMAC(vtep net.IP) (net.HardwareAddr, error) {
	ip := vtep.To4()
	if ip == nil {
		return nil, fmt.Errorf("vtep %v is not an IPv4 address", vtep)
	}
	return net.HardwareAddr{locallyAdministered, roleHostGateway, ip[0], ip[1], ip[2], ip[3]}, nil
}
