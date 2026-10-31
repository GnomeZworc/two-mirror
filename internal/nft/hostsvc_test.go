package nft

import (
	"strings"
	"testing"
)

const listedHostSvcChains = `table ip two {
	chain snat-ng-t46 {
		type nat hook postrouting priority srcnat; policy accept;
	}
	chain hostsvc {
		type nat hook prerouting priority dstnat; policy accept;
	}
	chain md-t90-m1 {
		type nat hook prerouting priority dstnat; policy accept;
	}
}
`

const listedHostSvcChain = `table ip two {
	chain hostsvc {
		type nat hook prerouting priority dstnat; policy accept;
		iifname "br-public" return
		iifname "l3-*" return
		fib daddr type local udp dport 53 dnat to 169.254.169.253
		fib daddr type local tcp dport 53 dnat to 169.254.169.253
		fib daddr type local udp dport 123 dnat to 169.254.169.253
	}
}
`

func TestHostSvcScript(t *testing.T) {
	want := `add table ip two
add chain ip two hostsvc { type nat hook prerouting priority dstnat; policy accept; }
flush chain ip two hostsvc
add rule ip two hostsvc iifname "br-public" return
add rule ip two hostsvc iifname "l3-*" return
add rule ip two hostsvc fib daddr type local udp dport 53 dnat to 169.254.169.253
add rule ip two hostsvc fib daddr type local tcp dport 53 dnat to 169.254.169.253
add rule ip two hostsvc fib daddr type local udp dport 123 dnat to 169.254.169.253
`
	if got := hostSvcScript("169.254.169.253"); got != want {
		t.Errorf("script :\n%s\nattendu :\n%s", got, want)
	}
}

func TestHostSvcRules_ExclusionsComeFirst(t *testing.T) {
	rules := hostSvcRules("169.254.169.253")
	if len(rules) != 5 {
		t.Fatalf("%d règles, 2 exclusions et 3 redirections attendues : %v", len(rules), rules)
	}
	for i, iface := range []string{"br-public", "l3-*"} {
		if want := `iifname "` + iface + `" return`; rules[i] != want {
			t.Errorf("règle %d = %q, %q attendu : le segment public et le lien de la VRF n'atteignent jamais le relais", i, rules[i], want)
		}
	}
}

func TestHostSvcRules_OnlyLocalDestinations(t *testing.T) {
	for _, r := range hostSvcRules("169.254.169.253") {
		if strings.Contains(r, "dnat") && !strings.HasPrefix(r, "fib daddr type local ") {
			t.Errorf("%q : un résolveur extérieur ne doit pas être détourné", r)
		}
	}
}

func TestHasChain(t *testing.T) {
	if !hasChain(listedHostSvcChains, "hostsvc") {
		t.Error("hostsvc non trouvée")
	}
	if hasChain(listedHostSvcChains, "md-t90") {
		t.Error("md-t90 trouvée par préfixe")
	}
	if hasChain(strings.Replace(listedHostSvcChains, "table ip two", "table ip nat", 1), "hostsvc") {
		t.Error("hostsvc trouvée dans une autre table")
	}
	if hasChain("", "hostsvc") {
		t.Error("hostsvc trouvée sans table")
	}
}

func TestHostSvcListingMatches(t *testing.T) {
	want := append([]string{hostSvc.header()}, hostSvcRules("169.254.169.253")...)
	if !sameRules(chainRules(listedHostSvcChain), want) {
		t.Errorf("affichage nft %v, %v attendu", chainRules(listedHostSvcChain), want)
	}
}

func TestHostSvcListingMismatches(t *testing.T) {
	want := append([]string{hostSvc.header()}, hostSvcRules("169.254.169.253")...)
	for label, listing := range map[string]string{
		"règle 123 absente":            strings.Replace(listedHostSvcChain, "\t\tfib daddr type local udp dport 123 dnat to 169.254.169.253\n", "", 1),
		"règle 123 sans fib":           strings.Replace(listedHostSvcChain, "fib daddr type local udp dport 123", "udp dport 123", 1),
		"autre cible":                  strings.Replace(listedHostSvcChain, "53 dnat to 169.254.169.253", "53 dnat to 10.0.0.1", 1),
		"exclusion après dnat":         strings.Replace(strings.Replace(listedHostSvcChain, "\t\tiifname \"br-public\" return\n", "", 1), "dport 123 dnat to 169.254.169.253\n", "dport 123 dnat to 169.254.169.253\n\t\tiifname \"br-public\" return\n", 1),
		"chaîne sans crochet":          strings.Replace(listedHostSvcChain, "\t\ttype nat hook prerouting priority dstnat; policy accept;\n", "", 1),
		"crochet d'une autre priorité": strings.Replace(listedHostSvcChain, "priority dstnat", "priority srcnat", 1),
	} {
		if sameRules(chainRules(listing), want) {
			t.Errorf("%s : chaîne jugée conforme", label)
		}
	}
}
