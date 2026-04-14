//go:build linux

package netif

import (
	"net"

	"github.com/vishvananda/netlink"
)

func RouteAdd(iface string, subnet *net.IPNet) error {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return err
	}
	return netlink.RouteAdd(&netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       subnet,
		Scope:     netlink.SCOPE_LINK,
	})
}
