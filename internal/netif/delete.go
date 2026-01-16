package netif

import (
	"github.com/vishvananda/netlink"
)

func DeleteLink(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return err
	}
	return netlink.LinkDel(link)
}
