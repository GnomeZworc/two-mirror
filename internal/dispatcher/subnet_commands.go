package dispatcher

import (
	"fmt"
	"os"
	"strconv"

	"git.g3e.fr/syonad/two/internal/subnet"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

type CreateSubnetCommand struct {
	Name      string
	VPC       string
	VxlanID   int
	LocalIP   string
	GatewayIP string
	CIDR      string
}

func (c CreateSubnetCommand) Execute(db *badger.DB) error {
	kv.AddInDB(db, "subnet/"+c.Name+"/state", "creating")
	kv.AddInDB(db, "subnet/"+c.Name+"/vpc", c.VPC)
	kv.AddInDB(db, "subnet/"+c.Name+"/vxlan_id", strconv.Itoa(c.VxlanID))
	kv.AddInDB(db, "subnet/"+c.Name+"/local_ip", c.LocalIP)
	kv.AddInDB(db, "subnet/"+c.Name+"/gateway_ip", c.GatewayIP)
	kv.AddInDB(db, "subnet/"+c.Name+"/cidr", c.CIDR)
	return subnet.CreateSubnet(db, c.Name)
}

type DeleteSubnetCommand struct {
	Name string
}

func (c DeleteSubnetCommand) Execute(db *badger.DB) error {
	kv.AddInDB(db, "subnet/"+c.Name+"/state", "deleting")
	if err := subnet.DeleteSubnet(db, c.Name); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if state, err := kv.GetFromDB(db, "subnet/"+c.Name+"/state"); err != nil {
		fmt.Println(err)
		os.Exit(1)
	} else if state == "deleted" {
		kv.DeleteInDB(db, "subnet/"+c.Name)
	}
	return nil
}
