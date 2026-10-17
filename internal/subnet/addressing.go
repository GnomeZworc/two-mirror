package subnet

import (
	"fmt"
	"net/netip"
)

const MaxAnycastPrefix = 29

type Addresses struct {
	Prefix    netip.Prefix
	Network   netip.Addr
	Broadcast netip.Addr
	Gateway   netip.Addr
	Relay     netip.Addr
}

func ParsePrefix(cidr string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("invalid cidr %q", cidr)
	}
	if !prefix.Addr().Is4() {
		return netip.Prefix{}, fmt.Errorf("cidr %q is not ipv4", cidr)
	}
	if prefix.Masked() != prefix {
		return netip.Prefix{}, fmt.Errorf("cidr %q has host bits set, expected %s", cidr, prefix.Masked())
	}
	return prefix, nil
}

func broadcastOf(prefix netip.Prefix) netip.Addr {
	b := prefix.Addr().As4()
	host := uint32(1)<<(32-prefix.Bits()) - 1
	v := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]) | host
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

func AnycastAddresses(cidr string) (Addresses, error) {
	prefix, err := ParsePrefix(cidr)
	if err != nil {
		return Addresses{}, err
	}
	if prefix.Bits() > MaxAnycastPrefix {
		return Addresses{}, fmt.Errorf("cidr %q is too small for a vxlan subnet, at most /%d", cidr, MaxAnycastPrefix)
	}
	broadcast := broadcastOf(prefix)
	gateway := broadcast.Prev()
	return Addresses{
		Prefix:    prefix,
		Network:   prefix.Addr(),
		Broadcast: broadcast,
		Gateway:   gateway,
		Relay:     gateway.Prev(),
	}, nil
}

func CheckHostAddress(cidr, ip string, reserved ...string) error {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil || !prefix.Addr().Is4() {
		return fmt.Errorf("invalid ipv4 cidr %q", cidr)
	}
	prefix = prefix.Masked()
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return fmt.Errorf("invalid ip %q", ip)
	}
	if !prefix.Contains(addr) {
		return fmt.Errorf("ip %s is outside %s", addr, prefix)
	}
	if prefix.Bits() < 31 {
		if addr == prefix.Addr() {
			return fmt.Errorf("ip %s is the network address of %s", addr, prefix)
		}
		if addr == broadcastOf(prefix) {
			return fmt.Errorf("ip %s is the broadcast address of %s", addr, prefix)
		}
	}
	for _, r := range reserved {
		if r == "" {
			continue
		}
		if other, err := netip.ParseAddr(r); err == nil && other == addr {
			return fmt.Errorf("ip %s is reserved by the subnet", addr)
		}
	}
	return nil
}
