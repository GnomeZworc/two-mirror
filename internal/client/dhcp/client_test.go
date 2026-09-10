package dhcpclient

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dhcpapi "git.g3e.fr/syonad/two/internal/api/dhcp"
	"git.g3e.fr/syonad/two/internal/dhcpd"
)

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "dhcpd")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func testSubnet() dhcpapi.Subnet {
	return dhcpapi.Subnet{
		Network:        "10.0.5.0/24",
		InterfaceIP:    "10.0.5.1",
		VPCRoute:       "10.0.0.0/16",
		DefaultGateway: "10.0.5.254",
	}
}

func testHost() dhcpapi.Host {
	return dhcpapi.Host{MAC: "00:22:33:00:00:0a", IP: "10.0.5.10", VM: "vm-test", DefaultRoute: true}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func serve(t *testing.T) (*Client, *dhcpd.Store, string) {
	t.Helper()

	dir := shortTempDir(t)
	store := dhcpd.NewStore(filepath.Join(dir, "s.state"))
	if err := store.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	socketPath := filepath.Join(dir, "s.sock")
	server, err := dhcpapi.Listen(store, socketPath, discardLogger())
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go server.Serve()
	t.Cleanup(func() { server.Close() })

	return New(socketPath), store, socketPath
}

func TestListen_SocketIsOwnerOnly(t *testing.T) {
	_, _, socketPath := serve(t)

	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 600: whoever reaches it rewrites the subnet addressing", got)
	}
}

func TestListen_ReplacesAStaleSocketFile(t *testing.T) {
	dir := shortTempDir(t)
	socketPath := filepath.Join(dir, "stale.sock")
	if err := os.WriteFile(socketPath, []byte("leftover"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store := dhcpd.NewStore(filepath.Join(dir, "s.state"))
	if err := store.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	server, err := dhcpapi.Listen(store, socketPath, discardLogger())
	if err != nil {
		t.Fatalf("a socket left by an unclean stop must not block startup: %v", err)
	}
	server.Close()
}

func TestSetSubnet_ReachesTheStore(t *testing.T) {
	client, store, _ := serve(t)

	if err := client.SetSubnet(testSubnet()); err != nil {
		t.Fatalf("SetSubnet: %v", err)
	}
	if _, configured := store.Subnet(); !configured {
		t.Error("the subnet configuration did not reach the store")
	}
}

func TestSetSubnet_InvalidNetworkIsRefused(t *testing.T) {
	client, _, _ := serve(t)

	subnet := testSubnet()
	subnet.Network = "10.0.5.0"
	err := client.SetSubnet(subnet)
	if err == nil {
		t.Fatal("an invalid network must be refused")
	}
	if !strings.Contains(err.Error(), "refused") {
		t.Errorf("error = %v, want the server refusal to surface", err)
	}
}

func TestSetHost_IsIdempotent(t *testing.T) {
	client, store, _ := serve(t)

	for range 3 {
		if err := client.SetHost(testHost()); err != nil {
			t.Fatalf("SetHost: %v", err)
		}
	}
	if got := len(store.Hosts()); got != 1 {
		t.Errorf("hosts = %d, want 1: set-host replaces the entry for that mac", got)
	}
}

func TestSetHost_InvalidMACIsRefused(t *testing.T) {
	client, _, _ := serve(t)

	host := testHost()
	host.MAC = "nope"
	if err := client.SetHost(host); err == nil {
		t.Fatal("an invalid mac must be refused")
	}
}

func TestDelHost_RemovesTheEntry(t *testing.T) {
	client, store, _ := serve(t)

	if err := client.SetHost(testHost()); err != nil {
		t.Fatalf("SetHost: %v", err)
	}
	if err := client.DelHost(testHost().MAC); err != nil {
		t.Fatalf("DelHost: %v", err)
	}
	if got := len(store.Hosts()); got != 0 {
		t.Errorf("hosts = %d, want 0", got)
	}
}

func TestDelHost_UnknownMACIsNotAnError(t *testing.T) {
	client, _, _ := serve(t)

	if err := client.DelHost("00:22:33:ff:ff:ff"); err != nil {
		t.Errorf("deleting an absent entry must be idempotent, got %v", err)
	}
}

func TestDelHost_InvalidMACIsRefused(t *testing.T) {
	client, _, _ := serve(t)

	if err := client.DelHost("not-a-mac"); err == nil {
		t.Fatal("an invalid mac must be refused")
	}
}

func TestGetState_ReturnsStateAndDigest(t *testing.T) {
	client, _, _ := serve(t)

	if err := client.SetSubnet(testSubnet()); err != nil {
		t.Fatalf("SetSubnet: %v", err)
	}
	if err := client.SetHost(testHost()); err != nil {
		t.Fatalf("SetHost: %v", err)
	}

	state, digest, err := client.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if state.Subnet == nil || len(state.Hosts) != 1 {
		t.Fatalf("state = %+v, want one subnet and one host", state)
	}
	if digest == "" {
		t.Fatal("the digest is what the watchdog compares")
	}

	local, err := dhcpapi.Digest(state)
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	if local != digest {
		t.Errorf("digest recomputed locally = %s, server said %s: the canonical form diverges", local, digest)
	}
}

func TestGetState_DigestFollowsTheState(t *testing.T) {
	client, _, _ := serve(t)

	if err := client.SetSubnet(testSubnet()); err != nil {
		t.Fatalf("SetSubnet: %v", err)
	}
	_, before, err := client.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}

	if err := client.SetHost(testHost()); err != nil {
		t.Fatalf("SetHost: %v", err)
	}
	_, after, err := client.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}

	if before == after {
		t.Error("adding a reservation must change the digest")
	}
}

