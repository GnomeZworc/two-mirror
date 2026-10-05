package render

import (
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"git.g3e.fr/syonad/two/internal/lab/topology"
)

const labKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFG/JMmjfko96WkJV8DiL6rip/H/q/R++y8s27Z+Cj6O two-lab-automation"

const twoHypervisors = `name: evpn-2hv
images:
  deb:
    url: https://example.invalid/deb.qcow2
    sums: https://example.invalid/SHA512SUMS
segments:
  underlay: { switch: sw1, cidr: 10.250.0.0/24, mtu: 9000 }
nodes:
  sw1: { role: switch,     image: deb, cpus: 2, memory: 1024 }
  rr1: { role: rr,         image: deb, cpus: 1, memory: 1024, segments: [underlay] }
  hv1: { role: hypervisor, image: deb, cpus: 4, memory: 16384, segments: [underlay], release: 0.2.0rc002 }
  hv2: { role: hypervisor, image: deb, cpus: 4, memory: 16384, segments: [underlay], release: 0.2.0rc002 }
`

const twoSegments = `name: two-seg
images:
  deb:
    url: https://example.invalid/deb.qcow2
    sums: https://example.invalid/SHA512SUMS
segments:
  red:  { switch: sw, cidr: 10.1.0.0/24 }
  blue: { switch: sw, cidr: 10.2.0.0/24, mtu: 1500 }
nodes:
  sw: { role: switch, image: deb, cpus: 1, memory: 512 }
  hv: { role: hypervisor, image: deb, cpus: 1, memory: 512, segments: [blue, red], release: 0.2.0rc002 }
`

func plan(t *testing.T, doc string) *topology.Plan {
	t.Helper()
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

func renderAll(t *testing.T, doc string) map[string]Node {
	t.Helper()
	nodes, err := Render(plan(t, doc), Options{RunDir: "/srv/lab", AuthorizedKeys: []string{labKey}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := map[string]Node{}
	for _, n := range nodes {
		out[n.Name] = n
	}
	return out
}

func nodeNamed(t *testing.T, nodes map[string]Node, name string) Node {
	t.Helper()
	n, ok := nodes[name]
	if !ok {
		t.Fatalf("no rendered node %s", name)
	}
	return n
}

func value(t *testing.T, args []string, flag string) []string {
	t.Helper()
	var out []string
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			out = append(out, args[i+1])
		}
	}
	if len(out) == 0 {
		t.Fatalf("no %s in %v", flag, args)
	}
	return out
}

func hasValue(args []string, flag, want string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == want {
			return true
		}
	}
	return false
}

func network(t *testing.T, n Node) networkDoc {
	t.Helper()
	var doc networkDoc
	if err := yaml.Unmarshal(n.NetworkConfig, &doc); err != nil {
		t.Fatalf("network-config of %s is not yaml: %v", n.Name, err)
	}
	return doc
}

func iface(t *testing.T, doc networkDoc, name string) ethernet {
	t.Helper()
	e, ok := doc.Ethernets[name]
	if !ok {
		t.Fatalf("no interface %s in %v", name, doc.Ethernets)
	}
	return e
}

func user(t *testing.T, n Node) cloudConfig {
	t.Helper()
	if !strings.HasPrefix(string(n.UserData), "#cloud-config\n") {
		t.Fatalf("user-data of %s does not start with #cloud-config", n.Name)
	}
	var cfg cloudConfig
	if err := yaml.Unmarshal(n.UserData, &cfg); err != nil {
		t.Fatalf("user-data of %s is not yaml: %v", n.Name, err)
	}
	return cfg
}

