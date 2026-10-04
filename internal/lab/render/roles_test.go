package render

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"reflect"
	"strings"
	"testing"
)

const withRoles = `name: evpn-2hv
images:
  deb:
    url: https://example.invalid/deb.qcow2
    sums: https://example.invalid/SHA512SUMS
segments:
  underlay: { switch: sw1, cidr: 192.168.14.0/24 }
nodes:
  sw1: { role: switch, image: deb, cpus: 2, memory: 1024, secondary: { underlay: [169.254.0.1/28] }, frr: sw1.conf }
  rr1: { role: rr, image: deb, cpus: 1, memory: 1024, segments: [underlay], secondary: { underlay: [169.254.0.3/28] }, loopback: 10.255.255.1/32, frr: rr1.conf }
  hv1: { role: hypervisor, image: deb, cpus: 4, memory: 16384, segments: [underlay], frr: hv1.conf, release: 0.2.0rc002 }
  hv2: { role: hypervisor, image: deb, cpus: 4, memory: 16384, segments: [underlay], release: 0.2.0rc002 }
`

var frrConfigs = map[string]string{
	"sw1": "hostname sw1\nrouter bgp 65100\n",
	"rr1": "hostname rr1\nrouter bgp 65000\n",
	"hv1": "hostname hv1\nrouter bgp 64600\n",
}

func renderRoles(t *testing.T) map[string]Node {
	t.Helper()
	nodes, err := Render(plan(t, withRoles), Options{RunDir: "/srv/lab", AuthorizedKeys: []string{labKey}, FRR: frrConfigs})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := map[string]Node{}
	for _, n := range nodes {
		out[n.Name] = n
	}
	return out
}

func runcmd(t *testing.T, n Node) []string {
	t.Helper()
	var out []string
	for _, c := range user(t, n).Runcmd {
		out = append(out, strings.Join(c, " "))
	}
	return out
}

const frrScriptHead = `#!/bin/sh
set -eu
export DEBIAN_FRONTEND=noninteractive
. /etc/os-release
echo "deb [signed-by=/usr/share/keyrings/frrouting.gpg] https://deb.frrouting.org/frr ${VERSION_CODENAME} frr-stable" > /etc/apt/sources.list.d/frr.list
n=0
until apt-get update -qq --error-on=any; do
    n=$((n + 1))
    [ "$n" -lt 30 ] || exit 1
    sleep 10
done
apt-get install -y -qq --no-install-recommends frr frr-pythontools
sed -i 's/^bgpd=no/bgpd=yes/' /etc/frr/daemons
`

func TestRoles_SecondaryAddressFollowsThePrimaryOnTheNode(t *testing.T) {
	doc := network(t, nodeNamed(t, renderRoles(t), "rr1"))
	got := iface(t, doc, "underlay").Addresses
	if !reflect.DeepEqual(got, []string{"192.168.14.2/24", "169.254.0.3/28"}) {
		t.Errorf("rr1 underlay addresses = %v", got)
	}
}

func TestRoles_SwitchCarriesItsSecondaryOnTheBridge(t *testing.T) {
	script := fileAt(t, user(t, nodeNamed(t, renderRoles(t), "sw1")), "/usr/local/sbin/lab-switch").Content
	want := "ip addr replace 192.168.14.1/24 dev br-underlay\nip addr replace 169.254.0.1/28 dev br-underlay\nip link set dev br-underlay up\n"
	if !strings.Contains(script, want) {
		t.Errorf("lab-switch:\n%s\ndoes not contain:\n%s", script, want)
	}
}

func TestRoles_LoopbackOnADummyInterfaceReplayedAtBoot(t *testing.T) {
	rr1 := nodeNamed(t, renderRoles(t), "rr1")
	cfg := user(t, rr1)

	script := fileAt(t, cfg, "/usr/local/sbin/lab-node")
	want := "#!/bin/sh\nset -eu\nip link add lo1 type dummy 2>/dev/null || true\nip addr replace 10.255.255.1/32 dev lo1\nip link set dev lo1 up\n"
	if script.Content != want || script.Permissions != "0755" {
		t.Errorf("lab-node (%s):\n%s", script.Permissions, script.Content)
	}
	unit := fileAt(t, cfg, "/etc/systemd/system/lab-node.service").Content
	if !strings.Contains(unit, "ExecStart=/usr/local/sbin/lab-node\n") || !strings.Contains(unit, "WantedBy=multi-user.target\n") {
		t.Errorf("lab-node.service:\n%s", unit)
	}
	want = "#!/bin/sh\nset -eu\nsystemctl daemon-reload\nsystemctl enable --now lab-node.service\n/usr/local/sbin/lab-frr\n"
	if got := fileAt(t, cfg, "/usr/local/sbin/lab-provision").Content; got != want {
		t.Errorf("lab-provision:\n%s\nwant:\n%s", got, want)
	}
}

