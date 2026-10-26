package iptables

import (
	"reflect"
	"strings"
	"testing"
)

func TestHostSvcJump_OnlyLocalDestinations(t *testing.T) {
	want := []string{"PREROUTING", "-m", "addrtype", "--dst-type", "LOCAL", "-j", "TWO-HOSTSVC"}
	if got := hostSvcJump(); !reflect.DeepEqual(got, want) {
		t.Errorf("jump = %v, want %v: traffic to an outside resolver must not be hijacked", got, want)
	}
}

func TestHostSvcRules_ExclusionsComeFirst(t *testing.T) {
	rules := hostSvcRules("169.254.169.253")
	if len(rules) != 5 {
		t.Fatalf("%d rules, want 2 exclusions and 3 redirections: %v", len(rules), rules)
	}
	for i, iface := range []string{"br-public", "l3-+"} {
		want := []string{"TWO-HOSTSVC", "-i", iface, "-j", "RETURN"}
		if !reflect.DeepEqual(rules[i], want) {
			t.Errorf("rule %d = %v, want %v: the public segment and the vrf link must never reach the relay", i, rules[i], want)
		}
	}
}

func TestHostSvcRules_RedirectDNSAndNTPOnly(t *testing.T) {
	var got []string
	for _, rule := range hostSvcRules("169.254.169.253") {
		if rule[len(rule)-3] != "DNAT" {
			continue
		}
		if rule[len(rule)-1] != "169.254.169.253" {
			t.Errorf("rule %v does not target 169.254.169.253", rule)
		}
		got = append(got, strings.Join(rule[1:7], " "))
	}
	want := []string{
		"-p udp -m udp --dport 53",
		"-p tcp -m tcp --dport 53",
		"-p udp -m udp --dport 123",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("redirections = %v, want %v", got, want)
	}
}
