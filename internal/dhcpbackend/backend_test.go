package dhcpbackend

import (
	"net"
	"testing"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
)

func testSubnet(t *testing.T) Subnet {
	t.Helper()
	_, network, err := net.ParseCIDR("10.0.5.0/24")
	if err != nil {
		t.Fatalf("ParseCIDR: %v", err)
	}
	_, vpcRoute, err := net.ParseCIDR("10.0.0.0/16")
	if err != nil {
		t.Fatalf("ParseCIDR: %v", err)
	}
	return Subnet{
		Name:           "sn-000001",
		VPC:            "vp-admin",
		Bridge:         "br-000001",
		Network:        network,
		InterfaceIP:    net.ParseIP("10.0.5.1"),
		VPCRoute:       vpcRoute,
		DefaultGateway: net.ParseIP("10.0.5.254"),
	}
}

func configFor(backend string) *configuration.Config {
	cfg := &configuration.Config{}
	cfg.DHCP.Backend = backend
	return cfg
}

func TestInstance_JoinsVPCAndBridge(t *testing.T) {
	if got := testSubnet(t).Instance(); got != "vp-admin_br-000001" {
		t.Errorf("Instance = %s, want vp-admin_br-000001", got)
	}
}

func TestNew_DnsmasqIsTheDefault(t *testing.T) {
	backend, err := New(configFor(configuration.BackendDnsmasq))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := backend.(Dnsmasq); !ok {
		t.Errorf("backend = %T, want Dnsmasq", backend)
	}
}

func TestNew_ReturnsTheTwoBackendWhenAsked(t *testing.T) {
	backend, err := New(configFor(configuration.BackendTwo))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := backend.(Two); !ok {
		t.Errorf("backend = %T, want Two", backend)
	}
}

func TestNew_RejectsAnUnknownBackend(t *testing.T) {
	if _, err := New(configFor("dhcpd")); err == nil {
		t.Fatal("an unknown backend must be reported rather than silently defaulted")
	}
}

func TestNew_RejectsAnEmptyBackend(t *testing.T) {
	if _, err := New(configFor("")); err == nil {
		t.Fatal("an empty backend must be reported: the config default is what fills it")
	}
}

func TestNew_RejectsANilConfig(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("a nil config must be reported")
	}
}

func TestUnit_NamesADistinctUnitPerBackend(t *testing.T) {
	s := testSubnet(t)

	if got := (Dnsmasq{}).Unit(s); got != "dnsmasq@vp-admin_br-000001.service" {
		t.Errorf("dnsmasq unit = %s", got)
	}
	if got := (Two{}).Unit(s); got != "dhcp@vp-admin_br-000001.service" {
		t.Errorf("two unit = %s", got)
	}
}

func TestTag_IsPerInterfaceNotPerVM(t *testing.T) {
	if tag("vm-web", 0) == tag("vm-web", 1) {
		t.Error("two interfaces of the same vm must get distinct tags")
	}
	if got := tag("vm-web", 1); got != "vm-web-1" {
		t.Errorf("tag = %s, want vm-web-1", got)
	}
}
