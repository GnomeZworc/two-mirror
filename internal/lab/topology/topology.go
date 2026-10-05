package topology

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

const (
	RoleSwitch     = "switch"
	RoleRR         = "rr"
	RoleHypervisor = "hypervisor"

	DefaultMTU = 9000
)

type Topology struct {
	Name     string
	Images   []Image
	Segments []Segment
	Nodes    []Node
}

type Image struct {
	Name string
	URL  string
	Sums string
}

type Segment struct {
	Name   string
	Switch string
	CIDR   string
	MTU    int
}

type Node struct {
	Name      string
	Role      string
	Image     string
	CPUs      int
	Memory    int
	Segments  []string
	Addresses map[string]string
	Secondary map[string][]string
	Loopback  string
	FRR       string
	Release   string
	Agent     string
}

type fileImage struct {
	URL  string `yaml:"url"`
	Sums string `yaml:"sums"`
}

type fileSegment struct {
	Switch string `yaml:"switch"`
	CIDR   string `yaml:"cidr"`
	MTU    int    `yaml:"mtu"`
}

type fileNode struct {
	Role      string              `yaml:"role"`
	Image     string              `yaml:"image"`
	CPUs      int                 `yaml:"cpus"`
	Memory    int                 `yaml:"memory"`
	Segments  []string            `yaml:"segments"`
	Addresses map[string]string   `yaml:"addresses"`
	Secondary map[string][]string `yaml:"secondary"`
	Loopback  string              `yaml:"loopback"`
	FRR       string              `yaml:"frr"`
	Release   string              `yaml:"release"`
	Agent     string              `yaml:"agent"`
}

type file struct {
	Name     string                 `yaml:"name"`
	Images   map[string]fileImage   `yaml:"images"`
	Segments map[string]fileSegment `yaml:"segments"`
	Nodes    map[string]fileNode    `yaml:"nodes"`
}

func Load(path string) (*Topology, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	t, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for i, n := range t.Nodes {
		if n.FRR != "" && !filepath.IsAbs(n.FRR) {
			t.Nodes[i].FRR = filepath.Join(filepath.Dir(path), n.FRR)
		}
		if n.Agent != "" && !filepath.IsAbs(n.Agent) {
			t.Nodes[i].Agent = filepath.Join(filepath.Dir(path), n.Agent)
		}
	}
	return t, nil
}

func Parse(data []byte) (*Topology, error) {
	var f file
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	order, err := keyOrder(&root)
	if err != nil {
		return nil, err
	}

	t := &Topology{Name: f.Name}
	for _, name := range order["images"] {
		i := f.Images[name]
		t.Images = append(t.Images, Image{Name: name, URL: i.URL, Sums: i.Sums})
	}
	for _, name := range order["segments"] {
		s := f.Segments[name]
		mtu := s.MTU
		if mtu == 0 {
			mtu = DefaultMTU
		}
		t.Segments = append(t.Segments, Segment{Name: name, Switch: s.Switch, CIDR: s.CIDR, MTU: mtu})
	}
	for _, name := range order["nodes"] {
		n := f.Nodes[name]
		t.Nodes = append(t.Nodes, Node{
			Name:      name,
			Role:      n.Role,
			Image:     n.Image,
			CPUs:      n.CPUs,
			Memory:    n.Memory,
			Segments:  n.Segments,
			Addresses: n.Addresses,
			Secondary: n.Secondary,
			Loopback:  n.Loopback,
			FRR:       n.FRR,
			Release:   n.Release,
			Agent:     n.Agent,
		})
	}
	return t, nil
}

func keyOrder(root *yaml.Node) (map[string][]string, error) {
	order := map[string][]string{}
	if root.Kind != yaml.DocumentNode || len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("yaml: the document must be a mapping")
	}
	top := root.Content[0]
	for i := 0; i+1 < len(top.Content); i += 2 {
		section := top.Content[i].Value
		value := top.Content[i+1]
		if value.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(value.Content); j += 2 {
			order[section] = append(order[section], value.Content[j].Value)
		}
	}
	return order, nil
}
