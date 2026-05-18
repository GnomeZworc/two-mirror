//go:build linux

package netif

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

func GetDefaultGateway() (net.IP, error) {
	routes, err := netlink.RouteList(nil, netlink.FAMILY_V4)
	if err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	for _, r := range routes {
		if r.Dst == nil && r.Gw != nil {
			return r.Gw, nil
		}
	}
	return nil, fmt.Errorf("no default gateway found")
}
