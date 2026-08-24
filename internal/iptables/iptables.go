package iptables

import (
	"fmt"
	"os/exec"

	"git.g3e.fr/syonad/two/internal/metadata"
)

func addRule(args ...string) error {
	return exec.Command("iptables", append([]string{"-t", "nat", "-A"}, args...)...).Run()
}

func deleteRule(args ...string) error {
	return exec.Command("iptables", append([]string{"-t", "nat", "-D"}, args...)...).Run()
}

func AddMetadataRedirect(vmIP, gatewayIP, metadataPort string) error {
	if err := addRule("PREROUTING",
		"-s", vmIP+"/32",
		"-d", metadata.ServiceIP+"/32",
		"-p", "tcp", "-m", "tcp",
		"--dport", "80",
		"-j", "DNAT",
		"--to-destination", gatewayIP+":"+metadataPort,
	); err != nil {
		return fmt.Errorf("iptables metadata redirect: %w", err)
	}
	return nil
}

func DeleteMetadataRedirect(vmIP, gatewayIP, metadataPort string) error {
	if err := deleteRule("PREROUTING",
		"-s", vmIP+"/32",
		"-d", metadata.ServiceIP+"/32",
		"-p", "tcp", "-m", "tcp",
		"--dport", "80",
		"-j", "DNAT",
		"--to-destination", gatewayIP+":"+metadataPort,
	); err != nil {
		return fmt.Errorf("iptables delete metadata redirect: %w", err)
	}
	return nil
}
