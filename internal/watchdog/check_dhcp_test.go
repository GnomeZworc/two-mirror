package watchdog

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"

	dhcpapi "git.g3e.fr/syonad/two/internal/api/dhcp"
	"git.g3e.fr/syonad/two/internal/dhcpbackend"
	"git.g3e.fr/syonad/two/internal/dhcpd"
	"git.g3e.fr/syonad/two/internal/state"

	"github.com/dgraph-io/badger/v4"
)

const (
	testSubnetName = "sn-000001"
	testVPC        = "vp-admin"
	testBridge     = "br-000001"
)

func twoSubnet() dhcpbackend.Subnet {
	return dhcpbackend.Subnet{Name: testSubnetName, VPC: testVPC, Bridge: testBridge}
}

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "dhcpd")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func servedBy(t *testing.T) (dhcpbackend.Two, *dhcpd.Store) {
	t.Helper()

	b := dhcpbackend.Two{RunDir: shortTempDir(t)}
	s := twoSubnet()

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

	_, network, err := net.ParseCIDR("10.0.5.0/24")
	if err != nil {
		t.Fatalf("ParseCIDR: %v", err)
	}
	if err := store.SetSubnet(dhcpd.SubnetConfig{Network: network, InterfaceIP: net.ParseIP("10.0.5.1")}); err != nil {
		t.Fatalf("SetSubnet: %v", err)
	}
	return b, store
}

func seedSubnetWithVM(t *testing.T, db *badger.DB, vmName, ip, mac string, primary bool) {
	t.Helper()

	seedKV(t, db, "subnet/"+testSubnetName+"/vpc", testVPC)
	seedKV(t, db, "subnet/"+testSubnetName+"/dhcp/"+ip, mac)

	if err := state.Set(db, "vm/"+vmName, state.Running); err != nil {
		t.Fatalf("state.Set: %v", err)
	}
	seedKV(t, db, "vm/"+vmName+"/nic/0/subnet", testSubnetName)
	seedKV(t, db, "vm/"+vmName+"/nic/0/ip", ip)
	seedKV(t, db, "vm/"+vmName+"/nic/0/primary", fmt.Sprintf("%t", primary))
}

