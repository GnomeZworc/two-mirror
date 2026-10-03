package main

import (
	"fmt"
	"io"
	"os"

	"git.g3e.fr/syonad/two/internal/lab/topology"
)

const usage = `usage: lab <command> <topology.yml>

  plan    validate the topology and print the deterministic plan: addresses, cables, ports
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "plan":
		return plan(args[1], stdout, stderr)
	default:
		fmt.Fprint(stderr, usage)
		return 2
	}
}

func plan(path string, stdout, stderr io.Writer) int {
	t, err := topology.Load(path)
	if err != nil {
		fmt.Fprintf(stderr, "lab: %v\n", err)
		return 1
	}
	p, err := topology.Compute(t)
	if err != nil {
		fmt.Fprintf(stderr, "lab: %s:\n%v\n", path, err)
		return 1
	}
	if err := p.Write(stdout); err != nil {
		fmt.Fprintf(stderr, "lab: %v\n", err)
		return 1
	}
	return 0
}
