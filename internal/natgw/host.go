package natgw

import (
	"net"

	"git.g3e.fr/syonad/two/internal/l3vni"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/nft"
	"git.g3e.fr/syonad/two/internal/sysctl"

	"github.com/dgraph-io/badger/v4"
)

type host interface {
	CreateMacvlan(ns, name, parent string, mac net.HardwareAddr) error
	BridgeAddLocalMAC(ns, bridge string, mac net.HardwareAddr) error
	BridgeDelLocalMAC(ns, bridge string, mac net.HardwareAddr) error
	Sysctl(ns, key, value string) error
	AddrAdd(ns, iface string, prefix *net.IPNet) error
	LinkUp(ns, iface string) error
	DefaultVia(ns string, gw net.IP, iface string) error
	SetNAT(ns string, g nft.NATGateway) error
	DeleteNAT(ns, iface string) error
	DeleteLink(ns, name string) error
	ExportDefault(vpc string) error
	WithdrawDefault(vpc string) error
	NetnsDefaultToVRF(vpc string) error
}

var defaultRoute = &net.IPNet{IP: net.IPv4zero.To4(), Mask: net.CIDRMask(0, 32)}

type system struct {
	db *badger.DB
}

func newSystem(db *badger.DB) system {
	return system{db: db}
}

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

func (system) AddrAdd(ns, iface string, prefix *net.IPNet) error {
	return netns.Call(ns, func() error { return netif.AddrAddPrefix(iface, prefix) })
}

func (system) LinkUp(ns, iface string) error {
	return netns.Call(ns, func() error { return netif.LinkSetUp(iface) })
}

func (system) DefaultVia(ns string, gw net.IP, iface string) error {
	return netns.Call(ns, func() error { return netif.RouteReplaceVia(0, defaultRoute, gw, iface) })
}

func (system) SetNAT(ns string, g nft.NATGateway) error {
	return netns.Call(ns, func() error { return nft.SetNATGateway(g) })
}

func (system) DeleteNAT(ns, iface string) error {
	return netns.Call(ns, func() error { return nft.DeleteNATGateway(iface) })
}

func (system) DeleteLink(ns, name string) error {
	return netns.Call(ns, func() error { return netif.DeleteLinkIfExists(name) })
}

func (s system) ExportDefault(vpc string) error { return l3vni.ExportDefault(s.db, vpc) }

func (s system) WithdrawDefault(vpc string) error { return l3vni.WithdrawDefault(s.db, vpc) }

func (s system) NetnsDefaultToVRF(vpc string) error { return l3vni.NetnsDefaultToVRF(s.db, vpc) }
