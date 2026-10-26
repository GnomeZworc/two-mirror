package watchdog

import (
	"errors"
	"testing"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
)

type fakePublicProbe struct {
	problems map[string]string
	ports    []string
	err      error
	asked    []string
}

func (f *fakePublicProbe) LinkProblem(name string) string {
	f.asked = append(f.asked, name)
	return f.problems[name]
}

func (f *fakePublicProbe) VxlanPorts(bridge string) ([]string, error) {
	f.asked = append(f.asked, "ports:"+bridge)
	return f.ports, f.err
}

func TestCheckPublicSegment_SilentWithAVxlanPort(t *testing.T) {
	r := &recorder{}
	p := &fakePublicProbe{ports: []string{"vxlan-public"}}
	CheckPublicSegment(&configuration.Config{}, p, r)
	if len(r.calls) != 0 {
		t.Errorf("notifications = %+v, aucune attendue", r.calls)
	}
	if len(p.asked) != 2 || p.asked[0] != "br-public" || p.asked[1] != "ports:br-public" {
		t.Errorf("sondes = %q, br-public attendu par défaut", p.asked)
	}
}

func TestCheckPublicSegment_FollowsInterfacesInternet(t *testing.T) {
	r := &recorder{}
	p := &fakePublicProbe{ports: []string{"vxlan-public"}}
	CheckPublicSegment(&configuration.Config{DefaultInterface: "br-000000", Interfaces: map[string]string{"internet": "br-wan"}}, p, r)
	if len(p.asked) != 2 || p.asked[0] != "br-wan" || p.asked[1] != "ports:br-wan" {
		t.Errorf("sondes = %q, br-wan attendu", p.asked)
	}
}

func TestCheckPublicSegment_ReportsAMissingBridgeWithoutReadingPorts(t *testing.T) {
	r := &recorder{}
	p := &fakePublicProbe{problems: map[string]string{"br-public": "interface br-public not found: Link not found"}}
	CheckPublicSegment(&configuration.Config{}, p, r)
	got := r.forName("br-public")
	want := "interface br-public not found: Link not found (public bridge, interfaces.internet): the vpcs of this host have no public segment"
	if len(got) != 1 || got[0].kind != "public" || got[0].problem != want {
		t.Fatalf("notifications = %+v, une attendue : %q", r.calls, want)
	}
	if len(p.asked) != 1 {
		t.Errorf("ports lus alors que le bridge manque : %q", p.asked)
	}
}

func TestCheckPublicSegment_ReportsABridgeWithoutVxlan(t *testing.T) {
	r := &recorder{}
	CheckPublicSegment(&configuration.Config{}, &fakePublicProbe{}, r)
	got := r.forName("br-public")
	if len(got) != 1 || got[0].problem != "no vxlan port: the public segment is unreachable from this host" {
		t.Errorf("notifications = %+v", r.calls)
	}
}

func TestCheckPublicSegment_ReportsUnreadablePorts(t *testing.T) {
	r := &recorder{}
	CheckPublicSegment(&configuration.Config{}, &fakePublicProbe{err: errors.New("netlink: permission denied")}, r)
	if !r.hasProblemContaining("vxlan ports unreadable: netlink: permission denied") {
		t.Errorf("notifications = %+v", r.calls)
	}
}
