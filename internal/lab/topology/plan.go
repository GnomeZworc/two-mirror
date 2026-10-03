package topology

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
)

const (
	SSHBasePort   = 2200
	CableBasePort = 20000

	maxNodes    = 1000
	maxSegments = 256
)

type Plan struct {
	Name     string
	Segments []SegmentPlan
	Nodes    []NodePlan
	Cables   []Cable
}

type SegmentPlan struct {
	Name    string
	Switch  string
	Bridge  string
	Network netip.Prefix
	Gateway netip.Addr
	MTU     int
}

type NodePlan struct {
	Name    string
	Role    string
	Image   string
	CPUs    int
	Memory  int
	SSHPort int
}

type Cable struct {
	Segment string
	MTU     int

	Node          string
	NodeInterface string
	NodeMAC       net.HardwareAddr
	NodeAddress   netip.Prefix
	NodePort      int

	Switch          string
	SwitchInterface string
	SwitchMAC       net.HardwareAddr
	SwitchPort      int
}

func Compute(t *Topology) (*Plan, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}

	p := &Plan{Name: t.Name}
	for i, n := range t.Nodes {
		p.Nodes = append(p.Nodes, NodePlan{
			Name:    n.Name,
			Role:    n.Role,
			Image:   n.Image,
			CPUs:    n.CPUs,
			Memory:  n.Memory,
			SSHPort: SSHBasePort + i,
		})
	}

	var errs []error
	for segIndex, s := range t.Segments {
		network := netip.MustParsePrefix(s.CIDR)
		gateway := network.Addr().Next()
		p.Segments = append(p.Segments, SegmentPlan{
			Name:    s.Name,
			Switch:  s.Switch,
			Bridge:  "br-" + s.Name,
			Network: network,
			Gateway: gateway,
			MTU:     s.MTU,
		})

		addresses, err := allocate(t, s, network, gateway)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		for nodeIndex, n := range t.Nodes {
			address, ok := addresses[n.Name]
			if !ok {
				continue
			}
			cable := len(p.Cables)
			p.Cables = append(p.Cables, Cable{
				Segment:         s.Name,
				MTU:             s.MTU,
				Node:            n.Name,
				NodeInterface:   s.Name,
				NodeMAC:         mac(nodeIndex, segIndex, 0),
				NodeAddress:     netip.PrefixFrom(address, network.Bits()),
				NodePort:        CableBasePort + 2*cable,
				Switch:          s.Switch,
				SwitchInterface: "p" + strconv.Itoa(cable),
				SwitchMAC:       mac(nodeIndex, segIndex, 1),
				SwitchPort:      CableBasePort + 2*cable + 1,
			})
		}
	}

	if last := CableBasePort + 2*len(p.Cables) - 1; last > 65535 {
		errs = append(errs, fmt.Errorf("cables: %d cables need udp ports up to %d, beyond 65535", len(p.Cables), last))
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return p, nil
}

func allocate(t *Topology, s Segment, network netip.Prefix, gateway netip.Addr) (map[string]netip.Addr, error) {
	broadcast := lastAddr(network)
	used := map[netip.Addr]string{gateway: s.Switch + " (gateway)"}
	result := map[string]netip.Addr{}
	var errs []error

	var auto []string
	for _, n := range t.Nodes {
		if !contains(n.Segments, s.Name) {
			continue
		}
		raw, ok := n.Addresses[s.Name]
		if !ok {
			auto = append(auto, n.Name)
			continue
		}
		addr := netip.MustParseAddr(raw)
		switch {
		case !network.Contains(addr):
			errs = append(errs, fmt.Errorf("node %s: address %s is outside segment %s (%s)", n.Name, addr, s.Name, network))
		case addr == network.Addr() || addr == broadcast:
			errs = append(errs, fmt.Errorf("node %s: address %s is the network or broadcast address of segment %s", n.Name, addr, s.Name))
		case used[addr] != "":
			errs = append(errs, fmt.Errorf("node %s: address %s on segment %s is already taken by %s", n.Name, addr, s.Name, used[addr]))
		default:
			used[addr] = n.Name
			result[n.Name] = addr
		}
	}

	next := gateway.Next()
	for _, name := range auto {
		for next != broadcast && used[next] != "" {
			next = next.Next()
		}
		if next == broadcast {
			errs = append(errs, fmt.Errorf("segment %s: no address left in %s for node %s", s.Name, network, name))
			break
		}
		used[next] = name
		result[name] = next
		next = next.Next()
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return result, nil
}

func lastAddr(network netip.Prefix) netip.Addr {
	a := network.Addr().As4()
	host := uint32(1)<<(32-network.Bits()) - 1
	v := (uint32(a[0])<<24 | uint32(a[1])<<16 | uint32(a[2])<<8 | uint32(a[3])) | host
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

func mac(node, segment, side int) net.HardwareAddr {
	return net.HardwareAddr{0x02, 0x4c, byte(node >> 8), byte(node), byte(segment), byte(side)}
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}
