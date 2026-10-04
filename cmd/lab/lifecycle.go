package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"git.g3e.fr/syonad/two/internal/lab/machine"
	"git.g3e.fr/syonad/two/internal/lab/provision"
)

const (
	topologyFile = "topology.yml"
	pollInterval = 5 * time.Second
	stopTimeout  = 30 * time.Second
)

var (
	execve  = syscall.Exec
	procDir = "/proc"
)

func defaultDir(parts ...string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(append([]string{home}, parts...)...)
}

func flags(name string, stderr io.Writer) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	return fs, fs.String("run", defaultDir("lab-run"), "run directory of the lab")
}

func lab(runDir string, plan string, stdout, stderr io.Writer) (machine.Lab, bool) {
	dir, err := filepath.Abs(runDir)
	if err != nil {
		fmt.Fprintf(stderr, "lab: %v\n", err)
		return machine.Lab{}, false
	}
	p, ok := load(plan, stderr)
	if !ok {
		return machine.Lab{}, false
	}
	return machine.Lab{
		Plan:    p,
		RunDir:  dir,
		ProcDir: procDir,
		Runner:  provision.ExecRunner{},
		Poll:    pollInterval,
		Stop:    stopTimeout,
		Out:     stdout,
	}, true
}

func current(runDir string, stdout, stderr io.Writer) (machine.Lab, bool) {
	saved := filepath.Join(runDir, topologyFile)
	if _, err := os.Stat(saved); err != nil {
		fmt.Fprintf(stderr, "lab: no lab in %s: %v\n", runDir, err)
		return machine.Lab{}, false
	}
	return lab(runDir, saved, stdout, stderr)
}

func upCmd(args []string, stdout, stderr io.Writer) int {
	fs, runDir := flags("up", stderr)
	cacheDir := fs.String("cache", defaultDir(".cache", "two-lab"), "image cache directory")
	timeout := fs.Duration("timeout", 20*time.Minute, "how long to wait for the nodes to be ready")
	if err := fs.Parse(args); err != nil || fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	cache, err := filepath.Abs(*cacheDir)
	if err != nil {
		fmt.Fprintf(stderr, "lab: %v\n", err)
		return 1
	}

	if _, err := os.Stat(filepath.Join(*runDir, topologyFile)); err == nil {
		previous, ok := current(*runDir, stdout, stderr)
		if !ok {
			return 1
		}
		running, err := previous.Running()
		if err != nil {
			fmt.Fprintf(stderr, "lab: %v\n", err)
			return 1
		}
		if len(running) > 0 {
			fmt.Fprintf(stderr, "lab: lab %s is still running in %s: 'lab down' first\n", previous.Plan.Name, previous.RunDir)
			return 1
		}
	}

	l, ok := lab(*runDir, fs.Arg(0), stdout, stderr)
	if !ok {
		return 1
	}
	source, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "lab: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(l.RunDir, 0o700); err != nil {
		fmt.Fprintf(stderr, "lab: %v\n", err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(l.RunDir, topologyFile), source, 0o600); err != nil {
		fmt.Fprintf(stderr, "lab: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fetcher := provision.Fetcher{Client: &http.Client{}, CacheDir: cache}
	if err := l.Up(ctx, fetcher, *timeout); err != nil {
		fmt.Fprintf(stderr, "lab: %v\nlab: started nodes keep running: 'lab status', 'lab down'\n", err)
		return 1
	}
	return 0
}

func statusCmd(args []string, stdout, stderr io.Writer) int {
	fs, runDir := flags("status", stderr)
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	l, ok := current(*runDir, stdout, stderr)
	if !ok {
		return 1
	}
	if err := l.Status(stdout); err != nil {
		fmt.Fprintf(stderr, "lab: %v\n", err)
		return 1
	}
	return 0
}

func downCmd(args []string, stdout, stderr io.Writer) int {
	fs, runDir := flags("down", stderr)
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	l, ok := current(*runDir, stdout, stderr)
	if !ok {
		return 1
	}
	if err := l.Down(context.Background()); err != nil {
		fmt.Fprintf(stderr, "lab: %v\n", err)
		return 1
	}
	return 0
}

func sshCmd(args []string, stdout, stderr io.Writer) int {
	fs, runDir := flags("ssh", stderr)
	if err := fs.Parse(args); err != nil || fs.NArg() < 1 {
		fs.Usage()
		return 2
	}
	l, ok := current(*runDir, stdout, stderr)
	if !ok {
		return 1
	}
	argv, err := l.SSH(fs.Arg(0), isTerminal(os.Stdin), fs.Args()[1:])
	if err != nil {
		fmt.Fprintf(stderr, "lab: %v\n", err)
		return 1
	}
	path, err := exec.LookPath(argv[0])
	if err != nil {
		fmt.Fprintf(stderr, "lab: %v\n", err)
		return 1
	}
	if err := execve(path, argv, os.Environ()); err != nil {
		fmt.Fprintf(stderr, "lab: %v\n", err)
		return 1
	}
	return 0
}
