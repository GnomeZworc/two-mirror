package vm

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/iptables"
	"git.g3e.fr/syonad/two/internal/metadata"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/qmp"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

func StopVM(db *badger.DB, name string, cfg *configuration.Config) error {
	state, err := kv.GetFromDB(db, "vm/"+name+"/state")
	if err != nil {
		return err
	}
	if state != "stopping" {
		return nil
	}

	d, err := loadVM(db, name)
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


	if err := netns.Call(d.vpcName, func() error {
		return iptables.DeleteMetadataRedirect(d.ip, d.interfaceIP, d.metadataPort)
	}); err != nil {
		return fmt.Errorf("delete metadata redirect: %w", err)
	}

	if err := metadata.StopMetadata(name, cfg, false); err != nil {
		return fmt.Errorf("stop metadata: %w", err)
	}

	if err := netif.DeleteTap(d.tapID, d.vpcName); err != nil {
		return fmt.Errorf("delete tap: %w", err)
	}

	if d.uefi {
		varsPath := filepath.Join(cfg.QEMU.UEFIVarsDir, name+"-uefi-vars.fd")
		os.Remove(varsPath)
	}

	return kv.AddInDB(db, "vm/"+name+"/state", "stopped")
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
