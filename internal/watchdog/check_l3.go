package watchdog

import (
	"fmt"
	"net"
	"sort"
	"strings"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/frr"
	"git.g3e.fr/syonad/two/internal/l3vni"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/internal/watchdog/notify"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

const kindBGP = "bgp"

type l3Probe interface {
	VTEPAddress(iface string) (net.IP, error)
	NeighborEstablished(peer net.IP) (bool, error)
	VNI(vni uint32) (frr.VNIState, error)
	ExportedRoutes(table uint32, iface string) ([]*net.IPNet, error)
	LinkProblem(ns, iface string) string
}

type systemProbe struct {
	frr *frr.Client
}

func (systemProbe) VTEPAddress(iface string) (net.IP, error) { return netif.VTEPAddress(iface) }

func (p systemProbe) NeighborEstablished(peer net.IP) (bool, error) {
	return p.frr.NeighborEstablished(peer)
}

func (p systemProbe) VNI(vni uint32) (frr.VNIState, error) { return p.frr.VNI(vni) }

func (systemProbe) ExportedRoutes(table uint32, iface string) ([]*net.IPNet, error) {
	return netif.TableRoutesVia(int(table), iface)
}

func (systemProbe) LinkProblem(ns, iface string) string {
	if ns == "" {
		return linkProblem(iface)
	}
	var p string
	if err := netns.Call(ns, func() error {
		p = linkProblem(iface)
		return nil
	}); err != nil {
		return fmt.Sprintf("cannot enter netns %s: %v", ns, err)
	}
	if p != "" {
		return p + " (in netns)"
	}
	return ""
}

func CheckBGP(cfg *configuration.Config, p l3Probe, n notify.Notifier) {
	if !cfg.BGP.Enabled() {
		return
	}
	reflectors, err := cfg.BGP.Reflectors()
	if err != nil {
		n.Notify(kindBGP, "config", err.Error())
		return
	}
	for _, rr := range reflectors {
		up, err := p.NeighborEstablished(rr)
		switch {
		case err != nil:
			n.Notify(kindBGP, rr.String(), fmt.Sprintf("session unreadable: %v", err))
		case !up:
			n.Notify(kindBGP, rr.String(), "session to the route reflector is not established")
		}
	}
}

func CheckL3VPCs(db *badger.DB, cfg *configuration.Config, p l3Probe, n notify.Notifier) error {
	pairs, err := kv.ListByPrefix(db, prefixVPC)
	if err != nil {
		return fmt.Errorf("watchdog: listing vpcs: %w", err)
	}
	var rmac string
	for _, name := range resourceNames(pairs, prefixVPC) {
		if st, err := state.Get(db, prefixVPC+name); err != nil || st != state.Running {
			continue
		}
		r, ok, err := l3vni.LoadRouting(db, name)
		if err != nil {
			n.Notify(kindVPC, name, err.Error())
			continue
		}
		if !ok {
			continue
		}
		if !cfg.BGP.Enabled() {
			n.Notify(kindVPC, name, "vpc has an l3vni but the agent configuration has no bgp section")
			continue
		}
		if rmac == "" {
			rmac = expectedRouterMAC(cfg, p, n)
		}
		checkL3VPC(db, name, r, rmac, p, n)
	}
	return nil
}

func expectedRouterMAC(cfg *configuration.Config, p l3Probe, n notify.Notifier) string {
	vtep, err := p.VTEPAddress(cfg.DefaultInterface)
	if err != nil {
		n.Notify(kindBGP, "vtep", fmt.Sprintf("vtep address unreadable on %s: %v", cfg.DefaultInterface, err))
		return "unknown"
	}
	mac, err := l3vni.RouterMAC(vtep)
	if err != nil {
		n.Notify(kindBGP, "vtep", err.Error())
		return "unknown"
	}
	return mac.String()
}

func checkL3VPC(db *badger.DB, name string, r l3vni.Routing, rmac string, p l3Probe, n notify.Notifier) {
	for _, iface := range []string{r.VRF, r.Bridge, r.Vxlan, r.VethHost} {
		if problem := p.LinkProblem("", iface); problem != "" {
			n.Notify(kindVPC, name, problem)
		}
	}
	if problem := p.LinkProblem(name, r.VethNetns); problem != "" {
		n.Notify(kindVPC, name, problem)
	}

	s, err := p.VNI(r.VNI)
	switch {
	case err != nil:
		n.Notify(kindVPC, name, fmt.Sprintf("l3vni %d unreadable in frr: %v", r.VNI, err))
	case s.Type != "L3" || s.State != "Up":
		n.Notify(kindVPC, name, fmt.Sprintf("l3vni %d is type %q state %q in frr, expected L3 Up", r.VNI, s.Type, s.State))
	case rmac != "unknown" && s.RouterMAC != rmac:
		n.Notify(kindVPC, name, fmt.Sprintf("l3vni %d has router mac %q in frr, expected %s", r.VNI, s.RouterMAC, rmac))
	}

	actual, err := p.ExportedRoutes(r.VNI, r.VethHost)
	if err != nil {
		n.Notify(kindVPC, name, fmt.Sprintf("routes of %s unreadable: %v", r.VRF, err))
		return
	}
	expected, tolerated, err := expectedExports(db, name)
	if err != nil {
		n.Notify(kindVPC, name, err.Error())
		return
	}
	exported := make(map[string]bool, len(actual))
	for _, dst := range actual {
		exported[dst.String()] = true
	}
	for _, dst := range sortedKeys(exported) {
		if !expected[dst] && !tolerated[dst] {
			n.Notify(kindVPC, name, fmt.Sprintf("route %s exported in %s without a running owner: its traffic is black-holed", dst, r.VRF))
		}
	}
	for _, dst := range sortedKeys(expected) {
		if !exported[dst] {
			n.Notify(kindVPC, name, fmt.Sprintf("route %s not exported in %s", dst, r.VRF))
		}
	}
}

func expectedExports(db *badger.DB, vpcName string) (expected, tolerated map[string]bool, err error) {
	expected, tolerated = map[string]bool{}, map[string]bool{}
	classify := func(st state.State, dst string) {
		switch st {
		case state.Running:
			expected[dst] = true
		case state.Creating, state.Deleting, state.Error:
			tolerated[dst] = true
		}
	}

	subnets, err := kv.ListByPrefix(db, prefixSubnet)
	if err != nil {
		return nil, nil, fmt.Errorf("watchdog: listing subnets: %w", err)
	}
	routed := map[string]bool{}
	for key, value := range subnets {
		subnet, ok := strings.CutSuffix(strings.TrimPrefix(key, prefixSubnet), "/vpc")
		if !ok || strings.Contains(subnet, "/") || value != vpcName || subnets[prefixSubnet+subnet+"/mode"] != "vxlan" {
			continue
		}
		routed[subnet] = true
		_, cidr, err := net.ParseCIDR(subnets[prefixSubnet+subnet+"/cidr"])
		if err != nil {
			continue
		}
		if st, err := state.Get(db, prefixSubnet+subnet); err == nil {
			classify(st, cidr.String())
		}
	}

	vms, err := kv.ListByPrefix(db, prefixVM)
	if err != nil {
		return nil, nil, fmt.Errorf("watchdog: listing vms: %w", err)
	}
	for key, value := range vms {
		parts := strings.Split(key, "/")
		if len(parts) != 5 || parts[2] != "nic" || parts[4] != "subnet" || !routed[value] {
			continue
		}
		ip := net.ParseIP(vms[strings.TrimSuffix(key, "subnet")+"ip"]).To4()
		if ip == nil {
			continue
		}
		if st, err := state.Get(db, prefixVM+parts[1]); err == nil {
			classify(st, ip.String()+"/32")
		}
	}
	return expected, tolerated, nil
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
