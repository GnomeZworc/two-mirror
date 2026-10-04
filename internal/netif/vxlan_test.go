package netif

import (
	"net"
	"strings"
	"testing"

	"github.com/vishvananda/netlink"
)

func addr(cidr string, scope, flags int) netlink.Addr {
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	network.IP = ip
	return netlink.Addr{IPNet: network, Scope: scope, Flags: flags}
}

func TestVtepAddress_PicksThePrimaryGlobalIPv4(t *testing.T) {
	addrs := []netlink.Addr{
		addr("127.0.0.1/8", 254, 0),
		addr("169.254.0.3/28", 0, 0x01),
		addr("192.168.14.11/24", 0, 0x80),
		addr("192.168.14.99/24", 0, 0x01),
	}
	got, err := vtepAddress("br-000000", addrs)
	if err != nil || got.String() != "192.168.14.11" {
		t.Errorf("vtepAddress = %v, %v, want 192.168.14.11", got, err)
	}
}

func TestVtepAddress_SkipsLinkScopeAndSecondaryAddresses(t *testing.T) {
	addrs := []netlink.Addr{
		addr("169.254.0.3/28", 253, 0),
		addr("192.168.14.99/24", 0, 0x01),
		addr("192.168.14.12/24", 0, 0),
	}
	got, err := vtepAddress("br-000000", addrs)
	if err != nil || got.String() != "192.168.14.12" {
		t.Errorf("vtepAddress = %v, %v, want 192.168.14.12", got, err)
	}
}

func TestVtepAddress_RefusesAnInterfaceWithoutUsableAddress(t *testing.T) {
	for name, addrs := range map[string][]netlink.Addr{
		"no address":     nil,
		"only secondary": {addr("192.168.14.99/24", 0, 0x01)},
		"only link":      {addr("169.254.0.3/28", 253, 0)},
		"only IPv6":      {addr("fd00::11/64", 0, 0)},
	} {
		_, err := vtepAddress("br-000000", addrs)
		if err == nil || !strings.Contains(err.Error(), "no primary global IPv4 address on br-000000") {
			t.Errorf("%s: error = %v", name, err)
		}
	}
}

func TestVtepAddress_ReturnsAFourByteAddress(t *testing.T) {
	got, err := vtepAddress("br-000000", []netlink.Addr{addr("192.168.14.11/24", 0, 0)})
	if err != nil || len(got) != 4 {
		t.Errorf("vtepAddress = %v (%d bytes), %v", got, len(got), err)
	}
}
