package fip

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"

	"git.g3e.fr/syonad/two/internal/nft"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
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

func (f *fakeHost) BridgeSetLocalMAC(ns, bridge string, mac net.HardwareAddr) error {
	return f.record(fmt.Sprintf("fdb add [%s] %s %s", ns, bridge, mac))
}

func (f *fakeHost) BridgeDelLocalMAC(ns, bridge string, mac net.HardwareAddr) error {
	return f.record(fmt.Sprintf("fdb del [%s] %s %s", ns, bridge, mac))
}

func (f *fakeHost) Sysctl(ns, key, value string) error {
	return f.record(fmt.Sprintf("sysctl [%s] %s=%s", ns, key, value))
}

func (f *fakeHost) SetFloatingIP(ns string, r nft.FloatingIP) error {
	return f.record(fmt.Sprintf("nft set [%s] %s %s<->%s", ns, r.Iface, r.Public, r.Private))
}

func (f *fakeHost) DeleteFloatingIP(ns, iface string) error {
	return f.record(fmt.Sprintf("nft del [%s] %s", ns, iface))
}

func (f *fakeHost) AddrAdd(ns, iface string, prefix *net.IPNet) error {
	return f.record(fmt.Sprintf("addr [%s] %s %s", ns, iface, prefix))
}

func (f *fakeHost) LinkUp(ns, iface string) error {
	return f.record(fmt.Sprintf("up [%s] %s", ns, iface))
}

func (f *fakeHost) TableRouteDev(ns string, table int, dst *net.IPNet, iface string) error {
	return f.record(fmt.Sprintf("table %d [%s] %s dev %s", table, ns, dst, iface))
}

func (f *fakeHost) TableDefaultVia(ns string, table int, gw net.IP, iface string) error {
	return f.record(fmt.Sprintf("table %d [%s] default via %s dev %s", table, ns, gw, iface))
}

func rule(src net.IP, dst *net.IPNet, table, priority int) string {
	to := "all"
	if dst != nil {
		to = dst.String()
	}
	return fmt.Sprintf("%d from %s to %s lookup %d", priority, src, to, table)
}

func (f *fakeHost) RuleAdd(ns string, src net.IP, dst *net.IPNet, table, priority int) error {
	return f.record(fmt.Sprintf("rule add [%s] %s", ns, rule(src, dst, table, priority)))
}

func (f *fakeHost) RuleDel(ns string, src net.IP, dst *net.IPNet, table, priority int) error {
	return f.record(fmt.Sprintf("rule del [%s] %s", ns, rule(src, dst, table, priority)))
}

func (f *fakeHost) DeleteLink(ns, name string) error {
	return f.record(fmt.Sprintf("del [%s] %s", ns, name))
}

