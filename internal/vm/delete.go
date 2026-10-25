package vm

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/dhcpbackend"
	"git.g3e.fr/syonad/two/internal/iptables"
	"git.g3e.fr/syonad/two/internal/l3vni"
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

	if err := applyHostRoutes(db, d, l3vni.WithdrawRoute); err != nil {
		return err
	}

	socketPath := filepath.Join(cfg.QEMU.QMPDir, name+".sock")

	if _, err := os.Stat(socketPath); err == nil {
		// socket présent : tenter l'arrêt gracieux
		shutdownQEMU(qmp.Send, socketPath, cfg.Times.VMsStop(), cfg.Times.VMsShutdown(), cfg.Times.Poll())
	}
	// socket absent ou QEMU déjà arrêté : cleanup direct

	if err := removeQEMUSockets(cfg, name); err != nil {
		return err
	}

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

func removeQEMUSockets(cfg *configuration.Config, name string) error {
	for _, dir := range []string{cfg.QEMU.SerialDir, cfg.QEMU.MonitorDir, cfg.QEMU.QMPDir} {
		path := filepath.Join(dir, name+".sock")
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove qemu socket: %w", err)
		}
	}
	return nil
}

func removeDHCPFiles(d vmData, name string, backend dhcpbackend.Backend) error {
	for _, f := range dhcpReservations(d) {
		if err := backend.DelVM(f.subnet, name, f.reservations); err != nil {
			return err
		}
	}
	return nil
}

type qmpSender func(socketPath string, commands []string) ([]json.RawMessage, error)

const (
	qmpPowerdown = `{"execute":"system_powerdown"}`
	qmpQuit      = `{"execute":"quit"}`
)

func shutdownQEMU(send qmpSender, socketPath string, timeout, resend, poll time.Duration) {
	if _, err := send(socketPath, []string{qmpPowerdown}); err != nil {
		return
	}
	deadline := time.After(timeout)
	again := time.NewTicker(resend)
	defer again.Stop()
	for {
		select {
		case <-deadline:
			send(socketPath, []string{qmpQuit})
			return
		case <-again.C:
			if _, err := send(socketPath, []string{qmpPowerdown}); err != nil {
				return
			}
		case <-time.After(poll):
			if _, err := send(socketPath, nil); err != nil {
				return
			}
		}
	}
}
