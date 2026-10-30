package nft

import (
	"fmt"
	"strings"
)

type NATGateway struct {
	Iface string
	IP    string
}

type natChain struct {
	prefix   string
	kind     string
	hook     string
	priority string
}

var natChains = []natChain{
	{"snat-", "nat", "postrouting", "srcnat"},
	{"fwd-", "filter", "forward", "filter"},
	{"in-", "filter", "input", "filter"},
}

func (c natChain) declare(b *strings.Builder, iface string) string {
	name := c.prefix + iface
	fmt.Fprintf(b, "add chain ip %s %s { type %s hook %s priority %s; policy accept; }\n", Table, name, c.kind, c.hook, c.priority)
	fmt.Fprintf(b, "flush chain ip %s %s\n", Table, name)
	return name
}

func (g NATGateway) rules(c natChain) []string {
	if c.kind == "nat" {
		return []string{fmt.Sprintf("oifname %q snat to %s", g.Iface, g.IP)}
	}
	return []string{
		fmt.Sprintf("iifname %q ct state established,related accept", g.Iface),
		fmt.Sprintf("iifname %q drop", g.Iface),
	}
}

func natSetScript(g NATGateway) string {
	var b strings.Builder
	fmt.Fprintf(&b, "add table ip %s\n", Table)
	for _, c := range natChains {
		name := c.declare(&b, g.Iface)
		for _, r := range g.rules(c) {
			fmt.Fprintf(&b, "add rule ip %s %s %s\n", Table, name, r)
		}
	}
	return b.String()
}

func natDeleteScript(iface string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "add table ip %s\n", Table)
	for _, c := range natChains {
		name := c.declare(&b, iface)
		fmt.Fprintf(&b, "delete chain ip %s %s\n", Table, name)
	}
	return b.String()
}

func SetNATGateway(g NATGateway) error {
	if err := apply(natSetScript(g)); err != nil {
		return fmt.Errorf("set nat gateway %s: %w", g.Iface, err)
	}
	return nil
}

func DeleteNATGateway(iface string) error {
	if err := apply(natDeleteScript(iface)); err != nil {
		return fmt.Errorf("delete nat gateway %s: %w", iface, err)
	}
	return nil
}
