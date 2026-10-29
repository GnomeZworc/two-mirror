package nft

import (
	"fmt"
	"strings"
)

var guardChains = []natChain{
	{"in-", "filter", "input", "filter"},
	{"fwd-", "filter", "forward", "filter"},
}

func guardScript(bridge string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "add table ip %s\n", Table)
	for _, c := range guardChains {
		name := c.declare(&b, bridge)
		fmt.Fprintf(&b, "add rule ip %s %s iifname %q drop\n", Table, name, bridge)
	}
	return b.String()
}

func SetBridgeGuard(bridge string) error {
	if err := apply(guardScript(bridge)); err != nil {
		return fmt.Errorf("guard %s: %w", bridge, err)
	}
	return nil
}