func TestRoles_FRRConfigIsWrittenVerbatimAndPrivately(t *testing.T) {
	for _, name := range []string{"sw1", "rr1", "hv1"} {
		f := fileAt(t, user(t, nodeNamed(t, renderRoles(t), name)), "/etc/lab/frr.conf")
		if f.Content != frrConfigs[name] || f.Permissions != "0640" {
			t.Errorf("%s frr.conf (%s) = %q", name, f.Permissions, f.Content)
		}
	}
}

func TestRoles_FRRDaemonsDependOnTheRole(t *testing.T) {
	tail := "install -o frr -g frr -m 0640 /etc/lab/frr.conf /etc/frr/frr.conf\nsystemctl restart frr\n"
	bfd := "sed -i 's/^bfdd=no/bfdd=yes/' /etc/frr/daemons\n"
	nodes := renderRoles(t)
	for name, want := range map[string]string{
		"sw1": frrScriptHead + bfd + tail,
		"rr1": frrScriptHead + bfd + tail,
		"hv1": frrScriptHead + tail,
	} {
		f := fileAt(t, user(t, nodeNamed(t, nodes, name)), "/usr/local/sbin/lab-frr")
		if f.Content != want || f.Permissions != "0755" {
			t.Errorf("%s lab-frr (%s):\n%s\nwant:\n%s", name, f.Permissions, f.Content, want)
		}
	}
}

func TestRoles_FRRKeyIsThePinnedRepositoryKey(t *testing.T) {
	f := fileAt(t, user(t, nodeNamed(t, renderRoles(t), "hv1")), "/usr/share/keyrings/frrouting.gpg")
	if f.Encoding != "b64" || f.Permissions != "0644" {
		t.Errorf("key file: encoding %q, permissions %q", f.Encoding, f.Permissions)
	}
	key, err := base64.StdEncoding.DecodeString(f.Content)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(key)
	if got := hex.EncodeToString(sum[:]); got != "bf10935b9296e2ce7c5d9855fa29ef30c35810b0fc4b1f53005494a04a33554d" {
		t.Errorf("key sha256 = %s", got)
	}
}

func TestRoles_ProvisioningIsOneScriptThatStopsAtTheFirstFailure(t *testing.T) {
	nodes := renderRoles(t)
	for name, steps := range map[string]string{
		"sw1": "systemctl daemon-reload\nsystemctl enable --now lab-switch.service\n/usr/local/sbin/lab-frr\n",
		"hv1": "/usr/local/sbin/lab-deploy\n/usr/local/sbin/lab-frr\n",
		"hv2": "/usr/local/sbin/lab-deploy\n",
	} {
		cfg := user(t, nodeNamed(t, nodes, name))
		if !reflect.DeepEqual(cfg.Runcmd, [][]string{{"/usr/local/sbin/lab-provision"}}) {
			t.Errorf("%s runcmd = %q", name, cfg.Runcmd)
		}
		f := fileAt(t, cfg, "/usr/local/sbin/lab-provision")
		if f.Content != "#!/bin/sh\nset -eu\n"+steps || f.Permissions != "0755" {
			t.Errorf("%s lab-provision (%s):\n%s", name, f.Permissions, f.Content)
		}
	}
}

func TestRoles_HypervisorDeploysTheReleaseThroughTheRepositoryScripts(t *testing.T) {
	cfg := user(t, nodeNamed(t, renderRoles(t), "hv1"))

	f := fileAt(t, cfg, "/usr/local/sbin/lab-deploy")
	want := `#!/bin/sh
set -eu
n=0
until curl -fsS -o /dev/null https://git.g3e.fr/; do
    n=$((n + 1))
    [ "$n" -lt 30 ] || exit 1
    sleep 10
done
cd /opt/two/scripts
bash ./deploy.sh --noup_script -i -u underlay -t 0.2.0rc002
`
	if f.Content != want || f.Permissions != "0755" {
		t.Errorf("lab-deploy (%s):\n%s\nwant:\n%s", f.Permissions, f.Content, want)
	}

	for path, source := range map[string]string{
		"/opt/two/scripts/deploy.sh":        "../../../scripts/deploy.sh",
		"/opt/two/scripts/bootstrap_kvm.sh": "../../../scripts/bootstrap_kvm.sh",
	} {
		f := fileAt(t, cfg, path)
		got, err := base64.StdEncoding.DecodeString(f.Content)
		if err != nil {
			t.Fatal(err)
		}
		repo, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if f.Encoding != "b64" || f.Permissions != "0755" || !bytes.Equal(got, repo) {
			t.Errorf("%s: encoding %q, permissions %q, identical to %s: %v", path, f.Encoding, f.Permissions, source, bytes.Equal(got, repo))
		}
	}
}

