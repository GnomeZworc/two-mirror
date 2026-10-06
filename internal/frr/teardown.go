package frr

import (
	"fmt"
	"strings"
)

type step []string

func routingTeardown(v VRF, running string) []step {
	lines := strings.Split(running, "\n")
	bgp := hasLine(lines, v.bgpInstance())
	var steps []step
	if bgp {
		steps = append(steps, step{v.bgpInstance(), "address-family ipv4 unicast", "no redistribute kernel"})
	}
	if vni := vrfVNI(lines, v.Name()); vni != "" {
		steps = append(steps, step{"vrf " + v.Name(), "no vni " + vni})
	}
	if bgp {
		steps = append(steps, step{"no " + v.bgpInstance()})
	}
	if hasPrefix(lines, "route-map "+v.routeMap()+" ") {
		steps = append(steps, step{"no route-map " + v.routeMap()})
	}
	if hasPrefix(lines, "ip prefix-list "+v.prefixList()+" ") {
		steps = append(steps, step{"no ip prefix-list " + v.prefixList()})
	}
	return steps
}

func vrfTeardown(v VRF, running string) []step {
	if hasLine(strings.Split(running, "\n"), "vrf "+v.Name()) {
		return []step{{"no vrf " + v.Name()}}
	}
	return nil
}

func hasLine(lines []string, want string) bool {
	for _, l := range lines {
		if strings.TrimRight(l, " \r") == want {
			return true
		}
	}
	return false
}

func hasPrefix(lines []string, prefix string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			return true
		}
	}
	return false
}

func vrfVNI(lines []string, name string) string {
	in := false
	for _, l := range lines {
		l = strings.TrimRight(l, " \r")
		switch {
		case l == "vrf "+name:
			in = true
		case in && l == "exit-vrf":
			return ""
		case in:
			if f := strings.Fields(l); len(f) >= 2 && f[0] == "vni" {
				return f[1]
			}
		}
	}
	return ""
}

func remaining(steps []step) error {
	if len(steps) == 0 {
		return nil
	}
	var parts []string
	for _, s := range steps {
		parts = append(parts, strings.Join(s, " / "))
	}
	return fmt.Errorf("configuration still present after teardown: %s", strings.Join(parts, "; "))
}
