package configuration

import (
	"fmt"
	"net"
)

type BGP struct {
	ASN             uint32   `mapstructure:"asn"`
	RouteReflectors []string `mapstructure:"route_reflectors"`
}

func (b BGP) Enabled() bool {
	return b.ASN != 0 || len(b.RouteReflectors) > 0
}

func (b BGP) Reflectors() ([]net.IP, error) {
	if b.ASN == 0 {
		return nil, fmt.Errorf("bgp.asn must be set")
	}
	if len(b.RouteReflectors) == 0 {
		return nil, fmt.Errorf("bgp.route_reflectors must list at least one address")
	}
	seen := make(map[string]bool, len(b.RouteReflectors))
	ips := make([]net.IP, 0, len(b.RouteReflectors))
	for _, rr := range b.RouteReflectors {
		ip := net.ParseIP(rr).To4()
		if ip == nil {
			return nil, fmt.Errorf("bgp.route_reflectors: %q is not an IPv4 address", rr)
		}
		if seen[ip.String()] {
			return nil, fmt.Errorf("bgp.route_reflectors: %s is listed twice", ip)
		}
		seen[ip.String()] = true
		ips = append(ips, ip)
	}
	return ips, nil
}
