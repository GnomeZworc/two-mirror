package dispatcher

import (
	"fmt"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/vpc"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

type CreateVPCCommand struct {
	Name string
}

func (c CreateVPCCommand) Prepare(db *badger.DB, _ *configuration.Config) error {
	if _, err := kv.GetFromDB(db, "vpc/"+c.Name+"/state"); err == nil {
		return fmt.Errorf("vpc %q already exists", c.Name)
	}
	return kv.AddInDB(db, "vpc/"+c.Name+"/state", "creating")
}

func (c CreateVPCCommand) Execute(db *badger.DB, _ *configuration.Config) error {
	return vpc.CreateVPC(db, c.Name)
}

type DeleteVPCCommand struct {
	Name string
}

func (c DeleteVPCCommand) Prepare(db *badger.DB, _ *configuration.Config) error {
	if _, err := kv.GetFromDB(db, "vpc/"+c.Name+"/state"); err != nil {
		return fmt.Errorf("vpc %q not found", c.Name)
	}
	return kv.AddInDB(db, "vpc/"+c.Name+"/state", "deleting")
}

func (c DeleteVPCCommand) Execute(db *badger.DB, _ *configuration.Config) error {
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
