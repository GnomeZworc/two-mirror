package watchdog

import (
	"fmt"
	"sort"
	"strings"

	"git.g3e.fr/syonad/two/internal/netif"
)

const (
	prefixVPC    = "vpc/"
	prefixSubnet = "subnet/"
	prefixVM     = "vm/"

	kindVPC    = "vpc"
	kindSubnet = "subnet"
	kindVM     = "vm"

	stateSuffix = "/state"
)

func resourceNames(pairs map[string]string, prefix string) []string {
	names := make([]string, 0, len(pairs))
	for key := range pairs {
		name, ok := strings.CutPrefix(key, prefix)
		if !ok {
			continue
		}
		name, ok = strings.CutSuffix(name, stateSuffix)
		if !ok {
			continue
		}
		if name == "" || strings.Contains(name, "/") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func linkProblem(iface string) string {
	up, err := netif.LinkIsUp(iface)
	switch {
	case err != nil:
		return fmt.Sprintf("interface %s introuvable: %v", iface, err)
	case !up:
		return fmt.Sprintf("interface %s down", iface)
	}
	return ""
}
