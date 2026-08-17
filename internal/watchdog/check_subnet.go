package watchdog

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"git.g3e.fr/syonad/two/internal/dhcp"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/internal/watchdog/notify"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

const (
	modeVxlan  = "vxlan"
	modeBridge = "bridge"
)

func subnetIfaceNames(subnetName string) (hostVeth, nsVeth, bridge string, err error) {
	parts := strings.SplitN(subnetName, "-", 2)
	if len(parts) < 2 || parts[1] == "" {
		return "", "", "", fmt.Errorf("subnet name %q has no identifier after the dash, interface names cannot be derived", subnetName)
	}
	id := parts[1]
	return "v-" + id + "-e", "v-" + id + "-i", "br-" + id, nil
}

func dnsmasqName(vpc, bridge string) string {
	return vpc + "_" + bridge
}

func CheckSubnets(db *badger.DB, u unitChecker, n notify.Notifier) error {
	pairs, err := kv.ListByPrefix(db, prefixSubnet)
	if err != nil {
		return fmt.Errorf("watchdog: listing subnets: %w", err)
	}

	for _, name := range resourceNames(pairs, prefixSubnet) {
		st, err := state.Get(db, prefixSubnet+name)
		if err != nil {
			n.Notify(kindSubnet, name, fmt.Sprintf("state unreadable in database: %v", err))
			continue
		}
		if st != state.Running {
			continue
		}
		checkSubnet(db, name, u, n)
	}
	return nil
}

func checkSubnet(db *badger.DB, name string, u unitChecker, n notify.Notifier) {
	hostVeth, nsVeth, bridge, err := subnetIfaceNames(name)
	if err != nil {
		n.Notify(kindSubnet, name, err.Error())
		return
	}

	vpc, err := kv.GetFromDB(db, prefixSubnet+name+"/vpc")
	if err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("vpc unreadable in database: %v", err))
		return
	}

	mode, err := kv.GetFromDB(db, prefixSubnet+name+"/mode")
	if err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("mode unreadable in database: %v", err))
		return
	}

	if p := linkProblem(hostVeth); p != "" {
		n.Notify(kindSubnet, name, p)
	}

	switch mode {
	case modeVxlan:
		if p := linkProblem(bridge); p != "" {
			n.Notify(kindSubnet, name, p+" (host)")
		}
		checkVxlanIface(db, name, n)
	case modeBridge:
	default:
		n.Notify(kindSubnet, name, fmt.Sprintf("unknown mode %q", mode))
	}

	checkSubnetNetns(name, vpc, nsVeth, bridge, n)

	dnsName := dnsmasqName(vpc, bridge)
	conf := filepath.Join(dhcp.DefaultConfDir, dnsName+".conf")
	if _, err := os.Stat(conf); err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("dnsmasq config missing (%s): %v", conf, err))
	}

	checkUnit(kindSubnet, name, "dnsmasq@"+dnsName+".service", u, n)
}

func checkVxlanIface(db *badger.DB, name string, n notify.Notifier) {
	raw, err := kv.GetFromDB(db, prefixSubnet+name+"/vxlan_id")
	if err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("vxlan_id unreadable in database: %v", err))
		return
	}
	id, err := strconv.Atoi(raw)
	if err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("invalid vxlan_id %q: %v", raw, err))
		return
	}
	if p := linkProblem(fmt.Sprintf("vxlan-%d", id)); p != "" {
		n.Notify(kindSubnet, name, p)
	}
}

func checkSubnetNetns(name, vpc, nsVeth, bridge string, n notify.Notifier) {
	if !netns.Exist(vpc) {
		n.Notify(kindSubnet, name, "netns "+vpc+" missing (/var/run/netns/"+vpc+")")
		return
	}

	if err := netns.Call(vpc, func() error {
		for _, iface := range []string{nsVeth, bridge} {
			if p := linkProblem(iface); p != "" {
				n.Notify(kindSubnet, name, p+" (in netns "+vpc+")")
			}
		}
		return nil
	}); err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("cannot enter netns %s: %v", vpc, err))
	}
}
