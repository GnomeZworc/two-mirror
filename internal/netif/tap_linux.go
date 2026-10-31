//go:build linux

package netif

import (
	"bytes"
	"fmt"
	"net"
	"syscall"

	"git.g3e.fr/syonad/two/internal/netns"
	"github.com/vishvananda/netlink"
)

func tapName(tapID int) string {
	return fmt.Sprintf("tap%d", tapID)
}

func tapMACEntry(tapIndex int, mac net.HardwareAddr) *netlink.Neigh {
	return &netlink.Neigh{
		LinkIndex:    tapIndex,
		Family:       syscall.AF_BRIDGE,
		State:        netlink.NUD_NOARP,
		Flags:        netlink.NTF_MASTER | netlink.NTF_STICKY,
		HardwareAddr: mac,
	}
}

func hasTapMAC(entries []netlink.Neigh, tapIndex int, mac net.HardwareAddr) bool {
	for _, e := range entries {
		if e.LinkIndex == tapIndex && e.MasterIndex != 0 && e.Vlan == 0 &&
			e.State&netlink.NUD_NOARP != 0 && e.Flags&netlink.NTF_STICKY != 0 &&
			bytes.Equal(e.HardwareAddr, mac) {
			return true
		}
	}
	return false
}

func CreateTap(tapID int, bridgeName, vpcName string, mtu int, mac net.HardwareAddr) error {
	name := tapName(tapID)

	return netns.Call(vpcName, func() error {
		tap := &netlink.Tuntap{
			LinkAttrs: netlink.LinkAttrs{Name: name, MTU: mtu},
			Mode:      netlink.TUNTAP_MODE_TAP,
		}
		if err := netlink.LinkAdd(tap); err != nil {
			return err
		}
		if err := BridgeSetMaster(name, bridgeName); err != nil {
			return err
		}
		link, err := netlink.LinkByName(name)
		if err != nil {
			return err
		}
		if err := netlink.NeighSet(tapMACEntry(link.Attrs().Index, mac)); err != nil {
			return fmt.Errorf("pin %s on %s: %w", mac, name, err)
		}
		return LinkSetUp(name)
	})
}

func TapHasMAC(tapID int, mac net.HardwareAddr) (bool, error) {
	link, err := netlink.LinkByName(tapName(tapID))
	if err != nil {
		return false, err
	}
	entries, err := netlink.NeighList(link.Attrs().Index, syscall.AF_BRIDGE)
	if err != nil {
		return false, err
	}
	return hasTapMAC(entries, link.Attrs().Index, mac), nil
}

func DeleteTap(tapID int, vpcName string) error {
	return netns.Call(vpcName, func() error {
		return DeleteLink(tapName(tapID))
	})
}
