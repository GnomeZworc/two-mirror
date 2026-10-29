//go:build linux

package netif

import (
	"net"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestSourceRule(t *testing.T) {
	r := sourceRule(net.ParseIP("10.46.1.10"), 170786058, 101, false)
	if r.Family != netlink.FAMILY_V4 || r.Src.String() != "10.46.1.10/32" || r.Table != 170786058 || r.Priority != 101 {
		t.Errorf("règle %+v", r)
	}
	if r.SuppressPrefixlen != -1 {
		t.Errorf("suppress_prefixlength %d sans suppression demandée, -1 attendu", r.SuppressPrefixlen)
	}
	main := sourceRule(net.ParseIP("10.46.1.10"), 254, 100, true)
	if main.SuppressPrefixlen != 0 || main.Table != 254 {
		t.Errorf("règle de la table main %+v", main)
	}
}
