package dispatcher

import (
	"fmt"
	"strconv"
	"time"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/internal/subnet"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

type CreateSubnetCommand struct {
	Name         string
	VPC          string
	Mode         string
	VxlanID      int
	IfaceType    string
	InterfaceIP  string
	CIDR         string
	DefaultRoute bool
	Gateway      string
}

func (c CreateSubnetCommand) Key() string { return "subnet/" + c.Name }

func (c CreateSubnetCommand) Prepare(db *badger.DB, cfg *configuration.Config) error {
	if c.Mode == "" {
		c.Mode = subnet.ModeVxlan
	}
	if !subnet.ValidMode(c.Mode) {
		return fmt.Errorf("unknown subnet mode %q", c.Mode)
	}
	if _, err := kv.GetFromDB(db, "subnet/"+c.Name+"/state"); err == nil {
		return fmt.Errorf("subnet %q already exists", c.Name)
	}
	vpcState, err := state.Get(db, "vpc/"+c.VPC)
	if err != nil {
		return fmt.Errorf("vpc %q not found", c.VPC)
	}
	if vpcState != state.Creating && vpcState != state.Running {
		return fmt.Errorf("vpc %q is %s", c.VPC, vpcState)
	}
	localIface, ok := cfg.Interfaces[c.IfaceType]
	if !ok {
		localIface = cfg.DefaultInterface
	}
	state.Set(db, c.Key(), state.Creating)
	kv.AddInDB(db, "subnet/"+c.Name+"/vpc", c.VPC)
	kv.AddInDB(db, "subnet/"+c.Name+"/mode", c.Mode)
	kv.AddInDB(db, "subnet/"+c.Name+"/local_iface", localIface)
	kv.AddInDB(db, "subnet/"+c.Name+"/interface_ip", c.InterfaceIP)
	kv.AddInDB(db, "subnet/"+c.Name+"/cidr", c.CIDR)
	kv.AddInDB(db, "subnet/"+c.Name+"/default_route", strconv.FormatBool(c.DefaultRoute))
	if c.Mode == subnet.ModeVxlan {
		kv.AddInDB(db, "subnet/"+c.Name+"/vxlan_id", strconv.Itoa(c.VxlanID))
	}
	if c.Gateway != "" {
		if err := kv.AddInDB(db, "subnet/"+c.Name+"/gateway", c.Gateway); err != nil {
			return fmt.Errorf("store gateway: %w", err)
		}
	}
	return nil
}

func (c CreateSubnetCommand) Execute(db *badger.DB, cfg *configuration.Config) error {
	timeout := time.After(time.Duration(cfg.Dispatcher.TimeoutSeconds) * time.Second)
	for {
		vpcState, err := state.Get(db, "vpc/"+c.VPC)
		if err != nil {
			return fmt.Errorf("vpc %q not found while waiting", c.VPC)
		}
		if vpcState == state.Running {
			break
		}
		if vpcState != state.Creating {
			return fmt.Errorf("vpc %q is %s, cannot create subnet %q", c.VPC, vpcState, c.Name)
		}
		select {
		case <-timeout:
			return fmt.Errorf("timed out waiting for vpc %q to be running", c.VPC)
		case <-time.After(time.Duration(cfg.Dispatcher.PollSeconds) * time.Second):
		}
	}
	return subnet.CreateSubnet(db, c.Name)
}

type DeleteSubnetCommand struct {
	Name string
}

func (c DeleteSubnetCommand) Key() string { return "subnet/" + c.Name }

func (c DeleteSubnetCommand) Prepare(db *badger.DB, _ *configuration.Config) error {
	current, err := state.Get(db, c.Key())
	if err != nil {
		return fmt.Errorf("subnet %q not found", c.Name)
	}
	if !state.CanDelete(current) {
		return fmt.Errorf("subnet %q cannot be deleted while %s", c.Name, current)
	}
	return state.Set(db, c.Key(), state.Deleting)
}

func (c DeleteSubnetCommand) Execute(db *badger.DB, _ *configuration.Config) error {
	if err := subnet.DeleteSubnet(db, c.Name); err != nil {
		return err
	}
	current, err := state.Get(db, c.Key())
	if err != nil {
		return err
	}
	if current == state.Deleted {
		kv.DeleteInDB(db, "subnet/"+c.Name)
	}
	return nil
}
