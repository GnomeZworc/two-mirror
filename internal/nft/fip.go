package nft

import (
	"fmt"
	"strings"
)

type FloatingIP struct {
	Iface   string
	Public  string
	Private string
}

var fipChains = []natChain{
	{"pre-", "filter", "prerouting", "raw"},
	{"post-", "filter", "postrouting", "srcnat"},
}

func (f FloatingIP) rule(c natChain) string {
	if c.hook == "prerouting" {
		return fmt.Sprintf("iifname %q ip daddr %s ip daddr set %s", f.Iface, f.Public, f.Private)
	}
	return fmt.Sprintf("oifname %q ip saddr %s ip saddr set %s", f.Iface, f.Private, f.Public)
}

func fipSetScript(f FloatingIP) string {
	var b strings.Builder
	fmt.Fprintf(&b, "add table ip %s\n", Table)
	for _, c := range fipChains {
		name := c.declare(&b, f.Iface)
		fmt.Fprintf(&b, "add rule ip %s %s %s\n", Table, name, f.rule(c))
	}
	return b.String()
}

func fipDeleteScript(iface string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "add table ip %s\n", Table)
	for _, c := range fipChains {
		name := c.declare(&b, iface)
		fmt.Fprintf(&b, "delete chain ip %s %s\n", Table, name)
	}
	return b.String()
}

func SetFloatingIP(f FloatingIP) error {
	if err := apply(fipSetScript(f)); err != nil {
		return fmt.Errorf("set floating ip %s: %w", f.Iface, err)
	}
	return nil
}

func DeleteFloatingIP(iface string) error {
	if err := apply(fipDeleteScript(iface)); err != nil {
		return fmt.Errorf("delete floating ip %s: %w", iface, err)
	}
	return nil
}
