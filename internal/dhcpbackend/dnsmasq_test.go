package dhcpbackend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dnsmasqBackend(t *testing.T) Dnsmasq {
	t.Helper()
	return Dnsmasq{ConfDir: t.TempDir()}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

func TestDnsmasq_ConfDirDefaultsToTheSystemOne(t *testing.T) {
	if got := (Dnsmasq{}).confDir(); got != "/etc/dnsmasq.d" {
		t.Errorf("confDir = %s, want /etc/dnsmasq.d", got)
	}
}

func TestDnsmasqSetVM_WritesOneReservationPerInterface(t *testing.T) {
	b := dnsmasqBackend(t)
	s := testSubnet(t)

	res := []Reservation{
		{Index: 0, MAC: "00:22:33:00:00:0a", IP: "10.0.5.10", DefaultRoute: true},
		{Index: 1, MAC: "00:22:33:00:00:0b", IP: "10.0.5.11"},
	}
	if err := b.SetVM(s, "vm-web", res); err != nil {
		t.Fatalf("SetVM: %v", err)
	}

	hosts := readFile(t, filepath.Join(b.ConfDir, s.Instance()+".hosts.d", "vm-web"))
	for _, want := range []string{"00:22:33:00:00:0a,10.0.5.10,set:vm-web-0", "00:22:33:00:00:0b,10.0.5.11,set:vm-web-1"} {
		if !strings.Contains(hosts, want) {
			t.Errorf("hosts file missing %q:\n%s", want, hosts)
		}
	}
}

func TestDnsmasqSetVM_TagsOnlyTheInterfacesWithoutADefaultRoute(t *testing.T) {
	b := dnsmasqBackend(t)
	s := testSubnet(t)

	res := []Reservation{
		{Index: 0, MAC: "00:22:33:00:00:0a", IP: "10.0.5.10", DefaultRoute: true},
		{Index: 1, MAC: "00:22:33:00:00:0b", IP: "10.0.5.11"},
	}
	if err := b.SetVM(s, "vm-web", res); err != nil {
		t.Fatalf("SetVM: %v", err)
	}

	opts := readFile(t, filepath.Join(b.ConfDir, s.Instance()+".opts.d", "vm-web"))
	if strings.Contains(opts, "tag:vm-web-0") {
		t.Errorf("the interface carrying the default route must get no override:\n%s", opts)
	}
	if !strings.Contains(opts, "tag:vm-web-1,3\n") {
		t.Errorf("the secondary interface must get a bare option 3:\n%s", opts)
	}
	if !strings.Contains(opts, "tag:vm-web-1,121,") {
		t.Errorf("the secondary interface must get its own option 121:\n%s", opts)
	}
}

func TestDnsmasqSetVM_SecondaryOptionsKeepTheMetadataRoute(t *testing.T) {
	b := dnsmasqBackend(t)
	s := testSubnet(t)

	res := []Reservation{{Index: 1, MAC: "00:22:33:00:00:0b", IP: "10.0.5.11"}}
	if err := b.SetVM(s, "vm-web", res); err != nil {
		t.Fatalf("SetVM: %v", err)
	}

	opts := readFile(t, filepath.Join(b.ConfDir, s.Instance()+".opts.d", "vm-web"))
	if !strings.Contains(opts, "169.254.169.254/32,10.0.5.1") {
		t.Errorf("overriding option 121 without the metadata route breaks cloud-init:\n%s", opts)
	}
	if strings.Contains(opts, "0.0.0.0/0") {
		t.Errorf("a secondary interface must not receive a default route:\n%s", opts)
	}
}

func TestDnsmasqSetVM_AllInterfacesDefaultRoutedWritesNoOptions(t *testing.T) {
	b := dnsmasqBackend(t)
	s := testSubnet(t)

	res := []Reservation{{Index: 0, MAC: "00:22:33:00:00:0a", IP: "10.0.5.10", DefaultRoute: true}}
	if err := b.SetVM(s, "vm-web", res); err != nil {
		t.Fatalf("SetVM: %v", err)
	}

	path := filepath.Join(b.ConfDir, s.Instance()+".opts.d", "vm-web")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("no options file expected, got %v: the subnet-wide options already carry the default route", err)
	}
}

func TestDnsmasqSetVM_RejectsAVMWithoutReservation(t *testing.T) {
	b := dnsmasqBackend(t)

	if err := b.SetVM(testSubnet(t), "vm-web", nil); err == nil {
		t.Fatal("a vm with no reservation would get no address: that must be reported")
	}
}

func TestDnsmasqConfig_CarriesTheMTU(t *testing.T) {
	if c := (Dnsmasq{}).config(Subnet{VPC: "vp-s9", Bridge: "br-s9r1", MTU: 1450}); c.MTU != 1450 {
		t.Errorf("mtu = %d, want 1450", c.MTU)
	}
}

func TestDnsmasqConfig_ListenOnGatewayExcludesTheBridge(t *testing.T) {
	c := Dnsmasq{}.config(Subnet{VPC: "vp-s9", Bridge: "br-s9r1", Listen: "gw-s9r1"})
	if c.Listen != "gw-s9r1" || c.Except != "br-s9r1" {
		t.Errorf("Listen %q, Except %q : gw-s9r1 et br-s9r1 attendus", c.Listen, c.Except)
	}
	if c.Name != "vp-s9_br-s9r1" {
		t.Errorf("instance %q : le nom doit rester celui du bridge", c.Name)
	}
}

func TestDnsmasqConfig_NoListenKeepsTheCommandLineInterface(t *testing.T) {
	for _, listen := range []string{"", "br-s9r1"} {
		c := Dnsmasq{}.config(Subnet{VPC: "vp-s9", Bridge: "br-s9r1", Listen: listen})
		if c.Listen != "" || c.Except != "" {
			t.Errorf("Listen=%q : Listen %q, Except %q, vides attendus", listen, c.Listen, c.Except)
		}
	}
}