func fileAt(t *testing.T, cfg cloudConfig, path string) writeFile {
	t.Helper()
	for _, f := range cfg.WriteFiles {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("no write_files entry %s", path)
	return writeFile{}
}

func TestQEMU_HypervisorArguments(t *testing.T) {
	hv := nodeNamed(t, renderAll(t, twoHypervisors), "hv1")
	for flag, want := range map[string]string{
		"-name":    "hv1",
		"-machine": "q35",
		"-accel":   "kvm",
		"-cpu":     "host",
		"-smp":     "4",
		"-m":       "16384",
		"-display": "none",
		"-serial":  "file:/srv/lab/hv1/console.log",
		"-qmp":     "unix:/srv/lab/hv1/qmp.sock,server=on,wait=off",
		"-pidfile": "/srv/lab/hv1/qemu.pid",
	} {
		if !hasValue(hv.QEMU, flag, want) {
			t.Errorf("%s %s missing from %v", flag, want, hv.QEMU)
		}
	}
	for _, want := range []string{
		"file=/srv/lab/hv1/disk.qcow2,if=virtio,format=qcow2",
		"file=/srv/lab/hv1/seed.iso,media=cdrom,readonly=on",
	} {
		if !hasValue(hv.QEMU, "-drive", want) {
			t.Errorf("-drive %s missing", want)
		}
	}
	if hv.Dir != "/srv/lab/hv1" {
		t.Errorf("Dir = %s", hv.Dir)
	}
	nodefaults := 0
	for _, a := range hv.QEMU {
		if a == "-nodefaults" {
			nodefaults++
		}
	}
	if nodefaults != 1 {
		t.Errorf("-nodefaults appears %d times", nodefaults)
	}
}

func TestQEMU_HypervisorNetwork(t *testing.T) {
	hv := nodeNamed(t, renderAll(t, twoHypervisors), "hv1")
	wantNetdevs := []string{
		"user,id=mgmt0,restrict=on,ipv6=off,hostfwd=tcp:127.0.0.1:2202-:22",
		"dgram,id=underlay,local.type=inet,local.host=127.0.0.1,local.port=20002,remote.type=inet,remote.host=127.0.0.1,remote.port=20003",
	}
	wantDevices := []string{
		"virtio-net-pci,netdev=mgmt0,mac=02:4d:00:02:00:00,romfile=",
		"virtio-net-pci,netdev=underlay,mac=02:4c:00:02:00:00,host_mtu=9000,romfile=",
	}
	if got := value(t, hv.QEMU, "-netdev"); !reflect.DeepEqual(got, wantNetdevs) {
		t.Errorf("netdevs = %v\nwant %v", got, wantNetdevs)
	}
	if got := value(t, hv.QEMU, "-device"); !reflect.DeepEqual(got, wantDevices) {
		t.Errorf("devices = %v\nwant %v", got, wantDevices)
	}
}

func TestQEMU_SwitchNetwork(t *testing.T) {
	sw := nodeNamed(t, renderAll(t, twoHypervisors), "sw1")
	wantNetdevs := []string{
		"user,id=mgmt0,restrict=off,ipv6=off,hostfwd=tcp:127.0.0.1:2200-:22",
		"dgram,id=p0,local.type=inet,local.host=127.0.0.1,local.port=20001,remote.type=inet,remote.host=127.0.0.1,remote.port=20000",
		"dgram,id=p1,local.type=inet,local.host=127.0.0.1,local.port=20003,remote.type=inet,remote.host=127.0.0.1,remote.port=20002",
		"dgram,id=p2,local.type=inet,local.host=127.0.0.1,local.port=20005,remote.type=inet,remote.host=127.0.0.1,remote.port=20004",
	}
	if got := value(t, sw.QEMU, "-netdev"); !reflect.DeepEqual(got, wantNetdevs) {
		t.Errorf("netdevs = %v\nwant %v", got, wantNetdevs)
	}
	if !hasValue(sw.QEMU, "-device", "virtio-net-pci,netdev=p2,mac=02:4c:00:03:00:01,host_mtu=9000,romfile=") {
		t.Errorf("p2 device missing: %v", sw.QEMU)
	}
}

func TestQEMU_OnlyTheSwitchReachesTheOutsideThroughAdministration(t *testing.T) {
	for name, n := range renderAll(t, twoHypervisors) {
		admin := value(t, n.QEMU, "-netdev")[0]
		wantRestrict := "restrict=on"
		if name == "sw1" {
			wantRestrict = "restrict=off"
		}
		if !strings.Contains(admin, ","+wantRestrict+",") {
			t.Errorf("%s admin netdev %q, want %s", name, admin, wantRestrict)
		}
		if !strings.Contains(admin, "hostfwd=tcp:127.0.0.1:") {
			t.Errorf("%s ssh forward not bound to loopback: %q", name, admin)
		}
	}
}

func TestQEMU_EveryCableEndsMatch(t *testing.T) {
	ends := map[string]int{}
	for _, n := range renderAll(t, twoHypervisors) {
		for _, nd := range value(t, n.QEMU, "-netdev") {
			if !strings.HasPrefix(nd, "dgram,") {
				continue
			}
			var local, remote string
			for _, kv := range strings.Split(nd, ",") {
				if v, ok := strings.CutPrefix(kv, "local.port="); ok {
					local = v
				}
				if v, ok := strings.CutPrefix(kv, "remote.port="); ok {
					remote = v
				}
			}
			ends[local+">"+remote]++
		}
	}
	for pair, count := range ends {
		local, remote, _ := strings.Cut(pair, ">")
		if count != 1 || ends[remote+">"+local] != 1 {
			t.Errorf("cable end %s has no single matching end", pair)
		}
	}
	if len(ends) != 6 {
		t.Errorf("%d cable ends, want 6", len(ends))
	}
}

func TestNetworkConfig_Hypervisor(t *testing.T) {
	doc := network(t, nodeNamed(t, renderAll(t, twoHypervisors), "hv1"))
	if doc.Version != 2 || len(doc.Ethernets) != 2 {
		t.Fatalf("network-config = %+v", doc)
	}
	admin := iface(t, doc, "mgmt0")
	if admin.Match.MACAddress != "02:4d:00:02:00:00" || admin.SetName != "mgmt0" ||
		!reflect.DeepEqual(admin.Addresses, []string{"10.0.2.15/24"}) || len(admin.Routes) != 0 || admin.Nameservers != nil {
		t.Errorf("mgmt0 = %+v", admin)
	}
	under := iface(t, doc, "underlay")
	if under.Match.MACAddress != "02:4c:00:02:00:00" || under.SetName != "underlay" || under.MTU != 9000 ||
		!reflect.DeepEqual(under.Addresses, []string{"10.250.0.3/24"}) {
		t.Errorf("underlay = %+v", under)
	}
	if !reflect.DeepEqual(under.Routes, []route{{To: "0.0.0.0/0", Via: "10.250.0.1"}}) {
		t.Errorf("underlay routes = %+v", under.Routes)
	}
	if under.Nameservers == nil || !reflect.DeepEqual(under.Nameservers.Addresses, []string{"1.1.1.1", "8.8.8.8"}) {
		t.Errorf("underlay nameservers = %+v", under.Nameservers)
	}
}

func TestNetworkConfig_DHCPIsExplicitlyOffEverywhere(t *testing.T) {
	for name, n := range renderAll(t, twoHypervisors) {
		if c := strings.Count(string(n.NetworkConfig), "dhcp4: false"); c != len(network(t, n).Ethernets) {
			t.Errorf("%s: %d explicit dhcp4: false for %d interfaces", name, c, len(network(t, n).Ethernets))
		}
	}
}

func TestNetworkConfig_Switch(t *testing.T) {
	doc := network(t, nodeNamed(t, renderAll(t, twoHypervisors), "sw1"))
	admin := iface(t, doc, "mgmt0")
	if !reflect.DeepEqual(admin.Routes, []route{{To: "0.0.0.0/0", Via: "10.0.2.2"}}) ||
		admin.Nameservers == nil || !reflect.DeepEqual(admin.Nameservers.Addresses, []string{"10.0.2.3"}) {
		t.Errorf("switch mgmt0 = %+v", admin)
	}
	for port, mac := range map[string]string{"p0": "02:4c:00:01:00:01", "p1": "02:4c:00:02:00:01", "p2": "02:4c:00:03:00:01"} {
		e := iface(t, doc, port)
		if e.Match.MACAddress != mac || e.SetName != port || e.MTU != 9000 || len(e.Addresses) != 0 || len(e.Routes) != 0 {
			t.Errorf("%s = %+v", port, e)
		}
	}
}

func TestNetworkConfig_DefaultRouteOnlyOnFirstSegment(t *testing.T) {
	doc := network(t, nodeNamed(t, renderAll(t, twoSegments), "hv"))
	red, blue := iface(t, doc, "red"), iface(t, doc, "blue")
	if !reflect.DeepEqual(red.Routes, []route{{To: "0.0.0.0/0", Via: "10.1.0.1"}}) || red.Nameservers == nil {
		t.Errorf("red = %+v", red)
	}
	if len(blue.Routes) != 0 || blue.Nameservers != nil || blue.MTU != 1500 {
		t.Errorf("blue = %+v", blue)
	}
}

func TestUserData_Hypervisor(t *testing.T) {
	cfg := user(t, nodeNamed(t, renderAll(t, twoHypervisors), "hv1"))
	if cfg.Hostname != "hv1" || !cfg.DisableRoot || len(cfg.Packages) != 0 || !reflect.DeepEqual(cfg.Runcmd, [][]string{{"/usr/local/sbin/lab-provision"}}) {
		t.Errorf("hv1 user-data = %+v", cfg)
	}
	if !reflect.DeepEqual(cfg.SSHAuthorizedKeys, []string{labKey}) {
		t.Errorf("keys = %v", cfg.SSHAuthorizedKeys)
	}
}

func TestUserData_PasswordLoginIsExplicitlyOff(t *testing.T) {
	for name, n := range renderAll(t, twoHypervisors) {
		if !strings.Contains(string(n.UserData), "\nssh_pwauth: false\n") || !strings.Contains(string(n.UserData), "\ndisable_root: true\n") {
			t.Errorf("%s user-data:\n%s", name, n.UserData)
		}
	}
}

func TestUserData_SwitchBuildsBridgeGatewayAndNAT(t *testing.T) {
	cfg := user(t, nodeNamed(t, renderAll(t, twoHypervisors), "sw1"))
	if !reflect.DeepEqual(cfg.Packages, []string{"nftables"}) {
		t.Errorf("packages = %v", cfg.Packages)
	}
	script := fileAt(t, cfg, "/usr/local/sbin/lab-switch")
	if script.Permissions != "0755" {
		t.Errorf("script permissions = %s", script.Permissions)
	}
	wantScript := `#!/bin/sh
set -eu
sysctl -qw net.ipv4.ip_forward=1
ip link add br-underlay type bridge stp_state 0 2>/dev/null || true
ip link set dev p0 master br-underlay
ip link set dev p0 up
ip link set dev p1 master br-underlay
ip link set dev p1 up
ip link set dev p2 master br-underlay
ip link set dev p2 up
ip link set dev br-underlay mtu 9000
ip addr replace 10.250.0.1/24 dev br-underlay
ip link set dev br-underlay up
nft -f /etc/lab-switch.nft
`
	if script.Content != wantScript {
		t.Errorf("script:\n%s\nwant:\n%s", script.Content, wantScript)
	}
	nft := fileAt(t, cfg, "/etc/lab-switch.nft").Content
	for _, want := range []string{
		"add table ip lab_nat\ndelete table ip lab_nat\n",
		"type nat hook postrouting priority srcnat;",
		`ip saddr { 10.250.0.0/24 } oifname "mgmt0" masquerade`,
	} {
		if !strings.Contains(nft, want) {
			t.Errorf("nft rules miss %q:\n%s", want, nft)
		}
	}
	unit := fileAt(t, cfg, "/etc/systemd/system/lab-switch.service").Content
	for _, want := range []string{"Type=oneshot", "RemainAfterExit=yes", "ExecStart=/usr/local/sbin/lab-switch", "After=network-online.target", "WantedBy=multi-user.target"} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit misses %q", want)
		}
	}
	if !reflect.DeepEqual(cfg.Runcmd, [][]string{{"/usr/local/sbin/lab-provision"}}) {
		t.Errorf("runcmd = %v", cfg.Runcmd)
	}
	if got := fileAt(t, cfg, "/usr/local/sbin/lab-provision").Content; got != "#!/bin/sh\nset -eu\nsystemctl daemon-reload\nsystemctl enable --now lab-switch.service\n" {
		t.Errorf("lab-provision:\n%s", got)
	}
}

