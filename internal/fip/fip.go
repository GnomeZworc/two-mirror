package fip

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"

	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/nft"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

const (
	publicBridge = "br-public"
	mainTable    = 254
	prefMain     = 100
	prefOwn      = 101
)

type Target struct {
	VPC     string
	CIDR    *net.IPNet
	Private net.IP
}

type floatingIP struct {
	netns   string
	vpcCIDR *net.IPNet
	iface   string
	mac     net.HardwareAddr
	public  *net.IPNet
	private net.IP
	gateway net.IP
	table   int
}

func Iface(public net.IP) (string, error) {
	ip := public.To4()
	if ip == nil {
		return "", fmt.Errorf("public ip %v is not an IPv4 address", public)
	}
	return fmt.Sprintf("fip-%08x", binary.BigEndian.Uint32(ip)), nil
}

func table(private net.IP) (int, error) {
	ip := private.To4()
	if ip == nil {
		return 0, fmt.Errorf("private ip %v is not an IPv4 address", private)
	}
	t := int(binary.BigEndian.Uint32(ip))
	if t < 256 {
		return 0, fmt.Errorf("private ip %v gives a reserved routing table %d", private, t)
	}
	return t, nil
}

func Resolve(db *badger.DB, vmName, ifaceName string) (Target, error) {
	prefix := "vm/" + vmName + "/nic/"
	entries, err := kv.ListByPrefix(db, prefix)
	if err != nil {
		return Target{}, fmt.Errorf("vm %q: listing interfaces: %w", vmName, err)
	}
	for key, value := range entries {
		nic, ok := strings.CutSuffix(strings.TrimPrefix(key, prefix), "/name")
		if !ok || strings.Contains(nic, "/") || value != ifaceName {
			continue
		}
		ip := net.ParseIP(entries[prefix+nic+"/ip"]).To4()
		if ip == nil {
			return Target{}, fmt.Errorf("vm %q: interface %q has no IPv4 address", vmName, ifaceName)
		}
		subnet := entries[prefix+nic+"/subnet"]
		vpc, err := kv.GetFromDB(db, "subnet/"+subnet+"/vpc")
		if err != nil {
			return Target{}, fmt.Errorf("vm %q: interface %q: vpc of subnet %q: %w", vmName, ifaceName, subnet, err)
		}
		raw, err := kv.GetFromDB(db, "vpc/"+vpc+"/cidr")
		if err != nil {
			return Target{}, fmt.Errorf("vm %q: interface %q: cidr of vpc %q: %w", vmName, ifaceName, vpc, err)
		}
		_, cidr, err := net.ParseCIDR(raw)
		if err != nil {
			return Target{}, fmt.Errorf("vm %q: interface %q: invalid cidr %q of vpc %q", vmName, ifaceName, raw, vpc)
		}
		return Target{VPC: vpc, CIDR: cidr, Private: ip}, nil
	}
	return Target{}, fmt.Errorf("vm %q has no interface %q", vmName, ifaceName)
}

func build(t Target, public *net.IPNet, gw net.IP) (floatingIP, error) {
	iface, err := Iface(public.IP)
	if err != nil {
		return floatingIP{}, err
	}
	mac, err := netif.FloatingIPMAC(public.IP)
	if err != nil {
		return floatingIP{}, err
	}
	tbl, err := table(t.Private)
	if err != nil {
		return floatingIP{}, err
	}
	return floatingIP{netns: t.VPC, vpcCIDR: t.CIDR, iface: iface, mac: mac, public: public, private: t.Private, gateway: gw, table: tbl}, nil
}

func Create(t Target, public *net.IPNet, gw net.IP) error {
	f, err := build(t, public, gw)
	if err != nil {
		return err
	}
	return create(system{}, f)
}

func Delete(t Target, public *net.IPNet) error {
	f, err := build(t, public, nil)
	if err != nil {
		return err
	}
	return remove(system{}, f)
}

type step struct {
	what string
	run  func() error
}

func runSteps(f floatingIP, steps []step) error {
	for _, s := range steps {
		if err := s.run(); err != nil {
			return fmt.Errorf("floating ip %s of %s: %s: %w", f.public.IP, f.private, s.what, err)
		}
	}
	return nil
}

func (f floatingIP) host() *net.IPNet {
	return &net.IPNet{IP: f.public.IP.To4(), Mask: net.CIDRMask(32, 32)}
}

func (f floatingIP) segment() *net.IPNet {
	return &net.IPNet{IP: f.public.IP.Mask(f.public.Mask), Mask: f.public.Mask}
}

func sysctls(iface string) [][2]string {
	return [][2]string{
		{"net.ipv4.conf." + iface + ".arp_ignore", "1"},
		{"net.ipv4.conf." + iface + ".arp_announce", "2"},
		{"net.ipv4.conf." + iface + ".arp_notify", "1"},
	}
}

func create(h host, f floatingIP) error {
	ns := f.netns
	rules := nft.FloatingIP{Iface: f.iface, Public: f.public.IP.String(), Private: f.private.String()}
	steps := []step{
		{"create " + f.iface, func() error { return h.CreateMacvlan(ns, f.iface, publicBridge, f.mac) }},
		{"local fdb entry " + f.mac.String(), func() error { return h.BridgeSetLocalMAC(ns, publicBridge, f.mac) }},
	}
	for _, sc := range sysctls(f.iface) {
		steps = append(steps, step{sc[0], func() error { return h.Sysctl(ns, sc[0], sc[1]) }})
	}
	steps = append(steps,
		step{"translation rules", func() error { return h.SetFloatingIP(ns, rules) }},
		step{"address " + f.host().String(), func() error { return h.AddrAdd(ns, f.iface, f.host()) }},
		step{"set up " + f.iface, func() error { return h.LinkUp(ns, f.iface) }},
		step{fmt.Sprintf("route %s of table %d", f.segment(), f.table), func() error {
			return h.TableRouteDev(ns, f.table, f.segment(), f.iface)
		}},
		step{fmt.Sprintf("default route of table %d via %s", f.table, f.gateway), func() error {
			return h.TableDefaultVia(ns, f.table, f.gateway, f.iface)
		}},
		step{"rule to the main table for " + f.vpcCIDR.String(), func() error { return h.RuleAdd(ns, f.private, f.vpcCIDR, mainTable, prefMain) }},
		step{fmt.Sprintf("rule to table %d", f.table), func() error { return h.RuleAdd(ns, f.private, nil, f.table, prefOwn) }},
	)
	return runSteps(f, steps)
}

func remove(h host, f floatingIP) error {
	ns := f.netns
	return runSteps(f, []step{
		{fmt.Sprintf("rule to table %d", f.table), func() error { return h.RuleDel(ns, f.private, nil, f.table, prefOwn) }},
		{"rule to the main table for " + f.vpcCIDR.String(), func() error { return h.RuleDel(ns, f.private, f.vpcCIDR, mainTable, prefMain) }},
		{"translation rules", func() error { return h.DeleteFloatingIP(ns, f.iface) }},
		{"local fdb entry " + f.mac.String(), func() error { return h.BridgeDelLocalMAC(ns, publicBridge, f.mac) }},
		{"delete " + f.iface, func() error { return h.DeleteLink(ns, f.iface) }},
	})
}
