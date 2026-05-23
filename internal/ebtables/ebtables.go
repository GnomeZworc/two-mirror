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
