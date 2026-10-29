package natgw

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"

	"git.g3e.fr/syonad/two/internal/nft"
	"git.g3e.fr/syonad/two/pkg/db/kv"
)

type fakeHost struct {
	calls []string
	fail  map[string]error
}

func (f *fakeHost) record(call string) error {
	f.calls = append(f.calls, call)
	for prefix, err := range f.fail {
		if strings.HasPrefix(call, prefix) {
			return err
		}
	}
	return nil
}

func (f *fakeHost) CreateMacvlan(ns, name, parent string, mac net.HardwareAddr) error {
	return f.record(fmt.Sprintf("macvlan [%s] %s parent %s mac %s", ns, name, parent, mac))
}

func (f *fakeHost) BridgeAddLocalMAC(ns, bridge string, mac net.HardwareAddr) error {
	return f.record(fmt.Sprintf("fdb add [%s] %s %s", ns, bridge, mac))
}

func (f *fakeHost) BridgeDelLocalMAC(ns, bridge string, mac net.HardwareAddr) error {
	return f.record(fmt.Sprintf("fdb del [%s] %s %s", ns, bridge, mac))
}

func (f *fakeHost) Sysctl(ns, key, value string) error {
	return f.record(fmt.Sprintf("sysctl [%s] %s=%s", ns, key, value))
}

func (f *fakeHost) AddrAdd(ns, iface string, prefix *net.IPNet) error {
	return f.record(fmt.Sprintf("addr [%s] %s %s", ns, iface, prefix))
}

func (f *fakeHost) LinkUp(ns, iface string) error {
	return f.record(fmt.Sprintf("up [%s] %s", ns, iface))
}

func (f *fakeHost) DefaultVia(ns string, gw net.IP, iface string) error {
	return f.record(fmt.Sprintf("default [%s] via %s dev %s", ns, gw, iface))
}

func (f *fakeHost) SetNAT(ns string, g nft.NATGateway) error {
	return f.record(fmt.Sprintf("nat set [%s] %s %s", ns, g.Iface, g.IP))
}

func (f *fakeHost) DeleteNAT(ns, iface string) error {
	return f.record(fmt.Sprintf("nat del [%s] %s", ns, iface))
}

func (f *fakeHost) DeleteLink(ns, name string) error {
	return f.record(fmt.Sprintf("del [%s] %s", ns, name))
}

func (f *fakeHost) ExportDefault(vpc string) error {
	return f.record("export default " + vpc)
}

func (f *fakeHost) WithdrawDefault(vpc string) error {
	return f.record("withdraw default " + vpc)
}

func (f *fakeHost) NetnsDefaultToVRF(vpc string) error {
	return f.record("netns default to vrf " + vpc)
}

func labGateway(t *testing.T) gateway {
	t.Helper()
	ip, prefix, err := net.ParseCIDR("198.51.100.50/24")
	if err != nil {
		t.Fatal(err)
	}
	prefix.IP = ip
	mac, err := net.ParseMAC("02:04:00:00:0a:8c")
	if err != nil {
		t.Fatal(err)
	}
	return gateway{vpc: "vp-s7", iface: "ng-s7", mac: mac, ip: prefix, gateway: net.ParseIP("198.51.100.1")}
}

func TestCreateBuildsThePublicFaceThenTheRoutesThenTheNAT(t *testing.T) {
	f := &fakeHost{}
	if err := create(f, labGateway(t)); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"macvlan [vp-s7] ng-s7 parent br-public mac 02:04:00:00:0a:8c",
		"fdb add [vp-s7] br-public 02:04:00:00:0a:8c",
		"sysctl [vp-s7] net.ipv4.conf.ng-s7.arp_ignore=1",
		"sysctl [vp-s7] net.ipv4.conf.ng-s7.arp_announce=2",
		"sysctl [vp-s7] net.ipv4.conf.ng-s7.arp_notify=1",
		"addr [vp-s7] ng-s7 198.51.100.50/24",
		"up [vp-s7] ng-s7",
		"default [vp-s7] via 198.51.100.1 dev ng-s7",
		"nat set [vp-s7] ng-s7 198.51.100.50",
		"export default vp-s7",
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("appels :\n%s\nattendu :\n%s", strings.Join(f.calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestCreateStopsAtTheFirstFailure(t *testing.T) {
	f := &fakeHost{fail: map[string]error{"default ": errors.New("network unreachable")}}
	err := create(f, labGateway(t))
	if err == nil || !strings.Contains(err.Error(), "nat gateway of vp-s7: default route via 198.51.100.1: network unreachable") {
		t.Fatalf("erreur %v", err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "nat ") || strings.HasPrefix(c, "export ") {
			t.Errorf("étape exécutée après l'échec : %q", c)
		}
	}
}

func TestCreateExportsTheDefaultOnlyOnceTheNATIsInPlace(t *testing.T) {
	f := &fakeHost{fail: map[string]error{"nat set ": errors.New("nft absent")}}
	if err := create(f, labGateway(t)); err == nil {
		t.Fatal("erreur attendue")
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "export ") {
			t.Errorf("route par défaut annoncée sans NAT : %q", c)
		}
	}
}

func TestRemoveWithdrawsTheDefaultFirstThenUndoesTheHost(t *testing.T) {
	f := &fakeHost{}
	if err := remove(f, labGateway(t)); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"withdraw default vp-s7",
		"nat del [vp-s7] ng-s7",
		"netns default to vrf vp-s7",
		"fdb del [vp-s7] br-public 02:04:00:00:0a:8c",
		"del [vp-s7] ng-s7",
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("appels :\n%s\nattendu :\n%s", strings.Join(f.calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestRemoveKeepsTheHostWhenTheDefaultCannotBeWithdrawn(t *testing.T) {
	f := &fakeHost{fail: map[string]error{"withdraw ": errors.New("no such table")}}
	err := remove(f, labGateway(t))
	if err == nil || !strings.Contains(err.Error(), "withdraw default: no such table") {
		t.Fatalf("erreur %v", err)
	}
	if len(f.calls) != 1 {
		t.Errorf("appels après l'échec : %q", f.calls)
	}
}

func TestIface(t *testing.T) {
	got, err := Iface("vp-s7")
	if err != nil || got != "ng-s7" {
		t.Errorf("%q, %v ; ng-s7 attendu", got, err)
	}
	if _, err := Iface("nodash"); err == nil {
		t.Error("nom de VPC sans tiret accepté")
	}
}

func TestLoadDerivesTheMACFromTheL3VNI(t *testing.T) {
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })
	kv.AddInDB(db, "vpc/vp-s7/cidr", "10.241.0.0/16")
	kv.AddInDB(db, "vpc/vp-s7/l3vni", "2700")
	g, err := load(db, "vp-s7")
	if err != nil {
		t.Fatal(err)
	}
	if g.vpc != "vp-s7" || g.iface != "ng-s7" || g.mac.String() != "02:04:00:00:0a:8c" {
		t.Errorf("passerelle %+v", g)
	}
}

func TestLoadRefusesAVPCWithoutL3VNI(t *testing.T) {
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })
	kv.AddInDB(db, "vpc/vp-flat/cidr", "10.242.0.0/16")
	if _, err := load(db, "vp-flat"); err == nil || !strings.Contains(err.Error(), `vpc "vp-flat" has no l3vni`) {
		t.Errorf("erreur %v", err)
	}
}
