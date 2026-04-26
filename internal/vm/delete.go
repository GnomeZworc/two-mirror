package vm

import (
	"fmt"
	"strconv"
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

	vpcName, err := kv.GetFromDB(db, "vm/"+name+"/vpc")
	if err != nil {
		return fmt.Errorf("get vpc: %w", err)
	}

	tapIDStr, err := kv.GetFromDB(db, "vm/"+name+"/tap_id")
	if err != nil {
		return fmt.Errorf("get tap_id: %w", err)
	}
	tapID, err := strconv.Atoi(tapIDStr)
	if err != nil {
		return fmt.Errorf("parse tap_id: %w", err)
	}

	vmIP, err := kv.GetFromDB(db, "vm/"+name+"/ip")
	if err != nil {
		return fmt.Errorf("get ip: %w", err)
	}

	gatewayIP, err := kv.GetFromDB(db, "vm/"+name+"/gateway_ip")
	if err != nil {
		return fmt.Errorf("get gateway_ip: %w", err)
	}

	metadataPort, err := kv.GetFromDB(db, "vm/"+name+"/metadata_port")
	if err != nil {
		return fmt.Errorf("get metadata_port: %w", err)
	}

	socketPath := fmt.Sprintf("/tmp/%s.qmp-sock", name)

	if _, err := qmp.Send(socketPath, []string{`{"execute":"system_powerdown"}`}); err != nil {
		return fmt.Errorf("qmp system_powerdown: %w", err)
	}

	// attendre l'arrêt effectif de la VM ; forcer via quit après timeout
	timeout := time.After(time.Duration(cfg.Dispatcher.TimeoutSeconds) * time.Second)
	poll := time.Duration(cfg.Dispatcher.PollSeconds) * time.Second
	stopped := false
	for !stopped {
		select {
		case <-timeout:
			qmp.Send(socketPath, []string{`{"execute":"quit"}`})
			stopped = true
		case <-time.After(poll):
			if _, err := qmp.Send(socketPath, nil); err != nil {
				stopped = true
			}
		}
	}

	if err := netns.Call(vpcName, func() error {
		return iptables.DeleteMetadataRedirect(vmIP, gatewayIP, metadataPort)
	}); err != nil {
		return fmt.Errorf("delete metadata redirect: %w", err)
	}

	if err := metadata.StopMetadata(name, db, false); err != nil {
		return fmt.Errorf("stop metadata: %w", err)
	}

	if err := netif.DeleteTap(tapID, vpcName); err != nil {
		return fmt.Errorf("delete tap: %w", err)
	}

	return kv.AddInDB(db, "vm/"+name+"/state", "stopped")
}
