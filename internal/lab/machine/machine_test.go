package machine

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"git.g3e.fr/syonad/two/internal/lab/provision"
	"git.g3e.fr/syonad/two/internal/lab/topology"
)

const labKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFG/JMmjfko96WkJV8DiL6rip/H/q/R++y8s27Z+Cj6O two-lab"

type exitErr int

func (e exitErr) Error() string { return "exit status " + strconv.Itoa(int(e)) }
func (e exitErr) ExitCode() int { return int(e) }

type fakeRunner struct {
	mu     sync.Mutex
	calls  [][]string
	fail   string
	ssh    map[string][]error
	always map[string]error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string{name}, args...))
	if name == f.fail {
		return errors.New(name + " failed")
	}
	switch name {
	case "ssh-keygen":
		private := args[len(args)-1]
		if err := os.WriteFile(private, []byte("private"), 0o600); err != nil {
			return err
		}
		return os.WriteFile(private+".pub", []byte(labKey+"\n"), 0o644)
	case "ssh":
		port := args[indexOf(args, "-p")+1]
		answers := f.ssh[port]
		if len(answers) == 0 {
			return f.always[port]
		}
		f.ssh[port] = answers[1:]
		return answers[0]
	}
	return nil
}

func (f *fakeRunner) commands(name string) [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out [][]string
	for _, c := range f.calls {
		if c[0] == name {
			out = append(out, c)
		}
	}
	return out
}

func indexOf(list []string, value string) int {
	for i, v := range list {
		if v == value {
			return i
		}
	}
	return -1
}

func mirror(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	content := []byte("qcow2 image")
	h := sha512.Sum512(content)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "SHA512SUMS") {
			fmt.Fprintf(w, "%s  deb.qcow2\n", hex.EncodeToString(h[:]))
			return
		}
		w.Write(content)
	}))
	t.Cleanup(server.Close)
	return server, server.URL
}

func labPlan(t *testing.T, base, nodes string) *topology.Plan {
	t.Helper()
	doc := `name: evpn-2hv
images:
  deb:
    url: ` + base + `/deb.qcow2
    sums: ` + base + `/SHA512SUMS
segments:
  underlay: { switch: sw1, cidr: 10.250.0.0/24, mtu: 9000 }
nodes:
` + nodes
	topo, err := topology.Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p, err := topology.Compute(topo)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	return p
}

const switchLast = `  rr1: { role: rr,         image: deb, cpus: 1, memory: 1024, segments: [underlay] }
  hv1: { role: hypervisor, image: deb, cpus: 4, memory: 16384, segments: [underlay] }
  sw1: { role: switch,     image: deb, cpus: 2, memory: 1024 }
`

type fixture struct {
	lab     Lab
	runner  *fakeRunner
	out     *bytes.Buffer
	fetcher provision.Fetcher
}

func newFixture(t *testing.T, nodes string) *fixture {
	t.Helper()
	server, base := mirror(t)
	root := t.TempDir()
	runner := &fakeRunner{ssh: map[string][]error{}, always: map[string]error{}}
	out := &bytes.Buffer{}
	return &fixture{
		lab: Lab{
			Plan:    labPlan(t, base, nodes),
			RunDir:  filepath.Join(root, "run"),
			ProcDir: filepath.Join(root, "proc"),
			Runner:  runner,
			Poll:    time.Millisecond,
			Stop:    500 * time.Millisecond,
			Out:     out,
		},
		runner:  runner,
		out:     out,
		fetcher: provision.Fetcher{Client: server.Client(), CacheDir: filepath.Join(root, "cache")},
	}
}

func (f *fixture) process(t *testing.T, node, script string, cmdline ...string) int {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		cmd.Process.Kill()
		<-done
	})
	pid := cmd.Process.Pid
	proc := filepath.Join(f.lab.ProcDir, strconv.Itoa(pid))
	if err := os.MkdirAll(proc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proc, "cmdline"), []byte(strings.Join(cmdline, "\x00")+"\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.pidFile(t, node, strconv.Itoa(pid)+"\n")
	return pid
}

