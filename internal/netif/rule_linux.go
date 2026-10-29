//go:build linux

package netif

import (
	"errors"
	"net"
	"syscall"

	"github.com/vishvananda/netlink"
)

func sourceRule(src net.IP, table, priority int, suppressDefault bool) *netlink.Rule {
	r := netlink.NewRule()
	r.Family = netlink.FAMILY_V4
	r.Src = &net.IPNet{IP: src.To4(), Mask: net.CIDRMask(32, 32)}
	r.Table = table
	r.Priority = priority
	if suppressDefault {
		r.SuppressPrefixlen = 0
	}
	return r
}

func RuleAddFrom(src net.IP, table, priority int, suppressDefault bool) error {
	return netlink.RuleAdd(sourceRule(src, table, priority, suppressDefault))
}

func RuleDelFromIfExists(src net.IP, table, priority int, suppressDefault bool) error {
	if err := netlink.RuleDel(sourceRule(src, table, priority, suppressDefault)); err != nil && !errors.Is(err, syscall.ENOENT) {
		return err
	}
	return nil
}
