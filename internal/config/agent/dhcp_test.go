package configuration

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.yml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestValidBackend_AcceptsTheTwoKnownBackends(t *testing.T) {
	for _, backend := range []string{BackendDnsmasq, BackendTwo} {
		if err := ValidBackend(backend); err != nil {
			t.Errorf("ValidBackend(%q) = %v, want nil", backend, err)
		}
	}
}

func TestValidBackend_RejectsAnythingElse(t *testing.T) {
	for _, backend := range []string{"", "dhcpd", "DNSMASQ", "two "} {
		if err := ValidBackend(backend); err == nil {
			t.Errorf("ValidBackend(%q) = nil, want an error", backend)
		}
	}
}

func TestLoadConfig_DefaultsToDnsmasq(t *testing.T) {
	path := writeConfig(t, "database:\n  path: /tmp/two\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DHCP.Backend != BackendDnsmasq {
		t.Errorf("backend = %q, want %q: a 0.1.0 config must keep behaving as before", cfg.DHCP.Backend, BackendDnsmasq)
	}
}

func TestLoadConfig_ReadsTheTwoBackend(t *testing.T) {
	path := writeConfig(t, "dhcp:\n  backend: two\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DHCP.Backend != BackendTwo {
		t.Errorf("backend = %q, want two", cfg.DHCP.Backend)
	}
}