func (f *fixture) pidFile(t *testing.T, node, content string) {
	t.Helper()
	dir := filepath.Join(f.lab.RunDir, node)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "qemu.pid"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func qemu(node string) []string {
	return []string{"qemu-system-x86_64", "-name", node, "-machine", "q35"}
}

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func waitDead(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for alive(pid) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if alive(pid) {
		t.Errorf("pid %d is still alive", pid)
	}
}

func TestPID_NoPidFileMeansStopped(t *testing.T) {
	f := newFixture(t, switchLast)
	if pid, err := f.lab.PID("hv1"); pid != 0 || err != nil {
		t.Errorf("PID = %d, %v", pid, err)
	}
}

func TestPID_RefusesACorruptedPidFile(t *testing.T) {
	for _, content := range []string{"", "abc", "0", "-12"} {
		f := newFixture(t, switchLast)
		f.pidFile(t, "hv1", content)
		if _, err := f.lab.PID("hv1"); err == nil || !strings.Contains(err.Error(), "not a pid") {
			t.Errorf("pid file %q: error = %v", content, err)
		}
	}
}

func TestPID_RecognisesTheNodeProcess(t *testing.T) {
	f := newFixture(t, switchLast)
	want := f.process(t, "hv1", "sleep 30", qemu("hv1")...)
	if pid, err := f.lab.PID("hv1"); pid != want || err != nil {
		t.Errorf("PID = %d, %v, want %d", pid, err, want)
	}
}

func TestPID_IgnoresAPidReusedByAnotherProcess(t *testing.T) {
	cases := map[string][]string{
		"another program": {"/usr/sbin/sshd", "-D"},
		"another node":    qemu("hv10"),
		"name as a value": {"qemu-system-x86_64", "-serial", "-name", "-name", "hv2"},
	}
	for name, cmdline := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, switchLast)
			f.process(t, "hv1", "sleep 30", cmdline...)
			if pid, err := f.lab.PID("hv1"); pid != 0 || err != nil {
				t.Errorf("PID = %d, %v, want 0", pid, err)
			}
		})
	}
}

func TestPID_IgnoresADeadProcess(t *testing.T) {
	f := newFixture(t, switchLast)
	pid := f.process(t, "hv1", "exit 0", qemu("hv1")...)
	waitDead(t, pid)
	if got, err := f.lab.PID("hv1"); got != 0 || err != nil {
		t.Errorf("PID = %d, %v, want 0", got, err)
	}
}