func TestExpectedHosts_ReadsRunningVMsOnThatSubnet(t *testing.T) {
	db := newTestDB(t)
	seedSubnetWithVM(t, db, "vm-web", "10.0.5.10", "00:22:33:00:00:0A", true)

	hosts, err := expectedHosts(db, testSubnetName)
	if err != nil {
		t.Fatalf("expectedHosts: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("hosts = %+v, want one", hosts)
	}
	if hosts[0].MAC != "00:22:33:00:00:0A" || hosts[0].IP != "10.0.5.10" {
		t.Errorf("host = %+v, want the mac derived from the address plan", hosts[0])
	}
	if hosts[0].VM != "vm-web" || !hosts[0].DefaultRoute {
		t.Errorf("host = %+v, want vm-web carrying the default route", hosts[0])
	}
}

func TestExpectedHosts_IgnoresVMsThatAreNotRunning(t *testing.T) {
	db := newTestDB(t)
	seedSubnetWithVM(t, db, "vm-web", "10.0.5.10", "00:22:33:00:00:0A", true)
	if err := state.Set(db, "vm/vm-web", state.Deleting); err != nil {
		t.Fatalf("state.Set: %v", err)
	}

	hosts, err := expectedHosts(db, testSubnetName)
	if err != nil {
		t.Fatalf("expectedHosts: %v", err)
	}
	if len(hosts) != 0 {
		t.Errorf("hosts = %+v, want none: a vm being deleted is not expected to be served", hosts)
	}
}

func TestExpectedHosts_IgnoresInterfacesOnOtherSubnets(t *testing.T) {
	db := newTestDB(t)
	seedSubnetWithVM(t, db, "vm-web", "10.0.5.10", "00:22:33:00:00:0A", true)
	seedKV(t, db, "vm/vm-web/nic/1/subnet", "sn-000002")
	seedKV(t, db, "vm/vm-web/nic/1/ip", "10.0.6.10")
	seedKV(t, db, "vm/vm-web/nic/1/primary", "false")

	hosts, err := expectedHosts(db, testSubnetName)
	if err != nil {
		t.Fatalf("expectedHosts: %v", err)
	}
	if len(hosts) != 1 {
		t.Errorf("hosts = %+v, want only the interface on this subnet", hosts)
	}
}

func TestExpectedHosts_SortsByMAC(t *testing.T) {
	db := newTestDB(t)
	seedSubnetWithVM(t, db, "vm-b", "10.0.5.11", "00:22:33:00:00:0B", true)
	seedSubnetWithVM(t, db, "vm-a", "10.0.5.10", "00:22:33:00:00:0A", true)

	hosts, err := expectedHosts(db, testSubnetName)
	if err != nil {
		t.Fatalf("expectedHosts: %v", err)
	}
	if len(hosts) != 2 || hosts[0].MAC != "00:22:33:00:00:0A" {
		t.Errorf("hosts = %+v, want sorted by mac", hosts)
	}
}

func TestExpectedHosts_ReportsAnIPWithoutAMAC(t *testing.T) {
	db := newTestDB(t)
	seedSubnetWithVM(t, db, "vm-web", "10.0.5.10", "00:22:33:00:00:0A", true)
	seedKV(t, db, "vm/vm-web/nic/0/ip", "10.0.5.99")

	if _, err := expectedHosts(db, testSubnetName); err == nil {
		t.Fatal("an ip absent from the address plan must be reported, not skipped")
	}
}

func TestCheckDHCPState_MACCaseAloneIsNotADivergence(t *testing.T) {
	db := newTestDB(t)
	seedSubnetWithVM(t, db, "vm-web", "10.0.5.10", "00:22:33:00:00:0A", true)

	b, _ := servedBy(t)
	if err := b.SetVM(twoSubnet(), "vm-web", []dhcpbackend.Reservation{
		{Index: 0, MAC: "00:22:33:00:00:0A", IP: "10.0.5.10", DefaultRoute: true},
	}); err != nil {
		t.Fatalf("SetVM: %v", err)
	}

	r := &recorder{}
	checkDHCPState(db, testSubnetName, twoSubnet(), b, r)

	if len(r.calls) != 0 {
		t.Errorf("dhcp.Entries stores uppercase macs and the server normalizes to lowercase: that alone must not read as drift, got %+v", r.calls)
	}
}

func TestCheckDHCPState_ADivergenceIsReportedOnceNotAsBothMissingAndStale(t *testing.T) {
	db := newTestDB(t)
	seedSubnetWithVM(t, db, "vm-web", "10.0.5.10", "00:22:33:00:00:0A", true)

	b, _ := servedBy(t)
	if err := b.SetVM(twoSubnet(), "vm-web", []dhcpbackend.Reservation{
		{Index: 0, MAC: "00:22:33:00:00:0A", IP: "10.0.5.99", DefaultRoute: true},
	}); err != nil {
		t.Fatalf("SetVM: %v", err)
	}

	r := &recorder{}
	checkDHCPState(db, testSubnetName, twoSubnet(), b, r)

	if len(r.calls) != 1 {
		t.Errorf("notifications = %+v, want a single diverging-reservation report", r.calls)
	}
	if r.hasProblemContaining("missing on the server") || r.hasProblemContaining("stale dhcp") {
		t.Errorf("without mac normalization the same host reads as both missing and stale: %+v", r.calls)
	}
}

func TestCheckDHCPState_SilentWhenServerMatchesDatabase(t *testing.T) {
	db := newTestDB(t)
	seedSubnetWithVM(t, db, "vm-web", "10.0.5.10", "00:22:33:00:00:0A", true)

	b, _ := servedBy(t)
	if err := b.SetVM(twoSubnet(), "vm-web", []dhcpbackend.Reservation{
		{Index: 0, MAC: "00:22:33:00:00:0A", IP: "10.0.5.10", DefaultRoute: true},
	}); err != nil {
		t.Fatalf("SetVM: %v", err)
	}

	r := &recorder{}
	checkDHCPState(db, testSubnetName, twoSubnet(), b, r)

	if len(r.calls) != 0 {
		t.Errorf("notifications = %+v, want none when the server agrees with the database", r.calls)
	}
}

func TestCheckDHCPState_ReportsAReservationTheServerNeverGot(t *testing.T) {
	db := newTestDB(t)
	seedSubnetWithVM(t, db, "vm-web", "10.0.5.10", "00:22:33:00:00:0A", true)

	b, _ := servedBy(t)

	r := &recorder{}
	checkDHCPState(db, testSubnetName, twoSubnet(), b, r)

	if !r.hasProblemContaining("dhcp reservation missing on the server") {
		t.Errorf("a lost set-host order must be reported, got %+v", r.calls)
	}
	if !r.hasProblemContaining("vm vm-web") {
		t.Errorf("the report must name the vm, got %+v", r.calls)
	}
}

func TestCheckDHCPState_ReportsAStaleReservation(t *testing.T) {
	db := newTestDB(t)
	seedKV(t, db, "subnet/"+testSubnetName+"/vpc", testVPC)

	b, _ := servedBy(t)
	if err := b.SetVM(twoSubnet(), "vm-gone", []dhcpbackend.Reservation{
		{Index: 0, MAC: "00:22:33:00:00:0A", IP: "10.0.5.10", DefaultRoute: true},
	}); err != nil {
		t.Fatalf("SetVM: %v", err)
	}

	r := &recorder{}
	checkDHCPState(db, testSubnetName, twoSubnet(), b, r)

	if !r.hasProblemContaining("stale dhcp reservation on the server") {
		t.Errorf("a lost del-host order must be reported, got %+v", r.calls)
	}
}

func TestCheckDHCPState_ReportsADivergingIP(t *testing.T) {
	db := newTestDB(t)
	seedSubnetWithVM(t, db, "vm-web", "10.0.5.10", "00:22:33:00:00:0A", true)

	b, _ := servedBy(t)
	if err := b.SetVM(twoSubnet(), "vm-web", []dhcpbackend.Reservation{
		{Index: 0, MAC: "00:22:33:00:00:0A", IP: "10.0.5.99", DefaultRoute: true},
	}); err != nil {
		t.Fatalf("SetVM: %v", err)
	}

	r := &recorder{}
	checkDHCPState(db, testSubnetName, twoSubnet(), b, r)

	if !r.hasProblemContaining("dhcp reservation diverges") {
		t.Errorf("a mac served with the wrong address must be reported, got %+v", r.calls)
	}
}

func TestCheckDHCPState_ReportsADivergingDefaultRoute(t *testing.T) {
	db := newTestDB(t)
	seedSubnetWithVM(t, db, "vm-web", "10.0.5.10", "00:22:33:00:00:0A", true)

	b, _ := servedBy(t)
	if err := b.SetVM(twoSubnet(), "vm-web", []dhcpbackend.Reservation{
		{Index: 0, MAC: "00:22:33:00:00:0A", IP: "10.0.5.10", DefaultRoute: false},
	}); err != nil {
		t.Fatalf("SetVM: %v", err)
	}

	r := &recorder{}
	checkDHCPState(db, testSubnetName, twoSubnet(), b, r)

	if !r.hasProblemContaining("dhcp default route diverges") {
		t.Errorf("a wrong default route would break multi-subnet routing, got %+v", r.calls)
	}
}

func TestCheckDHCPState_ReportsAnUnreachableServer(t *testing.T) {
	db := newTestDB(t)
	seedKV(t, db, "subnet/"+testSubnetName+"/vpc", testVPC)

	b := dhcpbackend.Two{RunDir: shortTempDir(t)}

	r := &recorder{}
	checkDHCPState(db, testSubnetName, twoSubnet(), b, r)

	if !r.hasProblemContaining("dhcp server unreachable") {
		t.Errorf("a dead server must be reported, got %+v", r.calls)
	}
}

func TestCheckDHCPState_ReportsAServerWithNoSubnetConfiguration(t *testing.T) {
	db := newTestDB(t)
	seedKV(t, db, "subnet/"+testSubnetName+"/vpc", testVPC)

	b := dhcpbackend.Two{RunDir: shortTempDir(t)}
	s := twoSubnet()
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

	r := &recorder{}
	checkDHCPState(db, testSubnetName, s, b, r)

	if !r.hasProblemContaining("no subnet configuration") {
		t.Errorf("a server that was never configured serves nothing, got %+v", r.calls)
	}
}

func TestCheckDHCPConfigFile_ReportsAMissingDnsmasqConfig(t *testing.T) {
	r := &recorder{}
	checkDHCPConfigFile(testSubnetName, filepath.Join(t.TempDir(), "absent.conf"), r)

	if !r.hasProblemContaining("dnsmasq config missing") {
		t.Errorf("notifications = %+v, want the missing config reported", r.calls)
	}
}

func TestCheckDHCP_WithoutABackendChecksNothing(t *testing.T) {
	db := newTestDB(t)
	r := &recorder{}

	checkDHCP(db, testSubnetName, twoSubnet(), nil, nil, r)

	if len(r.calls) != 0 {
		t.Errorf("notifications = %+v, want none: the caller already reported the unusable backend", r.calls)
	}
}
