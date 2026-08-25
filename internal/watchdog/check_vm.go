package watchdog

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/qemu"
	"git.g3e.fr/syonad/two/internal/qmp"
	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/internal/watchdog/notify"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

func tapName(tapID int) string {
	return fmt.Sprintf("tap%d", tapID)
}

func CheckVMs(db *badger.DB, cfg *configuration.Config, u unitChecker, n notify.Notifier) error {
	if cfg == nil {
		return errors.New("watchdog: configuration required to check vms")
	}

	pairs, err := kv.ListByPrefix(db, prefixVM)
	if err != nil {
		return fmt.Errorf("watchdog: listing vms: %w", err)
	}

	for _, name := range resourceNames(pairs, prefixVM) {
		st, err := state.Get(db, prefixVM+name)
		if err != nil {
			n.Notify(kindVM, name, fmt.Sprintf("state unreadable in database: %v", err))
			continue
		}
		if st != state.Running {
			continue
		}
		checkVM(db, cfg, name, u, n)
	}
	return nil
}

func checkVM(db *badger.DB, cfg *configuration.Config, name string, u unitChecker, n notify.Notifier) {
	entries, err := kv.ListByPrefix(db, prefixVM+name+"/nic/")
	if err != nil {
		n.Notify(kindVM, name, fmt.Sprintf("interfaces unreadable in database: %v", err))
		return
	}
	indexes := nicIndexes(entries, prefixVM+name+"/nic/")
	if len(indexes) == 0 {
		n.Notify(kindVM, name, "no interface in database")
		return
	}

	for _, idx := range indexes {
		prefix := fmt.Sprintf("%s%s/nic/%d/", prefixVM, name, idx)
		subnetName := entries[prefix+"subnet"]
		if subnetName == "" {
			n.Notify(kindVM, name, fmt.Sprintf("interface %d has no subnet in database", idx))
			continue
		}
		vpc, err := kv.GetFromDB(db, prefixSubnet+subnetName+"/vpc")
		if err != nil {
			n.Notify(kindVM, name, fmt.Sprintf("vpc of subnet %s unreadable in database: %v", subnetName, err))
			continue
		}
		checkVMTap(entries[prefix+"tap_id"], name, vpc, idx, n)
	}

	checkVMQemu(cfg, name, n)
	checkUnit(kindVM, name, "metadata@"+name+".service", u, n)
	checkUnit(kindVM, name, qemu.ScopeName(name), u, n)
}

// nicIndexes retourne les index d'interface présents en base, triés.
func nicIndexes(entries map[string]string, prefix string) []int {
	seen := make(map[int]bool)
	for key := range entries {
		parts := strings.Split(strings.TrimPrefix(key, prefix), "/")
		if len(parts) != 2 {
			continue
		}
		if idx, err := strconv.Atoi(parts[0]); err == nil {
			seen[idx] = true
		}
	}
	indexes := make([]int, 0, len(seen))
	for idx := range seen {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	return indexes
}

func checkVMTap(raw, name, vpc string, idx int, n notify.Notifier) {
	if raw == "" {
		n.Notify(kindVM, name, fmt.Sprintf("interface %d has no tap_id in database", idx))
		return
	}
	tapID, err := strconv.Atoi(raw)
	if err != nil {
		n.Notify(kindVM, name, fmt.Sprintf("interface %d has an invalid tap_id %q: %v", idx, raw, err))
		return
	}

	if !netns.Exist(vpc) {
		n.Notify(kindVM, name, "netns "+vpc+" missing (/var/run/netns/"+vpc+")")
		return
	}

	if err := netns.Call(vpc, func() error {
		if p := linkProblem(tapName(tapID)); p != "" {
			n.Notify(kindVM, name, p+" (in netns "+vpc+")")
		}
		return nil
	}); err != nil {
		n.Notify(kindVM, name, fmt.Sprintf("cannot enter netns %s: %v", vpc, err))
	}
}

func checkVMQemu(cfg *configuration.Config, name string, n notify.Notifier) {
	sock := filepath.Join(cfg.QEMU.QMPDir, name+".sock")
	if _, err := qmp.Send(sock, nil); err != nil {
		n.Notify(kindVM, name, fmt.Sprintf("qemu not responding on %s: %v", sock, err))
	}
}
