package dispatcher

import (
	"fmt"
	"strconv"
	"time"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/subnet"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

type CreateSubnetCommand struct {
	Name      string
	VPC       string
	VxlanID   int
	IfaceType string
	GatewayIP string
	CIDR      string
}

func (c CreateSubnetCommand) Prepare(db *badger.DB, cfg *configuration.Config) error {
	if _, err := kv.GetFromDB(db, "subnet/"+c.Name+"/state"); err == nil {
		return fmt.Errorf("subnet %q already exists", c.Name)
	}
	vpcState, err := kv.GetFromDB(db, "vpc/"+c.VPC+"/state")
	if err != nil {
		return fmt.Errorf("vpc %q not found", c.VPC)
	}
	if vpcState == "deleting" || vpcState == "deleted" {
		return fmt.Errorf("vpc %q is %s", c.VPC, vpcState)
	}
	localIface, ok := cfg.Interfaces[c.IfaceType]
	if !ok {
		localIface = cfg.DefaultInterface
	}
	kv.AddInDB(db, "subnet/"+c.Name+"/state", "creating")
	kv.AddInDB(db, "subnet/"+c.Name+"/vpc", c.VPC)
	kv.AddInDB(db, "subnet/"+c.Name+"/vxlan_id", strconv.Itoa(c.VxlanID))
	kv.AddInDB(db, "subnet/"+c.Name+"/local_iface", localIface)
	kv.AddInDB(db, "subnet/"+c.Name+"/gateway_ip", c.GatewayIP)
	kv.AddInDB(db, "subnet/"+c.Name+"/cidr", c.CIDR)
	return nil
}

func (c CreateSubnetCommand) Execute(db *badger.DB, cfg *configuration.Config) error {
	timeout := time.After(time.Duration(cfg.Dispatcher.TimeoutSeconds) * time.Second)
	for {
		state, err := kv.GetFromDB(db, "vpc/"+c.VPC+"/state")
		if err != nil {
			return fmt.Errorf("vpc %q not found while waiting", c.VPC)
		}
		if state == "created" {
			break
		}
		select {
		case <-timeout:
			return fmt.Errorf("timed out waiting for vpc %q to be created", c.VPC)
		case <-time.After(time.Duration(cfg.Dispatcher.PollSeconds) * time.Second):
		}
	}
	return subnet.CreateSubnet(db, c.Name)
}

type DeleteSubnetCommand struct {
	Name string
}

func (c DeleteSubnetCommand) Prepare(db *badger.DB, _ *configuration.Config) error {
	if _, err := kv.GetFromDB(db, "subnet/"+c.Name+"/state"); err != nil {
		return fmt.Errorf("subnet %q not found", c.Name)
	}
	return kv.AddInDB(db, "subnet/"+c.Name+"/state", "deleting")
}

func (c DeleteSubnetCommand) Execute(db *badger.DB, _ *configuration.Config) error {
	if err := subnet.DeleteSubnet(db, c.Name); err != nil {
		return err
	}
	state, err := kv.GetFromDB(db, "subnet/"+c.Name+"/state")
	if err != nil {
		return err
	}
	if state == "deleted" {
		kv.DeleteInDB(db, "subnet/"+c.Name)
	}
	return nil
}
