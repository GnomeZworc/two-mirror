package subnet

import (
	"net"
	"testing"
)

func TestAnycastOnlyForVxlanWithRelay(t *testing.T) {
	relay := net.ParseIP("10.0.0.253")
	cases := []struct {
		label string
		d     subnetData
		want  bool
	}{
		{"vxlan avec relay_ip", subnetData{mode: ModeVxlan, relayIP: relay}, true},
		{"vxlan existant sans relay_ip", subnetData{mode: ModeVxlan}, false},
		{"bridge", subnetData{mode: ModeBridge, relayIP: relay}, false},
		{"public_ip", subnetData{mode: ModePublicIP, relayIP: relay}, false},
	}
	for _, c := range cases {
		if got := c.d.anycast(); got != c.want {
			t.Errorf("%s : %v, %v attendu", c.label, got, c.want)
		}
	}
}

func TestGatewayLinkFitsInterfaceNameLimit(t *testing.T) {
	d := subnetData{subnetID: "s9r1", bridge: "br-s9r1"}
	if d.gatewayLink() != "gw-s9r1" {
		t.Errorf("%s, gw-s9r1 attendu", d.gatewayLink())
	}
	if len(d.gatewayLink()) != len(d.bridge) {
		t.Errorf("%s et %s doivent avoir la même longueur", d.gatewayLink(), d.bridge)
	}
}

func TestArpSysctls(t *testing.T) {
	want := [][2]string{
		{"net.ipv4.conf.br-s9r1.arp_ignore", "1"},
		{"net.ipv4.conf.br-s9r1.arp_announce", "2"},
	}
	got := arpSysctls("br-s9r1")
	if len(got) != len(want) {
		t.Fatalf("%d sysctls, %d attendus", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sysctl %d : %v, %v attendu", i, got[i], want[i])
		}
	}
}
