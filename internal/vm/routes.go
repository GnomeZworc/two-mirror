package vm

import (
	"fmt"
	"net"

	"github.com/dgraph-io/badger/v4"
)

type hostRoute struct {
	vpc string
	dst *net.IPNet
}

type routeFunc func(db *badger.DB, vpcName string, dst *net.IPNet) error

func hostRoutes(d vmData) ([]hostRoute, error) {
	var routes []hostRoute
	for _, n := range d.nics {
		if n.mode != "vxlan" {
			continue
		}
		ip := net.ParseIP(n.ip).To4()
		if ip == nil {
			return nil, fmt.Errorf("interface %d: %q is not an IPv4 address", n.index, n.ip)
		}
		routes = append(routes, hostRoute{vpc: n.vpcName, dst: &net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}})
	}
	return routes, nil
}

func applyHostRoutes(db *badger.DB, d vmData, apply routeFunc) error {
	routes, err := hostRoutes(d)
	if err != nil {
		return err
	}
	for _, r := range routes {
		if err := apply(db, r.vpc, r.dst); err != nil {
			return err
		}
	}
	return nil
}
