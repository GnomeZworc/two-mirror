package subnet

import (
	"strings"
	"testing"
)

func assertRules(t *testing.T, hook string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("%s : %q, %q attendu", hook, got, want)
	}
}

func TestVxlanFabricPort(t *testing.T) {
	p := VxlanFabricPort("v-s9r1-i", "10.9.1.254")
	if p.Name != "v-s9r1-i" {
		t.Errorf("port %q", p.Name)
	}
	assertRules(t, "prerouting", p.Prerouting, nil)
	assertRules(t, "forward", p.Forward, []string{
		"arp operation request arp daddr ip 10.9.1.254 drop",
		"ip saddr 10.9.1.254 udp sport 67-68 udp dport 67-68 drop",
	})
	assertRules(t, "output", p.Output, nil)
}

func TestAnycastFabricPort(t *testing.T) {
	p := AnycastFabricPort("v-s9r1-i", "02:01:00:00:0b:5f", "10.9.1.254", "10.9.1.253")
	assertRules(t, "prerouting", p.Prerouting, []string{"ether saddr 02:01:00:00:0b:5f drop"})
	assertRules(t, "forward", p.Forward, []string{
		"ether saddr 02:01:00:00:0b:5f drop",
		"arp operation request arp daddr ip 10.9.1.254 drop",
		"arp operation request arp daddr ip 10.9.1.253 drop",
		"ip saddr 10.9.1.254 udp sport 67-68 udp dport 67-68 drop",
		"ip saddr 10.9.1.253 udp sport 67-68 udp dport 67-68 drop",
	})
	assertRules(t, "output", p.Output, []string{"ether saddr 02:01:00:00:0b:5f drop"})
}

func TestBridgeFabricPort(t *testing.T) {
	p := BridgeFabricPort("v-t33a-i", "10.33.0.1")
	assertRules(t, "prerouting", p.Prerouting, nil)
	assertRules(t, "forward", p.Forward, []string{
		"arp operation request arp daddr ip 10.33.0.1 drop",
		"ip saddr 10.33.0.1 udp sport 67-68 udp dport 67-68 drop",
	})
	assertRules(t, "output", p.Output, []string{
		"arp saddr ip 10.33.0.1 drop",
		"ip saddr 10.33.0.1 udp sport 67-68 udp dport 67-68 drop",
	})
}
