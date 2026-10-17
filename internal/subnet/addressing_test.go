package subnet

import (
	"strings"
	"testing"
)

func TestAnycastAddresses_Slash24(t *testing.T) {
	a, err := AnycastAddresses("10.0.0.0/24")
	if err != nil {
		t.Fatalf("AnycastAddresses a échoué : %v", err)
	}
	for label, c := range map[string][2]string{
		"network":   {"10.0.0.0", a.Network.String()},
		"broadcast": {"10.0.0.255", a.Broadcast.String()},
		"gateway":   {"10.0.0.254", a.Gateway.String()},
		"relay":     {"10.0.0.253", a.Relay.String()},
	} {
		if c[0] != c[1] {
			t.Errorf("%s attendu %s, obtenu %s", label, c[0], c[1])
		}
	}
	if a.Prefix.String() != "10.0.0.0/24" {
		t.Errorf("prefix attendu 10.0.0.0/24, obtenu %s", a.Prefix)
	}
}

func TestAnycastAddresses_Slash16(t *testing.T) {
	a, err := AnycastAddresses("172.16.0.0/16")
	if err != nil {
		t.Fatalf("AnycastAddresses a échoué : %v", err)
	}
	if a.Broadcast.String() != "172.16.255.255" {
		t.Errorf("broadcast attendu 172.16.255.255, obtenu %s", a.Broadcast)
	}
	if a.Gateway.String() != "172.16.255.254" {
		t.Errorf("gateway attendue 172.16.255.254, obtenu %s", a.Gateway)
	}
	if a.Relay.String() != "172.16.255.253" {
		t.Errorf("relay attendu 172.16.255.253, obtenu %s", a.Relay)
	}
}

func TestAnycastAddresses_Slash29IsTheSmallestAccepted(t *testing.T) {
	a, err := AnycastAddresses("192.168.1.8/29")
	if err != nil {
		t.Fatalf("un /29 doit être accepté : %v", err)
	}
	if a.Gateway.String() != "192.168.1.14" {
		t.Errorf("gateway attendue 192.168.1.14, obtenu %s", a.Gateway)
	}
	if a.Relay.String() != "192.168.1.13" {
		t.Errorf("relay attendu 192.168.1.13, obtenu %s", a.Relay)
	}
}

func TestAnycastAddresses_Refused(t *testing.T) {
	cases := map[string]struct {
		cidr string
		want string
	}{
		"/30 trop petit":      {"10.0.0.0/30", "too small"},
		"bits d'hôte":         {"10.0.0.1/24", "host bits"},
		"ipv6":                {"fd00::/64", "not ipv4"},
		"cidr illisible":      {"pas-un-cidr", "invalid cidr"},
		"adresse sans prefix": {"10.0.0.0", "invalid cidr"},
	}
	for label, c := range cases {
		_, err := AnycastAddresses(c.cidr)
		if err == nil {
			t.Errorf("%s : %q devrait être refusé", label, c.cidr)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s : erreur attendue contenant %q, obtenu %v", label, c.want, err)
		}
	}
}

func TestCheckHostAddress_ValidHost(t *testing.T) {
	if err := CheckHostAddress("10.0.0.0/24", "10.0.0.5", "10.0.0.254", "10.0.0.253"); err != nil {
		t.Errorf("10.0.0.5 devrait être accepté : %v", err)
	}
}

func TestCheckHostAddress_Refused(t *testing.T) {
	cases := map[string]struct {
		ip   string
		want string
	}{
		"hors cidr":           {"10.0.1.5", "outside"},
		"adresse réseau":      {"10.0.0.0", "network address"},
		"adresse broadcast":   {"10.0.0.255", "broadcast address"},
		"passerelle réservée": {"10.0.0.254", "reserved"},
		"relais réservé":      {"10.0.0.253", "reserved"},
		"ip illisible":        {"10.0.0", "invalid ip"},
		"ipv6":                {"fd00::5", "outside"},
	}
	for label, c := range cases {
		err := CheckHostAddress("10.0.0.0/24", c.ip, "10.0.0.254", "10.0.0.253")
		if err == nil {
			t.Errorf("%s : %s devrait être refusé", label, c.ip)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s : erreur attendue contenant %q, obtenu %v", label, c.want, err)
		}
	}
}

func TestCheckHostAddress_EmptyReservedIgnored(t *testing.T) {
	if err := CheckHostAddress("10.0.0.0/24", "10.0.0.5", "", ""); err != nil {
		t.Errorf("des réservations vides ne doivent rien bloquer : %v", err)
	}
}

func TestCheckHostAddress_LenientCIDRWithHostBits(t *testing.T) {
	if err := CheckHostAddress("10.0.0.1/24", "10.0.0.5"); err != nil {
		t.Errorf("un cidr avec bits d'hôte doit être normalisé : %v", err)
	}
	if err := CheckHostAddress("10.0.0.1/24", "10.0.0.0"); err == nil {
		t.Error("l'adresse réseau du cidr normalisé doit rester refusée")
	}
}

func TestCheckHostAddress_Slash31And32HaveNoNetworkOrBroadcast(t *testing.T) {
	for _, ip := range []string{"10.0.0.0", "10.0.0.1"} {
		if err := CheckHostAddress("10.0.0.0/31", ip); err != nil {
			t.Errorf("/31 : %s devrait être accepté : %v", ip, err)
		}
	}
	if err := CheckHostAddress("10.0.0.7/32", "10.0.0.7"); err != nil {
		t.Errorf("/32 : 10.0.0.7 devrait être accepté : %v", err)
	}
	if err := CheckHostAddress("10.0.0.7/32", "10.0.0.8"); err == nil {
		t.Error("/32 : 10.0.0.8 est hors du cidr et devrait être refusé")
	}
}

func TestCheckHostAddress_InvalidCIDR(t *testing.T) {
	for _, cidr := range []string{"", "pas-un-cidr", "fd00::/64"} {
		if err := CheckHostAddress(cidr, "10.0.0.5"); err == nil {
			t.Errorf("cidr %q devrait être refusé", cidr)
		}
	}
}
