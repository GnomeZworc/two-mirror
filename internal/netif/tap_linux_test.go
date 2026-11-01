//go:build linux

package netif

import (
	"net"
	"syscall"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestTapMACEntry(t *testing.T) {
	mac, _ := net.ParseMAC("52:54:00:34:01:32")
	e := tapMACEntry(7, mac)
	if e.LinkIndex != 7 || e.Family != syscall.AF_BRIDGE {
		t.Errorf("index ou famille inattendus : %d, %d", e.LinkIndex, e.Family)
	}
	if e.State != 0x40 || e.Flags != 0x04|0x40 {
		t.Errorf("état %#x et drapeaux %#x, entrée static (NUD_NOARP) master sticky attendue", e.State, e.Flags)
	}
}

func TestHasTapMAC(t *testing.T) {
	mac, _ := net.ParseMAC("52:54:00:34:01:32")
	other, _ := net.ParseMAC("52:54:00:34:01:33")
	pinned := netlink.Neigh{LinkIndex: 7, MasterIndex: 3, State: netlink.NUD_NOARP, Flags: netlink.NTF_MASTER | netlink.NTF_STICKY, HardwareAddr: mac}
	cases := []struct {
		label   string
		entries []netlink.Neigh
		want    bool
	}{
		{"static sticky sur le tap", []netlink.Neigh{pinned}, true},
		{"static sans sticky", []netlink.Neigh{{LinkIndex: 7, MasterIndex: 3, State: netlink.NUD_NOARP, Flags: netlink.NTF_MASTER, HardwareAddr: mac}}, false},
		{"apprise", []netlink.Neigh{{LinkIndex: 7, MasterIndex: 3, State: netlink.NUD_REACHABLE, Flags: netlink.NTF_MASTER, HardwareAddr: mac}}, false},
		{"seulement l'entrée du vlan 1", []netlink.Neigh{{LinkIndex: 7, MasterIndex: 3, Vlan: 1, State: netlink.NUD_NOARP, Flags: netlink.NTF_MASTER | netlink.NTF_STICKY, HardwareAddr: mac}}, false},
		{"déplacée sur un autre port", []netlink.Neigh{{LinkIndex: 9, MasterIndex: 3, State: netlink.NUD_REACHABLE, Flags: netlink.NTF_MASTER, HardwareAddr: mac}}, false},
		{"liste propre du tap, hors pont", []netlink.Neigh{{LinkIndex: 7, State: netlink.NUD_NOARP, Flags: netlink.NTF_SELF | netlink.NTF_STICKY, HardwareAddr: mac}}, false},
		{"autre MAC", []netlink.Neigh{{LinkIndex: 7, MasterIndex: 3, State: netlink.NUD_NOARP, Flags: netlink.NTF_MASTER | netlink.NTF_STICKY, HardwareAddr: other}}, false},
		{"aucune entrée", nil, false},
	}
	for _, c := range cases {
		if got := hasTapMAC(c.entries, 7, mac); got != c.want {
			t.Errorf("%s : %v, %v attendu", c.label, got, c.want)
		}
	}
}
