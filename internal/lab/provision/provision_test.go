package provision

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"git.g3e.fr/syonad/two/internal/lab/render"
	"git.g3e.fr/syonad/two/internal/lab/topology"
)

const generatedKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFG/JMmjfko96WkJV8DiL6rip/H/q/R++y8s27Z+Cj6O two-lab"

type fakeRunner struct {
	calls [][]string
	fail  string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, append([]string{name}, args...))
	if name == f.fail {
		return errors.New(name + " failed")
	}
	if name == "ssh-keygen" {
		private := args[len(args)-1]
		if err := os.WriteFile(private, []byte("private"), 0o600); err != nil {
			return err
		}
		return os.WriteFile(private+".pub", []byte(generatedKey+"\n"), 0o644)
	}
	return nil
}

func (f *fakeRunner) commands(name string) [][]string {
	var out [][]string
	for _, c := range f.calls {
		if c[0] == name {
			out = append(out, c)
		}
	}
	return out
}

func labPlan(t *testing.T, m *mirror) *topology.Plan {
	t.Helper()
	img := m.image()
	doc := `name: evpn-2hv
images:
  debian12:
    url: ` + img.URL + `
    sums: ` + img.Sums + `
segments:
  underlay: { switch: sw1, cidr: 10.250.0.0/24, mtu: 9000 }
nodes:
  sw1: { role: switch,     image: debian12, cpus: 2, memory: 1024 }
  rr1: { role: rr,         image: debian12, cpus: 1, memory: 1024, segments: [underlay] }
  hv1: { role: hypervisor, image: debian12, cpus: 4, memory: 16384, segments: [underlay] }
  hv2: { role: hypervisor, image: debian12, cpus: 4, memory: 16384, segments: [underlay] }
`
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

func prepare(t *testing.T, runner *fakeRunner) (string, string, *mirror, []render.Node, error) {
	t.Helper()
	m := newMirror(t)
	publish(m, []byte("qcow2 image"))
	root := t.TempDir()
	run := filepath.Join(root, "run")
	cache := filepath.Join(root, "cache")
	nodes, err := Prepare(context.Background(), labPlan(t, m), Options{
		RunDir:  run,
		Fetcher: Fetcher{Client: m.server.Client(), CacheDir: cache},
		Runner:  runner,
	})
	return run, cache, m, nodes, err
}

func TestPrepare_StagesEveryNode(t *testing.T) {
	runner := &fakeRunner{}
	run, cache, _, nodes, err := prepare(t, runner)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	base := filepath.Join(cache, "debian12", "debian-12-generic-amd64.qcow2")
	want := [][]string{
		{"ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "two-lab", "-f", filepath.Join(run, "lab_ed25519")},
	}
	for _, n := range []string{"sw1", "rr1", "hv1", "hv2"} {
		dir := filepath.Join(run, n)
		want = append(want,
			[]string{"qemu-img", "create", "-q", "-f", "qcow2", "-F", "qcow2", "-b", base, filepath.Join(dir, "disk.qcow2"), "20G"},
			[]string{"genisoimage", "-quiet", "-output", filepath.Join(dir, "seed.iso"), "-volid", "cidata", "-joliet", "-rock",
				filepath.Join(dir, "user-data"), filepath.Join(dir, "meta-data"), filepath.Join(dir, "network-config")},
		)
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Errorf("commands:\n got %q\nwant %q", runner.calls, want)
	}

	var names []string
	for _, n := range nodes {
		names = append(names, n.Name)
	}
	if !reflect.DeepEqual(names, []string{"sw1", "rr1", "hv1", "hv2"}) {
		t.Errorf("nodes = %v", names)
	}
	if info, err := os.Stat(run); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("run dir mode = %v, %v", info.Mode().Perm(), err)
	}
}

func TestPrepare_DownloadsASharedImageOnce(t *testing.T) {
	_, _, m, _, err := prepare(t, &fakeRunner{})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if n := m.count("/bookworm/latest/" + imageName); n != 1 {
		t.Errorf("image downloaded %d times for four nodes, want 1", n)
	}
	if n := m.count("/bookworm/latest/SHA512SUMS"); n != 1 {
		t.Errorf("sums read %d times for four nodes, want 1", n)
	}
}

func TestPrepare_TheGeneratedKeyIsTheOnlyAuthorizedKey(t *testing.T) {
	_, _, _, nodes, err := prepare(t, &fakeRunner{})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	for _, n := range nodes {
		data, err := os.ReadFile(filepath.Join(n.Dir, "user-data"))
		if err != nil {
			t.Fatal(err)
		}
		var cfg struct {
			Keys []string `yaml:"ssh_authorized_keys"`
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("%s user-data: %v", n.Name, err)
		}
		if !reflect.DeepEqual(cfg.Keys, []string{generatedKey}) {
			t.Errorf("%s authorized keys = %q, want only the generated key", n.Name, cfg.Keys)
		}
	}
}

func TestPrepare_WritesTheRenderedFilesPrivately(t *testing.T) {
	run, _, _, nodes, err := prepare(t, &fakeRunner{})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	hv1 := filepath.Join(run, "hv1")
	if nodes[2].Dir != hv1 {
		t.Fatalf("hv1 dir = %s, want %s", nodes[2].Dir, hv1)
	}
	for _, f := range []string{"qemu.args", "user-data", "meta-data", "network-config"} {
		info, err := os.Stat(filepath.Join(hv1, f))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %v, %v", f, info.Mode().Perm(), err)
		}
	}
	args, err := os.ReadFile(filepath.Join(hv1, "qemu.args"))
	if err != nil || !strings.HasPrefix(string(args), "-name\nhv1\n") || !strings.HasSuffix(string(args), "\n") {
		t.Errorf("qemu.args = %q, %v", args, err)
	}
}

