//go:build linux

package netif

import (
	"errors"
	"net"
	"syscall"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

var defaultIPv4 = &net.IPNet{IP: net.IPv4zero.To4(), Mask: net.CIDRMask(0, 32)}

func RouteAdd(iface string, subnet *net.IPNet) error {
	return RouteAddFrom(iface, subnet, nil)
}

func RouteAddFrom(iface string, subnet *net.IPNet, src net.IP) error {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return err
	}
	return netlink.RouteAdd(&netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       subnet,
		Src:       src,
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

func unreachableDefault(table, metric int) *netlink.Route {
	return &netlink.Route{
		Dst:      defaultIPv4,
		Table:    table,
		Type:     unix.RTN_UNREACHABLE,
		Priority: metric,
	}
}

func RouteReplaceUnreachableDefault(table, metric int) error {
	return netlink.RouteReplace(unreachableDefault(table, metric))
}

func RouteDelUnreachableDefaultIfExists(table, metric int) error {
	if err := netlink.RouteDel(unreachableDefault(table, metric)); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func HasUnreachableDefault(table, metric int) (bool, error) {
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{
		Table: table,
		Type:  unix.RTN_UNREACHABLE,
	}, netlink.RT_FILTER_TABLE|netlink.RT_FILTER_TYPE)
	if err != nil {
		return false, err
	}
	for _, r := range routes {
		if r.Priority == metric && (r.Dst == nil || isDefault(r.Dst)) {
			return true, nil
		}
	}
	return false, nil
}

func isDefault(dst *net.IPNet) bool {
	ones, _ := dst.Mask.Size()
	return ones == 0
}
