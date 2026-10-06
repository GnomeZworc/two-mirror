package netif

import (
	"net"

	"github.com/vishvananda/netlink"
)

func RouteReplaceVia(table int, dst *net.IPNet, gw net.IP, iface string) error {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return err
	}
	return netlink.RouteReplace(&netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       dst,
		Gw:        gw,
		Table:     table,
	})
}

func RouteDel(table int, dst *net.IPNet) error {
	return netlink.RouteDel(&netlink.Route{
		Dst:   dst,
		Table: table,
	})
}
