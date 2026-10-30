package watchdog

import (
	"errors"
	"net"
	"reflect"
	"testing"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/frr"
	"git.g3e.fr/syonad/two/internal/state"
)

type fakeProbe struct {
	established map[string]bool
	neighborErr error
	vni         map[uint32]frr.VNIState
	routes      map[uint32][]string
	routesErr   error
	links       map[string]string
	vtepErr     error
	unreachable map[uint32]bool
	unreachErr  error
}

func (f *fakeProbe) VTEPAddress(string) (net.IP, error) {
	if f.vtepErr != nil {
		return nil, f.vtepErr
	}
	return net.ParseIP("192.168.14.11"), nil
}

func (f *fakeProbe) NeighborEstablished(peer net.IP) (bool, error) {
	return f.established[peer.String()], f.neighborErr
}

func (f *fakeProbe) VNI(vni uint32) (frr.VNIState, error) {
	s, ok := f.vni[vni]
	if !ok {
		return s, errors.New("% VNI does not exist")
	}
	return s, nil
}

func (f *fakeProbe) ExportedRoutes(table uint32, _ string) ([]*net.IPNet, error) {
	var out []*net.IPNet
	for _, r := range f.routes[table] {
		_, n, _ := net.ParseCIDR(r)
		out = append(out, n)
	}
	return out, f.routesErr
}

func (f *fakeProbe) UnreachableDefault(table uint32) (bool, error) {
	if f.unreachable == nil {
		return true, f.unreachErr
	}
	return f.unreachable[table], f.unreachErr
}

func (f *fakeProbe) LinkProblem(ns, iface string) string {
	return f.links[ns+"/"+iface]
}

func bgpCfg() *configuration.Config {
	cfg := &configuration.Config{DefaultInterface: "br-000000"}
	cfg.BGP.ASN = 64600
	cfg.BGP.RouteReflectors = []string{"10.255.255.1", "10.255.255.2"}
	return cfg
}

func notified(r *recorder) []string {
	var out []string
	for _, c := range r.calls {
		out = append(out, c.kind+" "+c.name+": "+c.problem)
	}
	return out
}

func TestCheckBGPReportsEachReflectorWithoutSession(t *testing.T) {
	r := &recorder{}
	CheckBGP(bgpCfg(), &fakeProbe{established: map[string]bool{"10.255.255.1": true}}, r)
	want := []string{"bgp 10.255.255.2: session to the route reflector is not established"}
	if got := notified(r); !reflect.DeepEqual(got, want) {
		t.Errorf("notifications %q, attendu %q", got, want)
	}
}

func TestCheckBGPReportsAnUnreadableSession(t *testing.T) {
	r := &recorder{}
	CheckBGP(bgpCfg(), &fakeProbe{neighborErr: errors.New("vtysh absent")}, r)
	if len(r.calls) != 2 || !r.hasProblemContaining("session unreadable: vtysh absent") {
		t.Errorf("notifications %q", notified(r))
	}
}

func TestCheckBGPIsSilentWithoutBGPSection(t *testing.T) {
	r := &recorder{}
	CheckBGP(&configuration.Config{}, &fakeProbe{}, r)
	if len(r.calls) != 0 {
		t.Errorf("notifications %q", notified(r))
	}
}

func seedRoutedVPC(t *testing.T) *fakeProbe {
	return &fakeProbe{
		vni: map[uint32]frr.VNIState{2700: {Type: "L3", State: "Up", RouterMAC: "02:02:c0:a8:0e:0b"}},
	}
}

