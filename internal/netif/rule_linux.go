//go:build linux

package netif

import (
	"errors"
	"net"
	"syscall"

	"github.com/vishvananda/netlink"
)

func sourceRule(src net.IP, dst *net.IPNet, table, priority int) *netlink.Rule {
	r := netlink.NewRule()
	r.Family = netlink.FAMILY_V4
	r.Src = &net.IPNet{IP: src.To4(), Mask: net.CIDRMask(32, 32)}
	r.Dst = dst
	r.Table = table
	r.Priority = priority
	return r
}

func RuleAddFrom(src net.IP, dst *net.IPNet, table, priority int) error {
	return netlink.RuleAdd(sourceRule(src, dst, table, priority))
}

func RuleDelFromIfExists(src net.IP, dst *net.IPNet, table, priority int) error {
	if err := netlink.RuleDel(sourceRule(src, dst, table, priority)); err != nil && !errors.Is(err, syscall.ENOENT) {
		return err
	}
	return nil
}
