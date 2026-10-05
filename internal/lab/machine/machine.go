package machine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"git.g3e.fr/syonad/two/internal/lab/provision"
	"git.g3e.fr/syonad/two/internal/lab/render"
	"git.g3e.fr/syonad/two/internal/lab/topology"
)

const (
	User = "debian"

	sshUnreachable       = 255
	cloudInitRecoverable = 2
)

type Lab struct {
	Plan    *topology.Plan
	RunDir  string
	ProcDir string
	Runner  provision.Runner
	Poll    time.Duration
	Stop    time.Duration
	Out     io.Writer

	signal func(pid int, sig syscall.Signal) error
}

func (l Lab) node(name string) (topology.NodePlan, error) {
	for _, n := range l.Plan.Nodes {
		if n.Name == name {
			return n, nil
		}
	}
	return topology.NodePlan{}, fmt.Errorf("node %q is not in lab %s", name, l.Plan.Name)
}

func (l Lab) pidFile(name string) string {
	return filepath.Join(l.RunDir, name, render.PIDFile)
}

func (l Lab) PID(name string) (int, error) {
	data, err := os.ReadFile(l.pidFile(name))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("%s: not a pid: %q", l.pidFile(name), data)
	}
	if !l.runs(pid, name) {
		return 0, nil
	}
	return pid, nil
}

func (l Lab) runs(pid int, name string) bool {
	if err := syscall.Kill(pid, 0); err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	cmdline, err := os.ReadFile(filepath.Join(l.ProcDir, strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return false
	}
	return bytes.Contains(cmdline, []byte("\x00-name\x00"+name+"\x00"))
}

func (l Lab) Running() ([]string, error) {
	var running []string
	for _, n := range l.Plan.Nodes {
		pid, err := l.PID(n.Name)
		if err != nil {
			return nil, err
		}
		if pid != 0 {
			running = append(running, n.Name)
		}
	}
	return running, nil
}

func (l Lab) Up(ctx context.Context, fetcher provision.Fetcher, timeout time.Duration) error {
	running, err := l.Running()
	if err != nil {
		return err
	}
	if len(running) > 0 {
		return fmt.Errorf("lab %s is already running (%s): 'lab down' first", l.Plan.Name, strings.Join(running, ", "))
	}

	nodes, err := provision.Prepare(ctx, l.Plan, provision.Options{RunDir: l.RunDir, Fetcher: fetcher, Runner: l.Runner})
	if err != nil {
		return err
	}
	for _, n := range switchesFirst(l.Plan, nodes) {
		if err := os.Remove(l.pidFile(n.Name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := l.Runner.Run(ctx, render.QEMUBinary, append(n.QEMU, "-daemonize")...); err != nil {
			return fmt.Errorf("node %s: %w", n.Name, err)
		}
		fmt.Fprintf(l.Out, "%s: started\n", n.Name)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var errs []error
	for _, n := range l.Plan.Nodes {
		if err := l.waitReady(ctx, n); err != nil {
			errs = append(errs, fmt.Errorf("node %s: %w", n.Name, err))
			continue
		}
		if err := l.checkServices(ctx, n); err != nil {
			errs = append(errs, fmt.Errorf("node %s: %w", n.Name, err))
			continue
		}
		fmt.Fprintf(l.Out, "%s: ready\n", n.Name)
	}
	return errors.Join(errs...)
}

func switchesFirst(p *topology.Plan, nodes []render.Node) []render.Node {
	var first, rest []render.Node
	for i, n := range nodes {
		if p.Nodes[i].Role == topology.RoleSwitch {
			first = append(first, n)
		} else {
			rest = append(rest, n)
		}
	}
	return append(first, rest...)
}

func (l Lab) waitReady(ctx context.Context, n topology.NodePlan) error {
	args := l.sshArgs(n, true, []string{"cloud-init", "status", "--wait"})
	for {
		err := l.Runner.Run(ctx, "ssh", args...)
		switch code := exitCode(err); {
		case err == nil:
			return nil
		case code == cloudInitRecoverable:
			fmt.Fprintf(l.Out, "%s: cloud-init finished with recoverable errors: %v\n", n.Name, err)
			return nil
		case code != sshUnreachable:
			return fmt.Errorf("cloud-init failed: %w", err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("not reachable over ssh: %w", ctx.Err())
		case <-time.After(l.Poll):
		}
	}
}

func services(n topology.NodePlan) []string {
	var units []string
	if n.Role == topology.RoleHypervisor {
		units = append(units, "agent.service")
	}
	if n.FRR != "" {
		units = append(units, "frr.service")
	}
	return units
}

func (l Lab) checkServices(ctx context.Context, n topology.NodePlan) error {
	for _, unit := range services(n) {
		if err := l.Runner.Run(ctx, "ssh", l.sshArgs(n, true, []string{"systemctl", "is-active", "--quiet", unit})...); err != nil {
			return fmt.Errorf("%s is not active: %w", unit, err)
		}
	}
	return nil
}

func exitCode(err error) int {
	var coded interface{ ExitCode() int }
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	return -1
}

func (l Lab) sshArgs(n topology.NodePlan, batch bool, command []string) []string {
	args := []string{
		"-i", filepath.Join(l.RunDir, provision.KeyFile),
		"-o", "IdentitiesOnly=yes",
		"-o", "IdentityAgent=none",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-p", strconv.Itoa(n.SSHPort),
	}
	if batch {
		args = append(args, "-o", "BatchMode=yes", "-o", "ConnectTimeout=5")
	}
	args = append(args, User+"@127.0.0.1")
	return append(args, command...)
}

func (l Lab) SSH(name string, terminal bool, command []string) ([]string, error) {
	n, err := l.node(name)
	if err != nil {
		return nil, err
	}
	args := []string{"ssh"}
	if terminal && len(command) > 0 {
		args = append(args, "-t")
	}
	return append(args, l.sshArgs(n, false, command)...), nil
}

func (l Lab) Status(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "node\trole\tstate\tpid\tssh\n")
	for _, n := range l.Plan.Nodes {
		pid, err := l.PID(n.Name)
		if err != nil {
			return err
		}
		state, shown := "stopped", "-"
		if pid != 0 {
			state, shown = "running", strconv.Itoa(pid)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t127.0.0.1:%d\n", n.Name, n.Role, state, shown, n.SSHPort)
	}
	return tw.Flush()
}

func (l Lab) Down(ctx context.Context) error {
	var errs []error
	for _, n := range l.Plan.Nodes {
		pid, err := l.PID(n.Name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if pid == 0 {
			continue
		}
		if err := l.stop(ctx, n.Name, pid); err != nil {
			errs = append(errs, fmt.Errorf("node %s: %w", n.Name, err))
			continue
		}
		if err := os.Remove(l.pidFile(n.Name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
			continue
		}
		fmt.Fprintf(l.Out, "%s: stopped\n", n.Name)
	}
	return errors.Join(errs...)
}

func (l Lab) stop(ctx context.Context, name string, pid int) error {
	if pid <= 0 {
		return fmt.Errorf("refusing to signal pid %d", pid)
	}
	send := l.signal
	if send == nil {
		send = syscall.Kill
	}
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		if err := send(pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		deadline := time.Now().Add(l.Stop)
		for l.runs(pid, name) && time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(l.Poll):
			}
		}
		if !l.runs(pid, name) {
			return nil
		}
	}
	return fmt.Errorf("pid %d still running after SIGKILL", pid)
}
