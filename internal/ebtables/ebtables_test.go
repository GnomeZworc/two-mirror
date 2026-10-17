package ebtables

import (
	"strings"
	"testing"
)

func TestConfineRules(t *testing.T) {
	want := []string{
		"-t nat -A PREROUTING --in-interface v-s9r1-i -s 02:01:00:00:0b:5f -j DROP",
		"-t filter -A FORWARD --out-interface v-s9r1-i -s 02:01:00:00:0b:5f -j DROP",
		"-t filter -A OUTPUT --out-interface v-s9r1-i -s 02:01:00:00:0b:5f -j DROP",
	}
	rules := confineRules("v-s9r1-i", "02:01:00:00:0b:5f")
	if len(rules) != len(want) {
		t.Fatalf("%d règles, %d attendues", len(rules), len(want))
	}
	for i, rule := range rules {
		if got := strings.Join(withAction("-A", rule), " "); got != want[i] {
			t.Errorf("règle %d : %q, %q attendu", i, got, want[i])
		}
	}
}

func TestDeleteUsesTheSameRules(t *testing.T) {
	for _, rule := range confineRules("v-x-i", "02:01:00:00:00:01") {
		added := withAction("-A", rule)
		deleted := withAction("-D", rule)
		if strings.Replace(strings.Join(added, " "), " -A ", " -D ", 1) != strings.Join(deleted, " ") {
			t.Errorf("ajout %v et suppression %v divergent", added, deleted)
		}
	}
}

func TestWithActionDoesNotAlterTheRule(t *testing.T) {
	rule := confineRules("v-x-i", "02:01:00:00:00:01")[0]
	before := strings.Join(rule, " ")
	withAction("-A", rule)
	withAction("-D", rule)
	if strings.Join(rule, " ") != before {
		t.Errorf("la règle a été modifiée : %q devenu %q", before, strings.Join(rule, " "))
	}
}
