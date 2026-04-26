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

func DropARPToGateway(bridge, gatewayIP string) error {
	if err := addRule("FORWARD",
		"--out-interface", bridge,
		"-p", "arp",
		"--arp-op", "Request",
		"--arp-ip-dst", gatewayIP,
		"-j", "DROP"); err != nil {
		return fmt.Errorf("ebtables arp rule: %w", err)
	}
	return nil
}

func DropDHCP(bridge string) error {
	if err := addRule("FORWARD",
		"--out-interface", bridge,
		"-p", "IPv4",
		"--ip-protocol", "udp",
		"--ip-source-port", "67:68",
		"--ip-destination-port", "67:68",
		"-j", "DROP"); err != nil {
		return fmt.Errorf("ebtables dhcp rule: %w", err)
	}
	return nil
}

func DeleteARPToGateway(bridge, gatewayIP string) error {
	return deleteRule("FORWARD",
		"--out-interface", bridge,
		"-p", "arp",
		"--arp-op", "Request",
		"--arp-ip-dst", gatewayIP,
		"-j", "DROP")
}

func DeleteDHCP(bridge string) error {
	return deleteRule("FORWARD",
		"--out-interface", bridge,
		"-p", "IPv4",
		"--ip-protocol", "udp",
		"--ip-source-port", "67:68",
		"--ip-destination-port", "67:68",
		"-j", "DROP")
}
