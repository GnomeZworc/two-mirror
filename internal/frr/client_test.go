package frr

import (
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"
)

type fakeVtysh struct {
	calls   [][]string
	running []string
	fail    map[string]string
	status  map[string]error
}

func (f *fakeVtysh) run(args ...string) ([]byte, error) {
	var cmds []string
	for i := 1; i < len(args); i += 2 {
		cmds = append(cmds, args[i])
	}
	f.calls = append(f.calls, cmds)
	last := cmds[len(cmds)-1]
	if last == "show running-config" {
		if len(f.running) == 0 {
			return nil, errors.New("show running-config inattendu")
		}
		out := f.running[0]
		f.running = f.running[1:]
		return []byte(out), nil
	}
	for _, c := range cmds {
		if out, ok := f.fail[c]; ok {
			return []byte(out), f.status[c]
		}
	}
	return nil, nil
}

func (f *fakeVtysh) client() *Client {
	return &Client{run: f.run}
}

func (f *fakeVtysh) sent(cmd string) bool {
	for _, c := range f.calls {
		for _, x := range c {
			if x == cmd {
				return true
			}
		}
	}
	return false
}

func TestApplyVRFConfiguresThenSaves(t *testing.T) {
	f := &fakeVtysh{}
	if err := f.client().ApplyVRF(labVRF(t)); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		append([]string{"configure terminal"}, labVRF(t).commands()...),
		{"write memory"},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("appels :\n%q\nattendu :\n%q", f.calls, want)
	}
}

func TestApplyVRFRefusesAnInvalidVRFWithoutCallingVtysh(t *testing.T) {
	f := &fakeVtysh{}
	v := labVRF(t)
	v.L3VNI = 0
	if err := f.client().ApplyVRF(v); err == nil || !strings.Contains(err.Error(), "l3vni 0") {
		t.Errorf("erreur %v", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("vtysh appelé : %q", f.calls)
	}
}

func TestExecReportsAnErrorPrintedByVtysh(t *testing.T) {
	f := &fakeVtysh{fail: map[string]string{"vni 4001": "% Please unconfigure l3vni 4001\n"}}
	err := f.client().ApplyVRF(labVRF(t))
	if err == nil || !strings.Contains(err.Error(), "% Please unconfigure l3vni 4001") {
		t.Errorf("erreur %v", err)
	}
	if f.sent("write memory") {
		t.Error("configuration sauvegardée malgré l'erreur")
	}
}

func TestExecReportsANonZeroExit(t *testing.T) {
	f := &fakeVtysh{
		fail:   map[string]string{"write memory": "Building Configuration...\n"},
		status: map[string]error{"write memory": errors.New("exit status 1")},
	}
	err := f.client().ApplyVRF(labVRF(t))
	if err == nil || !strings.Contains(err.Error(), "exit status 1") || !strings.Contains(err.Error(), "Building Configuration...") {
		t.Errorf("erreur %v", err)
	}
}

func TestWithdrawVRFRunsEachStepInOrderThenSaves(t *testing.T) {
	f := &fakeVtysh{running: []string{fullRunning, otherOnlyRunning}}
	if err := f.client().WithdrawVRF(labVRF(t)); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"show running-config"},
		{"configure terminal", "router bgp 64600 vrf vrf-a1b2", "address-family ipv4 unicast", "no redistribute kernel"},
		{"configure terminal", "vrf vrf-a1b2", "no vni 4001"},
		{"configure terminal", "no router bgp 64600 vrf vrf-a1b2"},
		{"configure terminal", "no route-map EXPORT-a1b2"},
		{"configure terminal", "no ip prefix-list PL-a1b2"},
		{"write memory"},
		{"show running-config"},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("appels :\n%q\nattendu :\n%q", f.calls, want)
	}
}

func TestWithdrawVRFStopsAtTheFirstRefusedStep(t *testing.T) {
	f := &fakeVtysh{
		running: []string{fullRunning},
		fail:    map[string]string{"no vni 4001": "% Please unconfigure l3vni 4001\n"},
	}
	if err := f.client().WithdrawVRF(labVRF(t)); err == nil || !strings.Contains(err.Error(), "Please unconfigure") {
		t.Errorf("erreur %v", err)
	}
	if f.sent("no router bgp 64600 vrf vrf-a1b2") {
		t.Error("instance BGP supprimée après un refus de l'étape précédente")
	}
}

