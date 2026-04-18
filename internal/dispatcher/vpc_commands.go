package dispatcher

import (
	"git.g3e.fr/syonad/two/internal/vpc"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

type CreateVPCCommand struct {
	Name string
}

func (c CreateVPCCommand) Execute(db *badger.DB) error {
	kv.AddInDB(db, "vpc/"+c.Name+"/state", "creating")
	return vpc.CreateVPC(db, c.Name)
}

type DeleteVPCCommand struct {
	Name string
}

func (c DeleteVPCCommand) Execute(db *badger.DB) error {
	kv.AddInDB(db, "vpc/"+c.Name+"/state", "deleting")
	if err := vpc.DeleteVPC(db, c.Name); err != nil {
		return err
	}
	state, err := kv.GetFromDB(db, "vpc/"+c.Name+"/state")
	if err != nil {
		return err
	}
	if state == "deleted" {
		kv.DeleteInDB(db, "vpc/"+c.Name)
	}
	return nil
}
