package frr

import (
	"net"
	"reflect"
	"strings"
	"testing"
)

func mustCIDR(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func labVRF(t *testing.T) VRF {
	return VRF{
		ASN:      64600,
		ID:       "a1b2",
		L3VNI:    4001,
		VTEP:     net.ParseIP("192.168.14.11"),
		Supernet: mustCIDR(t, "10.10.0.0/16"),
	}
}

func TestBaseCommands(t *testing.T) {
	b := Base{
		ASN:             64600,
		RouterID:        net.ParseIP("192.168.14.13"),
		RouteReflectors: []net.IP{net.ParseIP("10.255.255.1"), net.ParseIP("10.255.255.2")},
	}
	want := []string{
		"router bgp 64600",
		"bgp router-id 192.168.14.13",
		"no bgp default ipv4-unicast",
		"neighbor fabric peer-group",
		"neighbor fabric remote-as 64600",
		"neighbor fabric capability extended-nexthop",
		"neighbor 10.255.255.1 peer-group fabric",
		"neighbor 10.255.255.2 peer-group fabric",
		"address-family l2vpn evpn",
		"neighbor fabric activate",
		"advertise-all-vni",
		"exit-address-family",
		"exit",
	}
	if got := b.commands(); !reflect.DeepEqual(got, want) {
		t.Errorf("commandes de base :\n%s\nattendu :\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestVRFCommands(t *testing.T) {
	want := []string{
		"ip prefix-list PL-a1b2 seq 10 permit 10.10.0.0/16 le 32",
		"route-map EXPORT-a1b2 permit 10",
		"match ip address prefix-list PL-a1b2",
		"exit",
		"route-map EXPORT-a1b2 deny 20",
		"exit",
		"vrf vrf-a1b2",
		"vni 4001",
		"exit-vrf",
		"router bgp 64600 vrf vrf-a1b2",
		"bgp router-id 192.168.14.11",
		"no bgp default ipv4-unicast",
		"address-family ipv4 unicast",
		"redistribute kernel route-map EXPORT-a1b2",
		"exit-address-family",
		"address-family l2vpn evpn",
		"rd 192.168.14.11:4001",
		"route-target both 64600:4001",
		"advertise ipv4 unicast",
		"exit-address-family",
		"exit",
	}
	if got := labVRF(t).commands(); !reflect.DeepEqual(got, want) {
		t.Errorf("commandes de VRF :\n%s\nattendu :\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestBaseValidateRejects(t *testing.T) {
	valid := func() Base {
		return Base{ASN: 64600, RouterID: net.ParseIP("192.168.14.13"), RouteReflectors: []net.IP{net.ParseIP("10.255.255.1")}}
	}
	if err := valid().validate(); err != nil {
		t.Fatalf("base valide refusée : %v", err)
	}
	cases := map[string]struct {
		mutate func(*Base)
		msg    string
	}{
		"asn nul":          {func(b *Base) { b.ASN = 0 }, "asn must not be 0"},
		"router-id absent": {func(b *Base) { b.RouterID = nil }, "router-id <nil> is not an IPv4 address"},
		"router-id IPv6":   {func(b *Base) { b.RouterID = net.ParseIP("fd00::1") }, "router-id fd00::1 is not an IPv4 address"},
		"aucun RR":         {func(b *Base) { b.RouteReflectors = nil }, "at least one route reflector is required"},
		"RR IPv6":          {func(b *Base) { b.RouteReflectors = append(b.RouteReflectors, net.ParseIP("fd00::2")) }, "route reflector fd00::2 is not an IPv4 address"},
	}
	for name, c := range cases {
		b := valid()
		c.mutate(&b)
		if err := b.validate(); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s : erreur %v, « %s » attendu", name, err, c.msg)
		}
	}
}

func TestVRFValidateRejects(t *testing.T) {
	if err := labVRF(t).validate(); err != nil {
		t.Fatalf("VRF valide refusée : %v", err)
	}
	cases := map[string]struct {
		mutate func(*VRF)
		msg    string
	}{
		"asn nul":           {func(v *VRF) { v.ASN = 0 }, "asn must not be 0"},
		"id vide":           {func(v *VRF) { v.ID = "" }, "vrf id must not be empty"},
		"l3vni nul":         {func(v *VRF) { v.L3VNI = 0 }, "l3vni 0 does not fit in 24 bits"},
		"l3vni sur 25 bits": {func(v *VRF) { v.L3VNI = 16777216 }, "l3vni 16777216 does not fit in 24 bits"},
		"vtep absent":       {func(v *VRF) { v.VTEP = nil }, "vtep <nil> is not an IPv4 address"},
		"vtep IPv6":         {func(v *VRF) { v.VTEP = net.ParseIP("fd00::1") }, "vtep fd00::1 is not an IPv4 address"},
		"supernet absent":   {func(v *VRF) { v.Supernet = nil }, "supernet <nil> is not an IPv4 prefix"},
		"supernet IPv6":     {func(v *VRF) { v.Supernet = mustCIDR(t, "fd00::/48") }, "supernet fd00::/48 is not an IPv4 prefix"},
	}
	for name, c := range cases {
		v := labVRF(t)
		c.mutate(&v)
		if err := v.validate(); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s : erreur %v, « %s » attendu", name, err, c.msg)
		}
	}
}

func TestVRFAcceptsTheLargest24BitVNI(t *testing.T) {
	v := labVRF(t)
	v.L3VNI = 16777215
	if err := v.validate(); err != nil {
		t.Errorf("l3vni 16777215 refusée : %v", err)
	}
}
