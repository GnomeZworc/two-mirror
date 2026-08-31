package dhcpd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

const (
	stateFileMode = 0o600
	stateDirMode  = 0o700
)

var (
	ErrNoMAC         = errors.New("host mac is required")
	ErrNotConfigured = errors.New("subnet is not configured")
)

type SubnetSnapshot struct {
	Network        string `json:"network"`
	InterfaceIP    string `json:"interface_ip"`
	VPCRoute       string `json:"vpc_route,omitempty"`
	DefaultGateway string `json:"default_gateway,omitempty"`
}

type HostSnapshot struct {
	MAC          string `json:"mac"`
	IP           string `json:"ip"`
	VM           string `json:"vm,omitempty"`
	DefaultRoute bool   `json:"default_route"`
}

type Snapshot struct {
	Subnet *SubnetSnapshot `json:"subnet,omitempty"`
	Hosts  []HostSnapshot  `json:"hosts"`
}

type Store struct {
	mu         sync.RWMutex
	path       string
	subnet     SubnetConfig
	configured bool
	hosts      map[string]Host
}

func NewStore(path string) *Store {
	return &Store{path: path, hosts: make(map[string]Host)}
}

func normalizeMAC(s string) (string, error) {
	mac, err := net.ParseMAC(s)
	if err != nil {
		return "", fmt.Errorf("invalid mac %q: %w", s, err)
	}
	return mac.String(), nil
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s.persist()
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", s.path, err)
	}

	var snap Snapshot
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &snap); err != nil {
			return fmt.Errorf("parse %s: %w", s.path, err)
		}
	}
	return s.apply(snap)
}

func (s *Store) apply(snap Snapshot) error {
	subnet := SubnetConfig{}
	configured := false
	if snap.Subnet != nil {
		parsed, err := parseSubnet(*snap.Subnet)
		if err != nil {
			return err
		}
		subnet = parsed
		configured = true
	}

	hosts := make(map[string]Host, len(snap.Hosts))
	for _, h := range snap.Hosts {
		host, err := parseHost(h)
		if err != nil {
			return err
		}
		hosts[host.MAC.String()] = host
	}

	s.subnet = subnet
	s.configured = configured
	s.hosts = hosts
	return nil
}

func parseSubnet(s SubnetSnapshot) (SubnetConfig, error) {
	_, network, err := net.ParseCIDR(s.Network)
	if err != nil {
		return SubnetConfig{}, fmt.Errorf("invalid network %q: %w", s.Network, err)
	}

	interfaceIP := net.ParseIP(s.InterfaceIP)
	if interfaceIP == nil {
		return SubnetConfig{}, ErrNoInterfaceIP
	}

	c := SubnetConfig{Network: network, InterfaceIP: interfaceIP}

	if s.VPCRoute != "" {
		_, vpcRoute, err := net.ParseCIDR(s.VPCRoute)
		if err != nil {
			return SubnetConfig{}, fmt.Errorf("invalid vpc route %q: %w", s.VPCRoute, err)
		}
		c.VPCRoute = vpcRoute
	}
	if s.DefaultGateway != "" {
		gw := net.ParseIP(s.DefaultGateway)
		if gw == nil {
			return SubnetConfig{}, fmt.Errorf("invalid default gateway %q", s.DefaultGateway)
		}
		c.DefaultGateway = gw
	}
	return c, nil
}

func parseHost(h HostSnapshot) (Host, error) {
	mac, err := net.ParseMAC(h.MAC)
	if err != nil {
		return Host{}, fmt.Errorf("invalid mac %q: %w", h.MAC, err)
	}
	ip := net.ParseIP(h.IP)
	if ip == nil {
		return Host{}, fmt.Errorf("invalid host ip %q", h.IP)
	}
	return Host{MAC: mac, IP: ip, VM: h.VM, DefaultRoute: h.DefaultRoute}, nil
}

func (s *Store) SetSubnet(snap SubnetSnapshot) error {
	c, err := parseSubnet(snap)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.subnet = c
	s.configured = true
	return s.persist()
}

func (s *Store) SetHost(snap HostSnapshot) error {
	if snap.MAC == "" {
		return ErrNoMAC
	}
	host, err := parseHost(snap)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.hosts[host.MAC.String()] = host
	return s.persist()
}

func (s *Store) DelHost(mac string) error {
	key, err := normalizeMAC(mac)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.hosts, key)
	return s.persist()
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

func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.snapshot()
}

func (s *Store) snapshot() Snapshot {
	snap := Snapshot{Hosts: make([]HostSnapshot, 0, len(s.hosts))}

	if s.configured {
		sub := SubnetSnapshot{
			Network:     s.subnet.Network.String(),
			InterfaceIP: s.subnet.InterfaceIP.String(),
		}
		if s.subnet.VPCRoute != nil {
			sub.VPCRoute = s.subnet.VPCRoute.String()
		}
		if s.subnet.DefaultGateway != nil {
			sub.DefaultGateway = s.subnet.DefaultGateway.String()
		}
		snap.Subnet = &sub
	}

	for _, h := range s.hosts {
		snap.Hosts = append(snap.Hosts, HostSnapshot{
			MAC:          h.MAC.String(),
			IP:           h.IP.String(),
			VM:           h.VM,
			DefaultRoute: h.DefaultRoute,
		})
	}
	sort.Slice(snap.Hosts, func(i, j int) bool { return snap.Hosts[i].MAC < snap.Hosts[j].MAC })

	return snap
}

func (s *Store) persist() error {
	raw, err := json.Marshal(s.snapshot())
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, stateDirMode); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(s.path)+".tmp")
	if err != nil {
		return fmt.Errorf("create temp state in %s: %w", dir, err)
	}
	defer os.Remove(tmp.Name())

	if err := tmp.Chmod(stateFileMode); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod %s: %w", tmp.Name(), err)
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp.Name(), err)
	}

	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmp.Name(), s.path, err)
	}
	return nil
}