func labFloatingIP(t *testing.T) floatingIP {
	t.Helper()
	ip, public, err := net.ParseCIDR("240.0.0.47/24")
	if err != nil {
		t.Fatal(err)
	}
	public.IP = ip
	_, cidr, _ := net.ParseCIDR("10.47.0.0/16")
	f, err := build(Target{VPC: "vp-t47", CIDR: cidr, Private: net.ParseIP("10.47.1.10")}, public, net.ParseIP("240.0.0.1"))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestBuildDerivesInterfaceMACAndTable(t *testing.T) {
	f := labFloatingIP(t)
	if f.iface != "fip-f000002f" || f.mac.String() != "02:05:f0:00:00:2f" || f.table != 0x0a2f010a {
		t.Errorf("interface %s, MAC %s, table %d", f.iface, f.mac, f.table)
	}
}

func TestBuildRefusesAReservedTable(t *testing.T) {
	_, public, _ := net.ParseCIDR("240.0.0.47/24")
	if _, err := build(Target{VPC: "vp-t47", Private: net.ParseIP("0.0.0.254")}, public, nil); err == nil || !strings.Contains(err.Error(), "reserved routing table 254") {
		t.Errorf("erreur %v", err)
	}
}

func TestCreateTranslatesBeforeRoutingThroughTheFloatingIP(t *testing.T) {
	f := &fakeHost{}
	if err := create(f, labFloatingIP(t)); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"macvlan [vp-t47] fip-f000002f parent br-public mac 02:05:f0:00:00:2f",
		"fdb add [vp-t47] br-public 02:05:f0:00:00:2f",
		"sysctl [vp-t47] net.ipv4.conf.fip-f000002f.arp_ignore=1",
		"sysctl [vp-t47] net.ipv4.conf.fip-f000002f.arp_announce=2",
		"sysctl [vp-t47] net.ipv4.conf.fip-f000002f.arp_notify=1",
		"nft set [vp-t47] fip-f000002f 240.0.0.47<->10.47.1.10",
		"addr [vp-t47] fip-f000002f 240.0.0.47/32",
		"up [vp-t47] fip-f000002f",
		"table 170852618 [vp-t47] 240.0.0.0/24 dev fip-f000002f",
		"table 170852618 [vp-t47] default via 240.0.0.1 dev fip-f000002f",
		"rule add [vp-t47] 100 from 10.47.1.10 to 10.47.0.0/16 lookup 254",
		"rule add [vp-t47] 101 from 10.47.1.10 to all lookup 170852618",
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("appels :\n%s\nattendu :\n%s", strings.Join(f.calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestCreateRoutesNothingThroughTheFloatingIPWithoutTranslation(t *testing.T) {
	f := &fakeHost{fail: map[string]error{"nft set ": errors.New("nft absent")}}
	err := create(f, labFloatingIP(t))
	if err == nil || !strings.Contains(err.Error(), "floating ip 240.0.0.47 of 10.47.1.10: translation rules: nft absent") {
		t.Fatalf("erreur %v", err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "rule ") || strings.HasPrefix(c, "table ") || strings.HasPrefix(c, "addr ") {
			t.Errorf("étape exécutée sans traduction : %q", c)
		}
	}
}

func TestRemoveDropsTheRulesFirstThenUndoesTheHost(t *testing.T) {
	f := &fakeHost{}
	if err := remove(f, labFloatingIP(t)); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"rule del [vp-t47] 101 from 10.47.1.10 to all lookup 170852618",
		"rule del [vp-t47] 100 from 10.47.1.10 to 10.47.0.0/16 lookup 254",
		"nft del [vp-t47] fip-f000002f",
		"fdb del [vp-t47] br-public 02:05:f0:00:00:2f",
		"del [vp-t47] fip-f000002f",
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("appels :\n%s\nattendu :\n%s", strings.Join(f.calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestRemoveStopsAtTheFirstFailure(t *testing.T) {
	f := &fakeHost{fail: map[string]error{"rule del [vp-t47] 101": errors.New("netns absent")}}
	if err := remove(f, labFloatingIP(t)); err == nil || len(f.calls) != 1 {
		t.Errorf("erreur %v, appels %q", err, f.calls)
	}
}

func seedVM(t *testing.T) *badger.DB {
	t.Helper()
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })
	for key, value := range map[string]string{
		"vpc/vp-t47/cidr":       "10.47.0.0/16",
		"subnet/sn-a/mode":      "vxlan",
		"subnet/sn-b/mode":      "public_ip",
		"subnet/sn-a/vpc":       "vp-t47",
		"subnet/sn-b/vpc":       "vp-t47",
		"vm/vm-1/nic/0/name":    "nic-a1b2",
		"vm/vm-1/nic/0/subnet":  "sn-a",
		"vm/vm-1/nic/0/ip":      "10.47.1.10",
		"vm/vm-1/nic/1/name":    "nic-c3d4",
		"vm/vm-1/nic/1/subnet":  "sn-b",
		"vm/vm-1/nic/1/ip":      "10.47.2.10",
		"vm/vm-10/nic/0/name":   "nic-a1b2",
		"vm/vm-10/nic/0/subnet": "sn-a",
		"vm/vm-10/nic/0/ip":     "10.47.1.99",
	} {
		kv.AddInDB(db, key, value)
	}
	return db
}

func TestResolveFindsTheInterfaceByName(t *testing.T) {
	got, err := Resolve(seedVM(t), "vm-1", "nic-c3d4")
	if err != nil {
		t.Fatal(err)
	}
	if got.VPC != "vp-t47" || got.CIDR.String() != "10.47.0.0/16" || got.Private.String() != "10.47.2.10" || got.Subnet != "sn-b" || got.Mode != "public_ip" {
		t.Errorf("cible %+v", got)
	}
}

func TestResolveIgnoresAVMWhoseNameExtendsTheOne(t *testing.T) {
	got, err := Resolve(seedVM(t), "vm-1", "nic-a1b2")
	if err != nil || got.Private.String() != "10.47.1.10" {
		t.Errorf("cible %+v, erreur %v", got, err)
	}
}

func TestResolveRefusesAnUnknownInterface(t *testing.T) {
	if _, err := Resolve(seedVM(t), "vm-1", "nic-zzzz"); err == nil || !strings.Contains(err.Error(), `vm "vm-1" has no interface "nic-zzzz"`) {
		t.Errorf("erreur %v", err)
	}
}

func TestIfaceFitsTheKernelLimit(t *testing.T) {
	name, err := Iface(net.ParseIP("255.255.255.255"))
	if err != nil || name != "fip-ffffffff" || len(name) > 15 {
		t.Errorf("%q (%d caractères), %v", name, len(name), err)
	}
	if _, err := Iface(net.ParseIP("2001:db8::1")); err == nil {
		t.Error("IPv6 acceptée")
	}
}