func TestPrepare_StopsAtTheFirstFailedCommand(t *testing.T) {
	runner := &fakeRunner{fail: "qemu-img"}
	_, _, _, _, err := prepare(t, runner)
	if err == nil || !strings.Contains(err.Error(), "node sw1: qemu-img failed") {
		t.Errorf("error = %v", err)
	}
	if n := len(runner.commands("genisoimage")); n != 0 {
		t.Errorf("genisoimage ran %d times after qemu-img failed", n)
	}
}

func TestPrepare_StopsBeforeAnyCommandWhenTheImageIsWrong(t *testing.T) {
	m := newMirror(t)
	publish(m, []byte("qcow2 image"))
	m.put("/bookworm/latest/"+imageName, []byte("tampered"))
	runner := &fakeRunner{}
	root := t.TempDir()
	_, err := Prepare(context.Background(), labPlan(t, m), Options{
		RunDir:  filepath.Join(root, "run"),
		Fetcher: Fetcher{Client: m.server.Client(), CacheDir: filepath.Join(root, "cache")},
		Runner:  runner,
	})
	if err == nil || !strings.Contains(err.Error(), "sha512") {
		t.Errorf("error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("commands ran after a bad image: %q", runner.calls)
	}
}

func TestPrepare_RefusesARelativeRunDir(t *testing.T) {
	m := newMirror(t)
	publish(m, []byte("qcow2 image"))
	runner := &fakeRunner{}
	_, err := Prepare(context.Background(), labPlan(t, m), Options{
		RunDir:  "run",
		Fetcher: Fetcher{Client: m.server.Client(), CacheDir: t.TempDir()},
		Runner:  runner,
	})
	if err == nil || !strings.Contains(err.Error(), `run dir "run" must be an absolute path`) {
		t.Errorf("error = %v", err)
	}
	if n := m.count("/bookworm/latest/SHA512SUMS"); n != 0 {
		t.Errorf("sums read %d times before the run dir was checked", n)
	}
}

func TestPrepare_RefusesAnUndeclaredImage(t *testing.T) {
	m := newMirror(t)
	p := labPlan(t, m)
	p.Images = nil
	_, err := Prepare(context.Background(), p, Options{RunDir: t.TempDir(), Runner: &fakeRunner{}})
	if err == nil || !strings.Contains(err.Error(), `node sw1: image "debian12" is not declared`) {
		t.Errorf("error = %v", err)
	}
}

func TestEnsureKey_GeneratesOnlyOnce(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeRunner{}

	for i := 0; i < 2; i++ {
		key, err := EnsureKey(context.Background(), runner, dir)
		if err != nil || key != generatedKey {
			t.Fatalf("EnsureKey #%d = %q, %v", i+1, key, err)
		}
	}
	if n := len(runner.commands("ssh-keygen")); n != 1 {
		t.Errorf("ssh-keygen ran %d times, want 1", n)
	}
}

func TestEnsureKey_Rejections(t *testing.T) {
	cases := map[string]string{
		"empty":       "\n",
		"two keys":    generatedKey + "\n" + generatedKey + "\n",
		"stray break": "ssh-ed25519\rAAAA",
	}
	for name, pub := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "lab_ed25519"), []byte("private"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "lab_ed25519.pub"), []byte(pub), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := EnsureKey(context.Background(), &fakeRunner{}, dir); err == nil || !strings.Contains(err.Error(), "not a single public key") {
				t.Errorf("error = %v", err)
			}
		})
	}
}

func TestEnsureKey_ReportsAFailedGeneration(t *testing.T) {
	if _, err := EnsureKey(context.Background(), &fakeRunner{fail: "ssh-keygen"}, t.TempDir()); err == nil || !strings.Contains(err.Error(), "ssh-keygen failed") {
		t.Errorf("error = %v", err)
	}
}

func TestStage_RefusesARelativeBaseImage(t *testing.T) {
	runner := &fakeRunner{}
	err := Stage(context.Background(), runner, render.Node{Name: "hv1", Dir: t.TempDir()}, "debian.qcow2")
	if err == nil || !strings.Contains(err.Error(), "must be an absolute path") {
		t.Errorf("error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("commands ran: %q", runner.calls)
	}
}

func TestExecRunner_ReportsTheCommandOutput(t *testing.T) {
	err := ExecRunner{}.Run(context.Background(), "sh", "-c", "echo boom >&2; exit 3")
	if err == nil || !strings.Contains(err.Error(), "sh: exit status 3: boom") {
		t.Errorf("error = %v", err)
	}
	if err := (ExecRunner{}).Run(context.Background(), "true"); err != nil {
		t.Errorf("true: %v", err)
	}
}

func TestEnsureKey_WithTheRealSSHKeygen(t *testing.T) {
	dir := t.TempDir()
	key, err := EnsureKey(context.Background(), ExecRunner{}, dir)
	if err != nil {
		t.Fatalf("EnsureKey: %v", err)
	}
	if !strings.HasPrefix(key, "ssh-ed25519 ") || !strings.HasSuffix(key, " two-lab") {
		t.Errorf("key = %q", key)
	}
	info, err := os.Stat(filepath.Join(dir, "lab_ed25519"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("private key mode = %v, %v", info.Mode().Perm(), err)
	}
}
