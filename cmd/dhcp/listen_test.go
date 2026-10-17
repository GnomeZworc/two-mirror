package main

import "testing"

func TestListenInterface(t *testing.T) {
	present := func(names ...string) func(string) bool {
		return func(name string) bool {
			for _, n := range names {
				if n == name {
					return true
				}
			}
			return false
		}
	}
	cases := []struct {
		label  string
		bridge string
		exists func(string) bool
		want   string
	}{
		{"gateway anycast présente", "br-s9r1", present("br-s9r1", "gw-s9r1"), "gw-s9r1"},
		{"ancien modèle, pas de gateway", "br-s9r1", present("br-s9r1"), "br-s9r1"},
		{"gateway d'un autre subnet", "br-s9r1", present("br-s9r1", "gw-s9r2"), "br-s9r1"},
		{"bridge sans préfixe br-", "vpc1br0", present("gw-vpc1br0"), "vpc1br0"},
	}
	for _, c := range cases {
		if got := listenInterface(c.bridge, c.exists); got != c.want {
			t.Errorf("%s : %s, %s attendu", c.label, got, c.want)
		}
	}
}
