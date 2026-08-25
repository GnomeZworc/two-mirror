package vm

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/dhcp"
	"git.g3e.fr/syonad/two/internal/iptables"
	"git.g3e.fr/syonad/two/internal/metadata"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/qmp"
	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/pkg/systemd"

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
		return iptables.DeleteMetadataRedirect(nic.ip, nic.interfaceIP, d.metadataPort)
	}); err != nil {
		return fmt.Errorf("delete metadata redirect: %w", err)
	}

	if err := metadata.StopMetadata(name, cfg, false); err != nil {
		return fmt.Errorf("stop metadata: %w", err)
	}

	if err := netif.DeleteTap(nic.tapID, nic.vpcName); err != nil {
		return fmt.Errorf("delete tap: %w", err)
	}

	if err := removeDHCPReservation(nic, name); err != nil {
		return err
	}

	if d.uefi {
		varsPath := filepath.Join(cfg.QEMU.UEFIVarsDir, name+"-uefi-vars.fd")
		os.Remove(varsPath)
	}

	return state.Set(db, "vm/"+name, state.Deleted)
}

// removeDHCPReservation retire le fichier de réservation puis redémarre dnsmasq :
// un fichier ajouté dans un dhcp-hostsdir est relu à chaud, un fichier retiré ne
// l'est pas (vérifié sur dnsmasq 2.90).
func removeDHCPReservation(nic nicData, name string) error {
	confName := nic.vpcName + "_" + nic.bridge

	if err := dhcp.RemoveReservations(dhcp.DefaultConfDir, confName, name); err != nil {
		return err
	}

	svc, err := systemd.New()
	if err != nil {
		return fmt.Errorf("connect to systemd: %w", err)
	}
	defer svc.Close()

	unit := dhcp.UnitName(confName)
	status, err := svc.Status(unit)
	if err != nil || status.ActiveState != "active" {
		return nil
	}
	if err := svc.Restart(unit); err != nil {
		return fmt.Errorf("restart %s: %w", unit, err)
	}
	if status, err := svc.Status(unit); err != nil {
		return fmt.Errorf("status %s after restart: %w", unit, err)
	} else if status.ActiveState != "active" {
		return fmt.Errorf("%s is %s after restart", unit, status.ActiveState)
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
