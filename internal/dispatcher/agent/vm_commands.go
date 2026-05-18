package dispatcher

import (
	"fmt"
	"strconv"
	"time"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/vm"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

type StartVMCommand struct {
	Name         string
	Subnet       string
	IP           string
	MetadataPort string
	VolumePath   string
	Memory       int
	CPUs         int
	Password     string
	SSHKey       string
}

func (c StartVMCommand) Prepare(db *badger.DB, _ *configuration.Config) error {
	if _, err := kv.GetFromDB(db, "vm/"+c.Name+"/state"); err == nil {
		return fmt.Errorf("vm %q already exists", c.Name)
	}
	subnetState, err := kv.GetFromDB(db, "subnet/"+c.Subnet+"/state")
	if err != nil {
		return fmt.Errorf("subnet %q not found", c.Subnet)
	}
	if subnetState == "deleting" || subnetState == "deleted" {
		return fmt.Errorf("subnet %q is %s", c.Subnet, subnetState)
	}
	kv.AddInDB(db, "vm/"+c.Name+"/state", "starting")
	kv.AddInDB(db, "vm/"+c.Name+"/subnet", c.Subnet)
	kv.AddInDB(db, "vm/"+c.Name+"/ip", c.IP)
	kv.AddInDB(db, "vm/"+c.Name+"/metadata_port", c.MetadataPort)
	kv.AddInDB(db, "vm/"+c.Name+"/volume_path", c.VolumePath)
	kv.AddInDB(db, "vm/"+c.Name+"/memory", strconv.Itoa(c.Memory))
	kv.AddInDB(db, "vm/"+c.Name+"/cpus", strconv.Itoa(c.CPUs))
	if c.Password != "" {
		kv.AddInDB(db, "vm/"+c.Name+"/password", c.Password)
	}
	if c.SSHKey != "" {
		kv.AddInDB(db, "vm/"+c.Name+"/sshkey", c.SSHKey)
	}
	return nil
}

func (c StartVMCommand) Execute(db *badger.DB, cfg *configuration.Config) error {
	timeout := time.After(time.Duration(cfg.Dispatcher.TimeoutSeconds) * time.Second)
	for {
		state, err := kv.GetFromDB(db, "subnet/"+c.Subnet+"/state")
		if err != nil {
			return fmt.Errorf("subnet %q not found while waiting", c.Subnet)
		}
		if state == "created" {
			break
		}
		select {
		case <-timeout:
			return fmt.Errorf("timed out waiting for subnet %q to be created", c.Subnet)
		case <-time.After(time.Duration(cfg.Dispatcher.PollSeconds) * time.Second):
		}
	}
	return vm.StartVM(db, c.Name, cfg)
}

type StopVMCommand struct {
	Name string
}

func (c StopVMCommand) Prepare(db *badger.DB, _ *configuration.Config) error {
	if _, err := kv.GetFromDB(db, "vm/"+c.Name+"/state"); err != nil {
		return fmt.Errorf("vm %q not found", c.Name)
	}
	return kv.AddInDB(db, "vm/"+c.Name+"/state", "stopping")
}

func (c StopVMCommand) Execute(db *badger.DB, cfg *configuration.Config) error {
	if err := vm.StopVM(db, c.Name, cfg); err != nil {
		return err
	}
	state, err := kv.GetFromDB(db, "vm/"+c.Name+"/state")
	if err != nil {
		return err
	}
	if state == "stopped" {
		kv.DeleteInDB(db, "vm/"+c.Name)
	}
	return nil
}
