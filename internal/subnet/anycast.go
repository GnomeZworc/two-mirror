package subnet

import (
	"errors"
	"fmt"

	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/nft"
	"git.g3e.fr/syonad/two/internal/sysctl"
)

func (d subnetData) anycast() bool { return d.mode == ModeVxlan && d.relayIP != nil }

func (d subnetData) gatewayLink() string { return "gw-" + d.subnetID }

func arpSysctls(iface string) [][2]string {
	return [][2]string{
		{"net.ipv4.conf." + iface + ".arp_ignore", "1"},
		{"net.ipv4.conf." + iface + ".arp_announce", "2"},
	}
}

func setupAnycastGateway(d subnetData, vethI string) error {
	mac, err := netif.AnycastMAC(d.vxlanID)
	if err != nil {
		return err
	}
	gw := d.gatewayLink()
	steps := []struct {
		name string
		fn   func() error
	}{
		{"filter " + vethI, func() error {
			return nft.SetPort(AnycastFabricPort(vethI, mac.String(), d.interfaceIP.String(), d.relayIP.String()))
		}},
		{"arp sysctls on " + d.bridge, func() error { return setSysctls(arpSysctls(d.bridge)) }},
		{"relay address on " + d.bridge, func() error { return netif.AddrAdd(d.bridge, d.relayIP) }},
		{"create " + gw, func() error { return netif.CreateMacvlan(gw, d.bridge, mac) }},
		{"local fdb entry " + mac.String(), func() error { return netif.BridgeAddLocalMAC(d.bridge, mac) }},
		{"arp sysctls on " + gw, func() error { return setSysctls(arpSysctls(gw)) }},
		{"gateway address on " + gw, func() error { return netif.AddrAdd(gw, d.interfaceIP) }},
		{"route " + d.cidr.String(), func() error { return netif.RouteAddFrom(d.bridge, d.cidr, d.interfaceIP) }},
		{"set up " + gw, func() error { return netif.LinkSetUp(gw) }},
	}
	return netns.Call(d.vpc, func() error {
		for _, step := range steps {
			if err := step.fn(); err != nil {
				return fmt.Errorf("%s: %w", step.name, err)
			}
		}
		return nil
	})
}

func deleteAnycastGateway(d subnetData, vethI string) error {
	return netns.Call(d.vpc, func() error {
		var errs []error
		if err := nft.DeletePort(vethI); err != nil {
			errs = append(errs, err)
		}
		if err := netif.DeleteLink(d.gatewayLink()); err != nil {
			errs = append(errs, fmt.Errorf("delete %s: %w", d.gatewayLink(), err))
		}
		if err := netif.DeleteLink(d.bridge); err != nil {
			errs = append(errs, fmt.Errorf("delete bridge: %w", err))
		}
		return errors.Join(errs...)
	})
}

func setSysctls(values [][2]string) error {
	for _, kv := range values {
		if err := sysctl.Set(kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}
