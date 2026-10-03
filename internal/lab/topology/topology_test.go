package topology

import (
	"strings"
	"testing"
)

const header = `name: lab-test
images:
  deb:
    url: https://example.invalid/deb.qcow2
    sums: https://example.invalid/SHA512SUMS
`

func parse(t *testing.T, doc string) *Topology {
	t.Helper()
	topo, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return topo
}

func validationError(t *testing.T, doc string) string {
	t.Helper()
	err := parse(t, doc).Validate()
	if err == nil {
		t.Fatalf("Validate accepted an invalid topology:\n%s", doc)
	}
	return err.Error()
}

func requireContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("error does not mention %q:\n%s", want, got)
	}
}

func TestParse_KeepsDeclarationOrderOfNodesAndSegments(t *testing.T) {
	topo := parse(t, header+`
segments:
  zulu:  { switch: sw, cidr: 10.0.1.0/24 }
  alpha: { switch: sw, cidr: 10.0.2.0/24 }
nodes:
  sw:   { role: switch, image: deb, cpus: 1, memory: 512 }
  zeta: { role: rr, image: deb, cpus: 1, memory: 512, segments: [zulu, alpha] }
  beta: { role: rr, image: deb, cpus: 1, memory: 512, segments: [zulu] }
`)
	var nodes, segments []string
	for _, n := range topo.Nodes {
		nodes = append(nodes, n.Name)
	}
	for _, s := range topo.Segments {
		segments = append(segments, s.Name)
	}
	if got := strings.Join(nodes, ","); got != "sw,zeta,beta" {
		t.Errorf("node order = %s, want sw,zeta,beta", got)
	}
	if got := strings.Join(segments, ","); got != "zulu,alpha" {
		t.Errorf("segment order = %s, want zulu,alpha", got)
	}
}

func TestParse_DefaultsMTUTo9000(t *testing.T) {
	topo := parse(t, header+`
segments:
  under: { switch: sw, cidr: 10.0.0.0/24 }
nodes:
  sw: { role: switch, image: deb, cpus: 1, memory: 512 }
`)
	if topo.Segments[0].MTU != 9000 {
		t.Errorf("MTU = %d, want 9000", topo.Segments[0].MTU)
	}
}

func TestParse_RejectsUnknownField(t *testing.T) {
	_, err := Parse([]byte(header + `
nodes:
  sw: { role: switch, image: deb, cpus: 1, memory: 512, ram: 4 }
`))
	if err == nil || !strings.Contains(err.Error(), "ram") {
		t.Fatalf("unknown field not rejected: %v", err)
	}
}

func TestParse_RejectsDuplicateNode(t *testing.T) {
	_, err := Parse([]byte(header + `
nodes:
  sw: { role: switch, image: deb, cpus: 1, memory: 512 }
  sw: { role: rr, image: deb, cpus: 1, memory: 512 }
`))
	if err == nil || !strings.Contains(err.Error(), "already defined") {
		t.Fatalf("duplicate node not rejected: %v", err)
	}
}

func TestParse_RejectsNonMappingDocument(t *testing.T) {
	if _, err := Parse([]byte("- a\n- b\n")); err == nil {
		t.Fatal("a list document was accepted")
	}
}

func TestLoad_ReportsPathOnError(t *testing.T) {
	if _, err := Load("/nonexistent/lab.yml"); err == nil {
		t.Fatal("missing file accepted")
	}
}

const valid = header + `
segments:
  under: { switch: sw, cidr: 10.0.0.0/24 }
nodes:
  sw: { role: switch, image: deb, cpus: 1, memory: 512 }
  rr: { role: rr, image: deb, cpus: 1, memory: 512, segments: [under] }
`

