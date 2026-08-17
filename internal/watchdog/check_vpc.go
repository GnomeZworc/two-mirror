package watchdog

import (
	"fmt"
	"strings"

	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/internal/watchdog/notify"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

const vpcBridge = "br-public"

func vpcIfaceNames(vpcName string) (host, ns string, err error) {
	parts := strings.SplitN(vpcName, "-", 2)
	if len(parts) < 2 || parts[1] == "" {
		return "", "", fmt.Errorf("nom de VPC %q sans identifiant après le tiret, interfaces indéductibles", vpcName)
	}
	return "vp-" + parts[1] + "-e", "vp-" + parts[1] + "-i", nil
}

func CheckVPCs(db *badger.DB, n notify.Notifier) error {
	pairs, err := kv.ListByPrefix(db, prefixVPC)
	if err != nil {
		return fmt.Errorf("watchdog: lecture des vpc: %w", err)
	}

	for _, name := range resourceNames(pairs, prefixVPC) {
		st, err := state.Get(db, prefixVPC+name)
		if err != nil {
			n.Notify(kindVPC, name, fmt.Sprintf("état illisible en base: %v", err))
			continue
		}
		if st != state.Running {
			continue
		}
		checkVPC(name, n)
	}
	return nil
}

func checkVPC(name string, n notify.Notifier) {
	hostVeth, nsVeth, err := vpcIfaceNames(name)
	if err != nil {
		n.Notify(kindVPC, name, err.Error())
		return
	}

	if !netns.Exist(name) {
		n.Notify(kindVPC, name, "netns absent (/var/run/netns/"+name+")")
		return
	}

	if p := linkProblem(hostVeth); p != "" {
		n.Notify(kindVPC, name, p)
	}

	if err := netns.Call(name, func() error {
		for _, iface := range []string{nsVeth, vpcBridge} {
			if p := linkProblem(iface); p != "" {
				n.Notify(kindVPC, name, p+" (dans le netns)")
			}
		}
		return nil
	}); err != nil {
		n.Notify(kindVPC, name, fmt.Sprintf("entrée dans le netns impossible: %v", err))
	}
}
