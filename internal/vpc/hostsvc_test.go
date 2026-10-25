package vpc

import (
	"errors"
	"net"
	"reflect"
	"testing"
)

type fakeHostSvc struct {
	calls  []string
	inside string
	failOn string
}

func (f *fakeHostSvc) record(kind, detail string) error {
	call := kind + " " + detail
	if f.inside != "" {
		call = f.inside + ": " + call
	}
	f.calls = append(f.calls, call)
	if kind == f.failOn {
		return errors.New("boom")
	}
	return nil
}

func (f *fakeHostSvc) InNetns(vpc string, fn func() error) error {
	f.inside = vpc
	defer func() { f.inside = "" }()
	return fn()
}

func (f *fakeHostSvc) CreateDummy(name string) error { return f.record("dummy", name) }
func (f *fakeHostSvc) AddrAdd(iface string, ip net.IP) error {
	return f.record("addr", ip.String()+" "+iface)
}
func (f *fakeHostSvc) AddRedirect(target string) error { return f.record("redirect", target) }
func (f *fakeHostSvc) Open(vpc string) error           { return f.record("open", vpc) }
func (f *fakeHostSvc) Close(vpc string) error          { return f.record("close", vpc) }

func TestSetupHostSvc_OrderAndPlace(t *testing.T) {
	f := &fakeHostSvc{}
	if err := setupHostSvc("vpc-00001", f); err != nil {
		t.Fatalf("setupHostSvc: %v", err)
	}
	want := []string{
		"vpc-00001: dummy hostsvc",
		"vpc-00001: addr 169.254.169.253 hostsvc",
		"vpc-00001: redirect 169.254.169.253",
		"open vpc-00001",
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("calls = %v, want %v: the address and the redirection live in the vpc netns, the open is asked from the host once they exist", f.calls, want)
	}
}

func TestSetupHostSvc_StopsAtTheFirstFailure(t *testing.T) {
	for _, c := range []struct {
		failOn string
		want   string
		last   string
	}{
		{"dummy", "hostsvc in vpc-00001: create hostsvc: boom", "vpc-00001: dummy hostsvc"},
		{"addr", "hostsvc in vpc-00001: address 169.254.169.253 on hostsvc: boom", "vpc-00001: addr 169.254.169.253 hostsvc"},
		{"redirect", "hostsvc in vpc-00001: redirect dns and ntp to 169.254.169.253: boom", "vpc-00001: redirect 169.254.169.253"},
		{"open", "hostsvc open vpc-00001: boom", "open vpc-00001"},
	} {
		f := &fakeHostSvc{failOn: c.failOn}
		err := setupHostSvc("vpc-00001", f)
		if err == nil || err.Error() != c.want {
			t.Errorf("fail on %s: error %v, want %q", c.failOn, err, c.want)
		}
		if got := f.calls[len(f.calls)-1]; got != c.last {
			t.Errorf("fail on %s: last call %q, want %q: nothing must run after a failure", c.failOn, got, c.last)
		}
	}
}

func TestTeardownHostSvc(t *testing.T) {
	f := &fakeHostSvc{}
	if err := teardownHostSvc("vpc-00001", f); err != nil {
		t.Fatalf("teardownHostSvc: %v", err)
	}
	if !reflect.DeepEqual(f.calls, []string{"close vpc-00001"}) {
		t.Errorf("calls = %v: only the close is needed, the dummy and the rules go away with the netns", f.calls)
	}
	f = &fakeHostSvc{failOn: "close"}
	if err := teardownHostSvc("vpc-00001", f); err == nil || err.Error() != "hostsvc close vpc-00001: boom" {
		t.Errorf("error %v: a failed close must stop the deletion", err)
	}
}