func TestWithdrawVRFFailsWhenFRRKeptTheConfiguration(t *testing.T) {
	f := &fakeVtysh{running: []string{fullRunning, fullRunning}}
	err := f.client().WithdrawVRF(labVRF(t))
	if err == nil || !strings.Contains(err.Error(), "configuration still present after teardown") || !strings.Contains(err.Error(), "no router bgp 64600 vrf vrf-a1b2") {
		t.Errorf("erreur %v", err)
	}
}

func TestRemoveVRFDeletesTheVRFAndChecksNothingIsLeft(t *testing.T) {
	gone := "vrf vrf-a1b2c\n vni 4002\nexit-vrf\n"
	f := &fakeVtysh{running: []string{otherOnlyRunning, gone, gone}}
	if err := f.client().RemoveVRF(labVRF(t)); err != nil {
		t.Fatal(err)
	}
	if !f.sent("no vrf vrf-a1b2") {
		t.Errorf("appels : %q", f.calls)
	}
}

func TestRemoveVRFFailsWhenRoutingConfigurationIsStillThere(t *testing.T) {
	f := &fakeVtysh{running: []string{otherOnlyRunning, "", fullRunning}}
	err := f.client().RemoveVRF(labVRF(t))
	if err == nil || !strings.Contains(err.Error(), "configuration still present after teardown") {
		t.Errorf("erreur %v", err)
	}
}

func TestTeardownNeedsOnlyTheASNAndTheID(t *testing.T) {
	f := &fakeVtysh{running: []string{"", ""}}
	if err := f.client().WithdrawVRF(VRF{ASN: 64600, ID: "a1b2"}); err != nil {
		t.Errorf("démontage refusé : %v", err)
	}
	f = &fakeVtysh{}
	if err := f.client().WithdrawVRF(VRF{ASN: 64600}); err == nil || len(f.calls) != 0 {
		t.Errorf("démontage sans id : erreur %v, appels %q", err, f.calls)
	}
}

const baseRunning = `frr version 10.7.1
router bgp 64600
 bgp router-id 192.168.14.13
 neighbor fabric peer-group
 neighbor fabric remote-as 64600
 neighbor 10.255.255.1 peer-group fabric
 neighbor 10.255.255.9 peer-group fabric
 neighbor 10.0.0.5 peer-group monitoring
 !
 address-family l2vpn evpn
  neighbor fabric activate
  advertise-all-vni
 exit-address-family
exit
!
router bgp 64600 vrf vrf-a1b2
 neighbor 10.255.255.7 peer-group fabric
exit
`

func labBase() Base {
	return Base{ASN: 64600, RouterID: net.ParseIP("192.168.14.13"), RouteReflectors: []net.IP{net.ParseIP("10.255.255.1")}}
}

func TestApplyBaseOnAFreshFRRConfiguresThenSaves(t *testing.T) {
	f := &fakeVtysh{running: []string{""}}
	if err := f.client().ApplyBase(labBase()); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"show running-config"},
		append([]string{"configure terminal"}, labBase().commands()...),
		{"write memory"},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("appels :\n%q\nattendu :\n%q", f.calls, want)
	}
}

func TestApplyBaseRemovesReflectorsNoLongerConfiguredAfterAddingTheNewOnes(t *testing.T) {
	f := &fakeVtysh{running: []string{baseRunning}}
	if err := f.client().ApplyBase(labBase()); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"show running-config"},
		append([]string{"configure terminal"}, labBase().commands()...),
		{"configure terminal", "router bgp 64600", "no neighbor 10.255.255.9", "exit"},
		{"write memory"},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("appels :\n%q\nattendu :\n%q", f.calls, want)
	}
}

func TestApplyBaseRefusesToChangeTheASN(t *testing.T) {
	f := &fakeVtysh{running: []string{baseRunning}}
	b := labBase()
	b.ASN = 64601
	err := f.client().ApplyBase(b)
	if err == nil || !strings.Contains(err.Error(), "bgp already runs as AS 64600, not 64601") {
		t.Errorf("erreur %v", err)
	}
	if len(f.calls) != 1 {
		t.Errorf("appels après le refus : %q", f.calls)
	}
}

func TestApplyBaseStopsWhenTheRunningConfigIsUnreadable(t *testing.T) {
	f := &fakeVtysh{}
	if err := f.client().ApplyBase(labBase()); err == nil {
		t.Fatal("aucune erreur")
	}
	if f.sent("write memory") || f.sent("router bgp 64600") {
		t.Errorf("appels : %q", f.calls)
	}
}
