package dispatcher

import (
	"fmt"
	"strings"
	"time"

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
	entries, err := kv.ListByPrefix(db, "subnet/")
	if err != nil {
		return fmt.Errorf("failed to list subnets: %w", err)
	}
	for key, value := range entries {
		if !strings.HasSuffix(key, "/vpc") || value != c.Name {
			continue
		}
		subnetName := strings.Split(key, "/")[1]
		state, err := kv.GetFromDB(db, "subnet/"+subnetName+"/state")
		if err != nil || (state != "deleting" && state != "deleted") {
			return fmt.Errorf("subnet %q must be deleted before deleting vpc %q", subnetName, c.Name)
		}
	}
	return kv.AddInDB(db, "vpc/"+c.Name+"/state", "deleting")
}

func (c DeleteVPCCommand) Execute(db *badger.DB, cfg *configuration.Config) error {
	timeout := time.After(time.Duration(cfg.Dispatcher.TimeoutSeconds) * time.Second)
	for {
		entries, err := kv.ListByPrefix(db, "subnet/")
		if err != nil {
			return fmt.Errorf("failed to list subnets: %w", err)
		}
		pending := false
		for key, value := range entries {
			if strings.HasSuffix(key, "/vpc") && value == c.Name {
				subnetName := strings.Split(key, "/")[1]
				state, _ := kv.GetFromDB(db, "subnet/"+subnetName+"/state")
				if state == "deleting" {
					pending = true
					break
				}
			}
		}
		if !pending {
			break
		}
		select {
		case <-timeout:
			return fmt.Errorf("timed out waiting for subnets of vpc %q to be deleted", c.Name)
		case <-time.After(time.Duration(cfg.Dispatcher.PollSeconds) * time.Second):
		}
	}
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
