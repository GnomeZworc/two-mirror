package render

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"

	"git.g3e.fr/syonad/two/internal/lab/topology"
)

const (
	QEMUBinary = "qemu-system-x86_64"

	AdminInterface = topology.ReservedInterface
	AdminAddress   = "10.0.2.15/24"
	AdminGateway   = "10.0.2.2"
	AdminDNS       = "10.0.2.3"

	DiskFile    = "disk.qcow2"
	SeedFile    = "seed.iso"
	ConsoleFile = "console.log"
	QMPFile     = "qmp.sock"
	PIDFile     = "qemu.pid"
)

var Nameservers = []string{"1.1.1.1", "8.8.8.8"}

type Options struct {
	RunDir         string
	AuthorizedKeys []string
	FRR            map[string]string
	Agent          map[string]string
}

type Node struct {
	Name          string
	Dir           string
	QEMU          []string
	MetaData      []byte
	UserData      []byte
	NetworkConfig []byte
}

func Render(p *topology.Plan, o Options) ([]Node, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	for _, n := range p.Nodes {
		if _, ok := o.FRR[n.Name]; n.FRR != "" && !ok {
			return nil, fmt.Errorf("node %s: frr configuration %s was not read", n.Name, n.FRR)
		}
		if _, ok := o.Agent[n.Name]; n.Agent != "" && !ok {
			return nil, fmt.Errorf("node %s: agent configuration %s was not read", n.Name, n.Agent)
		}
	}
	var nodes []Node
	for index, n := range p.Nodes {
		dir := filepath.Join(o.RunDir, n.Name)
		meta, err := metaData(p, n)
		if err != nil {
			return nil, err
		}
		user, err := userData(p, n, o)
		if err != nil {
			return nil, err
		}
		network, err := networkConfig(p, n, index)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, Node{
			Name:          n.Name,
			Dir:           dir,
			QEMU:          qemuArgs(p, n, index, dir),
			MetaData:      meta,
			UserData:      user,
			NetworkConfig: network,
		})
	}
	return nodes, nil
}

func (o Options) validate() error {
	var errs []error
	if !filepath.IsAbs(o.RunDir) {
		errs = append(errs, fmt.Errorf("run dir %q must be an absolute path", o.RunDir))
	}
	if len(o.AuthorizedKeys) == 0 {
		errs = append(errs, errors.New("at least one authorized ssh key is required"))
	}
	for i, k := range o.AuthorizedKeys {
		if strings.ContainsAny(k, "\r\n") {
			errs = append(errs, fmt.Errorf("authorized key %d spans several lines", i+1))
			continue
		}
		fields := strings.Fields(k)
		if len(fields) < 2 || !validKeyType(fields[0]) {
			errs = append(errs, fmt.Errorf("authorized key %d is not an ssh public key", i+1))
		}
	}
	return errors.Join(errs...)
}

func validKeyType(t string) bool {
	return strings.HasPrefix(t, "ssh-") || strings.HasPrefix(t, "ecdsa-sha2-") || strings.HasPrefix(t, "sk-")
}

func adminMAC(index int) net.HardwareAddr {
	return net.HardwareAddr{0x02, 0x4d, byte(index >> 8), byte(index), 0x00, 0x00}
}

func nodeCables(p *topology.Plan, name string) []topology.Cable {
	var out []topology.Cable
	for _, c := range p.Cables {
		if c.Node == name {
			out = append(out, c)
		}
	}
	return out
}

func switchCables(p *topology.Plan, name string) []topology.Cable {
	var out []topology.Cable
	for _, c := range p.Cables {
		if c.Switch == name {
			out = append(out, c)
		}
	}
	return out
}

func switchSegments(p *topology.Plan, name string) []topology.SegmentPlan {
	var out []topology.SegmentPlan
	for _, s := range p.Segments {
		if s.Switch == name {
			out = append(out, s)
		}
	}
	return out
}
