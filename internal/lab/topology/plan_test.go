package topology

import (
	"bytes"
	"fmt"
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

func compute(t *testing.T, doc string) *Plan {
	t.Helper()
	p, err := Compute(parse(t, doc))
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	return p
}

func computeError(t *testing.T, doc string) string {
	t.Helper()
	_, err := Compute(parse(t, doc))
	if err == nil {
		t.Fatalf("Compute accepted:\n%s", doc)
	}
	return err.Error()
}

func cableOf(t *testing.T, p *Plan, node, segment string) Cable {
	t.Helper()
	for _, c := range p.Cables {
		if c.Node == node && c.Segment == segment {
			return c
		}
	}
	t.Fatalf("no cable for %s on %s", node, segment)
	return Cable{}
}

func nodeOf(t *testing.T, p *Plan, name string) NodePlan {
	t.Helper()
	for _, n := range p.Nodes {
		if n.Name == name {
			return n
		}
	}
	t.Fatalf("no node %s", name)
	return NodePlan{}
}

const twoHypervisors = header + `
segments:
  underlay: { switch: sw1, cidr: 10.250.0.0/24, mtu: 9000 }
nodes:
  sw1: { role: switch,     image: deb, cpus: 2, memory: 1024 }
  rr1: { role: rr,         image: deb, cpus: 1, memory: 1024, segments: [underlay] }
  hv1: { role: hypervisor, image: deb, cpus: 4, memory: 16384, segments: [underlay] }
  hv2: { role: hypervisor, image: deb, cpus: 4, memory: 16384, segments: [underlay] }
`

func TestCompute_TwoHypervisorsPlan(t *testing.T) {
	p := compute(t, twoHypervisors)

	if len(p.Segments) != 1 {
		t.Fatalf("%d segments, want 1", len(p.Segments))
	}
	s := p.Segments[0]
	if s.Bridge != "br-underlay" || s.Gateway != netip.MustParseAddr("10.250.0.1") || s.MTU != 9000 || s.Switch != "sw1" {
		t.Errorf("segment = %+v", s)
	}

	want := []struct {
		node, address, nodeMAC, switchMAC, switchIface string
		nodePort, switchPort                           int
	}{
		{"rr1", "10.250.0.2/24", "02:4c:00:01:00:00", "02:4c:00:01:00:01", "p0", 20000, 20001},
		{"hv1", "10.250.0.3/24", "02:4c:00:02:00:00", "02:4c:00:02:00:01", "p1", 20002, 20003},
		{"hv2", "10.250.0.4/24", "02:4c:00:03:00:00", "02:4c:00:03:00:01", "p2", 20004, 20005},
	}
	if len(p.Cables) != len(want) {
		t.Fatalf("%d cables, want %d", len(p.Cables), len(want))
	}
	for _, w := range want {
		c := cableOf(t, p, w.node, "underlay")
		if c.NodeAddress.String() != w.address {
			t.Errorf("%s address = %s, want %s", w.node, c.NodeAddress, w.address)
		}
		if c.NodeMAC.String() != w.nodeMAC || c.SwitchMAC.String() != w.switchMAC {
			t.Errorf("%s macs = %s / %s, want %s / %s", w.node, c.NodeMAC, c.SwitchMAC, w.nodeMAC, w.switchMAC)
		}
		if c.NodePort != w.nodePort || c.SwitchPort != w.switchPort {
			t.Errorf("%s ports = %d / %d, want %d / %d", w.node, c.NodePort, c.SwitchPort, w.nodePort, w.switchPort)
		}
		if c.NodeInterface != "underlay" || c.SwitchInterface != w.switchIface || c.Switch != "sw1" || c.MTU != 9000 {
			t.Errorf("%s cable = %+v", w.node, c)
		}
	}

	for name, port := range map[string]int{"sw1": 2200, "rr1": 2201, "hv1": 2202, "hv2": 2203} {
		if got := nodeOf(t, p, name).SSHPort; got != port {
			t.Errorf("%s ssh port = %d, want %d", name, got, port)
		}
	}
	if hv := nodeOf(t, p, "hv1"); hv.Role != "hypervisor" || hv.CPUs != 4 || hv.Memory != 16384 || hv.Image != "deb" {
		t.Errorf("hv1 = %+v", hv)
	}
}

func TestCompute_CarriesTheDeclaredImages(t *testing.T) {
	p := compute(t, twoHypervisors)
	want := []Image{{Name: "deb", URL: "https://example.invalid/deb.qcow2", Sums: "https://example.invalid/SHA512SUMS"}}
	if !reflect.DeepEqual(p.Images, want) {
		t.Errorf("images = %+v, want %+v", p.Images, want)
	}
}

func TestCompute_IsStableAcrossRuns(t *testing.T) {
	first := compute(t, twoHypervisors)
	for i := 0; i < 20; i++ {
		if again := compute(t, twoHypervisors); !reflect.DeepEqual(first, again) {
			t.Fatalf("plan differs on run %d", i)
		}
	}
}

func TestCompute_AddressesFollowDeclarationOrderNotNames(t *testing.T) {
	p := compute(t, header+`
segments:
  under: { switch: sw, cidr: 10.0.0.0/24 }
nodes:
  sw:   { role: switch, image: deb, cpus: 1, memory: 512 }
  zeta: { role: rr, image: deb, cpus: 1, memory: 512, segments: [under] }
  alfa: { role: rr, image: deb, cpus: 1, memory: 512, segments: [under] }
`)
	if got := cableOf(t, p, "zeta", "under").NodeAddress.String(); got != "10.0.0.2/24" {
		t.Errorf("zeta = %s, want 10.0.0.2/24", got)
	}
	if got := cableOf(t, p, "alfa", "under").NodeAddress.String(); got != "10.0.0.3/24" {
		t.Errorf("alfa = %s, want 10.0.0.3/24", got)
	}
}

func TestCompute_ExplicitAddressIsKeptAndSkippedByAllocation(t *testing.T) {
	p := compute(t, header+`
segments:
  under: { switch: sw, cidr: 10.0.0.0/24 }
nodes:
  sw: { role: switch, image: deb, cpus: 1, memory: 512 }
  a:  { role: rr, image: deb, cpus: 1, memory: 512, segments: [under] }
  b:  { role: rr, image: deb, cpus: 1, memory: 512, segments: [under], addresses: {under: 10.0.0.3} }
  c:  { role: rr, image: deb, cpus: 1, memory: 512, segments: [under] }
`)
	for node, want := range map[string]string{"a": "10.0.0.2/24", "b": "10.0.0.3/24", "c": "10.0.0.4/24"} {
		if got := cableOf(t, p, node, "under").NodeAddress.String(); got != want {
			t.Errorf("%s = %s, want %s", node, got, want)
		}
	}
}

func TestCompute_TwoSegmentsCableOrderMacsAndPorts(t *testing.T) {
	p := compute(t, header+`
segments:
  red:  { switch: sw, cidr: 10.1.0.0/24 }
  blue: { switch: sw, cidr: 10.2.0.0/24, mtu: 1500 }
nodes:
  sw: { role: switch, image: deb, cpus: 1, memory: 512 }
  hv: { role: hypervisor, image: deb, cpus: 1, memory: 512, segments: [blue, red] }
  rr: { role: rr, image: deb, cpus: 1, memory: 512, segments: [red] }
`)
	got := make([]string, 0, len(p.Cables))
	for _, c := range p.Cables {
		got = append(got, c.Segment+"/"+c.Node+"/"+c.SwitchInterface)
	}
	if strings.Join(got, ",") != "red/hv/p0,red/rr/p1,blue/hv/p2" {
		t.Errorf("cable order = %v", got)
	}
	blue := cableOf(t, p, "hv", "blue")
	if blue.NodeMAC.String() != "02:4c:00:01:01:00" || blue.NodePort != 20004 || blue.MTU != 1500 || blue.NodeAddress.String() != "10.2.0.2/24" {
		t.Errorf("hv on blue = %+v", blue)
	}
	if red := cableOf(t, p, "hv", "red"); red.NodeMAC.String() != "02:4c:00:01:00:00" {
		t.Errorf("hv on red mac = %s", red.NodeMAC)
	}
}

func TestCompute_EveryMacAndPortIsUnique(t *testing.T) {
	p := compute(t, header+`
segments:
  a: { switch: sw, cidr: 10.1.0.0/24 }
  b: { switch: sw, cidr: 10.2.0.0/24 }
nodes:
  sw: { role: switch, image: deb, cpus: 1, memory: 512 }
  n1: { role: rr, image: deb, cpus: 1, memory: 512, segments: [a, b] }
  n2: { role: rr, image: deb, cpus: 1, memory: 512, segments: [a, b] }
  n3: { role: hypervisor, image: deb, cpus: 1, memory: 512, segments: [b, a] }
`)
	macs := map[string]bool{}
	ports := map[int]bool{}
	for _, n := range p.Nodes {
		ports[n.SSHPort] = true
	}
	for _, c := range p.Cables {
		for _, m := range []string{c.NodeMAC.String(), c.SwitchMAC.String()} {
			if macs[m] {
				t.Errorf("mac %s used twice", m)
			}
			macs[m] = true
		}
		for _, port := range []int{c.NodePort, c.SwitchPort} {
			if ports[port] {
				t.Errorf("port %d used twice", port)
			}
			ports[port] = true
		}
	}
	if len(macs) != 12 {
		t.Errorf("%d distinct macs, want 12", len(macs))
	}
}

func TestCompute_MacsAreLocallyAdministeredUnicast(t *testing.T) {
	for _, c := range compute(t, twoHypervisors).Cables {
		for _, m := range []string{c.NodeMAC.String(), c.SwitchMAC.String()} {
			if !strings.HasPrefix(m, "02:") {
				t.Errorf("mac %s is not locally administered unicast", m)
			}
		}
	}
}

func TestCompute_AddressRejections(t *testing.T) {
	base := func(addresses string) string {
		return header + `
segments:
  under: { switch: sw, cidr: 10.0.0.0/29 }
nodes:
  sw: { role: switch, image: deb, cpus: 1, memory: 512 }
  a:  { role: rr, image: deb, cpus: 1, memory: 512, segments: [under]` + addresses + ` }
  b:  { role: rr, image: deb, cpus: 1, memory: 512, segments: [under], addresses: {under: 10.0.0.5} }
`
	}
	cases := []struct{ name, addresses, want string }{
		{"outside", ", addresses: {under: 10.0.1.2}", "node a: address 10.0.1.2 is outside segment under"},
		{"network", ", addresses: {under: 10.0.0.0}", "node a: address 10.0.0.0 is the network or broadcast"},
		{"broadcast", ", addresses: {under: 10.0.0.7}", "node a: address 10.0.0.7 is the network or broadcast"},
		{"gateway", ", addresses: {under: 10.0.0.1}", "node a: address 10.0.0.1 on segment under is already taken by sw (gateway)"},
		{"duplicate", ", addresses: {under: 10.0.0.5}", "node b: address 10.0.0.5 on segment under is already taken by a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			requireContains(t, computeError(t, base(c.addresses)), c.want)
		})
	}
}

