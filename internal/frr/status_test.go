package frr

import (
	"net"
	"strings"
	"testing"
)

const vniOutput = `VNI: 2700
  Type: L3
  Tenant VRF: vrf-s7
  Vxlan-Intf: vxl3-s7
  SVI-If: brl3-s7
  State: Up
  VNI Filter: none
  System MAC: 02:02:c0:a8:0e:aa
  Router MAC: 02:02:C0:A8:0E:0B
  L2 VNIs: 
`

func TestVNIReadsTypeStateAndRouterMAC(t *testing.T) {
	f := &fakeVtysh{fail: map[string]string{"show evpn vni 2700": vniOutput}}
	s, err := f.client().VNI(2700)
	if err != nil {
		t.Fatal(err)
	}
	want := VNIState{Type: "L3", State: "Up", RouterMAC: "02:02:c0:a8:0e:0b"}
	if s != want {
		t.Errorf("VNI = %+v, attendu %+v", s, want)
	}
}

func TestVNIReportsAnUnknownVNI(t *testing.T) {
	f := &fakeVtysh{fail: map[string]string{"show evpn vni 9": "% VNI 9 does not exist\n"}}
	if _, err := f.client().VNI(9); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("erreur %v", err)
	}
}

func TestNeighborEstablished(t *testing.T) {
	rr := net.ParseIP("10.255.255.1")
	f := &fakeVtysh{fail: map[string]string{"show bgp neighbors 10.255.255.1": "BGP neighbor is 10.255.255.1, remote AS 64600, local AS 64600, internal link\n  BGP version 4, remote router ID 10.255.255.1\n  BGP state = Established, up for 00:10:02\n"}}
	if up, err := f.client().NeighborEstablished(rr); err != nil || !up {
		t.Errorf("établie : %v, %v", up, err)
	}
	f = &fakeVtysh{fail: map[string]string{"show bgp neighbors 10.255.255.1": "BGP neighbor is 10.255.255.1\n  BGP state = Active\n"}}
	if up, err := f.client().NeighborEstablished(rr); err != nil || up {
		t.Errorf("active : %v, %v", up, err)
	}
}
