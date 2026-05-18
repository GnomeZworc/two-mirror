package dhcp

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func GenerateConfig(c Config) (string, map[string]string, error) {
	mask := fmt.Sprintf("%d.%d.%d.%d", c.Network.Mask[0], c.Network.Mask[1], c.Network.Mask[2], c.Network.Mask[3])

	var sb strings.Builder
	fmt.Fprintf(&sb, "no-resolv\n")
	fmt.Fprintf(&sb, "dhcp-range=%s,static,%s,12h\n", c.Network.IP.String(), mask)
	fmt.Fprintf(&sb, "dhcp-option=3,%s\n", c.Gateway.String())
	fmt.Fprintf(&sb, "dhcp-option=6,1.1.1.1,8.8.8.8\n\n")

	entries := make(map[string]string)
	i := 0
	for ip := cloneIP(c.Network.IP); c.Network.Contains(ip); incrementIP(ip) {
		mac := fmt.Sprintf("00:22:33:%02X:%02X:%02X", (i>>16)&0xFF, (i>>8)&0xFF, i&0xFF)
		fmt.Fprintf(&sb, "dhcp-host=%s,%s\n", mac, ip)
		entries[ip.String()] = mac
		i++
	}

	outPath := filepath.Join(c.ConfDir, c.Name+".conf")
	if err := os.MkdirAll(c.ConfDir, 0755); err != nil {
		return "", nil, err
	}
	return outPath, entries, os.WriteFile(outPath, []byte(sb.String()), 0644)
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