func TestUserData_SwitchWithTwoSegments(t *testing.T) {
	cfg := user(t, nodeNamed(t, renderAll(t, twoSegments), "sw"))
	script := fileAt(t, cfg, "/usr/local/sbin/lab-switch").Content
	for _, want := range []string{
		"ip link set dev p0 master br-red\n",
		"ip link set dev p1 master br-blue\n",
		"ip link set dev br-blue mtu 1500\n",
		"ip addr replace 10.2.0.1/24 dev br-blue\n",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script misses %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, "p1 master br-red") || strings.Contains(script, "p0 master br-blue") {
		t.Errorf("port bridged on the wrong segment:\n%s", script)
	}
	if nft := fileAt(t, cfg, "/etc/lab-switch.nft").Content; !strings.Contains(nft, "ip saddr { 10.1.0.0/24, 10.2.0.0/24 }") {
		t.Errorf("nft:\n%s", nft)
	}
}

func TestMetaData(t *testing.T) {
	var doc metaDoc
	if err := yaml.Unmarshal(nodeNamed(t, renderAll(t, twoHypervisors), "hv2").MetaData, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.InstanceID != "evpn-2hv-hv2" || doc.LocalHostname != "hv2" {
		t.Errorf("meta-data = %+v", doc)
	}
}

func TestRender_AdminMacsAreUniqueAndApartFromCableMacs(t *testing.T) {
	seen := map[string]string{}
	for name, n := range renderAll(t, twoHypervisors) {
		for _, d := range value(t, n.QEMU, "-device") {
			for _, kv := range strings.Split(d, ",") {
				if mac, ok := strings.CutPrefix(kv, "mac="); ok {
					if other, dup := seen[mac]; dup {
						t.Errorf("mac %s used by %s and %s", mac, other, name)
					}
					seen[mac] = name
				}
			}
		}
	}
	if len(seen) != 10 {
		t.Errorf("%d distinct macs, want 10", len(seen))
	}
}

func TestRender_IsStable(t *testing.T) {
	first := renderAll(t, twoHypervisors)
	for i := 0; i < 20; i++ {
		if again := renderAll(t, twoHypervisors); !reflect.DeepEqual(first, again) {
			t.Fatalf("render differs on run %d", i)
		}
	}
}

func TestRender_RejectsBadOptions(t *testing.T) {
	p := plan(t, twoHypervisors)
	cases := []struct {
		name string
		opts Options
		want string
	}{
		{"relative run dir", Options{RunDir: "lab", AuthorizedKeys: []string{labKey}}, `run dir "lab" must be an absolute path`},
		{"no key", Options{RunDir: "/srv/lab"}, "at least one authorized ssh key"},
		{"multi-line key", Options{RunDir: "/srv/lab", AuthorizedKeys: []string{labKey + "\nssh-rsa AAAA x"}}, "authorized key 1 spans several lines"},
		{"not a key", Options{RunDir: "/srv/lab", AuthorizedKeys: []string{"hello world"}}, "authorized key 1 is not an ssh public key"},
		{"type only", Options{RunDir: "/srv/lab", AuthorizedKeys: []string{"ssh-ed25519"}}, "authorized key 1 is not an ssh public key"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Render(p, c.opts)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
		})
	}
}

func TestRender_AcceptsUsualKeyTypes(t *testing.T) {
	p := plan(t, twoHypervisors)
	for _, k := range []string{
		"ssh-rsa AAAAB3NzaC1yc2E user",
		"ecdsa-sha2-nistp256 AAAAE2VjZHNh user",
		"sk-ssh-ed25519@openssh.com AAAAGnNr user",
	} {
		if _, err := Render(p, Options{RunDir: "/srv/lab", AuthorizedKeys: []string{k}}); err != nil {
			t.Errorf("%s rejected: %v", strings.Fields(k)[0], err)
		}
	}
}
