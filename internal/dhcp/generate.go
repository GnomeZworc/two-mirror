package dhcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"git.g3e.fr/syonad/two/internal/metadata"
)

func GenerateConfig(c Config) (string, error) {
	if c.InterfaceIP == nil {
		return "", fmt.Errorf("interface ip is required: guests would have no route to the metadata server")
	}
	mask := fmt.Sprintf("%d.%d.%d.%d", c.Network.Mask[0], c.Network.Mask[1], c.Network.Mask[2], c.Network.Mask[3])

	var sb strings.Builder
	fmt.Fprintf(&sb, "no-resolv\n")
	if c.Listen != "" {
		fmt.Fprintf(&sb, "interface=%s\n", c.Listen)
	}
	if c.Except != "" {
		fmt.Fprintf(&sb, "except-interface=%s\n", c.Except)
	}
	fmt.Fprintf(&sb, "dhcp-range=%s,static,%s,12h\n", c.Network.IP.String(), mask)
	fmt.Fprintf(&sb, "dhcp-option=121,%s\n", strings.Join(classlessRoutes(c), ","))
	if c.DefaultGateway != nil {
		fmt.Fprintf(&sb, "dhcp-option=3,%s\n", c.DefaultGateway.String())
	} else {
		fmt.Fprintf(&sb, "dhcp-option=3\n")
	}
	fmt.Fprintf(&sb, "dhcp-option=6,%s\n", c.InterfaceIP.String())
	fmt.Fprintf(&sb, "dhcp-option=42,%s\n", c.InterfaceIP.String())
	fmt.Fprintf(&sb, "dhcp-option=26,%d\n", c.interfaceMTU())
	fmt.Fprintf(&sb, "dhcp-hostsdir=%s\n", HostsDir(c.ConfDir, c.Name))
	fmt.Fprintf(&sb, "dhcp-optsdir=%s\n", OptsDir(c.ConfDir, c.Name))

	for _, dir := range []string{c.ConfDir, HostsDir(c.ConfDir, c.Name), OptsDir(c.ConfDir, c.Name)} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("create %s: %w", dir, err)
		}
	}

	outPath := filepath.Join(c.ConfDir, c.Name+".conf")
	return outPath, os.WriteFile(outPath, []byte(sb.String()), 0644)
}

func (c Config) interfaceMTU() int {
	if c.MTU == 0 {
		return 1500
	}
	return c.MTU
}

func classlessRoutes(c Config) []string {
	nextHop := c.InterfaceIP.String()

	routes := []string{metadata.ServiceIP + "/32," + nextHop}
	if c.VPCRoute != nil {
		routes = append(routes, c.VPCRoute.String()+","+nextHop)
	}
	if c.DefaultGateway != nil {
		routes = append(routes, "0.0.0.0/0,"+c.DefaultGateway.String())
	}
	return routes
}
