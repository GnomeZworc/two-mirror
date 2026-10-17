//go:build linux

package netif

import (
	"syscall"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestBridgeLocalEntry(t *testing.T) {
	mac, _ := AnycastMAC(2911)
	e := bridgeLocalEntry(7, mac)
	if e.LinkIndex != 7 || e.Family != syscall.AF_BRIDGE {
		t.Errorf("index ou famille inattendus : %d, %d", e.LinkIndex, e.Family)
	}
	if e.State != netlink.NUD_PERMANENT || e.Flags != netlink.NTF_SELF {
		t.Errorf("état %#x et drapeaux %#x, entrée permanente du bridge attendue", e.State, e.Flags)
	}
}

func TestHasBridgeLocalMAC(t *testing.T) {
	mac, _ := AnycastMAC(2911)
	other, _ := AnycastMAC(2912)
	local := netlink.Neigh{LinkIndex: 7, MasterIndex: 7, State: netlink.NUD_PERMANENT, HardwareAddr: mac}
	cases := []struct {
		label   string
		entries []netlink.Neigh
		want    bool
	}{
		{"entrée locale du bridge", []netlink.Neigh{local}, true},
		{"seulement la liste unicast du bridge (macvlan)", []netlink.Neigh{{LinkIndex: 7, State: netlink.NUD_PERMANENT, Flags: netlink.NTF_SELF, HardwareAddr: mac}}, false},
		{"apprise sur un port", []netlink.Neigh{{LinkIndex: 9, MasterIndex: 7, State: netlink.NUD_REACHABLE, HardwareAddr: mac}}, false},
		{"autre MAC", []netlink.Neigh{{LinkIndex: 7, MasterIndex: 7, State: netlink.NUD_PERMANENT, HardwareAddr: other}}, false},
		{"aucune entrée", nil, false},
	}
	for _, c := range cases {
		if got := hasBridgeLocalMAC(c.entries, 7, mac); got != c.want {
			t.Errorf("%s : %v, %v attendu", c.label, got, c.want)
		}
	}
}
