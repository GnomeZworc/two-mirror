package vpc

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/frr"
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

func (f *fakeHost) VTEPAddress(iface string) (net.IP, error) {
	if err := f.record("vtep " + iface); err != nil {
		return nil, err
	}
	return net.ParseIP("192.168.14.11"), nil
}

func (f *fakeHost) Sysctl(ns, key, value string) error {
	return f.record(fmt.Sprintf("sysctl [%s] %s=%s", ns, key, value))
}

func (f *fakeHost) CreateVRF(name string, table uint32) error {
	return f.record(fmt.Sprintf("vrf %s table %d", name, table))
}

func (f *fakeHost) CreateBridge(name string, mtu int) error {
	return f.record(fmt.Sprintf("bridge %s mtu %d", name, mtu))
}

func (f *fakeHost) SetHardwareAddr(name string, mac net.HardwareAddr) error {
	return f.record(fmt.Sprintf("mac %s %s", name, mac))
}

func (f *fakeHost) SetMaster(iface, master string) error {
	return f.record(fmt.Sprintf("master %s %s", iface, master))
}

func (f *fakeHost) CreateVxlan(name string, vni int, iface string, mtu int) error {
	return f.record(fmt.Sprintf("vxlan %s id %d dev %s mtu %d", name, vni, iface, mtu))
}

func (f *fakeHost) CreateVeth(hostIf, nsIf, ns string, mtu int) error {
	return f.record(fmt.Sprintf("veth %s peer %s netns %s mtu %d", hostIf, nsIf, ns, mtu))
}

func (f *fakeHost) AddrAdd(ns, iface string, prefix *net.IPNet) error {
	return f.record(fmt.Sprintf("addr [%s] %s %s", ns, iface, prefix))
}

func (f *fakeHost) LinkUp(ns, iface string) error {
	return f.record(fmt.Sprintf("up [%s] %s", ns, iface))
}

func (f *fakeHost) RouteVia(ns string, dst *net.IPNet, gw net.IP, iface string) error {
	return f.record(fmt.Sprintf("route [%s] %s via %s dev %s", ns, dst, gw, iface))
}

func (f *fakeHost) DeleteLink(name string) error {
	return f.record("del " + name)
}

func (f *fakeHost) TableRouteVia(table uint32, dst *net.IPNet, gw net.IP, iface string) error {
	return f.record(fmt.Sprintf("table %d route %s via %s dev %s", table, dst, gw, iface))
}

func (f *fakeHost) TableRouteDel(table uint32, dst *net.IPNet) error {
	return f.record(fmt.Sprintf("table %d del %s", table, dst))
}

func (f *fakeHost) ApplyVRF(v frr.VRF) error {
	return f.record(fmt.Sprintf("frr apply asn=%d id=%s vni=%d vtep=%s supernet=%s", v.ASN, v.ID, v.L3VNI, v.VTEP, v.Supernet))
}

func (f *fakeHost) WithdrawVRF(v frr.VRF) error {
	return f.record(fmt.Sprintf("frr withdraw asn=%d id=%s vni=%d", v.ASN, v.ID, v.L3VNI))
}

func (f *fakeHost) RemoveVRF(v frr.VRF) error {
	return f.record(fmt.Sprintf("frr remove asn=%d id=%s vni=%d", v.ASN, v.ID, v.L3VNI))
}

func bgpConfig() *configuration.Config {
	cfg := &configuration.Config{DefaultInterface: "br-000000"}
	cfg.BGP.ASN = 64600
	cfg.BGP.RouteReflectors = []string{"10.255.255.1"}
	return cfg
}

