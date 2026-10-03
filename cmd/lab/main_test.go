package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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
