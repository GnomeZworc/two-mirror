package frr

import (
	"reflect"
	"testing"
)

const fullRunning = `frr version 10.7.1
frr defaults traditional
hostname hv1
!
vrf vrf-a1b2
 vni 4001
exit-vrf
!
vrf vrf-a1b2c
 vni 4002
exit-vrf
!
router bgp 64600
 bgp router-id 192.168.14.11
 no bgp default ipv4-unicast
 neighbor fabric peer-group
 neighbor fabric remote-as 64600
 neighbor 10.255.255.1 peer-group fabric
 !
 address-family l2vpn evpn
  neighbor fabric activate
  advertise-all-vni
 exit-address-family
exit
!
router bgp 64600 vrf vrf-a1b2
 bgp router-id 192.168.14.11
 no bgp default ipv4-unicast
 !
 address-family ipv4 unicast
  redistribute kernel route-map EXPORT-a1b2
 exit-address-family
 !
 address-family l2vpn evpn
  advertise ipv4 unicast
  rd 192.168.14.11:4001
  route-target import 64600:4001
  route-target export 64600:4001
 exit-address-family
exit
!
router bgp 64600 vrf vrf-a1b2c
 bgp router-id 192.168.14.11
exit
!
ip prefix-list PL-a1b2 seq 10 permit 10.10.0.0/16 le 32
ip prefix-list PL-a1b2-default seq 10 permit 0.0.0.0/0
ip prefix-list PL-a1b2-transit seq 10 permit 169.254.0.2/32
ip prefix-list PL-a1b2c seq 10 permit 10.20.0.0/16 le 32
!
route-map EXPORT-a1b2 permit 10
 match ip address prefix-list PL-a1b2
exit
!
route-map EXPORT-a1b2 permit 15
 match ip address prefix-list PL-a1b2-default
 match ip next-hop prefix-list PL-a1b2-transit
exit
!
route-map EXPORT-a1b2 deny 20
exit
!
route-map EXPORT-a1b2c permit 10
exit
!
end
`

const otherOnlyRunning = `vrf vrf-a1b2
exit-vrf
!
vrf vrf-a1b2c
 vni 4002
exit-vrf
!
router bgp 64600 vrf vrf-a1b2c
exit
!
ip prefix-list PL-a1b2c seq 10 permit 10.20.0.0/16 le 32
!
route-map EXPORT-a1b2c permit 10
exit
!
end
`

func TestRoutingTeardownFollowsTheOrderFRRRequires(t *testing.T) {
	want := []step{
		{"router bgp 64600 vrf vrf-a1b2", "address-family ipv4 unicast", "no redistribute kernel"},
		{"vrf vrf-a1b2", "no vni 4001"},
		{"no router bgp 64600 vrf vrf-a1b2"},
		{"no route-map EXPORT-a1b2"},
		{"no ip prefix-list PL-a1b2"},
		{"no ip prefix-list PL-a1b2-default"},
		{"no ip prefix-list PL-a1b2-transit"},
	}
	if got := routingTeardown(labVRF(t), fullRunning); !reflect.DeepEqual(got, want) {
		t.Errorf("démontage :\n%q\nattendu :\n%q", got, want)
	}
}

func TestRoutingTeardownTakesTheASNOfTheInstanceFromFRR(t *testing.T) {
	want := []step{
		{"router bgp 64600 vrf vrf-a1b2", "address-family ipv4 unicast", "no redistribute kernel"},
		{"vrf vrf-a1b2", "no vni 4001"},
		{"no router bgp 64600 vrf vrf-a1b2"},
		{"no route-map EXPORT-a1b2"},
		{"no ip prefix-list PL-a1b2"},
		{"no ip prefix-list PL-a1b2-default"},
		{"no ip prefix-list PL-a1b2-transit"},
	}
	for _, asn := range []uint32{0, 64601} {
		v := labVRF(t)
		v.ASN = asn
		if got := routingTeardown(v, fullRunning); !reflect.DeepEqual(got, want) {
			t.Errorf("asn %d :\n%q\nattendu :\n%q", asn, got, want)
		}
	}
}

func TestRoutingTeardownIgnoresAVRFWhoseNameExtendsOurs(t *testing.T) {
	if got := routingTeardown(labVRF(t), otherOnlyRunning); got != nil {
		t.Errorf("étapes visant vrf-a1b2c : %q", got)
	}
}

func TestRoutingTeardownUsesTheVNIFoundInFRR(t *testing.T) {
	v := labVRF(t)
	v.L3VNI = 9999
	running := "vrf vrf-a1b2\n vni 4001\nexit-vrf\n"
	want := []step{{"vrf vrf-a1b2", "no vni 4001"}}
	if got := routingTeardown(v, running); !reflect.DeepEqual(got, want) {
		t.Errorf("démontage : %q, %q attendu", got, want)
	}
}

func TestRoutingTeardownSkipsWhatIsAbsent(t *testing.T) {
	running := "vrf vrf-a1b2\n vni 4001\nexit-vrf\n!\nip prefix-list PL-a1b2 seq 10 permit 10.10.0.0/16 le 32\n"
	want := []step{
		{"vrf vrf-a1b2", "no vni 4001"},
		{"no ip prefix-list PL-a1b2"},
	}
	if got := routingTeardown(labVRF(t), running); !reflect.DeepEqual(got, want) {
		t.Errorf("démontage :\n%q\nattendu :\n%q", got, want)
	}
}

func TestRoutingTeardownDoesNotReadAVNIOutsideTheVRFBlock(t *testing.T) {
	running := "vrf vrf-a1b2\nexit-vrf\n!\nvrf vrf-a1b2c\n vni 4002\nexit-vrf\n"
	if got := routingTeardown(labVRF(t), running); got != nil {
		t.Errorf("étapes : %q", got)
	}
}

func TestVRFTeardown(t *testing.T) {
	want := []step{{"no vrf vrf-a1b2"}}
	if got := vrfTeardown(labVRF(t), otherOnlyRunning); !reflect.DeepEqual(got, want) {
		t.Errorf("démontage : %q, %q attendu", got, want)
	}
	if got := vrfTeardown(labVRF(t), "vrf vrf-a1b2c\nexit-vrf\n"); got != nil {
		t.Errorf("étapes visant vrf-a1b2c : %q", got)
	}
}
