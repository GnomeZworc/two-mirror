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

func testSubnetSnapshot() SubnetSnapshot {
	return SubnetSnapshot{
		Network:        "10.0.5.0/24",
		InterfaceIP:    "10.0.5.1",
		VPCRoute:       "10.0.0.0/16",
		DefaultGateway: "10.0.5.254",
	}
}

func testHostSnapshot() HostSnapshot {
	return HostSnapshot{MAC: "00:22:33:00:00:0a", IP: "10.0.5.10", VM: "vm-test", DefaultRoute: true}
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

func TestStore_PersistRestoresTheModeAfterAnExternalChmod(t *testing.T) {
	s, path := loadedStore(t)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	if err := s.SetHost(testHostSnapshot()); err != nil {
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

func TestStore_LoadOnEmptyFileYieldsNoSubnet(t *testing.T) {
	path := statePath(t)
	if err := os.WriteFile(path, nil, stateFileMode); err != nil {
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
	if err := os.WriteFile(path, []byte("{not json"), stateFileMode); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := NewStore(path).Load(); err == nil {
		t.Fatal("a corrupted state file must be reported, not silently ignored")
	}
}

func TestStore_SetSubnetIsPersisted(t *testing.T) {
	s, path := loadedStore(t)
	if err := s.SetSubnet(testSubnetSnapshot()); err != nil {
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
	snap := testSubnetSnapshot()
	snap.InterfaceIP = ""

	if err := s.SetSubnet(snap); !errors.Is(err, ErrNoInterfaceIP) {
		t.Fatalf("error = %v, want ErrNoInterfaceIP", err)
	}
}

func TestStore_SetSubnetRejectsAnInvalidNetwork(t *testing.T) {
	s, _ := loadedStore(t)
	snap := testSubnetSnapshot()
	snap.Network = "10.0.5.0"

	if err := s.SetSubnet(snap); err == nil {
		t.Fatal("a network without a prefix length must be rejected")
	}
}

func TestStore_SetHostIsPersistedAndFound(t *testing.T) {
	s, path := loadedStore(t)
	if err := s.SetHost(testHostSnapshot()); err != nil {
		t.Fatalf("SetHost: %v", err)
	}

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	mac, err := net.ParseMAC("00:22:33:00:00:0a")
	if err != nil {
		t.Fatalf("ParseMAC: %v", err)
	}
	h, known := reloaded.Lookup(mac)
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
	snap := testHostSnapshot()
	if err := s.SetHost(snap); err != nil {
		t.Fatalf("SetHost: %v", err)
	}

	snap.IP = "10.0.5.11"
	snap.DefaultRoute = false
	if err := s.SetHost(snap); err != nil {
		t.Fatalf("SetHost: %v", err)
	}

	got := s.Snapshot()
	if len(got.Hosts) != 1 {
		t.Fatalf("hosts = %d, want 1: the mac is the key", len(got.Hosts))
	}
	if got.Hosts[0].IP != "10.0.5.11" || got.Hosts[0].DefaultRoute {
		t.Errorf("entry = %+v, want the second order to have replaced the first", got.Hosts[0])
	}
}

func TestStore_SetHostNormalizesTheMACCase(t *testing.T) {
	s, _ := loadedStore(t)
	snap := testHostSnapshot()
	snap.MAC = "00:22:33:AA:BB:CC"
	if err := s.SetHost(snap); err != nil {
		t.Fatalf("SetHost: %v", err)
	}

	mac, err := net.ParseMAC("00:22:33:aa:bb:cc")
	if err != nil {
		t.Fatalf("ParseMAC: %v", err)
	}
	if _, known := s.Lookup(mac); !known {
		t.Error("an uppercase mac must be found in lowercase: the key would diverge")
	}
}

func TestStore_LoadNormalizesTheMACCase(t *testing.T) {
	path := statePath(t)
	raw, err := json.Marshal(Snapshot{Hosts: []HostSnapshot{{
		MAC: "00:22:33:AA:BB:CC", IP: "10.0.5.12", VM: "vm-test", DefaultRoute: true,
	}}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, raw, stateFileMode); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := NewStore(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	m, err := net.ParseMAC("00:22:33:aa:bb:cc")
	if err != nil {
		t.Fatalf("ParseMAC: %v", err)
	}
	if _, known := s.Lookup(m); !known {
		t.Error("a reloaded uppercase mac must be keyed in lowercase: the host would silently stop being served")
	}
}

func TestStore_SetHostRejectsAMissingMAC(t *testing.T) {
	s, _ := loadedStore(t)
	snap := testHostSnapshot()
	snap.MAC = ""

	if err := s.SetHost(snap); !errors.Is(err, ErrNoMAC) {
		t.Fatalf("error = %v, want ErrNoMAC", err)
	}
}

func TestStore_SetHostRejectsAnInvalidIP(t *testing.T) {
	s, _ := loadedStore(t)
	snap := testHostSnapshot()
	snap.IP = "10.0.5.300"

	if err := s.SetHost(snap); err == nil {
		t.Fatal("an invalid host ip must be rejected")
	}
}

func TestStore_DelHostRemovesTheEntry(t *testing.T) {
	s, path := loadedStore(t)
	if err := s.SetHost(testHostSnapshot()); err != nil {
		t.Fatalf("SetHost: %v", err)
	}
	if err := s.DelHost("00:22:33:00:00:0A"); err != nil {
		t.Fatalf("DelHost: %v", err)
	}

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(reloaded.Snapshot().Hosts); got != 0 {
		t.Errorf("hosts = %d, want 0 after deletion", got)
	}
}

func TestStore_DelHostOnAnUnknownMACIsNotAnError(t *testing.T) {
	s, _ := loadedStore(t)
	if err := s.DelHost("00:22:33:ff:ff:ff"); err != nil {
		t.Errorf("deleting an absent entry must be idempotent, got %v", err)
	}
}

func TestStore_DelHostRejectsAnInvalidMAC(t *testing.T) {
	s, _ := loadedStore(t)
	if err := s.DelHost("not-a-mac"); err == nil {
		t.Fatal("an invalid mac must be reported")
	}
}

func TestStore_SnapshotSortsHostsByMAC(t *testing.T) {
	s, _ := loadedStore(t)
	for _, mac := range []string{"00:22:33:00:00:0c", "00:22:33:00:00:0a", "00:22:33:00:00:0b"} {
		snap := testHostSnapshot()
		snap.MAC = mac
		if err := s.SetHost(snap); err != nil {
			t.Fatalf("SetHost: %v", err)
		}
	}

	hosts := s.Snapshot().Hosts
	for i := 1; i < len(hosts); i++ {
		if hosts[i-1].MAC >= hosts[i].MAC {
			t.Fatalf("hosts are not sorted: %v", hosts)
		}
	}
}

func TestStore_PersistLeavesNoTemporaryFileBehind(t *testing.T) {
	s, path := loadedStore(t)
	if err := s.SetHost(testHostSnapshot()); err != nil {
		t.Fatalf("SetHost: %v", err)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want only the state file: %v", len(entries), entries)
	}
}

func TestStore_PersistedStateIsValidJSON(t *testing.T) {
	s, path := loadedStore(t)
	if err := s.SetSubnet(testSubnetSnapshot()); err != nil {
		t.Fatalf("SetSubnet: %v", err)
	}
	if err := s.SetHost(testHostSnapshot()); err != nil {
		t.Fatalf("SetHost: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("the state file must stay parseable: %v", err)
	}
	if snap.Subnet == nil || len(snap.Hosts) != 1 {
		t.Errorf("snapshot = %+v, want one subnet and one host", snap)
	}
}
