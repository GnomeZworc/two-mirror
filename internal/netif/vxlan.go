package netif

import (
	"net"

	"github.com/vishvananda/netlink"
)

func CreateVxlan(name string, vxlanID int, localIP net.IP) error {
	vxlan := &netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{
			Name: name,
		},
		VxlanId:  vxlanID,
		Port:     4789,
		SrcAddr:  localIP,
		Learning: false,
	}
	return netlink.LinkAdd(vxlan)
}