func TestCheckL3VPCsReconcilesExportedRoutesWithTheirOwners(t *testing.T) {
	db := newTestDB(t)
	seedResource(t, db, prefixVPC, "vp-s7", state.Running)
	seedKV(t, db, "vpc/vp-s7/cidr", "10.241.0.0/16")
	seedKV(t, db, "vpc/vp-s7/l3vni", "2700")
	seedResource(t, db, prefixVPC, "vp-flat", state.Running)
	seedKV(t, db, "vpc/vp-flat/cidr", "10.242.0.0/16")

	seedResource(t, db, prefixSubnet, "sn-a", state.Running)
	seedKV(t, db, "subnet/sn-a/vpc", "vp-s7")
	seedKV(t, db, "subnet/sn-a/mode", "vxlan")
	seedKV(t, db, "subnet/sn-a/cidr", "10.241.1.0/24")
	seedResource(t, db, prefixSubnet, "sn-b", state.Creating)
	seedKV(t, db, "subnet/sn-b/vpc", "vp-s7")
	seedKV(t, db, "subnet/sn-b/mode", "vxlan")
	seedKV(t, db, "subnet/sn-b/cidr", "10.241.2.0/24")
	seedResource(t, db, prefixSubnet, "sn-br", state.Running)
	seedKV(t, db, "subnet/sn-br/vpc", "vp-s7")
	seedKV(t, db, "subnet/sn-br/mode", "bridge")
	seedKV(t, db, "subnet/sn-br/cidr", "192.168.50.0/24")

	vm := func(name string, st state.State, subnet, ip string) {
		seedResource(t, db, prefixVM, name, st)
		seedKV(t, db, "vm/"+name+"/nic/0/subnet", subnet)
		seedKV(t, db, "vm/"+name+"/nic/0/ip", ip)
	}
	vm("i-run", state.Running, "sn-a", "10.241.1.10")
	vm("i-missing", state.Running, "sn-a", "10.241.1.11")
	vm("i-starting", state.Creating, "sn-a", "10.241.1.12")
	vm("i-gone", state.Deleted, "sn-a", "10.241.1.13")
	vm("i-bridge", state.Running, "sn-br", "192.168.50.10")
	seedResource(t, db, prefixSubnet, "sn-flat", state.Running)
	seedKV(t, db, "subnet/sn-flat/vpc", "vp-flat")
	seedKV(t, db, "subnet/sn-flat/mode", "vxlan")
	seedKV(t, db, "subnet/sn-flat/cidr", "10.242.1.0/24")
	vm("i-flat", state.Running, "sn-flat", "10.242.1.10")

	p := seedRoutedVPC(t)
	p.routes = map[uint32][]string{2700: {
		"10.241.1.0/24", "10.241.2.0/24",
		"10.241.1.10/32", "10.241.1.12/32", "10.241.1.13/32", "10.241.1.50/32",
	}}
	r := &recorder{}
	if err := CheckL3VPCs(db, bgpCfg(), p, r); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"vpc vp-s7: route 10.241.1.13/32 exported in vrf-s7 without a running owner: its traffic is black-holed",
		"vpc vp-s7: route 10.241.1.50/32 exported in vrf-s7 without a running owner: its traffic is black-holed",
		"vpc vp-s7: route 10.241.1.11/32 not exported in vrf-s7",
	}
	if got := notified(r); !reflect.DeepEqual(got, want) {
		t.Errorf("notifications :\n%q\nattendu :\n%q", got, want)
	}
}

func TestCheckL3VPCsChecksInterfacesAndTheL3VNI(t *testing.T) {
	db := newTestDB(t)
	seedResource(t, db, prefixVPC, "vp-s7", state.Running)
	seedKV(t, db, "vpc/vp-s7/cidr", "10.241.0.0/16")
	seedKV(t, db, "vpc/vp-s7/l3vni", "2700")
	seedResource(t, db, prefixVPC, "vp-down", state.Error)
	seedKV(t, db, "vpc/vp-down/cidr", "10.243.0.0/16")
	seedKV(t, db, "vpc/vp-down/l3vni", "2800")

	p := &fakeProbe{
		vni:   map[uint32]frr.VNIState{2700: {Type: "L3", State: "Up", RouterMAC: "02:02:c0:a8:0e:99"}},
		links: map[string]string{"/vxl3-s7": "interface vxl3-s7 down", "vp-s7/l3-s7-i": "interface l3-s7-i not found (in netns)"},
	}
	r := &recorder{}
	if err := CheckL3VPCs(db, bgpCfg(), p, r); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"vpc vp-s7: interface vxl3-s7 down",
		"vpc vp-s7: interface l3-s7-i not found (in netns)",
		`vpc vp-s7: l3vni 2700 has router mac "02:02:c0:a8:0e:99" in frr, expected 02:02:c0:a8:0e:0b`,
	}
	if got := notified(r); !reflect.DeepEqual(got, want) {
		t.Errorf("notifications :\n%q\nattendu :\n%q", got, want)
	}

	p.vni[2700] = frr.VNIState{Type: "L2", State: "Up"}
	p.links = nil
	r = &recorder{}
	CheckL3VPCs(db, bgpCfg(), p, r)
	if !r.hasProblemContaining(`l3vni 2700 is type "L2" state "Up" in frr, expected L3 Up`) {
		t.Errorf("notifications %q", notified(r))
	}

	p.vni[2700] = frr.VNIState{Type: "L3", State: "Down", RouterMAC: "02:02:c0:a8:0e:0b"}
	r = &recorder{}
	CheckL3VPCs(db, bgpCfg(), p, r)
	if !r.hasProblemContaining(`l3vni 2700 is type "L3" state "Down" in frr, expected L3 Up`) {
		t.Errorf("notifications %q", notified(r))
	}

	delete(p.vni, 2700)
	r = &recorder{}
	CheckL3VPCs(db, bgpCfg(), p, r)
	if !r.hasProblemContaining("l3vni 2700 unreadable in frr") {
		t.Errorf("notifications %q", notified(r))
	}
}

func TestCheckL3VPCsReportsAnL3VNIWithoutBGP(t *testing.T) {
	db := newTestDB(t)
	seedResource(t, db, prefixVPC, "vp-s7", state.Running)
	seedKV(t, db, "vpc/vp-s7/cidr", "10.241.0.0/16")
	seedKV(t, db, "vpc/vp-s7/l3vni", "2700")
	r := &recorder{}
	CheckL3VPCs(db, &configuration.Config{}, &fakeProbe{}, r)
	want := []string{"vpc vp-s7: vpc has an l3vni but the agent configuration has no bgp section"}
	if got := notified(r); !reflect.DeepEqual(got, want) {
		t.Errorf("notifications %q, attendu %q", got, want)
	}
}

