package subnet

import "testing"

func TestAnycastGatewayMAC(t *testing.T) {
	cases := []struct {
		vni  int
		want string
	}{
		{2811, "02:01:00:00:0a:fb"},
		{0, "02:01:00:00:00:00"},
		{1, "02:01:00:00:00:01"},
		{0x123456, "02:01:00:12:34:56"},
		{maxVNI, "02:01:00:ff:ff:ff"},
	}
	for _, c := range cases {
		got, err := anycastGatewayMAC(c.vni)
		if err != nil {
			t.Fatalf("VNI %d : erreur inattendue %v", c.vni, err)
		}
		if got.String() != c.want {
			t.Errorf("VNI %d : %s, %s attendu", c.vni, got, c.want)
		}
	}
}

func TestAnycastGatewayMACIsLocalUnicast(t *testing.T) {
	got, err := anycastGatewayMAC(2811)
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

func TestAnycastGatewayMACIsIdenticalForTheSameVNIAndDistinctOtherwise(t *testing.T) {
	a, _ := anycastGatewayMAC(2811)
	b, _ := anycastGatewayMAC(2811)
	c, _ := anycastGatewayMAC(2812)
	if a.String() != b.String() {
		t.Errorf("même VNI, MAC différentes : %s et %s", a, b)
	}
	if a.String() == c.String() {
		t.Errorf("VNI différentes, même MAC : %s", a)
	}
}

func TestAnycastGatewayMACRejectsVNIOutside24Bits(t *testing.T) {
	for _, vni := range []int{-1, maxVNI + 1} {
		if mac, err := anycastGatewayMAC(vni); err == nil {
			t.Errorf("VNI %d accepté : %s", vni, mac)
		}
	}
}
