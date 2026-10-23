package ebtables

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func addRule(args ...string) error {
	return exec.Command("ebtables", append([]string{"-A"}, args...)...).Run()
}

func deleteRule(args ...string) error {
	return exec.Command("ebtables", append([]string{"-D"}, args...)...).Run()
}

func DropARPToGateway(iface, ip string) error {
	if err := addRule("FORWARD",
		"--out-interface", iface,
		"-p", "arp",
		"--arp-op", "Request",
		"--arp-ip-dst", ip,
		"-j", "DROP"); err != nil {
		return fmt.Errorf("ebtables arp rule: %w", err)
	}
	return nil
}

func DropDHCP(iface, ip string) error {
	if err := addRule("FORWARD",
		"--out-interface", iface,
		"-p", "IPv4",
		"--ip-protocol", "udp",
		"--ip-source-port", "67:68",
		"--ip-destination-port", "67:68",
		"--ip-source", ip,
		"-j", "DROP"); err != nil {
		return fmt.Errorf("ebtables dhcp rule: %w", err)
	}
	return nil
}

func DeleteARPToGateway(iface, ip string) error {
	return deleteRule("FORWARD",
		"--out-interface", iface,
		"-p", "arp",
		"--arp-op", "Request",
		"--arp-ip-dst", ip,
		"-j", "DROP")
}

func DeleteDHCP(iface, ip string) error {
	return deleteRule("FORWARD",
		"--out-interface", iface,
		"-p", "IPv4",
		"--ip-protocol", "udp",
		"--ip-source-port", "67:68",
		"--ip-destination-port", "67:68",
		"--ip-source", ip,
		"-j", "DROP")
}

func confineRules(iface, mac string) [][]string {
	return [][]string{
		{"-t", "nat", "PREROUTING", "--in-interface", iface, "-s", mac, "-j", "DROP"},
		{"-t", "filter", "FORWARD", "--out-interface", iface, "-s", mac, "-j", "DROP"},
		{"-t", "filter", "OUTPUT", "--out-interface", iface, "-s", mac, "-j", "DROP"},
	}
}

func withAction(action string, rule []string) []string {
	return append([]string{rule[0], rule[1], action}, rule[2:]...)
}

func ConfineMAC(iface, mac string) error {
	for _, rule := range confineRules(iface, mac) {
		if err := exec.Command("ebtables", withAction("-A", rule)...).Run(); err != nil {
			return fmt.Errorf("ebtables confine %s on %s (%s %s): %w", mac, iface, rule[1], rule[2], err)
		}
	}
	return nil
}

func DeleteConfineMAC(iface, mac string) error {
	var first error
	for _, rule := range confineRules(iface, mac) {
		if err := exec.Command("ebtables", withAction("-D", rule)...).Run(); err != nil && first == nil {
			first = fmt.Errorf("ebtables unconfine %s on %s (%s %s): %w", mac, iface, rule[1], rule[2], err)
		}
	}
	return first
}

func gatewayRules(iface, ip string) [][]string {
	dhcp := []string{"-p", "IPv4", "--ip-protocol", "udp", "--ip-source-port", "67:68", "--ip-destination-port", "67:68", "--ip-source", ip, "-j", "DROP"}
	return [][]string{
		{"-t", "filter", "FORWARD", "--out-interface", iface, "-p", "arp", "--arp-op", "Request", "--arp-ip-dst", ip, "-j", "DROP"},
		{"-t", "filter", "OUTPUT", "--out-interface", iface, "-p", "arp", "--arp-ip-src", ip, "-j", "DROP"},
		append([]string{"-t", "filter", "FORWARD", "--out-interface", iface}, dhcp...),
		append([]string{"-t", "filter", "OUTPUT", "--out-interface", iface}, dhcp...),
	}
}

func ruleExists(rule []string) (bool, error) {
	err := exec.Command("ebtables", withAction("--check", rule)...).Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return false, nil
	default:
		return false, fmt.Errorf("ebtables --check %s: %w", strings.Join(rule, " "), err)
	}
}

func ConfineGateway(iface, ip string) error {
	for _, rule := range gatewayRules(iface, ip) {
		if err := exec.Command("ebtables", withAction("-A", rule)...).Run(); err != nil {
			return fmt.Errorf("ebtables confine gateway %s on %s (%s): %w", ip, iface, strings.Join(rule, " "), err)
		}
	}
	return nil
}

func DeleteConfineGateway(iface, ip string) error {
	var first error
	for _, rule := range gatewayRules(iface, ip) {
		exists, err := ruleExists(rule)
		if err == nil && exists {
			err = exec.Command("ebtables", withAction("-D", rule)...).Run()
		}
		if err != nil && first == nil {
			first = fmt.Errorf("ebtables unconfine gateway %s on %s (%s): %w", ip, iface, strings.Join(rule, " "), err)
		}
	}
	return first
}

func MissingGatewayRules(iface, ip string) ([]string, error) {
	var missing []string
	for _, rule := range gatewayRules(iface, ip) {
		exists, err := ruleExists(rule)
		if err != nil {
			return nil, err
		}
		if !exists {
			missing = append(missing, strings.Join(rule, " "))
		}
	}
	return missing, nil
}
