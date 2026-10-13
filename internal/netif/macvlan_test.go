package netif

import (
	"net"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestMacvlanLink(t *testing.T) {
	mac, _ := AnycastMAC(2911)
	link := macvlanLink("gw-sn1", 7, mac)
	if link.Name != "gw-sn1" || link.ParentIndex != 7 {
		t.Errorf("nom ou parent inattendu : %s, %d", link.Name, link.ParentIndex)
	}
	if link.HardwareAddr.String() != "02:01:00:00:0b:5f" {
		t.Errorf("MAC %s, 02:01:00:00:0b:5f attendue", link.HardwareAddr)
	}
	if link.Mode != netlink.MACVLAN_MODE_BRIDGE {
		t.Errorf("mode %v, bridge attendu", link.Mode)
	}
	if link.Flags&net.FlagUp != 0 {
		t.Error("le macvlan doit être créé éteint")
	}
}
