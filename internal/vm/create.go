package vm

import (
	"fmt"
	"strconv"

	"git.g3e.fr/syonad/two/internal/metadata"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/internal/qemu"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
)

func StartVM(db *badger.DB, name string) error {
	state, err := kv.GetFromDB(db, "vm/"+name+"/state")
	if err != nil {
		return err
	}
	if state != "starting" {
		return nil
	}

	vpcName, err := kv.GetFromDB(db, "vm/"+name+"/vpc")
	if err != nil {
		return fmt.Errorf("get vpc: %w", err)
	}

	bridge, err := kv.GetFromDB(db, "vm/"+name+"/bridge")
	if err != nil {
		return fmt.Errorf("get bridge: %w", err)
	}

	tapIDStr, err := kv.GetFromDB(db, "vm/"+name+"/tap_id")
	if err != nil {
		return fmt.Errorf("get tap_id: %w", err)
	}
	tapID, err := strconv.Atoi(tapIDStr)
	if err != nil {
		return fmt.Errorf("parse tap_id: %w", err)
	}

	mac, err := kv.GetFromDB(db, "vm/"+name+"/mac")
	if err != nil {
		return fmt.Errorf("get mac: %w", err)
	}

	volumePath, err := kv.GetFromDB(db, "vm/"+name+"/volume_path")
	if err != nil {
		return fmt.Errorf("get volume_path: %w", err)
	}

	memoryStr, err := kv.GetFromDB(db, "vm/"+name+"/memory")
	if err != nil {
		return fmt.Errorf("get memory: %w", err)
	}
	memory, _ := strconv.Atoi(memoryStr)

	cpusStr, err := kv.GetFromDB(db, "vm/"+name+"/cpus")
	if err != nil {
		return fmt.Errorf("get cpus: %w", err)
	}
	cpus, _ := strconv.Atoi(cpusStr)

	bindIP, err := kv.GetFromDB(db, "vm/"+name+"/metadata_bind_ip")
	if err != nil {
		return fmt.Errorf("get metadata_bind_ip: %w", err)
	}

	bindPort, err := kv.GetFromDB(db, "vm/"+name+"/metadata_bind_port")
	if err != nil {
		return fmt.Errorf("get metadata_bind_port: %w", err)
	}

	password, _ := kv.GetFromDB(db, "vm/"+name+"/password")
	sshkey, _ := kv.GetFromDB(db, "vm/"+name+"/sshkey")

	if err := netif.CreateTap(tapID, bridge, vpcName); err != nil {
		return fmt.Errorf("create tap: %w", err)
	}

	if err := metadata.StartMetadata(metadata.NoCloudConfig{
		Name:     name,
		VpcName:  vpcName,
		BindIP:   bindIP,
		BindPort: bindPort,
		Password: password,
		SSHKEY:   sshkey,
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