func TestCompute_ReportsExhaustedSegment(t *testing.T) {
	got := computeError(t, header+`
segments:
  tiny: { switch: sw, cidr: 10.0.0.0/30 }
nodes:
  sw: { role: switch, image: deb, cpus: 1, memory: 512 }
  a:  { role: rr, image: deb, cpus: 1, memory: 512, segments: [tiny] }
  b:  { role: rr, image: deb, cpus: 1, memory: 512, segments: [tiny] }
`)
	requireContains(t, got, "segment tiny: no address left in 10.0.0.0/30 for node b")
}

func TestCompute_FillsSegmentExactly(t *testing.T) {
	p := compute(t, header+`
segments:
  tiny: { switch: sw, cidr: 10.0.0.0/29 }
nodes:
  sw: { role: switch, image: deb, cpus: 1, memory: 512 }
  a:  { role: rr, image: deb, cpus: 1, memory: 512, segments: [tiny] }
  b:  { role: rr, image: deb, cpus: 1, memory: 512, segments: [tiny] }
  c:  { role: rr, image: deb, cpus: 1, memory: 512, segments: [tiny] }
  d:  { role: rr, image: deb, cpus: 1, memory: 512, segments: [tiny] }
  e:  { role: rr, image: deb, cpus: 1, memory: 512, segments: [tiny] }
`)
	if got := cableOf(t, p, "e", "tiny").NodeAddress.String(); got != "10.0.0.6/29" {
		t.Errorf("last node = %s, want 10.0.0.6/29", got)
	}
}

