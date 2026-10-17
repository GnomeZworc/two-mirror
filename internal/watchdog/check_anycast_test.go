package watchdog

import (
	"strings"
	"testing"
)

func TestGatewayProblems_Healthy(t *testing.T) {
	st := gatewayState{
		MAC:          "02:01:00:00:0b:5f",
		LocalFDB:     true,
		GatewayAddrs: []string{"10.0.0.254"},
		BridgeAddrs:  []string{"10.0.0.253"},
	}
	if p := gatewayProblems("gw-x", "br-x", st, "02:01:00:00:0b:5f", "10.0.0.254", "10.0.0.253"); len(p) != 0 {
		t.Errorf("aucun problème attendu, obtenu %v", p)
	}
}

func TestGatewayProblems_EachDefectIsReported(t *testing.T) {
	healthy := func() gatewayState {
		return gatewayState{
			MAC:          "02:01:00:00:0b:5f",
			LocalFDB:     true,
			GatewayAddrs: []string{"10.0.0.254"},
			BridgeAddrs:  []string{"10.0.0.253"},
		}
	}
	cases := []struct {
		label  string
		mutate func(*gatewayState)
		want   string
	}{
		{"MAC du macvlan changée", func(s *gatewayState) { s.MAC = "02:03:c0:a8:0e:0b" }, "expected anycast 02:01:00:00:0b:5f"},
		{"entrée FDB locale absente du bridge", func(s *gatewayState) { s.LocalFDB = false }, "no local fdb entry for 02:01:00:00:0b:5f"},
		{"gateway absente du macvlan", func(s *gatewayState) { s.GatewayAddrs = nil }, "lacks the gateway address 10.0.0.254"},
		{"adresse d'émission absente du bridge", func(s *gatewayState) { s.BridgeAddrs = nil }, "lacks the relay address 10.0.0.253"},
		{"gateway posée sur le bridge", func(s *gatewayState) { s.BridgeAddrs = append(s.BridgeAddrs, "10.0.0.254") }, "bypass the anycast mac"},
	}
	for _, c := range cases {
		st := healthy()
		c.mutate(&st)
		p := gatewayProblems("gw-x", "br-x", st, "02:01:00:00:0b:5f", "10.0.0.254", "10.0.0.253")
		if len(p) != 1 || !strings.Contains(p[0], c.want) {
			t.Errorf("%s : un problème contenant %q attendu, obtenu %v", c.label, c.want, p)
		}
	}
}

func TestCheckSubnets_LegacyVxlanHasNoAnycastCheck(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "br-000042", "vp-admin", modeVxlan)
	seedKV(t, db, prefixSubnet+"br-000042/vxlan_id", "42")
	r := &recorder{}

	if err := CheckSubnets(db, dnsmasqConfig(), newFakeUnits(), r); err != nil {
		t.Fatalf("erreur inattendue: %v", err)
	}
	for _, c := range r.calls {
		if strings.Contains(c.problem, "gw-") || strings.Contains(c.problem, "anycast") {
			t.Errorf("un subnet sans relay_ip ne doit rien signaler sur la gateway anycast : %q", c.problem)
		}
	}
}