func TestStatus(t *testing.T) {
	f := newFixture(t, switchLast)
	pid := strconv.Itoa(f.process(t, "hv1", "sleep 30", qemu("hv1")...))
	var out bytes.Buffer
	if err := f.lab.Status(&out); err != nil {
		t.Fatal(err)
	}
	w := len(pid)
	if w < 3 {
		w = 3
	}
	want := fmt.Sprintf("node  role        state    %-*s  ssh\n", w, "pid") +
		fmt.Sprintf("rr1   rr          stopped  %-*s  127.0.0.1:2200\n", w, "-") +
		fmt.Sprintf("hv1   hypervisor  running  %-*s  127.0.0.1:2201\n", w, pid) +
		fmt.Sprintf("sw1   switch      stopped  %-*s  127.0.0.1:2202\n", w, "-")
	if out.String() != want {
		t.Errorf("status:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestDown_StopsWithSIGTERM(t *testing.T) {
	f := newFixture(t, switchLast)
	pid := f.process(t, "hv1", "sleep 30", qemu("hv1")...)

	if err := f.lab.Down(context.Background()); err != nil {
		t.Fatalf("Down: %v", err)
	}

	waitDead(t, pid)
	if _, err := os.Stat(filepath.Join(f.lab.RunDir, "hv1", "qemu.pid")); !os.IsNotExist(err) {
		t.Errorf("pid file still there: %v", err)
	}
	if f.out.String() != "hv1: stopped\n" {
		t.Errorf("output = %q", f.out.String())
	}
}

func TestDown_FallsBackToSIGKILL(t *testing.T) {
	f := newFixture(t, switchLast)
	f.lab.Stop = 100 * time.Millisecond
	pid := f.process(t, "hv1", `trap "" TERM; while :; do sleep 0.05; done`, qemu("hv1")...)
	time.Sleep(50 * time.Millisecond)

	start := time.Now()
	if err := f.lab.Down(context.Background()); err != nil {
		t.Fatalf("Down: %v", err)
	}

	waitDead(t, pid)
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Errorf("SIGKILL after %v, want at least the 100ms grace period", elapsed)
	}
}

func TestDown_NeverSignalsAProcessThatIsNotTheNode(t *testing.T) {
	f := newFixture(t, switchLast)
	pid := f.process(t, "hv1", "sleep 30", "/usr/sbin/sshd", "-D")

	if err := f.lab.Down(context.Background()); err != nil {
		t.Fatalf("Down: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	if !alive(pid) {
		t.Error("a process that is not the node was killed")
	}
	if f.out.Len() != 0 {
		t.Errorf("output = %q", f.out.String())
	}
}

func TestDown_ReportsACorruptedPidFileAndStopsTheOthers(t *testing.T) {
	f := newFixture(t, switchLast)
	f.pidFile(t, "rr1", "garbage")
	pid := f.process(t, "hv1", "sleep 30", qemu("hv1")...)

	err := f.lab.Down(context.Background())

	if err == nil || !strings.Contains(err.Error(), "not a pid") {
		t.Errorf("error = %v", err)
	}
	waitDead(t, pid)
}

func TestSSH_Arguments(t *testing.T) {
	f := newFixture(t, switchLast)
	key := filepath.Join(f.lab.RunDir, "lab_ed25519")
	base := []string{
		"-i", key,
		"-o", "IdentitiesOnly=yes",
		"-o", "IdentityAgent=none",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-p", "2201",
		"debian@127.0.0.1",
	}
	cases := map[string]struct {
		terminal bool
		command  []string
		want     []string
	}{
		"interactive shell":       {true, nil, append([]string{"ssh"}, base...)},
		"command from a terminal": {true, []string{"top"}, append(append([]string{"ssh", "-t"}, base...), "top")},
		"command from a script":   {false, []string{"ip", "-br", "a"}, append(append([]string{"ssh"}, base...), "ip", "-br", "a")},
		"shell from a script":     {false, nil, append([]string{"ssh"}, base...)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := f.lab.SSH("hv1", c.terminal, c.command)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("\n got %q\nwant %q", got, c.want)
			}
		})
	}
}

func TestSSH_UnknownNode(t *testing.T) {
	f := newFixture(t, switchLast)
	if _, err := f.lab.SSH("hv9", true, nil); err == nil || !strings.Contains(err.Error(), `node "hv9" is not in lab evpn-2hv`) {
		t.Errorf("error = %v", err)
	}
}

func TestUp_StartsSwitchesFirstAndWaitsForEveryNode(t *testing.T) {
	f := newFixture(t, switchLast)

	if err := f.lab.Up(context.Background(), f.fetcher, time.Second); err != nil {
		t.Fatalf("Up: %v", err)
	}

	var started []string
	for _, c := range f.runner.commands("qemu-system-x86_64") {
		if c[len(c)-1] != "-daemonize" {
			t.Errorf("qemu not daemonized: %q", c)
		}
		started = append(started, c[indexOf(c, "-name")+1])
	}
	if !reflect.DeepEqual(started, []string{"sw1", "rr1", "hv1"}) {
		t.Errorf("start order = %v, want the switch first", started)
	}

	ssh := f.runner.commands("ssh")
	if len(ssh) != 3 {
		t.Fatalf("%d ssh calls, want 3", len(ssh))
	}
	want := []string{"ssh",
		"-i", filepath.Join(f.lab.RunDir, "lab_ed25519"),
		"-o", "IdentitiesOnly=yes",
		"-o", "IdentityAgent=none",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-p", "2200",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		"debian@127.0.0.1",
		"cloud-init", "status", "--wait",
	}
	if !reflect.DeepEqual(ssh[0], want) {
		t.Errorf("readiness check:\n got %q\nwant %q", ssh[0], want)
	}
	if f.out.String() != "sw1: started\nrr1: started\nhv1: started\nrr1: ready\nhv1: ready\nsw1: ready\n" {
		t.Errorf("output = %q", f.out.String())
	}
}

