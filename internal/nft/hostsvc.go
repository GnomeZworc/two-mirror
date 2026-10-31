package nft

import (
	"fmt"
	"strings"
)

const HostSvcChain = "hostsvc"

var hostSvc = natChain{"", "nat", "prerouting", "dstnat"}

var hostSvcExcluded = []string{"br-public", "l3-*"}

func hostSvcRules(target string) []string {
	var rules []string
	for _, iface := range hostSvcExcluded {
		rules = append(rules, fmt.Sprintf("iifname %q return", iface))
	}
	for _, p := range []struct{ proto, port string }{{"udp", "53"}, {"tcp", "53"}, {"udp", "123"}} {
		rules = append(rules, fmt.Sprintf("fib daddr type local %s dport %s dnat to %s", p.proto, p.port, target))
	}
	return rules
}

func hostSvcScript(target string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "add table ip %s\n", Table)
	name := hostSvc.declare(&b, HostSvcChain)
	for _, r := range hostSvcRules(target) {
		fmt.Fprintf(&b, "add rule ip %s %s %s\n", Table, name, r)
	}
	return b.String()
}

func SetHostSvcRedirect(target string) error {
	if err := apply(hostSvcScript(target)); err != nil {
		return fmt.Errorf("set dns and ntp redirect: %w", err)
	}
	return nil
}

func HasHostSvcRedirect(target string) (bool, error) {
	out, err := run([]string{"list", "chains", "ip"}, "")
	if err != nil {
		return false, err
	}
	if !hasChain(string(out), HostSvcChain) {
		return false, nil
	}
	out, err = run([]string{"list", "chain", "ip", Table, HostSvcChain}, "")
	if err != nil {
		return false, err
	}
	return sameRules(chainRules(string(out)), append([]string{hostSvc.header()}, hostSvcRules(target)...)), nil
}

func hasChain(listing, chain string) bool {
	table := ""
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		switch {
		case len(fields) == 4 && fields[0] == "table" && fields[1] == "ip":
			table = fields[2]
		case len(fields) == 3 && fields[0] == "chain" && table == Table && fields[1] == chain:
			return true
		}
	}
	return false
}

func sameRules(have, want []string) bool {
	if len(have) != len(want) {
		return false
	}
	for i := range want {
		if have[i] != want[i] {
			return false
		}
	}
	return true
}
