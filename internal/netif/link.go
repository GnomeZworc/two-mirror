package netif

import (
	"net"

	"github.com/vishvananda/netlink"
)

func LinkIsUp(name string) (bool, error) {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return false, err
	}
	return link.Attrs().Flags&net.FlagUp != 0, nil
}
