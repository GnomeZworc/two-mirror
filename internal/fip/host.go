package fip

import (
	"net"

	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/nft"
	"git.g3e.fr/syonad/two/internal/sysctl"
)

type host interface {
	CreateMacvlan(ns, name, parent string, mac net.HardwareAddr) error
	BridgeAddLocalMAC(ns, bridge string, mac net.HardwareAddr) error
	BridgeDelLocalMAC(ns, bridge string, mac net.HardwareAddr) error
	Sysctl(ns, key, value string) error
	SetFloatingIP(ns string, f nft.FloatingIP) error
	DeleteFloatingIP(ns, iface string) error
	AddrAdd(ns, iface string, prefix *net.IPNet) error
	LinkUp(ns, iface string) error
	TableRouteDev(ns string, table int, dst *net.IPNet, iface string) error
	TableDefaultVia(ns string, table int, gw net.IP, iface string) error
	RuleAdd(ns string, src net.IP, dst *net.IPNet, table, priority int) error
	RuleDel(ns string, src net.IP, dst *net.IPNet, table, priority int) error
	DeleteLink(ns, name string) error
}

var defaultRoute = &net.IPNet{IP: net.IPv4zero.To4(), Mask: net.CIDRMask(0, 32)}

type system struct{}

func (system) CreateMacvlan(ns, name, parent string, mac net.HardwareAddr) error {
	return netns.Call(ns, func() error { return netif.CreateMacvlan(name, parent, mac) })
}

func (system) BridgeAddLocalMAC(ns, bridge string, mac net.HardwareAddr) error {
	return netns.Call(ns, func() error { return netif.BridgeAddLocalMAC(bridge, mac) })
}

func (system) BridgeDelLocalMAC(ns, bridge string, mac net.HardwareAddr) error {
	return netns.Call(ns, func() error { return netif.BridgeDelLocalMACIfExists(bridge, mac) })
}

func (system) Sysctl(ns, key, value string) error {
	return netns.Call(ns, func() error { return sysctl.Set(key, value) })
}

func (system) SetFloatingIP(ns string, f nft.FloatingIP) error {
	return netns.Call(ns, func() error { return nft.SetFloatingIP(f) })
}

func (system) DeleteFloatingIP(ns, iface string) error {
	return netns.Call(ns, func() error { return nft.DeleteFloatingIP(iface) })
}

func (system) AddrAdd(ns, iface string, prefix *net.IPNet) error {
	return netns.Call(ns, func() error { return netif.AddrAddPrefix(iface, prefix) })
}

func (system) LinkUp(ns, iface string) error {
	return netns.Call(ns, func() error { return netif.LinkSetUp(iface) })
}

func (system) TableRouteDev(ns string, table int, dst *net.IPNet, iface string) error {
	return netns.Call(ns, func() error { return netif.RouteReplaceDev(table, dst, iface) })
}

func (system) TableDefaultVia(ns string, table int, gw net.IP, iface string) error {
	return netns.Call(ns, func() error { return netif.RouteReplaceVia(table, defaultRoute, gw, iface) })
}

func (system) RuleAdd(ns string, src net.IP, dst *net.IPNet, table, priority int) error {
	return netns.Call(ns, func() error { return netif.RuleAddFrom(src, dst, table, priority) })
}

func (system) RuleDel(ns string, src net.IP, dst *net.IPNet, table, priority int) error {
	return netns.Call(ns, func() error { return netif.RuleDelFromIfExists(src, dst, table, priority) })
}

func (system) DeleteLink(ns, name string) error {
	return netns.Call(ns, func() error { return netif.DeleteLinkIfExists(name) })
}
