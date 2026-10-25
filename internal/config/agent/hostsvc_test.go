package configuration

import (
	"reflect"
	"strings"
	"testing"
)

func TestLoadConfig_HostSvcDefaults(t *testing.T) {
	cfg, err := LoadConfig(writeYAML(t, ""))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.HostSvc.DNS()) != 0 || len(cfg.HostSvc.NTP()) != 0 {
		t.Errorf("without upstreams the lists must stay empty, the host serves: %+v", cfg.HostSvc)
	}
	if cfg.HostSvc.Rate() != 100 {
		t.Errorf("default rate per vpc = %d, want 100", cfg.HostSvc.Rate())
	}
}

func TestLoadConfig_HostSvcDefaultPorts(t *testing.T) {
	cfg, err := LoadConfig(writeYAML(t, "hostsvc:\n  dns_upstream: [192.0.2.53, \"192.0.2.54:5353\"]\n  ntp_upstream: [192.0.2.123]\n  rate_limit_per_vpc: 20\n"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got, want := cfg.HostSvc.DNS(), []string{"192.0.2.53:53", "192.0.2.54:5353"}; !reflect.DeepEqual(got, want) {
		t.Errorf("dns = %v, want %v", got, want)
	}
	if got, want := cfg.HostSvc.NTP(), []string{"192.0.2.123:123"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ntp = %v, want %v", got, want)
	}
	if cfg.HostSvc.Rate() != 20 {
		t.Errorf("rate = %d, want 20", cfg.HostSvc.Rate())
	}
}

func TestLoadConfig_HostSvcInvalidIsRefused(t *testing.T) {
	for _, c := range []struct{ yaml, msg string }{
		{"hostsvc:\n  rate_limit_per_vpc: 0\n", "hostsvc.rate_limit_per_vpc 0 must be a positive number"},
		{"hostsvc:\n  rate_limit_per_vpc: -5\n", "hostsvc.rate_limit_per_vpc -5"},
		{"hostsvc:\n  dns_upstream: [dns.example]\n", `hostsvc.dns_upstream: "dns.example" is neither an ip nor ip:port`},
		{"hostsvc:\n  dns_upstream: [\"dns.example:53\"]\n", `"dns.example" is not an ip address`},
		{"hostsvc:\n  ntp_upstream: [\"192.0.2.1:0\"]\n", `hostsvc.ntp_upstream: "192.0.2.1:0": invalid port "0"`},
		{"hostsvc:\n  ntp_upstream: [\"192.0.2.1:70000\"]\n", `invalid port "70000"`},
	} {
		if _, err := LoadConfig(writeYAML(t, c.yaml)); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%q: error %v, want %q", c.yaml, err, c.msg)
		}
	}
}
