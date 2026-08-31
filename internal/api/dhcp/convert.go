package dhcpapi

import (
	"fmt"
	"net"

	"git.g3e.fr/syonad/two/internal/dhcpd"
)

func (s Subnet) toConfig() (dhcpd.SubnetConfig, error) {
	_, network, err := net.ParseCIDR(s.Network)
	if err != nil {
		return dhcpd.SubnetConfig{}, fmt.Errorf("invalid network %q: %w", s.Network, err)
	}
	interfaceIP := net.ParseIP(s.InterfaceIP)
	if interfaceIP == nil {
		return dhcpd.SubnetConfig{}, fmt.Errorf("invalid interface ip %q", s.InterfaceIP)
	}

	c := dhcpd.SubnetConfig{Network: network, InterfaceIP: interfaceIP}

	if s.VPCRoute != "" {
		if _, c.VPCRoute, err = net.ParseCIDR(s.VPCRoute); err != nil {
			return dhcpd.SubnetConfig{}, fmt.Errorf("invalid vpc route %q: %w", s.VPCRoute, err)
		}
	}
	if s.DefaultGateway != "" {
		if c.DefaultGateway = net.ParseIP(s.DefaultGateway); c.DefaultGateway == nil {
			return dhcpd.SubnetConfig{}, fmt.Errorf("invalid default gateway %q", s.DefaultGateway)
		}
	}
	return c, nil
}

func (h Host) toHost() (dhcpd.Host, error) {
	mac, err := net.ParseMAC(h.MAC)
	if err != nil {
		return dhcpd.Host{}, fmt.Errorf("invalid mac %q: %w", h.MAC, err)
	}
	ip := net.ParseIP(h.IP)
	if ip == nil {
		return dhcpd.Host{}, fmt.Errorf("invalid host ip %q", h.IP)
	}
	return dhcpd.Host{MAC: mac, IP: ip, VM: h.VM, DefaultRoute: h.DefaultRoute}, nil
}

func subnetFromConfig(c dhcpd.SubnetConfig) Subnet {
	s := Subnet{
		Network:     c.Network.String(),
		InterfaceIP: c.InterfaceIP.String(),
	}
	if c.VPCRoute != nil {
		s.VPCRoute = c.VPCRoute.String()
	}
	if c.DefaultGateway != nil {
		s.DefaultGateway = c.DefaultGateway.String()
	}
	return s
}

func hostFromHost(h dhcpd.Host) Host {
	return Host{
		MAC:          h.MAC.String(),
		IP:           h.IP.String(),
		VM:           h.VM,
		DefaultRoute: h.DefaultRoute,
	}
}

func stateFromStore(store *dhcpd.Store) State {
	state := State{Hosts: make([]Host, 0)}

	if config, configured := store.Subnet(); configured {
		subnet := subnetFromConfig(config)
		state.Subnet = &subnet
	}
	for _, h := range store.Hosts() {
		state.Hosts = append(state.Hosts, hostFromHost(h))
	}
	SortHosts(state.Hosts)

	return state
}