func TestCheckL3VPCsReportsUnreadableRoutesAndVTEP(t *testing.T) {
	db := newTestDB(t)
	seedResource(t, db, prefixVPC, "vp-s7", state.Running)
	seedKV(t, db, "vpc/vp-s7/cidr", "10.241.0.0/16")
	seedKV(t, db, "vpc/vp-s7/l3vni", "2700")
	seedResource(t, db, prefixVPC, "vp-s8", state.Running)
	seedKV(t, db, "vpc/vp-s8/cidr", "10.242.0.0/16")
	seedKV(t, db, "vpc/vp-s8/l3vni", "2800")
	p := seedRoutedVPC(t)
	p.routesErr = errors.New("netlink refused")
	p.vtepErr = errors.New("no address")
	r := &recorder{}
	CheckL3VPCs(db, bgpCfg(), p, r)
	if n := len(r.forName("vtep")); n != 1 {
		t.Errorf("VTEP illisible signalé %d fois, attendu une fois par passage : %q", n, notified(r))
	}
	if !r.hasProblemContaining("routes of vrf-s7 unreadable: netlink refused") || !r.hasProblemContaining("vtep address unreadable on br-000000") {
		t.Errorf("notifications %q", notified(r))
	}
	if r.hasProblemContaining("router mac") {
		t.Errorf("RMAC comparée sans VTEP : %q", notified(r))
	}
}

func TestCheckL3VPCsReportsAVRFWithoutUnreachableDefault(t *testing.T) {
	db := newTestDB(t)
	seedResource(t, db, prefixVPC, "vp-s7", state.Running)
	seedKV(t, db, "vpc/vp-s7/cidr", "10.241.0.0/16")
	seedKV(t, db, "vpc/vp-s7/l3vni", "2700")
	seedResource(t, db, prefixVPC, "vp-s8", state.Running)
	seedKV(t, db, "vpc/vp-s8/cidr", "10.242.0.0/16")
	seedKV(t, db, "vpc/vp-s8/l3vni", "2800")
	p := seedRoutedVPC(t)
	p.vni[2800] = frr.VNIState{Type: "L3", State: "Up", RouterMAC: "02:02:c0:a8:0e:0b"}
	p.unreachable = map[uint32]bool{2700: true}
	r := &recorder{}
	if err := CheckL3VPCs(db, bgpCfg(), p, r); err != nil {
		t.Fatal(err)
	}
	want := []string{"vpc vp-s8: no unreachable default in vrf-s8: traffic to an unknown destination leaves through the host routing table"}
	if got := notified(r); !reflect.DeepEqual(got, want) {
		t.Errorf("notifications :\n%q\nattendu :\n%q", got, want)
	}

	p.unreachable = map[uint32]bool{2700: true, 2800: true}
	p.unreachErr = errors.New("netlink refused")
	r = &recorder{}
	CheckL3VPCs(db, bgpCfg(), p, r)
	if !r.hasProblemContaining("unreachable default of vrf-s7 unreadable: netlink refused") || r.hasProblemContaining("no unreachable default") {
		t.Errorf("notifications %q", notified(r))
	}
}

func TestCheckL3VPCsExpectsTheDefaultRouteOfARunningNATGateway(t *testing.T) {
	cases := []struct {
		label    string
		gateway  state.State
		vpc      string
		exported bool
		want     []string
	}{
		{"passerelle running, route exportée", state.Running, "vp-s7", true, nil},
		{"passerelle running, route absente", state.Running, "vp-s7", false,
			[]string{"vpc vp-s7: route 0.0.0.0/0 not exported in vrf-s7"}},
		{"passerelle en création, route exportée", state.Creating, "vp-s7", true, nil},
		{"passerelle en erreur, route absente", state.Error, "vp-s7", false, nil},
		{"passerelle d'un autre VPC, route exportée", state.Running, "vp-s70", true,
			[]string{"vpc vp-s7: route 0.0.0.0/0 exported in vrf-s7 without a running owner: its traffic is black-holed"}},
	}
	for _, c := range cases {
		db := newTestDB(t)
		seedResource(t, db, prefixVPC, "vp-s7", state.Running)
		seedKV(t, db, "vpc/vp-s7/cidr", "10.241.0.0/16")
		seedKV(t, db, "vpc/vp-s7/l3vni", "2700")
		seedResource(t, db, prefixNATGW, "ng-1", c.gateway)
		seedKV(t, db, "nat-gateway/ng-1/vpc", c.vpc)
		p := seedRoutedVPC(t)
		p.routes = map[uint32][]string{2700: nil}
		if c.exported {
			p.routes[2700] = []string{"0.0.0.0/0"}
		}
		r := &recorder{}
		if err := CheckL3VPCs(db, bgpCfg(), p, r); err != nil {
			t.Fatal(err)
		}
		if got := notified(r); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s :\n%q\nattendu :\n%q", c.label, got, c.want)
		}
	}
}
