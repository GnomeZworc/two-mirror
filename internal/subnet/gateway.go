package subnet

import (
	"fmt"
	"net"
)

const (
	maxVNI              = 1<<24 - 1
	roleAnycastGateway  = 0x01
	locallyAdministered = 0x02
)

func anycastGatewayMAC(vni int) (net.HardwareAddr, error) {
	if vni < 0 || vni > maxVNI {
		return nil, fmt.Errorf("vxlan_id %d does not fit in 24 bits", vni)
	}
	return net.HardwareAddr{locallyAdministered, roleAnycastGateway, 0x00, byte(vni >> 16), byte(vni >> 8), byte(vni)}, nil
}
