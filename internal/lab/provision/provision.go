package provision

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"git.g3e.fr/syonad/two/internal/lab/render"
	"git.g3e.fr/syonad/two/internal/lab/topology"
)

const (
	KeyFile   = "lab_ed25519"
	KeyLabel  = "two-lab"
	DiskSize  = "20G"
	SeedLabel = "cidata"

	ArgsFile = "qemu.args"
)

var SeedFiles = []string{"user-data", "meta-data", "network-config"}

type Runner interface {
	Run(ctx context.Context, name string, args ...string) error
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) error {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

type Options struct {
	RunDir  string
	Fetcher Fetcher
	Runner  Runner
}

func Prepare(ctx context.Context, p *topology.Plan, o Options) ([]render.Node, error) {
	if !filepath.IsAbs(o.RunDir) {
		return nil, fmt.Errorf("run dir %q must be an absolute path", o.RunDir)
	}
	if err := os.MkdirAll(o.RunDir, 0o700); err != nil {
		return nil, err
	}

	images := map[string]string{}
	for _, n := range p.Nodes {
		if _, ok := images[n.Image]; ok {
			continue
		}
		img, ok := imageNamed(p, n.Image)
		if !ok {
			return nil, fmt.Errorf("node %s: image %q is not declared", n.Name, n.Image)
		}
		base, err := o.Fetcher.Image(ctx, img)
		if err != nil {
			return nil, err
		}
		images[n.Image] = base
	}

	key, err := EnsureKey(ctx, o.Runner, o.RunDir)
	if err != nil {
		return nil, err
	}
	frr, err := ReadFRR(p)
	if err != nil {
		return nil, err
	}
	agent, err := ReadAgent(p)
	if err != nil {
		return nil, err
	}
	nodes, err := render.Render(p, render.Options{RunDir: o.RunDir, AuthorizedKeys: []string{key}, FRR: frr, Agent: agent})
	if err != nil {
		return nil, err
	}
	for i, n := range nodes {
		if err := Stage(ctx, o.Runner, n, images[p.Nodes[i].Image]); err != nil {
			return nil, fmt.Errorf("node %s: %w", n.Name, err)
		}
	}
	return nodes, nil
}

func ReadFRR(p *topology.Plan) (map[string]string, error) {
	return readNodeFiles(p, func(n topology.NodePlan) string { return n.FRR })
}

func ReadAgent(p *topology.Plan) (map[string]string, error) {
	return readNodeFiles(p, func(n topology.NodePlan) string { return n.Agent })
}

func readNodeFiles(p *topology.Plan, path func(topology.NodePlan) string) (map[string]string, error) {
	files := map[string]string{}
	for _, n := range p.Nodes {
		if path(n) == "" {
			continue
		}
		data, err := os.ReadFile(path(n))
		if err != nil {
			return nil, fmt.Errorf("node %s: %w", n.Name, err)
		}
		files[n.Name] = string(data)
	}
	return files, nil
}

func EnsureKey(ctx context.Context, r Runner, dir string) (string, error) {
	private := filepath.Join(dir, KeyFile)
	if _, err := os.Stat(private); errors.Is(err, os.ErrNotExist) {
		if err := r.Run(ctx, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", KeyLabel, "-f", private); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	data, err := os.ReadFile(private + ".pub")
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(string(data))
	if key == "" || strings.ContainsAny(key, "\r\n") {
		return "", fmt.Errorf("%s.pub: not a single public key", private)
	}
	return key, nil
}

func WriteFiles(n render.Node) error {
	if err := os.MkdirAll(n.Dir, 0o700); err != nil {
		return err
	}
	files := map[string][]byte{
		ArgsFile:         []byte(strings.Join(n.QEMU, "\n") + "\n"),
		"meta-data":      n.MetaData,
		"user-data":      n.UserData,
		"network-config": n.NetworkConfig,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(n.Dir, name), content, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func Stage(ctx context.Context, r Runner, n render.Node, base string) error {
	if !filepath.IsAbs(base) {
		return fmt.Errorf("base image %q must be an absolute path", base)
	}
	if err := WriteFiles(n); err != nil {
		return err
	}
	disk := filepath.Join(n.Dir, render.DiskFile)
	seed := filepath.Join(n.Dir, render.SeedFile)
	if err := r.Run(ctx, "qemu-img", "create", "-q", "-f", "qcow2", "-F", "qcow2", "-b", base, disk, DiskSize); err != nil {
		return err
	}
	args := []string{"-quiet", "-output", seed, "-volid", SeedLabel, "-joliet", "-rock"}
	for _, f := range SeedFiles {
		args = append(args, filepath.Join(n.Dir, f))
	}
	return r.Run(ctx, "genisoimage", args...)
}

func imageNamed(p *topology.Plan, name string) (topology.Image, bool) {
	for _, i := range p.Images {
		if i.Name == name {
			return i, true
		}
	}
	return topology.Image{}, false
}
