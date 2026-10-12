package netif

import (
	"github.com/vishvananda/netlink"
)

func CreateVRF(name string, table uint32) error {
	vrf := &netlink.Vrf{
		LinkAttrs: netlink.LinkAttrs{Name: name},
		Table:     table,
	}
	if err := netlink.LinkAdd(vrf); err != nil {
		return err
	}
	return netlink.LinkSetUp(vrf)
}