func TestCompute_RefusesInvalidTopology(t *testing.T) {
	requireContains(t, computeError(t, strings.Replace(valid, "role: rr", "role: router", 1)), `role "router"`)
}

func TestLastAddr(t *testing.T) {
	for cidr, want := range map[string]string{
		"10.0.0.0/24":    "10.0.0.255",
		"10.0.0.0/30":    "10.0.0.3",
		"10.250.0.0/16":  "10.250.255.255",
		"192.168.4.8/29": "192.168.4.15",
	} {
		if got := lastAddr(netip.MustParsePrefix(cidr)).String(); got != want {
			t.Errorf("lastAddr(%s) = %s, want %s", cidr, got, want)
		}
	}
}

func TestWrite_TwoHypervisorsPlan(t *testing.T) {
	var buf bytes.Buffer
	if err := compute(t, twoHypervisors).Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	want := `lab lab-test: nodes 4, segments 1, cables 3

nodes
  name  role        image  cpus  memory     ssh
  sw1   switch      deb    2     1024 MiB   127.0.0.1:2200
  rr1   rr          deb    1     1024 MiB   127.0.0.1:2201
  hv1   hypervisor  deb    4     16384 MiB  127.0.0.1:2202
  hv2   hypervisor  deb    4     16384 MiB  127.0.0.1:2203

segment underlay: 10.250.0.0/24, mtu 9000, switch sw1, bridge br-underlay, gateway 10.250.0.1
  node  interface  address        mac                udp         switch port  mac                udp
  rr1   underlay   10.250.0.2/24  02:4c:00:01:00:00  20000  <->  sw1 p0       02:4c:00:01:00:01  20001
  hv1   underlay   10.250.0.3/24  02:4c:00:02:00:00  20002  <->  sw1 p1       02:4c:00:02:00:01  20003
  hv2   underlay   10.250.0.4/24  02:4c:00:03:00:00  20004  <->  sw1 p2       02:4c:00:03:00:01  20005
`
	if buf.String() != want {
		t.Errorf("output:\n%s\nwant:\n%s", buf.String(), want)
	}
}