func TestValidate_AcceptsMinimalTopology(t *testing.T) {
	if err := parse(t, valid).Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidate_Rejections(t *testing.T) {
	cases := []struct {
		name, doc, want string
	}{
		{"bad lab name", strings.Replace(valid, "name: lab-test", "name: Lab_Test", 1), `name "Lab_Test"`},
		{"image url not https", strings.Replace(valid, "url: https://", "url: http://", 1), "image deb: url"},
		{"image sums not https", strings.Replace(valid, "sums: https://", "sums: ftp://", 1), "image deb: sums"},
		{"no node", header + "\nsegments: {}\nnodes: {}\n", "at least one node"},
		{"segment switch missing", strings.Replace(valid, "switch: sw, cidr", "cidr", 1), "segment under: switch is required"},
		{"segment switch unknown", strings.Replace(valid, "switch: sw, cidr", "switch: ghost, cidr", 1), "switch ghost is not a declared node"},
		{"segment switch not a switch", strings.Replace(valid, "switch: sw, cidr", "switch: rr, cidr", 1), "rr is a rr, not a switch"},
		{"segment name too long", strings.ReplaceAll(valid, "under", "underlayunder"), "segment underlayunder: name must match"},
		{"segment named like the admin interface", strings.ReplaceAll(valid, "under", "mgmt0"), "segment mgmt0: name is reserved"},
		{"segment name with dash", strings.ReplaceAll(valid, "under", "un-der"), "segment un-der: name must match"},
		{"mtu too high", strings.Replace(valid, "cidr: 10.0.0.0/24", "cidr: 10.0.0.0/24, mtu: 9001", 1), "mtu 9001 out of range"},
		{"mtu too low", strings.Replace(valid, "cidr: 10.0.0.0/24", "cidr: 10.0.0.0/24, mtu: 1279", 1), "mtu 1279 out of range"},
		{"cidr unparsable", strings.Replace(valid, "10.0.0.0/24", "10.0.0/24", 1), `cidr "10.0.0/24"`},
		{"cidr ipv6", strings.Replace(valid, "10.0.0.0/24", "fd00::/64", 1), "is not IPv4"},
		{"cidr host bits", strings.Replace(valid, "10.0.0.0/24", "10.0.0.5/24", 1), "network is 10.0.0.0/24"},
		{"cidr too small", strings.Replace(valid, "10.0.0.0/24", "10.0.0.0/31", 1), "prefix length out of range"},
		{"cidr too large", strings.Replace(valid, "10.0.0.0/24", "10.0.0.0/7", 1), "prefix length out of range"},
		{"bad node name", strings.Replace(valid, "  rr: {", "  RR: {", 1), "node RR: name must match"},
		{"bad role", strings.Replace(valid, "role: rr", "role: router", 1), `role "router"`},
		{"unknown image", strings.Replace(valid, "role: rr, image: deb", "role: rr, image: ubuntu", 1), `image "ubuntu" is not declared`},
		{"no cpu", strings.Replace(valid, "role: rr, image: deb, cpus: 1", "role: rr, image: deb, cpus: 0", 1), "node rr: cpus must be at least 1"},
		{"memory too low", strings.Replace(valid, "cpus: 1, memory: 512, segments", "cpus: 1, memory: 255, segments", 1), "node rr: memory must be at least 256"},
		{"switch with segments", strings.Replace(valid, "role: switch, image: deb, cpus: 1, memory: 512 }", "role: switch, image: deb, cpus: 1, memory: 512, segments: [under] }", 1), "node sw: a switch carries its segments"},
		{"switch with addresses", strings.Replace(valid, "role: switch, image: deb, cpus: 1, memory: 512 }", "role: switch, image: deb, cpus: 1, memory: 512, addresses: {under: 10.0.0.9} }", 1), "node sw: a switch carries its segments"},
		{"switch without segment", valid + "  sw2: { role: switch, image: deb, cpus: 1, memory: 512 }\n", "node sw2: switch carries no segment"},
		{"node without segment", valid + "  rr2: { role: rr, image: deb, cpus: 1, memory: 512 }\n", "node rr2: must be attached to at least one segment"},
		{"node on unknown segment", strings.Replace(valid, "segments: [under]", "segments: [over]", 1), "node rr: segment over is not declared"},
		{"node on segment twice", strings.Replace(valid, "segments: [under]", "segments: [under, under]", 1), "node rr: segment under listed twice"},
		{"segment without node", strings.Replace(valid, "  under: { switch: sw, cidr: 10.0.0.0/24 }", "  under: { switch: sw, cidr: 10.0.0.0/24 }\n  empty: { switch: sw, cidr: 10.0.9.0/24 }", 1), "segment empty: no node is attached"},
		{"address on foreign segment", strings.Replace(valid, "segments: [under]", "segments: [under], addresses: {over: 10.0.0.9}", 1), "address given for segment over"},
		{"address unparsable", strings.Replace(valid, "segments: [under]", "segments: [under], addresses: {under: 10.0.0}", 1), `address "10.0.0" on under`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			requireContains(t, validationError(t, c.doc), c.want)
		})
	}
}

func TestValidate_ReportsEveryErrorAtOnce(t *testing.T) {
	doc := strings.Replace(valid, "role: rr", "role: router", 1)
	doc = strings.Replace(doc, "10.0.0.0/24", "10.0.0.0/31", 1)
	got := validationError(t, doc)
	requireContains(t, got, `role "router"`)
	requireContains(t, got, "prefix length out of range")
}
