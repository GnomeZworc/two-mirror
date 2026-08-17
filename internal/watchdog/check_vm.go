package watchdog

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"

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
		return errors.New("watchdog: configuration requise pour vérifier les VMs")
	}

	pairs, err := kv.ListByPrefix(db, prefixVM)
	if err != nil {
		return fmt.Errorf("watchdog: lecture des vm: %w", err)
	}

	for _, name := range resourceNames(pairs, prefixVM) {
		st, err := state.Get(db, prefixVM+name)
		if err != nil {
			n.Notify(kindVM, name, fmt.Sprintf("état illisible en base: %v", err))
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
	subnetName, err := kv.GetFromDB(db, prefixVM+name+"/subnet")
	if err != nil {
		n.Notify(kindVM, name, fmt.Sprintf("subnet illisible en base: %v", err))
		return
	}

	vpc, err := kv.GetFromDB(db, prefixSubnet+subnetName+"/vpc")
	if err != nil {
		n.Notify(kindVM, name, fmt.Sprintf("vpc du subnet %s illisible en base: %v", subnetName, err))
		return
	}

	checkVMTap(db, name, vpc, n)
	checkVMQemu(cfg, name, n)
	checkUnit(kindVM, name, "metadata@"+name+".service", u, n)
	checkUnit(kindVM, name, qemu.ScopeName(name), u, n)
}

func checkVMTap(db *badger.DB, name, vpc string, n notify.Notifier) {
	raw, err := kv.GetFromDB(db, prefixVM+name+"/tap_id")
	if err != nil {
		n.Notify(kindVM, name, fmt.Sprintf("tap_id illisible en base: %v", err))
		return
	}
	tapID, err := strconv.Atoi(raw)
	if err != nil {
		n.Notify(kindVM, name, fmt.Sprintf("tap_id invalide %q: %v", raw, err))
		return
	}

	if !netns.Exist(vpc) {
		n.Notify(kindVM, name, "netns "+vpc+" absent (/var/run/netns/"+vpc+")")
		return
	}

	if err := netns.Call(vpc, func() error {
		if p := linkProblem(tapName(tapID)); p != "" {
			n.Notify(kindVM, name, p+" (dans le netns "+vpc+")")
		}
		return nil
	}); err != nil {
		n.Notify(kindVM, name, fmt.Sprintf("entrée dans le netns %s impossible: %v", vpc, err))
	}
}

func checkVMQemu(cfg *configuration.Config, name string, n notify.Notifier) {
	sock := filepath.Join(cfg.QEMU.QMPDir, name+".sock")
	if _, err := qmp.Send(sock, nil); err != nil {
		n.Notify(kindVM, name, fmt.Sprintf("qemu ne répond pas sur %s: %v", sock, err))
	}
}
