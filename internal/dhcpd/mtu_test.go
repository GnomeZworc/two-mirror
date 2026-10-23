package dhcpd

import (
	"os"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

func replyMTU(t *testing.T, c SubnetConfig) uint16 {
	t.Helper()
	h := testHost(t)
	reply, err := BuildReply(c, h, request(t, dhcpv4.MessageTypeRequest, h.MAC))
	if err != nil {
		t.Fatalf("BuildReply: %v", err)
	}
	m, err := dhcpv4.GetUint16(dhcpv4.OptionInterfaceMTU, reply.Options)
	if err != nil {
		t.Fatalf("option 26 absente : %v", err)
	}
	return m
}

func TestBuildReply_CarriesTheInterfaceMTU(t *testing.T) {
	c := testConfig(t)
	c.MTU = 1450
	if got := replyMTU(t, c); got != 1450 {
		t.Errorf("option 26 = %d, attendu 1450", got)
	}
}

func TestBuildReply_InterfaceMTUIs1500WhenUnset(t *testing.T) {
	if got := replyMTU(t, testConfig(t)); got != 1500 {
		t.Errorf("option 26 = %d, attendu 1500", got)
	}
}

func TestStore_MTUIsPersisted(t *testing.T) {
	s, path := loadedStore(t)
	c := fullConfig(t)
	c.MTU = 1450
	if err := s.SetSubnet(c); err != nil {
		t.Fatalf("SetSubnet: %v", err)
	}
	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, _ := reloaded.Subnet(); got.MTU != 1450 {
		t.Errorf("mtu relu = %d, attendu 1450", got.MTU)
	}
}

func TestStore_StateWithoutMTUIsServedAt1500(t *testing.T) {
	path := statePath(t)
	legacy := `{"subnet":{"network":"10.0.5.0/24","interface_ip":"10.0.5.1"},"hosts":[]}`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewStore(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c, _ := s.Subnet()
	if got := replyMTU(t, c); got != 1500 {
		t.Errorf("ancien état sans mtu : option 26 = %d, attendu 1500", got)
	}
}
