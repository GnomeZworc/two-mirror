package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func runLab(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRun_UsageOnMissingArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"plan"}, {"deploy", "x.yml"}, {"plan", "a.yml", "b.yml"}, {"render", "a.yml", "dir"}, {"render", "-key", "k.pub", "a.yml"}, {"render", "-bogus"}} {
		code, _, stderr := runLab(args...)
		if code != 2 || !strings.Contains(stderr, "usage: lab") {
			t.Errorf("args %v: code %d, stderr %q", args, code, stderr)
		}
	}
}

func TestRun_PlanOfTheShippedExampleTopology(t *testing.T) {
	code, stdout, stderr := runLab("plan", filepath.Join("..", "..", "conf", "lab", "evpn-2hv.yml"))
	if code != 0 {
		t.Fatalf("code %d, stderr %s", code, stderr)
	}
	for _, want := range []string{
		"lab evpn-2hv: nodes 4, segments 1, cables 3",
		"gateway 10.250.0.1",
		"hv2   underlay   10.250.0.4/24  02:4c:00:03:00:00  20004  <->  sw1 p2",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output does not contain %q:\n%s", want, stdout)
		}
	}
}

func TestRun_InvalidTopologyExitsWithErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yml")
	doc := `name: bad
images:
  deb: { url: https://example.invalid/a, sums: https://example.invalid/b }
segments:
  under: { switch: sw, cidr: 10.0.0.0/31 }
nodes:
  sw: { role: switch, image: deb, cpus: 1, memory: 512 }
  rr: { role: router, image: deb, cpus: 1, memory: 512, segments: [under] }
`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runLab("plan", path)
	if code != 1 || stdout != "" {
		t.Fatalf("code %d, stdout %q", code, stdout)
	}
	for _, want := range []string{path, `role "router"`, "prefix length out of range"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr does not mention %q:\n%s", want, stderr)
		}
	}
}

func TestRun_MissingFile(t *testing.T) {
	code, _, stderr := runLab("plan", filepath.Join(t.TempDir(), "absent.yml"))
	if code != 1 || !strings.Contains(stderr, "absent.yml") {
		t.Errorf("code %d, stderr %q", code, stderr)
	}
}

