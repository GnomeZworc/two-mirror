package frr

import (
	"fmt"
	"net"
	"strings"
)

const maxVNI = 1<<24 - 1

type Base struct {
	ASN             uint32
	RouterID        net.IP
	RouteReflectors []net.IP
}

type VRF struct {
	ASN      uint32
	ID       string
	L3VNI    uint32
	VTEP     net.IP
	Supernet *net.IPNet
}

func (b Base) validate() error {
	if b.ASN == 0 {
		return fmt.Errorf("frr: asn must not be 0")
	}
	if b.RouterID.To4() == nil {
		return fmt.Errorf("frr: router-id %v is not an IPv4 address", b.RouterID)
	}
	if len(b.RouteReflectors) == 0 {
		return fmt.Errorf("frr: at least one route reflector is required")
	}
	for _, rr := range b.RouteReflectors {
		if rr.To4() == nil {
			return fmt.Errorf("frr: route reflector %v is not an IPv4 address", rr)
		}
	}
	return nil
}

func (b Base) commands() []string {
	cmds := []string{
		fmt.Sprintf("router bgp %d", b.ASN),
		fmt.Sprintf("bgp router-id %s", b.RouterID),
		"no bgp default ipv4-unicast",
		"neighbor fabric peer-group",
		fmt.Sprintf("neighbor fabric remote-as %d", b.ASN),
		"neighbor fabric capability extended-nexthop",
	}
	for _, rr := range b.RouteReflectors {
		cmds = append(cmds, fmt.Sprintf("neighbor %s peer-group fabric", rr))
	}
	return append(cmds,
		"address-family l2vpn evpn",
		"neighbor fabric activate",
		"advertise-all-vni",
		"exit-address-family",
		"exit",
	)
}

func (b Base) staleReflectors(running string) ([]string, error) {
	want := make(map[string]bool, len(b.RouteReflectors))
	for _, rr := range b.RouteReflectors {
		want[rr.String()] = true
	}
	var stale []string
	in := false
	for _, l := range strings.Split(running, "\n") {
		l = strings.TrimRight(l, " \r")
		f := strings.Fields(l)
		switch {
		case len(f) == 3 && f[0] == "router" && f[1] == "bgp":
			if f[2] != fmt.Sprint(b.ASN) {
				return nil, fmt.Errorf("frr: bgp already runs as AS %s, not %d: changing the asn requires removing that instance by hand", f[2], b.ASN)
			}
			in = true
		case in && l == "exit":
			in = false
		case in && len(f) == 4 && f[0] == "neighbor" && f[2] == "peer-group" && f[3] == "fabric" && !want[f[1]]:
			stale = append(stale, f[1])
		}
	}
	return stale, nil
}

func (v VRF) Name() string {
	return "vrf-" + v.ID
}

func (v VRF) prefixList() string {
	return "PL-" + v.ID
}

func (v VRF) routeMap() string {
	return "EXPORT-" + v.ID
}

func (v VRF) bgpInstance() string {
	return fmt.Sprintf("router bgp %d vrf %s", v.ASN, v.Name())
}

func (v VRF) validateName() error {
	if v.ASN == 0 {
		return fmt.Errorf("frr: asn must not be 0")
	}
	if v.ID == "" {
		return fmt.Errorf("frr: vrf id must not be empty")
	}
	return nil
}

func (v VRF) validate() error {
	if err := v.validateName(); err != nil {
		return err
	}
	if v.L3VNI == 0 || v.L3VNI > maxVNI {
		return fmt.Errorf("frr: l3vni %d does not fit in 24 bits", v.L3VNI)
	}
	if v.VTEP.To4() == nil {
		return fmt.Errorf("frr: vtep %v is not an IPv4 address", v.VTEP)
	}
	if v.Supernet == nil || v.Supernet.IP.To4() == nil {
		return fmt.Errorf("frr: supernet %v is not an IPv4 prefix", v.Supernet)
	}
	return nil
}

func (v VRF) commands() []string {
	return []string{
		fmt.Sprintf("ip prefix-list %s seq 10 permit %s le 32", v.prefixList(), v.Supernet),
		fmt.Sprintf("route-map %s permit 10", v.routeMap()),
		fmt.Sprintf("match ip address prefix-list %s", v.prefixList()),
		"exit",
		fmt.Sprintf("route-map %s deny 20", v.routeMap()),
		"exit",
		fmt.Sprintf("vrf %s", v.Name()),
		fmt.Sprintf("vni %d", v.L3VNI),
		"exit-vrf",
		v.bgpInstance(),
		fmt.Sprintf("bgp router-id %s", v.VTEP),
		"no bgp default ipv4-unicast",
		"address-family ipv4 unicast",
		fmt.Sprintf("redistribute kernel route-map %s", v.routeMap()),
		"exit-address-family",
		"address-family l2vpn evpn",
		fmt.Sprintf("rd %s:%d", v.VTEP, v.L3VNI),
		fmt.Sprintf("route-target both %d:%d", v.ASN, v.L3VNI),
		"advertise ipv4 unicast",
		"exit-address-family",
		"exit",
	}
}
