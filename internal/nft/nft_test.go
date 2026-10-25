package nft

import (
	"strings"
	"testing"
)

const listedTable = `{"nftables": [{"metainfo": {"version": "1.0.6", "release_name": "Lester Gooch #5", "json_schema_version": 1}}, {"table": {"family": "bridge", "name": "two", "handle": 1}}, {"map": {"family": "bridge", "name": "pre_ports", "table": "two", "type": "ifname", "handle": 1, "map": "verdict"}}, {"map": {"family": "bridge", "name": "fwd_ports", "table": "two", "type": "ifname", "handle": 2, "map": "verdict", "elem": [["v-t33a-i", {"jump": {"target": "fwd-v-t33a-i"}}], ["v-other-i", {"jump": {"target": "fwd-v-other-i"}}]]}}, {"map": {"family": "bridge", "name": "out_ports", "table": "two", "type": "ifname", "handle": 3, "map": "verdict", "elem": [["v-t33a-i", {"jump": {"target": "out-v-t33a-i"}}]]}}, {"chain": {"family": "bridge", "table": "two", "name": "prerouting", "handle": 4, "type": "filter", "hook": "prerouting", "prio": -300, "policy": "accept"}}, {"chain": {"family": "bridge", "table": "two", "name": "forward", "handle": 5, "type": "filter", "hook": "forward", "prio": -200, "policy": "accept"}}, {"chain": {"family": "bridge", "table": "two", "name": "output", "handle": 6, "type": "filter", "hook": "output", "prio": -200, "policy": "accept"}}, {"chain": {"family": "bridge", "table": "two", "name": "fwd-v-t33a-i", "handle": 10}}, {"chain": {"family": "bridge", "table": "two", "name": "out-v-t33a-i", "handle": 11}}, {"chain": {"family": "bridge", "table": "two", "name": "fwd-v-other-i", "handle": 12}}, {"rule": {"family": "bridge", "table": "two", "chain": "prerouting", "handle": 7, "expr": [{"vmap": {"key": {"meta": {"key": "iifname"}}, "data": "@pre_ports"}}]}}, {"rule": {"family": "bridge", "table": "two", "chain": "forward", "handle": 8, "expr": [{"vmap": {"key": {"meta": {"key": "oifname"}}, "data": "@fwd_ports"}}]}}, {"rule": {"family": "bridge", "table": "two", "chain": "output", "handle": 9, "expr": [{"vmap": {"key": {"meta": {"key": "oifname"}}, "data": "@out_ports"}}]}}]}`

const noTable = `{"nftables": [{"metainfo": {"version": "1.0.6", "release_name": "Lester Gooch #5", "json_schema_version": 1}}]}`

const listedChain = `table bridge two {
	chain fwd-v-t33a-i {
		arp operation request arp daddr ip 10.33.0.1 drop
		ip saddr 10.33.0.1 udp sport 67-68 udp dport 67-68 drop
	}
}
`

func bridgePort() Port {
	return Port{
		Name:    "v-t33a-i",
		Forward: []string{"arp operation request arp daddr ip 10.33.0.1 drop", "ip saddr 10.33.0.1 udp sport 67-68 udp dport 67-68 drop"},
		Output:  []string{"arp saddr ip 10.33.0.1 drop", "ip saddr 10.33.0.1 udp sport 67-68 udp dport 67-68 drop"},
	}
}

func mustParse(t *testing.T, data string) table {
	t.Helper()
	tb, err := parseListing([]byte(data))
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}
	return tb
}

func TestRuleTexts(t *testing.T) {
	for got, want := range map[string]string{
		DropSourceMAC("02:01:00:00:0b:5f"):             "ether saddr 02:01:00:00:0b:5f drop",
		DropARPRequestTo("10.33.0.1"):                  "arp operation request arp daddr ip 10.33.0.1 drop",
		DropARPFrom("10.33.0.1"):                       "arp saddr ip 10.33.0.1 drop",
		DropDHCPFrom("10.33.0.1"):                      "ip saddr 10.33.0.1 udp sport 67-68 udp dport 67-68 drop",
		DropSourceMACOtherThan("52:54:00:00:0a:05"):    "ether saddr != 52:54:00:00:0a:05 drop",
		DropEtherTypesOtherThanIPAndARP():              "ether type != { ip, arp, ip6 } drop",
		DropARPSenderMACOtherThan("52:54:00:00:0a:05"): "arp saddr ether != 52:54:00:00:0a:05 drop",
		DropARPSenderIPOtherThan("10.0.0.5"):           "arp saddr ip != { 0.0.0.0, 10.0.0.5 } drop",
		DropDHCPServer():                               "udp sport 67 drop",
		DropIPSourceOtherThan("10.0.0.5"):              "ip saddr != { 0.0.0.0, 10.0.0.5 } drop",
	} {
		if got != want {
			t.Errorf("%q, %q attendu", got, want)
		}
	}
}

