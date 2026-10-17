package netif

import (
	"net"

	"github.com/vishvananda/netlink"
)

func macvlanLink(name string, parentIndex int, mac net.HardwareAddr) *netlink.Macvlan {
	return &netlink.Macvlan{
		LinkAttrs: netlink.LinkAttrs{
			Name:         name,
			ParentIndex:  parentIndex,
			HardwareAddr: mac,
		},
		Mode: netlink.MACVLAN_MODE_BRIDGE,
	}
}

func CreateMacvlan(name, parent string, mac net.HardwareAddr) error {
	link, err := netlink.LinkByName(parent)
	if err != nil {
		return err
	}
	return netlink.LinkAdd(macvlanLink(name, link.Attrs().Index, mac))
}
