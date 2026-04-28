package vm

import (
	"fmt"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/iptables"
	"git.g3e.fr/syonad/two/internal/metadata"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/qemu"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

func StartVM(db *badger.DB, name string, cfg *configuration.Config) error {
	state, err := kv.GetFromDB(db, "vm/"+name+"/state")
	if err != nil {
		return err
	}
	if state != "starting" {
		return nil
	}

	d, err := loadVM(db, name)
	if err != nil {
		return err
	}

	if err := netif.CreateTap(d.tapID, d.bridge, d.vpcName); err != nil {
		return fmt.Errorf("create tap: %w", err)
	}

	if err := netns.Call(d.vpcName, func() error {
		return iptables.AddMetadataRedirect(d.ip, d.gatewayIP, d.metadataPort)
	}); err != nil {
		return fmt.Errorf("add metadata redirect: %w", err)
	}

	if err := metadata.StartMetadata(metadata.NoCloudConfig{
		Name:     name,
		VpcName:  d.vpcName,
		BindIP:   d.gatewayIP,
		BindPort: d.metadataPort,
		Password: d.password,
		SSHKEY:   d.sshkey,
	}, db, false); err != nil {
		return fmt.Errorf("start metadata: %w", err)
	}

	if err := netns.Call(d.vpcName, func() error {
		return qemu.Start(qemu.Config{
			Name:       name,
			TapID:      d.tapID,
			Mac:        d.mac,
			VolumePath: d.volumePath,
			Memory:     d.memory,
			CPUs:       d.cpus,
		})
	}); err != nil {
		return fmt.Errorf("start qemu: %w", err)
	}

	return kv.AddInDB(db, "vm/"+name+"/state", "started")
}
