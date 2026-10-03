package topology

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"
)

const (
	MinMTU    = 1280
	MaxMTU    = 9000
	MinMemory = 256
	MaxPrefix = 30
	MinPrefix = 8

	ReservedInterface = "mgmt0"
)

var (
	namePattern    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,14}$`)
	segmentPattern = regexp.MustCompile(`^[a-z][a-z0-9]{0,11}$`)
)

func (t *Topology) Validate() error {
	var errs []error
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}

	if !namePattern.MatchString(t.Name) {
		add("name %q: must match %s", t.Name, namePattern)
	}

	images := map[string]bool{}
	for _, i := range t.Images {
		images[i.Name] = true
		if !strings.HasPrefix(i.URL, "https://") {
			add("image %s: url must be an https:// URL", i.Name)
		}
		if !strings.HasPrefix(i.Sums, "https://") {
			add("image %s: sums must be an https:// URL", i.Name)
		}
	}

	nodes := map[string]Node{}
	for _, n := range t.Nodes {
		nodes[n.Name] = n
	}
	if len(t.Nodes) == 0 {
		add("nodes: at least one node is required")
	}
	if len(t.Segments) > maxSegments {
		add("segments: %d declared, at most %d", len(t.Segments), maxSegments)
	}
	if len(t.Nodes) > maxNodes {
		add("nodes: %d declared, at most %d", len(t.Nodes), maxNodes)
	}

	segments := map[string]Segment{}
	for _, s := range t.Segments {
		segments[s.Name] = s
		if !segmentPattern.MatchString(s.Name) {
			add("segment %s: name must match %s", s.Name, segmentPattern)
		}
		if s.Name == ReservedInterface {
			add("segment %s: name is reserved for the administration interface", s.Name)
		}
		sw, ok := nodes[s.Switch]
		switch {
		case s.Switch == "":
			add("segment %s: switch is required", s.Name)
		case !ok:
			add("segment %s: switch %s is not a declared node", s.Name, s.Switch)
		case sw.Role != RoleSwitch:
			add("segment %s: %s is a %s, not a switch", s.Name, s.Switch, sw.Role)
		}
		if s.MTU < MinMTU || s.MTU > MaxMTU {
			add("segment %s: mtu %d out of range [%d, %d]", s.Name, s.MTU, MinMTU, MaxMTU)
		}
		prefix, err := netip.ParsePrefix(s.CIDR)
		switch {
		case err != nil:
			add("segment %s: cidr %q: %v", s.Name, s.CIDR, err)
		case !prefix.Addr().Is4():
			add("segment %s: cidr %s is not IPv4", s.Name, s.CIDR)
		case prefix.Masked() != prefix:
			add("segment %s: cidr %s has host bits set, network is %s", s.Name, s.CIDR, prefix.Masked())
		case prefix.Bits() < MinPrefix || prefix.Bits() > MaxPrefix:
			add("segment %s: cidr %s prefix length out of range [/%d, /%d]", s.Name, s.CIDR, MinPrefix, MaxPrefix)
		}
	}

	attached := map[string]int{}
	hosting := map[string]int{}
	for _, s := range t.Segments {
		hosting[s.Switch]++
	}
	for _, n := range t.Nodes {
		if !namePattern.MatchString(n.Name) {
			add("node %s: name must match %s", n.Name, namePattern)
		}
		switch n.Role {
		case RoleSwitch, RoleRR, RoleHypervisor:
		default:
			add("node %s: role %q must be one of %s, %s, %s", n.Name, n.Role, RoleSwitch, RoleRR, RoleHypervisor)
		}
		if !images[n.Image] {
			add("node %s: image %q is not declared", n.Name, n.Image)
		}
		if n.CPUs < 1 {
			add("node %s: cpus must be at least 1", n.Name)
		}
		if n.Memory < MinMemory {
			add("node %s: memory must be at least %d MiB", n.Name, MinMemory)
		}
		if n.Role == RoleSwitch {
			if len(n.Segments) > 0 || len(n.Addresses) > 0 {
				add("node %s: a switch carries its segments through segments.<name>.switch, not through segments or addresses", n.Name)
			}
			if hosting[n.Name] == 0 {
				add("node %s: switch carries no segment", n.Name)
			}
			continue
		}
		if len(n.Segments) == 0 {
			add("node %s: must be attached to at least one segment", n.Name)
		}
		seen := map[string]bool{}
		for _, name := range n.Segments {
			if seen[name] {
				add("node %s: segment %s listed twice", n.Name, name)
				continue
			}
			seen[name] = true
			if _, ok := segments[name]; !ok {
				add("node %s: segment %s is not declared", n.Name, name)
				continue
			}
			attached[name]++
		}
		for _, name := range sortedKeys(n.Addresses) {
			address := n.Addresses[name]
			if !seen[name] {
				add("node %s: address given for segment %s it is not attached to", n.Name, name)
				continue
			}
			if _, err := netip.ParseAddr(address); err != nil {
				add("node %s: address %q on %s: %v", n.Name, address, name, err)
			}
		}
	}

	for _, s := range t.Segments {
		if attached[s.Name] == 0 {
			add("segment %s: no node is attached", s.Name)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