func TestProbe_DescribesWhatWouldBeSent(t *testing.T) {
	client, _, _ := serve(t)

	if err := client.SetSubnet(testSubnet()); err != nil {
		t.Fatalf("SetSubnet: %v", err)
	}
	if err := client.SetHost(testHost()); err != nil {
		t.Fatalf("SetHost: %v", err)
	}

	lease, err := client.Probe("00:22:33:00:00:0A")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if lease.IP != "10.0.5.10" {
		t.Errorf("ip = %s, want 10.0.5.10", lease.IP)
	}
	if lease.Netmask != "255.255.255.0" {
		t.Errorf("netmask = %s, want 255.255.255.0", lease.Netmask)
	}
	if lease.Router != "10.0.5.254" {
		t.Errorf("router = %s, want 10.0.5.254", lease.Router)
	}
	if len(lease.DNS) != 2 {
		t.Errorf("dns = %v, want two servers", lease.DNS)
	}
	if len(lease.Routes) != 3 {
		t.Errorf("routes = %v, want metadata, vpc and default", lease.Routes)
	}
	if lease.LeaseSeconds != 43200 {
		t.Errorf("lease = %ds, want 43200", lease.LeaseSeconds)
	}
	if lease.MAC != "00:22:33:00:00:0a" {
		t.Errorf("mac = %s, want the normalized form", lease.MAC)
	}
}

func TestProbe_UnservedMACIsReported(t *testing.T) {
	client, _, _ := serve(t)

	if err := client.SetSubnet(testSubnet()); err != nil {
		t.Fatalf("SetSubnet: %v", err)
	}
	if _, err := client.Probe("00:22:33:ff:ff:ff"); !errors.Is(err, ErrNotServed) {
		t.Fatalf("error = %v, want ErrNotServed", err)
	}
}

func TestProbe_WithoutSubnetConfigurationIsRefused(t *testing.T) {
	client, _, _ := serve(t)

	if _, err := client.Probe("00:22:33:00:00:0a"); err == nil {
		t.Fatal("probing an unconfigured subnet must be refused")
	}
}

func TestCall_UnknownVerbIsRefused(t *testing.T) {
	_, _, socketPath := serve(t)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte(`{"verb":"drop-everything"}` + "\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(string(buf[:n]), "unknown verb") {
		t.Errorf("reply = %s, want an unknown verb refusal", buf[:n])
	}
}

func TestCall_MalformedLineIsRefusedWithoutClosingTheConnection(t *testing.T) {
	_, _, socketPath := serve(t)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("{not json\n" + `{"verb":"get-state"}` + "\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(string(buf[:n]), "malformed request") {
		t.Errorf("first reply = %s, want a malformed request refusal", buf[:n])
	}
}

func TestCall_OnAnAbsentSocketFails(t *testing.T) {
	client := New(filepath.Join(shortTempDir(t), "nothing.sock"))

	if err := client.DelHost("00:22:33:00:00:0a"); err == nil {
		t.Fatal("an absent socket must be reported")
	}
}

func TestCall_HonoursItsTimeout(t *testing.T) {
	socketPath := filepath.Join(shortTempDir(t), "mute.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(3 * time.Second)
	}()

	client := New(socketPath).WithTimeout(150 * time.Millisecond)
	start := time.Now()
	if err := client.DelHost("00:22:33:00:00:0a"); err == nil {
		t.Fatal("a mute server must not hang the caller")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("returned after %s, want the 150ms deadline to apply", elapsed)
	}
}
