//go:build linux

package netif

import (
	"net"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestSourceRule(t *testing.T) {
	r := sourceRule(net.ParseIP("10.46.1.10"), nil, 170786058, 101)
	if r.Family != netlink.FAMILY_V4 || r.Src.String() != "10.46.1.10/32" || r.Dst != nil || r.Table != 170786058 || r.Priority != 101 {
		t.Errorf("règle %+v", r)
	}
	_, vpc, _ := net.ParseCIDR("10.46.0.0/16")
	toVPC := sourceRule(net.ParseIP("10.46.1.10"), vpc, 254, 100)
	if toVPC.Dst.String() != "10.46.0.0/16" || toVPC.Table != 254 || toVPC.SuppressPrefixlen != -1 {
		t.Errorf("règle vers le VPC %+v", toVPC)
	}
}
