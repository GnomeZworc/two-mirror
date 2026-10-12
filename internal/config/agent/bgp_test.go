package configuration

import (
	"strings"
	"testing"
)

func TestLoadConfig_BGPAbsentIsDisabled(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, "database:\n  path: /tmp/two\n"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.BGP.Enabled() {
		t.Errorf("bgp enabled without a bgp section: %+v", cfg.BGP)
	}
}

func TestLoadConfig_ReadsTheBGPSection(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, "bgp:\n  asn: 4200000000\n  route_reflectors: [10.255.255.1, 10.255.255.2]\n"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.BGP.Enabled() || cfg.BGP.ASN != 4200000000 {
		t.Fatalf("bgp = %+v", cfg.BGP)
	}
	rrs, err := cfg.BGP.Reflectors()
	if err != nil {
		t.Fatalf("Reflectors: %v", err)
	}
	if len(rrs) != 2 || rrs[0].String() != "10.255.255.1" || rrs[1].String() != "10.255.255.2" {
		t.Errorf("reflectors = %v", rrs)
	}
}

func TestBGPReflectors_RejectsAnIncompleteOrInvalidSection(t *testing.T) {
	cases := []struct {
		bgp  BGP
		want string
	}{
		{BGP{RouteReflectors: []string{"10.255.255.1"}}, "bgp.asn must be set"},
		{BGP{ASN: 64600}, "at least one address"},
		{BGP{ASN: 64600, RouteReflectors: []string{"rr1"}}, `"rr1" is not an IPv4 address`},
		{BGP{ASN: 64600, RouteReflectors: []string{"fd00::1"}}, `"fd00::1" is not an IPv4 address`},
		{BGP{ASN: 64600, RouteReflectors: []string{"10.255.255.1", "10.255.255.1"}}, "10.255.255.1 is listed twice"},
	}
	for _, c := range cases {
		if !c.bgp.Enabled() {
			t.Errorf("%+v: Enabled() = false", c.bgp)
		}
		if _, err := c.bgp.Reflectors(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: erreur %v, attendu %q", c.bgp, err, c.want)
		}
	}
}
