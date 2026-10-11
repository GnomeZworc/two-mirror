package subnet

import (
	"net"
	"testing"
)

func TestHostGatewayMAC(t *testing.T) {
	cases := map[string]string{
		"192.168.14.11": "02:03:c0:a8:0e:0b",
		"192.168.14.12": "02:03:c0:a8:0e:0c",
		"10.0.0.1":      "02:03:0a:00:00:01",
	}
	for vtep, want := range cases {
		got, err := hostGatewayMAC(net.ParseIP(vtep))
		if err != nil {
			t.Fatalf("VTEP %s : erreur inattendue %v", vtep, err)
		}
		if got.String() != want {
			t.Errorf("VTEP %s : %s, %s attendu", vtep, got, want)
		}
	}
}

func TestHostGatewayMACIsLocalUnicast(t *testing.T) {
	got, err := hostGatewayMAC(net.ParseIP("192.168.14.11"))
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

func TestHostGatewayMACDiffersFromTheRouterMACOfTheSameHost(t *testing.T) {
	got, _ := hostGatewayMAC(net.ParseIP("192.168.14.11"))
	if got.String() == "02:02:c0:a8:0e:0b" {
		t.Errorf("%s : même MAC que la RMAC du L3VNI de cet hôte", got)
	}
}

func TestHostGatewayMACRejectsANonIPv4VTEP(t *testing.T) {
	for _, vtep := range []net.IP{nil, net.ParseIP("fd00::1")} {
		if mac, err := hostGatewayMAC(vtep); err == nil {
			t.Errorf("VTEP %v accepté : %s", vtep, mac)
		}
	}
}