func TestUp_RetriesWhileSSHIsUnreachable(t *testing.T) {
	f := newFixture(t, switchLast)
	f.runner.ssh["2201"] = []error{exitErr(255), exitErr(255), nil}

	if err := f.lab.Up(context.Background(), f.fetcher, time.Second); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if n := len(f.runner.commands("ssh")); n != 5 {
		t.Errorf("%d ssh calls, want 5", n)
	}
}

func TestUp_ReadinessFailures(t *testing.T) {
	cases := map[string]struct {
		answers []error
		want    string
	}{
		"cloud-init error":    {[]error{exitErr(1)}, "node hv1: cloud-init failed: exit status 1"},
		"not an exit status":  {[]error{errors.New("fork failed")}, "node hv1: cloud-init failed: fork failed"},
		"ssh never reachable": {nil, "node hv1: not reachable over ssh: context deadline exceeded"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, switchLast)
			f.lab.Poll = 10 * time.Millisecond
			f.runner.ssh["2201"] = c.answers
			f.runner.always["2201"] = exitErr(255)

			err := f.lab.Up(context.Background(), f.fetcher, 300*time.Millisecond)

			if err == nil || err.Error() != c.want {
				t.Errorf("error = %v, want %q", err, c.want)
			}
			if !strings.Contains(f.out.String(), "rr1: ready\n") || !strings.Contains(f.out.String(), "sw1: ready\n") {
				t.Errorf("the other nodes were not waited for: %q", f.out.String())
			}
		})
	}
}

func TestUp_RecoverableCloudInitErrorsAreReportedNotFatal(t *testing.T) {
	f := newFixture(t, switchLast)
	f.runner.ssh["2201"] = []error{exitErr(2)}

	if err := f.lab.Up(context.Background(), f.fetcher, time.Second); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if !strings.Contains(f.out.String(), "hv1: cloud-init finished with recoverable errors: exit status 2\nhv1: ready\n") {
		t.Errorf("output = %q", f.out.String())
	}
}

func TestUp_RefusesARunningLab(t *testing.T) {
	f := newFixture(t, switchLast)
	f.process(t, "hv1", "sleep 30", qemu("hv1")...)

	err := f.lab.Up(context.Background(), f.fetcher, time.Second)

	if err == nil || err.Error() != "lab evpn-2hv is already running (hv1): 'lab down' first" {
		t.Errorf("error = %v", err)
	}
	if len(f.runner.calls) != 0 {
		t.Errorf("commands ran: %q", f.runner.calls)
	}
}

func TestUp_RemovesAStalePidFileBeforeStarting(t *testing.T) {
	f := newFixture(t, switchLast)
	pid := f.process(t, "hv1", "exit 0", qemu("hv1")...)
	waitDead(t, pid)

	if err := f.lab.Up(context.Background(), f.fetcher, time.Second); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.lab.RunDir, "hv1", "qemu.pid")); !os.IsNotExist(err) {
		t.Errorf("stale pid file kept: %v", err)
	}
}

func TestUp_StopsAtTheFirstQEMUFailure(t *testing.T) {
	f := newFixture(t, switchLast)
	f.runner.fail = "qemu-system-x86_64"

	err := f.lab.Up(context.Background(), f.fetcher, time.Second)

	if err == nil || err.Error() != "node sw1: qemu-system-x86_64 failed" {
		t.Errorf("error = %v", err)
	}
	if n := len(f.runner.commands("qemu-system-x86_64")); n != 1 {
		t.Errorf("%d qemu starts, want 1", n)
	}
	if n := len(f.runner.commands("ssh")); n != 0 {
		t.Errorf("%d ssh calls after a failed start", n)
	}
}

func TestStop_RefusesPidsThatTargetAGroup(t *testing.T) {
	f := newFixture(t, switchLast)
	f.lab.signal = func(pid int, sig syscall.Signal) error {
		t.Fatalf("signal %v sent to pid %d", sig, pid)
		return nil
	}
	for _, pid := range []int{0, -1} {
		if err := f.lab.stop(context.Background(), "hv1", pid); err == nil || err.Error() != fmt.Sprintf("refusing to signal pid %d", pid) {
			t.Errorf("pid %d: error = %v", pid, err)
		}
	}
}
