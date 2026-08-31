package dhcpd

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func statePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "vp-admin_br-000001.state")
}

func loadedStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := statePath(t)
	s := NewStore(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s, path
}

func TestStore_LoadCreatesTheStateFileWhenAbsent(t *testing.T) {
	_, path := loadedStore(t)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the state file must be created on load: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 600: the file exposes tenant mac and ip", got)
	}
}

func TestStore_LoadOnEmptyFileYieldsNoSubnet(t *testing.T) {
	path := statePath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := NewStore(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, configured := s.Subnet(); configured {
		t.Error("an empty state file must not report a configured subnet")
	}
}

func TestStore_LoadRejectsCorruptedState(t *testing.T) {
	path := statePath(t)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := NewStore(path).Load(); err == nil {
		t.Fatal("a corrupted state file must be reported, not silently ignored")
	}
}

func TestStore_LoadRejectsAnInvalidStoredMAC(t *testing.T) {
	path := statePath(t)
	raw := []byte(`{"hosts":[{"mac":"nope","ip":"10.0.5.10","default_route":true}]}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := NewStore(path).Load(); err == nil {
		t.Fatal("an unparseable stored mac must be reported")
	}
}

func TestStore_SetSubnetIsPersisted(t *testing.T) {
	s, path := loadedStore(t)
	if err := s.SetSubnet(fullConfig(t)); err != nil {
		t.Fatalf("SetSubnet: %v", err)
	}

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	c, configured := reloaded.Subnet()
	if !configured {
		t.Fatal("subnet lost across a restart")
	}
	if got := c.Network.String(); got != "10.0.5.0/24" {
		t.Errorf("network = %s, want 10.0.5.0/24", got)
	}
	if !c.InterfaceIP.Equal(net.ParseIP("10.0.5.1")) {
		t.Errorf("interface ip = %s, want 10.0.5.1", c.InterfaceIP)
	}
	if got := c.VPCRoute.String(); got != "10.0.0.0/16" {
		t.Errorf("vpc route = %s, want 10.0.0.0/16", got)
	}
	if !c.DefaultGateway.Equal(net.ParseIP("10.0.5.254")) {
		t.Errorf("default gateway = %s, want 10.0.5.254", c.DefaultGateway)
	}
}

func TestStore_SetSubnetRejectsAMissingInterfaceIP(t *testing.T) {
	s, _ := loadedStore(t)
	c := fullConfig(t)
	c.InterfaceIP = nil

	if err := s.SetSubnet(c); !errors.Is(err, ErrNoInterfaceIP) {
		t.Fatalf("error = %v, want ErrNoInterfaceIP", err)
	}
}

func TestStore_SetSubnetRejectsAMissingNetwork(t *testing.T) {
	s, _ := loadedStore(t)
	c := fullConfig(t)
	c.Network = nil

	if err := s.SetSubnet(c); !errors.Is(err, ErrNoNetwork) {
		t.Fatalf("error = %v, want ErrNoNetwork", err)
	}
}

func TestStore_SetHostIsPersistedAndFound(t *testing.T) {
	s, path := loadedStore(t)
	if err := s.SetHost(testHost(t)); err != nil {
		t.Fatalf("SetHost: %v", err)
	}

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	h, known := reloaded.Lookup(mac(t, "00:22:33:00:00:0a"))
	if !known {
		t.Fatal("host lost across a restart")
	}
	if !h.IP.Equal(net.ParseIP("10.0.5.10")) {
		t.Errorf("ip = %s, want 10.0.5.10", h.IP)
	}
	if h.VM != "vm-test" {
		t.Errorf("vm = %q, want vm-test", h.VM)
	}
	if !h.DefaultRoute {
		t.Error("default route flag lost across a restart")
	}
}

func TestStore_SetHostIsIdempotentOnTheSameMAC(t *testing.T) {
	s, _ := loadedStore(t)
	h := testHost(t)
	if err := s.SetHost(h); err != nil {
		t.Fatalf("SetHost: %v", err)
	}

	h.IP = net.ParseIP("10.0.5.11")
	h.DefaultRoute = false
	if err := s.SetHost(h); err != nil {
		t.Fatalf("SetHost: %v", err)
	}

	hosts := s.Hosts()
	if len(hosts) != 1 {
		t.Fatalf("hosts = %d, want 1: the mac is the key", len(hosts))
	}
	if !hosts[0].IP.Equal(net.ParseIP("10.0.5.11")) || hosts[0].DefaultRoute {
		t.Errorf("entry = %+v, want the second order to have replaced the first", hosts[0])
	}
}

func TestStore_LookupNormalizesTheMACCase(t *testing.T) {
	s, _ := loadedStore(t)
	h := testHost(t)
	h.MAC = mac(t, "00:22:33:AA:BB:CC")
	if err := s.SetHost(h); err != nil {
		t.Fatalf("SetHost: %v", err)
	}

	if _, known := s.Lookup(mac(t, "00:22:33:aa:bb:cc")); !known {
		t.Error("an uppercase mac must be found in lowercase: the key would diverge")
	}
}

func TestStore_LoadNormalizesTheMACCase(t *testing.T) {
	path := statePath(t)
	raw := []byte(`{"hosts":[{"mac":"00:22:33:AA:BB:CC","ip":"10.0.5.12","default_route":true}]}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := NewStore(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, known := s.Lookup(mac(t, "00:22:33:aa:bb:cc")); !known {
		t.Error("a reloaded uppercase mac must be keyed in lowercase: the host would silently stop being served")
	}
}

func TestStore_SetHostRejectsAMissingMAC(t *testing.T) {
	s, _ := loadedStore(t)
	h := testHost(t)
	h.MAC = nil

	if err := s.SetHost(h); !errors.Is(err, ErrNoMAC) {
		t.Fatalf("error = %v, want ErrNoMAC", err)
	}
}

func TestStore_SetHostRejectsAMissingIP(t *testing.T) {
	s, _ := loadedStore(t)
	h := testHost(t)
	h.IP = nil

	if err := s.SetHost(h); !errors.Is(err, ErrNoHostIP) {
		t.Fatalf("error = %v, want ErrNoHostIP", err)
	}
}

func TestStore_DelHostRemovesTheEntry(t *testing.T) {
	s, path := loadedStore(t)
	if err := s.SetHost(testHost(t)); err != nil {
		t.Fatalf("SetHost: %v", err)
	}
	if err := s.DelHost(mac(t, "00:22:33:00:00:0A")); err != nil {
		t.Fatalf("DelHost: %v", err)
	}

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(reloaded.Hosts()); got != 0 {
		t.Errorf("hosts = %d, want 0 after deletion", got)
	}
}

func TestStore_DelHostOnAnUnknownMACIsNotAnError(t *testing.T) {
	s, _ := loadedStore(t)
	if err := s.DelHost(mac(t, "00:22:33:ff:ff:ff")); err != nil {
		t.Errorf("deleting an absent entry must be idempotent, got %v", err)
	}
}

func TestStore_DelHostRejectsAnEmptyMAC(t *testing.T) {
	s, _ := loadedStore(t)
	if err := s.DelHost(nil); !errors.Is(err, ErrNoMAC) {
		t.Fatalf("error = %v, want ErrNoMAC", err)
	}
}

func TestStore_HostsAreSortedByMAC(t *testing.T) {
	s, _ := loadedStore(t)
	for _, m := range []string{"00:22:33:00:00:0c", "00:22:33:00:00:0a", "00:22:33:00:00:0b"} {
		h := testHost(t)
		h.MAC = mac(t, m)
		if err := s.SetHost(h); err != nil {
			t.Fatalf("SetHost: %v", err)
		}
	}

	hosts := s.Hosts()
	for i := 1; i < len(hosts); i++ {
		if hosts[i-1].MAC.String() >= hosts[i].MAC.String() {
			t.Fatalf("hosts are not sorted: %v", hosts)
		}
	}
}

func TestStore_PersistedStateIsSortedOnDisk(t *testing.T) {
	s, path := loadedStore(t)
	for _, m := range []string{"00:22:33:00:00:0c", "00:22:33:00:00:0a"} {
		h := testHost(t)
		h.MAC = mac(t, m)
		if err := s.SetHost(h); err != nil {
			t.Fatalf("SetHost: %v", err)
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var state struct {
		Hosts []struct {
			MAC string `json:"mac"`
		} `json:"hosts"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("the state file must stay parseable: %v", err)
	}
	if len(state.Hosts) != 2 || state.Hosts[0].MAC != "00:22:33:00:00:0a" {
		t.Errorf("hosts on disk = %+v, want sorted by mac", state.Hosts)
	}
}

func TestStore_PersistRestoresTheModeAfterAnExternalChmod(t *testing.T) {
	s, path := loadedStore(t)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	if err := s.SetHost(testHost(t)); err != nil {
		t.Fatalf("SetHost: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 600: each write must replace the file, not edit it in place", got)
	}
}

func TestStore_PathReportsTheStateFile(t *testing.T) {
	s, path := loadedStore(t)
	if got := s.Path(); got != path {
		t.Errorf("Path = %s, want %s", got, path)
	}
}
