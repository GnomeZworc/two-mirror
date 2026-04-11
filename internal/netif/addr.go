package netif

import (
	"net"

	"github.com/vishvananda/netlink"
)

func AddrAdd(iface string, ip net.IP) error {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return err
	}
	addr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   ip,
			Mask: net.CIDRMask(32, 32),
		},
	}
	return netlink.AddrAdd(link, addr)
}
