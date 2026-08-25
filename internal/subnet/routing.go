package subnet

import (
	"fmt"
	"net"
)

// dhcpRouting resolves what the DHCP server advertises to the guests of a subnet.
//
// The default route always points at the subnet gateway (interface_ip); default_route
// swaps that next-hop for the supplied gateway, or for the deduced one when none was
// supplied. The VPC route keeps interface_ip as its next-hop in every mode but bridge,
// so that traffic to the VPC ranges never leaves through a public gateway.
func dhcpRouting(d subnetData, deduceGateway func() (net.IP, error)) (net.IP, *net.IPNet, error) {
	defaultGateway := d.interfaceIP
	if d.defaultRoute {
		if d.gateway != nil {
			defaultGateway = d.gateway
		} else {
			deduced, err := deduceGateway()
			if err != nil {
				return nil, nil, fmt.Errorf("get default gateway: %w", err)
			}
			defaultGateway = deduced
		}
	}

	var vpcRoute *net.IPNet
	if d.mode != ModeBridge {
		vpcRoute = d.vpcCIDR
	}

	return defaultGateway, vpcRoute, nil
}
