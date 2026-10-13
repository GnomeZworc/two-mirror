package ebtables

import (
	"fmt"
	"os/exec"
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
