package dhcpbackend

import (
	"io"
	"log/slog"
	"net"
	"os"
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

func twoBackend(t *testing.T) (Two, Subnet, *dhcpd.Store) {
	t.Helper()

	b := Two{RunDir: shortTempDir(t)}
	s := testSubnet(t)

	store := dhcpd.NewStore(dhcpapi.StatePath(b.RunDir, s.Instance()))
	if err := store.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	server, err := dhcpapi.Listen(store, dhcpapi.SocketPath(b.RunDir, s.Instance()), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go server.Serve()
	t.Cleanup(func() { server.Close() })

	return b, s, store
}

func TestTwo_RunDirDefaultsToTheSharedOne(t *testing.T) {
	if got := (Two{}).runDir(); got != dhcpapi.DefaultRunDir {
		t.Errorf("runDir = %s, want %s", got, dhcpapi.DefaultRunDir)
	}
}

func TestTwoWaitReady_ReturnsOnceTheServerAnswers(t *testing.T) {
	b, s, _ := twoBackend(t)

	if err := b.waitReady(s, time.Second, 10*time.Millisecond); err != nil {
		t.Fatalf("waitReady: %v", err)
	}
}

func TestTwoWaitReady_TimesOutWhenNothingListens(t *testing.T) {
	b := Two{RunDir: shortTempDir(t)}
	s := testSubnet(t)

	start := time.Now()
	err := b.waitReady(s, 200*time.Millisecond, 10*time.Millisecond)
	if err == nil {
		t.Fatal("waitReady must report a server that never came up")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("waitReady took %s, want the 200ms budget to apply", elapsed)
	}
}

func TestTwoPushSubnet_ReachesTheStore(t *testing.T) {
	b, s, store := twoBackend(t)

	if err := b.pushSubnet(s); err != nil {
		t.Fatalf("pushSubnet: %v", err)
	}

	got, configured := store.Subnet()
	if !configured {
		t.Fatal("the subnet configuration did not reach the server")
	}
	if !got.InterfaceIP.Equal(net.ParseIP("10.0.5.1")) {
		t.Errorf("interface ip = %s, want 10.0.5.1", got.InterfaceIP)
	}
	if got.VPCRoute == nil || got.VPCRoute.String() != "10.0.0.0/16" {
		t.Errorf("vpc route = %v, want 10.0.0.0/16", got.VPCRoute)
	}
	if !got.DefaultGateway.Equal(net.ParseIP("10.0.5.254")) {
		t.Errorf("default gateway = %s, want 10.0.5.254", got.DefaultGateway)
	}
}

func TestTwoPushSubnet_OmitsAnAbsentVPCRouteAndGateway(t *testing.T) {
	b, s, store := twoBackend(t)
	s.VPCRoute = nil
	s.DefaultGateway = nil

	if err := b.pushSubnet(s); err != nil {
		t.Fatalf("pushSubnet: %v", err)
	}

	got, _ := store.Subnet()
	if got.VPCRoute != nil {
		t.Errorf("vpc route = %v, want none", got.VPCRoute)
	}
	if got.DefaultGateway != nil {
		t.Errorf("default gateway = %v, want none: a bridge subnet has no gateway of ours", got.DefaultGateway)
	}
}

func TestTwoSetVM_ReservesEveryInterface(t *testing.T) {
	b, s, store := twoBackend(t)
	if err := b.pushSubnet(s); err != nil {
		t.Fatalf("pushSubnet: %v", err)
	}

	res := []Reservation{
		{Index: 0, MAC: "00:22:33:00:00:0a", IP: "10.0.5.10", DefaultRoute: true},
		{Index: 1, MAC: "00:22:33:00:00:0b", IP: "10.0.5.11"},
	}
	if err := b.SetVM(s, "vm-web", res); err != nil {
		t.Fatalf("SetVM: %v", err)
	}

	hosts := store.Hosts()
	if len(hosts) != 2 {
		t.Fatalf("hosts = %d, want 2", len(hosts))
	}
	for _, h := range hosts {
		if h.VM != "vm-web" {
			t.Errorf("host %s carries vm %q, want vm-web", h.MAC, h.VM)
		}
	}
}

func TestTwoSetVM_CarriesTheDefaultRouteFlagPerInterface(t *testing.T) {
	b, s, store := twoBackend(t)
	if err := b.pushSubnet(s); err != nil {
		t.Fatalf("pushSubnet: %v", err)
	}

	res := []Reservation{
		{Index: 0, MAC: "00:22:33:00:00:0a", IP: "10.0.5.10", DefaultRoute: true},
		{Index: 1, MAC: "00:22:33:00:00:0b", IP: "10.0.5.11"},
	}
	if err := b.SetVM(s, "vm-web", res); err != nil {
		t.Fatalf("SetVM: %v", err)
	}

	primary, known := store.Lookup(mustMAC(t, "00:22:33:00:00:0a"))
	if !known || !primary.DefaultRoute {
		t.Errorf("primary interface = %+v, want the default route", primary)
	}
	secondary, known := store.Lookup(mustMAC(t, "00:22:33:00:00:0b"))
	if !known || secondary.DefaultRoute {
		t.Errorf("secondary interface = %+v, want no default route", secondary)
	}
}

func TestTwoSetVM_IsIdempotent(t *testing.T) {
	b, s, store := twoBackend(t)
	if err := b.pushSubnet(s); err != nil {
		t.Fatalf("pushSubnet: %v", err)
	}

	res := []Reservation{{Index: 0, MAC: "00:22:33:00:00:0a", IP: "10.0.5.10", DefaultRoute: true}}
	for range 3 {
		if err := b.SetVM(s, "vm-web", res); err != nil {
			t.Fatalf("SetVM: %v", err)
		}
	}
	if got := len(store.Hosts()); got != 1 {
		t.Errorf("hosts = %d, want 1", got)
	}
}

func TestTwoSetVM_RejectsAnInvalidMAC(t *testing.T) {
	b, s, _ := twoBackend(t)
	if err := b.pushSubnet(s); err != nil {
		t.Fatalf("pushSubnet: %v", err)
	}

	res := []Reservation{{Index: 0, MAC: "nope", IP: "10.0.5.10"}}
	if err := b.SetVM(s, "vm-web", res); err == nil {
		t.Fatal("an invalid mac must be reported")
	}
}

func TestTwoDelVM_ReleasesEveryInterface(t *testing.T) {
	b, s, store := twoBackend(t)
	if err := b.pushSubnet(s); err != nil {
		t.Fatalf("pushSubnet: %v", err)
	}

	res := []Reservation{
		{Index: 0, MAC: "00:22:33:00:00:0a", IP: "10.0.5.10", DefaultRoute: true},
		{Index: 1, MAC: "00:22:33:00:00:0b", IP: "10.0.5.11"},
	}
	if err := b.SetVM(s, "vm-web", res); err != nil {
		t.Fatalf("SetVM: %v", err)
	}
	if err := b.DelVM(s, "vm-web", res); err != nil {
		t.Fatalf("DelVM: %v", err)
	}

	if got := len(store.Hosts()); got != 0 {
		t.Errorf("hosts = %d, want 0", got)
	}
}

func TestTwoDelVM_LeavesOtherVMsAlone(t *testing.T) {
	b, s, store := twoBackend(t)
	if err := b.pushSubnet(s); err != nil {
		t.Fatalf("pushSubnet: %v", err)
	}

	web := []Reservation{{Index: 0, MAC: "00:22:33:00:00:0a", IP: "10.0.5.10", DefaultRoute: true}}
	db := []Reservation{{Index: 0, MAC: "00:22:33:00:00:0b", IP: "10.0.5.11", DefaultRoute: true}}
	for name, res := range map[string][]Reservation{"vm-web": web, "vm-db": db} {
		if err := b.SetVM(s, name, res); err != nil {
			t.Fatalf("SetVM %s: %v", name, err)
		}
	}

	if err := b.DelVM(s, "vm-web", web); err != nil {
		t.Fatalf("DelVM: %v", err)
	}

	hosts := store.Hosts()
	if len(hosts) != 1 || hosts[0].VM != "vm-db" {
		t.Errorf("remaining hosts = %+v, want only vm-db", hosts)
	}
}

func TestTwoDelVM_OnAnUnknownMACIsNotAnError(t *testing.T) {
	b, s, _ := twoBackend(t)

	res := []Reservation{{Index: 0, MAC: "00:22:33:ff:ff:ff", IP: "10.0.5.99"}}
	if err := b.DelVM(s, "vm-gone", res); err != nil {
		t.Errorf("releasing an absent reservation must be idempotent, got %v", err)
	}
}

func mustMAC(t *testing.T, s string) net.HardwareAddr {
	t.Helper()
	m, err := net.ParseMAC(s)
	if err != nil {
		t.Fatalf("ParseMAC(%q): %v", s, err)
	}
	return m
}
