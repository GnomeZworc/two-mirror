package iptables

import (
	"errors"
	"fmt"
	"os/exec"
)

const HostSvcChain = "TWO-HOSTSVC"

var hostSvcExcluded = []string{"br-public", "l3-+"}

func hostSvcJump() []string {
	return []string{"PREROUTING", "-m", "addrtype", "--dst-type", "LOCAL", "-j", HostSvcChain}
}

func hostSvcRules(target string) [][]string {
	var rules [][]string
	for _, iface := range hostSvcExcluded {
		rules = append(rules, []string{HostSvcChain, "-i", iface, "-j", "RETURN"})
	}
	for _, p := range []struct{ proto, port string }{{"udp", "53"}, {"tcp", "53"}, {"udp", "123"}} {
		rules = append(rules, []string{HostSvcChain,
			"-p", p.proto, "-m", p.proto, "--dport", p.port,
			"-j", "DNAT", "--to-destination", target,
		})
	}
	return rules
}

func nat(args ...string) *exec.Cmd {
	return exec.Command("iptables", append([]string{"-t", "nat"}, args...)...)
}

func AddHostSvcRedirect(target string) error {
	if out, err := nat("-N", HostSvcChain).CombinedOutput(); err != nil {
		return fmt.Errorf("iptables create chain %s: %w: %s", HostSvcChain, err, out)
	}
	for _, rule := range append(hostSvcRules(target), hostSvcJump()) {
		if out, err := nat(append([]string{"-A"}, rule...)...).CombinedOutput(); err != nil {
			return fmt.Errorf("iptables hostsvc redirect %v: %w: %s", rule, err, out)
		}
	}
	return nil
}

func HasHostSvcRedirect(target string) (bool, error) {
	for _, rule := range append(hostSvcRules(target), hostSvcJump()) {
		err := nat(append([]string{"-C"}, rule...)...).Run()
		var exit *exec.ExitError
		switch {
		case err == nil:
		case errors.As(err, &exit) && exit.ExitCode() == 1:
			return false, nil
		default:
			return false, fmt.Errorf("iptables check %v: %w", rule, err)
		}
	}
	return true, nil
}
