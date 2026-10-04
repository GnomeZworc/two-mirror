package topology

import (
	"bytes"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const withRoles = header + `
segments:
  underlay: { switch: sw1, cidr: 192.168.14.0/24 }
nodes:
  sw1: { role: switch, image: deb, cpus: 2, memory: 1024, secondary: { underlay: [169.254.0.1/28] }, frr: frr/sw1.conf }
  rr1: { role: rr, image: deb, cpus: 1, memory: 1024, segments: [underlay], secondary: { underlay: [169.254.0.3/28] }, loopback: 10.255.255.1/32, frr: frr/rr1.conf }
  hv1: { role: hypervisor, image: deb, cpus: 4, memory: 16384, segments: [underlay], frr: /etc/lab/hv1.conf, release: 0.2.0rc002 }
`

func TestCompute_CarriesTheRoleFields(t *testing.T) {
	p := compute(t, withRoles)

	rr1 := nodeOf(t, p, "rr1")
	if !reflect.DeepEqual(rr1.Secondary, map[string][]netip.Prefix{"underlay": {netip.MustParsePrefix("169.254.0.3/28")}}) {
		t.Errorf("rr1 secondary = %v", rr1.Secondary)
	}
	if rr1.Loopback != netip.MustParsePrefix("10.255.255.1/32") {
		t.Errorf("rr1 loopback = %v", rr1.Loopback)
	}
	if rr1.FRR != "frr/rr1.conf" {
		t.Errorf("rr1 frr = %q", rr1.FRR)
	}
	hv1 := nodeOf(t, p, "hv1")
	if hv1.Secondary != nil || hv1.Loopback.IsValid() {
		t.Errorf("hv1 = %+v, want no secondary and no loopback", hv1)
	}
}

func TestLoad_ResolvesFRRPathsAgainstTheTopologyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lab.yml")
	if err := os.WriteFile(path, []byte(withRoles), 0o600); err != nil {
		t.Fatal(err)
	}

	topo, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := map[string]string{
		"sw1": filepath.Join(dir, "frr", "sw1.conf"),
		"rr1": filepath.Join(dir, "frr", "rr1.conf"),
		"hv1": "/etc/lab/hv1.conf",
	}
	for _, n := range topo.Nodes {
		if n.FRR != want[n.Name] {
			t.Errorf("%s frr = %q, want %q", n.Name, n.FRR, want[n.Name])
		}
	}
}

func TestValidate_RoleFieldRejections(t *testing.T) {
	cases := map[string]struct {
		node string
		want string
	}{
		"secondary on a segment not attached": {
			`rr1: { role: rr, image: deb, cpus: 1, memory: 512, segments: [red], secondary: { blue: [169.254.0.3/28] } }`,
			"node rr1: secondary address given for segment blue it is not attached to",
		},
		"secondary without prefix length": {
			`rr1: { role: rr, image: deb, cpus: 1, memory: 512, segments: [red], secondary: { red: [169.254.0.3] } }`,
			`node rr1: secondary address "169.254.0.3" on red`,
		},
		"secondary in IPv6": {
			`rr1: { role: rr, image: deb, cpus: 1, memory: 512, segments: [red], secondary: { red: ["fd00::3/64"] } }`,
			"node rr1: secondary address fd00::3/64 on red is not IPv4",
		},
		"secondary inside the segment": {
			`rr1: { role: rr, image: deb, cpus: 1, memory: 512, segments: [red], secondary: { red: [10.1.0.9/24] } }`,
			"node rr1: secondary address 10.1.0.9/24 is inside segment red (10.1.0.0/24), use addresses instead",
		},
		"loopback without prefix length": {
			`rr1: { role: rr, image: deb, cpus: 1, memory: 512, segments: [red], loopback: 10.255.255.1 }`,
			`node rr1: loopback "10.255.255.1"`,
		},
		"loopback in IPv6": {
			`rr1: { role: rr, image: deb, cpus: 1, memory: 512, segments: [red], loopback: "fd00::1/128" }`,
			"node rr1: loopback fd00::1/128 is not IPv4",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			doc := header + `
segments:
  red:  { switch: sw, cidr: 10.1.0.0/24 }
  blue: { switch: sw, cidr: 10.2.0.0/24 }
nodes:
  sw: { role: switch, image: deb, cpus: 1, memory: 512 }
  hv: { role: hypervisor, image: deb, cpus: 1, memory: 512, segments: [red, blue], release: 0.2.0rc002 }
  ` + c.node + `
`
			requireContains(t, validationError(t, doc), c.want)
		})
	}
}

