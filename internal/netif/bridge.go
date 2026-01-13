package netif

import (
	"fmt"
	"os"

	"github.com/vishvananda/netlink"
)

func setBridgeSTP(bridge string, enable bool) error {
	path := fmt.Sprintf("/sys/class/net/%s/bridge/stp_state", bridge)

	val := "0"
	if enable {
		val = "1"
	}

	return os.WriteFile(path, []byte(val), 0644)
}

func CreateBridge(name string, mtu int, stp bool) error {
	br := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name: name,
			MTU:  mtu,
		},
	}

	if err := netlink.LinkAdd(br); err != nil {
		return err
	}

	if err := netlink.LinkSetUp(br); err != nil {
		return err
	}

	return setBridgeSTP(name, stp)
}
