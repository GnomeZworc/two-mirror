package netif

import (
	"github.com/vishvananda/netlink"
)

func CreateVxlan(name string, vxlanID int, localIface string) error {
	link, err := netlink.LinkByName(localIface)
	if err != nil {
		return err
	}
	vxlan := &netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{
			Name: name,
		},
		VxlanId:      vxlanID,
		Port:         4789,
		VtepDevIndex: link.Attrs().Index,
		Learning:     false,
	}
	return netlink.LinkAdd(vxlan)
}
