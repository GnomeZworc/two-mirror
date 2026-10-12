package netif

import (
	"net"

	"github.com/vishvananda/netlink"
)

func CreateBridge(name string, mtu int, mac net.HardwareAddr) error {
	br := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name:         name,
			MTU:          mtu,
			HardwareAddr: mac,
		},
	}

	if err := netlink.LinkAdd(br); err != nil {
		return err
	}

	if err := netlink.LinkSetUp(br); err != nil {
		return err
	}

	return nil
}

func BridgeSetMaster(iface, bridge string) error {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return err
	}

	br, err := netlink.LinkByName(bridge)
	if err != nil {
		return err
	}

	return netlink.LinkSetMaster(link, br)
}