func TestRun_RenderWritesEveryNodeFiles(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "lab.pub")
	if err := os.WriteFile(key, []byte("# lab key\n\nssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFG/JMmjfko96WkJV8DiL6rip/H/q/R++y8s27Z+Cj6O two-lab-automation\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "run")
	code, stdout, stderr := runLab("render", "-key", key, filepath.Join("..", "..", "conf", "lab", "evpn-2hv.yml"), out)
	if code != 0 {
		t.Fatalf("code %d, stderr %s", code, stderr)
	}
	for _, node := range []string{"sw1", "rr1", "hv1", "hv2"} {
		if !strings.Contains(stdout, filepath.Join(out, node)) {
			t.Errorf("stdout does not list %s:\n%s", node, stdout)
		}
		for _, f := range []string{"qemu.args", "meta-data", "user-data", "network-config"} {
			info, err := os.Stat(filepath.Join(out, node, f))
			if err != nil {
				t.Errorf("%s/%s: %v", node, f, err)
				continue
			}
			if info.Mode().Perm() != 0o600 {
				t.Errorf("%s/%s mode %o, want 600", node, f, info.Mode().Perm())
			}
		}
	}
	args, err := os.ReadFile(filepath.Join(out, "hv1", "qemu.args"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "\n-netdev\nuser,id=mgmt0,restrict=on,ipv6=off,hostfwd=tcp:127.0.0.1:2202-:22\n") {
		t.Errorf("qemu.args is not one argument per line:\n%s", args)
	}
	userData, err := os.ReadFile(filepath.Join(out, "hv1", "user-data"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(userData), "two-lab-automation") || strings.Contains(string(userData), "# lab key") {
		t.Errorf("keys not read as an authorized_keys file:\n%s", userData)
	}
}

func TestRun_RenderRefusesMissingKeyFile(t *testing.T) {
	code, _, stderr := runLab("render", "-key", filepath.Join(t.TempDir(), "absent.pub"), filepath.Join("..", "..", "conf", "lab", "evpn-2hv.yml"), t.TempDir())
	if code != 1 || !strings.Contains(stderr, "absent.pub") {
		t.Errorf("code %d, stderr %q", code, stderr)
	}
}

func savedLab(t *testing.T) string {
	t.Helper()
	run := filepath.Join(t.TempDir(), "run")
	if err := os.MkdirAll(run, 0o700); err != nil {
		t.Fatal(err)
	}
	example, err := os.ReadFile(filepath.Join("..", "..", "conf", "lab", "evpn-2hv.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "topology.yml"), example, 0o600); err != nil {
		t.Fatal(err)
	}
	return run
}

func TestRun_LifecycleUsage(t *testing.T) {
	for _, args := range [][]string{{"up"}, {"up", "a.yml", "b.yml"}, {"status", "x"}, {"down", "x"}, {"ssh"}, {"up", "-bogus", "a.yml"}} {
		code, _, stderr := runLab(args...)
		if code != 2 || !strings.Contains(stderr, "usage: lab") {
			t.Errorf("args %v: code %d, stderr %q", args, code, stderr)
		}
	}
}

func TestRun_CommandsWithoutALabInTheRunDir(t *testing.T) {
	run := t.TempDir()
	for _, cmd := range []string{"status", "down", "ssh"} {
		args := []string{cmd, "-run", run}
		if cmd == "ssh" {
			args = append(args, "hv1")
		}
		code, _, stderr := runLab(args...)
		if code != 1 || !strings.Contains(stderr, "lab: no lab in "+run) {
			t.Errorf("%s: code %d, stderr %q", cmd, code, stderr)
		}
	}
}

func TestRun_StatusOfAStoppedLab(t *testing.T) {
	code, stdout, stderr := runLab("status", "-run", savedLab(t))
	want := `node  role        state    pid  ssh
sw1   switch      stopped  -    127.0.0.1:2200
rr1   rr          stopped  -    127.0.0.1:2201
hv1   hypervisor  stopped  -    127.0.0.1:2202
hv2   hypervisor  stopped  -    127.0.0.1:2203
`
	if code != 0 || stdout != want {
		t.Errorf("code %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
}

func TestRun_DownOfAStoppedLab(t *testing.T) {
	code, stdout, stderr := runLab("down", "-run", savedLab(t))
	if code != 0 || stdout != "" || stderr != "" {
		t.Errorf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestRun_SSHExecsSSHWithTheNodePort(t *testing.T) {
	run := savedLab(t)
	var gotPath string
	var gotArgv []string
	execve = func(path string, argv []string, env []string) error {
		gotPath, gotArgv = path, argv
		return nil
	}
	t.Cleanup(func() { execve = syscall.Exec })

	code, _, stderr := runLab("ssh", "-run", run, "hv2", "ip", "-br", "a")

	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if filepath.Base(gotPath) != "ssh" {
		t.Errorf("path = %q", gotPath)
	}
	want := []string{"ssh",
		"-i", filepath.Join(run, "lab_ed25519"),
		"-o", "IdentitiesOnly=yes",
		"-o", "IdentityAgent=none",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-p", "2203",
		"debian@127.0.0.1",
		"ip", "-br", "a",
	}
	if strings.Join(gotArgv, " ") != strings.Join(want, " ") {
		t.Errorf("\n got %q\nwant %q", gotArgv, want)
	}
}

func TestRun_SSHUnknownNode(t *testing.T) {
	code, _, stderr := runLab("ssh", "-run", savedLab(t), "hv9")
	if code != 1 || !strings.Contains(stderr, `node "hv9" is not in lab evpn-2hv`) {
		t.Errorf("code %d, stderr %q", code, stderr)
	}
}

func TestRun_UpRefusesAnInvalidTopologyAndKeepsTheSavedOne(t *testing.T) {
	run := savedLab(t)
	before, _ := os.ReadFile(filepath.Join(run, "topology.yml"))
	bad := filepath.Join(t.TempDir(), "bad.yml")
	if err := os.WriteFile(bad, []byte("name: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := runLab("up", "-run", run, bad)

	if code != 1 || !strings.Contains(stderr, "at least one node is required") {
		t.Errorf("code %d, stderr %q", code, stderr)
	}
	if after, _ := os.ReadFile(filepath.Join(run, "topology.yml")); string(after) != string(before) {
		t.Error("the saved topology was replaced by an invalid one")
	}
}

func TestIsTerminal_DevNullAndPipesAreNotTerminals(t *testing.T) {
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	for name, f := range map[string]*os.File{"/dev/null": null, "pipe": r} {
		if isTerminal(f) {
			t.Errorf("%s is detected as a terminal", name)
		}
	}
}

func TestRun_UpRefusesToReplaceARunningLab(t *testing.T) {
	run := savedLab(t)
	before, _ := os.ReadFile(filepath.Join(run, "topology.yml"))
	proc := t.TempDir()
	procDir = proc
	t.Cleanup(func() { procDir = "/proc" })

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	pid := strconv.Itoa(cmd.Process.Pid)
	if err := os.MkdirAll(filepath.Join(proc, pid), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proc, pid, "cmdline"), []byte("qemu-system-x86_64\x00-name\x00hv1\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(run, "hv1"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "hv1", "qemu.pid"), []byte(pid+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	other := filepath.Join(t.TempDir(), "other.yml")
	if err := os.WriteFile(other, bytes.ReplaceAll(before, []byte("evpn-2hv"), []byte("other")), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runLab("up", "-run", run, other)

	if code != 1 || !strings.Contains(stderr, "lab evpn-2hv is still running in "+run) {
		t.Errorf("code %d, stderr %q", code, stderr)
	}
	if after, _ := os.ReadFile(filepath.Join(run, "topology.yml")); string(after) != string(before) {
		t.Error("the topology of a running lab was replaced")
	}
}
