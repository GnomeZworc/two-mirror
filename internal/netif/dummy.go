package netif

import (
	"github.com/vishvananda/netlink"
)

func CreateDummy(name string) error {
	dummy := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: name}}
	if err := netlink.LinkAdd(dummy); err != nil {
		return err
	}
	return netlink.LinkSetUp(dummy)
}
