//go:build linux

package netif

import (
	"bytes"
	"net"
	"syscall"

	"github.com/vishvananda/netlink"
)

func bridgeLocalEntry(bridgeIndex int, mac net.HardwareAddr) *netlink.Neigh {
	return &netlink.Neigh{
		LinkIndex:    bridgeIndex,
		Family:       syscall.AF_BRIDGE,
		State:        netlink.NUD_PERMANENT,
		Flags:        netlink.NTF_SELF,
		HardwareAddr: mac,
	}
}

func hasBridgeLocalMAC(entries []netlink.Neigh, bridgeIndex int, mac net.HardwareAddr) bool {
	for _, e := range entries {
		if e.LinkIndex == bridgeIndex && e.MasterIndex == bridgeIndex &&
			e.State&netlink.NUD_PERMANENT != 0 && bytes.Equal(e.HardwareAddr, mac) {
			return true
		}
	}
	return false
}

func BridgeAddLocalMAC(bridge string, mac net.HardwareAddr) error {
	link, err := netlink.LinkByName(bridge)
	if err != nil {
		return err
	}
	return netlink.NeighAdd(bridgeLocalEntry(link.Attrs().Index, mac))
}

func BridgeHasLocalMAC(bridge string, mac net.HardwareAddr) (bool, error) {
	link, err := netlink.LinkByName(bridge)
	if err != nil {
		return false, err
	}
	entries, err := netlink.NeighList(link.Attrs().Index, syscall.AF_BRIDGE)
	if err != nil {
		return false, err
	}
	return hasBridgeLocalMAC(entries, link.Attrs().Index, mac), nil
}
