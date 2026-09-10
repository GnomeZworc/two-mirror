package dhcpd

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"sync"

	"git.g3e.fr/syonad/two/pkg/db/statefile"
)

var (
	ErrNoMAC         = errors.New("host mac is required")
	ErrNotConfigured = errors.New("subnet is not configured")
)

type diskSubnet struct {
	Network        string `json:"network"`
	InterfaceIP    string `json:"interface_ip"`
	VPCRoute       string `json:"vpc_route,omitempty"`
	DefaultGateway string `json:"default_gateway,omitempty"`
}

type diskHost struct {
	MAC          string `json:"mac"`
	IP           string `json:"ip"`
	VM           string `json:"vm,omitempty"`
	DefaultRoute bool   `json:"default_route"`
}

type diskState struct {
	Subnet *diskSubnet `json:"subnet,omitempty"`
	Hosts  []diskHost  `json:"hosts"`
}

type Store struct {
	mu         sync.RWMutex
	file       *statefile.File[diskState]
	subnet     SubnetConfig
	configured bool
	hosts      map[string]Host
}

func NewStore(path string) *Store {
	return &Store{
		file:  statefile.New[diskState](path),
		hosts: make(map[string]Host),
	}
}

func (s *Store) Path() string {
	return s.file.Path()
}

func (s *Store) Load() error {
	state, err := s.file.Load()
	if err != nil {
		return err
	}

	subnet := SubnetConfig{}
	configured := false
	if state.Subnet != nil {
		parsed, err := subnetFromDisk(*state.Subnet)
		if err != nil {
			return err
		}
		subnet = parsed
		configured = true
	}

	hosts := make(map[string]Host, len(state.Hosts))
	for _, h := range state.Hosts {
		host, err := hostFromDisk(h)
		if err != nil {
			return err
		}
		hosts[host.MAC.String()] = host
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.subnet = subnet
	s.configured = configured
	s.hosts = hosts
	return nil
}

func subnetFromDisk(d diskSubnet) (SubnetConfig, error) {
	_, network, err := net.ParseCIDR(d.Network)
	if err != nil {
		return SubnetConfig{}, fmt.Errorf("invalid network %q: %w", d.Network, err)
	}
	interfaceIP := net.ParseIP(d.InterfaceIP)
	if interfaceIP == nil {
		return SubnetConfig{}, ErrNoInterfaceIP
	}

	c := SubnetConfig{Network: network, InterfaceIP: interfaceIP}

	if d.VPCRoute != "" {
		if _, c.VPCRoute, err = net.ParseCIDR(d.VPCRoute); err != nil {
			return SubnetConfig{}, fmt.Errorf("invalid vpc route %q: %w", d.VPCRoute, err)
		}
	}
	if d.DefaultGateway != "" {
		if c.DefaultGateway = net.ParseIP(d.DefaultGateway); c.DefaultGateway == nil {
			return SubnetConfig{}, fmt.Errorf("invalid default gateway %q", d.DefaultGateway)
		}
	}
	return c, nil
}

func hostFromDisk(d diskHost) (Host, error) {
	mac, err := net.ParseMAC(d.MAC)
	if err != nil {
		return Host{}, fmt.Errorf("invalid mac %q: %w", d.MAC, err)
	}
	ip := net.ParseIP(d.IP)
	if ip == nil {
		return Host{}, fmt.Errorf("invalid host ip %q", d.IP)
	}
	return Host{MAC: mac, IP: ip, VM: d.VM, DefaultRoute: d.DefaultRoute}, nil
}

func (s *Store) SetSubnet(c SubnetConfig) error {
	if c.Network == nil {
		return ErrNoNetwork
	}
	if c.InterfaceIP == nil {
		return ErrNoInterfaceIP
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	previous, wasConfigured := s.subnet, s.configured
	s.subnet, s.configured = c, true

	if err := s.persist(); err != nil {
		s.subnet, s.configured = previous, wasConfigured
		return err
	}
	return nil
}

func (s *Store) SetHost(h Host) error {
	if len(h.MAC) == 0 {
		return ErrNoMAC
	}
	if h.IP == nil {
		return ErrNoHostIP
	}

	key := h.MAC.String()

	s.mu.Lock()
	defer s.mu.Unlock()

	previous, existed := s.hosts[key]
	s.hosts[key] = h

	if err := s.persist(); err != nil {
		if existed {
			s.hosts[key] = previous
		} else {
			delete(s.hosts, key)
		}
		return err
	}
	return nil
}

func (s *Store) DelHost(mac net.HardwareAddr) error {
	if len(mac) == 0 {
		return ErrNoMAC
	}
	key := mac.String()

	s.mu.Lock()
	defer s.mu.Unlock()

	previous, existed := s.hosts[key]
	delete(s.hosts, key)

	if err := s.persist(); err != nil {
		if existed {
			s.hosts[key] = previous
		}
		return err
	}
	return nil
}

func (s *Store) Lookup(mac net.HardwareAddr) (Host, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	h, ok := s.hosts[mac.String()]
	return h, ok
}

func (s *Store) Subnet() (SubnetConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.subnet, s.configured
}

func (s *Store) Hosts() []Host {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.sortedHosts()
}

func (s *Store) sortedHosts() []Host {
	hosts := make([]Host, 0, len(s.hosts))
	for _, h := range s.hosts {
		hosts = append(hosts, h)
	}
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].MAC.String() < hosts[j].MAC.String() })
	return hosts
}

func (s *Store) persist() error {
	state := diskState{Hosts: make([]diskHost, 0, len(s.hosts))}

	if s.configured {
		sub := diskSubnet{
			Network:     s.subnet.Network.String(),
			InterfaceIP: s.subnet.InterfaceIP.String(),
		}
		if s.subnet.VPCRoute != nil {
			sub.VPCRoute = s.subnet.VPCRoute.String()
		}
		if s.subnet.DefaultGateway != nil {
			sub.DefaultGateway = s.subnet.DefaultGateway.String()
		}
		state.Subnet = &sub
	}

	for _, h := range s.sortedHosts() {
		state.Hosts = append(state.Hosts, diskHost{
			MAC:          h.MAC.String(),
			IP:           h.IP.String(),
			VM:           h.VM,
			DefaultRoute: h.DefaultRoute,
		})
	}

	return s.file.Save(state)
}
