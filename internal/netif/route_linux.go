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

func TableRoutesVia(table int, iface string) ([]*net.IPNet, error) {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return nil, err
	}
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{
		Table:     table,
		LinkIndex: link.Attrs().Index,
	}, netlink.RT_FILTER_TABLE|netlink.RT_FILTER_OIF)
	if err != nil {
		return nil, err
	}
	dsts := make([]*net.IPNet, 0, len(routes))
	for _, r := range routes {
		if r.Dst != nil && r.Gw != nil {
			dsts = append(dsts, r.Dst)
		}
	}
	return dsts, nil
}
