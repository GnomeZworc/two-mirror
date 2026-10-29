package dispatcher

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
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
	relayIP := ""
	if c.Mode == subnet.ModeVxlan {
		if c.InterfaceIP != "" {
			return fmt.Errorf("interface_ip is computed for vxlan subnets, do not set it")
		}
		addrs, err := subnet.AnycastAddresses(c.CIDR)
		if err != nil {
			return err
		}
		c.InterfaceIP = addrs.Gateway.String()
		relayIP = addrs.Relay.String()
	} else if c.InterfaceIP == "" {
		return fmt.Errorf("interface_ip is required for %s subnets", c.Mode)
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
	if c.Mode == subnet.ModePublicIP {
		localIface = cfg.PublicBridge()
	}
	switch c.Mode {
	case subnet.ModeVxlan:
		if err := checkUnderlay(cfg, localIface); err != nil {
			return err
		}
	case subnet.ModeBridge, subnet.ModePublicIP:
		if err := checkBridge(cfg, localIface); err != nil {
			return err
		}
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
		kv.AddInDB(db, "subnet/"+c.Name+"/relay_ip", relayIP)
	}
	if c.Gateway != "" {
		if err := kv.AddInDB(db, "subnet/"+c.Name+"/gateway", c.Gateway); err != nil {
			return fmt.Errorf("store gateway: %w", err)
		}
	}
	return nil
}

func (c CreateSubnetCommand) Execute(db *badger.DB, cfg *configuration.Config) error {
	err := waitUntil(cfg.Times.SubnetStart(), cfg.Times.Poll(), func() (bool, error) {
		vpcState, err := state.Get(db, "vpc/"+c.VPC)
		if err != nil {
			return false, fmt.Errorf("vpc %q not found while waiting", c.VPC)
		}
		if vpcState != state.Running && vpcState != state.Creating {
			return false, fmt.Errorf("vpc %q is %s, cannot create subnet %q", c.VPC, vpcState, c.Name)
		}
		return vpcState == state.Running, nil
	})
	if errors.Is(err, errWaitTimeout) {
		return fmt.Errorf("timed out waiting for vpc %q to be running", c.VPC)
	}
	if err != nil {
		return err
	}
	return subnet.CreateSubnet(db, c.Name, cfg)
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
	vms, err := vmsUsingSubnet(db, c.Name)
	if err != nil {
		return err
	}
	var blocking []string
	for _, name := range vms {
		if s, err := state.Get(db, "vm/"+name); err != nil || s != state.Deleting {
			blocking = append(blocking, name)
		}
	}
	if len(blocking) > 0 {
		return fmt.Errorf("subnet %q is still used by vms %s", c.Name, strings.Join(blocking, ", "))
	}
	return state.Set(db, c.Key(), state.Deleting)
}

func waitVMsDeleted(db *badger.DB, subnetName string, timeout, poll time.Duration) error {
	err := waitUntil(timeout, poll, func() (bool, error) {
		vms, err := vmsUsingSubnet(db, subnetName)
		if err != nil {
			return false, err
		}
		for _, name := range vms {
			s, err := state.Get(db, "vm/"+name)
			if err != nil {
				continue
			}
			if s != state.Deleting {
				return false, fmt.Errorf("vm %q is %s, subnet %q cannot be deleted", name, s, subnetName)
			}
		}
		return len(vms) == 0, nil
	})
	if errors.Is(err, errWaitTimeout) {
		return fmt.Errorf("timed out waiting for the vms of subnet %q to be deleted", subnetName)
	}
	return err
}

func vmsUsingSubnet(db *badger.DB, subnetName string) ([]string, error) {
	entries, err := kv.ListByPrefix(db, "vm/")
	if err != nil {
		return nil, fmt.Errorf("failed to list vms: %w", err)
	}
	var users []string
	for key, value := range entries {
		parts := strings.Split(key, "/")
		if len(parts) != 5 || parts[2] != "nic" || parts[4] != "subnet" || value != subnetName {
			continue
		}
		if s, err := state.Get(db, "vm/"+parts[1]); err == nil && s == state.Deleted {
			continue
		}
		users = append(users, parts[1])
	}
	sort.Strings(users)
	return users, nil
}

func (c DeleteSubnetCommand) Execute(db *badger.DB, cfg *configuration.Config) error {
	if err := waitVMsDeleted(db, c.Name, cfg.Times.SubnetStop(), cfg.Times.Poll()); err != nil {
		return err
	}
	if err := subnet.DeleteSubnet(db, c.Name, cfg); err != nil {
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