func generated(segments, nodes int) string {
	var b strings.Builder
	b.WriteString(header + "segments:\n")
	names := make([]string, segments)
	for i := range names {
		names[i] = fmt.Sprintf("s%d", i)
		fmt.Fprintf(&b, "  %s: { switch: sw, cidr: 10.%d.0.0/16 }\n", names[i], i%250)
	}
	b.WriteString("nodes:\n  sw: { role: switch, image: deb, cpus: 1, memory: 512 }\n")
	for i := 1; i < nodes; i++ {
		fmt.Fprintf(&b, "  n%d: { role: rr, image: deb, cpus: 1, memory: 512, segments: [%s] }\n", i, strings.Join(names, ", "))
	}
	return b.String()
}

func TestValidate_RejectsTooManyNodes(t *testing.T) {
	requireContains(t, validationError(t, generated(1, 1001)), "nodes: 1001 declared, at most 1000")
}

func TestValidate_AcceptsExactlyMaxNodes(t *testing.T) {
	if err := parse(t, generated(1, 1000)).Validate(); err != nil {
		t.Fatalf("1000 nodes rejected: %v", err)
	}
}

func TestValidate_RejectsTooManySegments(t *testing.T) {
	requireContains(t, validationError(t, generated(257, 2)), "segments: 257 declared, at most 256")
}

func TestCompute_RejectsCablesBeyondUDPPortRange(t *testing.T) {
	requireContains(t, computeError(t, generated(23, 1000)), "cables: 22977 cables need udp ports up to 65953, beyond 65535")
}
