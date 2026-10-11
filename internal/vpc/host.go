package vpc

import (
	"net"

	"git.g3e.fr/syonad/two/internal/frr"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/sysctl"
)

type host interface {
	VTEPAddress(iface string) (net.IP, error)
	Sysctl(ns, key, value string) error
	CreateVRF(name string, table uint32) error
	CreateBridge(name string, mtu int) error
	SetHardwareAddr(name string, mac net.HardwareAddr) error
	SetMaster(iface, master string) error
	CreateVxlan(name string, vni int, iface string, mtu int) error
	CreateVeth(hostIf, nsIf, ns string, mtu int) error
	AddrAdd(ns, iface string, prefix *net.IPNet) error
	LinkUp(ns, iface string) error
	RouteVia(ns string, dst *net.IPNet, gw net.IP, iface string) error
	DeleteLink(name string) error
	TableRouteVia(table uint32, dst *net.IPNet, gw net.IP, iface string) error
	TableRouteDel(table uint32, dst *net.IPNet) error
	ApplyVRF(v frr.VRF) error
	WithdrawVRF(v frr.VRF) error
	RemoveVRF(v frr.VRF) error
}

type system struct {
	frr *frr.Client
}

func newSystem() system {
	return system{frr: frr.NewClient()}
}

func in(ns string, fn func() error) error {
	if ns == "" {
		return fn()
	}
	return netns.Call(ns, fn)
}

func (system) VTEPAddress(iface string) (net.IP, error) { return netif.VTEPAddress(iface) }

func (system) Sysctl(ns, key, value string) error {
	return in(ns, func() error { return sysctl.Set(key, value) })
}

func (system) CreateVRF(name string, table uint32) error { return netif.CreateVRF(name, table) }

func (system) CreateBridge(name string, mtu int) error { return netif.CreateBridge(name, mtu) }

func (system) SetHardwareAddr(name string, mac net.HardwareAddr) error {
	return netif.LinkSetHardwareAddr(name, mac)
}

func (system) SetMaster(iface, master string) error { return netif.BridgeSetMaster(iface, master) }

func (system) CreateVxlan(name string, vni int, iface string, mtu int) error {
	return netif.CreateVxlan(name, vni, iface, mtu)
}

func (system) CreateVeth(hostIf, nsIf, ns string, mtu int) error {
	return netif.CreateVethToNetns(hostIf, nsIf, "/var/run/netns/"+ns, mtu)
}

func (system) AddrAdd(ns, iface string, prefix *net.IPNet) error {
	return in(ns, func() error { return netif.AddrAddPrefix(iface, prefix) })
}

func (system) LinkUp(ns, iface string) error {
	return in(ns, func() error { return netif.LinkSetUp(iface) })
}

func (system) RouteVia(ns string, dst *net.IPNet, gw net.IP, iface string) error {
	return in(ns, func() error { return netif.RouteReplaceVia(0, dst, gw, iface) })
}

func (system) DeleteLink(name string) error { return netif.DeleteLinkIfExists(name) }

func (system) TableRouteVia(table uint32, dst *net.IPNet, gw net.IP, iface string) error {
	return netif.RouteReplaceVia(int(table), dst, gw, iface)
}

func (system) TableRouteDel(table uint32, dst *net.IPNet) error {
	return netif.RouteDelIfExists(int(table), dst)
}

func (s system) ApplyVRF(v frr.VRF) error { return s.frr.ApplyVRF(v) }

func (s system) WithdrawVRF(v frr.VRF) error { return s.frr.WithdrawVRF(v) }

func (s system) RemoveVRF(v frr.VRF) error { return s.frr.RemoveVRF(v) }