func TestRoles_UplinkIsTheInterfaceCarryingTheDefaultRoute(t *testing.T) {
	hv := nodeNamed(t, renderAll(t, twoSegments), "hv")
	var withDefault []string
	for name, e := range network(t, hv).Ethernets {
		for _, r := range e.Routes {
			if r.To == "0.0.0.0/0" {
				withDefault = append(withDefault, name)
			}
		}
	}
	if !reflect.DeepEqual(withDefault, []string{"red"}) {
		t.Fatalf("default route on %v, want [red]", withDefault)
	}
	if got := fileAt(t, user(t, hv), "/usr/local/sbin/lab-deploy").Content; !strings.HasSuffix(got, "bash ./deploy.sh --noup_script -i -u red -t 0.2.0rc002\n") {
		t.Errorf("lab-deploy:\n%s", got)
	}
}

func TestRoles_OnlyHypervisorsDeployTwo(t *testing.T) {
	nodes := renderRoles(t)
	for _, name := range []string{"sw1", "rr1"} {
		for _, f := range user(t, nodeNamed(t, nodes, name)).WriteFiles {
			if strings.HasPrefix(f.Path, "/opt/two/") || f.Path == "/usr/local/sbin/lab-deploy" {
				t.Errorf("%s receives %s", name, f.Path)
			}
		}
	}
}

func TestRoles_NodeWithoutRoleFieldsGetsNoFRR(t *testing.T) {
	for _, f := range user(t, nodeNamed(t, renderRoles(t), "hv2")).WriteFiles {
		if strings.Contains(f.Path, "frr") {
			t.Errorf("hv2 receives %s", f.Path)
		}
	}
}

func TestRoles_RefusesAnUnreadFRRConfig(t *testing.T) {
	configs := map[string]string{"sw1": "x", "rr1": "y"}
	_, err := Render(plan(t, withRoles), Options{RunDir: "/srv/lab", AuthorizedKeys: []string{labKey}, FRR: configs})
	if err == nil || err.Error() != "node hv1: frr configuration hv1.conf was not read" {
		t.Errorf("error = %v", err)
	}
}

func TestRoles_SwitchLoopbackIsCreatedByTheSwitchScript(t *testing.T) {
	doc := strings.Replace(withRoles, "secondary: { underlay: [169.254.0.1/28] }, frr: sw1.conf", "secondary: { underlay: [169.254.0.1/28] }, loopback: 10.255.254.1/32, frr: sw1.conf", 1)
	nodes, err := Render(plan(t, doc), Options{RunDir: "/srv/lab", AuthorizedKeys: []string{labKey}, FRR: frrConfigs})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	var sw1 Node
	for _, n := range nodes {
		if n.Name == "sw1" {
			sw1 = n
		}
	}
	script := fileAt(t, user(t, sw1), "/usr/local/sbin/lab-switch").Content
	want := "ip link set dev br-underlay up\nip link add lo1 type dummy 2>/dev/null || true\nip addr replace 10.255.254.1/32 dev lo1\nip link set dev lo1 up\nnft -f /etc/lab-switch.nft\n"
	if !strings.HasSuffix(script, want) {
		t.Errorf("lab-switch:\n%s\ndoes not end with:\n%s", script, want)
	}
}

func TestRoles_HypervisorAgentConfigIsWrittenBeforeTheDeployment(t *testing.T) {
	doc := strings.Replace(withRoles, "frr: hv1.conf, release: 0.2.0rc002 }", "frr: hv1.conf, release: 0.2.0rc002, agent: two.yml }", 1)
	nodes, err := Render(plan(t, doc), Options{RunDir: "/srv/lab", AuthorizedKeys: []string{labKey}, FRR: frrConfigs, Agent: map[string]string{"hv1": "dhcp:\n  backend: two\n"}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	var hv1, hv2 Node
	for _, n := range nodes {
		switch n.Name {
		case "hv1":
			hv1 = n
		case "hv2":
			hv2 = n
		}
	}
	f := fileAt(t, user(t, hv1), "/etc/two/agent.yml")
	if f.Content != "dhcp:\n  backend: two\n" || f.Permissions != "0640" {
		t.Errorf("agent.yml (%s) = %q", f.Permissions, f.Content)
	}
	for _, w := range user(t, hv2).WriteFiles {
		if w.Path == "/etc/two/agent.yml" {
			t.Error("hv2 receives an agent.yml it does not declare")
		}
	}
}

func TestRoles_RefusesAnUnreadAgentConfig(t *testing.T) {
	doc := strings.Replace(withRoles, "frr: hv1.conf, release: 0.2.0rc002 }", "frr: hv1.conf, release: 0.2.0rc002, agent: two.yml }", 1)
	_, err := Render(plan(t, doc), Options{RunDir: "/srv/lab", AuthorizedKeys: []string{labKey}, FRR: frrConfigs})
	if err == nil || err.Error() != "node hv1: agent configuration two.yml was not read" {
		t.Errorf("error = %v", err)
	}
}
