package natgw

import (
	"fmt"
	"net"
	"strings"

	"git.g3e.fr/syonad/two/internal/l3vni"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/nft"

	"github.com/dgraph-io/badger/v4"
)

const publicBridge = "br-public"

type gateway struct {
	vpc     string
	iface   string
	mac     net.HardwareAddr
	ip      *net.IPNet
	gateway net.IP
}

func Iface(vpcName string) (string, error) {
	parts := strings.SplitN(vpcName, "-", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid vpc name %q", vpcName)
	}
	return "ng-" + parts[1], nil
}

func load(db *badger.DB, vpcName string) (gateway, error) {
	r, ok, err := l3vni.LoadRouting(db, vpcName)
	if err != nil {
		return gateway{}, err
	}
	if !ok {
		return gateway{}, fmt.Errorf("vpc %q has no l3vni", vpcName)
	}
	iface, err := Iface(vpcName)
	if err != nil {
		return gateway{}, err
	}
	mac, err := netif.NATGatewayMAC(int(r.VNI))
	if err != nil {
		return gateway{}, err
	}
	return gateway{vpc: vpcName, iface: iface, mac: mac}, nil
}

func Create(db *badger.DB, vpcName string, ip *net.IPNet, gw net.IP) error {
	g, err := load(db, vpcName)
	if err != nil {
		return err
	}
	g.ip, g.gateway = ip, gw
	return create(newSystem(db), g)
}

func Delete(db *badger.DB, vpcName string) error {
	g, err := load(db, vpcName)
	if err != nil {
		return err
	}
	return remove(newSystem(db), g)
}

type step struct {
	what string
	run  func() error
}

func runSteps(g gateway, steps []step) error {
	for _, s := range steps {
		if err := s.run(); err != nil {
			return fmt.Errorf("nat gateway of %s: %s: %w", g.vpc, s.what, err)
		}
	}
	return nil
}

func sysctls(iface string) [][2]string {
	return [][2]string{
		{"net.ipv4.conf." + iface + ".arp_ignore", "1"},
		{"net.ipv4.conf." + iface + ".arp_announce", "2"},
		{"net.ipv4.conf." + iface + ".arp_notify", "1"},
	}
}

func create(h host, g gateway) error {
	ns := g.vpc
	steps := []step{
		{"create " + g.iface, func() error { return h.CreateMacvlan(ns, g.iface, publicBridge, g.mac) }},
		{"local fdb entry " + g.mac.String(), func() error { return h.BridgeSetLocalMAC(ns, publicBridge, g.mac) }},
	}
	for _, kv := range sysctls(g.iface) {
		steps = append(steps, step{kv[0], func() error { return h.Sysctl(ns, kv[0], kv[1]) }})
	}
	steps = append(steps,
		step{"address " + g.ip.String(), func() error { return h.AddrAdd(ns, g.iface, g.ip) }},
		step{"set up " + g.iface, func() error { return h.LinkUp(ns, g.iface) }},
		step{"default route via " + g.gateway.String(), func() error { return h.DefaultVia(ns, g.gateway, g.iface) }},
		step{"nat rules", func() error { return h.SetNAT(ns, nft.NATGateway{Iface: g.iface, IP: g.ip.IP.String()}) }},
		step{"export default", func() error { return h.ExportDefault(g.vpc) }},
	)
	return runSteps(g, steps)
}

func remove(h host, g gateway) error {
	ns := g.vpc
	return runSteps(g, []step{
		{"withdraw default", func() error { return h.WithdrawDefault(g.vpc) }},
		{"nat rules", func() error { return h.DeleteNAT(ns, g.iface) }},
		{"default route towards the vrf", func() error { return h.NetnsDefaultToVRF(g.vpc) }},
		{"local fdb entry " + g.mac.String(), func() error { return h.BridgeDelLocalMAC(ns, publicBridge, g.mac) }},
		{"delete " + g.iface, func() error { return h.DeleteLink(ns, g.iface) }},
	})
}
