package netif

import (
	"fmt"
	"net"
	"syscall"

	"github.com/vishvananda/netlink"
)

const (
	scopeUniverse    = 0
	addressSecondary = 0x01
)

func CreateVxlan(name string, vxlanID int, localIface string, mtu int) error {
	link, err := netlink.LinkByName(localIface)
	if err != nil {
		return err
	}
	local, err := linkVTEPAddress(link)
	if err != nil {
		return err
	}
	vxlan := &netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{
			Name: name,
			MTU:  mtu,
		},
		VxlanId:      vxlanID,
		Port:         4789,
		VtepDevIndex: link.Attrs().Index,
		SrcAddr:      local,
		Learning:     false,
	}
	return netlink.LinkAdd(vxlan)
}

func VTEPAddress(iface string) (net.IP, error) {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return nil, err
	}
	return linkVTEPAddress(link)
}

func linkVTEPAddress(link netlink.Link) (net.IP, error) {
	addrs, err := netlink.AddrList(link, syscall.AF_INET)
	if err != nil {
		return nil, err
	}
	return vtepAddress(link.Attrs().Name, addrs)
}

func vtepAddress(iface string, addrs []netlink.Addr) (net.IP, error) {
	for _, a := range addrs {
		if a.IPNet == nil || a.IP.To4() == nil || a.Scope != scopeUniverse || a.Flags&addressSecondary != 0 {
			continue
		}
		return a.IP.To4(), nil
	}
	return nil, fmt.Errorf("no primary global IPv4 address on %s for the VXLAN local endpoint", iface)
}
