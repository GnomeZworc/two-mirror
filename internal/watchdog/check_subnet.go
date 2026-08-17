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
		return "", "", "", fmt.Errorf("nom de subnet %q sans identifiant après le tiret, interfaces indéductibles", subnetName)
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
		return fmt.Errorf("watchdog: lecture des subnets: %w", err)
	}

	for _, name := range resourceNames(pairs, prefixSubnet) {
		st, err := state.Get(db, prefixSubnet+name)
		if err != nil {
			n.Notify(kindSubnet, name, fmt.Sprintf("état illisible en base: %v", err))
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
		n.Notify(kindSubnet, name, fmt.Sprintf("vpc illisible en base: %v", err))
		return
	}

	mode, err := kv.GetFromDB(db, prefixSubnet+name+"/mode")
	if err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("mode illisible en base: %v", err))
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
		n.Notify(kindSubnet, name, fmt.Sprintf("mode inconnu %q", mode))
	}

	checkSubnetNetns(name, vpc, nsVeth, bridge, n)

	dnsName := dnsmasqName(vpc, bridge)
	conf := filepath.Join(dhcp.DefaultConfDir, dnsName+".conf")
	if _, err := os.Stat(conf); err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("config dnsmasq absente (%s): %v", conf, err))
	}

	checkUnit(kindSubnet, name, "dnsmasq@"+dnsName+".service", u, n)
}

func checkVxlanIface(db *badger.DB, name string, n notify.Notifier) {
	raw, err := kv.GetFromDB(db, prefixSubnet+name+"/vxlan_id")
	if err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("vxlan_id illisible en base: %v", err))
		return
	}
	id, err := strconv.Atoi(raw)
	if err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("vxlan_id invalide %q: %v", raw, err))
		return
	}
	if p := linkProblem(fmt.Sprintf("vxlan-%d", id)); p != "" {
		n.Notify(kindSubnet, name, p)
	}
}

func checkSubnetNetns(name, vpc, nsVeth, bridge string, n notify.Notifier) {
	if !netns.Exist(vpc) {
		n.Notify(kindSubnet, name, "netns "+vpc+" absent (/var/run/netns/"+vpc+")")
		return
	}

	if err := netns.Call(vpc, func() error {
		for _, iface := range []string{nsVeth, bridge} {
			if p := linkProblem(iface); p != "" {
				n.Notify(kindSubnet, name, p+" (dans le netns "+vpc+")")
			}
		}
		return nil
	}); err != nil {
		n.Notify(kindSubnet, name, fmt.Sprintf("entrée dans le netns %s impossible: %v", vpc, err))
	}
}
