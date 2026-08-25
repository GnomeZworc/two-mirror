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

	if err := removeDHCPFiles(d, name); err != nil {
		return err
	}

	if d.uefi {
		varsPath := filepath.Join(cfg.QEMU.UEFIVarsDir, name+"-uefi-vars.fd")
		os.Remove(varsPath)
	}

	return state.Set(db, "vm/"+name, state.Deleted)
}

// removeDHCPFiles retire les fichiers de la VM dans chaque subnet qu'elle
// touche, puis redémarre les dnsmasq concernés : un fichier ajouté dans un
// dhcp-hostsdir est relu à chaud, un fichier retiré ne l'est pas (vérifié sur
// dnsmasq 2.90).
func removeDHCPFiles(d vmData, name string) error {
	seen := make(map[string]bool)
	for _, n := range d.nics {
		confName := n.vpcName + "_" + n.bridge
		if seen[confName] {
			continue
		}
		seen[confName] = true
		if err := removeDHCPReservation(confName, name); err != nil {
			return err
		}
	}
	return nil
}

func removeDHCPReservation(confName, name string) error {
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