func TestValidate_SwitchSecondaryOnlyOnItsOwnSegments(t *testing.T) {
	doc := header + `
segments:
  red:  { switch: sw, cidr: 10.1.0.0/24 }
  blue: { switch: other, cidr: 10.2.0.0/24 }
nodes:
  sw:    { role: switch, image: deb, cpus: 1, memory: 512, secondary: { red: [169.254.0.1/28], blue: [169.254.1.1/28] } }
  other: { role: switch, image: deb, cpus: 1, memory: 512 }
  hv:    { role: hypervisor, image: deb, cpus: 1, memory: 512, segments: [red, blue], release: 0.2.0rc002 }
`
	msg := validationError(t, doc)
	requireContains(t, msg, "node sw: secondary address given for segment blue it is not attached to")
	if bytes.Contains([]byte(msg), []byte("segment red")) {
		t.Errorf("the switch's own segment was refused:\n%s", msg)
	}
}

func TestValidate_LoopbackInterfaceNameIsReserved(t *testing.T) {
	doc := header + `
segments:
  lo1: { switch: sw, cidr: 10.1.0.0/24 }
nodes:
  sw: { role: switch, image: deb, cpus: 1, memory: 512 }
  hv: { role: hypervisor, image: deb, cpus: 1, memory: 512, segments: [lo1], release: 0.2.0rc002 }
`
	requireContains(t, validationError(t, doc), "segment lo1: name is reserved for the loopback interface")
}

func TestWrite_ShowsTheRoles(t *testing.T) {
	var buf bytes.Buffer
	if err := compute(t, withRoles).Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	want := `
roles
  name  loopback             secondary                frr       release
  sw1   -                    underlay 169.254.0.1/28  sw1.conf  -
  rr1   lo1 10.255.255.1/32  underlay 169.254.0.3/28  rr1.conf  -
  hv1   -                    -                        hv1.conf  0.2.0rc002
`
	if !bytes.Contains(buf.Bytes(), []byte(want)) {
		t.Errorf("plan:\n%s\ndoes not contain:\n%s", buf.String(), want)
	}
}

func TestWrite_NoRolesSectionWithoutRoleFields(t *testing.T) {
	var buf bytes.Buffer
	doc := header + `
segments:
  underlay: { switch: sw1, cidr: 10.250.0.0/24 }
nodes:
  sw1: { role: switch, image: deb, cpus: 1, memory: 512 }
  rr1: { role: rr, image: deb, cpus: 1, memory: 512, segments: [underlay] }
`
	if err := compute(t, doc).Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if bytes.Contains(buf.Bytes(), []byte("roles")) {
		t.Errorf("plan shows a roles section:\n%s", buf.String())
	}
}

func TestValidate_ReleaseRejections(t *testing.T) {
	cases := map[string]struct {
		node string
		want string
	}{
		"hypervisor without release": {
			`hv: { role: hypervisor, image: deb, cpus: 1, memory: 512, segments: [red] }`,
			"node hv: a hypervisor needs the release of two to deploy (release: <tag>)",
		},
		"release with shell characters": {
			`hv: { role: hypervisor, image: deb, cpus: 1, memory: 512, segments: [red], release: "0.2.0; reboot" }`,
			`node hv: release "0.2.0; reboot" must match`,
		},
		"release on a route reflector": {
			`hv: { role: rr, image: deb, cpus: 1, memory: 512, segments: [red], release: 0.2.0rc002 }`,
			"node hv: release is only for hypervisors",
		},
		"release on a switch": {
			`hv: { role: hypervisor, image: deb, cpus: 1, memory: 512, segments: [red], release: 0.2.0rc002 }
  sw2: { role: switch, image: deb, cpus: 1, memory: 512, release: 0.2.0rc002 }`,
			"node sw2: release is only for hypervisors",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			doc := header + `
segments:
  red: { switch: sw, cidr: 10.1.0.0/24 }
nodes:
  sw: { role: switch, image: deb, cpus: 1, memory: 512 }
  ` + c.node + `
`
			requireContains(t, validationError(t, doc), c.want)
		})
	}
}

func TestCompute_CarriesTheRelease(t *testing.T) {
	if got := nodeOf(t, compute(t, withRoles), "hv1").Release; got != "0.2.0rc002" {
		t.Errorf("hv1 release = %q", got)
	}
	if got := nodeOf(t, compute(t, withRoles), "rr1").Release; got != "" {
		t.Errorf("rr1 release = %q", got)
	}
}
