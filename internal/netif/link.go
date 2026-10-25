package netif

import (
	"net"
	"syscall"

	"github.com/vishvananda/netlink"
)

func LinkIsUp(name string) (bool, error) {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return false, err
	}
	return link.Attrs().Flags&net.FlagUp != 0, nil
}

func LinkMAC(name string) (net.HardwareAddr, error) {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, err
	}
	return link.Attrs().HardwareAddr, nil
}

func LinkIPv4(name string) ([]net.IP, error) {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, err
	}
	addrs, err := netlink.AddrList(link, syscall.AF_INET)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, nil
}

func LinkMTU(name string) (int, error) {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return 0, err
	}
	return link.Attrs().MTU, nil
}

func BridgeVxlanPorts(bridge string) ([]string, error) {
	br, err := netlink.LinkByName(bridge)
	if err != nil {
		return nil, err
	}
	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}
	var ports []string
	for _, l := range links {
		if l.Attrs().MasterIndex == br.Attrs().Index && l.Type() == "vxlan" {
			ports = append(ports, l.Attrs().Name)
		}
	}
	return ports, nil
}
