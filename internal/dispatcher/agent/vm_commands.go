package dispatcher

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/internal/vm"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

type VMDisk struct {
	Path string
	Dev  string
}

type VMNIC struct {
	Subnet  string
	IP      string
	Primary bool
}

type StartVMCommand struct {
	Name      string
	NICs      []VMNIC
	Disks     []VMDisk
	Memory    int
	CPUs      int
	UEFI      bool
	Password  string
	SSHKey    string
	Documents map[string]string
}

func (c StartVMCommand) Key() string { return "vm/" + c.Name }

func (c StartVMCommand) Prepare(db *badger.DB, _ *configuration.Config) error {
	if _, err := kv.GetFromDB(db, "vm/"+c.Name+"/state"); err == nil {
		return fmt.Errorf("vm %q already exists", c.Name)
	}
	if err := c.validateNICs(db); err != nil {
		return err
	}
	port, err := allocateMetadataPort(db)
	if err != nil {
		return fmt.Errorf("allocate metadata port: %w", err)
	}
	state.Set(db, c.Key(), state.Creating)
	for i, n := range c.NICs {
		prefix := fmt.Sprintf("vm/%s/nic/%d/", c.Name, i)
		if err := kv.AddInDB(db, prefix+"subnet", n.Subnet); err != nil {
			return fmt.Errorf("store nic %d subnet: %w", i, err)
		}
		if err := kv.AddInDB(db, prefix+"ip", n.IP); err != nil {
			return fmt.Errorf("store nic %d ip: %w", i, err)
		}
		if n.Primary {
			if err := kv.AddInDB(db, prefix+"primary", "true"); err != nil {
				return fmt.Errorf("store nic %d primary: %w", i, err)
			}
		}
	}
	kv.AddInDB(db, "vm/"+c.Name+"/metadata_port", strconv.Itoa(port))
	for _, d := range c.Disks {
		kv.AddInDB(db, "vm/"+c.Name+"/disk/"+d.Dev, d.Path)
	}
	kv.AddInDB(db, "vm/"+c.Name+"/memory", strconv.Itoa(c.Memory))
	kv.AddInDB(db, "vm/"+c.Name+"/cpus", strconv.Itoa(c.CPUs))
	if c.UEFI {
		kv.AddInDB(db, "vm/"+c.Name+"/uefi", "true")
	}
	if c.Password != "" {
		kv.AddInDB(db, "vm/"+c.Name+"/password", c.Password)
	}
	if c.SSHKey != "" {
		kv.AddInDB(db, "vm/"+c.Name+"/sshkey", c.SSHKey)
	}
	for doc, content := range c.Documents {
		if err := kv.AddInDB(db, "vm/"+c.Name+"/metadata/"+doc, content); err != nil {
			return fmt.Errorf("store metadata %s: %w", doc, err)
		}
	}
	return nil
}

// validateNICs vérifie qu'il y a exactement une interface primaire et que
// chaque subnet référencé est utilisable.
func (c StartVMCommand) validateNICs(db *badger.DB) error {
	if len(c.NICs) == 0 {
		return fmt.Errorf("vm %q has no interface", c.Name)
	}
	primaries := 0
	for _, n := range c.NICs {
		if n.Primary {
			primaries++
		}
		subnetState, err := state.Get(db, "subnet/"+n.Subnet)
		if err != nil {
			return fmt.Errorf("subnet %q not found", n.Subnet)
		}
		if subnetState != state.Creating && subnetState != state.Running {
			return fmt.Errorf("subnet %q is %s", n.Subnet, subnetState)
		}
	}
	if primaries != 1 {
		return fmt.Errorf("vm %q has %d primary interfaces, expected exactly one", c.Name, primaries)
	}
	return nil
}

func allocateMetadataPort(db *badger.DB) (int, error) {
	entries, err := kv.ListByPrefix(db, "vm/")
	if err != nil {
		return 0, err
	}
	used := make(map[int]struct{})
	for key, value := range entries {
		if strings.HasSuffix(key, "/metadata_port") {
			if p, err := strconv.Atoi(value); err == nil {
				used[p] = struct{}{}
			}
		}
	}
	for range 100 {
		p := rand.Intn(9000) + 1000
		if _, taken := used[p]; !taken {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free metadata port available in [1000, 9999]")
}

func (c StartVMCommand) Execute(db *badger.DB, cfg *configuration.Config) error {
	timeout := time.After(time.Duration(cfg.Dispatcher.TimeoutSeconds) * time.Second)
	for _, n := range c.NICs {
		for {
			subnetState, err := state.Get(db, "subnet/"+n.Subnet)
			if err != nil {
				return fmt.Errorf("subnet %q not found while waiting", n.Subnet)
			}
			if subnetState == state.Running {
				break
			}
			if subnetState != state.Creating {
				return fmt.Errorf("subnet %q is %s, cannot start vm %q", n.Subnet, subnetState, c.Name)
			}
			select {
			case <-timeout:
				return fmt.Errorf("timed out waiting for subnet %q to be running", n.Subnet)
			case <-time.After(time.Duration(cfg.Dispatcher.PollSeconds) * time.Second):
			}
		}
	}
	return vm.StartVM(db, c.Name, cfg)
}

type StopVMCommand struct {
	Name string
}

func (c StopVMCommand) Key() string { return "vm/" + c.Name }

func (c StopVMCommand) Prepare(db *badger.DB, _ *configuration.Config) error {
	current, err := state.Get(db, c.Key())
	if err != nil {
		return fmt.Errorf("vm %q not found", c.Name)
	}
	if !state.CanDelete(current) {
		return fmt.Errorf("vm %q cannot be stopped while %s", c.Name, current)
	}
	return state.Set(db, c.Key(), state.Deleting)
}

func (c StopVMCommand) Execute(db *badger.DB, cfg *configuration.Config) error {
	if err := vm.StopVM(db, c.Name, cfg); err != nil {
		return err
	}
	current, err := state.Get(db, c.Key())
	if err != nil {
		return err
	}
	if current == state.Deleted {
		kv.DeleteInDB(db, "vm/"+c.Name)
	}
	return nil
}
