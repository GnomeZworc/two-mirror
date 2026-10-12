package l3vni

import (
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

func routedDB(t *testing.T) *badger.DB {
	t.Helper()
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })
	kv.AddInDB(db, "vpc/vp-s7/cidr", "10.241.0.0/16")
	kv.AddInDB(db, "vpc/vp-s7/l3vni", "2700")
	kv.AddInDB(db, "vpc/vp-flat/cidr", "10.242.0.0/16")
	return db
}

func cidr(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestExportRouteInstallsTheRouteInTheVRFTowardsTheNetns(t *testing.T) {
	f := &fakeHost{}
	if err := exportRoute(f, routedDB(t), "vp-s7", cidr(t, "10.241.1.0/24")); err != nil {
		t.Fatal(err)
	}
	want := []string{"table 2700 route 10.241.1.0/24 via 169.254.0.2 dev l3-s7-o"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("appels %q, attendu %q", f.calls, want)
	}
}

func TestExportRouteRefusesAPrefixOutsideTheVPC(t *testing.T) {
	f := &fakeHost{}
	err := exportRoute(f, routedDB(t), "vp-s7", cidr(t, "10.99.1.0/24"))
	if err == nil || !strings.Contains(err.Error(), "outside 10.241.0.0/16") || len(f.calls) != 0 {
		t.Errorf("erreur %v, appels %q", err, f.calls)
	}
}

func TestWithdrawRouteDeletesFromTheVRFTable(t *testing.T) {
	f := &fakeHost{}
	if err := withdrawRoute(f, routedDB(t), "vp-s7", cidr(t, "10.241.1.10/32")); err != nil {
		t.Fatal(err)
	}
	want := []string{"table 2700 del 10.241.1.10/32"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("appels %q, attendu %q", f.calls, want)
	}
}

func TestRoutesAreIgnoredForAVPCWithoutL3VNI(t *testing.T) {
	f := &fakeHost{}
	db := routedDB(t)
	if err := exportRoute(f, db, "vp-flat", cidr(t, "10.242.1.0/24")); err != nil {
		t.Error(err)
	}
	if err := withdrawRoute(f, db, "vp-flat", cidr(t, "10.242.1.0/24")); err != nil {
		t.Error(err)
	}
	if len(f.calls) != 0 {
		t.Errorf("appels %q", f.calls)
	}
}

func TestRouteErrorsArePropagated(t *testing.T) {
	db := routedDB(t)
	f := &fakeHost{fail: map[string]error{"table ": errors.New("network unreachable")}}
	if err := exportRoute(f, db, "vp-s7", cidr(t, "10.241.1.0/24")); err == nil || !strings.Contains(err.Error(), "export 10.241.1.0/24 in vrf-s7") {
		t.Errorf("export : %v", err)
	}
	if err := withdrawRoute(f, db, "vp-s7", cidr(t, "10.241.1.0/24")); err == nil || !strings.Contains(err.Error(), "withdraw 10.241.1.0/24 from vrf-s7") {
		t.Errorf("withdraw : %v", err)
	}
	if err := exportRoute(f, db, "novpc", cidr(t, "10.241.1.0/24")); err == nil {
		t.Error("nom de VPC sans tiret accepté")
	}
}
