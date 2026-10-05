package netif

import (
	"net"

	"github.com/vishvananda/netlink"
)

func LinkSetUp(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return err
	}
	return netlink.LinkSetUp(link)
}

func LinkSetDown(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return err
	}
	return netlink.LinkSetDown(link)
}

func LinkSetHardwareAddr(name string, mac net.HardwareAddr) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return err
	}
	return netlink.LinkSetHardwareAddr(link, mac)
}
