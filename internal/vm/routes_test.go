package vm

import (
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"

	"github.com/dgraph-io/badger/v4"
)

func routedVM() vmData {
	return vmData{nics: []nicData{
		{index: 0, vpcName: "vp-s7", mode: "vxlan", ip: "10.241.1.10"},
		{index: 1, vpcName: "vp-s7", mode: "bridge", ip: "192.168.50.10"},
		{index: 2, vpcName: "vp-s7", mode: "vxlan", ip: "10.241.2.10"},
	}}
}

type recorder struct {
	calls []string
	fail  string
}

func (r *recorder) apply(_ *badger.DB, vpcName string, dst *net.IPNet) error {
	r.calls = append(r.calls, vpcName+" "+dst.String())
	if dst.String() == r.fail {
		return errors.New("refused")
	}
	return nil
}

func TestApplyHostRoutesCoversEveryVxlanInterfaceAsA32(t *testing.T) {
	r := &recorder{}
	if err := applyHostRoutes(nil, routedVM(), r.apply); err != nil {
		t.Fatal(err)
	}
	want := []string{"vp-s7 10.241.1.10/32", "vp-s7 10.241.2.10/32"}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("appels %q, attendu %q", r.calls, want)
	}
}

func TestApplyHostRoutesStopsAtTheFirstError(t *testing.T) {
	r := &recorder{fail: "10.241.1.10/32"}
	if err := applyHostRoutes(nil, routedVM(), r.apply); err == nil || err.Error() != "refused" {
		t.Errorf("erreur %v", err)
	}
	if len(r.calls) != 1 {
		t.Errorf("appels après l'échec : %q", r.calls)
	}
}

func TestApplyHostRoutesRefusesANonIPv4AddressBeforeAnyCall(t *testing.T) {
	d := routedVM()
	d.nics[2].ip = "fd00::10"
	r := &recorder{}
	err := applyHostRoutes(nil, d, r.apply)
	if err == nil || !strings.Contains(err.Error(), `interface 2: "fd00::10" is not an IPv4 address`) {
		t.Errorf("erreur %v", err)
	}
	if len(r.calls) != 0 {
		t.Errorf("appels : %q", r.calls)
	}
}