func TestDropUnspecifiedSourceOutsideDHCP(t *testing.T) {
	want := "ip saddr 0.0.0.0 ip protocol != udp drop|ip saddr 0.0.0.0 udp dport != 67 drop"
	if got := strings.Join(DropUnspecifiedSourceOutsideDHCP(), "|"); got != want {
		t.Errorf("%q, %q attendu", got, want)
	}
}

func TestSetScriptOnAnEmptyNetns(t *testing.T) {
	want := `add table bridge two
add map bridge two pre_ports { type ifname : verdict; }
add map bridge two fwd_ports { type ifname : verdict; }
add map bridge two out_ports { type ifname : verdict; }
add chain bridge two prerouting { type filter hook prerouting priority dstnat; policy accept; }
add chain bridge two forward { type filter hook forward priority filter; policy accept; }
add chain bridge two output { type filter hook output priority filter; policy accept; }
add rule bridge two prerouting iifname vmap @pre_ports
add rule bridge two forward oifname vmap @fwd_ports
add rule bridge two output oifname vmap @out_ports
add chain bridge two fwd-v-t33a-i
flush chain bridge two fwd-v-t33a-i
add rule bridge two fwd-v-t33a-i arp operation request arp daddr ip 10.33.0.1 drop
add rule bridge two fwd-v-t33a-i ip saddr 10.33.0.1 udp sport 67-68 udp dport 67-68 drop
add element bridge two fwd_ports { "v-t33a-i" : jump fwd-v-t33a-i }
add chain bridge two out-v-t33a-i
flush chain bridge two out-v-t33a-i
add rule bridge two out-v-t33a-i arp saddr ip 10.33.0.1 drop
add rule bridge two out-v-t33a-i ip saddr 10.33.0.1 udp sport 67-68 udp dport 67-68 drop
add element bridge two out_ports { "v-t33a-i" : jump out-v-t33a-i }
`
	if got := setScript(bridgePort(), mustParse(t, noTable)); got != want {
		t.Errorf("script :\n%s\nattendu :\n%s", got, want)
	}
}

func TestSetScriptAgainKeepsDispatchAndElements(t *testing.T) {
	got := setScript(bridgePort(), mustParse(t, listedTable))
	for _, absent := range []string{"vmap", "add element", "delete"} {
		if strings.Contains(got, absent) {
			t.Errorf("%q présent alors que la table est déjà en place :\n%s", absent, got)
		}
	}
	if !strings.Contains(got, "flush chain bridge two fwd-v-t33a-i\n") {
		t.Errorf("la chaîne n'est pas vidée avant d'être réécrite :\n%s", got)
	}
}

func TestSetScriptReplacesAnElementPointingElsewhere(t *testing.T) {
	data := strings.Replace(listedTable, `["v-t33a-i", {"jump": {"target": "fwd-v-t33a-i"}}]`, `["v-t33a-i", {"jump": {"target": "fwd-v-other-i"}}]`, 1)
	got := setScript(bridgePort(), mustParse(t, data))
	want := "delete element bridge two fwd_ports { \"v-t33a-i\" }\nadd element bridge two fwd_ports { \"v-t33a-i\" : jump fwd-v-t33a-i }\n"
	if !strings.Contains(got, want) {
		t.Errorf("élément non remplacé :\n%s", got)
	}
}

func TestSetScriptRemovesAHookThePortNoLongerUses(t *testing.T) {
	p := bridgePort()
	p.Output = nil
	got := setScript(p, mustParse(t, listedTable))
	want := "delete element bridge two out_ports { \"v-t33a-i\" }\ndelete chain bridge two out-v-t33a-i\n"
	if !strings.Contains(got, want) {
		t.Errorf("crochet output non retiré :\n%s", got)
	}
	if strings.Contains(got, "pre-v-t33a-i") {
		t.Errorf("chaîne prerouting touchée alors qu'elle n'existe pas :\n%s", got)
	}
}

func TestDeleteScriptRemovesElementsBeforeChains(t *testing.T) {
	want := `delete element bridge two fwd_ports { "v-t33a-i" }
delete element bridge two out_ports { "v-t33a-i" }
delete chain bridge two fwd-v-t33a-i
delete chain bridge two out-v-t33a-i
`
	if got := deleteScript("v-t33a-i", mustParse(t, listedTable)); got != want {
		t.Errorf("script :\n%s\nattendu :\n%s", got, want)
	}
}

