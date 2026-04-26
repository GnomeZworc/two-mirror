//go:build linux

package netif

import (
	"fmt"

	"git.g3e.fr/syonad/two/internal/netns"
	"github.com/vishvananda/netlink"
)

func CreateTap(tapID int, bridgeName, vpcName string) error {
	name := fmt.Sprintf("tap%d", tapID)

	return netns.Call(vpcName, func() error {
		tap := &netlink.Tuntap{
			LinkAttrs: netlink.LinkAttrs{Name: name},
			Mode:      netlink.TUNTAP_MODE_TAP,
		}
		if err := netlink.LinkAdd(tap); err != nil {
			return err
		}
		if err := BridgeSetMaster(name, bridgeName); err != nil {
			return err
		}
		return LinkSetUp(name)
	})
}