func labL3(t *testing.T) l3VPC {
	t.Helper()
	_, supernet, err := net.ParseCIDR("10.241.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	return l3VPC{netns: "vp-s7", id: "s7", vni: 2700, supernet: supernet}
}

func TestCreateL3FollowsTheLabRecipe(t *testing.T) {
	f := &fakeHost{}
	if err := createL3(f, bgpConfig(), labL3(t)); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"vtep br-000000",
		"vrf vrf-s7 table 2700",
		"bridge brl3-s7 mtu 1500",
		"mac brl3-s7 02:02:c0:a8:0e:0b",
		"master brl3-s7 vrf-s7",
		"vxlan vxl3-s7 id 2700 dev br-000000 mtu 1500",
		"master vxl3-s7 brl3-s7",
		"up [] vxl3-s7",
		"veth l3-s7-o peer l3-s7-i netns vp-s7 mtu 1500",
		"master l3-s7-o vrf-s7",
		"addr [] l3-s7-o 169.254.0.1/30",
		"up [] l3-s7-o",
		"sysctl [] net.ipv4.conf.brl3-s7.forwarding=1",
		"sysctl [] net.ipv4.conf.l3-s7-o.forwarding=1",
		"addr [vp-s7] l3-s7-i 169.254.0.2/30",
		"up [vp-s7] l3-s7-i",
		"sysctl [vp-s7] net.ipv4.ip_forward=1",
		"route [vp-s7] 10.241.0.0/16 via 169.254.0.1 dev l3-s7-i",
		"frr apply asn=64600 id=s7 vni=2700 vtep=192.168.14.11 supernet=10.241.0.0/16",
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("appels :\n%s\nattendu :\n%s", strings.Join(f.calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestCreateL3RequiresTheBGPSectionBeforeTouchingTheHost(t *testing.T) {
	f := &fakeHost{}
	cfg := &configuration.Config{DefaultInterface: "br-000000"}
	if err := createL3(f, cfg, labL3(t)); err == nil || !strings.Contains(err.Error(), "no bgp section") {
		t.Errorf("erreur %v", err)
	}
	if err := createL3(f, nil, labL3(t)); err == nil {
		t.Error("config absente acceptée")
	}
	if len(f.calls) != 0 {
		t.Errorf("appels : %q", f.calls)
	}
}

func TestCreateL3StopsAtTheFirstFailure(t *testing.T) {
	f := &fakeHost{fail: map[string]error{"vxlan ": errors.New("file exists")}}
	err := createL3(f, bgpConfig(), labL3(t))
	if err == nil || !strings.Contains(err.Error(), "l3vni 2700: create vxl3-s7: file exists") {
		t.Errorf("erreur %v", err)
	}
	if last := f.calls[len(f.calls)-1]; !strings.HasPrefix(last, "vxlan ") {
		t.Errorf("appels après l'échec : %q", f.calls)
	}
}

func TestDeleteL3WithdrawsFRRThenTheKernelThenTheVRF(t *testing.T) {
	f := &fakeHost{}
	if err := deleteL3(f, bgpConfig(), labL3(t)); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"frr withdraw asn=64600 id=s7 vni=0",
		"del l3-s7-o",
		"del vxl3-s7",
		"del brl3-s7",
		"del vrf-s7",
		"frr remove asn=64600 id=s7 vni=0",
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("appels :\n%s\nattendu :\n%s", strings.Join(f.calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestDeleteL3KeepsTheKernelWhenFRRRefusesTheWithdrawal(t *testing.T) {
	f := &fakeHost{fail: map[string]error{"frr withdraw": errors.New("% Please unconfigure l3vni 2700")}}
	err := deleteL3(f, bgpConfig(), labL3(t))
	if err == nil || !strings.Contains(err.Error(), "withdraw frr") {
		t.Errorf("erreur %v", err)
	}
	if len(f.calls) != 1 {
		t.Errorf("le noyau a été touché malgré le refus de FRR : %q", f.calls)
	}
}

func TestDeleteL3KeepsTheFRRVRFWhenAKernelLinkCannotBeDeleted(t *testing.T) {
	f := &fakeHost{fail: map[string]error{"del brl3-s7": errors.New("device busy")}}
	if err := deleteL3(f, bgpConfig(), labL3(t)); err == nil || !strings.Contains(err.Error(), "delete brl3-s7") {
		t.Errorf("erreur %v", err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "frr remove") || c == "del vrf-s7" {
			t.Errorf("étape exécutée après l'échec : %q", f.calls)
		}
	}
}

func TestDeleteL3RequiresTheBGPSection(t *testing.T) {
	f := &fakeHost{}
	if err := deleteL3(f, &configuration.Config{}, labL3(t)); err == nil || len(f.calls) != 0 {
		t.Errorf("erreur %v, appels %q", err, f.calls)
	}
}

func TestRouterMAC(t *testing.T) {
	mac, err := routerMAC(net.ParseIP("192.168.14.11"))
	if err != nil || mac.String() != "02:02:c0:a8:0e:0b" {
		t.Errorf("routerMAC = %v, %v", mac, err)
	}
	if _, err := routerMAC(net.ParseIP("fd00::1")); err == nil {
		t.Error("VTEP IPv6 accepté")
	}
}

func TestLoadL3(t *testing.T) {
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })
	kv.AddInDB(db, "vpc/vp-a/cidr", "10.241.0.0/16")
	if _, ok, err := loadL3(db, "vp-a", "a"); ok || err != nil {
		t.Errorf("VPC sans l3vni : ok=%v err=%v", ok, err)
	}
	kv.AddInDB(db, "vpc/vp-a/l3vni", "2700")
	got, ok, err := loadL3(db, "vp-a", "a")
	if !ok || err != nil || got.vni != 2700 || got.id != "a" || got.netns != "vp-a" || got.supernet.String() != "10.241.0.0/16" {
		t.Errorf("loadL3 = %+v, %v, %v", got, ok, err)
	}
	for _, bad := range []string{"abc", "254", "0"} {
		kv.AddInDB(db, "vpc/vp-a/l3vni", bad)
		if _, _, err := loadL3(db, "vp-a", "a"); err == nil {
			t.Errorf("l3vni %q acceptée", bad)
		}
	}
	kv.AddInDB(db, "vpc/vp-b/l3vni", "2700")
	if _, _, err := loadL3(db, "vp-b", "b"); err == nil || !strings.Contains(err.Error(), "no cidr") {
		t.Errorf("VPC sans cidr : %v", err)
	}
}
