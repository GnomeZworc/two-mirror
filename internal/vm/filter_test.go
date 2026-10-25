package vm

import (
	"strings"
	"testing"
)

func TestTapPortFiltersEverythingTheVMMustNotSend(t *testing.T) {
	p := TapPort(12345678, "52:54:00:00:0A:05", "10.0.0.5", false)
	if p.Name != "tap12345678" {
		t.Errorf("port %q, tap12345678 attendu", p.Name)
	}
	want := []string{
		"ether saddr != 52:54:00:00:0a:05 drop",
		"ether type != { ip, arp, ip6 } drop",
		"arp saddr ether != 52:54:00:00:0a:05 drop",
		"arp saddr ip != { 0.0.0.0, 10.0.0.5 } drop",
		"ip saddr != { 0.0.0.0, 10.0.0.5 } drop",
		"ip saddr 0.0.0.0 ip protocol != udp drop",
		"ip saddr 0.0.0.0 udp dport != 67 drop",
		"udp sport 67 drop",
	}
	if strings.Join(p.Prerouting, "\n") != strings.Join(want, "\n") {
		t.Errorf("règles :\n%s\nattendues :\n%s", strings.Join(p.Prerouting, "\n"), strings.Join(want, "\n"))
	}
	if len(p.Forward) != 0 || len(p.Output) != 0 {
		t.Errorf("seul ce qu'émet la VM est filtré : forward %q, output %q", p.Forward, p.Output)
	}
}

func TestTapPortLetsAnAuthorizedNICAnswerDHCP(t *testing.T) {
	allowed := TapPort(12345678, "52:54:00:00:0a:05", "10.0.0.5", true)
	denied := TapPort(12345678, "52:54:00:00:0a:05", "10.0.0.5", false)
	for _, r := range allowed.Prerouting {
		if r == "udp sport 67 drop" {
			t.Errorf("la NIC autorisée garde le blocage du serveur DHCP")
		}
	}
	if len(allowed.Prerouting) != len(denied.Prerouting)-1 {
		t.Errorf("seul le blocage du serveur DHCP doit disparaître : %q", allowed.Prerouting)
	}
}

func TestNICTapPortUsesItsOwnData(t *testing.T) {
	n := nicData{tapID: 87654321, mac: "52:54:00:00:0b:07", ip: "10.0.1.7", dhcpServer: true}
	p := n.tapPort()
	if p.Name != "tap87654321" || p.Prerouting[0] != "ether saddr != 52:54:00:00:0b:07 drop" || p.Prerouting[3] != "arp saddr ip != { 0.0.0.0, 10.0.1.7 } drop" {
		t.Errorf("port %+v", p)
	}
	if strings.Contains(strings.Join(p.Prerouting, "|"), "udp sport 67 drop") {
		t.Errorf("dhcpServer de la NIC ignoré")
	}
}
