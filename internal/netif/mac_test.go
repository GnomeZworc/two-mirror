package netif

import (
	"net"
	"testing"
)

func TestHostMAC(t *testing.T) {
	cases := []struct {
		role byte
		vtep string
		want string
	}{
		{RoleHostGateway, "192.168.14.11", "02:03:c0:a8:0e:0b"},
		{RoleHostGateway, "192.168.14.12", "02:03:c0:a8:0e:0c"},
		{RoleHostGateway, "10.0.0.1", "02:03:0a:00:00:01"},
		{RoleRouter, "192.168.14.11", "02:02:c0:a8:0e:0b"},
	}
	for _, c := range cases {
		got, err := HostMAC(c.role, net.ParseIP(c.vtep))
		if err != nil {
			t.Fatalf("rôle %#x, VTEP %s : erreur inattendue %v", c.role, c.vtep, err)
		}
		if got.String() != c.want {
			t.Errorf("rôle %#x, VTEP %s : %s, %s attendu", c.role, c.vtep, got, c.want)
		}
	}
}

func TestHostMACIsLocalUnicast(t *testing.T) {
	for _, role := range []byte{RoleRouter, RoleHostGateway} {
		got, err := HostMAC(role, net.ParseIP("192.168.14.11"))
		if err != nil {
			t.Fatal(err)
		}
		if got[0]&0x02 == 0 {
			t.Errorf("%s : bit « administrée localement » absent", got)
		}
		if got[0]&0x01 != 0 {
			t.Errorf("%s : MAC de groupe (multicast)", got)
		}
	}
}

func TestHostMACRolesDifferOnTheSameHost(t *testing.T) {
	if RoleRouter == RoleHostGateway {
		t.Fatalf("rôles identiques : %#x", RoleRouter)
	}
	if RoleRouter != 0x02 || RoleHostGateway != 0x03 {
		t.Errorf("rôles %#x / %#x, 0x02 / 0x03 attendus (valeurs publiées dans la doc)", RoleRouter, RoleHostGateway)
	}
}

func TestHostMACRejectsANonIPv4VTEP(t *testing.T) {
	for _, vtep := range []net.IP{nil, net.ParseIP("fd00::1")} {
		if mac, err := HostMAC(RoleHostGateway, vtep); err == nil {
			t.Errorf("VTEP %v accepté : %s", vtep, mac)
		}
	}
}

func TestAnycastMAC(t *testing.T) {
	cases := map[int]string{
		1:        "02:01:00:00:00:01",
		2911:     "02:01:00:00:0b:5f",
		70000:    "02:01:00:01:11:70",
		MaxVNI:   "02:01:00:ff:ff:ff",
		0x123456: "02:01:00:12:34:56",
	}
	for vni, want := range cases {
		got, err := AnycastMAC(vni)
		if err != nil {
			t.Fatalf("VNI %d : erreur inattendue %v", vni, err)
		}
		if got.String() != want {
			t.Errorf("VNI %d : %s, %s attendu", vni, got, want)
		}
		if got[0]&0x02 == 0 || got[0]&0x01 != 0 {
			t.Errorf("VNI %d : %s n'est pas une MAC unicast administrée localement", vni, got)
		}
	}
}

func TestAnycastMACRefusesOutOfRangeVNI(t *testing.T) {
	for _, vni := range []int{0, -1, MaxVNI + 1} {
		if _, err := AnycastMAC(vni); err == nil {
			t.Errorf("VNI %d : une erreur était attendue", vni)
		}
	}
}

func TestAnycastMACDiffersFromHostMACs(t *testing.T) {
	anycast, _ := AnycastMAC(2911)
	for _, role := range []byte{RoleRouter, RoleHostGateway} {
		host, _ := HostMAC(role, net.ParseIP("192.168.14.11"))
		if host[1] == anycast[1] {
			t.Errorf("rôle %#x : même octet de rôle que la MAC anycast (%s, %s)", role, host, anycast)
		}
	}
}

func TestNATGatewayMAC(t *testing.T) {
	cases := map[int]string{
		1:      "02:04:00:00:00:01",
		2911:   "02:04:00:00:0b:5f",
		65535:  "02:04:00:00:ff:ff",
		MaxVNI: "02:04:00:ff:ff:ff",
	}
	for l3vni, want := range cases {
		got, err := NATGatewayMAC(l3vni)
		if err != nil {
			t.Fatalf("l3vni %d : erreur inattendue %v", l3vni, err)
		}
		if got.String() != want {
			t.Errorf("l3vni %d : %s, %s attendu", l3vni, got, want)
		}
		if got[0]&0x02 == 0 || got[0]&0x01 != 0 {
			t.Errorf("l3vni %d : %s n'est pas une MAC unicast administrée localement", l3vni, got)
		}
	}
}

func TestNATGatewayMACRefusesOutOfRangeVNI(t *testing.T) {
	for _, l3vni := range []int{0, -1, MaxVNI + 1} {
		if _, err := NATGatewayMAC(l3vni); err == nil {
			t.Errorf("l3vni %d : une erreur était attendue", l3vni)
		}
	}
}

func TestNATGatewayMACDiffersFromOtherRoles(t *testing.T) {
	nat, _ := NATGatewayMAC(2911)
	anycast, _ := AnycastMAC(2911)
	if nat.String() == anycast.String() {
		t.Errorf("même MAC que la gateway anycast de même VNI : %s", nat)
	}
	for _, role := range []byte{RoleRouter, RoleHostGateway} {
		host, _ := HostMAC(role, net.ParseIP("192.168.14.11"))
		if host[1] == nat[1] {
			t.Errorf("rôle %#x : même octet de rôle que la passerelle NAT (%s, %s)", role, host, nat)
		}
	}
}

func TestFloatingIPMAC(t *testing.T) {
	cases := map[string]string{
		"203.0.113.20": "02:05:cb:00:71:14",
		"240.0.0.47":   "02:05:f0:00:00:2f",
	}
	for ip, want := range cases {
		got, err := FloatingIPMAC(net.ParseIP(ip))
		if err != nil {
			t.Fatalf("%s : erreur inattendue %v", ip, err)
		}
		if got.String() != want {
			t.Errorf("%s : %s, %s attendu", ip, got, want)
		}
	}
	if mac, err := FloatingIPMAC(net.ParseIP("2001:db8::20")); err == nil {
		t.Errorf("IPv6 acceptée : %s", mac)
	}
}

func TestFloatingIPMACDiffersFromOtherRoles(t *testing.T) {
	fip, _ := FloatingIPMAC(net.ParseIP("192.168.14.11"))
	for _, role := range []byte{RoleAnycastGateway, RoleRouter, RoleHostGateway, RoleNATGateway} {
		if fip[1] == role {
			t.Errorf("rôle %#x : même octet de rôle que l'IP flottante (%s)", role, fip)
		}
	}
	if fip[1] != 0x05 {
		t.Errorf("%s : octet de rôle 0x05 attendu (valeur publiée dans la doc)", fip)
	}
}
