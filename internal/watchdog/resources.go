package watchdog

import (
	"fmt"
	"sort"
	"strings"

	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/watchdog/notify"
	"git.g3e.fr/syonad/two/pkg/systemd"
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

type unitChecker interface {
	Status(unit string) (*systemd.ServiceStatus, error)
}

func checkUnit(kind, name, unit string, u unitChecker, n notify.Notifier) {
	if u == nil {
		return
	}
	st, err := u.Status(unit)
	if err != nil {
		n.Notify(kind, name, fmt.Sprintf("unit %s unreadable: %v", unit, err))
		return
	}
	if st.ActiveState != "active" {
		n.Notify(kind, name, fmt.Sprintf("unit %s %s (%s)", unit, st.ActiveState, st.SubState))
	}
}

func linkProblem(iface string) string {
	up, err := netif.LinkIsUp(iface)
	switch {
	case err != nil:
		return fmt.Sprintf("interface %s not found: %v", iface, err)
	case !up:
		return fmt.Sprintf("interface %s down", iface)
	}
	return ""
}
