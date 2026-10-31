package nft

import (
	"fmt"
	"strings"

	"git.g3e.fr/syonad/two/internal/metadata"
)

var metadataChain = natChain{"md-", "nat", "prerouting", "dstnat"}

func metadataRule(ip, target string) string {
	return fmt.Sprintf("ip saddr %s ip daddr %s tcp dport 80 dnat to %s", ip, metadata.ServiceIP, target)
}

func metadataSetScript(vm string, ips []string, target string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "add table ip %s\n", Table)
	name := metadataChain.declare(&b, vm)
	for _, ip := range ips {
		fmt.Fprintf(&b, "add rule ip %s %s %s\n", Table, name, metadataRule(ip, target))
	}
	return b.String()
}

func metadataDeleteScript(vm string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "add table ip %s\n", Table)
	name := metadataChain.declare(&b, vm)
	fmt.Fprintf(&b, "delete chain ip %s %s\n", Table, name)
	return b.String()
}

func SetMetadataRedirect(vm string, ips []string, target string) error {
	if err := apply(metadataSetScript(vm, ips, target)); err != nil {
		return fmt.Errorf("set metadata redirect %s: %w", vm, err)
	}
	return nil
}

func DeleteMetadataRedirect(vm string) error {
	if err := apply(metadataDeleteScript(vm)); err != nil {
		return fmt.Errorf("delete metadata redirect %s: %w", vm, err)
	}
	return nil
}
