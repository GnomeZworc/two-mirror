package watchdog

import (
	"errors"
	"testing"

	"git.g3e.fr/syonad/two/internal/state"
)

type fakeHostSvcProbe struct {
	served    []string
	servedErr error
	address   map[string]bool
	redirect  map[string]bool
	readErr   error
	asked     int
}

func (f *fakeHostSvcProbe) Served() ([]string, error) {
	f.asked++
	return f.served, f.servedErr
}

func (f *fakeHostSvcProbe) ServiceAddress(vpc string) (bool, error) {
	return f.address[vpc], f.readErr
}

func (f *fakeHostSvcProbe) Redirect(vpc string) (bool, error) {
	return f.redirect[vpc], f.readErr
}

func healthyHostSvc(vpcs ...string) *fakeHostSvcProbe {
	f := &fakeHostSvcProbe{served: vpcs, address: map[string]bool{}, redirect: map[string]bool{}}
	for _, v := range vpcs {
		f.address[v] = true
		f.redirect[v] = true
	}
	return f
}

func TestCheckHostSvc_SilentWhenEverythingIsInPlace(t *testing.T) {
	db := newTestDB(t)
	seedResource(t, db, prefixVPC, "vp-a", state.Running)
	r := &recorder{}
	if err := CheckHostSvc(db, healthyHostSvc("vp-a"), r); err != nil {
		t.Fatalf("CheckHostSvc: %v", err)
	}
	if len(r.calls) != 0 {
		t.Errorf("notifications = %+v, want none", r.calls)
	}
}

func TestCheckHostSvc_NoRunningVPCDoesNotAskTheRelay(t *testing.T) {
	db := newTestDB(t)
	seedResource(t, db, prefixVPC, "vp-a", state.Creating)
	p := &fakeHostSvcProbe{servedErr: errors.New("down")}
	r := &recorder{}
	if err := CheckHostSvc(db, p, r); err != nil {
		t.Fatalf("CheckHostSvc: %v", err)
	}
	if p.asked != 0 || len(r.calls) != 0 {
		t.Errorf("asked %d times, notifications %+v: a host without running vpc needs no relay", p.asked, r.calls)
	}
}

func TestCheckHostSvc_ReportsAnUnreachableRelay(t *testing.T) {
	db := newTestDB(t)
	seedResource(t, db, prefixVPC, "vp-a", state.Running)
	p := healthyHostSvc("vp-a")
	p.servedErr = errors.New("dial /run/two/hostsvc/hostsvc.sock: connection refused")
	r := &recorder{}
	if err := CheckHostSvc(db, p, r); err != nil {
		t.Fatalf("CheckHostSvc: %v", err)
	}
	got := r.forName("two-hostsvc.service")
	if len(got) != 1 || got[0].kind != "hostsvc" {
		t.Fatalf("notifications = %+v, want one for two-hostsvc.service", r.calls)
	}
	if !r.hasProblemContaining("unreachable, no vpc of this host gets dns or ntp: dial /run/two/hostsvc/hostsvc.sock: connection refused") {
		t.Errorf("problem = %q", got[0].problem)
	}
}

func TestCheckHostSvc_ReportsEachMissingPiece(t *testing.T) {
	db := newTestDB(t)
	for _, v := range []string{"vp-ok", "vp-unserved", "vp-noaddr", "vp-noredirect"} {
		seedResource(t, db, prefixVPC, v, state.Running)
	}
	p := healthyHostSvc("vp-ok", "vp-unserved", "vp-noaddr", "vp-noredirect")
	p.served = []string{"vp-ok", "vp-noaddr", "vp-noredirect"}
	p.address["vp-noaddr"] = false
	p.redirect["vp-noredirect"] = false
	r := &recorder{}
	if err := CheckHostSvc(db, p, r); err != nil {
		t.Fatalf("CheckHostSvc: %v", err)
	}
	for name, want := range map[string]string{
		"vp-unserved":   "not served by two-hostsvc: its vms get no dns nor ntp",
		"vp-noaddr":     "169.254.169.253 missing on hostsvc (in netns)",
		"vp-noredirect": "dns and ntp redirection to 169.254.169.253 missing or incomplete (chain TWO-HOSTSVC)",
	} {
		got := r.forName(name)
		if len(got) != 1 || got[0].kind != "vpc" || got[0].problem != want {
			t.Errorf("%s: notifications %+v, want one vpc problem %q", name, got, want)
		}
	}
	if got := r.forName("vp-ok"); len(got) != 0 {
		t.Errorf("vp-ok: notifications %+v, want none", got)
	}
}

func TestCheckHostSvc_ReportsUnreadableChecks(t *testing.T) {
	db := newTestDB(t)
	seedResource(t, db, prefixVPC, "vp-a", state.Running)
	p := healthyHostSvc("vp-a")
	p.readErr = errors.New("cannot enter netns")
	r := &recorder{}
	if err := CheckHostSvc(db, p, r); err != nil {
		t.Fatalf("CheckHostSvc: %v", err)
	}
	for _, want := range []string{
		"169.254.169.253 on hostsvc unreadable: cannot enter netns",
		"dns and ntp redirection unreadable: cannot enter netns",
	} {
		if !r.hasProblemContaining(want) {
			t.Errorf("missing %q in %+v", want, r.calls)
		}
	}
}
