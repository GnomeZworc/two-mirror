package dhcp

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"git.g3e.fr/syonad/two/internal/metadata"
)

func GenerateConfig(c Config) (string, map[string]string, error) {
	if c.InterfaceIP == nil {
		return "", nil, fmt.Errorf("interface ip is required: guests would have no route to the metadata server")
	}
	mask := fmt.Sprintf("%d.%d.%d.%d", c.Network.Mask[0], c.Network.Mask[1], c.Network.Mask[2], c.Network.Mask[3])

	var sb strings.Builder
	fmt.Fprintf(&sb, "no-resolv\n")
	fmt.Fprintf(&sb, "dhcp-range=%s,static,%s,12h\n", c.Network.IP.String(), mask)
	fmt.Fprintf(&sb, "dhcp-option=121,%s\n", strings.Join(classlessRoutes(c), ","))
	if c.DefaultGateway != nil {
		fmt.Fprintf(&sb, "dhcp-option=3,%s\n", c.DefaultGateway.String())
	} else {
		fmt.Fprintf(&sb, "dhcp-option=3\n")
	}
	fmt.Fprintf(&sb, "dhcp-option=6,1.1.1.1,8.8.8.8\n")
	fmt.Fprintf(&sb, "dhcp-hostsdir=%s\n", HostsDir(c.ConfDir, c.Name))
	fmt.Fprintf(&sb, "dhcp-optsdir=%s\n", OptsDir(c.ConfDir, c.Name))

	entries := make(map[string]string)
	i := 0
	for ip := cloneIP(c.Network.IP); c.Network.Contains(ip); incrementIP(ip) {
		entries[ip.String()] = fmt.Sprintf("00:22:33:%02X:%02X:%02X", (i>>16)&0xFF, (i>>8)&0xFF, i&0xFF)
		i++
	}

	for _, dir := range []string{c.ConfDir, HostsDir(c.ConfDir, c.Name), OptsDir(c.ConfDir, c.Name)} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", nil, fmt.Errorf("create %s: %w", dir, err)
		}
	}

	outPath := filepath.Join(c.ConfDir, c.Name+".conf")
	return outPath, entries, os.WriteFile(outPath, []byte(sb.String()), 0644)
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

func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] != 0 {
			break
		}
	}
}

func cloneIP(ip net.IP) net.IP {
	clone := make(net.IP, len(ip))
	copy(clone, ip)
	return clone
}
