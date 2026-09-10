package vm

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/dhcpbackend"
	"git.g3e.fr/syonad/two/internal/iptables"
	"git.g3e.fr/syonad/two/internal/metadata"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/qmp"
	"git.g3e.fr/syonad/two/internal/state"

	"github.com/dgraph-io/badger/v4"
)

func StopVM(db *badger.DB, name string, cfg *configuration.Config) error {
	current, err := state.Get(db, "vm/"+name)
	if err != nil {
		return err
	}
	if current != state.Deleting {
		return nil
	}

	d, err := loadVM(db, name)
	if err != nil {
		return err
	}
	nic := d.primary()

	backend, err := dhcpbackend.New(cfg)
	if err != nil {
		return err
	}

	socketPath := filepath.Join(cfg.QEMU.QMPDir, name+".sock")

	if _, err := os.Stat(socketPath); err == nil {
		// socket présent : tenter l'arrêt gracieux
		if _, err := qmp.Send(socketPath, []string{`{"execute":"system_powerdown"}`}); err == nil {
			waitQMPDead(socketPath,
				time.Duration(cfg.Dispatcher.TimeoutSeconds)*time.Second,
				time.Duration(cfg.Dispatcher.PollSeconds)*time.Second,
			)
		}
		// connexion QMP échouée : QEMU déjà mort
	}
	// socket absent ou QEMU déjà arrêté : cleanup direct

	if err := netns.Call(nic.vpcName, func() error {
		for _, n := range d.nics {
			if err := iptables.DeleteMetadataRedirect(n.ip, nic.interfaceIP, d.metadataPort); err != nil {
				return fmt.Errorf("interface %d: %w", n.index, err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("delete metadata redirect: %w", err)
	}

	if err := metadata.StopMetadata(name, cfg, false); err != nil {
		return fmt.Errorf("stop metadata: %w", err)
	}

	for _, n := range d.nics {
		if err := netif.DeleteTap(n.tapID, n.vpcName); err != nil {
			return fmt.Errorf("delete tap of interface %d: %w", n.index, err)
		}
	}

	if err := removeDHCPFiles(d, name, backend); err != nil {
		return err
	}

	if d.uefi {
		varsPath := filepath.Join(cfg.QEMU.UEFIVarsDir, name+"-uefi-vars.fd")
		os.Remove(varsPath)
	}

	return state.Set(db, "vm/"+name, state.Deleted)
}

func removeDHCPFiles(d vmData, name string, backend dhcpbackend.Backend) error {
	for _, f := range dhcpReservations(d) {
		if err := backend.DelVM(f.subnet, name, f.reservations); err != nil {
			return err
		}
	}
	return nil
}

func waitQMPDead(socketPath string, timeout, poll time.Duration) {
	timer := time.After(timeout)
	for {
		select {
		case <-timer:
			qmp.Send(socketPath, []string{`{"execute":"quit"}`})
			return
		case <-time.After(poll):
			if _, err := qmp.Send(socketPath, nil); err != nil {
				return
			}
		}
	}
}
