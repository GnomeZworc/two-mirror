package l3vni

import (
	"fmt"
	"net"
	"strconv"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/frr"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

const l3MTU = 1500

var (
	transitHost  = &net.IPNet{IP: net.IPv4(169, 254, 0, 1).To4(), Mask: net.CIDRMask(30, 32)}
	transitNetns = &net.IPNet{IP: net.IPv4(169, 254, 0, 2).To4(), Mask: net.CIDRMask(30, 32)}
)

type l3VPC struct {
	netns    string
	id       string
	vni      uint32
	supernet *net.IPNet
}

func loadL3(db *badger.DB, name, id string) (l3VPC, bool, error) {
	raw, err := kv.GetFromDB(db, "vpc/"+name+"/l3vni")
	if err != nil {
		return l3VPC{}, false, nil
	}
	vni, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return l3VPC{}, false, fmt.Errorf("vpc %q: invalid l3vni %q", name, raw)
	}
	if err := Valid(uint32(vni)); err != nil {
		return l3VPC{}, false, fmt.Errorf("vpc %q: %w", name, err)
	}
	cidr, err := kv.GetFromDB(db, "vpc/"+name+"/cidr")
	if err != nil {
		return l3VPC{}, false, fmt.Errorf("vpc %q: no cidr: %w", name, err)
	}
	_, supernet, err := net.ParseCIDR(cidr)
	if err != nil {
		return l3VPC{}, false, fmt.Errorf("vpc %q: invalid cidr %q: %w", name, cidr, err)
	}
	return l3VPC{netns: name, id: id, vni: uint32(vni), supernet: supernet}, true, nil
}

func (v l3VPC) vrf() string       { return "vrf-" + v.id }
func (v l3VPC) bridge() string    { return "brl3-" + v.id }
func (v l3VPC) vxlan() string     { return "vxl3-" + v.id }
func (v l3VPC) vethHost() string  { return "l3-" + v.id + "-o" }
func (v l3VPC) vethNetns() string { return "l3-" + v.id + "-i" }

func bgpASN(cfg *configuration.Config) (uint32, error) {
	if cfg == nil || !cfg.BGP.Enabled() {
		return 0, fmt.Errorf("the vpc has an l3vni but the agent configuration has no bgp section")
	}
	return cfg.BGP.ASN, nil
}

func createL3(h host, cfg *configuration.Config, v l3VPC) error {
	asn, err := bgpASN(cfg)
	if err != nil {
		return err
	}
	vtep, err := h.VTEPAddress(cfg.DefaultInterface)
	if err != nil {
		return fmt.Errorf("vtep: %w", err)
	}
	rmac, err := RouterMAC(vtep)
	if err != nil {
		return err
	}
	steps := []struct {
		what string
		run  func() error
	}{
		{"create " + v.vrf(), func() error { return h.CreateVRF(v.vrf(), v.vni) }},
		{"create " + v.bridge(), func() error { return h.CreateBridge(v.bridge(), l3MTU) }},
		{"set router mac on " + v.bridge(), func() error { return h.SetHardwareAddr(v.bridge(), rmac) }},
		{"attach " + v.bridge(), func() error { return h.SetMaster(v.bridge(), v.vrf()) }},
		{"create " + v.vxlan(), func() error { return h.CreateVxlan(v.vxlan(), int(v.vni), cfg.DefaultInterface, l3MTU) }},
		{"attach " + v.vxlan(), func() error { return h.SetMaster(v.vxlan(), v.bridge()) }},
		{"set up " + v.vxlan(), func() error { return h.LinkUp("", v.vxlan()) }},
		{"create " + v.vethHost(), func() error { return h.CreateVeth(v.vethHost(), v.vethNetns(), v.netns, l3MTU) }},
		{"attach " + v.vethHost(), func() error { return h.SetMaster(v.vethHost(), v.vrf()) }},
		{"address " + v.vethHost(), func() error { return h.AddrAdd("", v.vethHost(), transitHost) }},
		{"set up " + v.vethHost(), func() error { return h.LinkUp("", v.vethHost()) }},
		{"forwarding on " + v.bridge(), func() error { return h.Sysctl("", "net.ipv4.conf."+v.bridge()+".forwarding", "1") }},
		{"forwarding on " + v.vethHost(), func() error { return h.Sysctl("", "net.ipv4.conf."+v.vethHost()+".forwarding", "1") }},
		{"address " + v.vethNetns(), func() error { return h.AddrAdd(v.netns, v.vethNetns(), transitNetns) }},
		{"set up " + v.vethNetns(), func() error { return h.LinkUp(v.netns, v.vethNetns()) }},
		{"forwarding in " + v.netns, func() error { return h.Sysctl(v.netns, "net.ipv4.ip_forward", "1") }},
		{"route " + v.supernet.String() + " in " + v.netns, func() error {
			return h.RouteVia(v.netns, v.supernet, transitHost.IP, v.vethNetns())
		}},
		{"frr " + v.vrf(), func() error {
			return h.ApplyVRF(frr.VRF{ASN: asn, ID: v.id, L3VNI: v.vni, VTEP: vtep, Supernet: v.supernet})
		}},
	}
	for _, s := range steps {
		if err := s.run(); err != nil {
			return fmt.Errorf("l3vni %d: %s: %w", v.vni, s.what, err)
		}
	}
	return nil
}

func deleteL3(h host, cfg *configuration.Config, v l3VPC) error {
	asn, err := bgpASN(cfg)
	if err != nil {
		return err
	}
	frrVRF := frr.VRF{ASN: asn, ID: v.id}
	if err := h.WithdrawVRF(frrVRF); err != nil {
		return fmt.Errorf("l3vni %d: withdraw frr: %w", v.vni, err)
	}
	for _, link := range []string{v.vethHost(), v.vxlan(), v.bridge(), v.vrf()} {
		if err := h.DeleteLink(link); err != nil {
			return fmt.Errorf("l3vni %d: delete %s: %w", v.vni, link, err)
		}
	}
	if err := h.RemoveVRF(frrVRF); err != nil {
		return fmt.Errorf("l3vni %d: remove frr vrf: %w", v.vni, err)
	}
	return nil
}

func Create(db *badger.DB, vpcName string, cfg *configuration.Config) error {
	v, ok, err := loadRouted(db, vpcName)
	if err != nil || !ok {
		return err
	}
	return createL3(newSystem(), cfg, v)
}

func Delete(db *badger.DB, vpcName string, cfg *configuration.Config) error {
	v, ok, err := loadRouted(db, vpcName)
	if err != nil || !ok {
		return err
	}
	return deleteL3(newSystem(), cfg, v)
}

type Routing struct {
	VNI       uint32
	Supernet  *net.IPNet
	VRF       string
	Bridge    string
	Vxlan     string
	VethHost  string
	VethNetns string
}

func LoadRouting(db *badger.DB, vpcName string) (Routing, bool, error) {
	v, ok, err := loadRouted(db, vpcName)
	if err != nil || !ok {
		return Routing{}, ok, err
	}
	return Routing{
		VNI:       v.vni,
		Supernet:  v.supernet,
		VRF:       v.vrf(),
		Bridge:    v.bridge(),
		Vxlan:     v.vxlan(),
		VethHost:  v.vethHost(),
		VethNetns: v.vethNetns(),
	}, true, nil
}

func RouterMAC(vtep net.IP) (net.HardwareAddr, error) {
	return netif.HostMAC(netif.RoleRouter, vtep)
}
