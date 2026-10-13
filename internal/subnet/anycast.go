package subnet

import (
	"errors"
	"fmt"
	"net"

	"git.g3e.fr/syonad/two/internal/ebtables"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
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
		{"confine " + mac.String(), func() error { return ebtables.ConfineMAC(vethI, mac.String()) }},
		{"arp sysctls on " + d.bridge, func() error { return setSysctls(arpSysctls(d.bridge)) }},
		{"relay address on " + d.bridge, func() error { return netif.AddrAdd(d.bridge, d.relayIP) }},
		{"route " + d.cidr.String(), func() error { return netif.RouteAdd(d.bridge, d.cidr) }},
		{"create " + gw, func() error { return netif.CreateMacvlan(gw, d.bridge, mac) }},
		{"arp sysctls on " + gw, func() error { return setSysctls(arpSysctls(gw)) }},
		{"gateway address on " + gw, func() error { return netif.AddrAdd(gw, d.interfaceIP) }},
		{"set up " + gw, func() error { return netif.LinkSetUp(gw) }},
		{"drop arp to gateway", func() error { return ebtables.DropARPToGateway(vethI, d.interfaceIP.String()) }},
		{"drop arp to relay", func() error { return ebtables.DropARPToGateway(vethI, d.relayIP.String()) }},
		{"drop dhcp from gateway", func() error { return ebtables.DropDHCP(vethI, d.interfaceIP.String()) }},
		{"drop dhcp from relay", func() error { return ebtables.DropDHCP(vethI, d.relayIP.String()) }},
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
	mac, err := netif.AnycastMAC(d.vxlanID)
	if err != nil {
		return err
	}
	return netns.Call(d.vpc, func() error {
		var errs []error
		for _, ip := range []net.IP{d.interfaceIP, d.relayIP} {
			if err := ebtables.DeleteARPToGateway(vethI, ip.String()); err != nil {
				errs = append(errs, fmt.Errorf("delete ebtables arp rule %s: %w", ip, err))
			}
			if err := ebtables.DeleteDHCP(vethI, ip.String()); err != nil {
				errs = append(errs, fmt.Errorf("delete ebtables dhcp rule %s: %w", ip, err))
			}
		}
		if err := ebtables.DeleteConfineMAC(vethI, mac.String()); err != nil {
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
