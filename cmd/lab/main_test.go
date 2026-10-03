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
	for _, args := range [][]string{nil, {"plan"}, {"deploy", "x.yml"}, {"plan", "a.yml", "b.yml"}} {
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