func TestDeleteScriptLeavesOtherPorts(t *testing.T) {
	got := deleteScript("v-t33a-i", mustParse(t, listedTable))
	if strings.Contains(got, "v-other-i") {
		t.Errorf("un autre port est touché :\n%s", got)
	}
}

func TestDeleteScriptIsEmptyForAnAbsentPort(t *testing.T) {
	for name, data := range map[string]string{"sans table": noTable, "port inconnu": listedTable} {
		if got := deleteScript("v-gone-i", mustParse(t, data)); got != "" {
			t.Errorf("%s : script %q, vide attendu", name, got)
		}
	}
}

func TestParseListing(t *testing.T) {
	tb := mustParse(t, listedTable)
	if !tb.exists {
		t.Errorf("table two non vue")
	}
	if got := tb.elements["fwd_ports"]["v-t33a-i"]; got != "fwd-v-t33a-i" {
		t.Errorf("fwd_ports[v-t33a-i] = %q", got)
	}
	if got := len(tb.elements["pre_ports"]); got != 0 {
		t.Errorf("pre_ports : %d éléments, 0 attendu", got)
	}
	for _, c := range []string{"prerouting", "forward", "output", "fwd-v-t33a-i", "out-v-t33a-i", "fwd-v-other-i"} {
		if !tb.chains[c] {
			t.Errorf("chaîne %s non vue", c)
		}
	}
	for _, h := range hooks {
		if !tb.hasDispatch(h) {
			t.Errorf("aiguillage de %s non vu : %q", h.chain, tb.dispatch[h.chain])
		}
	}
	if mustParse(t, noTable).exists {
		t.Errorf("table vue alors qu'absente")
	}
}

func TestParseListingRejectsAMalformedElement(t *testing.T) {
	data := strings.Replace(listedTable, `["v-other-i", {"jump": {"target": "fwd-v-other-i"}}]`, `"v-other-i"`, 1)
	if _, err := parseListing([]byte(data)); err == nil || !strings.Contains(err.Error(), "map fwd_ports") {
		t.Errorf("erreur = %v, erreur sur fwd_ports attendue", err)
	}
}

func TestChainRules(t *testing.T) {
	got := chainRules(listedChain)
	want := []string{"arp operation request arp daddr ip 10.33.0.1 drop", "ip saddr 10.33.0.1 udp sport 67-68 udp dport 67-68 drop"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("règles %q, %q attendues", got, want)
	}
	if got := chainRules("table bridge two {\n\tchain fwd-b {\n\t}\n}\n"); len(got) != 0 {
		t.Errorf("chaîne vide : %q", got)
	}
}

func healthyListings() map[string][]string {
	return map[string][]string{
		"fwd-v-t33a-i": chainRules(listedChain),
		"out-v-t33a-i": {"arp saddr ip 10.33.0.1 drop", "ip saddr 10.33.0.1 udp sport 67-68 udp dport 67-68 drop"},
	}
}

func TestMissingIsEmptyWhenEverythingIsInPlace(t *testing.T) {
	if got := missing(bridgePort(), mustParse(t, listedTable), healthyListings()); len(got) != 0 {
		t.Errorf("manques %q, aucun attendu", got)
	}
}

func TestMissingReportsEachDefect(t *testing.T) {
	data := strings.Replace(listedTable, `, "elem": [["v-t33a-i", {"jump": {"target": "out-v-t33a-i"}}]]`, "", 1)
	data = strings.Replace(data, `"data": "@fwd_ports"`, `"data": "@pre_ports"`, 1)
	listings := healthyListings()
	listings["fwd-v-t33a-i"] = []string{"arp operation request arp daddr ip 10.33.0.1 drop", "accept"}
	got := strings.Join(missing(bridgePort(), mustParse(t, data), listings), "\n")
	for _, want := range []string{
		"forward: oifname vmap @fwd_ports",
		`out_ports: "v-t33a-i" : jump out-v-t33a-i`,
		"fwd-v-t33a-i: ip saddr 10.33.0.1 udp sport 67-68 udp dport 67-68 drop",
		"fwd-v-t33a-i: unexpected accept",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("%q absent de :\n%s", want, got)
		}
	}
}

func TestMissingReportsEverythingWithoutTable(t *testing.T) {
	got := missing(bridgePort(), mustParse(t, noTable), nil)
	if len(got) != 8 {
		t.Errorf("%d manques, 8 attendus (2 aiguillages, 2 éléments, 4 règles) : %q", len(got), got)
	}
}
