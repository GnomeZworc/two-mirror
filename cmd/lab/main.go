package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"git.g3e.fr/syonad/two/internal/lab/render"
	"git.g3e.fr/syonad/two/internal/lab/topology"
)

const usage = `usage: lab <command> [options] <topology.yml> [dir]

  plan <topology.yml>
      validate the topology and print the deterministic plan: addresses, cables, ports

  render -key <public key file> <topology.yml> <dir>
      write, for each node, <dir>/<node>/qemu.args (one argument per line) and the
      cloud-init seed files meta-data, user-data and network-config
`

type keyFiles []string

func (k *keyFiles) String() string     { return strings.Join(*k, ",") }
func (k *keyFiles) Set(v string) error { *k = append(*k, v); return nil }

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "plan":
		if len(args) != 2 {
			fmt.Fprint(stderr, usage)
			return 2
		}
		return plan(args[1], stdout, stderr)
	case "render":
		return renderCmd(args[1:], stdout, stderr)
	default:
		fmt.Fprint(stderr, usage)
		return 2
	}
}

func load(path string, stderr io.Writer) (*topology.Plan, bool) {
	t, err := topology.Load(path)
	if err != nil {
		fmt.Fprintf(stderr, "lab: %v\n", err)
		return nil, false
	}
	p, err := topology.Compute(t)
	if err != nil {
		fmt.Fprintf(stderr, "lab: %s:\n%v\n", path, err)
		return nil, false
	}
	return p, true
}

func plan(path string, stdout, stderr io.Writer) int {
	p, ok := load(path, stderr)
	if !ok {
		return 1
	}
	if err := p.Write(stdout); err != nil {
		fmt.Fprintf(stderr, "lab: %v\n", err)
		return 1
	}
	return 0
}

func renderCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	var keys keyFiles
	fs.Var(&keys, "key", "public key file allowed to log in, repeatable")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 2 || len(keys) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	p, ok := load(fs.Arg(0), stderr)
	if !ok {
		return 1
	}
	dir, err := filepath.Abs(fs.Arg(1))
	if err != nil {
		fmt.Fprintf(stderr, "lab: %v\n", err)
		return 1
	}
	authorized, err := readKeys(keys)
	if err != nil {
		fmt.Fprintf(stderr, "lab: %v\n", err)
		return 1
	}
	nodes, err := render.Render(p, render.Options{RunDir: dir, AuthorizedKeys: authorized})
	if err != nil {
		fmt.Fprintf(stderr, "lab: %v\n", err)
		return 1
	}
	for _, n := range nodes {
		if err := writeNode(n); err != nil {
			fmt.Fprintf(stderr, "lab: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s\n", n.Dir)
	}
	return 0
}

func readKeys(files []string) ([]string, error) {
	var keys []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			keys = append(keys, line)
		}
	}
	return keys, nil
}

func writeNode(n render.Node) error {
	if err := os.MkdirAll(n.Dir, 0o700); err != nil {
		return err
	}
	files := map[string][]byte{
		"qemu.args":      []byte(strings.Join(n.QEMU, "\n") + "\n"),
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
