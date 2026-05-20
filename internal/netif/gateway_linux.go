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
		if r.Gw == nil {
			continue
		}
		if r.Dst == nil {
			return r.Gw, nil
		}
		if ones, _ := r.Dst.Mask.Size(); ones == 0 {
			return r.Gw, nil
		}
	}
	return nil, fmt.Errorf("no default gateway found")
}
