package dhcpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"sort"
)

func canonicalMAC(s string) (string, error) {
	mac, err := net.ParseMAC(s)
	if err != nil {
		return "", fmt.Errorf("invalid mac %q: %w", s, err)
	}
	return mac.String(), nil
}

func canonicalIP(s string) (string, error) {
	ip := net.ParseIP(s)
	if ip == nil {
		return "", fmt.Errorf("invalid ip %q", s)
	}
	return ip.String(), nil
}

func canonicalCIDR(s string) (string, error) {
	_, network, err := net.ParseCIDR(s)
	if err != nil {
		return "", fmt.Errorf("invalid cidr %q: %w", s, err)
	}
	return network.String(), nil
}

func SortHosts(hosts []Host) {
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].MAC < hosts[j].MAC })
}

func CanonicalSubnet(s Subnet) (Subnet, error) {
	network, err := canonicalCIDR(s.Network)
	if err != nil {
		return Subnet{}, err
	}
	interfaceIP, err := canonicalIP(s.InterfaceIP)
	if err != nil {
		return Subnet{}, err
	}

	out := Subnet{Network: network, InterfaceIP: interfaceIP}

	if s.VPCRoute != "" {
		if out.VPCRoute, err = canonicalCIDR(s.VPCRoute); err != nil {
			return Subnet{}, err
		}
	}
	if s.DefaultGateway != "" {
		if out.DefaultGateway, err = canonicalIP(s.DefaultGateway); err != nil {
			return Subnet{}, err
		}
	}
	return out, nil
}

func CanonicalHost(h Host) (Host, error) {
	mac, err := canonicalMAC(h.MAC)
	if err != nil {
		return Host{}, err
	}
	ip, err := canonicalIP(h.IP)
	if err != nil {
		return Host{}, err
	}
	return Host{MAC: mac, IP: ip, VM: h.VM, DefaultRoute: h.DefaultRoute}, nil
}

func Canonical(s State) (State, error) {
	out := State{Hosts: make([]Host, 0, len(s.Hosts))}

	if s.Subnet != nil {
		subnet, err := CanonicalSubnet(*s.Subnet)
		if err != nil {
			return State{}, err
		}
		out.Subnet = &subnet
	}

	seen := make(map[string]struct{}, len(s.Hosts))
	for _, h := range s.Hosts {
		host, err := CanonicalHost(h)
		if err != nil {
			return State{}, err
		}
		if _, dup := seen[host.MAC]; dup {
			return State{}, fmt.Errorf("duplicate mac %s", host.MAC)
		}
		seen[host.MAC] = struct{}{}
		out.Hosts = append(out.Hosts, host)
	}
	SortHosts(out.Hosts)

	return out, nil
}

func Digest(s State) (string, error) {
	canonical, err := Canonical(s)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode state: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
